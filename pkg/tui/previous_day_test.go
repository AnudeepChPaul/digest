package tui

import (
	"strings"
	"testing"
	"time"

	"app/pkg/model"
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

func TestPreviousNoteDay(t *testing.T) {
	m := syncTestModel(t)
	completedLater := manualNote("completed", dayBefore(m, 9))
	completedLater.Status, completedLater.Updated = model.StatusDone, dayBefore(m, 2)
	reviewNote := &model.Note{Summary: "review", Created: dayBefore(m, 1), Updated: dayBefore(m, 1), Source: model.SourcePRReview}
	archivedYesterday := manualNote("archived", dayBefore(m, 1))
	archivedYesterday.Status = model.StatusArchived
	cases := []struct {
		name  string
		notes []*model.Note
		want  int
	}{
		{"manual note yesterday", []*model.Note{manualNote("y", dayBefore(m, 1))}, 1},
		{"gap back to an older day", []*model.Note{manualNote("old", dayBefore(m, 2))}, 2},
		{"twenty days back", []*model.Note{manualNote("older", dayBefore(m, 20))}, 20},
		{"completed date counts", []*model.Note{completedLater}, 2},
		{"no notes", nil, 1},
		{"today's notes ignored", []*model.Note{manualNote("today", m.currentDate), manualNote("old", dayBefore(m, 3))}, 3},
		{"PR review notes ignored", []*model.Note{reviewNote, manualNote("old", dayBefore(m, 3))}, 3},
		{"archived notes ignored", []*model.Note{archivedYesterday, manualNote("old", dayBefore(m, 2))}, 2},
	}
	for _, c := range cases {
		m.notes = c.notes
		if got := m.previousNoteDay(); !isSameDay(got, dayBefore(m, c.want)) {
			t.Errorf("%s: previous day %s, want %s", c.name, got.Format("2006-01-02"), dayBefore(m, c.want).Format("2006-01-02"))
		}
	}
}

func TestPreviousDaySectionListsThatDaysDoneNotes(t *testing.T) {
	m := syncTestModel(t)
	done := manualNote("done-on-previous-day", dayBefore(m, 3))
	done.Status = model.StatusDone
	yesterdayDone := &model.Note{Summary: "review-yesterday", Created: dayBefore(m, 1), Updated: dayBefore(m, 1), Status: model.StatusDone, Source: model.SourcePRReview}
	m.notes = []*model.Note{done, yesterdayDone}
	notes := m.getYesterdayDoneNotes()
	if len(notes) != 1 || notes[0].Summary != "done-on-previous-day" {
		t.Errorf("previous-day notes = %v", notes)
	}
}

func TestPreviousDayTitles(t *testing.T) {
	m := syncTestModel(t)
	m.notes = []*model.Note{manualNote("old", dayBefore(m, 3))}
	weekday := spacedTitle(dayBefore(m, 3).Format("Monday")) + "  ·  " + strings.ToUpper(dayBefore(m, 3).Format("02 Jan"))
	content, _ := m.dashboardContent()
	strip, _ := m.renderGitStrip(m.width-3, false)
	for place, text := range map[string]string{"notes section": stripANSI(content), "git strip": stripANSI(strings.Join(strip, "\n"))} {
		if !strings.Contains(text, weekday) || strings.Contains(text, "Y E S T E R D A Y") {
			t.Errorf("%s should title the previous day %q:\n%s", place, weekday, text)
		}
	}

	m.notes = []*model.Note{manualNote("y", dayBefore(m, 1))}
	content, _ = m.dashboardContent()
	if want := "Y E S T E R D A Y  ·  " + strings.ToUpper(dayBefore(m, 1).Format("02 Jan")); !strings.Contains(stripANSI(content), want) {
		t.Errorf("yesterday title missing %q", want)
	}
}

func TestCommitsLoadForThePreviousDay(t *testing.T) {
	days := stubDayCommits(t)
	m := syncTestModel(t)
	m.notes = []*model.Note{manualNote("old", dayBefore(m, 3))}
	m.loadCommitsCmd()()
	if !strings.Contains(strings.Join(*days, ","), dayBefore(m, 3).Format("2006-01-02")) {
		t.Errorf("commits fetched for %v, want the previous day", *days)
	}
}

func TestNotesMovingThePreviousDayRefetchIt(t *testing.T) {
	m := syncTestModel(t)
	notes := []*model.Note{manualNote("old", dayBefore(m, 3))}
	generation := m.fetchGeneration
	next, cmd := m.Update(loadNotesMsg{notes: notes})
	m = next.(Model)
	if cmd == nil || m.fetchGeneration == generation || !isSameDay(m.fetchedPreviousDay, dayBefore(m, 3)) {
		t.Fatalf("moving the previous day should refetch it: generation %d→%d fetched %s", generation, m.fetchGeneration, m.fetchedPreviousDay.Format("2006-01-02"))
	}
	generation = m.fetchGeneration
	next, _ = m.Update(loadNotesMsg{notes: notes})
	if next.(Model).fetchGeneration != generation {
		t.Error("an unchanged previous day should not refetch")
	}
}

func TestPreviousDayGroupsByTypeThenFirstUpdatedFirst(t *testing.T) {
	m := syncTestModel(t)
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
