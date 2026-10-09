package tui

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/config"
)

const launchJobsRootEnv = "DIGEST_TEST_LAUNCH_JOBS_ROOT"

func TestHelperLaunchesJobsThenQuits(t *testing.T) {
	root := os.Getenv(launchJobsRootEnv)
	if root == "" {
		t.Skip("only runs as a helper process")
	}
	digestRoot = root
	showGit := false
	cfg := &config.Config{DigestRoot: root, ShowGit: &showGit, Jobs: []config.JobSpec{{Name: "survivor", Command: "sleep 0.5; echo after the app quit"}}}
	if err := os.MkdirAll(cfg.NotesDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := startDryRunBackground(config.JobSpec{Name: "survivor-dry", DryRunCommand: "sleep 0.5; echo after the app quit; exit 4"}); err != nil {
		t.Fatal(err)
	}
	if err := executeJobBackground(cfg, "survivor"); err != nil {
		t.Fatal(err)
	}
	os.Exit(0)
}

func TestJobsKeepRunningAfterTheAppQuits(t *testing.T) {
	root := t.TempDir()
	helper := exec.Command(os.Args[0], "-test.run=^TestHelperLaunchesJobsThenQuits$")
	helper.Env = append(os.Environ(), launchJobsRootEnv+"="+root)
	if output, err := helper.CombinedOutput(); err != nil {
		t.Fatalf("helper failed: %v\n%s", err, output)
	}
	previousRoot := digestRoot
	digestRoot = root
	t.Cleanup(func() { digestRoot = previousRoot })
	deadline := time.Now().Add(5 * time.Second)
	output, exitCode, finished := loadDryRunResult("survivor-dry")
	for (!finished || !strings.Contains(readJobLog("survivor"), "after the app quit")) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		output, exitCode, finished = loadDryRunResult("survivor-dry")
	}
	if !finished || exitCode != 4 || !strings.Contains(output, "after the app quit") {
		t.Errorf("dry run should finish after the app quits: finished %v exit %d output %q", finished, exitCode, output)
	}
	if log := readJobLog("survivor"); !strings.Contains(log, "after the app quit") {
		t.Errorf("job should keep logging after the app quits: %q", log)
	}
}
