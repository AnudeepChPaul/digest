package textarea

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func editorWithValue(value string, row, col int) Model {
	editor := New()
	editor.Prompt = ""
	editor.ShowLineNumbers = false
	editor.SetWidth(40)
	editor.Focus()
	editor.SetValue(value)
	editor.row = row
	editor.col = col
	return editor
}

func pressKey(editor Model, keyMsg tea.KeyMsg) Model {
	editor, _ = editor.Update(keyMsg)
	return editor
}

func altRune(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: true}
}

func assertEditorState(t *testing.T, editor Model, wantValue string, wantRow, wantCol int) {
	t.Helper()
	if editor.Value() != wantValue {
		t.Fatalf("value = %q, want %q", editor.Value(), wantValue)
	}
	if editor.row != wantRow || editor.col != wantCol {
		t.Fatalf("cursor = (%d,%d), want (%d,%d)", editor.row, editor.col, wantRow, wantCol)
	}
}

func TestEditingKeysPinTheirBehaviour(t *testing.T) {
	cases := []struct {
		name      string
		value     string
		row, col  int
		keyMsg    tea.KeyMsg
		wantValue string
		wantRow   int
		wantCol   int
	}{
		{"backspace mid-line", "hello", 0, 3, tea.KeyMsg{Type: tea.KeyBackspace}, "helo", 0, 2},
		{"ctrl+h mid-line", "hello", 0, 3, tea.KeyMsg{Type: tea.KeyCtrlH}, "helo", 0, 2},
		{"backspace at line start merges up", "ab\ncd", 1, 0, tea.KeyMsg{Type: tea.KeyBackspace}, "abcd", 0, 2},
		{"backspace at very start does nothing", "ab", 0, 0, tea.KeyMsg{Type: tea.KeyBackspace}, "ab", 0, 0},
		{"delete forward mid-line", "hello", 0, 1, tea.KeyMsg{Type: tea.KeyDelete}, "hllo", 0, 1},
		{"ctrl+d at line end merges down", "ab\ncd", 0, 2, tea.KeyMsg{Type: tea.KeyCtrlD}, "abcd", 0, 2},
		{"delete at the last line end does nothing", "ab", 0, 2, tea.KeyMsg{Type: tea.KeyDelete}, "ab", 0, 2},
		{"ctrl+k cuts to line end", "hello world", 0, 5, tea.KeyMsg{Type: tea.KeyCtrlK}, "hello", 0, 5},
		{"ctrl+k at line end merges down", "ab\ncd", 0, 2, tea.KeyMsg{Type: tea.KeyCtrlK}, "abcd", 0, 2},
		{"ctrl+u cuts to line start", "hello world", 0, 6, tea.KeyMsg{Type: tea.KeyCtrlU}, "world", 0, 0},
		{"ctrl+u at line start merges up", "ab\ncd", 1, 0, tea.KeyMsg{Type: tea.KeyCtrlU}, "abcd", 0, 2},
		{"alt+backspace deletes the previous word", "foo bar", 0, 7, tea.KeyMsg{Type: tea.KeyBackspace, Alt: true}, "foo ", 0, 4},
		{"ctrl+w deletes the only word", "foo", 0, 3, tea.KeyMsg{Type: tea.KeyCtrlW}, "", 0, 0},
		{"ctrl+w over trailing spaces deletes the word and spaces", "foo bar   ", 0, 10, tea.KeyMsg{Type: tea.KeyCtrlW}, "foo ", 0, 4},
		{"ctrl+w over only spaces clears them", "   ", 0, 3, tea.KeyMsg{Type: tea.KeyCtrlW}, "", 0, 0},
		{"ctrl+w at line start merges up", "ab\ncd", 1, 0, tea.KeyMsg{Type: tea.KeyCtrlW}, "abcd", 0, 2},
		{"alt+d deletes the next word", "foo bar", 0, 0, altRune('d'), " bar", 0, 0},
		{"alt+delete skips spaces then deletes the word", "foo   bar baz", 0, 3, tea.KeyMsg{Type: tea.KeyDelete, Alt: true}, "foo baz", 0, 3},
		{"alt+d at line end merges down", "ab\ncd", 0, 2, altRune('d'), "abcd", 0, 2},
		{"home jumps to line start", "hello", 0, 3, tea.KeyMsg{Type: tea.KeyHome}, "hello", 0, 0},
		{"ctrl+a jumps to line start", "hello", 0, 3, tea.KeyMsg{Type: tea.KeyCtrlA}, "hello", 0, 0},
		{"end jumps to line end", "hello", 0, 1, tea.KeyMsg{Type: tea.KeyEnd}, "hello", 0, 5},
		{"ctrl+e jumps to line end", "hello", 0, 1, tea.KeyMsg{Type: tea.KeyCtrlE}, "hello", 0, 5},
		{"right moves one character", "ab", 0, 0, tea.KeyMsg{Type: tea.KeyRight}, "ab", 0, 1},
		{"ctrl+f at line end wraps to next line", "ab\ncd", 0, 2, tea.KeyMsg{Type: tea.KeyCtrlF}, "ab\ncd", 1, 0},
		{"right at the end of text stays", "ab", 0, 2, tea.KeyMsg{Type: tea.KeyRight}, "ab", 0, 2},
		{"left moves one character", "ab", 0, 2, tea.KeyMsg{Type: tea.KeyLeft}, "ab", 0, 1},
		{"left at line start goes to previous line end", "ab\ncd", 1, 0, tea.KeyMsg{Type: tea.KeyLeft}, "ab\ncd", 0, 2},
		{"ctrl+b at the start of text stays", "ab", 0, 0, tea.KeyMsg{Type: tea.KeyCtrlB}, "ab", 0, 0},
		{"alt+left moves to the word start", "foo bar", 0, 7, tea.KeyMsg{Type: tea.KeyLeft, Alt: true}, "foo bar", 0, 4},
		{"alt+b crosses to the previous line word", "foo\nbar", 1, 0, altRune('b'), "foo\nbar", 0, 0},
		{"alt+right moves to the word end", "foo bar", 0, 0, tea.KeyMsg{Type: tea.KeyRight, Alt: true}, "foo bar", 0, 3},
		{"alt+f skips spaces to the next word end", "foo bar", 0, 3, altRune('f'), "foo bar", 0, 7},
		{"alt+f crosses to the next line", "foo\nbar", 0, 3, altRune('f'), "foo\nbar", 1, 3},
		{"alt+f at the end of text stays", "foo", 0, 3, altRune('f'), "foo", 0, 3},
		{"alt+u uppercases the next word", "hello world", 0, 0, altRune('u'), "HELLO world", 0, 5},
		{"alt+l lowercases the next word", "HELLO WORLD", 0, 5, altRune('l'), "HELLO world", 0, 11},
		{"alt+c capitalizes the next word", "hello world", 0, 0, altRune('c'), "Hello world", 0, 5},
		{"ctrl+t swaps with the previous character and advances", "abc", 0, 1, tea.KeyMsg{Type: tea.KeyCtrlT}, "bac", 0, 2},
		{"ctrl+t at line end swaps the last two characters", "ab", 0, 2, tea.KeyMsg{Type: tea.KeyCtrlT}, "ba", 0, 2},
		{"ctrl+t at line start does nothing", "ab", 0, 0, tea.KeyMsg{Type: tea.KeyCtrlT}, "ab", 0, 0},
		{"down on the last line keeps the row", "ab", 0, 1, tea.KeyMsg{Type: tea.KeyDown}, "ab", 0, 1},
		{"alt+< jumps to the start of input", "ab\ncd", 1, 2, altRune('<'), "ab\ncd", 0, 0},
		{"ctrl+end jumps to the end of input", "ab\ncd", 0, 0, tea.KeyMsg{Type: tea.KeyCtrlEnd}, "ab\ncd", 1, 2},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			editor := pressKey(editorWithValue(testCase.value, testCase.row, testCase.col), testCase.keyMsg)
			assertEditorState(t, editor, testCase.wantValue, testCase.wantRow, testCase.wantCol)
		})
	}
}

