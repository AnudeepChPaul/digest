package sourcecontrol

import (
	"context"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"app/pkg/config"
)

func TestDefaultCommitsCommandListsTodaysCommits(t *testing.T) {
	repoPath := makeRepoWithCommit(t, filepath.Join(t.TempDir(), "service"), "Add date picker")
	now := time.Now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	commits := fetchRepoCommits(context.Background(), gitCommitsCommand, repoPath, dayStart, dayStart.Add(24*time.Hour-time.Second))
	if len(commits) != 1 || !strings.HasSuffix(commits[0].Title, ": Add date picker") || commits[0].Repository != "service" {
		t.Fatalf("commits = %+v", commits)
	}
}

func TestDefaultCommitsCommandListsOnlyOwnCommits(t *testing.T) {
	repoPath := makeRepoWithCommit(t, filepath.Join(t.TempDir(), "shared"), "My change")
	runGit(t, "-C", repoPath, "-c", "user.name=mate", "-c", "user.email=mate@example.com", "commit", "-q", "--allow-empty", "-m", "Teammate change")
	now := time.Now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	commits := fetchRepoCommits(context.Background(), gitCommitsCommand, repoPath, dayStart, dayStart.Add(24*time.Hour-time.Second))
	if len(commits) != 1 || !strings.HasSuffix(commits[0].Title, ": My change") {
		t.Fatalf("commits = %+v", commits)
	}
}

func TestLocalCommitsComeOnlyFromConfiguredRoots(t *testing.T) {
	configuredRoot := t.TempDir()
	otherRoot := t.TempDir()
	makeRepoWithCommit(t, filepath.Join(configuredRoot, "inside"), "Inside change")
	makeRepoWithCommit(t, filepath.Join(otherRoot, "outside"), "Outside change")
	cfg := config.DefaultConfig()
	previousCommand := gitCommitsCommand
	gitCommitsCommand = `git log --since="{since}" --until="{until}" --pretty="format:%h|%s"`
	t.Cleanup(func() { gitCommitsCommand = previousCommand })
	cfg.GitRepositoryRoots = []string{configuredRoot}
	now := time.Now()
	commitsByRepo := FetchLocalCommitsBetween(context.Background(), cfg, now.Add(-time.Hour), now.Add(time.Hour))
	if len(commitsByRepo) != 1 || len(commitsByRepo["inside"]) != 1 {
		t.Fatalf("commits = %+v", commitsByRepo)
	}
}

func runGit(t *testing.T, args ...string) {
	t.Helper()
	if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, output)
	}
}

func makeRepoWithCommit(t *testing.T, path, message string) string {
	t.Helper()
	runGit(t, "init", "-q", path)
	runGit(t, "-C", path, "config", "user.name", "me")
	runGit(t, "-C", path, "config", "user.email", "me@example.com")
	runGit(t, "-C", path, "commit", "-q", "--allow-empty", "-m", message)
	return path
}

func TestCommitsForSeveralDaysDiscoverReposOnceAndCapConcurrentGit(t *testing.T) {
	var repos []string
	for index := range maxConcurrentGitReads * 3 {
		repos = append(repos, filepath.Join(t.TempDir(), "repo"+strconv.Itoa(index)))
	}
	discoveries := 0
	originalDiscover := discoverRepoPaths
	discoverRepoPaths = func(*config.Config) []string {
		discoveries++
		return repos
	}
	var running, peak atomic.Int32
	originalRun := runCommitsCommand
	runCommitsCommand = func(ctx context.Context, dir string, command string) []byte {
		current := running.Add(1)
		for {
			seen := peak.Load()
			if current <= seen || peak.CompareAndSwap(seen, current) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		running.Add(-1)
		return []byte("abc|change")
	}
	t.Cleanup(func() {
		discoverRepoPaths = originalDiscover
		runCommitsCommand = originalRun
	})
	now := time.Now()
	days := FetchLocalCommitsForDays(context.Background(), config.DefaultConfig(), now, now.AddDate(0, 0, -1))
	if discoveries != 1 {
		t.Errorf("repo discovery ran %d times, want 1", discoveries)
	}
	if len(days) != 2 || len(days[0]) != len(repos) || len(days[1]) != len(repos) {
		t.Fatalf("days = %d", len(days))
	}
	if peak.Load() > int32(maxConcurrentGitReads) {
		t.Errorf("peak concurrent git reads = %d, cap %d", peak.Load(), maxConcurrentGitReads)
	}
}

func TestShellCommandTimeoutHoldsWhenAChildKeepsTheOutputOpen(t *testing.T) {
	original := commandWaitDelay
	commandWaitDelay = 200 * time.Millisecond
	t.Cleanup(func() { commandWaitDelay = original })
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	started := time.Now()
	executeShellCommand(ctx, t.TempDir(), "sleep 5 & sleep 5")
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Errorf("command ran %v past its timeout", elapsed)
	}
}
