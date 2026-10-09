package jobs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/paths"
	"github.com/AnudeepChPaul/digest/pkg/review"
	"github.com/AnudeepChPaul/digest/pkg/system"

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

type reviewFolders struct {
	ref      review.PRRef
	stateDir string
	cloneDir string
}

func foldersOf(root, stateRoot, name string) reviewFolders {
	return reviewFolders{stateDir: filepath.Join(stateRoot, name), cloneDir: filepath.Join(root, name)}
}

func reviewRunning(stateDir string) bool {
	_, running := review.RunningPID(stateDir)
	return running
}

func (folders reviewFolders) lastTouched() time.Time {
	candidates := []string{folders.stateDir, folders.cloneDir, filepath.Join(folders.cloneDir, ".git")}
	if entries, err := system.List(folders.stateDir); err == nil {
		for _, entry := range entries {
			candidates = append(candidates, filepath.Join(folders.stateDir, entry.Name()))
		}
	}
	var latest time.Time
	for _, path := range candidates {
		if info, err := system.Stat(path); err == nil && info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}
	return latest
}

func (folders reviewFolders) remove(reason string, dryRun bool) (string, string) {
	if dryRun {
		return fmt.Sprintf("would remove review clone %s (%s)", folders.cloneDir, reason), ""
	}
	for _, dir := range []string{folders.cloneDir, folders.cloneDir + review.PartialCloneSuffix} {
		if err := system.RemoveAll(dir); err != nil {
			return "", fmt.Sprintf("remove %s: %v", dir, err)
		}
	}
	if err := system.RemoveAll(folders.stateDir); err != nil {
		return "", fmt.Sprintf("remove state %s: %v", folders.stateDir, err)
	}
	return fmt.Sprintf("removed review clone %s (%s)", folders.cloneDir, reason), ""
}

func reapPartialClones(root, stateRoot string, dryRun bool, record func(action, failure string)) {
	entries, err := system.List(root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		cloneName, isPartial := strings.CutSuffix(entry.Name(), review.PartialCloneSuffix)
		if !entry.IsDir() || !isPartial || reviewRunning(filepath.Join(stateRoot, cloneName)) {
			continue
		}
		partialDir := filepath.Join(root, entry.Name())
		if dryRun {
			record(fmt.Sprintf("would remove partial clone %s", partialDir), "")
			continue
		}
		if err := system.RemoveAll(partialDir); err != nil {
			record("", fmt.Sprintf("remove %s: %v", partialDir, err))
			continue
		}
		record(fmt.Sprintf("removed partial clone %s", partialDir), "")
	}
}

func reapReviewClones(root string, dryRun bool) ([]string, []string) {
	var actions, failures []string
	record := func(action, failure string) {
		if action != "" {
			actions = append(actions, action)
		}
		if failure != "" {
			failures = append(failures, failure)
		}
	}
	stateRoot := filepath.Join(root, ".state")
	reapPartialClones(root, stateRoot, dryRun, record)
	entries, err := system.List(stateRoot)
	if err != nil {
		return actions, failures
	}
	idleCutoff := time.Now().Add(-reviewCloneIdleAge)
	var candidates []reviewFolders
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		folders := foldersOf(root, stateRoot, entry.Name())
		if reviewRunning(folders.stateDir) {
			continue
		}
		meta, err := review.ReadMeta(folders.stateDir)
		if err != nil || meta.Ref.URL == "" {
			continue
		}
		folders.ref = meta.Ref
		if folders.lastTouched().Before(idleCutoff) {
			record(folders.remove("idle 7d", dryRun))
			continue
		}
		candidates = append(candidates, folders)
	}
	if len(candidates) == 0 {
		return actions, failures
	}
	refs := make([]review.PRRef, len(candidates))
	for index, folders := range candidates {
		refs[index] = folders.ref
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	states, err := lookupPRStates(ctx, refs)
	if err != nil {
		return actions, append(failures, fmt.Sprintf("pr states: %v", err))
	}
	for _, folders := range candidates {
		state, found := states[folders.ref.URL]
		if !found {
			failures = append(failures, fmt.Sprintf("pr state %s: not returned", folders.ref.URL))
			continue
		}
		if state != "MERGED" && state != "CLOSED" {
			continue
		}
		record(folders.remove(strings.ToLower(state), dryRun))
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
		entries, err := system.List(expanded)
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
					if err := system.MkdirAll(dateDir); err != nil {
						logger.Error("Failed to create quarantine directory", "dir", dateDir, "err", err)
						failures = append(failures, fmt.Sprintf("mkdir %s: %v", dateDir, err))
						break
					}
					if err := system.Rename(src, dest); err != nil {
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

	qEntries, _ := system.List(quarantineRoot)
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
				_ = system.RemoveAll(targetDir)
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

	summaryFormat := "%d quarantined · %d KiB reclaimed · %d expired batches purged · %d review clones removed"
	if dryRun {
		summaryFormat = "%d to quarantine · %d KiB to reclaim · %d expired batches to purge · %d review clones to remove"
	}
	summary := fmt.Sprintf(summaryFormat, len(matched), reclaimed/1024, len(purged), len(reviewActions))
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
