package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/AnudeepChPaul/digest/pkg/review"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

func stubReviewControl(t *testing.T) (*[]string, *[]string) {
	t.Helper()
	originalStart, originalStop := startBackground, stopReview
	var started, stopped []string
	startBackground = func(pr review.QueuedPR, root string) error {
		started = append(started, pr.Ref.URL)
		return nil
	}
	stopReview = func(root string, ref review.PRRef) error {
		stopped = append(stopped, ref.URL)
		return nil
	}
	t.Cleanup(func() { startBackground, stopReview = originalStart, originalStop })
	return &started, &stopped
}

func markRunning(t *testing.T, m Model, ref review.PRRef) {
	t.Helper()
	dir := review.StateDir(m.reviewRoot(), ref)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "review.pid"), []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestROnDetailsTabAsksBeforeStarting(t *testing.T) {
	started, _ := stubReviewControl(t)
	m := reviewTestModel(t)
	m.previewTab = previewTabDetails
	m = press(t, m, runes("r"))
	if m.mode != ViewReviewRunConfirm || m.reviewRunAction != "start" || len(*started) != 0 {
		t.Fatalf("mode=%v action=%q started=%v", m.mode, m.reviewRunAction, *started)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != ViewPreview || len(*started) != 0 {
		t.Fatalf("esc: mode=%v started=%v", m.mode, *started)
	}
	m = press(t, m, runes("r"))
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != ViewPreview || len(*started) != 1 {
		t.Fatalf("confirm: mode=%v started=%v", m.mode, *started)
	}
}

func TestDStopsRunningReviewOtherwiseRejects(t *testing.T) {
	_, stopped := stubReviewControl(t)
	m := reviewTestModel(t)
	idle := press(t, m, runes("d"))
	if idle.mode != ViewRejectComment {
		t.Fatalf("idle d: mode=%v, want reject comment", idle.mode)
	}
	item := m.currentPRItem()
	markRunning(t, m, item.PR.Ref)
	m = press(t, m, runes("d"))
	if m.mode != ViewReviewRunConfirm || m.reviewRunAction != "stop" {
		t.Fatalf("running d: mode=%v action=%q", m.mode, m.reviewRunAction)
	}
	m = press(t, m, runes("y"))
	if len(*stopped) != 1 || (*stopped)[0] != item.PR.Ref.URL || m.mode != ViewPreview {
		t.Fatalf("stopped=%v mode=%v", *stopped, m.mode)
	}
}

func TestDOnRunningReviewRowStopsIt(t *testing.T) {
	_, stopped := stubReviewControl(t)
	m, running, failed := reviewRunsModel(t)
	selectNavItem(t, &m, "review:"+failed.URL)
	m = press(t, m, runes("d"))
	if m.mode == ViewReviewRunConfirm {
		t.Fatalf("failed review row should not offer stop")
	}
	selectNavItem(t, &m, "review:"+running.URL)
	m = press(t, m, runes("d"))
	if m.mode != ViewReviewRunConfirm {
		t.Fatalf("mode=%v", m.mode)
	}
	if !strings.Contains(m.View(), "console:1") {
		t.Errorf("confirm dialog does not name the review")
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(*stopped) != 1 || (*stopped)[0] != running.URL || m.mode != ViewDashboard {
		t.Fatalf("stopped=%v mode=%v", *stopped, m.mode)
	}
}

func TestPreviewHeaderShowsReviewPID(t *testing.T) {
	m := reviewTestModel(t)
	markRunning(t, m, m.currentPRItem().PR.Ref)
	want := fmt.Sprintf("PID %d", os.Getpid())
	if !strings.Contains(m.View(), want) {
		t.Errorf("PR preview missing %q", want)
	}
	runs, running, _ := reviewRunsModel(t)
	selectNavItem(t, &runs, "review:"+running.URL)
	runs.mode = ViewPreview
	runs.updatePreviewViewport()
	if !strings.Contains(runs.View(), want) {
		t.Errorf("review job preview missing %q", want)
	}
}

func TestReviewFooterLabelsD(t *testing.T) {
	m := reviewTestModel(t)
	item := m.currentPRItem()
	labelFor := func() string {
		for _, footer := range m.reviewFooterItems(item) {
			if footer.key == "d" {
				return footer.action
			}
		}
		return ""
	}
	if labelFor() != "reject" {
		t.Errorf("idle d label = %q", labelFor())
	}
	markRunning(t, m, item.PR.Ref)
	if labelFor() != "stop" {
		t.Errorf("running d label = %q", labelFor())
	}
}

func TestJobPreviewHeaderShowsPID(t *testing.T) {
	m := selectionTestModel(t)
	if err := os.WriteFile(filepath.Join(getLogsDir(), "janitor.pid"), []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		t.Fatal(err)
	}
	selectNavItem(t, &m, "job:janitor")
	m.mode = ViewPreview
	m.updatePreviewViewport()
	if want := fmt.Sprintf("PID %d", os.Getpid()); !strings.Contains(m.View(), want) {
		t.Errorf("job preview missing %q", want)
	}
}

func TestStoppingFromReviewRowPreviewReturnsToDashboard(t *testing.T) {
	stubReviewControl(t)
	m, running, _ := reviewRunsModel(t)
	stopReview = func(root string, ref review.PRRef) error {
		return os.Remove(filepath.Join(review.StateDir(root, ref), "review.pid"))
	}
	selectNavItem(t, &m, "review:"+running.URL)
	m.mode = ViewPreview
	m.updatePreviewViewport()
	m = press(t, m, runes("d"))
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != ViewDashboard {
		t.Fatalf("mode = %v, want dashboard once the stopped row is gone", m.mode)
	}
	for _, item := range m.allNavItems() {
		if navItemKey(item) == "review:"+running.URL {
			t.Errorf("stopped review still listed")
		}
	}
}

func TestReviewPollRerendersPreviewOnlyWhenThisPRChanged(t *testing.T) {
	m := reviewTestModel(t)
	stateDir := review.StateDir(m.reviewRoot(), m.currentPRItem().PR.Ref)
	baseline, _ := m.handleReviewPoll(reviewPollSnapshot{localReviews: map[string]localReviewState{stateDir: {status: review.RunRunning, pid: 7}}})
	m = baseline.(Model)
	m.previewViewport = viewport.New(80, 20)
	m.previewViewport.SetContent("sentinel")
	unchanged := map[string]localReviewState{stateDir: {status: review.RunRunning, pid: 7}, "other": {pid: 9}}
	next, _ := m.handleReviewPoll(reviewPollSnapshot{localReviews: unchanged})
	m = next.(Model)
	if !strings.Contains(m.previewViewport.View(), "sentinel") {
		t.Errorf("unchanged review state should keep the preview")
	}
	changed := map[string]localReviewState{stateDir: {status: review.RunDone, finished: true}}
	next, _ = m.handleReviewPoll(reviewPollSnapshot{localReviews: changed})
	m = next.(Model)
	if strings.Contains(m.previewViewport.View(), "sentinel") {
		t.Errorf("changed review state should re-render the preview")
	}
}

func TestRunStatePollLeavesUnrelatedPreviewAlone(t *testing.T) {
	m := reviewTestModel(t)
	m.dryRunsInFlight = map[string]bool{"janitor": true}
	m.previewViewport = viewport.New(80, 20)
	m.previewViewport.SetContent("sentinel")
	next, _ := m.Update(runStatePollTickMsg{})
	m = next.(Model)
	if !strings.Contains(m.previewViewport.View(), "sentinel") {
		t.Errorf("a busy job should not rebuild a PR preview every poll")
	}
}

func TestCloneReadyDoesNotRunTmuxOnTheUILoop(t *testing.T) {
	m := reviewTestModel(t)
	opened := 0
	previous := openInNvim
	openInNvim = func(string, review.PRRef) error {
		opened++
		return nil
	}
	t.Cleanup(func() { openInNvim = previous })
	next, _ := m.handleCloneReady(reviewCloneReadyMsg{dir: t.TempDir(), ref: m.currentPRItem().PR.Ref})
	if opened != 0 || next.(Model).reviewNotice != "Opened clone in a new tmux window" {
		t.Errorf("opened %d notice %q", opened, next.(Model).reviewNotice)
	}
	next, _ = m.handleCloneReady(reviewCloneReadyMsg{dir: t.TempDir(), ref: m.currentPRItem().PR.Ref, openErr: errors.New("tmux new-window: no server")})
	if next.(Model).reviewNotice != "tmux new-window: no server" {
		t.Errorf("notice %q", next.(Model).reviewNotice)
	}
}

func TestRunningReviewPreviewRefreshesWhenItsLogGrows(t *testing.T) {
	m := reviewTestModel(t)
	ref := m.currentPRItem().PR.Ref
	markRunning(t, m, ref)
	logPath := filepath.Join(review.StateDir(m.reviewRoot(), ref), review.LogFile)
	if err := os.WriteFile(logPath, []byte("cloning\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m.refreshLocalReviews()
	poll := func() {
		next, _ := m.handleReviewPoll(m.loadReviewPollSnapshot())
		m = next.(Model)
	}
	poll()
	m.previewViewport = viewport.New(80, 20)
	m.previewViewport.SetContent("sentinel")
	poll()
	if !strings.Contains(m.previewViewport.View(), "sentinel") {
		t.Fatalf("an idle log should not rebuild the preview")
	}
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	logFile.WriteString("installing dependencies\n")
	logFile.Close()
	poll()
	if strings.Contains(m.previewViewport.View(), "sentinel") {
		t.Errorf("a growing review log should refresh the preview")
	}
}

func TestReviewPollFillsInPRsListedSinceItStarted(t *testing.T) {
	m := reviewTestModel(t)
	stateDir := review.StateDir(m.reviewRoot(), m.currentPRItem().PR.Ref)
	next, _ := m.handleReviewPoll(reviewPollSnapshot{localReviews: map[string]localReviewState{}})
	m = next.(Model)
	if _, cached := m.localReviews[stateDir]; !cached {
		t.Errorf("a listed PR missing from the poll snapshot should be read again")
	}
}
