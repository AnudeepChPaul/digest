package tui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/config"
	"github.com/achandrapaul/digest/pkg/model"
	"github.com/achandrapaul/digest/pkg/store"

	tea "github.com/charmbracelet/bubbletea"
)

func noteSaveModel(t *testing.T, notes ...*model.Note) Model {
	t.Helper()
	showGit := false
	cfg := &config.Config{DigestRoot: t.TempDir(), GreenOnly: true, ShowGit: &showGit, WorkDays: everyDay}
	noteStore := store.New(cfg.NotesDir())
	for _, note := range notes {
		if err := noteStore.Save(note); err != nil {
			t.Fatal(err)
		}
	}
	m := NewModel(cfg, nil)
	m.width, m.height = 120, 50
	return m
}

func loadedSummaries(m Model) []string {
	var names []string
	for _, note := range m.notes {
		names = append(names, note.Summary)
	}
	slices.Sort(names)
	return names
}

func noteBySummary(t *testing.T, m Model, summary string) *model.Note {
	t.Helper()
	for _, note := range m.notes {
		if note.Summary == summary {
			return note
		}
	}
	t.Fatalf("no note %q in %v", summary, loadedSummaries(m))
	return nil
}

func selectSummary(t *testing.T, m *Model, summary string) {
	t.Helper()
	for index, item := range m.allNavItems() {
		if item.Note != nil && item.Note.Summary == summary {
			m.selected = index
			return
		}
	}
	t.Fatalf("no row %q", summary)
}

func savedMessages(cmd tea.Cmd) []tea.Msg {
	var found []tea.Msg
	for _, msg := range collectMsgs(cmd) {
		switch msg.(type) {
		case notesSavedMsg, notesDeletedMsg, loadNotesMsg, browsedNotesMsg:
			found = append(found, msg)
		}
	}
	return found
}

func applyNoteMessages(t *testing.T, m Model, cmd tea.Cmd) (Model, tea.Cmd) {
	t.Helper()
	var followUps []tea.Cmd
	for _, msg := range savedMessages(cmd) {
		next, followUp := m.Update(msg)
		m = next.(Model)
		followUps = append(followUps, followUp)
	}
	return m, tea.Batch(followUps...)
}

func dashboardNotes(now time.Time) []*model.Note {
	return []*model.Note{
		{Summary: "active", Status: model.StatusActive, Source: model.SourceManual, Created: now.AddDate(0, 0, -30), Updated: now.AddDate(0, 0, -30)},
		{Summary: "done yesterday", Status: model.StatusDone, Source: model.SourceManual, Created: now.AddDate(0, 0, -2), Updated: now.AddDate(0, 0, -1)},
		{Summary: "done long ago", Status: model.StatusDone, Source: model.SourceManual, Created: now.AddDate(0, 0, -21), Updated: now.AddDate(0, 0, -20)},
		{Summary: "archived", Status: model.StatusArchived, Source: model.SourceManual, Created: now.AddDate(0, 0, -5), Updated: now.AddDate(0, 0, -4)},
	}
}

func TestStartupReadsOnlyDashboardNotes(t *testing.T) {
	m := noteSaveModel(t, dashboardNotes(time.Now())...)
	if got := loadedSummaries(m); !slices.Equal(got, []string{"active", "done yesterday"}) {
		t.Errorf("startup notes = %q", got)
	}
	m, _ = applyNoteMessages(t, m, m.Init())
	if got := loadedSummaries(m); !slices.Equal(got, []string{"active", "done yesterday"}) || m.browsedNotes != nil {
		t.Errorf("startup should never load every note, notes = %q browsed = %d", got, len(m.browsedNotes))
	}
}

func TestADashboardReloadKeepsTheNotesSearchIsShowing(t *testing.T) {
	m := noteSaveModel(t, dashboardNotes(time.Now())...)
	next, cmd := m.openSearch(tea.KeyMsg{})
	m, _ = applyNoteMessages(t, next.(Model), cmd)
	partial := m.loadNotesCmd()
	m = update(m, loadNotesMsg{notes: partial.(loadNotesMsg).notes[:1]})
	if len(m.notes) != 1 || len(m.browsedNotes) != 4 {
		t.Errorf("a dashboard reload should only replace the dashboard notes: %q browsed %d", loadedSummaries(m), len(m.browsedNotes))
	}
}

