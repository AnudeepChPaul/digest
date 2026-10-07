package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m *Model) updateScrollOffset() {
	m.scrollPending = true
}

const (
	pulsePhaseCount = 24
	maxCachedFrames = 64
)

type dashboardFrameKey struct {
	contentVersion int
	width          int
	height         int
	selected       int
	mode           ViewMode
	pulsePhase     int
}

type dashboardFrame struct {
	header       string
	footer       string
	content      string
	selectedLine int
}

type dashboardFrameCache struct {
	frames            map[dashboardFrameKey]*dashboardFrame
	lastContentLength int
}

func newDashboardFrameCache() *dashboardFrameCache {
	return &dashboardFrameCache{frames: map[dashboardFrameKey]*dashboardFrame{}}
}

func (m Model) dashboardFrameKey() dashboardFrameKey {
	return dashboardFrameKey{
		contentVersion: m.contentVersion,
		width:          m.width,
		height:         m.height,
		selected:       m.selected,
		mode:           m.mode,
		pulsePhase:     m.syncPulseFrame % pulsePhaseCount,
	}
}

func (m Model) buildDashboardFrame() *dashboardFrame {
	frame := &dashboardFrame{}
	frame.content, frame.selectedLine = m.dashboardContent()
	return frame
}

func (m Model) currentDashboardFrame() *dashboardFrame {
	if m.frames == nil {
		frame := m.buildDashboardFrame()
		frame.header, frame.footer = m.renderHeader(), m.renderFooter()
		return frame
	}
	key := m.dashboardFrameKey()
	cached, found := m.frames.frames[key]
	if !found {
		for existingKey := range m.frames.frames {
			if existingKey.contentVersion != key.contentVersion || existingKey.width != key.width || existingKey.height != key.height || existingKey.selected != key.selected || existingKey.mode != key.mode || len(m.frames.frames) >= maxCachedFrames {
				clear(m.frames.frames)
				break
			}
		}
		cached = m.buildDashboardFrame()
		m.frames.frames[key] = cached
	}
	return &dashboardFrame{header: m.renderHeader(), footer: m.renderFooter(), content: cached.content, selectedLine: cached.selectedLine}
}

func (m Model) frameBodyHeight(frame *dashboardFrame) int {
	return max(m.height-lipgloss.Height(frame.header)-lipgloss.Height(frame.footer), 10)
}

func (m Model) showsDashboard() bool {
	return m.mode == ViewDashboard || m.mode == ViewInlineEdit || (m.mode == ViewGitDetails && m.gitPopupRepo == nil)
}

func (m *Model) settleScroll() {
	if !m.showsDashboard() {
		return
	}
	m.scrollPending = false
	if m.selected == 0 {
		m.scrollOffset = 0
		return
	}
	frame := m.currentDashboardFrame()
	bodyHeight := m.frameBodyHeight(frame)
	selectedLineIdx := frame.selectedLine
	totalLines := strings.Count(frame.content, "\n") + 1

	lookaheadTop := selectedLineIdx - 2
	if lookaheadTop < 0 {
		lookaheadTop = 0
	}

	if lookaheadTop < m.scrollOffset {
		m.scrollOffset = lookaheadTop
	} else if selectedLineIdx >= m.scrollOffset+bodyHeight {
		m.scrollOffset = selectedLineIdx - bodyHeight + 1
	}

	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
	if m.scrollOffset > totalLines-bodyHeight && totalLines > bodyHeight {
		m.scrollOffset = totalLines - bodyHeight
	}
}
