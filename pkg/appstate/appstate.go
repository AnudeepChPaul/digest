package appstate

import (
	"path/filepath"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/habit"
	"github.com/AnudeepChPaul/digest/pkg/model"
	"github.com/AnudeepChPaul/digest/pkg/system"
)

const (
	fileName    = ".app.state.json"
	dayLayout   = "2006-01-02"
	maxLookBack = 366
)

type State struct {
	FirstNoteCreated time.Time `json:"first_note_created"`
	Streak           int       `json:"streak"`
	LastDoneDay      string    `json:"last_done_day,omitempty"`
}

func Path(root string) string {
	return filepath.Join(root, fileName)
}

func Load(root string) (State, bool, error) {
	var state State
	found, err := system.ReadJSON(Path(root), &state)
	if err != nil {
		return State{}, false, err
	}
	return state, found, nil
}

func Save(root string, state State) error {
	return system.WriteJSON(Path(root), state)
}

func dayOf(moment time.Time) string {
	return moment.Format(dayLayout)
}

func previousWorkDay(now time.Time, isWorkDay func(time.Weekday) bool) string {
	day := now.AddDate(0, 0, -1)
	for range maxLookBack {
		if isWorkDay(day.Weekday()) {
			break
		}
		day = day.AddDate(0, 0, -1)
	}
	return dayOf(day)
}

func (s State) CurrentStreak(now time.Time, isWorkDay func(time.Weekday) bool) int {
	if s.LastDoneDay == "" {
		return 0
	}
	if s.LastDoneDay == dayOf(now) || s.LastDoneDay == previousWorkDay(now, isWorkDay) {
		return s.Streak
	}
	return 0
}

func (s State) MarkDone(now time.Time, isWorkDay func(time.Weekday) bool) (State, bool) {
	if !isWorkDay(now.Weekday()) || s.LastDoneDay == dayOf(now) {
		return s, false
	}
	s.Streak = s.CurrentStreak(now, isWorkDay) + 1
	s.LastDoneDay = dayOf(now)
	return s, true
}

func (s State) NoteCreated(created time.Time) (State, bool) {
	if created.IsZero() || (!s.FirstNoteCreated.IsZero() && !created.Before(s.FirstNoteCreated)) {
		return s, false
	}
	s.FirstNoteCreated = created
	return s, true
}

func FromNotes(notes []*model.Note, now time.Time, isWorkDay func(time.Weekday) bool) State {
	var state State
	today := dayOf(now)
	for _, note := range notes {
		state, _ = state.NoteCreated(note.Created)
		if note.Status != model.StatusDone || note.Updated.IsZero() || !isWorkDay(note.Updated.Weekday()) {
			continue
		}
		if doneDay := dayOf(note.Updated.In(now.Location())); doneDay <= today && doneDay > state.LastDoneDay {
			state.LastDoneDay = doneDay
		}
	}
	state.Streak = habit.Streak(notes, now, isWorkDay)
	return state
}
