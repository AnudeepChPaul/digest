package tui

import (
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/appstate"
	"github.com/achandrapaul/digest/pkg/config"
	"github.com/achandrapaul/digest/pkg/model"
	"github.com/achandrapaul/digest/pkg/store"

	tea "github.com/charmbracelet/bubbletea"
)

var everyDay = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

func appStateModel(t *testing.T, state *appstate.State, notes ...*model.Note) Model {
	t.Helper()
	showGit := false
	cfg := &config.Config{DigestRoot: t.TempDir(), GreenOnly: true, ShowGit: &showGit, WorkDays: everyDay}
	noteStore := store.New(cfg.NotesDir())
	for _, note := range notes {
		if err := noteStore.Save(note); err != nil {
			t.Fatal(err)
		}
	}
	if state != nil {
		if err := appstate.Save(cfg.Root(), *state); err != nil {
			t.Fatal(err)
		}
	}
	m := NewModel(cfg, nil)
	m.width, m.height = 120, 50
	return m
}

func applyAllMessages(m Model, cmd tea.Cmd) Model {
	for _, msg := range collectMsgs(cmd) {
		switch msg.(type) {
		case notesSavedMsg, appStateSavedMsg, loadNotesMsg, appStateRebuiltMsg:
			next, followUp := m.Update(msg)
			m = applyAllMessages(next.(Model), followUp)
		}
	}
	return m
}

func markDone(t *testing.T, m Model, summary string) Model {
	t.Helper()
	selectSummary(t, &m, summary)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")})
	return applyAllMessages(next.(Model), cmd)
}

func twoActiveNotes(now time.Time) []*model.Note {
	return []*model.Note{
		{Summary: "first", Status: model.StatusActive, Source: model.SourceManual, Created: now.Add(-time.Hour), Updated: now.Add(-time.Hour)},
		{Summary: "second", Status: model.StatusActive, Source: model.SourceManual, Created: now.Add(-time.Minute), Updated: now.Add(-time.Minute)},
	}
}

func TestFirstDoneOfTheDayWritesTheStateOnce(t *testing.T) {
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
	m := appStateModel(t, &appstate.State{FirstNoteCreated: now.AddDate(-1, 0, 0), Streak: 2, LastDoneDay: yesterday}, twoActiveNotes(now)...)
	m = markDone(t, m, "first")
	saved, found, err := appstate.Load(m.cfg.Root())
	if err != nil || !found || saved.Streak != 3 || saved.LastDoneDay != now.Format("2006-01-02") {
		t.Fatalf("state after the first done = %+v, %v, %v", saved, found, err)
	}
	if header := stripANSI(headerSection{}.Render(m)); !strings.Contains(header, "3-day streak") {
		t.Errorf("header should show the stored streak: %q", header)
	}
	if err := os.Remove(appstate.Path(m.cfg.Root())); err != nil {
		t.Fatal(err)
	}
	m = markDone(t, m, "second")
	if _, found, _ := appstate.Load(m.cfg.Root()); found {
		t.Error("a second done the same day should not write the state file")
	}
	if header := stripANSI(headerSection{}.Render(m)); !strings.Contains(header, "3-day streak") {
		t.Errorf("streak should stay at 3: %q", header)
	}
}

func TestHeaderStreakComesFromTheStateFile(t *testing.T) {
	now := time.Now()
	m := appStateModel(t, &appstate.State{FirstNoteCreated: now.AddDate(-1, 0, 0), Streak: 7, LastDoneDay: now.Format("2006-01-02")}, twoActiveNotes(now)...)
	if header := stripANSI(headerSection{}.Render(m)); !strings.Contains(header, "7-day streak") {
		t.Errorf("header should show the stored streak before notes load: %q", header)
	}
}

func TestMissingStateIsRebuiltFromDiskAtStartup(t *testing.T) {
	now := time.Now()
	m := appStateModel(t, nil, dashboardNotes(now)...)
	if _, found, _ := appstate.Load(m.cfg.Root()); found {
		t.Fatal("no state file should exist yet")
	}
	m = applyAllMessages(m, m.rebuildAppStateCmd())
	saved, found, err := appstate.Load(m.cfg.Root())
	if err != nil || !found || !m.appStateKnown {
		t.Fatalf("state should be rebuilt from every note on disk: %v, %v, known %v", found, err, m.appStateKnown)
	}
	if slices.Contains(loadedSummaries(m), "done long ago") {
		t.Errorf("rebuilding the state should not keep every note: %q", loadedSummaries(m))
	}
	if want := noteBySummary(t, m, "active").Created; !saved.FirstNoteCreated.Equal(want) {
		t.Errorf("first note created = %v, want %v", saved.FirstNoteCreated, want)
	}
	if saved.Streak != 1 {
		t.Errorf("streak = %d, want 1 from yesterday's done note", saved.Streak)
	}
}

func TestCorruptStateShowsInTheHeaderThenIsOverwritten(t *testing.T) {
	now := time.Now()
	showGit := false
	cfg := &config.Config{DigestRoot: t.TempDir(), GreenOnly: true, ShowGit: &showGit, WorkDays: everyDay}
	for _, note := range dashboardNotes(now) {
		if err := store.New(cfg.NotesDir()).Save(note); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(cfg.Root(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(appstate.Path(cfg.Root()), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := NewModel(cfg, nil)
	m.width, m.height = 120, 50
	if header := stripANSI(headerSection{}.Render(m)); !strings.Contains(header, "state") {
		t.Errorf("a corrupt state file should show in the header: %q", header)
	}
	m = applyAllMessages(m, m.rebuildAppStateCmd())
	if saved, found, err := appstate.Load(m.cfg.Root()); err != nil || !found || saved.Streak != 1 {
		t.Errorf("the corrupt state should be overwritten from the notes: %+v %v %v", saved, found, err)
	}
}

func TestBragListsWeeksFromTheStoredFirstNoteDate(t *testing.T) {
	now := time.Now()
	first := time.Date(now.Year()-2, time.March, 4, 9, 0, 0, 0, time.Local)
	m := appStateModel(t, &appstate.State{FirstNoteCreated: first}, twoActiveNotes(now)...)
	if !m.firstNoteTime().Equal(first) {
		t.Fatalf("first note time = %v, want %v", m.firstNoteTime(), first)
	}
	rows := m.bragRows()
	if rows[len(rows)-1].year != first.Year() {
		t.Errorf("brag should reach back to %d before notes finish loading, last row year %d", first.Year(), rows[len(rows)-1].year)
	}
}
