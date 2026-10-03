package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"app/pkg/paths"

	"gopkg.in/yaml.v3"
)

type JobSpec struct {
	Name          string `yaml:"name"`
	DryRunCommand string `yaml:"dry-run-command"`
	Command       string `yaml:"command"`
}

func (j *JobSpec) UnmarshalYAML(value *yaml.Node) error {
	var raw struct {
		Name          string `yaml:"name"`
		DryRunCommand string `yaml:"dry-run-command"`
		DryRunCmd     string `yaml:"dry_run_command"`
		Command       string `yaml:"command"`
		Cmd           string `yaml:"cmd"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	j.Name = raw.Name
	if raw.DryRunCommand != "" {
		j.DryRunCommand = raw.DryRunCommand
	} else {
		j.DryRunCommand = raw.DryRunCmd
	}
	if raw.Command != "" {
		j.Command = raw.Command
	} else {
		j.Command = raw.Cmd
	}
	return nil
}

type Config struct {
	NotesDir             string    `yaml:"notes_dir"`
	GitRepositoryRoots   []string  `yaml:"git_repository_roots"`
	GitLookbackDays      int       `yaml:"git_lookback_days"`
	GitCommitsCmd        string    `yaml:"git_commits_cmd"`
	GitAutoSyncInterval  int       `yaml:"git_auto_sync_interval"`
	JanitorPatterns      []string  `yaml:"janitor_patterns"`
	Jobs                 []JobSpec `yaml:"jobs"`
	ReviewRoot           string    `yaml:"review_root"`
	ReviewCommand        string    `yaml:"review_command"`
	GreenOnly            bool      `yaml:"green_only"`
	JiraBaseURL          string    `yaml:"jira_base_url"`
	GHPendingPRs         string    `yaml:"gh_pending_prs"`
	GHDirectRequestedPRs string    `yaml:"gh_direct_requested_prs"`
	GHRereviewPRs        string    `yaml:"gh_rereview_prs"`
	GHReviewedPRs        string    `yaml:"gh_reviewed_prs"`
	GHPRDetails          string    `yaml:"gh_pr_details"`
}

func (c *Config) ReviewRootDir() string {
	if c.ReviewRoot == "" {
		return paths.Expand(DefaultReviewRoot)
	}
	return paths.Expand(c.ReviewRoot)
}

func (c *Config) ReviewCommandTemplate() string {
	if c.ReviewCommand == "" {
		return DefaultReviewCommand
	}
	return c.ReviewCommand
}

func stringOrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func (c *Config) PendingPRsCommand() string {
	if c == nil {
		return DefaultGHPendingPRs
	}
	return stringOrDefault(c.GHPendingPRs, DefaultGHPendingPRs)
}

func (c *Config) DirectRequestedPRsCommand() string {
	if c == nil {
		return DefaultGHDirectRequestedPRs
	}
	return stringOrDefault(c.GHDirectRequestedPRs, DefaultGHDirectRequestedPRs)
}

func (c *Config) RereviewPRsCommand() string {
	if c == nil {
		return DefaultGHRereviewPRs
	}
	return stringOrDefault(c.GHRereviewPRs, DefaultGHRereviewPRs)
}

func (c *Config) ReviewedPRsCommand() string {
	if c == nil {
		return DefaultGHReviewedPRs
	}
	return stringOrDefault(c.GHReviewedPRs, DefaultGHReviewedPRs)
}

func (c *Config) PRDetailsCommand() string {
	if c == nil {
		return DefaultGHPRDetails
	}
	return stringOrDefault(c.GHPRDetails, DefaultGHPRDetails)
}

func (c *Config) UnmarshalYAML(value *yaml.Node) error {
	var raw struct {
		NotesDir             string      `yaml:"notes_dir"`
		GitRepositoryRoots   []string    `yaml:"git_repository_roots"`
		LegacyRepoRoots      []string    `yaml:"repo_roots"`
		GitLookbackDays      int         `yaml:"git_lookback_days"`
		GitCommitsCmd        string      `yaml:"git_commits_cmd"`
		GitAutoSyncInterval  interface{} `yaml:"git_auto_sync_interval"`
		JanitorPatterns      []string    `yaml:"janitor_patterns"`
		Jobs                 []JobSpec   `yaml:"jobs"`
		ReviewRoot           string      `yaml:"review_root"`
		ReviewCommand        string      `yaml:"review_command"`
		GreenOnly            *bool       `yaml:"green_only"`
		JiraBaseURL          string      `yaml:"jira_base_url"`
		GHPendingPRs         string      `yaml:"gh_pending_prs"`
		GHDirectRequestedPRs string      `yaml:"gh_direct_requested_prs"`
		GHRereviewPRs        string      `yaml:"gh_rereview_prs"`
		GHReviewedPRs        string      `yaml:"gh_reviewed_prs"`
		GHPRDetails          string      `yaml:"gh_pr_details"`
	}

	if err := value.Decode(&raw); err != nil {
		return err
	}

	c.NotesDir = raw.NotesDir
	c.GitRepositoryRoots = raw.GitRepositoryRoots
	if len(c.GitRepositoryRoots) == 0 {
		c.GitRepositoryRoots = raw.LegacyRepoRoots
	}
	c.GitLookbackDays = raw.GitLookbackDays
	c.GitCommitsCmd = raw.GitCommitsCmd
	c.JanitorPatterns = raw.JanitorPatterns
	c.Jobs = raw.Jobs
	c.ReviewRoot = raw.ReviewRoot
	c.ReviewCommand = raw.ReviewCommand
	c.GreenOnly = raw.GreenOnly == nil || *raw.GreenOnly
	c.JiraBaseURL = raw.JiraBaseURL
	c.GHPendingPRs = raw.GHPendingPRs
	c.GHDirectRequestedPRs = raw.GHDirectRequestedPRs
	c.GHRereviewPRs = raw.GHRereviewPRs
	c.GHReviewedPRs = raw.GHReviewedPRs
	c.GHPRDetails = raw.GHPRDetails

	c.GitAutoSyncInterval = parseSyncInterval(raw.GitAutoSyncInterval)

	return nil
}

func parseSyncInterval(val interface{}) int {
	if val == nil {
		return 30
	}

	switch v := val.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		str := strings.TrimSpace(v)
		if d, err := time.ParseDuration(str); err == nil {
			return int(d.Seconds())
		}
		if i, err := strconv.Atoi(str); err == nil {
			return i
		}
	}

	return 30
}

func xdgConfigHome() string {
	if xdgHome := os.Getenv("XDG_CONFIG_HOME"); xdgHome != "" && filepath.IsAbs(xdgHome) {
		return xdgHome
	}
	return paths.Expand("~/.config")
}

func configCandidates() []string {
	base := xdgConfigHome()
	return []string{
		filepath.Join(base, "digest.yaml"),
		filepath.Join(base, "digest", "config.yaml"),
	}
}

func resolveConfigPath(path string) string {
	if path != "" {
		return paths.Expand(path)
	}

	candidates := configCandidates()
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return candidates[len(candidates)-1]
}

func Load(resolvedPath string) (*Config, error) {
	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("yaml parse error in %s: %w", resolvedPath, err)
	}

	return &cfg, nil
}

func LoadOrCreate(path string) (*Config, error) {
	resolved := resolveConfigPath(path)

	cfg, err := Load(resolved)
	if err == nil {
		return cfg, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return DefaultConfig(), fmt.Errorf("failed to load config %s, using defaults: %w", resolved, err)
	}

	defaultCfg := DefaultConfig()

	if err := os.MkdirAll(filepath.Dir(resolved), 0755); err != nil {
		return defaultCfg, fmt.Errorf("failed to create config directory for %s: %w", resolved, err)
	}

	data, err := yaml.Marshal(defaultCfg)
	if err != nil {
		return defaultCfg, fmt.Errorf("failed to encode default config: %w", err)
	}

	if err := os.WriteFile(resolved, data, 0644); err != nil {
		return defaultCfg, fmt.Errorf("failed to write default config %s: %w", resolved, err)
	}

	return defaultCfg, nil
}
