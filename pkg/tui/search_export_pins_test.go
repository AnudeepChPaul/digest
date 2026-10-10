package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/model"
	"github.com/charmbracelet/lipgloss"
)

func TestSearchWithNoMatchesSaysSo(t *testing.T) {
	m := typeQuery(t, searchTestModel(t), "zzzz")
	if view := stripANSI(m.View()); !strings.Contains(view, "(no matching notes)") || !strings.Contains(view, "0 results") {
		t.Fatalf("view:\n%s", view)
	}
	m.showSearchPreviewAt(0)
	if m.searchPreviewNote() != nil || m.mode != ViewSearch || m.searchPreviewing {
		t.Fatal("no results means nothing to preview")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "(no matching notes)") {
		t.Fatalf("the preview should fall back to the search list:\n%s", view)
	}
}

func TestSearchByDateHighlightsTheDateColumn(t *testing.T) {
	m := typeQuery(t, searchTestModel(t), "date:1d")
	results := m.searchResults()
	if len(results) == 0 {
		t.Fatal("expected results for date:1d")
	}
	query := parseSearchQuery(m.searchInput.Value())
	row := m.renderSearchRow(results[0], false, query, 10, 80)
	if !strings.Contains(row, searchHighlightStyle.Render(searchDateLabel(results[0].Updated, time.Now()))) {
		t.Fatalf("date should be highlighted: %q", row)
	}
}

func TestSearchRowsShowUnmatchedTagsPlainly(t *testing.T) {
	m := searchTestModel(t)
	note := &model.Note{Summary: "Review", Source: model.SourcePRReview, Subject: "console", Status: model.StatusActive, Updated: time.Now()}
	row := m.renderSearchRow(note, false, parseSearchQuery("tag:console"), 10, 80)
	if !strings.Contains(row, tagStyle.Render("#pr-review")) || !strings.Contains(row, searchHighlightStyle.Render("#console")) {
		t.Fatalf("row = %q", row)
	}
}

func TestSearchHelpersWithoutAMemoOrDate(t *testing.T) {
	m := typeQuery(t, searchTestModel(t), "flaky")
	m.searchCache = nil
	if len(m.searchResults()) == 0 {
		t.Fatal("search without a memo should still match")
	}
	if searchDateLabel(time.Time{}, time.Now()) != "" {
		t.Fatal("a zero time has no label")
	}
	if marked := matchedRunes("abc", []string{"", "b"}); marked[0] || !marked[1] {
		t.Fatalf("marked = %v", marked)
	}
}

func TestExportDefaultsToDownloads(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	if dir, err := searchExportDir(); err != nil || dir != filepath.Join(home, "Downloads") {
		t.Fatalf("dir %q err %v", dir, err)
	}
	if got := displayPath(filepath.Join(home, "Downloads", "x.csv")); got != "~/Downloads/x.csv" {
		t.Fatalf("display = %q", got)
	}
	if got := displayPath("/elsewhere/x.csv"); got != "/elsewhere/x.csv" {
		t.Fatalf("display = %q", got)
	}
}

func TestExportFailuresAreReported(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := writeSearchCSV(filepath.Join(blocker, "dir"), nil, time.Now()); err == nil {
		t.Fatal("a directory under a file should fail")
	}
	readOnly := t.TempDir()
	if err := os.Chmod(readOnly, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(readOnly, 0o700) })
	if _, err := writeSearchCSV(readOnly, nil, time.Now()); err == nil {
		t.Fatal("an unwritable directory should fail")
	}
	previous := searchExportDir
	searchExportDir = func() (string, error) { return "", errors.New("no home") }
	t.Cleanup(func() { searchExportDir = previous })
	exported, ok := searchTestModel(t).exportSearchCmd(nil)().(searchExportedMsg)
	if !ok || exported.err == nil || exported.err.Error() != "no home" {
		t.Fatalf("msg = %#v", exported)
	}
}

func TestVisibleRowsAlwaysShowAtLeastOne(t *testing.T) {
	if first, last := visibleGitRows(3, 10, 0); first != 3 || last != 4 {
		t.Fatalf("rows = %d..%d", first, last)
	}
}

func TestPopupLinesWiderThanTheTerminalAreClipped(t *testing.T) {
	fitted := fitPopup("short\n"+strings.Repeat("w", 50), 20, 10)
	for _, line := range strings.Split(fitted, "\n") {
		if lipgloss.Width(line) > 20 {
			t.Fatalf("line wider than the terminal: %q", line)
		}
	}
}

func TestNoHintPillOnRepoRowsOrWithoutRows(t *testing.T) {
	m := gitStripTestModel(t)
	m.cfg.ShowKeyHints = true
	selectNavKind(t, &m, KindGitRepo)
	if m.keyHintPill() != "" {
		t.Fatal("repo rows get no hint pill")
	}
	m.selected = 999
	if m.selectedRowHints() != nil {
		t.Fatal("no row has no hints")
	}
}

func TestHintPillSitsAboveARowAtTheBottom(t *testing.T) {
	m := prRowModel(t, true)
	pill := m.keyHintPill()
	frame := m.currentDashboardFrame()
	rowLine := lipgloss.Height(frame.header) + frame.selectedLine - m.visibleScrollOffset(frame)
	m.height = rowLine + 1
	lines := strings.Split(stripANSI(m.overlayUnderSelectedRow(pill, 0)), "\n")
	pillTop := max(rowLine-lipgloss.Height(pill), 0)
	if pillTop >= len(lines) || !strings.Contains(lines[pillTop+1], "open") {
		t.Fatalf("pill should sit above the row at line %d:\n%s", pillTop, strings.Join(lines, "\n"))
	}
}
