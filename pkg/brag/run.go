package brag

import (
	"context"
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

	"github.com/achandrapaul/digest/pkg/config"
	"github.com/achandrapaul/digest/pkg/model"
	"github.com/achandrapaul/digest/pkg/system"
)

const (
	runPIDFile   = "brag.pid"
	runExitFile  = "brag.exit"
	RunLogFile   = "brag.log"
	runMetaFile  = "meta.json"
	stateDirName = ".state"
)

type RunStatus int

const (
	RunIdle RunStatus = iota
	RunRunning
	RunFailed
	RunDone
)

type RunMeta struct {
	ID         string `json:"id"`
	Regenerate bool   `json:"regenerate"`
}

type Run struct {
	Meta      RunMeta
	Status    RunStatus
	StartedAt time.Time
}

var ErrBragRunning = errors.New("a brag is already being generated for this period")

var executablePath = os.Executable

func StateDir(root, id string) string {
	return filepath.Join(root, stateDirName, id)
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

func runningPID(root, id string) (int, bool) {
	pidPath := filepath.Join(StateDir(root, id), runPIDFile)
	pid, ok := readInt(pidPath)
	if !ok {
		return 0, false
	}
	if !processAlive(pid) {
		_ = system.Remove(pidPath)
		return 0, false
	}
	return pid, true
}

func clearExitedPID(pidPath string) {
	if pid, ok := readInt(pidPath); ok && !processAlive(pid) {
		_ = system.Remove(pidPath)
	}
}

func recordUnreportedExit(dir string, state *os.ProcessState) {
	exitPath := filepath.Join(dir, runExitFile)
	if state == nil || !state.Exited() || system.Exists(exitPath) {
		return
	}
	_ = system.Write(exitPath, []byte(strconv.Itoa(state.ExitCode())))
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

func IsRunning(root, id string) bool {
	_, running := runningPID(root, id)
	return running
}

func Status(root, id string) RunStatus {
	if IsRunning(root, id) {
		return RunRunning
	}
	exitPath := filepath.Join(StateDir(root, id), runExitFile)
	exitCode, ok := readInt(exitPath)
	if !ok {
		if system.Exists(exitPath) {
			return RunFailed
		}
		return RunIdle
	}
	if exitCode != 0 {
		return RunFailed
	}
	return RunDone
}

func writeMeta(dir string, meta RunMeta) error {
	return system.WriteJSON(filepath.Join(dir, runMetaFile), meta)
}

func StartBackground(root string, period Period, regenerate bool) error {
	id := period.ID()
	if IsRunning(root, id) {
		return ErrBragRunning
	}
	dir := StateDir(root, id)
	if err := system.MkdirAll(dir); err != nil {
		return err
	}
	for _, stale := range []string{runExitFile, runPIDFile} {
		_ = system.Remove(filepath.Join(dir, stale))
	}
	if err := writeMeta(dir, RunMeta{ID: id, Regenerate: regenerate}); err != nil {
		return err
	}
	executable, err := executablePath()
	if err != nil {
		return fmt.Errorf("locate digest binary: %w", err)
	}
	logFile, err := system.OpenLog(filepath.Join(dir, RunLogFile))
	if err != nil {
		return err
	}
	defer logFile.Close()
	regenerateFlag := "--regenerate=false"
	if regenerate {
		regenerateFlag = "--regenerate"
	}
	pidPath := filepath.Join(dir, runPIDFile)
	cmd := exec.Command(executable, "brag", "--"+string(period.Kind()), id, regenerateFlag)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.Dir = root
	if err := cmd.Start(); err != nil {
		return err
	}
	pidErr := system.Write(pidPath, []byte(strconv.Itoa(cmd.Process.Pid)))
	go func() {
		_ = cmd.Wait()
		recordUnreportedExit(dir, cmd.ProcessState)
		clearExitedPID(pidPath)
	}()
	if pidErr != nil {
		return fmt.Errorf("brag started but its pid file could not be written: %w", pidErr)
	}
	return nil
}

func ListRuns(root string) []Run {
	entries, err := system.List(filepath.Join(root, stateDirName))
	if err != nil {
		return nil
	}
	var runs []Run
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		status := Status(root, id)
		if status != RunRunning && status != RunFailed {
			continue
		}
		run := Run{Meta: RunMeta{ID: id}, Status: status}
		_, _ = system.ReadJSON(filepath.Join(StateDir(root, id), runMetaFile), &run.Meta)
		if info, err := system.Stat(filepath.Join(StateDir(root, id), runMetaFile)); err == nil {
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

func Stop(root, id string) error {
	dir := StateDir(root, id)
	if pid, running := runningPID(root, id); running {
		target := pid
		if groupID, err := syscall.Getpgid(pid); err == nil && groupID != syscall.Getpgrp() {
			target = -groupID
		}
		if err := terminateGroup(target); err != nil {
			return fmt.Errorf("stop brag %s: %w", id, err)
		}
	}
	if err := system.Remove(filepath.Join(dir, runPIDFile)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return system.Write(filepath.Join(dir, runExitFile), []byte("143"))
}

func Dismiss(root, id string) error {
	if IsRunning(root, id) {
		return ErrBragRunning
	}
	return system.RemoveAll(StateDir(root, id))
}

func claimRun(root, id string) (func(runErr error), error) {
	dir := StateDir(root, id)
	if err := system.MkdirAll(dir); err != nil {
		return nil, err
	}
	pidPath := filepath.Join(dir, runPIDFile)
	if pid, ok := readInt(pidPath); ok && processAlive(pid) && pid != os.Getpid() {
		return nil, ErrBragRunning
	}
	_ = system.Remove(filepath.Join(dir, runExitFile))
	if err := system.Write(pidPath, []byte(strconv.Itoa(os.Getpid()))); err != nil {
		return nil, err
	}
	return func(runErr error) {
		exitCode := "0"
		if runErr != nil {
			exitCode = "1"
		}
		_ = system.Write(filepath.Join(dir, runExitFile), []byte(exitCode))
		_ = system.Remove(pidPath)
	}, nil
}

func Execute(ctx context.Context, cfg *config.Config, period Period, regenerate bool, loadNotes func() ([]*model.Note, error), now time.Time) error {
	root := cfg.BragDir()
	command, prompt := cfg.BragCommandTemplate(), cfg.BragPrompt(period.Kind())
	if regenerate {
		_, err := Regenerate(ctx, root, command, prompt, period)
		return err
	}
	if _, loadErr := Load(root, period); loadErr == nil {
		return fmt.Errorf("%s already has a brag; use --regenerate to rewrite its summary", period.Label())
	}
	if !Braggable(period, now) {
		return fmt.Errorf("%s has not ended yet", period.Label())
	}
	var facts string
	var err error
	switch typed := period.(type) {
	case Week:
		facts, err = weekFacts(ctx, cfg, typed, loadNotes, now)
	case Month:
		facts, err = MonthFacts(root, typed)
	case Year:
		facts, err = YearFacts(root, typed)
	default:
		err = fmt.Errorf("unsupported period %s", period.ID())
	}
	if err != nil {
		return err
	}
	_, err = Create(ctx, root, command, prompt, period, facts)
	return err
}

func weekFacts(ctx context.Context, cfg *config.Config, week Week, loadNotes func() ([]*model.Note, error), now time.Time) (string, error) {
	notes, err := loadNotes()
	if err != nil {
		return "", fmt.Errorf("load notes: %w", err)
	}
	sources, err := Gather(ctx, cfg, week, notes, now)
	if err != nil {
		return "", fmt.Errorf("collect authored PRs: %w", err)
	}
	return BuildFacts(week, sources), nil
}

func RunJob(ctx context.Context, cfg *config.Config, period Period, regenerate bool, loadNotes func() ([]*model.Note, error)) (runErr error) {
	release, err := claimRun(cfg.BragDir(), period.ID())
	if err != nil {
		return err
	}
	defer func() { release(runErr) }()
	return Execute(ctx, cfg, period, regenerate, loadNotes, time.Now())
}

func PeriodFromFlags(week, month, year string) (Period, error) {
	flags := map[config.PromptKind]string{config.PromptWeek: week, config.PromptMonth: month, config.PromptYear: year}
	var chosen config.PromptKind
	for kind, value := range flags {
		if value == "" {
			continue
		}
		if chosen != "" {
			return nil, errors.New("pass only one of --week, --month or --year")
		}
		chosen = kind
	}
	if chosen == "" {
		return nil, errors.New("pass one of --week 2026-W40, --month 2026-10 or --year 2026")
	}
	period, err := ParsePeriod(flags[chosen], time.Local)
	if err != nil {
		return nil, err
	}
	if period.Kind() != chosen {
		return nil, fmt.Errorf("--%s expects a %s id, got %q", chosen, chosen, flags[chosen])
	}
	return period, nil
}
