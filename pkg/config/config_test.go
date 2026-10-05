package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestReviewSettingsDefaultWhenAbsent(t *testing.T) {
	var cfg Config
	if err := yaml.Unmarshal([]byte("notes_dir: ~/n\n"), &cfg); err != nil {
		t.Fatal(err)
	}
	if !cfg.GreenOnly {
		t.Errorf("GreenOnly = false, want true by default")
	}
	if !strings.HasSuffix(cfg.ReviewRootDir(), "/digest/reviews") {
		t.Errorf("ReviewRootDir = %q", cfg.ReviewRootDir())
	}
	if !strings.Contains(cfg.ReviewCommandTemplate(), "{url}") || !strings.Contains(cfg.ReviewCommandTemplate(), "{findings}") {
		t.Errorf("ReviewCommandTemplate = %q", cfg.ReviewCommandTemplate())
	}
}

func TestReviewSettingsOverride(t *testing.T) {
	var cfg Config
	raw := "review_root: /tmp/r\nreview_command: echo {url}\ngreen_only: false\njira_base_url: https://jira/browse/\n"
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.GreenOnly {
		t.Errorf("GreenOnly = true, want false")
	}
	if cfg.ReviewRootDir() != "/tmp/r" || cfg.ReviewCommandTemplate() != "echo {url}" || cfg.JiraBaseURL != "https://jira/browse/" {
		t.Errorf("unexpected %+v", cfg)
	}
}

func TestPRsPerRepo(t *testing.T) {
	cases := map[string]int{
		"notes_dir: ~/n\n":            DefaultPRQuantityPerRepo,
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
		"notes_dir: ~/n\n":            true,
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
