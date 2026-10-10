package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/model"
	"github.com/achandrapaul/digest/pkg/review"
	tea "github.com/charmbracelet/bubbletea"
)

var (
	realOpenURL         = openURL
	realCopyToClipboard = copyToClipboard
)

func resize(m Model, width, height int) Model {
	next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return next.(Model)
}

func TestResizingSizesTheEditorToThePreviewModal(t *testing.T) {
	m := resize(syncTestModel(t), 100, 30)
	_, _, editorHeight := previewModalSize(100, 30)
	if m.width != 100 || m.height != 30 || m.editor.Height() != editorHeight {
		t.Fatalf("size %dx%d editor height %d", m.width, m.height, m.editor.Height())
	}
	narrowWidth := m.editor.Width()
	if m = resize(m, 160, 30); m.editor.Width() <= narrowWidth {
		t.Fatalf("a wider terminal should widen the editor: %d <= %d", m.editor.Width(), narrowWidth)
	}
}

func TestResizingWhileEditingInlineRefitsTheInput(t *testing.T) {
	m := inlineEditModel(t)
	if m.mode != ViewInlineEdit {
		t.Fatalf("mode = %v", m.mode)
	}
	before := m.inlineInput.Width
	m = resize(m, m.width+40, m.height)
	if m.inlineInput.Width != m.inlineEditWidth(m.currentNote) || m.inlineInput.Width == before {
		t.Fatalf("inline width %d (was %d)", m.inlineInput.Width, before)
	}
}

func TestResizingRefreshesOpenPreviews(t *testing.T) {
	m := reviewTestModel(t)
	m.updatePreviewViewport()
	m = resize(m, 80, 30)
	_, innerWidth, _ := previewModalSize(80, 30)
	if m.previewViewport.Width != innerWidth {
		t.Fatalf("preview width %d, want %d", m.previewViewport.Width, innerWidth)
	}
	search := typeQuery(t, searchTestModel(t), "flaky")
	search.showSearchPreviewAt(0)
	search = resize(search, 90, 30)
	_, innerWidth, _ = previewModalSize(90, 30)
	if search.previewViewport.Width != innerWidth {
		t.Fatalf("search preview width %d, want %d", search.previewViewport.Width, innerWidth)
	}
}

func fakeCommandOnPath(t *testing.T, name, script string) {
	t.Helper()
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, name), []byte("#!/bin/sh\n"+script+"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
}

func TestOpenURLRunsTheSystemOpener(t *testing.T) {
	if realOpenURL("") != nil {
		t.Fatal("an empty URL opens nothing")
	}
	openedFile := filepath.Join(t.TempDir(), "opened")
	fakeCommandOnPath(t, "open", `echo "$1" > `+openedFile)
	if err := realOpenURL("https://example.com/pr/1"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool {
		opened, _ := os.ReadFile(openedFile)
		return strings.TrimSpace(string(opened)) == "https://example.com/pr/1"
	})
	t.Setenv("PATH", t.TempDir())
	if realOpenURL("https://example.com") == nil {
		t.Fatal("a missing opener should fail")
	}
}

func TestCopyToClipboardPipesThroughPbcopy(t *testing.T) {
	if realCopyToClipboard("") != nil {
		t.Fatal("empty text copies nothing")
	}
	copiedFile := filepath.Join(t.TempDir(), "copied")
	fakeCommandOnPath(t, "pbcopy", "/bin/cat > "+copiedFile)
	if err := realCopyToClipboard("hello"); err != nil {
		t.Fatal(err)
	}
	if copied, _ := os.ReadFile(copiedFile); string(copied) != "hello" {
		t.Fatalf("copied = %q", copied)
	}
}

func TestDaysAgoIsAtLeastOne(t *testing.T) {
	now := time.Now()
	if daysAgo(now, now) != 1 || daysAgo(now, now.AddDate(0, 0, -3)) != 1 {
		t.Fatal("same or future days count as one")
	}
}

func TestInitStartsThePollsThatWereRunning(t *testing.T) {
	m := syncTestModel(t)
	m.git.syncOnLoad, m.git.loadingMyPRs = false, true
	m.runStatePolling, m.reviewPolling = true, true
	if m.Init() == nil {
		t.Fatal("init should batch the polls")
	}
}

func TestShowErrorIgnoresNilErrors(t *testing.T) {
	m := syncTestModel(t)
	m.messages = nil
	m.showError("TITLE", nil, nil)
	if len(m.messages) != 0 {
		t.Fatalf("messages = %+v", m.messages)
	}
	if navItemKey(NavItem{}) != "" {
		t.Fatal("an empty nav item has no key")
	}
}

func TestSelectionSurvivesDuplicateRows(t *testing.T) {
	m := syncTestModel(t)
	note := &model.Note{ID: "same", Summary: "twice", Source: model.SourceManual, Status: model.StatusActive, Created: m.currentDate, Updated: m.currentDate}
	twin := *note
	m.notes = []*model.Note{note, &twin}
	m.contentVersion++
	items := m.allNavItems()
	if len(items) < 2 || navItemKey(items[0]) != navItemKey(items[1]) {
		t.Skipf("rows are not duplicated: %d items", len(items))
	}
	m.selected = 1
	key, occurrence := m.selectedNavKey()
	if occurrence != 1 {
		t.Fatalf("occurrence = %d", occurrence)
	}
	m.selected = 0
	m.restoreSelection(key, occurrence)
	if m.selected != 1 {
		t.Fatalf("selected = %d, want the second copy", m.selected)
	}
}

func TestPreviewWithNoRowsKeepsTheViewport(t *testing.T) {
	m := syncTestModel(t)
	m.notes = nil
	m.contentVersion++
	before := m.previewViewport
	m.updatePreviewViewport()
	if m.previewViewport.Width != before.Width || m.previewViewport.Height != before.Height {
		t.Fatal("no rows means no preview change")
	}
}

func TestOpeningAPRWhileAReviewRunsStartsThePoll(t *testing.T) {
	m := withLocalReview(reviewTestModel(t), localReviewState{status: review.RunRunning})
	m.mode = ViewDashboard
	m.reviewPolling = false
	next, _ := m.openPreview(m.allNavItems()[m.selected])
	if !next.(Model).reviewPolling {
		t.Fatal("a running review should start the poll")
	}
}

func TestScrollOffsetIsClampedToTheContent(t *testing.T) {
	m := manyTodayNotesModel(t, 80)
	frame := m.currentDashboardFrame()
	m.scrollOffset = 10000
	lineCount := strings.Count(frame.content, "\n") + 1
	if got := m.visibleScrollOffset(frame); got != lineCount-m.frameBodyHeight(frame) {
		t.Fatalf("offset = %d", got)
	}
}