func TestMergingAMiddleLineShiftsTheRestUp(t *testing.T) {
	editor := pressKey(editorWithValue("a\nb\nc\nd", 1, 0), tea.KeyMsg{Type: tea.KeyBackspace})
	assertEditorState(t, editor, "ab\nc\nd", 0, 1)
	editor = pressKey(editorWithValue("a\nb\nc\nd", 1, 1), tea.KeyMsg{Type: tea.KeyDelete})
	assertEditorState(t, editor, "a\nbc\nd", 1, 1)
}

func TestWordHelpersIgnoreEdgesWhenCalledDirectly(t *testing.T) {
	editor := editorWithValue("foo", 0, 0)
	editor.deleteWordLeft()
	editor.col = 3
	editor.deleteWordRight()
	editor.mergeLineAbove(0)
	assertEditorState(t, editor, "foo", 0, 3)
}

func TestUnfocusedEditorIgnoresKeys(t *testing.T) {
	editor := editorWithValue("hello", 0, 5)
	editor.Blur()
	if editor.Focused() {
		t.Fatal("editor should report blurred")
	}
	editor = pressKey(editor, keyPress('x').(tea.KeyMsg))
	assertEditorState(t, editor, "hello", 0, 5)
}

func TestPasteMessagesInsertTextOrRecordTheError(t *testing.T) {
	editor := editorWithValue("ab", 0, 1)
	_, pasteCmd := editor.Update(tea.KeyMsg{Type: tea.KeyCtrlV})
	if pasteCmd == nil {
		t.Fatal("ctrl+v should return the paste command")
	}
	editor, _ = editor.Update(pasteMsg("XY"))
	assertEditorState(t, editor, "aXYb", 0, 3)
	clipboardFailure := errors.New("no clipboard")
	editor, _ = editor.Update(pasteErrMsg{clipboardFailure})
	if editor.Err == nil || editor.Err.Error() != clipboardFailure.Error() {
		t.Fatalf("Err = %v, want the clipboard failure", editor.Err)
	}
	switch Paste().(type) {
	case pasteMsg, pasteErrMsg:
	default:
		t.Fatal("Paste should return a paste or paste error message")
	}
}

