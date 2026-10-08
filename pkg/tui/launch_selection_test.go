package tui

import (
	"testing"

	"github.com/AnudeepChPaul/digest/pkg/config"
	"github.com/AnudeepChPaul/digest/pkg/model"
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
	return launchedWithDefault(t, config.SelectionNotesToday, launchNotes(syncTestModel(t), includeToday, includeCarried))
}

func launchedWithDefault(t *testing.T, selectionDefault string, notes []*model.Note) Model {
	t.Helper()
	m := syncTestModel(t)
	m.cfg.SelectionDefault = selectionDefault
	next, _ := m.Update(loadNotesMsg{notes: notes})
	return next.(Model)
}

func TestLaunchSelectsYesterdayWhenConfigured(t *testing.T) {
	m := syncTestModel(t)
	notes := append(launchNotes(m, true, true), &model.Note{Summary: "yesterday-second", Created: m.currentDate.AddDate(0, 0, -1), Updated: m.currentDate.AddDate(0, 0, -1), Status: model.StatusDone})
	if got := selectedSummary(launchedWithDefault(t, config.SelectionNotesYesterday, notes)); got != "yesterday-done" {
		t.Errorf("notes_yesterday should select the first yesterday note, got %q", got)
	}
}

func TestLaunchWithoutSelectionDefaultSelectsTheFirstItem(t *testing.T) {
	m := syncTestModel(t)
	notes := launchNotes(m, true, true)[1:]
	for _, selectionDefault := range []string{"", "unknown"} {
		if got := selectedSummary(launchedWithDefault(t, selectionDefault, notes)); got != "carried" {
			t.Errorf("selection_default %q should select the first item, got %q", selectionDefault, got)
		}
	}
	if got := selectedSummary(launchedWithDefault(t, config.SelectionNotesYesterday, notes)); got != "carried" {
		t.Errorf("an empty yesterday section should fall back to the first item, got %q", got)
	}
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
