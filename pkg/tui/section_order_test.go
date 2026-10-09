package tui

import (
	"strings"
	"testing"

	"github.com/AnudeepChPaul/digest/pkg/model"
	"github.com/AnudeepChPaul/digest/pkg/review"
)

func sectionOrderModel(t *testing.T) Model {
	m := syncTestModel(t)
	m.git.loadingGit, m.git.loadingCommits = false, false
	m.notes = []*model.Note{
		{ID: "y1", Summary: "yesterday-done", Created: m.currentDate.AddDate(0, 0, -1), Updated: m.currentDate.AddDate(0, 0, -1), Status: model.StatusDone},
		{ID: "c1", Summary: "carried-note", Created: m.currentDate.AddDate(0, 0, -3)},
		{ID: "t1", Summary: "today-note", Created: m.currentDate},
		{ID: "d1", Summary: "closed-note", Created: m.currentDate, Updated: m.currentDate, Status: model.StatusDone},
	}
	m.git.yesterdayGitRepo = []*GitRepoStat{{Name: "alpha"}}
	m.git.todayGitRepos = []*GitRepoStat{{Name: "gamma"}}
	m.git.myPRs = []review.QueuedPR{{Ref: prRef("acme/web", 7)}}
	m.git.pendingGitAction = []GitPRItem{reviewedItem("acme/web", 9)}
	return m
}

func TestGitSectionSitsBelowClosedToday(t *testing.T) {
	m := sectionOrderModel(t)
	content, _ := m.dashboardContent()
	lines := strings.Split(stripANSI(content), "\n")
	order := []string{"yesterday-done", "T O D A Y  ·", "carried-note", "today-note", "Closed Today", "closed-note", "G I T", "alpha", "M Y   P R", "Pending Git Actions", "PR", "Jobs"}
	previous := -1
	for _, marker := range order {
		index := -1
		for lineIndex := previous + 1; lineIndex < len(lines); lineIndex++ {
			if strings.Contains(lines[lineIndex], marker) {
				index = lineIndex
				break
			}
		}
		if index < 0 {
			t.Fatalf("%q missing after line %d:\n%s", marker, previous, strings.Join(lines, "\n"))
		}
		previous = index
	}
}

func TestNavigationFollowsTheSectionOrder(t *testing.T) {
	m := sectionOrderModel(t)
	var kinds []NavItemKind
	for _, item := range m.allNavItems() {
		kinds = append(kinds, item.Kind)
	}
	want := []NavItemKind{KindYesterdayDone, KindCarriedNote, KindTodayNote, KindTodayDone, KindGitRepo, KindGitRepo, KindMyPR, KindPendingGit}
	if len(kinds) < len(want) {
		t.Fatalf("nav kinds = %v, want prefix %v", kinds, want)
	}
	for index, kind := range want {
		if kinds[index] != kind {
			t.Fatalf("nav kinds = %v, want prefix %v", kinds, want)
		}
	}
}

func TestSelectedRowsHighlightInTheNewOrder(t *testing.T) {
	m := sectionOrderModel(t)
	for index, item := range m.allNavItems() {
		m.selected = index
		content, selectedLine := m.dashboardContent()
		line := stripANSI(strings.Split(content, "\n")[selectedLine])
		var want string
		switch {
		case item.Note != nil:
			want = item.Note.Summary
		case item.GitRepo != nil:
			want = item.GitRepo.Name
		case item.MyPR != nil:
			want = "#7"
		case item.PendingGitPR != nil:
			want = item.PendingGitPR.Title
		default:
			continue
		}
		if !strings.Contains(line, want) {
			t.Errorf("selected %d (%v) points at %q, want a line with %q", index, item.Kind, line, want)
		}
	}
}

func TestNoteLoadRefreshesYesterdayThroughTheGitSection(t *testing.T) {
	m := sectionOrderModel(t)
	m.git.fetchedPreviousDay = m.currentDate.AddDate(0, 0, -10)
	m.git.ghReviewedYesterday = []GitPRItem{reviewedItem("alpha", 1)}
	if cmd := (gitSection{}).refreshPreviousDay(&m); cmd == nil {
		t.Fatal("a changed previous day should start a git refetch")
	}
	if m.git.ghReviewedYesterday != nil || len(m.git.yesterdayGitRepo) != 0 {
		t.Errorf("yesterday's git data should be cleared, got %v / %v", m.git.ghReviewedYesterday, m.git.yesterdayGitRepo)
	}

	m.git.fetchedPreviousDay = m.previousNoteDay()
	m.git.ghReviewedYesterday = []GitPRItem{reviewedItem("alpha", 1)}
	if cmd := (gitSection{}).refreshPreviousDay(&m); cmd != nil || len(m.git.ghReviewedYesterday) != 1 {
		t.Errorf("an unchanged previous day should keep yesterday's git data and not refetch")
	}
}

func TestEveryKeyActionHasExactlyOneSectionOwner(t *testing.T) {
	owners := map[keyAction][]string{}
	for name, handlers := range map[string]sectionKeystrokes{"app": appKeystrokes, "header": headerKeystrokes, "notes": notesKeystrokes, "git": gitKeystrokes, "jobs": jobsKeystrokes} {
		for action := range handlers {
			owners[action] = append(owners[action], name)
		}
	}
	for action := actionQuit; action <= actionDiscardMissingNote; action++ {
		if len(owners[action]) != 1 {
			t.Errorf("action %d owned by %v, want exactly one section", action, owners[action])
		}
	}
}
