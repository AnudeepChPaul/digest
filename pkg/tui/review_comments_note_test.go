package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/model"
	"github.com/AnudeepChPaul/digest/pkg/review"
	"github.com/AnudeepChPaul/digest/pkg/store"
)

const changesPRURL = "https://github.com/o/console/pull/7"

func changesRecord(comments string, reviewedAt time.Time) review.ActivityPR {
	return review.ActivityPR{Number: 7, Title: "Fix", URL: changesPRURL, Repository: "console", State: "CHANGES_REQUESTED", ReviewedAt: reviewedAt, Comments: comments}
}

func loadNote(t *testing.T, noteStore *store.NoteStore, id string) *model.Note {
	t.Helper()
	notes, err := noteStore.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, note := range notes {
		if note.ID == id {
			return note
		}
	}
	t.Fatalf("no note %s", id)
	return nil
}

func TestRequestChangesNoteKeepsTheCommentsOnCreation(t *testing.T) {
	noteStore := store.New(t.TempDir())
	reviewedAt := time.Date(2026, 10, 5, 14, 0, 0, 0, time.Local)
	reviewNotesCmd(noteStore, []review.ActivityPR{changesRecord("Please add tests", reviewedAt)})()
	note := loadNote(t, noteStore, "console:7")
	if !strings.HasSuffix(note.Body, "\n\n"+changesPRURL) || !strings.Contains(note.Body, "Please add tests") || !strings.Contains(note.Body, "2026-10-05 14:00") {
		t.Errorf("body = %q", note.Body)
	}
	if prReviewNoteURL(note) != changesPRURL {
		t.Errorf("url = %q", prReviewNoteURL(note))
	}
}

func TestRequestChangesAppendsCommentsOnALaterRound(t *testing.T) {
	noteStore := store.New(t.TempDir())
	firstAt := time.Date(2026, 10, 5, 10, 0, 0, 0, time.Local)
	reviewNotesCmd(noteStore, []review.ActivityPR{changesRecord("First round", firstAt)})()
	reviewNotesCmd(noteStore, []review.ActivityPR{changesRecord("Second round see https://github.com/o/other/pull/9", firstAt.Add(time.Hour))})()
	note := loadNote(t, noteStore, "console:7")
	second := strings.Index(note.Body, "Second round")
	first := strings.Index(note.Body, "First round")
	if second < 0 || first < 0 || second > first {
		t.Errorf("want newest block first, body = %q", note.Body)
	}
	if !strings.HasSuffix(note.Body, "\n\n"+changesPRURL) {
		t.Errorf("url should stay at the bottom, body = %q", note.Body)
	}
	if historyLine := strings.Index(note.Body, "2026-10-05 10:00 "); historyLine < 0 || historyLine > second {
		t.Errorf("history line should stay above the blocks, body = %q", note.Body)
	}
	if prReviewNoteURL(note) != changesPRURL {
		t.Errorf("url = %q", prReviewNoteURL(note))
	}
}

func TestSyncedReviewWithoutCommentsAddsNoBlock(t *testing.T) {
	noteStore := store.New(t.TempDir())
	reviewNotesCmd(noteStore, []review.ActivityPR{changesRecord("", time.Now())})()
	if body := loadNote(t, noteStore, "console:7").Body; body != changesPRURL {
		t.Errorf("body = %q", body)
	}
}

func TestSubmittedRequestChangesCarriesThePostedComments(t *testing.T) {
	pr := review.QueuedPR{Ref: review.PRRef{Repo: "console", Number: 7, URL: changesPRURL}, Title: "Fix"}
	payload := review.Payload{Body: "Needs work", Comments: []review.ReviewComment{{Path: "a.go", Line: 3, Body: "nil check"}}}
	record := submittedReviewRecord(reviewSubmittedMsg{event: review.EventRequestChanges, pr: pr, payload: payload}, time.Now())
	for _, want := range []string{"Needs work", "a.go:3", "nil check"} {
		if !strings.Contains(record.Comments, want) {
			t.Errorf("comments miss %q: %q", want, record.Comments)
		}
	}
	approved := submittedReviewRecord(reviewSubmittedMsg{event: review.EventApprove, pr: pr, payload: payload}, time.Now())
	if approved.Comments != "" {
		t.Errorf("approve should not carry comments: %q", approved.Comments)
	}
}

func TestCommentsLandEvenWhenTheSyncedRecordArrivedFirst(t *testing.T) {
	noteStore := store.New(t.TempDir())
	syncedAt := time.Date(2026, 10, 5, 14, 0, 0, 0, time.Local)
	reviewNotesCmd(noteStore, []review.ActivityPR{changesRecord("", syncedAt)})()
	reviewNotesCmd(noteStore, []review.ActivityPR{changesRecord("Late comments", syncedAt.Add(10*time.Second))})()
	note := loadNote(t, noteStore, "console:7")
	if !strings.Contains(note.Body, "Late comments") || strings.Count(note.Body, "RequestedChanges") != 0 {
		t.Errorf("body = %q", note.Body)
	}
	reviewNotesCmd(noteStore, []review.ActivityPR{changesRecord("Late comments", syncedAt.Add(20*time.Second))})()
	if body := loadNote(t, noteStore, "console:7").Body; strings.Count(body, "Late comments") != 1 {
		t.Errorf("block duplicated: %q", body)
	}
}
