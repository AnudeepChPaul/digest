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

	"github.com/achandrapaul/digest/pkg/model"
	"github.com/achandrapaul/digest/pkg/paths"
	"github.com/achandrapaul/digest/pkg/system"

	"gopkg.in/yaml.v3"
)

type NoteStore struct {
	Root    string
	rootErr error
}

func New(root string) *NoteStore {
	expanded, err := paths.Expand(root)
	if err != nil {
		return &NoteStore{Root: root, rootErr: fmt.Errorf("notes root: %w", err)}
	}
	return &NoteStore{Root: expanded}
}

type SkippedNote struct {
	File   string
	Reason error
}

type SkippedNotesError struct {
	Notes []SkippedNote
}

func (skipped *SkippedNotesError) Error() string {
	lines := make([]string, len(skipped.Notes))
	for index, note := range skipped.Notes {
		lines[index] = note.File + ": " + note.Reason.Error()
	}
	return strings.Join(lines, "; ")
}

type noteCollector struct {
	root    string
	notes   []*model.Note
	skipped []SkippedNote
}

func (collector *noteCollector) skip(path string, reason error) {
	file := strings.TrimPrefix(path, collector.root+string(filepath.Separator))
	collector.skipped = append(collector.skipped, SkippedNote{File: file, Reason: reason})
}

func (collector *noteCollector) read(path string) *model.Note {
	data, err := system.Read(path)
	if err == nil {
		var note *model.Note
		if note, err = parseNote(data, path); err == nil {
			return note
		}
	}
	collector.skip(path, err)
	return nil
}

func (collector *noteCollector) load(path string) *model.Note {
	note := collector.read(path)
	if note != nil {
		collector.notes = append(collector.notes, note)
	}
	return note
}

func (collector *noteCollector) skippedErr() error {
	if len(collector.skipped) == 0 {
		return nil
	}
	return &SkippedNotesError{Notes: collector.skipped}
}

const (
	noteExtension     = ".md"
	doneExtension     = ".done.md"
	archivedExtension = ".archived.md"
)

var (
	ErrNoteFileMissing = errors.New("note file is missing")
	ErrUnlockNote      = system.ErrUnlock
	ErrLockNote        = system.ErrLock
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
	if _, err := system.Stat(filepath.Join(dir, stem+noteExtension)); err == nil {
		return true
	}
	matches, _ := system.Glob(filepath.Join(dir, stem+"-*"))
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
	rel, err := filepath.Rel(s.Root, path)
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
	return s.write(n, createMissing)
}

func (s *NoteStore) write(n *model.Note, createMissing bool) error {
	if s.rootErr != nil {
		return s.rootErr
	}
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

	lockErr := writeNoteFile(currentPath, buf.Bytes(), isNew || createMissing)
	if lockErr != nil && !errors.Is(lockErr, ErrLockNote) {
		return lockErr
	}
	n.FilePath = currentPath

	if !IsStemID(n.ID) {
		return lockErr
	}
	targetPath := filepath.Join(filepath.Dir(currentPath), FileName(n))
	if targetPath == currentPath {
		return lockErr
	}
	lockErr = system.Rename(currentPath, targetPath)
	if lockErr != nil && !errors.Is(lockErr, ErrLockNote) {
		return fmt.Errorf("failed to rename note file: %w", lockErr)
	}
	n.FilePath = targetPath
	return lockErr
}

func writeNoteFile(path string, content []byte, create bool) error {
	write := system.WriteExisting
	if create {
		write = system.Write
	}
	err := write(path, content)
	if errors.Is(err, os.ErrNotExist) && !create {
		return fmt.Errorf("%w: %s", ErrNoteFileMissing, path)
	}
	if err != nil && !errors.Is(err, ErrUnlockNote) && !errors.Is(err, ErrLockNote) {
		return fmt.Errorf("failed to write note file: %w", err)
	}
	return err
}

func (s *NoteStore) Delete(n *model.Note) error {
	if n.FilePath == "" {
		return fmt.Errorf("note %q has no file to delete", n.Summary)
	}
	if s.rootErr != nil {
		return s.rootErr
	}
	if err := s.withinRoot(n.FilePath); err != nil {
		return err
	}
	saveLock.Lock()
	defer saveLock.Unlock()
	err := system.Remove(n.FilePath)
	switch {
	case err == nil, errors.Is(err, os.ErrNotExist):
		return nil
	case errors.Is(err, ErrUnlockNote):
		return err
	}
	return fmt.Errorf("failed to delete note file: %w", err)
}

