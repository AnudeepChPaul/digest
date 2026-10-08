package tui

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/brag"
	"github.com/AnudeepChPaul/digest/pkg/review"
	"github.com/AnudeepChPaul/digest/pkg/tui/textarea"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	bragClock    = time.Now
	startBragRun = brag.StartBackground
)

type bragRow struct {
	year   int
	period brag.Period
}

func (r bragRow) title() string {
	switch period := r.period.(type) {
	case nil:
		return fmt.Sprint(r.year)
	case brag.Week:
		return fmt.Sprintf("Week %02d · %s", period.Number, period.Days())
	default:
		return period.Label()
	}
}

func (m Model) bragRoot() string {
	return m.cfg.BragDir()
}

func (m Model) firstNoteTime() time.Time {
	var first time.Time
	for _, note := range m.notes {
		if !note.Created.IsZero() && (first.IsZero() || note.Created.Before(first)) {
			first = note.Created
		}
	}
	return first
}

func (m Model) bragYearExpanded(year int) bool {
	if expanded, ok := m.bragExpanded[year]; ok {
		return expanded
	}
	return year == bragClock().Year()
}

func (m Model) bragRows() []bragRow {
	now := bragClock()
	first := m.firstNoteTime()
	weeks := brag.WeeksSince(first, now)
	weeksByMonth := map[string][]brag.Week{}
	for _, week := range weeks {
		monthID := brag.MonthOfWeek(week).ID()
		weeksByMonth[monthID] = append(weeksByMonth[monthID], week)
	}
	newestMonth := brag.MonthOfWeek(weeks[0])
	oldestMonth := brag.MonthOfWeek(weeks[len(weeks)-1])
	if !first.IsZero() {
		oldestMonth = brag.MonthOf(first.In(now.Location()))
	}
	var rows []bragRow
	for month := newestMonth; !month.Start.Before(oldestMonth.Start); month = month.Previous() {
		year := month.Start.Year()
		if len(rows) == 0 || rows[len(rows)-1].year != year {
			rows = append(rows, bragRow{year: year})
			if m.bragYearExpanded(year) {
				rows = append(rows, bragRow{year: year, period: brag.YearOf(month.Start)})
			}
		}
		if !m.bragYearExpanded(year) {
			continue
		}
		rows = append(rows, bragRow{year: year, period: month})
		for _, week := range weeksByMonth[month.ID()] {
			rows = append(rows, bragRow{year: year, period: week})
		}
	}
	return rows
}

type bragRowState struct {
	label  string
	failed bool
}

func (m Model) bragRowStateFor(period brag.Period) bragRowState {
	if state, cached := m.bragStates[period.ID()]; cached {
		return state
	}
	state := m.readBragRowState(period)
	if m.bragStates != nil {
		m.bragStates[period.ID()] = state
	}
	return state
}

func (m Model) bragStateLabel(row bragRow) string {
	if row.period == nil {
		return ""
	}
	return m.bragRowStateFor(row.period).label
}

func (m Model) readBragRowState(period brag.Period) bragRowState {
	root := m.bragRoot()
	switch {
	case brag.IsRunning(root, period.ID()):
		return bragRowState{label: "Bragging..."}
	case brag.Exists(root, period):
		return bragRowState{label: "View your brag"}
	}
	failed := brag.Braggable(period, bragClock()) && brag.Status(root, period.ID()) == brag.RunFailed
	return bragRowState{label: fmt.Sprintf("Brag about this %s?", period.Kind()), failed: failed}
}

func (m Model) selectedBragRow() (bragRow, bool) {
	rows := m.bragRows()
	if m.bragSelected < 0 || m.bragSelected >= len(rows) {
		return bragRow{}, false
	}
	return rows[m.bragSelected], true
}

func (m *Model) refreshBragRuns() {
	m.bragStates = nil
	m.applyBragRuns(brag.ListRuns(m.bragRoot()))
}

func (m *Model) applyBragRuns(runs []brag.Run) {
	unchanged := m.bragStates != nil && slices.EqualFunc(m.bragRuns, runs, func(previous, current brag.Run) bool {
		return previous.Meta == current.Meta && previous.Status == current.Status && previous.StartedAt.Equal(current.StartedAt)
	})
	m.bragRuns = runs
	if unchanged {
		return
	}
	m.bragStates = make(map[string]bragRowState)
	m.bragSaved = &bragSavedMemo{}
}

