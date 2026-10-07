package tui

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"app/pkg/brag"
	"app/pkg/model"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

type startedBrag struct {
	id         string
	regenerate bool
}

func bragTestModel(t *testing.T, now time.Time) (Model, *[]startedBrag) {
	t.Helper()
	m := syncTestModel(t)
	originalClock, originalStart := bragClock, startBragRun
	bragClock = func() time.Time { return now }
	var started []startedBrag
	startBragRun = func(root string, period brag.Period, regenerate bool) error {
		started = append(started, startedBrag{period.ID(), regenerate})
		return nil
	}
	t.Cleanup(func() { bragClock, startBragRun = originalClock, originalStart })
	m.notes = []*model.Note{{Summary: "first", Created: time.Date(2025, 11, 20, 9, 0, 0, 0, time.Local), Source: model.SourceManual}}
	return m, &started
}

func wednesday() time.Time { return time.Date(2026, 10, 7, 11, 0, 0, 0, time.Local) }

func rowTexts(m Model) []string {
	var texts []string
	for _, row := range m.bragRows() {
		texts = append(texts, row.title())
	}
	return texts
}

func selectBragRow(t *testing.T, m Model, title string) Model {
	t.Helper()
	for index, row := range m.bragRows() {
		if row.title() == title {
			m.bragSelected = index
			return m
		}
	}
	t.Fatalf("row %q not found in %v", title, rowTexts(m))
	return m
}

func TestBragKeyOpensModalWithOnlyBragKeys(t *testing.T) {
	m, _ := bragTestModel(t, wednesday())
	m = press(t, m, runes("b"))
	if m.mode != ViewBragList {
		t.Fatalf("mode = %v", m.mode)
	}
	if m = press(t, m, runes("a")); m.mode != ViewBragList {
		t.Errorf("dashboard key leaked into brag modal: mode %v", m.mode)
	}
	if !strings.Contains(stripANSI(m.View()), "BRAG") {
		t.Errorf("modal not rendered:\n%s", stripANSI(m.View()))
	}
	if m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc}); m.mode != ViewDashboard {
		t.Errorf("esc should close the brag modal, mode %v", m.mode)
	}
}

