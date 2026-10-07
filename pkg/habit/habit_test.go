package habit

import (
	"strings"
	"testing"
	"time"

	"app/pkg/model"
)

var weekdaysOnly = func(day time.Weekday) bool { return day != time.Saturday && day != time.Sunday }

func at(year int, month time.Month, day, hour int) time.Time {
	return time.Date(year, month, day, hour, 0, 0, 0, time.Local)
}

func closedOn(moment time.Time) *model.Note {
	return &model.Note{Summary: "closed", Status: model.StatusDone, Created: moment.AddDate(0, 0, -1), Updated: moment, Source: model.SourceManual}
}

func TestGatherCountsTheDay(t *testing.T) {
	now := at(2026, time.October, 7, 18)
	notes := []*model.Note{
		{Summary: "old one", Created: now.AddDate(0, 0, -8), Source: model.SourceManual},
		{Summary: "fresh", Created: now, Source: model.SourceManual},
		{Summary: "NewPR: acme:web:7", Created: now.AddDate(0, 0, -1), Source: model.SourceMyPR},
		closedOn(now), closedOn(now.Add(-time.Hour)), closedOn(at(2026, time.October, 5, 10)),
		{Summary: "archived", Status: model.StatusArchived, Created: now},
	}
	facts := Gather(notes, now, weekdaysOnly)
	if facts.Pending != 3 || facts.CarriedOver != 2 || facts.ClosedToday != 2 || facts.OpenMyPRs != 1 || facts.OldestDays != 8 || facts.ClosedThisWeek != 3 {
		t.Errorf("facts = %+v", facts)
	}
}

func TestStreakSkipsNonWorkDaysAndWaitsForToday(t *testing.T) {
	monday := at(2026, time.October, 12, 9)
	notes := []*model.Note{closedOn(at(2026, time.October, 9, 15)), closedOn(at(2026, time.October, 8, 15)), closedOn(at(2026, time.October, 6, 15))}
	if streak := Streak(notes, monday, weekdaysOnly); streak != 2 {
		t.Errorf("Thu+Fri closed, weekend skipped, Monday not over yet: streak = %d, want 2", streak)
	}
	notes = append(notes, closedOn(monday))
	if streak := Streak(notes, monday, weekdaysOnly); streak != 3 {
		t.Errorf("closing on Monday extends the streak: %d, want 3", streak)
	}
	if streak := Streak(nil, monday, weekdaysOnly); streak != 0 {
		t.Errorf("no closes means no streak, got %d", streak)
	}
	sundayWorker := func(day time.Weekday) bool { return day != time.Friday && day != time.Saturday }
	sunday := at(2026, time.October, 11, 20)
	if streak := Streak([]*model.Note{closedOn(sunday), closedOn(at(2026, time.October, 8, 10))}, sunday, sundayWorker); streak != 2 {
		t.Errorf("Sun-Thu worker: Thu then Sun is a 2-day streak, got %d", streak)
	}
}

func TestSummaryLinesUseOnlyFactsThatApply(t *testing.T) {
	morning := SummaryLines(Facts{Pending: 4, OldestDays: 8, OpenMyPRs: 2, Streak: 5}, SlotMorning)
	joined := strings.Join(morning, "\n")
	for _, want := range []string{"4 notes waiting", "oldest is 8 days old", "2 of your PRs", "Day 5"} {
		if !strings.Contains(joined, want) {
			t.Errorf("morning lines missing %q:\n%s", want, joined)
		}
	}
	evening := strings.Join(SummaryLines(Facts{Pending: 2, ClosedToday: 3, ClosedThisWeek: 9}, SlotEvening), "\n")
	if !strings.Contains(evening, "3 closed today, 2 to go") || !strings.Contains(evening, "9 closed this week") {
		t.Errorf("evening lines:\n%s", evening)
	}
	if quiet := SummaryLines(Facts{}, SlotEvening); len(quiet) != 0 {
		t.Errorf("a quiet evening should send nothing: %v", quiet)
	}
	if clean := SummaryLines(Facts{}, SlotMorning); len(clean) != 1 || !strings.Contains(clean[0], "Clean slate") {
		t.Errorf("an empty morning = %v", clean)
	}
}

func TestPickLineRotatesByDayAndSlot(t *testing.T) {
	lines := []string{"a", "b", "c"}
	monday, tuesday := at(2026, time.October, 12, 9), at(2026, time.October, 13, 9)
	if PickLine(lines, monday, SlotMorning) == PickLine(lines, tuesday, SlotMorning) {
		t.Errorf("consecutive days should not repeat the same line")
	}
	if PickLine(lines, monday, SlotMorning) != PickLine(lines, monday.Add(time.Hour), SlotMorning) {
		t.Errorf("the same slot on the same day should be stable")
	}
}
