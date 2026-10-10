package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/review"
	"github.com/achandrapaul/digest/pkg/sourcecontrol"
	tea "github.com/charmbracelet/bubbletea"
)

func mixedGitDetailsModel(t *testing.T) Model {
	t.Helper()
	m := gitDetailsModel(t, 0)
	m.git.gitPopupRepo.Items = []GitPRItem{
		{Title: "reviewed", Kind: "Reviewed", URL: "https://github.com/o/console/pull/1"},
		{Title: "assigned", Kind: "Assigned", URL: "https://github.com/o/console/pull/2"},
		{Title: "commit", Kind: "Commit", URL: "https://github.com/o/console/commit/abc"},
	}
	return m
}

func TestGitDetailsFilterTabsShowOneKindEach(t *testing.T) {
	m := mixedGitDetailsModel(t)
	for tab, want := range map[int]string{1: "reviewed", 2: "assigned", 3: "commit"} {
		m.git.gitPopupTab = tab
		items := m.filteredGitItems()
		if len(items) != 1 || items[0].Title != want {
			t.Fatalf("tab %d items = %+v", tab, items)
		}
	}
	m.git.gitPopupRepo = nil
	if m.filteredGitItems() != nil {
		t.Fatal("no popup repo has no items")
	}
}

func TestGitDetailsUpMovesTheCursorAndEnterOpensTheItem(t *testing.T) {
	opened := stubOpenURL(t)
	m := mixedGitDetailsModel(t)
	m = press(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = press(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = press(t, m, runes("k"))
	if m.git.gitPopupSelected != 1 {
		t.Fatalf("selected = %d, want 1", m.git.gitPopupSelected)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyUp})
	m = press(t, m, tea.KeyMsg{Type: tea.KeyUp})
	if m.git.gitPopupSelected != 0 {
		t.Fatalf("selected = %d, want 0", m.git.gitPopupSelected)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(*opened) != 1 || (*opened)[0] != "https://github.com/o/console/pull/1" {
		t.Fatalf("opened = %v", *opened)
	}
	m.git.gitPopupRepo.Items = nil
	press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(*opened) != 1 {
		t.Fatalf("an empty list should open nothing, opened = %v", *opened)
	}
}

func TestGitDetailsModalWithoutARepoFallsBackToTheDashboard(t *testing.T) {
	m := mixedGitDetailsModel(t)
	m.git.gitPopupRepo = nil
	if m.renderGitDetailsModal(80, 70) != composeDashboard(m) {
		t.Fatal("without a repo the dashboard should render")
	}
}

func TestWaitForGitSectionTurnsEachSectionIntoItsMessage(t *testing.T) {
	sections := make(chan sourcecontrol.Section, 3)
	sections <- sourcecontrol.Section{MyPRs: &sourcecontrol.MyPRsResult{FailedHosts: []string{"ghe"}}}
	sections <- sourcecontrol.Section{Day: &sourcecontrol.DayResult{Day: sourcecontrol.Today}}
	sections <- sourcecontrol.Section{Pending: &sourcecontrol.PendingResult{Items: []GitPRItem{{Title: "p"}}}}
	close(sections)
	wait := waitForGitSection(sections, 4)
	if mine, ok := wait().(gitMyPRsMsg); !ok || !mine.partOfSync || mine.generation != 4 || mine.failedHosts[0] != "ghe" {
		t.Fatalf("my PRs msg = %#v", mine)
	}
	if day, ok := wait().(gitDaySectionMsg); !ok || day.day != sourcecontrol.Today {
		t.Fatalf("day msg = %#v", day)
	}
	if pending, ok := wait().(gitPendingMsg); !ok || len(pending.pending) != 1 {
		t.Fatalf("pending msg = %#v", pending)
	}
	if wait() != nil {
		t.Fatal("a closed channel should end the wait")
	}
}

func TestSectionMessagesKeepWaitingForTheRest(t *testing.T) {
	m := syncTestModel(t)
	sections := make(chan sourcecontrol.Section)
	close(sections)
	for _, msg := range []tea.Msg{
		gitDaySectionMsg{generation: m.git.fetchGeneration, day: sourcecontrol.Today, date: m.currentDate.Format("2006-01-02"), sections: sections},
		gitMyPRsMsg{generation: m.git.fetchGeneration, partOfSync: true, sections: sections},
		gitPendingMsg{generation: m.git.fetchGeneration, sections: sections},
	} {
		_, cmd := m.Update(msg)
		if cmd == nil {
			t.Fatalf("%T should keep waiting for sections", msg)
		}
	}
}

func TestMyPRNotesMessageOutcomes(t *testing.T) {
	m := syncTestModel(t)
	_, cmd := m.Update(myPRNotesMsg{err: errors.New("search failed")})
	if cmd != nil {
		t.Fatal("an unsaved result should not reload notes")
	}
}

func TestAutoSyncTickWithGitOffDoesNothing(t *testing.T) {
	m := syncTestModel(t)
	off := false
	m.cfg.ShowGit = &off
	if _, cmd := m.Update(autoSyncTickMsg(time.Now())); cmd != nil {
		t.Fatal("no sync should start with git off")
	}
	if autoSyncTickCmd(0) != nil {
		t.Fatal("a zero interval should not tick")
	}
	if _, isTick := autoSyncTickCmd(1)().(autoSyncTickMsg); !isTick {
		t.Fatal("the tick should produce an auto sync message")
	}
}

func TestReviewSubmittedMessageReachesTheHandler(t *testing.T) {
	m := reviewTestModel(t)
	next, _ := m.Update(reviewSubmittedMsg{event: review.EventApprove})
	if next.(Model).reviewNotice != "Approved ✓" {
		t.Fatalf("notice = %q", next.(Model).reviewNotice)
	}
}

func fakeGhOnPath(t *testing.T, script string) {
	t.Helper()
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "gh"), []byte("#!/bin/sh\n"+script+"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestConfirmReviewSubmitsThroughGh(t *testing.T) {
	inputFile := filepath.Join(t.TempDir(), "input")
	fakeGhOnPath(t, `case "$*" in *pulls/7/reviews*) cat > `+inputFile+`;; esac`)
	m := reviewTestModel(t)
	m.reviewEvent = review.EventApprove
	m.mode = ViewReviewConfirm
	next, cmd := m.confirmReview(tea.KeyMsg{})
	if next.(Model).reviewNotice != "Submitting to GitHub…" || cmd == nil {
		t.Fatalf("notice %q", next.(Model).reviewNotice)
	}
	submitted, ok := cmd().(reviewSubmittedMsg)
	if !ok || submitted.err != nil || submitted.event != review.EventApprove {
		t.Fatalf("msg = %#v", submitted)
	}
	if body, _ := os.ReadFile(inputFile); !strings.Contains(string(body), `"event":"APPROVE"`) {
		t.Fatalf("gh input = %s", body)
	}
}

func TestConfirmReviewWithoutAPROrWithABadPayload(t *testing.T) {
	m := reviewTestModel(t)
	m.selected = 99
	if _, cmd := m.confirmReview(tea.KeyMsg{}); cmd != nil {
		t.Fatal("no PR means nothing to submit")
	}
	m.selected = 0
	m.reviewEvent = review.EventComment
	next, cmd := m.confirmReview(tea.KeyMsg{})
	if cmd != nil || next.(Model).reviewNotice == "" {
		t.Fatalf("a comment without picks should fail, notice %q", next.(Model).reviewNotice)
	}
}

func TestConfirmReviewRunGuardsAndStopFailures(t *testing.T) {
	m := reviewTestModel(t)
	m.reviewRunAction = reviewActionStart
	m.reviewRunTarget = review.PRRef{URL: "https://github.com/o/other/pull/1"}
	m.reviewRunReturnMode = ViewPreview
	if _, cmd := m.confirmReviewRun(tea.KeyMsg{}); cmd != nil {
		t.Fatal("a different PR should not start")
	}
	previous := stopReview
	stopReview = func(string, review.PRRef) error { return errors.New("kill failed") }
	t.Cleanup(func() { stopReview = previous })
	m.reviewRunAction = reviewActionStop
	next, _ := m.confirmReviewRun(tea.KeyMsg{})
	if next.(Model).reviewNotice == "Review stopped" {
		t.Fatal("a failed stop should not report success")
	}
}

func TestPRKeysDoNothingWithoutAPR(t *testing.T) {
	m := reviewTestModel(t)
	m.selected = 99
	for name, action := range map[string]func(tea.KeyMsg) (tea.Model, tea.Cmd){
		"start":   m.startReviewFromKey,
		"all":     m.selectAllFindings,
		"post":    m.postReview,
		"approve": m.approvePR,
		"reject":  m.rejectOrStopReview,
		"clone":   m.openCloneFromKey,
		"down":    m.findingDown,
	} {
		if _, cmd := action(tea.KeyMsg{}); cmd != nil {
			t.Fatalf("%s should do nothing", name)
		}
	}
	m.rejectInput.SetValue("needs work")
	if next, _ := m.submitRejectComment(tea.KeyMsg{}); next.(Model).mode != ViewPreview {
		t.Fatal("a comment without a PR returns to the preview")
	}
}

func TestPRTagCellsAreCachedPerItem(t *testing.T) {
	m := reviewTestModel(t)
	m.tagCells = &rowTagCellCache{prs: map[*GitPRItem][3]string{}}
	item := m.currentPRItem()
	size, age, state := m.prTagCells(item)
	cachedSize, cachedAge, cachedState := m.prTagCells(item)
	if size != cachedSize || age != cachedAge || state != cachedState {
		t.Fatal("cached cells should match")
	}
}

func TestFetchCommandsRunWithoutASessionContext(t *testing.T) {
	fakeGhOnPath(t, "echo offline >&2; exit 1")
	m := syncTestModel(t)
	if mine, ok := m.myPRsFetchCmd()().(gitMyPRsMsg); !ok || mine.partOfSync || mine.err == nil {
		t.Fatalf("my PRs msg = %#v", mine)
	}
	m.git.gitFetchCtx = nil
	switch msg := m.gitFetchCmd()().(type) {
	case gitDaySectionMsg, gitPendingMsg, gitMyPRsMsg:
	default:
		t.Fatalf("the first section message = %#v", msg)
	}
}

func TestAutoSyncTickWithoutAnIntervalDoesNothing(t *testing.T) {
	m := syncTestModel(t)
	m.cfg.GitAutoSyncInterval = 0
	if _, cmd := m.Update(autoSyncTickMsg(time.Now())); cmd != nil {
		t.Fatal("no interval means no sync")
	}
}

func TestRefreshCommitsWithDailyCommitsOffAndOutsideASync(t *testing.T) {
	m := syncTestModel(t)
	off := false
	m.cfg.ShowDailyCommits = &off
	if m.refreshCommitsCmd() != nil {
		t.Fatal("daily commits off means nothing to load")
	}
	on := true
	m.cfg.ShowDailyCommits = &on
	m.git.loadingGit = false
	stubDayCommits(t)
	if m.refreshCommitsCmd() == nil || !m.git.loadingCommits {
		t.Fatal("commits should load")
	}
}

func TestScheduledDaySyncFiresAfterTheDelay(t *testing.T) {
	m := syncTestModel(t)
	cmd := m.scheduleDaySync()
	for _, msg := range collectMsgs(cmd) {
		if due, ok := msg.(daySyncDueMsg); ok && due.generation == m.git.daySyncGeneration {
			return
		}
	}
	t.Fatal("expected the day sync due message")
}
