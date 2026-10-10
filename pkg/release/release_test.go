package release

import (
	"strings"
	"testing"
	"time"
)

func commitsFrom(subjects ...string) []Commit {
	commits := make([]Commit, len(subjects))
	for index, subject := range subjects {
		commits[index] = Commit{Subject: subject}
	}
	return commits
}

func TestBreakingCommitsMakeAMajorRelease(t *testing.T) {
	cases := map[string][]Commit{
		"bang":            commitsFrom("fix: small thing", "feat!: drop the old config format"),
		"scoped bang":     commitsFrom("refactor(config)!: rename every key"),
		"breaking footer": {{Subject: "feat: new flags", Body: "BREAKING CHANGE: --root is gone"}},
	}
	for name, commits := range cases {
		if got := NextLevel(commits); got != LevelMajor {
			t.Errorf("%s: NextLevel = %q, want major", name, got)
		}
	}
}

func TestFeaturesMakeAMinorAndFixesAPatchRelease(t *testing.T) {
	cases := []struct {
		commits []Commit
		want    Level
	}{
		{commitsFrom("fix: crash on empty notes", "feat(tui): streak beside the logo"), LevelMinor},
		{commitsFrom("docs: readme", "fix: crash on empty notes"), LevelPatch},
		{commitsFrom("perf(git): batch the PR search"), LevelPatch},
	}
	for _, c := range cases {
		if got := NextLevel(c.commits); got != c.want {
			t.Errorf("NextLevel(%v) = %q, want %q", c.commits, got, c.want)
		}
	}
}

func TestChoresAndReleaseCommitsMakeNoRelease(t *testing.T) {
	commits := commitsFrom("docs: readme", "ci: cache mise", "chore: tidy", "test: hermetic doctor", "chore: release v1.1.56", "release:minor", "Fetch PRs per repo")
	if got := NextLevel(commits); got != LevelNone {
		t.Errorf("NextLevel = %q, want none", got)
	}
	if got := NextLevel(nil); got != LevelNone {
		t.Errorf("no commits: NextLevel = %q, want none", got)
	}
}

func TestChangelogListsEveryCommitWithItsLinkAndDescription(t *testing.T) {
	commits := []Commit{
		{Hash: "072f17d8c1e2a3b4c5d6e7f8091a2b3c4d5e6f70", Subject: "fix: central file access", Body: "fix: locked digest files\n\nci: release only on a pushed version bump\n"},
		{Hash: "b30d08f000000000000000000000000000000000", Subject: "docs: readme"},
		{Hash: "613f87f000000000000000000000000000000000", Subject: "Initial import of digest"},
	}
	got := Changelog("1.5.0", time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC), "https://github.com/achandrapaul/digest", commits)
	want := `## v1.5.0 — 2026-10-09

### Changes
- [072f17d](https://github.com/achandrapaul/digest/commit/072f17d8c1e2a3b4c5d6e7f8091a2b3c4d5e6f70) fix: central file access
  fix: locked digest files
  ci: release only on a pushed version bump
- [b30d08f](https://github.com/achandrapaul/digest/commit/b30d08f000000000000000000000000000000000) docs: readme
- [613f87f](https://github.com/achandrapaul/digest/commit/613f87f000000000000000000000000000000000) Initial import of digest
`
	if got != want {
		t.Errorf("Changelog =\n%s\nwant\n%s", got, want)
	}
}

func TestChangelogSkipsReleaseBookkeepingCommits(t *testing.T) {
	commits := []Commit{
		{Hash: "aaaaaaa1", Subject: "chore: release v1.4.0"},
		{Hash: "aaaaaaa2", Subject: "chore: changelog v1.4.0"},
		{Hash: "aaaaaaa3", Subject: "chore: benchmark v1.4.0"},
		{Hash: "aaaaaaa4", Subject: "chore: bump deps"},
		{Hash: "aaaaaaa5", Subject: "release:minor"},
	}
	got := Changelog("1.5.0", time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC), "https://example.com/repo", commits)
	if strings.Contains(got, "release v1.4.0") || strings.Contains(got, "changelog v1.4.0") || strings.Contains(got, "benchmark v1.4.0") || strings.Contains(got, "release:minor") {
		t.Errorf("bookkeeping commits should be skipped:\n%s", got)
	}
	if !strings.Contains(got, "- [aaaaaaa](https://example.com/repo/commit/aaaaaaa4) chore: bump deps\n") {
		t.Errorf("other chores should be listed:\n%s", got)
	}
}

func TestChangelogWithoutCommitsHasOnlyTheHeading(t *testing.T) {
	got := Changelog("1.5.0", time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC), "https://example.com/repo", []Commit{{Hash: "aaaaaaa1", Subject: "chore: release v1.5.0"}})
	if got != "## v1.5.0 — 2026-10-09\n" {
		t.Errorf("Changelog = %q", got)
	}
}

func TestPrependPutsTheNewSectionOnTop(t *testing.T) {
	section := "## v1.2.0 — 2026-10-08\n\n### Features\n- streak\n"
	fresh := Prepend("", section)
	if fresh != "# Changelog\n\n"+section {
		t.Errorf("fresh changelog = %q", fresh)
	}
	for _, titleOnly := range []string{"# Changelog\n", "# Changelog\n\n", "# Changelog"} {
		if got := Prepend(titleOnly, section); got != "# Changelog\n\n"+section {
			t.Errorf("Prepend(%q) = %q", titleOnly, got)
		}
	}
	older := "## v1.1.0 — 2026-09-01\n\n### Fixes\n- crash\n"
	updated := Prepend("# Changelog\n\n"+older, section)
	if updated != "# Changelog\n\n"+section+"\n"+older {
		t.Errorf("updated changelog = %q", updated)
	}
}

func TestLatestSectionReturnsTheTopEntry(t *testing.T) {
	changelog := "# Changelog\n\n## v1.2.0 — 2026-10-08\n\n### Features\n- streak\n\n## v1.1.0 — 2026-09-01\n\n### Fixes\n- crash\n"
	if got := LatestSection(changelog); got != "### Features\n- streak\n" {
		t.Errorf("LatestSection = %q", got)
	}
}
