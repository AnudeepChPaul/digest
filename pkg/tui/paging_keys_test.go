package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/model"

	tea "github.com/charmbracelet/bubbletea"
)

const textareaTab = "    "

func emptyDashboardModel(t *testing.T) Model {
	t.Helper()
	m := syncTestModel(t)
	m.notes = nil
	m.contentVersion++
	if items := m.allNavItems(); len(items) != 0 {
		t.Fatalf("expected no rows, got %d", len(items))
	}
	return m
}

func boundActions(m Model, keys ...tea.KeyMsg) map[string]keyAction {
	bound := map[string]keyAction{}
	for _, msg := range keys {
		if binding, found := m.resolveKey(msg); found {
			bound[msg.String()] = binding.action
		}
	}
	return bound
}

var rowActionKeys = []tea.KeyMsg{{Type: tea.KeyEnter}, {Type: tea.KeyTab}, {Type: tea.KeySpace, Runes: []rune(" ")}, runes("i"), runes("d"), runes("r"), runes("@"), runes(".")}

func TestRowKeysAreUnboundOnAnEmptyDashboard(t *testing.T) {
	m := emptyDashboardModel(t)
	if bound := boundActions(m, rowActionKeys...); len(bound) != 0 {
		t.Errorf("row keys bound without rows: %v", bound)
	}
	m.notes = []*model.Note{{ID: "n1", Summary: "one", Created: m.currentDate, Source: model.SourceManual}}
	m.contentVersion++
	if bound := boundActions(m, rowActionKeys...); len(bound) < 6 {
		t.Errorf("row keys should be bound once there is a row: %v", bound)
	}
}

func TestRIsNotARunJobKeyOnNonJobRows(t *testing.T) {
	m := noteSelectedModel(t)
	if binding, found := m.resolveKey(runes("r")); found {
		t.Errorf("r on a note row is bound to %v", binding.action)
	}
}

func TestCIsUnboundWhenDailyCommitsAreOff(t *testing.T) {
	m := syncTestModel(t)
	off := false
	m.cfg.ShowDailyCommits = &off
	if binding, found := m.resolveKey(runes("c")); found {
		t.Errorf("c bound to %v with daily commits off", binding.action)
	}
	on := true
	m.cfg.ShowDailyCommits = &on
	if binding, found := m.resolveKey(runes("c")); !found || binding.action != actionRefreshCommits {
		t.Errorf("c should refresh commits when daily commits are on")
	}
}

func TestRejectBoxIgnoresHalfPageKeys(t *testing.T) {
	m := rejectCommentModel(t)
	for _, name := range []string{"ctrl+d", "ctrl+u"} {
		if binding, found := m.resolveKey(keyFor(name)); found {
			t.Errorf("%s bound to %v in the reject box", name, binding.action)
		}
	}
	m.rejectInput.SetValue("first\nsecond\nthird")
	line := m.rejectInput.Line()
	m = pressControl(t, m, tea.KeyCtrlU)
	m = pressControl(t, m, tea.KeyCtrlD)
	if m.rejectInput.Value() != "first\nsecond\nthird" || m.rejectInput.Line() != line || m.mode != ViewRejectComment {
		t.Errorf("value %q line %d->%d mode %v", m.rejectInput.Value(), line, m.rejectInput.Line(), m.mode)
	}
}

func TestPageKeysMoveAFullBodyOnTheDashboard(t *testing.T) {
	m := manyTodayNotesModel(t, 120)
	page := max(m.dashboardBodyHeight(), 5)
	lastIndex := len(m.allNavItems()) - 1
	m.selected = 0
	if moved := press(t, m, tea.KeyMsg{Type: tea.KeyPgDown}); moved.selected != page {
		t.Errorf("pgdown from 0: selected=%d, want %d", moved.selected, page)
	}
	m.selected = lastIndex - 1
	if moved := press(t, m, tea.KeyMsg{Type: tea.KeyPgDown}); moved.selected != lastIndex {
		t.Errorf("pgdown near the end: selected=%d, want %d", moved.selected, lastIndex)
	}
	m.selected = page + 1
	if moved := press(t, m, tea.KeyMsg{Type: tea.KeyPgUp}); moved.selected != 1 {
		t.Errorf("pgup from %d: selected=%d, want 1", page+1, moved.selected)
	}
	m.selected = 1
	if moved := press(t, m, tea.KeyMsg{Type: tea.KeyPgUp}); moved.selected != 0 {
		t.Errorf("pgup near the top: selected=%d, want 0", moved.selected)
	}
}

