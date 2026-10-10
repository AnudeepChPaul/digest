package store

import (
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/model"
)

func TestListDoneSinceReadsOnlyNotesClosedSinceThen(t *testing.T) {
	noteStore := New(t.TempDir())
	now := time.Now()
	for _, note := range []*model.Note{
		{Summary: "closed recently", Status: model.StatusDone, Created: now.AddDate(0, 0, -3), Updated: now.Add(-time.Hour)},
		{Summary: "closed long ago", Status: model.StatusDone, Created: now.AddDate(0, 0, -30), Updated: now.AddDate(0, 0, -20)},
		{Summary: "still open", Status: model.StatusActive, Created: now, Updated: now},
	} {
		if err := noteStore.Save(note); err != nil {
			t.Fatal(err)
		}
	}
	notes, err := noteStore.ListDoneSince(now.AddDate(0, 0, -2))
	if err != nil || len(notes) != 1 || notes[0].Summary != "closed recently" {
		t.Errorf("notes = %+v, err %v", notes, err)
	}
}
