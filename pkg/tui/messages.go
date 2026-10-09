package tui

import (
	"maps"
	"slices"
	"strings"
	"time"

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
	messageSourceGit   = "git"
	successMessageLife = 4 * time.Second
	maxMessages        = 50
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
)

func (m *Model) postMessage(source string, kind messageKind, text string) {
	messages := make([]appMessage, 0, len(m.messages)+1)
	for _, message := range m.messages {
		if message.source == source && (kind == messageSuccess || message.kind != messageError) {
			message.dismissed = true
		}
		messages = append(messages, message)
	}
	messages = append(messages, appMessage{source: source, kind: kind, text: text, at: messageNow()})
	if len(messages) > maxMessages {
		messages = messages[len(messages)-maxMessages:]
	}
	m.messages = messages
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
	text := strings.ReplaceAll(message.text, "\n", " ")
	if message.source != messageSourceGit {
		text = strings.ToLower(message.source) + ": " + text
	} else {
		text = "git " + text
	}
	text = ansi.Truncate(text, room-2, "…")
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

func (m Model) openMessageLog(tea.KeyMsg) (tea.Model, tea.Cmd) {
	var lines []string
	for index := len(m.messages) - 1; index >= 0; index-- {
		message := m.messages[index]
		lines = append(lines, message.at.Format("15:04:05")+"  "+message.source+": "+message.text)
	}
	if len(lines) == 0 {
		lines = []string{"No messages yet."}
	}
	m.errorReturnMode = m.mode
	m.errorTitle = "MESSAGES"
	m.errorLines = lines
	m.mode = ViewError
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
