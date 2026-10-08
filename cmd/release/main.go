package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/release"
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
		commits, err := commitsSinceLastTag()
		if err != nil {
			return err
		}
		fmt.Println(release.NextLevel(commits))
		return nil
	case "changelog":
		if len(args) != 2 {
			return errors.New("usage: release changelog <version>")
		}
		commits, err := commitsSinceLastTag()
		if err != nil {
			return err
		}
		existing, err := os.ReadFile(changelogPath)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		section := release.Changelog(args[1], time.Now().UTC(), commits)
		return os.WriteFile(changelogPath, []byte(release.Prepend(string(existing), section)), 0o644)
	case "notes":
		changelog, err := os.ReadFile(changelogPath)
		if err != nil {
			return err
		}
		fmt.Print(release.LatestSection(string(changelog)))
		return nil
	}
	return fmt.Errorf("unknown command %q", args[0])
}

func commitsSinceLastTag() ([]release.Commit, error) {
	logArgs := []string{"log", "--format=%s%x1f%b%x1e"}
	if lastTag, err := exec.Command("git", "describe", "--tags", "--abbrev=0", "--match", "v*").Output(); err == nil {
		logArgs = append(logArgs, strings.TrimSpace(string(lastTag))+"..HEAD")
	}
	output, err := exec.Command("git", logArgs...).Output()
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
