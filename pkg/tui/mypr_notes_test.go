package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/model"
	"github.com/achandrapaul/digest/pkg/review"
	"github.com/achandrapaul/digest/pkg/store"
)

func myPR(number int, mutate func(*review.QueuedPR)) review.QueuedPR {
	pr := review.QueuedPR{
		Ref:       review.PRRef{Host: "github.com", Owner: "org", Repo: "console", Number: number, URL: "https://github.com/org/console/pull/" + string(rune('0'+number))},
		Title:     "Fix picker",
		Body:      "Fixes the date picker",
		Author:    "me",
		HeadRef:   "fix/picker",
		CreatedAt: time.Date(2026, 10, 6, 9, 0, 0, 0, time.Local),
	}
	if mutate != nil {
		mutate(&pr)
	}
	return pr
}

func runMyPRNotes(t *testing.T, noteStore *store.NoteStore, seenPath string, prs []review.QueuedPR, closed map[string]string, now time.Time) myPRNotesMsg {
	t.Helper()
	msg, ok := myPRNotesCmd(noteStore, seenPath, prs, closed, nil, now)().(myPRNotesMsg)
	if !ok || msg.err != nil {
		t.Fatalf("msg = %#v", msg)
	}
	return msg
}

func onlyMyPRNote(t *testing.T, noteStore *store.NoteStore) *model.Note {
	t.Helper()
	notes, err := noteStore.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 {
		t.Fatalf("notes = %d, want 1", len(notes))
	}
	return notes[0]
}

func TestMyPRNoteCreatedWithLinkDescriptionAndUpdates(t *testing.T) {
	noteStore := store.New(t.TempDir())
	seenPath := filepath.Join(t.TempDir(), "seen.json")
	now := time.Date(2026, 10, 6, 10, 0, 0, 0, time.Local)
	pr := myPR(4, func(pr *review.QueuedPR) {
		pr.CIState = "SUCCESS"
		pr.Reviews = []review.PRReview{{Author: "alice", State: "APPROVED", SubmittedAt: now.Add(-time.Minute)}}
	})
	msg := runMyPRNotes(t, noteStore, seenPath, []review.QueuedPR{pr}, nil, now)
	note := onlyMyPRNote(t, noteStore)
	if note.Ref != "org/console#4" || note.ID != store.NoteID(pr.CreatedAt) || !note.Created.Equal(pr.CreatedAt) || note.Summary != "NewPR: org:console:4: Fix picker" || note.Source != model.SourceMyPR || note.Status != model.StatusActive || note.Repo != "console" {
		t.Errorf("note = %+v", note)
	}
	lines := strings.Split(note.Body, "\n")
	if lines[0] != pr.Ref.URL {
		t.Errorf("first body line = %q, want the PR link", lines[0])
	}
	want := pr.Ref.URL + "\n\n## Description\n\nFixes the date picker\n\n## Updates\n\n" +
		"- 2026-10-06 10:00 CI passing\n" +
		"- 2026-10-06 09:59 Approved by @alice\n" +
		"- 2026-10-06 09:00 Opened"
	if note.Body != want {
		t.Errorf("body =\n%s\nwant\n%s", note.Body, want)
	}
	if len(msg.known) != 1 || msg.known[0].URL != pr.Ref.URL {
		t.Errorf("known = %+v", msg.known)
	}
}

func TestMyPRNoteUnchangedPRDoesNotRewrite(t *testing.T) {
	noteStore := store.New(t.TempDir())
	seenPath := filepath.Join(t.TempDir(), "seen.json")
	now := time.Date(2026, 10, 6, 10, 0, 0, 0, time.Local)
	pr := myPR(4, nil)
	runMyPRNotes(t, noteStore, seenPath, []review.QueuedPR{pr}, nil, now)
	before := onlyMyPRNote(t, noteStore)
	msg := runMyPRNotes(t, noteStore, seenPath, []review.QueuedPR{pr}, nil, now.Add(time.Hour))
	after := onlyMyPRNote(t, noteStore)
	if msg.saved || after.Body != before.Body || !after.Updated.Equal(before.Updated) {
		t.Errorf("unchanged PR rewrote the note: saved=%v\n%s", msg.saved, after.Body)
	}
}

