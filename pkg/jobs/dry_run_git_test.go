package jobs

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func cloneWithStaleRemoteBranch(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	origin, seed, clone := filepath.Join(base, "origin.git"), filepath.Join(base, "seed"), filepath.Join(base, "clone")
	runGit(t, base, "init", "--quiet", "--bare", "-b", "main", origin)
	runGit(t, base, "clone", "--quiet", origin, seed)
	if err := os.WriteFile(filepath.Join(seed, "README"), []byte("hi\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, seed, "add", "README")
	runGit(t, seed, "commit", "--quiet", "-m", "init")
	runGit(t, seed, "push", "--quiet", "origin", "main", "main:gone")
	runGit(t, base, "clone", "--quiet", origin, clone)
	runGit(t, seed, "push", "--quiet", "origin", "--delete", "gone")
	return clone
}

func remoteBranchExists(repo, branch string) bool {
	return exec.Command("git", "-C", repo, "rev-parse", "--verify", "--quiet", "refs/remotes/origin/"+branch).Run() == nil
}

func TestRepoSyncDryRunDoesNotPruneRemoteBranches(t *testing.T) {
	clone := cloneWithStaleRemoteBranch(t)
	ClassifyRepo(clone, true)
	if !remoteBranchExists(clone, "gone") {
		t.Error("dry run pruned origin/gone")
	}
	ClassifyRepo(clone, false)
	if remoteBranchExists(clone, "gone") {
		t.Error("a real run should prune origin/gone")
	}
}

func TestGitStatusLeavesTheIndexAlone(t *testing.T) {
	clone := cloneWithStaleRemoteBranch(t)
	readme := filepath.Join(clone, "README")
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(readme, later, later); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(clone, ".git", "index")
	before, err := os.Stat(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	gitCmd(clone, "status", "--porcelain")
	after, err := os.Stat(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("git status rewrote .git/index")
	}
}
