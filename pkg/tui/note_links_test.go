package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/AnudeepChPaul/digest/pkg/model"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	docsLink   = "https://docs.example.com/guide"
	pullLink   = "https://github.com/acme/console/pull/7"
	ticketLink = "https://jira.example.com/browse/PROJ-1"
)

func noteWithBody(t *testing.T, body string) Model {
	t.Helper()
	m := noteSelectedModel(t)
	m.notes[0].Body = body
	return m
}

func pressO(t *testing.T, m Model) Model {
	t.Helper()
	return press(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
}

func TestNoteLinksAreTrimmedAndDeduplicated(t *testing.T) {
	note := &model.Note{
		Summary: "Review " + pullLink,
		Body:    "See " + docsLink + ". Then " + pullLink + ", and (" + ticketLink + ");\nagain " + docsLink,
	}
	if got, want := noteLinks(note), []string{pullLink, docsLink, ticketLink}; !slices.Equal(got, want) {
		t.Errorf("noteLinks = %q, want %q", got, want)
	}
	if got := noteLinks(&model.Note{Summary: "no links", Body: "none here"}); len(got) != 0 {
		t.Errorf("noteLinks without links = %q", got)
	}
}

func TestOWithOneLinkOpensItDirectly(t *testing.T) {
	opened := stubOpenURL(t)
	for _, mode := range []ViewMode{ViewDashboard, ViewPreview} {
		*opened = nil
		m := noteWithBody(t, "read "+docsLink)
		if mode == ViewPreview {
			m.mode = ViewPreview
			m.updatePreviewViewport()
		}
		if m = pressO(t, m); m.mode != mode || !slices.Equal(*opened, []string{docsLink}) {
			t.Errorf("from %v: mode=%v opened=%v", mode, m.mode, *opened)
		}
	}
}

func TestOWithSeveralLinksOpensTheLinkMenu(t *testing.T) {
	opened := stubOpenURL(t)
	m := pressO(t, noteWithBody(t, docsLink+"\n"+pullLink))
	if m.mode != ViewLinkMenu || !slices.Equal(m.linkMenuItems, []string{docsLink, pullLink}) || len(*opened) != 0 {
		t.Errorf("mode=%v items=%v opened=%v", m.mode, m.linkMenuItems, *opened)
	}
}

func TestLinkMenuOpensTheChosenLinkAndReturns(t *testing.T) {
	opened := stubOpenURL(t)
	m := noteWithBody(t, docsLink+"\n"+pullLink+"\n"+ticketLink)
	m.mode = ViewPreview
	m.updatePreviewViewport()
	m = pressO(t, m)
	for _, key := range []rune("jjk") {
		m = press(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != ViewPreview || !slices.Equal(*opened, []string{pullLink}) {
		t.Errorf("mode=%v opened=%v", m.mode, *opened)
	}
}

func TestEscClosesTheLinkMenuWithoutOpening(t *testing.T) {
	opened := stubOpenURL(t)
	m := pressO(t, noteWithBody(t, docsLink+"\n"+pullLink))
	if m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc}); m.mode != ViewDashboard || len(*opened) != 0 {
		t.Errorf("mode=%v opened=%v", m.mode, *opened)
	}
}

func TestOFooterLabelFollowsTheLinkCount(t *testing.T) {
	cases := map[string]struct {
		body  string
		label string
	}{
		"none":    {"no links", ""},
		"one":     {docsLink, "Open link"},
		"several": {docsLink + " " + pullLink, "Open links"},
	}
	for name, c := range cases {
		m := noteWithBody(t, c.body)
		m.mode = ViewPreview
		m.updatePreviewViewport()
		footer := stripANSI(m.View())
		hasO := strings.Contains(footer, " o ")
		switch {
		case c.label == "" && hasO:
			t.Errorf("%s: o should be hidden:\n%s", name, footer)
		case c.label != "" && (!hasO || !strings.Contains(footer, strings.Fields(c.label)[0])):
			t.Errorf("%s: footer should offer o %s:\n%s", name, c.label, footer)
		}
	}
}

func TestLinkMenuUsesTheActionDropdownStyle(t *testing.T) {
	m := pressO(t, noteWithBody(t, docsLink+"\n"+pullLink))
	dropdown := stripANSI(m.renderLinkDropdown())
	lines := strings.Split(dropdown, "\n")
	selected, other := -1, -1
	for index, line := range lines {
		if strings.Contains(line, "› "+docsLink) {
			selected = index
		}
		if strings.Contains(line, "  "+pullLink) {
			other = index
		}
	}
	if selected < 0 || other <= selected {
		t.Errorf("first link should be selected with › and the next listed below:\n%s", dropdown)
	}
	if !strings.Contains(stripANSI(m.View()), docsLink) {
		t.Errorf("the menu should be drawn over the screen")
	}
}

func oLabel(t *testing.T, m Model, onDashboard bool) string {
	t.Helper()
	item, _ := m.selectedNavItem()
	for _, binding := range m.itemBindings(item, onDashboard) {
		if binding.action == actionOpenNoteLinks && !binding.hidden {
			return binding.binding.Help().Desc
		}
	}
	return ""
}

func TestOLabelDependsOnPlaceAndLinkCount(t *testing.T) {
	cases := []struct {
		body             string
		rowHint, preview string
	}{
		{"no links", "", ""},
		{docsLink, "link", "open link"},
		{docsLink + " " + pullLink, "links", "open links"},
	}
	for _, c := range cases {
		m := noteWithBody(t, c.body)
		if got := oLabel(t, m, true); got != c.rowHint {
			t.Errorf("%q row hint = %q, want %q", c.body, got, c.rowHint)
		}
		if got := oLabel(t, m, false); got != c.preview {
			t.Errorf("%q preview label = %q, want %q", c.body, got, c.preview)
		}
	}
}
