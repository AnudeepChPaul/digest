package review

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/log"
)

var commandWaitDelay = 2 * time.Second

var runStep = func(ctx context.Context, output io.Writer, dir string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = commandWaitDelay
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "CI=true")
	cmd.Stdout = output
	cmd.Stderr = output
	return cmd.Run()
}

func Prepare(ctx context.Context, ref PRRef, root string, logger *log.Logger) (string, error) {
	return PrepareTo(ctx, ref, root, logger, os.Stdout)
}

func reuseClone(ctx context.Context, ref PRRef, dir string, logger *log.Logger, output io.Writer) error {
	logger.Info("Reusing existing clone", "dir", dir)
	steps := [][]string{
		{"git", "reset", "--hard"},
		{"git", "clean", "-fd"},
		{"gh", "pr", "checkout", fmt.Sprint(ref.Number), "--force"},
	}
	for _, step := range steps {
		if err := runStep(ctx, output, dir, step[0], step[1:]...); err != nil {
			return fmt.Errorf("%s: %w", strings.Join(step, " "), err)
		}
	}
	return nil
}

func freshClone(ctx context.Context, ref PRRef, root, dir string, logger *log.Logger, output io.Writer) error {
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove previous clone: %w", err)
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return err
	}
	logger.Info("Cloning repository", "repo", ref.CloneSpec(), "dir", dir)
	if err := runStep(ctx, output, root, "gh", "repo", "clone", ref.CloneSpec(), dir, "--", "--filter=blob:none", "--quiet"); err != nil {
		return fmt.Errorf("clone %s: %w", ref.CloneSpec(), err)
	}
	logger.Info("Checking out pull request", "number", ref.Number)
	if err := runStep(ctx, output, dir, "gh", "pr", "checkout", fmt.Sprint(ref.Number)); err != nil {
		return fmt.Errorf("checkout PR #%d: %w", ref.Number, err)
	}
	return nil
}

func PrepareTo(ctx context.Context, ref PRRef, root string, logger *log.Logger, output io.Writer) (string, error) {
	dir := CloneDir(root, ref)
	if _, statErr := os.Stat(filepath.Join(dir, ".git")); statErr == nil {
		if err := reuseClone(ctx, ref, dir, logger, output); err != nil {
			if ctx.Err() != nil {
				return "", err
			}
			logger.Warn("Reusing clone failed, cloning fresh", "err", err)
		} else {
			return dir, installDependencies(ctx, dir, logger, output)
		}
	}
	if err := freshClone(ctx, ref, root, dir, logger, output); err != nil {
		return "", err
	}
	return dir, installDependencies(ctx, dir, logger, output)
}

func installDependencies(ctx context.Context, dir string, logger *log.Logger, output io.Writer) error {
	if script := installScript(dir); script != "" {
		logger.Info("Installing dependencies", "command", script)
		if err := runStep(ctx, output, dir, "bash", "-c", script); err != nil {
			logger.Warn("Dependency install failed, reviewing without dependencies", "err", err)
		}
	} else {
		logger.Info("No supported lockfile found, skipping dependency install")
	}
	return nil
}
