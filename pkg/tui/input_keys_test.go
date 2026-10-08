package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/brag"
	"github.com/AnudeepChPaul/digest/pkg/model"

	tea "github.com/charmbracelet/bubbletea"
)

func pressControl(t *testing.T, m Model, keyType tea.KeyType) Model {
	t.Helper()
	next, _ := m.Update(tea.KeyMsg{Type: keyType})
	return next.(Model)
}

func TestEditorHalfPageKeysMoveTheCursorAndKeepText(t *testing.T) {
	m := syncTestModel(t)
	m, _ = pressKey(t, m, "a")
	if m.mode != ViewEdit {
		t.Fatalf("a should open the editor, mode %v", m.mode)
	}
	lines := make([]string, 60)
	for index := range lines {
		lines[index] = "line"
	}
	text := strings.Join(lines, "\n")
	m.editor.SetValue(text)
	lastLine := m.editor.Line()

	m = pressControl(t, m, tea.KeyCtrlU)
	if m.editor.Value() != text {
		t.Fatalf("ctrl+u should not delete text")
	}
	afterUp := m.editor.Line()
	if afterUp >= lastLine {
		t.Errorf("ctrl+u should move up from line %d, got %d", lastLine, afterUp)
	}

	m = pressControl(t, m, tea.KeyCtrlD)
	if m.editor.Value() != text {
		t.Fatalf("ctrl+d should not delete text")
	}
	if m.editor.Line() <= afterUp {
		t.Errorf("ctrl+d should move down from line %d, got %d", afterUp, m.editor.Line())
	}
}

func TestSingleLineInputsKeepTextOnHalfPageKeys(t *testing.T) {
	m := syncTestModel(t)
	m, _ = pressKey(t, m, "/")
	if m.mode != ViewSearch {
		t.Fatalf("/ should open search, mode %v", m.mode)
	}
	m = typeText(t, m, "hello")
	m.searchInput.CursorStart()
	m = pressControl(t, m, tea.KeyCtrlD)
	m.searchInput.CursorEnd()
	m = pressControl(t, m, tea.KeyCtrlU)
	if got := m.searchInput.Value(); got != "hello" {
		t.Errorf("search text = %q, want hello", got)
	}
	if m.inlineInput.KeyMap.DeleteBeforeCursor.Enabled() || m.notifyInput.KeyMap.DeleteBeforeCursor.Enabled() {
		t.Errorf("inline and notify inputs should not delete on ctrl+u")
	}
	if newSetupInput().KeyMap.DeleteBeforeCursor.Enabled() {
		t.Errorf("setup input should not delete on ctrl+u")
	}
}

func TestCommaOnDashboardOpensSetup(t *testing.T) {
	m := syncTestModel(t)
	m.configPath = "/tmp/digest/config.yaml"
	m, _ = pressKey(t, m, ",")
	if m.mode != ViewSetup || m.setup == nil || m.setup.configPath != m.configPath {
		t.Fatalf("comma should open setup on the config path, mode %v", m.mode)
	}
	var helpKeys []string
	for _, entry := range syncTestModel(t).helpEntries() {
		helpKeys = append(helpKeys, entry.key)
	}
	if !strings.Contains(strings.Join(helpKeys, " "), ",") {
		t.Errorf("help should list the setup key, got %v", helpKeys)
	}
}

func TestEditorViewScrollsWithHalfPageKeys(t *testing.T) {
	m := syncTestModel(t)
	m, _ = pressKey(t, m, "a")
	lines := make([]string, 80)
	for index := range lines {
		lines[index] = fmt.Sprintf("row-%02d", index)
	}
	m.editor.SetValue(strings.Join(lines, "\n"))
	for range 20 {
		m = pressControl(t, m, tea.KeyCtrlU)
	}
	if view := m.editor.View(); !strings.Contains(view, "row-00") || strings.Contains(view, "row-79") {
		t.Errorf("ctrl+u to the top should scroll the view up:\n%s", view)
	}
	for range 20 {
		m = pressControl(t, m, tea.KeyCtrlD)
	}
	if view := m.editor.View(); !strings.Contains(view, "row-79") {
		t.Errorf("ctrl+d to the bottom should scroll the view down:\n%s", view)
	}
}

func assertEditorAtTop(t *testing.T, name string, m Model, firstLine string) {
	t.Helper()
	if m.editor.Line() != 0 || m.editor.LineInfo().ColumnOffset != 0 {
		t.Errorf("%s editor cursor at line %d column %d, want 0 0", name, m.editor.Line(), m.editor.LineInfo().ColumnOffset)
	}
	if !strings.Contains(m.editor.View(), firstLine) {
		t.Errorf("%s editor should show its first line:\n%s", name, m.editor.View())
	}
}

func TestModalEditorsOpenAtTheTop(t *testing.T) {
	longBody := strings.Repeat("filler line\n", 80) + "last line"
	m := syncTestModel(t)
	next, _ := m.beginNoteEdit(&model.Note{ID: "long", Summary: "first summary", Body: longBody}, ViewDashboard)
	assertEditorAtTop(t, "note", next.(Model), "first summary")

	m = syncTestModel(t)
	m.bragEntry = &brag.Brag{Period: brag.WeekOf(time.Now()), Facts: "- first fact\n" + longBody}
	next, _ = m.editBrag(tea.KeyMsg{})
	assertEditorAtTop(t, "brag", next.(Model), "## Facts")
}
