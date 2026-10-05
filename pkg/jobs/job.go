package jobs

import (
	"fmt"
	"os"

	"app/pkg/paths"

	"github.com/charmbracelet/log"
)

type JobResult struct {
	Changed      bool     `json:"changed"`
	Degraded     bool     `json:"degraded"`
	Summary      string   `json:"summary"`
	Details      string   `json:"details"`
	ActionsTaken []string `json:"actions_taken"`
	Drafts       []string `json:"drafts"`
}

type Runner interface {
	Name() string
	Run(dryRun bool) (*JobResult, error)
}

func NeedsUserAction(res *JobResult, dryRun bool) bool {
	if res == nil {
		return false
	}
	if res.Degraded || len(res.Drafts) > 0 {
		return true
	}
	return dryRun && res.Changed
}

// RunRepoSync executes the background repository synchronization runner.
func RunRepoSync(roots []string, dryRun bool) (bool, error) {
	logger := log.New(os.Stderr)
	job := &RepoSyncJob{Roots: roots, Logger: logger}
	res, err := job.Run(dryRun)
	if err != nil {
		return false, err
	}
	fmt.Println(res.Summary)
	return NeedsUserAction(res, dryRun), nil
}

// RunJanitor scans and cleans up quarantined or temporary files.
func RunJanitor(roots, patterns []string, reviewRoot string, dryRun bool) (bool, error) {
	logger := log.New(os.Stderr)
	job := &JanitorJob{
		Roots:          roots,
		Patterns:       patterns,
		QuarantineRoot: paths.Expand("~/digest/.quarantine"),
		RetentionDays:  14,
		ReviewRoot:     reviewRoot,
		Logger:         logger,
	}
	res, err := job.Run(dryRun)
	if err != nil {
		return false, err
	}
	fmt.Println(res.Summary)
	return NeedsUserAction(res, dryRun), nil
}

// RunBranchReaper scans and prunes stale local git branches.
func RunBranchReaper(roots []string, dryRun bool) (bool, error) {
	logger := log.New(os.Stderr)
	job := &BranchReaperJob{
		Roots:      roots,
		MinAgeDays: 7,
		Logger:     logger,
	}
	res, err := job.Run(dryRun)
	if err != nil {
		return false, err
	}
	fmt.Println(res.Summary)
	return NeedsUserAction(res, dryRun), nil
}
