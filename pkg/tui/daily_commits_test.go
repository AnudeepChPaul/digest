package tui

import (
	"strings"
	"testing"
)

func commitItem(repo string) GitPRItem {
	return GitPRItem{Title: "abc123 fix", Kind: "Commit", Repository: repo}
}

func TestDailyCommitsHiddenWhenDisabled(t *testing.T) {
	m := syncTestModel(t)
	disabled := false
	m.cfg.ShowDailyCommits = &disabled
	m.ghReviewedYesterday = []GitPRItem{reviewedItem("console", 1)}
	m.localCommitsYesterday = map[string][]GitPRItem{"console": {commitItem("console")}, "tool": {commitItem("tool")}}
	m.rebuildGitRepoStats()
	if len(m.yesterdayGitRepo) != 1 || m.yesterdayGitRepo[0].Commits != 0 {
		t.Fatalf("repos = %+v; commits must not create rows or counts", m.yesterdayGitRepo)
	}
	if row := m.renderGitRepoRow(m.yesterdayGitRepo[0], false, 100); strings.Contains(row, "commits") {
		t.Errorf("row shows commits: %q", row)
	}
	if view := m.View(); strings.Contains(view, "commits") {
		t.Errorf("view shows commits:\n%s", view)
	}
}

func TestDailyCommitsShownByDefault(t *testing.T) {
	m := syncTestModel(t)
	m.localCommitsYesterday = map[string][]GitPRItem{"console": {commitItem("console")}}
	m.rebuildGitRepoStats()
	if len(m.yesterdayGitRepo) != 1 || m.yesterdayGitRepo[0].Commits != 1 {
		t.Fatalf("repos = %+v", m.yesterdayGitRepo)
	}
	if row := m.renderGitRepoRow(m.yesterdayGitRepo[0], false, 100); !strings.Contains(row, "1 commits") {
		t.Errorf("row = %q", row)
	}
}
