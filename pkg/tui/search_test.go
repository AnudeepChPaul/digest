package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/model"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func searchTestModel(t *testing.T) Model {
	t.Helper()
	m := selectionTestModel(t)
	now := time.Now()
	m.notes = []*model.Note{
		{ID: "a", Summary: "Fix flaky test", Source: model.SourceManual, Status: model.StatusActive, Updated: now},
		{ID: "b", Summary: "Standup notes", Body: "retried the Flaky console test twice", Source: model.SourceStandup, Status: model.StatusDone, Updated: now.Add(-48 * time.Hour)},
		{ID: "c", Summary: "Approved: console#12", Source: model.SourcePRReview, Subject: "console", Status: model.StatusDone, Updated: now.Add(-24 * time.Hour)},
		{ID: "d", Summary: "Archived flaky", Source: model.SourceManual, Status: model.StatusArchived, Updated: now},
		{ID: "e", Summary: "Inbox flaky", Source: model.SourceManual, Status: model.StatusInbox, Updated: now},
	}
	return press(t, m, runes("/"))
}

func typeQuery(t *testing.T, m Model, query string) Model {
	t.Helper()
	for _, r := range query {
		m = press(t, m, runes(string(r)))
	}
	return m
}

func resultIDs(notes []*model.Note) string {
	var ids []string
	for _, note := range notes {
		ids = append(ids, note.ID)
	}
	return strings.Join(ids, ",")
}

func TestParseSearchQuery(t *testing.T) {
	query := parseSearchQuery("Flaky tag:PR-review  test tag:")
	if strings.Join(query.words, ",") != "flaky,test" || strings.Join(query.tags, ",") != "pr-review" {
		t.Errorf("query = %+v", query)
	}
}

func TestSearchResultsFilterAndSort(t *testing.T) {
	m := searchTestModel(t)
	for query, want := range map[string]string{
		"":                   "a,c,b",
		"flaky":              "a,b",
		"tag:pr-review":      "c",
		"tag:manual":         "a",
		"tag:console":        "c",
		"flaky tag:standup":  "b",
		"console tag:manual": "",
	} {
		m.searchInput.SetValue(query)
		if got := resultIDs(m.searchResults()); got != want {
			t.Errorf("%q = %q, want %q", query, got, want)
		}
	}
}

func TestSearchDateLabel(t *testing.T) {
	now := time.Date(2026, 10, 2, 9, 0, 0, 0, time.Local)
	for daysBack, want := range map[int]string{0: "today", 1: "yesterday", 7: "7 days ago", 8: "Thursday, 24 September 2026"} {
		if got := searchDateLabel(now.AddDate(0, 0, -daysBack), now); got != want {
			t.Errorf("%d days = %q, want %q", daysBack, got, want)
		}
	}
}

func TestHighlightKeepsCase(t *testing.T) {
	rendered := highlightMatches("Fix FLAKY flaky", []string{"flaky"}, lipgloss.NewStyle())
	if stripANSI(rendered) != "Fix FLAKY flaky" || strings.Count(rendered, searchHighlightStyle.Render("FLAKY")) != 1 || strings.Count(rendered, searchHighlightStyle.Render("flaky")) != 1 {
		t.Errorf("rendered = %q", rendered)
	}
}

func TestBodySnippet(t *testing.T) {
	body := strings.Repeat("word ", 30) + "needle\n" + strings.Repeat("tail ", 30)
	snippet, found := bodySnippet(body, []string{"needle"}, 40)
	if !found || !strings.HasPrefix(snippet, "…") || !strings.HasSuffix(snippet, "…") || !strings.Contains(snippet, "needle") || strings.Contains(snippet, "\n") {
		t.Errorf("snippet = %q", snippet)
	}
	if _, found := bodySnippet("nothing here", []string{"needle"}, 40); found {
		t.Error("unexpected snippet")
	}
}

func TestSearchModalRows(t *testing.T) {
	m := typeQuery(t, searchTestModel(t), "flaky")
	lines := plainLines(m.View())
	if !strings.Contains(lines[searchTopMargin], "╭") {
		t.Errorf("modal not near top:\n%s", strings.Join(lines[:4], "\n"))
	}
	view := strings.Join(lines, "\n")
	for _, want := range []string{"today", "Fix flaky test", "#manual", "2 days ago", "#standup", "DONE", "retried the Flaky console test twice"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Archived flaky") || strings.Contains(view, "Inbox flaky") {
		t.Errorf("archived/inbox listed:\n%s", view)
	}
	if strings.Count(view, "Fix flaky test") != 1 {
		t.Errorf("summary-only match should have no snippet:\n%s", view)
	}
}

