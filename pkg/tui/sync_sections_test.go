package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"app/pkg/config"
	"app/pkg/review"
	"app/pkg/sourcecontrol"
	"app/pkg/store"

	tea "github.com/charmbracelet/bubbletea"
)

func TestMain(m *testing.M) {
	cacheDir, err := os.MkdirTemp("", "digest-cache")
	if err != nil {
		panic(err)
	}
	gitCachePath = func() string { return filepath.Join(cacheDir, "git-sync.json") }
	code := m.Run()
	os.RemoveAll(cacheDir)
	os.Exit(code)
}

func syncTestModel(t *testing.T) Model {
	t.Helper()
	cachePath := filepath.Join(t.TempDir(), "git-sync.json")
	originalPath := gitCachePath
	gitCachePath = func() string { return cachePath }
	t.Cleanup(func() { gitCachePath = originalPath })
	cfg := &config.Config{DigestRoot: t.TempDir(), GreenOnly: true}
	m := NewModel(cfg, nil)
	m.width, m.height = 120, 50
	return m
}

func reviewedItem(repo string, number int) GitPRItem {
	return GitPRItem{Title: "PR", URL: prRef(repo, number).URL, Kind: "Reviewed", Number: number, Repository: repo}
}

func collectMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var msgs []tea.Msg
		for _, inner := range batch {
			msgs = append(msgs, collectMsgs(inner)...)
		}
		return msgs
	}
	return []tea.Msg{msg}
}

func TestSectionsApplyIndependently(t *testing.T) {
	m := syncTestModel(t)
	generation := m.fetchGeneration
	m.applyGitDay(gitDaySectionMsg{generation: generation, day: gitDayYesterday, reviewed: []GitPRItem{reviewedItem("console", 1)}})
	if len(m.ghReviewedYesterday) != 1 || !m.loadingGit {
		t.Fatalf("yesterday=%d loading=%v", len(m.ghReviewedYesterday), m.loadingGit)
	}
	m.applyGitPending(gitPendingMsg{generation: generation, pending: []GitPRItem{pendingItem(1)}})
	if len(m.ghPendingPRs) != 1 || !m.loadingGit {
		t.Fatalf("pending=%d loading=%v", len(m.ghPendingPRs), m.loadingGit)
	}
	m.applyMyPRs(gitMyPRsMsg{generation: generation, partOfSync: true})
	if !m.loadingGit {
		t.Fatalf("my PRs finished the sync early")
	}
	m.applyGitDay(gitDaySectionMsg{generation: generation, day: gitDayToday, reviewed: []GitPRItem{reviewedItem("console", 2)}})
	if len(m.ghReviewedToday) != 1 || m.loadingGit {
		t.Fatalf("today=%d loading=%v", len(m.ghReviewedToday), m.loadingGit)
	}
	m.applyGitPending(gitPendingMsg{generation: generation - 1, pending: nil})
	if len(m.ghPendingPRs) != 1 {
		t.Errorf("stale generation replaced pending")
	}
}

func TestFailedSectionKeepsRowsAndShowsToast(t *testing.T) {
	m := syncTestModel(t)
	generation := m.fetchGeneration
	m.applyGitDay(gitDaySectionMsg{generation: generation, day: gitDayToday, reviewed: []GitPRItem{reviewedItem("console", 2)}})
	m.applyGitPending(gitPendingMsg{generation: generation, pending: []GitPRItem{pendingItem(1)}})

	m.startLoadGitStatsCmd(false)
	generation = m.fetchGeneration
	m.applyGitDay(gitDaySectionMsg{generation: generation, day: gitDayToday, err: errors.New("HTTP 502")})
	m.applyGitPending(gitPendingMsg{generation: generation, err: errors.New("timeout")})
	if len(m.ghReviewedToday) != 1 || len(m.ghPendingPRs) != 1 {
		t.Fatalf("failed sync wiped rows: today=%d pending=%d", len(m.ghReviewedToday), len(m.ghPendingPRs))
	}
	view := m.View()
	for _, want := range []string{"GIT SYNC FAILED", "HTTP 502", "timeout"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q", want)
		}
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if strings.Contains(m.View(), "GIT SYNC FAILED") {
		t.Errorf("esc did not dismiss the toast")
	}
	if m.mode != ViewDashboard {
		t.Errorf("mode = %v", m.mode)
	}
}

func TestSuccessfulSectionClearsItsError(t *testing.T) {
	m := syncTestModel(t)
	generation := m.fetchGeneration
	m.applyGitPending(gitPendingMsg{generation: generation, err: errors.New("timeout")})
	m.applyGitPending(gitPendingMsg{generation: generation, pending: []GitPRItem{pendingItem(1)}})
	if strings.Contains(m.View(), "GIT SYNC FAILED") {
		t.Errorf("toast stayed after the section succeeded")
	}
}

func TestDateChangeClearsFailedSection(t *testing.T) {
	m := syncTestModel(t)
	m.applyGitDay(gitDaySectionMsg{generation: m.fetchGeneration, day: gitDayToday, date: m.currentDate.Format("2006-01-02"), reviewed: []GitPRItem{reviewedItem("console", 2)}})
	m.currentDate = m.currentDate.AddDate(0, 0, -3)
	m.startLoadGitStatsCmd(false)
	m.applyGitDay(gitDaySectionMsg{generation: m.fetchGeneration, day: gitDayToday, date: m.currentDate.Format("2006-01-02"), err: errors.New("HTTP 502")})
	if len(m.ghReviewedToday) != 0 {
		t.Errorf("rows from another day kept: %d", len(m.ghReviewedToday))
	}
}

