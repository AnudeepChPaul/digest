package appstate

import (
	"os"
	"testing"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/model"
	"github.com/AnudeepChPaul/digest/pkg/paths"
)

func weekdaysOnly(day time.Weekday) bool {
	return day != time.Saturday && day != time.Sunday
}

var friday = time.Date(2026, 10, 9, 15, 0, 0, 0, time.Local)

func TestSaveAndLoadRoundTrip(t *testing.T) {
	root := t.TempDir()
	if _, found, err := Load(root); found || err != nil {
		t.Fatalf("missing file = %v, %v", found, err)
	}
	state := State{FirstNoteCreated: friday.AddDate(-1, 0, 0), Streak: 4, LastDoneDay: "2026-10-09"}
	if err := Save(root, state); err != nil {
		t.Fatal(err)
	}
	loaded, found, err := Load(root)
	if err != nil || !found || !loaded.FirstNoteCreated.Equal(state.FirstNoteCreated) || loaded.Streak != 4 || loaded.LastDoneDay != "2026-10-09" {
		t.Fatalf("loaded = %+v, %v, %v", loaded, found, err)
	}
	if info, err := os.Stat(Path(root)); err != nil || info.Mode().Perm() != paths.PrivateFileMode {
		t.Errorf("state file should be owner only: %v", err)
	}
}

func TestCorruptFileIsAnError(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(Path(root), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Load(root); err == nil {
		t.Error("a corrupt state file should be reported")
	}
}

func TestCurrentStreak(t *testing.T) {
	cases := []struct {
		name     string
		lastDone string
		now      time.Time
		want     int
	}{
		{"done today", "2026-10-09", friday, 3},
		{"done on the previous work day", "2026-10-08", friday, 3},
		{"weekend keeps friday's streak", "2026-10-09", friday.AddDate(0, 0, 2), 3},
		{"monday still counts friday", "2026-10-09", friday.AddDate(0, 0, 3), 3},
		{"a missed work day ends it", "2026-10-07", friday, 0},
		{"never done", "", friday, 0},
	}
	for _, tc := range cases {
		state := State{Streak: 3, LastDoneDay: tc.lastDone}
		if got := state.CurrentStreak(tc.now, weekdaysOnly); got != tc.want {
			t.Errorf("%s: streak = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestMarkDone(t *testing.T) {
	state := State{Streak: 3, LastDoneDay: "2026-10-08"}
	next, changed := state.MarkDone(friday, weekdaysOnly)
	if !changed || next.Streak != 4 || next.LastDoneDay != "2026-10-09" {
		t.Fatalf("first done today = %+v, %v", next, changed)
	}
	if again, changed := next.MarkDone(friday.Add(time.Hour), weekdaysOnly); changed || again != next {
		t.Errorf("a second done the same day should change nothing: %+v, %v", again, changed)
	}
	if weekend, changed := next.MarkDone(friday.AddDate(0, 0, 1), weekdaysOnly); changed || weekend != next {
		t.Errorf("a done on a non-work day should change nothing: %+v, %v", weekend, changed)
	}
	missed := State{Streak: 3, LastDoneDay: "2026-10-06"}
	if reset, changed := missed.MarkDone(friday, weekdaysOnly); !changed || reset.Streak != 1 {
		t.Errorf("after a missed work day the streak restarts at 1: %+v", reset)
	}
}

func TestFromNotes(t *testing.T) {
	notes := []*model.Note{
		{Status: model.StatusActive, Created: friday.AddDate(0, -2, 0)},
		{Status: model.StatusDone, Created: friday.AddDate(0, 0, -3), Updated: friday.AddDate(0, 0, -1)},
		{Status: model.StatusDone, Created: friday.AddDate(0, 0, -3), Updated: friday.AddDate(0, 0, -2)},
		{Status: model.StatusDone, Created: friday.AddDate(0, 0, -3), Updated: friday.AddDate(0, 0, -6)},
	}
	state := FromNotes(notes, friday, weekdaysOnly)
	if !state.FirstNoteCreated.Equal(friday.AddDate(0, -2, 0)) || state.Streak != 2 || state.LastDoneDay != "2026-10-08" {
		t.Errorf("state = %+v", state)
	}
}
