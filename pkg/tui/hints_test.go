package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func hintModel(t *testing.T, enabled bool) Model {
	t.Helper()
	m, _ := actionsTestModel(t)
	m.cfg.ShowKeyHints = enabled
	previous := hintIdleDelay
	hintIdleDelay = time.Millisecond
	t.Cleanup(func() { hintIdleDelay = previous })
	return m
}

func idle(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	for _, msg := range collectMsgs(cmd) {
		if hint, isHint := msg.(hintIdleMsg); isHint {
			next, _ := m.Update(hint)
			m = next.(Model)
		}
	}
	return m
}

func TestHintAppearsAfterIdleOnlyWhenEnabled(t *testing.T) {
	m := hintModel(t, true)
	m, staleCmd := pressKey(t, m, "j")
	m, cmd := pressKey(t, m, "k")
	if m = idle(t, m, staleCmd); m.hintVisible {
		t.Errorf("an older idle timer must not show the hint")
	}
	m = idle(t, m, cmd)
	lines := plainLines(m.View())
	row := slicesIndex(lines, "Flaky deploys")
	if !m.hintVisible || row < 0 || !strings.HasSuffix(lines[row+1], "╮ │") || !strings.Contains(lines[row+2], "(↵)open") || !strings.Contains(lines[row+2], "(.|@)actions") || !strings.HasSuffix(lines[row+2], "actions │ │") {
		t.Fatalf("hint pill should sit bordered at the right edge under the row:\n%s", strings.Join(lines[max(row, 0):row+4], "\n"))
	}
	if strings.Contains(m.keyHintPill(), "\x1b[48") {
		t.Errorf("hint pill should have no background")
	}
	if m, _ = pressKey(t, m, "j"); m.hintVisible {
		t.Errorf("the next key should hide the hint")
	}
	disabled := hintModel(t, false)
	disabled, cmd = pressKey(t, disabled, "k")
	if disabled = idle(t, disabled, cmd); disabled.hintVisible {
		t.Errorf("hints off should never show the pill")
	}
}

func slicesIndex(lines []string, text string) int {
	for index, line := range lines {
		if strings.Contains(line, text) {
			return index
		}
	}
	return -1
}

func prRowModel(t *testing.T, hints bool) Model {
	t.Helper()
	stubReviewControl(t)
	m := reviewTestModel(t)
	m.mode = ViewDashboard
	m.cfg.ShowKeyHints = hints
	for index, item := range m.allNavItems() {
		if item.Kind == KindPendingGit {
			m.selected = index
			return m
		}
	}
	t.Fatal("no PR row")
	return m
}

func TestPRRowTakesPreviewKeysWhenHintsAreOn(t *testing.T) {
	cases := map[string]ViewMode{"y": ViewReviewConfirm, "d": ViewReviewConfirm, "r": ViewReviewRunConfirm}
	for key, want := range cases {
		m := prRowModel(t, true)
		if m, _ = pressKey(t, m, key); m.mode != want {
			t.Errorf("%s on a PR row: mode %v, want %v", key, m.mode, want)
		}
		if m, _ = pressKey(t, m, "esc"); m.mode != ViewDashboard {
			t.Errorf("%s then esc should return to the dashboard, mode %v", key, m.mode)
		}
	}
	m := prRowModel(t, true)
	if hint := stripANSI(m.keyHintPill()); !strings.Contains(hint, "(y)approve") || !strings.Contains(hint, "(d)reject") || !strings.Contains(hint, "(r)review") {
		t.Errorf("PR hint = %q", hint)
	}
	off := prRowModel(t, false)
	if off, _ = pressKey(t, off, "y"); off.mode != ViewDashboard {
		t.Errorf("with hints off y does nothing on the dashboard, mode %v", off.mode)
	}
}

func TestHintDelayIsAQuarterSecondAndShowsAtStartup(t *testing.T) {
	if hintIdleDelay != 250*time.Millisecond {
		t.Errorf("hint delay = %v, want 250ms", hintIdleDelay)
	}
	m := hintModel(t, true)
	m = idle(t, m, m.startupHintCmd())
	if !m.hintVisible {
		t.Errorf("the initially selected row should get its hint")
	}
	if off := hintModel(t, false); off.startupHintCmd() != nil {
		t.Errorf("hints off should not start a timer")
	}
}

func TestHintTextIsCompact(t *testing.T) {
	m := hintModel(t, true)
	hint := stripANSI(m.keyHintPill())
	if !strings.Contains(hint, "(↵)open (␣)done (i)inline (.|@)actions") || strings.Contains(hint, "?") {
		t.Errorf("hint = %q", hint)
	}
}

func TestDoneNoteHintOffersActiveWithoutActions(t *testing.T) {
	m := hintModel(t, true)
	selectNote(t, &m, "note-3")
	hint := stripANSI(m.keyHintPill())
	if !strings.Contains(hint, "(↵)open (␣)active (i)inline") || strings.Contains(hint, "actions") || strings.Contains(hint, "(␣)done") {
		t.Errorf("done note hint = %q", hint)
	}
}

func TestInlineEditDropsTagsAndShowsEditingPill(t *testing.T) {
	m := hintModel(t, false)
	m, _ = pressKey(t, m, "i")
	if m.mode != ViewInlineEdit {
		t.Fatalf("i should start inline edit, mode %v", m.mode)
	}
	if line := noteLine(m, "Flaky deploys"); strings.Contains(line, "#manual") {
		t.Errorf("inline edit should hide every tag: %q", line)
	}
	if want := m.width - 3 - 7; m.inlineInput.Width != want {
		t.Errorf("inline width = %d, want %d", m.inlineInput.Width, want)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "✎ editing (↵)save (esc)cancel") {
		t.Errorf("inline edit should show the editing pill")
	}
}
