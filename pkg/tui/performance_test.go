package tui

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"app/pkg/review"
	"app/pkg/sourcecontrol"

	tea "github.com/charmbracelet/bubbletea"
)

func writeLivePID(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		t.Fatal(err)
	}
}

func saveCacheNow(t *testing.T, m Model) {
	t.Helper()
	if err := writeGitCache(m.gitCacheSnapshot()); err != nil {
		t.Fatal(err)
	}
}

func TestSyncPulseRunsAsOneLoop(t *testing.T) {
	m := syncTestModel(t)
	for range 7 {
		m = update(m, runes("p"))
	}
	if !m.syncPulseRunning {
		t.Fatal("day switches should start the pulse")
	}
	if m.ensureSyncPulse() != nil {
		t.Error("a second start should not add another pulse loop")
	}
	m.cancelGitSync()
	m.loadingGit, m.loadingCommits = false, false
	next, cmd := m.Update(syncPulseTickMsg{})
	if cmd != nil || next.(Model).syncPulseRunning {
		t.Error("the pulse should stop once nothing is syncing")
	}
	stopped := next.(Model)
	if stopped.ensureSyncPulse() == nil {
		t.Error("a stopped pulse should start again")
	}
}

func jobPreviewModel(t *testing.T) Model {
	t.Helper()
	m := selectionTestModel(t)
	writeLivePID(t, filepath.Join(getLogsDir(), "janitor.pid"))
	if err := os.WriteFile(filepath.Join(getLogsDir(), "janitor.log"), []byte(strings.Repeat("line\n", 300)), 0644); err != nil {
		t.Fatal(err)
	}
	m.refreshJobStates()
	selectNavItem(t, &m, "job:janitor")
	return m
}

func TestJobLogRefreshRunsOnceKeepsScrollAndStopsWhenTheJobEnds(t *testing.T) {
	m := jobPreviewModel(t)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.mode != ViewPreview || cmd == nil || !m.jobLogRunning {
		t.Fatalf("opening a running job should start the log refresh: mode=%v", m.mode)
	}
	if m.ensureJobLogRefresh() != nil {
		t.Error("a second start should not add another log loop")
	}
	m.previewViewport.SetYOffset(5)
	m = update(m, jobLogTickMsg{})
	if m.previewViewport.YOffset != 5 {
		t.Errorf("an unchanged log should keep the scroll, offset=%d", m.previewViewport.YOffset)
	}
	if err := os.Remove(filepath.Join(getLogsDir(), "janitor.pid")); err != nil {
		t.Fatal(err)
	}
	next, cmd = m.Update(jobLogTickMsg{})
	if cmd != nil || next.(Model).jobLogRunning {
		t.Error("the log refresh should stop once the job ends")
	}
}

