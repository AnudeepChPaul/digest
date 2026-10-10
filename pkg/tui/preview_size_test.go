package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/achandrapaul/digest/pkg/model"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func modalHeight(t *testing.T, view string) int {
	t.Helper()
	lines := plainLines(view)
	top, bottom := -1, -1
	for index, line := range lines {
		if strings.Contains(line, "╭") && top < 0 {
			top = index
		}
		if strings.Contains(line, "╰") {
			bottom = index
		}
	}
	if top < 0 || bottom < 0 {
		t.Fatalf("no modal in view:\n%s", view)
	}
	return bottom - top + 1
}

func gitDetailsModel(t *testing.T, itemCount int) Model {
	t.Helper()
	m := selectionTestModel(t)
	repo := &GitRepoStat{Name: "console"}
	for index := range itemCount {
		repo.Items = append(repo.Items, GitPRItem{Title: fmt.Sprintf("item-%02d", index), Kind: "Reviewed", URL: fmt.Sprintf("https://github.com/o/console/pull/%d", index)})
	}
	m.git.gitPopupRepo = repo
	m.git.gitPopupTab = 0
	m.git.gitPopupSelected = 0
	m.mode = ViewGitDetails
	return m
}

func TestGitDetailsMatchesPreviewHeight(t *testing.T) {
	noteHeight := modalHeight(t, notePreviewModel(t, model.SourceManual, "body").View())
	for _, itemCount := range []int{3, 60} {
		if height := modalHeight(t, gitDetailsModel(t, itemCount).View()); height != noteHeight {
			t.Errorf("git details with %d items: height %d, note preview %d", itemCount, height, noteHeight)
		}
	}
	if noteHeight != previewModalHeight(selectionTestModel(t).height) {
		t.Errorf("note preview height %d != previewModalHeight %d", noteHeight, previewModalHeight(40))
	}
}

func TestGitDetailsKeepsSelectionVisible(t *testing.T) {
	m := gitDetailsModel(t, 60)
	for range 40 {
		m = press(t, m, runes("j"))
	}
	view := m.View()
	if !strings.Contains(view, "item-40") {
		t.Errorf("selected item-40 not on screen")
	}
	if lipgloss.Height(view) > m.height {
		t.Errorf("view taller than terminal")
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if view := m.View(); !strings.Contains(view, "item-00") {
		t.Errorf("tab did not restart at top")
	}
}

func TestNotePreviewHeightIgnoresTag(t *testing.T) {
	plain := notePreviewModel(t, model.SourceManual, "body")
	tagged := notePreviewModel(t, model.SourceManual, "body")
	tagged.notes[0].Subject = "general"
	tagged.updatePreviewViewport()
	if a, b := modalHeight(t, plain.View()), modalHeight(t, tagged.View()); a != b {
		t.Errorf("plain %d vs tagged %d", a, b)
	}
}

func TestVisibleGitRows(t *testing.T) {
	cases := []struct{ selected, total, rows, first, last int }{
		{0, 3, 10, 0, 3},
		{0, 60, 10, 0, 10},
		{40, 60, 10, 31, 41},
		{59, 60, 10, 50, 60},
	}
	for _, c := range cases {
		if first, last := visibleGitRows(c.selected, c.total, c.rows); first != c.first || last != c.last {
			t.Errorf("visibleGitRows(%d,%d,%d) = %d,%d want %d,%d", c.selected, c.total, c.rows, first, last, c.first, c.last)
		}
	}
}
