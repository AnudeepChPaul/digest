package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTabCyclesPRPreviewTabs(t *testing.T) {
	m := reviewTestModel(t)
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.mode != ViewPreview || m.previewTab != previewTabReview {
		t.Fatalf("first tab: mode=%v tab=%d", m.mode, m.previewTab)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.mode != ViewPreview || m.previewTab != previewTabDetails {
		t.Fatalf("second tab: mode=%v tab=%d", m.mode, m.previewTab)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != ViewDashboard {
		t.Fatalf("esc should close preview, mode=%v", m.mode)
	}
}

func TestHAndLDoNothingInPRPreview(t *testing.T) {
	m := reviewTestModel(t)
	for _, key := range []string{"l", "h"} {
		m = press(t, m, runes(key))
		if m.mode != ViewPreview || m.previewTab != previewTabDetails {
			t.Fatalf("%s: mode=%v tab=%d", key, m.mode, m.previewTab)
		}
	}
	m.previewTab = previewTabReview
	m = press(t, m, runes("h"))
	if m.previewTab != previewTabReview {
		t.Fatalf("h switched tab to %d", m.previewTab)
	}
}

func TestPRFooterShowsTabKey(t *testing.T) {
	m := reviewTestModel(t)
	footer := m.reviewFooterItems(m.currentPRItem())
	if footer[0].key != "tab" || footer[0].action != "tabs" {
		t.Errorf("first footer item = %+v", footer[0])
	}
}

func footerText(items []footerItem) string {
	var parts []string
	for _, item := range items {
		parts = append(parts, item.key+" "+item.action)
	}
	return strings.Join(parts, ", ")
}

func TestReviewRunFooterItems(t *testing.T) {
	running := footerText(reviewRunFooterItems(true))
	failed := footerText(reviewRunFooterItems(false))
	if strings.Contains(running, "run job") || strings.Contains(failed, "run job") {
		t.Errorf("review-run footer offers run job: %q / %q", running, failed)
	}
	if !strings.Contains(running, "d stop review") {
		t.Errorf("running footer = %q", running)
	}
	if strings.Contains(failed, "stop review") || !strings.Contains(failed, "esc|tab close") {
		t.Errorf("failed footer = %q", failed)
	}
}

func TestReviewRunPreviewUsesReviewRunFooter(t *testing.T) {
	m, running, _ := reviewRunsModel(t)
	selectNavItem(t, &m, "review:"+running.URL)
	m.mode = ViewPreview
	m.updatePreviewViewport()
	view := m.View()
	if strings.Contains(view, "Run") || !strings.Contains(view, "Stop") || !strings.Contains(view, "Review") {
		t.Errorf("review-run preview footer wrong")
	}
}
