package review

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/system"

	"github.com/charmbracelet/log"
)

var prepareClone = PrepareTo

var shellSafeValue = regexp.MustCompile(`^[A-Za-z0-9._/:~+@-]+$`)

var findingsTarget = regexp.MustCompile(`[ \t]*--emit-to[ =]\{findings\}`)

var ErrNoFindingsJSON = errors.New("review produced no findings JSON")

func withoutFindingsTarget(template string) string {
	return strings.ReplaceAll(findingsTarget.ReplaceAllString(template, ""), "{findings}", "")
}

func expandReviewCommand(template string, values map[string]string) (string, error) {
	command := withoutFindingsTarget(template)
	for key, value := range values {
		if !shellSafeValue.MatchString(value) {
			return "", fmt.Errorf("refusing unsafe value for {%s}: %q", key, value)
		}
		command = strings.ReplaceAll(command, "{"+key+"}", value)
	}
	return command, nil
}

func findingsJSON(output []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(output)
	if bytes.HasPrefix(trimmed, []byte("{")) && json.Valid(trimmed) {
		return trimmed, nil
	}
	first, last := bytes.IndexByte(trimmed, '{'), bytes.LastIndexByte(trimmed, '}')
	if first < 0 || last <= first || !json.Valid(trimmed[first:last+1]) {
		return nil, ErrNoFindingsJSON
	}
	return trimmed[first : last+1], nil
}

func Run(ctx context.Context, ref PRRef, root, commandTemplate string, logger *log.Logger) error {
	err := run(ctx, ref, root, commandTemplate, logger, os.Stdout, os.Stderr)
	if err != nil {
		_ = sendNotification("PR review failed", fmt.Sprintf("%s #%d: %v", ref.Repo, ref.Number, err), ref.URL)
	}
	return err
}

var parentPID = os.Getppid

var reviewCommandTimeout = 45 * time.Minute

var cloneTimeout = 20 * time.Minute

var errReviewStopped = errors.New("review stopped")

func claimRun(stateDir string) (func(runErr error), error) {
	if err := system.MkdirAll(stateDir); err != nil {
		return nil, err
	}
	pidPath := filepath.Join(stateDir, pidFile)
	ownPID := os.Getpid()
	wrapped := false
	if pid, ok := readInt(pidPath); ok && pid != ownPID && processAlive(pid) {
		if pid != parentPID() {
			return nil, ErrReviewRunning
		}
		wrapped = true
	}
	if !wrapped {
		for _, stale := range []string{exitFile, FindingsFile} {
			_ = system.Remove(filepath.Join(stateDir, stale))
		}
	}
	if err := system.Write(pidPath, []byte(strconv.Itoa(ownPID))); err != nil {
		return nil, err
	}
	return func(runErr error) {
		if wrapped {
			return
		}
		if !errors.Is(runErr, errReviewStopped) {
			exitCode := "0"
			if runErr != nil {
				exitCode = "1"
			}
			_ = system.Write(filepath.Join(stateDir, exitFile), []byte(exitCode))
		}
		if pid, ok := readInt(pidPath); ok && pid == ownPID {
			_ = system.Remove(pidPath)
		}
	}, nil
}

func run(ctx context.Context, ref PRRef, root, commandTemplate string, logger *log.Logger, stdout, stderr io.Writer) (runErr error) {
	AdoptLegacyDirs(root, ref)
	stateDir := StateDir(root, ref)
	release, err := claimRun(stateDir)
	if err != nil {
		return err
	}
	defer func() {
		if runErr != nil && ctx.Err() != nil {
			release(errReviewStopped)
			return
		}
		release(runErr)
	}()
	findingsPath := filepath.Join(stateDir, FindingsFile)
	_ = system.Remove(findingsPath)
	if meta, err := ReadMeta(stateDir); err != nil || meta.Ref.URL != ref.URL {
		if err := WriteMeta(stateDir, Meta{Ref: ref}); err != nil {
			return err
		}
	}

	cloneCtx, cancelClone := context.WithTimeout(ctx, cloneTimeout)
	cloneDir, err := prepareClone(cloneCtx, ref, root, logger, stdout)
	cancelClone()
	if err != nil {
		return err
	}

	command, err := expandReviewCommand(commandTemplate, map[string]string{
		"url":   ref.URL,
		"clone": cloneDir,
	})
	if err != nil {
		return err
	}

	logger.Info("Running Claude review", "command", command)
	reviewCtx, cancelReview := context.WithTimeout(ctx, reviewCommandTimeout)
	defer cancelReview()
	var reviewOutput bytes.Buffer
	cmd := exec.CommandContext(reviewCtx, "sh", "-c", command)
	cmd.WaitDelay = commandWaitDelay
	killGroupOnCancel(cmd)
	cmd.Dir = cloneDir
	cmd.Env = ReviewEnv()
	cmd.Stdout = io.MultiWriter(&reviewOutput, stdout)
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("review command failed: %w", err)
	}

	findings, err := findingsJSON(reviewOutput.Bytes())
	if err != nil {
		return err
	}
	if err := system.Write(findingsPath, append(findings, '\n')); err != nil {
		return fmt.Errorf("save findings: %w", err)
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
