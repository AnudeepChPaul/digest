package sourcecontrol

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/review"
	"github.com/AnudeepChPaul/digest/pkg/system"

	"github.com/charmbracelet/log"
)

var ErrCloneInProgress = errors.New("the running review is still cloning; try again shortly")

var prepareClone = review.PrepareTo

func ClonePR(ctx context.Context, root string, pr review.QueuedPR) (string, error) {
	ref := pr.Ref
	review.AdoptLegacyDirs(root, ref)
	dir := review.CloneDir(root, ref)
	if review.CloneExists(root, ref) {
		return dir, nil
	}
	stateDir := review.StateDir(root, ref)
	if review.Status(stateDir) == review.RunRunning {
		return "", ErrCloneInProgress
	}
	logPath := filepath.Join(stateDir, "clone.log")
	if err := system.Write(logPath, nil); err != nil {
		return "", err
	}
	logOutput := system.LogWriter(logPath)
	if err := review.WriteMeta(stateDir, review.Meta{Ref: ref, Title: pr.Title, HeadSHA: pr.HeadSHA}); err != nil {
		return "", err
	}
	cloneCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	return prepareClone(cloneCtx, ref, root, log.New(logOutput), logOutput)
}