func IsDonePath(path string) bool {
	return strings.HasSuffix(path, doneExtension)
}

func (s *NoteStore) LoadByID(id string) (*model.Note, error) {
	if s.rootErr != nil {
		return nil, s.rootErr
	}
	collector := &noteCollector{root: s.Root}
	monthDir := ""
	if IsStemID(id) {
		createdMillis, _ := strconv.ParseInt(id[11:], 10, 64)
		created := time.UnixMilli(createdMillis).Local()
		monthDir = filepath.Join(s.Root, created.Format("2006"), created.Format("01"))
		if note := collector.loadNamed(monthDir, id); note != nil {
			return note, collector.skippedErr()
		}
	}
	monthDirs, _ := system.Glob(filepath.Join(s.Root, "*", "*"))
	for _, dir := range monthDirs {
		if dir == monthDir {
			continue
		}
		if note := collector.loadNamed(dir, id); note != nil {
			return note, collector.skippedErr()
		}
	}
	missing := fmt.Errorf("note %s: %w", id, os.ErrNotExist)
	if skipped := collector.skippedErr(); skipped != nil {
		return nil, errors.Join(missing, skipped)
	}
	return nil, missing
}

func copyFinishTime(name string) time.Time {
	for _, extension := range []string{doneExtension, archivedExtension} {
		if !strings.HasSuffix(name, extension) {
			continue
		}
		if finished, named := finishTime(name, extension); named {
			return finished
		}
	}
	return time.Time{}
}

func (collector *noteCollector) loadNamed(dir, id string) *model.Note {
	matches, _ := system.Glob(filepath.Join(dir, id+"*"))
	var activeCopies, finishedCopies []string
	for _, path := range matches {
		switch name := filepath.Base(path); {
		case name == id+noteExtension:
			activeCopies = append(activeCopies, path)
		case strings.HasPrefix(name, id+"-"):
			finishedCopies = append(finishedCopies, path)
		}
	}
	slices.SortFunc(finishedCopies, func(first, second string) int {
		if order := copyFinishTime(filepath.Base(second)).Compare(copyFinishTime(filepath.Base(first))); order != 0 {
			return order
		}
		return strings.Compare(first, second)
	})
	for _, path := range append(activeCopies, finishedCopies...) {
		if note := collector.load(path); note != nil {
			return note
		}
	}
	return nil
}

func Load(path string) (*model.Note, error) {
	data, err := system.Read(path)
	if err != nil {
		return nil, err
	}

	note, err := parseNote(data, path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return note, nil
}

func parseNote(data []byte, path string) (*model.Note, error) {
	content := string(data)
	if !strings.HasPrefix(content, "---\n") {
		return nil, errors.New("no front matter: the file must start with ---")
	}

	parts := strings.SplitN(content[4:], "\n---\n", 2)
	if len(parts) < 2 {
		return nil, errors.New("front matter is not closed with ---")
	}

	var note model.Note
	frontmatter := spacedDatePattern.ReplaceAllString(parts[0], "${1}T${2}")
	if err := yaml.Unmarshal([]byte(frontmatter), &note); err != nil {
		return nil, fmt.Errorf("yaml parse error: %w", err)
	}
	if !note.Status.Known() {
		note.Status = model.StatusActive
	}

	note.Body = strings.TrimSpace(parts[1])
	note.FilePath = path
	return &note, nil
}

func (s *NoteStore) List() ([]*model.Note, error) {
	if s.rootErr != nil {
		return nil, s.rootErr
	}
	collector := &noteCollector{root: s.Root}
	if !system.Exists(s.Root) {
		return collector.notes, nil
	}

	err := system.Walk(s.Root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			collector.skip(path, err)
			return nil
		}
		if d.IsDir() {
			if path != s.Root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(d.Name(), noteExtension) {
			collector.load(path)
		}
		return nil
	})
	return collector.notes, errors.Join(err, collector.skippedErr())
}

func (s *NoteStore) FindByID(idPrefix string) (*model.Note, error) {
	notes, err := s.List()
	var skipped *SkippedNotesError
	if err != nil && !errors.As(err, &skipped) {
		return nil, err
	}

	var matches []*model.Note
	for _, n := range notes {
		if strings.HasPrefix(strings.ToUpper(n.ID), strings.ToUpper(idPrefix)) {
			matches = append(matches, n)
		}
	}

	if len(matches) == 0 {
		return nil, errors.Join(fmt.Errorf("note not found: %s", idPrefix), err)
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("ambiguous ID: %s", idPrefix)
	}

	return matches[0], nil
}
