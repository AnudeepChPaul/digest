package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"app/pkg/paths"

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
	raw := "digest_root: /tmp/d\ngreen_only: false\njira_base_url: https://jira/browse/\nautomations:\n  - name: pr review\n    prompt: /check {url}\n"
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.GreenOnly {
		t.Errorf("GreenOnly = true, want false")
	}
	if cfg.ReviewRootDir() != "/tmp/d/reviews" || !strings.HasPrefix(cfg.ReviewCommandTemplate(), "claude -p '/check {url}'") || cfg.JiraBaseURL != "https://jira/browse/" {
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

func TestBuiltInAutomationsByDefault(t *testing.T) {
	var cfg Config
	if err := yaml.Unmarshal([]byte("jira_base_url: x\n"), &cfg); err != nil {
		t.Fatal(err)
	}
	var nilConfig *Config
	for _, automations := range [][]AutomationSpec{cfg.AutomationList(), nilConfig.AutomationList()} {
		var names []string
		for _, spec := range automations {
			names = append(names, spec.Name)
			if spec.Name != AutomationPRReview && (spec.DraftPrompt == "" || spec.CreatePrompt == "" || len(spec.Match) == 0) {
				t.Errorf("%s should have default prompts and match phrases: %+v", spec.Name, spec)
			}
		}
		if strings.Join(names, ",") != "jira,confluence,google doc,pr review" {
			t.Errorf("automations = %v", names)
		}
	}
}

func TestConfiguredAutomationsFillEmptyFieldsFromBuiltIns(t *testing.T) {
	var cfg Config
	raw := "automations:\n  - name: jira\n    draft_prompt: \"\"\n    match: [\"make a jira\"]\n  - name: DOCS\n    plugins: [docs-plugin]\n    match: [\"create docs\"]\n    draft_prompt:\n      - one\n      - two\n    create_prompt: make it\n    reauth_command: open https://example.com\n"
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	automations := cfg.AutomationList()
	if len(automations) != 3 || automations[2].Name != AutomationPRReview {
		t.Fatalf("automations = %+v, want jira, DOCS and the built-in pr review", automations)
	}
	jira := automations[0]
	if !strings.Contains(string(jira.DraftPrompt), "jira-ticket-creator") || strings.Join(jira.Plugins, ",") != "jira-inator" || strings.Join(jira.Match, ",") != "make a jira" {
		t.Errorf("jira = %+v", jira)
	}
	docs := automations[1]
	if docs.DraftPrompt != "one\n\ntwo" || strings.Join(docs.Plugins, ",") != "docs-plugin" {
		t.Errorf("docs = %+v", docs)
	}
	if err := yaml.Unmarshal([]byte("automations: []\n"), &cfg); err != nil {
		t.Fatal(err)
	}
	if got := cfg.AutomationList(); len(got) != 1 || got[0].Name != AutomationPRReview {
		t.Errorf("an empty list keeps only pr review, got %+v", got)
	}
}

func TestAIToolTypeOnlyAcceptsClaude(t *testing.T) {
	var cfg Config
	if err := yaml.Unmarshal([]byte("ai_tool_type: claude\n"), &cfg); err != nil {
		t.Errorf("claude should be accepted: %v", err)
	}
	if err := yaml.Unmarshal([]byte("ai_tool_type: codex\n"), &cfg); err == nil {
		t.Errorf("only claude is supported")
	}
}

func TestDefaultTemplateMatchesDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.AIToolType != "claude" || cfg.SelectionDefault != SelectionNotesToday || !cfg.GitEnabled() || cfg.Root() == "" || cfg.PRsPerRepo() != DefaultPRQuantityPerRepo || cfg.GitAutoSyncInterval != 600 || len(cfg.Jobs) != 2 {
		t.Errorf("default config = %+v", cfg)
	}
	if cfg.BragPrompt(PromptWeek) != DefaultWeekBragPrompt || !strings.Contains(cfg.ReviewCommandTemplate(), DefaultReviewPrompt) {
		t.Errorf("empty prompts should fall back to the built-in defaults")
	}
	for _, removed := range []string{"git_commits_cmd", "git_lookback_days", "review_command", "brag_command", "command: claude", "check_command"} {
		if strings.Contains(DefaultConfigYAML, removed) {
			t.Errorf("template should not expose %q", removed)
		}
	}
	if !strings.Contains(DefaultConfigYAML, "# claude is the only supported tool") {
		t.Errorf("template should keep its comments")
	}
}

