package jobs

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
)

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

func commitFile(t *testing.T, repo, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", name)
	runGit(t, repo, "commit", "--quiet", "-m", "add "+name)
}

func originWithSeed(t *testing.T) (origin, seed string) {
	t.Helper()
	base := t.TempDir()
	origin, seed = filepath.Join(base, "origin.git"), filepath.Join(base, "seed")
	runGit(t, base, "init", "--quiet", "--bare", "-b", "main", origin)
	runGit(t, base, "clone", "--quiet", origin, seed)
	commitFile(t, seed, "README", "hi\n")
	runGit(t, seed, "push", "--quiet", "origin", "main")
	return origin, seed
}

func TestRepoSyncRealRunFastForwardsACleanRepoBehindUpstream(t *testing.T) {
	origin, seed := originWithSeed(t)
	syncRoot := t.TempDir()
	clone := filepath.Join(syncRoot, "clone")
	runGit(t, syncRoot, "clone", "--quiet", origin, clone)
	commitFile(t, seed, "CHANGES", "newer\n")
	runGit(t, seed, "push", "--quiet", "origin", "main")
	upstreamHead := gitOutput(t, seed, "rev-parse", "HEAD")

	job := &RepoSyncJob{Roots: []string{syncRoot}, Logger: log.New(io.Discard)}
	result, err := job.Run(false)
	if err != nil {
		t.Fatal(err)
	}
	if head := gitOutput(t, clone, "rev-parse", "HEAD"); head != upstreamHead {
		t.Errorf("clone HEAD = %s, want fast-forwarded to %s", head, upstreamHead)
	}
	if !slices.Contains(result.ActionsTaken, "clone: fast-forwarded") {
		t.Errorf("actions = %v, want clone: fast-forwarded", result.ActionsTaken)
	}
	if NeedsUserAction(result, false) {
		t.Errorf("a clean fast-forward needs no user action: %+v", result)
	}
}
