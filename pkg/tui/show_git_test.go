package tui

import (
	"strings"
	"testing"

	"app/pkg/config"
	"app/pkg/model"
	"app/pkg/review"

	tea "github.com/charmbracelet/bubbletea"
)

func gitOffModel(t *testing.T) Model {
	t.Helper()
	showGit := false
	m := syncTestModel(t)
	m.cfg.ShowGit = &showGit
	m.cfg.GitAutoSyncInterval = 600
	m.cfg.Jobs = []config.JobSpec{
		{Name: "branch reaper", DryRunCommand: "digest branch-reaper --dry-run", Command: "digest branch-reaper"},
		{Name: "janitor", DryRunCommand: "digest janitor --dry-run", Command: "digest janitor"},
		{Name: "repo sync", DryRunCommand: "digest repo-sync --dry-run", Command: "digest repo-sync"},
	}
	m = NewModel(m.cfg, nil)
	m.width, m.height = 120, 50
	m.yesterdayGitRepo = []*GitRepoStat{{Name: "alpha"}}
	m.todayGitRepos = []*GitRepoStat{{Name: "gamma"}}
	m.myPRs = []review.QueuedPR{{Ref: prRef("acme/web", 7)}}
	m.pendingGitAction = []GitPRItem{reviewedItem("acme/web", 9)}
	m.notes = []*model.Note{{ID: "note-1", Summary: "write docs", Created: m.currentDate, Source: model.SourceManual}}
	return m
}

func TestGitOffSkipsEveryFetch(t *testing.T) {
	m := gitOffModel(t)
	if m.syncOnLoad || m.loadingGit || m.loadingMyPRs || m.loadingCommits {
		t.Errorf("git off: syncOnLoad=%v loadingGit=%v loadingMyPRs=%v loadingCommits=%v, want all false", m.syncOnLoad, m.loadingGit, m.loadingMyPRs, m.loadingCommits)
	}
	if m.gitFetchCmd() != nil || m.myPRsFetchCmd() != nil {
		t.Errorf("git off: fetch commands should be nil")
	}
	if m.startLoadGitStatsCmd() != nil || m.scheduleDaySync() != nil {
		t.Errorf("git off: stats reload and day sync should be nil")
	}
	if _, cmd := m.Update(autoSyncTickMsg{}); cmd != nil {
		t.Errorf("git off: auto-sync tick should not schedule anything")
	}
}

func TestGitOffHidesGitSections(t *testing.T) {
	m := gitOffModel(t)
	body := strings.Join(plainLines(m.renderDashboardBody()), "\n")
	for _, hidden := range []string{"G I T", "M Y   P R", "Pending Git Actions", "alpha", "gamma"} {
		if strings.Contains(body, hidden) {
			t.Errorf("git off: dashboard still shows %q:\n%s", hidden, body)
		}
	}
	if !strings.Contains(body, "write docs") {
		t.Errorf("git off: notes should stay:\n%s", body)
	}
	for _, item := range m.allNavItems() {
		if item.Kind == KindGitRepo || item.Kind == KindMyPR || item.Kind == KindPendingGit {
			t.Errorf("git off: nav item kind %v should be gone", item.Kind)
		}
	}
}

func TestGitOffHidesGitJobs(t *testing.T) {
	m := gitOffModel(t)
	var names []string
	for _, draft := range m.getJobDrafts() {
		names = append(names, draft.Name)
	}
	if strings.Join(names, ",") != "janitor" {
		t.Errorf("git off: jobs = %v, want only janitor", names)
	}
}

func TestGitOffDropsGitKeys(t *testing.T) {
	m := gitOffModel(t)
	gitKeys := map[string]bool{"g": true, "c": true, "h": true, "l": true, "left": true, "right": true, "s": true, "w": true, "m": true}
	for _, binding := range m.dashboardBindings() {
		for _, key := range binding.binding.Keys() {
			if gitKeys[key] {
				t.Errorf("git off: key %q still bound to %v", key, binding.action)
			}
		}
	}
	for _, entry := range m.helpEntries() {
		if strings.Contains(entry.text, "pending PRs") || strings.Contains(entry.text, "git") || strings.Contains(entry.text, "commits") {
			t.Errorf("git off: help still lists %v", entry)
		}
	}
	for _, key := range []string{"g", "s"} {
		if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}); cmd != nil {
			if msgs := collectMsgs(cmd); len(msgs) > 0 {
				t.Errorf("git off: %q produced %v", key, msgs)
			}
		}
	}
}
