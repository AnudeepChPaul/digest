package review

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/system"
)

const (
	pidFile      = "review.pid"
	exitFile     = "review.exit"
	LogFile      = "review.log"
	metaFile     = "meta.json"
	FindingsFile = "findings.json"
	stateDirName = ".state"
)

type RunStatus int

const (
	RunIdle RunStatus = iota
	RunRunning
	RunFailed
	RunDone
)

type Meta struct {
	Ref     PRRef  `json:"ref"`
	Title   string `json:"title"`
	HeadSHA string `json:"head_sha"`
}

func CloneDir(root string, ref PRRef) string {
	return filepath.Join(root, ref.DirName())
}

func StateDir(root string, ref PRRef) string {
	return filepath.Join(root, stateDirName, ref.DirName())
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func readInt(path string) (int, bool) {
	data, err := system.Read(path)
	if err != nil {
		return 0, false
	}
	value, err := strconv.Atoi(strings.TrimSpace(string(data)))
	return value, err == nil
}

func Status(dir string) RunStatus {
	pidPath := filepath.Join(dir, pidFile)
	if pid, ok := readInt(pidPath); ok {
		if processAlive(pid) {
			return RunRunning
		}
		_ = system.Remove(pidPath)
	}
	exitCode, ok := readInt(filepath.Join(dir, exitFile))
	if !ok {
		if system.Exists(filepath.Join(dir, exitFile)) {
			return RunFailed
		}
		if system.Exists(filepath.Join(dir, FindingsFile)) {
			return RunDone
		}
		return RunIdle
	}
	if exitCode != 0 {
		return RunFailed
	}
	if !system.Exists(filepath.Join(dir, FindingsFile)) {
		return RunFailed
	}
	return RunDone
}

func LocalReviewFinishedAt(dir string) (time.Time, bool) {
	for _, name := range []string{FindingsFile, exitFile} {
		if info, err := system.Stat(filepath.Join(dir, name)); err == nil {
			return info.ModTime(), true
		}
	}
	return time.Time{}, false
}

func WriteMeta(dir string, meta Meta) error {
	return system.WriteJSON(filepath.Join(dir, metaFile), meta)
}

func LocalReviewOutdated(root string, pr QueuedPR) bool {
	if pr.HeadSHA == "" {
		return false
	}
	dir := StateDir(root, pr.Ref)
	if Status(dir) != RunDone {
		return false
	}
	meta, err := ReadMeta(dir)
	return err == nil && meta.HeadSHA != "" && meta.HeadSHA != pr.HeadSHA
}

func ReadMeta(dir string) (Meta, error) {
	var meta Meta
	data, err := system.Read(filepath.Join(dir, metaFile))
	if err != nil {
		return meta, err
	}
	err = json.Unmarshal(data, &meta)
	return meta, err
}

func clearExitedPID(pidPath string) {
	if pid, ok := readInt(pidPath); ok && !processAlive(pid) {
		_ = system.Remove(pidPath)
	}
}

var stopGracePeriod = 3 * time.Second

func terminateGroup(target int) error {
	if err := syscall.Kill(target, syscall.SIGTERM); err != nil {
		return err
	}
	deadline := time.Now().Add(stopGracePeriod)
	go func() {
		for time.Now().Before(deadline) {
			if syscall.Kill(target, 0) != nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		_ = syscall.Kill(target, syscall.SIGKILL)
	}()
	return nil
}

var ErrReviewRunning = errors.New("a review is already running for this PR")

func StartBackground(pr QueuedPR, root string) error {
	AdoptLegacyDirs(root, pr.Ref)
	dir := StateDir(root, pr.Ref)
	if Status(dir) == RunRunning {
		return ErrReviewRunning
	}
	if err := system.MkdirAll(dir); err != nil {
		return err
	}
	for _, stale := range []string{exitFile, FindingsFile, pidFile} {
		_ = system.Remove(filepath.Join(dir, stale))
	}
	if err := WriteMeta(dir, Meta{Ref: pr.Ref, Title: pr.Title, HeadSHA: pr.HeadSHA}); err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate digest binary: %w", err)
	}
	logFile, err := system.OpenLog(filepath.Join(dir, LogFile))
	if err != nil {
		return err
	}
	defer logFile.Close()
	pidPath := filepath.Join(dir, pidFile)
	cmd := exec.Command(executable, "pr-review", "--url", pr.Ref.URL)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.Dir = root
	if err := cmd.Start(); err != nil {
		return err
	}
	pidErr := system.Write(pidPath, []byte(strconv.Itoa(cmd.Process.Pid)))
	go func() {
		_ = cmd.Wait()
		clearExitedPID(pidPath)
	}()
	if pidErr != nil {
		return fmt.Errorf("review started but its pid file could not be written: %w", pidErr)
	}
	return nil
}

type ReviewRun struct {
	Meta      Meta
	Status    RunStatus
	StartedAt time.Time
}

func ListRuns(root string) []ReviewRun {
	stateRoot := filepath.Join(root, ".state")
	entries, err := system.List(stateRoot)
	if err != nil {
		return nil
	}
	var runs []ReviewRun
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(stateRoot, entry.Name())
		status := Status(dir)
		if status != RunRunning && status != RunFailed {
			continue
		}
		meta, err := ReadMeta(dir)
		if err != nil || meta.Ref.URL == "" {
			continue
		}
		run := ReviewRun{Meta: meta, Status: status}
		if info, err := system.Stat(filepath.Join(dir, metaFile)); err == nil {
			run.StartedAt = info.ModTime()
		}
		runs = append(runs, run)
	}
	sort.SliceStable(runs, func(i, j int) bool {
		if runs[i].Status != runs[j].Status {
			return runs[i].Status == RunRunning
		}
		return runs[i].StartedAt.After(runs[j].StartedAt)
	})
	return runs
}

func RunningPID(dir string) (int, bool) {
	pid, ok := readInt(filepath.Join(dir, pidFile))
	if !ok || !processAlive(pid) {
		return 0, false
	}
	return pid, true
}

func Stop(root string, ref PRRef) error {
	dir := StateDir(root, ref)
	pid, running := RunningPID(dir)
	if running {
		target := pid
		if groupID, err := syscall.Getpgid(pid); err == nil && groupID != syscall.Getpgrp() {
			target = -groupID
		}
		if err := terminateGroup(target); err != nil {
			return fmt.Errorf("stop review %s: %w", ref.DirName(), err)
		}
	}
	if err := system.Remove(filepath.Join(dir, pidFile)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func AdoptLegacyDirs(root string, ref PRRef) {
	if ref.Owner == "" || ref.URL == "" {
		return
	}
	stateDir := StateDir(root, ref)
	if system.Exists(stateDir) {
		return
	}
	legacyState := filepath.Join(root, stateDirName, ref.legacyDirName())
	if meta, err := ReadMeta(legacyState); err != nil || meta.Ref.URL != ref.URL || Status(legacyState) == RunRunning {
		return
	}
	legacyClone := filepath.Join(root, ref.legacyDirName())
	if system.Exists(legacyClone) {
		if err := system.Rename(legacyClone, CloneDir(root, ref)); err != nil {
			return
		}
	}
	_ = system.Rename(legacyState, stateDir)
}
