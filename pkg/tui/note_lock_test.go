//go:build darwin

package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/model"
	"github.com/AnudeepChPaul/digest/pkg/store"
	"github.com/AnudeepChPaul/digest/pkg/system"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/sys/unix"
)

func lockedNoteModel(t *testing.T) Model {
	t.Helper()
	m := noteSaveModel(t, dashboardNotes(time.Now())...)
	system.Protect(m.cfg.Root(), m.cfg.ReviewRootDir())
	if _, err := system.LockTree(m.cfg.Root()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		system.Protect("")
		if err := system.UnlockTree(m.cfg.Root()); err != nil {
			t.Error(err)
		}
	})
	return m
}

func failNoteFlagChanges(t *testing.T, failWhen func(flags int) bool) func() {
	t.Helper()
	original := system.ChangeFileFlags
	system.ChangeFileFlags = func(path string, flags int) error {
		if failWhen(flags) {
			return unix.EPERM
		}
		return original(path, flags)
	}
	restore := func() { system.ChangeFileFlags = original }
	t.Cleanup(restore)
	return restore
}

func editAndSave(t *testing.T, m Model, note *model.Note, text string) Model {
	t.Helper()
	next, _ := m.beginNoteEdit(note, ViewDashboard)
	m = next.(Model)
	m.replaceEditorText(text)
	next, cmd := m.saveNote(tea.KeyMsg{})
	m, _ = applyNoteMessages(t, next.(Model), cmd)
	return m
}

func TestUnlockFailureKeepsTheEditSoSavingAgainRetries(t *testing.T) {
	m := lockedNoteModel(t)
	active := noteBySummary(t, m, "active")
	restore := failNoteFlagChanges(t, func(flags int) bool { return flags&unix.UF_IMMUTABLE == 0 })
	m = editAndSave(t, m, active, "active\n\ntyped while locked")
	if header := stripANSI(headerSection{}.Render(m)); !strings.Contains(header, "save failed") {
		t.Errorf("header should show the failed save: %q", header)
	}
	if active.Body != "typed while locked" {
		t.Fatalf("the edit should stay in memory, body = %q", active.Body)
	}
	if onDisk, _ := store.Load(active.FilePath); onDisk == nil || onDisk.Body == "typed while locked" {
		t.Fatalf("nothing should be written while unlocking fails: %+v", onDisk)
	}
	restore()
	m = editAndSave(t, m, active, "active\n\n"+active.Body)
	if onDisk, _ := store.Load(active.FilePath); onDisk == nil || onDisk.Body != "typed while locked" {
		t.Errorf("saving again should write the kept edit: %+v", onDisk)
	}
}

func TestLockFailureSavesAndWarns(t *testing.T) {
	m := lockedNoteModel(t)
	active := noteBySummary(t, m, "active")
	restore := failNoteFlagChanges(t, func(flags int) bool { return flags&unix.UF_IMMUTABLE != 0 })
	m = editAndSave(t, m, active, "active\n\nsaved unlocked")
	if header := stripANSI(headerSection{}.Render(m)); !strings.Contains(header, "saved but not locked") || strings.Contains(header, "save failed") {
		t.Errorf("header should warn the note isn't locked: %q", header)
	}
	if onDisk, _ := store.Load(active.FilePath); onDisk == nil || onDisk.Body != "saved unlocked" {
		t.Fatalf("the note should be saved: %+v", onDisk)
	}
	restore()
	m = editAndSave(t, m, active, "active\n\nsaved locked")
	var stat unix.Stat_t
	if err := unix.Lstat(active.FilePath, &stat); err != nil || stat.Flags&unix.UF_IMMUTABLE == 0 {
		t.Errorf("the next save should lock the note again: %v %#x", err, stat.Flags)
	}
}

func TestDeleteFailureKeepsTheNote(t *testing.T) {
	m := lockedNoteModel(t)
	active := noteBySummary(t, m, "active")
	restore := failNoteFlagChanges(t, func(flags int) bool { return flags&unix.UF_IMMUTABLE == 0 })
	m = update(m, m.deleteNotesCmd(active)())
	if header := stripANSI(headerSection{}.Render(m)); !strings.Contains(header, "delete failed") {
		t.Errorf("header should show the failed delete: %q", header)
	}
	noteBySummary(t, m, "active")
	restore()
	m = update(m, m.deleteNotesCmd(active)())
	for _, note := range m.notes {
		if note.Summary == "active" {
			t.Error("deleting again should remove the note")
		}
	}
}
