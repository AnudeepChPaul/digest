package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/automation"
	"github.com/achandrapaul/digest/pkg/brag"
	"github.com/achandrapaul/digest/pkg/config"
	"github.com/achandrapaul/digest/pkg/model"
	"github.com/achandrapaul/digest/pkg/review"

	tea "github.com/charmbracelet/bubbletea"
)

func assertViewportScrollPairs(t *testing.T, name string, build func(t *testing.T) Model, offset func(Model) int) {
	t.Helper()
	pairs := map[string][2]tea.KeyMsg{
		"j/k":         {keyFor("j"), keyFor("k")},
		"arrows":      {{Type: tea.KeyDown}, {Type: tea.KeyUp}},
		"pgdown/pgup": {keyFor("pgdown"), keyFor("pgup")},
		"ctrl+d/u":    {keyFor("ctrl+d"), keyFor("ctrl+u")},
	}
	for label, pair := range pairs {
		m := build(t)
		start := offset(m)
		if m = press(t, m, pair[0]); offset(m) <= start {
			t.Errorf("%s: %s did not scroll down from %d", name, label, start)
		}
		if m = press(t, m, pair[1]); offset(m) != start {
			t.Errorf("%s: %s did not scroll back to %d, at %d", name, label, start, offset(m))
		}
	}
}

func scrolledBragView(t *testing.T) Model {
	t.Helper()
	m := savedBragModel(t)
	m.previewViewport.SetYOffset(5)
	return m
}

func longSearchPreview(t *testing.T) Model {
	t.Helper()
	m := selectionTestModel(t)
	m.notes = []*model.Note{{ID: "long", Summary: "long flaky note", Body: strings.Repeat("flaky body line\n\n", 120), Status: model.StatusActive, Source: model.SourceManual, Updated: time.Now()}}
	m = typeQuery(t, press(t, m, runes("/")), "flaky")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.mode != ViewPreview || !m.searchPreviewing {
		t.Fatalf("mode %v", m.mode)
	}
	m.previewViewport.SetYOffset(5)
	return m
}

func previewOffset(m Model) int { return m.previewViewport.YOffset }

func TestBragViewScrollKeys(t *testing.T) {
	assertViewportScrollPairs(t, "brag view", scrolledBragView, previewOffset)
	m := scrolledBragView(t)
	if m = press(t, m, runes("j")); m.mode != ViewBragView {
		t.Errorf("scrolling left the brag view, mode %v", m.mode)
	}
}

func TestSearchPreviewScrollKeys(t *testing.T) {
	assertViewportScrollPairs(t, "search preview", longSearchPreview, previewOffset)
	m := longSearchPreview(t)
	if m = press(t, m, keyFor("ctrl+d")); m.mode != ViewPreview || !m.searchPreviewing || m.searchPreviewNote().ID != "long" {
		t.Errorf("scrolling changed the search preview, mode %v", m.mode)
	}
}

func rejectCommentModel(t *testing.T) Model {
	t.Helper()
	m := reviewTestModel(t)
	m = press(t, m, runes("d"))
	if m.mode != ViewRejectComment {
		t.Fatalf("d should open the comment box, mode %v", m.mode)
	}
	return m
}

func TestRejectBoxEscReturnsToThePreviewWithoutAReview(t *testing.T) {
	m := rejectCommentModel(t)
	m.rejectInput.SetValue("half written")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != ViewPreview || m.reviewEvent != "" || m.rejectInput.Focused() {
		t.Errorf("mode %v event %q focused %v", m.mode, m.reviewEvent, m.rejectInput.Focused())
	}
	m = press(t, m, runes("d"))
	if m.mode != ViewRejectComment || m.rejectInput.Value() != "" {
		t.Errorf("reopening should start empty, got %q", m.rejectInput.Value())
	}
}

