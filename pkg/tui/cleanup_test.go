package tui

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/brag"
	"github.com/AnudeepChPaul/digest/pkg/model"
	"github.com/AnudeepChPaul/digest/pkg/review"
	"github.com/AnudeepChPaul/digest/pkg/sourcecontrol"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

func TestSearchReusesResultsUntilTheQueryOrNotesChange(t *testing.T) {
	lowered := 0
	original := lowerSearchText
	lowerSearchText = func(text string) string {
		lowered++
		return original(text)
	}
	t.Cleanup(func() { lowerSearchText = original })
	m := typeQuery(t, searchTestModel(t), "flaky")
	lowered = 0
	m.View()
	m = press(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = press(t, m, tea.KeyMsg{Type: tea.KeyPgDown})
	m.View()
	if lowered != 0 {
		t.Errorf("redraws and cursor moves lowered %d texts", lowered)
	}
	m.notes[0].Summary = "renamed"
	if got := resultIDs(m.searchResults()); got != "b" {
		t.Errorf("an edited note should refresh the results, got %q", got)
	}
	if lowered > 2 {
		t.Errorf("only the edited note should be lowered again, lowered %d", lowered)
	}
}

func TestReviewFindingsLoadOncePerReviewChange(t *testing.T) {
	loads := 0
	original := loadReviewReport
	loadReviewReport = func(dir string) (*review.Report, error) {
		loads++
		return original(dir)
	}
	t.Cleanup(func() { loadReviewReport = original })
	m := press(t, reviewTestModel(t), tea.KeyMsg{Type: tea.KeyTab})
	for _, key := range []tea.KeyMsg{runes("j"), runes("k"), {Type: tea.KeySpace}, {Type: tea.KeyCtrlA}} {
		m = press(t, m, key)
	}
	m.View()
	if loads != 1 {
		t.Errorf("findings loaded %d times, want 1", loads)
	}
	if m.selectedCount() != 2 || m.previewFindingsCount != 2 {
		t.Errorf("selected=%d findings=%d", m.selectedCount(), m.previewFindingsCount)
	}
}

func TestNavKeyBuildsTheDashboardOnce(t *testing.T) {
	m := selectionTestModel(t)
	m.git.ghPendingPRs = []GitPRItem{pendingItem(1), pendingItem(2), pendingItem(3)}
	m.rebuildGitRepoStats()
	builds := 0
	original := dashboardContentBuilder
	dashboardContentBuilder = func(m Model) (string, int) {
		builds++
		return original(m)
	}
	t.Cleanup(func() { dashboardContentBuilder = original })
	m = update(m, runes("j"))
	m.View()
	if builds != 1 {
		t.Errorf("a nav key built the dashboard %d times", builds)
	}
}

func TestSyncDoesNotTickAHiddenSpinner(t *testing.T) {
	m := syncTestModel(t)
	m.git.loadingGit = true
	if _, cmd := m.Update(spinner.TickMsg{}); cmd != nil {
		t.Error("a spinner that is never drawn should not tick")
	}
}

func TestNewPendingPRsReadTheirReviewStateOnce(t *testing.T) {
	m := reviewTestModel(t)
	m.mode = ViewDashboard
	reads := 0
	original := readLocalReview
	readLocalReview = func(stateDir string) localReviewState {
		reads++
		return original(stateDir)
	}
	t.Cleanup(func() { readLocalReview = original })
	fresh := []GitPRItem{m.git.ghPendingPRs[0], sourcecontrol.NewPRItem(review.QueuedPR{Ref: prRef("console", 8), Title: "New"}, "Pending Review")}
	m.applyGitPending(gitPendingMsg{generation: m.git.fetchGeneration, pending: fresh})
	m.View()
	if reads != 2 {
		t.Errorf("review states read %d times for 2 PRs", reads)
	}
}

func TestJobLogReadsOnlyItsTail(t *testing.T) {
	m := jobPreviewModel(t)
	if err := os.WriteFile(filepath.Join(getLogsDir(), "janitor.log"), []byte(strings.Repeat("x", jobLogTailBytes*2)+"\nlast line\n"), 0644); err != nil {
		t.Fatal(err)
	}
	logText := readJobLog("janitor")
	if len(logText) > jobLogTailBytes || !strings.HasSuffix(logText, "last line\n") {
		t.Errorf("read %d bytes, want the last %d", len(logText), jobLogTailBytes)
	}
	_ = m
}

func TestDryRunResultsReloadOnlyWhenTheirLogChanges(t *testing.T) {
	m := selectionTestModel(t)
	for suffix, content := range map[string]string{"exit": "0", "log": "found 2 files"} {
		if err := os.WriteFile(dryRunFilePath("janitor", suffix), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	reads := 0
	original := readDryRunLog
	readDryRunLog = func(path string) ([]byte, error) {
		reads++
		return original(path)
	}
	t.Cleanup(func() { readDryRunLog = original })
	m.refreshDryRunResults()
	m.refreshDryRunResults()
	if reads != 1 {
		t.Errorf("an unchanged dry-run log was read %d times", reads)
	}
	future := time.Now().Add(time.Minute)
	if err := os.WriteFile(dryRunFilePath("janitor", "log"), []byte("found 3 files"), 0644); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(dryRunFilePath("janitor", "log"), future, future)
	m.refreshDryRunResults()
	if reads != 2 || m.jobDryRunOutputs["janitor"] != "found 3 files" {
		t.Errorf("a changed log should reload: reads=%d output=%q", reads, m.jobDryRunOutputs["janitor"])
	}
}

func TestSortToggleSavesInTheBackground(t *testing.T) {
	writes := 0
	original := writeGitCache
	writeGitCache = func(cache gitSyncCache) error {
		writes++
		return original(cache)
	}
	t.Cleanup(func() { writeGitCache = original })
	m := syncTestModel(t)
	m.git.loadingGit = false
	next, cmd := m.Update(runes("s"))
	if writes != 0 || cmd == nil {
		t.Fatalf("the sort key wrote the cache on the UI thread (%d writes)", writes)
	}
	m = next.(Model)
	for _, msg := range collectMsgs(cmd) {
		m = update(m, msg)
	}
	cache, _ := loadGitCache()
	if writes != 1 || cache.PendingSort == nil || !cache.PendingSort.ByCreated {
		t.Errorf("writes=%d sort=%+v", writes, cache.PendingSort)
	}
}

func TestBragStatesAreReadOncePerRefresh(t *testing.T) {
	m, _ := bragTestModel(t, wednesday())
	m = press(t, m, runes("b"))
	before := stripANSI(m.View())
	failedWeek := brag.WeekOf(wednesday()).Previous()
	writeBragExit(t, m, failedWeek.ID(), "1")
	if stripANSI(m.View()) != before {
		t.Error("a redraw should not read brag state from disk")
	}
	m.refreshBragRuns()
	if !strings.Contains(stripANSI(m.View()), "last run failed") {
		t.Error("a refresh should pick up the failed run")
	}
}

func writeBragExit(t *testing.T, m Model, id, code string) {
	t.Helper()
	dir := brag.StateDir(m.cfg.BragDir(), id)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "brag.exit"), []byte(code), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestRecordedReviewNotesSkipReloadingTheStore(t *testing.T) {
	m := syncTestModel(t)
	reviewedAt := time.Now().Add(-time.Hour)
	approval := []review.ActivityPR{{Number: 4, Title: "Same", URL: prRef("console", 4).URL, Repository: "console", State: "APPROVED", ReviewedAt: reviewedAt}}
	if changed, ok := reviewNotesCmd(m.store, approval)().(notesChangedMsg); !ok || len(changed.notes) != 1 {
		t.Fatal("a new review should send back only the note it saved")
	}
	if msg := reviewNotesCmd(m.store, approval)(); msg != nil {
		t.Errorf("an already recorded review should not reload the notes, got %T", msg)
	}
}

func TestOpeningAURLReapsTheOpener(t *testing.T) {
	exited, err := startReaped(exec.Command("true"))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("the opener was never waited on")
	}
}

func TestQuitCancelsCommitLoading(t *testing.T) {
	m := syncTestModel(t)
	m.refreshCommitsCmd()
	commitsCtx := m.git.commitsCtx
	for range 3 {
		m = update(m, tea.KeyMsg{Type: tea.KeyCtrlC})
	}
	if commitsCtx.Err() == nil {
		t.Error("quitting should cancel commit loading")
	}
}

func TestOldJobLogArchivesArePruned(t *testing.T) {
	logsDir := t.TempDir()
	now := time.Now()
	old, recent, other := "janitor-20200101-000000.log", "janitor-20991231-000000.log", "notes.txt"
	for _, name := range []string{old, recent, other, "janitor.log"} {
		if err := os.WriteFile(filepath.Join(logsDir, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	stale := now.AddDate(0, 0, -30)
	for _, name := range []string{old, other} {
		_ = os.Chtimes(filepath.Join(logsDir, name), stale, stale)
	}
	pruneJobLogArchives(logsDir, "janitor", 14, now)
	for name, kept := range map[string]bool{old: false, recent: true, other: true, "janitor.log": true} {
		if _, err := os.Stat(filepath.Join(logsDir, name)); (err == nil) != kept {
			t.Errorf("%s kept=%v, want %v", name, err == nil, kept)
		}
	}
}

func TestSyncTrimsPRDetailsToListedPRs(t *testing.T) {
	m := syncTestModel(t)
	m.git.prDetails = map[string]json.RawMessage{"https://github.com/o/gone/pull/1": json.RawMessage(`{}`)}
	m.startLoadGitStatsCmd()
	generation := m.git.fetchGeneration
	listed := pendingItem(1)
	for _, msg := range []tea.Msg{
		gitDaySectionMsg{generation: generation, day: gitDayYesterday},
		gitDaySectionMsg{generation: generation, day: gitDayToday, date: m.currentDate.Format("2006-01-02")},
		gitPendingMsg{generation: generation, pending: []GitPRItem{listed}, details: map[string]json.RawMessage{listed.URL: json.RawMessage(`{}`)}},
		gitMyPRsMsg{generation: generation, partOfSync: true},
		commitsLoadedMsg{generation: m.git.commitsGeneration},
	} {
		m = update(m, msg)
	}
	if _, kept := m.git.prDetails["https://github.com/o/gone/pull/1"]; kept || len(m.git.prDetails) != 1 {
		t.Errorf("details = %v, want only the listed PR", m.git.prDetails)
	}
}

func TestStartupReusesTheNotesItAlreadyListed(t *testing.T) {
	m := syncTestModel(t)
	if err := m.store.Save(&model.Note{ID: "kept", Summary: "kept", Status: model.StatusActive}); err != nil {
		t.Fatal(err)
	}
	m = NewModel(m.cfg, nil)
	if err := os.RemoveAll(m.cfg.NotesDir()); err != nil {
		t.Fatal(err)
	}
	if loaded, ok := m.startupNotesCmd()().(loadNotesMsg); !ok || len(loaded.notes) != 1 {
		t.Errorf("startup listed the notes store again: %d notes", len(loaded.notes))
	}
}

func TestDryRunResultReadsOnlyItsTail(t *testing.T) {
	jobPreviewModel(t)
	if err := os.WriteFile(dryRunFilePath("janitor", "exit"), []byte("0"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dryRunFilePath("janitor", "log"), []byte(strings.Repeat("x", jobLogTailBytes*3)+"\nwould remove 2 clones\n"), 0600); err != nil {
		t.Fatal(err)
	}
	output, _, ok := loadDryRunResult("janitor")
	if !ok || len(output) > jobLogTailBytes || !strings.HasSuffix(output, "would remove 2 clones\n") {
		t.Errorf("ok %v, read %d bytes, want at most %d ending in the summary", ok, len(output), jobLogTailBytes)
	}
}
