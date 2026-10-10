package tui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/model"

	"github.com/charmbracelet/lipgloss"
)

func TestAutoSyncTickStartsASyncAndSchedulesTheNext(t *testing.T) {
	m := syncTestModel(t)
	m.cfg.GitAutoSyncInterval = 600
	m.git.loadingGit = false
	generation := m.git.fetchGeneration
	next, cmd := m.Update(autoSyncTickMsg(time.Now()))
	m = next.(Model)
	t.Cleanup(m.cancelGitSync)
	if cmd == nil || !m.git.loadingGit || m.git.fetchGeneration != generation+1 || !syncRunning(m) {
		t.Errorf("cmd=%v loading=%v generation=%d->%d", cmd != nil, m.git.loadingGit, generation, m.git.fetchGeneration)
	}
}

func TestAutoSyncTickIsIgnoredWhenTurnedOff(t *testing.T) {
	m := syncTestModel(t)
	m.cfg.GitAutoSyncInterval = 0
	generation := m.git.fetchGeneration
	next, cmd := m.Update(autoSyncTickMsg(time.Now()))
	if cmd != nil || next.(Model).git.fetchGeneration != generation {
		t.Errorf("cmd=%v generation=%d->%d", cmd != nil, generation, next.(Model).git.fetchGeneration)
	}
}

func TestNarrowTerminalSaysItIsTooSmall(t *testing.T) {
	m := gitStripTestModel(t)
	for _, width := range []int{0, 1, 20, 39} {
		m.width = width
		if view := m.View(); view != "Terminal window is too small." {
			t.Errorf("width %d: view = %q", width, view)
		}
	}
}

func TestTheHeadersWidthIsTheMinimumAndRendersWithinIt(t *testing.T) {
	m := myPRStripModel(t)
	m.applyGitPending(gitPendingMsg{generation: m.git.fetchGeneration, pending: []GitPRItem{pendingItem(1)}})
	m.notes = append(m.notes, &model.Note{ID: "long", Summary: strings.Repeat("a very long summary ", 6), Status: model.StatusActive, Source: model.SourceManual, Created: m.currentDate, Updated: m.currentDate})
	minimumWidth := max(40, lipgloss.Width(headerMiddleLine(m))+4)
	m.width, m.height = minimumWidth-1, 30
	if view := m.View(); view != "Terminal window is too small." {
		t.Errorf("one column under the header's width should be too small, got %d-wide view", lipgloss.Width(view))
	}
	m.width = minimumWidth
	m.contentVersion++
	view := m.View()
	if strings.Contains(view, "too small") {
		t.Fatalf("%d columns should render the dashboard", minimumWidth)
	}
	lines := strings.Split(view, "\n")
	if len(lines) > m.height {
		t.Errorf("view has %d lines, height is %d", len(lines), m.height)
	}
	for index, line := range lines {
		if width := lipgloss.Width(line); width > m.width {
			t.Errorf("line %d is %d wide: %q", index, width, stripANSI(line))
		}
	}
}

func TestMyPRRowShowsDraftAndFileCount(t *testing.T) {
	m := myPRStripModel(t)
	m.git.myPRs[0].IsDraft = true
	m.git.myPRs[0].ChangedFiles = 1
	m.git.myPRs[1].ChangedFiles = 7
	m.contentVersion++
	body := strings.Join(plainLines(m.renderDashboardBody()), "\n")
	for _, want := range []string{m.git.myPRs[0].HeadRef + " (draft)", "1 file ·", "7 files ·"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, m.git.myPRs[1].HeadRef+" (draft)") {
		t.Errorf("non-draft PR labelled draft")
	}
}

func TestPendingSortHintFollowsTheSortAndScope(t *testing.T) {
	m := syncTestModel(t)
	m.git.loadingGit = false
	hint := func() string { return strings.Join(strings.Fields(stripANSI(m.renderPendingSortHint())), " ") }
	if got, want := hint(), fmt.Sprintf("s Updated w ↓ Desc m %s %s", directReviewIcon, teamReviewIcon); got != want {
		t.Errorf("default hint = %q, want %q", got, want)
	}
	m = press(t, m, runes("s"))
	m = press(t, m, runes("w"))
	m = press(t, m, runes("m"))
	if got, want := hint(), fmt.Sprintf("s Created w ↑ Asc m %s", directReviewIcon); got != want {
		t.Errorf("toggled hint = %q, want %q", got, want)
	}
	m.contentVersion++
	for _, line := range plainLines(m.renderDashboardBody()) {
		if strings.Contains(line, "Pending Git Actions") {
			if !strings.Contains(line, "Created") || !strings.Contains(line, "↑ Asc") {
				t.Errorf("pending header lacks the hint: %q", line)
			}
			return
		}
	}
	t.Error("no Pending Git Actions header")
}

func TestJobIconTurnsToACheckAfterACleanDryRun(t *testing.T) {
	m := selectionTestModel(t)
	m.jobDryRunHasRun = map[string]bool{"janitor": true, "repo sync": true}
	m.jobDryRunExitCodes = map[string]int{"janitor": 0, "repo sync": 2}
	m.contentVersion++
	rowFor := func(name string) string {
		for _, line := range plainLines(m.renderDashboardBody()) {
			if strings.Contains(line, name) && strings.Contains(line, "#job") {
				return line
			}
		}
		t.Fatalf("no job row for %q", name)
		return ""
	}
	if row := rowFor("janitor"); !strings.Contains(row, "✔") || strings.Contains(row, "◆") || !strings.Contains(row, "done") {
		t.Errorf("clean dry run row = %q", row)
	}
	if row := rowFor("repo sync"); !strings.Contains(row, "◆") || strings.Contains(row, "✔") || !strings.Contains(row, "act") {
		t.Errorf("failed dry run row = %q", row)
	}
}

func TestMessageLogListsNewestFirstWithSecondTimestamps(t *testing.T) {
	m := syncTestModel(t)
	m.messages = nil
	stamps := []time.Time{time.Date(2026, 10, 9, 9, 5, 7, 0, time.Local), time.Date(2026, 10, 9, 14, 30, 59, 0, time.Local)}
	for index, stamp := range stamps {
		messageNow = func() time.Time { return stamp }
		m.postMessage(fmt.Sprintf("source%d", index), messageError, fmt.Sprintf("text %d", index))
	}
	t.Cleanup(func() { messageNow = time.Now })
	m, _ = pressKey(t, m, "!")
	lines := m.messageLogLines()
	if m.mode != ViewError || len(lines) != 2 {
		t.Fatalf("mode=%v lines=%q", m.mode, lines)
	}
	if want := "14:30:59  source1: text 1"; lines[0] != want {
		t.Errorf("first line = %q, want %q", lines[0], want)
	}
	if want := "09:05:07  source0: text 0"; lines[1] != want {
		t.Errorf("second line = %q, want %q", lines[1], want)
	}
	timestamp := regexp.MustCompile(`^\d{2}:\d{2}:\d{2}  `)
	for _, line := range lines {
		if !timestamp.MatchString(line) {
			t.Errorf("line lacks HH:MM:SS: %q", line)
		}
	}
}
