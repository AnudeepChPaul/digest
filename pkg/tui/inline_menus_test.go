package tui

import (
	"slices"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/model"
	"github.com/achandrapaul/digest/pkg/store"

	tea "github.com/charmbracelet/bubbletea"
)

func inlineEditModel(t *testing.T) Model {
	t.Helper()
	now := time.Now()
	m := noteSaveModel(t, &model.Note{Summary: "Original summary", Status: model.StatusActive, Source: model.SourceManual, Created: now, Updated: now})
	selectSummary(t, &m, "Original summary")
	m = press(t, m, runes("i"))
	if m.mode != ViewInlineEdit || m.inlineInput.Value() != "Original summary" {
		t.Fatalf("i should open inline edit prefilled: mode=%v value=%q", m.mode, m.inlineInput.Value())
	}
	return m
}

func TestInlineEnterSavesTheTrimmedSummary(t *testing.T) {
	m := inlineEditModel(t)
	m.inlineInput.SetValue("   Renamed summary  ")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.mode != ViewDashboard || m.currentNote.Summary != "Renamed summary" {
		t.Fatalf("mode=%v summary=%q", m.mode, m.currentNote.Summary)
	}
	m, _ = applyNoteMessages(t, m, cmd)
	saved, err := store.New(m.cfg.NotesDir()).LoadByID(m.currentNote.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Summary != "Renamed summary" {
		t.Errorf("saved summary = %q", saved.Summary)
	}
}

func TestInlineTypingGoesIntoTheSummary(t *testing.T) {
	m := inlineEditModel(t)
	m = typeText(t, m, " v2")
	if m.mode != ViewInlineEdit || m.inlineInput.Value() != "Original summary v2" {
		t.Errorf("mode=%v value=%q", m.mode, m.inlineInput.Value())
	}
}

func TestInlineEscCancelsWithoutSaving(t *testing.T) {
	m := inlineEditModel(t)
	m.inlineInput.SetValue("Discarded")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.mode != ViewDashboard || cmd != nil {
		t.Fatalf("mode=%v cmd=%v", m.mode, cmd != nil)
	}
	if note := noteBySummary(t, m, "Original summary"); note.Summary != "Original summary" {
		t.Errorf("summary changed to %q", note.Summary)
	}
	saved, err := store.New(m.cfg.NotesDir()).LoadByID(m.currentNote.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Summary != "Original summary" {
		t.Errorf("saved summary = %q", saved.Summary)
	}
}

func TestNotifyInputEscCancelsWithoutAReminder(t *testing.T) {
	m, _ := actionsTestModel(t)
	m, _ = pressKey(t, m, "@")
	m, _ = pressKey(t, m, "enter")
	if m.mode != ViewNotifyInput {
		t.Fatalf("choosing notify should open the input, mode %v", m.mode)
	}
	m = typeText(t, m, "2h")
	m, cmd := pressKey(t, m, "esc")
	if m.mode != ViewDashboard || cmd != nil || m.notifyInput.Focused() {
		t.Errorf("mode=%v cmd=%v focused=%v", m.mode, cmd != nil, m.notifyInput.Focused())
	}
	if len(m.notifyEntries) != 0 {
		t.Errorf("esc saved a reminder: %v", m.notifyEntries)
	}
}

func TestActionMenuJAndKMoveWithinTheItems(t *testing.T) {
	m, _ := actionsTestModel(t)
	m, _ = pressKey(t, m, "@")
	if !slices.Equal(actionNames(m), []string{"notify", "jira"}) {
		t.Fatalf("actions = %v", actionNames(m))
	}
	for _, step := range []struct {
		key  string
		want int
	}{{"j", 1}, {"j", 1}, {"k", 0}, {"k", 0}} {
		m, _ = pressKey(t, m, step.key)
		if m.actionMenuSelected != step.want {
			t.Errorf("after %s: selected=%d, want %d", step.key, m.actionMenuSelected, step.want)
		}
	}
}

func TestActionMenuEnterAndYChooseTheHighlightedItem(t *testing.T) {
	for _, chooseKey := range []string{"enter", "y"} {
		m, started := actionsTestModel(t)
		m, _ = pressKey(t, m, "@")
		m, _ = pressKey(t, m, "j")
		m, _ = pressKey(t, m, chooseKey)
		if m.mode != ViewAutomationConfirm || len(*started) != 0 {
			t.Errorf("%s on jira: mode=%v started=%v", chooseKey, m.mode, *started)
		}
		m, _ = actionsTestModel(t)
		m, _ = pressKey(t, m, "@")
		m, _ = pressKey(t, m, chooseKey)
		if m.mode != ViewNotifyInput {
			t.Errorf("%s on notify: mode=%v", chooseKey, m.mode)
		}
	}
}

func TestActionMenuNCancels(t *testing.T) {
	m, started := actionsTestModel(t)
	m, _ = pressKey(t, m, "@")
	m, _ = pressKey(t, m, "j")
	m, cmd := pressKey(t, m, "n")
	if m.mode != ViewDashboard || cmd != nil || len(*started) != 0 {
		t.Errorf("mode=%v cmd=%v started=%v", m.mode, cmd != nil, *started)
	}
}

func TestLinkMenuJAndKClampToTheLinks(t *testing.T) {
	stubOpenURL(t)
	m := pressO(t, noteWithBody(t, docsLink+"\n"+pullLink))
	for _, step := range []struct {
		key  string
		want int
	}{{"j", 1}, {"j", 1}, {"k", 0}, {"k", 0}} {
		m = press(t, m, runes(step.key))
		if m.linkMenuSelected != step.want {
			t.Errorf("after %s: selected=%d, want %d", step.key, m.linkMenuSelected, step.want)
		}
	}
}

func TestLinkMenuOpensTheHighlightedLinkOnEnterOnly(t *testing.T) {
	opened := stubOpenURL(t)
	m := pressO(t, noteWithBody(t, docsLink+"\n"+pullLink))
	m = press(t, m, runes("j"))
	for _, ignored := range []string{"y", "Y"} {
		if m = press(t, m, runes(ignored)); m.mode != ViewLinkMenu || len(*opened) != 0 {
			t.Fatalf("%s: mode=%v opened=%v", ignored, m.mode, *opened)
		}
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != ViewDashboard || !slices.Equal(*opened, []string{pullLink}) {
		t.Errorf("mode=%v opened=%v", m.mode, *opened)
	}
}

func TestLinkMenuNCancelsWithoutOpening(t *testing.T) {
	opened := stubOpenURL(t)
	m := pressO(t, noteWithBody(t, docsLink+"\n"+pullLink))
	m = press(t, m, runes("n"))
	if m.mode != ViewDashboard || len(*opened) != 0 || len(m.linkMenuItems) != 0 {
		t.Errorf("mode=%v opened=%v items=%v", m.mode, *opened, m.linkMenuItems)
	}
}
