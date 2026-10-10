package tui

import (
	"encoding/csv"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/model"
	"github.com/achandrapaul/digest/pkg/paths"

	tea "github.com/charmbracelet/bubbletea"
)

func stubExport(t *testing.T) (string, *string) {
	t.Helper()
	exportDir := filepath.Join(t.TempDir(), "Downloads")
	copied := new(string)
	originalDir, originalCopy := searchExportDir, copyExportPath
	searchExportDir = func() (string, error) { return exportDir, nil }
	copyExportPath = func(text string) error { *copied = text; return nil }
	t.Cleanup(func() { searchExportDir, copyExportPath = originalDir, originalCopy })
	return exportDir, copied
}

func runExport(t *testing.T, m Model) Model {
	t.Helper()
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	m = next.(Model)
	if cmd == nil {
		return m
	}
	next, _ = m.Update(cmd())
	return next.(Model)
}

func TestWriteSearchCSV(t *testing.T) {
	dir := t.TempDir()
	updated := time.Date(2026, 10, 2, 9, 14, 0, 0, time.Local)
	notes := []*model.Note{
		{Summary: "Fix, flaky", Body: "line one\nline \"two\"", Source: model.SourcePRReview, Subject: "console", Status: model.StatusDone, Updated: updated, Created: updated.Add(-time.Hour)},
		{Summary: "Plain", Source: model.SourceManual, Status: model.StatusActive},
	}
	exportPath, err := writeSearchCSV(dir, notes, updated)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(exportPath) != "search-2026-10-02-091400.csv" {
		t.Errorf("file = %s", exportPath)
	}
	if info, err := os.Stat(exportPath); err != nil || info.Mode().Perm() != paths.PrivateFileMode {
		t.Errorf("export should be owner-only: %v %v", info, err)
	}
	exportFile, err := os.Open(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	defer exportFile.Close()
	records, err := csv.NewReader(exportFile).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		searchCSVHeader,
		{"2026-10-02 09:14", "2026-10-02 08:14", "done", "pr-review;console", "Fix, flaky", "line one\nline \"two\""},
		{"", "", "active", "manual", "Plain", ""},
	}
	if len(records) != len(want) {
		t.Fatalf("records = %q", records)
	}
	for row := range want {
		if strings.Join(records[row], "|") != strings.Join(want[row], "|") {
			t.Errorf("row %d = %q, want %q", row, records[row], want[row])
		}
	}
}

func TestCtrlEExportsResults(t *testing.T) {
	exportDir, copied := stubExport(t)
	m := typeQuery(t, searchTestModel(t), "flaky")
	m = runExport(t, m)
	entries, _ := os.ReadDir(exportDir)
	if len(entries) != 1 || !regexp.MustCompile(`^search-\d{4}-\d{2}-\d{2}-\d{6}\.csv$`).MatchString(entries[0].Name()) {
		t.Fatalf("entries = %v", entries)
	}
	exportPath := filepath.Join(exportDir, entries[0].Name())
	if *copied != exportPath || !strings.HasPrefix(m.searchNotice, "Exported 2 notes to ") {
		t.Errorf("copied = %q notice = %q", *copied, m.searchNotice)
	}
	content, _ := os.ReadFile(exportPath)
	if strings.Count(string(content), "\n") != 3 || !strings.Contains(string(content), "Fix flaky test") || strings.Contains(string(content), "Archived flaky") {
		t.Errorf("content = %s", content)
	}
	if !strings.Contains(stripANSI(m.View()), "Exported 2 notes") || !strings.Contains(stripANSI(m.View()), "Export") {
		t.Error("notice or footer missing")
	}
	m = press(t, m, runes("x"))
	if m.searchNotice != "" {
		t.Errorf("notice not cleared: %q", m.searchNotice)
	}
}

func TestExportNothing(t *testing.T) {
	exportDir, _ := stubExport(t)
	m := typeQuery(t, searchTestModel(t), "zzznomatch")
	m = runExport(t, m)
	if m.searchNotice != nothingToExportNotice {
		t.Errorf("notice = %q", m.searchNotice)
	}
	if _, err := os.Stat(exportDir); !os.IsNotExist(err) {
		t.Error("export dir created")
	}
}

func TestExportErrorShowsInTheHeader(t *testing.T) {
	m := searchTestModel(t)
	next, _ := m.Update(searchExportedMsg{err: errors.New("disk full")})
	m = next.(Model)
	if m.mode != ViewSearch || !strings.Contains(latestMessageText(m), "disk full") {
		t.Errorf("mode = %d message = %q", m.mode, latestMessageText(m))
	}
}
