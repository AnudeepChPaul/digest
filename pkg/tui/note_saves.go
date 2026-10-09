package tui

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/AnudeepChPaul/digest/pkg/automation"
	"github.com/AnudeepChPaul/digest/pkg/model"
	"github.com/AnudeepChPaul/digest/pkg/store"

	tea "github.com/charmbracelet/bubbletea"
)

const messageSourceNotes = "notes"

type noteSaveOrigin int

const (
	saveFromRow noteSaveOrigin = iota
	saveFromPreview
)

type savedNoteResult struct {
	target *model.Note
	before model.Note
	after  model.Note
}

type failedNoteSave struct {
	target    *model.Note
	attempted model.Note
	onDisk    *model.Note
	err       error
}

type notesSavedMsg struct {
	saved  []savedNoteResult
	failed []failedNoteSave
	origin noteSaveOrigin
}

type notesDeletedMsg struct {
	deleted []model.Note
	errs    []error
}

func (m Model) saveNotesCmd(notes ...*model.Note) tea.Cmd {
	return m.saveNotesWithOrigin(saveFromRow, notes)
}

func (m Model) saveNotesFromPreviewCmd(notes ...*model.Note) tea.Cmd {
	return m.saveNotesWithOrigin(saveFromPreview, notes)
}

func (m Model) saveNotesWithOrigin(origin noteSaveOrigin, notes []*model.Note) tea.Cmd {
	noteStore := m.store
	var targets []*model.Note
	var snapshots []model.Note
	for _, note := range notes {
		if note != nil {
			targets, snapshots = append(targets, note), append(snapshots, *note)
		}
	}
	return func() tea.Msg {
		msg := notesSavedMsg{origin: origin}
		for index, before := range snapshots {
			after := before
			if err := noteStore.Save(&after); err != nil {
				failed := failedNoteSave{target: targets[index], attempted: before, err: err}
				if before.FilePath != "" {
					if onDisk, loadErr := store.Load(before.FilePath); loadErr == nil {
						failed.onDisk = onDisk
					}
				}
				msg.failed = append(msg.failed, failed)
				continue
			}
			if reloaded, err := store.Load(after.FilePath); err == nil {
				after = *reloaded
			}
			msg.saved = append(msg.saved, savedNoteResult{target: targets[index], before: before, after: after})
		}
		return msg
	}
}

func recreateNoteCmd(noteStore *store.NoteStore, missing failedNoteSave) tea.Cmd {
	return func() tea.Msg {
		after := missing.attempted
		if err := noteStore.Recreate(&after); err != nil {
			return notesSavedMsg{failed: []failedNoteSave{{target: missing.target, attempted: missing.attempted, err: err}}}
		}
		return notesSavedMsg{saved: []savedNoteResult{{target: missing.target, before: missing.attempted, after: after}}}
	}
}

func (m Model) deleteNotesCmd(notes ...*model.Note) tea.Cmd {
	noteStore, automationRoot := m.store, m.automationRoot()
	copies := copyNotes(notes)
	return func() tea.Msg {
		var msg notesDeletedMsg
		for index := range copies {
			if err := noteStore.Delete(&copies[index]); err != nil {
				msg.errs = append(msg.errs, fmt.Errorf("delete %q: %w", copies[index].Summary, err))
				continue
			}
			forgetWrittenPRNote(noteStore, copies[index])
			msg.deleted = append(msg.deleted, copies[index])
			if err := automation.Dismiss(automationRoot, copies[index].ID); err != nil && !errors.Is(err, automation.ErrInvalidNoteID) {
				msg.errs = append(msg.errs, fmt.Errorf("remove automation for %q: %w", copies[index].Summary, err))
			}
		}
		return msg
	}
}

func copyNotes(notes []*model.Note) []model.Note {
	copies := make([]model.Note, 0, len(notes))
	for _, note := range notes {
		if note != nil {
			copies = append(copies, *note)
		}
	}
	return copies
}

func (m Model) noteIndex(target *model.Note, snapshot model.Note) int {
	if index := slices.Index(m.notes, target); index >= 0 {
		return index
	}
	return slices.IndexFunc(m.notes, func(note *model.Note) bool {
		return (snapshot.ID != "" && note.ID == snapshot.ID) || (snapshot.FilePath != "" && note.FilePath == snapshot.FilePath)
	})
}

func mergeSavedFields(note *model.Note, before, after model.Note) {
	if after.ID != before.ID {
		note.ID = after.ID
	}
	if after.FilePath != before.FilePath {
		note.FilePath = after.FilePath
	}
	if !after.Created.Equal(before.Created) {
		note.Created = after.Created
	}
	if !after.Updated.Equal(before.Updated) {
		note.Updated = after.Updated
	}
}

