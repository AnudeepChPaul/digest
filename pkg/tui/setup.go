package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"app/pkg/config"
	"app/pkg/doctor"
	"app/pkg/notify"
	"app/pkg/paths"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type setupStep int

const (
	setupStepGit setupStep = iota
	setupStepRoots
	setupStepWorkDays
	setupStepTimes
	setupStepHints
	setupStepDoctor
)

const setupStepCount = 5

type setupState struct {
	configPath          string
	step                setupStep
	answers             config.SetupAnswers
	dayCursor           int
	editingEvening      bool
	notice              string
	results             []doctor.Result
	form                bool
	field               setupField
	notificationsOn     bool
	notificationsWereOn bool
	editing             bool
	saving              bool
	initialAnswers      config.SetupAnswers
	spinner             spinner.Model
}

var installNotifications = func(cfg *config.Config) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return notify.Install(executable, filepath.Join(cfg.LogsDir(), "notify.log"))
}

var notificationsInstalled = notify.Installed

var uninstallNotifications = notify.Uninstall

var runDoctor = func(cfg *config.Config) []doctor.Result {
	return doctor.Check(cfg, exec.LookPath)
}

const setupTimeCharLimit = 16

func parseSetupRoots(answer string) []string {
	var roots []string
	for _, root := range strings.FieldsFunc(answer, func(character rune) bool { return character == ',' || unicode.IsSpace(character) }) {
		if root != "-" {
			roots = append(roots, root)
		}
	}
	return roots
}

func newSetupInput() *textinput.Model {
	input := textinput.New()
	input.Prompt = "› "
	input.CharLimit = setupTimeCharLimit
	disableInputDeleteShortcuts(&input)
	return &input
}

func (m Model) startSetup(configPath string) Model {
	roots := m.cfg.GitRepositoryRoots
	if len(roots) == 0 {
		roots = []string{"~/Projects"}
	}
	workDays := m.cfg.WorkDays
	if len(workDays) == 0 {
		workDays = config.DefaultWorkDays
	}
	morning, evening := m.cfg.DigestNotifications.Morning, m.cfg.DigestNotifications.Evening
	if morning == "" && evening == "" {
		morning, evening = "09:30", "18:00"
	}
	m.setup = &setupState{configPath: configPath, answers: config.SetupAnswers{
		ShowGit: m.cfg.GitEnabled(), RepositoryRoots: roots, WorkDays: slices.Clone(workDays),
		Morning: morning, Evening: evening, ShowKeyHints: m.cfg.ShowKeyHints, TerminalApp: os.Getenv("__CFBundleIdentifier"),
	}}
	if m.setupInput == nil {
		m.setupInput = newSetupInput()
	}
	m.mode = ViewSetup
	return m
}

func (m Model) setupAnswerYes(tea.KeyMsg) (tea.Model, tea.Cmd) { return m.setupYesNo(true) }

func (m Model) setupAnswerNo(tea.KeyMsg) (tea.Model, tea.Cmd) { return m.setupYesNo(false) }

func (m Model) setupYesNo(yes bool) (tea.Model, tea.Cmd) {
	switch m.setup.step {
	case setupStepGit:
		m.setup.answers.ShowGit = yes
		if yes {
			return m.enterSetupStep(setupStepRoots), nil
		}
		return m.enterSetupStep(setupStepWorkDays), nil
	case setupStepHints:
		m.setup.answers.ShowKeyHints = yes
		return m.enterSetupStep(setupStepDoctor), nil
	case setupStepDoctor:
		if yes {
			return m.finishSetup(true)
		}
		return m.finishSetup(false)
	}
	return m, nil
}

func (m Model) enterSetupStep(step setupStep) Model {
	m.setup.step, m.setup.notice = step, ""
	m.setupInput.Blur()
	switch step {
	case setupStepRoots:
		m.setupInput.CharLimit = 0
		m.setupInput.SetValue(strings.Join(m.setup.answers.RepositoryRoots, ", "))
		m.setupInput.Focus()
	case setupStepTimes:
		m.setupInput.CharLimit = setupTimeCharLimit
		m.setup.editingEvening = false
		m.setupInput.SetValue(m.setup.answers.Morning)
		m.setupInput.Focus()
	case setupStepDoctor:
		m.setup.results = runDoctor(m.cfg)
	}
	m.setupInput.CursorEnd()
	return m
}

func (m *Model) storeSetupTime() {
	if m.setup.editingEvening {
		m.setup.answers.Evening = strings.TrimSpace(m.setupInput.Value())
	} else {
		m.setup.answers.Morning = strings.TrimSpace(m.setupInput.Value())
	}
}

func (m Model) setupSwitchTime(tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.setup.step != setupStepTimes {
		return m, nil
	}
	m.storeSetupTime()
	m.setup.editingEvening = !m.setup.editingEvening
	value := m.setup.answers.Morning
	if m.setup.editingEvening {
		value = m.setup.answers.Evening
	}
	m.setupInput.SetValue(value)
	m.setupInput.CursorEnd()
	return m, nil
}

