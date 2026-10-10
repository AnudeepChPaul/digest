package tui

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/config"
)

func TestJobsWithoutADryRunHideD(t *testing.T) {
	m, _, dryRun := jobTestModel(t)
	m.cfg.Jobs = []config.JobSpec{{Name: "nightly", Command: "true"}, {Name: "janitor", Command: "digest janitor"}, {Name: "branch-reaper", Command: "digest branch-reaper", DryRunCommand: "digest branch-reaper --dry-run"}}
	m.contentVersion++
	selectNavItem(t, &m, "job:nightly")
	if hint := hintText(m); strings.Contains(hint, "(d)") {
		t.Errorf("a job with no dry-run command offers d: %q", hint)
	}
	if m = press(t, m, runes("d")); len(*dryRun) != 0 {
		t.Errorf("d started a dry run for a job without one: %v", *dryRun)
	}
	selectNavItem(t, &m, "job:janitor")
	if hint := hintText(m); strings.Contains(hint, "(d)") {
		t.Errorf("a built-in job with no dry-run command offers d: %q", hint)
	}
	if m = press(t, m, runes("d")); len(*dryRun) != 0 {
		t.Errorf("d started a dry run for a built-in job without one: %v", *dryRun)
	}
	selectNavItem(t, &m, "job:branch-reaper")
	if hint := hintText(m); !strings.Contains(hint, "(d)dry run") {
		t.Errorf("a job with a dry-run command offers d: %q", hint)
	}
}

func TestDigestDryRunCommandsMustSayDryRun(t *testing.T) {
	_, err := resolveJobCommand(config.JobSpec{Name: "janitor", DryRunCommand: "digest janitor --root ~"}, true)
	if !errors.Is(err, errDryRunFlagMissing) {
		t.Errorf("a digest dry-run command without --dry-run should be refused, err = %v", err)
	}
	if _, err := resolveJobCommand(config.JobSpec{Name: "custom", DryRunCommand: "./check.sh"}, true); err != nil {
		t.Errorf("custom dry-run commands run as written: %v", err)
	}
}

func TestConfiguredDigestCommandsUseTheRunningBinary(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Skip(err)
	}
	for _, dryRun := range []bool{true, false} {
		command, err := resolveJobCommand(config.JobSpec{Name: "janitor", Command: "digest janitor --root ~", DryRunCommand: "digest janitor --dry-run --root ~"}, dryRun)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(command, shellQuote(executable)+" janitor ") {
			t.Errorf("dry run %v: command = %q, want it to start with this binary", dryRun, command)
		}
	}
	if command, _ := resolveJobCommand(config.JobSpec{Name: "custom", Command: "./digest-like.sh run"}, false); command != "./digest-like.sh run" {
		t.Errorf("non-digest command rewritten: %q", command)
	}
}

func TestDryRunRecordsTheCommandsExitCodeAndOutput(t *testing.T) {
	previousRoot := digestRoot
	digestRoot = t.TempDir()
	t.Cleanup(func() { digestRoot = previousRoot })
	cases := map[string]struct {
		command  string
		exitCode int
	}{
		"dry-exit":   {command: "echo checked; echo warned >&2; exit 3", exitCode: 3},
		"dry-ok":     {command: "echo checked", exitCode: 0},
		"dry-signal": {command: "echo checked; kill -TERM $$", exitCode: 143},
	}
	for jobName, expected := range cases {
		if err := startDryRunBackground(config.JobSpec{Name: jobName, DryRunCommand: expected.command}); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(5 * time.Second)
		output, exitCode, finished := loadDryRunResult(jobName)
		for !finished && time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
			output, exitCode, finished = loadDryRunResult(jobName)
		}
		if !finished {
			t.Fatalf("%s never wrote its exit file", jobName)
		}
		for isDryRunInFlight(jobName) && time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
		}
		if exitCode != expected.exitCode || !strings.Contains(output, "checked") {
			t.Errorf("%s: exit %d output %q, want exit %d with its output", jobName, exitCode, output, expected.exitCode)
		}
		if jobName == "dry-exit" && !strings.Contains(output, "warned") {
			t.Errorf("stderr missing from the dry-run log: %q", output)
		}
	}
}
