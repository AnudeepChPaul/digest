package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/model"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCtrlRReloadsTheViewedDayAndKeepsTheSelection(t *testing.T) {
	m := syncTestModel(t)
	kept := &model.Note{Summary: "kept", Status: model.StatusActive, Source: model.SourceManual, Created: time.Now().Add(-time.Minute)}
	gone := &model.Note{Summary: "deleted elsewhere", Status: model.StatusActive, Source: model.SourceManual, Created: time.Now().Add(-2 * time.Minute)}
	oldDone := &model.Note{Summary: "done long ago", Status: model.StatusDone, Source: model.SourceManual, Created: time.Now().AddDate(0, 0, -40), Updated: time.Now().AddDate(0, 0, -40)}
	for _, note := range []*model.Note{kept, gone, oldDone} {
		if err := m.store.Save(note); err != nil {
			t.Fatal(err)
		}
	}
	m.notes, m.notesComplete = []*model.Note{kept, gone, oldDone}, true
	if err := m.store.Delete(gone); err != nil {
		t.Fatal(err)
	}
	added := &model.Note{Summary: "added elsewhere", Status: model.StatusActive, Source: model.SourceManual, Created: time.Now()}
	if err := m.store.Save(added); err != nil {
		t.Fatal(err)
	}
	m.selectNoteByID(kept.ID)

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	m = applyMsgs(t, next.(Model), cmd)
	var summaries []string
	for _, note := range m.notes {
		summaries = append(summaries, note.Summary)
	}
	joined := strings.Join(summaries, ",")
	if !strings.Contains(joined, "kept") || !strings.Contains(joined, "added elsewhere") || strings.Contains(joined, "deleted elsewhere") || !strings.Contains(joined, "done long ago") {
		t.Errorf("notes after ctrl+r = %v", summaries)
	}
	if item, ok := m.selectedNavItem(); !ok || item.Note == nil || item.Note.Summary != "kept" {
		t.Errorf("selection should stay on the kept note, got %+v", item)
	}
	if !strings.Contains(latestMessageText(m), "reloaded") {
		t.Errorf("header should say the notes reloaded, got %q", latestMessageText(m))
	}
}

func TestCtrlRIsListedOnTheShortcutsScreen(t *testing.T) {
	m := syncTestModel(t)
	binding, found := m.resolveKey(tea.KeyMsg{Type: tea.KeyCtrlR})
	if !found || binding.action != actionReloadNotes {
		t.Fatalf("ctrl+r should resolve to reload notes, got %v %v", found, binding.action)
	}
	var listed bool
	for _, entry := range m.helpEntries() {
		listed = listed || (entry.key == "ctrl+r" && entry.text == "reload notes")
	}
	if !listed {
		t.Error("ctrl+r should be listed on the ? screen")
	}
}
