package tui

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	markdownTitleStyle   = lipgloss.NewStyle().Bold(true).Foreground(colourMauve)
	markdownHeadingStyle = subSectionStyle
	markdownBoldStyle    = boldTextStyle
	markdownItalicStyle  = lipgloss.NewStyle().Italic(true).Foreground(colourText)
	markdownCodeStyle    = lipgloss.NewStyle().Foreground(colourPeach)
	markdownLinkStyle    = lipgloss.NewStyle().Underline(true).Foreground(colourSapphire)
	markdownQuoteStyle   = lipgloss.NewStyle().Italic(true).Foreground(colourSubtext)
	markdownMarkerStyle  = lipgloss.NewStyle().Foreground(colourBlue)
)

var renderMarkdown = func(body string, width int) string {
	if strings.TrimSpace(body) == "" {
		return mutedStyle.Render("(No note body text)")
	}
	var rendered []string
	inFence := false
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			rendered = append(rendered, "  "+markdownCodeStyle.Render(strings.TrimRight(line, " \t")))
			continue
		}
		if trimmed == "" {
			if len(rendered) > 0 && rendered[len(rendered)-1] != "" {
				rendered = append(rendered, "")
			}
			continue
		}
		rendered = append(rendered, renderMarkdownLine(line, width)...)
	}
	for len(rendered) > 0 && rendered[len(rendered)-1] == "" {
		rendered = rendered[:len(rendered)-1]
	}
	return strings.Join(rendered, "\n")
}

func renderMarkdownLine(line string, width int) []string {
	trimmed := strings.TrimSpace(line)
	if level, title := markdownHeading(trimmed); level > 0 {
		style := markdownHeadingStyle
		if level == 1 {
			style = markdownTitleStyle
		}
		return wrapMarkdown(style.Render(stripMarkdownInline(title)), "", "", width)
	}
	if isMarkdownRule(trimmed) {
		return []string{borderStyle.Render(strings.Repeat("─", max(width, 3)))}
	}
	if strings.HasPrefix(trimmed, ">") {
		quote := strings.TrimSpace(strings.TrimLeft(trimmed, "> "))
		bar := borderStyle.Render("│") + " "
		return wrapMarkdown(markdownQuoteStyle.Render(stripMarkdownInline(quote)), bar, bar, width)
	}
	indent := strings.Repeat("  ", markdownIndentLevel(line))
	if marker, item, ok := markdownListItem(trimmed); ok {
		pad := indent + strings.Repeat(" ", ansi.StringWidth(marker)+1)
		return wrapMarkdown(renderMarkdownInline(item), indent+markdownMarkerStyle.Render(marker)+" ", pad, width)
	}
	return wrapMarkdown(renderMarkdownInline(trimmed), indent, indent, width)
}

func markdownHeading(trimmed string) (int, string) {
	level := 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level == 0 || level > 6 || (level < len(trimmed) && trimmed[level] != ' ') {
		return 0, ""
	}
	return level, strings.TrimSpace(strings.TrimRight(trimmed[level:], "#"))
}

func isMarkdownRule(trimmed string) bool {
	compact := strings.ReplaceAll(trimmed, " ", "")
	if len(compact) < 3 {
		return false
	}
	return strings.Count(compact, compact[:1]) == len(compact) && strings.ContainsAny(compact[:1], "-*_")
}

func markdownIndentLevel(line string) int {
	spaces := 0
	for _, r := range line {
		switch r {
		case ' ':
			spaces++
		case '\t':
			spaces += 4
		default:
			return spaces / 2
		}
	}
	return 0
}

