package textarea

import (
	"strings"
	"unicode"
)

func dedent(raw string) string {
	skipFirstLine := true
	if strings.HasPrefix(raw, "\n") {
		raw, skipFirstLine = raw[1:], false
	}
	lines := strings.Split(raw, "\n")
	minIndent := int(^uint(0) >> 1)
	for index, line := range lines {
		if index == 0 && skipFirstLine {
			continue
		}
		indent := len(line) - len(strings.TrimLeftFunc(line, unicode.IsSpace))
		switch {
		case indent == len(line) && index == len(lines)-1 && indent < minIndent:
			lines[index] = ""
		case indent < len(line) && indent < minIndent:
			minIndent = indent
		}
	}
	for index, line := range lines {
		if index == 0 && skipFirstLine {
			continue
		}
		if len(line) >= minIndent {
			lines[index] = line[minIndent:]
		}
	}
	return strings.Join(lines, "\n")
}
