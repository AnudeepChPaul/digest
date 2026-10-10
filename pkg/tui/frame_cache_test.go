package tui

import (
	"strings"
	"testing"
	"unicode/utf8"
	"unsafe"

	"github.com/achandrapaul/digest/pkg/model"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func countDashboardBuilds(t *testing.T) *int {
	t.Helper()
	builds := 0
	original := dashboardContentBuilder
	dashboardContentBuilder = func(m Model) (string, int) {
		builds++
		return original(m)
	}
	t.Cleanup(func() { dashboardContentBuilder = original })
	return &builds
}

func freshView(m Model) string {
	m.frames = newDashboardFrameCache()
	return m.View()
}

func TestAnimationTicksReuseTheDashboardContent(t *testing.T) {
	m := gitStripTestModel(t)
	m.git.loadingGit, m.syncPulseRunning = true, true
	builds := countDashboardBuilds(t)
	for range 2 * pulsePhaseCount {
		m = update(m, syncPulseTickMsg{})
		if m.View() != freshView(m) {
			t.Fatalf("cached frame differs from a fresh build at pulse frame %d", m.syncPulseFrame)
		}
	}
	*builds = 0
	for range pulsePhaseCount {
		m = update(m, syncPulseTickMsg{})
		m.View()
	}
	if *builds != 0 {
		t.Errorf("a repeated pulse cycle rebuilt the dashboard %d times", *builds)
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyDown})
	m.View()
	if *builds == 0 {
		t.Error("a selection change should rebuild the dashboard")
	}
}

func TestDataMessagesInvalidateTheCachedFrame(t *testing.T) {
	m := gitStripTestModel(t)
	m.View()
	m = update(m, loadNotesMsg{notes: m.notes[:0]})
	if m.View() != freshView(m) {
		t.Error("a data message left a stale frame")
	}
}

func TestPreviewNavigationSkipsTheDashboardBuild(t *testing.T) {
	m := myPRStripModel(t)
	m.selected = 3
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	builds := countDashboardBuilds(t)
	m = press(t, m, runes("n"))
	m.View()
	if *builds != 0 {
		t.Errorf("preview navigation built the hidden dashboard %d times", *builds)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.View(); *builds == 0 {
		t.Error("returning to the dashboard should build it")
	}
}

func TestModelStaysSmallToCopy(t *testing.T) {
	if size := unsafe.Sizeof(Model{}); size > 8*1024 {
		t.Errorf("Model is %d bytes and is copied on every update", size)
	}
}

func TestCachedNoteRowsFollowNoteChanges(t *testing.T) {
	m := gitStripTestModel(t)
	note := m.notes[0]
	before := m.renderRow(note, false, 80)
	if m.renderRow(note, false, 80) != before {
		t.Fatal("the same note should render the same row")
	}
	note.Summary = "renamed in place"
	if row := stripANSI(m.renderRow(note, false, 80)); !strings.Contains(row, "renamed in place") {
		t.Errorf("row kept the old summary: %q", row)
	}
	m.currentDate = m.currentDate.AddDate(0, 0, 1)
	if row := stripANSI(m.renderRow(note, true, 80)); !strings.Contains(row, "2d ago") {
		t.Errorf("row kept the old age after the day changed: %q", row)
	}
	if m.renderRow(note, true, 80) == m.renderRow(note, false, 80) {
		t.Error("selection should change the row")
	}
}

func TestFramedPopupIsReusedWhileItsContentIsUnchanged(t *testing.T) {
	m := gitStripTestModel(t)
	direct := lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, fitPopup(modalStyle.Width(60).Render("hello"), m.width, m.height))
	if got := m.framedPopup("hello", 60); got != direct {
		t.Fatal("framed popup differs from the direct render")
	}
	if m.popupMemo.content != "hello" || m.framedPopup("hello", 60) != direct {
		t.Error("the framed popup should be memoised")
	}
	if strings.Contains(m.framedPopup("changed", 60), "hello") {
		t.Error("new content should be framed again")
	}
}

func TestLongMultibyteLabelsTruncateOnCharacterBoundaries(t *testing.T) {
	row := renderJobStyleRow("◆", strings.Repeat("é", 200), "#job", false, 60)
	if !utf8.ValidString(row) || !strings.Contains(row, "…") {
		t.Errorf("job row split a character or did not truncate: %q", row)
	}
}

func TestNoteWithoutCreationDateHasNoMadeUpAge(t *testing.T) {
	m := gitStripTestModel(t)
	age, _ := m.renderNoteTagCells(&model.Note{Summary: "undated"})
	if stripANSI(age) != "" {
		t.Errorf("age = %q", stripANSI(age))
	}
}