func TestMyPRNoteAppendsNewestFirstAndReplacesDescription(t *testing.T) {
	noteStore := store.New(t.TempDir())
	seenPath := filepath.Join(t.TempDir(), "seen.json")
	now := time.Date(2026, 10, 6, 10, 0, 0, 0, time.Local)
	runMyPRNotes(t, noteStore, seenPath, []review.QueuedPR{myPR(4, nil)}, nil, now)
	later := now.Add(2 * time.Hour)
	changed := myPR(4, func(pr *review.QueuedPR) {
		pr.Title = "Fix picker for real"
		pr.Body = "New description"
		pr.CIState = "FAILURE"
		pr.LastReplyAt = later.Add(-30 * time.Minute)
		pr.Reviews = []review.PRReview{
			{Author: "bob", State: "CHANGES_REQUESTED", SubmittedAt: later.Add(-time.Hour)},
			{Author: "me", State: "COMMENTED", SubmittedAt: later.Add(-time.Hour)},
			{Author: "copilot-pull-request-reviewer[bot]", State: "COMMENTED", SubmittedAt: later.Add(-time.Hour)},
		}
	})
	runMyPRNotes(t, noteStore, seenPath, []review.QueuedPR{changed}, nil, later)
	note := onlyMyPRNote(t, noteStore)
	want := changed.Ref.URL + "\n\n## Description\n\nNew description\n\n## Updates\n\n" +
		"- 2026-10-06 12:00 Title changed to \"Fix picker for real\"\n" +
		"- 2026-10-06 12:00 Description updated\n" +
		"- 2026-10-06 12:00 CI failing\n" +
		"- 2026-10-06 11:30 New comment\n" +
		"- 2026-10-06 11:00 Changes requested by @bob\n" +
		"- 2026-10-06 09:00 Opened"
	if note.Body != want {
		t.Errorf("body =\n%s\nwant\n%s", note.Body, want)
	}
	if note.Summary != "NewPR: org:console:4: Fix picker for real" {
		t.Errorf("summary = %q", note.Summary)
	}
}

func TestMyPRNoteClosedMarksDoneAndForgetsPR(t *testing.T) {
	noteStore := store.New(t.TempDir())
	seenPath := filepath.Join(t.TempDir(), "seen.json")
	now := time.Date(2026, 10, 6, 10, 0, 0, 0, time.Local)
	pr := myPR(4, nil)
	runMyPRNotes(t, noteStore, seenPath, []review.QueuedPR{pr}, nil, now)
	msg := runMyPRNotes(t, noteStore, seenPath, nil, map[string]string{pr.Ref.URL: "MERGED"}, now.Add(time.Hour))
	note := onlyMyPRNote(t, noteStore)
	if note.Status != model.StatusDone || !strings.Contains(note.Body, "## Updates\n\n- 2026-10-06 11:00 Merged\n- 2026-10-06 09:00 Opened") {
		t.Errorf("status=%v body=\n%s", note.Status, note.Body)
	}
	if !strings.Contains(note.Body, "## Description\n\nFixes the date picker") {
		t.Errorf("description lost on close:\n%s", note.Body)
	}
	if len(msg.known) != 0 {
		t.Errorf("closed PR still known: %+v", msg.known)
	}
}

func TestMyPRNoteDeletedInAppIsNotRecreated(t *testing.T) {
	noteStore := store.New(t.TempDir())
	seenPath := filepath.Join(t.TempDir(), "seen.json")
	now := time.Date(2026, 10, 6, 10, 0, 0, 0, time.Local)
	pr := myPR(4, nil)
	runMyPRNotes(t, noteStore, seenPath, []review.QueuedPR{pr}, nil, now)
	deleted := onlyMyPRNote(t, noteStore)
	if err := noteStore.Delete(deleted); err != nil {
		t.Fatal(err)
	}
	forgetWrittenPRNote(noteStore, *deleted)
	approved := myPR(4, func(pr *review.QueuedPR) {
		pr.Reviews = []review.PRReview{{Author: "alice", State: "APPROVED", SubmittedAt: now.Add(time.Minute)}}
	})
	runMyPRNotes(t, noteStore, seenPath, []review.QueuedPR{approved}, nil, now.Add(time.Hour))
	if notes, _ := noteStore.List(); len(notes) != 0 {
		t.Errorf("deleted note came back: %+v", notes)
	}
}

func TestMyPRNoteDeletedOutsideAppIsReported(t *testing.T) {
	noteStore := store.New(t.TempDir())
	seenPath := filepath.Join(t.TempDir(), "seen.json")
	now := time.Date(2026, 10, 6, 10, 0, 0, 0, time.Local)
	pr := myPR(4, nil)
	runMyPRNotes(t, noteStore, seenPath, []review.QueuedPR{pr}, nil, now)
	gone := onlyMyPRNote(t, noteStore)
	if err := os.Remove(gone.FilePath); err != nil {
		t.Fatal(err)
	}
	msg, _ := myPRNotesCmd(noteStore, seenPath, []review.QueuedPR{pr}, nil, nil, now.Add(time.Hour))().(myPRNotesMsg)
	if msg.err != nil || !errors.Is(msg.missing, os.ErrNotExist) || !strings.Contains(msg.missing.Error(), gone.Ref) {
		t.Fatalf("a gone file should be reported as missing with its ref, got %#v", msg)
	}
	if notes, _ := noteStore.List(); len(notes) != 0 {
		t.Errorf("gone note came back: %+v", notes)
	}
}

func TestMyPRNoteKeepsPRsFromFailedHostsKnown(t *testing.T) {
	noteStore := store.New(t.TempDir())
	seenPath := filepath.Join(t.TempDir(), "seen.json")
	now := time.Date(2026, 10, 6, 10, 0, 0, 0, time.Local)
	pr := myPR(4, nil)
	runMyPRNotes(t, noteStore, seenPath, []review.QueuedPR{pr}, nil, now)
	msg, _ := myPRNotesCmd(noteStore, seenPath, nil, nil, []string{"github.com"}, now.Add(time.Hour))().(myPRNotesMsg)
	if len(msg.known) != 1 {
		t.Errorf("PR on a failed host was forgotten: %+v", msg.known)
	}
}
