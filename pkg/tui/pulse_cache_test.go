package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestPulseTicksReuseTheDashboardButStillAnimateTheHeader(t *testing.T) {
	originalProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(originalProfile) })
	m := reviewTestModel(t)
	m.mode = ViewDashboard
	m.width, m.height = 160, 50
	markRunning(t, m, m.git.ghPendingPRs[0].PR.Ref)
	writeLivePID(t, filepath.Join(getLogsDir(), "janitor.pid"))
	m = update(m, reviewPollTickMsg{snapshot: m.loadReviewPollSnapshot()})
	m.git.loadingGit = true
	m.postMessage(messageSourceGit, messageProgress, "syncing")
	builds := countDashboardBuilds(t)
	frames := map[string]bool{}
	for tick := 0; tick < 8; tick++ {
		m = update(m, syncPulseTickMsg{})
		view := m.View()
		if !strings.Contains(view, "reviewing...") {
			t.Fatalf("running review missing from the dashboard")
		}
		frames[view] = true
	}
	if *builds > 1 {
		t.Errorf("dashboard content built %d times for 8 pulse ticks, want at most 1", *builds)
	}
	if len(frames) < 2 {
		t.Errorf("the header sync spinner should still animate between ticks")
	}
}

func TestRunningLabelsHaveNoPulseDot(t *testing.T) {
	m := reviewTestModel(t)
	for name, label := range map[string]string{
		"review":  m.renderReviewRunningIndicator(),
		"pulse":   m.renderPulseIndicator("bragging..."),
		"dry run": m.renderDryRunIndicator(),
		"job":     m.renderJobRunningIndicator(),
	} {
		if strings.Contains(label, "\x1b[8;9m") || strings.Contains(stripANSI(label), "●") {
			t.Errorf("%s label still has a pulse dot: %q", name, label)
		}
	}
}

func TestSelectedReviewingRowStaysUnderlinedThroughTheLabel(t *testing.T) {
	originalProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(originalProfile) })
	m := reviewTestModel(t)
	markRunning(t, m, m.git.ghPendingPRs[0].PR.Ref)
	m = update(m, reviewPollTickMsg{snapshot: m.loadReviewPollSnapshot()})
	m.mode = ViewDashboard
	m.width, m.height = 160, 50
	selectNavItem(t, &m, "pr:"+m.git.ghPendingPRs[0].PR.Ref.URL)
	var row string
	for _, line := range strings.Split(m.View(), "\n") {
		if strings.Contains(line, "reviewing...") {
			row = line
		}
	}
	labelAt := strings.Index(row, "reviewing...")
	if labelAt < 0 {
		t.Fatalf("row lacks the reviewing label: %q", row)
	}
	beforeLabel := row[:labelAt]
	if strings.LastIndex(beforeLabel, underlineOn) < strings.LastIndex(beforeLabel, styleReset) || strings.LastIndex(beforeLabel, underlineOff) > strings.LastIndex(beforeLabel, underlineOn) {
		t.Errorf("underline is off when the reviewing label starts: %q", row)
	}
}