func TestChangingDayReloadsTheDashboardNotesForThatDay(t *testing.T) {
	now := time.Now()
	m := noteSaveModel(t, dashboardNotes(now)...)
	m.currentDate = now.AddDate(0, 0, -18)
	next, cmd := m.Update(runes("p"))
	m = next.(Model)
	reloaded := false
	for _, msg := range collectMsgsWithoutTicks(cmd) {
		if loaded, ok := msg.(loadNotesMsg); ok {
			reloaded = true
			m = update(m, loaded)
		}
	}
	if !reloaded {
		t.Fatal("changing day should reload the dashboard notes")
	}
	if !slices.Contains(loadedSummaries(m), "done long ago") {
		t.Errorf("notes for the viewed day = %q", loadedSummaries(m))
	}
}

func TestSavingMergesTheNoteWithoutReloadingEveryNote(t *testing.T) {
	m := noteSaveModel(t, dashboardNotes(time.Now())...)
	active := noteBySummary(t, m, "active")
	selectSummary(t, &m, "active")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")})
	m = next.(Model)
	m, followUp := applyNoteMessages(t, m, cmd)
	if noteBySummary(t, m, "active") != active {
		t.Error("the in-memory note should be updated in place")
	}
	if !strings.HasSuffix(active.FilePath, ".done.md") {
		t.Errorf("merged file path = %q", active.FilePath)
	}
	for _, msg := range collectMsgsWithoutTicks(followUp) {
		if _, reloaded := msg.(loadNotesMsg); reloaded {
			t.Error("a save should not reload every note")
		}
	}
}

func TestDeletingRemovesTheNoteFromMemoryWithoutReloading(t *testing.T) {
	m := noteSaveModel(t, dashboardNotes(time.Now())...)
	next, cmd := m.openArchive(tea.KeyMsg{})
	m, _ = applyNoteMessages(t, next.(Model), cmd)
	archived := m.getArchivedNotes()[0]
	m.deleteTargetNotes, m.deleteReturnMode, m.mode = []*model.Note{archived}, ViewArchived, ViewDeleteConfirm
	next, cmd = m.Update(runes("y"))
	m = next.(Model)
	m, _ = applyNoteMessages(t, m, cmd)
	if slices.Contains(browsedSummaries(m), "archived") || len(m.browsedNotes) != 3 || len(m.notes) != 2 {
		t.Errorf("notes after delete = %q, archive list %q", loadedSummaries(m), browsedSummaries(m))
	}
	if _, err := os.Stat(archived.FilePath); !os.IsNotExist(err) {
		t.Errorf("file should be gone: %v", err)
	}
}

func TestAFailedSaveUndoesTheChangeAndShowsTheErrorInTheHeader(t *testing.T) {
	m := noteSaveModel(t, dashboardNotes(time.Now())...)
	active := noteBySummary(t, m, "active")
	if err := os.Chmod(active.FilePath, 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(active.FilePath, 0o600) })
	m.git.loadingGit = true
	m.postMessage(messageSourceGit, messageProgress, "syncing")
	selectSummary(t, &m, "active")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")})
	m = next.(Model)
	m, _ = applyNoteMessages(t, m, cmd)
	if active.Status != model.StatusActive {
		t.Errorf("failed save should undo the change, status = %s", active.Status)
	}
	if m.mode == ViewError {
		t.Error("a save error should show in the header, not a popup")
	}
	header := stripANSI(headerSection{}.Render(m))
	if !strings.Contains(header, "save failed") || strings.Contains(header, "syncing") {
		t.Errorf("header should show the error instead of the sync status: %q", header)
	}
	if m.headerAnimating() {
		t.Error("the sync spinner should stop while the error shows")
	}
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if header := stripANSI(headerSection{}.Render(m)); !strings.Contains(header, "syncing") || cmd == nil {
		t.Errorf("esc should bring the animated sync status back: %q", header)
	}
}

func removeNoteFile(t *testing.T, note *model.Note) {
	t.Helper()
	if err := os.Remove(note.FilePath); err != nil {
		t.Fatal(err)
	}
}

