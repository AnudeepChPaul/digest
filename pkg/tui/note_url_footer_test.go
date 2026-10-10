package tui

import (
	"strings"
	"testing"

	"github.com/achandrapaul/digest/pkg/model"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func stubOpenURL(t *testing.T) *[]string {
	t.Helper()
	originalOpen := openURL
	var opened []string
	openURL = func(url string) error {
		opened = append(opened, url)
		return nil
	}
	t.Cleanup(func() { openURL = originalOpen })
	return &opened
}

func notePreviewModel(t *testing.T, source model.Source, body string) Model {
	t.Helper()
	m := selectionTestModel(t)
	m.notes = []*model.Note{{ID: "console:7", Summary: "Approved: thing", Source: source, Body: body, Status: model.StatusDone, Created: m.currentDate, Updated: m.currentDate}}
	found := false
	for index, item := range m.allNavItems() {
		if item.Note != nil {
			m.selected = index
			found = true
		}
	}
	if !found {
		t.Fatal("note row not found")
	}
	m.mode = ViewPreview
	m.updatePreviewViewport()
	return m
}

func TestEnterEditsPRReviewNoteInsteadOfOpeningTheLink(t *testing.T) {
	opened := stubOpenURL(t)
	m := notePreviewModel(t, model.SourcePRReview, "https://github.com/acme/console/pull/19162")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != ViewEdit || len(*opened) != 0 {
		t.Errorf("mode=%v opened=%v", m.mode, *opened)
	}
}

func TestOOpensPRReviewNoteLink(t *testing.T) {
	opened := stubOpenURL(t)
	m := notePreviewModel(t, model.SourcePRReview, "https://github.com/acme/console/pull/19162")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	if m.mode != ViewPreview {
		t.Errorf("mode = %v", m.mode)
	}
	if len(*opened) != 1 || (*opened)[0] != "https://github.com/acme/console/pull/19162" {
		t.Errorf("opened %v", *opened)
	}
}

func TestOInManualNotePreviewOpensItsLink(t *testing.T) {
	opened := stubOpenURL(t)
	m := notePreviewModel(t, model.SourceManual, "https://github.com/o/r/pull/1")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	if m.mode != ViewPreview || len(*opened) != 1 || (*opened)[0] != "https://github.com/o/r/pull/1" {
		t.Errorf("mode=%v opened=%v", m.mode, *opened)
	}
}

func TestPRReviewNoteURL(t *testing.T) {
	cases := map[string]string{
		"See https://docs.example.com/guide first.\nThen review https://git.example.com/team/monkey/pull/42 today.": "https://git.example.com/team/monkey/pull/42",
		"only https://docs.example.com/guide here": "",
		"no links at all":                          "",
	}
	for body, want := range cases {
		note := &model.Note{Source: model.SourcePRReview, Body: body}
		if got := prReviewNoteURL(note); got != want {
			t.Errorf("prReviewNoteURL(%q) = %q, want %q", body, got, want)
		}
	}
	manual := &model.Note{Source: model.SourceManual, Body: "https://github.com/o/r/pull/1"}
	if got := prReviewNoteURL(manual); got != "" {
		t.Errorf("manual note gave %q", got)
	}
}

func TestEnterInManualNoteEditsInsteadOfOpeningTheLink(t *testing.T) {
	opened := stubOpenURL(t)
	m := notePreviewModel(t, model.SourceManual, "https://github.com/o/r/pull/1")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != ViewEdit || len(*opened) != 0 {
		t.Errorf("mode=%v opened=%v", m.mode, *opened)
	}
}

func TestPRReviewNoteFooter(t *testing.T) {
	prNote := stripANSI(notePreviewModel(t, model.SourcePRReview, "https://github.com/o/r/pull/1").View())
	if !strings.Contains(prNote, "enter") || !strings.Contains(prNote, "Edit") || !strings.Contains(prNote, " o ") || !strings.Contains(prNote, "Open") {
		t.Errorf("pr-review footer should offer enter Edit and o Open PR:\n%s", prNote)
	}
	if manual := stripANSI(notePreviewModel(t, model.SourceManual, "x").View()); strings.Contains(manual, "Open") || !strings.Contains(manual, "Edit") {
		t.Errorf("manual note without links should offer enter Edit and no o")
	}
}

func runeColumn(line, text string) int {
	byteIndex := strings.Index(line, text)
	if byteIndex < 0 {
		return -1
	}
	return len([]rune(line[:byteIndex]))
}

func fromRuneColumn(line string, column int) string {
	runesInLine := []rune(line)
	if column > len(runesInLine) {
		return ""
	}
	return string(runesInLine[column:])
}

func plainLines(rendered string) []string {
	var lines []string
	for _, line := range strings.Split(rendered, "\n") {
		lines = append(lines, strings.TrimRight(stripANSI(line), " "))
	}
	return lines
}

func stripANSI(text string) string {
	var out strings.Builder
	inEscape := false
	for _, r := range text {
		switch {
		case r == '\x1b':
			inEscape = true
		case inEscape && ((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')):
			inEscape = false
		case !inEscape:
			out.WriteRune(r)
		}
	}
	return out.String()
}

func TestFooterSplitsTwoWordLabels(t *testing.T) {
	lines := plainLines(renderModalFooter([]footerItem{{"esc", "close", false}, {"ctrl+d|u", "half page", false}}, 200))
	if len(lines) != 3 {
		t.Fatalf("lines = %q", lines)
	}
	column := runeColumn(lines[0], "ctrl+d|u")
	if column < 0 || !strings.HasPrefix(fromRuneColumn(lines[1], column), "Half") || !strings.HasPrefix(fromRuneColumn(lines[2], column), "Page") {
		t.Errorf("lines = %q", lines)
	}
	if !strings.HasPrefix(lines[1], "Close") {
		t.Errorf("close not capitalised: %q", lines[1])
	}
	if single := plainLines(renderModalFooter([]footerItem{{"esc", "close", false}, {"y", "confirm", false}}, 200)); len(single) != 2 {
		t.Errorf("one-word footer lines = %d", len(single))
	}
}

func TestCtrlCFooterShowsTwoMore(t *testing.T) {
	m := selectionTestModel(t)
	m.width = 260
	m = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlC})
	lines := plainLines(m.renderFooter())
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "press") || strings.Contains(strings.ToLower(joined), "2 more") {
		t.Fatalf("footer still has old label:\n%s", joined)
	}
	column := runeColumn(lines[1], "ctrl+c")
	if column < 0 || !strings.HasPrefix(fromRuneColumn(lines[2], column), "2") || !strings.HasPrefix(fromRuneColumn(lines[3], column), "More") {
		t.Errorf("footer:\n%s", joined)
	}
}

func TestViewsFitTerminal(t *testing.T) {
	views := map[string]Model{
		"dashboard": selectionTestModel(t),
		"note":      longNotePreview(t),
		"archive":   archiveModel(t),
	}
	for name, m := range views {
		if height := lipgloss.Height(m.View()); height > m.height {
			t.Errorf("%s view height %d > terminal %d", name, height, m.height)
		}
	}
}