func (m Model) setupConfirm(tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.setup.step {
	case setupStepRoots:
		roots := parseSetupRoots(m.setupInput.Value())
		if len(roots) == 0 {
			roots = []string{"~/Projects"}
		}
		m.setup.answers.RepositoryRoots = roots
		return m.enterSetupStep(setupStepWorkDays), nil
	case setupStepWorkDays:
		if len(m.setup.answers.WorkDays) == 0 {
			m.setup.notice = "Pick at least one day 🙂"
			return m, nil
		}
		return m.enterSetupStep(setupStepTimes), nil
	case setupStepTimes:
		m.storeSetupTime()
		for _, clock := range []string{m.setup.answers.Morning, m.setup.answers.Evening} {
			if _, err := config.ParseClock(clock); clock != "" && err != nil {
				m.setup.notice = err.Error()
				return m, nil
			}
		}
		return m.enterSetupStep(setupStepHints), nil
	case setupStepDoctor:
		return m.finishSetup(true)
	}
	return m, nil
}

func (m Model) setupSkip(tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.setup.step == setupStepDoctor {
		return m.finishSetup(false)
	}
	return m, nil
}

func (m Model) setupMoveDay(delta int) Model {
	m.setup.dayCursor = (m.setup.dayCursor + delta + len(config.WeekdayOrder)) % len(config.WeekdayOrder)
	return m
}

func (m Model) setupDayLeft(tea.KeyMsg) (tea.Model, tea.Cmd)  { return m.setupMoveDay(-1), nil }
func (m Model) setupDayRight(tea.KeyMsg) (tea.Model, tea.Cmd) { return m.setupMoveDay(1), nil }

func (m Model) setupToggleDay(tea.KeyMsg) (tea.Model, tea.Cmd) {
	day := config.WeekdayOrder[m.setup.dayCursor]
	answers := &m.setup.answers
	if index := slices.Index(answers.WorkDays, day); index >= 0 {
		answers.WorkDays = slices.Delete(slices.Clone(answers.WorkDays), index, index+1)
	} else {
		answers.WorkDays = append(slices.Clone(answers.WorkDays), day)
	}
	slices.SortFunc(answers.WorkDays, func(left, right string) int {
		return slices.Index(config.WeekdayOrder, left) - slices.Index(config.WeekdayOrder, right)
	})
	m.setup.notice = ""
	return m, nil
}

func writeSetupConfig(configPath string, answers config.SetupAnswers) (*config.Config, error) {
	existing, readErr := os.ReadFile(configPath)
	text := config.DefaultConfigYAML
	if readErr == nil && string(existing) != config.DefaultConfigYAML {
		text = string(existing)
		if err := os.WriteFile(configPath+".bak", existing, paths.PrivateFileMode); err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(configPath), paths.PrivateDirMode); err != nil {
		return nil, err
	}
	if err := os.WriteFile(configPath, []byte(config.RenderConfig(text, answers)), paths.PrivateFileMode); err != nil {
		return nil, err
	}
	return config.Load(configPath)
}

func (m Model) finishSetup(install bool) (tea.Model, tea.Cmd) {
	cfg, err := writeSetupConfig(m.setup.configPath, m.setup.answers)
	if err != nil {
		m.showError("SETUP ERROR", err)
		return m, nil
	}
	if install {
		if err := installNotifications(cfg); err != nil {
			m.showError("NOTIFICATIONS", err)
			return m, nil
		}
	}
	fresh := NewModel(cfg, nil)
	fresh.width, fresh.height = m.width, m.height
	fresh.configPath = m.setup.configPath
	return fresh, fresh.Init()
}

var (
	setupAccent    = lipgloss.NewStyle().Foreground(colourAccent).Bold(true)
	workDayChip    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colourAccent).Foreground(colourText).Padding(0, 1)
	offDayChip     = workDayChip.BorderForeground(colourOverlay).Foreground(colourOverlay)
	cursorDayLabel = lipgloss.NewStyle().Underline(true).Bold(true)
)

func renderDayChips(state *setupState) string {
	var chips []string
	for index, day := range config.WeekdayOrder {
		label := strings.ToUpper(day[:1]) + day[1:]
		if index == state.dayCursor {
			label = cursorDayLabel.Render(label)
		}
		chip := offDayChip
		if slices.Contains(state.answers.WorkDays, day) {
			chip = workDayChip
		}
		chips = append(chips, chip.Render(label))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, chips...)
}

