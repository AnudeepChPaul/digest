package tui

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/config"
	"github.com/AnudeepChPaul/digest/pkg/paths"
	"github.com/AnudeepChPaul/digest/pkg/review"

	tea "github.com/charmbracelet/bubbletea"
)

var digestRoot = (*config.Config)(nil).Root()

var createdLogsDirs sync.Map

func getLogsDir() string {
	dir := filepath.Join(digestRoot, "logs")
	if _, created := createdLogsDirs.Load(dir); !created {
		if os.MkdirAll(dir, paths.PrivateDirMode) == nil {
			createdLogsDirs.Store(dir, true)
		}
	}
	return dir
}

func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", "/c", fmt.Sprintf("tasklist /FI \"PID eq %d\"", pid))
		out, err := cmd.Output()
		if err != nil {
			return false
		}
		return strings.Contains(string(out), strconv.Itoa(pid))
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func runningJobPID(jobName string) (int, bool) {
	data, err := os.ReadFile(filepath.Join(getLogsDir(), fmt.Sprintf("%s.pid", jobName)))
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || !isProcessAlive(pid) {
		return 0, false
	}
	return pid, true
}

func isJobRunning(jobName string) bool {
	pidFile := filepath.Join(getLogsDir(), fmt.Sprintf("%s.pid", jobName))
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return false
	}
	return isProcessAlive(pid)
}

func dryRunFilePath(jobName string, suffix string) string {
	return filepath.Join(getLogsDir(), fmt.Sprintf("%s.dryrun.%s", jobName, suffix))
}

func isDryRunInFlight(jobName string) bool {
	pidFile := dryRunFilePath(jobName, "pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err == nil && isProcessAlive(pid) {
		return true
	}
	_ = os.Remove(pidFile)
	return false
}

func loadDryRunResult(jobName string) (string, int, bool) {
	exitData, err := os.ReadFile(dryRunFilePath(jobName, "exit"))
	if err != nil {
		return "", 0, false
	}
	exitCode, err := strconv.Atoi(strings.TrimSpace(string(exitData)))
	if err != nil {
		exitCode = 1
	}
	output, _ := readDryRunLog(dryRunFilePath(jobName, "log"))
	return string(output), exitCode, true
}

var readDryRunLog = func(path string) ([]byte, error) {
	return []byte(readFileTail(path, jobLogTailBytes)), nil
}

func dryRunStamp(jobName string) string {
	var stamp strings.Builder
	for _, suffix := range []string{"exit", "log"} {
		if info, err := os.Stat(dryRunFilePath(jobName, suffix)); err == nil {
			fmt.Fprintf(&stamp, "%s:%d:%d|", suffix, info.Size(), info.ModTime().UnixNano())
		}
	}
	return stamp.String()
}

func writeDryRunFailure(jobName string, failure error) {
	_ = os.WriteFile(dryRunFilePath(jobName, "log"), []byte(failure.Error()+"\n"), paths.PrivateFileMode)
	_ = os.WriteFile(dryRunFilePath(jobName, "exit"), []byte("1"), paths.PrivateFileMode)
}

