package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/habit"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type headerSection struct{}

var gatherHabits = habit.Gather

func (m Model) progressText() string {
	facts := gatherHabits(m.notes, time.Now(), m.cfg.IsWorkDay)
	if m.appStateKnown {
		facts.Streak = m.appState.CurrentStreak(time.Now(), m.cfg.IsWorkDay)
	}
	var parts []string
	if facts.Streak > 0 {
		parts = append(parts, yellowBadgeStyle.Render(fmt.Sprintf("🔥 %d-day streak", facts.Streak)))
	}
	if facts.ClosedThisWeek > 0 {
		parts = append(parts, mutedStyle.Render(fmt.Sprintf("%d closed this week", facts.ClosedThisWeek)))
	}
	return strings.Join(parts, mutedStyle.Render(" · "))
}

func (headerSection) Render(m Model) string {
	renderedDate := mutedStyle.Bold(true).Render("— " + time.Now().Format("Monday 02 Jan"))
	middleLine := m.renderBannerLine(bannerMiddleRow) + "  "
	if version := displayVersion(); version != "" {
		middleLine += versionStyle.Render(version) + "  "
	}
	middleLine += renderedDate
	if notice := m.unbraggedWeekNotice(); notice != "" {
		noticeRoom := m.width - lipgloss.Width(middleLine) - 7
		if noticeRoom > 0 {
			noticeText := yellowBadgeStyle.Render(ansi.Truncate(notice, noticeRoom, "…"))
			middleLine += safeRepeat(" ", m.width-4-lipgloss.Width(middleLine)-lipgloss.Width(noticeText)) + noticeText
		}
	}

	bottomLine := m.renderBannerLine(bannerBottomRow)

	topLine := m.renderBannerLine(bannerTopRow)
	if progress := m.progressText(); progress != "" {
		versionColumn := lipgloss.Width(m.renderBannerLine(bannerMiddleRow)) + 2
		if paddedLine := topLine + safeRepeat(" ", versionColumn-lipgloss.Width(topLine)) + progress; lipgloss.Width(paddedLine) <= m.width-4 {
			topLine = paddedLine
		}
	}
	if message := m.renderActiveMessage(m.width - 4 - lipgloss.Width(topLine) - 4); message != "" {
		topLine += safeRepeat(" ", m.width-4-lipgloss.Width(topLine)-lipgloss.Width(message)) + message
	}

	headerLines := []string{borderStyle.Render("┌" + safeRepeat("─", m.width-2) + "┐")}
	for _, content := range []string{topLine, middleLine, bottomLine} {
		headerLines = append(headerLines, fmt.Sprintf("│ %s%s │", content, safeRepeat(" ", m.width-lipgloss.Width(content)-4)))
	}
	headerLines = append(headerLines, borderStyle.Render("├"+safeRepeat("─", m.width-2)+"┤"))
	return strings.Join(headerLines, "\n")
}

func (headerSection) ApplyMessage(m Model, msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg.(type) {
	case messageExpiryMsg:
		m.messageExpiryPending = false
		return messageHandled(m, nil)

	case bannerWaveTickMsg:
		if !m.bannerWaveActive {
			return messageHandled(m, nil)
		}
		m.bannerWaveFrame++
		if m.bannerWaveFrame > bannerWaveLastFrame() {
			m.bannerWaveActive = false
			return messageHandled(m, nil)
		}
		return messageHandled(m, tickBannerWaveCmd())

	case syncPulseTickMsg:
		if m.headerAnimating() {
			m.syncPulseFrame++
			return messageHandled(m, tickSyncPulseCmd())
		}
		m.syncPulseRunning = false
		return messageHandled(m, nil)
	}
	return m, nil, false
}

var headerKeystrokes sectionKeystrokes

func init() {
	headerKeystrokes = sectionKeystrokes{
		actionOpenMessages:  Model.openMessageLog,
		actionDismissErrors: Model.dismissHeaderErrors,
	}
}

func (section headerSection) ApplyKeystrokes(m Model, binding keyBinding, msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	if binding.action == actionDismissErrors && !section.hasErrors(m) {
		return m, nil, false
	}
	return headerKeystrokes.apply(m, binding, msg)
}

func (headerSection) hasErrors(m Model) bool {
	if len(m.git.syncErrors) > 0 {
		return true
	}
	for _, message := range m.messages {
		if message.kind == messageError && !message.dismissed {
			return true
		}
	}
	return false
}

func (headerSection) dismissErrors(m Model) Model {
	m.git.syncErrors = nil
	m.dismissErrorMessages()
	return m
}

func (m Model) dismissHeaderErrors(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m = headerSection{}.dismissErrors(m)
	return m, m.ensureSyncPulse()
}
