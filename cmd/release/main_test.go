package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	latestReleaseTag = func(string) string { return "" }
	os.Exit(m.Run())
}

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
	if got := subjectsSinceLastRelease(t, repoDir); len(got) != 0 {
		t.Errorf("right after a release commit nothing is pending, got %v", got)
	}

	commitFile(t, repoDir, "c.go", "d", "fix: fourth")
	if got := subjectsSinceLastRelease(t, repoDir); !slices.Equal(got, []string{"fix: fourth"}) {
		t.Errorf("after the release commit only newer commits are listed, got %v", got)
	}
}

func TestCommitsSinceLastReleaseSpanEveryUnreleasedVersion(t *testing.T) {
	repoDir := t.TempDir()
	gitIn(t, repoDir, "init", "-q")
	commitFile(t, repoDir, ".version", "1.0.0\n", "release:patch")
	gitIn(t, repoDir, "tag", "-m", "t", "v1.0.0")
	commitFile(t, repoDir, "a.go", "a", "fix: second")
	commitFile(t, repoDir, ".version", "1.0.1\n", "release:patch")
	gitIn(t, repoDir, "tag", "-m", "t", "v1.0.1")
	commitFile(t, repoDir, "b.go", "b", "feat: third")

	latestReleaseTag = func(string) string { return "v1.0.0" }
	t.Cleanup(func() { latestReleaseTag = func(string) string { return "" } })
	want := []string{"feat: third", "release:patch", "fix: second"}
	if got := subjectsSinceLastRelease(t, repoDir); !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	latestReleaseTag = func(string) string { return "v9.9.9" }
	if got := subjectsSinceLastRelease(t, repoDir); !slices.Equal(got, []string{"feat: third"}) {
		t.Errorf("an unknown release tag falls back to the last version change, got %v", got)
	}
}

func TestNotesWritesTheReleaseSectionOnceAndPrintsIt(t *testing.T) {
	repoDir := t.TempDir()
	gitIn(t, repoDir, "init", "-q")
	commitFile(t, repoDir, ".version", "1.4.0\n", "chore: release v1.4.0")
	commitFile(t, repoDir, "a.go", "a", "fix: second")
	changelogFile := filepath.Join(repoDir, changelogPath)
	if err := os.WriteFile(changelogFile, []byte("# Changelog\n\n## v1.4.0 — 2026-10-01\n\n### Changes\n- old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var firstRun, secondRun strings.Builder
	if err := writeNotes(repoDir, "1.4.1", &firstRun); err != nil {
		t.Fatal(err)
	}
	if err := writeNotes(repoDir, "1.4.1", &secondRun); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(firstRun.String(), "### Changes\n- [") || !strings.Contains(firstRun.String(), ") fix: second\n") {
		t.Errorf("notes = %q", firstRun.String())
	}
	if secondRun.String() != firstRun.String() {
		t.Errorf("second run printed %q, want %q", secondRun.String(), firstRun.String())
	}
	changelog, err := os.ReadFile(changelogFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(changelog), "## v1.4.1 — ") != 1 || !strings.Contains(string(changelog), "## v1.4.0 — 2026-10-01") {
		t.Errorf("changelog =\n%s", changelog)
	}
	if !strings.Contains(string(changelog), "](https://github.com/achandrapaul/digest/commit/") {
		t.Errorf("changelog should link each commit:\n%s", changelog)
	}
}
