package review

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/charmbracelet/log"
)

func stubParentPID(t *testing.T, pid int) {
	t.Helper()
	original := parentPID
	parentPID = func() int { return pid }
	t.Cleanup(func() { parentPID = original })
}

func readPID(t *testing.T, dir string) int {
	t.Helper()
	pid, ok := readInt(filepath.Join(dir, pidFile))
	if !ok {
		t.Fatalf("no pid file in %s", dir)
	}
	return pid
}

func TestClaimRunDirectWritesOwnPIDAndExit(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, exitFile), "1")
	writeFile(t, filepath.Join(dir, FindingsFile), "{}")
	stubParentPID(t, 1)
	release, err := claimRun(dir)
	if err != nil {
		t.Fatal(err)
	}
	if readPID(t, dir) != os.Getpid() || Status(dir) != RunRunning {
		t.Fatalf("pid=%d status=%v", readPID(t, dir), Status(dir))
	}
	if _, err := os.Stat(filepath.Join(dir, exitFile)); err == nil {
		t.Errorf("stale exit file kept")
	}
	if _, err := os.Stat(filepath.Join(dir, FindingsFile)); err == nil {
		t.Errorf("stale findings kept")
	}
	release(nil)
	if code, ok := readInt(filepath.Join(dir, exitFile)); !ok || code != 0 {
		t.Errorf("exit = %d, %v", code, ok)
	}
}

func TestClaimRunDirectFailureWritesExitOne(t *testing.T) {
	dir := t.TempDir()
	stubParentPID(t, 1)
	release, err := claimRun(dir)
	if err != nil {
		t.Fatal(err)
	}
	release(errors.New("clone failed"))
	if code, _ := readInt(filepath.Join(dir, exitFile)); code != 1 {
		t.Errorf("exit = %d", code)
	}
}

func TestClaimRunUnderWrapperReplacesPIDOnly(t *testing.T) {
	dir := t.TempDir()
	wrapper := exec.Command("sleep", "30")
	if err := wrapper.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = wrapper.Process.Kill(); _ = wrapper.Wait() }()
	writeFile(t, filepath.Join(dir, pidFile), strconv.Itoa(wrapper.Process.Pid))
	stubParentPID(t, wrapper.Process.Pid)
	release, err := claimRun(dir)
	if err != nil {
		t.Fatal(err)
	}
	if readPID(t, dir) != os.Getpid() {
		t.Errorf("pid = %d, want own pid", readPID(t, dir))
	}
	release(errors.New("x"))
	if _, err := os.Stat(filepath.Join(dir, exitFile)); err == nil {
		t.Errorf("exit written under wrapper; wrapper owns it")
	}
}

func TestClaimRunRefusesOtherLiveRun(t *testing.T) {
	dir := t.TempDir()
	other := exec.Command("sleep", "30")
	if err := other.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Process.Kill(); _ = other.Wait() }()
	writeFile(t, filepath.Join(dir, pidFile), strconv.Itoa(other.Process.Pid))
	stubParentPID(t, 1)
	if _, err := claimRun(dir); !errors.Is(err, ErrReviewRunning) {
		t.Fatalf("err = %v", err)
	}
	if readPID(t, dir) != other.Process.Pid {
		t.Errorf("pid file changed")
	}
}

func TestRunRefusedWhileAnotherRuns(t *testing.T) {
	root := t.TempDir()
	ref := PRRef{Repo: "console", Number: 8, URL: "https://github.com/o/console/pull/8"}
	other := exec.Command("sleep", "30")
	if err := other.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Process.Kill(); _ = other.Wait() }()
	writeFile(t, filepath.Join(StateDir(root, ref), pidFile), strconv.Itoa(other.Process.Pid))
	stubParentPID(t, 1)
	originalPrepare := prepareClone
	cloned := false
	prepareClone = func(ctx context.Context, ref PRRef, root string, logger *log.Logger, output io.Writer) (string, error) {
		cloned = true
		return "", nil
	}
	defer func() { prepareClone = originalPrepare }()
	if err := run(context.Background(), ref, root, "true", log.New(os.Stderr), os.Stdout, os.Stderr); !errors.Is(err, ErrReviewRunning) || cloned {
		t.Errorf("err=%v cloned=%v", err, cloned)
	}
}

func TestStopKillsProcessGroupAndClearsPID(t *testing.T) {
	root := t.TempDir()
	ref := PRRef{Repo: "console", Number: 9, URL: "https://github.com/o/console/pull/9"}
	leader := exec.Command("sh", "-c", "sleep 30 & wait")
	leader.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := leader.Start(); err != nil {
		t.Fatal(err)
	}
	dir := StateDir(root, ref)
	writeFile(t, filepath.Join(dir, pidFile), strconv.Itoa(leader.Process.Pid))
	if Status(dir) != RunRunning {
		t.Fatalf("expected running")
	}
	if err := Stop(root, ref); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = leader.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = leader.Process.Kill()
		t.Fatal("process group still alive after Stop")
	}
	if Status(dir) != RunFailed {
		t.Errorf("status = %v, want failed", Status(dir))
	}
	if _, ok := RunningPID(dir); ok {
		t.Errorf("RunningPID still reports a pid")
	}
}

