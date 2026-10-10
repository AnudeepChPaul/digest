package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/model"
)

func dayBefore(m Model, days int) time.Time {
	return m.currentDate.AddDate(0, 0, -days)
}

func manualNote(summary string, created time.Time) *model.Note {
	return &model.Note{Summary: summary, Created: created, Updated: created, Source: model.SourceManual}
}

func spacedTitle(word string) string {
	return strings.Join(strings.Split(strings.ToUpper(word), ""), " ")
}

func workDaysExcept(m Model, daysBack ...int) []string {
	skipped := map[string]bool{}
	for _, back := range daysBack {
		skipped[strings.ToLower(dayBefore(m, back).Format("Mon"))] = true
	}
	var workDays []string
	for _, name := range everyDay {
		if !skipped[name] {
			workDays = append(workDays, name)
		}
	}
	return workDays
}

func TestPreviousNoteDay(t *testing.T) {
	m := syncTestModel(t)
	manualOnSkippedDay := manualNote("weekend note", dayBefore(m, 1))
	cases := []struct {
		name     string
		workDays []string
		notes    []*model.Note
		want     int
	}{
		{"every day works", everyDay, nil, 1},
		{"skips non-work days", workDaysExcept(m, 1, 2), nil, 3},
		{"manual notes on a non-work day do not move it", workDaysExcept(m, 1, 2), []*model.Note{manualOnSkippedDay}, 3},
		{"older manual notes do not move it", everyDay, []*model.Note{manualNote("old", dayBefore(m, 5))}, 1},
		{"only today works falls back a week", workDaysExcept(m, 1, 2, 3, 4, 5, 6), nil, 7},
	}
	for _, c := range cases {
		m.cfg.WorkDays, m.notes = c.workDays, c.notes
		if got := m.previousNoteDay(); !isSameDay(got, dayBefore(m, c.want)) {
			t.Errorf("%s: previous day %s, want %s", c.name, got.Format("2006-01-02"), dayBefore(m, c.want).Format("2006-01-02"))
		}
	}
}

func TestPreviousDaySectionListsOnlyTheLastWorkDaysDoneNotes(t *testing.T) {
	m := syncTestModel(t)
	m.cfg.WorkDays = workDaysExcept(m, 1, 2)
	doneOn := func(summary string, daysBack int, source model.Source) *model.Note {
		return &model.Note{Summary: summary, Created: dayBefore(m, 9), Updated: dayBefore(m, daysBack), Status: model.StatusDone, Source: source}
	}
	m.notes = []*model.Note{
		doneOn("last-work-day", 3, model.SourceManual),
		doneOn("skipped-day-two", 2, model.SourcePRReview),
		doneOn("skipped-day-one", 1, model.SourceJanitor),
		doneOn("before-last-work-day", 4, model.SourceManual),
		doneOn("done-today", 0, model.SourceManual),
	}
	var got []string
	for _, note := range m.getYesterdayDoneNotes() {
		got = append(got, note.Summary)
	}
	if want := "last-work-day"; strings.Join(got, ",") != want {
		t.Errorf("previous-day notes = %v, want %s", got, want)
	}
}

func TestPreviousDaySectionListsThatDaysDoneNotes(t *testing.T) {
	m := syncTestModel(t)
	m.cfg.WorkDays = everyDay
	done := manualNote("done-on-previous-day", dayBefore(m, 3))
	done.Status = model.StatusDone
	yesterdayDone := &model.Note{Summary: "review-yesterday", Created: dayBefore(m, 1), Updated: dayBefore(m, 1), Status: model.StatusDone, Source: model.SourcePRReview}
	m.notes = []*model.Note{done, yesterdayDone}
	notes := m.getYesterdayDoneNotes()
	if len(notes) != 1 || notes[0].Summary != "review-yesterday" {
		t.Errorf("previous-day notes = %v", notes)
	}
}