func TestSearchEscClosesAndTermStays(t *testing.T) {
	m := typeQuery(t, searchTestModel(t), "flaky")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != ViewDashboard || m.ctrlCCount != 0 {
		t.Fatalf("mode = %d, ctrlC = %d", m.mode, m.ctrlCCount)
	}
	m = press(t, m, runes("/"))
	if m.mode != ViewSearch || m.searchInput.Value() != "flaky" {
		t.Errorf("mode = %d, query = %q", m.mode, m.searchInput.Value())
	}
}

func TestSearchPagingClamps(t *testing.T) {
	m := selectionTestModel(t)
	for index := range 60 {
		m.notes = append(m.notes, &model.Note{ID: fmt.Sprint(index), Summary: fmt.Sprintf("note %d", index), Status: model.StatusActive, Updated: time.Now().Add(-time.Duration(index) * time.Minute)})
	}
	m = press(t, m, runes("/"))
	m = press(t, m, tea.KeyMsg{Type: tea.KeyPgDown})
	if m.searchSelected <= 1 {
		t.Errorf("pgdown selected = %d", m.searchSelected)
	}
	pageSelected := m.searchSelected
	m = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlU})
	if m.searchSelected >= pageSelected || m.searchSelected < 0 {
		t.Errorf("ctrl+u selected = %d", m.searchSelected)
	}
	for range 20 {
		m = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlD})
	}
	if m.searchSelected != 59 {
		t.Errorf("ctrl+d end = %d", m.searchSelected)
	}
	if !strings.Contains(m.View(), "note 59") {
		t.Error("last row not visible")
	}
	for range 20 {
		m = press(t, m, tea.KeyMsg{Type: tea.KeyPgUp})
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyUp})
	if m.searchSelected != 0 {
		t.Errorf("pgup start = %d", m.searchSelected)
	}
}

