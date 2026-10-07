package jobs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"app/pkg/paths"
	"app/pkg/review"

	"github.com/charmbracelet/log"
)

type JanitorJob struct {
	Roots          []string
	Patterns       []string
	QuarantineRoot string
	RetentionDays  int
	ReviewRoot     string
	Logger         *log.Logger
}

var lookupPRStates = review.FetchPRStates

const reviewCloneIdleAge = 7 * 24 * time.Hour

func reviewCloneLastTouched(root string, ref review.PRRef) time.Time {
	stateDir := review.StateDir(root, ref)
	cloneDir := review.CloneDir(root, ref)
	candidates := []string{stateDir, cloneDir, filepath.Join(cloneDir, ".git")}
	if entries, err := os.ReadDir(stateDir); err == nil {
		for _, entry := range entries {
			candidates = append(candidates, filepath.Join(stateDir, entry.Name()))
		}
	}
	var latest time.Time
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}
	return latest
}

func removeReviewClone(root string, ref review.PRRef, reason string, dryRun bool) (string, string) {
	cloneDir := review.CloneDir(root, ref)
	if dryRun {
		return fmt.Sprintf("would remove review clone %s (%s)", cloneDir, reason), ""
	}
	if err := os.RemoveAll(cloneDir); err != nil {
		return "", fmt.Sprintf("remove %s: %v", cloneDir, err)
	}
	if err := os.RemoveAll(review.StateDir(root, ref)); err != nil {
		return "", fmt.Sprintf("remove state for %s: %v", ref.DirName(), err)
	}
	return fmt.Sprintf("removed review clone %s (%s)", cloneDir, reason), ""
}

func reapReviewClones(root string, dryRun bool) ([]string, []string) {
	stateRoot := filepath.Join(root, ".state")
	entries, err := os.ReadDir(stateRoot)
	if err != nil {
		return nil, nil
	}
	var actions, failures []string
	record := func(action, failure string) {
		if action != "" {
			actions = append(actions, action)
		}
		if failure != "" {
			failures = append(failures, failure)
		}
	}
	idleCutoff := time.Now().Add(-reviewCloneIdleAge)
	var refs []review.PRRef
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		stateDir := filepath.Join(stateRoot, entry.Name())
		if review.Status(stateDir) == review.RunRunning {
			continue
		}
		meta, err := review.ReadMeta(stateDir)
		if err != nil || meta.Ref.URL == "" {
			continue
		}
		if reviewCloneLastTouched(root, meta.Ref).Before(idleCutoff) {
			record(removeReviewClone(root, meta.Ref, "idle 7d", dryRun))
			continue
		}
		refs = append(refs, meta.Ref)
	}
	if len(refs) == 0 {
		return actions, failures
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	states, err := lookupPRStates(ctx, refs)
	if err != nil {
		return actions, append(failures, fmt.Sprintf("pr states: %v", err))
	}
	for _, ref := range refs {
		state, found := states[ref.URL]
		if !found {
			failures = append(failures, fmt.Sprintf("pr state %s: not returned", ref.URL))
			continue
		}
		if state != "MERGED" && state != "CLOSED" {
			continue
		}
		record(removeReviewClone(root, ref, strings.ToLower(state), dryRun))
	}
	return actions, failures
}

func (j *JanitorJob) Run(dryRun bool) (*JobResult, error) {
	logger := j.Logger
	if logger == nil {
		logger = log.New(os.Stderr)
	}

	logger.Info("Scanning for cleanup targets", "roots", strings.Join(j.Roots, ", "))

	now := time.Now()
	quarantineRoot := paths.Expand(j.QuarantineRoot)
	dateDir := filepath.Join(quarantineRoot, now.Format("2006-01-02"))

	var matched []string
	var failures []string
	var reclaimed int64

	for _, root := range j.Roots {
		expanded := paths.Expand(root)
		entries, err := os.ReadDir(expanded)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}

			for _, pattern := range j.Patterns {
				ok, _ := filepath.Match(pattern, entry.Name())
				if !ok {
					continue
				}

				src := filepath.Join(expanded, entry.Name())
				info, err := entry.Info()
				if err != nil {
					logger.Error("Failed to stat file", "file", src, "err", err)
					failures = append(failures, fmt.Sprintf("stat %s: %v", src, err))
					break
				}

				dest := filepath.Join(dateDir, entry.Name())
				if dryRun {
					logger.Warn("Would quarantine", "file", entry.Name(), "size_bytes", info.Size())
				} else {
					if err := os.MkdirAll(dateDir, paths.PrivateDirMode); err != nil {
						logger.Error("Failed to create quarantine directory", "dir", dateDir, "err", err)
						failures = append(failures, fmt.Sprintf("mkdir %s: %v", dateDir, err))
						break
					}
					if err := os.Rename(src, dest); err != nil {
						logger.Error("Failed to quarantine file", "file", src, "err", err)
						failures = append(failures, fmt.Sprintf("move %s: %v", src, err))
						break
					}
					logger.Info("Quarantined file", "file", entry.Name(), "dest", dest)
				}
				reclaimed += info.Size()
				matched = append(matched, fmt.Sprintf("%s -> %s (%d bytes)", src, dest, info.Size()))
				break
			}
		}
	}

	var purged []string
	cutoff := now.AddDate(0, 0, -j.RetentionDays)

	qEntries, _ := os.ReadDir(quarantineRoot)
	for _, qe := range qEntries {
		if !qe.IsDir() {
			continue
		}
		t, err := time.Parse("2006-01-02", qe.Name())
		if err == nil && t.Before(cutoff) {
			targetDir := filepath.Join(quarantineRoot, qe.Name())
			purged = append(purged, fmt.Sprintf("purged quarantine from %s", qe.Name()))
			if dryRun {
				logger.Warn("Would purge expired quarantine", "batch", qe.Name())
			} else {
				_ = os.RemoveAll(targetDir)
				logger.Info("Purged expired quarantine", "batch", qe.Name())
			}
		}
	}

	var reviewActions []string
	if j.ReviewRoot != "" {
		var reviewFailures []string
		reviewActions, reviewFailures = reapReviewClones(paths.Expand(j.ReviewRoot), dryRun)
		failures = append(failures, reviewFailures...)
		for _, action := range reviewActions {
			if dryRun {
				logger.Warn("Review clone", "action", action)
			} else {
				logger.Info("Review clone", "action", action)
			}
		}
	}

	summary := fmt.Sprintf("%d quarantined · %d KiB reclaimed · %d expired batches purged · %d review clones removed",
		len(matched), reclaimed/1024, len(purged), len(reviewActions))
	if len(failures) > 0 {
		summary = fmt.Sprintf("%s · %d failed", summary, len(failures))
	}

	return &JobResult{
		Changed:      len(matched) > 0 || len(purged) > 0 || len(reviewActions) > 0,
		Degraded:     len(failures) > 0,
		Summary:      summary,
		Details:      strings.Join(failures, "\n"),
		ActionsTaken: append(append(matched, purged...), reviewActions...),
	}, nil
}
