package jobs

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"app/pkg/paths"

	"github.com/charmbracelet/log"
)

type RepoState string

const (
	UpToDate    RepoState = "up-to-date"
	Behind      RepoState = "behind"
	Ahead       RepoState = "ahead"
	Dirty       RepoState = "dirty"
	Diverged    RepoState = "diverged"
	Detached    RepoState = "detached"
	NoUpstream  RepoState = "no-upstream"
	OffDefault  RepoState = "off-default"
	NoRemote    RepoState = "no-remote"
	Unreachable RepoState = "unreachable"
)

type RepoInfo struct {
	Path   string
	State  RepoState
	Branch string
	Detail string
}

type RepoSyncJob struct {
	Roots   []string
	Workers int
	Logger  *log.Logger
}

func DiscoverRepos(roots []string) ([]string, error) {
	var repos []string
	for _, root := range roots {
		expanded := paths.Expand(root)
		if _, err := os.Stat(expanded); os.IsNotExist(err) {
			continue
		}

		_ = filepath.WalkDir(expanded, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				// Skip hidden directories except the root itself
				if path != expanded && strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}

				// Check if current directory is a Git repository
				gitDir := filepath.Join(path, ".git")
				if _, err := os.Stat(gitDir); err == nil {
					repos = append(repos, path)
					return filepath.SkipDir // Stop traversing deeper into this repository
				}
			}
			return nil
		})
	}
	return repos, nil
}

func ClassifyRepo(repo string) RepoInfo {
	if !gitCmd(repo, "remote").Ok {
		return RepoInfo{Path: repo, State: NoRemote}
	}
	if !gitCmdTimeout(repo, 10*time.Second, "fetch", "--prune", "origin").Ok {
		return RepoInfo{Path: repo, State: Unreachable}
	}

	branch := gitCmd(repo, "rev-parse", "--abbrev-ref", "HEAD").Stdout
	if branch == "HEAD" {
		return RepoInfo{Path: repo, State: Detached, Branch: branch}
	}

	defaultBranch := "main"
	if res := gitCmd(repo, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); res.Ok {
		defaultBranch = strings.TrimPrefix(res.Stdout, "origin/")
	}

	if branch != defaultBranch {
		return RepoInfo{Path: repo, State: OffDefault, Branch: branch, Detail: fmt.Sprintf("on %s, default is %s", branch, defaultBranch)}
	}

	if !gitCmd(repo, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}").Ok {
		return RepoInfo{Path: repo, State: NoUpstream, Branch: branch}
	}

	if gitCmd(repo, "status", "--porcelain").Stdout != "" {
		return RepoInfo{Path: repo, State: Dirty, Branch: branch}
	}

	counts := gitCmd(repo, "rev-list", "--left-right", "--count", "HEAD...@{upstream}").Stdout
	var ahead, behind int
	fmt.Sscanf(counts, "%d\t%d", &ahead, &behind)

	if ahead > 0 && behind > 0 {
		return RepoInfo{Path: repo, State: Diverged, Branch: branch, Detail: fmt.Sprintf("%d ahead, %d behind", ahead, behind)}
	}
	if behind > 0 {
		return RepoInfo{Path: repo, State: Behind, Branch: branch, Detail: fmt.Sprintf("%d behind", behind)}
	}
	if ahead > 0 {
		return RepoInfo{Path: repo, State: Ahead, Branch: branch, Detail: fmt.Sprintf("%d ahead", ahead)}
	}

	return RepoInfo{Path: repo, State: UpToDate, Branch: branch}
}

