package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func recordOpenedURLs(t *testing.T) *[]string {
	t.Helper()
	var opened []string
	originalOpen := openURL
	openURL = func(url string) error {
		opened = append(opened, url)
		return nil
	}
	t.Cleanup(func() { openURL = originalOpen })
	return &opened
}

func TestEnterInMyPRModalOpensPR(t *testing.T) {
	opened := recordOpenedURLs(t)
	m := myPRStripModel(t)
	m.selected = 3
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(*opened) != 1 || (*opened)[0] != m.myPRs[0].Ref.URL || m.mode != ViewPreview {
		t.Errorf("opened=%v mode=%v", *opened, m.mode)
	}
}

func TestEnterInPRReviewModalOpensPRUnlessPostingFindings(t *testing.T) {
	opened := recordOpenedURLs(t)
	m := reviewTestModel(t)
	url := m.ghPendingPRs[0].URL
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(*opened) != 1 || (*opened)[0] != url || m.mode != ViewPreview {
		t.Fatalf("details tab: opened=%v mode=%v", *opened, m.mode)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(*opened) != 2 {
		t.Fatalf("review tab without selection should open the PR: opened=%v", *opened)
	}
	m = press(t, m, runes(" "))
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(*opened) != 2 || m.mode == ViewPreview {
		t.Errorf("selected findings should post the review instead: opened=%v mode=%v", *opened, m.mode)
	}
}
