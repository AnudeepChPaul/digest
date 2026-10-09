package tui

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/model"
	"github.com/AnudeepChPaul/digest/pkg/paths"
	"github.com/AnudeepChPaul/digest/pkg/system"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	searchExportTimeFormat = "2006-01-02 15:04"
	searchExportFileFormat = "search-2006-01-02-150405.csv"
	nothingToExportNotice  = "Nothing to export"
)

var searchCSVHeader = []string{"updated", "created", "status", "tags", "summary", "body"}

var searchExportDir = func() string { return paths.Expand("~/Downloads") }

var copyExportPath = copyToClipboard

type searchExportedMsg struct {
	path  string
	count int
	err   error
}

func formatExportTime(moment time.Time) string {
	if moment.IsZero() {
		return ""
	}
	return moment.Local().Format(searchExportTimeFormat)
}

func searchCSVRecord(note *model.Note) []string {
	return []string{
		formatExportTime(note.Updated),
		formatExportTime(note.Created),
		string(note.Status),
		strings.Join(noteTags(note), ";"),
		note.Summary,
		note.Body,
	}
}

func writeSearchCSV(dir string, notes []*model.Note, now time.Time) (string, error) {
	if err := system.MkdirAllWithMode(dir, 0755); err != nil {
		return "", err
	}
	var encoded bytes.Buffer
	records := [][]string{searchCSVHeader}
	for _, note := range notes {
		records = append(records, searchCSVRecord(note))
	}
	if err := csv.NewWriter(&encoded).WriteAll(records); err != nil {
		return "", err
	}
	exportPath := filepath.Join(dir, now.Local().Format(searchExportFileFormat))
	if err := system.Write(exportPath, encoded.Bytes()); err != nil {
		return "", err
	}
	return exportPath, nil
}

func (m Model) exportSearchCmd(results []*model.Note) tea.Cmd {
	dir := searchExportDir()
	notes := make([]*model.Note, len(results))
	for index, note := range results {
		noteCopy := *note
		notes[index] = &noteCopy
	}
	return func() tea.Msg {
		exportPath, err := writeSearchCSV(dir, notes, time.Now())
		return searchExportedMsg{path: exportPath, count: len(notes), err: err}
	}
}

func displayPath(path string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(path, home+string(filepath.Separator)) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

func (m Model) handleSearchExported(msg searchExportedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.showError("EXPORT ERROR", msg.err)
		return m, nil
	}
	_ = copyExportPath(msg.path)
	m.searchNotice = fmt.Sprintf("Exported %d notes to %s (path copied)", msg.count, displayPath(msg.path))
	return m, nil
}
