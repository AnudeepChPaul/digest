package store

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"app/pkg/model"
	"app/pkg/paths"

	"gopkg.in/yaml.v3"
)

type NoteStore struct {
	Root string
}

func New(root string) *NoteStore {
	return &NoteStore{Root: paths.Expand(root)}
}

func noteStem(t time.Time) string {
	return t.Format("02-01-2006") + "-" + strconv.FormatInt(t.UnixMilli(), 10)
}

func (s *NoteStore) newNotePath(created time.Time) (string, string) {
	dir := filepath.Join(s.Root, created.Format("2006"), created.Format("01"))
	candidate := created
	for {
		stem := noteStem(candidate)
		path := filepath.Join(dir, stem+".md")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return path, stem
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
	now := time.Now()
	if n.Created.IsZero() {
		n.Created = now
	}
	if n.Updated.IsZero() {
		n.Updated = now
	}

	targetPath := n.FilePath
	if targetPath == "" {
		path, stem := s.newNotePath(n.Created)
		targetPath = path
		if n.ID == "" {
			n.ID = stem
		}
	}

	if err := s.withinRoot(targetPath); err != nil {
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

	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(targetPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write note file: %w", err)
	}

	n.FilePath = targetPath
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
	if err := yaml.Unmarshal([]byte(parts[0]), &note); err != nil {
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