var startDryRunBackground = func(spec config.JobSpec) error {
	cmdStr, err := resolveJobCommand(spec, true)
	if err != nil {
		writeDryRunFailure(spec.Name, err)
		return err
	}

	logPath := dryRunFilePath(spec.Name, "log")
	pidPath := dryRunFilePath(spec.Name, "pid")
	exitPath := dryRunFilePath(spec.Name, "exit")
	_ = os.Remove(exitPath)

	f, err := paths.CreatePrivate(logPath)
	if err != nil {
		writeDryRunFailure(spec.Name, err)
		return err
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", cmdStr)
	} else {
		cmd = exec.Command("sh", "-c", `sh -c "$1"; echo $? > "$2"`, "digest-dry-run", cmdStr, exitPath)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setsid: true,
		}
	}
	cmd.Env = os.Environ()
	if home, err := os.UserHomeDir(); err == nil {
		cmd.Dir = home
	}
	cmd.Stdout = f
	cmd.Stderr = f

	if err := cmd.Start(); err != nil {
		f.Close()
		writeDryRunFailure(spec.Name, err)
		return err
	}

	ownPid := strconv.Itoa(cmd.Process.Pid)
	pidWriteErr := os.WriteFile(pidPath, []byte(ownPid), paths.PrivateFileMode)

	go func() {
		waitErr := cmd.Wait()
		_ = f.Close()
		if runtime.GOOS == "windows" {
			exitCode := 0
			if exitErr, ok := waitErr.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else if waitErr != nil {
				exitCode = 1
			}
			_ = os.WriteFile(exitPath, []byte(strconv.Itoa(exitCode)), paths.PrivateFileMode)
		}
		if data, err := os.ReadFile(pidPath); err == nil && strings.TrimSpace(string(data)) == ownPid {
			_ = os.Remove(pidPath)
		}
	}()

	if pidWriteErr != nil {
		return fmt.Errorf("dry run for %q started but its pid file could not be written: %w", spec.Name, pidWriteErr)
	}
	return nil
}

func (m Model) jobRunning(jobName string) bool {
	return m.runningJobPIDs[jobName] > 0
}

func (m Model) isAnyDryRunInFlight() bool {
	return len(m.dryRunsInFlight) > 0
}

type previewedJob struct {
	name                   string
	inFlight, hasRun       bool
	running                bool
	exitCode, outputLength int
}

func (m Model) previewedJobState() previewedJob {
	item, found := m.selectedNavItem()
	if !found || item.Kind != KindJobDraft || item.Draft == nil {
		return previewedJob{}
	}
	name := item.Draft.Name
	_, running := m.runningJobPIDs[name]
	return previewedJob{name: name, inFlight: m.dryRunsInFlight[name], hasRun: m.jobDryRunHasRun[name], running: running, exitCode: m.jobDryRunExitCodes[name], outputLength: len(m.jobDryRunOutputs[name])}
}

func (m Model) isAnyJobRunning() bool {
	return len(m.runningJobPIDs) > 0
}

var builtinJobNames = map[string]bool{
	"repo-sync":     true,
	"janitor":       true,
	"branch-reaper": true,
}

var errNoJobCommand = errors.New("no command configured")

var errDryRunFlagMissing = errors.New("dry-run command does not pass --dry-run")

func shellQuote(text string) string {
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}

func digestExecutable() string {
	if execPath, err := os.Executable(); err == nil {
		return shellQuote(execPath)
	}
	return "digest"
}

func jobHasDryRun(name, dryRunCommand string) bool {
	return dryRunCommand != "" || builtinJobNames[name]
}

func withRunningDigest(command string, dryRun bool) (string, error) {
	words := strings.Fields(command)
	if len(words) < 2 || words[0] != "digest" || !builtinJobNames[words[1]] {
		return command, nil
	}
	if dryRun && !slices.ContainsFunc(words[2:], func(word string) bool { return word == "--dry-run" || word == "-dry-run" }) {
		return "", errDryRunFlagMissing
	}
	return digestExecutable() + strings.TrimPrefix(strings.TrimSpace(command), "digest"), nil
}

func resolveJobCommand(spec config.JobSpec, dryRun bool) (string, error) {
	configured := spec.Command
	if dryRun {
		configured = spec.DryRunCommand
	}
	if configured != "" {
		command, err := withRunningDigest(configured, dryRun)
		if err != nil {
			return "", fmt.Errorf("job %q: %w", spec.Name, err)
		}
		return command, nil
	}
	if !builtinJobNames[spec.Name] {
		return "", fmt.Errorf("job %q: %w", spec.Name, errNoJobCommand)
	}
	commandStr := fmt.Sprintf("%s %s", digestExecutable(), spec.Name)
	if dryRun {
		commandStr += " --dry-run"
	}
	return commandStr, nil
}

