//go:build darwin

package system

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func protectedRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	Protect(root, filepath.Join(root, "reviews"), filepath.Join(root, "config.yaml"))
	t.Cleanup(func() {
		Protect("")
		if err := UnlockTree(root); err != nil {
			t.Error(err)
		}
	})
	return root
}

func isLocked(t *testing.T, path string) bool {
	t.Helper()
	var stat unix.Stat_t
	if err := unix.Lstat(path, &stat); err != nil {
		t.Fatal(err)
	}
	return stat.Flags&unix.UF_IMMUTABLE != 0
}

func failFlagChanges(t *testing.T, failWhen func(flags int) bool) {
	t.Helper()
	original := ChangeFileFlags
	ChangeFileFlags = func(path string, flags int) error {
		if failWhen(flags) {
			return unix.EPERM
		}
		return original(path, flags)
	}
	t.Cleanup(func() { ChangeFileFlags = original })
}

func unlocking(flags int) bool { return flags&unix.UF_IMMUTABLE == 0 }
func locking(flags int) bool   { return flags&unix.UF_IMMUTABLE != 0 }

func TestProtectedFilesAreLockedAgainstOutsideChanges(t *testing.T) {
	root := protectedRoot(t)
	path := filepath.Join(root, "notes", "2026", "10", "note.md")
	if err := Write(path, []byte("original")); err != nil {
		t.Fatal(err)
	}
	if !isLocked(t, path) {
		t.Fatal("a protected file should be locked after writing")
	}
	if content, err := Read(path); err != nil || string(content) != "original" {
		t.Fatalf("a locked file should stay readable: %q %v", content, err)
	}
	if err := os.WriteFile(path, []byte("outside"), 0o600); err == nil {
		t.Error("an outside write should fail")
	}
	if err := os.Remove(path); err == nil {
		t.Error("an outside delete should fail")
	}
	if err := os.Rename(path, path+".moved"); err == nil {
		t.Error("an outside rename should fail")
	}
	if err := Write(path, []byte("edited")); err != nil {
		t.Fatalf("digest should still write: %v", err)
	}
	moved := filepath.Join(root, "notes", "2026", "10", "note.done.md")
	if err := Rename(path, moved); err != nil || !isLocked(t, moved) {
		t.Fatalf("rename should keep the file locked: %v", err)
	}
	if err := Append(filepath.Join(root, "cache", "events.txt"), []byte("line\n")); err != nil || !isLocked(t, filepath.Join(root, "cache", "events.txt")) {
		t.Errorf("appended files should be locked: %v", err)
	}
	if err := Remove(moved); err != nil || Exists(moved) {
		t.Errorf("digest should still delete: %v", err)
	}
}

func TestReviewsConfigAndOutsideFilesStayUnlocked(t *testing.T) {
	root := protectedRoot(t)
	outside := filepath.Join(t.TempDir(), "export.md")
	for _, path := range []string{filepath.Join(root, "reviews", ".state", "pr", "findings.json"), filepath.Join(root, "config.yaml"), outside} {
		if err := Write(path, []byte("x")); err != nil {
			t.Fatal(err)
		}
		if isLocked(t, path) {
			t.Errorf("%s should not be locked", path)
		}
	}
}

