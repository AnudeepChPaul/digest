package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"app/pkg/config"

	tea "github.com/charmbracelet/bubbletea"
)

func stubDayCommits(t *testing.T) *[]string {
	t.Helper()
	original := fetchDaysCommits
	var days []string
	fetchDaysCommits = func(ctx context.Context, cfg *config.Config, dates ...time.Time) []map[string][]GitPRItem {
		var results []map[string][]GitPRItem
		for _, date := range dates {
			days = append(days, date.Format("2006-01-02"))
			results = append(results, map[string][]GitPRItem{"console": {commitItem("console")}})
		}
		return results
	}
	t.Cleanup(func() { fetchDaysCommits = original })
	return &days
}

func TestCommitsLoadWithoutGitSync(t *testing.T) {
	days := stubDayCommits(t)
	m := syncTestModel(t)
	cmd := m.loadCommitsCmd()
	if cmd == nil {
		t.Fatal("commits command missing")
	}
	msg := cmd().(commitsLoadedMsg)
	want := []string{m.currentDate.Format("2006-01-02"), m.currentDate.AddDate(0, 0, -1).Format("2006-01-02")}
	if len(*days) != 2 || (*days)[0] != want[0] || (*days)[1] != want[1] {
		t.Errorf("fetched days = %v want %v", *days, want)
	}
	next, _ := m.Update(msg)
	updated := next.(Model)
	if len(updated.localCommitsToday["console"]) != 1 || len(updated.localCommitsYesterday["console"]) != 1 {
		t.Errorf("commits not applied: %+v %+v", updated.localCommitsToday, updated.localCommitsYesterday)
	}
	if updated.fetchGeneration != m.fetchGeneration {
		t.Errorf("loading commits must not start a git sync")
	}
}

func TestStaleCommitsIgnored(t *testing.T) {
	stubDayCommits(t)
	m := syncTestModel(t)
	msg := m.loadCommitsCmd()().(commitsLoadedMsg)
	m.commitsGeneration++
	next, _ := m.Update(msg)
	if len(next.(Model).localCommitsToday) != 0 {
		t.Errorf("stale commits applied")
	}
}

func TestCommitsDisabledLoadsNothing(t *testing.T) {
	m := syncTestModel(t)
	disabled := false
	m.cfg.ShowDailyCommits = &disabled
	if m.loadCommitsCmd() != nil {
		t.Errorf("commits command should be nil when show_daily_commits is false")
	}
}

func TestCommitRefreshTriggers(t *testing.T) {
	stubDayCommits(t)
	m := syncTestModel(t)
	generation, fetches := m.commitsGeneration, m.fetchGeneration

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	afterC := next.(Model)
	if afterC.commitsGeneration != generation+1 || afterC.fetchGeneration != fetches || cmd == nil {
		t.Errorf("c: commits gen %d→%d, fetch gen %d→%d", generation, afterC.commitsGeneration, fetches, afterC.fetchGeneration)
	}

	next, _ = afterC.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	afterG := next.(Model)
	if afterG.commitsGeneration != afterC.commitsGeneration+1 || afterG.fetchGeneration != fetches+1 {
		t.Errorf("g should refresh git and commits")
	}

	next, _ = afterG.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	if next.(Model).commitsGeneration != afterG.commitsGeneration+1 {
		t.Errorf("date change should reload commits")
	}
}

func gitStripHeader(m Model) string {
	lines, _ := m.renderGitStrip(m.width-4, false)
	return stripANSI(lines[0])
}

func settledCommitsModel(t *testing.T) Model {
	t.Helper()
	m := syncTestModel(t)
	next, _ := m.Update(m.loadCommitsCmd()())
	return next.(Model)
}

func TestCommitRefreshAnimatesGitStripUntilCommitsLand(t *testing.T) {
	stubDayCommits(t)
	m := settledCommitsModel(t)
	if strings.Contains(gitStripHeader(m), "syncing") {
		t.Fatalf("strip should start synced: %q", gitStripHeader(m))
	}
	m = press(t, m, runes("c"))
	if !strings.Contains(gitStripHeader(m), "syncing") {
		t.Errorf("c should show syncing on the strip: %q", gitStripHeader(m))
	}
	next, _ := m.Update(m.loadCommitsCmd()())
	if header := gitStripHeader(next.(Model)); strings.Contains(header, "syncing") {
		t.Errorf("strip should be synced once commits land: %q", header)
	}
}

func TestGitStripIgnoresGlobalGitSync(t *testing.T) {
	stubDayCommits(t)
	m := settledCommitsModel(t)
	m.loadingGit = true
	m.yesterdayGitRepo, m.todayGitRepos = nil, nil
	lines, _ := m.renderGitStrip(m.width-4, false)
	if strip := stripANSI(strings.Join(lines, "\n")); strings.Contains(strip, "syncing") || strings.Contains(strip, "checking") {
		t.Errorf("global git sync must not affect the strip:\n%s", strip)
	}
	if !strings.Contains(stripANSI(m.renderLiveSyncDot()), "syncing") {
		t.Errorf("pending PR dot should still follow global git sync")
	}
	m.loadingGit = false
	m = press(t, m, runes("c"))
	if strings.Contains(stripANSI(m.renderLiveSyncDot()), "syncing") {
		t.Errorf("commit refresh must not animate the pending PR dot")
	}
}

func TestDateChangeDropsOldCommitsAndFetchesNewDates(t *testing.T) {
	days := stubDayCommits(t)
	m := settledCommitsModel(t)
	if len(m.localCommitsToday) == 0 {
		t.Fatal("setup: commits missing")
	}
	m = press(t, m, runes("p"))
	if len(m.localCommitsToday) != 0 || len(m.localCommitsYesterday) != 0 {
		t.Errorf("old dates' commits should be cleared on date change")
	}
	for _, repo := range append(m.todayGitRepos, m.yesterdayGitRepo...) {
		if repo.Commits != 0 {
			t.Errorf("repo %s still shows old commits", repo.Name)
		}
	}
	*days = nil
	m.loadCommitsCmd()()
	want := []string{m.currentDate.Format("2006-01-02"), m.currentDate.AddDate(0, 0, -1).Format("2006-01-02")}
	if strings.Join(*days, ",") != strings.Join(want, ",") {
		t.Errorf("fetched %v, want %v", *days, want)
	}
}
