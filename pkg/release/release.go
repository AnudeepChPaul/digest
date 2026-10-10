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
	Hash    string
	Subject string
	Body    string
}

const changelogTitle = "# Changelog\n\n"

var (
	conventionalSubject = regexp.MustCompile(`^(\w+)(?:\(([^)]+)\))?(!)?: (.+)$`)
	releaseSubject      = regexp.MustCompile(`^(chore: release v|release:(patch|minor|major)$)`)
	bookkeepingSubject  = regexp.MustCompile(`^(chore: (release|changelog|benchmark) v|release:(patch|minor|major)$)`)
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

func Changelog(version string, date time.Time, repoURL string, commits []Commit) string {
	var section strings.Builder
	fmt.Fprintf(&section, "## v%s — %s\n", version, date.Format("2006-01-02"))
	listed := false
	for _, commit := range commits {
		if bookkeepingSubject.MatchString(commit.Subject) {
			continue
		}
		if !listed {
			section.WriteString("\n### Changes\n")
			listed = true
		}
		shortHash := commit.Hash[:min(7, len(commit.Hash))]
		fmt.Fprintf(&section, "- [%s](%s/commit/%s) %s\n", shortHash, repoURL, commit.Hash, commit.Subject)
		for line := range strings.SplitSeq(commit.Body, "\n") {
			if trimmed := strings.TrimSpace(line); trimmed != "" {
				fmt.Fprintf(&section, "  %s\n", trimmed)
			}
		}
	}
	return section.String()
}

func Prepend(existing, section string) string {
	olderSections := strings.TrimPrefix(existing, strings.TrimSpace(changelogTitle))
	if strings.TrimSpace(olderSections) == "" {
		return changelogTitle + section
	}
	return changelogTitle + section + "\n" + strings.TrimLeft(olderSections, "\n")
}

func LatestSection(changelog string) string {
	_, afterHeading, found := strings.Cut(strings.TrimPrefix(changelog, changelogTitle), "\n")
	if !found {
		return ""
	}
	latest, _, _ := strings.Cut(afterHeading, "\n## ")
	return strings.TrimLeft(strings.TrimRight(latest, "\n"), "\n") + "\n"
}
