package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type footerItem struct {
	key    string
	action string
	isWarn bool
}

func capitaliseWord(word string) string {
	if word == "" {
		return word
	}
	return strings.ToUpper(word[:1]) + word[1:]
}

func splitFooterAction(action string) (firstLine, secondLine string) {
	words := strings.Fields(action)
	for index, word := range words {
		words[index] = capitaliseWord(word)
	}
	if len(words) == 0 {
		return "", ""
	}
	return words[0], strings.Join(words[1:], " ")
}

func footerLineCount(items []footerItem) int {
	for _, item := range items {
		if _, secondLine := splitFooterAction(item.action); secondLine != "" {
			return 3
		}
	}
	return 2
}

func renderFooterLines(items []footerItem) (keysLine, firstActionLine, secondActionLine string) {
	var keysParts, firstParts, secondParts []string
	for _, item := range items {
		firstWord, secondWord := splitFooterAction(item.action)
		width := max(lipgloss.Width(item.key), lipgloss.Width(firstWord), lipgloss.Width(secondWord))
		keyPadded := item.key + safeRepeat(" ", width-lipgloss.Width(item.key))
		firstPadded := firstWord + safeRepeat(" ", width-lipgloss.Width(firstWord))
		secondPadded := secondWord + safeRepeat(" ", width-lipgloss.Width(secondWord))
		currentKeyStyle, currentActionStyle := keyStyle, actionStyle
		if item.isWarn {
			currentKeyStyle, currentActionStyle = warnKeyStyle, warnActionStyle
		}
		keysParts = append(keysParts, currentKeyStyle.Render(keyPadded))
		firstParts = append(firstParts, currentActionStyle.Render(firstPadded))
		secondParts = append(secondParts, currentActionStyle.Render(secondPadded))
	}
	return strings.Join(keysParts, "   "), strings.Join(firstParts, "   "), strings.Join(secondParts, "   ")
}

func renderModalFooter(items []footerItem, maxWidth int) string {
	var renderedRows []string
	for _, row := range splitFooterRows(items, maxWidth) {
		keysLine, firstActionLine, secondActionLine := renderFooterLines(row)
		if footerLineCount(row) == 2 {
			renderedRows = append(renderedRows, keysLine+"\n"+firstActionLine)
		} else {
			renderedRows = append(renderedRows, keysLine+"\n"+firstActionLine+"\n"+secondActionLine)
		}
	}
	return strings.Join(renderedRows, "\n\n")
}

func (m Model) archiveFooterItems() []footerItem {
	return footerItemsFrom(m.archivedBindings())
}

func (m Model) dashboardBodyHeight() int {
	return m.height - lipgloss.Height(headerSection{}.Render(m)) - lipgloss.Height(m.renderFooter())
}

func (m Model) footerLines() []string {
	var lines []string
	if m.ctrlCCount > 0 {
		var warnings []footerItem
		for _, binding := range m.dashboardBindings() {
			if binding.action == actionQuit {
				warnings = footerItemsFrom([]keyBinding{binding})
			}
		}
		keysLine, firstActionLine, secondActionLine := renderFooterLines(warnings)
		lines = append(lines, keysLine, firstActionLine)
		if footerLineCount(warnings) == 3 {
			lines = append(lines, secondActionLine)
		}
	}
	return lines
}

func (m Model) renderFooter() string {
	lines := m.footerLines()
	bottomBorder := borderStyle.Render("└" + safeRepeat("─", m.width-2) + "┘")
	if len(lines) == 0 {
		return bottomBorder
	}
	topBorder := borderStyle.Render("├" + safeRepeat("─", m.width-2) + "┤")

	rows := []string{topBorder}
	for _, line := range lines {
		padded := " " + line + " "
		rows = append(rows, "│"+padded+safeRepeat(" ", m.width-lipgloss.Width(padded)-2)+"│")
	}
	rows = append(rows, bottomBorder)
	return strings.Join(rows, "\n")
}

func footerColumnWidth(item footerItem) int {
	firstWord, secondWord := splitFooterAction(item.action)
	return max(lipgloss.Width(item.key), lipgloss.Width(firstWord), lipgloss.Width(secondWord))
}

func splitFooterRows(items []footerItem, maxWidth int) [][]footerItem {
	var rows [][]footerItem
	var currentRow []footerItem
	rowWidth := 0
	for _, item := range items {
		columnWidth := footerColumnWidth(item)
		if len(currentRow) > 0 && rowWidth+3+columnWidth > maxWidth {
			rows = append(rows, currentRow)
			currentRow, rowWidth = nil, 0
		}
		if len(currentRow) > 0 {
			rowWidth += 3
		}
		currentRow = append(currentRow, item)
		rowWidth += columnWidth
	}
	if len(currentRow) > 0 {
		rows = append(rows, currentRow)
	}
	return rows
}
