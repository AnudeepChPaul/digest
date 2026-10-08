package doctor

import (
	"fmt"
	"runtime"
	"slices"
	"strings"

	"github.com/AnudeepChPaul/digest/pkg/aitool"
	"github.com/AnudeepChPaul/digest/pkg/config"
	"github.com/AnudeepChPaul/digest/pkg/jobs"
)

type Result struct {
	Name     string
	Purpose  string
	Path     string
	Found    bool
	Required bool
	Note     string
}

var operatingSystem = runtime.GOOS

type tool struct {
	name     string
	purpose  string
	required bool
	gitOnly  bool
}

func builtInTools() []tool {
	tools := []tool{
		{name: "go", purpose: "build digest"},
		{name: "git", purpose: "repo sync, branch reaper, commits", required: true, gitOnly: true},
		{name: "gh", purpose: "pull requests and reviews", required: true, gitOnly: true},
		{name: "tmux", purpose: "open review clones in a window"},
		{name: "nvim", purpose: "edit review clones"},
	}
	if operatingSystem == "darwin" {
		tools = append(tools, tool{name: "terminal-notifier", purpose: "notifications", required: true}, tool{name: "pbcopy", purpose: "copy to clipboard", required: true})
	}
	return tools
}

var linuxClipboardTools = []string{"wl-copy", "xclip", "xsel"}

func Check(cfg *config.Config, lookPath func(string) (string, error)) []Result {
	gitEnabled := cfg.GitEnabled()
	var results []Result
	for _, builtIn := range builtInTools() {
		if builtIn.gitOnly && !gitEnabled {
			results = append(results, Result{Name: builtIn.name, Purpose: builtIn.purpose, Note: "not needed (show_git: false)"})
			continue
		}
		path, err := lookPath(builtIn.name)
		results = append(results, Result{Name: builtIn.name, Purpose: builtIn.purpose, Path: path, Found: err == nil, Required: builtIn.required})
	}
	if gitEnabled {
		roots := Result{Name: "git_repository_roots", Purpose: "folders holding your git repos", Required: true, Found: true, Path: strings.Join(cfg.GitRepositoryRoots, ", ")}
		if err := jobs.CheckGitRoots(true, cfg.GitRepositoryRoots); err != nil {
			roots.Found, roots.Purpose = false, err.Error()
		}
		results = append(results, roots)
	}
	if operatingSystem == "linux" {
		clipboard := Result{Name: strings.Join(linuxClipboardTools, "|"), Purpose: "copy to clipboard", Required: true}
		for _, candidate := range linuxClipboardTools {
			if path, err := lookPath(candidate); err == nil {
				clipboard.Path, clipboard.Found = path, true
				break
			}
		}
		results = append(results, clipboard)
	}
	results = append(results, configuredResults(cfg, lookPath, results)...)
	return append(results, pluginResults(cfg)...)
}

func configuredCommands(cfg *config.Config) [][2]string {
	commands := [][2]string{{"ai_tool_type " + aitool.TypeClaude, aitool.TypeClaude}}
	for _, job := range cfg.JobList() {
		commands = append(commands, [2]string{"job " + job.Name, job.Command}, [2]string{"job " + job.Name, job.DryRunCommand})
	}
	for _, automation := range cfg.AutomationList() {
		for _, check := range aitool.TokenChecks(automation.Plugins) {
			commands = append(commands, [2]string{check.Plugin + " token check", check.Command})
		}
	}
	return commands
}

func pluginResults(cfg *config.Config) []Result {
	var results []Result
	indexByPlugin := map[string]int{}
	for _, automation := range cfg.AutomationList() {
		for _, plugin := range automation.Plugins {
			if index, seen := indexByPlugin[plugin]; seen {
				results[index].Purpose += ", " + automation.Name
				continue
			}
			result := Result{Name: "plugin " + plugin, Purpose: automation.Name, Found: aitool.Available(plugin)}
			if dir, installed := aitool.PluginDir(plugin); installed {
				result.Path = dir
			}
			if aitool.IsConnector(plugin) {
				result.Note = "claude.ai connector, checked by Claude at run time"
			}
			indexByPlugin[plugin] = len(results)
			results = append(results, result)
		}
	}
	return results
}

func configuredResults(cfg *config.Config, lookPath func(string) (string, error), builtIns []Result) []Result {
	var results []Result
	indexByName := map[string]int{}
	for _, command := range configuredCommands(cfg) {
		purpose, binary := command[0], commandBinary(command[1])
		if binary == "" || slices.ContainsFunc(builtIns, func(builtIn Result) bool { return builtIn.Name == binary }) {
			continue
		}
		if index, seen := indexByName[binary]; seen {
			if !slices.Contains(strings.Split(results[index].Purpose, ", "), purpose) {
				results[index].Purpose += ", " + purpose
			}
			continue
		}
		path, err := lookPath(binary)
		indexByName[binary] = len(results)
		results = append(results, Result{Name: binary, Purpose: purpose, Path: path, Found: err == nil, Required: true})
	}
	return results
}

func commandBinary(command string) string {
	remaining := strings.TrimSpace(command)
	for remaining != "" {
		var token string
		if quote := remaining[0]; quote == '"' || quote == '\'' {
			end := strings.IndexByte(remaining[1:], quote)
			if end < 0 {
				return ""
			}
			token, remaining = remaining[1:end+1], strings.TrimSpace(remaining[end+2:])
		} else {
			token, remaining, _ = strings.Cut(remaining, " ")
			remaining = strings.TrimSpace(remaining)
		}
		if name, _, isAssignment := strings.Cut(token, "="); isAssignment && name != "" && !strings.ContainsAny(name, "/\"'") {
			continue
		}
		return token
	}
	return ""
}

func Failed(results []Result) bool {
	return slices.ContainsFunc(results, func(result Result) bool { return result.Required && !result.Found })
}

func Format(results []Result) string {
	nameWidth := 0
	for _, result := range results {
		nameWidth = max(nameWidth, len(result.Name))
	}
	var lines []string
	for _, result := range results {
		var mark, status string
		switch {
		case result.Note != "":
			mark, status = "-", result.Note
		case result.Found:
			mark, status = "✓", result.Path
		case result.Required:
			mark, status = "✗", "missing"
		default:
			mark, status = "!", "missing (optional)"
		}
		lines = append(lines, fmt.Sprintf("%s %-*s  %s  %s", mark, nameWidth, result.Name, status, result.Purpose))
	}
	return strings.Join(lines, "\n")
}
