//go:build darwin

package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/model"
	"github.com/achandrapaul/digest/pkg/system"

	"golang.org/x/sys/unix"
)

func lockedStore(t *testing.T) *NoteStore {
	t.Helper()
	root := t.TempDir()
	system.Protect(root)
	t.Cleanup(func() {
		system.Protect("")
		if err := system.UnlockTree(root); err != nil {
			t.Error(err)
		}
	})
	return New(root)
}

func isLocked(t *testing.T, path string) bool {
	t.Helper()
	var stat unix.Stat_t
	if err := unix.Lstat(path, &stat); err != nil {
		t.Fatal(err)
	}
	return stat.Flags&unix.UF_IMMUTABLE != 0
}

func savedLockedNote(t *testing.T, noteStore *NoteStore) *model.Note {
	t.Helper()
	note := &model.Note{Summary: "locked", Body: "original", Status: model.StatusActive, Source: model.SourceManual, Created: time.Now()}
	if err := noteStore.Save(note); err != nil {
		t.Fatal(err)
	}
	return note
}

func failFlagChanges(t *testing.T, failWhen func(flags int) bool) {
	t.Helper()
	original := system.ChangeFileFlags
	system.ChangeFileFlags = func(path string, flags int) error {
		if failWhen(flags) {
			return unix.EPERM
		}
		return original(path, flags)
	}
	t.Cleanup(func() { system.ChangeFileFlags = original })
}

func TestDigestStillChangesALockedNote(t *testing.T) {
	noteStore := lockedStore(t)
	note := savedLockedNote(t, noteStore)
	if !isLocked(t, note.FilePath) {
		t.Fatal("saved note should be locked")
	}
	note.Body = "edited"
	if err := noteStore.Save(note); err != nil {
		t.Fatalf("edit: %v", err)
	}
	if loaded, _ := Load(note.FilePath); loaded == nil || loaded.Body != "edited" || !isLocked(t, note.FilePath) {
		t.Fatalf("edited note should be saved and locked: %+v", loaded)
	}
	for _, status := range []model.Status{model.StatusDone, model.StatusArchived, model.StatusActive} {
		previousPath := note.FilePath
		note.Status, note.Updated = status, time.Now()
		if err := noteStore.Save(note); err != nil {
			t.Fatalf("%s: %v", status, err)
		}
		if !isLocked(t, note.FilePath) {
			t.Errorf("%s note should be locked", status)
		}
		if previousPath != note.FilePath {
			if _, err := os.Stat(previousPath); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("%s left the old file behind: %v", status, err)
			}
		}
	}
	if err := noteStore.Delete(note); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(note.FilePath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("deleted note is still there: %v", err)
	}
}

func TestSavingAMissingLockedNoteStillReportsItMissing(t *testing.T) {
	noteStore := lockedStore(t)
	note := savedLockedNote(t, noteStore)
	if err := system.Remove(note.FilePath); err != nil {
		t.Fatal(err)
	}
	if err := noteStore.Save(note); !errors.Is(err, ErrNoteFileMissing) {
		t.Fatalf("err = %v, want ErrNoteFileMissing", err)
	}
	if err := noteStore.Recreate(note); err != nil || !isLocked(t, note.FilePath) {
		t.Errorf("recreated note should be saved and locked: %v", err)
	}
}

func TestUnlockFailureWritesNothing(t *testing.T) {
	noteStore := lockedStore(t)
	note := savedLockedNote(t, noteStore)
	failFlagChanges(t, func(flags int) bool { return flags&unix.UF_IMMUTABLE == 0 })
	edited := *note
	edited.Body = "edited"
	if err := noteStore.Save(&edited); !errors.Is(err, ErrUnlockNote) {
		t.Fatalf("err = %v, want ErrUnlockNote", err)
	}
	if loaded, _ := Load(note.FilePath); loaded == nil || loaded.Body != "original" || !isLocked(t, note.FilePath) {
		t.Errorf("note should be untouched and locked: %+v", loaded)
	}
	if err := noteStore.Delete(note); !errors.Is(err, ErrUnlockNote) {
		t.Errorf("delete err = %v, want ErrUnlockNote", err)
	}
}

func TestLockFailureStillSavesTheNote(t *testing.T) {
	noteStore := lockedStore(t)
	note := savedLockedNote(t, noteStore)
	failFlagChanges(t, func(flags int) bool { return flags&unix.UF_IMMUTABLE != 0 })
	note.Body, note.Status = "edited", model.StatusDone
	if err := noteStore.Save(note); !errors.Is(err, ErrLockNote) {
		t.Fatalf("err = %v, want ErrLockNote", err)
	}
	if loaded, _ := Load(note.FilePath); loaded == nil || loaded.Body != "edited" || !IsDonePath(note.FilePath) {
		t.Errorf("note should be saved and renamed even though locking failed: %+v %s", loaded, note.FilePath)
	}
}

func TestFailedRenameLeavesTheNoteLocked(t *testing.T) {
	noteStore := lockedStore(t)
	note := savedLockedNote(t, noteStore)
	monthDir := filepath.Dir(note.FilePath)
	edited := *note
	edited.Status = model.StatusDone
	if err := os.Chmod(monthDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(monthDir, 0o700) })
	if err := noteStore.Save(&edited); err == nil || errors.Is(err, ErrLockNote) {
		t.Fatalf("err = %v, want a write failure", err)
	}
	if !isLocked(t, note.FilePath) {
		t.Error("note should be locked again after a failed save")
	}
}
