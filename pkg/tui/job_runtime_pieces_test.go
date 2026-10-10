package tui

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/config"
	"github.com/achandrapaul/digest/pkg/model"
)

var realExecuteJobBackground = executeJobBackground

func tempDigestRoot(t *testing.T) string {
	t.Helper()
	previousRoot := digestRoot
	digestRoot = t.TempDir()
	t.Cleanup(func() {
		createdLogsDirs.Delete(filepath.Join(digestRoot, "logs"))
		digestRoot = previousRoot
	})
	return getLogsDir()
}

func writeLogsFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(getLogsDir(), name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func blockWithADirectory(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
}

func readOnlyLogsDir(t *testing.T, logsDir string) {
	t.Helper()
	if err := os.Chmod(logsDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(logsDir, 0o700) })
}

func waitUntil(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition never became true")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestPIDFilesWithBadOrDeadPIDsAreNotRunning(t *testing.T) {
	tempDigestRoot(t)
	if isProcessAlive(0) || isProcessAlive(-4) {
		t.Fatal("non-positive pids are never alive")
	}
	writeLogsFile(t, "garbled.pid", "not a pid")
	if isJobRunning("garbled") {
		t.Fatal("a garbled pid file is not running")
	}
	writeLogsFile(t, "dead.pid", "999999")
	if _, running := runningJobPID("dead"); running {
		t.Fatal("a dead pid is not running")
	}
}

func TestStaleDryRunPIDFilesAreRemoved(t *testing.T) {
	tempDigestRoot(t)
	pidPath := writeLogsFile(t, "nightly.dryrun.pid", "999999")
	if isDryRunInFlight("nightly") {
		t.Fatal("a dead dry run is not in flight")
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Fatalf("the stale pid file should be removed, stat err = %v", err)
	}
}

func TestUnreadableDryRunExitFileCountsAsAFailure(t *testing.T) {
	tempDigestRoot(t)
	writeLogsFile(t, "nightly.dryrun.exit", "???")
	writeLogsFile(t, "nightly.dryrun.log", "partial output\n")
	output, exitCode, finished := loadDryRunResult("nightly")
	if !finished || exitCode != 1 || !strings.Contains(output, "partial output") {
		t.Fatalf("output %q exit %d finished %v", output, exitCode, finished)
	}
}

func TestDryRunWithoutTheDryRunFlagRecordsTheFailure(t *testing.T) {
	tempDigestRoot(t)
	err := startDryRunBackground(config.JobSpec{Name: "janitor", DryRunCommand: "digest janitor"})
	if !errors.Is(err, errDryRunFlagMissing) {
		t.Fatalf("err = %v", err)
	}
	output, exitCode, finished := loadDryRunResult("janitor")
	if !finished || exitCode != 1 || !strings.Contains(output, "dry-run command does not pass --dry-run") {
		t.Fatalf("output %q exit %d finished %v", output, exitCode, finished)
	}
}

func TestDryRunFailsWhenTheLogCannotOpen(t *testing.T) {
	logsDir := tempDigestRoot(t)
	readOnlyLogsDir(t, logsDir)
	if err := startDryRunBackground(config.JobSpec{Name: "nightly", DryRunCommand: "true"}); err == nil {
		t.Fatal("an unwritable logs dir should fail")
	}
}

func TestDryRunFailsWhenTheShellIsMissing(t *testing.T) {
	tempDigestRoot(t)
	t.Setenv("PATH", t.TempDir())
	if err := startDryRunBackground(config.JobSpec{Name: "nightly", DryRunCommand: "true"}); err == nil {
		t.Fatal("a missing shell should fail")
	}
	if _, exitCode, finished := loadDryRunResult("nightly"); !finished || exitCode != 1 {
		t.Fatal("the start failure should be recorded")
	}
}

func TestDryRunReportsAnUnwritablePIDFile(t *testing.T) {
	tempDigestRoot(t)
	blockWithADirectory(t, dryRunFilePath("nightly", "pid"))
	err := startDryRunBackground(config.JobSpec{Name: "nightly", DryRunCommand: "true"})
	if err == nil || !strings.Contains(err.Error(), "pid file could not be written") {
		t.Fatalf("err = %v", err)
	}
	waitUntil(t, func() bool { _, _, finished := loadDryRunResult("nightly"); return finished })
}

