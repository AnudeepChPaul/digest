package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

type commitsLoadedMsg struct {
	generation int
	today      map[string][]GitPRItem
	yesterday  map[string][]GitPRItem
}

func (m *Model) resetCommitsForDate() {
	m.git.localCommitsToday, m.git.localCommitsYesterday = nil, nil
	m.rebuildGitRepoStats()
}

func (m *Model) applyCommits(msg commitsLoadedMsg) {
	if msg.generation != m.git.commitsGeneration {
		return
	}
	m.git.loadingCommits = false
	m.noteGitSyncDone()
	selectedKey, selectedOccurrence := m.selectedNavKey()
	m.git.localCommitsToday, m.git.localCommitsYesterday = msg.today, msg.yesterday
	m.rebuildGitRepoStats()
	m.restoreSelection(selectedKey, selectedOccurrence)
	m.updateScrollOffset()
}

func (m Model) refreshCommitsFromKey(tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m, m.refreshCommitsCmd()
}
