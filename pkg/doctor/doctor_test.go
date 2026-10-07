package doctor

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"app/pkg/config"
)

func fakeLookPath(installed ...string) func(string) (string, error) {
	return func(name string) (string, error) {
		for _, tool := range installed {
			if tool == name {
				return "/bin/" + name, nil
			}
		}
		return "", errors.New("not found")
	}
}

func resultFor(results []Result, name string) (Result, bool) {
	for _, result := range results {
		if result.Name == name {
			return result, true
		}
	}
	return Result{}, false
}

func testConfig() *config.Config {
	return &config.Config{
		Jobs:        []config.JobSpec{{Name: "janitor", Command: "digest janitor"}, {Name: "repo sync", Command: "digest repo-sync"}},
		Automations: []config.AutomationSpec{{Name: "jira", Plugins: []string{"jira-inator"}}, {Name: "google doc", Plugins: []string{"claude_ai_Google_Drive"}}},
	}
}

func TestPluginsAreReportedPerAutomation(t *testing.T) {
	previous := operatingSystem
	operatingSystem = "darwin"
	t.Cleanup(func() { operatingSystem = previous })
	results := Check(testConfig(), fakeLookPath())
	plugin, found := resultFor(results, "plugin jira-inator")
	if !found || plugin.Required || !strings.Contains(plugin.Purpose, "jira") {
		t.Errorf("jira-inator = %+v found %v", plugin, found)
	}
	if connector, _ := resultFor(results, "plugin claude_ai_Google_Drive"); !connector.Found || !strings.Contains(connector.Note, "connector") {
		t.Errorf("connector = %+v", connector)
	}
}

func TestCheckCoversBuiltInAndConfiguredTools(t *testing.T) {
	previous := operatingSystem
	operatingSystem = "darwin"
	t.Cleanup(func() { operatingSystem = previous })
	results := Check(testConfig(), fakeLookPath("git", "gh", "terminal-notifier", "pbcopy", "claude", "digest"))
	for _, name := range []string{"go", "git", "gh", "terminal-notifier", "tmux", "nvim", "pbcopy", "claude", "digest", "uv"} {
		if _, found := resultFor(results, name); !found {
			t.Errorf("missing check for %q in %+v", name, results)
		}
	}
	if git, _ := resultFor(results, "git"); !git.Found || !git.Required || git.Path != "/bin/git" {
		t.Errorf("git = %+v", git)
	}
	if uv, _ := resultFor(results, "uv"); uv.Found || !uv.Required || !strings.Contains(uv.Purpose, "jira") {
		t.Errorf("uv = %+v", uv)
	}
	if goTool, _ := resultFor(results, "go"); goTool.Required {
		t.Errorf("go is only needed to build: %+v", goTool)
	}
	if claude, _ := resultFor(results, "claude"); !claude.Found || !claude.Required || !strings.Contains(claude.Purpose, "ai_tool_type") {
		t.Errorf("claude = %+v", claude)
	}
	if !Failed(results) {
		t.Errorf("a missing required tool should fail")
	}
}

func TestCheckPassesWhenEverythingRequiredIsInstalled(t *testing.T) {
	previous := operatingSystem
	operatingSystem = "darwin"
	t.Cleanup(func() { operatingSystem = previous })
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "app", ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Automations: []config.AutomationSpec{}, GitRepositoryRoots: []string{repoRoot}}
	results := Check(cfg, fakeLookPath("git", "gh", "terminal-notifier", "pbcopy", "claude"))
	if Failed(results) {
		t.Errorf("optional tools missing should not fail: %+v", results)
	}
}

func TestGitToolsNotNeededWhenGitIsOff(t *testing.T) {
	previous := operatingSystem
	operatingSystem = "darwin"
	t.Cleanup(func() { operatingSystem = previous })
	showGit := false
	cfg := testConfig()
	cfg.ShowGit = &showGit
	results := Check(cfg, fakeLookPath("terminal-notifier", "pbcopy", "claude", "digest", "uv", "my tool"))
	for _, name := range []string{"git", "gh"} {
		if tool, _ := resultFor(results, name); tool.Required || !strings.Contains(tool.Note, "show_git") {
			t.Errorf("%s with git off = %+v", name, tool)
		}
	}
	if Failed(results) {
		t.Errorf("git off should not fail on git or gh: %+v", results)
	}
}

func TestLinuxNeedsOneClipboardTool(t *testing.T) {
	previous := operatingSystem
	operatingSystem = "linux"
	t.Cleanup(func() { operatingSystem = previous })
	results := Check(&config.Config{}, fakeLookPath("git", "gh", "xclip", "claude"))
	clipboard, found := resultFor(results, "wl-copy|xclip|xsel")
	if !found || !clipboard.Found || clipboard.Path != "/bin/xclip" {
		t.Errorf("clipboard = %+v found %v", clipboard, found)
	}
	if _, found := resultFor(results, "terminal-notifier"); found {
		t.Errorf("terminal-notifier is macOS only")
	}
}

func TestFormatMarksEachTool(t *testing.T) {
	report := Format([]Result{
		{Name: "git", Purpose: "repos", Found: true, Required: true, Path: "/bin/git"},
		{Name: "uv", Purpose: "jira", Required: true},
		{Name: "tmux", Purpose: "open clones"},
		{Name: "gh", Note: "not needed (show_git: false)"},
	})
	for _, want := range []string{"✓ git", "/bin/git", "✗ uv", "missing", "! tmux", "optional", "- gh", "show_git: false"} {
		if !strings.Contains(report, want) {
			t.Errorf("report missing %q:\n%s", want, report)
		}
	}
}

func TestCommandBinary(t *testing.T) {
	cases := map[string]string{
		`claude -p "x"`:              "claude",
		`  uv run "$(ls x)"`:         "uv",
		`FOO=1 BAR=2 digest janitor`: "digest",
		`'my tool' login`:            "my tool",
		`"/opt/bin/x" --y`:           "/opt/bin/x",
		``:                           "",
		`FOO=1`:                      "",
	}
	for command, want := range cases {
		if got := commandBinary(command); got != want {
			t.Errorf("commandBinary(%q) = %q, want %q", command, got, want)
		}
	}
}

func TestDoctorReportsGitRoots(t *testing.T) {
	cfg := testConfig()
	if roots, found := resultFor(Check(cfg, fakeLookPath()), "git_repository_roots"); !found || roots.Found || !roots.Required || !Failed([]Result{roots}) {
		t.Errorf("empty roots with git on should fail: %+v", roots)
	}
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "app", ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	cfg.GitRepositoryRoots = []string{repoRoot}
	if roots, _ := resultFor(Check(cfg, fakeLookPath()), "git_repository_roots"); !roots.Found {
		t.Errorf("roots with a repo should pass: %+v", roots)
	}
}
