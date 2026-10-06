package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestReviewSettingsDefaultWhenAbsent(t *testing.T) {
	var cfg Config
	if err := yaml.Unmarshal([]byte("jira_base_url: x\n"), &cfg); err != nil {
		t.Fatal(err)
	}
	if !cfg.GreenOnly {
		t.Errorf("GreenOnly = false, want true by default")
	}
	if !strings.HasSuffix(cfg.ReviewRootDir(), "/digest/reviews") || !strings.HasSuffix(cfg.NotesDir(), "/digest/notes") {
		t.Errorf("ReviewRootDir = %q NotesDir = %q", cfg.ReviewRootDir(), cfg.NotesDir())
	}
	if !strings.Contains(cfg.ReviewCommandTemplate(), "{url}") || !strings.Contains(cfg.ReviewCommandTemplate(), "{findings}") {
		t.Errorf("ReviewCommandTemplate = %q", cfg.ReviewCommandTemplate())
	}
}

func TestReviewSettingsOverride(t *testing.T) {
	var cfg Config
	raw := "digest_root: /tmp/d\nreview_command: echo {url}\ngreen_only: false\njira_base_url: https://jira/browse/\n"
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.GreenOnly {
		t.Errorf("GreenOnly = true, want false")
	}
	if cfg.ReviewRootDir() != "/tmp/d/reviews" || cfg.ReviewCommandTemplate() != "echo {url}" || cfg.JiraBaseURL != "https://jira/browse/" {
		t.Errorf("unexpected %+v", cfg)
	}
}

func TestPRsPerRepo(t *testing.T) {
	cases := map[string]int{
		"jira_base_url: x\n":          DefaultPRQuantityPerRepo,
		"pr_quantity_per_repo: 20\n":  20,
		"pr_quantity_per_repo: 0\n":   DefaultPRQuantityPerRepo,
		"pr_quantity_per_repo: -3\n":  1,
		"pr_quantity_per_repo: 500\n": 100,
	}
	for raw, want := range cases {
		var cfg Config
		if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
			t.Fatal(err)
		}
		if got := cfg.PRsPerRepo(); got != want {
			t.Errorf("%q: PRsPerRepo = %d, want %d", raw, got, want)
		}
	}
	if DefaultPRQuantityPerRepo != 15 || DefaultConfig().PRsPerRepo() != 15 {
		t.Errorf("default per-repo quantity should be 15")
	}
	var nilConfig *Config
	if nilConfig.PRsPerRepo() != DefaultPRQuantityPerRepo {
		t.Errorf("nil config should use default")
	}
}

func TestDailyCommitsEnabled(t *testing.T) {
	cases := map[string]bool{
		"jira_base_url: x\n":          true,
		"show_daily_commits: true\n":  true,
		"show_daily_commits: false\n": false,
	}
	for raw, want := range cases {
		var cfg Config
		if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
			t.Fatal(err)
		}
		if got := cfg.DailyCommitsEnabled(); got != want {
			t.Errorf("%q: DailyCommitsEnabled = %v, want %v", raw, got, want)
		}
	}
	var nilConfig *Config
	if !nilConfig.DailyCommitsEnabled() || !(&Config{}).DailyCommitsEnabled() || !DefaultConfig().DailyCommitsEnabled() {
		t.Errorf("daily commits should default to enabled")
	}
}

func TestDigestRootDerivesEveryDirectory(t *testing.T) {
	cfg := &Config{DigestRoot: "/tmp/d"}
	got := map[string]string{
		"root":       cfg.Root(),
		"notes":      cfg.NotesDir(),
		"reviews":    cfg.ReviewRootDir(),
		"brag":       cfg.BragDir(),
		"cache":      cfg.CacheDir(),
		"logs":       cfg.LogsDir(),
		"quarantine": cfg.QuarantineDir(),
	}
	want := map[string]string{
		"root":       "/tmp/d",
		"notes":      "/tmp/d/notes",
		"reviews":    "/tmp/d/reviews",
		"brag":       "/tmp/d/brag",
		"cache":      "/tmp/d/cache",
		"logs":       "/tmp/d/logs",
		"quarantine": "/tmp/d/.quarantine",
	}
	for name, path := range want {
		if got[name] != path {
			t.Errorf("%s = %q, want %q", name, got[name], path)
		}
	}
	var nilConfig *Config
	if !strings.HasSuffix(nilConfig.Root(), "/digest") || DefaultConfig().DigestRoot != DefaultDigestRoot {
		t.Errorf("default root = %q", nilConfig.Root())
	}
}

func TestBragCommandDefault(t *testing.T) {
	if cmd := (&Config{}).BragCommandTemplate(); !strings.Contains(cmd, "claude -p") {
		t.Errorf("default brag command = %q", cmd)
	}
	if cmd := (&Config{BragCommand: "cat"}).BragCommandTemplate(); cmd != "cat" {
		t.Errorf("override = %q", cmd)
	}
}

func TestBragPromptsAcceptStringOrList(t *testing.T) {
	var cfg Config
	input := "brag_prompts: |\n  Week prompt line one.\n  Line two.\nmonth_brag_prompts:\n  - First part.\n  - |\n    Second part\n    spans lines.\n"
	if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatal(err)
	}
	if got := cfg.BragPrompt(PromptWeek); got != "Week prompt line one.\nLine two." {
		t.Errorf("week prompt = %q", got)
	}
	if got := cfg.BragPrompt(PromptMonth); got != "First part.\n\nSecond part\nspans lines." {
		t.Errorf("month prompt = %q", got)
	}
	if got := cfg.BragPrompt(PromptYear); got != DefaultPerformanceReviewPrompt {
		t.Errorf("year prompt should default, got %q", got)
	}
	if err := yaml.Unmarshal([]byte("brag_prompts:\n  key: value\n"), &cfg); err == nil {
		t.Errorf("mapping prompt should be rejected")
	}
}

func TestBragDefaultsAreIsolatedClaude(t *testing.T) {
	var nilConfig *Config
	command := nilConfig.BragCommandTemplate()
	for _, flag := range []string{"claude -p", `--tools ""`, `--setting-sources ""`, "--strict-mcp-config", "--permission-prompts none", "--disable-slash-commands"} {
		if !strings.Contains(command, flag) {
			t.Errorf("default command %q missing %s", command, flag)
		}
	}
	for _, kind := range []PromptKind{PromptWeek, PromptMonth, PromptYear} {
		if strings.TrimSpace(nilConfig.BragPrompt(kind)) == "" {
			t.Errorf("default prompt for %s is empty", kind)
		}
	}
	if !strings.Contains(DefaultPerformanceReviewPrompt, "## Key accomplishments") {
		t.Errorf("performance review prompt missing sections")
	}
}

func TestRetentionDays(t *testing.T) {
	cases := map[string]int{
		"jira_base_url: x\n":  DefaultRetentionDays,
		"retention_days: 3\n": 3,
		"retention_days: 0\n": DefaultRetentionDays,
	}
	for raw, want := range cases {
		var cfg Config
		if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
			t.Fatal(err)
		}
		if got := cfg.Retention(); got != want {
			t.Errorf("%q: Retention = %d, want %d", raw, got, want)
		}
	}
	var nilConfig *Config
	if nilConfig.Retention() != DefaultRetentionDays {
		t.Errorf("nil config should use default")
	}
}
