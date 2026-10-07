package tui

import (
	"strings"
	"testing"

	"app/pkg/model"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func widestLine(view string) int {
	widest := 0
	for _, line := range strings.Split(view, "\n") {
		widest = max(widest, lipgloss.Width(line))
	}
	return widest
}

func keyAboveLabel(t *testing.T, lines []string, key, label string) bool {
	t.Helper()
	for row, line := range lines {
		column := runeColumn(line, key)
		if column < 0 || row+1 >= len(lines) {
			continue
		}
		if strings.HasPrefix(fromRuneColumn(lines[row+1], column), label) {
			return true
		}
	}
	return false
}

func TestModalWidthFor(t *testing.T) {
	for terminalWidth, want := range map[int]int{200: 180, 100: 90, 90: 81, 70: 63, 50: 45} {
		if got := modalWidthFor(terminalWidth); got != want {
			t.Errorf("modalWidthFor(%d) = %d, want %d", terminalWidth, got, want)
		}
	}
}

func TestLongFooterSplitsIntoRows(t *testing.T) {
	items := []footerItem{{"tab", "tabs", false}, {"r", "review", false}, {"space", "select", false}, {"ctrl+a", "all", false}, {"enter", "post review", false}, {"a|y", "approve", false}, {"d", "reject", true}, {"o", "nvim", false}, {"p|n", "prev/next", false}, {"ctrl+y", "copy", false}, {"esc", "close", false}}
	footer := renderModalFooter(items, 50)
	if widestLine(footer) > 50 {
		t.Errorf("footer wider than 50:\n%s", stripANSI(footer))
	}
	lines := plainLines(footer)
	for _, pair := range [][2]string{{"tab", "Tabs"}, {"enter", "Post"}, {"p|n", "Prev/next"}, {"esc", "Close"}} {
		if !keyAboveLabel(t, lines, pair[0], pair[1]) {
			t.Errorf("%s not above %s:\n%s", pair[0], pair[1], strings.Join(lines, "\n"))
		}
	}
	if short := renderModalFooter([]footerItem{{"esc", "close", false}}, 50); lipgloss.Height(short) != 2 {
		t.Errorf("short footer height %d", lipgloss.Height(short))
	}
}

func TestReviewTabFooterAlignedAndHeightFixed(t *testing.T) {
	m := reviewTestModel(t)
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	before := m.View()
	m = press(t, m, tea.KeyMsg{Type: tea.KeySpace})
	after := m.View()
	if modalHeight(t, before) != modalHeight(t, after) {
		t.Errorf("height changed after selecting: %d -> %d", modalHeight(t, before), modalHeight(t, after))
	}
	lines := plainLines(after)
	for _, pair := range [][2]string{{"enter", "Post"}, {"esc", "Close"}, {"ctrl+y", "Copy"}, {"tab", "Tabs"}} {
		if !keyAboveLabel(t, lines, pair[0], pair[1]) {
			t.Errorf("%s not above %s", pair[0], pair[1])
		}
	}
	for _, line := range lines {
		if strings.Contains(line, "│") && strings.Count(line, "│") < 2 {
			t.Errorf("line broke out of the window: %q", line)
		}
	}
}

func TestPopupsFitSmallTerminal(t *testing.T) {
	shrink := func(m Model) Model {
		m.width, m.height = 60, 20
		m.updatePreviewViewport()
		return m
	}
	review := reviewTestModel(t)
	review = press(t, review, tea.KeyMsg{Type: tea.KeyTab})
	review = press(t, review, tea.KeyMsg{Type: tea.KeySpace})
	errorPopup := selectionTestModel(t)
	errorPopup.showError("Boom", errBoom{})
	edit := selectionTestModel(t)
	edit = press(t, edit, runes("a"))
	popups := map[string]Model{
		"note":    notePreviewModel(t, model.SourcePRReview, strings.Repeat("line\n", 80)),
		"review":  review,
		"git":     gitDetailsModel(t, 60),
		"archive": archiveModel(t),
		"error":   errorPopup,
		"edit":    edit,
	}
	for name, m := range popups {
		m = shrink(m)
		view := m.View()
		if height := lipgloss.Height(view); height > m.height {
			t.Errorf("%s popup height %d > %d", name, height, m.height)
		}
		if width := widestLine(view); width > m.width {
			t.Errorf("%s popup width %d > %d", name, width, m.width)
		}
	}
}

type errBoom struct{}

func (errBoom) Error() string { return strings.Repeat("something failed badly ", 40) }
