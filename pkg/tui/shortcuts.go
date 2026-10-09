package tui

import (
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type helpEntry struct {
	section string
	key     string
	text    string
}

const (
	generalSection = "GENERAL"
	byRowSection   = "BY ROW"
)

var actionDescriptions = map[keyAction]string{
	actionCursorDown:            "move down",
	actionCursorUp:              "move up",
	actionPreviousDay:           "previous day",
	actionNextDay:               "next day",
	actionToday:                 "today",
	actionNewNote:               "new note",
	actionOpenPreview:           "preview the selected row",
	actionOpenArchive:           "open archive",
	actionReloadNotes:           "reload notes",
	actionSync:                  "run git",
	actionRefreshCommits:        "refresh commits",
	actionSwitchGitColumn:       "switch git column",
	actionHalfPageDown:          "half page down",
	actionHalfPageUp:            "half page up",
	actionOpenSearch:            "search",
	actionOpenBrag:              "brag",
	actionQuit:                  "quit",
	actionToggleSortField:       "sort field (pending PRs)",
	actionToggleSortOrder:       "sort order (pending PRs)",
	actionTogglePendingScope:    "me only (pending PRs)",
	actionDismissErrors:         "dismiss errors",
	actionOpenMessages:          "all messages",
	actionOpenSetup:             "setup",
	actionClosePreview:          "close",
	actionPreviewPrevious:       "previous item",
	actionPreviewNext:           "next item",
	actionCopyPreviewItem:       "copy",
	actionSwitchPreviewTab:      "switch tab",
	actionToggleFinding:         "select finding",
	actionSelectAllFindings:     "select all findings",
	actionPostReview:            "post review",
	actionFindingDown:           "next finding",
	actionFindingUp:             "previous finding",
	actionCloseSearchPreview:    "close",
	actionSearchPreviewNext:     "next result",
	actionSearchPreviewPrevious: "previous result",
	actionEditSearchResult:      "edit",
	actionCopySearchResult:      "copy",
	actionDeleteSearchResult:    "archive",
}

type rowMeaning struct {
	key      string
	meanings []string
}

func (m Model) rowMeanings() []rowMeaning {
	rows := []rowMeaning{
		{"enter", []string{"note: edit", "PR: open in browser", "my PR: open in browser", "job: preview", "run: preview"}},
		{"space", []string{"note: mark done", "done note: mark active"}},
		{"i", []string{"note: edit inline"}},
		{"d", []string{"note: archive", "PR: reject, or stop its review", "job: dry run, or stop it", "run: stop or dismiss"}},
		{"y", []string{"PR: approve"}},
		{"r", []string{"PR: review", "job: run job"}},
		{"o", []string{"note: open its links", "PR: open the review clone in nvim"}},
		{"@|.", []string{"note: actions on the note"}},
	}
	if m.cfg.GitEnabled() {
		return rows
	}
	for index := range rows {
		rows[index].meanings = slices.DeleteFunc(slices.Clone(rows[index].meanings), func(meaning string) bool { return strings.Contains(meaning, "PR") })
	}
	return slices.DeleteFunc(rows, func(row rowMeaning) bool { return len(row.meanings) == 0 })
}

var rowActions = []keyAction{
	actionOpenItem, actionToggleDone, actionInlineEdit, actionDeleteItem, actionRunSelectedJob, actionOpenActions,
	actionOpenNoteLinks, actionDashboardApprove, actionDashboardReject, actionApprove, actionRejectOrStopReview,
	actionStartReview, actionOpenClone, actionDryRunSelectedJob, actionStopSelectedItem,
}

var keyDisplayNames = map[string]string{" ": "space", "down": "↓", "up": "↑", "left": "←", "right": "→"}

func bindingKeys(binding keyBinding) string {
	var keys []string
	for _, name := range binding.binding.Keys() {
		if display, mapped := keyDisplayNames[name]; mapped {
			name = display
		}
		if !slices.Contains(keys, name) {
			keys = append(keys, name)
		}
	}
	return strings.Join(keys, "|")
}

func actionEntries(section string, bindings []keyBinding, skip func(keyAction) bool) []helpEntry {
	var entries []helpEntry
	var seen []keyAction
	for _, binding := range bindings {
		if skip(binding.action) || slices.Contains(seen, binding.action) || !binding.binding.Enabled() && !binding.hidden {
			continue
		}
		text, described := actionDescriptions[binding.action]
		if !described {
			text = binding.binding.Help().Desc
		}
		if text == "" {
			continue
		}
		seen = append(seen, binding.action)
		entries = append(entries, helpEntry{section: section, key: bindingKeys(binding), text: text})
	}
	return entries
}

func (m Model) helpEntries() []helpEntry {
	switch m.helpReturnMode {
	case ViewPreview:
		return actionEntries("PREVIEW", m.previewBindings(), func(action keyAction) bool { return action == actionOpenHelp })
	case ViewSearchPreview:
		return actionEntries("SEARCH PREVIEW", searchPreviewBindings(), func(action keyAction) bool { return action == actionOpenHelp })
	}
	entries := actionEntries(generalSection, m.dashboardBindings(), func(action keyAction) bool {
		return action == actionOpenHelp || slices.Contains(rowActions, action)
	})
	for _, row := range m.rowMeanings() {
		entries = append(entries, helpEntry{section: byRowSection, key: row.key, text: strings.Join(row.meanings, " · ")})
	}
	return entries
}

func (m Model) openHelp(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.helpReturnMode = m.mode
	m.mode = ViewHelp
	return m, nil
}

func (m Model) closeHelp(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = m.helpReturnMode
	m.helpReturnMode = ViewDashboard
	return m, nil
}

const sideBySideHelpWidth = 100

func helpSectionLines(entries []helpEntry, textWidth int) []string {
	keyWidth := 0
	for _, entry := range entries {
		keyWidth = max(keyWidth, lipgloss.Width(entry.key))
	}
	var lines []string
	section := ""
	for _, entry := range entries {
		if entry.section != section {
			if section != "" {
				lines = append(lines, "")
			}
			section = entry.section
			lines = append(lines, subSectionStyle.Render(section))
		}
		wrapped := strings.Split(lipgloss.NewStyle().Width(max(textWidth-keyWidth-5, 12)).Render(entry.text), "\n")
		for index, line := range wrapped {
			keyCell := safeRepeat(" ", keyWidth)
			if index == 0 {
				keyCell = keyStyle.Render(entry.key + safeRepeat(" ", keyWidth-lipgloss.Width(entry.key)))
			}
			lines = append(lines, "  "+keyCell+"   "+itemStyle.Render(strings.TrimRight(line, " ")))
		}
	}
	return lines
}

func (m Model) renderHelp(modalWidth int) string {
	entries := m.helpEntries()
	innerWidth := modalWidth - 6
	var body string
	general := slices.DeleteFunc(slices.Clone(entries), func(entry helpEntry) bool { return entry.section == byRowSection })
	byRow := slices.DeleteFunc(slices.Clone(entries), func(entry helpEntry) bool { return entry.section != byRowSection })
	if len(byRow) > 0 && innerWidth >= sideBySideHelpWidth {
		leftWidth := innerWidth * 2 / 5
		left := lipgloss.NewStyle().Width(leftWidth).Render(strings.Join(helpSectionLines(general, leftWidth), "\n"))
		right := strings.Join(helpSectionLines(byRow, innerWidth-leftWidth), "\n")
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	} else {
		body = strings.Join(helpSectionLines(entries, innerWidth), "\n")
	}
	content := lipgloss.JoinVertical(lipgloss.Left,
		modalTitleStyle.Render(" SHORTCUTS "),
		"",
		body,
		"",
		renderModalFooter(footerItemsFrom(helpBindings()), innerWidth),
	)
	return m.placeBragModal(content, modalWidth)
}
