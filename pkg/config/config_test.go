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