func TestAMissingFileOnARowSaveAsksInTheRowHint(t *testing.T) {
	for _, answer := range []string{"y", "n"} {
		m := noteSaveModel(t, dashboardNotes(time.Now())...)
		active := noteBySummary(t, m, "active")
		removeNoteFile(t, active)
		selectSummary(t, &m, "active")
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")})
		m = next.(Model)
		m, _ = applyNoteMessages(t, m, cmd)
		if m.mode != ViewRecreateRow || !strings.Contains(stripANSI(m.View()), "create it again") {
			t.Fatalf("row save of a missing file should ask in the row hint, mode = %v", m.mode)
		}
		next, cmd = m.Update(runes(answer))
		m = next.(Model)
		m, _ = applyNoteMessages(t, m, cmd)
		if m.mode != ViewDashboard {
			t.Errorf("%s: mode = %v, want the dashboard", answer, m.mode)
		}
		switch answer {
		case "y":
			loaded, err := store.Load(active.FilePath)
			if err != nil || loaded.Status != model.StatusDone {
				t.Errorf("y should recreate the note as saved: %+v %v", loaded, err)
			}
		case "n":
			if slices.Contains(loadedSummaries(m), "active") {
				t.Error("n should drop the note whose file is gone")
			}
		}
	}
}

func TestAMissingFileOnAPreviewSaveAsksInAModal(t *testing.T) {
	m := noteSaveModel(t, dashboardNotes(time.Now())...)
	active := noteBySummary(t, m, "active")
	removeNoteFile(t, active)
	next, _ := m.beginNoteEdit(active, ViewPreview)
	m = next.(Model)
	m.editor.SetValue("active\n\nnew body")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m = next.(Model)
	m, _ = applyNoteMessages(t, m, cmd)
	if m.mode != ViewRecreateConfirm || !strings.Contains(stripANSI(m.View()), "create it again") {
		t.Fatalf("preview save of a missing file should ask in a modal, mode = %v", m.mode)
	}
	next, cmd = m.Update(runes("y"))
	m = next.(Model)
	m, _ = applyNoteMessages(t, m, cmd)
	if loaded, err := store.Load(active.FilePath); err != nil || loaded.Body != "new body" {
		t.Errorf("recreated note = %+v %v", loaded, err)
	}
	if filepath.Base(active.FilePath) != store.FileName(active) {
		t.Errorf("recreated file = %q", active.FilePath)
	}
}

func collectMsgsWithoutTicks(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		if batch, ok := msg.(tea.BatchMsg); ok {
			var msgs []tea.Msg
			for _, inner := range batch {
				msgs = append(msgs, collectMsgsWithoutTicks(inner)...)
			}
			return msgs
		}
		return []tea.Msg{msg}
	case <-time.After(200 * time.Millisecond):
		return nil
	}
}

func TestArchiveSearchAndBragLoadEveryNoteWhenOpened(t *testing.T) {
	for name, open := range map[string]func(Model) (tea.Model, tea.Cmd){
		"archive": func(m Model) (tea.Model, tea.Cmd) { return m.openArchive(tea.KeyMsg{}) },
		"search":  func(m Model) (tea.Model, tea.Cmd) { return m.openSearch(tea.KeyMsg{}) },
		"brag":    func(m Model) (tea.Model, tea.Cmd) { return m.openBrag(tea.KeyMsg{}) },
	} {
		m := noteSaveModel(t, dashboardNotes(time.Now())...)
		next, cmd := open(m)
		m, _ = applyNoteMessages(t, next.(Model), cmd)
		if len(m.browsedNotes) != 4 || !slices.Contains(browsedSummaries(m), "archived") {
			t.Errorf("%s should load every note, got %q", name, browsedSummaries(m))
		}
	}
}

func TestDiscardingAMissingNoteFromThePreviewStaysInThePreview(t *testing.T) {
	for _, discardKey := range []tea.KeyMsg{runes("n"), {Type: tea.KeyEsc}} {
		now := time.Now()
		m := noteSaveModel(t, append(dashboardNotes(now), &model.Note{Summary: "later", Status: model.StatusActive, Source: model.SourceManual, Created: now, Updated: now})...)
		active := noteBySummary(t, m, "active")
		removeNoteFile(t, active)
		selectSummary(t, &m, "active")
		m.mode = ViewPreview
		next, _ := m.beginNoteEdit(active, ViewPreview)
		m = next.(Model)
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
		m, _ = applyNoteMessages(t, next.(Model), cmd)
		if m.mode != ViewRecreateConfirm {
			t.Fatalf("mode = %v", m.mode)
		}
		m = press(t, m, discardKey)
		if m.mode != ViewPreview || slices.Contains(loadedSummaries(m), "active") {
			t.Errorf("%s: mode = %v, want the preview without the note", discardKey, m.mode)
		}
	}
}