func (j *RepoSyncJob) Run(dryRun bool) (*JobResult, error) {
	logger := j.Logger
	if logger == nil {
		logger = log.New(os.Stderr)
	}

	logger.Info("Discovering repositories", "roots", strings.Join(j.Roots, ", "))

	repos, err := DiscoverRepos(j.Roots)
	if err != nil {
		return nil, err
	}

	total := len(repos)
	if total == 0 {
		logger.Warn("No repositories found under roots", "roots", j.Roots)
		return &JobResult{
			Changed: false,
			Summary: "0 repositories discovered",
			Details: fmt.Sprintf("No Git repositories found under roots: %v", j.Roots),
		}, nil
	}

	logger.Info("Inspecting repositories", "total", total)

	jobsChan := make(chan string, total)
	resultsChan := make(chan RepoInfo, total)
	var wg sync.WaitGroup
	var outMux sync.Mutex
	var completed int32

	workers := j.Workers
	if workers <= 0 {
		workers = 8
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for repo := range jobsChan {
				info := ClassifyRepo(repo)
				curr := atomic.AddInt32(&completed, 1)

				outMux.Lock()
				rel := filepath.Base(repo)
				progress := fmt.Sprintf("[%d/%d]", curr, total)

				switch info.State {
				case UpToDate:
					logger.Info(progress+" "+rel, "state", info.State)
				case Behind:
					logger.Warn(progress+" "+rel, "state", info.State, "detail", info.Detail)
				case Dirty, Diverged, OffDefault, Ahead, NoUpstream, Detached, NoRemote:
					logger.Warn(progress+" "+rel, "state", info.State, "detail", info.Detail)
				case Unreachable:
					logger.Error(progress+" "+rel, "state", info.State)
				}
				outMux.Unlock()

				resultsChan <- info
			}
		}()
	}

	for _, repo := range repos {
		jobsChan <- repo
	}
	close(jobsChan)
	wg.Wait()
	close(resultsChan)

	logger.Info("Evaluating fast-forward actions")

	var actions, drafts []string
	degraded := false

	for info := range resultsChan {
		rel := filepath.Base(info.Path)
		switch info.State {
		case Behind:
			if dryRun {
				actions = append(actions, fmt.Sprintf("%s: would fast-forward %s", rel, info.Detail))
			} else {
				if gitCmd(info.Path, "merge", "--ff-only", "@{upstream}").Ok {
					actions = append(actions, fmt.Sprintf("%s: fast-forwarded", rel))
				} else {
					drafts = append(drafts, fmt.Sprintf("%s: fast-forward refused", rel))
					degraded = true
				}
			}
		case Dirty, Diverged, Detached, NoUpstream, OffDefault, Ahead, NoRemote:
			drafts = append(drafts, fmt.Sprintf("%s: %s", rel, info.State))
		case Unreachable:
			degraded = true
			drafts = append(drafts, fmt.Sprintf("%s: remote unreachable", rel))
		}
	}

	verb := "to fast-forward"
	if !dryRun {
		verb = "fast-forwarded"
	}

	summary := fmt.Sprintf("%d repositories · %d %s · %d need attention", total, len(actions), verb, len(drafts))

	return &JobResult{
		Changed:      len(actions) > 0 || len(drafts) > 0,
		Degraded:     degraded,
		Summary:      summary,
		ActionsTaken: actions,
		Drafts:       drafts,
	}, nil
}

type CmdResult struct {
	Stdout string
	Stderr string
	Ok     bool
}

func gitCmd(cwd string, args ...string) CmdResult {
	return gitCmdTimeout(cwd, 10*time.Second, args...)
}

var commandWaitDelay = 2 * time.Second

func gitCmdTimeout(cwd string, timeout time.Duration, args ...string) CmdResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.WaitDelay = commandWaitDelay
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_SSH_COMMAND=ssh -o BatchMode=yes -o ConnectTimeout=5",
	)

	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	if err := cmd.Run(); err != nil {
		return CmdResult{Stdout: "", Stderr: strings.TrimSpace(errOut.String()), Ok: false}
	}
	return CmdResult{Stdout: strings.TrimSpace(out.String()), Stderr: strings.TrimSpace(errOut.String()), Ok: true}
}
