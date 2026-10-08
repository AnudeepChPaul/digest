package tui

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/AnudeepChPaul/digest/pkg/config"
)

func TestJobsWithoutADryRunHideD(t *testing.T) {
	m, _, dryRun := jobTestModel(t)
	m.cfg.Jobs = []config.JobSpec{{Name: "nightly", Command: "true"}, {Name: "janitor", Command: "digest janitor"}}
	m.contentVersion++
	selectNavItem(t, &m, "job:nightly")
	if hint := hintText(m); strings.Contains(hint, "(d)") {
		t.Errorf("a job with no dry-run command offers d: %q", hint)
	}
	if m = press(t, m, runes("d")); len(*dryRun) != 0 {
		t.Errorf("d started a dry run for a job without one: %v", *dryRun)
	}
	selectNavItem(t, &m, "job:janitor")
	if hint := hintText(m); !strings.Contains(hint, "(d)dry run") {
		t.Errorf("built-in jobs always have a dry run: %q", hint)
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