func TestBragRowsGroupYearMonthWeeks(t *testing.T) {
	m, _ := bragTestModel(t, wednesday())
	m = press(t, m, runes("b"))
	rows := rowTexts(m)
	want := []string{"2026", "2026 performance review", "October 2026", "Week 41 · 05 Oct – 11 Oct", "Week 40 · 28 Sep – 04 Oct", "September 2026", "Week 39 · 21 Sep – 27 Sep"}
	for index, title := range want {
		if index >= len(rows) || rows[index] != title {
			t.Fatalf("rows[%d] = %q want %q\nall: %v", index, rows[index], title, rows)
		}
	}
	if !contains(rows, "January 2026") || !contains(rows, "Week 01 · 29 Dec – 04 Jan") {
		t.Errorf("2026-W01 should sit under January 2026: %v", rows)
	}
	if rows[len(rows)-1] != "2025" || contains(rows, "November 2025") {
		t.Fatalf("2025 should be collapsed at the end: %v", rows)
	}
	m = selectBragRow(t, m, "2025")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	rows = rowTexts(m)
	if !contains(rows, "2025 performance review") || !contains(rows, "November 2025") || !contains(rows, "Week 47 · 17 Nov – 23 Nov") || contains(rows, "Week 46 · 10 Nov – 16 Nov") {
		t.Errorf("expanded 2025 rows wrong: %v", rows)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if contains(rowTexts(m), "November 2025") {
		t.Errorf("enter on an open year should collapse it")
	}
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func TestBragEntryLabels(t *testing.T) {
	m, _ := bragTestModel(t, wednesday())
	root := m.cfg.BragDir()
	week40 := brag.WeekOf(time.Date(2026, 9, 30, 12, 0, 0, 0, time.Local))
	if err := (&brag.Brag{Period: week40, Facts: "- x"}).Save(root); err != nil {
		t.Fatal(err)
	}
	week39 := week40.Previous()
	if err := os.MkdirAll(brag.StateDir(root, week39.ID()), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(brag.StateDir(root, week39.ID())+"/brag.pid", []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		t.Fatal(err)
	}
	labels := map[string]string{}
	for _, row := range m.bragRows() {
		labels[row.title()] = m.bragStateLabel(row)
	}
	for title, want := range map[string]string{
		"Week 40 · 28 Sep – 04 Oct": "View your brag",
		"Week 39 · 21 Sep – 27 Sep": "Bragging...",
		"Week 38 · 14 Sep – 20 Sep": "Brag about this week?",
		"September 2026":            "Brag about this month?",
		"2026 performance review":   "Brag about this year?",
	} {
		if labels[title] != want {
			t.Errorf("%s label = %q want %q", title, labels[title], want)
		}
	}
}

func TestEnterOnMissingWeekConfirmsAndStartsJob(t *testing.T) {
	m, started := bragTestModel(t, wednesday())
	m = press(t, m, runes("b"))
	m = selectBragRow(t, m, "Week 40 · 28 Sep – 04 Oct")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != ViewBragConfirm || len(*started) != 0 {
		t.Fatalf("mode = %v started = %v", m.mode, *started)
	}
	m = press(t, m, runes("y"))
	if len(*started) != 1 || (*started)[0] != (startedBrag{"2026-W40", false}) || m.mode != ViewBragList {
		t.Errorf("started = %v mode = %v", *started, m.mode)
	}
}

func TestUnfinishedPeriodsAreBlocked(t *testing.T) {
	m, started := bragTestModel(t, wednesday())
	m = press(t, m, runes("b"))
	for _, title := range []string{"Week 41 · 05 Oct – 11 Oct", "October 2026"} {
		m = selectBragRow(t, m, title)
		m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
		if m.mode != ViewBragList || len(*started) != 0 || !strings.Contains(m.bragNotice, "isn't over") {
			t.Errorf("%s: mode=%v started=%v notice=%q", title, m.mode, *started, m.bragNotice)
		}
	}
}

func TestExistingBragOpensViewAndEnterEdits(t *testing.T) {
	m, started := bragTestModel(t, wednesday())
	root := m.cfg.BragDir()
	week40 := brag.WeekOf(time.Date(2026, 9, 30, 12, 0, 0, 0, time.Local))
	created := time.Date(2026, 10, 5, 9, 0, 0, 0, time.Local)
	if err := (&brag.Brag{Period: week40, Created: created, Facts: "- collected", Summary: "- Did things"}).Save(root); err != nil {
		t.Fatal(err)
	}
	m = press(t, m, runes("b"))
	m = selectBragRow(t, m, "Week 40 · 28 Sep – 04 Oct")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != ViewBragView || !strings.Contains(stripANSI(m.View()), "Did things") {
		t.Fatalf("mode = %v view:\n%s", m.mode, stripANSI(m.View()))
	}
	if m = press(t, m, runes("e")); m.mode != ViewBragView {
		t.Errorf("e should not edit, mode %v", m.mode)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != ViewBragEdit || !strings.Contains(m.editor.Value(), "## Facts") {
		t.Fatalf("mode = %v editor = %q", m.mode, m.editor.Value())
	}
	m.editor.SetValue("## Facts\n\n- collected\n- my manual point\n\n## Summary\n\n- Did things")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlO})
	saved, err := brag.Load(root, week40)
	if err != nil || !strings.Contains(saved.Facts, "- my manual point") || !saved.Created.Equal(created) || m.mode != ViewBragView {
		t.Fatalf("saved = %+v err = %v mode = %v", saved, err, m.mode)
	}
	m = press(t, m, runes("b"))
	if m.mode != ViewBragConfirm {
		t.Fatalf("b in view should confirm, mode %v", m.mode)
	}
	m = press(t, m, runes("y"))
	if len(*started) != 1 || (*started)[0] != (startedBrag{"2026-W40", true}) {
		t.Errorf("started = %v", *started)
	}
	if m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc}); m.mode != ViewBragList {
		t.Errorf("esc from view should return to list, mode %v", m.mode)
	}
}