func findJobSpec(cfg *config.Config, jobName string) config.JobSpec {
	if cfg != nil {
		for _, j := range cfg.JobList() {
			if j.Name == jobName {
				return j
			}
		}
	}
	return config.JobSpec{Name: jobName}
}

const jobLogArchiveFormat = "20060102-150405"

func pruneJobLogArchives(logsDir, jobName string, retentionDays int, now time.Time) {
	archives, _ := filepath.Glob(filepath.Join(logsDir, jobName+"-*.log"))
	cutoff := now.AddDate(0, 0, -retentionDays)
	for _, archive := range archives {
		stamp := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(archive), jobName+"-"), ".log")
		if _, err := time.Parse(jobLogArchiveFormat, stamp); err != nil {
			continue
		}
		if info, err := os.Stat(archive); err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(archive)
		}
	}
}

var executeJobBackground = func(cfg *config.Config, jobName string) error {
	commandStr, err := resolveJobCommand(findJobSpec(cfg, jobName), false)
	if err != nil {
		return err
	}

	logsDir := getLogsDir()
	activeLog := filepath.Join(logsDir, fmt.Sprintf("%s.log", jobName))
	pidFile := filepath.Join(logsDir, fmt.Sprintf("%s.pid", jobName))

	if _, err := os.Stat(activeLog); err == nil {
		timestamp := time.Now().Format(jobLogArchiveFormat)
		archivedLog := filepath.Join(logsDir, fmt.Sprintf("%s-%s.log", jobName, timestamp))
		_ = os.Rename(activeLog, archivedLog)
	}
	pruneJobLogArchives(logsDir, jobName, cfg.Retention(), time.Now())

	f, err := paths.CreatePrivate(activeLog)
	if err != nil {
		return err
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", commandStr)
	} else {
		cmd = exec.Command("sh", "-c", commandStr)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setsid: true,
		}
	}

	cmd.Env = os.Environ()

	if home, err := os.UserHomeDir(); err == nil {
		cmd.Dir = home
	}
	if cfg != nil {
		cmd.Dir = cfg.NotesDir()
	}

	cmd.Stdout = f
	cmd.Stderr = f

	if err := cmd.Start(); err != nil {
		f.Close()
		return err
	}

	ownPid := strconv.Itoa(cmd.Process.Pid)
	pidWriteErr := os.WriteFile(pidFile, []byte(ownPid), paths.PrivateFileMode)

	go func() {
		_ = cmd.Wait()
		_ = f.Close()
		if data, err := os.ReadFile(pidFile); err == nil && strings.TrimSpace(string(data)) == ownPid {
			_ = os.Remove(pidFile)
		}
	}()

	if pidWriteErr != nil {
		return fmt.Errorf("job %q started but its pid file could not be written (abort unavailable): %w", jobName, pidWriteErr)
	}
	return nil
}

func signalJobGroup(pid int, sig syscall.Signal) error {
	if runtime.GOOS == "windows" {
		return exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid)).Run()
	}
	return syscall.Kill(-pid, sig)
}

func abortJobCmd(jobName string) tea.Cmd {
	return func() tea.Msg {
		pidFile := filepath.Join(getLogsDir(), fmt.Sprintf("%s.pid", jobName))
		data, err := os.ReadFile(pidFile)
		if err != nil {
			return jobAbortedMsg{jobName: jobName, err: fmt.Errorf("failed to read pid file for %q: %w", jobName, err)}
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			return jobAbortedMsg{jobName: jobName, err: fmt.Errorf("invalid pid file for %q: %w", jobName, err)}
		}

		var abortErr error
		if isProcessAlive(pid) {
			if err := signalJobGroup(pid, syscall.SIGTERM); err != nil {
				abortErr = fmt.Errorf("failed to terminate job %q: %w", jobName, err)
			}
			deadline := time.Now().Add(3 * time.Second)
			for isProcessAlive(pid) && time.Now().Before(deadline) {
				time.Sleep(100 * time.Millisecond)
			}
			if isProcessAlive(pid) {
				if err := signalJobGroup(pid, syscall.SIGKILL); err != nil {
					abortErr = fmt.Errorf("failed to kill job %q: %w", jobName, err)
				}
			}
		}

		activeLog := filepath.Join(getLogsDir(), fmt.Sprintf("%s.log", jobName))
		if f, err := os.OpenFile(activeLog, os.O_WRONLY|os.O_APPEND, paths.PrivateFileMode); err == nil {
			_, _ = f.WriteString("\n[JOB ABORTED BY USER]\n")
			f.Close()
		}

		if current, err := os.ReadFile(pidFile); err == nil && strings.TrimSpace(string(current)) == strconv.Itoa(pid) {
			_ = os.Remove(pidFile)
		}
		return jobAbortedMsg{jobName: jobName, err: abortErr}
	}
}

