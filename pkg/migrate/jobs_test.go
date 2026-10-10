package migrate

import (
	"bytes"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/achandrapaul/digest/pkg/config"
)

const commandJobsConfig = `digest_root: ~/digest
jobs:
  # mine
  - name: branch reaper
    dry-run-command: "digest branch-reaper --dry-run --root ~/Projects/"
    command: "digest branch-reaper --root ~/Projects/"
  - name: janitor
    dry-run-command: "digest janitor --dry-run --root ~ --root ~/Projects/ --root ~/digest/reviews/"
    command: "digest janitor --root ~ --root=~/Projects/"
    options:
      grace_days: 21
  - name: repo sync
    dry-run-command: "digest repo-sync --dry-run --root ~/src"
    options:
      roots: [~/kept]
  - name: nightly
    command: ./backup.sh --root /data
`

func TestMigrateMovesBuiltInJobCommandsIntoDigest(t *testing.T) {
	f := newFixture(t)
	options, configPath := f.withConfig(t, commandJobsConfig)
	var out bytes.Buffer
	options.Out = &out
	if _, err := Run(options); err != nil {
		t.Fatal(err)
	}
	content, cfg := readConfig(t, configPath)
	for _, gone := range []string{"digest branch-reaper", "digest janitor", "digest repo-sync", "name: branch reaper", "name: repo sync"} {
		if strings.Contains(content, gone) {
			t.Errorf("%q should be gone:\n%s", gone, content)
		}
	}
	if !strings.Contains(content, "# mine") || !strings.Contains(content, "command: ./backup.sh --root /data") {
		t.Errorf("comments and custom jobs should be kept:\n%s", content)
	}
	for job, want := range map[string][]string{
		config.JobBranchReaper: {"~/Projects/"},
		config.JobJanitor:      {"~", "~/Projects/"},
		config.JobRepoSync:     {"~/kept"},
	} {
		if got := cfg.JobOptionList(job, config.OptionRoots, nil); !reflect.DeepEqual(got, want) {
			t.Errorf("%s roots = %v, want %v", job, got, want)
		}
	}
	if days, _ := cfg.JobOptionInt(config.JobJanitor, config.OptionGraceDays, 0); days != 21 {
		t.Errorf("janitor grace_days = %d", days)
	}
	if len(cfg.Jobs) != 4 {
		t.Errorf("jobs = %+v", cfg.Jobs)
	}
	if !strings.Contains(out.String(), "removed the commands of branch-reaper, janitor, repo-sync in "+configPath+"; their --root values are now the roots option") {
		t.Errorf("output = %q", out.String())
	}

	out.Reset()
	if _, err := Run(options); err != nil {
		t.Fatal(err)
	}
	if again, _ := os.ReadFile(configPath); string(again) != content || strings.Contains(out.String(), "commands") {
		t.Errorf("a second migrate should change nothing, output %q", out.String())
	}
}

func TestMigrateDryRunOnlyReportsJobCommands(t *testing.T) {
	f := newFixture(t)
	options, configPath := f.withConfig(t, commandJobsConfig)
	var out bytes.Buffer
	options.Out, options.DryRun = &out, true
	if _, err := Run(options); err != nil {
		t.Fatal(err)
	}
	if content, _ := os.ReadFile(configPath); string(content) != commandJobsConfig {
		t.Errorf("a dry run changed the config:\n%s", content)
	}
	if !strings.Contains(out.String(), "would remove the commands of branch-reaper, janitor, repo-sync in "+configPath) {
		t.Errorf("output = %q", out.String())
	}
}
