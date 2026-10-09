package store

import (
	"errors"
	"testing"
	"time"

	"os"
	"path/filepath"

	"github.com/AnudeepChPaul/digest/pkg/model"
	"github.com/AnudeepChPaul/digest/pkg/paths"
)

func TestAutomatedKindRoundTrips(t *testing.T) {
	noteStore := New(t.TempDir())
	note := &model.Note{Summary: "Create a ticket for X", Status: model.StatusInbox, Source: model.SourceManual, Created: time.Now(), Automated: "ticket"}
	if err := noteStore.Save(note); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(note.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Automated != "ticket" {
		t.Errorf("automated = %q", loaded.Automated)
	}
}

func TestSavedNotesAreOwnerOnly(t *testing.T) {
	note := &model.Note{Summary: "private", Status: model.StatusInbox, Source: model.SourceManual, Created: time.Now()}
	if err := New(t.TempDir()).Save(note); err != nil {
		t.Fatal(err)
	}
	assertPrivateFile(t, note.FilePath)
}

func assertPrivateFile(t *testing.T, path string) {
	t.Helper()
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != paths.PrivateFileMode {
		t.Errorf("%s should be %o: %v %v", path, paths.PrivateFileMode, info, err)
	}
	if info, err := os.Stat(filepath.Dir(path)); err != nil || info.Mode().Perm() != paths.PrivateDirMode {
		t.Errorf("%s should be %o: %v %v", filepath.Dir(path), paths.PrivateDirMode, info, err)
	}
}

func TestLoadByIDFindsTheNoteInItsMonthFolderWhateverItsStatus(t *testing.T) {
	noteStore := New(t.TempDir())
	note := &model.Note{Summary: "find me", Status: model.StatusActive, Source: model.SourceManual, Created: time.Date(2026, 3, 4, 9, 0, 0, 0, time.Local)}
	if err := noteStore.Save(note); err != nil {
		t.Fatal(err)
	}
	if loaded, err := noteStore.LoadByID(note.ID); err != nil || loaded.Summary != "find me" || loaded.FilePath != note.FilePath {
		t.Fatalf("active note: %+v, %v", loaded, err)
	}
	note.Status, note.Updated = model.StatusDone, time.Now()
	if err := noteStore.Save(note); err != nil {
		t.Fatal(err)
	}
	if loaded, err := noteStore.LoadByID(note.ID); err != nil || loaded.Status != model.StatusDone || loaded.FilePath != note.FilePath {
		t.Fatalf("done note: %+v, %v", loaded, err)
	}
}

func TestLoadByIDReportsMissingAndLegacyIDs(t *testing.T) {
	noteStore := New(t.TempDir())
	for _, id := range []string{NoteID(time.Now()), "MyPR:acme:web:7"} {
		if _, err := noteStore.LoadByID(id); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("LoadByID(%q) error = %v, want os.ErrNotExist", id, err)
		}
	}
}

func TestLoadByIDFindsArchivedNotes(t *testing.T) {
	noteStore := New(t.TempDir())
	note := &model.Note{Summary: "archived one", Status: model.StatusActive, Source: model.SourceManual, Created: time.Now()}
	if err := noteStore.Save(note); err != nil {
		t.Fatal(err)
	}
	note.Status, note.Updated = model.StatusArchived, time.Now()
	if err := noteStore.Save(note); err != nil {
		t.Fatal(err)
	}
	if loaded, err := noteStore.LoadByID(note.ID); err != nil || loaded.Status != model.StatusArchived {
		t.Fatalf("archived note: %+v, %v", loaded, err)
	}
}

func TestLoadByIDFindsANoteOutsideItsMonthFolder(t *testing.T) {
	root := t.TempDir()
	noteStore := New(root)
	note := &model.Note{Summary: "moved", Status: model.StatusActive, Source: model.SourceManual, Created: time.Date(2026, 3, 4, 9, 0, 0, 0, time.Local)}
	if err := noteStore.Save(note); err != nil {
		t.Fatal(err)
	}
	otherMonth := filepath.Join(root, "2026", "05")
	if err := os.MkdirAll(otherMonth, 0o700); err != nil {
		t.Fatal(err)
	}
	movedPath := filepath.Join(otherMonth, filepath.Base(note.FilePath))
	if err := os.Rename(note.FilePath, movedPath); err != nil {
		t.Fatal(err)
	}
	if loaded, err := noteStore.LoadByID(note.ID); err != nil || loaded.FilePath != movedPath {
		t.Fatalf("moved note: %+v, %v", loaded, err)
	}
}
