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

	"github.com/AnudeepChPaul/digest/pkg/config"
	"github.com/AnudeepChPaul/digest/pkg/model"
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
	writeState(t, root, "2026-W40", map[string]string{runPIDFile: strconv.Itoa(os.Getpid())})
	original := parentPID
	parentPID = func() int { return -1 }
	defer func() { parentPID = original }()
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

func TestBackgroundWrapperRemovesThePIDFileWhenDone(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, runPIDFile)
	if err := os.WriteFile(pidPath, []byte("1"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("sh", "-c", backgroundScript("true"), "digest-brag", filepath.Join(dir, runExitFile), pidPath).Run(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pidPath); err == nil {
		t.Error("the wrapper should remove the pid file when the brag ends")
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
