package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/config"

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
	m.cfg.WorkDays = everyDay
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
	if len(updated.git.localCommitsToday["console"]) != 1 || len(updated.git.localCommitsYesterday["console"]) != 1 {
		t.Errorf("commits not applied: %+v %+v", updated.git.localCommitsToday, updated.git.localCommitsYesterday)
	}
	if updated.git.fetchGeneration != m.git.fetchGeneration {
		t.Errorf("loading commits must not start a git sync")
	}
}

func TestStaleCommitsIgnored(t *testing.T) {
	stubDayCommits(t)
	m := syncTestModel(t)
	msg := m.loadCommitsCmd()().(commitsLoadedMsg)
	m.git.commitsGeneration++
	next, _ := m.Update(msg)
	if len(next.(Model).git.localCommitsToday) != 0 {
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
	generation, fetches := m.git.commitsGeneration, m.git.fetchGeneration

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	afterC := next.(Model)
	if afterC.git.commitsGeneration != generation+1 || afterC.git.fetchGeneration != fetches || cmd == nil {
		t.Errorf("c: commits gen %d→%d, fetch gen %d→%d", generation, afterC.git.commitsGeneration, fetches, afterC.git.fetchGeneration)
	}

	next, _ = afterC.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	afterG := next.(Model)
	if afterG.git.commitsGeneration != afterC.git.commitsGeneration+1 || afterG.git.fetchGeneration != fetches+1 {
		t.Errorf("g should refresh git and commits")
	}

	next, _ = afterG.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	if next.(Model).git.commitsGeneration != afterG.git.commitsGeneration+1 {
		t.Errorf("date change should reload commits")
	}
}

func gitStripHeader(m Model) string {
	lines, _ := m.renderGitStrip(m.width-4, false, m.groupNotes())
	return stripANSI(lines[0])
}

func settledCommitsModel(t *testing.T) Model {
	t.Helper()
	m := syncTestModel(t)
	next, _ := m.Update(m.loadCommitsCmd()())
	return next.(Model)
}

func TestCommitRefreshShowsSyncingInTheHeaderUntilCommitsLand(t *testing.T) {
	stubDayCommits(t)
	m := settledCommitsModel(t)
	m.git.loadingGit = false
	m = press(t, m, runes("c"))
	if !strings.Contains(headerTopRow(m), "syncing") || strings.Contains(gitStripHeader(m), "syncing") {
		t.Errorf("c should show syncing in the header only: %q / %q", headerTopRow(m), gitStripHeader(m))
	}
	next, _ := m.Update(m.loadCommitsCmd()())
	if row := headerTopRow(next.(Model)); !strings.Contains(row, "synced") {
		t.Errorf("header should say synced once commits land: %q", row)
	}
}

func TestGitStripIgnoresGlobalGitSync(t *testing.T) {
	stubDayCommits(t)
	m := settledCommitsModel(t)
	m.git.loadingGit = true
	m.git.yesterdayGitRepo, m.git.todayGitRepos = nil, nil
	lines, _ := m.renderGitStrip(m.width-4, false, m.groupNotes())
	if strip := stripANSI(strings.Join(lines, "\n")); strings.Contains(strip, "syncing") || strings.Contains(strip, "checking") {
		t.Errorf("global git sync must not affect the strip:\n%s", strip)
	}
	m.git.loadingGit = false
	m = press(t, m, runes("c"))
	if body := stripANSI(m.renderDashboardBody()); strings.Contains(body, "syncing") {
		t.Errorf("sync progress belongs in the header only:\n%s", body)
	}
}

func TestDateChangeDropsOldCommitsAndFetchesNewDates(t *testing.T) {
	days := stubDayCommits(t)
	m := settledCommitsModel(t)
	m.cfg.WorkDays = everyDay
	if len(m.git.localCommitsToday) == 0 {
		t.Fatal("setup: commits missing")
	}
	m = press(t, m, runes("p"))
	if len(m.git.localCommitsToday) != 0 || len(m.git.localCommitsYesterday) != 0 {
		t.Errorf("old dates' commits should be cleared on date change")
	}
	for _, repo := range append(m.git.todayGitRepos, m.git.yesterdayGitRepo...) {
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

func TestStartupCommitsLoadIsCancelledByGitSyncCancel(t *testing.T) {
	stubDayCommits(t)
	var loadCtx context.Context
	stubbed := fetchDaysCommits
	fetchDaysCommits = func(ctx context.Context, cfg *config.Config, dates ...time.Time) []map[string][]GitPRItem {
		loadCtx = ctx
		return stubbed(ctx, cfg, dates...)
	}
	m := syncTestModel(t)
	load := m.loadCommitsCmd()
	m.cancelGitSync()
	load()
	if loadCtx == nil || loadCtx.Err() == nil {
		t.Errorf("startup commits load should see the cancellation")
	}
}
