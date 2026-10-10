package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/model"

	tea "github.com/charmbracelet/bubbletea"
)

func gitStripTestModel(t *testing.T) Model {
	t.Helper()
	m := syncTestModel(t)
	m.cfg.WorkDays = everyDay
	m.git.yesterdayGitRepo = []*GitRepoStat{{Name: "alpha"}, {Name: "beta"}}
	m.git.todayGitRepos = []*GitRepoStat{{Name: "gamma"}}
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

func TestBoardShowsYesterdayThenTodayThenGit(t *testing.T) {
	m := gitStripTestModel(t)
	m.cfg.WorkDays = everyDay
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
	if yesterdayLine < 0 || todayLine < yesterdayLine || stripLine < todayLine || captionLine < stripLine || sharedLine < captionLine {
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
	if items[0].Kind != KindYesterdayDone || items[1].Kind != KindCarriedNote || items[2].GitRepo == nil || items[2].GitRepo.Name != "alpha" {
		t.Fatalf("first items = %+v, %+v, %+v", items[0], items[1], items[2])
	}
}

func TestGitStripNavigation(t *testing.T) {
	m := gitStripTestModel(t)
	m.selected = 1
	var order []string
	for range 3 {
		order = append(order, selectedRepoName(m))
		m = press(t, m, runes("j"))
	}
	if strings.Join(order, ",") != "alpha,beta,gamma" {
		t.Fatalf("j order = %v", order)
	}
}

func TestColumnKeysDoNothingInTheGitStrip(t *testing.T) {
	m := gitStripTestModel(t)
	startDate := m.currentDate
	m.selected = 2
	for _, key := range []tea.KeyMsg{runes("l"), runes("h"), {Type: tea.KeyRight}, {Type: tea.KeyLeft}} {
		if m = press(t, m, key); selectedRepoName(m) != "beta" {
			t.Errorf("%s should do nothing in the strip, got %q", key, selectedRepoName(m))
		}
	}
	if !m.currentDate.Equal(startDate) {
		t.Errorf("column keys changed the day from %v to %v", startDate, m.currentDate)
	}
	for _, binding := range m.dashboardBindings() {
		for _, name := range binding.binding.Keys() {
			if name == "h" || name == "l" || name == "left" || name == "right" {
				t.Errorf("%q still bound to %v", name, binding.action)
			}
		}
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
	m.git.yesterdayGitRepo = []*GitRepoStat{{Name: "digest", Commits: 5, Reviewed: 1, Assigned: 2}, {Name: "a-very-long-repository-name", Commits: 7}}
	m.git.todayGitRepos = []*GitRepoStat{{Name: "web-console", Commits: 1}}
	lines, _ := m.renderGitStrip(m.width-4, false, m.groupNotes())
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
	if row := stripANSI(m.renderGitRepoRow(m.git.todayGitRepos[0], false, 36)); !strings.Contains(row, "web-console ") {
		t.Errorf("name and stats need a gap: %q", row)
	}
}
