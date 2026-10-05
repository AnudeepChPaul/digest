package tui

import (
	"context"

	"app/pkg/sourcecontrol"

	tea "github.com/charmbracelet/bubbletea"
)

type commitsLoadedMsg struct {
	generation int
	today      map[string][]GitPRItem
	yesterday  map[string][]GitPRItem
}

var fetchDaysCommits = sourcecontrol.FetchLocalCommitsForDays

func (m Model) loadCommitsCmd() tea.Cmd {
	if m.cfg == nil || !m.cfg.DailyCommitsEnabled() {
		return nil
	}
	cfg, date, previousDay, generation := m.cfg, m.currentDate, m.previousNoteDay(), m.commitsGeneration
	ctx := m.commitsCtx
	if ctx == nil {
		ctx = context.Background()
	}
	return func() tea.Msg {
		days := fetchDaysCommits(ctx, cfg, date, previousDay)
		return commitsLoadedMsg{generation: generation, today: days[0], yesterday: days[1]}
	}
}

func (m *Model) refreshCommitsCmd() tea.Cmd {
	if m.commitsCancel != nil {
		m.commitsCancel()
	}
	m.commitsCtx, m.commitsCancel = context.WithCancel(context.Background())
	m.commitsGeneration++
	load := m.loadCommitsCmd()
	if load == nil {
		return nil
	}
	m.loadingCommits = true
	return tea.Batch(load, m.ensureSyncPulse())
}

func (m *Model) resetCommitsForDate() {
	m.localCommitsToday, m.localCommitsYesterday = nil, nil
	m.rebuildGitRepoStats()
}

func (m *Model) applyCommits(msg commitsLoadedMsg) {
	if msg.generation != m.commitsGeneration {
		return
	}
	m.loadingCommits = false
	selectedKey, selectedOccurrence := m.selectedNavKey()
	m.localCommitsToday, m.localCommitsYesterday = msg.today, msg.yesterday
	m.rebuildGitRepoStats()
	m.restoreSelection(selectedKey, selectedOccurrence)
	m.updateScrollOffset()
}

func (m Model) refreshCommitsFromKey(tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m, m.refreshCommitsCmd()
}