func TestAccessorsReportCursorAndSize(t *testing.T) {
	editor := editorWithValue("ab\ncd", 1, 1)
	editor.InsertRune('z')
	if editor.Value() != "ab\nczd" || editor.LineCount() != 2 || editor.Line() != 1 {
		t.Fatalf("value=%q lines=%d line=%d", editor.Value(), editor.LineCount(), editor.Line())
	}
	editor.CursorStart()
	if editor.col != 0 {
		t.Fatalf("CursorStart col = %d", editor.col)
	}
	editor.CursorEnd()
	if editor.col != 3 {
		t.Fatalf("CursorEnd col = %d", editor.col)
	}
	editor.SetHeight(4)
	if editor.Height() != 4 {
		t.Fatalf("Height = %d", editor.Height())
	}
	if (Model{}).Value() != "" {
		t.Fatal("a zero model should have an empty value")
	}
	if Blink() == nil {
		t.Fatal("Blink should return a message")
	}
	if clamp(5, 10, 0) != 5 || clamp(-1, 10, 0) != 0 {
		t.Fatal("clamp should swap reversed bounds")
	}
}

func TestLineInfoIsEmptyWhenTheCursorIsPastTheLine(t *testing.T) {
	editor := editorWithValue("ab", 0, 50)
	if editor.LineInfo() != (LineInfo{}) {
		t.Fatalf("LineInfo = %+v, want zero", editor.LineInfo())
	}
}

