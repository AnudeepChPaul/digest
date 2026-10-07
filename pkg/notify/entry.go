package notify

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"app/pkg/paths"

	"gopkg.in/yaml.v3"
)

type Entry struct {
	NoteID     string    `yaml:"note_id"`
	Summary    string    `yaml:"summary"`
	Interval   string    `yaml:"interval"`
	NotifiedAt time.Time `yaml:"notified_at"`
	OpenURL    string    `yaml:"open_url,omitempty"`
	Terminal   string    `yaml:"terminal,omitempty"`
}

const entryExtension = ".yaml"

func Dir(root string) string {
	return filepath.Join(root, "notify")
}

func entryPath(root, noteID string) string {
	return filepath.Join(Dir(root), noteID+entryExtension)
}

func Save(root string, entry Entry) error {
	if err := os.MkdirAll(Dir(root), paths.PrivateDirMode); err != nil {
		return err
	}
	data, err := yaml.Marshal(entry)
	if err != nil {
		return err
	}
	temporaryPath := entryPath(root, entry.NoteID) + ".tmp"
	if err := os.WriteFile(temporaryPath, data, paths.PrivateFileMode); err != nil {
		return err
	}
	return os.Rename(temporaryPath, entryPath(root, entry.NoteID))
}

func Load(root, noteID string) (Entry, bool) {
	data, err := os.ReadFile(entryPath(root, noteID))
	if err != nil {
		return Entry{}, false
	}
	var entry Entry
	if yaml.Unmarshal(data, &entry) != nil {
		return Entry{}, false
	}
	return entry, true
}

func Remove(root, noteID string) error {
	if err := os.Remove(entryPath(root, noteID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func List(root string) ([]Entry, error) {
	files, err := os.ReadDir(Dir(root))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []Entry
	for _, file := range files {
		if file.IsDir() || strings.HasPrefix(file.Name(), ".") || !strings.HasSuffix(file.Name(), entryExtension) {
			continue
		}
		if entry, found := Load(root, strings.TrimSuffix(file.Name(), entryExtension)); found {
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].NoteID < entries[right].NoteID })
	return entries, nil
}
