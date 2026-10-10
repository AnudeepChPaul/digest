package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/brag"
	"github.com/achandrapaul/digest/pkg/model"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func captureClipboard(t *testing.T) *[]string {
	t.Helper()
	var copied []string
	previous := copyToClipboard
	copyToClipboard = func(text string) error {
		copied = append(copied, text)
		return nil
	}
	t.Cleanup(func() { copyToClipboard = previous })
	return &copied
}

func noteNavKey(t *testing.T, m Model, noteID string) string {
	t.Helper()
	for _, item := range m.allNavItems() {
		if item.Note != nil && item.Note.ID == noteID {
			return navItemKey(item)
		}
	}
	t.Fatalf("no row for note %q", noteID)
	return ""
}

func twoNotePreviewModel(t *testing.T) Model {
	t.Helper()
	m := selectionTestModel(t)
	m.notes = []*model.Note{
		{ID: "first", Summary: "first note", Body: "first body", Status: model.StatusActive, Source: model.SourceManual, Created: m.currentDate, Updated: m.currentDate},
		{ID: "second", Summary: "second note", Body: "second body", Status: model.StatusActive, Source: model.SourceManual, Created: m.currentDate, Updated: m.currentDate},
	}
	m.contentVersion++
	return m
}

func selectedKey(m Model) string {
	item, _ := m.selectedNavItem()
	return navItemKey(item)
}

func openPreviewOn(t *testing.T, m Model, key string) Model {
	t.Helper()
	selectNavItem(t, &m, key)
	m.mode = ViewPreview
	m.resetReviewView()
	m.updatePreviewViewport()
	return m
}

func TestPAndNMoveBetweenRowsInsideThePreview(t *testing.T) {
	m := twoNotePreviewModel(t)
	keys := make([]string, 0)
	for _, item := range m.allNavItems() {
		keys = append(keys, navItemKey(item))
	}
	m = openPreviewOn(t, m, keys[0])
	for index := 1; index < len(keys); index++ {
		m = press(t, m, runes("n"))
		if m.mode != ViewPreview || selectedKey(m) != keys[index] {
			t.Fatalf("n step %d: mode=%v selected=%q want %q", index, m.mode, selectedKey(m), keys[index])
		}
	}
	m = press(t, m, runes("n"))
	if selectedKey(m) != keys[len(keys)-1] {
		t.Errorf("n past the last row moved to %q", selectedKey(m))
	}
	for index := len(keys) - 2; index >= 0; index-- {
		m = press(t, m, runes("p"))
		if m.mode != ViewPreview || selectedKey(m) != keys[index] {
			t.Fatalf("p step %d: mode=%v selected=%q want %q", index, m.mode, selectedKey(m), keys[index])
		}
	}
	m = press(t, m, runes("p"))
	if selectedKey(m) != keys[0] {
		t.Errorf("p before the first row moved to %q", selectedKey(m))
	}
}

func openFirstNotePreview(t *testing.T) Model {
	t.Helper()
	m := twoNotePreviewModel(t)
	return openPreviewOn(t, m, noteNavKey(t, m, "first"))
}

func TestPAndNRefreshThePreviewContent(t *testing.T) {
	m := openFirstNotePreview(t)
	if !strings.Contains(stripANSI(m.View()), "first body") {
		t.Fatalf("preview should show the first note")
	}
	key := navItemKey(m.allNavItems()[m.selected+1])
	m = press(t, m, runes("n"))
	if selectedKey(m) != key || !strings.Contains(stripANSI(m.View()), "second body") {
		t.Errorf("n should show the second note, selected %q", selectedKey(m))
	}
	m = press(t, m, runes("p"))
	if !strings.Contains(stripANSI(m.View()), "first body") {
		t.Errorf("p should show the first note again")
	}
}

func TestPAndNInPRPreviewMoveToTheNextPR(t *testing.T) {
	m := selectionTestModel(t)
	m.applyGitPending(gitPendingMsg{generation: m.git.fetchGeneration, pending: []GitPRItem{pendingItem(1), pendingItem(2)}})
	first, second := "pr:"+pendingItem(1).URL, "pr:"+pendingItem(2).URL
	m = openPreviewOn(t, m, first)
	m.previewTab = previewTabReview
	m = press(t, m, runes("n"))
	if selectedKey(m) != second || m.mode != ViewPreview {
		t.Fatalf("n selected %q mode %v", selectedKey(m), m.mode)
	}
	if m.previewTab != previewTabDetails {
		t.Errorf("moving to another PR should reset to the Details tab, tab %d", m.previewTab)
	}
	m = press(t, m, runes("p"))
	if selectedKey(m) != first {
		t.Errorf("p selected %q", selectedKey(m))
	}
}

func TestCtrlYInNotePreviewCopiesSummaryAndBody(t *testing.T) {
	copied := captureClipboard(t)
	m := openFirstNotePreview(t)
	press(t, m, tea.KeyMsg{Type: tea.KeyCtrlY})
	if len(*copied) != 1 || (*copied)[0] != "first note\n\nfirst body" {
		t.Errorf("copied = %q", *copied)
	}
}