func TestOldCommandKeysAreIgnored(t *testing.T) {
	var cfg Config
	raw := "review_command: echo {url}\nbrag_command: cat\ngit_commits_cmd: git log\ngit_lookback_days: 3\n"
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("old keys should still load: %v", err)
	}
	if strings.Contains(cfg.ReviewCommandTemplate(), "echo") || cfg.BragCommandTemplate() != DefaultBragCommand {
		t.Errorf("old command keys should be ignored")
	}
}

func TestExportWritesTheTemplateAndBacksUpTheExistingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "digest", "config.yaml")
	written, backup, err := Export(path)
	if err != nil || written != path || backup != "" {
		t.Fatalf("first export = %q %q %v", written, backup, err)
	}
	if err := os.WriteFile(path, []byte("digest_root: /mine\n"), 0644); err != nil {
		t.Fatal(err)
	}
	written, backup, err = Export(path)
	if err != nil || backup != path+".bak" {
		t.Fatalf("second export = %q %q %v", written, backup, err)
	}
	if saved, _ := os.ReadFile(backup); string(saved) != "digest_root: /mine\n" {
		t.Errorf("backup = %q", saved)
	}
	if exported, _ := os.ReadFile(path); string(exported) != DefaultConfigYAML {
		t.Errorf("exported file should be the template")
	}
}

func TestAutomationDir(t *testing.T) {
	cfg := Config{DigestRoot: "/tmp/d"}
	if cfg.AutomationDir() != "/tmp/d/automations" {
		t.Errorf("AutomationDir = %q", cfg.AutomationDir())
	}
}

