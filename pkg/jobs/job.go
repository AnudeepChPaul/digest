package jobs

import (
	"fmt"
	"os"

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

func RunJanitor(roots, patterns []string, reviewRoot, quarantineRoot string, graceDays int, dryRun bool) (bool, error) {
	return RunJanitorJob(&JanitorJob{
		Roots:          roots,
		Patterns:       patterns,
		QuarantineRoot: quarantineRoot,
		GraceDays:      graceDays,
		ReviewRoot:     reviewRoot,
	}, dryRun)
}

func RunJanitorJob(job *JanitorJob, dryRun bool) (bool, error) {
	if job.Logger == nil {
		job.Logger = log.New(os.Stderr)
	}
	res, err := job.Run(dryRun)
	if err != nil {
		return false, err
	}
	fmt.Println(res.Summary)
	return NeedsUserAction(res, dryRun), nil
}

func RunBranchReaper(roots []string, graceDays int, dryRun bool) (bool, error) {
	logger := log.New(os.Stderr)
	job := &BranchReaperJob{
		Roots:      roots,
		MinAgeDays: graceDays,
		Logger:     logger,
	}
	res, err := job.Run(dryRun)
	if err != nil {
		return false, err
	}
	fmt.Println(res.Summary)
	return NeedsUserAction(res, dryRun), nil
}
