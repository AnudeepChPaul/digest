package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"app/pkg/model"

	tea "github.com/charmbracelet/bubbletea"
)

func gitStripTestModel(t *testing.T) Model {
	t.Helper()
	m := syncTestModel(t)
	m.yesterdayGitRepo = []*GitRepoStat{{Name: "alpha"}, {Name: "beta"}}
	m.todayGitRepos = []*GitRepoStat{{Name: "gamma"}}
	m.notes = []*model.Note{{Summary: "carried", Created: m.currentDate.AddDate(0, 0, -1), Source: model.SourceManual}}
	return m
}

func selectedRepoName(m Model) string {
	item := m.allNavItems()[m.selected]
	if item.Kind != KindGitRepo {
		return ""
	}
	return item.GitRepo.Name
}

func TestBoardShowsYesterdayThenGitThenToday(t *testing.T) {
	m := gitStripTestModel(t)
	lines := plainLines(m.renderDashboardBody())
	yesterdayLine, stripLine, captionLine, sharedLine, todayLine := -1, -1, -1, -1, -1
	for index, line := range lines {
		switch {
		case strings.TrimSpace(line) != "" && strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(line, "│")), "G I T") && stripLine < 0:
			stripLine = index
		case strings.Contains(line, "Y E S T E R D A Y") && strings.Contains(line, "T O D A Y"):
			captionLine = index
		case strings.Contains(line, "Y E S T E R D A Y"):
			yesterdayLine = index
		case strings.Contains(line, "alpha") && strings.Contains(line, "gamma"):
			sharedLine = index
		case strings.Contains(line, "T O D A Y") && strings.Contains(line, "jobs"):
			todayLine = index
		}
	}
	if yesterdayLine < 0 || stripLine < yesterdayLine || captionLine < stripLine || sharedLine < captionLine || todayLine < sharedLine {
		t.Fatalf("yesterday %d strip %d captions %d shared %d today %d:\n%s", yesterdayLine, stripLine, captionLine, sharedLine, todayLine, strings.Join(lines, "\n"))
	}
	if body := strings.Join(lines, "\n"); strings.Contains(body, "Git Updates") || strings.Contains(body, "/ Git") {
		t.Errorf("Git Updates subsection should be gone:\n%s", body)
	}
}

func TestNavOrderFollowsBoard(t *testing.T) {
	m := gitStripTestModel(t)
	yesterday := m.currentDate.AddDate(0, 0, -1)
	m.notes = append(m.notes, &model.Note{Summary: "done yesterday", Status: model.StatusDone, Created: yesterday, Updated: yesterday, Source: model.SourceManual})
	items := m.allNavItems()
	if items[0].Kind != KindYesterdayDone || items[1].GitRepo == nil || items[1].GitRepo.Name != "alpha" {
		t.Fatalf("first items = %+v, %+v", items[0], items[1])
	}
	m.selected = 2
	if m = press(t, m, runes("l")); selectedRepoName(m) != "gamma" {
		t.Errorf("l from beta should land on gamma, got %q", selectedRepoName(m))
	}
	if m = press(t, m, runes("h")); selectedRepoName(m) != "alpha" {
		t.Errorf("h from gamma should land on alpha, got %q", selectedRepoName(m))
	}
}

func TestGitStripNavigation(t *testing.T) {
	m := gitStripTestModel(t)
	var order []string
	for range 3 {
		order = append(order, selectedRepoName(m))
		m = press(t, m, runes("j"))
	}
	if strings.Join(order, ",") != "alpha,beta,gamma" {
		t.Fatalf("j order = %v", order)
	}
	m.selected = 1
	if m = press(t, m, runes("l")); selectedRepoName(m) != "gamma" {
		t.Errorf("l from beta should clamp to gamma, got %q", selectedRepoName(m))
	}
	if m = press(t, m, runes("h")); selectedRepoName(m) != "alpha" {
		t.Errorf("h from gamma should land on alpha, got %q", selectedRepoName(m))
	}
	if m = press(t, m, runes("h")); selectedRepoName(m) != "alpha" {
		t.Errorf("h in the yesterday column should do nothing, got %q", selectedRepoName(m))
	}
	m.selected = 3
	if m = press(t, m, runes("l")); m.selected != 3 {
		t.Errorf("l outside the strip should do nothing, selected %d", m.selected)
	}
}

func TestGitStripArrowKeysSwitchColumnsWithoutChangingDay(t *testing.T) {
	m := gitStripTestModel(t)
	startDate := m.currentDate
	m.selected = 1
	if m = press(t, m, tea.KeyMsg{Type: tea.KeyRight}); selectedRepoName(m) != "gamma" {
		t.Errorf("right from beta should clamp to gamma, got %q", selectedRepoName(m))
	}
	if m = press(t, m, tea.KeyMsg{Type: tea.KeyLeft}); selectedRepoName(m) != "alpha" {
		t.Errorf("left from gamma should land on alpha, got %q", selectedRepoName(m))
	}
	if !m.currentDate.Equal(startDate) {
		t.Errorf("arrow keys changed the day from %v to %v", startDate, m.currentDate)
	}
}

func TestDashboardScrollKeepsSelectedNoteVisible(t *testing.T) {
	m := gitStripTestModel(t)
	for index := range 30 {
		m.notes = append(m.notes, &model.Note{Summary: fmt.Sprintf("note-%02d", index), Created: m.currentDate.Add(time.Duration(index) * time.Minute), Source: model.SourceManual})
	}
	m.height = 24
	items := m.allNavItems()
	lastNote := len(items) - 1
	for items[lastNote].Note == nil {
		lastNote--
	}
	for range lastNote {
		m = press(t, m, runes("j"))
	}
	lastSummary := items[lastNote].Note.Summary
	if body := strings.Join(plainLines(m.renderDashboardBody()), "\n"); !strings.Contains(body, lastSummary) {
		t.Errorf("%s should be scrolled into view:\n%s", lastSummary, body)
	}
}

func TestGitStripColumnsStayAlignedWhenNarrow(t *testing.T) {
	m := gitStripTestModel(t)
	m.width = 80
	m.yesterdayGitRepo = []*GitRepoStat{{Name: "digest", Commits: 5, Reviewed: 1, Assigned: 2}, {Name: "a-very-long-repository-name", Commits: 7}}
	m.todayGitRepos = []*GitRepoStat{{Name: "web-console", Commits: 1}}
	lines, _ := m.renderGitStrip(m.width-4, false)
	dividerColumn := -1
	for _, line := range lines[2:] {
		if strings.TrimSpace(stripANSI(line)) == "" {
			break
		}
		plain := []rune(stripANSI(line))
		column := strings.Index(string(plain), " │")
		column = len([]rune(string(plain)[:column]))
		if dividerColumn < 0 {
			dividerColumn = column
		}
		if column != dividerColumn {
			t.Errorf("divider at %d, want %d: %q", column, dividerColumn, string(plain))
		}
	}
	if row := stripANSI(m.renderGitRepoRow(m.todayGitRepos[0], false, 36)); !strings.Contains(row, "web-console ") {
		t.Errorf("name and stats need a gap: %q", row)
	}
}
