package tui

import (
	"strings"
	"testing"

	"github.com/achandrapaul/digest/pkg/model"

	tea "github.com/charmbracelet/bubbletea"
)

func runUpdates(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	for _, msg := range collectMsgs(cmd) {
		switch msg.(type) {
		case notesSavedMsg, loadNotesMsg:
			next, followUp := m.Update(msg)
			m = runUpdates(t, next.(Model), followUp)
		}
	}
	return m
}

func TestNewNoteIsSelectedAfterSave(t *testing.T) {
	m := gitStripTestModel(t)
	m.selected = 0
	m = press(t, m, runes("a"))
	m.editor.SetValue("Freshly added")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m = runUpdates(t, next.(Model), cmd)
	item, ok := m.selectedNavItem()
	if !ok || item.Note == nil || item.Note.Summary != "Freshly added" {
		t.Errorf("selected = %+v", item)
	}
	if m.selectAfterReload != "" {
		t.Errorf("selection request not cleared: %q", m.selectAfterReload)
	}
}

func TestInlineEditScrollsToCursor(t *testing.T) {
	m := gitStripTestModel(t)
	m.width = 60
	for index, item := range m.allNavItems() {
		if item.Note != nil {
			m.selected = index
		}
	}
	m = press(t, m, runes("i"))
	if m.inlineInput.Width <= 0 {
		t.Fatalf("inline width not set: %d", m.inlineInput.Width)
	}
	typed := strings.Repeat("x", 80) + "END"
	m = press(t, m, runes(typed))
	view := stripANSI(m.inlineInput.View())
	if !strings.Contains(view, "END") {
		t.Errorf("newest characters not visible: %q", view)
	}
	if visibleWidth := len([]rune(strings.TrimRight(view, " "))); visibleWidth > m.inlineInput.Width+1 {
		t.Errorf("view wider than input: %d > %d", visibleWidth, m.inlineInput.Width)
	}
	if body := strings.Join(plainLines(m.renderDashboardBody()), "\n"); !strings.Contains(body, "END") {
		t.Errorf("dashboard row does not show the cursor end:\n%s", body)
	}
}

func TestRowTitlesShareTheRowColour(t *testing.T) {
	withTrueColor(t)
	m := gitStripTestModel(t)
	rowColour := "38;2;205;214;243"
	note := m.notes[0]
	for name, row := range map[string]string{
		"unselected note": m.renderRow(note, false, 80),
		"selected note":   m.renderRow(note, true, 80),
		"selected title":  selectedTitle("PR title"),
	} {
		if !strings.Contains(row, rowColour) {
			t.Errorf("%s does not use the row colour: %q", name, row)
		}
		if strings.Contains(row, "38;2;245;224;220") {
			t.Errorf("%s still uses rosewater: %q", name, row)
		}
	}
}

func TestInlineEditLeavesRoomForWiderTagSlots(t *testing.T) {
	m := gitStripTestModel(t)
	m.notes = append(m.notes, &model.Note{Summary: "other", Created: m.currentDate.AddDate(0, 0, -2), Source: "a-very-long-source-name"})
	for index, item := range m.allNavItems() {
		if item.Note == m.notes[0] {
			m.selected = index
		}
	}
	m = press(t, m, runes("i"))
	m = press(t, m, runes(strings.Repeat("x", 150)+"END"))
	for _, line := range plainLines(m.renderDashboardBody()) {
		if strings.Contains(line, "xxx") && (!strings.Contains(line, "END") || strings.Contains(line, "…")) {
			t.Errorf("inline text runs into the tags: %q", line)
		}
	}
}

func TestTodaysClosedNotesHaveNoStrikethrough(t *testing.T) {
	withTrueColor(t)
	m := gitStripTestModel(t)
	closed := &model.Note{Summary: "finished", Status: model.StatusDone, Created: m.currentDate, Updated: m.currentDate, Source: model.SourceManual}
	for _, selected := range []bool{false, true} {
		row := m.renderRow(closed, selected, 80)
		if strings.Contains(row, "\x1b[9m") || strings.Contains(row, ";9m") || strings.Contains(row, ";9;") || strings.Contains(row, "[9;") {
			t.Errorf("selected=%v row is struck through: %q", selected, row)
		}
	}
}
