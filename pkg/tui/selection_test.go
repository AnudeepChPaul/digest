package tui

import (
	"testing"

	"github.com/achandrapaul/digest/pkg/config"
	"github.com/achandrapaul/digest/pkg/review"
	"github.com/achandrapaul/digest/pkg/sourcecontrol"
)

func selectionTestModel(t *testing.T) Model {
	t.Helper()
	cfg := &config.Config{DigestRoot: t.TempDir(), GreenOnly: true, Jobs: []config.JobSpec{{Name: "janitor"}, {Name: "repo sync"}}}
	m := NewModel(cfg, nil)
	m.width, m.height = 120, 40
	return m
}

func selectNavItem(t *testing.T, m *Model, key string) {
	t.Helper()
	for index, item := range m.allNavItems() {
		if navItemKey(item) == key {
			m.selected = index
			return
		}
	}
	t.Fatalf("no nav item %q", key)
}

func pendingItem(number int) GitPRItem {
	ref := review.PRRef{Host: "github.com", Owner: "o", Repo: "console", Number: number, URL: "https://github.com/o/console/pull/" + string(rune('0'+number))}
	return sourcecontrol.NewPRItem(review.QueuedPR{Ref: ref, Title: "t", CIState: "SUCCESS"}, "Pending Review")
}

func TestSelectedJobSurvivesPRLoad(t *testing.T) {
	m := selectionTestModel(t)
	selectNavItem(t, &m, "job:repo sync")
	m.applyGitPending(gitPendingMsg{generation: m.git.fetchGeneration, pending: []GitPRItem{pendingItem(1), pendingItem(2), pendingItem(3)}})
	items := m.allNavItems()
	if m.selected >= len(items) || navItemKey(items[m.selected]) != "job:repo sync" {
		t.Fatalf("selected %d is not the repo sync job", m.selected)
	}
}

func TestSelectedPRSurvivesReorder(t *testing.T) {
	m := selectionTestModel(t)
	m.applyGitPending(gitPendingMsg{generation: m.git.fetchGeneration, pending: []GitPRItem{pendingItem(1), pendingItem(2)}})
	selectNavItem(t, &m, "pr:"+pendingItem(2).URL)
	m.applyGitPending(gitPendingMsg{generation: m.git.fetchGeneration, pending: []GitPRItem{pendingItem(3), pendingItem(1), pendingItem(2)}})
	items := m.allNavItems()
	if navItemKey(items[m.selected]) != "pr:"+pendingItem(2).URL {
		t.Fatalf("selected %q", navItemKey(items[m.selected]))
	}
}

func TestSelectionClampsWhenRowDisappears(t *testing.T) {
	m := selectionTestModel(t)
	m.applyGitPending(gitPendingMsg{generation: m.git.fetchGeneration, pending: []GitPRItem{pendingItem(1)}})
	selectNavItem(t, &m, "pr:"+pendingItem(1).URL)
	m.applyGitPending(gitPendingMsg{generation: m.git.fetchGeneration})
	if m.selected < 0 || m.selected >= len(m.allNavItems()) {
		t.Fatalf("selected %d out of range", m.selected)
	}
}