func TestJobLogShowsAPlainTextTail(t *testing.T) {
	m := jobPreviewModel(t)
	if err := os.WriteFile(filepath.Join(getLogsDir(), "janitor.log"), []byte("first line\n"+strings.Repeat("filler\n", jobLogTailLines+50)+"last line\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m.mode = ViewPreview
	m.updatePreviewViewport()
	m.previewViewport.GotoTop()
	if strings.Contains(m.previewViewport.View(), "first line") {
		t.Error("the log should show only its tail")
	}
	m.previewViewport.GotoBottom()
	if !strings.Contains(m.previewViewport.View(), "last line") {
		t.Error("the tail should end with the latest line")
	}
}

func TestRunStatePollRunsAsOneLoop(t *testing.T) {
	m := selectionTestModel(t)
	if m.ensureRunStatePoll() == nil || m.ensureRunStatePoll() != nil {
		t.Fatal("the run state poll should start once")
	}
	next, cmd := m.Update(runStatePollTickMsg{})
	if cmd != nil || next.(Model).runStatePolling {
		t.Error("the poll should stop when no job or dry run is in flight")
	}
}

func TestReviewPollRunsOnlyWhileAReviewRuns(t *testing.T) {
	m := reviewTestModel(t)
	m.mode = ViewDashboard
	m.reviewPolling = false
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m = next.(Model); m.mode != ViewPreview || m.reviewPolling || cmd != nil {
		t.Errorf("an idle PR preview should not poll: mode=%v polling=%v", m.mode, m.reviewPolling)
	}
	m.reviewPolling = true
	next, cmd = m.Update(reviewPollTickMsg{snapshot: m.loadReviewPollSnapshot()})
	if cmd != nil || next.(Model).reviewPolling {
		t.Error("the review poll should stop when nothing runs")
	}
}

func TestRedrawsUseCachedRunStates(t *testing.T) {
	m := reviewTestModel(t)
	m.mode = ViewDashboard
	m.width, m.height = 160, 50
	m.refreshReviewRuns()
	markRunning(t, m, m.ghPendingPRs[0].PR.Ref)
	writeLivePID(t, filepath.Join(getLogsDir(), "janitor.pid"))
	if strings.Contains(m.View(), "reviewing...") {
		t.Error("a redraw should not read review state from disk")
	}
	m = update(m, reviewPollTickMsg{snapshot: m.loadReviewPollSnapshot()})
	if !strings.Contains(m.View(), "reviewing...") {
		t.Error("the review poll should refresh the cached state")
	}
}

func TestModalsSkipBuildingTheDashboard(t *testing.T) {
	builds := 0
	original := composeDashboard
	composeDashboard = func(m Model) string {
		builds++
		return original(m)
	}
	t.Cleanup(func() { composeDashboard = original })
	m := syncTestModel(t)
	m.mode = ViewHelp
	m.View()
	if builds != 0 {
		t.Errorf("a modal redraw built the dashboard %d times", builds)
	}
	m.mode = ViewDashboard
	if m.View(); builds != 1 {
		t.Errorf("the dashboard should build once, got %d", builds)
	}
}

func TestMarkdownRendererIsReusedPerWidth(t *testing.T) {
	first := markdownRendererFor(80)
	if first == nil || markdownRendererFor(80) != first || markdownRendererFor(60) == first {
		t.Error("renderers should be cached per width")
	}
}

func TestGitCacheIsWrittenOncePerSyncOffTheUIThread(t *testing.T) {
	writes := 0
	original := writeGitCache
	writeGitCache = func(cache gitSyncCache) error {
		writes++
		return original(cache)
	}
	t.Cleanup(func() { writeGitCache = original })
	m := syncTestModel(t)
	m.startLoadGitStatsCmd()
	generation := m.fetchGeneration
	today := m.currentDate.Format("2006-01-02")
	var cmds []tea.Cmd
	for _, msg := range []tea.Msg{
		gitDaySectionMsg{generation: generation, day: gitDayYesterday},
		gitPendingMsg{generation: generation, pending: []GitPRItem{pendingItem(1)}},
		gitDaySectionMsg{generation: generation, day: gitDayToday, date: today, reviewed: []GitPRItem{reviewedItem("console", 2)}},
		gitMyPRsMsg{generation: generation, partOfSync: true},
		commitsLoadedMsg{generation: m.commitsGeneration},
	} {
		next, cmd := m.Update(msg)
		m = next.(Model)
		cmds = append(cmds, cmd)
	}
	if writes != 0 {
		t.Fatalf("Update wrote the cache itself %d times", writes)
	}
	for _, cmd := range cmds {
		for _, msg := range collectMsgs(cmd) {
			m = update(m, msg)
		}
	}
	if writes != 1 {
		t.Errorf("cache writes per sync = %d, want 1", writes)
	}
}

func TestPRHistoryLoadsInTheBackground(t *testing.T) {
	calls := 0
	original := gitLogForFiles
	gitLogForFiles = func(dir string, files []string) string {
		calls++
		return "abc123 touch the files"
	}
	t.Cleanup(func() { gitLogForFiles = original })
	m := reviewTestModel(t)
	pr := m.ghPendingPRs[0].PR
	pr.Files = []string{"a.ts"}
	if err := os.MkdirAll(filepath.Join(review.CloneDir(m.reviewRoot(), pr.Ref), ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	m.ghPendingPRs = []GitPRItem{sourcecontrol.NewPRItem(*pr, "Pending Review")}
	m.rebuildGitRepoStats()
	m.mode = ViewDashboard
	m.height = 200
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	if calls != 0 || cmd == nil {
		t.Fatalf("opening the preview ran git log on the UI thread (%d calls)", calls)
	}
	for _, msg := range collectMsgs(cmd) {
		m = update(m, msg)
	}
	if calls != 1 || !strings.Contains(m.previewViewport.View(), "abc123") {
		t.Errorf("history should appear once loaded: calls=%d", calls)
	}
}

func TestReviewPollReadsRunStatesInTheBackground(t *testing.T) {
	originalInterval := reviewPollInterval
	reviewPollInterval = time.Millisecond
	t.Cleanup(func() { reviewPollInterval = originalInterval })
	m := reviewTestModel(t)
	m.mode = ViewDashboard
	m.width, m.height = 160, 50
	ref := m.ghPendingPRs[0].PR.Ref
	markRunning(t, m, ref)
	msg := m.tickReviewPollCmd()()
	if err := os.Remove(filepath.Join(review.StateDir(m.reviewRoot(), ref), "review.pid")); err != nil {
		t.Fatal(err)
	}
	if m = update(m, msg); !strings.Contains(m.View(), "reviewing...") {
		t.Error("the poll should apply the states it read in the background, not re-read them")
	}
}
