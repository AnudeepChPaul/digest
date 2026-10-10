package aitool

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/achandrapaul/digest/pkg/system"
)

const (
	TypeClaude        = "claude"
	connectorPrefix   = "claude_ai_"
	automationModel   = "sonnet"
	tokenCheckScript  = "validate_token.py"
	pluginSkillRunner = "run-skill.py"
)

var pluginCacheRoot = func() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "plugins", "cache")
}

func IsConnector(name string) bool {
	return strings.HasPrefix(name, connectorPrefix)
}

func versionParts(version string) []int {
	var parts []int
	for _, field := range strings.Split(version, ".") {
		number, _ := strconv.Atoi(field)
		parts = append(parts, number)
	}
	return parts
}

func PluginDir(name string) (string, bool) {
	versionDirs, _ := system.Glob(filepath.Join(pluginCacheRoot(), "*", name, "*"))
	var installed []string
	for _, dir := range versionDirs {
		if info, err := system.Stat(dir); err == nil && info.IsDir() {
			installed = append(installed, dir)
		}
	}
	if len(installed) == 0 {
		return "", false
	}
	slices.SortFunc(installed, func(left, right string) int {
		return slices.Compare(versionParts(filepath.Base(left)), versionParts(filepath.Base(right)))
	})
	return installed[len(installed)-1], true
}

func Available(name string) bool {
	if IsConnector(name) {
		return true
	}
	_, installed := PluginDir(name)
	return installed
}

func AllowedTools(plugins []string) []string {
	var tools []string
	for _, plugin := range plugins {
		switch {
		case IsConnector(plugin):
			tools = append(tools, "mcp__"+plugin+"__*")
		case Available(plugin):
			tools = append(tools, "Skill("+plugin+":*)", "mcp__plugin_"+plugin+"_*")
		}
	}
	return tools
}

func shellQuote(text string) string {
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}

func allowedToolsFlag(plugins []string) string {
	tools := AllowedTools(plugins)
	if len(tools) == 0 {
		return ""
	}
	quoted := make([]string, len(tools))
	for index, tool := range tools {
		quoted[index] = shellQuote(tool)
	}
	return " --allowedTools " + strings.Join(quoted, " ")
}

func AutomationCommand(plugins []string) string {
	return "claude -p --model " + automationModel + " --permission-prompts none" + allowedToolsFlag(plugins)
}

func ReviewCommand(prompt string, plugins []string) string {
	isolation := " --setting-sources user --strict-mcp-config"
	if slices.ContainsFunc(plugins, IsConnector) {
		isolation = " --setting-sources user"
	}
	return "claude -p " + shellQuote(prompt) + isolation + " --permission-prompts none" + allowedToolsFlag(plugins)
}

type TokenCheck struct {
	Plugin     string
	Command    string
	ReauthHint string
}

func TokenChecks(plugins []string) []TokenCheck {
	var checks []TokenCheck
	for _, plugin := range plugins {
		dir, installed := PluginDir(plugin)
		if !installed || IsConnector(plugin) {
			continue
		}
		if !system.Exists(filepath.Join(dir, "scripts", tokenCheckScript)) {
			continue
		}
		runner := `uv run "` + filepath.Join(dir, "scripts", pluginSkillRunner) + `"`
		checks = append(checks, TokenCheck{Plugin: plugin, Command: runner + " validate_token", ReauthHint: reauthHint(plugin, runner)})
	}
	return checks
}

var atlassianSecrets = map[string]string{"jira-inator": "JIRA_API_TOKEN", "confluence-inator": "CONFLUENCE_API_TOKEN"}

func reauthHint(plugin, runner string) string {
	if secret, atlassian := atlassianSecrets[plugin]; atlassian {
		return "The " + plugin + " token check failed (expired token or no connection). Create a token at https://id.atlassian.com/manage-profile/security/api-tokens, then run: " + runner + " manage_secrets store claude-marketplace/atlassian " + secret
	}
	return "The " + plugin + " token check failed. Run: " + runner + " check_setup"
}
