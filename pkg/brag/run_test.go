package brag

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/config"
	"github.com/achandrapaul/digest/pkg/model"
)

func writeState(t *testing.T, root, id string, files map[string]string) {
	t.Helper()
	dir := StateDir(root, id)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestListRunsShowsRunningAndFailedOnly(t *testing.T) {
	root := t.TempDir()
	writeState(t, root, "2026-W40", map[string]string{runMetaFile: `{"id":"2026-W40"}`, runPIDFile: strconv.Itoa(os.Getpid())})
	writeState(t, root, "2026-09", map[string]string{runMetaFile: `{"id":"2026-09"}`, runExitFile: "1"})
	writeState(t, root, "2026-W39", map[string]string{runMetaFile: `{"id":"2026-W39"}`, runExitFile: "0"})
	runs := ListRuns(root)
	if len(runs) != 2 || runs[0].Meta.ID != "2026-W40" || runs[0].Status != RunRunning || runs[1].Meta.ID != "2026-09" || runs[1].Status != RunFailed {
		t.Errorf("runs = %+v", runs)
	}
	if !IsRunning(root, "2026-W40") || IsRunning(root, "2026-09") {
		t.Errorf("IsRunning wrong")
	}
}

func TestClaimRunWritesPIDAndExitCode(t *testing.T) {
	root := t.TempDir()
	release, err := claimRun(root, "2026-W40")
	if err != nil {
		t.Fatal(err)
	}
	if pid, ok := readInt(filepath.Join(StateDir(root, "2026-W40"), runPIDFile)); !ok || pid != os.Getpid() {
		t.Errorf("pid = %d", pid)
	}
	release(errors.New("boom"))
	if code, _ := readInt(filepath.Join(StateDir(root, "2026-W40"), runExitFile)); code != 1 {
		t.Errorf("exit = %d", code)
	}
	if Status(root, "2026-W40") != RunFailed {
		t.Errorf("status = %v", Status(root, "2026-W40"))
	}
}

func TestClaimRunRefusesOtherLiveRun(t *testing.T) {
	root := t.TempDir()
	writeState(t, root, "2026-W40", map[string]string{runPIDFile: strconv.Itoa(os.Getppid())})
	if _, err := claimRun(root, "2026-W40"); !errors.Is(err, ErrBragRunning) {
		t.Errorf("err = %v", err)
	}
}

func stubCommand(t *testing.T, output string) *[]string {
	t.Helper()
	original := runBragCommand
	var inputs []string
	runBragCommand = func(ctx context.Context, command, input string) (string, error) {
		inputs = append(inputs, input)
		return output, nil
	}
	t.Cleanup(func() { runBragCommand = original })
	return &inputs
}

func TestExecuteBuildsMonthFromWeeksWithMonthPrompt(t *testing.T) {
	cfg := &config.Config{DigestRoot: t.TempDir(), MonthBragPrompts: "MONTH PROMPT"}
	week := WeekOf(localDate(2026, 9, 30))
	if err := (&Brag{Period: week, Facts: "- shipped", Summary: "- Shipped"}).Save(cfg.BragDir()); err != nil {
		t.Fatal(err)
	}
	inputs := stubCommand(t, "## Summary\n- Great month")
	october := MonthOf(localDate(2026, 10, 1))
	noNotes := func() ([]*model.Note, error) { return nil, nil }
	if err := Execute(context.Background(), cfg, october, false, noNotes, localDate(2026, 11, 2)); err != nil {
		t.Fatal(err)
	}
	if len(*inputs) != 1 || !strings.HasPrefix((*inputs)[0], "MONTH PROMPT") || !strings.Contains((*inputs)[0], "- shipped") {
		t.Errorf("inputs = %q", *inputs)
	}
	saved, err := Load(cfg.BragDir(), october)
	if err != nil || saved.Summary != "- Great month" {
		t.Errorf("saved = %+v err = %v", saved, err)
	}
	if err := Execute(context.Background(), cfg, october, false, noNotes, localDate(2026, 11, 2)); err == nil {
		t.Errorf("creating over an existing brag should fail")
	}
}

func TestExecuteRefusesUnfinishedPeriods(t *testing.T) {
	cfg := &config.Config{DigestRoot: t.TempDir()}
	stubCommand(t, "## Summary\n- x")
	now := localDate(2026, 10, 7)
	noNotes := func() ([]*model.Note, error) { return nil, nil }
	if err := Execute(context.Background(), cfg, WeekOf(now), false, noNotes, now); err == nil {
		t.Errorf("current week should be refused")
	}
	if err := Execute(context.Background(), cfg, MonthOf(now), false, noNotes, now); err == nil {
		t.Errorf("current month should be refused")
	}
}

func TestExecuteRegenerateUsesSavedFacts(t *testing.T) {
	cfg := &config.Config{DigestRoot: t.TempDir(), PerformancePrompts: "YEAR PROMPT"}
	year := YearOf(localDate(2026, 1, 1))
	if err := (&Brag{Period: year, Created: time.Now(), Facts: "### September 2026\n- edited by me"}).Save(cfg.BragDir()); err != nil {
		t.Fatal(err)
	}
	inputs := stubCommand(t, "## Summary\nReview body")
	if err := Execute(context.Background(), cfg, year, true, nil, localDate(2026, 10, 7)); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix((*inputs)[0], "YEAR PROMPT") || !strings.Contains((*inputs)[0], "- edited by me") {
		t.Errorf("inputs = %q", *inputs)
	}
}

func TestPeriodFromFlags(t *testing.T) {
	if period, err := PeriodFromFlags("2026-W40", "", ""); err != nil || period.ID() != "2026-W40" {
		t.Errorf("week flag: %v %v", period, err)
	}
	if period, err := PeriodFromFlags("", "2026-10", ""); err != nil || period.Kind() != config.PromptMonth {
		t.Errorf("month flag: %v %v", period, err)
	}
	for _, flags := range [][3]string{{"", "", ""}, {"2026-W40", "2026-10", ""}, {"2026-10", "", ""}, {"", "", "2026-10"}} {
		if _, err := PeriodFromFlags(flags[0], flags[1], flags[2]); err == nil {
			t.Errorf("flags %v should be rejected", flags)
		}
	}
}

func TestStatusClearsAPIDFileOfAnExitedProcess(t *testing.T) {
	root := t.TempDir()
	exited := exec.Command("true")
	if err := exited.Run(); err != nil {
		t.Fatal(err)
	}
	writeState(t, root, "2026-W40", map[string]string{runPIDFile: strconv.Itoa(exited.Process.Pid)})
	if IsRunning(root, "2026-W40") {
		t.Fatal("an exited process is not running")
	}
	if _, err := os.Stat(filepath.Join(StateDir(root, "2026-W40"), runPIDFile)); err == nil {
		t.Error("the pid file of an exited process should be removed")
	}
}

func fakeDigest(t *testing.T, body string) string {
	t.Helper()
	script := filepath.Join(t.TempDir(), "fake-digest")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"+body+"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	original := executablePath
	executablePath = func() (string, error) { return script, nil }
	t.Cleanup(func() { executablePath = original })
	return script
}

func waitForBrag(t *testing.T, root, id string) RunStatus {
	t.Helper()
	pidPath := filepath.Join(StateDir(root, id), runPIDFile)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(pidPath); err != nil && !IsRunning(root, id) {
			return Status(root, id)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("background brag did not finish")
	return RunIdle
}

func TestBackgroundRunRemovesThePIDFileWhenDone(t *testing.T) {
	root := t.TempDir()
	fakeDigest(t, `echo "$@"`)
	week := WeekOf(localDate(2026, 9, 30))
	if err := StartBackground(root, week, false); err != nil {
		t.Fatal(err)
	}
	if status := waitForBrag(t, root, week.ID()); status != RunDone {
		t.Errorf("status = %v", status)
	}
	if _, err := os.Stat(filepath.Join(StateDir(root, week.ID()), runPIDFile)); err == nil {
		t.Error("the pid file should be removed when the brag ends")
	}
	logText, _ := os.ReadFile(filepath.Join(StateDir(root, week.ID()), RunLogFile))
	if !strings.Contains(string(logText), "brag --week "+week.ID()+" --regenerate=false") {
		t.Errorf("log = %q", logText)
	}
}

func TestBackgroundRunRecordsAnUnreportedExitCode(t *testing.T) {
	root := t.TempDir()
	fakeDigest(t, "exit 2")
	week := WeekOf(localDate(2026, 9, 30))
	if err := StartBackground(root, week, false); err != nil {
		t.Fatal(err)
	}
	if status := waitForBrag(t, root, week.ID()); status != RunFailed {
		t.Errorf("status = %v", status)
	}
	if code, _ := readInt(filepath.Join(StateDir(root, week.ID()), runExitFile)); code != 2 {
		t.Errorf("exit = %d", code)
	}
}

func TestBackgroundRunKeepsTheExitCodeTheChildWrote(t *testing.T) {
	root := t.TempDir()
	week := WeekOf(localDate(2026, 9, 30))
	exitPath := filepath.Join(StateDir(root, week.ID()), runExitFile)
	fakeDigest(t, `printf 1 > "`+exitPath+`"; exit 0`)
	if err := StartBackground(root, week, false); err != nil {
		t.Fatal(err)
	}
	if status := waitForBrag(t, root, week.ID()); status != RunFailed {
		t.Errorf("status = %v", status)
	}
}

func TestClaimRunAcceptsThePIDItsLauncherWrote(t *testing.T) {
	root := t.TempDir()
	writeState(t, root, "2026-W40", map[string]string{runPIDFile: strconv.Itoa(os.Getpid()), runExitFile: "1"})
	release, err := claimRun(root, "2026-W40")
	if err != nil {
		t.Fatal(err)
	}
	if Status(root, "2026-W40") != RunRunning {
		t.Errorf("status = %v", Status(root, "2026-W40"))
	}
	release(nil)
	if Status(root, "2026-W40") != RunDone {
		t.Errorf("status = %v", Status(root, "2026-W40"))
	}
	if _, err := os.Stat(filepath.Join(StateDir(root, "2026-W40"), runPIDFile)); err == nil {
		t.Error("release should remove the pid file")
	}
}

func TestStopKillsAGroupThatIgnoresTerm(t *testing.T) {
	original := stopGracePeriod
	stopGracePeriod = 200 * time.Millisecond
	t.Cleanup(func() { stopGracePeriod = original })
	root := t.TempDir()
	stubborn := exec.Command("sh", "-c", `trap "" TERM; sleep 30 & wait`)
	stubborn.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := stubborn.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	writeState(t, root, "2026-W40", map[string]string{runPIDFile: strconv.Itoa(stubborn.Process.Pid)})
	if err := Stop(root, "2026-W40"); err != nil {
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

func TestChildOutputAndCrashesLandInTheRunLog(t *testing.T) {
	root := t.TempDir()
	fakeDigest(t, `echo out; echo "panic: boom" >&2; exit 2`)
	week := WeekOf(localDate(2026, 9, 30))
	if err := StartBackground(root, week, false); err != nil {
		t.Fatal(err)
	}
	waitForBrag(t, root, week.ID())
	logText, _ := os.ReadFile(filepath.Join(StateDir(root, week.ID()), RunLogFile))
	if !strings.Contains(string(logText), "out\n") || !strings.Contains(string(logText), "panic: boom") {
		t.Errorf("stdout and stderr of the child should land in the run log: %q", logText)
	}
}

func TestExecuteReplacesAnUnreadableBrag(t *testing.T) {
	cfg := &config.Config{DigestRoot: t.TempDir()}
	week := WeekOf(localDate(2026, 9, 30))
	if err := (&Brag{Period: week, Facts: "- shipped", Summary: "- Shipped"}).Save(cfg.BragDir()); err != nil {
		t.Fatal(err)
	}
	stubCommand(t, "## Summary\n- Great month")
	october := MonthOf(localDate(2026, 10, 1))
	if err := os.MkdirAll(filepath.Dir(october.Path(cfg.BragDir())), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(october.Path(cfg.BragDir()), []byte("not a brag"), 0o600); err != nil {
		t.Fatal(err)
	}
	noNotes := func() ([]*model.Note, error) { return nil, nil }
	if err := Execute(context.Background(), cfg, october, false, noNotes, localDate(2026, 11, 2)); err != nil {
		t.Fatalf("an unreadable brag should be replaced, err = %v", err)
	}
	if saved, err := Load(cfg.BragDir(), october); err != nil || saved.Summary != "- Great month" {
		t.Errorf("saved = %+v err = %v", saved, err)
	}
}
