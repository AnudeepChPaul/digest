package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

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

func renderJobStyleRow(icon, label, rightBlock string, selected bool, width int) string {
	rightColWidth := 26
	if width < 60 {
		rightColWidth = 20
	}
	leftWidth := max(width-rightColWidth, 15)
	label = ansi.Truncate(label, max(leftWidth-5, 5), "…")
	labelText := itemStyle.Render(label)
	if selected {
		labelText = selectedTitle(label)
	}
	leftBlock := "   " + icon + " " + labelText
	leftPadding := max(leftWidth-lipgloss.Width(leftBlock), 0)
	return leftBlock + safeRepeat(" ", leftPadding) + underlinedWhen(selected, rightBlock) + "\n"
}
