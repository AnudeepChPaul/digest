package review

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/paths"

	"github.com/charmbracelet/log"
)

var commandWaitDelay = 2 * time.Second

var runStep = func(ctx context.Context, output io.Writer, dir string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = commandWaitDelay
	killGroupOnCancel(cmd)
	cmd.Dir = dir
	cmd.Env = stepEnv()
	cmd.Stdout = output
	cmd.Stderr = output
	return cmd.Run()
}

var runInstall = func(ctx context.Context, output io.Writer, dir, script string) error {
	cmd := exec.CommandContext(ctx, "bash", "-c", script)
	cmd.WaitDelay = commandWaitDelay
	killGroupOnCancel(cmd)
	cmd.Dir = dir
	cmd.Env = installEnv()
	cmd.Stdout = output
	cmd.Stderr = output
	return cmd.Run()
}

func killGroupOnCancel(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
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

const PartialCloneSuffix = ".partial"

func CloneExists(root string, ref PRRef) bool {
	_, err := os.Stat(filepath.Join(CloneDir(root, ref), ".git"))
	return err == nil
}

func freshClone(ctx context.Context, ref PRRef, root, dir string, logger *log.Logger, output io.Writer) error {
	partialDir := dir + PartialCloneSuffix
	for _, stale := range []string{dir, partialDir} {
		if err := os.RemoveAll(stale); err != nil {
			return fmt.Errorf("remove previous clone: %w", err)
		}
	}
	if err := os.MkdirAll(root, paths.PrivateDirMode); err != nil {
		return err
	}
	logger.Info("Cloning repository", "repo", ref.CloneSpec(), "dir", dir)
	if err := runStep(ctx, output, root, "gh", "repo", "clone", ref.CloneSpec(), partialDir, "--", "--filter=blob:none", "--quiet"); err != nil {
		return fmt.Errorf("clone %s: %w", ref.CloneSpec(), err)
	}
	logger.Info("Checking out pull request", "number", ref.Number)
	if err := runStep(ctx, output, partialDir, "gh", "pr", "checkout", fmt.Sprint(ref.Number)); err != nil {
		return fmt.Errorf("checkout PR #%d: %w", ref.Number, err)
	}
	return os.Rename(partialDir, dir)
}

func PrepareTo(ctx context.Context, ref PRRef, root string, logger *log.Logger, output io.Writer) (string, error) {
	dir := CloneDir(root, ref)
	if CloneExists(root, ref) {
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
	if reason := riskyInstallConfig(dir); reason != "" {
		logger.Warn("Skipping dependency install", "reason", reason)
		return nil
	}
	if script := installScript(dir); script != "" {
		logger.Info("Installing dependencies", "command", script)
		if err := runInstall(ctx, output, dir, script); err != nil {
			logger.Warn("Dependency install failed, reviewing without dependencies", "err", err)
		}
	} else {
		logger.Info("No supported lockfile found, skipping dependency install")
	}
	return nil
}
