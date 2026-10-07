package sourcecontrol

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"app/pkg/paths"
	"app/pkg/review"

	"github.com/charmbracelet/log"
)

var ErrCloneInProgress = errors.New("the running review is still cloning; try again shortly")

var prepareClone = review.PrepareTo

func ClonePR(ctx context.Context, root string, pr review.QueuedPR) (string, error) {
	ref := pr.Ref
	dir := review.CloneDir(root, ref)
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return dir, nil
	}
	stateDir := review.StateDir(root, ref)
	if review.Status(stateDir) == review.RunRunning {
		return "", ErrCloneInProgress
	}
	if err := os.MkdirAll(stateDir, paths.PrivateDirMode); err != nil {
		return "", err
	}
	logOutput, err := paths.CreatePrivate(filepath.Join(stateDir, "clone.log"))
	if err != nil {
		return "", err
	}
	defer logOutput.Close()
	if err := review.WriteMeta(stateDir, review.Meta{Ref: ref, Title: pr.Title, HeadSHA: pr.HeadSHA}); err != nil {
		return "", err
	}
	cloneCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	return prepareClone(cloneCtx, ref, root, log.New(logOutput), logOutput)
}