func TestBragEditRejectsMissingFacts(t *testing.T) {
	m, _ := bragTestModel(t, wednesday())
	week40 := brag.WeekOf(time.Date(2026, 9, 30, 12, 0, 0, 0, time.Local))
	if err := (&brag.Brag{Period: week40, Facts: "- collected"}).Save(m.cfg.BragDir()); err != nil {
		t.Fatal(err)
	}
	m = press(t, m, runes("b"))
	m = selectBragRow(t, m, "Week 40 · 28 Sep – 04 Oct")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m.editor.SetValue("no headings")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlO})
	if m.mode != ViewBragEdit || !strings.Contains(m.bragNotice, "Facts") {
		t.Errorf("mode = %v notice = %q", m.mode, m.bragNotice)
	}
}

func TestBragRunsShownUnderJobs(t *testing.T) {
	m, _ := bragTestModel(t, wednesday())
	root := m.cfg.BragDir()
	if err := os.MkdirAll(brag.StateDir(root, "2026-W40"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(brag.StateDir(root, "2026-W40")+"/brag.pid", []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		t.Fatal(err)
	}
	m.refreshBragRuns()
	found := false
	for _, item := range m.allNavItems() {
		if item.Kind == KindBragRun && item.BragRun != nil && item.BragRun.Meta.ID == "2026-W40" {
			found = true
		}
	}
	if !found {
		t.Fatalf("brag run missing from nav items")
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "brag 2026-W40") || !strings.Contains(view, "bragging...") {
		t.Errorf("jobs section missing brag run:\n%s", view)
	}
}

func writeBragState(t *testing.T, m Model, id, name, content string) {
	t.Helper()
	dir := brag.StateDir(m.cfg.BragDir(), id)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/"+name, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestUnbraggedWeekNoticeShowsAllWeekUntilTheBragIsSaved(t *testing.T) {
	monday := time.Date(2026, 10, 5, 9, 0, 0, 0, time.Local)
	for offset := 0; offset < 7; offset++ {
		day, _ := bragTestModel(t, monday.AddDate(0, 0, offset))
		if !strings.Contains(stripANSI(day.View()), "Last week (W40) isn't bragged") {
			t.Errorf("notice missing on %s", monday.AddDate(0, 0, offset).Format("Monday"))
		}
	}
	m, _ := bragTestModel(t, monday.AddDate(0, 0, 2))
	lastWeek := brag.WeekOf(monday).Previous()
	writeBragState(t, m, lastWeek.ID(), "brag.pid", strconv.Itoa(os.Getpid()))
	m.refreshBragRuns()
	if !strings.Contains(stripANSI(m.View()), "isn't bragged") {
		t.Errorf("notice should stay while bragging runs")
	}
	if err := os.Remove(brag.StateDir(m.cfg.BragDir(), lastWeek.ID()) + "/brag.pid"); err != nil {
		t.Fatal(err)
	}
	writeBragState(t, m, lastWeek.ID(), "brag.exit", "1")
	m.refreshBragRuns()
	if !strings.Contains(stripANSI(m.View()), "isn't bragged") {
		t.Errorf("notice should stay after a failed run")
	}
	if err := (&brag.Brag{Period: lastWeek, Facts: "- x"}).Save(m.cfg.BragDir()); err != nil {
		t.Fatal(err)
	}
	writeBragState(t, m, lastWeek.ID(), "brag.pid", strconv.Itoa(os.Getpid()))
	m.refreshBragRuns()
	if strings.Contains(stripANSI(m.View()), "isn't bragged") {
		t.Errorf("notice should clear once the brag is saved, even while regenerating")
	}
}

func TestHelpModalReplacesDashboardFooter(t *testing.T) {
	m, _ := bragTestModel(t, wednesday())
	if strings.Contains(stripANSI(m.View()), "run jobs") {
		t.Errorf("dashboard footer still lists keys")
	}
	m = press(t, m, runes("?"))
	view := stripANSI(m.View())
	if m.mode != ViewHelp || !strings.Contains(view, "SHORTCUTS") {
		t.Fatalf("mode = %v view:\n%s", m.mode, view)
	}
	for _, label := range []string{"brag", "refresh commits", "run jobs", "sort field", "sort order"} {
		if !strings.Contains(strings.ToLower(view), label) {
			t.Errorf("help missing %q:\n%s", label, view)
		}
	}
	if m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc}); m.mode != ViewDashboard {
		t.Errorf("esc should close help")
	}
}

