package sourcecontrol

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"app/pkg/config"
	"app/pkg/jobs"
)

var maxConcurrentGitReads = max(runtime.NumCPU(), 2)

var commandWaitDelay = 2 * time.Second

var runCommitsCommand = executeShellCommand

var discoverRepoPaths = localRepoPaths

func executeShellCommand(ctx context.Context, dir string, cmdStr string) []byte {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/c", cmdStr)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", cmdStr)
	}
	cmd.WaitDelay = commandWaitDelay
	cmd.Env = os.Environ()
	if dir != "" {
		cmd.Dir = dir
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil
	}
	return out.Bytes()
}

func fetchRepoCommits(ctx context.Context, cmdTemplate string, repoPath string, since, until time.Time) []PRItem {
	fullCmd := strings.ReplaceAll(cmdTemplate, "{since}", since.Format("2006-01-02 15:04:05"))
	fullCmd = strings.ReplaceAll(fullCmd, "{until}", until.Format("2006-01-02 15:04:05"))

	output := runCommitsCommand(ctx, repoPath, fullCmd)
	if len(output) == 0 {
		return nil
	}

	var items []PRItem
	repoName := filepath.Base(repoPath)
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		hash := parts[0]
		message := line
		if len(parts) == 2 {
			message = parts[1]
		}
		items = append(items, PRItem{
			Title:      fmt.Sprintf("commit %s: %s", hash, message),
			Kind:       CommitKind,
			Repository: repoName,
		})
	}
	return items
}

func localRepoPaths(cfg *config.Config) []string {
	home, _ := os.UserHomeDir()
	searchRoots := []string{
		filepath.Join(home, "Projects"),
		filepath.Join(home, "Developer"),
		filepath.Join(home, "Code"),
		".",
	}
	searchRoots = append([]string{cfg.NotesDir()}, searchRoots...)
	if len(cfg.GitRepositoryRoots) > 0 {
		searchRoots = cfg.GitRepositoryRoots
	}

	repoPaths, _ := jobs.DiscoverRepos(searchRoots)
	uniquePaths := make(map[string]bool)
	var cleanLocalRepos []string
	for _, repoPath := range repoPaths {
		abs, err := filepath.Abs(repoPath)
		if err == nil && !uniquePaths[abs] {
			uniquePaths[abs] = true
			cleanLocalRepos = append(cleanLocalRepos, abs)
		}
	}
	return cleanLocalRepos
}

type commitWindow struct {
	since, until time.Time
}

func dayWindow(date time.Time) commitWindow {
	dayStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	return commitWindow{since: dayStart, until: dayStart.Add(24*time.Hour - time.Second)}
}

func FetchLocalCommits(ctx context.Context, cfg *config.Config, date time.Time) map[string][]PRItem {
	return FetchLocalCommitsForDays(ctx, cfg, date)[0]
}

func FetchLocalCommitsForDays(ctx context.Context, cfg *config.Config, dates ...time.Time) []map[string][]PRItem {
	windows := make([]commitWindow, len(dates))
	for index, date := range dates {
		windows[index] = dayWindow(date)
	}
	return fetchCommitWindows(ctx, cfg, windows)
}

func FetchLocalCommitsBetween(ctx context.Context, cfg *config.Config, since, until time.Time) map[string][]PRItem {
	return fetchCommitWindows(ctx, cfg, []commitWindow{{since: since, until: until}})[0]
}

func fetchCommitWindows(ctx context.Context, cfg *config.Config, windows []commitWindow) []map[string][]PRItem {
	results := make([]map[string][]PRItem, len(windows))
	for index := range results {
		results[index] = make(map[string][]PRItem)
	}
	if cfg == nil {
		return results
	}
	var resultsMu sync.Mutex
	var wg sync.WaitGroup
	gitReadSlots := make(chan struct{}, maxConcurrentGitReads)

	fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	for _, repoPath := range discoverRepoPaths(cfg) {
		for index, window := range windows {
			wg.Add(1)
			go func() {
				defer wg.Done()
				select {
				case gitReadSlots <- struct{}{}:
				case <-fetchCtx.Done():
					return
				}
				commits := fetchRepoCommits(fetchCtx, cfg.GitCommitsCmd, repoPath, window.since, window.until)
				<-gitReadSlots
				if len(commits) > 0 {
					resultsMu.Lock()
					results[index][filepath.Base(repoPath)] = commits
					resultsMu.Unlock()
				}
			}()
		}
	}
	wg.Wait()
	return results
}