func TestCtrlYInJobPreviewCopiesTheLog(t *testing.T) {
	copied := captureClipboard(t)
	m := selectionTestModel(t)
	if err := os.WriteFile(filepath.Join(getLogsDir(), "janitor.log"), []byte("cleaned 3 files\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m = openPreviewOn(t, m, "job:janitor")
	press(t, m, tea.KeyMsg{Type: tea.KeyCtrlY})
	if len(*copied) != 1 || (*copied)[0] != "cleaned 3 files" {
		t.Errorf("copied = %q", *copied)
	}
}

func TestCtrlYInJobPreviewWithoutALogCopiesNothing(t *testing.T) {
	copied := captureClipboard(t)
	m := openPreviewOn(t, selectionTestModel(t), "job:janitor")
	press(t, m, tea.KeyMsg{Type: tea.KeyCtrlY})
	if len(*copied) != 0 {
		t.Errorf("copied = %q", *copied)
	}
}

func assertCopiedTheWholePreview(t *testing.T, m Model, copied []string) {
	t.Helper()
	m.height = 8
	m.updatePreviewViewport()
	if len(copied) != 1 || strings.TrimSpace(copied[0]) == "" {
		t.Fatalf("copied = %q", copied)
	}
	if strings.Contains(copied[0], "\x1b[") {
		t.Errorf("copy should be plain text: %q", copied[0])
	}
	visible := strings.TrimSpace(stripANSI(m.previewViewport.View()))
	if lipgloss.Height(copied[0]) <= lipgloss.Height(visible) && m.previewViewport.TotalLineCount() > m.previewViewport.Height {
		t.Errorf("copy should hold every preview line, not only the visible part: %q", copied[0])
	}
}

func TestCtrlYInRunPreviewCopiesTheWholePreview(t *testing.T) {
	copied := captureClipboard(t)
	m, _, failed := reviewRunsModel(t)
	m = openPreviewOn(t, m, "review:"+failed.URL)
	press(t, m, tea.KeyMsg{Type: tea.KeyCtrlY})
	assertCopiedTheWholePreview(t, m, *copied)
	if !strings.Contains((*copied)[0], "failed") && !strings.Contains((*copied)[0], "Failed") && !strings.Contains((*copied)[0], "FAILED") {
		t.Errorf("run copy should include its status: %q", (*copied)[0])
	}
}

func TestCtrlYInMyPRPreviewCopiesTheWholePreview(t *testing.T) {
	copied := captureClipboard(t)
	m := myPRStripModel(t)
	m = openPreviewOn(t, m, "mine:"+m.git.myPRs[0].Ref.URL)
	press(t, m, tea.KeyMsg{Type: tea.KeyCtrlY})
	assertCopiedTheWholePreview(t, m, *copied)
	if !strings.Contains((*copied)[0], m.git.myPRs[0].Title) {
		t.Errorf("My PR copy should include the title %q: %q", m.git.myPRs[0].Title, (*copied)[0])
	}
}

func TestCtrlYInPRPreviewCopiesTitleURLAndFindings(t *testing.T) {
	copied := captureClipboard(t)
	m := reviewTestModel(t)
	press(t, m, tea.KeyMsg{Type: tea.KeyCtrlY})
	if len(*copied) != 1 {
		t.Fatalf("copied = %q", *copied)
	}
	for _, want := range []string{"Fix\nhttps://github.com/o/console/pull/7", "## Review findings", "C1", "H1"} {
		if !strings.Contains((*copied)[0], want) {
			t.Errorf("copy missing %q: %q", want, (*copied)[0])
		}
	}
}

func TestCtrlYInSearchPreviewCopiesTheNote(t *testing.T) {
	copied := captureClipboard(t)
	m := typeQuery(t, searchTestModel(t), "flaky")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = press(t, m, runes("n"))
	press(t, m, tea.KeyMsg{Type: tea.KeyCtrlY})
	if len(*copied) != 1 || (*copied)[0] != "Standup notes\n\nretried the Flaky console test twice" {
		t.Errorf("copied = %q", *copied)
	}
}

func TestCtrlYInNoteEditorCopiesTheTrimmedText(t *testing.T) {
	copied := captureClipboard(t)
	m := openFirstNotePreview(t)
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != ViewEdit {
		t.Fatalf("enter should edit, mode %v", m.mode)
	}
	m.editor.SetValue("edited title\nedited body\n")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlY})
	if m.mode != ViewEdit || len(*copied) != 1 || (*copied)[0] != "edited title\nedited body" {
		t.Errorf("mode=%v copied = %q", m.mode, *copied)
	}
}

func savedBragModel(t *testing.T) Model {
	t.Helper()
	m, _ := bragTestModel(t, wednesday())
	week40 := brag.WeekOf(time.Date(2026, 9, 30, 12, 0, 0, 0, time.Local))
	if err := (&brag.Brag{Period: week40, Created: wednesday(), Facts: "- collected", Summary: strings.Repeat("- Did things\n", 80)}).Save(m.cfg.BragDir()); err != nil {
		t.Fatal(err)
	}
	m = press(t, m, runes("b"))
	m = selectBragRow(t, m, "Week 40 · 28 Sep – 04 Oct")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != ViewBragView {
		t.Fatalf("mode = %v", m.mode)
	}
	return m
}

func TestCtrlYInBragViewCopiesTheBrag(t *testing.T) {
	copied := captureClipboard(t)
	m := savedBragModel(t)
	m = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlY})
	if m.mode != ViewBragView || len(*copied) != 1 || (*copied)[0] != strings.TrimSpace(m.bragEntry.Body()) {
		t.Fatalf("mode=%v copied = %q", m.mode, *copied)
	}
	if !strings.Contains((*copied)[0], "## Facts") || !strings.Contains((*copied)[0], "- Did things") {
		t.Errorf("copy should hold facts and summary: %q", (*copied)[0])
	}
}

func TestCtrlYInBragEditorCopiesTheTrimmedText(t *testing.T) {
	copied := captureClipboard(t)
	m := press(t, savedBragModel(t), tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != ViewBragEdit {
		t.Fatalf("mode = %v", m.mode)
	}
	m.editor.SetValue("\n## Facts\n\n- typed\n\n")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlY})
	if m.mode != ViewBragEdit || len(*copied) != 1 || (*copied)[0] != "## Facts\n\n- typed" {
		t.Errorf("mode=%v copied = %q", m.mode, *copied)
	}
}
