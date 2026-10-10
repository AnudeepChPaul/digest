package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/achandrapaul/digest/pkg/release"
	"github.com/achandrapaul/digest/pkg/system"
)

const (
	changelogPath = "CHANGELOG.md"
	repoURL       = "https://github.com/achandrapaul/digest"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "release:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: release level | notes <version>")
	}
	switch args[0] {
	case "level":
		commits, err := commitsSinceLastRelease(".")
		if err != nil {
			return err
		}
		fmt.Println(release.NextLevel(commits))
		return nil
	case "notes":
		if len(args) != 2 {
			return errors.New("usage: release notes <version>")
		}
		return writeNotes(".", args[1], os.Stdout)
	}
	return fmt.Errorf("unknown command %q", args[0])
}

func writeNotes(repoDir, version string, out io.Writer) error {
	changelogFile := filepath.Join(repoDir, changelogPath)
	existing, err := system.Read(changelogFile)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	changelog := string(existing)
	if !strings.Contains(changelog, "\n## v"+version+" — ") {
		commits, err := commitsSinceLastRelease(repoDir)
		if err != nil {
			return err
		}
		changelog = release.Prepend(changelog, release.Changelog(version, time.Now().UTC(), repoURL, commits))
		if err := system.WriteWithMode(changelogFile, []byte(changelog), 0o644); err != nil {
			return err
		}
	}
	_, err = fmt.Fprint(out, release.LatestSection(changelog))
	return err
}

var latestReleaseTag = githubReleaseTag

func githubReleaseTag(repoDir string) string {
	command := exec.Command("gh", "release", "view", "--json", "tagName", "--jq", ".tagName")
	command.Dir = repoDir
	output, _ := command.Output()
	return strings.TrimSpace(string(output))
}

func lastReleaseCommit(repoDir string) (string, bool) {
	gitOutput := func(args ...string) string {
		command := exec.Command("git", args...)
		command.Dir = repoDir
		output, _ := command.Output()
		return strings.TrimSpace(string(output))
	}
	if releaseTag := latestReleaseTag(repoDir); releaseTag != "" {
		if releaseCommit := gitOutput("rev-list", "-n", "1", "refs/tags/"+releaseTag); releaseCommit != "" {
			return releaseCommit, true
		}
	}
	releaseCommit := gitOutput("log", "-1", "--format=%H", "--", ".version")
	return releaseCommit, releaseCommit != ""
}

func commitsSinceLastRelease(repoDir string) ([]release.Commit, error) {
	logArgs := []string{"log", "--format=%H%x1f%s%x1f%b%x1e"}
	if releaseCommit, found := lastReleaseCommit(repoDir); found {
		logArgs = append(logArgs, releaseCommit+"..HEAD")
	}
	command := exec.Command("git", logArgs...)
	command.Dir = repoDir
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}
	var commits []release.Commit
	for record := range strings.SplitSeq(string(output), "\x1e") {
		hash, rest, _ := strings.Cut(strings.TrimLeft(record, "\n"), "\x1f")
		subject, body, _ := strings.Cut(rest, "\x1f")
		if subject != "" {
			commits = append(commits, release.Commit{Hash: hash, Subject: subject, Body: body})
		}
	}
	return commits, nil
}
