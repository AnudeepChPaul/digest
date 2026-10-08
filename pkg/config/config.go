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

	"github.com/AnudeepChPaul/digest/pkg/aitool"
	"github.com/AnudeepChPaul/digest/pkg/paths"

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

type AutomationSpec struct {
	Name         string   `yaml:"name"`
	Plugins      []string `yaml:"plugins,omitempty"`
	Match        []string `yaml:"match,omitempty"`
	Prompt       Prompt   `yaml:"prompt,omitempty"`
	DraftPrompt  Prompt   `yaml:"draft_prompt,omitempty"`
	CreatePrompt Prompt   `yaml:"create_prompt,omitempty"`
	ReauthHint   string   `yaml:"reauth_hint,omitempty"`
	Command      string   `yaml:"-"`
}

const (
	SelectionNotesToday     = "notes_today"
	SelectionNotesYesterday = "notes_yesterday"
)

type Config struct {
	DigestRoot          string              `yaml:"digest_root"`
	AIToolType          string              `yaml:"ai_tool_type"`
	SelectionDefault    string              `yaml:"selection_default"`
	GitRepositoryRoots  []string            `yaml:"git_repository_roots"`
	GitAutoSyncInterval int                 `yaml:"git_auto_sync_interval"`
	JanitorPatterns     []string            `yaml:"janitor_patterns"`
	Jobs                []JobSpec           `yaml:"jobs"`
	GreenOnly           bool                `yaml:"green_only"`
	JiraBaseURL         string              `yaml:"jira_base_url"`
	PRQuantityPerRepo   int                 `yaml:"pr_quantity_per_repo"`
	ShowDailyCommits    *bool               `yaml:"show_daily_commits"`
	ShowGit             *bool               `yaml:"show_git"`
	BragPrompts         Prompt              `yaml:"brag_prompts"`
	MonthBragPrompts    Prompt              `yaml:"month_brag_prompts"`
	PerformancePrompts  Prompt              `yaml:"performance_review_prompts"`
	RetentionDays       int                 `yaml:"retention_days"`
	DigestNotifications DigestNotifications `yaml:"digest_notifications"`
	WorkDays            []string            `yaml:"work_days"`
	ShowKeyHints        bool                `yaml:"show_key_hints"`
	ShowTags            string              `yaml:"show_tags"`
	TerminalApp         string              `yaml:"terminal_app"`
	Automations         []AutomationSpec    `yaml:"automations,omitempty"`
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
func (c *Config) AutomationDir() string { return filepath.Join(c.Root(), "automations") }
func (c *Config) TUIMarkerPath() string { return filepath.Join(c.Root(), ".state", "tui.pid") }

func findAutomation(specs []AutomationSpec, name string) (AutomationSpec, bool) {
	for _, spec := range specs {
		if spec.Name == name {
			return spec, true
		}
	}
	return AutomationSpec{}, false
}

func withBuiltInDefaults(configured, builtIn AutomationSpec) AutomationSpec {
	if len(configured.Plugins) == 0 {
		configured.Plugins = builtIn.Plugins
	}
	if len(configured.Match) == 0 {
		configured.Match = builtIn.Match
	}
	if configured.Prompt == "" {
		configured.Prompt = builtIn.Prompt
	}
	if configured.DraftPrompt == "" {
		configured.DraftPrompt = builtIn.DraftPrompt
	}
	if configured.CreatePrompt == "" {
		configured.CreatePrompt = builtIn.CreatePrompt
	}
	return configured
}

func (c *Config) AutomationList() []AutomationSpec {
	builtIns := builtInAutomations()
	if c == nil || c.Automations == nil {
		return builtIns
	}
	var specs []AutomationSpec
	for _, configured := range c.Automations {
		if builtIn, found := findAutomation(builtIns, configured.Name); found {
			configured = withBuiltInDefaults(configured, builtIn)
		}
		specs = append(specs, configured)
	}
	if _, found := findAutomation(specs, AutomationPRReview); !found {
		review, _ := findAutomation(builtIns, AutomationPRReview)
		specs = append(specs, review)
	}
	return specs
}

func (c *Config) BragCommandTemplate() string {
	return DefaultBragCommand
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
	review, _ := findAutomation(c.AutomationList(), AutomationPRReview)
	if review.Command != "" {
		return review.Command
	}
	return aitool.ReviewCommand(string(review.Prompt), review.Plugins)
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

func (c *Config) GitEnabled() bool {
	return c == nil || c.ShowGit == nil || *c.ShowGit
}

func (c *Config) DailyCommitsEnabled() bool {
	return c.GitEnabled() && (c == nil || c.ShowDailyCommits == nil || *c.ShowDailyCommits)
}

var gitJobCommands = []string{"digest branch-reaper", "digest repo-sync"}

func (j JobSpec) usesGit() bool {
	for _, gitCommand := range gitJobCommands {
		if strings.Contains(j.Command, gitCommand) || strings.Contains(j.DryRunCommand, gitCommand) {
			return true
		}
	}
	return false
}

func (c *Config) JobList() []JobSpec {
	if c == nil {
		return nil
	}
	if c.GitEnabled() {
		return c.Jobs
	}
	var jobs []JobSpec
	for _, job := range c.Jobs {
		if !job.usesGit() {
			jobs = append(jobs, job)
		}
	}
	return jobs
}

func (c *Config) UnmarshalYAML(value *yaml.Node) error {
	var raw struct {
		DigestRoot          string               `yaml:"digest_root"`
		AIToolType          string               `yaml:"ai_tool_type"`
		SelectionDefault    string               `yaml:"selection_default"`
		GitRepositoryRoots  []string             `yaml:"git_repository_roots"`
		LegacyRepoRoots     []string             `yaml:"repo_roots"`
		GitAutoSyncInterval interface{}          `yaml:"git_auto_sync_interval"`
		JanitorPatterns     []string             `yaml:"janitor_patterns"`
		Jobs                []JobSpec            `yaml:"jobs"`
		GreenOnly           *bool                `yaml:"green_only"`
		JiraBaseURL         string               `yaml:"jira_base_url"`
		PRQuantityPerRepo   int                  `yaml:"pr_quantity_per_repo"`
		ShowDailyCommits    *bool                `yaml:"show_daily_commits"`
		ShowGit             *bool                `yaml:"show_git"`
		BragPrompts         Prompt               `yaml:"brag_prompts"`
		MonthBragPrompts    Prompt               `yaml:"month_brag_prompts"`
		PerformancePrompts  Prompt               `yaml:"performance_review_prompts"`
		RetentionDays       int                  `yaml:"retention_days"`
		DigestNotifications *DigestNotifications `yaml:"digest_notifications"`
		WorkDays            []string             `yaml:"work_days"`
		ShowKeyHints        bool                 `yaml:"show_key_hints"`
		ShowTags            string               `yaml:"show_tags"`
		TerminalApp         string               `yaml:"terminal_app"`
		Automations         []AutomationSpec     `yaml:"automations"`
	}

	if err := value.Decode(&raw); err != nil {
		return err
	}

	if raw.AIToolType != "" && raw.AIToolType != aitool.TypeClaude {
		return fmt.Errorf("ai_tool_type %q is not supported; use %q", raw.AIToolType, aitool.TypeClaude)
	}
	c.DigestRoot = raw.DigestRoot
	c.AIToolType = raw.AIToolType
	c.SelectionDefault = raw.SelectionDefault
	c.GitRepositoryRoots = raw.GitRepositoryRoots
	if len(c.GitRepositoryRoots) == 0 {
		c.GitRepositoryRoots = raw.LegacyRepoRoots
	}
	c.JanitorPatterns = raw.JanitorPatterns
	c.Jobs = raw.Jobs
	if c.Jobs == nil {
		c.Jobs = DefaultJobs()
	}
	c.GreenOnly = raw.GreenOnly == nil || *raw.GreenOnly
	c.JiraBaseURL = raw.JiraBaseURL
	c.PRQuantityPerRepo = raw.PRQuantityPerRepo
	c.ShowDailyCommits = raw.ShowDailyCommits
	c.ShowGit = raw.ShowGit
	c.BragPrompts = raw.BragPrompts
	c.MonthBragPrompts = raw.MonthBragPrompts
	c.PerformancePrompts = raw.PerformancePrompts
	c.RetentionDays = raw.RetentionDays
	if raw.DigestNotifications == nil {
		raw.DigestNotifications = &DigestNotifications{Morning: DefaultMorning, Evening: DefaultEvening}
	}
	for _, clock := range []string{raw.DigestNotifications.Morning, raw.DigestNotifications.Evening} {
		if _, err := ParseClock(clock); clock != "" && err != nil {
			return err
		}
	}
	for _, day := range raw.WorkDays {
		if _, known := weekdayNames[strings.ToLower(day)]; !known {
			return fmt.Errorf("work_days: %q is not a day; use mon, tue, wed, thu, fri, sat or sun", day)
		}
	}
	c.DigestNotifications = *raw.DigestNotifications
	c.WorkDays = raw.WorkDays
	if raw.ShowTags != "" && raw.ShowTags != ShowTagsSelected && raw.ShowTags != ShowTagsAlways {
		return fmt.Errorf("show_tags %q is not supported; use %q or %q", raw.ShowTags, ShowTagsSelected, ShowTagsAlways)
	}
	c.ShowKeyHints = raw.ShowKeyHints
	c.ShowTags = raw.ShowTags
	c.TerminalApp = raw.TerminalApp
	c.Automations = raw.Automations

	c.GitAutoSyncInterval = parseSyncInterval(raw.GitAutoSyncInterval)

	return nil
}

func parseSyncInterval(val interface{}) int {
	if val == nil {
		return DefaultGitAutoSyncInterval
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

	return DefaultGitAutoSyncInterval
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
	if err := writeDefaultConfig(resolved); err != nil {
		return defaultCfg, err
	}
	return defaultCfg, nil
}

func writeDefaultConfig(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), paths.PrivateDirMode); err != nil {
		return fmt.Errorf("failed to create config directory for %s: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(DefaultConfigYAML), paths.PrivateFileMode); err != nil {
		return fmt.Errorf("failed to write default config %s: %w", path, err)
	}
	return nil
}

func Export(path string) (written, backup string, err error) {
	written = resolveConfigPath(path)
	if _, statErr := os.Stat(written); statErr == nil {
		backup = written + ".bak"
		if err := os.Rename(written, backup); err != nil {
			return written, "", fmt.Errorf("back up %s: %w", written, err)
		}
	}
	return written, backup, writeDefaultConfig(written)
}

func (c *Config) TightenPermissions(configPath string) error {
	resolved := resolveConfigPath(configPath)
	targets := map[string]os.FileMode{resolved: paths.PrivateFileMode}
	for _, ownedDir := range []string{c.NotesDir(), c.ReviewRootDir(), c.BragDir(), c.CacheDir(), c.LogsDir(), c.AutomationDir(), filepath.Join(c.Root(), "notify")} {
		targets[ownedDir] = paths.PrivateDirMode
	}
	if home, err := os.UserHomeDir(); err == nil && filepath.Clean(c.Root()) != filepath.Clean(home) && strings.Contains(strings.ToLower(filepath.Base(c.Root())), "digest") {
		targets[c.Root()] = paths.PrivateDirMode
	}
	if configDir := filepath.Dir(resolved); filepath.Base(configDir) == "digest" {
		targets[configDir] = paths.PrivateDirMode
	}
	var failures []error
	for path, mode := range targets {
		info, err := os.Stat(path)
		if errors.Is(err, fs.ErrNotExist) || (err == nil && info.Mode().Perm() == mode) {
			continue
		}
		if err == nil {
			err = os.Chmod(path, mode)
		}
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func LoadOrDefault(path string) (*Config, error) {
	resolved := resolveConfigPath(path)
	cfg, err := Load(resolved)
	if err == nil {
		return cfg, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return DefaultConfig(), nil
	}
	return DefaultConfig(), fmt.Errorf("failed to load config %s, using defaults: %w", resolved, err)
}
