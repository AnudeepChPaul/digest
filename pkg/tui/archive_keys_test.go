package tui

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/model"
	"github.com/achandrapaul/digest/pkg/store"

	tea "github.com/charmbracelet/bubbletea"
)

func archivedNotes(now time.Time) []*model.Note {
	return []*model.Note{
		{Summary: "archived newest", Status: model.StatusArchived, Source: model.SourceManual, Created: now.AddDate(0, 0, -3), Updated: now.Add(-1 * time.Hour)},
		{Summary: "archived middle", Status: model.StatusArchived, Source: model.SourceManual, Created: now.AddDate(0, 0, -4), Updated: now.Add(-2 * time.Hour)},
		{Summary: "archived oldest", Status: model.StatusArchived, Source: model.SourceManual, Created: now.AddDate(0, 0, -5), Updated: now.Add(-3 * time.Hour)},
	}
}

func openArchiveModel(t *testing.T) Model {
	t.Helper()
	m := noteSaveModel(t, archivedNotes(time.Now())...)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	m, _ = applyNoteMessages(t, next.(Model), cmd)
	if m.mode != ViewArchived || len(m.getArchivedNotes()) != 3 {
		t.Fatalf("mode %v archived %d", m.mode, len(m.getArchivedNotes()))
	}
	return m
}

func archivedSummaries(m Model) []string {
	var summaries []string
	for _, note := range m.getArchivedNotes() {
		summaries = append(summaries, note.Summary)
	}
	return summaries
}

func reloadedStatus(t *testing.T, m Model, summary string) (model.Status, bool) {
	t.Helper()
	notes, err := store.New(m.cfg.NotesDir()).List()
	if err != nil {
		t.Fatal(err)
	}
	for _, note := range notes {
		if note.Summary == summary {
			return note.Status, true
		}
	}
	return "", false
}

func TestArchiveJAndKMoveAndClamp(t *testing.T) {
	m := openArchiveModel(t)
	moves := []struct {
		key  string
		want int
	}{{"k", 0}, {"j", 1}, {"j", 2}, {"j", 2}, {"k", 1}, {"down", 2}, {"up", 1}}
	for _, move := range moves {
		msg := runes(move.key)
		switch move.key {
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		case "up":
			msg = tea.KeyMsg{Type: tea.KeyUp}
		}
		if m = press(t, m, msg); m.archivedSelected != move.want || m.mode != ViewArchived {
			t.Fatalf("after %s selected %d want %d mode %v", move.key, m.archivedSelected, move.want, m.mode)
		}
	}
}

func TestArchiveSpaceTogglesTheHighlightedNote(t *testing.T) {
	m := openArchiveModel(t)
	m = press(t, m, tea.KeyMsg{Type: tea.KeySpace})
	if !m.archivedSelectedMap[0] || len(m.archivedSelectedMap) != 1 {
		t.Fatalf("space should select row 0: %v", m.archivedSelectedMap)
	}
	m = press(t, m, runes("j"))
	m = press(t, m, tea.KeyMsg{Type: tea.KeySpace})
	if !m.archivedSelectedMap[0] || !m.archivedSelectedMap[1] {
		t.Fatalf("space should add row 1: %v", m.archivedSelectedMap)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeySpace})
	if m.archivedSelectedMap[1] || !m.archivedSelectedMap[0] {
		t.Errorf("second space should deselect row 1: %v", m.archivedSelectedMap)
	}
}

func confirmArchiveRestore(t *testing.T, m Model, key tea.KeyMsg) Model {
	t.Helper()
	m = press(t, m, key)
	if m.mode != ViewDeleteConfirm || !strings.Contains(strings.ToLower(stripANSI(m.View())), "restore") {
		t.Fatalf("%s should ask before restoring, mode %v", key, m.mode)
	}
	next, cmd := m.Update(runes("y"))
	m, _ = applyNoteMessages(t, next.(Model), cmd)
	return m
}

func TestArchiveRestoreCanBeCancelled(t *testing.T) {
	m := openArchiveModel(t)
	m = press(t, m, runes("u"))
	next, cmd := m.Update(runes("n"))
	m, _ = applyNoteMessages(t, next.(Model), cmd)
	if m.mode != ViewArchived || len(m.getArchivedNotes()) != 3 {
		t.Errorf("n should keep the archive as it was: mode %v archived %d", m.mode, len(m.getArchivedNotes()))
	}
}

