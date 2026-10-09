package store

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/model"
	"github.com/AnudeepChPaul/digest/pkg/paths"

	"gopkg.in/yaml.v3"
)

type NoteStore struct {
	Root string
}

func New(root string) *NoteStore {
	return &NoteStore{Root: paths.Expand(root)}
}

const (
	noteExtension     = ".md"
	doneExtension     = ".done.md"
	archivedExtension = ".archived.md"
)

var (
	ErrNoteFileMissing = errors.New("note file is missing")
	saveLock           sync.Mutex
	spacedDatePattern  = regexp.MustCompile(`(?m)^((?:created|updated):[ \t]+['"]?\d{4}-\d{2}-\d{2}) (\d{2}:)`)
)

func noteStem(t time.Time) string {
	return t.Local().Format("02-01-2006") + "-" + strconv.FormatInt(t.UnixMilli(), 10)
}

func NoteID(created time.Time) string {
	return noteStem(created)
}

func IsStemID(id string) bool {
	if len(id) < len("02-01-2006-0") {
		return false
	}
	if _, err := time.Parse("02-01-2006", id[:10]); err != nil || id[10] != '-' {
		return false
	}
	_, err := strconv.ParseInt(id[11:], 10, 64)
	return err == nil
}

func FileName(n *model.Note) string {
	finished := "-" + strconv.FormatInt(n.Updated.UnixMilli(), 10)
	switch n.Status {
	case model.StatusDone:
		return n.ID + finished + doneExtension
	case model.StatusArchived:
		return n.ID + finished + archivedExtension
	}
	return n.ID + noteExtension
}

func idTaken(dir, stem string) bool {
	if _, err := os.Stat(filepath.Join(dir, stem+noteExtension)); err == nil {
		return true
	}
	matches, _ := filepath.Glob(filepath.Join(dir, stem+"-*"))
	return len(matches) > 0
}

func (s *NoteStore) newNoteID(created time.Time) (string, string) {
	dir := filepath.Join(s.Root, created.Local().Format("2006"), created.Local().Format("01"))
	candidate := created
	for {
		stem := noteStem(candidate)
		if !idTaken(dir, stem) {
			return dir, stem
		}
		candidate = candidate.Add(time.Millisecond)
	}
}

func (s *NoteStore) withinRoot(path string) error {
	absRoot, err := filepath.Abs(s.Root)
	if err != nil {
		return fmt.Errorf("failed to resolve notes root: %w", err)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve note path: %w", err)
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("note path %s is outside notes directory %s", path, s.Root)
	}
	return nil
}

func (s *NoteStore) Save(n *model.Note) error {
	return s.save(n, false)
}

func (s *NoteStore) Recreate(n *model.Note) error {
	return s.save(n, true)
}

func (s *NoteStore) save(n *model.Note, createMissing bool) error {
	saveLock.Lock()
	defer saveLock.Unlock()
	now := time.Now()
	if n.Created.IsZero() {
		n.Created = now
	}
	if n.Updated.IsZero() {
		n.Updated = now
	}

	currentPath := n.FilePath
	isNew := currentPath == ""
	if isNew {
		dir, stem := s.newNoteID(n.Created)
		if n.ID == "" {
			n.ID = stem
		}
		currentPath = filepath.Join(dir, stem+noteExtension)
	}
	if err := s.withinRoot(currentPath); err != nil {
		return err
	}

	yamlBytes, err := yaml.Marshal(n)
	if err != nil {
		return fmt.Errorf("failed to marshal frontmatter: %w", err)
	}
	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(yamlBytes)
	buf.WriteString("---\n")
	if n.Body != "" {
		buf.WriteString(strings.TrimSpace(n.Body))
		buf.WriteString("\n")
	}

	if err := os.MkdirAll(filepath.Dir(currentPath), paths.PrivateDirMode); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	if err := writeNoteFile(currentPath, buf.Bytes(), isNew || createMissing); err != nil {
		return err
	}
	n.FilePath = currentPath

	if !IsStemID(n.ID) {
		return nil
	}
	targetPath := filepath.Join(filepath.Dir(currentPath), FileName(n))
	if targetPath == currentPath {
		return nil
	}
	if err := os.Rename(currentPath, targetPath); err != nil {
		return fmt.Errorf("failed to rename note file: %w", err)
	}
	n.FilePath = targetPath
	return nil
}

