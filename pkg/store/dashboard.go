package store

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/achandrapaul/digest/pkg/model"
	"github.com/achandrapaul/digest/pkg/system"
)

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

func (s *NoteStore) ListDashboard(viewedDay, previousDay time.Time) ([]*model.Note, error) {
	if s.rootErr != nil {
		return nil, s.rootErr
	}
	var notes []*model.Note
	if !system.Exists(s.Root) {
		return notes, nil
	}
	dayStart := startOfDay(viewedDay)
	dayEnd := dayStart.AddDate(0, 0, 1)
	previousDayStart := startOfDay(previousDay)
	previousDayEnd := previousDayStart.AddDate(0, 0, 1)
	collector := &noteCollector{root: s.Root}
	load := func(path string) { collector.load(path) }
	err := system.Walk(s.Root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			collector.skip(path, err)
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
			inViewedDay := !finished.Before(dayStart) && finished.Before(dayEnd)
			inPreviousDay := !finished.Before(previousDayStart) && finished.Before(previousDayEnd)
			if !named || inViewedDay || inPreviousDay {
				load(path)
			}
		case strings.HasSuffix(name, noteExtension):
			load(path)
		}
		return nil
	})
	return collector.notes, errors.Join(err, collector.skippedErr())
}
