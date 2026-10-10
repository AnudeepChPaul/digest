package tui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/model"

	tea "github.com/charmbracelet/bubbletea"
)

func browsedSummaries(m Model) []string {
	var names []string
	for _, note := range m.browsedNotes {
		names = append(names, note.Summary)
	}
	slices.Sort(names)
	return names
}

func applyNoteLoads(m Model, cmd tea.Cmd) Model {
	for _, msg := range collectMsgsWithoutTicks(cmd) {
		switch msg.(type) {
		case loadNotesMsg, browsedNotesMsg, appStateRebuiltMsg, closedThisWeekMsg:
			next, followUp := m.Update(msg)
			m = applyNoteLoads(next.(Model), followUp)
		}
	}
	return m
}

func applyInitNoteMessages(t *testing.T, m Model) Model {
	t.Helper()
	return applyNoteLoads(m, m.Init())
}

func TestStartupNeverLoadsEveryNote(t *testing.T) {
	m := applyInitNoteMessages(t, noteSaveModel(t, dashboardNotes(time.Now())...))
	if got := loadedSummaries(m); !slices.Equal(got, []string{"active", "done yesterday"}) {
		t.Errorf("notes after startup = %q", got)
	}
	if m.browsedNotes != nil {
		t.Errorf("startup should not keep every note: %q", browsedSummaries(m))
	}
}

func TestChangingDayDropsTheOtherDaysNotes(t *testing.T) {
	now := time.Now()
	m := noteSaveModel(t, dashboardNotes(now)...)
	m.currentDate = now.AddDate(0, 0, -19)
	m, _ = applyNoteMessages(t, m, m.loadNotesCmd)
	if !slices.Contains(loadedSummaries(m), "done long ago") {
		t.Fatalf("viewing the day after it was closed should load it: %q", loadedSummaries(m))
	}
	next, cmd := m.Update(runes("t"))
	for _, msg := range collectMsgsWithoutTicks(cmd) {
		if loaded, ok := msg.(loadNotesMsg); ok {
			next = update(next.(Model), loaded)
		}
	}
	if got := loadedSummaries(next.(Model)); !slices.Equal(got, []string{"active", "done yesterday"}) {
		t.Errorf("back on today the other days should be dropped: %q", got)
	}
}

func TestSearchArchiveAndBragReadEveryNoteAndDropThemOnClose(t *testing.T) {
	for name, open := range map[string]func(Model) (tea.Model, tea.Cmd){
		"archive": func(m Model) (tea.Model, tea.Cmd) { return m.openArchive(tea.KeyMsg{}) },
		"search":  func(m Model) (tea.Model, tea.Cmd) { return m.openSearch(tea.KeyMsg{}) },
		"brag":    func(m Model) (tea.Model, tea.Cmd) { return m.openBrag(tea.KeyMsg{}) },
	} {
		m := noteSaveModel(t, dashboardNotes(time.Now())...)
		next, cmd := open(m)
		m, _ = applyNoteMessages(t, next.(Model), cmd)
		if got := browsedSummaries(m); !slices.Equal(got, []string{"active", "archived", "done long ago", "done yesterday"}) {
			t.Errorf("%s should read every note from disk, got %q", name, got)
		}
		if got := loadedSummaries(m); !slices.Equal(got, []string{"active", "done yesterday"}) {
			t.Errorf("%s should leave the dashboard notes alone, got %q", name, got)
		}
		m = update(m, tea.KeyMsg{Type: tea.KeyEsc})
		if m.mode != ViewDashboard || m.browsedNotes != nil {
			t.Errorf("closing %s should drop its notes, mode %v kept %q", name, m.mode, browsedSummaries(m))
		}
	}
}

func TestNotesReadAfterTheViewClosedAreDropped(t *testing.T) {
	m := noteSaveModel(t, dashboardNotes(time.Now())...)
	next, cmd := m.openSearch(tea.KeyMsg{})
	m = update(next.(Model), tea.KeyMsg{Type: tea.KeyEsc})
	m, _ = applyNoteMessages(t, m, cmd)
	if m.browsedNotes != nil {
		t.Errorf("a load finishing after search closed should be dropped: %q", browsedSummaries(m))
	}
}

func TestSearchSharesTheDashboardNotes(t *testing.T) {
	m := noteSaveModel(t, dashboardNotes(time.Now())...)
	next, cmd := m.openSearch(tea.KeyMsg{})
	m, _ = applyNoteMessages(t, next.(Model), cmd)
	active := noteBySummary(t, m, "active")
	if !slices.Contains(m.browsedNotes, active) {
		t.Error("search should show the dashboard's own note so edits reach both")
	}
}