func writeNoteFile(path string, content []byte, create bool) error {
	flags := os.O_WRONLY | os.O_TRUNC
	if create {
		flags |= os.O_CREATE
	}
	file, err := os.OpenFile(path, flags, paths.PrivateFileMode)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %s", ErrNoteFileMissing, path)
	}
	if err != nil {
		return fmt.Errorf("failed to write note file: %w", err)
	}
	if _, err := file.Write(content); err != nil {
		file.Close()
		return fmt.Errorf("failed to write note file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to write note file: %w", err)
	}
	return nil
}

func (s *NoteStore) Delete(n *model.Note) error {
	if n.FilePath == "" {
		return fmt.Errorf("note %q has no file to delete", n.Summary)
	}
	if err := s.withinRoot(n.FilePath); err != nil {
		return err
	}
	if err := os.Remove(n.FilePath); err != nil {
		return fmt.Errorf("failed to delete note file: %w", err)
	}
	return nil
}

func (s *NoteStore) LoadByID(id string) (*model.Note, error) {
	monthDir := ""
	if IsStemID(id) {
		createdMillis, _ := strconv.ParseInt(id[11:], 10, 64)
		created := time.UnixMilli(createdMillis).Local()
		monthDir = filepath.Join(s.Root, created.Format("2006"), created.Format("01"))
		if note := loadNamedNote(monthDir, id); note != nil {
			return note, nil
		}
	}
	monthDirs, _ := filepath.Glob(filepath.Join(s.Root, "*", "*"))
	for _, dir := range monthDirs {
		if dir == monthDir {
			continue
		}
		if note := loadNamedNote(dir, id); note != nil {
			return note, nil
		}
	}
	return nil, fmt.Errorf("note %s: %w", id, os.ErrNotExist)
}

func loadNamedNote(dir, id string) *model.Note {
	matches, _ := filepath.Glob(filepath.Join(dir, id+"*"))
	activeName := id + noteExtension
	slices.SortFunc(matches, func(first, second string) int {
		firstActive, secondActive := filepath.Base(first) == activeName, filepath.Base(second) == activeName
		if firstActive != secondActive {
			if firstActive {
				return -1
			}
			return 1
		}
		return strings.Compare(first, second)
	})
	for _, path := range matches {
		name := filepath.Base(path)
		if name != activeName && !strings.HasPrefix(name, id+"-") {
			continue
		}
		if note, err := Load(path); err == nil {
			return note
		}
	}
	return nil
}

func Load(path string) (*model.Note, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	content := string(data)
	if !strings.HasPrefix(content, "---\n") {
		return nil, fmt.Errorf("invalid format in %s", path)
	}

	parts := strings.SplitN(content[4:], "\n---\n", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid frontmatter in %s", path)
	}

	var note model.Note
	frontmatter := spacedDatePattern.ReplaceAllString(parts[0], "${1}T${2}")
	if err := yaml.Unmarshal([]byte(frontmatter), &note); err != nil {
		return nil, fmt.Errorf("yaml parse error in %s: %w", path, err)
	}

	note.Body = strings.TrimSpace(parts[1])
	note.FilePath = path
	return &note, nil
}

func (s *NoteStore) List() ([]*model.Note, error) {
	var notes []*model.Note

	if _, err := os.Stat(s.Root); os.IsNotExist(err) {
		return notes, nil
	}

	err := filepath.WalkDir(s.Root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != s.Root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		note, err := Load(path)
		if err == nil {
			notes = append(notes, note)
		}
		return nil
	})

	return notes, err
}

func (s *NoteStore) FindByID(idPrefix string) (*model.Note, error) {
	notes, err := s.List()
	if err != nil {
		return nil, err
	}

	var matches []*model.Note
	for _, n := range notes {
		if strings.HasPrefix(strings.ToUpper(n.ID), strings.ToUpper(idPrefix)) {
			matches = append(matches, n)
		}
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("note not found: %s", idPrefix)
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("ambiguous ID: %s", idPrefix)
	}

	return matches[0], nil
}
