package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestPulseTicksReuseTheDashboardButStillAnimate(t *testing.T) {
	originalProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(originalProfile) })
	m := reviewTestModel(t)
	m.mode = ViewDashboard
	m.width, m.height = 160, 50
	markRunning(t, m, m.ghPendingPRs[0].PR.Ref)
	writeLivePID(t, filepath.Join(getLogsDir(), "janitor.pid"))
	m = update(m, reviewPollTickMsg{snapshot: m.loadReviewPollSnapshot()})
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
		t.Errorf("the pulse dot should still change colour between ticks")
	}
}
