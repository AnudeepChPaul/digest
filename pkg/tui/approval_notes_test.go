package tui

import (
	"testing"
	"time"

	"app/pkg/model"
	"app/pkg/review"
	"app/pkg/store"
)

func TestApprovalNotesCreatedOnceWithRepoPRID(t *testing.T) {
	noteStore := store.New(t.TempDir())
	approvedAt := time.Date(2026, 10, 1, 15, 30, 0, 0, time.Local)
	existing := &model.Note{ID: "console:5", Summary: "Approved: Old", Status: model.StatusDone, Source: model.SourcePRReview}
	if err := noteStore.Save(existing); err != nil {
		t.Fatal(err)
	}
	approvals := []review.ActivityPR{
		{Number: 5, Title: "Old", URL: "https://github.com/o/console/pull/5", Repository: "console", State: "APPROVED", ReviewedAt: approvedAt},
		{Number: 6, Title: "Fix date picker", URL: "https://github.com/o/console/pull/6", Repository: "console", State: "APPROVED", ReviewedAt: approvedAt},
		{Number: 6, Title: "Fix date picker", URL: "https://github.com/o/console/pull/6", Repository: "console", State: "APPROVED", ReviewedAt: approvedAt},
	}
	for range 2 {
		msg := reviewNotesCmd(noteStore, approvals)()
		if loaded, ok := msg.(loadNotesMsg); !ok || loaded.err != nil {
			t.Fatalf("msg = %#v", msg)
		}
	}
	notes, err := noteStore.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 2 {
		t.Fatalf("notes = %d, want 2", len(notes))
	}
	var created *model.Note
	for _, note := range notes {
		if note.ID == "console:6" {
			created = note
		}
	}
	if created == nil {
		t.Fatalf("no console:6 note")
	}
	if created.Summary != "Approved: Fix date picker" || created.Status != model.StatusDone || created.Source != model.SourcePRReview || created.Repo != "console" {
		t.Errorf("note = %+v", created)
	}
	if !created.Updated.Equal(approvedAt) || created.Body != "https://github.com/o/console/pull/6" {
		t.Errorf("updated=%v body=%q", created.Updated, created.Body)
	}
}

func TestApprovalNotesExactIDMatch(t *testing.T) {
	noteStore := store.New(t.TempDir())
	if err := noteStore.Save(&model.Note{ID: "console:19162", Summary: "x", Status: model.StatusDone}); err != nil {
		t.Fatal(err)
	}
	approvals := []review.ActivityPR{{Number: 1, Title: "One", URL: "https://github.com/o/console/pull/1", Repository: "console", State: "APPROVED", ReviewedAt: time.Now()}}
	reviewNotesCmd(noteStore, approvals)()
	notes, _ := noteStore.List()
	if len(notes) != 2 {
		t.Errorf("console:1 was treated as existing; notes = %d", len(notes))
	}
}

func TestApprovalNotesNothingToDo(t *testing.T) {
	if cmd := reviewNotesCmd(store.New(t.TempDir()), nil); cmd != nil {
		t.Errorf("want nil cmd without approvals")
	}
}