func TestLockingKeepsOtherFileFlags(t *testing.T) {
	root := protectedRoot(t)
	path := filepath.Join(root, "file")
	if err := Write(path, []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := setLocked(path, false); err != nil {
		t.Fatal(err)
	}
	if err := unix.Chflags(path, unix.UF_HIDDEN); err != nil {
		t.Fatal(err)
	}
	if err := setLocked(path, true); err != nil {
		t.Fatal(err)
	}
	var stat unix.Stat_t
	if err := unix.Lstat(path, &stat); err != nil || stat.Flags&unix.UF_HIDDEN == 0 || stat.Flags&unix.UF_IMMUTABLE == 0 {
		t.Errorf("flags = %#x, %v", stat.Flags, err)
	}
}

func TestUnlockFailureWritesNothing(t *testing.T) {
	root := protectedRoot(t)
	path := filepath.Join(root, "note.md")
	if err := Write(path, []byte("original")); err != nil {
		t.Fatal(err)
	}
	failFlagChanges(t, unlocking)
	if err := Write(path, []byte("edited")); !errors.Is(err, ErrUnlock) {
		t.Fatalf("err = %v, want ErrUnlock", err)
	}
	if err := Remove(path); !errors.Is(err, ErrUnlock) {
		t.Errorf("remove err = %v, want ErrUnlock", err)
	}
	if content, _ := Read(path); string(content) != "original" || !isLocked(t, path) {
		t.Errorf("file should be untouched and locked: %q", content)
	}
}

func TestLockFailureStillWrites(t *testing.T) {
	root := protectedRoot(t)
	path := filepath.Join(root, "note.md")
	failFlagChanges(t, locking)
	if err := Write(path, []byte("saved")); !errors.Is(err, ErrLock) {
		t.Fatalf("err = %v, want ErrLock", err)
	}
	if content, _ := Read(path); string(content) != "saved" || isLocked(t, path) {
		t.Errorf("file should be saved but unlocked: %q", content)
	}
}

func TestFailedWriteRelocks(t *testing.T) {
	root := protectedRoot(t)
	path := filepath.Join(root, "notes", "note.md")
	if err := Write(path, []byte("original")); err != nil {
		t.Fatal(err)
	}
	if err := setLocked(path, false); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatal(err)
	}
	if err := setLocked(path, true); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		setLocked(path, false)
		os.Chmod(path, 0o600)
	})
	if err := Write(path, []byte("edited")); err == nil || errors.Is(err, ErrLock) {
		t.Fatalf("err = %v, want a write failure", err)
	}
	if !isLocked(t, path) {
		t.Error("file should be locked again after a failed write")
	}
}