func TestSearchPreviewCycleAndEdit(t *testing.T) {
	m := typeQuery(t, searchTestModel(t), "flaky")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.mode != ViewSearchPreview || m.searchPreviewNote().ID != "a" {
		t.Fatalf("mode = %d", m.mode)
	}
	m = press(t, m, runes("n"))
	if m.searchPreviewNote().ID != "b" || !strings.Contains(m.View(), "SEARCH PREVIEW") {
		t.Fatalf("after n = %s", m.searchPreviewNote().ID)
	}
	m = press(t, m, runes("n"))
	m = press(t, m, runes("p"))
	if m.searchPreviewNote().ID != "a" {
		t.Fatalf("after p = %s", m.searchPreviewNote().ID)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != ViewEdit || m.currentNote.ID != "a" {
		t.Fatalf("mode = %d", m.mode)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlO})
	if m.mode != ViewSearchPreview {
		t.Fatalf("after save mode = %d", m.mode)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != ViewSearchPreview {
		t.Fatalf("after cancel mode = %d", m.mode)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != ViewSearch {
		t.Fatalf("esc preview mode = %d", m.mode)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != ViewDashboard {
		t.Fatalf("esc search mode = %d", m.mode)
	}
}

func TestDateFilters(t *testing.T) {
	m := selectionTestModel(t)
	now := time.Now()
	christmas := time.Date(2026, 12, 24, 15, 0, 0, 0, time.Local)
	m.notes = []*model.Note{
		{ID: "d3", Summary: "three", Source: model.SourceManual, Status: model.StatusActive, Updated: now.AddDate(0, 0, -3)},
		{ID: "d4", Summary: "four", Source: model.SourceManual, Status: model.StatusActive, Updated: now.AddDate(0, 0, -4)},
		{ID: "d7", Summary: "seven", Source: model.SourcePRReview, Status: model.StatusActive, Updated: now.AddDate(0, 0, -7)},
		{ID: "d8", Summary: "eight", Source: model.SourceManual, Status: model.StatusActive, Updated: now.AddDate(0, 0, -8)},
		{ID: "d14", Summary: "fourteen", Source: model.SourceManual, Status: model.StatusActive, Updated: now.AddDate(0, 0, -14)},
		{ID: "d15", Summary: "fifteen", Source: model.SourceManual, Status: model.StatusActive, Updated: now.AddDate(0, 0, -15)},
		{ID: "d27", Summary: "twenty-seven", Source: model.SourceManual, Status: model.StatusActive, Updated: now.AddDate(0, 0, -27)},
		{ID: "d40", Summary: "forty", Source: model.SourceManual, Status: model.StatusActive, Updated: now.AddDate(0, 0, -40)},
		{ID: "d360", Summary: "year-ish", Source: model.SourceManual, Status: model.StatusActive, Updated: now.AddDate(0, 0, -360)},
		{ID: "d370", Summary: "over a year", Source: model.SourceManual, Status: model.StatusActive, Updated: now.AddDate(0, 0, -370)},
		{ID: "xmas", Summary: "christmas", Source: model.SourceManual, Status: model.StatusDone, Updated: christmas},
	}
	for query, want := range map[string]string{
		"date:3d":                  "d3",
		"date:7d":                  "d3,d4,d7",
		"date:1w":                  "d3,d4,d7",
		"date:2w":                  "d3,d4,d7,d8,d14",
		"date:0d":                  "",
		"date:1m":                  "d3,d4,d7,d8,d14,d15,d27",
		"date:1y":                  "d3,d4,d7,d8,d14,d15,d27,d40,d360",
		"date:24-12-2026":          "xmas",
		"date:7d tag:manual":       "d3,d4",
		"date:7d tag:manual three": "d3",
	} {
		m.searchInput.SetValue(query)
		if got := resultIDs(m.searchResults()); got != want {
			t.Errorf("%q = %q, want %q", query, got, want)
		}
	}
}

func TestInvalidDateIgnoredWithHint(t *testing.T) {
	m := typeQuery(t, searchTestModel(t), "flaky date:31-02-2026")
	if got := resultIDs(m.searchResults()); got != "a,b" {
		t.Errorf("results = %q", got)
	}
	if !strings.Contains(stripANSI(m.View()), invalidDateHint) {
		t.Error("hint missing")
	}
	for _, value := range []string{"abc", "7days", "d", "-1d", "3x"} {
		if _, valid := parseDateFilter(value); valid {
			t.Errorf("%q parsed", value)
		}
	}
}

func savedNote(id, summary string, updated time.Time) *model.Note {
	return &model.Note{ID: id, Summary: summary, Source: model.SourceManual, Status: model.StatusActive, Created: updated, Updated: updated, FilePath: "/tmp/" + id + ".md"}
}

func TestSearchPreviewDelete(t *testing.T) {
	m := selectionTestModel(t)
	now := time.Now()
	m.notes = []*model.Note{savedNote("a", "alpha", now), savedNote("b", "beta", now.Add(-time.Hour))}
	m = press(t, m, runes("/"))
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if !strings.Contains(stripANSI(m.View()), "Delete") {
		t.Error("footer missing delete")
	}
	m = press(t, m, runes("d"))
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != ViewSearchPreview || m.notes[0].Status != model.StatusActive {
		t.Fatalf("cancel mode = %d", m.mode)
	}
	m = press(t, m, runes("d"))
	m = press(t, m, runes("y"))
	if m.notes[0].Status != model.StatusArchived || m.mode != ViewSearchPreview || m.searchPreviewNote().ID != "b" {
		t.Fatalf("after delete mode = %d", m.mode)
	}
	m = press(t, m, runes("d"))
	m = press(t, m, runes("y"))
	if m.mode != ViewSearch {
		t.Fatalf("last delete mode = %d", m.mode)
	}
}

func TestDashboardPreviewDelete(t *testing.T) {
	m := selectionTestModel(t)
	m.notes = []*model.Note{savedNote("a", "alpha", m.currentDate), savedNote("b", "beta", m.currentDate)}
	selectNoteRow := func(id string) {
		for index, item := range m.allNavItems() {
			if item.Note != nil && item.Note.ID == id {
				m.selected = index
				return
			}
		}
		t.Fatalf("row %s not found", id)
	}
	selectNoteRow("a")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.mode != ViewPreview || !strings.Contains(stripANSI(m.View()), "Delete") {
		t.Fatalf("preview mode = %d", m.mode)
	}
	m = press(t, m, runes("d"))
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != ViewPreview {
		t.Fatalf("cancel mode = %d", m.mode)
	}
	firstIndex := m.selected
	m = press(t, m, runes("d"))
	m = press(t, m, runes("y"))
	if m.notes[0].Status != model.StatusArchived || m.mode != ViewPreview || m.selected != firstIndex {
		t.Fatalf("after delete mode = %d selected = %d", m.mode, m.selected)
	}
	if item, ok := m.selectedNavItem(); !ok || item.Note == nil || item.Note.ID != "b" {
		t.Fatalf("next item = %+v", item)
	}
	m.selected = len(m.allNavItems())
	m.afterPreviewArchive(ViewPreview)
	if m.mode != ViewDashboard || m.selected != len(m.allNavItems())-1 {
		t.Errorf("no next item: mode = %d selected = %d", m.mode, m.selected)
	}
}
