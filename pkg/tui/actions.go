package tui

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/automation"
	"github.com/AnudeepChPaul/digest/pkg/model"
	"github.com/AnudeepChPaul/digest/pkg/notify"
	"github.com/AnudeepChPaul/digest/pkg/system"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	actionNameNotify    = "notify"
	actionNameNotifyOff = "notify:off"
)

var typedIntervalPattern = regexp.MustCompile(`^[0-9]*[mhd]?$`)

type noteAction struct {
	name       string
	automation bool
}

type notifyEntriesMsg struct {
	entries map[string]notify.Entry
	err     error
}

type actionUsageSavedMsg struct {
	err error
}

func actionUsagePath(cacheDir string) string {
	return filepath.Join(cacheDir, "action-usage.json")
}

func loadActionUsage(cacheDir string) map[string]int {
	usage := map[string]int{}
	_, _ = system.ReadJSON(actionUsagePath(cacheDir), &usage)
	return usage
}

func saveActionUsageCmd(cacheDir string, usage map[string]int) tea.Cmd {
	snapshot := maps.Clone(usage)
	return func() tea.Msg {
		return actionUsageSavedMsg{err: system.WriteJSON(actionUsagePath(cacheDir), snapshot)}
	}
}

func noteIsActive(n *model.Note) bool {
	return n != nil && n.ID != "" && n.Status != model.StatusDone && n.Status != model.StatusArchived
}

func (m Model) actionTargetNote() (*model.Note, bool) {
	note, found := m.previewNote()
	return note, found && noteIsActive(note)
}

func (m Model) noteActions(n *model.Note) []noteAction {
	if !noteIsActive(n) {
		return nil
	}
	actions := []noteAction{{name: actionNameNotify}}
	if m.noteCanStartAutomation(n) {
		for _, spec := range automation.MatchAll(m.cfg.AutomationList(), n) {
			actions = append(actions, noteAction{name: spec.Name, automation: true})
		}
	}
	slices.SortStableFunc(actions, func(left, right noteAction) int {
		return m.actionUsage[right.name] - m.actionUsage[left.name]
	})
	if _, reminding := m.notifyEntries[n.ID]; reminding {
		actions = slices.Insert(actions, 0, noteAction{name: actionNameNotifyOff})
	}
	return actions
}

func (m Model) openActionMenu(tea.KeyMsg) (tea.Model, tea.Cmd) {
	note, found := m.actionTargetNote()
	if !found {
		return m, nil
	}
	m.actionMenuNoteID = note.ID
	m.actionMenuItems = m.noteActions(note)
	m.actionMenuSelected = 0
	m.actionMenuReturnMode = m.mode
	m.mode = ViewActionMenu
	return m, nil
}

func (m Model) actionMenuDown(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.actionMenuSelected = min(m.actionMenuSelected+1, max(len(m.actionMenuItems)-1, 0))
	return m, nil
}

func (m Model) actionMenuUp(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.actionMenuSelected = max(m.actionMenuSelected-1, 0)
	return m, nil
}

func (m Model) closeActionMenu(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = m.actionMenuReturnMode
	if m.mode == ViewPreview {
		m.updatePreviewViewport()
	}
	return m, nil
}

func (m Model) chooseAction(tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.actionMenuSelected >= len(m.actionMenuItems) {
		return m, nil
	}
	chosen := m.actionMenuItems[m.actionMenuSelected]
	if chosen.name == actionNameNotifyOff {
		return m.removeReminder()
	}
	m.actionUsage = maps.Clone(m.actionUsage)
	if m.actionUsage == nil {
		m.actionUsage = map[string]int{}
	}
	m.actionUsage[chosen.name]++
	saveUsage := saveActionUsageCmd(m.cfg.CacheDir(), m.actionUsage)
	switch {
	case !chosen.automation:
		m.notifyNotice = ""
		m.notifyInput.SetValue(m.notifyEntries[m.actionMenuNoteID].Interval)
		m.notifyInput.CursorEnd()
		m.notifyInput.Focus()
		m.mode = ViewNotifyInput
	default:
		m = m.askAutomationConfirm(m.actionMenuNoteID, chosen.name, automation.PhaseDraft)
		m.automationReturnMode = m.actionMenuReturnMode
	}
	return m, saveUsage
}

