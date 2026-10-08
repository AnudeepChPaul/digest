package tui

import (
	"strings"
	"testing"

	"github.com/AnudeepChPaul/digest/pkg/model"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func longNotePreview(t *testing.T) Model {
	t.Helper()
	m := selectionTestModel(t)
	body := strings.Repeat("line of note body text\n\n", 120)
	m.notes = []*model.Note{{ID: "long", Summary: "long note", Body: body, Status: model.StatusActive, Created: m.currentDate, Updated: m.currentDate}}
	for index, item := range m.allNavItems() {
		if item.Note != nil {
			m.selected = index
		}
	}
	m.mode = ViewPreview
	m.updatePreviewViewport()
	m.previewViewport.SetYOffset(10)
	return m
}

func keyFor(name string) tea.KeyMsg {
	switch name {
	case "pgdown":
		return tea.KeyMsg{Type: tea.KeyPgDown}
	case "pgup":
		return tea.KeyMsg{Type: tea.KeyPgUp}
	case "ctrl+d":
		return tea.KeyMsg{Type: tea.KeyCtrlD}
	case "ctrl+u":
		return tea.KeyMsg{Type: tea.KeyCtrlU}
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	}
	return runes(name)
}

func TestPreviewIgnoresRemovedScrollKeys(t *testing.T) {
	for _, name := range []string{"f", "b", "d", "u", "h", "l", "space", "left", "right"} {
		m := longNotePreview(t)
		before := m.previewViewport.YOffset
		m = press(t, m, keyFor(name))
		if m.previewViewport.YOffset != before || m.previewViewport.View() != longNotePreview(t).previewViewport.View() {
			t.Errorf("%s scrolled the preview", name)
		}
		if m.mode != ViewPreview {
			t.Errorf("%s changed mode to %v", name, m.mode)
		}
	}
}

func TestPreviewScrollKeys(t *testing.T) {
	for _, pair := range [][2]string{{"j", "k"}, {"pgdown", "pgup"}, {"ctrl+d", "ctrl+u"}} {
		m := longNotePreview(t)
		start := m.previewViewport.YOffset
		m = press(t, m, keyFor(pair[0]))
		if m.previewViewport.YOffset <= start {
			t.Errorf("%s did not scroll down", pair[0])
		}
		m = press(t, m, keyFor(pair[1]))
		if m.previewViewport.YOffset != start {
			t.Errorf("%s did not scroll back: %d vs %d", pair[1], m.previewViewport.YOffset, start)
		}
	}
}

func TestIdleJobPreviewIgnoresD(t *testing.T) {
	m := selectionTestModel(t)
	selectNavItem(t, &m, "job:janitor")
	m.mode = ViewPreview
	m.updatePreviewViewport()
	m = press(t, m, runes("d"))
	if m.mode != ViewPreview || m.jobToAbort != "" {
		t.Errorf("d on idle job: mode=%v abort=%q", m.mode, m.jobToAbort)
	}
}

func TestReviewTabEnterWithoutSelectionDoesNothing(t *testing.T) {
	m := reviewTestModel(t)
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != ViewPreview || m.reviewNotice != "" {
		t.Errorf("mode=%v notice=%q", m.mode, m.reviewNotice)
	}
}

func archiveModel(t *testing.T) Model {
	t.Helper()
	m := selectionTestModel(t)
	for index := range 60 {
		m.notes = append(m.notes, &model.Note{ID: "a" + string(rune('A'+index%26)) + string(rune('a'+index/26)), Summary: "archived", Status: model.StatusArchived, Created: m.currentDate, Updated: m.currentDate})
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlE})
	m.archivedViewport.SetYOffset(5)
	return m
}

