package tui

import (
	"time"

	"github.com/achandrapaul/digest/pkg/appstate"
	"github.com/achandrapaul/digest/pkg/model"
	"github.com/achandrapaul/digest/pkg/store"

	tea "github.com/charmbracelet/bubbletea"
)

type appStateSavedMsg struct {
	err error
}

func saveAppStateCmd(root string, state appstate.State) tea.Cmd {
	return func() tea.Msg {
		return appStateSavedMsg{err: appstate.Save(root, state)}
	}
}

func (m *Model) loadAppState() {
	state, found, err := appstate.Load(m.cfg.Root())
	if err != nil {
		m.showError("STATE ERROR", err)
		return
	}
	m.appState, m.appStateKnown = state, found
}

type appStateRebuiltMsg struct {
	state appstate.State
	built bool
	err   error
}

func (m Model) rebuildAppStateCmd() tea.Cmd {
	if m.appStateKnown {
		return nil
	}
	noteStore, root, isWorkDay := m.store, m.cfg.Root(), m.cfg.IsWorkDay
	return func() tea.Msg {
		notes, err := noteStore.List()
		if !storeResultUsable(err) {
			return appStateRebuiltMsg{err: err}
		}
		state := appstate.FromNotes(notes, time.Now(), isWorkDay)
		return appStateRebuiltMsg{state: state, built: true, err: appstate.Save(root, state)}
	}
}

func (m Model) applyRebuiltAppState(msg appStateRebuiltMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.showError("STATE ERROR", msg.err)
	}
	if msg.built && !m.appStateKnown {
		m.appState, m.appStateKnown = msg.state, true
	}
	return m, nil
}

func (m *Model) appStateAfterSaves(saved []savedNoteResult) tea.Cmd {
	if !m.appStateKnown {
		return nil
	}
	state, changed := m.appState, false
	for _, result := range saved {
		var created, done bool
		state, created = state.NoteCreated(result.after.Created)
		if !store.IsDonePath(result.before.FilePath) && result.after.Status == model.StatusDone {
			state, done = state.MarkDone(time.Now(), m.cfg.IsWorkDay)
		}
		changed = changed || created || done
	}
	if !changed {
		return nil
	}
	m.appState = state
	return saveAppStateCmd(m.cfg.Root(), state)
}