func TestRunningPID(t *testing.T) {
	dir := t.TempDir()
	if _, ok := RunningPID(dir); ok {
		t.Errorf("pid reported without pid file")
	}
	writeFile(t, filepath.Join(dir, pidFile), strconv.Itoa(os.Getpid()))
	if pid, ok := RunningPID(dir); !ok || pid != os.Getpid() {
		t.Errorf("pid=%d ok=%v", pid, ok)
	}
}

func TestFinishedRunRemovesItsPIDFile(t *testing.T) {
	dir := t.TempDir()
	stubParentPID(t, 1)
	release, err := claimRun(dir)
	if err != nil {
		t.Fatal(err)
	}
	release(nil)
	if _, err := os.Stat(filepath.Join(dir, pidFile)); err == nil {
		t.Error("a finished run should remove its pid file")
	}
}

func TestStatusClearsAPIDFileOfAnExitedProcess(t *testing.T) {
	dir := t.TempDir()
	exited := exec.Command("true")
	if err := exited.Run(); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, pidFile), strconv.Itoa(exited.Process.Pid))
	if Status(dir) == RunRunning {
		t.Fatal("an exited process is not running")
	}
	if _, err := os.Stat(filepath.Join(dir, pidFile)); err == nil {
		t.Error("the pid file of an exited process should be removed")
	}
}

func TestBackgroundChildClaimsThePIDItsLauncherWrote(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, pidFile)
	writeFile(t, pidPath, strconv.Itoa(os.Getpid()))
	writeFile(t, filepath.Join(dir, FindingsFile), "{}")
	stubParentPID(t, 1)
	release, err := claimRun(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, FindingsFile)); err == nil {
		t.Errorf("stale findings kept")
	}
	release(nil)
	if _, err := os.Stat(pidPath); err == nil {
		t.Error("the child should remove its pid file when the review ends")
	}
	if code, ok := readInt(filepath.Join(dir, exitFile)); !ok || code != 0 {
		t.Errorf("exit = %d, %v", code, ok)
	}
}

func TestStoppedRunLeavesNoExitCode(t *testing.T) {
	root := t.TempDir()
	ref := PRRef{Repo: "console", Number: 11, URL: "https://github.com/o/console/pull/11"}
	stubParentPID(t, 1)
	originalPrepare := prepareClone
	prepareClone = func(ctx context.Context, ref PRRef, root string, logger *log.Logger, output io.Writer) (string, error) {
		return root, nil
	}
	defer func() { prepareClone = originalPrepare }()
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(200*time.Millisecond, cancel)
	if err := run(ctx, ref, root, "sleep 30", log.New(io.Discard), io.Discard, io.Discard); err == nil {
		t.Fatal("expected the stopped review to fail")
	}
	dir := StateDir(root, ref)
	if _, err := os.Stat(filepath.Join(dir, exitFile)); err == nil {
		t.Error("a stopped review should not write an exit code")
	}
	if _, err := os.Stat(filepath.Join(dir, pidFile)); err == nil {
		t.Error("a stopped review should remove its pid file")
	}
	if Status(dir) != RunIdle {
		t.Errorf("status = %v, want idle", Status(dir))
	}
}

func TestStopKillsAGroupThatIgnoresTerm(t *testing.T) {
	original := stopGracePeriod
	stopGracePeriod = 200 * time.Millisecond
	t.Cleanup(func() { stopGracePeriod = original })
	root := t.TempDir()
	ref := PRRef{Repo: "console", Number: 10, URL: "https://github.com/o/console/pull/10"}
	stubborn := exec.Command("sh", "-c", `trap "" TERM; sleep 30 & wait`)
	stubborn.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := stubborn.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	writeFile(t, filepath.Join(StateDir(root, ref), pidFile), strconv.Itoa(stubborn.Process.Pid))
	if err := Stop(root, ref); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = stubborn.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = syscall.Kill(-stubborn.Process.Pid, syscall.SIGKILL)
		t.Fatal("stop should escalate to SIGKILL")
	}
}

func TestStoppedReviewStaysListedAsFailed(t *testing.T) {
	root := t.TempDir()
	ref := PRRef{Repo: "console", Number: 12, URL: "https://github.com/o/console/pull/12"}
	dir := StateDir(root, ref)
	if err := WriteMeta(dir, Meta{Ref: ref, Title: "Fix"}); err != nil {
		t.Fatal(err)
	}
	leader := exec.Command("sh", "-c", "sleep 30 & wait")
	leader.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := leader.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(-leader.Process.Pid, syscall.SIGKILL); _ = leader.Wait() })
	writeFile(t, filepath.Join(dir, pidFile), strconv.Itoa(leader.Process.Pid))
	if err := Stop(root, ref); err != nil {
		t.Fatal(err)
	}
	runs := ListRuns(root)
	if len(runs) != 1 || runs[0].Meta.Ref.URL != ref.URL || runs[0].Status != RunFailed {
		t.Errorf("runs = %+v, want the stopped review listed as failed", runs)
	}
}
