//go:build darwin

package system

import (
	"errors"
	"os"
	"path/filepath"
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