func (m Model) applySavedNotes(msg notesSavedMsg) (tea.Model, tea.Cmd) {
	for _, saved := range msg.saved {
		index := m.noteIndex(saved.target, saved.before)
		if index < 0 {
			m.notes = append(m.notes, saved.target)
			index = len(m.notes) - 1
		}
		if reflect.DeepEqual(*m.notes[index], saved.before) {
			*m.notes[index] = saved.after
		} else {
			mergeSavedFields(m.notes[index], saved.before, saved.after)
		}
		if m.awaitingNewNoteSave && saved.before.FilePath == "" {
			m.awaitingNewNoteSave = false
			m.selectNoteByID(saved.after.ID)
		}
	}
	var errorTexts []string
	for _, failed := range msg.failed {
		errorTexts = append(errorTexts, fmt.Sprintf("%q: %v", failed.attempted.Summary, failed.err))
		index := m.noteIndex(failed.target, failed.attempted)
		switch {
		case failed.onDisk != nil && index >= 0:
			*m.notes[index] = *failed.onDisk
		case errors.Is(failed.err, store.ErrNoteFileMissing) && m.missingSave == nil:
			missing := failed
			m.missingSave, m.missingSaveReturnMode = &missing, m.mode
			m.mode = ViewRecreateRow
			if msg.origin == saveFromPreview {
				m.mode = ViewRecreateConfirm
			}
		}
	}
	if len(errorTexts) > 0 {
		m.postMessage(messageSourceNotes, messageError, "save failed · "+strings.Join(errorTexts, " · "))
	}
	if m.mode == ViewArchived {
		m.refreshArchivedViewport()
	}
	return m, refreshNotifyCmd(m.cfg.Root(), m.notes)
}

func (m Model) applyDeletedNotes(msg notesDeletedMsg) (tea.Model, tea.Cmd) {
	for _, deleted := range msg.deleted {
		if index := m.noteIndex(nil, deleted); index >= 0 {
			m.notes = slices.Delete(m.notes, index, index+1)
		}
	}
	if len(msg.errs) > 0 {
		texts := make([]string, len(msg.errs))
		for index, err := range msg.errs {
			texts[index] = err.Error()
		}
		m.postMessage(messageSourceNotes, messageError, "delete failed · "+strings.Join(texts, " · "))
	}
	if m.mode == ViewArchived {
		m.refreshArchivedViewport()
	}
	return m, refreshNotifyCmd(m.cfg.Root(), m.notes)
}

func (m Model) confirmRecreateNote(tea.KeyMsg) (tea.Model, tea.Cmd) {
	missing := m.missingSave
	m.missingSave, m.mode = nil, m.missingSaveReturnMode
	if missing == nil {
		return m, nil
	}
	return m, recreateNoteCmd(m.store, *missing)
}

func (m Model) discardMissingNote(tea.KeyMsg) (tea.Model, tea.Cmd) {
	missing := m.missingSave
	m.missingSave, m.mode = nil, ViewDashboard
	if missing == nil {
		return m, nil
	}
	if index := m.noteIndex(missing.target, missing.attempted); index >= 0 {
		m.notes = slices.Delete(m.notes, index, index+1)
	}
	return m, refreshNotifyCmd(m.cfg.Root(), m.notes)
}

func recreateNoteBindings() []keyBinding {
	return []keyBinding{
		newKeyBinding(actionRecreateNote, []string{"y", "Y", "enter"}, "y", "create"),
		newKeyBinding(actionDiscardMissingNote, []string{"n", "N", "esc"}, "n", "discard"),
	}
}

func (m Model) missingNoteSummary() string {
	if m.missingSave == nil {
		return ""
	}
	return m.missingSave.attempted.Summary
}

func (m Model) renderRecreateConfirm(modalWidth int) string {
	prompt := fmt.Sprintf("The file for this note is gone. Do you want to create it again?\n\n\"%s\"", m.missingNoteSummary())
	content := strings.Join([]string{deleteTitleStyle.Render(" FILE MISSING "), "", prompt, "", renderModalFooter(footerItemsFrom(recreateNoteBindings()), modalWidth-6)}, "\n")
	return m.framedPopup(content, modalWidth)
}

func (m Model) recreateRowPill() string {
	return renderHintPill([]keyHint{{icon: "!", label: "file is gone, create it again?"}, {key: "y", label: "create"}, {key: "n", label: "discard"}})
}
