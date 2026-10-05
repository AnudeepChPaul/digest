package tui

import (
	"testing"

	"app/pkg/model"
)

func launchNotes(m Model, includeToday, includeCarried bool) []*model.Note {
	notes := []*model.Note{{Summary: "yesterday-done", Created: m.currentDate.AddDate(0, 0, -1), Updated: m.currentDate.AddDate(0, 0, -1), Status: model.StatusDone}}
	if includeCarried {
		notes = append(notes, &model.Note{Summary: "carried", Created: m.currentDate.AddDate(0, 0, -3)})
	}
	if includeToday {
		notes = append(notes, &model.Note{Summary: "added-today", Created: m.currentDate})
	}
	return notes
}

func selectedSummary(m Model) string {
	items := m.allNavItems()
	if m.selected >= len(items) || items[m.selected].Note == nil {
		return ""
	}
	return items[m.selected].Note.Summary
}

func launchedWith(t *testing.T, includeToday, includeCarried bool) Model {
	t.Helper()
	m := syncTestModel(t)
	next, _ := m.Update(loadNotesMsg{notes: launchNotes(m, includeToday, includeCarried)})
	return next.(Model)
}

func TestLaunchSelectsFirstAddedTodayNote(t *testing.T) {
	if got := selectedSummary(launchedWith(t, true, true)); got != "added-today" {
		t.Errorf("launch should select the first Added Today note, got %q", got)
	}
}

func TestLaunchFallsBackToCarriedOverNote(t *testing.T) {
	if got := selectedSummary(launchedWith(t, false, true)); got != "carried" {
		t.Errorf("launch should fall back to the first carried note, got %q", got)
	}
}

func TestLaunchFallsBackToFirstItem(t *testing.T) {
	if m := launchedWith(t, false, false); m.selected != 0 {
		t.Errorf("launch should fall back to the first item, got %d", m.selected)
	}
}

func TestLaunchSelectionSurvivesLaterGitLoad(t *testing.T) {
	m := launchedWith(t, true, true)
	m.applyGitDay(gitDaySectionMsg{generation: m.fetchGeneration, day: gitDayYesterday, date: m.currentDate.AddDate(0, 0, -1).Format("2006-01-02"), reviewed: []GitPRItem{reviewedItem("console", 9)}})
	if got := selectedSummary(m); got != "added-today" {
		t.Errorf("selection should stay on the launch note after git loads, got %q", got)
	}
}

func TestLaterNoteReloadKeepsCurrentSelection(t *testing.T) {
	m := launchedWith(t, true, true)
	m.selected = 0
	next, _ := m.Update(loadNotesMsg{notes: launchNotes(m, true, true)})
	if next.(Model).selected != 0 {
		t.Errorf("only the first load should move the cursor")
	}
}