func TestShowGitDefaultsOnAndOverridesDailyCommits(t *testing.T) {
	var nilConfig *Config
	if !nilConfig.GitEnabled() || !(&Config{}).GitEnabled() || !DefaultConfig().GitEnabled() {
		t.Errorf("git should be enabled by default")
	}
	var cfg Config
	if err := yaml.Unmarshal([]byte("show_git: false\nshow_daily_commits: true\n"), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.GitEnabled() || cfg.DailyCommitsEnabled() {
		t.Errorf("show_git false: GitEnabled = %v DailyCommitsEnabled = %v, want both false", cfg.GitEnabled(), cfg.DailyCommitsEnabled())
	}
	if DefaultConfig().ShowGit == nil || !*DefaultConfig().ShowGit {
		t.Errorf("default config should write show_git: true")
	}
}

func TestJobListHidesGitJobsWhenGitIsOff(t *testing.T) {
	raw := `jobs:
  - name: branch reaper
    command: digest branch-reaper --root ~/Projects/
  - name: janitor
    command: digest janitor --root ~
  - name: repo sync
    dry-run-command: digest repo-sync --dry-run --root ~/Projects/
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	if got := len(cfg.JobList()); got != 3 {
		t.Errorf("git on: %d jobs, want 3", got)
	}
	if err := yaml.Unmarshal([]byte("show_git: false\n"+raw), &cfg); err != nil {
		t.Fatal(err)
	}
	jobs := cfg.JobList()
	if len(jobs) != 1 || jobs[0].Name != "janitor" {
		t.Errorf("git off: jobs = %+v, want only janitor", jobs)
	}
}

func TestHabitSettingsParseAndDefault(t *testing.T) {
	var cfg Config
	raw := "digest_notifications:\n  morning: \"08:45\"\n  evening: \"\"\nwork_days: [sun, mon, tue, wed, thu]\nshow_key_hints: true\nterminal_app: com.example.term\n"
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.DigestNotifications.Morning != "08:45" || cfg.DigestNotifications.Evening != "" || !cfg.ShowKeyHints || cfg.TerminalApp != "com.example.term" {
		t.Errorf("cfg = %+v", cfg)
	}
	if !cfg.IsWorkDay(time.Sunday) || cfg.IsWorkDay(time.Friday) {
		t.Errorf("work days = %v", cfg.WorkDays)
	}
	var empty Config
	if !empty.IsWorkDay(time.Monday) || !empty.IsWorkDay(time.Friday) || empty.IsWorkDay(time.Saturday) || empty.ShowKeyHints {
		t.Errorf("defaults should be mon-fri and hints off")
	}
	if err := yaml.Unmarshal([]byte("work_days: [funday]\n"), &cfg); err == nil {
		t.Errorf("unknown work day should be rejected")
	}
	if err := yaml.Unmarshal([]byte("digest_notifications:\n  morning: \"25:00\"\n"), &cfg); err == nil {
		t.Errorf("invalid time should be rejected")
	}
	defaults := DefaultConfig()
	if defaults.DigestNotifications.Morning != "09:30" || defaults.DigestNotifications.Evening != "18:00" || len(defaults.WorkDays) != 5 || defaults.ShowKeyHints {
		t.Errorf("template defaults = %+v", defaults)
	}
}

func TestExistsReportsAConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if Exists(path) {
		t.Errorf("missing file should not exist")
	}
	if err := os.WriteFile(path, []byte("digest_root: /x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if !Exists(path) {
		t.Errorf("written file should exist")
	}
}

func TestSetupAnswersRenderIntoTheTemplate(t *testing.T) {
	text := RenderConfig(DefaultConfigYAML, SetupAnswers{
		ShowGit: false, RepositoryRoots: []string{"~/code", "~/work"},
		WorkDays: []string{"sun", "mon"}, Morning: "08:00", Evening: "",
		ShowKeyHints: true, TerminalApp: "com.mitchellh.ghostty",
	})
	var cfg Config
	if err := yaml.Unmarshal([]byte(text), &cfg); err != nil {
		t.Fatalf("rendered config does not parse: %v\n%s", err, text)
	}
	if cfg.GitEnabled() || strings.Join(cfg.GitRepositoryRoots, ",") != "~/code,~/work" || strings.Join(cfg.WorkDays, ",") != "sun,mon" ||
		cfg.DigestNotifications.Morning != "08:00" || cfg.DigestNotifications.Evening != "" || !cfg.ShowKeyHints || cfg.TerminalApp != "com.mitchellh.ghostty" {
		t.Errorf("rendered = %+v", cfg)
	}
	if !strings.Contains(text, "# claude is the only supported tool") || len(cfg.AutomationList()) != 4 {
		t.Errorf("render should keep the rest of the template")
	}
	custom := "digest_root: /mine\nshow_git: true\ngit_repository_roots:\n  - ~/old\njobs: []\n"
	merged := RenderConfig(custom, SetupAnswers{ShowGit: true, RepositoryRoots: []string{"~/new"}, WorkDays: []string{"mon"}, Morning: "09:00", Evening: "17:00"})
	if !strings.Contains(merged, "digest_root: /mine") || !strings.Contains(merged, "jobs: []") || strings.Contains(merged, "~/old") || !strings.Contains(merged, "  - ~/new") || !strings.Contains(merged, `evening: "17:00"`) {
		t.Errorf("render should keep other keys and replace setup keys:\n%s", merged)
	}
}

func TestDefaultAnswersRenderTheTemplateUnchanged(t *testing.T) {
	answers := SetupAnswers{ShowGit: true, WorkDays: DefaultWorkDays, Morning: "09:30", Evening: "18:00"}
	if rendered := RenderConfig(DefaultConfigYAML, answers); rendered != DefaultConfigYAML {
		t.Errorf("default answers should reproduce the template:\n%s", rendered)
	}
}

func TestShowTagsDefaultsToSelectedAndRejectsUnknown(t *testing.T) {
	var empty Config
	if empty.ShowAllTags() {
		t.Errorf("unset show_tags should only show tags on selection")
	}
	var always Config
	if err := yaml.Unmarshal([]byte("show_tags: always\n"), &always); err != nil || !always.ShowAllTags() {
		t.Errorf("show_tags always: err %v, all %v", err, always.ShowAllTags())
	}
	var selected Config
	if err := yaml.Unmarshal([]byte("show_tags: selected\n"), &selected); err != nil || selected.ShowAllTags() {
		t.Errorf("show_tags selected: err %v, all %v", err, selected.ShowAllTags())
	}
	if err := yaml.Unmarshal([]byte("show_tags: sometimes\n"), &selected); err == nil {
		t.Errorf("unknown show_tags should be rejected")
	}
	if defaults := DefaultConfig(); defaults.ShowTags != ShowTagsSelected {
		t.Errorf("template show_tags = %q", defaults.ShowTags)
	}
}

func TestMissingKeysUseBuiltInDefaults(t *testing.T) {
	var cfg Config
	if err := yaml.Unmarshal([]byte("digest_root: ~/digest\n"), &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Jobs) != 2 || cfg.Jobs[0].Name != "branch-reaper" || cfg.Jobs[1].Name != "janitor" || cfg.Jobs[1].Command != "digest janitor --root ~" {
		t.Errorf("jobs = %+v", cfg.Jobs)
	}
	if cfg.GitAutoSyncInterval != 600 {
		t.Errorf("interval = %d, want 600", cfg.GitAutoSyncInterval)
	}
	if cfg.DigestNotifications.Morning != "09:30" || cfg.DigestNotifications.Evening != "18:00" {
		t.Errorf("summaries = %+v", cfg.DigestNotifications)
	}
	var explicit Config
	raw := "jobs: []\ngit_auto_sync_interval: 120\ndigest_notifications:\n  morning: \"\"\n  evening: \"19:00\"\n"
	if err := yaml.Unmarshal([]byte(raw), &explicit); err != nil {
		t.Fatal(err)
	}
	if len(explicit.Jobs) != 0 || explicit.GitAutoSyncInterval != 120 || explicit.DigestNotifications.Morning != "" || explicit.DigestNotifications.Evening != "19:00" {
		t.Errorf("explicit values should be kept: %+v", explicit)
	}
	if defaults := DefaultConfig(); len(defaults.GitRepositoryRoots) != 0 {
		t.Errorf("template should ship without repo roots: %v", defaults.GitRepositoryRoots)
	}
}

func TestAutomationsNoLongerReadCheckCommands(t *testing.T) {
	var cfg Config
	raw := "automations:\n  - name: jira\n    check_command: echo ok\n    reauth_command: echo re\n"
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	encoded, _ := yaml.Marshal(cfg.AutomationList()[0])
	if strings.Contains(string(encoded), "check_command") || strings.Contains(string(encoded), "echo ok") {
		t.Errorf("check commands should be ignored:\n%s", encoded)
	}
}

func TestDefaultConfigIsOwnerOnly(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "digest", "config.yaml")
	if _, err := LoadOrCreate(configPath); err != nil {
		t.Fatal(err)
	}
	fileInfo, fileErr := os.Stat(configPath)
	dirInfo, dirErr := os.Stat(filepath.Dir(configPath))
	if fileErr != nil || dirErr != nil || fileInfo.Mode().Perm() != paths.PrivateFileMode || dirInfo.Mode().Perm() != paths.PrivateDirMode {
		t.Errorf("config modes = %v %v (%v %v)", fileInfo, dirInfo, fileErr, dirErr)
	}
}

func TestTightenPermissionsFixesOlderLooseInstalls(t *testing.T) {
	base := t.TempDir()
	digestRoot := filepath.Join(base, "digest")
	configPath := filepath.Join(base, "dotconfig", "digest", "config.yaml")
	if err := os.MkdirAll(filepath.Join(digestRoot, "notes"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("digest_root: "+digestRoot+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := &Config{DigestRoot: digestRoot}
	if err := cfg.TightenPermissions(configPath); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]os.FileMode{digestRoot: paths.PrivateDirMode, filepath.Dir(configPath): paths.PrivateDirMode, configPath: paths.PrivateFileMode} {
		if info, err := os.Stat(path); err != nil || info.Mode().Perm() != want {
			t.Errorf("%s mode = %v %v, want %v", path, info, err, want)
		}
	}
	loosePath := filepath.Join(base, "loose.yaml")
	if err := os.Chmod(base, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(loosePath, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := cfg.TightenPermissions(loosePath); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(base); err != nil || info.Mode().Perm() != 0755 {
		t.Errorf("a config outside a digest folder must not lock its parent: %v %v", info, err)
	}
	missing := &Config{DigestRoot: filepath.Join(base, "absent")}
	if err := missing.TightenPermissions(filepath.Join(base, "absent.yaml")); err != nil {
		t.Errorf("missing paths should be skipped: %v", err)
	}
}

func TestTightenPermissionsLeavesASharedRootAlone(t *testing.T) {
	sharedRoot := filepath.Join(t.TempDir(), "Documents")
	if err := os.MkdirAll(filepath.Join(sharedRoot, "notes"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sharedRoot, 0755); err != nil {
		t.Fatal(err)
	}
	cfg := &Config{DigestRoot: sharedRoot}
	if err := cfg.TightenPermissions(filepath.Join(t.TempDir(), "config.yaml")); err != nil {
		t.Fatal(err)
	}
	rootInfo, rootErr := os.Stat(sharedRoot)
	notesInfo, notesErr := os.Stat(cfg.NotesDir())
	if rootErr != nil || notesErr != nil || rootInfo.Mode().Perm() != 0755 || notesInfo.Mode().Perm() != paths.PrivateDirMode {
		t.Errorf("shared root %v, notes %v (%v %v)", rootInfo, notesInfo, rootErr, notesErr)
	}
}
