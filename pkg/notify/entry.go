package notify

import (
	"errors"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/system"

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
	data, err := yaml.Marshal(entry)
	if err != nil {
		return err
	}
	return system.Write(entryPath(root, entry.NoteID), data)
}

func Load(root, noteID string) (Entry, bool) {
	data, err := system.Read(entryPath(root, noteID))
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
	if err := system.Remove(entryPath(root, noteID)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func List(root string) ([]Entry, error) {
	files, err := system.List(Dir(root))
	if errors.Is(err, fs.ErrNotExist) {
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