func TestBothDaySectionsCreateApprovalNotes(t *testing.T) {
	m := syncTestModel(t)
	now := time.Now()
	approvals := map[gitDay]review.ActivityPR{
		gitDayToday:     {Number: 1, Title: "Today", URL: prRef("console", 1).URL, Repository: "console", State: "APPROVED", ReviewedAt: now},
		gitDayYesterday: {Number: 2, Title: "Yesterday", URL: prRef("console", 2).URL, Repository: "console", State: "APPROVED", ReviewedAt: now.AddDate(0, 0, -1)},
	}
	for day, approval := range approvals {
		next, cmd := m.Update(gitDaySectionMsg{generation: m.fetchGeneration, day: day, reviews: []review.ActivityPR{approval}})
		m = next.(Model)
		collectMsgs(cmd)
	}
	notes, err := m.store.List()
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, note := range notes {
		ids[note.ID] = true
	}
	if !ids["console:1"] || !ids["console:2"] || len(notes) != 2 {
		t.Errorf("notes = %v", ids)
	}
}

func TestParallelApprovalNotesMakeOneNote(t *testing.T) {
	noteStore := store.New(t.TempDir())
	approval := []review.ActivityPR{{Number: 9, Title: "Same", URL: prRef("console", 9).URL, Repository: "console", State: "APPROVED", ReviewedAt: time.Now()}}
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		cmd := reviewNotesCmd(noteStore, approval)
		go func() {
			defer wg.Done()
			<-start
			cmd()
		}()
	}
	close(start)
	wg.Wait()
	notes, _ := noteStore.List()
	if len(notes) != 1 {
		t.Errorf("notes = %d, want 1", len(notes))
	}
}

func TestApproveSchedulesDelayedSync(t *testing.T) {
	m := syncTestModel(t)
	originalDelay := approvalSyncDelay
	approvalSyncDelay = time.Millisecond
	t.Cleanup(func() { approvalSyncDelay = originalDelay })
	generation := m.fetchGeneration
	next, cmd := m.handleReviewSubmitted(reviewSubmittedMsg{event: review.EventApprove})
	m = next.(Model)
	if m.fetchGeneration != generation {
		t.Fatalf("sync started immediately")
	}
	if cmd == nil {
		t.Fatal("no delayed sync scheduled")
	}
	if _, ok := cmd().(approvalSyncMsg); !ok {
		t.Fatalf("cmd did not yield approvalSyncMsg")
	}
	next, _ = m.Update(approvalSyncMsg{})
	if next.(Model).fetchGeneration != generation+1 {
		t.Errorf("approvalSyncMsg did not start a sync")
	}
}

func TestGitCacheRoundTrip(t *testing.T) {
	m := syncTestModel(t)
	pending := sourcecontrol.NewPRItem(review.QueuedPR{Ref: prRef("console", 4), Title: "Cached", CIState: "SUCCESS", Additions: 7, Approved: true}, sourcecontrol.ReReviewKind)
	today := m.currentDate.Format("2006-01-02")
	m.applyGitPending(gitPendingMsg{generation: m.fetchGeneration, pending: []GitPRItem{pending}})
	m.applyGitDay(gitDaySectionMsg{generation: m.fetchGeneration, day: gitDayToday, date: today, reviewed: []GitPRItem{reviewedItem("console", 2)}})
	m.applyCommits(commitsLoadedMsg{generation: m.commitsGeneration, today: map[string][]GitPRItem{"console": {{Title: "c1", Kind: "Commit"}}}})
	saveCacheNow(t, m)
	if _, err := os.Stat(gitCachePath()); err != nil {
		t.Fatalf("cache not written: %v", err)
	}

	fresh := NewModel(m.cfg, nil)
	if len(fresh.ghPendingPRs) != 1 || fresh.ghPendingPRs[0].PR == nil {
		t.Fatalf("pending from cache = %+v", fresh.ghPendingPRs)
	}
	pr := fresh.ghPendingPRs[0].PR
	if pr.Title != "Cached" || pr.Additions != 7 || !pr.Approved || fresh.ghPendingPRs[0].Kind != sourcecontrol.ReReviewKind {
		t.Errorf("cached PR = %+v", pr)
	}
	if len(fresh.ghReviewedToday) != 1 || len(fresh.localCommitsToday["console"]) != 1 {
		t.Errorf("today from cache: reviewed=%d commits=%d", len(fresh.ghReviewedToday), len(fresh.localCommitsToday["console"]))
	}
	if fresh.loadingGit || fresh.syncOnLoad {
		t.Errorf("today's cache should skip the startup sync")
	}
}

func TestOldGitCacheShowsOnlyPending(t *testing.T) {
	m := syncTestModel(t)
	cache := gitSyncCache{
		Date:          m.currentDate.AddDate(0, 0, -1).Format("2006-01-02"),
		ReviewedToday: []cachedGitItem{{Item: reviewedItem("console", 2)}},
		Pending:       []cachedGitItem{{Item: pendingItem(1), PR: pendingItem(1).PR}},
	}
	if err := saveGitCache(cache); err != nil {
		t.Fatal(err)
	}
	fresh := NewModel(m.cfg, nil)
	if len(fresh.ghPendingPRs) != 1 || len(fresh.ghReviewedToday) != 0 {
		t.Errorf("pending=%d reviewedToday=%d", len(fresh.ghPendingPRs), len(fresh.ghReviewedToday))
	}
}

func TestCorruptGitCacheIgnored(t *testing.T) {
	m := syncTestModel(t)
	if err := os.WriteFile(gitCachePath(), []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, ok := loadGitCache(); ok {
		t.Errorf("corrupt cache loaded")
	}
	fresh := NewModel(m.cfg, nil)
	if len(fresh.ghPendingPRs) != 0 {
		t.Errorf("pending = %d", len(fresh.ghPendingPRs))
	}
}
