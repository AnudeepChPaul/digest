package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/release"
	"github.com/AnudeepChPaul/digest/pkg/system"
)

const changelogPath = "CHANGELOG.md"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "release:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: release level | changelog <version> | notes")
	}
	switch args[0] {
	case "level":
		commits, err := commitsSinceLastRelease(".")
		if err != nil {
			return err
		}
		fmt.Println(release.NextLevel(commits))
		return nil
	case "changelog":
		if len(args) != 2 {
			return errors.New("usage: release changelog <version>")
		}
		commits, err := commitsSinceLastRelease(".")
		if err != nil {
			return err
		}
		existing, err := system.Read(changelogPath)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		section := release.Changelog(args[1], time.Now().UTC(), commits)
		return system.WriteWithMode(changelogPath, []byte(release.Prepend(string(existing), section)), 0o644)
	case "notes":
		changelog, err := system.Read(changelogPath)
		if err != nil {
			return err
		}
		fmt.Print(release.LatestSection(string(changelog)))
		return nil
	}
	return fmt.Errorf("unknown command %q", args[0])
}

func lastReleaseCommit(repoDir string) (string, bool) {
	gitOutput := func(args ...string) string {
		command := exec.Command("git", args...)
		command.Dir = repoDir
		output, _ := command.Output()
		return strings.TrimSpace(string(output))
	}
	versionCommits := strings.Fields(gitOutput("log", "-2", "--format=%H", "--", ".version"))
	if len(versionCommits) > 0 && versionCommits[0] == gitOutput("rev-parse", "HEAD") {
		versionCommits = versionCommits[1:]
	}
	if len(versionCommits) == 0 {
		return "", false
	}
	return versionCommits[0], true
}

func commitsSinceLastRelease(repoDir string) ([]release.Commit, error) {
	logArgs := []string{"log", "--format=%s%x1f%b%x1e"}
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
		subject, body, _ := strings.Cut(strings.TrimLeft(record, "\n"), "\x1f")
		if subject != "" {
			commits = append(commits, release.Commit{Subject: subject, Body: body})
		}
	}
	return commits, nil
}
