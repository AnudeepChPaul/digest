package tui

import (
	"testing"

	"app/pkg/automation"
	"app/pkg/model"
	"app/pkg/notify"
)

func TestNoteRowKeyUsesRawReminderAndRunState(t *testing.T) {
	var m Model
	note := &model.Note{ID: "note-1", Summary: "Ship it", Status: model.StatusActive}
	m.automationRuns = map[string]automation.Run{note.ID: {Status: automation.RunRunning}}
	m.notifyEntries = map[string]notify.Entry{note.ID: {NoteID: note.ID, Interval: "30m"}}
	running := m.noteRowCacheKey(note, false, 80)
	if running.automationStatus != automation.RunRunning || running.notifyInterval != "30m" {
		t.Errorf("key = %+v", running)
	}
	m.automationRuns = map[string]automation.Run{note.ID: {Status: automation.RunFailed}}
	if m.noteRowCacheKey(note, false, 80) == running {
		t.Errorf("failed run should change the key")
	}
	m.automationRuns = map[string]automation.Run{note.ID: {Status: automation.RunRunning}}
	m.notifyEntries = map[string]notify.Entry{note.ID: {NoteID: note.ID, Interval: "1h"}}
	if m.noteRowCacheKey(note, false, 80) == running {
		t.Errorf("new interval should change the key")
	}
}
