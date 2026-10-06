package tui

import "strings"

const (
	underlineOn  = "\x1b[4;58;2;205;214;244m"
	underlineOff = "\x1b[24;59m"
	styleReset   = "\x1b[0m"
)

func underlined(block string) string {
	core := strings.TrimLeft(block, " ")
	leading := block[:len(block)-len(core)]
	trimmed := strings.TrimRight(core, " ")
	if trimmed == "" {
		return block
	}
	trailing := core[len(trimmed):]
	return leading + underlineOn + strings.ReplaceAll(trimmed, styleReset, styleReset+underlineOn) + underlineOff + trailing
}

func selectedTitle(text string) string {
	return underlined(selectedSummaryStyle.Render(text))
}

func underlinedWhen(selected bool, block string) string {
	if selected {
		return underlined(block)
	}
	return block
}
