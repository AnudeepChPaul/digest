package migrate

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/achandrapaul/digest/pkg/config"
)

func (f *fixture) withConfig(t *testing.T, content string) (Options, string) {
	t.Helper()
	configPath := filepath.Join(f.root, "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	options := f.options()
	options.ConfigPath = configPath
	return options, configPath
}

func readConfig(t *testing.T, path string) (string, *config.Config) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("migrated config does not load: %v\n%s", err, content)
	}
	return string(content), cfg
}

const oldJanitorConfig = `digest_root: ~/digest
# keep me
retention_days: 3
janitor_patterns:
  - a.log
  - "*.tmp"
jobs:
  - name: branch-reaper
    command: "digest branch-reaper --root ~/Projects"
  - name: janitor
    dry-run-command: "digest janitor --root ~ --dry-run"
    command: "digest janitor --root ~"
`

func TestMigrateMovesTheOldJanitorKeysIntoTheJanitorOptions(t *testing.T) {
	f := newFixture(t)
	options, configPath := f.withConfig(t, oldJanitorConfig)
	if _, err := Run(options); err != nil {
		t.Fatal(err)
	}
	content, cfg := readConfig(t, configPath)
	for _, removed := range []string{"retention_days", "janitor_patterns"} {
		if strings.Contains(content, removed) {
			t.Errorf("%s should be removed:\n%s", removed, content)
		}
	}
	if !strings.Contains(content, "# keep me") || !strings.Contains(content, "digest_root: ~/digest") {
		t.Errorf("the rest of the config should be kept:\n%s", content)
	}
	if got := cfg.Retention(); got != 3 {
		t.Errorf("janitor grace_days = %d, want 3", got)
	}
	if got := cfg.JobOptionList(config.JobJanitor, config.OptionPatterns, nil); !reflect.DeepEqual(got, []string{"a.log", "*.tmp"}) {
		t.Errorf("janitor patterns = %v", got)
	}
	if len(cfg.Jobs) != 3 || strings.Contains(content, "command:") || !reflect.DeepEqual(cfg.JobOptionList(config.JobJanitor, config.OptionRoots, nil), []string{"~"}) {
		t.Errorf("jobs = %+v\n%s", cfg.Jobs, content)
	}
	if !strings.Contains(f.output.String(), "moved retention_days, janitor_patterns into the janitor job's options") {
		t.Errorf("output = %q", f.output.String())
	}

	f.output.Reset()
	if _, err := Run(options); err != nil {
		t.Fatal(err)
	}
	if again, _ := readConfig(t, configPath); again != content {
		t.Errorf("a second migrate should change nothing:\n%s\nvs\n%s", again, content)
	}
	if strings.Contains(f.output.String(), "moved") {
		t.Errorf("second run output = %q", f.output.String())
	}
}

func TestMigrateKeepsJanitorOptionsAlreadySet(t *testing.T) {
	f := newFixture(t)
	options, configPath := f.withConfig(t, "retention_days: 3\njobs:\n  - name: janitor\n    command: digest janitor\n    options:\n      grace_days: 30\n")
	if _, err := Run(options); err != nil {
		t.Fatal(err)
	}
	content, cfg := readConfig(t, configPath)
	if strings.Contains(content, "retention_days") || cfg.Retention() != 30 {
		t.Errorf("the janitor's own grace_days should win and the old key go:\n%s", content)
	}
}

func TestMigrateWritesAnOptionsOnlyJanitorWhenNoJobsAreListed(t *testing.T) {
	f := newFixture(t)
	options, configPath := f.withConfig(t, "janitor_patterns: [core.*]\n")
	if _, err := Run(options); err != nil {
		t.Fatal(err)
	}
	content, cfg := readConfig(t, configPath)
	if !strings.Contains(content, "jobs:\n  - name: janitor\n    options:\n      patterns: [core.*]\n") || strings.Contains(content, "command") {
		t.Errorf("only the janitor's options should be written:\n%s", content)
	}
	if len(cfg.Jobs) != 3 || cfg.Jobs[0].Name != config.JobBranchReaper || cfg.Jobs[1].Name != config.JobJanitor {
		t.Fatalf("jobs = %+v, want the built-ins", cfg.Jobs)
	}
	if got := cfg.JobOptionList(config.JobJanitor, config.OptionPatterns, nil); !reflect.DeepEqual(got, []string{"core.*"}) {
		t.Errorf("patterns = %v", got)
	}
	if got := cfg.Retention(); got != config.DefaultJanitorGraceDays {
		t.Errorf("grace_days = %d, want the default", got)
	}
	if days, _ := cfg.JobOptionInt(config.JobBranchReaper, config.OptionGraceDays, 0); days != config.DefaultBranchReaperGraceDays {
		t.Errorf("branch-reaper grace_days = %d", days)
	}
}

func TestMigrateAddsAJanitorEntryNextToCustomJobs(t *testing.T) {
	f := newFixture(t)
	options, configPath := f.withConfig(t, "retention_days: 3\njobs:\n  - name: custom\n    command: echo hi\n")
	if _, err := Run(options); err != nil {
		t.Fatal(err)
	}
	content, cfg := readConfig(t, configPath)
	if !strings.Contains(content, "  - name: custom\n    command: echo hi\n  - name: janitor\n    options:\n      grace_days: 3\n") || cfg.Retention() != 3 {
		t.Errorf("the custom job should stay and the janitor get the old key:\n%s", content)
	}
}

func TestMigrateWithoutAConfigFileOrPath(t *testing.T) {
	f := newFixture(t)
	options := f.options()
	if _, err := Run(options); err != nil {
		t.Errorf("no config path: %v", err)
	}
	options.ConfigPath = filepath.Join(f.root, "missing.yaml")
	if _, err := Run(options); err != nil {
		t.Errorf("missing config: %v", err)
	}
	if _, err := os.Stat(options.ConfigPath); err == nil {
		t.Error("migrate should not create a config")
	}
}

func TestMigrateRejectsAConfigItCannotRead(t *testing.T) {
	for name, content := range map[string]string{
		"invalid yaml":    "jobs: [\n",
		"not a mapping":   "- a\n- b\n",
		"jobs not a list": "retention_days: 3\njobs: nope\n",
		"empty":           "",
	} {
		f := newFixture(t)
		options, configPath := f.withConfig(t, content)
		_, err := Run(options)
		if name == "empty" {
			if err != nil {
				t.Errorf("%s: %v", name, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), configPath) {
			t.Errorf("%s: err = %v, want one naming %s", name, err, configPath)
		}
	}
	f := newFixture(t)
	options, _ := f.withConfig(t, "")
	options.ConfigPath = f.root
	if _, err := Run(options); err == nil {
		t.Error("a config path that is a directory should be an error")
	}
}

func TestMigrateReportsAConfigItCannotWrite(t *testing.T) {
	f := newFixture(t)
	configDir := filepath.Join(f.root, "config")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(oldJanitorConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(configPath, 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(configDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(configDir, 0o700) })
	options := f.options()
	options.ConfigPath = configPath
	if _, err := Run(options); err == nil || !strings.Contains(err.Error(), "write "+configPath) {
		t.Errorf("err = %v, want a failed write", err)
	}
}
