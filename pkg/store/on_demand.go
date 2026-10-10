package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/achandrapaul/digest/pkg/model"
	"github.com/achandrapaul/digest/pkg/system"
)

func (s *NoteStore) ListDoneSince(since time.Time) ([]*model.Note, error) {
	if s.rootErr != nil {
		return nil, s.rootErr
	}
	var notes []*model.Note
	if !system.Exists(s.Root) {
		return notes, nil
	}
	collector := &noteCollector{root: s.Root}
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
		if !strings.HasSuffix(name, doneExtension) {
			return nil
		}
		if finished, named := finishTime(name, doneExtension); named && finished.Before(since) {
			return nil
		}
		if note := collector.read(path); note != nil && note.Status == model.StatusDone && !note.Updated.Before(since) {
			notes = append(notes, note)
		}
		return nil
	})
	return notes, errors.Join(err, collector.skippedErr())
}
