package tui

import (
	"strings"
	"testing"

	"github.com/achandrapaul/digest/pkg/model"

	tea "github.com/charmbracelet/bubbletea"
)

func TestEveryConfirmTakesYOrEnterAndCancelsOnNOrEsc(t *testing.T) {
	confirmModes := map[string]ViewMode{
		"delete":        ViewDeleteConfirm,
		"review":        ViewReviewConfirm,
		"review run":    ViewReviewRunConfirm,
		"brag":          ViewBragConfirm,
		"automation":    ViewAutomationConfirm,
		"setup discard": ViewSetupDiscard,
		"missing note":  ViewRecreateConfirm,
		"missing row":   ViewRecreateRow,
	}
	confirmKeys := map[string]tea.KeyMsg{"y": runes("y"), "enter": {Type: tea.KeyEnter}}
	cancelKeys := map[string]tea.KeyMsg{"n": runes("n"), "esc": {Type: tea.KeyEsc}}
	for name, mode := range confirmModes {
		m := syncTestModel(t)
		m.mode = mode
		bindings := m.activeBindings()
		confirmAction, cancelAction := bindings[0].action, bindings[1].action
		for keyName, keyMsg := range confirmKeys {
			if binding, found := m.resolveKey(keyMsg); !found || binding.action != confirmAction {
				t.Errorf("%s: %s should confirm, got %v found %v", name, keyName, binding.action, found)
			}
		}
		for keyName, keyMsg := range cancelKeys {
			if binding, found := m.resolveKey(keyMsg); !found || binding.action != cancelAction {
				t.Errorf("%s: %s should cancel, got %v found %v", name, keyName, binding.action, found)
			}
		}
		footer := footerItemsFrom(bindings)
		if len(footer) < 2 || footer[0].key != "y|enter" || footer[1].key != "n|esc" {
			t.Errorf("%s: footer = %q", name, footerText(footer))
		}
	}
}

func TestNCancelsTheRunConfirm(t *testing.T) {
	m, executed, _ := jobTestModel(t)
	m = press(t, m, runes("r"))
	m = press(t, m, runes("n"))
	if m.mode != ViewDashboard || m.jobToExecute != "" || len(*executed) != 0 {
		t.Errorf("mode = %v jobToExecute = %q executed = %v", m.mode, m.jobToExecute, *executed)
	}
}

func TestNCancelsTheAbortConfirm(t *testing.T) {
	m, _, _ := jobTestModel(t)
	markJobRunning(t, &m, "nightly")
	m = press(t, m, runes("d"))
	m = press(t, m, runes("n"))
	if m.mode != ViewDashboard || m.jobToAbort != "" {
		t.Errorf("mode = %v jobToAbort = %q", m.mode, m.jobToAbort)
	}
}

func TestNCancelsTheNoteDeleteConfirm(t *testing.T) {
	m := noteSelectedModel(t)
	m.notes[0].FilePath = "n1.md"
	m.contentVersion++
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = press(t, m, runes("d"))
	if m.mode != ViewDeleteConfirm {
		t.Fatalf("d in the note preview should ask, mode = %v", m.mode)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "n|esc") {
		t.Errorf("delete confirm footer should offer n|esc:\n%s", view)
	}
	m = press(t, m, runes("n"))
	if m.mode != ViewPreview || len(m.deleteTargetNotes) != 0 || m.noteByID("n1") == nil {
		t.Errorf("mode = %v targets = %v", m.mode, m.deleteTargetNotes)
	}
}

func TestNCancelsTheArchiveDeleteConfirm(t *testing.T) {
	m := syncTestModel(t)
	archived := &model.Note{ID: "a1", Summary: "old", FilePath: "a1.md", Status: model.StatusArchived}
	m.deleteTargetNotes, m.deleteReturnMode, m.mode = []*model.Note{archived}, ViewArchived, ViewDeleteConfirm
	m = press(t, m, runes("n"))
	if m.mode != ViewArchived || len(m.deleteTargetNotes) != 0 {
		t.Errorf("mode = %v targets = %v", m.mode, m.deleteTargetNotes)
	}
}

func TestEnterDiscardsSetupFromTheDiscardPrompt(t *testing.T) {
	m := syncTestModel(t)
	m.mode = ViewSetupDiscard
	if binding, found := m.resolveKey(tea.KeyMsg{Type: tea.KeyEnter}); !found || binding.action != actionCloseSetup {
		t.Errorf("enter should discard, got %v found %v", binding.action, found)
	}
}

func TestMissingNoteRowPillShowsConfirmKeys(t *testing.T) {
	m := syncTestModel(t)
	if pill := stripANSI(m.recreateRowPill()); !strings.Contains(pill, "y|enter") || !strings.Contains(pill, "n|esc") {
		t.Errorf("missing note pill = %q", pill)
	}
}
