package tui

import (
	"strings"
	"time"

	"app/pkg/model"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var hintIdleDelay = 250 * time.Millisecond

type hintIdleMsg struct {
	generation int
}

var (
	hintPillStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colourAccent).Padding(0, 1)
	hintKeyStyle  = lipgloss.NewStyle().Bold(true).Foreground(colourAccent)
	hintTextStyle = lipgloss.NewStyle().Foreground(colourText)
)

type keyHint struct {
	key   string
	label string
	icon  string
}

func (m *Model) restartHintTimer() tea.Cmd {
	m.hintVisible = false
	if !m.cfg.ShowKeyHints {
		return nil
	}
	m.hintGeneration++
	generation := m.hintGeneration
	return tea.Tick(hintIdleDelay, func(time.Time) tea.Msg { return hintIdleMsg{generation: generation} })
}

func (m Model) startupHintCmd() tea.Cmd {
	if !m.cfg.ShowKeyHints {
		return nil
	}
	generation := m.hintGeneration
	return tea.Tick(hintIdleDelay, func(time.Time) tea.Msg { return hintIdleMsg{generation: generation} })
}

func (m Model) selectedRowHints() []keyHint {
	item, found := m.selectedNavItem()
	if !found {
		return nil
	}
	switch item.Kind {
	case KindTodayNote, KindCarriedNote, KindYesterdayDone, KindTodayDone:
		return m.noteRowHints(item.Note)
	case KindPendingGit:
		return []keyHint{{key: "↵", label: "open"}, {key: "y", label: "approve"}, {key: "d", label: "reject"}, {key: "r", label: "review"}}
	case KindJobDraft, KindReviewRun, KindBragRun, KindAutomationRun:
		return []keyHint{{key: "↵", label: "open"}, {key: "r", label: "run"}, {key: "d", label: "stop"}}
	}
	return nil
}

func (m Model) noteRowHints(note *model.Note) []keyHint {
	toggleLabel := "done"
	if note != nil && note.Status == model.StatusDone {
		toggleLabel = "active"
	}
	hints := []keyHint{{key: "↵", label: "open"}, {key: "␣", label: toggleLabel}, {key: "i", label: "inline"}}
	if len(m.noteActions(note)) > 0 {
		hints = append(hints, keyHint{key: ".|@", label: "actions"})
	}
	return hints
}

func (m Model) keyHintPill() string {
	hints := m.selectedRowHints()
	if len(hints) == 0 {
		return ""
	}
	return renderHintPill(hints)
}

func renderHintPill(hints []keyHint) string {
	var parts []string
	for _, hint := range hints {
		marker := "(" + hint.key + ")"
		if hint.icon != "" {
			marker = hint.icon + " "
		}
		parts = append(parts, hintKeyStyle.Render(marker)+hintTextStyle.Render(hint.label))
	}
	return hintPillStyle.Render(strings.Join(parts, " "))
}

func (m Model) inlineEditPill() string {
	return renderHintPill([]keyHint{{icon: "✎", label: "editing"}, {key: "↵", label: "save"}, {key: "esc", label: "cancel"}})
}

func (m Model) overlayUnderSelectedRow(box string, left int) string {
	frame := m.currentDashboardFrame()
	dashboard := lipgloss.JoinVertical(lipgloss.Left, frame.header, m.renderFrameBody(frame), frame.footer)
	rowLine := lipgloss.Height(frame.header) + frame.selectedLine - m.visibleScrollOffset(frame)
	top := rowLine + 1
	if top+lipgloss.Height(box) > m.height {
		top = max(rowLine-lipgloss.Height(box), 0)
	}
	return overlayAt(dashboard, box, top, left)
}

func (m Model) rightAlignedColumn(box string) int {
	return max(m.width-lipgloss.Width(box)-2, 0)
}

func (m Model) prHintKeysActive() bool {
	return m.cfg.ShowKeyHints && m.currentPRItem() != nil
}

func (m Model) prRowBindings() []keyBinding {
	if !m.prHintKeysActive() {
		return nil
	}
	return []keyBinding{
		hiddenKeyBinding(actionDashboardApprove, "y"),
		hiddenKeyBinding(actionDashboardReject, "d"),
		hiddenKeyBinding(actionStartReview, "r"),
	}
}

func (m Model) dashboardApprove(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.prActionFromDashboard = true
	return m.approvePR(msg)
}

func (m Model) dashboardReject(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.prActionFromDashboard = true
	if item := m.currentPRItem(); item != nil {
		if _, findings := m.loadFindings(item); len(findings) > 0 {
			m.reviewSelected = make(map[int]bool, len(findings))
			for index := range findings {
				m.reviewSelected[index] = true
			}
		}
	}
	return m.rejectOrStopReview(msg)
}

func (m *Model) prActionReturnMode() ViewMode {
	if m.prActionFromDashboard {
		m.prActionFromDashboard = false
		return ViewDashboard
	}
	return ViewPreview
}
