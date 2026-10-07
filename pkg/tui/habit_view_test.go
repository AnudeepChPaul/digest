package tui

import (
	"slices"
	"strings"
	"testing"
	"time"

	"app/pkg/config"
	"app/pkg/model"
)

func tagRowsModel(t *testing.T) Model {
	t.Helper()
	m, _ := actionsTestModel(t)
	review := &model.Note{ID: "note-review", Summary: "Review console#7", Created: m.currentDate.AddDate(0, 0, -12), Source: model.SourcePRReview}
	if err := m.store.Save(review); err != nil {
		t.Fatal(err)
	}
	m.notes = append(m.notes, review)
	selectNote(t, &m, "note-1")
	return m
}

func TestUnselectedNotesShowOnlyActionTags(t *testing.T) {
	m := tagRowsModel(t)
	if line := noteLine(m, "Call the bank"); strings.Contains(line, "#manual") || strings.Contains(line, ":") {
		t.Errorf("unselected row should hide its tags: %q", line)
	}
	m = setNotify(t, m, "2")
	selectNote(t, &m, "note-2")
	line := noteLine(m, "Flaky deploys")
	if strings.Contains(line, "#manual") || !strings.HasSuffix(strings.TrimRight(line, "│ "), "@notify:2h") {
		t.Errorf("action tags stay at the right edge of unselected rows: %q", line)
	}
}

func TestSelectedNoteTagsTakeOnlyTheirOwnWidth(t *testing.T) {
	m := tagRowsModel(t)
	note := m.noteByID("note-1")
	age, _ := m.noteTagCells(note)
	if line := noteLine(m, "Flaky deploys"); !strings.Contains(line, stripANSI(age)+tagGap+"#manual") {
		t.Errorf("selected tags should not be padded to other rows' widths: %q", line)
	}
	alone := m
	alone.notes = []*model.Note{note}
	if m.inlineEditWidth(note) != alone.inlineEditWidth(note) {
		t.Errorf("inline edit width should depend only on the row's own tags: %d vs %d", m.inlineEditWidth(note), alone.inlineEditWidth(note))
	}
}

func TestPRReviewNotesAndAlwaysModeShowEveryTag(t *testing.T) {
	m := tagRowsModel(t)
	if line := noteLine(m, "Review console#7"); !strings.Contains(line, "#pr-review") || !strings.Contains(line, "12d ago") {
		t.Errorf("PR review notes always show their tags: %q", line)
	}
	always := tagRowsModel(t)
	always.cfg.ShowTags = config.ShowTagsAlways
	if line := noteLine(always, "Call the bank"); !strings.Contains(line, "#manual") {
		t.Errorf("always mode shows tags on every row: %q", line)
	}
}

func TestHeaderShowsTheStreakAndWeek(t *testing.T) {
	m, _ := actionsTestModel(t)
	m.notes = slices.DeleteFunc(m.notes, func(note *model.Note) bool { return note.Status == model.StatusDone })
	if header := stripANSI(m.renderHeader()); strings.Contains(header, "streak") || strings.Contains(header, "closed this week") {
		t.Errorf("nothing closed yet, header should stay quiet:\n%s", header)
	}
	today := m.currentDate
	if !m.cfg.IsWorkDay(today.Weekday()) {
		today = lastWorkDayBefore(m, today)
	}
	m.notes = append(m.notes,
		&model.Note{ID: "c1", Summary: "done today", Status: model.StatusDone, Created: today, Updated: today},
		&model.Note{ID: "c2", Summary: "done yesterday", Status: model.StatusDone, Created: today, Updated: lastWorkDayBefore(m, today)},
	)
	if header := stripANSI(m.renderHeader()); !strings.Contains(header, "🔥 2-day streak") || !strings.Contains(header, "closed this week") {
		t.Errorf("header should show the streak:\n%s", header)
	}
}

func lastWorkDayBefore(m Model, day time.Time) time.Time {
	for day = day.AddDate(0, 0, -1); !m.cfg.IsWorkDay(day.Weekday()); day = day.AddDate(0, 0, -1) {
	}
	return day
}
