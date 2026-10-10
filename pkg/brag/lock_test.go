//go:build darwin

package brag

import (
	"context"
	"errors"
	"testing"

	"github.com/achandrapaul/digest/pkg/system"

	"golang.org/x/sys/unix"
)

func failingLocks(t *testing.T, root string) {
	t.Helper()
	system.Protect(root)
	original := system.ChangeFileFlags
	system.ChangeFileFlags = func(path string, flags int) error {
		if flags&unix.UF_IMMUTABLE != 0 {
			return unix.EPERM
		}
		return original(path, flags)
	}
	t.Cleanup(func() {
		system.ChangeFileFlags = original
		system.Protect("")
		_ = system.UnlockTree(root)
	})
}

func TestLockFailureStillSavesTheBrag(t *testing.T) {
	root := t.TempDir()
	failingLocks(t, root)
	stubCommand(t, "- Shipped the picker")
	week := WeekOf(localDate(2026, 9, 30))
	entry, err := Create(context.Background(), root, "claude", "prompt", week, "- shipped")
	if err != nil || entry == nil {
		t.Fatalf("a brag whose file only failed to lock should count as saved: %v", err)
	}
	if loaded, err := Load(root, week); err != nil || loaded.Summary != "- Shipped the picker" {
		t.Fatalf("brag file should be written: %+v %v", loaded, err)
	}
	stubCommand(t, "- Shipped it again")
	if _, err := Regenerate(context.Background(), root, "claude", "prompt", week); err != nil {
		t.Errorf("regenerate should also count a lock failure as saved: %v", err)
	}
}

func TestUnlockFailureKeepsTheOldBrag(t *testing.T) {
	root := t.TempDir()
	system.Protect(root)
	t.Cleanup(func() {
		system.Protect("")
		_ = system.UnlockTree(root)
	})
	stubCommand(t, "- First")
	week := WeekOf(localDate(2026, 9, 30))
	if _, err := Create(context.Background(), root, "claude", "prompt", week, "- shipped"); err != nil {
		t.Fatal(err)
	}
	original := system.ChangeFileFlags
	system.ChangeFileFlags = func(path string, flags int) error {
		if flags&unix.UF_IMMUTABLE == 0 {
			return unix.EPERM
		}
		return original(path, flags)
	}
	t.Cleanup(func() { system.ChangeFileFlags = original })
	stubCommand(t, "- Second")
	if _, err := Regenerate(context.Background(), root, "claude", "prompt", week); !errors.Is(err, system.ErrUnlock) {
		t.Fatalf("err = %v, want ErrUnlock", err)
	}
	if loaded, _ := Load(root, week); loaded == nil || loaded.Summary != "- First" {
		t.Errorf("old brag should be untouched: %+v", loaded)
	}
}