func TestRemoveAllAndLockTree(t *testing.T) {
	root := protectedRoot(t)
	for _, name := range []string{"brag/.state/w1/meta.json", "brag/w1.md", "notes/a.md", "reviews/x.json"} {
		if err := Write(filepath.Join(root, name), []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if err := UnlockTree(root); err != nil {
		t.Fatal(err)
	}
	locked, err := LockTree(root)
	if err != nil || locked != 3 {
		t.Fatalf("LockTree = %d, %v; reviews/ should be skipped", locked, err)
	}
	if err := RemoveAll(filepath.Join(root, "brag")); err != nil || Exists(filepath.Join(root, "brag")) {
		t.Errorf("RemoveAll should remove a locked tree: %v", err)
	}
}

func TestLogsStayUnlocked(t *testing.T) {
	root := protectedRoot(t)
	Protect(root, filepath.Join(root, "logs"))
	runLog := filepath.Join(root, "brag", ".state", "w1", "run.log")
	if err := Append(runLog, []byte("line\n")); err != nil {
		t.Fatal(err)
	}
	if isLocked(t, runLog) {
		t.Error("*.log files should stay unlocked")
	}
	jobLog := filepath.Join(root, "logs", "sync.dryrun.exit")
	file, err := OpenLog(jobLog)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("0"); err != nil {
		t.Fatal(err)
	}
	file.Close()
	if content, _ := Read(jobLog); string(content) != "0" || isLocked(t, jobLog) {
		t.Errorf("OpenLog file should be written directly and stay unlocked: %q", content)
	}
	if _, err := OpenLog(filepath.Join(root, "notes", "note.md")); err == nil {
		t.Error("OpenLog should refuse a locked path, since a detached process couldn't write it")
	}
}

func failFlagChangesFor(t *testing.T, failWhen func(path string, flags int) bool) {
	t.Helper()
	original := ChangeFileFlags
	ChangeFileFlags = func(path string, flags int) error {
		if failWhen(path, flags) {
			return unix.EPERM
		}
		return original(path, flags)
	}
	t.Cleanup(func() { ChangeFileFlags = original })
}

func writeAll(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, name := range names {
		if err := Write(filepath.Join(root, name), []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRemoveAllRemovesWhatItCanAndListsEveryUnlockFailure(t *testing.T) {
	root := protectedRoot(t)
	writeAll(t, root, "brag/a.md", "brag/b.md", "brag/sub/c.md")
	stuck := map[string]bool{filepath.Join(root, "brag", "a.md"): true, filepath.Join(root, "brag", "sub", "c.md"): true}
	failFlagChangesFor(t, func(path string, flags int) bool { return stuck[path] && unlocking(flags) })
	err := RemoveAll(filepath.Join(root, "brag"))
	if !errors.Is(err, ErrUnlock) {
		t.Fatalf("err = %v, want ErrUnlock", err)
	}
	for path := range stuck {
		if !strings.Contains(err.Error(), path) {
			t.Errorf("error should name %s: %v", path, err)
		}
		if !Exists(path) {
			t.Errorf("%s could not be unlocked, so it should still be there", path)
		}
	}
	if Exists(filepath.Join(root, "brag", "b.md")) {
		t.Error("b.md could be unlocked, so it should be removed")
	}
}

func TestLockTreeLocksWhatItCanAndCountsFailures(t *testing.T) {
	root := protectedRoot(t)
	writeAll(t, root, "notes/a.md", "notes/b.md", "notes/c.md")
	if err := UnlockTree(root); err != nil {
		t.Fatal(err)
	}
	unreadable := filepath.Join(root, "notes", "sealed")
	if err := os.Mkdir(unreadable, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(unreadable, 0o700) })
	stuck := filepath.Join(root, "notes", "b.md")
	failFlagChangesFor(t, func(path string, flags int) bool { return path == stuck && locking(flags) })
	locked, err := LockTree(root)
	if locked != 2 {
		t.Errorf("locked = %d, want 2", locked)
	}
	if err == nil || !strings.Contains(err.Error(), "2 locked, 2 failed") || !strings.Contains(err.Error(), stuck) || !strings.Contains(err.Error(), unreadable) {
		t.Errorf("err = %v, want 2 locked, 2 failed naming %s and %s", err, stuck, unreadable)
	}
	if !isLocked(t, filepath.Join(root, "notes", "a.md")) || isLocked(t, stuck) {
		t.Error("a.md should be locked and b.md not")
	}
}

func TestUnlockTreeListsEveryFailure(t *testing.T) {
	root := protectedRoot(t)
	writeAll(t, root, "notes/a.md", "notes/b.md")
	failFlagChanges(t, unlocking)
	err := UnlockTree(root)
	for _, name := range []string{"a.md", "b.md"} {
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("err = %v, want %s named", err, name)
		}
	}
}

func TestLockTreeOnASingleFile(t *testing.T) {
	root := protectedRoot(t)
	writeAll(t, root, "notes/a.md")
	path := filepath.Join(root, "notes", "a.md")
	if err := UnlockTree(path); err != nil {
		t.Fatal(err)
	}
	if locked, err := LockTree(path); locked != 1 || err != nil || !isLocked(t, path) {
		t.Errorf("LockTree = %d, %v", locked, err)
	}
	failFlagChanges(t, unlocking)
	if err := UnlockTree(path); err == nil || !strings.Contains(err.Error(), path) {
		t.Errorf("err = %v, want the file named", err)
	}
}

func TestUnlockFailureStopsAppendRenameAndChmod(t *testing.T) {
	root := protectedRoot(t)
	path := filepath.Join(root, "notes", "note.md")
	writeAll(t, root, "notes/note.md")
	failFlagChanges(t, unlocking)
	if err := Append(path, []byte("more")); !errors.Is(err, ErrUnlock) {
		t.Errorf("Append err = %v, want ErrUnlock", err)
	}
	if err := Rename(path, path+".moved"); !errors.Is(err, ErrUnlock) {
		t.Errorf("Rename err = %v, want ErrUnlock", err)
	}
	if err := Chmod(path, 0o640); !errors.Is(err, ErrUnlock) {
		t.Errorf("Chmod err = %v, want ErrUnlock", err)
	}
	if content, _ := Read(path); string(content) != "x" || !isLocked(t, path) {
		t.Errorf("file should be untouched and locked: %q", content)
	}
}

func TestChmodKeepsAProtectedFileLocked(t *testing.T) {
	root := protectedRoot(t)
	path := filepath.Join(root, "notes", "note.md")
	writeAll(t, root, "notes/note.md")
	if err := Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	if info, _ := Stat(path); info.Mode().Perm() != 0o640 || !isLocked(t, path) {
		t.Errorf("mode = %v, locked = %v", info.Mode().Perm(), isLocked(t, path))
	}
}