type bragSavedMemo struct {
	weekStart time.Time
	saved     bool
}

func (m Model) weekBragged(week brag.Week) bool {
	if m.bragSaved == nil {
		return brag.Exists(m.bragRoot(), week)
	}
	if !m.bragSaved.weekStart.Equal(week.Start) {
		m.bragSaved.weekStart, m.bragSaved.saved = week.Start, brag.Exists(m.bragRoot(), week)
	}
	return m.bragSaved.saved
}

func (m Model) anyBragRunning() bool {
	for _, run := range m.bragRuns {
		if run.Status == brag.RunRunning {
			return true
		}
	}
	return false
}

func (m Model) openBrag(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = ViewBragList
	m.bragNotice = ""
	m.refreshBragRuns()
	if rows := m.bragRows(); m.bragSelected >= len(rows) {
		m.bragSelected = max(len(rows)-1, 0)
	}
	return m, nil
}

func (m Model) closeBrag(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = ViewDashboard
	m.bragNotice = ""
	return m, nil
}

func (m Model) bragCursorDown(tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.bragSelected < len(m.bragRows())-1 {
		m.bragSelected++
	}
	return m, nil
}

func (m Model) bragCursorUp(tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.bragSelected > 0 {
		m.bragSelected--
	}
	return m, nil
}

func (m Model) bragListEnter(tea.KeyMsg) (tea.Model, tea.Cmd) {
	row, ok := m.selectedBragRow()
	if !ok {
		return m, nil
	}
	m.bragNotice = ""
	if row.period == nil {
		expanded := make(map[int]bool, len(m.bragExpanded)+1)
		for year, value := range m.bragExpanded {
			expanded[year] = value
		}
		expanded[row.year] = !m.bragYearExpanded(row.year)
		m.bragExpanded = expanded
		return m, nil
	}
	root := m.bragRoot()
	switch {
	case brag.IsRunning(root, row.period.ID()):
		m.bragNotice = fmt.Sprintf("Already bragging about %s — see Jobs", row.period.Label())
		return m, nil
	case brag.Exists(root, row.period):
		return m.showBragView(row.period)
	case !brag.Braggable(row.period, bragClock()):
		_, end := row.period.Range()
		m.bragNotice = fmt.Sprintf("%s isn't over yet — brag about it from %s", row.title(), end.Format("Mon 02 Jan"))
		return m, nil
	}
	m.bragPeriod, m.bragRegenerate, m.bragConfirmReturn = row.period, false, ViewBragList
	m.mode = ViewBragConfirm
	return m, nil
}

func (m Model) bragModalSize() (int, int) {
	return modalWidthFor(m.width), max(12, m.height*90/100)
}

func (m *Model) loadBragView() error {
	entry, err := brag.Load(m.bragRoot(), m.bragPeriod)
	if err != nil {
		return err
	}
	m.bragEntry = entry
	modalWidth, modalHeight := m.bragModalSize()
	innerWidth := modalWidth - 6
	content := renderMarkdown(entry.Body(), innerWidth)
	m.previewViewport = viewport.New(innerWidth, max(3, modalHeight-10))
	m.previewViewport.SetContent(content)
	return nil
}

func (m *Model) reloadBragViewIfChanged() {
	if m.bragPeriod == nil || m.bragEntry == nil {
		return
	}
	latest, err := brag.Load(m.bragRoot(), m.bragPeriod)
	if err != nil || (latest.Updated.Equal(m.bragEntry.Updated) && latest.SummarizedAt.Equal(m.bragEntry.SummarizedAt)) {
		return
	}
	if err := m.loadBragView(); err == nil {
		m.bragNotice = "Summary updated"
	}
}

func (m Model) showBragView(period brag.Period) (tea.Model, tea.Cmd) {
	m.bragPeriod = period
	if err := m.loadBragView(); err != nil {
		m.bragNotice = err.Error()
		return m, nil
	}
	m.mode = ViewBragView
	return m, nil
}

func (m Model) closeBragView(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = ViewBragList
	m.bragNotice = ""
	return m, nil
}

