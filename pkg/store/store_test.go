package store

import (
	"testing"
	"time"

	"app/pkg/model"
	"app/pkg/paths"
	"os"
	"path/filepath"
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
