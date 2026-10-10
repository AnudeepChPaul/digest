package jobs

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/charmbracelet/log"
)

func fakeGitHubCLI(t *testing.T, mergedPRsJSON string) {
	t.Helper()
	binDir := t.TempDir()
	jsonPath := filepath.Join(binDir, "merged.json")
	if err := os.WriteFile(jsonPath, []byte(mergedPRsJSON), 0644); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n[ \"$1 $2\" = \"pr list\" ] || exit 1\ncat " + jsonPath + "\n"
	if err := os.WriteFile(filepath.Join(binDir, "gh"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func localBranchExists(t *testing.T, repo, branch string) bool {
	t.Helper()
	return gitCmd(repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch).Ok
}

func TestBranchReaperDeletesMergedBranchesAndKeepsUnmergedWork(t *testing.T) {
	origin, _ := originWithSeed(t)
	reapRoot := t.TempDir()
	clone := filepath.Join(reapRoot, "clone")
	runGit(t, reapRoot, "clone", "--quiet", origin, clone)

	runGit(t, clone, "checkout", "--quiet", "-b", "squashed")
	commitFile(t, clone, "squashed.txt", "done\n")
	squashedHead := gitOutput(t, clone, "rev-parse", "HEAD")
	runGit(t, clone, "push", "--quiet", "origin", "squashed:refs/pull/1/head")

	runGit(t, clone, "checkout", "--quiet", "main")
	runGit(t, clone, "checkout", "--quiet", "-b", "still-working")
	commitFile(t, clone, "wip.txt", "first\n")
	mergedHead := gitOutput(t, clone, "rev-parse", "HEAD")
	runGit(t, clone, "push", "--quiet", "origin", "still-working:refs/pull/2/head")
	commitFile(t, clone, "wip.txt", "after the merge\n")
	runGit(t, clone, "checkout", "--quiet", "main")

	fakeGitHubCLI(t, `[
		{"number": 1, "headRefName": "squashed", "headRefOid": "`+squashedHead+`", "mergedAt": "2020-01-01T00:00:00Z"},
		{"number": 2, "headRefName": "still-working", "headRefOid": "`+mergedHead+`", "mergedAt": "2020-01-01T00:00:00Z"}
	]`)

	job := &BranchReaperJob{Roots: []string{reapRoot}, MinAgeDays: 7, Logger: log.New(io.Discard)}
	result, err := job.Run(false)
	if err != nil {
		t.Fatal(err)
	}
	if localBranchExists(t, clone, "squashed") {
		t.Error("branch of a merged PR was kept")
	}
	if !localBranchExists(t, clone, "still-working") {
		t.Error("branch with commits after its merged PR was deleted")
	}
	if !slices.Contains(result.ActionsTaken, "clone: deleted squashed (PR #1)") {
		t.Errorf("actions = %v, want clone: deleted squashed (PR #1)", result.ActionsTaken)
	}
	if !slices.Contains(result.Drafts, "clone: still-working - git refused delete") {
		t.Errorf("drafts = %v, want still-working flagged", result.Drafts)
	}
	if !NeedsUserAction(result, false) {
		t.Error("a kept branch with unmerged work should need user action")
	}
}