func TestRestoringFromTheArchivePutsTheNoteOnTheDashboard(t *testing.T) {
	m := noteSaveModel(t, dashboardNotes(time.Now())...)
	next, cmd := m.openArchive(tea.KeyMsg{})
	m, _ = applyNoteMessages(t, next.(Model), cmd)
	m = press(t, m, runes("u"))
	next, cmd = m.Update(runes("y"))
	m, _ = applyNoteMessages(t, next.(Model), cmd)
	if !slices.Contains(loadedSummaries(m), "archived") || len(m.getArchivedNotes()) != 0 {
		t.Errorf("restored note should move to the dashboard: dashboard %q archive %d", loadedSummaries(m), len(m.getArchivedNotes()))
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	if item, ok := m.selectedNavItem(); !slices.Contains(loadedSummaries(m), "archived") || !ok {
		t.Errorf("restored note should stay after closing the archive: %q %+v", loadedSummaries(m), item)
	}
}

func TestEditingAnOldNoteInSearchKeepsItOffTheDashboard(t *testing.T) {
	m := noteSaveModel(t, dashboardNotes(time.Now())...)
	next, cmd := m.openSearch(tea.KeyMsg{})
	m, _ = applyNoteMessages(t, next.(Model), cmd)
	var old *model.Note
	for _, note := range m.browsedNotes {
		if note.Summary == "done long ago" {
			old = note
		}
	}
	next, _ = m.beginNoteEdit(old, ViewSearch)
	m = next.(Model)
	m.editor.SetValue("done long ago\n\nfresh body")
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m, _ = applyNoteMessages(t, next.(Model), cmd)
	if old.Body != "fresh body" {
		t.Errorf("search copy body = %q", old.Body)
	}
	if slices.Contains(loadedSummaries(m), "done long ago") {
		t.Errorf("an old note should not join the dashboard: %q", loadedSummaries(m))
	}
}

func TestDeletingFromTheArchiveRemovesItFromTheArchiveList(t *testing.T) {
	m := noteSaveModel(t, dashboardNotes(time.Now())...)
	next, cmd := m.openArchive(tea.KeyMsg{})
	m, _ = applyNoteMessages(t, next.(Model), cmd)
	next, _ = m.Update(runes("d"))
	next, cmd = next.(Model).Update(runes("y"))
	m, _ = applyNoteMessages(t, next.(Model), cmd)
	if len(m.getArchivedNotes()) != 0 || slices.Contains(browsedSummaries(m), "archived") {
		t.Errorf("deleted note should leave the archive: %q", browsedSummaries(m))
	}
}

func TestHeaderCountsNotesClosedEarlierThisWeek(t *testing.T) {
	now := time.Now()
	weekStart := startOfDay(now).AddDate(0, 0, -((int(now.Weekday()) + 6) % 7))
	earlier := weekStart.Add(time.Hour)
	if !earlier.Before(startOfDay(now).AddDate(0, 0, -1)) {
		t.Skip("needs a day this week before yesterday")
	}
	m := noteSaveModel(t,
		&model.Note{Summary: "closed monday", Status: model.StatusDone, Source: model.SourceManual, Created: earlier, Updated: earlier},
		&model.Note{Summary: "closed today", Status: model.StatusDone, Source: model.SourceManual, Created: now, Updated: now},
	)
	m = applyInitNoteMessages(t, m)
	if slices.Contains(loadedSummaries(m), "closed monday") {
		t.Fatalf("the dashboard should not hold earlier days: %q", loadedSummaries(m))
	}
	if header := stripANSI(headerSection{}.Render(m)); !strings.Contains(header, "2 closed this week") {
		t.Errorf("header should count every note closed this week:\n%s", header)
	}
}

func TestPRNoteFinderReadsNotesThatAreNotInMemory(t *testing.T) {
	m := noteSaveModel(t, &model.Note{Summary: "Review console#6", Ref: "acme/console#6", Status: model.StatusDone, Source: model.SourcePRReview, Repo: "console", Created: time.Now().AddDate(0, -2, 0), Updated: time.Now().AddDate(0, -2, 0)})
	finder := newPRNoteFinder(m.store, "", m.prNoteSnapshot(""))
	note, found, err := finder.find("acme/console#6", "console", 6, reviewNoteID("console", 6))
	if err != nil || !found || note.Summary != "Review console#6" {
		t.Errorf("finder should read old PR notes from disk: %+v %v %v", note, found, err)
	}
}

func writeCorruptNote(t *testing.T, m Model) {
	t.Helper()
	dir := filepath.Join(m.cfg.NotesDir(), "2026", "10")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.md"), []byte("no front matter here"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCorruptNotesShowInTheHeaderAndTheGoodNotesStay(t *testing.T) {
	seed := noteSaveModel(t, dashboardNotes(time.Now())...)
	writeCorruptNote(t, seed)
	m := NewModel(seed.cfg, nil)
	m.width, m.height = 160, 50
	m = applyInitNoteMessages(t, m)
	if got := loadedSummaries(m); !slices.Equal(got, []string{"active", "done yesterday"}) {
		t.Errorf("good notes should still load: %q", got)
	}
	want := "✗ store: " + filepath.Join("2026", "10", "broken.md") + ": no front matter"
	if top := headerTopRow(m); !strings.Contains(top, want) {
		t.Errorf("header = %q, want %q", top, want)
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	m = applyMsgs(t, next.(Model), cmd)
	if got := loadedSummaries(m); !slices.Equal(got, []string{"active", "done yesterday"}) || !strings.Contains(headerTopRow(m), want) {
		t.Errorf("ctrl+r should keep the good notes and report the bad one: %q header %q", got, headerTopRow(m))
	}
	next, cmd = m.openSearch(tea.KeyMsg{})
	m, _ = applyNoteMessages(t, next.(Model), cmd)
	if len(m.browsedNotes) != 4 {
		t.Errorf("search should keep the good notes: %q", browsedSummaries(m))
	}
}