func TestCommandExitCodes(t *testing.T) {
	if commandExitCode(nil, nil) != 0 || commandExitCode(nil, errors.New("wait failed")) != 1 {
		t.Fatal("a missing state uses the wait error")
	}
	exited := exec.Command("sh", "-c", "exit 3")
	_ = exited.Run()
	if code := commandExitCode(exited.ProcessState, nil); code != 3 {
		t.Fatalf("exit code = %d", code)
	}
	signaled := exec.Command("sh", "-c", "kill -TERM $$")
	_ = signaled.Run()
	if code := commandExitCode(signaled.ProcessState, nil); code != 143 {
		t.Fatalf("signal exit code = %d", code)
	}
}

func TestBuiltinJobsDefaultToTheRunningBinary(t *testing.T) {
	executable := digestExecutable()
	if command, err := resolveJobCommand(config.JobSpec{Name: "janitor"}, false); err != nil || command != executable+" janitor" {
		t.Fatalf("command %q err %v", command, err)
	}
	if command, err := resolveJobCommand(config.JobSpec{Name: "janitor"}, true); err != nil || command != executable+" janitor --dry-run" {
		t.Fatalf("dry command %q err %v", command, err)
	}
	if _, err := resolveJobCommand(config.JobSpec{Name: "custom"}, false); !errors.Is(err, errNoJobCommand) {
		t.Fatalf("err = %v", err)
	}
	if spec := findJobSpec(&config.Config{}, "unknown"); spec.Name != "unknown" || spec.Command != "" {
		t.Fatalf("spec = %+v", spec)
	}
}

func jobConfig(t *testing.T, command string) *config.Config {
	t.Helper()
	cfg := &config.Config{DigestRoot: t.TempDir(), Jobs: []config.JobSpec{{Name: "nightly", Command: command}}}
	if err := os.MkdirAll(cfg.NotesDir(), 0755); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestJobRunWithoutACommandFails(t *testing.T) {
	tempDigestRoot(t)
	if err := realExecuteJobBackground(&config.Config{DigestRoot: t.TempDir()}, "custom"); !errors.Is(err, errNoJobCommand) {
		t.Fatalf("err = %v", err)
	}
}

func TestJobRunRotatesThePreviousLogAndCleansItsPIDFile(t *testing.T) {
	logsDir := tempDigestRoot(t)
	writeLogsFile(t, "nightly.log", "previous run\n")
	cfg := jobConfig(t, "echo fresh run")
	if err := realExecuteJobBackground(cfg, "nightly"); err != nil {
		t.Fatal(err)
	}
	archives, _ := filepath.Glob(filepath.Join(logsDir, "nightly-*.log"))
	if len(archives) != 1 {
		t.Fatalf("archives = %v", archives)
	}
	if archived, _ := os.ReadFile(archives[0]); string(archived) != "previous run\n" {
		t.Fatalf("archived = %q", archived)
	}
	waitUntil(t, func() bool { return !fileExists(filepath.Join(logsDir, "nightly.pid")) })
	waitUntil(t, func() bool { return strings.Contains(readJobLog("nightly"), "fresh run") })
}

func TestJobRunFailsWhenTheLogCannotOpen(t *testing.T) {
	logsDir := tempDigestRoot(t)
	readOnlyLogsDir(t, logsDir)
	if err := realExecuteJobBackground(jobConfig(t, "true"), "nightly"); err == nil {
		t.Fatal("an unwritable logs dir should fail")
	}
}

func TestJobRunFailsWhenTheShellIsMissing(t *testing.T) {
	tempDigestRoot(t)
	t.Setenv("PATH", t.TempDir())
	if err := realExecuteJobBackground(jobConfig(t, "true"), "nightly"); err == nil {
		t.Fatal("a missing shell should fail")
	}
}

func TestJobRunReportsAnUnwritablePIDFile(t *testing.T) {
	logsDir := tempDigestRoot(t)
	blockWithADirectory(t, filepath.Join(logsDir, "nightly.pid"))
	err := realExecuteJobBackground(jobConfig(t, "true"), "nightly")
	if err == nil || !strings.Contains(err.Error(), "abort unavailable") {
		t.Fatalf("err = %v", err)
	}
}

func TestAbortReportsSignalFailuresForANonGroupProcess(t *testing.T) {
	tempDigestRoot(t)
	sleeper := exec.Command("sleep", "30")
	if err := sleeper.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = sleeper.Process.Kill()
		_ = sleeper.Wait()
	})
	writeLogsFile(t, "lonely.pid", strconv.Itoa(sleeper.Process.Pid))
	aborted, ok := abortJobCmd("lonely")().(jobAbortedMsg)
	if !ok || aborted.err == nil || !strings.Contains(aborted.err.Error(), `failed to kill job "lonely"`) {
		t.Fatalf("msg = %#v", aborted)
	}
}

func TestReadFileTailOfADirectoryIsEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "entry"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := readFileTail(dir, 1024); got != "" {
		t.Fatalf("tail = %q", got)
	}
}

func TestLatestJobOutputPicksTheNewerLog(t *testing.T) {
	tempDigestRoot(t)
	if output, isDryRun := latestJobOutput("nightly", "dry output"); output != "dry output" || !isDryRun {
		t.Fatalf("without a real log: %q %v", output, isDryRun)
	}
	realLog := writeLogsFile(t, "nightly.log", "real output\n")
	dryLog := writeLogsFile(t, "nightly.dryrun.log", "dry output\n")
	older, newer := time.Now().Add(-time.Hour), time.Now()
	if err := os.Chtimes(realLog, older, older); err != nil {
		t.Fatal(err)
	}
	if output, isDryRun := latestJobOutput("nightly", "dry output"); output != "dry output" || !isDryRun {
		t.Fatalf("newer dry run: %q %v", output, isDryRun)
	}
	if err := os.Chtimes(dryLog, older.Add(-time.Hour), older.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(realLog, newer, newer); err != nil {
		t.Fatal(err)
	}
	if output, isDryRun := latestJobOutput("nightly", "dry output"); !strings.Contains(output, "real output") || isDryRun {
		t.Fatalf("newer real run: %q %v", output, isDryRun)
	}
	if !jobLogModTime(filepath.Join(t.TempDir(), "missing")).IsZero() {
		t.Fatal("a missing log has no mod time")
	}
}

func TestDryRunOutputIsShownOnlyAfterARun(t *testing.T) {
	m := syncTestModel(t)
	m.jobDryRunHasRun = map[string]bool{"nightly": true}
	m.jobDryRunOutputs = map[string]string{"nightly": "checked"}
	if m.jobDryRunOutputFor("nightly") != "checked" {
		t.Fatal("a finished dry run shows its output")
	}
}

func TestPreviewedRunningJobOnlyForJobPreviews(t *testing.T) {
	tempDigestRoot(t)
	m, _, _ := jobTestModel(t)
	m.notes = []*model.Note{{ID: "n", Summary: "note", Source: model.SourceManual, Status: model.StatusActive, Created: m.currentDate, Updated: m.currentDate}}
	m.contentVersion++
	selectNavItem(t, &m, "job:nightly")
	if m.previewedRunningJob() != "" {
		t.Fatal("the dashboard previews no job")
	}
	m.mode = ViewPreview
	if m.previewedRunningJob() != "nightly" {
		t.Fatalf("job = %q", m.previewedRunningJob())
	}
	m.selected = 999
	if m.previewedRunningJob() != "" {
		t.Fatal("an out of range selection previews no job")
	}
	selectNavKind(t, &m, KindTodayNote)
	if m.previewedRunningJob() != "" {
		t.Fatal("a non-job row previews no job")
	}
}

func TestDryRunStartFailureShowsAJobError(t *testing.T) {
	tempDigestRoot(t)
	m, _, _ := jobTestModel(t)
	startDryRunBackground = func(config.JobSpec) error { return errors.New("no shell") }
	if cmd := m.startJobDryRunCmd("nightly"); cmd != nil || latestMessageText(m) != "no shell" {
		t.Fatalf("message %q", latestMessageText(m))
	}
}