func (m Model) renderSetup(modalWidth int) string {
	state := m.setup
	if state.form {
		return m.renderSetupForm(modalWidth)
	}
	progress := mutedStyle.Render(fmt.Sprintf("%d/%d", min(int(state.step)+1, setupStepCount+1), setupStepCount+1))
	var body []string
	switch state.step {
	case setupStepGit:
		body = []string{"👋 Hey! Let's get digest ready — a few quick questions.", "", "Do you work with git repos? " + setupAccent.Render("y/n")}
	case setupStepRoots:
		body = []string{"📁 Where do your repos live? (comma separated)", "", m.setupInput.View()}
	case setupStepWorkDays:
		body = []string{"🗓  Which days do you work?", mutedStyle.Render("Your weeks run Monday → Sunday."), "", renderDayChips(state), "", mutedStyle.Render("←/→ move · space toggle · enter next")}
	case setupStepTimes:
		morning, evening := "☀️  morning "+state.answers.Morning, "🌙 evening "+state.answers.Evening
		if state.editingEvening {
			evening = "🌙 evening " + m.setupInput.View()
		} else {
			morning = "☀️  morning " + m.setupInput.View()
		}
		body = []string{"⏰ When should I nudge you with a quick summary?", "", morning, evening, "", mutedStyle.Render("24h HH:MM · empty turns one off · tab switch · enter next")}
	case setupStepHints:
		body = []string{"💡 Want little key hints while you learn?", mutedStyle.Render("They pop up under the selected row after a second."), "", setupAccent.Render("y/n")}
	case setupStepDoctor:
		body = []string{"🩺 Quick check of your tools:", ""}
		for _, result := range state.results {
			mark := "✓"
			if !result.Found && result.Note == "" {
				mark = "✗"
			}
			body = append(body, fmt.Sprintf("%s %s", mark, result.Name))
		}
		body = append(body, "", "🔔 Turn on notifications? "+setupAccent.Render("y|enter")+mutedStyle.Render(" · n to skip"))
	}
	if state.notice != "" {
		body = append(body, "", yellowBadgeStyle.Render(state.notice))
	}
	body = append(body, "", mutedStyle.Render("esc close"))
	top := []string{modalTitleStyle.Render(" DIGEST SETUP ") + "  " + progress, ""}
	if answered := setupAnswersSoFar(state); len(answered) > 0 {
		top = append(top, append(answered, "")...)
	}
	content := lipgloss.JoinVertical(lipgloss.Left, append(top, body...)...)
	return m.framedPopup(content, modalWidth)
}

func (m Model) setupBindings() []keyBinding {
	switch m.setup.step {
	case setupStepGit, setupStepHints:
		return []keyBinding{
			newKeyBinding(actionSetupYes, []string{"y", "Y", "enter"}, "y", "yes"),
			newKeyBinding(actionSetupNo, []string{"n", "N"}, "n", "no"),
		}
	case setupStepWorkDays:
		return []keyBinding{
			newKeyBinding(actionSetupDayLeft, []string{"left", "h"}, "←", "left"),
			newKeyBinding(actionSetupDayRight, []string{"right", "l"}, "→", "right"),
			newKeyBinding(actionSetupToggleDay, []string{" ", "space"}, "space", "toggle"),
			newKeyBinding(actionSetupConfirm, []string{"enter"}, "enter", "next"),
		}
	case setupStepDoctor:
		return []keyBinding{
			newKeyBinding(actionSetupConfirm, []string{"y", "Y", "enter"}, "y|enter", "install"),
			newKeyBinding(actionSetupSkip, []string{"n", "N"}, "n", "skip"),
		}
	}
	return []keyBinding{
		newKeyBinding(actionSetupSwitchTime, []string{"tab"}, "tab", "switch"),
		newKeyBinding(actionSetupConfirm, []string{"enter"}, "enter", "next"),
	}
}

func (m Model) setupKeyBindings() []keyBinding {
	if m.setup.form {
		return m.setupFormBindings()
	}
	return append(m.setupBindings(), hiddenKeyBinding(actionCloseSetup, "esc"))
}

func (m Model) closeSetup(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.setup = nil
	m.mode = ViewDashboard
	return m, nil
}

func setupAnswersSoFar(state *setupState) []string {
	answers := state.answers
	var answered []string
	add := func(label, value string) {
		answered = append(answered, mutedStyle.Render(fmt.Sprintf("✓ %-10s %s", label, value)))
	}
	onOff := func(enabled bool, on, off string) string {
		if enabled {
			return on
		}
		return off
	}
	if state.step > setupStepGit {
		add("git", onOff(answers.ShowGit, "yes", "no"))
	}
	if state.step > setupStepRoots && answers.ShowGit {
		add("repos", strings.Join(answers.RepositoryRoots, ", "))
	}
	if state.step > setupStepWorkDays {
		var days []string
		for _, day := range config.WeekdayOrder {
			if slices.Contains(answers.WorkDays, day) {
				days = append(days, strings.ToUpper(day[:1])+day[1:])
			}
		}
		add("work days", strings.Join(days, " "))
	}
	if state.step > setupStepTimes {
		add("summaries", onOff(answers.Morning != "", answers.Morning, "off")+" · "+onOff(answers.Evening != "", answers.Evening, "off"))
	}
	if state.step > setupStepHints {
		add("key hints", onOff(answers.ShowKeyHints, "on", "off"))
	}
	return answered
}