func (m Model) editBrag(tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.bragEntry == nil {
		return m, nil
	}
	m.bragNotice = ""
	m.mode = ViewBragEdit
	m.replaceEditorText(m.bragEntry.Body())
	m.editor.Focus()
	startEditorAtTop(m.editor)
	return m, textarea.Blink
}

func (m Model) saveBragEdit(tea.KeyMsg) (tea.Model, tea.Cmd) {
	parsed, err := brag.ParseBody(m.bragPeriod, m.editor.Value())
	if err != nil {
		m.bragNotice = err.Error()
		return m, nil
	}
	now := time.Now()
	parsed.Created, parsed.SummarizedAt, parsed.Updated = m.bragEntry.Created, m.bragEntry.SummarizedAt, now
	if parsed.Created.IsZero() {
		parsed.Created = now
	}
	if err := parsed.Save(m.bragRoot()); err != nil {
		m.bragNotice = err.Error()
		return m, nil
	}
	m.editor.Blur()
	if err := m.loadBragView(); err != nil {
		m.bragNotice = err.Error()
	} else {
		m.bragNotice = "Saved"
	}
	m.mode = ViewBragView
	return m, nil
}

func (m Model) cancelBragEdit(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.editor.Blur()
	m.bragNotice = ""
	m.mode = ViewBragView
	return m, nil
}

func (m Model) copyBragEditor(tea.KeyMsg) (tea.Model, tea.Cmd) {
	_ = copyToClipboard(strings.TrimSpace(m.editor.Value()))
	return m, nil
}

func (m Model) copyBrag(tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.bragEntry != nil {
		_ = copyToClipboard(strings.TrimSpace(m.bragEntry.Body()))
	}
	return m, nil
}

func (m Model) bragAgain(tea.KeyMsg) (tea.Model, tea.Cmd) {
	if brag.IsRunning(m.bragRoot(), m.bragPeriod.ID()) {
		m.bragNotice = "Already bragging — see Jobs"
		return m, nil
	}
	m.bragRegenerate, m.bragConfirmReturn = true, ViewBragView
	m.mode = ViewBragConfirm
	return m, nil
}

func (m Model) confirmBrag(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = m.bragConfirmReturn
	if err := startBragRun(m.bragRoot(), m.bragPeriod, m.bragRegenerate); err != nil {
		if errors.Is(err, brag.ErrBragRunning) {
			m.bragNotice = "Already bragging — see Jobs"
		} else {
			m.bragNotice = err.Error()
		}
		return m, nil
	}
	m.bragNotice = fmt.Sprintf("Bragging about %s in the background", m.bragPeriod.Label())
	m.refreshBragRuns()
	return m, tea.Batch(m.ensureReviewPoll(), m.ensureSyncPulse())
}

func (m Model) cancelBrag(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = m.bragConfirmReturn
	return m, nil
}

func (m Model) unbraggedWeekNotice() string {
	now := bragClock()
	lastWeek := brag.WeekOf(now).Previous()
	first := m.firstNoteTime()
	if first.IsZero() || brag.WeekOf(first.In(now.Location())).Start.After(lastWeek.Start) {
		return ""
	}
	if m.weekBragged(lastWeek) {
		return ""
	}
	return fmt.Sprintf("Last week (W%02d) isn't bragged — press b", lastWeek.Number)
}

var (
	bragYearStyle  = lipgloss.NewStyle().Foreground(colourBlue).Bold(true)
	bragMonthStyle = lipgloss.NewStyle().Foreground(colourMauve).Bold(true)
	bragDoneStyle  = lipgloss.NewStyle().Foreground(colourGreen)
)

func (m Model) renderBragRow(row bragRow, selected bool, width int) string {
	indent, titleStyle := "     ", itemStyle
	switch row.period.(type) {
	case nil:
		marker := "▸"
		if m.bragYearExpanded(row.year) {
			marker = "▾"
		}
		indent, titleStyle = bragYearStyle.Render(marker+" "), bragYearStyle
	case brag.Month, brag.Year:
		indent, titleStyle = "   ", bragMonthStyle
	}
	if selected {
		titleStyle = titleStyle.Bold(true)
	}
	left := indent + underlinedWhen(selected, titleStyle.Render(row.title()))
	return alignRight(left, []string{m.renderBragStatus(row, selected)}, width, false)
}

