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
	"golang.org/x/sys/unix"
)

func myPRMoment() time.Time { return time.Date(2026, 10, 6, 10, 0, 0, 0, time.Local) }

func corruptSeenFile(t *testing.T, seenPath string) {
	t.Helper()
	if err := os.WriteFile(seenPath, []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestMyPRNotesReportAnUnreadableSeenFile(t *testing.T) {
	seenPath := t.TempDir()
	msg, _ := myPRNotesCmd(store.New(t.TempDir()), seenPath, nil, nil, nil, myPRMoment())().(myPRNotesMsg)
	if msg.err == nil {
		t.Fatal("a seen path that is a directory should fail")
	}
}

func TestMyPRNotesRebuildSeenStateFromTheNoteAfterACorruptSeenFile(t *testing.T) {
	noteStore := store.New(t.TempDir())
	seenPath := filepath.Join(t.TempDir(), "seen.json")
	now := myPRMoment()
	pr := myPR(4, func(pr *review.QueuedPR) { pr.Body = "" })
	runMyPRNotes(t, noteStore, seenPath, []review.QueuedPR{pr}, nil, now)
	note := onlyMyPRNote(t, noteStore)
	if !strings.Contains(note.Body, "_No description._") {
		t.Fatalf("body:\n%s", note.Body)
	}
	note.Body += "\n- garbage\n- 2026-13-45 99:99 not a time\n- 2026-10-06 09:30 New comment"
	if err := noteStore.Save(note); err != nil {
		t.Fatal(err)
	}
	for _, extra := range []*model.Note{
		{Ref: "org/other#1", Source: model.SourceMyPR, Status: model.StatusActive, Summary: "bad link", Body: "not a url"},
		{Ref: "org/other#2", Source: model.SourceMyPR, Status: model.StatusDone, Summary: "done", Body: "https://github.com/org/other/pull/2"},
	} {
		if err := noteStore.Save(extra); err != nil {
			t.Fatal(err)
		}
	}
	corruptSeenFile(t, seenPath)
	commented := myPR(4, func(pr *review.QueuedPR) {
		pr.Body = ""
		pr.LastReplyAt = time.Date(2026, 10, 6, 9, 0, 0, 0, time.Local)
	})
	msg := runMyPRNotes(t, noteStore, seenPath, []review.QueuedPR{commented}, map[string]string{"https://github.com/org/never/pull/9": "CLOSED"}, now.Add(time.Hour))
	if len(msg.changed) != 0 {
		t.Fatalf("an older reply should not add an update, changed %+v", msg.changed)
	}
	if len(msg.known) != 1 {
		t.Fatalf("only the listed PR should be known, got %+v", msg.known)
	}
}

func TestMyPRNoteMarksADraftReadyForReview(t *testing.T) {
	noteStore := store.New(t.TempDir())
	seenPath := filepath.Join(t.TempDir(), "seen.json")
	now := myPRMoment()
	runMyPRNotes(t, noteStore, seenPath, []review.QueuedPR{myPR(4, func(pr *review.QueuedPR) { pr.IsDraft = true })}, nil, now)
	runMyPRNotes(t, noteStore, seenPath, []review.QueuedPR{myPR(4, nil)}, nil, now.Add(time.Hour))
	if note := onlyMyPRNote(t, noteStore); !strings.Contains(note.Body, "Marked ready for review") {
		t.Fatalf("body:\n%s", note.Body)
	}
}

func TestSplitMyPRBodyWithoutUpdates(t *testing.T) {
	if head, updates := splitMyPRBody("just a link\n\n"); head != "just a link" || updates != "" {
		t.Fatalf("head %q updates %q", head, updates)
	}
}

func TestMyPRNotesStopWhenANoteCannotBeSaved(t *testing.T) {
	msg, _ := myPRNotesCmd(readOnlyNoteStore(t), filepath.Join(t.TempDir(), "seen.json"), []review.QueuedPR{myPR(4, nil)}, nil, nil, myPRMoment())().(myPRNotesMsg)
	if msg.err == nil || msg.saved {
		t.Fatalf("msg = %#v", msg)
	}
}

func TestMyPRNotesKeepGoingWhenLockingFails(t *testing.T) {
	m := lockedNoteModel(t)
	failNoteFlagChanges(t, func(flags int) bool { return flags&unix.UF_IMMUTABLE != 0 })
	msg, _ := myPRNotesCmd(m.store, filepath.Join(t.TempDir(), "seen.json"), []review.QueuedPR{myPR(4, nil)}, nil, nil, myPRMoment())().(myPRNotesMsg)
	if msg.err != nil || !errors.Is(msg.missing, store.ErrLockNote) || !msg.saved {
		t.Fatalf("msg = %#v", msg)
	}
}

func TestMyPRNotesReportAnUnwritableSeenFile(t *testing.T) {
	seenDir := t.TempDir()
	if err := os.Chmod(seenDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(seenDir, 0o700) })
	msg, _ := myPRNotesCmd(store.New(t.TempDir()), filepath.Join(seenDir, "seen.json"), []review.QueuedPR{myPR(4, nil)}, nil, nil, myPRMoment())().(myPRNotesMsg)
	if msg.err == nil || !msg.saved {
		t.Fatalf("msg = %#v", msg)
	}
}

func TestClosingAMyPRNoteReportsSaveAndLookupFailures(t *testing.T) {
	noteStore := store.New(t.TempDir())
	seenPath := filepath.Join(t.TempDir(), "seen.json")
	now := myPRMoment()
	pr := myPR(4, nil)
	runMyPRNotes(t, noteStore, seenPath, []review.QueuedPR{pr}, nil, now)
	monthDir := filepath.Dir(onlyMyPRNote(t, noteStore).FilePath)
	if err := os.Chmod(monthDir, 0o500); err != nil {
		t.Fatal(err)
	}
	msg, _ := myPRNotesCmd(noteStore, seenPath, nil, map[string]string{pr.Ref.URL: "CLOSED"}, nil, now.Add(time.Hour))().(myPRNotesMsg)
	os.Chmod(monthDir, 0o700)
	if msg.err == nil {
		t.Fatalf("closing into a read-only dir should fail, msg = %#v", msg)
	}

	otherStore := store.New(t.TempDir())
	otherSeen := filepath.Join(t.TempDir(), "seen.json")
	runMyPRNotes(t, otherStore, otherSeen, []review.QueuedPR{pr}, nil, now)
	if err := os.Remove(onlyMyPRNote(t, otherStore).FilePath); err != nil {
		t.Fatal(err)
	}
	msg, _ = myPRNotesCmd(otherStore, otherSeen, nil, map[string]string{pr.Ref.URL: "MERGED"}, nil, now.Add(time.Hour))().(myPRNotesMsg)
	if msg.err != nil || !errors.Is(msg.missing, os.ErrNotExist) {
		t.Fatalf("a gone note should be reported as missing, msg = %#v", msg)
	}
}

func TestClosingAPRWithoutANoteOnlyForgetsIt(t *testing.T) {
	noteStore := store.New(t.TempDir())
	seenPath := filepath.Join(t.TempDir(), "seen.json")
	now := myPRMoment()
	pr := myPR(4, nil)
	runMyPRNotes(t, noteStore, seenPath, []review.QueuedPR{pr}, nil, now)
	deleted := onlyMyPRNote(t, noteStore)
	if err := noteStore.Delete(deleted); err != nil {
		t.Fatal(err)
	}
	forgetWrittenPRNote(noteStore, *deleted)
	msg := runMyPRNotes(t, noteStore, seenPath, nil, map[string]string{pr.Ref.URL: "CLOSED"}, now.Add(time.Hour))
	if len(msg.known) != 0 || msg.saved {
		t.Fatalf("msg = %#v", msg)
	}
}

func TestRebuildingSeenStateReportsAnUnusableStore(t *testing.T) {
	noteStore := store.New("$DIGEST_TEST_UNSET_ROOT_VARIABLE/notes")
	seenPath := filepath.Join(t.TempDir(), "seen.json")
	corruptSeenFile(t, seenPath)
	msg, _ := myPRNotesCmd(noteStore, seenPath, nil, nil, nil, myPRMoment())().(myPRNotesMsg)
	if msg.err != nil || msg.missing == nil {
		t.Fatalf("an unusable store should be reported as missing, msg = %#v", msg)
	}
}

func TestLegacyMyPRNoteGetsItsRef(t *testing.T) {
	noteStore := store.New(t.TempDir())
	seenPath := filepath.Join(t.TempDir(), "seen.json")
	pr := myPR(4, nil)
	legacy := &model.Note{ID: myPRNoteID(pr.Ref), Source: model.SourceMyPR, Status: model.StatusActive, Summary: "old", Body: pr.Ref.URL, Updated: myPRMoment().Add(-time.Hour)}
	if err := noteStore.Save(legacy); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(legacy.FilePath, filepath.Join(filepath.Dir(legacy.FilePath), legacy.ID+".md")); err != nil {
		t.Fatal(err)
	}
	msg := runMyPRNotes(t, noteStore, seenPath, []review.QueuedPR{pr}, nil, myPRMoment())
	if len(msg.changed) != 1 || msg.changed[0].Ref != prNoteRef(pr.Ref.Owner, pr.Ref.Repo, pr.Ref.Number) {
		t.Fatalf("the legacy note should get its ref: %+v", msg.changed)
	}
}

func TestKnownMyPRRefsAreSortedByURL(t *testing.T) {
	refs := knownMyPRRefs(map[string]myPRSeen{"b": {Ref: review.PRRef{URL: "b"}}, "a": {Ref: review.PRRef{URL: "a"}}})
	if len(refs) != 2 || refs[0].URL != "a" {
		t.Fatalf("refs = %+v", refs)
	}
}
