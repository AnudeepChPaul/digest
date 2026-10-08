package tui

import (
	"os"
	"testing"

	"github.com/AnudeepChPaul/digest/pkg/automation"
)

func TestDeletingANoteRemovesItsAutomationFiles(t *testing.T) {
	m := noteSelectedModel(t)
	note := m.notes[0]
	if err := m.store.Save(note); err != nil {
		t.Fatal(err)
	}
	stateDir := automation.StateDir(m.automationRoot(), note.ID)
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(automation.LogPath(m.automationRoot(), note.ID), []byte("drafted from the note body\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m.deleteNotesCmd(note)()
	if _, err := os.Stat(automation.StateDir(m.automationRoot(), note.ID)); err == nil {
		t.Error("the deleted note's automation draft and log are still on disk")
	}
}