func longArchiveModel(t *testing.T) Model {
	t.Helper()
	m := selectionTestModel(t)
	for index := range 80 {
		m.notes = append(m.notes, &model.Note{ID: fmt.Sprintf("archived-%02d", index), Summary: fmt.Sprintf("archived %02d", index), Status: model.StatusArchived, Created: m.currentDate, Updated: m.currentDate.Add(-time.Duration(index) * time.Minute)})
	}
	return press(t, m, tea.KeyMsg{Type: tea.KeyCtrlE})
}

func archiveSelectionVisible(m Model) bool {
	summary := m.getArchivedNotes()[m.archivedSelected].Summary
	return strings.Contains(stripANSI(m.archivedViewport.View()), summary)
}

func TestArchivePagingMovesTheSelectionAndScrollFollows(t *testing.T) {
	m := longArchiveModel(t)
	page := max(m.archivedViewport.Height, 1)
	half := max(page/2, 1)
	steps := []struct {
		key  tea.KeyMsg
		want int
	}{
		{tea.KeyMsg{Type: tea.KeyCtrlD}, half},
		{tea.KeyMsg{Type: tea.KeyPgDown}, half + page},
		{tea.KeyMsg{Type: tea.KeyCtrlU}, page},
		{tea.KeyMsg{Type: tea.KeyPgUp}, 0},
	}
	for _, step := range steps {
		m = press(t, m, step.key)
		if m.archivedSelected != step.want || m.mode != ViewArchived {
			t.Fatalf("%s: selected %d want %d mode %v", step.key, m.archivedSelected, step.want, m.mode)
		}
		if !archiveSelectionVisible(m) {
			t.Errorf("%s: selected row %d scrolled out of view", step.key, m.archivedSelected)
		}
	}
	for range 10 {
		m = press(t, m, tea.KeyMsg{Type: tea.KeyPgDown})
	}
	if m.archivedSelected != len(m.getArchivedNotes())-1 || !archiveSelectionVisible(m) {
		t.Errorf("pgdown should clamp to the last note and show it, selected %d", m.archivedSelected)
	}
	for range 3 {
		m = press(t, m, runes("k"))
	}
	if !archiveSelectionVisible(m) {
		t.Errorf("k should keep the selection visible")
	}
}

func TestArchiveRowKeysAreUnboundWithoutNotes(t *testing.T) {
	m := press(t, selectionTestModel(t), tea.KeyMsg{Type: tea.KeyCtrlE})
	if m.mode != ViewArchived {
		t.Fatalf("mode %v", m.mode)
	}
	if bound := boundActions(m, runes("u"), tea.KeyMsg{Type: tea.KeyEnter}, runes("d")); len(bound) != 0 {
		t.Errorf("archive keys bound with no archived notes: %v", bound)
	}
	if footer := footerText(m.archiveFooterItems()); strings.Contains(footer, "unarchive") || strings.Contains(footer, "delete") {
		t.Errorf("footer offers restore or delete with no notes: %s", footer)
	}
	full := longArchiveModel(t)
	if bound := boundActions(full, runes("u"), tea.KeyMsg{Type: tea.KeyEnter}, runes("d")); len(bound) != 3 {
		t.Errorf("archive keys should be bound with notes: %v", bound)
	}
}

func TestNoteEditorPageKeysMoveTheCursorAFullPage(t *testing.T) {
	m := syncTestModel(t)
	m, _ = pressKey(t, m, "a")
	lines := make([]string, 200)
	for index := range lines {
		lines[index] = fmt.Sprintf("row-%03d", index)
	}
	text := strings.Join(lines, "\n")
	m.editor.SetValue(text)
	startEditorAtTop(m.editor)
	page := max(m.editor.Height(), 1)
	m = pressControl(t, m, tea.KeyPgDown)
	if m.editor.Line() != page || m.editor.Value() != text {
		t.Fatalf("pgdown: line %d want %d", m.editor.Line(), page)
	}
	m = pressControl(t, m, tea.KeyPgDown)
	m = pressControl(t, m, tea.KeyPgUp)
	if m.editor.Line() != page || m.mode != ViewEdit {
		t.Errorf("pgup: line %d want %d mode %v", m.editor.Line(), page, m.mode)
	}
}

func TestTabInsertsATabInEveryEditor(t *testing.T) {
	m := syncTestModel(t)
	m, _ = pressKey(t, m, "a")
	m = typeText(t, m, "a")
	m = pressControl(t, m, tea.KeyTab)
	m = typeText(t, m, "b")
	if got := m.editor.Value(); got != "a"+textareaTab+"b" || m.mode != ViewEdit {
		t.Errorf("note editor value %q mode %v", got, m.mode)
	}
	for _, mode := range []ViewMode{ViewBragEdit, ViewAutomationEdit} {
		m.mode = mode
		m.editor.Reset()
		m = pressControl(t, m, tea.KeyTab)
		if got := m.editor.Value(); got != textareaTab || m.mode != mode {
			t.Errorf("mode %v: value %q", mode, got)
		}
	}
}
