package tui

import (
	"fmt"
	"strings"
	"sync"

	"github.com/AnudeepChPaul/digest/pkg/model"
	"github.com/AnudeepChPaul/digest/pkg/review"
	"github.com/AnudeepChPaul/digest/pkg/store"
)

const orphanPROwner = "owner"

func prNoteRef(owner, repo string, number int) string {
	if owner == "" {
		owner = orphanPROwner
	}
	return fmt.Sprintf("%s/%s#%d", owner, repo, number)
}

func activityOwner(pr review.ActivityPR) string {
	if pr.Owner != "" {
		return pr.Owner
	}
	if ref, err := review.ParsePRURL(pr.URL); err == nil {
		return ref.Owner
	}
	return ""
}

type prNoteIndex struct {
	byRef      map[string]*model.Note
	byLegacyID map[string]*model.Note
}

func indexPRNotes(notes []*model.Note, source model.Source) prNoteIndex {
	index := prNoteIndex{byRef: map[string]*model.Note{}, byLegacyID: map[string]*model.Note{}}
	for _, note := range notes {
		if source != "" && note.Source != source {
			continue
		}
		if note.Ref != "" {
			index.byRef[note.Ref] = note
		} else {
			index.byLegacyID[note.ID] = note
		}
	}
	return index
}

func (index prNoteIndex) find(ref, repo string, number int, legacyID string) (*model.Note, bool) {
	if note, found := index.byRef[ref]; found {
		return note, true
	}
	if note, found := index.byRef[prNoteRef("", repo, number)]; found {
		return note, true
	}
	note, found := index.byLegacyID[legacyID]
	return note, found
}

func (index prNoteIndex) remember(note *model.Note) {
	index.byRef[note.Ref] = note
}

func (m Model) prNoteSnapshot(source model.Source) prNoteIndex {
	var copies []*model.Note
	for _, note := range m.notes {
		if source == "" || note.Source == source {
			noteCopy := *note
			copies = append(copies, &noteCopy)
		}
	}
	return indexPRNotes(copies, source)
}

var writtenPRNotes = struct {
	sync.Mutex
	ids map[string]string
}{ids: map[string]string{}}

type prNoteFinder struct {
	noteStore *store.NoteStore
	index     prNoteIndex
}

func newPRNoteFinder(noteStore *store.NoteStore, source model.Source, known prNoteIndex) *prNoteFinder {
	if known.byRef == nil {
		known = indexPRNotes(nil, source)
	}
	return &prNoteFinder{noteStore: noteStore, index: known}
}

func (finder *prNoteFinder) writtenKey(ref string) string {
	return finder.noteStore.Root + "\x00" + ref
}

func (finder *prNoteFinder) find(ref, repo string, number int, legacyID string) (*model.Note, bool, error) {
	noteID := ""
	if note, found := finder.index.find(ref, repo, number, legacyID); found {
		noteID = note.ID
	} else {
		writtenPRNotes.Lock()
		for _, candidate := range []string{ref, prNoteRef("", repo, number)} {
			if id, written := writtenPRNotes.ids[finder.writtenKey(candidate)]; written && noteID == "" {
				noteID = id
			}
		}
		writtenPRNotes.Unlock()
	}
	if noteID == "" {
		return nil, false, nil
	}
	note, err := finder.noteStore.LoadByID(noteID)
	if err != nil {
		return nil, false, fmt.Errorf("%s: %w", ref, err)
	}
	return note, true, nil
}

func (finder *prNoteFinder) remember(note *model.Note) {
	finder.index.remember(note)
	writtenPRNotes.Lock()
	writtenPRNotes.ids[finder.writtenKey(note.Ref)] = note.ID
	writtenPRNotes.Unlock()
}

func forgetWrittenPRNote(noteStore *store.NoteStore, note model.Note) {
	writtenPRNotes.Lock()
	defer writtenPRNotes.Unlock()
	for key, id := range writtenPRNotes.ids {
		if id == note.ID && strings.HasPrefix(key, noteStore.Root+"\x00") {
			delete(writtenPRNotes.ids, key)
		}
	}
}
