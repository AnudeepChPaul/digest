package tui

import (
	"strings"
	"testing"

	"github.com/achandrapaul/digest/pkg/model"
	"github.com/charmbracelet/lipgloss"
)

func TestJobRowsEndAtTheSameEdgeAsNoteRows(t *testing.T) {
	m := gitStripTestModel(t)
	for _, width := range []int{50, 80, 140} {
		jobRow := strings.TrimSuffix(m.renderDraftRow(&JobDraft{Name: "repo sync"}, false, width), "\n")
		noteRow := strings.TrimSuffix(m.renderNoteRowWithTags(&model.Note{Summary: "note"}, false, width, []string{"#tag"}), "\n")
		if lipgloss.Width(jobRow) != width || lipgloss.Width(noteRow) != width {
			t.Errorf("width %d: job row %d, note row %d", width, lipgloss.Width(jobRow), lipgloss.Width(noteRow))
		}
		if plain := stripANSI(jobRow); !strings.HasSuffix(plain, "#job   act") {
			t.Errorf("width %d: job status should end at the right edge: %q", width, plain)
		}
		passed := stripANSI(strings.TrimSuffix(m.renderDraftRow(&JobDraft{Name: "repo sync", HasRunDryRun: true}, false, width), "\n"))
		if !strings.HasSuffix(passed, "#job   done") {
			t.Errorf("width %d: a passing dry run should read done: %q", width, passed)
		}
	}
}

func TestLongJobLabelKeepsTheRightBlock(t *testing.T) {
	row := stripANSI(renderJobStyleRow("◆", strings.Repeat("long job name ", 20), "#job   act", false, 60))
	row = strings.TrimSuffix(row, "\n")
	if !strings.Contains(row, "…") || !strings.HasSuffix(row, "#job   act") || lipgloss.Width(row) != 60 {
		t.Errorf("long label should truncate and keep the right block at the edge: %q (%d)", row, lipgloss.Width(row))
	}
}
