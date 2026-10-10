package config

import (
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func configFromYAML(t *testing.T, raw string) *Config {
	t.Helper()
	var cfg Config
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	return &cfg
}

const jobsWithOptions = `jobs:
  - name: branch-reaper
    options:
      grace_days: 3
  - name: janitor
    options:
      grace-days: 30
      patterns:
        - "*.hprof"
        - core.*
      verbose: true
  - name: custom
    command: "echo hi"
`

func TestJobOptionsReachTheProcessAsDigestOptionEnv(t *testing.T) {
	cfg := configFromYAML(t, jobsWithOptions)
	want := []string{"DIGEST_OPTION_GRACE_DAYS=30", "DIGEST_OPTION_PATTERNS=*.hprof,core.*", "DIGEST_OPTION_ROOTS=~", "DIGEST_OPTION_VERBOSE=true"}
	if got := jobNamed(t, cfg, JobJanitor).OptionsEnv(); !reflect.DeepEqual(got, want) {
		t.Errorf("janitor env = %v, want %v", got, want)
	}
	if got := jobNamed(t, cfg, JobBranchReaper).OptionsEnv(); !reflect.DeepEqual(got, []string{"DIGEST_OPTION_GRACE_DAYS=3", "DIGEST_OPTION_ROOTS=~/Projects"}) {
		t.Errorf("branch-reaper env = %v", got)
	}
	if got := jobNamed(t, cfg, "custom").OptionsEnv(); len(got) != 0 {
		t.Errorf("a job without options gets no env: %v", got)
	}
	if reaper := jobNamed(t, cfg, JobBranchReaper); reaper.Command != "digest branch-reaper" || reaper.DryRunCommand != "digest branch-reaper --dry-run" {
		t.Errorf("options should not disturb the commands: %+v", reaper)
	}
}

func TestBuiltInJobOptionsPreferTheEnvThenTheConfigThenTheDefault(t *testing.T) {
	cfg := configFromYAML(t, jobsWithOptions)
	if days, err := cfg.JobOptionInt(JobBranchReaper, OptionGraceDays, 7); err != nil || days != 3 {
		t.Errorf("config grace_days = %d, %v; want 3", days, err)
	}
	if days, err := cfg.JobOptionInt("custom", OptionGraceDays, 7); err != nil || days != 7 {
		t.Errorf("unset option = %d, %v; want the default 7", days, err)
	}
	if days, err := cfg.JobOptionInt("missing", OptionGraceDays, 7); err != nil || days != 7 {
		t.Errorf("unknown job = %d, %v; want the default 7", days, err)
	}
	var nilConfig *Config
	if days, err := nilConfig.JobOptionInt(JobJanitor, OptionGraceDays, 14); err != nil || days != 14 {
		t.Errorf("nil config = %d, %v; want 14", days, err)
	}

	t.Setenv("DIGEST_OPTION_GRACE_DAYS", "11")
	if days, err := cfg.JobOptionInt(JobBranchReaper, OptionGraceDays, 7); err != nil || days != 11 {
		t.Errorf("env grace_days = %d, %v; want 11", days, err)
	}

	t.Setenv("DIGEST_OPTION_GRACE_DAYS", "soon")
	if _, err := cfg.JobOptionInt(JobBranchReaper, OptionGraceDays, 7); err == nil || !strings.Contains(err.Error(), "grace_days") {
		t.Errorf("unparseable grace_days should name the option: %v", err)
	}
	t.Setenv("DIGEST_OPTION_GRACE_DAYS", "-1")
	if _, err := cfg.JobOptionInt(JobBranchReaper, OptionGraceDays, 7); err == nil {
		t.Error("negative grace_days should be rejected")
	}
}

func TestListOptionsSplitOnCommas(t *testing.T) {
	cfg := configFromYAML(t, jobsWithOptions)
	if got := cfg.JobOptionList(JobJanitor, OptionPatterns, []string{"x"}); !reflect.DeepEqual(got, []string{"*.hprof", "core.*"}) {
		t.Errorf("config patterns = %v", got)
	}
	if got := cfg.JobOptionList("custom", OptionPatterns, []string{"x"}); !reflect.DeepEqual(got, []string{"x"}) {
		t.Errorf("unset patterns = %v, want the default", got)
	}
	t.Setenv("DIGEST_OPTION_PATTERNS", " a.log , ,b.tmp")
	if got := cfg.JobOptionList(JobJanitor, OptionPatterns, []string{"x"}); !reflect.DeepEqual(got, []string{"a.log", "b.tmp"}) {
		t.Errorf("env patterns = %v", got)
	}
	t.Setenv("DIGEST_OPTION_PATTERNS", " , ")
	if got := cfg.JobOptionList(JobJanitor, OptionPatterns, []string{"x"}); !reflect.DeepEqual(got, []string{"x"}) {
		t.Errorf("blank patterns = %v, want the default", got)
	}
}

func TestJanitorOptionsReplaceTheOldTopLevelKeys(t *testing.T) {
	old := configFromYAML(t, "retention_days: 3\njanitor_patterns: [a.log]\n")
	if got := old.Retention(); got != DefaultJanitorGraceDays {
		t.Errorf("top-level retention_days is no longer read: Retention = %d", got)
	}
	if got := old.JobOptionList(JobJanitor, OptionPatterns, nil); !reflect.DeepEqual(got, DefaultJanitorPatterns) {
		t.Errorf("top-level janitor_patterns is no longer read: %v", got)
	}
	configured := configFromYAML(t, "jobs:\n  - name: janitor\n    options:\n      grace_days: 5\n")
	if got := configured.Retention(); got != 5 {
		t.Errorf("Retention = %d, want the janitor's grace_days 5", got)
	}
	broken := configFromYAML(t, "jobs:\n  - name: janitor\n    options:\n      grace_days: never\n")
	if got := broken.Retention(); got != DefaultJanitorGraceDays {
		t.Errorf("Retention with a broken grace_days = %d, want the default", got)
	}
}

func TestDefaultConfigCarriesTheBuiltInJobOptions(t *testing.T) {
	for _, removed := range []string{"\nretention_days:", "\njanitor_patterns:"} {
		if strings.Contains(DefaultConfigYAML, removed) {
			t.Errorf("template should drop %q", removed)
		}
	}
	templateJobs := DefaultConfig().Jobs
	builtIns := DefaultJobs()
	if len(templateJobs) != len(builtIns) {
		t.Fatalf("template jobs %v, built-ins %v", templateJobs, builtIns)
	}
	for index := range builtIns {
		if !reflect.DeepEqual(templateJobs[index].OptionsEnv(), builtIns[index].OptionsEnv()) {
			t.Errorf("%s: template options %v, built-in %v", builtIns[index].Name, templateJobs[index].OptionsEnv(), builtIns[index].OptionsEnv())
		}
	}
	defaults := DefaultConfig()
	if got := defaults.JobOptionList(JobJanitor, OptionPatterns, nil); !reflect.DeepEqual(got, DefaultJanitorPatterns) {
		t.Errorf("template patterns = %v", got)
	}
}

func TestAnEmptyOptionIsUnset(t *testing.T) {
	cfg := configFromYAML(t, "jobs:\n  - name: nightly\n    command: ./backup.sh\n    options:\n      grace_days:\n  - name: janitor\n    options:\n      grace_days:\n")
	if got := jobNamed(t, cfg, "nightly").OptionsEnv(); !reflect.DeepEqual(got, []string{"DIGEST_OPTION_GRACE_DAYS="}) {
		t.Errorf("env = %v", got)
	}
	if got := cfg.Retention(); got != DefaultJanitorGraceDays {
		t.Errorf("Retention = %d, want the default", got)
	}
}

func TestBoolOptionsComeFromTheEnvOrTheConfig(t *testing.T) {
	cfg := configFromYAML(t, "jobs:\n  - name: janitor\n    options:\n      no_quarantine: true\n")
	if on, err := cfg.JobOptionBool(JobJanitor, OptionNoQuarantine); err != nil || !on {
		t.Errorf("config: %v, %v", on, err)
	}
	if on, err := configFromYAML(t, "jobs: []\n").JobOptionBool(JobJanitor, OptionNoQuarantine); err != nil || on {
		t.Errorf("unset: %v, %v", on, err)
	}
	t.Setenv("DIGEST_OPTION_NO_QUARANTINE", "false")
	if on, err := cfg.JobOptionBool(JobJanitor, OptionNoQuarantine); err != nil || on {
		t.Errorf("env: %v, %v", on, err)
	}
	t.Setenv("DIGEST_OPTION_NO_QUARANTINE", "maybe")
	if _, err := cfg.JobOptionBool(JobJanitor, OptionNoQuarantine); err == nil || !strings.Contains(err.Error(), "no_quarantine") {
		t.Errorf("an unparseable bool should be an error, got %v", err)
	}
}