func TestRejectBoxEscFromTheDashboardReturnsToTheDashboard(t *testing.T) {
	m := reviewTestModel(t)
	if err := os.Remove(filepath.Join(review.StateDir(m.reviewRoot(), m.currentPRItem().PR.Ref), review.FindingsFile)); err != nil {
		t.Fatal(err)
	}
	m.cfg.ShowKeyHints = true
	m.mode = ViewDashboard
	m = press(t, m, runes("d"))
	if m.mode != ViewRejectComment {
		t.Fatalf("mode %v", m.mode)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != ViewDashboard {
		t.Errorf("mode %v", m.mode)
	}
}

func TestBragEditorEscDiscardsTheEditAndReturnsToTheView(t *testing.T) {
	m := press(t, savedBragModel(t), tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != ViewBragEdit {
		t.Fatalf("mode %v", m.mode)
	}
	original := m.bragEntry.Body()
	m.editor.SetValue("## Facts\n\n- unsaved\n\n## Summary\n\n- unsaved")
	m.bragNotice = "stale"
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	m = press(t, m, runes("y"))
	if m.mode != ViewBragView || m.bragNotice != "" || m.editor.Focused() {
		t.Fatalf("mode %v notice %q focused %v", m.mode, m.bragNotice, m.editor.Focused())
	}
	week40 := brag.WeekOf(time.Date(2026, 9, 30, 12, 0, 0, 0, time.Local))
	saved, err := brag.Load(m.cfg.BragDir(), week40)
	if err != nil || strings.Contains(saved.Facts, "unsaved") || m.bragEntry.Body() != original {
		t.Errorf("esc saved the edit: facts %q err %v", saved.Facts, err)
	}
}

func TestAutomationDraftEditorEscKeepsTheSavedDraft(t *testing.T) {
	m, _ := automationTestModel(t)
	m = withDraft(t, m, automation.RunDraftReady, automation.PhaseDraft)
	m = openDraftTab(t, m)
	before, err := automation.LoadDraft(m.cfg.AutomationDir(), "note-1")
	if err != nil {
		t.Fatal(err)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != ViewAutomationEdit {
		t.Fatalf("mode %v", m.mode)
	}
	m.editor.SetValue("project: PROJ\nsummary: never saved\n")
	m.automationNotice = "stale"
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	m = press(t, m, runes("y"))
	if m.mode != ViewPreview || m.previewTab != previewTabDraft || m.automationNotice != "" || m.editor.Focused() {
		t.Fatalf("mode %v tab %v notice %q", m.mode, m.previewTab, m.automationNotice)
	}
	after, _ := automation.LoadDraft(m.cfg.AutomationDir(), "note-1")
	if after != before || strings.Contains(stripANSI(m.View()), "never saved") {
		t.Errorf("esc changed the draft: %q", after)
	}
}

func workDaysFormModel(t *testing.T) Model {
	t.Helper()
	m, _, _ := setupFormModel(t, "")
	for m.setup.field != setupFieldWorkDays {
		m, _ = pressKey(t, m, "j")
	}
	return m
}

func TestSettingsHAndLMoveTheDayCursorAndWrap(t *testing.T) {
	m := workDaysFormModel(t)
	days := len(config.WeekdayOrder)
	start := m.setup.dayCursor
	if m, _ = pressKey(t, m, "l"); m.setup.dayCursor != (start+1)%days {
		t.Fatalf("l moved to %d", m.setup.dayCursor)
	}
	if m, _ = pressKey(t, m, "h"); m.setup.dayCursor != start {
		t.Fatalf("h moved to %d", m.setup.dayCursor)
	}
	for range start + 1 {
		m, _ = pressKey(t, m, "h")
	}
	if m.setup.dayCursor != days-1 {
		t.Errorf("h past the first day should wrap to the last, got %d", m.setup.dayCursor)
	}
	if m, _ = pressKey(t, m, "l"); m.setup.dayCursor != 0 {
		t.Errorf("l past the last day should wrap to the first, got %d", m.setup.dayCursor)
	}
	if m.mode != ViewSetup || m.setup.field != setupFieldWorkDays {
		t.Errorf("h/l left the work days field: mode %v field %d", m.mode, m.setup.field)
	}
}

func TestSettingsHAndLPickTheDayThatSpaceToggles(t *testing.T) {
	m := workDaysFormModel(t)
	before := strings.Join(m.setup.answers.WorkDays, ",")
	m, _ = pressKey(t, m, "l")
	m, _ = pressKey(t, m, "l")
	day := config.WeekdayOrder[m.setup.dayCursor]
	wasOn := strings.Contains(before, day)
	m, _ = pressKey(t, m, " ")
	if strings.Contains(strings.Join(m.setup.answers.WorkDays, ","), day) == wasOn {
		t.Errorf("space did not toggle %s: before %s after %v", day, before, m.setup.answers.WorkDays)
	}
}

func TestSettingsHAndLDoNothingOutsideTheWorkDaysField(t *testing.T) {
	m, _, _ := setupFormModel(t, "")
	if m.setup.field == setupFieldWorkDays {
		t.Fatal("form should not start on work days")
	}
	start, answers := m.setup.dayCursor, m.setup.answers
	for _, key := range []string{"h", "l"} {
		m, _ = pressKey(t, m, key)
	}
	if m.setup.dayCursor != start || m.setup.answers.ShowGit != answers.ShowGit || m.mode != ViewSetup {
		t.Errorf("h/l off the work days field changed the form: cursor %d", m.setup.dayCursor)
	}
}
