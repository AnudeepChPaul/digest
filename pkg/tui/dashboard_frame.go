package tui

import (
	"strings"

	"github.com/achandrapaul/digest/pkg/automation"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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
		frame.header, frame.footer = headerSection{}.Render(m), m.renderFooter()
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
	return &dashboardFrame{header: headerSection{}.Render(m), footer: m.renderFooter(), content: cached.content, selectedLine: cached.selectedLine}
}

func (m Model) frameBodyHeight(frame *dashboardFrame) int {
	return max(m.height-lipgloss.Height(frame.header)-lipgloss.Height(frame.footer), 10)
}

func (m Model) showsDashboard() bool {
	return m.mode == ViewDashboard || m.mode == ViewInlineEdit || (m.mode == ViewGitDetails && m.git.gitPopupRepo == nil)
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

type dashboardBuilder struct {
	board        strings.Builder
	selected     int
	navIndex     int
	selectedLine int
	innerWidth   int
	rowWidth     int
}

func (builder *dashboardBuilder) lineCount() int {
	return strings.Count(builder.board.String(), "\n")
}

func (builder *dashboardBuilder) emitRow(render func(selected bool) string) {
	selected := builder.navIndex == builder.selected
	if selected {
		builder.selectedLine = builder.lineCount()
	}
	builder.board.WriteString(render(selected))
	builder.navIndex++
}

func (builder *dashboardBuilder) selectedWithin(start, end int) bool {
	return builder.selected >= start && builder.selected < end
}

type dashboardData struct {
	groups         noteGroups
	pendingGroups  []PendingRepoGroup
	drafts         []*JobDraft
	automationRuns []automation.Run
	jobsCount      int
	previousEnd    int
	carriedEnd     int
	addedEnd       int
	closedEnd      int
	gitStripEnd    int
	pendingEnd     int
	jobsEnd        int
}

func (m Model) dashboardData() dashboardData {
	data := dashboardData{
		groups:         m.groupNotes(),
		pendingGroups:  m.getPendingGitGroups(),
		drafts:         m.getJobDrafts(),
		automationRuns: m.automationJobRuns(),
	}
	data.jobsCount = len(data.drafts) + len(m.reviewRuns) + len(m.bragRuns) + len(data.automationRuns)
	data.previousEnd = len(data.groups.previousDone)
	data.carriedEnd = data.previousEnd + len(data.groups.carried)
	data.addedEnd = data.carriedEnd + len(data.groups.today)
	data.closedEnd = data.addedEnd + len(data.groups.todayDone)
	data.gitStripEnd = data.closedEnd
	if m.cfg.GitEnabled() {
		data.gitStripEnd += len(m.git.yesterdayGitRepo) + len(m.git.todayGitRepos) + len(m.git.myPRs)
	}
	data.pendingEnd = data.gitStripEnd
	for _, group := range data.pendingGroups {
		data.pendingEnd += len(group.Items)
	}
	data.jobsEnd = data.pendingEnd + data.jobsCount
	return data
}

const tagGap = "  "

func alignRight(left string, tags []string, width int, selected bool) string {
	tagBlock := joinTags(tags)
	room := width - lipgloss.Width(tagBlock) - 1
	if lipgloss.Width(left) > room {
		left = ansi.Truncate(left, max(room, 0), "…")
	}
	return left + safeRepeat(" ", width-lipgloss.Width(left)-lipgloss.Width(tagBlock)) + underlinedWhen(selected, tagBlock)
}

func joinTags(tags []string) string {
	var shown []string
	for _, tag := range tags {
		if tag != "" {
			shown = append(shown, tag)
		}
	}
	return strings.Join(shown, tagGap)
}
