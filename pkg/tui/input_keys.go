package tui

import (
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func disableInputDeleteShortcuts(input *textinput.Model) {
	input.KeyMap.DeleteBeforeCursor.SetEnabled(false)
	input.KeyMap.DeleteCharacterForward.SetKeys("delete")
}

func disableTextareaDeleteShortcuts(area *textarea.Model) {
	area.KeyMap.DeleteBeforeCursor.SetEnabled(false)
	area.KeyMap.DeleteCharacterForward.SetKeys("delete")
}

func (m Model) activeTextarea() *textarea.Model {
	if m.mode == ViewRejectComment {
		return m.rejectInput
	}
	return m.editor
}

func (m Model) moveEditorHalfPage(down bool) (tea.Model, tea.Cmd) {
	area := m.activeTextarea()
	step := tea.KeyMsg{Type: tea.KeyUp}
	if down {
		step = tea.KeyMsg{Type: tea.KeyDown}
	}
	for range max(area.Height()/2, 1) {
		*area, _ = area.Update(step)
	}
	return m, nil
}

func (m Model) editorHalfPageUp(tea.KeyMsg) (tea.Model, tea.Cmd) { return m.moveEditorHalfPage(false) }

func (m Model) editorHalfPageDown(tea.KeyMsg) (tea.Model, tea.Cmd) { return m.moveEditorHalfPage(true) }

func (m Model) openSetup(tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.configPath == "" {
		return m, nil
	}
	return m.startSetupForm(m.configPath), nil
}

func startEditorAtTop(area *textarea.Model) {
	*area, _ = area.Update(tea.KeyMsg{Type: tea.KeyCtrlHome})
}
