package tui

import (
	"fmt"

	"github.com/AnudeepChPaul/digest/pkg/model"
	"github.com/AnudeepChPaul/digest/pkg/review"
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
