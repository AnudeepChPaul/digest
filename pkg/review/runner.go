package review

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/log"
)

var prepareClone = Prepare

var shellSafeValue = regexp.MustCompile(`^[A-Za-z0-9._/:~+@-]+$`)

func expandReviewCommand(template string, values map[string]string) (string, error) {
	command := template
	for key, value := range values {
		if !shellSafeValue.MatchString(value) {
			return "", fmt.Errorf("refusing unsafe value for {%s}: %q", key, value)
		}
		command = strings.ReplaceAll(command, "{"+key+"}", value)
	}
	return command, nil
}

func Run(ctx context.Context, ref PRRef, root, commandTemplate string, logger *log.Logger) error {
	err := run(ctx, ref, root, commandTemplate, logger)
	if err != nil {
		_ = sendNotification("PR review failed", fmt.Sprintf("%s #%d: %v", ref.Repo, ref.Number, err), ref.URL)
	}
	return err
}

var parentPID = os.Getppid

func claimRun(stateDir string) (func(runErr error), error) {
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return nil, err
	}
	pidPath := filepath.Join(stateDir, pidFile)
	wrapped := false
	if pid, ok := readInt(pidPath); ok && processAlive(pid) {
		if pid != parentPID() {
			return nil, ErrReviewRunning
		}
		wrapped = true
	}
	if !wrapped {
		for _, stale := range []string{exitFile, FindingsFile} {
			_ = os.Remove(filepath.Join(stateDir, stale))
		}
	}
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		return nil, err
	}
	return func(runErr error) {
		if wrapped {
			return
		}
		exitCode := "0"
		if runErr != nil {
			exitCode = "1"
		}
		_ = os.WriteFile(filepath.Join(stateDir, exitFile), []byte(exitCode), 0644)
	}, nil
}

func run(ctx context.Context, ref PRRef, root, commandTemplate string, logger *log.Logger) (runErr error) {
	stateDir := StateDir(root, ref)
	release, err := claimRun(stateDir)
	if err != nil {
		return err
	}
	defer func() { release(runErr) }()
	findingsPath := filepath.Join(stateDir, FindingsFile)
	_ = os.Remove(findingsPath)
	if meta, err := ReadMeta(stateDir); err != nil || meta.Ref.URL != ref.URL {
		if err := WriteMeta(stateDir, Meta{Ref: ref}); err != nil {
			return err
		}
	}

	cloneDir, err := prepareClone(ctx, ref, root, logger)
	if err != nil {
		return err
	}

	command, err := expandReviewCommand(commandTemplate, map[string]string{
		"url":      ref.URL,
		"findings": findingsPath,
		"clone":    cloneDir,
	})
	if err != nil {
		return err
	}

	logger.Info("Running Claude review", "command", command)
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = cloneDir
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("review command failed: %w", err)
	}

	report, err := Load(stateDir)
	if err != nil {
		return fmt.Errorf("review produced no findings file: %w", err)
	}
	logger.Info("Review finished", "findings", len(report.Findings), "recommendation", report.Recommendation)
	message := fmt.Sprintf("%s #%d: %d findings, %s", ref.Repo, ref.Number, len(report.Findings), report.Recommendation)
	if err := sendNotification("PR reviewed", message, ref.URL); err != nil {
		logger.Warn("Notification failed", "err", err)
	}
	return nil
}