func TestInsertingMoreThanMaxLinesKeepsTheLimit(t *testing.T) {
	editor := editorWithValue("", 0, 0)
	editor.InsertString(strings.Repeat("\n", maxLines+5))
	if editor.LineCount() != maxLines {
		t.Fatalf("LineCount = %d, want %d", editor.LineCount(), maxLines)
	}
}

func TestEnterStopsAtMaxHeight(t *testing.T) {
	editor := editorWithValue("a\nb", 1, 1)
	editor.MaxHeight = 2
	editor = pressKey(editor, tea.KeyMsg{Type: tea.KeyEnter})
	assertEditorState(t, editor, "a\nb", 1, 1)
}

func TestVerticalMovesInsideASoftWrappedLine(t *testing.T) {
	editor := editorWithValue(strings.Repeat("word ", 20), 0, 2)
	editor.SetWidth(12)
	editor = pressKey(editor, tea.KeyMsg{Type: tea.KeyDown})
	if editor.row != 0 || editor.LineInfo().RowOffset != 1 {
		t.Fatalf("down should stay on row 0 wrapped offset 1, got row %d offset %d", editor.row, editor.LineInfo().RowOffset)
	}
	editor = pressKey(editor, tea.KeyMsg{Type: tea.KeyUp})
	if editor.row != 0 || editor.LineInfo().RowOffset != 0 || editor.col != 2 {
		t.Fatalf("up should return to col 2, got row %d offset %d col %d", editor.row, editor.LineInfo().RowOffset, editor.col)
	}
}

func TestPromptFuncPadsShortPrompts(t *testing.T) {
	editor := editorWithValue("ab\ncd", 0, 0)
	editor.SetPromptFunc(4, func(lineIndex int) string {
		if lineIndex == 0 {
			return ">"
		}
		return "1234"
	})
	editor.SetWidth(20)
	editor.SetHeight(3)
	view := stripString(editor.View())
	if !strings.HasPrefix(view, "   >ab") {
		t.Fatalf("first line should be padded, got %q", view)
	}
	if !strings.Contains(view, "1234cd") {
		t.Fatalf("second line should keep the full prompt, got %q", view)
	}
}

func TestLineNumbersStayWideWhenNoLineIsVisible(t *testing.T) {
	editor := editorWithValue("a", 0, 0)
	editor.ShowLineNumbers = true
	editor.SetWidth(20)
	editor.SetHeight(2)
	editor.viewport.YOffset = 2
	if strings.Contains(stripString(editor.View()), "a") {
		t.Fatal("the scrolled view should not show the line")
	}
}

func TestAWideRuneThatOverflowsTheWidthIsClippedFromTheView(t *testing.T) {
	editor := editorWithValue("abcd世", 0, 0)
	editor.SetWidth(5)
	editor.SetHeight(3)
	view := stripString(editor.View())
	if !strings.HasPrefix(view, "abcd") || strings.Contains(view, "世") {
		t.Fatalf("view = %q, want the wide rune clipped at the width", view)
	}
	if editor.Value() != "abcd世" {
		t.Fatalf("value = %q, the wide rune should stay in the text", editor.Value())
	}
}

func TestCharLimitTrimsAndThenBlocksInput(t *testing.T) {
	editor := editorWithValue("", 0, 0)
	editor.CharLimit = 3
	editor.InsertString("abcdef")
	editor.InsertString("g")
	assertEditorState(t, editor, "abc", 0, 3)
}

func TestTabInsertsAnIndentAtTheCursor(t *testing.T) {
	editor := editorWithValue("ab", 0, 1)
	editor = pressKey(editor, tea.KeyMsg{Type: tea.KeyTab})
	assertEditorState(t, editor, "a    b", 0, 5)
}
