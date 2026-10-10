package review

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestDismissClearsAFailedRunButNotARunningOne(t *testing.T) {
	root := t.TempDir()
	ref := PRRef{Host: "github.com", Owner: "o", Repo: "console", Number: 1, URL: "https://github.com/o/console/pull/1"}
	dir := StateDir(root, ref)
	if err := WriteMeta(dir, Meta{Ref: ref}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, exitFile), []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Dismiss(root, ref); err != nil || len(ListRuns(root)) != 0 {
		t.Fatalf("a failed run should be dismissed: %v, runs %d", err, len(ListRuns(root)))
	}
	if err := os.WriteFile(filepath.Join(dir, pidFile), []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Dismiss(root, ref); err != ErrReviewRunning {
		t.Errorf("a running review should not be dismissed, err = %v", err)
	}
}
