package review

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/charmbracelet/log"
)

var runStep = func(ctx context.Context, output io.Writer, dir string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "CI=true")
	cmd.Stdout = output
	cmd.Stderr = output
	return cmd.Run()
}

func Prepare(ctx context.Context, ref PRRef, root string, logger *log.Logger) (string, error) {
	return PrepareTo(ctx, ref, root, logger, os.Stdout)
}

func PrepareTo(ctx context.Context, ref PRRef, root string, logger *log.Logger, output io.Writer) (string, error) {
	dir := CloneDir(root, ref)
	if err := os.RemoveAll(dir); err != nil {
		return "", fmt.Errorf("remove previous clone: %w", err)
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return "", err
	}

	logger.Info("Cloning repository", "repo", ref.CloneSpec(), "dir", dir)
	if err := runStep(ctx, output, root, "gh", "repo", "clone", ref.CloneSpec(), dir, "--", "--filter=blob:none", "--quiet"); err != nil {
		return "", fmt.Errorf("clone %s: %w", ref.CloneSpec(), err)
	}

	logger.Info("Checking out pull request", "number", ref.Number)
	if err := runStep(ctx, output, dir, "gh", "pr", "checkout", fmt.Sprint(ref.Number)); err != nil {
		return "", fmt.Errorf("checkout PR #%d: %w", ref.Number, err)
	}

	if script := installScript(dir); script != "" {
		logger.Info("Installing dependencies", "command", script)
		if err := runStep(ctx, output, dir, "bash", "-c", script); err != nil {
			logger.Warn("Dependency install failed, reviewing without dependencies", "err", err)
		}
	} else {
		logger.Info("No supported lockfile found, skipping dependency install")
	}
	return dir, nil
}
