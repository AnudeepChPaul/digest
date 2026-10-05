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
	DigestRoot          string    `yaml:"digest_root"`
	GitRepositoryRoots  []string  `yaml:"git_repository_roots"`
	GitLookbackDays     int       `yaml:"git_lookback_days"`
	GitCommitsCmd       string    `yaml:"git_commits_cmd"`
	GitAutoSyncInterval int       `yaml:"git_auto_sync_interval"`
	JanitorPatterns     []string  `yaml:"janitor_patterns"`
	Jobs                []JobSpec `yaml:"jobs"`
	ReviewCommand       string    `yaml:"review_command"`
	GreenOnly           bool      `yaml:"green_only"`
	JiraBaseURL         string    `yaml:"jira_base_url"`
	PRQuantityPerRepo   int       `yaml:"pr_quantity_per_repo"`
	ShowDailyCommits    *bool     `yaml:"show_daily_commits"`
	BragCommand         string    `yaml:"brag_command"`
	BragPrompts         Prompt    `yaml:"brag_prompts"`
	MonthBragPrompts    Prompt    `yaml:"month_brag_prompts"`
	PerformancePrompts  Prompt    `yaml:"performance_review_prompts"`
	RetentionDays       int       `yaml:"retention_days"`
}

type Prompt string

func (p *Prompt) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		*p = Prompt(strings.TrimSpace(value.Value))
	case yaml.SequenceNode:
		var parts []string
		if err := value.Decode(&parts); err != nil {
			return err
		}
		var kept []string
		for _, part := range parts {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				kept = append(kept, trimmed)
			}
		}
		*p = Prompt(strings.Join(kept, "\n\n"))
	default:
		return fmt.Errorf("line %d: prompt must be a string or a list of strings", value.Line)
	}
	return nil
}

type PromptKind string

const (
	PromptWeek  PromptKind = "week"
	PromptMonth PromptKind = "month"
	PromptYear  PromptKind = "year"
)

func (c *Config) Root() string {
	if c == nil || c.DigestRoot == "" {
		return paths.Expand(DefaultDigestRoot)
	}
	return paths.Expand(c.DigestRoot)
}

func (c *Config) NotesDir() string      { return filepath.Join(c.Root(), "notes") }
func (c *Config) ReviewRootDir() string { return filepath.Join(c.Root(), "reviews") }
func (c *Config) BragDir() string       { return filepath.Join(c.Root(), "brag") }
func (c *Config) CacheDir() string      { return filepath.Join(c.Root(), "cache") }
func (c *Config) LogsDir() string       { return filepath.Join(c.Root(), "logs") }
func (c *Config) QuarantineDir() string { return filepath.Join(c.Root(), ".quarantine") }

func (c *Config) BragCommandTemplate() string {
	if c == nil || c.BragCommand == "" {
		return DefaultBragCommand
	}
	return c.BragCommand
}

func (c *Config) BragPrompt(kind PromptKind) string {
	defaults := map[PromptKind]string{PromptWeek: DefaultWeekBragPrompt, PromptMonth: DefaultMonthBragPrompt, PromptYear: DefaultPerformanceReviewPrompt}
	if c != nil {
		configured := map[PromptKind]Prompt{PromptWeek: c.BragPrompts, PromptMonth: c.MonthBragPrompts, PromptYear: c.PerformancePrompts}[kind]
		if configured != "" {
			return string(configured)
		}
	}
	return defaults[kind]
}

func (c *Config) ReviewCommandTemplate() string {
	if c.ReviewCommand == "" {
		return DefaultReviewCommand
	}
	return c.ReviewCommand
}

func (c *Config) PRsPerRepo() int {
	if c == nil || c.PRQuantityPerRepo == 0 {
		return DefaultPRQuantityPerRepo
	}
	return min(max(c.PRQuantityPerRepo, 1), maxPRQuantityPerRepo)
}

func (c *Config) Retention() int {
	if c == nil || c.RetentionDays <= 0 {
		return DefaultRetentionDays
	}
	return c.RetentionDays
}

func (c *Config) DailyCommitsEnabled() bool {
	return c == nil || c.ShowDailyCommits == nil || *c.ShowDailyCommits
}

func (c *Config) UnmarshalYAML(value *yaml.Node) error {
	var raw struct {
		DigestRoot          string      `yaml:"digest_root"`
		GitRepositoryRoots  []string    `yaml:"git_repository_roots"`
		LegacyRepoRoots     []string    `yaml:"repo_roots"`
		GitLookbackDays     int         `yaml:"git_lookback_days"`
		GitCommitsCmd       string      `yaml:"git_commits_cmd"`
		GitAutoSyncInterval interface{} `yaml:"git_auto_sync_interval"`
		JanitorPatterns     []string    `yaml:"janitor_patterns"`
		Jobs                []JobSpec   `yaml:"jobs"`
		ReviewCommand       string      `yaml:"review_command"`
		GreenOnly           *bool       `yaml:"green_only"`
		JiraBaseURL         string      `yaml:"jira_base_url"`
		PRQuantityPerRepo   int         `yaml:"pr_quantity_per_repo"`
		ShowDailyCommits    *bool       `yaml:"show_daily_commits"`
		BragCommand         string      `yaml:"brag_command"`
		BragPrompts         Prompt      `yaml:"brag_prompts"`
		MonthBragPrompts    Prompt      `yaml:"month_brag_prompts"`
		PerformancePrompts  Prompt      `yaml:"performance_review_prompts"`
		RetentionDays       int         `yaml:"retention_days"`
	}

	if err := value.Decode(&raw); err != nil {
		return err
	}

	c.DigestRoot = raw.DigestRoot
	c.GitRepositoryRoots = raw.GitRepositoryRoots
	if len(c.GitRepositoryRoots) == 0 {
		c.GitRepositoryRoots = raw.LegacyRepoRoots
	}
	c.GitLookbackDays = raw.GitLookbackDays
	c.GitCommitsCmd = raw.GitCommitsCmd
	c.JanitorPatterns = raw.JanitorPatterns
	c.Jobs = raw.Jobs
	c.ReviewCommand = raw.ReviewCommand
	c.GreenOnly = raw.GreenOnly == nil || *raw.GreenOnly
	c.JiraBaseURL = raw.JiraBaseURL
	c.PRQuantityPerRepo = raw.PRQuantityPerRepo
	c.ShowDailyCommits = raw.ShowDailyCommits
	c.BragCommand = raw.BragCommand
	c.BragPrompts = raw.BragPrompts
	c.MonthBragPrompts = raw.MonthBragPrompts
	c.PerformancePrompts = raw.PerformancePrompts
	c.RetentionDays = raw.RetentionDays

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
