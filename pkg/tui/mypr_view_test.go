package tui

import (
	"strings"
	"testing"
	"time"

	"app/pkg/review"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func myPRStripModel(t *testing.T) Model {
	t.Helper()
	m := gitStripTestModel(t)
	m.myPRs = []review.QueuedPR{
		myOpenPR("console", 4, "fix/a", "SUCCESS"),
		myOpenPR("console", 3, "feat/b", "FAILURE"),
		myOpenPR("digest", 9, "main-ui", "PENDING"),
	}
	sortMyPRs(m.myPRs)
	return m
}

func selectedMyPRNumber(m Model) int {
	item := m.allNavItems()[m.selected]
	if item.Kind != KindMyPR || item.MyPR == nil {
		return 0
	}
	return item.MyPR.Ref.Number
}

func TestMyPRBlockSitsBelowDaysAtFullWidth(t *testing.T) {
	m := myPRStripModel(t)
	m.myPRs[0].ChangedFiles = 12
	m.myPRs[0].CreatedAt = time.Now().Add(-49 * time.Hour)
	lines := plainLines(m.renderDashboardBody())
	body := strings.Join(lines, "\n")
	for _, want := range []string{"M Y   P R ( S )", "#4", "fix/a", "12 files", "2d ago", "✓", "#3", "feat/b", "✗", "digest", "#9", "main-ui", "◌"} {
		if !strings.Contains(body, want) {
			t.Errorf("block missing %q:\n%s", want, body)
		}
	}
	sharedLine, titleLine, consoleRow, firstPRRow, todayLine := -1, -1, -1, -1, -1
	for index, line := range lines {
		inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "│"), "│"))
		switch {
		case strings.Contains(line, "alpha") && strings.Contains(line, "gamma"):
			sharedLine = index
		case strings.Contains(line, "M Y   P R ( S )"):
			titleLine = index
		case consoleRow < 0 && inner == "console":
			consoleRow = index
		case firstPRRow < 0 && strings.Contains(line, "#4"):
			firstPRRow = index
			if strings.Contains(inner, "│") {
				t.Errorf("my PR row should not share a column divider: %q", line)
			}
		case strings.Contains(line, "T O D A Y") && strings.Contains(line, "jobs"):
			todayLine = index
		}
	}
	if sharedLine < 0 || titleLine < sharedLine || consoleRow < titleLine || firstPRRow != consoleRow+1 || todayLine < firstPRRow {
		t.Errorf("shared %d title %d console %d first PR %d today %d:\n%s", sharedLine, titleLine, consoleRow, firstPRRow, todayLine, body)
	}
}

func TestEmptyMyPRColumnSaysSo(t *testing.T) {
	m := gitStripTestModel(t)
	m.loadingMyPRs = false
	if body := strings.Join(plainLines(m.renderDashboardBody()), "\n"); !strings.Contains(body, "(no open PRs)") {
		t.Errorf("empty column has no hint:\n%s", body)
	}
}

func TestColumnKeysStayWithinDaysAndJKFlowIntoMyPRs(t *testing.T) {
	m := myPRStripModel(t)
	m.selected = 2
	if m = press(t, m, tea.KeyMsg{Type: tea.KeyRight}); selectedRepoName(m) != "gamma" {
		t.Fatalf("right in today should do nothing, got %q", selectedRepoName(m))
	}
	if m = press(t, m, runes("j")); selectedMyPRNumber(m) != 4 {
		t.Fatalf("j from today should land on the first my PR, got %d", selectedMyPRNumber(m))
	}
	if m = press(t, m, runes("j")); selectedMyPRNumber(m) != 3 {
		t.Errorf("j inside my PRs should move down, got %d", selectedMyPRNumber(m))
	}
	for _, key := range []tea.KeyMsg{runes("l"), runes("h"), {Type: tea.KeyLeft}, {Type: tea.KeyRight}} {
		if m = press(t, m, key); selectedMyPRNumber(m) != 3 {
			t.Errorf("%s in my PRs should do nothing, got %d", key, selectedMyPRNumber(m))
		}
	}
	m.selected = 3
	if m = press(t, m, runes("k")); selectedRepoName(m) != "gamma" {
		t.Errorf("k from the first my PR should land on today, got %q", selectedRepoName(m))
	}
}

func TestEnterOpensMyPRInBrowser(t *testing.T) {
	var opened []string
	originalOpen := openURL
	openURL = func(url string) error {
		opened = append(opened, url)
		return nil
	}
	t.Cleanup(func() { openURL = originalOpen })
	m := myPRStripModel(t)
	m.selected = 3
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(opened) != 1 || opened[0] != m.myPRs[0].Ref.URL || m.mode != ViewDashboard {
		t.Errorf("opened=%v mode=%v", opened, m.mode)
	}
}

func TestTabOpensMyPRDetails(t *testing.T) {
	m := myPRStripModel(t)
	m.myPRs[0].Title = "Fix the picker"
	m.myPRs[0].Body = "Long **description**"
	m.myPRs[0].ReviewDecision = "APPROVED"
	m.myPRs[0].ChangedFiles = 7
	m.myPRs[0].CreatedAt = time.Date(2026, 10, 1, 9, 30, 0, 0, time.Local)
	m.myPRs[0].Reviews = []review.PRReview{{Author: "alice", State: "APPROVED", SubmittedAt: time.Now()}, {Author: "bob", State: "CHANGES_REQUESTED", SubmittedAt: time.Now()}}
	m.selected = 3
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.mode != ViewPreview {
		t.Fatalf("mode = %v", m.mode)
	}
	content := stripANSI(m.previewViewport.View())
	for _, want := range []string{"Fix the picker", "fix/a", "passing", "approved", "@alice", "@bob", "description", "7 files", "2026-10-01"} {
		if !strings.Contains(strings.ToLower(content), strings.ToLower(want)) {
			t.Errorf("details missing %q:\n%s", want, content)
		}
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "MY PR") || !strings.Contains(view, "#my-pr") {
		t.Errorf("modal header missing:\n%s", view)
	}
	if m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc}); m.mode != ViewDashboard {
		t.Errorf("esc should close the details, mode=%v", m.mode)
	}
}

func TestApprovedMyPRShowsHandOkayBeforeCI(t *testing.T) {
	m := myPRStripModel(t)
	m.myPRs[0].ReviewDecision = "APPROVED"
	m.myPRs[1].ReviewDecision = "CHANGES_REQUESTED"
	approvedRow, otherRow := "", ""
	for _, line := range plainLines(m.renderDashboardBody()) {
		switch {
		case strings.Contains(line, "#4"):
			approvedRow = line
		case strings.Contains(line, "#3"):
			otherRow = line
		}
	}
	if !strings.Contains(approvedRow, myPRApprovedGlyph+" ✓") {
		t.Errorf("approved row should show the icon before CI: %q", approvedRow)
	}
	if strings.Contains(otherRow, myPRApprovedGlyph) || lipgloss.Width(otherRow[:strings.Index(otherRow, "✗")]) != lipgloss.Width(approvedRow[:strings.Index(approvedRow, "✓")]) {
		t.Errorf("non-approved row should keep CI aligned without the icon:\n%q\n%q", approvedRow, otherRow)
	}
	m.selected = 3
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if content := stripANSI(m.previewViewport.View()); !strings.Contains(content, myPRApprovedGlyph+" approved") {
		t.Errorf("modal should show the approval icon:\n%s", content)
	}
}