const (
	jobLogTailBytes = 64 * 1024
	logPeekBytes    = 16 * 1024
)

func readFileTail(path string, maxBytes int64) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() == 0 {
		return ""
	}
	offset := max(info.Size()-maxBytes, 0)
	data := make([]byte, info.Size()-offset)
	if _, err := file.ReadAt(data, offset); err != nil && !errors.Is(err, io.EOF) {
		return ""
	}
	if offset > 0 {
		if newline := bytes.IndexByte(data, '\n'); newline >= 0 {
			data = data[newline+1:]
		}
	}
	return review.PlainText(string(data))
}

func readJobLog(jobName string) string {
	return readFileTail(filepath.Join(getLogsDir(), fmt.Sprintf("%s.log", jobName)), jobLogTailBytes)
}

func jobLogModTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

func latestJobOutput(jobName string, dryRunOutput string) (string, bool) {
	realLog := readJobLog(jobName)
	if isJobRunning(jobName) || strings.TrimSpace(dryRunOutput) == "" {
		return realLog, false
	}
	if realLog == "" {
		return dryRunOutput, true
	}
	realModTime := jobLogModTime(filepath.Join(getLogsDir(), fmt.Sprintf("%s.log", jobName)))
	dryRunModTime := jobLogModTime(dryRunFilePath(jobName, "log"))
	if dryRunModTime.After(realModTime) {
		return dryRunOutput, true
	}
	return realLog, false
}

func (m Model) jobDryRunOutputFor(jobName string) string {
	if m.jobDryRunHasRun == nil || !m.jobDryRunHasRun[jobName] {
		return ""
	}
	return m.jobDryRunOutputs[jobName]
}

func tickCtrlCResetCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return ctrlCResetMsg{}
	})
}

func jobLogStampFor(jobName string, running bool) string {
	stamp := strconv.FormatBool(running)
	for _, path := range []string{filepath.Join(getLogsDir(), jobName+".log"), dryRunFilePath(jobName, "log")} {
		if info, err := os.Stat(path); err == nil {
			stamp += fmt.Sprintf("|%d:%d", info.Size(), info.ModTime().UnixNano())
		}
	}
	return stamp
}

func (m Model) previewedRunningJob() string {
	if m.mode != ViewPreview {
		return ""
	}
	navItems := m.allNavItems()
	if m.selected >= len(navItems) {
		return ""
	}
	if item := navItems[m.selected]; item.Kind == KindJobDraft && item.Draft != nil {
		return item.Draft.Name
	}
	return ""
}

func (m *Model) startJobDryRunCmd(jobName string) tea.Cmd {
	if m.cfg == nil || m.dryRunsInFlight[jobName] {
		return nil
	}
	err := startDryRunBackground(findJobSpec(m.cfg, jobName))
	m.refreshDryRunResults()
	if m.mode == ViewPreview {
		m.updatePreviewViewport()
	}
	if err != nil {
		m.showError("JOB ERROR", err)
		return nil
	}
	return tea.Batch(m.ensureRunStatePoll(), m.ensureSyncPulse())
}
