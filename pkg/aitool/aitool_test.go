package aitool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakePluginCache(t *testing.T, plugins map[string][]string) {
	t.Helper()
	cacheRoot := t.TempDir()
	for plugin, versions := range plugins {
		for _, version := range versions {
			scripts := filepath.Join(cacheRoot, "acme", plugin, version, "scripts")
			if err := os.MkdirAll(scripts, 0755); err != nil {
				t.Fatal(err)
			}
			for _, script := range []string{"run-skill.py", "validate_token.py"} {
				if err := os.WriteFile(filepath.Join(scripts, script), nil, 0644); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	previous := pluginCacheRoot
	pluginCacheRoot = func() string { return cacheRoot }
	t.Cleanup(func() { pluginCacheRoot = previous })
}

func TestPluginDirPicksTheNewestVersion(t *testing.T) {
	fakePluginCache(t, map[string][]string{"jira-inator": {"0.9.2", "0.11.13", "0.10.2"}})
	dir, found := PluginDir("jira-inator")
	if !found || !strings.HasSuffix(dir, filepath.Join("jira-inator", "0.11.13")) {
		t.Errorf("PluginDir = %q, %v", dir, found)
	}
	if _, found := PluginDir("missing"); found {
		t.Errorf("a plugin that is not installed should not be found")
	}
}

func TestAllowedToolsCoverOnlyInstalledPluginsAndConnectors(t *testing.T) {
	fakePluginCache(t, map[string][]string{"jira-inator": {"1.0.0"}})
	got := strings.Join(AllowedTools([]string{"jira-inator", "confluence-inator", "claude_ai_Google_Drive"}), " ")
	want := "Skill(jira-inator:*) mcp__plugin_jira-inator_* mcp__claude_ai_Google_Drive__*"
	if got != want {
		t.Errorf("AllowedTools = %q, want %q", got, want)
	}
}

func TestAutomationCommandAllowsOnlyItsPlugins(t *testing.T) {
	fakePluginCache(t, map[string][]string{"jira-inator": {"1.0.0"}})
	command := AutomationCommand([]string{"jira-inator"})
	for _, want := range []string{"claude -p", "--model sonnet", "--permission-prompts none", "--allowedTools 'Skill(jira-inator:*)' 'mcp__plugin_jira-inator_*'"} {
		if !strings.Contains(command, want) {
			t.Errorf("command %q missing %q", command, want)
		}
	}
	if strings.Contains(command, "permission-mode") {
		t.Errorf("auto mode should be gone: %q", command)
	}
	if bare := AutomationCommand(nil); strings.Contains(bare, "--allowedTools") {
		t.Errorf("no plugins should allow nothing: %q", bare)
	}
}

func TestReviewCommandPutsThePromptRightAfterP(t *testing.T) {
	fakePluginCache(t, map[string][]string{"review-toolkit": {"0.18.0"}})
	command := ReviewCommand("/review-toolkit:review {url} --emit-to {findings}", []string{"review-toolkit"})
	if !strings.HasPrefix(command, "claude -p '/review-toolkit:review {url} --emit-to {findings}' ") || !strings.HasSuffix(command, "--allowedTools 'Skill(review-toolkit:*)' 'mcp__plugin_review-toolkit_*'") {
		t.Errorf("review command = %q", command)
	}
	if !strings.Contains(command, " --setting-sources user ") {
		t.Errorf("review command should skip the clone's project settings: %q", command)
	}
	if !strings.Contains(command, " --strict-mcp-config ") {
		t.Errorf("review command should load no MCP servers: %q", command)
	}
	if connector := ReviewCommand("/check {url}", []string{"claude_ai_Slack"}); strings.Contains(connector, "--strict-mcp-config") {
		t.Errorf("a connector plugin needs its MCP server: %q", connector)
	}
	if quoted := ReviewCommand("it's", nil); !strings.HasPrefix(quoted, `claude -p 'it'\''s'`) {
		t.Errorf("single quotes should be escaped: %q", quoted)
	}
}

func TestTokenCheckUsesInstalledPluginsOnly(t *testing.T) {
	fakePluginCache(t, map[string][]string{"jira-inator": {"0.11.13"}})
	checks := TokenChecks([]string{"jira-inator", "confluence-inator", "claude_ai_Google_Drive"})
	if len(checks) != 1 || checks[0].Plugin != "jira-inator" || !strings.Contains(checks[0].Command, `uv run "`) || !strings.HasSuffix(checks[0].Command, `run-skill.py" validate_token`) {
		t.Fatalf("checks = %+v", checks)
	}
	if !strings.Contains(checks[0].ReauthHint, "api-tokens") || !strings.Contains(checks[0].ReauthHint, "JIRA_API_TOKEN") {
		t.Errorf("jira hint = %q", checks[0].ReauthHint)
	}
}
