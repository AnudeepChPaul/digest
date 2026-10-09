package automation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/config"
)

func TestRunJobRefusesNoteIDsOutsideTheRoot(t *testing.T) {
	cfg := &config.Config{DigestRoot: t.TempDir()}
	for _, noteID := range []string{"", "..", "../escape", "a/b"} {
		if err := RunJob(context.Background(), cfg, noteID, "PROJ", PhaseDraft); !errors.Is(err, ErrInvalidNoteID) {
			t.Errorf("RunJob(%q) = %v", noteID, err)
		}
	}
}

func TestClaimRunRecordsTheExitCodeAndClearsThePID(t *testing.T) {
	for runErr, want := range map[error]RunStatus{nil: RunDraftReady, fmt.Errorf("token: %w", ErrNeedsReauth): RunNeedsReauth, errors.New("boom"): RunFailed} {
		root := t.TempDir()
		writeRunState(t, root, "note-1", PhaseDraft, "")
		release, err := claimRun(root, "note-1")
		if err != nil {
			t.Fatal(err)
		}
		if pid, found := readInt(filepath.Join(StateDir(root, "note-1"), runPIDFile)); !found || pid != os.Getpid() {
			t.Errorf("pid = %d", pid)
		}
		if Status(root, "note-1").Status != RunRunning {
			t.Errorf("claimed run is not running")
		}
		release(runErr)
		if run := Status(root, "note-1"); run.Status != want {
			t.Errorf("%v: status = %v, want %v", runErr, run.Status, want)
		}
		if _, err := os.Stat(filepath.Join(StateDir(root, "note-1"), runPIDFile)); err == nil {
			t.Errorf("%v: pid file kept", runErr)
		}
	}
}

func TestClaimRunRefusesAnotherLiveRunButAcceptsItsOwnPID(t *testing.T) {
	root := t.TempDir()
	writeRunState(t, root, "note-1", PhaseDraft, "")
	pidPath := filepath.Join(StateDir(root, "note-1"), runPIDFile)
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getppid())), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := claimRun(root, "note-1"); !errors.Is(err, ErrRunning) {
		t.Errorf("err = %v", err)
	}
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		t.Fatal(err)
	}
	release, err := claimRun(root, "note-1")
	if err != nil {
		t.Fatal(err)
	}
	release(nil)
}

func TestKilledBackgroundRunWithoutExitFileFails(t *testing.T) {
	root := t.TempDir()
	script := filepath.Join(root, "fake-digest")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nkill -9 $$\n"), 0755); err != nil {
		t.Fatal(err)
	}
	previous := executablePath
	executablePath = func() (string, error) { return script, nil }
	t.Cleanup(func() { executablePath = previous })
	if err := StartBackground(root, "note-1", "PROJ", PhaseDraft); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for Status(root, "note-1").Status == RunRunning && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if run := Status(root, "note-1"); run.Status != RunFailed {
		t.Errorf("run = %+v", run)
	}
	if _, err := os.Stat(filepath.Join(StateDir(root, "note-1"), runExitFile)); err == nil {
		t.Error("a killed run should leave no exit file")
	}
}

func TestChildOutputAndCrashesLandInTheRunLog(t *testing.T) {
	root := t.TempDir()
	script := filepath.Join(root, "fake-digest")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho logged\necho \"panic: boom\" >&2\n"), 0755); err != nil {
		t.Fatal(err)
	}
	previous := executablePath
	executablePath = func() (string, error) { return script, nil }
	t.Cleanup(func() { executablePath = previous })
	if err := StartBackground(root, "note-1", "PROJ", PhaseDraft); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for Status(root, "note-1").Status == RunRunning && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if logText, _ := os.ReadFile(LogPath(root, "note-1")); string(logText) != "logged\npanic: boom\n" {
		t.Errorf("log = %q", logText)
	}
}
