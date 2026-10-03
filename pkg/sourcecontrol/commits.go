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

func executeShellCommand(ctx context.Context, dir string, cmdStr string) []byte {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/c", cmdStr)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", cmdStr)
	}
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

func fetchRepoCommits(ctx context.Context, cmdTemplate string, repoPath string, targetDate time.Time) []PRItem {
	fullCmd := strings.ReplaceAll(cmdTemplate, "{since}", targetDate.Format("2006-01-02 00:00:00"))
	fullCmd = strings.ReplaceAll(fullCmd, "{until}", targetDate.Format("2006-01-02 23:59:59"))

	output := executeShellCommand(ctx, repoPath, fullCmd)
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
	if cfg.NotesDir != "" {
		searchRoots = append([]string{cfg.NotesDir}, searchRoots...)
	}
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

func FetchLocalCommits(ctx context.Context, cfg *config.Config, date time.Time) map[string][]PRItem {
	commitsByRepo := make(map[string][]PRItem)
	if cfg == nil {
		return commitsByRepo
	}
	var commitMu sync.Mutex
	var wg sync.WaitGroup

	fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	for _, repoPath := range localRepoPaths(cfg) {
		wg.Add(1)
		go func(repoPath string) {
			defer wg.Done()
			commits := fetchRepoCommits(fetchCtx, cfg.GitCommitsCmd, repoPath, date)
			if len(commits) > 0 {
				commitMu.Lock()
				commitsByRepo[filepath.Base(repoPath)] = commits
				commitMu.Unlock()
			}
		}(repoPath)
	}
	wg.Wait()
	return commitsByRepo
}
