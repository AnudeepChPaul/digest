package tui

import (
	"context"
	"testing"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/config"

	tea "github.com/charmbracelet/bubbletea"
)

func syncRunning(m Model) bool {
	return m.gitFetchCtx != nil && m.gitFetchCtx.Err() == nil
}

func update(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

func TestDaySwitchSyncsOnceAfterTheDelay(t *testing.T) {
	m := syncTestModel(t)
	m = update(m, runes("p"))
	firstPress := m.daySyncGeneration
	m = update(m, runes("p"))
	if syncRunning(m) || !m.loadingGit {
		t.Fatalf("switching days should wait before syncing and show syncing: running=%v loading=%v", syncRunning(m), m.loadingGit)
	}
	if m = update(m, daySyncDueMsg{generation: firstPress}); syncRunning(m) {
		t.Fatal("an earlier press should not sync")
	}
	if m = update(m, daySyncDueMsg{generation: m.daySyncGeneration}); !syncRunning(m) {
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
	gitCtx := m.gitFetchCtx
	update(m, runes("n"))
	if gitCtx.Err() == nil || commitsCtx == nil || commitsCtx.Err() == nil {
		t.Errorf("day switch should cancel git (%v) and commits (%v)", gitCtx.Err(), commitsCtx)
	}
}

func TestSyncDuringTheWaitReplacesTheDelayedOne(t *testing.T) {
	m := syncTestModel(t)
	m = update(m, runes("p"))
	pending := m.daySyncGeneration
	m = update(m, runes("g"))
	generation := m.fetchGeneration
	if m = update(m, daySyncDueMsg{generation: pending}); m.fetchGeneration != generation {
		t.Error("the delayed sync should not run after a manual sync")
	}
}
