package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"app/pkg/review"

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
	if labelFor() != "stop review" {
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
