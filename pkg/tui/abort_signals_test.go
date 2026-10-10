package tui

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type spawnedJob struct {
	pid    int
	done   chan struct{}
	state  **os.ProcessState
	marker string
}

func spawnJobGroup(t *testing.T, ignoreTerm bool) spawnedJob {
	t.Helper()
	dir := t.TempDir()
	marker, ready := filepath.Join(dir, "term"), filepath.Join(dir, "ready")
	script := "echo up > '" + ready + "'; while :; do sleep 0.1; done"
	if ignoreTerm {
		script = "trap \"echo term >> '" + marker + "'\" TERM; " + script
	}
	cmd := exec.Command("sh", "-c", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var state *os.ProcessState
	go func() {
		_ = cmd.Wait()
		state = cmd.ProcessState
		close(done)
	}()
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	})
	deadline := time.Now().Add(5 * time.Second)
	for !fileExists(ready) {
		if time.Now().After(deadline) {
			t.Fatal("child never became ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return spawnedJob{pid: cmd.Process.Pid, done: done, state: &state, marker: marker}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func writeJobFiles(t *testing.T, jobName string, pid int) (string, string) {
	t.Helper()
	pidFile := filepath.Join(getLogsDir(), jobName+".pid")
	logFile := filepath.Join(getLogsDir(), jobName+".log")
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logFile, []byte("working\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return pidFile, logFile
}

func waitForExit(t *testing.T, job spawnedJob) *os.ProcessState {
	t.Helper()
	select {
	case <-job.done:
		return *job.state
	case <-time.After(5 * time.Second):
		t.Fatal("job still running after abort")
		return nil
	}
}

func exitSignal(state *os.ProcessState) syscall.Signal {
	status, ok := state.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() {
		return 0
	}
	return status.Signal()
}

func assertAbortLogged(t *testing.T, logFile, pidFile string) {
	t.Helper()
	data, err := os.ReadFile(logFile)
	if err != nil || !strings.HasSuffix(string(data), "\n[JOB ABORTED BY USER]\n") || !strings.HasPrefix(string(data), "working\n") {
		t.Errorf("log = %q err %v", data, err)
	}
	if fileExists(pidFile) {
		t.Errorf("pid file kept after abort")
	}
}

func TestAbortConfirmSendsTermThenKillsAfterThreeSecondsAndLogs(t *testing.T) {
	m := selectionTestModel(t)
	job := spawnJobGroup(t, true)
	pidFile, logFile := writeJobFiles(t, "janitor", job.pid)
	m.refreshJobStates()
	m = openPreviewOn(t, m, "job:janitor")
	m = press(t, m, runes("d"))
	if m.mode != ViewDeleteConfirm || m.jobToAbort != "janitor" {
		t.Fatalf("d on a running job: mode %v abort %q", m.mode, m.jobToAbort)
	}
	if !strings.Contains(stripANSI(m.View()), "abort running job 'janitor'") {
		t.Errorf("confirm should name the job")
	}
	next, cmd := m.Update(runes("y"))
	m = next.(Model)
	if m.mode != ViewPreview || m.jobToAbort != "" || cmd == nil {
		t.Fatalf("y: mode %v abort %q cmd %v", m.mode, m.jobToAbort, cmd != nil)
	}
	started := time.Now()
	msg := cmd()
	elapsed := time.Since(started)
	aborted, isAbort := msg.(jobAbortedMsg)
	if !isAbort || aborted.jobName != "janitor" || aborted.err != nil {
		t.Fatalf("msg = %#v", msg)
	}
	if elapsed < 3*time.Second || elapsed > 6*time.Second {
		t.Errorf("abort took %v, want SIGKILL after about 3s", elapsed)
	}
	if data, _ := os.ReadFile(job.marker); !strings.Contains(string(data), "term") {
		t.Errorf("SIGTERM was not delivered before SIGKILL")
	}
	if signal := exitSignal(waitForExit(t, job)); signal != syscall.SIGKILL {
		t.Errorf("job ended by %v, want SIGKILL", signal)
	}
	assertAbortLogged(t, logFile, pidFile)
	m = update(m, aborted)
	if m.mode != ViewPreview || isJobRunning("janitor") {
		t.Errorf("after abort: mode %v running %v", m.mode, isJobRunning("janitor"))
	}
}

func TestAbortStopsAtSigtermWhenTheJobExits(t *testing.T) {
	selectionTestModel(t)
	job := spawnJobGroup(t, false)
	pidFile, logFile := writeJobFiles(t, "janitor", job.pid)
	started := time.Now()
	aborted := abortJobCmd("janitor")().(jobAbortedMsg)
	if aborted.err != nil {
		t.Fatalf("err %v", aborted.err)
	}
	if elapsed := time.Since(started); elapsed >= 3*time.Second {
		t.Errorf("a job that exits on SIGTERM should not wait for SIGKILL, took %v", elapsed)
	}
	if signal := exitSignal(waitForExit(t, job)); signal != syscall.SIGTERM {
		t.Errorf("job ended by %v, want SIGTERM", signal)
	}
	assertAbortLogged(t, logFile, pidFile)
}

func TestAbortOfAnAlreadyDeadJobOnlyLogsAndCleansUp(t *testing.T) {
	selectionTestModel(t)
	finished := exec.Command("true")
	if err := finished.Run(); err != nil {
		t.Fatal(err)
	}
	pidFile, logFile := writeJobFiles(t, "janitor", finished.Process.Pid)
	started := time.Now()
	aborted := abortJobCmd("janitor")().(jobAbortedMsg)
	if aborted.err != nil || time.Since(started) > time.Second {
		t.Errorf("err %v took %v", aborted.err, time.Since(started))
	}
	assertAbortLogged(t, logFile, pidFile)
}

func TestAbortWithoutAPidFileShowsAJobError(t *testing.T) {
	m := selectionTestModel(t)
	aborted := abortJobCmd("janitor")().(jobAbortedMsg)
	if aborted.err == nil || !strings.Contains(aborted.err.Error(), "failed to read pid file") {
		t.Fatalf("err %v", aborted.err)
	}
	m = update(m, aborted)
	if m.mode != ViewDashboard || !strings.Contains(latestMessageText(m), "failed to read pid file") || !strings.Contains(headerTopRow(m), "failed to read pid file") {
		t.Errorf("mode %v message %q header %q", m.mode, latestMessageText(m), headerTopRow(m))
	}
	if fileExists(filepath.Join(getLogsDir(), "janitor.log")) {
		t.Errorf("abort without a pid should not create a log")
	}
}

func TestAbortWithAGarbledPidFileReportsIt(t *testing.T) {
	selectionTestModel(t)
	if err := os.WriteFile(filepath.Join(getLogsDir(), "janitor.pid"), []byte("not-a-pid"), 0644); err != nil {
		t.Fatal(err)
	}
	aborted := abortJobCmd("janitor")().(jobAbortedMsg)
	if aborted.err == nil || !strings.Contains(aborted.err.Error(), "invalid pid file") {
		t.Errorf("err %v", aborted.err)
	}
	var numError *strconv.NumError
	if !errors.As(aborted.err, &numError) {
		t.Errorf("err should wrap the parse error: %v", aborted.err)
	}
}

func TestAbortConfirmNOrEscKeepsTheJobRunning(t *testing.T) {
	for name, key := range map[string]tea.KeyMsg{"n": runes("n"), "esc": {Type: tea.KeyEsc}} {
		m := selectionTestModel(t)
		job := spawnJobGroup(t, false)
		writeJobFiles(t, "janitor", job.pid)
		m.refreshJobStates()
		m = openPreviewOn(t, m, "job:janitor")
		m = press(t, m, runes("d"))
		next, cmd := m.Update(key)
		m = next.(Model)
		if cmd != nil || m.jobToAbort != "" || m.mode != ViewPreview || !isJobRunning("janitor") {
			t.Errorf("%s: mode %v abort %q running %v", name, m.mode, m.jobToAbort, isJobRunning("janitor"))
		}
	}
}
