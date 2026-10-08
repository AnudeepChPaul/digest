package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

func longNoteBody(lines int) string {
	var body strings.Builder
	for line := 0; line < lines; line++ {
		fmt.Fprintf(&body, "line %d /sdfsd/asdfa {\"private_key\":\"%s\"}\n", line, strings.Repeat("MIIEvQIBADANBgkqhkiG9w0BAQEFAASC", 6))
	}
	return body.String()
}

func editLongNote(m Model, lines int) Model {
	note := m.notes[0]
	note.Body = longNoteBody(lines)
	next, _ := m.beginNoteEdit(note, ViewDashboard)
	return next.(Model)
}

func TestEnterAddsLinesPastNinetyNine(t *testing.T) {
	m := editLongNote(noteSelectedModel(t), 150)
	before := m.editor.LineCount()
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if after := m.editor.LineCount(); after != before+1 {
		t.Errorf("enter in a %d-line note: lines %d -> %d", before, before, after)
	}
}

func BenchmarkTypingInALongNote(b *testing.B) {
	m := editLongNote(benchModel(b), 500)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
		m = next.(Model)
		_ = m.View()
	}
}

func countEditorRenders(t *testing.T) *int {
	t.Helper()
	renders := 0
	original := renderEditorView
	renderEditorView = func(editor *textarea.Model) string {
		renders++
		return original(editor)
	}
	t.Cleanup(func() { renderEditorView = original })
	return &renders
}

func TestEditorRedrawsReuseTheViewUntilTheTextChanges(t *testing.T) {
	m := editLongNote(noteSelectedModel(t), 20)
	m.width, m.height = 120, 40
	renders := countEditorRenders(t)
	first := m.View()
	for _, msg := range []tea.Msg{syncPulseTickMsg{}, hintIdleMsg{}, bannerWaveTickMsg{}} {
		m = update(m, msg)
		if m.View() != first {
			t.Fatalf("%T changed the editor view", msg)
		}
	}
	if *renders != 1 {
		t.Errorf("editor rendered %d times for unchanged text, want 1", *renders)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Z")})
	if view := m.View(); view == first || !strings.Contains(stripANSI(view), "Z") {
		t.Errorf("typing should redraw the editor")
	}
}

func TestEditorCursorStaysSolid(t *testing.T) {
	m := editLongNote(noteSelectedModel(t), 3)
	if mode := m.editor.Cursor.Mode(); mode != cursor.CursorStatic {
		t.Errorf("editor cursor mode = %v, want static", mode)
	}
}

func TestTypingInTheEditorDoesNotStartTheHintTimer(t *testing.T) {
	m := editLongNote(noteSelectedModel(t), 3)
	m.cfg.ShowKeyHints = true
	generation := m.hintGeneration
	m = press(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if m.hintGeneration != generation {
		t.Errorf("typing in the editor restarted the dashboard hint timer")
	}
}

func BenchmarkPulseWhileEditingALongNote(b *testing.B) {
	m := editLongNote(benchModel(b), 500)
	m.loadingGit = true
	m.syncPulseRunning = true
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		next, _ := m.Update(syncPulseTickMsg{})
		m = next.(Model)
		_ = m.View()
	}
}
