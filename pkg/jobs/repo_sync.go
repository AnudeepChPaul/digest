package jobs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/achandrapaul/digest/pkg/paths"
	"github.com/achandrapaul/digest/pkg/system"

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

func discoverReposUnder(root string) ([]string, error) {
	expanded, err := paths.Expand(root)
	if err != nil {
		return nil, err
	}
	info, err := system.Stat(expanded)
	if err != nil {
		return nil, fmt.Errorf("repository root %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("repository root %s is not a directory", root)
	}
	var repos []string
	walkErr := system.Walk(expanded, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if path == expanded {
				return err
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if path != expanded && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		if system.Exists(filepath.Join(path, ".git")) {
			repos = append(repos, path)
			return filepath.SkipDir
		}
		return nil
	})
	if walkErr != nil {
		return repos, fmt.Errorf("repository root %s: %w", root, walkErr)
	}
	return repos, nil
}

func DiscoverRepos(roots []string) ([]string, error) {
	var repos []string
	var failures []error
	for _, root := range roots {
		found, err := discoverReposUnder(root)
		repos = append(repos, found...)
		if err != nil {
			failures = append(failures, err)
		}
	}
	return repos, errors.Join(failures...)
}

func remoteDefaultBranch(repo string) string {
	if res := gitCmd(repo, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); res.Ok {
		return strings.TrimPrefix(res.Stdout, "origin/")
	}
	if res := gitCmd(repo, "ls-remote", "--symref", "origin", "HEAD"); res.Ok {
		for _, line := range strings.Split(res.Stdout, "\n") {
			if target, found := strings.CutPrefix(line, "ref: refs/heads/"); found {
				return strings.TrimSuffix(target, "\tHEAD")
			}
		}
	}
	for _, candidate := range []string{"main", "master"} {
		if gitCmd(repo, "rev-parse", "--verify", "--quiet", "refs/remotes/origin/"+candidate).Ok {
			return candidate
		}
	}
	return "main"
}

func ClassifyRepo(repo string, dryRun bool) RepoInfo {
	if remotes := gitCmd(repo, "remote"); !remotes.Ok || remotes.Stdout == "" {
		return RepoInfo{Path: repo, State: NoRemote}
	}
	if !gitCmdTimeout(repo, 10*time.Second, fetchArgs(dryRun, "origin")...).Ok {
		return RepoInfo{Path: repo, State: Unreachable}
	}

	branch := gitCmd(repo, "rev-parse", "--abbrev-ref", "HEAD").Stdout
	if branch == "HEAD" {
		return RepoInfo{Path: repo, State: Detached, Branch: branch}
	}

	defaultBranch := remoteDefaultBranch(repo)

	if branch != defaultBranch {
		return RepoInfo{Path: repo, State: OffDefault, Branch: branch, Detail: fmt.Sprintf("on %s, default is %s", branch, defaultBranch)}
	}

	if !gitCmd(repo, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}").Ok {
		return RepoInfo{Path: repo, State: NoUpstream, Branch: branch}
	}

	if hasTrackedChanges(repo) {
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

	var repos, rootFailures []string
	var rootErrors []error
	for _, root := range j.Roots {
		found, err := discoverReposUnder(root)
		if err != nil {
			logger.Error("Skipping repository root", "err", err)
			rootFailures = append(rootFailures, err.Error())
			rootErrors = append(rootErrors, err)
			continue
		}
		repos = append(repos, found...)
	}
	if len(rootErrors) == len(j.Roots) && len(rootErrors) > 0 {
		return nil, errors.Join(rootErrors...)
	}
	rootsNote := ""
	if len(rootFailures) == 1 {
		rootsNote = " · 1 root unusable"
	} else if len(rootFailures) > 1 {
		rootsNote = fmt.Sprintf(" · %d roots unusable", len(rootFailures))
	}

	total := len(repos)
	if total == 0 {
		logger.Warn("No repositories found under roots", "roots", j.Roots)
		return &JobResult{
			Changed:  len(rootFailures) > 0,
			Degraded: len(rootFailures) > 0,
			Summary:  "0 repositories discovered" + rootsNote,
			Details:  strings.Join(append([]string{fmt.Sprintf("No Git repositories found under roots: %v", j.Roots)}, rootFailures...), "\n"),
			Drafts:   rootFailures,
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
				info := ClassifyRepo(repo, dryRun)
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

	var actions []string
	drafts := rootFailures
	degraded := len(rootFailures) > 0

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

	summary := fmt.Sprintf("%d repositories · %d %s · %d need attention", total, len(actions), verb, len(drafts)-len(rootFailures)) + rootsNote

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

func fetchArgs(dryRun bool, refspecs ...string) []string {
	if dryRun {
		return append([]string{"fetch", "--no-prune", "--no-auto-gc"}, refspecs...)
	}
	return append([]string{"fetch", "--prune"}, refspecs...)
}

func gitCmdTimeout(cwd string, timeout time.Duration, args ...string) CmdResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.WaitDelay = commandWaitDelay
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_OPTIONAL_LOCKS=0",
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
