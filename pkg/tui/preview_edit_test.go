package tui

import (
	"strings"
	"testing"

	"github.com/AnudeepChPaul/digest/pkg/model"

	tea "github.com/charmbracelet/bubbletea"
)

func TestEnterInNotePreviewOpensTheEditor(t *testing.T) {
	m := notePreviewModel(t, model.SourceManual, "the body")
	if footer := footerText(footerItemsFrom(m.previewBindings())); !strings.Contains(footer, "enter edit") {
		t.Errorf("footer should offer enter edit: %s", footer)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != ViewEdit || m.editor.Value() != "Approved: thing\n\nthe body" {
		t.Fatalf("mode=%v editor=%q", m.mode, m.editor.Value())
	}
}

func TestLeavingThePreviewEditReturnsToThePreview(t *testing.T) {
	m := notePreviewModel(t, model.SourceManual, "the body")
	m = press(t, press(t, m, tea.KeyMsg{Type: tea.KeyEnter}), tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != ViewPreview {
		t.Fatalf("cancel should return to the preview, mode=%v", m.mode)
	}

	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m.editor.SetValue("Renamed note\n\nnew body")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlO})
	if m.mode != ViewPreview || !strings.Contains(stripANSI(m.previewViewport.View()), "new body") {
		t.Fatalf("save should return to the updated preview, mode=%v:\n%s", m.mode, stripANSI(m.previewViewport.View()))
	}
}

func TestEnterInPRReviewNotePreviewDoesNotEdit(t *testing.T) {
	stubOpenURL(t)
	m := notePreviewModel(t, model.SourcePRReview, "https://github.com/acme/console/pull/19162")
	if m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter}); m.mode != ViewPreview {
		t.Errorf("PR review note should stay in the preview, mode=%v", m.mode)
	}
}

func TestEnterOnADashboardNoteEditsOverItsPreview(t *testing.T) {
	m := press(t, noteSelectedModel(t), tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != ViewEdit || m.editor.Value() != "Ship the trial banner\n\ndetails" {
		t.Fatalf("enter should open the editor, mode=%v editor=%q", m.mode, m.editor.Value())
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != ViewPreview || !strings.Contains(stripANSI(m.previewViewport.View()), "details") {
		t.Fatalf("esc should land on the note preview, mode=%v:\n%s", m.mode, stripANSI(m.previewViewport.View()))
	}
	if m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc}); m.mode != ViewDashboard {
		t.Errorf("esc on the preview should close it, mode=%v", m.mode)
	}
}
