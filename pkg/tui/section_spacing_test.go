package tui

import (
	"strings"
	"testing"

	"github.com/AnudeepChPaul/digest/pkg/model"
	"github.com/AnudeepChPaul/digest/pkg/review"
	"github.com/AnudeepChPaul/digest/pkg/sourcecontrol"
)

func lastLineContaining(lines []string, text string) int {
	found := -1
	for index, line := range lines {
		if strings.Contains(line, text) {
			found = index
		}
	}
	return found
}

func blankLinesBefore(lines []string, index int) int {
	count := 0
	for index--; index >= 0 && strings.TrimSpace(lines[index]) == ""; index-- {
		count++
	}
	return count
}

func TestSectionsAreEquallySpaced(t *testing.T) {
	m := syncTestModel(t)
	m.git.loadingGit = false
	m.notes = []*model.Note{
		{Summary: "yesterday-done", Created: m.currentDate.AddDate(0, 0, -1), Status: model.StatusDone, Updated: m.currentDate.AddDate(0, 0, -1)},
		{Summary: "carried-note", Created: m.currentDate.AddDate(0, 0, -3)},
	}
	m.git.ghPendingPRs = []GitPRItem{
		sourcecontrol.NewPRItem(review.QueuedPR{Ref: prRef("console", 1), Title: "One", CIState: "SUCCESS"}, "Pending Review"),
		sourcecontrol.NewPRItem(review.QueuedPR{Ref: prRef("digest", 2), Title: "Two", CIState: "SUCCESS"}, "Pending Review"),
	}
	m.rebuildGitRepoStats()
	content, _ := m.dashboardContent()
	lines := strings.Split(stripANSI(content), "\n")

	for _, section := range []string{"G I T", "jobs ◆", "Added Today", "Closed Today", "Pending Git Actions", "Jobs"} {
		if gap := blankLinesBefore(lines, lastLineContaining(lines, section)); gap != 2 {
			t.Errorf("%q should have 2 blank lines before it, got %d", section, gap)
		}
	}
	if gap := blankLinesBefore(lines, lastLineContaining(lines, "Pending, Carried Over")); gap != 1 {
		t.Errorf("first subsection should sit 1 blank line under the TODAY title, got %d", gap)
	}
	if gap := blankLinesBefore(lines, lastLineContaining(lines, "yesterday-done")); gap != 1 {
		t.Errorf("yesterday rows should sit 1 blank line under the title, got %d", gap)
	}
	secondRepo := lastLineContaining(lines, "    digest")
	if gap := blankLinesBefore(lines, secondRepo); gap != 1 {
		t.Errorf("repositories should be 1 blank line apart, got %d", gap)
	}
}

func TestEmptyYesterdayKeepsTheSectionGap(t *testing.T) {
	m := syncTestModel(t)
	content, _ := m.dashboardContent()
	lines := strings.Split(stripANSI(content), "\n")
	if gap := blankLinesBefore(lines, lastLineContaining(lines, "jobs ◆")); gap != 2 {
		t.Errorf("TODAY should stay 2 blank lines below an empty YESTERDAY, got %d", gap)
	}
}
