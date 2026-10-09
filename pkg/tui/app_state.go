package tui

import (
	"time"

	"github.com/AnudeepChPaul/digest/pkg/appstate"
	"github.com/AnudeepChPaul/digest/pkg/model"
	"github.com/AnudeepChPaul/digest/pkg/store"

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

func (m *Model) appStateFromAllNotes() tea.Cmd {
	if m.appStateKnown {
		return nil
	}
	m.appState, m.appStateKnown = appstate.FromNotes(m.notes, time.Now(), m.cfg.IsWorkDay), true
	return saveAppStateCmd(m.cfg.Root(), m.appState)
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