func (m Model) renderBragStatus(row bragRow, selected bool) string {
	label := m.bragStateLabel(row)
	if label == "" {
		return ""
	}
	return underlinedWhen(selected, m.bragStatusCell(row, label))
}

func (m Model) bragStatusCell(row bragRow, label string) string {
	if label == "Bragging..." {
		return m.renderPulseIndicator(label)
	}
	failed := m.bragRowStateFor(row.period).failed
	switch {
	case label == "View your brag":
		return bragDoneStyle.Render(label)
	case !brag.Braggable(row.period, bragClock()):
		return mutedStyle.Render(label)
	case failed:
		return yellowBadgeStyle.Render(label) + stateStyle(review.StateFailed).Render(" · last run failed")
	default:
		return yellowBadgeStyle.Render(label)
	}
}

func (m Model) renderBragList() string {
	modalWidth, modalHeight := m.bragModalSize()
	innerWidth := modalWidth - 6
	title := modalTitleStyle.Render(" BRAG ")
	footer := renderModalFooter(footerItemsFrom(bragListBindings()), innerWidth)
	below := []string{""}
	if m.bragNotice != "" {
		below = append(below, yellowBadgeStyle.Render(m.bragNotice), "")
	}
	below = append(below, footer)
	fixed := lipgloss.Height(title) + 1 + lipgloss.Height(strings.Join(below, "\n"))
	rowsAvailable := max(1, modalHeight-modalStyle.GetVerticalFrameSize()-fixed)
	rows := m.bragRows()
	first, last := visibleGitRows(m.bragSelected, len(rows), rowsAvailable)
	var lines []string
	for index := first; index < last; index++ {
		lines = append(lines, m.renderBragRow(rows[index], index == m.bragSelected, innerWidth))
	}
	for len(lines) < rowsAvailable {
		lines = append(lines, "")
	}
	content := lipgloss.JoinVertical(lipgloss.Left, append([]string{title, "", strings.Join(lines, "\n")}, below...)...)
	return m.placeBragModal(content, modalWidth)
}

func (m Model) placeBragModal(content string, modalWidth int) string {
	return m.framedPopup(content, modalWidth)
}

func (m Model) renderBragView() string {
	modalWidth, modalHeight := m.bragModalSize()
	innerWidth := modalWidth - 6
	title := modalTitleStyle.Render(fmt.Sprintf(" BRAG: %s ", strings.ToUpper(m.bragPeriod.Label())))
	if m.bragRowStateFor(m.bragPeriod).label == "Bragging..." {
		title += "  " + m.renderPulseIndicator("Bragging...")
	}
	below := []string{""}
	if m.bragNotice != "" {
		below = append(below, yellowBadgeStyle.Render(m.bragNotice), "")
	}
	below = append(below, renderModalFooter(footerItemsFrom(bragViewBindings()), innerWidth))
	fixed := lipgloss.Height(title) + 1 + lipgloss.Height(strings.Join(below, "\n"))
	m.previewViewport.Height = max(3, modalHeight-modalStyle.GetVerticalFrameSize()-fixed)
	content := lipgloss.JoinVertical(lipgloss.Left, append([]string{title, "", m.previewViewport.View()}, below...)...)
	return m.placeBragModal(content, modalWidth)
}

func (m Model) renderBragEdit() string {
	modalWidth, _ := m.bragModalSize()
	innerWidth := modalWidth - 6
	title := modalTitleStyle.Render(fmt.Sprintf(" EDIT BRAG: %s ", strings.ToUpper(m.bragPeriod.Label())))
	parts := []string{title, "", m.editorView(), ""}
	if m.bragNotice != "" {
		parts = append(parts, yellowBadgeStyle.Render(m.bragNotice), "")
	}
	parts = append(parts, renderModalFooter(footerItemsFrom(bragEditBindings()), innerWidth))
	return m.placeBragModal(lipgloss.JoinVertical(lipgloss.Left, parts...), modalWidth)
}

