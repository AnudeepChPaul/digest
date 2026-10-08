package review

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
)

func TestPrepareFreshClone(t *testing.T) {
	root := t.TempDir()
	ref := PRRef{Host: "github.com", Owner: "o", Repo: "console", Number: 9}
	stale := filepath.Join(CloneDir(root, ref), "stale.txt")
	writeFile(t, stale, "old")

	var calls []string
	original := runStep
	runStep = func(ctx context.Context, output io.Writer, dir string, name string, args ...string) error {
		calls = append(calls, dir+"|"+name+" "+strings.Join(args, " "))
		if name == "gh" && args[0] == "repo" {
			return os.MkdirAll(args[3], 0755)
		}
		return nil
	}
	defer func() { runStep = original }()

	dir, err := Prepare(context.Background(), ref, root, log.New(os.Stderr))
	if err != nil {
		t.Fatal(err)
	}
	if dir != CloneDir(root, ref) {
		t.Errorf("dir = %q", dir)
	}
	if _, err := os.Stat(stale); err == nil {
		t.Errorf("stale clone was not removed")
	}
	if len(calls) < 2 {
		t.Fatalf("calls = %v", calls)
	}
	partial := dir + partialCloneSuffix
	if !strings.HasPrefix(calls[0], root+"|gh repo clone github.com/o/console "+partial) {
		t.Errorf("clone call = %q", calls[0])
	}
	if calls[1] != partial+"|gh pr checkout 9" {
		t.Errorf("checkout call = %q", calls[1])
	}
	if _, err := os.Stat(partial); err == nil {
		t.Errorf("partial clone left behind")
	}
}

func TestCloneExistsOnlyAfterCheckoutSucceeds(t *testing.T) {
	root := t.TempDir()
	ref := PRRef{Host: "github.com", Owner: "o", Repo: "console", Number: 9}
	if CloneExists(root, ref) {
		t.Fatal("no clone yet")
	}
	original := runStep
	runStep = func(ctx context.Context, output io.Writer, dir string, name string, args ...string) error {
		if name == "gh" && args[0] == "repo" {
			return os.MkdirAll(filepath.Join(args[3], ".git"), 0755)
		}
		if CloneExists(root, ref) {
			t.Errorf("clone reported ready during %s %v", name, args)
		}
		if name == "gh" && args[0] == "pr" {
			return errors.New("checkout failed")
		}
		return nil
	}
	defer func() { runStep = original }()
	if _, err := Prepare(context.Background(), ref, root, log.New(io.Discard)); err == nil {
		t.Fatal("checkout failure should fail the clone")
	}
	if CloneExists(root, ref) {
		t.Error("a failed checkout must not leave a ready clone")
	}
	recordSteps(t, "")
	if _, err := Prepare(context.Background(), ref, root, log.New(io.Discard)); err != nil {
		t.Fatal(err)
	}
	if !CloneExists(root, ref) {
		t.Error("clone should be ready after checkout")
	}
}

func TestFreshCloneClearsALeftoverPartialFolder(t *testing.T) {
	root := t.TempDir()
	ref := PRRef{Host: "github.com", Owner: "o", Repo: "console", Number: 9}
	leftover := filepath.Join(CloneDir(root, ref)+partialCloneSuffix, "leftover.txt")
	writeFile(t, leftover, "old")
	recordSteps(t, "")
	if _, err := Prepare(context.Background(), ref, root, log.New(io.Discard)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(CloneDir(root, ref), "leftover.txt")); err == nil {
		t.Error("leftover partial files ended up in the clone")
	}
}

func recordSteps(t *testing.T, failOn string) *[]string {
	t.Helper()
	var calls []string
	original := runStep
	runStep = func(ctx context.Context, output io.Writer, dir string, name string, args ...string) error {
		call := name + " " + strings.Join(args, " ")
		calls = append(calls, call)
		if failOn != "" && strings.HasPrefix(call, failOn) {
			return errors.New("step failed")
		}
		if name == "gh" && args[0] == "repo" {
			return os.MkdirAll(filepath.Join(args[3], ".git"), 0755)
		}
		return nil
	}
	t.Cleanup(func() { runStep = original })
	return &calls
}

func TestPrepareReusesAnExistingClone(t *testing.T) {
	root := t.TempDir()
	ref := PRRef{Host: "github.com", Owner: "o", Repo: "console", Number: 9}
	kept := filepath.Join(CloneDir(root, ref), "node_modules", "kept.txt")
	writeFile(t, kept, "deps")
	writeFile(t, filepath.Join(CloneDir(root, ref), ".git", "HEAD"), "ref")
	calls := recordSteps(t, "")

	if _, err := Prepare(context.Background(), ref, root, log.New(io.Discard)); err != nil {
		t.Fatal(err)
	}
	want := []string{"git reset --hard", "git clean -fd", "gh pr checkout 9 --force"}
	if len(*calls) < len(want) || strings.Join((*calls)[:len(want)], ",") != strings.Join(want, ",") {
		t.Errorf("calls = %v, want prefix %v", *calls, want)
	}
	for _, call := range *calls {
		if strings.HasPrefix(call, "gh repo clone") {
			t.Errorf("existing clone was cloned again: %v", *calls)
		}
	}
	if _, err := os.Stat(kept); err != nil {
		t.Errorf("reuse deleted files: %v", err)
	}
}

func TestPrepareFallsBackToAFreshCloneWhenReuseFails(t *testing.T) {
	root := t.TempDir()
	ref := PRRef{Host: "github.com", Owner: "o", Repo: "console", Number: 9}
	writeFile(t, filepath.Join(CloneDir(root, ref), ".git", "HEAD"), "ref")
	calls := recordSteps(t, "git reset")

	if _, err := Prepare(context.Background(), ref, root, log.New(io.Discard)); err != nil {
		t.Fatal(err)
	}
	cloned := false
	for _, call := range *calls {
		cloned = cloned || strings.HasPrefix(call, "gh repo clone")
	}
	if !cloned {
		t.Errorf("no fresh clone after failed reuse: %v", *calls)
	}
	if _, err := os.Stat(filepath.Join(CloneDir(root, ref), ".git", "HEAD")); err == nil {
		t.Errorf("broken clone was not removed")
	}
}
