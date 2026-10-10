package tui

import (
	"context"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/config"

	tea "github.com/charmbracelet/bubbletea"
)

func syncRunning(m Model) bool {
	return m.git.gitFetchCtx != nil && m.git.gitFetchCtx.Err() == nil
}

func update(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

func TestDaySwitchSyncsOnceAfterTheDelay(t *testing.T) {
	m := syncTestModel(t)
	m = update(m, runes("p"))
	firstPress := m.git.daySyncGeneration
	m = update(m, runes("p"))
	if syncRunning(m) || !m.git.loadingGit {
		t.Fatalf("switching days should wait before syncing and show syncing: running=%v loading=%v", syncRunning(m), m.git.loadingGit)
	}
	if m = update(m, daySyncDueMsg{generation: firstPress}); syncRunning(m) {
		t.Fatal("an earlier press should not sync")
	}
	if m = update(m, daySyncDueMsg{generation: m.git.daySyncGeneration}); !syncRunning(m) {
		t.Fatal("the last press should sync once the delay passes")
	}
	if daySyncDelay != 2*time.Second {
		t.Errorf("delay = %v", daySyncDelay)
	}
}

func TestDaySwitchCancelsRunningSyncs(t *testing.T) {
	var commitsCtx context.Context
	original := fetchDaysCommits
	fetchDaysCommits = func(ctx context.Context, cfg *config.Config, dates ...time.Time) []map[string][]GitPRItem {
		commitsCtx = ctx
		return make([]map[string][]GitPRItem, len(dates))
	}
	t.Cleanup(func() { fetchDaysCommits = original })
	m := syncTestModel(t)
	m.refreshCommitsCmd()
	m.loadCommitsCmd()()
	gitCtx := m.git.gitFetchCtx
	update(m, runes("n"))
	if gitCtx.Err() == nil || commitsCtx == nil || commitsCtx.Err() == nil {
		t.Errorf("day switch should cancel git (%v) and commits (%v)", gitCtx.Err(), commitsCtx)
	}
}

func TestSyncDuringTheWaitReplacesTheDelayedOne(t *testing.T) {
	m := syncTestModel(t)
	m = update(m, runes("p"))
	pending := m.git.daySyncGeneration
	m = update(m, runes("g"))
	generation := m.git.fetchGeneration
	if m = update(m, daySyncDueMsg{generation: pending}); m.git.fetchGeneration != generation {
		t.Error("the delayed sync should not run after a manual sync")
	}
}
