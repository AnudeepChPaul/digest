package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
)

type MergedPR struct {
	Number      int    `json:"number"`
	HeadRefName string `json:"headRefName"`
	HeadRefOid  string `json:"headRefOid"`
	MergedAt    string `json:"mergedAt"`
}

type BranchReaperJob struct {
	Roots      []string
	MinAgeDays int
	Workers    int
	Logger     *log.Logger
}

type repoReapResult struct {
	actions []string
	drafts  []string
}

func fetchMergedPRs(repo string, timeout time.Duration) ([]MergedPR, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "pr", "list", "--state", "merged", "--limit", "200", "--json", "number,headRefName,headRefOid,mergedAt")
	cmd.Dir = repo
	cmd.WaitDelay = commandWaitDelay
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var prs []MergedPR
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, err
	}
	return prs, nil
}

func fetchPRHead(repo string, pr MergedPR) CmdResult {
	return gitCmdTimeout(repo, 30*time.Second, "fetch", "--no-tags", "--quiet", "origin", fmt.Sprintf("pull/%d/head", pr.Number))
}

func flattenGitOutput(output string) string {
	return strings.ReplaceAll(strings.TrimSpace(output), "\n", "; ")
}

func notInHead(repo, branch string) string {
	unmerged := gitCmd(repo, "log", "--oneline", "HEAD..refs/heads/"+branch)
	return flattenGitOutput(unmerged.Stdout + unmerged.Stderr)
}

func matchMergedPR(repo, branch string, prs []MergedPR) (*MergedPR, string) {
	branchRef := "refs/heads/" + branch
	var outputs []string
	for i := range prs {
		pr := prs[i]
		fetch := fetchPRHead(repo, pr)
		if !fetch.Ok {
			outputs = append(outputs, fmt.Sprintf("#%d: %s", pr.Number, flattenGitOutput(fetch.Stderr)))
			continue
		}
		ancestor := gitCmd(repo, "merge-base", "--is-ancestor", branchRef, pr.HeadRefOid)
		if ancestor.Ok {
			return &prs[i], ""
		}
		if ancestor.Stderr != "" {
			outputs = append(outputs, fmt.Sprintf("#%d: %s", pr.Number, flattenGitOutput(ancestor.Stderr)))
			continue
		}
		unmerged := gitCmd(repo, "log", "--oneline", pr.HeadRefOid+".."+branchRef)
		outputs = append(outputs, fmt.Sprintf("#%d: %s", pr.Number, flattenGitOutput(unmerged.Stdout+unmerged.Stderr)))
	}
	return nil, strings.Join(outputs, " | ")
}

