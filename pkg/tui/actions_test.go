package tui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/config"
	"github.com/AnudeepChPaul/digest/pkg/model"
	"github.com/AnudeepChPaul/digest/pkg/notify"

	tea "github.com/charmbracelet/bubbletea"
)

func actionsTestModel(t *testing.T) (Model, *[]startedAutomation) {
	t.Helper()
	m, started := automationTestModel(t)
	showGit := false
	m.cfg.ShowGit = &showGit
	m.notes = append(m.notes, &model.Note{ID: "note-2", Summary: "Call the bank", Created: m.currentDate, Source: model.SourceManual})
	m.notes = append(m.notes, &model.Note{ID: "note-3", Summary: "Shipped it", Created: m.currentDate, Status: model.StatusDone, Source: model.SourceManual})
	for _, note := range m.notes {
		if note.ID != "" {
			if err := m.store.Save(note); err != nil {
				t.Fatal(err)
			}
		}
	}
	selectNote(t, &m, "note-1")
	return m, started
}

func pressKey(t *testing.T, m Model, key string) (Model, tea.Cmd) {
	t.Helper()
	var msg tea.KeyMsg
	switch key {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	case "down":
		msg = tea.KeyMsg{Type: tea.KeyDown}
	case " ":
		msg = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

func typeText(t *testing.T, m Model, text string) Model {
	t.Helper()
	for _, character := range text {
		m, _ = pressKey(t, m, string(character))
	}
	return m
}

func applyMsgs(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	for _, msg := range collectMsgs(cmd) {
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

func followCmds(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	pending := []tea.Cmd{cmd}
	for round := 0; round < 4 && len(pending) > 0; round++ {
		var next []tea.Cmd
		for _, current := range pending {
			for _, msg := range collectMsgs(current) {
				updated, followUp := m.Update(msg)
				m = updated.(Model)
				if followUp != nil {
					next = append(next, followUp)
				}
			}
		}
		pending = next
	}
	return m
}

func actionNames(m Model) []string {
	var names []string
	for _, action := range m.actionMenuItems {
		names = append(names, action.name)
	}
	return names
}

func setNotify(t *testing.T, m Model, interval string) Model {
	t.Helper()
	m, _ = pressKey(t, m, "@")
	m, _ = pressKey(t, m, "enter")
	m = typeText(t, m, interval)
	m, cmd := pressKey(t, m, "enter")
	return applyMsgs(t, m, cmd)
}

func TestAtOpensActionsWithNotifyFirst(t *testing.T) {
	m, _ := actionsTestModel(t)
	m, _ = pressKey(t, m, "@")
	if m.mode != ViewActionMenu || strings.Join(actionNames(m), ",") != "notify,jira" || m.actionMenuSelected != 0 {
		t.Fatalf("mode %v actions %v selected %d", m.mode, actionNames(m), m.actionMenuSelected)
	}
	if view := m.View(); !strings.Contains(view, "@notify") || !strings.Contains(view, "@jira") {
		t.Errorf("menu should list @notify and @jira:\n%s", view)
	}
	m, _ = pressKey(t, m, "esc")
	if m.mode != ViewDashboard {
		t.Errorf("esc should close the menu, mode %v", m.mode)
	}
}

func TestAutomateOnlyListedWhenTheNoteCanBeAutomated(t *testing.T) {
	m, _ := actionsTestModel(t)
	selectNote(t, &m, "note-2")
	m, _ = pressKey(t, m, "@")
	if strings.Join(actionNames(m), ",") != "notify" {
		t.Errorf("actions = %v, want only notify", actionNames(m))
	}
}

func TestAtDoesNothingOnADoneNote(t *testing.T) {
	m, _ := actionsTestModel(t)
	selectNote(t, &m, "note-3")
	if m, _ = pressKey(t, m, "@"); m.mode != ViewDashboard {
		t.Errorf("done note: mode %v, want dashboard", m.mode)
	}
	if m, _ = pressKey(t, m, "."); m.mode != ViewDashboard {
		t.Errorf("done note: . should not open actions, mode %v", m.mode)
	}
}

func TestNotifyIntervalWritesTheEntryAndTag(t *testing.T) {
	cases := map[string]string{"90m": "90m", "2": "2h", "": "1h", "1d": "1d"}
	for typed, label := range cases {
		m, _ := actionsTestModel(t)
		before := time.Now()
		m = setNotify(t, m, typed)
		if m.mode != ViewDashboard {
			t.Fatalf("%q: mode %v, want dashboard", typed, m.mode)
		}
		entry, found := notify.Load(m.cfg.Root(), "note-1")
		if !found || entry.Interval != label || entry.Summary != "Flaky deploys" || entry.NotifiedAt.Before(before) {
			t.Errorf("%q: entry = %+v found %v", typed, entry, found)
		}
		if line := noteLine(m, "Flaky deploys"); !strings.Contains(line, "@notify:"+label) {
			t.Errorf("%q: row should show @notify:%s, got %q", typed, label, line)
		}
	}
}

func TestNotifyAcceptsYToConfirm(t *testing.T) {
	m, _ := actionsTestModel(t)
	m, _ = pressKey(t, m, "@")
	m, _ = pressKey(t, m, "enter")
	m = typeText(t, m, "3")
	m, cmd := pressKey(t, m, "y")
	m = applyMsgs(t, m, cmd)
	if entry, found := notify.Load(m.cfg.Root(), "note-1"); !found || entry.Interval != "3h" {
		t.Errorf("y should confirm: entry %+v found %v", entry, found)
	}
}

func TestNotifyRejectsAnIntervalOverThreeDays(t *testing.T) {
	m, _ := actionsTestModel(t)
	m, _ = pressKey(t, m, "@")
	m, _ = pressKey(t, m, "enter")
	m = typeText(t, m, "4d")
	m, cmd := pressKey(t, m, "enter")
	m = applyMsgs(t, m, cmd)
	if m.mode != ViewNotifyInput || m.notifyNotice == "" {
		t.Errorf("mode %v notice %q, want input with a notice", m.mode, m.notifyNotice)
	}
	if line := noteLine(m, "Flaky deploys"); !strings.Contains(line, "@notify:4d") || !strings.Contains(line, "at most 3d") {
		t.Errorf("the error should show on the row: %q", line)
	}
	if _, found := notify.Load(m.cfg.Root(), "note-1"); found {
		t.Errorf("an invalid interval should not write an entry")
	}
}

func TestAtAgainPrefillsTheCurrentInterval(t *testing.T) {
	m, _ := actionsTestModel(t)
	m = setNotify(t, m, "90m")
	m, _ = pressKey(t, m, "@")
	m, _ = pressKey(t, m, "enter")
	if m.notifyInput.Value() != "90m" {
		t.Errorf("input = %q, want the current interval", m.notifyInput.Value())
	}
}

func TestMarkingDoneRemovesTheNotifyEntry(t *testing.T) {
	m, _ := actionsTestModel(t)
	m = setNotify(t, m, "2")
	selectNote(t, &m, "note-1")
	m, cmd := pressKey(t, m, " ")
	m = followCmds(t, m, cmd)
	if _, found := notify.Load(m.cfg.Root(), "note-1"); found {
		t.Errorf("done note should lose its notify entry")
	}
	if line := noteLine(m, "Flaky deploys"); strings.Contains(line, "@notify") {
		t.Errorf("done note should lose its tag: %q", line)
	}
}

func TestNotesReloadPrunesEntriesOfClosedOrMissingNotes(t *testing.T) {
	m, _ := actionsTestModel(t)
	for _, noteID := range []string{"note-2", "note-3", "gone"} {
		if err := notify.Save(m.cfg.Root(), notify.Entry{NoteID: noteID, Interval: "1h"}); err != nil {
			t.Fatal(err)
		}
	}
	m = followCmds(t, m, func() tea.Msg { return m.loadNotesCmd() })
	entries, _ := notify.List(m.cfg.Root())
	if len(entries) != 1 || entries[0].NoteID != "note-2" {
		t.Errorf("entries = %+v, want only the active note-2", entries)
	}
}

func TestAutomateFromTheMenuConfirmsThenStarts(t *testing.T) {
	m, started := actionsTestModel(t)
	m, _ = pressKey(t, m, "@")
	m, _ = pressKey(t, m, "down")
	m, _ = pressKey(t, m, "enter")
	if m.mode != ViewAutomationConfirm {
		t.Fatalf("mode %v, want automation confirm", m.mode)
	}
	m, _ = pressKey(t, m, "enter")
	if len(*started) != 1 || (*started)[0].noteID != "note-1" {
		t.Errorf("started = %+v", *started)
	}
}

func TestCancellingAutomateFromTheMenuReturnsToTheDashboard(t *testing.T) {
	m, _ := actionsTestModel(t)
	m, _ = pressKey(t, m, "@")
	m, _ = pressKey(t, m, "down")
	m, _ = pressKey(t, m, "enter")
	if m, _ = pressKey(t, m, "esc"); m.mode != ViewDashboard {
		t.Errorf("mode %v, want dashboard", m.mode)
	}
}

func TestFrequentlyUsedActionsComeFirst(t *testing.T) {
	m, _ := actionsTestModel(t)
	for round, keys := range [][]string{{"@", "down", "enter"}, {"@", "enter"}} {
		var cmd tea.Cmd
		for _, key := range keys {
			m, cmd = pressKey(t, m, key)
		}
		if m.mode != ViewAutomationConfirm {
			t.Fatalf("round %d: mode %v, want automation confirm", round, m.mode)
		}
		m = applyMsgs(t, m, cmd)
		m, _ = pressKey(t, m, "esc")
	}
	m, _ = pressKey(t, m, "@")
	if strings.Join(actionNames(m), ",") != "jira,notify" {
		t.Errorf("actions = %v, want jira first after using it most", actionNames(m))
	}
	data, err := os.ReadFile(filepath.Join(m.cfg.CacheDir(), "action-usage.json"))
	if err != nil || !strings.Contains(string(data), `"jira": 2`) {
		t.Errorf("usage file = %s, %v", data, err)
	}
	reloaded := NewModel(m.cfg, nil)
	reloaded.notes = m.notes
	selectNote(t, &reloaded, "note-1")
	reloaded, _ = pressKey(t, reloaded, "@")
	if strings.Join(actionNames(reloaded), ",") != "jira,notify" {
		t.Errorf("after restart actions = %v, want the saved order", actionNames(reloaded))
	}
}

func TestNotifyEntryRemembersThePRLinkOrTheTerminal(t *testing.T) {
	t.Setenv("__CFBundleIdentifier", "com.example.terminal")
	m, _ := actionsTestModel(t)
	prNote := &model.Note{ID: "pr-note", Summary: "Review console #7", Body: "Review https://github.com/acme/console/pull/7 today", Created: m.currentDate, Source: model.SourcePRReview}
	m.notes = append(m.notes, prNote)
	selectNote(t, &m, "pr-note")
	m = setNotify(t, m, "1")
	selectNote(t, &m, "note-2")
	m = setNotify(t, m, "1")
	if entry, _ := notify.Load(m.cfg.Root(), "pr-note"); entry.OpenURL != "https://github.com/acme/console/pull/7" {
		t.Errorf("pr note entry = %+v", entry)
	}
	if entry, _ := notify.Load(m.cfg.Root(), "note-2"); entry.OpenURL != "" || entry.Terminal != "com.example.terminal" {
		t.Errorf("plain note entry = %+v", entry)
	}
}

func TestDotAlsoOpensTheActions(t *testing.T) {
	m, _ := actionsTestModel(t)
	if m, _ = pressKey(t, m, "."); m.mode != ViewActionMenu {
		t.Errorf("mode %v, want the action menu", m.mode)
	}
	m, _ = pressKey(t, m, "esc")
	m, _ = pressKey(t, m, "i")
	if m, _ = pressKey(t, m, "."); m.mode != ViewInlineEdit {
		t.Errorf("inline edit should type the dot, mode %v", m.mode)
	}
}

func TestEveryMatchingAutomationIsListed(t *testing.T) {
	m, _ := actionsTestModel(t)
	m.cfg.Automations = append(m.cfg.Automations, config.AutomationSpec{Name: "confluence", Match: []string{"flaky"}})
	m, _ = pressKey(t, m, "@")
	if strings.Join(actionNames(m), ",") != "notify,jira,confluence" {
		t.Errorf("actions = %v", actionNames(m))
	}
	m, _ = pressKey(t, m, "down")
	m, _ = pressKey(t, m, "down")
	m, _ = pressKey(t, m, "enter")
	if m.mode != ViewAutomationConfirm || m.automationName != "confluence" {
		t.Errorf("mode %v automation %q", m.mode, m.automationName)
	}
}

func TestDropdownSitsUnderTheSelectedRowAndFitsItsEntries(t *testing.T) {
	m, _ := actionsTestModel(t)
	before := plainLines(m.View())
	rowLine := slices.IndexFunc(before, func(line string) bool { return strings.Contains(line, "Flaky deploys") })
	m, _ = pressKey(t, m, "@")
	after := plainLines(m.View())
	if len(after) != len(before) || rowLine < 0 || !strings.Contains(after[rowLine], "Flaky deploys") {
		t.Fatalf("the dashboard should stay in place, row at %d", rowLine)
	}
	top, bottom := after[rowLine+1], after[rowLine+4]
	if !strings.Contains(top, "╭") || !strings.Contains(after[rowLine+2], "@notify") || !strings.Contains(after[rowLine+3], "@jira") || !strings.Contains(bottom, "╰") {
		t.Fatalf("dropdown should open under the row:\n%s", strings.Join(after[rowLine:rowLine+5], "\n"))
	}
	topRunes := []rune(top)
	boxWidth := slices.Index(topRunes, '╮') - slices.Index(topRunes, '╭') + 1
	if boxWidth <= 0 || boxWidth > len("› @notify")+6 {
		t.Errorf("dropdown should only fit its entries, width %d:\n%s", boxWidth, top)
	}
}

func TestNotifyIsTypedInlineOnTheRow(t *testing.T) {
	m, _ := actionsTestModel(t)
	m, _ = pressKey(t, m, "@")
	m, _ = pressKey(t, m, "enter")
	m = typeText(t, m, "90m")
	view := m.View()
	if strings.Contains(view, "Remind me every") {
		t.Errorf("notify should not open a popup:\n%s", view)
	}
	if line := noteLine(m, "Flaky deploys"); !strings.Contains(line, "@notify:90m") {
		t.Errorf("row should show the inline input, got %q", line)
	}
	m, _ = pressKey(t, m, "esc")
	if line := noteLine(m, "Flaky deploys"); m.mode != ViewDashboard || strings.Contains(line, "@notify") {
		t.Errorf("esc should restore the row: mode %v line %q", m.mode, line)
	}
	if _, found := notify.Load(m.cfg.Root(), "note-1"); found {
		t.Errorf("esc should not save")
	}
}

func TestActionsKeyIsOnlyInTheShortcutsHelp(t *testing.T) {
	m, _ := actionsTestModel(t)
	if footerHas(m, "actions") {
		t.Errorf("the dashboard should not list actions as a footer item")
	}
	listed := false
	for _, entry := range m.helpEntries() {
		listed = listed || (entry.key == "@|." && strings.Contains(entry.text, "actions"))
	}
	if !listed {
		t.Errorf("? shortcuts should list @|. actions: %+v", m.helpEntries())
	}
}

func TestNotifyEditStaysVisibleOnALongSummary(t *testing.T) {
	m, _ := actionsTestModel(t)
	long := "Gathered a new requirement. Need to create a ticket for 1console taskrouter search needs to be updated. EPIC: https://example.atlassian.net/browse/X-1"
	m.notes = append(m.notes, &model.Note{ID: "long", Summary: long, Created: m.currentDate, Source: model.SourceManual})
	selectNote(t, &m, "long")
	m, _ = pressKey(t, m, "@")
	m, _ = pressKey(t, m, "enter")
	if line := noteLine(m, "Gathered"); !strings.Contains(line, "@notify:1h") {
		t.Errorf("the empty input should show the 1h placeholder beside the tag: %q", line)
	}
	m = typeText(t, m, "2")
	line := noteLine(m, "Gathered")
	if !strings.Contains(line, "@notify:2") || !strings.Contains(line, "…") {
		t.Errorf("the summary should truncate, not the input: %q", line)
	}
	if notifyAt, ageAt := strings.Index(line, "@notify:2"), strings.Index(line, "#manual"); notifyAt < 0 || notifyAt > ageAt {
		t.Errorf("the input should sit in the tag area before the source tag: %q", line)
	}
	m = typeText(t, m, "0d")
	m, _ = pressKey(t, m, "enter")
	if line := noteLine(m, "Gathered"); !strings.Contains(line, "@notify:20d") || !strings.Contains(line, "at most 3d") {
		t.Errorf("the error should show beside the tag: %q", line)
	}
}

func TestActionMenuLooksLikeTheKeyHintPill(t *testing.T) {
	m, _ := actionsTestModel(t)
	m, _ = pressKey(t, m, "@")
	menu := m.renderActionDropdown()
	if strings.Contains(menu, "\x1b[48") {
		t.Errorf("action menu should have no background")
	}
	lines := strings.Split(stripANSI(menu), "\n")
	if !strings.HasPrefix(lines[0], "╭") || !strings.HasPrefix(lines[1], "│ › @") || !strings.HasSuffix(lines[1], " │") {
		t.Errorf("action menu should use the pill border with one space padding:\n%s", strings.Join(lines, "\n"))
	}
	if !strings.Contains(menu, hintKeyStyle.Render("› @"+m.actionMenuItems[0].name)) {
		t.Errorf("selected action should use the hint key style")
	}
}

func TestNotifyInputOnlyTakesWholeNumbersAndOneUnit(t *testing.T) {
	m, _ := actionsTestModel(t)
	m, _ = pressKey(t, m, "@")
	m, _ = pressKey(t, m, "enter")
	m = typeText(t, m, "2.5xhm")
	if got := m.notifyInput.Value(); got != "25h" {
		t.Errorf("input = %q, want 25h with the dot, x and second unit refused", got)
	}
	m.notifyInput.SetValue("")
	m = typeText(t, m, "h3d")
	if got := m.notifyInput.Value(); got != "3d" {
		t.Errorf("a unit needs a number first: %q", got)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "ⓘ 1m–3d · whole numbers · m h d") {
		t.Errorf("the notify input should show its info pill")
	}
}

func TestNotifyOffRemovesTheReminderOnly(t *testing.T) {
	m, _ := actionsTestModel(t)
	if names := offeredActionNames(m); slices.Contains(names, "notify:off") {
		t.Errorf("no reminder yet, so no off action: %v", names)
	}
	m = setNotify(t, m, "2")
	m.actionUsage = map[string]int{"jira": 9}
	names := offeredActionNames(m)
	if len(names) < 2 || names[0] != "notify:off" || names[1] != "jira" {
		t.Fatalf("notify:off should come first, ahead of more used actions: %v", names)
	}
	m, _ = pressKey(t, m, "@")
	m, cmd := pressKey(t, m, "enter")
	m = applyMsgs(t, m, cmd)
	if _, found := notify.Load(m.cfg.Root(), "note-1"); found {
		t.Errorf("notify:off should delete the reminder file")
	}
	if line := noteLine(m, "Flaky deploys"); strings.Contains(line, "@notify") || m.noteByID("note-1").Status == model.StatusDone {
		t.Errorf("tag should go and the note stay pending: %q", line)
	}
	if m.actionUsage["notify:off"] != 0 {
		t.Errorf("notify:off should not count as usage")
	}
}

func offeredActionNames(m Model) []string {
	note, _ := m.actionTargetNote()
	var names []string
	for _, action := range m.noteActions(note) {
		names = append(names, action.name)
	}
	return names
}

func TestActionMenuOpensAtTheRightEdgeLikeTheHintPill(t *testing.T) {
	m, _ := actionsTestModel(t)
	m, _ = pressKey(t, m, "@")
	lines := plainLines(m.View())
	row := slicesIndex(lines, "Flaky deploys")
	if row < 0 || !strings.HasSuffix(lines[row+1], "╮ │") || !strings.Contains(lines[row+2], "› @") || !strings.HasSuffix(lines[row+2], " │ │") {
		t.Fatalf("action menu should sit at the right edge under the row:\n%s", strings.Join(lines[max(row, 0):min(row+5, len(lines))], "\n"))
	}
}
