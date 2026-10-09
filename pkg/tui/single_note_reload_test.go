package tui

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/automation"
	"github.com/AnudeepChPaul/digest/pkg/model"
	"github.com/AnudeepChPaul/digest/pkg/review"
)

func TestSaveRereadsOnlyTheSavedNote(t *testing.T) {
	m := syncTestModel(t)
	note := &model.Note{Summary: "padded", Body: "  body with spaces  \n", Status: model.StatusActive, Source: model.SourceManual, Created: time.Now()}
	other := &model.Note{ID: "other", Summary: "untouched"}
	m.notes = []*model.Note{note, other}
	m = update(m, m.saveNotesCmd(note)())
	if note.Body != "body with spaces" || note.FilePath == "" {
		t.Errorf("saved note should be replaced by its file on disk, got body %q path %q", note.Body, note.FilePath)
	}
	if len(m.notes) != 2 || m.notes[1] != other || other.Summary != "untouched" {
		t.Errorf("other notes should stay as they are: %+v", m.notes)
	}
}

func TestSaveKeepsAnEditMadeWhileSaving(t *testing.T) {
	m := syncTestModel(t)
	note := &model.Note{Summary: "first", Status: model.StatusActive, Source: model.SourceManual, Created: time.Now()}
	m.notes = []*model.Note{note}
	saved := m.saveNotesCmd(note)()
	note.Summary = "edited meanwhile"
	m = update(m, saved)
	if note.Summary != "edited meanwhile" || note.FilePath == "" {
		t.Errorf("an edit made during the save should survive, got %q path %q", note.Summary, note.FilePath)
	}
}

func approvedReviewNote(t *testing.T, m Model) *model.Note {
	t.Helper()
	approvedAt := time.Now().Add(-48 * time.Hour)
	note := &model.Note{Ref: prNoteRef("", "console", 6), Created: approvedAt, Updated: approvedAt, Status: model.StatusDone, Source: model.SourcePRReview, Summary: "Approved:console:6 Fix", Body: "from memory"}
	if err := m.store.Save(note); err != nil {
		t.Fatal(err)
	}
	return note
}

func TestBackgroundWriterOpensTheMatchedNoteByID(t *testing.T) {
	m := syncTestModel(t)
	onDisk := approvedReviewNote(t, m)
	inMemory := *onDisk
	m.notes, m.notesComplete = []*model.Note{&inMemory, {ID: "manual", Summary: "untouched"}}, true
	onDisk.Body = "edited on disk"
	if err := m.store.Save(onDisk); err != nil {
		t.Fatal(err)
	}
	pending := []GitPRItem{{Number: 6, Repository: "console"}}
	msg, ok := m.reopenApprovedNotesCmd(pending, time.Now())().(notesChangedMsg)
	if !ok || msg.err != nil || len(msg.notes) != 1 {
		t.Fatalf("msg = %#v", msg)
	}
	if reopened := msg.notes[0]; reopened.Status != model.StatusActive || reopened.Body != "edited on disk" {
		t.Errorf("the writer should reopen the note read from its own file, got %s %q", reopened.Status, reopened.Body)
	}
	m = update(m, msg)
	if len(m.notes) != 2 || inMemory.Status != model.StatusActive || m.notes[1].Summary != "untouched" {
		t.Errorf("only the reopened note should change in memory: %+v", m.notes)
	}
}

func TestBackgroundWriterReportsAGoneFile(t *testing.T) {
	m := syncTestModel(t)
	onDisk := approvedReviewNote(t, m)
	inMemory := *onDisk
	m.notes = []*model.Note{&inMemory}
	if err := m.store.Delete(onDisk); err != nil {
		t.Fatal(err)
	}
	msg, _ := m.reopenApprovedNotesCmd([]GitPRItem{{Number: 6, Repository: "console"}}, time.Now())().(notesChangedMsg)
	if !errors.Is(msg.err, os.ErrNotExist) || !strings.Contains(msg.err.Error(), inMemory.Ref) || len(msg.notes) != 0 {
		t.Fatalf("a gone file should be reported with its ref and not recreated, got %#v", msg)
	}
	m = update(m, msg)
	if len(m.messages) == 0 || m.messages[len(m.messages)-1].source != "STORE ERROR" || !strings.Contains(latestMessageText(m), inMemory.Ref) {
		t.Errorf("the gone file should show as a STORE ERROR naming %s, got %+v", inMemory.Ref, m.messages)
	}
	if notes, _ := m.store.List(); len(notes) != 0 {
		t.Errorf("the gone note was recreated: %+v", notes)
	}
}

func TestBackgroundWriterFindsANoteItWroteEarlier(t *testing.T) {
	m := syncTestModel(t)
	approval := []review.ActivityPR{{Number: 3, Title: "Once", URL: prRef("console", 3).URL, Repository: "console", State: "APPROVED", ReviewedAt: time.Now().Add(-time.Hour)}}
	first, second := m.reviewNotesCmd(approval), m.reviewNotesCmd(approval)
	first()
	second()
	if notes, _ := m.store.List(); len(notes) != 1 {
		t.Errorf("two writers started from the same memory should make one note, got %d", len(notes))
	}
}

func TestCreatedAutomationReloadsOnlyItsNote(t *testing.T) {
	m, _ := automationTestModel(t)
	note := &model.Note{Summary: "Create a ticket for flaky deploys", Status: model.StatusActive, Source: model.SourceManual, Created: time.Now()}
	if err := m.store.Save(note); err != nil {
		t.Fatal(err)
	}
	inMemory := *note
	m.notes = append(m.notes, &inMemory)
	note.Automated = "jira"
	if err := m.store.Save(note); err != nil {
		t.Fatal(err)
	}
	meta := automation.RunMeta{NoteID: note.ID, Automation: "jira", Phase: automation.PhaseCreate}
	m.automationRuns = map[string]automation.Run{note.ID: {Meta: meta, Status: automation.RunRunning, HasDraft: true}}
	_, cmd := m.handleReviewPoll(reviewPollSnapshot{automationRuns: map[string]automation.Run{note.ID: {Meta: meta, Status: automation.RunCreated}}})
	var changed *notesChangedMsg
	for _, msg := range collectMsgs(cmd) {
		switch typed := msg.(type) {
		case loadNotesMsg:
			t.Fatal("a created automation should not reload every note")
		case notesChangedMsg:
			changed = &typed
		}
	}
	if changed == nil || len(changed.notes) != 1 || changed.notes[0].Automated != "jira" {
		t.Fatalf("expected only the automated note to reload, got %#v", changed)
	}
}
