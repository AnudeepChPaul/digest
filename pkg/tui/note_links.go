package tui

import (
	"slices"
	"strings"

	"github.com/AnudeepChPaul/digest/pkg/model"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func noteLinks(note *model.Note) []string {
	if note == nil {
		return nil
	}
	var links []string
	for _, link := range noteLinkPattern.FindAllString(note.Summary+"\n"+note.Body, -1) {
		link = strings.TrimRight(link, ".,;:")
		if !slices.Contains(links, link) {
			links = append(links, link)
		}
	}
	return links
}

func noteLinksLabel(links []string, onDashboard bool) string {
	label := "links"
	if len(links) == 1 {
		label = "link"
	}
	if onDashboard {
		return label
	}
	return "open " + label
}

func (m Model) openNoteLinks(tea.KeyMsg) (tea.Model, tea.Cmd) {
	item, ok := m.selectedNavItem()
	if !ok {
		return m, nil
	}
	links := noteLinks(item.Note)
	switch len(links) {
	case 0:
	case 1:
		_ = openURL(links[0])
	default:
		m.linkMenuItems = links
		m.linkMenuSelected = 0
		m.linkMenuReturnMode = m.mode
		m.mode = ViewLinkMenu
	}
	return m, nil
}

func (m Model) linkMenuDown(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.linkMenuSelected = min(m.linkMenuSelected+1, max(len(m.linkMenuItems)-1, 0))
	return m, nil
}

func (m Model) linkMenuUp(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.linkMenuSelected = max(m.linkMenuSelected-1, 0)
	return m, nil
}

func (m Model) closeLinkMenu(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = m.linkMenuReturnMode
	m.linkMenuItems = nil
	if m.mode == ViewPreview {
		m.updatePreviewViewport()
	}
	return m, nil
}

func (m Model) chooseLink(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.linkMenuSelected < len(m.linkMenuItems) {
		_ = openURL(m.linkMenuItems[m.linkMenuSelected])
	}
	return m.closeLinkMenu(msg)
}

func (m Model) renderLinkDropdown() string {
	maxLinkWidth := max(m.width-12, 20)
	var rows []string
	for index, link := range m.linkMenuItems {
		label := ansi.Truncate(link, maxLinkWidth, "…")
		if index == m.linkMenuSelected {
			rows = append(rows, hintKeyStyle.Render("› "+label))
		} else {
			rows = append(rows, hintTextStyle.Render("  "+label))
		}
	}
	return hintPillStyle.Render(strings.Join(rows, "\n"))
}

func (m Model) renderLinkMenu() string {
	dropdown := m.renderLinkDropdown()
	return m.overlayUnderSelectedRow(dropdown, m.rightAlignedColumn(dropdown))
}

func linkMenuBindings() []keyBinding {
	return []keyBinding{
		newKeyBinding(actionChooseLink, []string{"enter", "y", "Y"}, "y|enter", "open"),
		newKeyBinding(actionLinkMenuDown, []string{"j", "down"}, "j|k", "nav"),
		hiddenKeyBinding(actionLinkMenuUp, "k", "up"),
		newKeyBinding(actionCloseLinkMenu, []string{"esc", "n", "N"}, "esc", "cancel"),
	}
}