func TestArchiveURestoresTheHighlightedNoteAsActiveOnTheViewedDay(t *testing.T) {
	for name, key := range map[string]tea.KeyMsg{"u": runes("u"), "enter": {Type: tea.KeyEnter}} {
		m := openArchiveModel(t)
		m.currentDate = m.currentDate.AddDate(0, 0, -1)
		m = press(t, m, runes("j"))
		m = confirmArchiveRestore(t, m, key)
		restored := noteBySummary(t, m, "archived middle")
		if restored.Status != model.StatusActive || !isSameDay(restored.Created, m.currentDate) {
			t.Errorf("%s: status %v created %v", name, restored.Status, restored.Created)
		}
		if got := strings.Join(archivedSummaries(m), ","); got != "archived newest,archived oldest" {
			t.Errorf("%s: archive still lists %s", name, got)
		}
		if status, found := reloadedStatus(t, m, "archived middle"); !found || status != model.StatusActive {
			t.Errorf("%s: saved status %v found %v", name, status, found)
		}
		if m.mode != ViewArchived {
			t.Errorf("%s: mode %v", name, m.mode)
		}
	}
}

func TestArchiveURestoresEverySelectedNoteAndClearsTheSelection(t *testing.T) {
	m := openArchiveModel(t)
	m = press(t, m, tea.KeyMsg{Type: tea.KeySpace})
	m = press(t, m, runes("j"))
	m = press(t, m, runes("j"))
	m = press(t, m, tea.KeyMsg{Type: tea.KeySpace})
	m = confirmArchiveRestore(t, m, runes("u"))
	if len(m.archivedSelectedMap) != 0 {
		t.Errorf("selection kept: %v", m.archivedSelectedMap)
	}
	if got := strings.Join(archivedSummaries(m), ","); got != "archived middle" {
		t.Errorf("archive lists %s", got)
	}
	for _, summary := range []string{"archived newest", "archived oldest"} {
		if status, _ := reloadedStatus(t, m, summary); status != model.StatusActive {
			t.Errorf("%s saved as %v", summary, status)
		}
	}
}

func TestArchiveDAsksThenDeletesTheFilesPermanently(t *testing.T) {
	m := openArchiveModel(t)
	target := m.getArchivedNotes()[0]
	m = press(t, m, runes("d"))
	if m.mode != ViewDeleteConfirm || len(m.deleteTargetNotes) != 1 || m.deleteTargetNotes[0] != target {
		t.Fatalf("mode %v targets %v", m.mode, m.deleteTargetNotes)
	}
	if !strings.Contains(strings.ToLower(stripANSI(m.View())), "delete") {
		t.Errorf("confirm should mention the delete")
	}
	next, cmd := m.Update(runes("y"))
	m, _ = applyNoteMessages(t, next.(Model), cmd)
	if m.mode != ViewArchived {
		t.Errorf("mode %v", m.mode)
	}
	if _, err := os.Stat(target.FilePath); !os.IsNotExist(err) {
		t.Errorf("note file kept: %v", err)
	}
	if _, found := reloadedStatus(t, m, "archived newest"); found {
		t.Errorf("deleted note still loads")
	}
	if got := strings.Join(archivedSummaries(m), ","); got != "archived middle,archived oldest" {
		t.Errorf("archive lists %s", got)
	}
}

func TestArchiveDDeletesEverySelectedNote(t *testing.T) {
	m := openArchiveModel(t)
	m = press(t, m, tea.KeyMsg{Type: tea.KeySpace})
	m = press(t, m, runes("j"))
	m = press(t, m, tea.KeyMsg{Type: tea.KeySpace})
	m = press(t, m, runes("d"))
	if len(m.deleteTargetNotes) != 2 {
		t.Fatalf("targets %d", len(m.deleteTargetNotes))
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = applyNoteMessages(t, next.(Model), cmd)
	if got := strings.Join(archivedSummaries(m), ","); got != "archived oldest" || len(m.archivedSelectedMap) != 0 {
		t.Errorf("archive lists %s selection %v", got, m.archivedSelectedMap)
	}
}

func TestArchiveEscAndCtrlECloseAndClearTheSelection(t *testing.T) {
	for name, key := range map[string]tea.KeyMsg{"esc": {Type: tea.KeyEsc}, "ctrl+e": {Type: tea.KeyCtrlE}} {
		m := openArchiveModel(t)
		m = press(t, m, tea.KeyMsg{Type: tea.KeySpace})
		m = press(t, m, key)
		if m.mode != ViewDashboard || len(m.archivedSelectedMap) != 0 {
			t.Errorf("%s: mode %v selection %v", name, m.mode, m.archivedSelectedMap)
		}
		if m.browsedNotes != nil {
			t.Errorf("%s: closing should drop the archive's notes", name)
		}
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
		m, _ = applyNoteMessages(t, next.(Model), cmd)
		if got := len(m.getArchivedNotes()); got != 3 {
			t.Errorf("%s: closing changed the archive, %d left", name, got)
		}
	}
}

func TestArchiveOnEmptyArchiveIgnoresActionKeys(t *testing.T) {
	m := noteSaveModel(t)
	m = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlE})
	for _, key := range []tea.KeyMsg{{Type: tea.KeySpace}, runes("u"), runes("d"), runes("j")} {
		if m = press(t, m, key); m.mode != ViewArchived || len(m.archivedSelectedMap) != 0 || m.archivedSelected != 0 {
			t.Errorf("%s on empty archive: mode %v", key.String(), m.mode)
		}
	}
}
