package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

func gitIn(t *testing.T, repoDir string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = repoDir
	command.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func commitFile(t *testing.T, repoDir, name, content, subject string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repoDir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, repoDir, "add", name)
	gitIn(t, repoDir, "commit", "-q", "-m", subject)
}

func subjectsSinceLastRelease(t *testing.T, repoDir string) []string {
	t.Helper()
	commits, err := commitsSinceLastRelease(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	var subjects []string
	for _, commit := range commits {
		subjects = append(subjects, commit.Subject)
	}
	return subjects
}

func TestCommitsSinceLastReleaseStartAfterTheLastVersionChange(t *testing.T) {
	repoDir := t.TempDir()
	gitIn(t, repoDir, "init", "-q")
	commitFile(t, repoDir, "a.go", "a", "feat: first")
	if got := subjectsSinceLastRelease(t, repoDir); !slices.Equal(got, []string{"feat: first"}) {
		t.Errorf("without a version commit every commit is listed, got %v", got)
	}

	commitFile(t, repoDir, ".version", "1.0.0\n", "chore: release v1.0.0")
	commitFile(t, repoDir, "a.go", "b", "fix: second")
	commitFile(t, repoDir, "b.go", "c", "feat: third")
	want := []string{"feat: third", "fix: second"}
	if got := subjectsSinceLastRelease(t, repoDir); !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	if err := os.WriteFile(filepath.Join(repoDir, ".version"), []byte("1.1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := subjectsSinceLastRelease(t, repoDir); !slices.Equal(got, want) {
		t.Errorf("an uncommitted bump should not move the range, got %v", got)
	}

	gitIn(t, repoDir, "commit", "-q", "-am", "chore: release v1.1.0")
	wantOnReleaseCommit := []string{"chore: release v1.1.0", "feat: third", "fix: second"}
	if got := subjectsSinceLastRelease(t, repoDir); !slices.Equal(got, wantOnReleaseCommit) {
		t.Errorf("on the release commit the range should start at the previous release, got %v", got)
	}

	commitFile(t, repoDir, "c.go", "d", "fix: fourth")
	if got := subjectsSinceLastRelease(t, repoDir); !slices.Equal(got, []string{"fix: fourth"}) {
		t.Errorf("after the release commit only newer commits are listed, got %v", got)
	}
}