func TestArchiveScrollKeys(t *testing.T) {
	for _, name := range []string{"f", "b", "h", "l"} {
		m := archiveModel(t)
		before := m.archivedViewport.YOffset
		m = press(t, m, keyFor(name))
		if m.archivedViewport.YOffset != before {
			t.Errorf("%s scrolled the archive", name)
		}
	}
	for _, name := range []string{"pgdown", "ctrl+d"} {
		m := archiveModel(t)
		before := m.archivedViewport.YOffset
		m = press(t, m, keyFor(name))
		if m.archivedViewport.YOffset <= before {
			t.Errorf("%s did not scroll the archive", name)
		}
	}
}

func TestFootersHideScrollKeysOutsideDashboard(t *testing.T) {
	scrollKeys := []string{"j|k scroll", "pgup|pgdn", "ctrl+d|u"}
	pr := reviewTestModel(t)
	prFooter := footerText(pr.reviewFooterItems(pr.currentPRItem()))
	for _, want := range []string{"p|n prev/next", "ctrl+y copy"} {
		if !strings.Contains(prFooter, want) {
			t.Errorf("PR footer missing %q: %s", want, prFooter)
		}
	}
	footers := map[string]string{
		"pr":              prFooter,
		"review-run":      footerText(reviewRunFooterItems(false)),
		"review-run live": footerText(reviewRunFooterItems(true)),
		"archive":         footerText(archiveFooterItems),
	}
	jobPreview := selectionTestModel(t)
	selectNavItem(t, &jobPreview, "job:janitor")
	jobPreview.mode = ViewPreview
	jobPreview.updatePreviewViewport()
	footers["job"] = jobPreview.View()
	footers["note"] = longNotePreview(t).View()
	footers["archive view"] = archiveModel(t).View()
	for name, footer := range footers {
		for _, scrollKey := range scrollKeys {
			if strings.Contains(footer, scrollKey) {
				t.Errorf("%s footer shows %q", name, scrollKey)
			}
		}
	}
	if !strings.Contains(footerText(archiveFooterItems), "j|k nav") {
		t.Errorf("archive footer lost j|k nav")
	}
	dashboard := selectionTestModel(t)
	dashboard.width = 260
	if view := stripANSI(dashboard.View()); strings.Contains(view, "ctrl+d|u") {
		t.Errorf("dashboard footer should be gone; keys live in the ? shortcuts modal")
	}
	dashboard.mode = ViewHelp
	if view := dashboard.View(); !strings.Contains(view, "ctrl+d|u") || !strings.Contains(view, "search") || !strings.Contains(view, "←|→") {
		t.Errorf("shortcuts modal missing dashboard keys")
	}
}

func TestNotePreviewHeaderKeepsBadgeAndTagRight(t *testing.T) {
	m := notePreviewModel(t, model.SourceManual, "body")
	m.notes[0].Subject = "general"
	m.updatePreviewViewport()
	lines := plainLines(m.View())
	titleRow := -1
	for index, line := range lines {
		if strings.Contains(line, "PREVIEW NOTE") {
			titleRow = index
			break
		}
	}
	if titleRow < 0 {
		t.Fatal("no header")
	}
	titleLine, tagLine := strings.TrimRight(strings.TrimSpace(lines[titleRow]), "│ "), strings.TrimRight(strings.TrimSpace(lines[titleRow+1]), "│ ")
	if !strings.HasSuffix(titleLine, "DONE") {
		t.Errorf("DONE not at right of title line: %q", lines[titleRow])
	}
	if !strings.HasSuffix(tagLine, "#general") || runeColumn(lines[titleRow+1], "#general") != runeColumn(lines[titleRow], "DONE")+lipgloss.Width("DONE")-lipgloss.Width("#general") {
		t.Errorf("tag not right-aligned: %q", lines[titleRow+1])
	}
	if lipgloss.Width(lines[titleRow]) != lipgloss.Width(lines[titleRow+2]) {
		t.Errorf("header row width %d differs from modal row %d", lipgloss.Width(lines[titleRow]), lipgloss.Width(lines[titleRow+2]))
	}
}
