package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func previewModalSize(width, height int) (int, int, int) {
	modalWidth := modalWidthFor(width)
	innerHeight := height - 14
	if innerHeight < 4 {
		innerHeight = 4
	}
	return modalWidth, modalWidth - 6, innerHeight
}

func previewModalHeight(terminalHeight int) int {
	return max(12, terminalHeight-4)
}

func previewContentHeight(terminalHeight int) int {
	return previewModalHeight(terminalHeight) - modalStyle.GetVerticalFrameSize()
}

func visibleGitRows(selected, total, rowsAvailable int) (first, last int) {
	if rowsAvailable < 1 {
		rowsAvailable = 1
	}
	if total <= rowsAvailable {
		return 0, total
	}
	first = max(0, selected-rowsAvailable+1)
	return first, first + rowsAvailable
}

func modalWidthFor(digestWidth int) int {
	return min(digestWidth*90/100, digestWidth-modalStyle.GetHorizontalBorderSize())
}

type framedPopupMemo struct {
	content    string
	modalWidth int
	width      int
	height     int
	framed     string
}

func (m Model) framedPopup(content string, modalWidth int) string {
	if notice := m.screenErrorNotice(modalWidth - 6); notice != "" {
		content += "\n\n" + notice
	}
	memo := m.popupMemo
	if memo != nil && memo.content == content && memo.modalWidth == modalWidth && memo.width == m.width && memo.height == m.height {
		return memo.framed
	}
	framed := lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, fitPopup(modalStyle.Width(modalWidth).Render(content), m.width, m.height))
	if memo != nil {
		*memo = framedPopupMemo{content: content, modalWidth: modalWidth, width: m.width, height: m.height, framed: framed}
	}
	return framed
}

func fitPopup(popup string, terminalWidth, terminalHeight int) string {
	lines := strings.Split(popup, "\n")
	if len(lines) > terminalHeight {
		keptBottom := terminalHeight / 2
		lines = append(lines[:terminalHeight-keptBottom], lines[len(lines)-keptBottom:]...)
	}
	for index, line := range lines {
		if lipgloss.Width(line) > terminalWidth {
			lines[index] = ansi.Truncate(line, terminalWidth, "")
		}
	}
	return strings.Join(lines, "\n")
}
