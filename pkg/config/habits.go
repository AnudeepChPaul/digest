package config

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"time"
)

type DigestNotifications struct {
	Morning string `yaml:"morning"`
	Evening string `yaml:"evening"`
}

var weekdayNames = map[string]time.Weekday{
	"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
	"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
}

var WeekdayOrder = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

var DefaultWorkDays = []string{"mon", "tue", "wed", "thu", "fri"}

func ParseClock(clock string) (time.Duration, error) {
	parsed, err := time.Parse("15:04", strings.TrimSpace(clock))
	if err != nil {
		return 0, fmt.Errorf("time %q: use 24h HH:MM like 09:30", clock)
	}
	return time.Duration(parsed.Hour())*time.Hour + time.Duration(parsed.Minute())*time.Minute, nil
}

func (c *Config) workDays() []string {
	if c == nil || len(c.WorkDays) == 0 {
		return DefaultWorkDays
	}
	return c.WorkDays
}

func (c *Config) IsWorkDay(day time.Weekday) bool {
	return slices.ContainsFunc(c.workDays(), func(name string) bool { return weekdayNames[strings.ToLower(name)] == day })
}

func Exists(path string) bool {
	_, err := os.Stat(resolveConfigPath(path))
	return err == nil
}

func Path(path string) string {
	return resolveConfigPath(path)
}

type SetupAnswers struct {
	ShowGit         bool
	RepositoryRoots []string
	WorkDays        []string
	Morning         string
	Evening         string
	ShowKeyHints    bool
	TerminalApp     string
}

var setupKeys = []string{"show_git", "git_repository_roots", "repo_roots", "work_days", "digest_notifications", "show_key_hints", "terminal_app"}

func (answers SetupAnswers) block() []string {
	lines := []string{
		"# false hides every git section and stops all git and GitHub fetching",
		fmt.Sprintf("show_git: %t", answers.ShowGit),
		"git_repository_roots:",
	}
	for _, root := range answers.RepositoryRoots {
		lines = append(lines, "  - "+root)
	}
	if len(answers.RepositoryRoots) == 0 {
		lines[len(lines)-1] = "git_repository_roots: []"
	}
	return append(lines,
		"# days that count for your streak and get summaries; weeks always run Monday to Sunday",
		"work_days: ["+strings.Join(answers.WorkDays, ", ")+"]",
		"# morning and evening summary notifications (24h HH:MM); empty turns one off",
		"digest_notifications:",
		fmt.Sprintf("  morning: %q", answers.Morning),
		fmt.Sprintf("  evening: %q", answers.Evening),
		"# a small key hint under the selected row after a moment of idle",
		fmt.Sprintf("show_key_hints: %t", answers.ShowKeyHints),
		"# terminal app opened when you click a notification",
		fmt.Sprintf("terminal_app: %q", answers.TerminalApp),
	)
}

func topLevelKey(line string) string {
	if line == "" || line[0] == ' ' || line[0] == '-' || line[0] == '#' {
		return ""
	}
	key, _, found := strings.Cut(line, ":")
	if !found {
		return ""
	}
	return key
}

func RenderConfig(text string, answers SetupAnswers) string {
	lines := strings.Split(text, "\n")
	var kept []string
	insertAt := -1
	for index := 0; index < len(lines); index++ {
		key := topLevelKey(lines[index])
		if !slices.Contains(setupKeys, key) {
			kept = append(kept, lines[index])
			if key == "selection_default" || (key == "digest_root" && insertAt < 0) {
				insertAt = len(kept)
			}
			continue
		}
		for len(kept) > 0 && strings.HasPrefix(kept[len(kept)-1], "#") {
			kept = kept[:len(kept)-1]
		}
		if insertAt > len(kept) {
			insertAt = len(kept)
		}
		for index+1 < len(lines) && (strings.HasPrefix(lines[index+1], " ") || strings.HasPrefix(lines[index+1], "-")) {
			index++
		}
	}
	if insertAt < 0 {
		insertAt = 0
	}
	rendered := append(slices.Clone(kept[:insertAt]), answers.block()...)
	return strings.Join(append(rendered, kept[insertAt:]...), "\n")
}

const (
	ShowTagsSelected = "selected"
	ShowTagsAlways   = "always"
)

func (c *Config) ShowAllTags() bool {
	return c.ShowTags == ShowTagsAlways
}