func (m Model) renderBragConfirm(modalWidth int) string {
	prompt := fmt.Sprintf("Brag about %s?\n\nFacts are collected and Claude writes the summary in the background.", m.bragPeriod.Label())
	if m.bragRegenerate {
		prompt = fmt.Sprintf("Brag again about %s?\n\nClaude rewrites only the summary from the saved facts.", m.bragPeriod.Label())
	}
	content := lipgloss.JoinVertical(lipgloss.Left,
		modalTitleStyle.Render(" BRAG "),
		"",
		prompt,
		"",
		renderModalFooter(footerItemsFrom(bragConfirmBindings()), modalWidth-6),
	)
	return m.placeBragModal(content, modalWidth)
}

func bragRunLabel(run brag.Run) string {
	return "brag " + run.Meta.ID
}

func (m Model) renderBragRunRow(run brag.Run, selected bool, width int) string {
	rightBlock := stateStyle(review.StateFailed).Render("failed")
	if run.Status == brag.RunRunning {
		rightBlock = m.renderPulseIndicator("bragging...")
	}
	return renderJobStyleRow(amberDiamond.Render(), bragRunLabel(run), rightBlock, selected, width)
}

func bragRunPreview(root string, run brag.Run) runPreview {
	status := "FAILED"
	if run.Status == brag.RunRunning {
		status = "RUNNING"
	}
	logText := strings.TrimSpace(readFileTail(filepath.Join(brag.StateDir(root, run.Meta.ID), brag.RunLogFile), jobLogTailBytes))
	if logText == "" {
		logText = "(no log output yet)"
	}
	return runPreview{heading: fmt.Sprintf("# Brag: %s (%s)", run.Meta.ID, status), log: logText}
}

var stopBragRun = brag.Stop

func (m Model) stopOrDismissBragRun(run *brag.Run) (tea.Model, tea.Cmd) {
	root := m.bragRoot()
	var err error
	if brag.IsRunning(root, run.Meta.ID) {
		err = stopBragRun(root, run.Meta.ID)
	} else {
		err = brag.Dismiss(root, run.Meta.ID)
	}
	if err != nil {
		m.showError("BRAG ERROR", err)
		return m, nil
	}
	selectedKey, selectedOccurrence := m.selectedNavKey()
	m.refreshBragRuns()
	m.restoreSelection(selectedKey, selectedOccurrence)
	if m.mode == ViewPreview {
		if _, ok := m.selectedNavItem(); ok {
			m.updatePreviewViewport()
		} else {
			m.mode = ViewDashboard
		}
	}
	return m, nil
}

type helpEntry struct {
	key  string
	text string
}

var hiddenDashboardHelp = map[keyAction]helpEntry{
	actionToggleSortField:    {"s", "sort field (pending PRs)"},
	actionToggleSortOrder:    {"w", "sort order (pending PRs)"},
	actionTogglePendingScope: {"m", "me only (pending PRs)"},
	actionDismissSyncErrors:  {"esc", "dismiss messages"},
	actionOpenMessages:       {"!", "all messages"},
	actionOpenActions:        {"@|.", "actions on the selected note"},
	actionOpenSetup:          {",", "setup"},
}

func (m Model) helpEntries() []helpEntry {
	var entries []helpEntry
	for _, binding := range m.dashboardBindings() {
		if entry, ok := hiddenDashboardHelp[binding.action]; ok {
			entries = append(entries, entry)
			continue
		}
		help := binding.binding.Help()
		if help.Key == "" || binding.action == actionOpenHelp {
			continue
		}
		entries = append(entries, helpEntry{help.Key, help.Desc})
	}
	return entries
}

func (m Model) openHelp(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = ViewHelp
	return m, nil
}

func (m Model) closeHelp(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = ViewDashboard
	return m, nil
}

func (m Model) renderHelp(modalWidth int) string {
	entries := m.helpEntries()
	keyWidth := 0
	for _, entry := range entries {
		keyWidth = max(keyWidth, lipgloss.Width(entry.key))
	}
	var lines []string
	for _, entry := range entries {
		lines = append(lines, "  "+keyStyle.Render(entry.key+safeRepeat(" ", keyWidth-lipgloss.Width(entry.key)))+"   "+itemStyle.Render(entry.text))
	}
	content := lipgloss.JoinVertical(lipgloss.Left,
		modalTitleStyle.Render(" SHORTCUTS "),
		"",
		strings.Join(lines, "\n"),
		"",
		renderModalFooter(footerItemsFrom(helpBindings()), modalWidth-6),
	)
	return m.placeBragModal(content, modalWidth)
}
