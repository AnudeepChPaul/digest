package textarea

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rivo/uniseg"
)

func upstreamView(m Model) string {
	if m.Value() == "" && m.row == 0 && m.col == 0 && m.Placeholder != "" {
		return m.placeholderView()
	}
	m.Cursor.TextStyle = m.style.computedCursorLine()

	var (
		s                strings.Builder
		style            lipgloss.Style
		newLines         int
		widestLineNumber int
		lineInfo         = m.LineInfo()
	)

	displayLine := 0
	for l, line := range m.value {
		wrappedLines := m.memoizedWrap(line, m.width)

		if m.row == l {
			style = m.style.computedCursorLine()
		} else {
			style = m.style.computedText()
		}

		for wl, wrappedLine := range wrappedLines {
			prompt := m.getPromptString(displayLine)
			prompt = m.style.computedPrompt().Render(prompt)
			s.WriteString(style.Render(prompt))
			displayLine++

			var ln string
			if m.ShowLineNumbers { //nolint:nestif
				if wl == 0 {
					if m.row == l {
						ln = style.Render(m.style.computedCursorLineNumber().Render(m.formatLineNumber(l + 1)))
						s.WriteString(ln)
					} else {
						ln = style.Render(m.style.computedLineNumber().Render(m.formatLineNumber(l + 1)))
						s.WriteString(ln)
					}
				} else {
					if m.row == l {
						ln = style.Render(m.style.computedCursorLineNumber().Render(m.formatLineNumber(" ")))
						s.WriteString(ln)
					} else {
						ln = style.Render(m.style.computedLineNumber().Render(m.formatLineNumber(" ")))
						s.WriteString(ln)
					}
				}
			}

			// Note the widest line number for padding purposes later.
			lnw := lipgloss.Width(ln)
			if lnw > widestLineNumber {
				widestLineNumber = lnw
			}

			strwidth := uniseg.StringWidth(string(wrappedLine))
			padding := m.width - strwidth
			// If the trailing space causes the line to be wider than the
			// width, we should not draw it to the screen since it will result
			// in an extra space at the end of the line which can look off when
			// the cursor line is showing.
			if strwidth > m.width {
				// The character causing the line to be wider than the width is
				// guaranteed to be a space since any other character would
				// have been wrapped.
				wrappedLine = []rune(strings.TrimSuffix(string(wrappedLine), " "))
				padding -= m.width - strwidth
			}
			if m.row == l && lineInfo.RowOffset == wl {
				s.WriteString(style.Render(string(wrappedLine[:lineInfo.ColumnOffset])))
				if m.col >= len(line) && lineInfo.CharOffset >= m.width {
					m.Cursor.SetChar(" ")
					s.WriteString(m.Cursor.View())
				} else {
					m.Cursor.SetChar(string(wrappedLine[lineInfo.ColumnOffset]))
					s.WriteString(style.Render(m.Cursor.View()))
					s.WriteString(style.Render(string(wrappedLine[lineInfo.ColumnOffset+1:])))
				}
			} else {
				s.WriteString(style.Render(string(wrappedLine)))
			}
			s.WriteString(style.Render(strings.Repeat(" ", max(0, padding))))
			s.WriteRune('\n')
			newLines++
		}
	}

	// Always show at least `m.Height` lines at all times.
	// To do this we can simply pad out a few extra new lines in the view.
	for i := 0; i < m.height; i++ {
		prompt := m.getPromptString(displayLine)
		prompt = m.style.computedPrompt().Render(prompt)
		s.WriteString(prompt)
		displayLine++

		// Write end of buffer content
		leftGutter := string(m.EndOfBufferCharacter)
		rightGapWidth := m.Width() - lipgloss.Width(leftGutter) + widestLineNumber
		rightGap := strings.Repeat(" ", max(0, rightGapWidth))
		s.WriteString(m.style.computedEndOfBuffer().Render(leftGutter + rightGap))
		s.WriteRune('\n')
	}

	m.viewport.SetContent(s.String())
	return m.style.Base.Render(m.viewport.View())
}

func longWrappedText(lines int) string {
	var text strings.Builder
	for line := 0; line < lines; line++ {
		fmt.Fprintf(&text, "line %d %s\n", line, strings.Repeat("word ", line%9))
	}
	return text.String()
}

func TestVisibleWindowViewMatchesTheFullRender(t *testing.T) {
	for _, showLineNumbers := range []bool{true, false} {
		area := New()
		area.ShowLineNumbers = showLineNumbers
		area.MaxHeight = 0
		area.SetWidth(30)
		area.SetHeight(8)
		area.Focus()
		area.SetValue(longWrappedText(120))
		steps := []tea.KeyMsg{
			{Type: tea.KeyCtrlHome},
			{Type: tea.KeyDown}, {Type: tea.KeyDown}, {Type: tea.KeyDown},
			{Type: tea.KeyPgDown}, {Type: tea.KeyPgDown},
			{Type: tea.KeyRunes, Runes: []rune("typed")},
			{Type: tea.KeyEnter},
			{Type: tea.KeyCtrlEnd},
			{Type: tea.KeyBackspace},
			{Type: tea.KeyUp}, {Type: tea.KeyUp},
		}
		scrolled := false
		for index, step := range steps {
			area, _ = area.Update(step)
			got := area.View()
			want := upstreamView(area)
			if got != want {
				t.Fatalf("line numbers %v, step %d (%v): visible-window view differs\nwant:\n%s\ngot:\n%s", showLineNumbers, index, step, want, got)
			}
			scrolled = scrolled || area.viewport.YOffset > 0
		}
		if !scrolled {
			t.Errorf("line numbers %v: the steps never scrolled the area", showLineNumbers)
		}
	}
}

func TestEmptyAreaShowsThePlaceholder(t *testing.T) {
	area := New()
	area.Placeholder = "write here"
	area.SetWidth(30)
	area.SetHeight(4)
	if view := area.View(); !strings.Contains(view, "write here") {
		t.Errorf("empty area should show the placeholder:\n%s", view)
	}
	area.SetValue("\n")
	if view := area.View(); strings.Contains(view, "write here") {
		t.Errorf("an area holding a newline is not empty:\n%s", view)
	}
}