func TestSelectedBragRowHighlightsOnlyTitleAndStatus(t *testing.T) {
	originalProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(originalProfile) })
	m, _ := bragTestModel(t, wednesday())
	m = press(t, m, runes("b"))
	m = selectBragRow(t, m, "Week 40 · 28 Sep – 04 Oct")
	row, _ := m.selectedBragRow()
	line := m.renderBragRow(row, true, 90)
	if !strings.HasPrefix(line, "     \x1b[") {
		t.Errorf("indent should stay unstyled: %q", line)
	}
	if !strings.Contains(line, selectedTitle("Week 40 · 28 Sep – 04 Oct")+" ") {
		t.Errorf("title should be highlighted and followed by a plain gap: %q", line)
	}
	if !strings.HasSuffix(line, underlined(m.renderBragStatus(row, false))) {
		t.Errorf("status should keep its colour and be underlined: %q", line)
	}
}

func TestMondayNoticeSitsRightOfHeaderDate(t *testing.T) {
	monday := time.Date(2026, 10, 5, 9, 0, 0, 0, time.Local)
	m, _ := bragTestModel(t, monday)
	headerTop := plainLines(m.renderHeader())[2]
	dateAt := strings.Index(headerTop, "— ")
	noticeAt := strings.Index(headerTop, "Last week (W40) isn't bragged — press b")
	if dateAt < 0 || noticeAt <= dateAt || !strings.HasSuffix(strings.TrimRight(headerTop, " │"), "press b") {
		t.Errorf("notice should be right-aligned after the date: %q", headerTop)
	}
	if strings.Contains(stripANSI(m.renderFooter()), "isn't bragged") {
		t.Errorf("footer should no longer show the notice")
	}
}

func TestUnbraggedWeekNoticeDoesNotReadDiskPerRedraw(t *testing.T) {
	monday := time.Date(2026, 10, 5, 9, 0, 0, 0, time.Local)
	m, _ := bragTestModel(t, monday)
	m.View()
	lastWeek := brag.WeekOf(monday).Previous()
	if err := (&brag.Brag{Period: lastWeek, Facts: "- x"}).Save(m.cfg.BragDir()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stripANSI(m.View()), "isn't bragged") {
		t.Error("a redraw should use the cached brag check")
	}
	m.refreshBragRuns()
	if strings.Contains(stripANSI(m.View()), "isn't bragged") {
		t.Error("refreshing brag runs should re-check the saved brag")
	}
}

func TestUnchangedBragRunsKeepTheirMemos(t *testing.T) {
	var m Model
	runs := []brag.Run{{Meta: brag.RunMeta{ID: "week-1"}, Status: brag.RunRunning, StartedAt: time.Now()}}
	m.applyBragRuns(runs)
	m.bragStates["week-1"] = bragRowState{label: "cached"}
	m.applyBragRuns(append([]brag.Run(nil), runs...))
	if m.bragStates["week-1"].label != "cached" {
		t.Errorf("unchanged runs should keep the row memo")
	}
	m.applyBragRuns([]brag.Run{{Meta: brag.RunMeta{ID: "week-1"}, Status: brag.RunDone, StartedAt: runs[0].StartedAt}})
	if _, kept := m.bragStates["week-1"]; kept {
		t.Errorf("changed runs should reset the row memo")
	}
}
