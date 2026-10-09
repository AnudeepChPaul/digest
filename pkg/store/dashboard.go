package store

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/model"
	"github.com/AnudeepChPaul/digest/pkg/system"
)

type doneFile struct {
	path     string
	finished time.Time
}

func finishTime(name, extension string) (time.Time, bool) {
	stem := strings.TrimSuffix(name, extension)
	separator := strings.LastIndex(stem, "-")
	if separator < 0 || !IsStemID(stem[:separator]) {
		return time.Time{}, false
	}
	millis, err := strconv.ParseInt(stem[separator+1:], 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.UnixMilli(millis), true
}

func startOfDay(moment time.Time) time.Time {
	local := moment.Local()
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.Local)
}

func (s *NoteStore) ListDashboard(viewedDay time.Time) ([]*model.Note, error) {
	var notes []*model.Note
	if !system.Exists(s.Root) {
		return notes, nil
	}
	dayStart := startOfDay(viewedDay)
	dayEnd := dayStart.AddDate(0, 0, 1)
	var earlierDone []doneFile
	load := func(path string) {
		if note, err := Load(path); err == nil {
			notes = append(notes, note)
		}
	}
	err := system.Walk(s.Root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := entry.Name()
		if entry.IsDir() {
			if path != s.Root && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		switch {
		case strings.HasSuffix(name, archivedExtension):
			if _, named := finishTime(name, archivedExtension); !named {
				load(path)
			}
		case strings.HasSuffix(name, doneExtension):
			finished, named := finishTime(name, doneExtension)
			switch {
			case !named || (!finished.Before(dayStart) && finished.Before(dayEnd)):
				load(path)
			case finished.Before(dayStart):
				earlierDone = append(earlierDone, doneFile{path: path, finished: finished})
			}
		case strings.HasSuffix(name, noteExtension):
			load(path)
		}
		return nil
	})
	if err != nil {
		return notes, err
	}

	latestMark := latestPreviousDayMark(notes, dayStart)
	slices.SortFunc(earlierDone, func(a, b doneFile) int { return b.finished.Compare(a.finished) })
	for _, file := range earlierDone {
		if !latestMark.IsZero() && file.finished.Before(startOfDay(latestMark)) {
			break
		}
		note, err := Load(file.path)
		if err != nil {
			continue
		}
		notes = append(notes, note)
		if mark := latestPreviousDayMark([]*model.Note{note}, dayStart); mark.After(latestMark) {
			latestMark = mark
		}
	}
	return notes, nil
}

func latestPreviousDayMark(notes []*model.Note, dayStart time.Time) time.Time {
	var latest time.Time
	for _, note := range notes {
		if !note.MarksPreviousDay() {
			continue
		}
		for _, stamp := range []time.Time{note.Created, note.Updated} {
			if stamp.Before(dayStart) && stamp.After(latest) {
				latest = stamp
			}
		}
	}
	return latest
}