func (j *BranchReaperJob) reapRepo(repo string, dryRun bool, cutoff time.Time, logger *log.Logger) repoReapResult {
	var result repoReapResult
	name := filepath.Base(repo)

	if gitCmd(repo, "status", "--porcelain").Stdout != "" {
		logger.Warn("Skipped repository (dirty working tree)", "repo", name)
		return result
	}

	prs, err := fetchMergedPRs(repo, 30*time.Second)
	if err != nil {
		var syntaxErr *json.SyntaxError
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &syntaxErr) || errors.As(err, &typeErr) {
			return result
		}
		logger.Warn("Skipped repository (failed gh query)", "repo", name)
		result.drafts = append(result.drafts, fmt.Sprintf("%s: skipped, failed gh query", name))
		return result
	}

	mergedMap := make(map[string][]MergedPR)
	for _, pr := range prs {
		if t, err := time.Parse(time.RFC3339, pr.MergedAt); err == nil && t.Before(cutoff) {
			mergedMap[pr.HeadRefName] = append(mergedMap[pr.HeadRefName], pr)
		}
	}

	branchesOut := gitCmd(repo, "for-each-ref", "--format=%(refname:short)", "refs/heads").Stdout
	branches := strings.Split(branchesOut, "\n")
	current := gitCmd(repo, "rev-parse", "--abbrev-ref", "HEAD").Stdout

	for _, b := range branches {
		b = strings.TrimSpace(b)
		if b == "" || b == current || b == "main" || b == "master" {
			continue
		}

		branchPRs, ok := mergedMap[b]
		if !ok {
			continue
		}

		if dryRun {
			refused := notInHead(repo, b)
			if refused == "" {
				logger.Warn("Would delete branch", "repo", name, "branch", b, "via", "-d")
				result.actions = append(result.actions, fmt.Sprintf("%s: would delete %s", name, b))
				continue
			}
			matched, gitOutput := matchMergedPR(repo, b, branchPRs)
			if matched != nil {
				logger.Warn("Would delete branch", "repo", name, "branch", b, "via", "-D", "pr", fmt.Sprintf("#%d", matched.Number), "refused", refused)
				result.actions = append(result.actions, fmt.Sprintf("%s: would delete %s (PR #%d)", name, b, matched.Number))
			} else {
				logger.Warn("Would skip branch", "repo", name, "branch", b, "refused", refused, "unmerged", gitOutput)
				result.drafts = append(result.drafts, fmt.Sprintf("%s: would skip %s", name, b))
			}
			continue
		}

		softDelete := gitCmd(repo, "branch", "-d", b)
		if softDelete.Ok {
			logger.Info("Deleted branch", "repo", name, "branch", b)
			result.actions = append(result.actions, fmt.Sprintf("%s: deleted %s", name, b))
			continue
		}

		matched, gitOutput := matchMergedPR(repo, b, branchPRs)
		if matched == nil {
			logger.Error("Git refused to delete branch", "repo", name, "branch", b, "git", flattenGitOutput(softDelete.Stderr), "unmerged", gitOutput)
			result.drafts = append(result.drafts, fmt.Sprintf("%s: %s - git refused delete", name, b))
			continue
		}

		forceDelete := gitCmd(repo, "branch", "-D", b)
		if forceDelete.Ok {
			logger.Info("Deleted branch via -D", "repo", name, "branch", b, "pr", fmt.Sprintf("#%d", matched.Number), "git", flattenGitOutput(softDelete.Stderr))
			result.actions = append(result.actions, fmt.Sprintf("%s: deleted %s (PR #%d)", name, b, matched.Number))
		} else {
			logger.Error("Git refused to force delete branch", "repo", name, "branch", b, "git", flattenGitOutput(forceDelete.Stderr))
			result.drafts = append(result.drafts, fmt.Sprintf("%s: %s - git refused delete", name, b))
		}
	}

	return result
}

func (j *BranchReaperJob) Run(dryRun bool) (*JobResult, error) {
	logger := j.Logger
	if logger == nil {
		logger = log.New(os.Stderr)
	}

	repos, err := DiscoverRepos(j.Roots)
	if err != nil {
		return nil, err
	}

	logger.Info("Checking repositories for merged branches", "total", len(repos))

	workers := j.Workers
	if workers <= 0 {
		workers = 8
	}
	cutoff := time.Now().AddDate(0, 0, -j.MinAgeDays)

	results := make([]repoReapResult, len(repos))
	repoIndexes := make(chan int, len(repos))
	for i := range repos {
		repoIndexes <- i
	}
	close(repoIndexes)

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range repoIndexes {
				results[i] = j.reapRepo(repos[i], dryRun, cutoff, logger)
			}
		}()
	}
	wg.Wait()

	var actions, drafts []string
	for _, result := range results {
		actions = append(actions, result.actions...)
		drafts = append(drafts, result.drafts...)
	}

	summary := fmt.Sprintf("%d repositories · %d branches processed", len(repos), len(actions))
	return &JobResult{
		Changed:      len(actions) > 0,
		Summary:      summary,
		ActionsTaken: actions,
		Drafts:       drafts,
	}, nil
}
