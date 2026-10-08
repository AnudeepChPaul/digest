package release

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

type Level string

const (
	LevelNone  Level = "none"
	LevelPatch Level = "patch"
	LevelMinor Level = "minor"
	LevelMajor Level = "major"
)

type Commit struct {
	Subject string
	Body    string
}

const changelogTitle = "# Changelog\n\n"

var (
	conventionalSubject = regexp.MustCompile(`^(\w+)(?:\(([^)]+)\))?(!)?: (.+)$`)
	releaseSubject      = regexp.MustCompile(`^chore: release v`)
	breakingFooter      = regexp.MustCompile(`(?m)^BREAKING[ -]CHANGE: `)
)

type parsedCommit struct {
	kind        string
	scope       string
	description string
	breaking    bool
}

func parse(commit Commit) (parsedCommit, bool) {
	if releaseSubject.MatchString(commit.Subject) {
		return parsedCommit{}, false
	}
	match := conventionalSubject.FindStringSubmatch(commit.Subject)
	if match == nil {
		return parsedCommit{}, false
	}
	return parsedCommit{
		kind:        match[1],
		scope:       match[2],
		description: match[4],
		breaking:    match[3] == "!" || breakingFooter.MatchString(commit.Body),
	}, true
}

func NextLevel(commits []Commit) Level {
	level := LevelNone
	for _, commit := range commits {
		parsed, ok := parse(commit)
		switch {
		case !ok:
		case parsed.breaking:
			return LevelMajor
		case parsed.kind == "feat":
			level = LevelMinor
		case (parsed.kind == "fix" || parsed.kind == "perf") && level == LevelNone:
			level = LevelPatch
		}
	}
	return level
}

func Changelog(version string, date time.Time, commits []Commit) string {
	groups := []struct {
		heading string
		matches func(parsedCommit) bool
		entries []string
	}{
		{heading: "Breaking changes", matches: func(c parsedCommit) bool { return c.breaking }},
		{heading: "Features", matches: func(c parsedCommit) bool { return c.kind == "feat" }},
		{heading: "Fixes", matches: func(c parsedCommit) bool { return c.kind == "fix" }},
		{heading: "Performance", matches: func(c parsedCommit) bool { return c.kind == "perf" }},
	}
	for _, commit := range commits {
		parsed, ok := parse(commit)
		if !ok {
			continue
		}
		entry := parsed.description
		if parsed.scope != "" {
			entry = parsed.scope + ": " + entry
		}
		for index := range groups {
			if groups[index].matches(parsed) {
				groups[index].entries = append(groups[index].entries, entry)
				break
			}
		}
	}
	var section strings.Builder
	fmt.Fprintf(&section, "## v%s — %s\n", version, date.Format("2006-01-02"))
	for _, group := range groups {
		if len(group.entries) == 0 {
			continue
		}
		fmt.Fprintf(&section, "\n### %s\n", group.heading)
		for _, entry := range group.entries {
			fmt.Fprintf(&section, "- %s\n", entry)
		}
	}
	return section.String()
}

func Prepend(existing, section string) string {
	olderSections := strings.TrimPrefix(existing, changelogTitle)
	if strings.TrimSpace(olderSections) == "" {
		return changelogTitle + section
	}
	return changelogTitle + section + "\n" + olderSections
}

func LatestSection(changelog string) string {
	_, afterHeading, found := strings.Cut(strings.TrimPrefix(changelog, changelogTitle), "\n")
	if !found {
		return ""
	}
	latest, _, _ := strings.Cut(afterHeading, "\n## ")
	return strings.TrimLeft(strings.TrimRight(latest, "\n"), "\n") + "\n"
}
