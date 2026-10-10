package config

import (
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func jobNamed(t *testing.T, cfg *Config, name string) JobSpec {
	t.Helper()
	for _, job := range cfg.Jobs {
		if job.Name == name {
			return job
		}
	}
	t.Fatalf("no job %q in %+v", name, cfg.Jobs)
	return JobSpec{}
}

func TestBuiltInJobsComeFromDigest(t *testing.T) {
	cfg := configFromYAML(t, "digest_root: ~/digest\n")
	var names []string
	for _, job := range cfg.Jobs {
		names = append(names, job.Name)
	}
	if strings.Join(names, ",") != "branch-reaper,janitor,repo-sync" {
		t.Fatalf("jobs = %v", names)
	}
	for _, job := range cfg.Jobs {
		if job.Command != "digest "+job.Name || job.DryRunCommand != "digest "+job.Name+" --dry-run" {
			t.Errorf("%s commands = %q / %q", job.Name, job.Command, job.DryRunCommand)
		}
	}
	if got := cfg.JobOptionList(JobJanitor, OptionRoots, nil); !reflect.DeepEqual(got, []string{"~"}) {
		t.Errorf("janitor roots = %v", got)
	}
	for _, name := range []string{JobBranchReaper, JobRepoSync} {
		if got := cfg.JobOptionList(name, OptionRoots, nil); !reflect.DeepEqual(got, []string{"~/Projects"}) {
			t.Errorf("%s roots = %v", name, got)
		}
	}
	if days, _ := cfg.JobOptionInt(JobBranchReaper, OptionGraceDays, 0); days != DefaultBranchReaperGraceDays {
		t.Errorf("branch-reaper grace_days = %d", days)
	}
}

func TestConfiguredOptionsMergeOntoBuiltInDefaults(t *testing.T) {
	cfg := configFromYAML(t, `jobs:
  - name: Janitor
    options:
      grace-days: 30
      roots: [~, ~/Projects/]
      verbose: true
`)
	janitor := jobNamed(t, cfg, JobJanitor)
	want := []string{
		"DIGEST_OPTION_GRACE_DAYS=30",
		"DIGEST_OPTION_PATTERNS=" + strings.Join(DefaultJanitorPatterns, ","),
		"DIGEST_OPTION_ROOTS=~,~/Projects/",
		"DIGEST_OPTION_VERBOSE=true",
	}
	if got := janitor.OptionsEnv(); !reflect.DeepEqual(got, want) {
		t.Errorf("janitor env = %v, want %v", got, want)
	}
	if janitor.Command != "digest janitor" || len(cfg.Jobs) != 3 {
		t.Errorf("a configured built-in keeps digest's command and the list keeps every built-in: %+v", cfg.Jobs)
	}
	if got := cfg.JobOptionList(JobRepoSync, OptionRoots, nil); !reflect.DeepEqual(got, []string{"~/Projects"}) {
		t.Errorf("a job left out of config.yaml uses its defaults: %v", got)
	}
}

func TestBuiltInJobNamesIgnoreCaseSpacesAndUnderscores(t *testing.T) {
	cfg := configFromYAML(t, "jobs:\n  - name: branch reaper\n    options:\n      grace_days: 2\n  - name: repo_sync\n    options:\n      roots: [~/src]\n")
	if days, _ := cfg.JobOptionInt(JobBranchReaper, OptionGraceDays, 0); days != 2 {
		t.Errorf("branch reaper grace_days = %d", days)
	}
	if got := cfg.JobOptionList(JobRepoSync, OptionRoots, nil); !reflect.DeepEqual(got, []string{"~/src"}) {
		t.Errorf("repo_sync roots = %v", got)
	}
	if len(cfg.Jobs) != 3 {
		t.Errorf("jobs = %+v", cfg.Jobs)
	}
}

func TestBuiltInJobsRejectCommands(t *testing.T) {
	for _, entry := range []string{
		"  - name: janitor\n    command: digest janitor --root ~\n",
		"  - name: branch reaper\n    dry-run-command: digest branch-reaper --dry-run\n",
		"  - name: repo-sync\n    cmd: digest repo-sync\n",
	} {
		var cfg Config
		err := yaml.Unmarshal([]byte("jobs:\n"+entry), &cfg)
		if err == nil || !strings.Contains(err.Error(), "command and dry-run-command are built in; only options can be set; run digest migrate") {
			t.Errorf("%q: err = %v", entry, err)
		}
	}
}

func TestCustomJobsKeepTheirCommands(t *testing.T) {
	cfg := configFromYAML(t, "jobs:\n  - name: nightly\n    command: ./backup.sh\n    dry-run-command: ./backup.sh --check\n    options:\n      target: s3\n")
	nightly := jobNamed(t, cfg, "nightly")
	if nightly.Command != "./backup.sh" || nightly.DryRunCommand != "./backup.sh --check" || !reflect.DeepEqual(nightly.OptionsEnv(), []string{"DIGEST_OPTION_TARGET=s3"}) {
		t.Errorf("nightly = %+v", nightly)
	}
	if len(cfg.Jobs) != 4 || cfg.Jobs[3].Name != "nightly" {
		t.Errorf("custom jobs follow the built-ins: %+v", cfg.Jobs)
	}
}

func TestTheTemplateHasNoJobCommands(t *testing.T) {
	for _, removed := range []string{"\njobs:", "command:", "dry-run-command:"} {
		if strings.Contains(DefaultConfigYAML, removed) {
			t.Errorf("the template should not contain %q", removed)
		}
	}
	if got := len(DefaultConfig().Jobs); got != 3 {
		t.Errorf("default config jobs = %d", got)
	}
}

func TestDefaultJobsAreCopies(t *testing.T) {
	first := DefaultJobs()
	first[1].Options[OptionPatterns].([]string)[0] = "changed"
	first[1].Options[OptionGraceDays] = 1
	again := DefaultJobs()
	if again[1].Options[OptionPatterns].([]string)[0] == "changed" || again[1].Options[OptionGraceDays] != DefaultJanitorGraceDays {
		t.Error("callers must not change digest's default jobs")
	}
}
