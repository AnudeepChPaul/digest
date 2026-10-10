package tui

import (
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type messageKind int

const (
	messageProgress messageKind = iota
	messageSuccess
	messageError
)

const (
	messageSourceGit       = "git"
	messageSourceClipboard = "clipboard"
	messageLogTitle        = "MESSAGES"
	successMessageLife     = 4 * time.Second
	maxMessages            = 50
)

var messageNow = time.Now

type appMessage struct {
	source    string
	kind      messageKind
	text      string
	at        time.Time
	dismissed bool
}

type messageExpiryMsg struct{}

var progressFrames = []string{"◐", "◓", "◑", "◒"}

var (
	messageSuccessStyle = lipgloss.NewStyle().Foreground(colourGreen)
	messageErrorStyle   = lipgloss.NewStyle().Foreground(colourRed)
	messageProgressText = lipgloss.NewStyle().Foreground(colourSubtext)

	messageLogProgressStyle = lipgloss.NewStyle().Foreground(colourMauve)
)

func (m *Model) postMessage(source string, kind messageKind, text string) {
	messages := make([]appMessage, 0, len(m.messages)+1)
	for _, message := range m.messages {
		if message.source == source && (kind == messageSuccess || message.kind != messageError) {
			message.dismissed = true
		}
		messages = append(messages, message)
	}
	message := appMessage{source: source, kind: kind, text: text, at: messageNow()}
	messages = append(messages, message)
	if len(messages) > maxMessages {
		messages = messages[len(messages)-maxMessages:]
	}
	m.messages = messages
	switch {
	case kind == messageError && m.showsScreenErrors():
		m.screenError, m.screenErrorMode = message.displayText(), m.mode
	case kind == messageSuccess:
		m.screenError = ""
	}
}

var screensShowingTheHeader = []ViewMode{ViewDashboard, ViewInlineEdit, ViewNotifyInput, ViewRecreateRow, ViewActionMenu, ViewLinkMenu, ViewError}

func (m Model) showsScreenErrors() bool {
	return !slices.Contains(screensShowingTheHeader, m.mode)
}

func (m *Model) clearScreenErrorOffScreen() {
	if m.screenError != "" && m.mode != m.screenErrorMode {
		m.screenError = ""
	}
}

func (m Model) screenErrorNotice(width int) string {
	if m.screenError == "" || m.mode != m.screenErrorMode {
		return ""
	}
	return messageErrorStyle.Render(ansi.Truncate("✗ "+m.screenError, width, "…"))
}

func (message appMessage) displayText() string {
	text := strings.ReplaceAll(message.text, "\n", " ")
	switch message.source {
	case messageSourceGit:
		return "git " + text
	case messageSourceClipboard:
		if message.kind != messageSuccess {
			return message.source + ": " + text
		}
		return text
	}
	return strings.ToLower(message.source) + ": " + text
}

func (m *Model) dismissErrorMessages() bool {
	dismissed := false
	messages := make([]appMessage, len(m.messages))
	for index, message := range m.messages {
		if message.kind == messageError && !message.dismissed {
			message.dismissed, dismissed = true, true
		}
		messages[index] = message
	}
	m.messages = messages
	return dismissed
}

func (message appMessage) visible(now time.Time) bool {
	if message.dismissed {
		return false
	}
	return message.kind != messageSuccess || now.Sub(message.at) < successMessageLife
}

func (m Model) activeMessage() (appMessage, bool) {
	now := messageNow()
	for index := len(m.messages) - 1; index >= 0; index-- {
		if m.messages[index].visible(now) {
			return m.messages[index], true
		}
	}
	return appMessage{}, false
}

func (m Model) renderActiveMessage(room int) string {
	message, found := m.activeMessage()
	if !found || room < 4 {
		return ""
	}
	text := ansi.Truncate(message.displayText(), room-2, "…")
	switch message.kind {
	case messageProgress:
		return syncPulseStyles[m.syncPulseFrame%len(syncPulseStyles)].Render(progressFrames[m.syncPulseFrame%len(progressFrames)]) + " " + messageProgressText.Render(text)
	case messageSuccess:
		return messageSuccessStyle.Render("✓ " + text)
	}
	return messageErrorStyle.Render("✗ " + text)
}

func (m Model) messageExpiryCmd() tea.Cmd {
	now := messageNow()
	var nextExpiry time.Duration
	for _, message := range m.messages {
		if message.kind == messageSuccess && message.visible(now) {
			if remaining := successMessageLife - now.Sub(message.at); nextExpiry == 0 || remaining < nextExpiry {
				nextExpiry = remaining
			}
		}
	}
	if nextExpiry <= 0 {
		return nil
	}
	return tea.Tick(nextExpiry, func(time.Time) tea.Msg { return messageExpiryMsg{} })
}

func (message appMessage) logLine() string {
	return message.at.Format("15:04:05") + "  " + message.source + ": " + message.text
}

func (m Model) messageLogLines() []string {
	lines := make([]string, 0, len(m.messages))
	for index := len(m.messages) - 1; index >= 0; index-- {
		lines = append(lines, m.messages[index].logLine())
	}
	return lines
}

func (m Model) renderMessageLogEntry(message appMessage, width int) string {
	text := message.logLine()
	if m.messageLogExpanded {
		text = strings.ReplaceAll(lipgloss.NewStyle().Width(max(width-2, 1)).Render(text), "\n", "\n  ")
	} else {
		text = ansi.Truncate(strings.ReplaceAll(text, "\n", " "), width-2, "…")
	}
	switch message.kind {
	case messageSuccess:
		return messageSuccessStyle.Render("✓ " + text)
	case messageError:
		return messageErrorStyle.Render("✗ " + text)
	}
	marker := messageLogProgressStyle.Render("•")
	if !message.dismissed {
		marker = syncPulseStyles[m.syncPulseFrame%len(syncPulseStyles)].Render(progressFrames[m.syncPulseFrame%len(progressFrames)])
	}
	return marker + " " + messageLogProgressStyle.Render(text)
}

func (m Model) messageLogEntries(width int) string {
	entries := make([]string, 0, len(m.messages))
	for index := len(m.messages) - 1; index >= 0; index-- {
		entries = append(entries, m.renderMessageLogEntry(m.messages[index], width))
	}
	return strings.Join(entries, "\n")
}

func (m Model) messageLogView() viewport.Model {
	width := modalWidthFor(m.width) - 6
	fixedHeight := 3 + lipgloss.Height(renderModalFooter(footerItemsFrom(m.errorBindings()), width))
	view := m.messageLogViewport
	view.Width, view.Height = width, max(3, previewContentHeight(m.height)-fixedHeight)
	view.SetContent(m.messageLogEntries(width))
	return view
}

func (m Model) renderMessageLog(width int) string {
	if m.messageLogExpanded {
		return m.messageLogView().View()
	}
	return m.messageLogEntries(width)
}

func (m Model) openMessageLog(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.errorReturnMode = m.mode
	m.errorTitle = messageLogTitle
	m.errorLines = nil
	m.messageLogExpanded = false
	m.mode = ViewError
	return m, nil
}

func (m Model) expandMessageLog(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.messageLogExpanded = true
	m.messageLogViewport = viewport.Model{}
	return m, nil
}

func (m Model) scrollMessageLog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	view := m.messageLogView()
	scrollViewport(&view, msg.String())
	m.messageLogViewport = view
	return m, nil
}

func (m *Model) noteGitSyncDone() {
	if m.git.loadingGit || m.git.loadingCommits {
		return
	}
	if !m.gitSyncInProgress() {
		return
	}
	if len(m.git.syncErrors) == 0 {
		m.postMessage(messageSourceGit, messageSuccess, "synced")
		m.git.prAlertsDue = true
		return
	}
	var failures []string
	for _, section := range slices.Sorted(maps.Keys(m.git.syncErrors)) {
		failures = append(failures, section+": "+m.git.syncErrors[section])
	}
	m.postMessage(messageSourceGit, messageError, "sync failed · "+strings.Join(failures, " · "))
}

func (m Model) gitSyncInProgress() bool {
	for index := len(m.messages) - 1; index >= 0; index-- {
		if message := m.messages[index]; message.source == messageSourceGit && !message.dismissed {
			return message.kind == messageProgress
		}
	}
	return false
}