func TestPreviousDayTitles(t *testing.T) {
	m := syncTestModel(t)
	m.cfg.WorkDays = workDaysExcept(m, 1, 2)
	m.notes = []*model.Note{manualNote("old", dayBefore(m, 3))}
	weekday := spacedTitle(dayBefore(m, 3).Format("Monday")) + "  ·  " + strings.ToUpper(dayBefore(m, 3).Format("02 Jan"))
	content, _ := m.dashboardContent()
	strip, _ := m.renderGitStrip(m.width-3, false, m.groupNotes())
	for place, text := range map[string]string{"notes section": stripANSI(content), "git strip": stripANSI(strings.Join(strip, "\n"))} {
		if !strings.Contains(text, weekday) || strings.Contains(text, "Y E S T E R D A Y") {
			t.Errorf("%s should title the previous day %q:\n%s", place, weekday, text)
		}
	}

	m.cfg.WorkDays = everyDay
	m.notes = []*model.Note{manualNote("y", dayBefore(m, 1))}
	content, _ = m.dashboardContent()
	if want := "Y E S T E R D A Y  ·  " + strings.ToUpper(dayBefore(m, 1).Format("02 Jan")); !strings.Contains(stripANSI(content), want) {
		t.Errorf("yesterday title missing %q", want)
	}
}

func TestCommitsLoadForThePreviousDay(t *testing.T) {
	days := stubDayCommits(t)
	m := syncTestModel(t)
	m.cfg.WorkDays = workDaysExcept(m, 1, 2)
	m.notes = []*model.Note{manualNote("old", dayBefore(m, 3))}
	m.loadCommitsCmd()()
	if !strings.Contains(strings.Join(*days, ","), dayBefore(m, 3).Format("2006-01-02")) {
		t.Errorf("commits fetched for %v, want the previous day", *days)
	}
}

func TestLoadingNotesDoesNotMoveThePreviousDay(t *testing.T) {
	m := syncTestModel(t)
	m.cfg.WorkDays = workDaysExcept(m, 1, 2)
	m.git.fetchedPreviousDay = m.previousNoteDay()
	notes := []*model.Note{manualNote("on a non-work day", dayBefore(m, 1))}
	generation := m.git.fetchGeneration
	next, _ := m.Update(loadNotesMsg{notes: notes})
	m = next.(Model)
	if m.git.fetchGeneration != generation || !isSameDay(m.git.fetchedPreviousDay, dayBefore(m, 3)) {
		t.Errorf("notes should not move the previous day: generation %d→%d fetched %s", generation, m.git.fetchGeneration, m.git.fetchedPreviousDay.Format("2006-01-02"))
	}
}

func TestPreviousDayGroupsByTypeThenFirstUpdatedFirst(t *testing.T) {
	m := syncTestModel(t)
	m.cfg.WorkDays = everyDay
	day := dayBefore(m, 1)
	doneAt := func(summary string, source model.Source, hour int) *model.Note {
		at := time.Date(day.Year(), day.Month(), day.Day(), hour, 0, 0, 0, day.Location())
		return &model.Note{Summary: summary, Created: at, Updated: at, Status: model.StatusDone, Source: source}
	}
	m.notes = []*model.Note{
		doneAt("review-late", model.SourcePRReview, 16),
		doneAt("manual-late", model.SourceManual, 15),
		doneAt("janitor", model.SourceJanitor, 12),
		doneAt("review-early", model.SourcePRReview, 8),
		doneAt("manual-early", model.SourceManual, 9),
	}
	want := []string{"janitor", "manual-early", "manual-late", "review-early", "review-late"}
	var got []string
	for _, note := range m.getYesterdayDoneNotes() {
		got = append(got, note.Summary)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("order = %v, want %v", got, want)
	}
	var navigated []string
	for _, item := range m.allNavItems() {
		if item.Kind == KindYesterdayDone {
			navigated = append(navigated, item.Note.Summary)
		}
	}
	if strings.Join(navigated, ",") != strings.Join(want, ",") {
		t.Errorf("navigation = %v, want %v", navigated, want)
	}
}
