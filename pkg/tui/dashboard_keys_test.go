package tui

import (
	"testing"

	"github.com/achandrapaul/digest/pkg/model"

	tea "github.com/charmbracelet/bubbletea"
)

func TestEnterAndTabOpenJobPreview(t *testing.T) {
	for _, key := range []tea.KeyMsg{{Type: tea.KeyEnter}, {Type: tea.KeyTab}} {
		m := selectionTestModel(t)
		selectNavItem(t, &m, "job:janitor")
		m = press(t, m, key)
		if m.mode != ViewPreview {
			t.Errorf("%s on job row: mode=%v", key.String(), m.mode)
		}
	}
}

func TestEnterOpensReviewRunPreview(t *testing.T) {
	m, _, failed := reviewRunsModel(t)
	selectNavItem(t, &m, "review:"+failed.URL)
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != ViewPreview {
		t.Errorf("mode=%v", m.mode)
	}
}

func TestEDoesNothingOnAnyRow(t *testing.T) {
	originalOpen := openURL
	var opened []string
	openURL = func(url string) error {
		opened = append(opened, url)
		return nil
	}
	t.Cleanup(func() { openURL = originalOpen })
	m := selectionTestModel(t)
	m.notes = []*model.Note{{ID: "n1", Summary: "note", Status: model.StatusActive, Created: m.currentDate, Updated: m.currentDate}}
	m.applyGitPending(gitPendingMsg{generation: m.git.fetchGeneration, pending: []GitPRItem{pendingItem(1)}})
	for _, key := range []string{"note:", "pr:" + pendingItem(1).URL, "job:janitor"} {
		found := false
		for index, item := range m.allNavItems() {
			if navItemKey(item) == key || (key == "note:" && item.Note != nil) {
				m.selected = index
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("no row for %q", key)
		}
		next := press(t, m, runes("e"))
		if next.mode != ViewDashboard {
			t.Errorf("e on %q: mode=%v", key, next.mode)
		}
	}
	if len(opened) != 0 {
		t.Errorf("e opened %v", opened)
	}
}