func markdownListItem(trimmed string) (marker, item string, ok bool) {
	if len(trimmed) > 1 && strings.ContainsRune("-*+", rune(trimmed[0])) && trimmed[1] == ' ' {
		item = strings.TrimSpace(trimmed[2:])
		switch {
		case strings.HasPrefix(item, "[ ] "):
			return "☐", item[4:], true
		case strings.HasPrefix(item, "[x] "), strings.HasPrefix(item, "[X] "):
			return "✔", item[4:], true
		}
		return "•", item, true
	}
	digits := 0
	for digits < len(trimmed) && trimmed[digits] >= '0' && trimmed[digits] <= '9' {
		digits++
	}
	if digits > 0 && digits+1 < len(trimmed) && strings.ContainsRune(".)", rune(trimmed[digits])) && trimmed[digits+1] == ' ' {
		number, _ := strconv.Atoi(trimmed[:digits])
		return strconv.Itoa(number) + ".", strings.TrimSpace(trimmed[digits+2:]), true
	}
	return "", "", false
}

func wrapMarkdown(text, firstPrefix, restPrefix string, width int) []string {
	limit := width - ansi.StringWidth(firstPrefix)
	if width <= 0 || limit < 1 {
		return []string{firstPrefix + text}
	}
	wrapped := strings.Split(ansi.Hardwrap(ansi.Wordwrap(text, limit, ""), limit, true), "\n")
	for index, line := range wrapped {
		prefix := restPrefix
		if index == 0 {
			prefix = firstPrefix
		}
		wrapped[index] = prefix + strings.TrimRight(line, " ")
	}
	return wrapped
}

func stripMarkdownInline(text string) string {
	return ansi.Strip(renderMarkdownInline(text))
}

func renderMarkdownInline(text string) string {
	var out strings.Builder
	runes := []rune(text)
	for index := 0; index < len(runes); index++ {
		current := runes[index]
		switch {
		case current == '`':
			if end := indexRune(runes, index+1, '`'); end > index+1 {
				out.WriteString(markdownCodeStyle.Render(string(runes[index+1 : end])))
				index = end
				continue
			}
		case (current == '*' || current == '_') && index+1 < len(runes) && runes[index+1] == current:
			if end := closingEmphasis(runes, index+2, string([]rune{current, current})); end > index+2 {
				out.WriteString(markdownBoldStyle.Render(ansi.Strip(renderMarkdownInline(string(runes[index+2 : end])))))
				index = end + 1
				continue
			}
		case current == '*' || (current == '_' && (index == 0 || !isWordRune(runes[index-1]))):
			if end := closingEmphasis(runes, index+1, string(current)); end > index+1 {
				out.WriteString(markdownItalicStyle.Render(ansi.Strip(renderMarkdownInline(string(runes[index+1 : end])))))
				index = end
				continue
			}
		case current == '[':
			if label, target, end, ok := markdownLink(runes, index); ok {
				out.WriteString(markdownLinkStyle.Render(label))
				if target != label {
					out.WriteString(" " + mutedStyle.Render("("+target+")"))
				}
				index = end
				continue
			}
		}
		out.WriteRune(current)
	}
	return out.String()
}

func indexRune(runes []rune, from int, target rune) int {
	for index := from; index < len(runes); index++ {
		if runes[index] == target {
			return index
		}
	}
	return -1
}

func closingEmphasis(runes []rune, from int, delimiter string) int {
	if from >= len(runes) || unicode.IsSpace(runes[from]) {
		return -1
	}
	width := len([]rune(delimiter))
	for index := from + 1; index+width <= len(runes); index++ {
		if string(runes[index:index+width]) != delimiter || unicode.IsSpace(runes[index-1]) {
			continue
		}
		if delimiter[0] == '_' && index+width < len(runes) && isWordRune(runes[index+width]) {
			continue
		}
		return index
	}
	return -1
}

func markdownLink(runes []rune, start int) (label, target string, end int, ok bool) {
	closeLabel := indexRune(runes, start+1, ']')
	if closeLabel < 0 || closeLabel+1 >= len(runes) || runes[closeLabel+1] != '(' {
		return "", "", 0, false
	}
	closeTarget := indexRune(runes, closeLabel+2, ')')
	if closeTarget < 0 {
		return "", "", 0, false
	}
	return string(runes[start+1 : closeLabel]), string(runes[closeLabel+2 : closeTarget]), closeTarget, true
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
