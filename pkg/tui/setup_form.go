package tui

import (
	"reflect"
	"slices"
	"strings"

	"github.com/achandrapaul/digest/pkg/config"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type setupField int

const (
	setupFieldGit setupField = iota
	setupFieldRoots
	setupFieldWorkDays
	setupFieldMorning
	setupFieldEvening
	setupFieldHints
	setupFieldNotifications
	setupFieldCount
)

const setupFormLabelWidth = 16

func isSetupTextField(field setupField) bool {
	return field == setupFieldRoots || field == setupFieldMorning || field == setupFieldEvening
}

func (m Model) startSetupForm(configPath string) Model {
	m = m.startSetup(configPath)
	m.setup.form = true
	m.setup.results = runDoctor(m.cfg)
	m.setup.initialAnswers = cloneSetupAnswers(m.setup.answers)
	m.setup.notificationsOn = notificationsInstalled()
	m.setup.notificationsWereOn = m.setup.notificationsOn
	m.setup.spinner = spinner.New(spinner.WithSpinner(spinner.Dot))
	return m.focusSetupField(setupFieldGit)
}

func cloneSetupAnswers(answers config.SetupAnswers) config.SetupAnswers {
	answers.RepositoryRoots = slices.Clone(answers.RepositoryRoots)
	answers.WorkDays = slices.Clone(answers.WorkDays)
	return answers
}

func (m Model) pendingSetupAnswers() config.SetupAnswers {
	state := m.setup
	answers := cloneSetupAnswers(state.answers)
	value := strings.TrimSpace(m.setupInput.Value())
	field := state.field
	switch {
	case state.form && !state.editing:
		return answers
	case !state.form && state.step == setupStepRoots:
		field = setupFieldRoots
	case !state.form && state.step == setupStepTimes && state.editingEvening:
		field = setupFieldEvening
	case !state.form && state.step == setupStepTimes:
		field = setupFieldMorning
	case !state.form:
		return answers
	}
	switch field {
	case setupFieldRoots:
		answers.RepositoryRoots = parseSetupRoots(value)
		if len(answers.RepositoryRoots) == 0 && !state.form {
			answers.RepositoryRoots = []string{"~/Projects"}
		}
	case setupFieldMorning:
		answers.Morning = value
	case setupFieldEvening:
		answers.Evening = value
	}
	return answers
}

func (m Model) setupDirty() bool {
	return m.setup.notificationsOn != m.setup.notificationsWereOn || !reflect.DeepEqual(m.pendingSetupAnswers(), m.setup.initialAnswers)
}

func (m Model) setupSaving() bool {
	return m.mode == ViewSetup && m.setup != nil && m.setup.saving
}

func (m *Model) storeSetupField() {
	value := strings.TrimSpace(m.setupInput.Value())
	switch m.setup.field {
	case setupFieldRoots:
		m.setup.answers.RepositoryRoots = parseSetupRoots(value)
	case setupFieldMorning:
		m.setup.answers.Morning = value
	case setupFieldEvening:
		m.setup.answers.Evening = value
	}
}

func (m Model) focusSetupField(field setupField) Model {
	m.setup.field, m.setup.notice, m.setup.editing = field, "", false
	m.setupInput.Blur()
	return m
}

func (m Model) startEditingSetupField() Model {
	value := strings.Join(m.setup.answers.RepositoryRoots, ", ")
	m.setupInput.CharLimit = 0
	if m.setup.field == setupFieldMorning || m.setup.field == setupFieldEvening {
		m.setupInput.CharLimit = setupTimeCharLimit
		value = m.setup.answers.Morning
		if m.setup.field == setupFieldEvening {
			value = m.setup.answers.Evening
		}
	}
	m.setupInput.SetValue(value)
	m.setupInput.CursorEnd()
	m.setupInput.Focus()
	m.setup.editing, m.setup.notice, m.setup.editStartValue = true, "", value
	return m
}

func (m Model) setupFieldNext(tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !m.setup.form {
		return m.moveSetupStep(true)
	}
	return m.focusSetupField(min(m.setup.field+1, setupFieldCount-1)), nil
}

func (m Model) setupFieldPrevious(tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !m.setup.form {
		return m.moveSetupStep(false)
	}
	return m.focusSetupField(max(m.setup.field-1, setupFieldGit)), nil
}

func (m Model) setupFormTab(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !isSetupTextField(m.setup.field) {
		return m.setupFormToggle(msg)
	}
	if !m.setup.editing {
		return m.startEditingSetupField(), nil
	}
	m.storeSetupField()
	return m.focusSetupField(m.setup.field), nil
}

func (m Model) setupFormToggle(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	answers := &m.setup.answers
	switch m.setup.field {
	case setupFieldGit:
		answers.ShowGit = !answers.ShowGit
	case setupFieldWorkDays:
		return m.setupToggleDay(msg)
	case setupFieldHints:
		answers.ShowKeyHints = !answers.ShowKeyHints
	case setupFieldNotifications:
		m.setup.notificationsOn = !m.setup.notificationsOn
	}
	return m, nil
}

func (m Model) setupFormEscape(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.setup.editing && strings.TrimSpace(m.setupInput.Value()) != strings.TrimSpace(m.setup.editStartValue) {
		m.setup.undoingEdit, m.mode = true, ViewSetupDiscard
		return m, nil
	}
	if m.setup.editing {
		return m.focusSetupField(m.setup.field), nil
	}
	if m.setupDirty() {
		m.mode = ViewSetupDiscard
		return m, nil
	}
	return m.closeSetup(msg)
}

func (m Model) keepEditingSetup(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.setup.undoingEdit, m.mode = false, ViewSetup
	return m, nil
}

func (m Model) setupFormSave(tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !m.setupDirty() {
		return m, nil
	}
	if m.setup.editing {
		m.storeSetupField()
		m = m.focusSetupField(m.setup.field)
	}
	if len(m.setup.answers.WorkDays) == 0 {
		m = m.focusSetupField(setupFieldWorkDays)
		m.setup.notice = "Pick at least one day 🙂"
		return m, nil
	}
	for _, field := range []setupField{setupFieldMorning, setupFieldEvening} {
		clock := m.setup.answers.Morning
		if field == setupFieldEvening {
			clock = m.setup.answers.Evening
		}
		if _, err := config.ParseClock(clock); clock != "" && err != nil {
			m = m.focusSetupField(field)
			m.setup.notice = err.Error()
			return m, nil
		}
	}
	if len(m.setup.answers.RepositoryRoots) == 0 {
		m.setup.answers.RepositoryRoots = []string{"~/Projects"}
	}
	m.setup.saving, m.setup.notice = true, ""
	return m, tea.Batch(saveSetupCmd(m.setup.configPath, m.setup.answers, m.setup.backedUp, m.setup.notificationsWereOn, m.setup.notificationsOn), m.setup.spinner.Tick)
}

type setupSavedMsg struct {
	cfg      *config.Config
	backedUp bool
	err      error
}

func saveSetupCmd(configPath string, answers config.SetupAnswers, alreadyBackedUp, notificationsWereOn, notificationsOn bool) tea.Cmd {
	return func() tea.Msg {
		cfg, backedUp, err := writeSetupConfig(configPath, answers, alreadyBackedUp)
		switch {
		case err != nil || notificationsWereOn == notificationsOn:
		case notificationsOn:
			err = installNotifications(cfg)
		default:
			err = uninstallNotifications()
		}
		return setupSavedMsg{cfg: cfg, backedUp: backedUp, err: err}
	}
}

func (m Model) applySetupSaved(msg setupSavedMsg) (tea.Model, tea.Cmd) {
	if m.setup == nil {
		return m, nil
	}
	m.setup.backedUp = m.setup.backedUp || msg.backedUp
	if msg.err != nil {
		m.setup.saving, m.setup.notice = false, msg.err.Error()
		return m, nil
	}
	fresh := NewModel(msg.cfg, nil)
	fresh.width, fresh.height = m.width, m.height
	fresh.configPath = m.setup.configPath
	return fresh, fresh.Init()
}

func (m Model) tickSetupSpinner(msg spinner.TickMsg) (tea.Model, tea.Cmd) {
	if m.setup == nil || !m.setup.saving {
		return m, nil
	}
	var cmd tea.Cmd
	m.setup.spinner, cmd = m.setup.spinner.Update(msg)
	return m, cmd
}

func (m Model) setupFormBindings() []keyBinding {
	if m.setup.saving {
		return nil
	}
	escape := hiddenKeyBinding(actionSetupFormEscape, "esc")
	save := newKeyBinding(actionSetupFormSave, []string{"enter"}, "enter", "save")
	if m.setup.editing {
		return []keyBinding{newKeyBinding(actionSetupFormTab, []string{"tab"}, "tab", "done"), save, escape}
	}
	bindings := []keyBinding{
		newKeyBinding(actionSetupFieldNext, []string{"j", "down"}, "j|k", "move"),
		hiddenKeyBinding(actionSetupFieldPrevious, "k", "up"),
		newKeyBinding(actionSetupFormTab, []string{"tab"}, "tab", "change"),
		save,
		escape,
	}
	if !isSetupTextField(m.setup.field) {
		bindings = append(bindings, hiddenKeyBinding(actionSetupFormToggle, " ", "space"))
	}
	if m.setup.field == setupFieldWorkDays {
		bindings = append(bindings,
			hiddenKeyBinding(actionSetupDayLeft, "left", "h"),
			hiddenKeyBinding(actionSetupDayRight, "right", "l"),
		)
	}
	return bindings
}

func setupDiscardBindings() []keyBinding {
	return yesNoBindings(actionCloseSetup, "discard", actionKeepEditingSetup, "keep editing")
}

func (m Model) renderSetupDiscard(modalWidth int) string {
	title, question := " DISCARD CHANGES ", "Close setup without saving your changes?"
	if m.setup != nil && m.setup.undoingEdit {
		title, question = " UNDO CHANGE ", "Undo the change you're typing?"
	}
	content := lipgloss.JoinVertical(lipgloss.Left,
		deleteTitleStyle.Render(title),
		"",
		question,
		"",
		renderModalFooter(footerItemsFrom(setupDiscardBindings()), modalWidth-6),
	)
	return m.framedPopup(content, modalWidth)
}

func (m Model) renderSetupForm(modalWidth int) string {
	state := m.setup
	answers := state.answers
	onOff := func(enabled bool) string {
		if enabled {
			return setupAccent.Render("on")
		}
		return mutedStyle.Render("off")
	}
	textValue := func(field setupField, value string) string {
		if state.field == field && state.editing {
			return m.setupInput.View()
		}
		if value == "" {
			return mutedStyle.Render("off")
		}
		return value
	}
	fieldRows := map[setupField]string{
		setupFieldGit:           onOff(answers.ShowGit),
		setupFieldRoots:         textValue(setupFieldRoots, strings.Join(answers.RepositoryRoots, ", ")),
		setupFieldWorkDays:      "\n" + renderDayChips(state),
		setupFieldMorning:       textValue(setupFieldMorning, answers.Morning),
		setupFieldEvening:       textValue(setupFieldEvening, answers.Evening),
		setupFieldHints:         onOff(answers.ShowKeyHints),
		setupFieldNotifications: onOff(state.notificationsOn),
	}
	labels := map[setupField]string{
		setupFieldGit: "Git", setupFieldRoots: "Repos", setupFieldWorkDays: "Work days", setupFieldMorning: "Morning",
		setupFieldEvening: "Evening", setupFieldHints: "Key hints", setupFieldNotifications: "Notifications",
	}
	var lines []string
	focusedStart, focusedEnd := 0, 0
	for field := setupFieldGit; field < setupFieldCount; field++ {
		marker, label := "  ", lipgloss.NewStyle().Width(setupFormLabelWidth).Render(labels[field])
		if field == state.field {
			marker, label = setupAccent.Render("▸ "), setupAccent.Width(setupFormLabelWidth).Render(labels[field])
			focusedStart = len(lines)
		}
		for index, row := range strings.Split(fieldRows[field], "\n") {
			if index == 0 {
				lines = append(lines, marker+label+row)
			} else {
				lines = append(lines, "  "+row)
			}
		}
		if field == state.field {
			focusedEnd = len(lines) - 1
		}
	}
	lines = append(lines, "", "Tools")
	for _, result := range state.results {
		mark := "✓"
		if !result.Found && result.Note == "" {
			mark = "✗"
		}
		lines = append(lines, mark+" "+result.Name)
	}
	hint := "j|k move · tab change · ←/→ day · enter save · esc close"
	if state.editing {
		hint = "tab done · enter save · esc undo"
	}
	footer := []string{"", mutedStyle.Render(hint)}
	if state.saving {
		footer = []string{"", state.spinner.View() + " Saving…"}
	}
	if state.notice != "" {
		footer = append([]string{"", yellowBadgeStyle.Render(state.notice)}, footer...)
	}
	visible := visibleSetupLines(lines, focusedStart, focusedEnd, state.field == setupFieldNotifications, max(m.height-8-len(footer), 5))
	content := lipgloss.JoinVertical(lipgloss.Left, append(append([]string{modalTitleStyle.Render(" DIGEST SETUP "), ""}, visible...), footer...)...)
	return m.framedPopup(content, modalWidth)
}

func visibleSetupLines(lines []string, focusedStart, focusedEnd int, showEnd bool, maxLines int) []string {
	if len(lines) <= maxLines {
		return lines
	}
	offset := 0
	if focusedEnd >= maxLines {
		offset = focusedEnd - maxLines + 1
	}
	if showEnd {
		offset = len(lines) - maxLines
	}
	offset = max(0, min(offset, focusedStart, len(lines)-maxLines))
	return lines[offset : offset+maxLines]
}
