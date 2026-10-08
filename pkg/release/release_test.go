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
	commits := commitsFrom("docs: readme", "ci: cache mise", "chore: tidy", "test: hermetic doctor", "chore: release v1.1.56", "Fetch PRs per repo")
	if got := NextLevel(commits); got != LevelNone {
		t.Errorf("NextLevel = %q, want none", got)
	}
	if got := NextLevel(nil); got != LevelNone {
		t.Errorf("no commits: NextLevel = %q, want none", got)
	}
}

func TestChangelogGroupsCommitsByKind(t *testing.T) {
	commits := []Commit{
		{Subject: "fix: crash on empty notes"},
		{Subject: "feat(tui): show the streak beside the logo"},
		{Subject: "docs: readme"},
		{Subject: "perf: batch the PR search"},
		{Subject: "feat!: drop the old config format"},
		{Subject: "feat: publish a changelog"},
		{Subject: "chore: release v1.1.56"},
	}
	got := Changelog("1.2.0", time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC), commits)
	want := `## v1.2.0 — 2026-10-08

### Breaking changes
- drop the old config format

### Features
- tui: show the streak beside the logo
- publish a changelog

### Fixes
- crash on empty notes

### Performance
- batch the PR search
`
	if got != want {
		t.Errorf("Changelog =\n%s\nwant\n%s", got, want)
	}
}

func TestChangelogListsBreakingFooterCommitsUnderBreakingChanges(t *testing.T) {
	commits := []Commit{{Subject: "feat: new flags", Body: "BREAKING CHANGE: --root is gone"}}
	got := Changelog("2.0.0", time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC), commits)
	if !strings.Contains(got, "### Breaking changes\n- new flags\n") || strings.Contains(got, "### Features") {
		t.Errorf("breaking footer should list under breaking changes only:\n%s", got)
	}
}

func TestPrependPutsTheNewSectionOnTop(t *testing.T) {
	section := "## v1.2.0 — 2026-10-08\n\n### Features\n- streak\n"
	fresh := Prepend("", section)
	if fresh != "# Changelog\n\n"+section {
		t.Errorf("fresh changelog = %q", fresh)
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
