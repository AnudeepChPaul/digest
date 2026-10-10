package store

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/model"
)

func savedNote(t *testing.T, noteStore *NoteStore, note *model.Note) *model.Note {
	t.Helper()
	if err := noteStore.Save(note); err != nil {
		t.Fatal(err)
	}
	return note
}

func TestNewNoteFileIsNamedAfterItsLocalDateID(t *testing.T) {
	created := time.Date(2026, 10, 5, 23, 30, 0, 0, time.UTC)
	note := savedNote(t, New(t.TempDir()), &model.Note{Summary: "a", Status: model.StatusActive, Created: created})
	wantID := created.Local().Format("02-01-2006") + "-" + strconv.FormatInt(created.UnixMilli(), 10)
	if note.ID != wantID {
		t.Errorf("id = %q, want %q", note.ID, wantID)
	}
	if filepath.Base(note.FilePath) != wantID+".md" {
		t.Errorf("file = %q, want %s.md", note.FilePath, wantID)
	}
}

func TestStatusChangesRenameTheSameFileAndKeepTheID(t *testing.T) {
	noteStore := New(t.TempDir())
	note := savedNote(t, noteStore, &model.Note{Summary: "a", Status: model.StatusActive, Created: time.Now()})
	originalID, originalPath := note.ID, note.FilePath
	before, err := os.Stat(originalPath)
	if err != nil {
		t.Fatal(err)
	}
	finished := time.Now().Add(time.Hour)
	note.Status, note.Updated = model.StatusDone, finished
	savedNote(t, noteStore, note)
	wantDone := originalID + "-" + strconv.FormatInt(finished.UnixMilli(), 10) + ".done.md"
	if note.ID != originalID || filepath.Base(note.FilePath) != wantDone {
		t.Fatalf("done note = %q at %q, want %q", note.ID, note.FilePath, wantDone)
	}
	after, err := os.Stat(note.FilePath)
	if err != nil || !os.SameFile(before, after) {
		t.Errorf("done note should be the same file renamed: %v", err)
	}
	if _, err := os.Stat(originalPath); !os.IsNotExist(err) {
		t.Errorf("old file still there: %v", err)
	}

	refinished := finished.Add(time.Hour)
	note.Updated = refinished
	savedNote(t, noteStore, note)
	if want := originalID + "-" + strconv.FormatInt(refinished.UnixMilli(), 10) + ".done.md"; filepath.Base(note.FilePath) != want {
		t.Errorf("new finish time should rename: %q, want %q", note.FilePath, want)
	}

	note.Status = model.StatusArchived
	savedNote(t, noteStore, note)
	if want := originalID + "-" + strconv.FormatInt(refinished.UnixMilli(), 10) + ".archived.md"; filepath.Base(note.FilePath) != want {
		t.Errorf("archived file = %q, want %q", note.FilePath, want)
	}

	note.Status = model.StatusActive
	savedNote(t, noteStore, note)
	if filepath.Base(note.FilePath) != originalID+".md" {
		t.Errorf("reopened file = %q", note.FilePath)
	}
	entries, _ := os.ReadDir(filepath.Dir(note.FilePath))
	if len(entries) != 1 {
		t.Errorf("want exactly one file, got %d", len(entries))
	}
}

func TestSavingANoteWhoseFileIsGoneFailsUntilRecreated(t *testing.T) {
	noteStore := New(t.TempDir())
	note := savedNote(t, noteStore, &model.Note{Summary: "a", Status: model.StatusActive, Created: time.Now()})
	if err := os.Remove(note.FilePath); err != nil {
		t.Fatal(err)
	}
	note.Summary = "b"
	if err := noteStore.Save(note); !errors.Is(err, ErrNoteFileMissing) {
		t.Fatalf("err = %v, want ErrNoteFileMissing", err)
	}
	if _, err := os.Stat(note.FilePath); !os.IsNotExist(err) {
		t.Fatalf("save should not create a missing file: %v", err)
	}
	if err := noteStore.Recreate(note); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(note.FilePath)
	if err != nil || loaded.Summary != "b" {
		t.Errorf("recreated note = %+v, %v", loaded, err)
	}
}

func TestNewNotesNeverReuseTheIDOfARenamedNote(t *testing.T) {
	noteStore := New(t.TempDir())
	created := time.Now()
	first := savedNote(t, noteStore, &model.Note{Summary: "a", Status: model.StatusActive, Created: created})
	first.Status, first.Updated = model.StatusDone, created
	savedNote(t, noteStore, first)
	second := savedNote(t, noteStore, &model.Note{Summary: "b", Status: model.StatusActive, Created: created})
	if second.ID == first.ID {
		t.Errorf("second note reused id %q", first.ID)
	}
}

func TestNotesWithSpaceSeparatedDatesLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "01M3C68JAKBCQAHTEAEB3A644W-trying-something-new.md")
	content := "---\nid: 01M3C68JAKBCQAHTEAEB3A644W\nstatus: inbox\nsource: manual\ntags: []\ncreated: 2026-09-25 11:47:26.163310+00:00\nupdated: 2026-09-25 18:34:48.334663+00:00\ncompleted_at: null\n---\ntrying something new\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	note, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 9, 25, 11, 47, 26, 163310000, time.UTC); !note.Created.Equal(want) {
		t.Errorf("created = %v, want %v", note.Created, want)
	}
	if want := time.Date(2026, 9, 25, 18, 34, 48, 334663000, time.UTC); !note.Updated.Equal(want) {
		t.Errorf("updated = %v, want %v", note.Updated, want)
	}
}