func (m Model) confirmNotify(tea.KeyMsg) (tea.Model, tea.Cmd) {
	interval, err := notify.ParseInterval(m.notifyInput.Value())
	if err != nil {
		m.notifyNotice = err.Error()
		return m, nil
	}
	note := m.noteByID(m.actionMenuNoteID)
	if note == nil {
		m.mode = ViewDashboard
		return m, nil
	}
	entry := notify.Entry{NoteID: note.ID, Summary: note.Summary, Interval: notify.IntervalLabel(interval), NotifiedAt: time.Now(), OpenURL: notePullRequestURL(note)}
	if entry.OpenURL == "" {
		entry.Terminal = os.Getenv("__CFBundleIdentifier")
	}
	m.notifyInput.Blur()
	m.mode = ViewDashboard
	entries := maps.Clone(m.notifyEntries)
	if entries == nil {
		entries = map[string]notify.Entry{}
	}
	entries[note.ID] = entry
	m.notifyEntries = entries
	root := m.cfg.Root()
	return m, func() tea.Msg {
		if err := notify.Save(root, entry); err != nil {
			return notifyEntriesMsg{err: err}
		}
		return listNotifyEntries(root)
	}
}

func (m Model) removeReminder() (tea.Model, tea.Cmd) {
	m.mode = ViewDashboard
	noteID, root := m.actionMenuNoteID, m.cfg.Root()
	entries := maps.Clone(m.notifyEntries)
	delete(entries, noteID)
	m.notifyEntries = entries
	return m, func() tea.Msg {
		if err := notify.Remove(root, noteID); err != nil {
			return notifyEntriesMsg{err: err}
		}
		return listNotifyEntries(root)
	}
}

func notifyInputAccepts(current string, msg tea.KeyMsg) bool {
	if msg.Type != tea.KeyRunes {
		return true
	}
	candidate := current + strings.ToLower(string(msg.Runes))
	return typedIntervalPattern.MatchString(candidate) && !strings.ContainsAny(candidate[:1], "mhd")
}

func (m Model) notifyInfoPill() string {
	return renderHintPill([]keyHint{{icon: "ⓘ", label: "1m–3d · whole numbers · m h d"}})
}

func (m Model) cancelNotify(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.notifyInput.Blur()
	m.mode = ViewDashboard
	return m, nil
}

func (m Model) noteByID(noteID string) *model.Note {
	for _, note := range m.notes {
		if note.ID == noteID {
			return note
		}
	}
	return nil
}

func listNotifyEntries(root string) notifyEntriesMsg {
	entries, err := notify.List(root)
	byNoteID := make(map[string]notify.Entry, len(entries))
	for _, entry := range entries {
		byNoteID[entry.NoteID] = entry
	}
	return notifyEntriesMsg{entries: byNoteID, err: err}
}

func refreshNotifyCmd(root string, notes []*model.Note) tea.Cmd {
	activeNoteIDs := map[string]bool{}
	for _, note := range notes {
		if noteIsActive(note) {
			activeNoteIDs[note.ID] = true
		}
	}
	return func() tea.Msg {
		entries, err := notify.List(root)
		if err != nil {
			return notifyEntriesMsg{err: err}
		}
		for _, entry := range entries {
			if !activeNoteIDs[entry.NoteID] {
				if err := notify.Remove(root, entry.NoteID); err != nil {
					return notifyEntriesMsg{err: err}
				}
			}
		}
		return listNotifyEntries(root)
	}
}

func (m Model) notifyTag(n *model.Note) string {
	entry, found := m.notifyEntries[n.ID]
	if !found || !noteIsActive(n) {
		return ""
	}
	return yellowBadgeStyle.Render("@notify:" + entry.Interval)
}

func (m Model) renderActionDropdown() string {
	labelWidth := 0
	for _, action := range m.actionMenuItems {
		labelWidth = max(labelWidth, lipgloss.Width("@"+action.name))
	}
	var rows []string
	for index, action := range m.actionMenuItems {
		label := fmt.Sprintf("%-*s", labelWidth, "@"+action.name)
		if index == m.actionMenuSelected {
			rows = append(rows, hintKeyStyle.Render("› "+label))
		} else {
			rows = append(rows, hintTextStyle.Render("  "+label))
		}
	}
	return hintPillStyle.Render(strings.Join(rows, "\n"))
}

func (m Model) renderActionMenu() string {
	dropdown := m.renderActionDropdown()
	return m.overlayUnderSelectedRow(dropdown, m.rightAlignedColumn(dropdown))
}

func actionMenuBindings() []keyBinding {
	return []keyBinding{
		newKeyBinding(actionChooseAction, []string{"enter", "y", "Y"}, "y|enter", "choose"),
		newKeyBinding(actionActionMenuDown, []string{"j", "down"}, "j|k", "nav"),
		hiddenKeyBinding(actionActionMenuUp, "k", "up"),
		newKeyBinding(actionCloseActionMenu, []string{"esc", "n", "N"}, "esc", "cancel"),
	}
}

func notifyInputBindings() []keyBinding {
	return []keyBinding{
		newKeyBinding(actionConfirmNotify, []string{"enter", "y", "Y"}, "y|enter", "confirm"),
		newKeyBinding(actionCancelNotify, []string{"esc"}, "esc", "cancel"),
	}
}
