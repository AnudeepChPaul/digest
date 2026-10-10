package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/model"

	tea "github.com/charmbracelet/bubbletea"
)

func noteSelectedModel(t *testing.T) Model {
	t.Helper()
	m := selectionTestModel(t)
	m.notes = []*model.Note{{ID: "n1", Summary: "Ship the trial banner", Body: "details", Status: model.StatusActive, Source: model.SourceManual, Created: m.currentDate, Updated: m.currentDate}}
	for index, item := range m.allNavItems() {
		if item.Note != nil && item.Note.ID == "n1" {
			m.selected = index
			return m
		}
	}
	t.Fatal("note n1 not listed")
	return m
}

func pressCtrlC(m Model) (Model, bool) {
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		return next.(Model), false
	}
	result := make(chan tea.Msg, 1)
	go func() { result <- cmd() }()
	select {
	case msg := <-result:
		_, quits := msg.(tea.QuitMsg)
		return next.(Model), quits
	case <-time.After(100 * time.Millisecond):
		return next.(Model), false
	}
}

func TestCtrlCOnlyQuitsOnTheThirdPressFromAnyScreen(t *testing.T) {
	screens := map[string]func(t *testing.T) Model{
		"dashboard": func(t *testing.T) Model { return syncTestModel(t) },
		"preview": func(t *testing.T) Model {
			return press(t, noteSelectedModel(t), tea.KeyMsg{Type: tea.KeyTab})
		},
		"editor": func(t *testing.T) Model { return press(t, syncTestModel(t), runes("a")) },
		"search": func(t *testing.T) Model { return typeQuery(t, searchTestModel(t), "flaky") },
		"setup": func(t *testing.T) Model {
			m, _, _ := setupModel(t, "")
			return m
		},
	}
	for name, open := range screens {
		t.Run(name, func(t *testing.T) { assertOnlyThirdCtrlCQuits(t, open(t)) })
	}
}

func assertOnlyThirdCtrlCQuits(t *testing.T, m Model) {
	t.Helper()
	mode := m.mode
	for pressCount := 1; pressCount <= 2; pressCount++ {
		var quits bool
		if m, quits = pressCtrlC(m); quits {
			t.Fatalf("press %d quit the app", pressCount)
		}
		if m.mode != mode {
			t.Fatalf("press %d changed mode %d to %d", pressCount, mode, m.mode)
		}
	}
	if _, quits := pressCtrlC(m); !quits {
		t.Errorf("third press should quit")
	}
}

func TestCtrlCCountdownShowsOnModals(t *testing.T) {
	m := press(t, noteSelectedModel(t), tea.KeyMsg{Type: tea.KeyTab})
	m, _ = pressCtrlC(m)
	if !strings.Contains(stripANSI(m.View()), "2 more") {
		t.Errorf("the quit countdown should show over a modal:\n%s", stripANSI(m.View()))
	}
}

func TestCtrlCStaleResetTickDoesNotClearLaterPresses(t *testing.T) {
	m := syncTestModel(t)
	m, _ = pressCtrlC(m)
	firstPressReset := ctrlCResetMsg{pressSequence: m.ctrlCPressSequence}
	m, _ = pressCtrlC(m)
	next, _ := m.Update(firstPressReset)
	m = next.(Model)
	if m.ctrlCCount != 2 {
		t.Fatalf("a stale reset tick cleared the count to %d", m.ctrlCCount)
	}
	if _, quits := pressCtrlC(m); !quits {
		t.Errorf("third press should quit after a stale reset tick")
	}
}

func TestCtrlCLatestResetTickClearsTheCount(t *testing.T) {
	m := syncTestModel(t)
	m, _ = pressCtrlC(m)
	m, _ = pressCtrlC(m)
	next, _ := m.Update(ctrlCResetMsg{pressSequence: m.ctrlCPressSequence})
	m = next.(Model)
	if m.ctrlCCount != 0 {
		t.Errorf("the latest reset tick should clear the count, got %d", m.ctrlCCount)
	}
}

func TestOtherKeyBetweenCtrlCPressesResetsTheCount(t *testing.T) {
	m := syncTestModel(t)
	m, _ = pressCtrlC(m)
	m, _ = pressCtrlC(m)
	m = press(t, m, runes("j"))
	if m.ctrlCCount != 0 {
		t.Fatalf("another key should reset the count, got %d", m.ctrlCCount)
	}
	if _, quits := pressCtrlC(m); quits {
		t.Errorf("ctrl+c after another key should start counting again")
	}
}
