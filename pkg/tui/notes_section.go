package tui

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/achandrapaul/digest/pkg/automation"
	"github.com/achandrapaul/digest/pkg/model"
	"github.com/achandrapaul/digest/pkg/store"
	"github.com/achandrapaul/digest/pkg/tui/textarea"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type notesSection struct{}

type loadNotesMsg struct {
	notes []*model.Note
	err   error
}

type browsedNotesMsg struct {
	notes []*model.Note
	err   error
}

type closedThisWeekMsg struct {
	count int
	err   error
}

func (m Model) startupNotesCmd() tea.Cmd {
	notes, err := m.notes, m.startupNotesErr
	return func() tea.Msg {
		return loadNotesMsg{notes: notes, err: err}
	}
}

func (m Model) loadNotesCmd() tea.Msg {
	notes, err := m.store.ListDashboard(m.currentDate, m.previousNoteDay())
	return loadNotesMsg{notes: notes, err: err}
}

func (m *Model) refreshArchivedViewport() {
	archivedNotes := m.getArchivedNotes()
	if m.archivedSelected >= len(archivedNotes) && len(archivedNotes) > 0 {
		m.archivedSelected = len(archivedNotes) - 1
	}
	if m.archivedSelected < 0 {
		m.archivedSelected = 0
	}
	modalWidth := modalWidthFor(m.width)
	m.archivedViewport.SetContent(m.renderArchivedContent(modalWidth-6, m.archivedSelected))
}

func startOfDay(moment time.Time) time.Time {
	return time.Date(moment.Year(), moment.Month(), moment.Day(), 0, 0, 0, 0, moment.Location())
}

func (m Model) previousNoteDay() time.Time {
	viewedDayStart := startOfDay(m.currentDate)
	for daysBack := 1; daysBack <= 7; daysBack++ {
		if candidate := viewedDayStart.AddDate(0, 0, -daysBack); m.cfg.IsWorkDay(candidate.Weekday()) {
			return candidate
		}
	}
	return viewedDayStart.AddDate(0, 0, -1)
}

type noteGroups struct {
	previousDay  time.Time
	previousDone []*model.Note
	carried      []*model.Note
	today        []*model.Note
	todayDone    []*model.Note
}

func (m Model) groupNotes() noteGroups {
	groups := noteGroups{previousDay: m.previousNoteDay()}
	for _, n := range m.notes {
		switch n.Status {
		case model.StatusArchived:
		case model.StatusDone:
			if !n.Updated.Before(groups.previousDay) && n.Updated.Before(groups.previousDay.AddDate(0, 0, 1)) {
				groups.previousDone = append(groups.previousDone, n)
			}
			if isSameDay(n.Updated, m.currentDate) {
				groups.todayDone = append(groups.todayDone, n)
			}
		default:
			if isSameDay(n.Created, m.currentDate) {
				groups.today = append(groups.today, n)
			} else if n.Created.Before(m.currentDate) {
				groups.carried = append(groups.carried, n)
			}
		}
	}
	slices.SortStableFunc(groups.previousDone, func(a, b *model.Note) int {
		if bySource := strings.Compare(string(a.Source), string(b.Source)); bySource != 0 {
			return bySource
		}
		return a.Updated.Compare(b.Updated)
	})
	return groups
}

func (m Model) previousDayTitleFor(previousDay time.Time) string {
	if isSameDay(previousDay, time.Now().AddDate(0, 0, -1)) {
		return m.dayTitleText(previousDay, "Y E S T E R D A Y")
	}
	return m.dayTitleText(previousDay, letterSpaced(strings.ToUpper(previousDay.Format("Monday"))))
}

func letterSpaced(word string) string {
	return strings.Join(strings.Split(word, ""), " ")
}

func (m Model) getArchivedNotes() []*model.Note {
	var list []*model.Note
	for _, n := range m.notesForBrowsing() {
		if n.Status == model.StatusArchived {
			list = append(list, n)
		}
	}
	sort.SliceStable(list, func(i, j int) bool {
		return list[i].Updated.After(list[j].Updated)
	})
	return list
}

func (m Model) dayTitleText(date time.Time, spacedLabel string) string {
	if isSameDay(date, m.currentDate) && !isSameDay(m.currentDate, time.Now()) {
		spacedLabel = letterSpaced(strings.ToUpper(date.Format("Monday")))
	}
	return fmt.Sprintf("%s  ·  %s", spacedLabel, strings.ToUpper(date.Format("02 Jan")))
}

func (m Model) renderArchivedContent(width int, selectedIndex int) string {
	archived := m.getArchivedNotes()
	if len(archived) == 0 {
		return mutedStyle.Render("(No archived notes)")
	}

	var listing strings.Builder
	currentGroupDate := ""

	for index, note := range archived {
		dateLabel := note.Updated.Local().Format("Monday 02 Jan 2006")
		if dateLabel != currentGroupDate {
			if currentGroupDate != "" {
				listing.WriteString("\n")
			}
			listing.WriteString(subSectionStyle.Render(dateLabel) + "\n")
			currentGroupDate = dateLabel
		}

		prefix := "  "
		if m.archivedSelectedMap != nil && m.archivedSelectedMap[index] {
			prefix = amberDiamond.Render() + " "
		}

		box := checkDone.Render()
		if note.Status != model.StatusDone {
			box = checkPending.Render()
		}

		age := note.Updated.Local().Format("15:04")
		sourceText := noteSourceText(note)
		summaryWidth := max(width-ansi.StringWidth(prefix)-2-ansi.StringWidth(sourceText)-len(age)-6, 10)
		summary := ansi.Truncate(note.Summary, summaryWidth, "…")
		gap := summaryWidth - ansi.StringWidth(summary)

		selected := index == selectedIndex
		if selected {
			summary = selectedTitle(summary)
		} else {
			summary = itemStyle.Render(summary)
		}
		rightBlock := fmt.Sprintf("%s   %s", dimBlueText.Render(sourceText), mutedStyle.Render(age))
		listing.WriteString(fmt.Sprintf("%s%s %s%s%s\n", prefix, box, summary, safeRepeat(" ", gap), underlinedWhen(selected, rightBlock)))
	}

	return listing.String()
}

const messageSourceStore = "store"

func storeResultUsable(err error) bool {
	var skipped *store.SkippedNotesError
	return err == nil || errors.As(err, &skipped)
}

func (m *Model) reportStoreError(err error) {
	switch {
	case err == nil:
	case storeResultUsable(err):
		m.postMessage(messageSourceStore, messageError, err.Error())
	default:
		m.showError("STORE ERROR", err)
	}
}

func (m Model) browseNotesCmd() tea.Cmd {
	noteStore := m.store
	return func() tea.Msg {
		notes, err := noteStore.List()
		return browsedNotesMsg{notes: notes, err: err}
	}
}

func (m Model) notesForBrowsing() []*model.Note {
	if m.browsedNotes != nil {
		return m.browsedNotes
	}
	return m.notes
}

func (m *Model) refreshBrowsedView() {
	switch m.mode {
	case ViewArchived:
		m.refreshArchivedViewport()
	case ViewSearch:
		m.keepSearchSelectionVisible()
	case ViewPreview:
		if m.previewingSearch() {
			m.updatePreviewViewport()
		}
	case ViewBragList:
		if rows := m.bragRows(); m.bragSelected >= len(rows) {
			m.bragSelected = max(len(rows)-1, 0)
		}
	}
}

func (m Model) browsing() bool {
	return m.mode != ViewDashboard && (m.mode != ViewPreview || m.searchPreviewing)
}

func (m *Model) shareDashboardNotes() {
	if m.browsedNotes == nil {
		return
	}
	byPath := make(map[string]*model.Note, len(m.notes))
	for _, note := range m.notes {
		byPath[note.FilePath] = note
	}
	for index, note := range m.browsedNotes {
		if shared, onDashboard := byPath[note.FilePath]; onDashboard {
			m.browsedNotes[index] = shared
		}
	}
}

func (m Model) belongsOnDashboard(note *model.Note) bool {
	switch note.Status {
	case model.StatusArchived:
		return false
	case model.StatusDone:
		return isSameDay(note.Updated, m.currentDate) || isSameDay(note.Updated, m.previousNoteDay())
	}
	return true
}

func startOfISOWeek(moment time.Time) time.Time {
	return startOfDay(moment).AddDate(0, 0, -((int(moment.Weekday()) + 6) % 7))
}

func (m Model) closedThisWeekCmd() tea.Cmd {
	noteStore := m.store
	dashboardPaths := make(map[string]bool, len(m.notes))
	for _, note := range m.notes {
		dashboardPaths[note.FilePath] = true
	}
	weekStart := startOfISOWeek(time.Now())
	return func() tea.Msg {
		closed, err := noteStore.ListDoneSince(weekStart)
		count := 0
		for _, note := range closed {
			if !dashboardPaths[note.FilePath] {
				count++
			}
		}
		return closedThisWeekMsg{count: count, err: err}
	}
}

func (m Model) renderArchivedModal(modalWidth int) string {
	titleText := modalTitleStyle.Render(" ARCHIVED NOTES ")

	innerHeight := m.height - 10 - footerLineCount(m.archiveFooterItems())
	if innerHeight < 4 {
		innerHeight = 4
	}
	m.archivedViewport.Height = innerHeight

	footerText := renderModalFooter(m.archiveFooterItems(), modalWidth-6)

	popupContent := lipgloss.JoinVertical(
		lipgloss.Left,
		titleText,
		"",
		m.archivedViewport.View(),
		"",
		footerText,
	)

	return m.framedPopup(popupContent, modalWidth)
}

func (m Model) renderEditModal(modalWidth int) string {
	titleText := modalTitleStyle.Render(" ADD / EDIT NOTE ")

	footerText := renderModalFooter(footerItemsFrom(editBindings()), modalWidth-6)

	popupContent := lipgloss.JoinVertical(
		lipgloss.Left,
		titleText,
		"",
		m.editorView(),
		m.editorNoticeLine(),
		footerText,
	)

	return m.framedPopup(popupContent, modalWidth)
}

func (m Model) editorNoticeLine() string {
	if m.editorNotice == "" {
		return ""
	}
	return staleStyle.Render(m.editorNotice) + "\n"
}

func (m Model) openArchive(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.archivedSelected = 0
	m.archivedSelectedMap = make(map[int]bool)
	modalWidth := modalWidthFor(m.width)
	innerWidth := modalWidth - 6
	innerHeight := m.height - 10 - footerLineCount(m.archiveFooterItems())
	if innerHeight < 4 {
		innerHeight = 4
	}

	m.archivedViewport = viewport.New(innerWidth, innerHeight)
	m.archivedViewport.SetContent(m.renderArchivedContent(innerWidth, m.archivedSelected))
	m.mode = ViewArchived
	return m, m.browseNotesCmd()
}

func (m Model) newNote(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = ViewEdit
	m.currentNote = &model.Note{
		Status:  model.StatusActive,
		Source:  model.SourceManual,
		Created: m.currentDate,
	}
	m.editor.Reset()
	m.editorNotice = ""
	m.editorRevision++
	m.editor.Focus()
	return m, textarea.Blink
}

func (m Model) inlineEditSelected(tea.KeyMsg) (tea.Model, tea.Cmd) {
	if item, ok := m.selectedNavItem(); ok && item.Note != nil {
		m.currentNote = item.Note
		m.mode = ViewInlineEdit
		m.inlineInput.Width = m.inlineEditWidth(item.Note)
		m.inlineInput.SetValue(item.Note.Summary)
		m.inlineInput.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

func (m *Model) beginNoteDelete(note *model.Note, returnMode ViewMode) {
	if note == nil {
		return
	}
	m.deleteTargetNotes = []*model.Note{note}
	m.deleteReturnMode = returnMode
	m.clearPendingConfirms()
	if note.FilePath == "" {
		m.confirmTitle, m.confirmPrompt = " DELETE CONFIRMATION ", "This note doesn't exist. Delete?"
	}
	m.mode = ViewDeleteConfirm
}

func (m *Model) dropUnsavedNotes(targets []*model.Note) []*model.Note {
	var saved []*model.Note
	for _, target := range targets {
		if target.FilePath != "" {
			saved = append(saved, target)
			continue
		}
		m.notes = slices.DeleteFunc(m.notes, func(note *model.Note) bool { return note == target })
		m.browsedNotes = slices.DeleteFunc(m.browsedNotes, func(note *model.Note) bool { return note == target })
	}
	m.contentVersion++
	return saved
}

func (m *Model) afterPreviewArchive(returnMode ViewMode) {
	switch returnMode {
	case ViewPreview:
		if m.previewingSearch() {
			m.afterSearchPreviewArchive()
			return
		}
		navItems := m.allNavItems()
		if m.selected >= len(navItems) {
			m.selected = max(len(navItems)-1, 0)
			m.mode = ViewDashboard
		} else {
			m.resetReviewView()
			m.updatePreviewViewport()
		}
		m.updateScrollOffset()
	}
}

func (m Model) toggleSelectedDone(tea.KeyMsg) (tea.Model, tea.Cmd) {
	if item, ok := m.selectedNavItem(); ok && item.Note != nil {
		if item.Note.Status == model.StatusDone {
			item.Note.Status = model.StatusActive
		} else {
			item.Note.Status = model.StatusDone
			item.Note.Updated = m.currentDate
		}
		return m, m.saveNotesCmd(item.Note)
	}
	return m, nil
}

func (m Model) archivedInnerWidth() int {
	return modalWidthFor(m.width) - 6
}

func (m Model) archivedTargets() []*model.Note {
	archivedNotes := m.getArchivedNotes()
	var targets []*model.Note
	if len(m.archivedSelectedMap) > 0 {
		for idx := range m.archivedSelectedMap {
			if idx < len(archivedNotes) {
				targets = append(targets, archivedNotes[idx])
			}
		}
	} else if len(archivedNotes) > 0 && m.archivedSelected < len(archivedNotes) {
		targets = append(targets, archivedNotes[m.archivedSelected])
	}
	return targets
}

func (m Model) closeArchive(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.archivedSelectedMap = make(map[int]bool)
	m.mode = ViewDashboard
	return m, nil
}

func (m Model) toggleArchiveSelection(tea.KeyMsg) (tea.Model, tea.Cmd) {
	archivedNotes := m.getArchivedNotes()
	if len(archivedNotes) > 0 && m.archivedSelected < len(archivedNotes) {
		if m.archivedSelectedMap == nil {
			m.archivedSelectedMap = make(map[int]bool)
		}
		if m.archivedSelectedMap[m.archivedSelected] {
			delete(m.archivedSelectedMap, m.archivedSelected)
		} else {
			m.archivedSelectedMap[m.archivedSelected] = true
		}
		m.archivedViewport.SetContent(m.renderArchivedContent(m.archivedInnerWidth(), m.archivedSelected))
	}
	return m, nil
}

func (m Model) archivedRowLine(selectedIndex int) int {
	line, currentGroupDate := 0, ""
	for index, note := range m.getArchivedNotes() {
		if dateLabel := note.Updated.Local().Format("Monday 02 Jan 2006"); dateLabel != currentGroupDate {
			if currentGroupDate != "" {
				line++
			}
			line++
			currentGroupDate = dateLabel
		}
		if index == selectedIndex {
			return line
		}
		line++
	}
	return line
}

func (m Model) moveArchiveSelection(delta int) (tea.Model, tea.Cmd) {
	m.archivedSelected = max(min(m.archivedSelected+delta, len(m.getArchivedNotes())-1), 0)
	m.archivedViewport.SetContent(m.renderArchivedContent(m.archivedInnerWidth(), m.archivedSelected))
	selectedLine, viewHeight := m.archivedRowLine(m.archivedSelected), max(m.archivedViewport.Height, 1)
	switch {
	case selectedLine < m.archivedViewport.YOffset:
		m.archivedViewport.SetYOffset(selectedLine)
	case selectedLine >= m.archivedViewport.YOffset+viewHeight:
		m.archivedViewport.SetYOffset(selectedLine - viewHeight + 1)
	}
	return m, nil
}

func (m Model) archivePageSize() int {
	return max(m.archivedViewport.Height, 1)
}

func (m Model) archiveCursorDown(tea.KeyMsg) (tea.Model, tea.Cmd) { return m.moveArchiveSelection(1) }

func (m Model) archiveCursorUp(tea.KeyMsg) (tea.Model, tea.Cmd) { return m.moveArchiveSelection(-1) }

func (m Model) archiveHalfPageDown(tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.moveArchiveSelection(max(m.archivePageSize()/2, 1))
}

func (m Model) archiveHalfPageUp(tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.moveArchiveSelection(-max(m.archivePageSize()/2, 1))
}

func (m Model) archivePageDown(tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.moveArchiveSelection(m.archivePageSize())
}

func (m Model) archivePageUp(tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.moveArchiveSelection(-m.archivePageSize())
}

func (m Model) deleteArchived(tea.KeyMsg) (tea.Model, tea.Cmd) {
	if targets := m.archivedTargets(); len(targets) > 0 {
		m.deleteTargetNotes = targets
		m.deleteReturnMode = ViewArchived
		m.clearPendingConfirms()
		m.mode = ViewDeleteConfirm
	}
	return m, nil
}

func (m Model) restoreArchived(tea.KeyMsg) (tea.Model, tea.Cmd) {
	targets := m.archivedTargets()
	if len(targets) == 0 {
		return m, nil
	}
	m.deleteTargetNotes = targets
	m.deleteReturnMode = ViewArchived
	m.clearPendingConfirms()
	m.restoreOnConfirm = true
	m.confirmTitle = " RESTORE CONFIRMATION "
	m.confirmPrompt = fmt.Sprintf("Restore %d note(s) to %s?", len(targets), m.currentDate.Format("Mon 02 Jan"))
	if len(targets) == 1 {
		m.confirmPrompt = fmt.Sprintf("Restore this note to %s?\n\n\"%s\"", m.currentDate.Format("Mon 02 Jan"), targets[0].Summary)
	}
	m.mode = ViewDeleteConfirm
	return m, nil
}

func (m Model) restoreArchivedNotes(targets []*model.Note) (tea.Model, tea.Cmd) {
	m.mode = ViewArchived
	for _, noteToRestore := range targets {
		noteToRestore.Status = model.StatusActive
		noteToRestore.Created = m.currentDate
		noteToRestore.Updated = m.currentDate
	}
	m.archivedSelectedMap = make(map[int]bool)
	return m, m.saveNotesCmd(targets...)
}

func (m Model) cancelInlineEdit(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = ViewDashboard
	return m, nil
}

func (m Model) saveInlineEdit(tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.currentNote != nil && strings.TrimSpace(m.inlineInput.Value()) == "" {
		m.beginNoteDelete(m.currentNote, ViewDashboard)
		m.confirmTitle, m.confirmPrompt = " DELETE CONFIRMATION ", "Delete this note?"
		return m, nil
	}
	if m.currentNote != nil {
		m.currentNote.Summary = strings.TrimSpace(m.inlineInput.Value())
		m.mode = ViewDashboard
		return m, m.saveNotesCmd(m.currentNote)
	}
	m.mode = ViewDashboard
	return m, m.loadNotesCmd
}

func (m Model) saveNote(tea.KeyMsg) (tea.Model, tea.Cmd) {
	text := m.editor.Value()
	if strings.TrimSpace(text) == "" {
		m.editorNotice = "note is empty"
		return m, nil
	}
	lines := strings.SplitN(strings.TrimSpace(text), "\n", 2)
	summary := strings.TrimSpace(lines[0])
	body := ""
	if len(lines) > 1 {
		body = strings.TrimSpace(lines[1])
	}

	m.currentNote.Summary = summary
	m.currentNote.Body = body
	if m.currentNote.ID == "" {
		m.awaitingNewNoteSave = true
	}

	m.mode = m.returnFromEdit()
	return m, m.saveNotesFromPreviewCmd(m.currentNote)
}

func (m Model) copyEditor(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.copyText(strings.TrimSpace(m.editor.Value()))
	return m, nil
}

func (m Model) cancelEdit(tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.editorChanged() {
		return m.beginDiscardConfirm(), nil
	}
	m.mode = m.returnFromEdit()
	return m, nil
}

func (m *Model) returnFromEdit() ViewMode {
	returnMode := m.editReturnMode
	m.editReturnMode = ViewDashboard
	switch returnMode {
	case ViewPreview:
		m.updatePreviewViewport()
	}
	return returnMode
}

func (m Model) beginNoteEdit(note *model.Note, returnMode ViewMode) (tea.Model, tea.Cmd) {
	m.currentNote = note
	m.editReturnMode = returnMode
	m.mode = ViewEdit
	m.replaceEditorText(fmt.Sprintf("%s\n\n%s", note.Summary, note.Body))
	m.editorNotice = ""
	m.editor.Focus()
	startEditorAtTop(m.editor)
	return m, textarea.Blink
}

const maxCachedNoteRows = 4096

type noteTagKey struct {
	source      model.Source
	status      model.Status
	created     time.Time
	currentDate time.Time
}

type noteRowKey struct {
	tags              noteTagKey
	automationTracked bool
	automationStatus  automation.RunStatus
	automated         string
	notifyInterval    string
	summary           string
	selected          bool
	width             int
}

type noteRowCache struct {
	tags map[noteTagKey][2]string
	rows map[noteRowKey]string
}

func newNoteRowCache() *noteRowCache {
	return &noteRowCache{tags: map[noteTagKey][2]string{}, rows: map[noteRowKey]string{}}
}

func (m Model) noteTagKey(n *model.Note) noteTagKey {
	return noteTagKey{source: n.Source, status: n.Status, created: n.Created, currentDate: m.currentDate}
}

func (m Model) noteTagCells(n *model.Note) (age, source string) {
	if m.noteRows == nil {
		return m.renderNoteTagCells(n)
	}
	key := m.noteTagKey(n)
	if cells, cached := m.noteRows.tags[key]; cached {
		return cells[0], cells[1]
	}
	if len(m.noteRows.tags) >= maxCachedNoteRows {
		clear(m.noteRows.tags)
	}
	age, source = m.renderNoteTagCells(n)
	m.noteRows.tags[key] = [2]string{age, source}
	return age, source
}

func noteSourceText(n *model.Note) string {
	sourceText := string(n.Source)
	if sourceText == "" {
		sourceText = "manual"
	}
	if !strings.HasPrefix(sourceText, "#") {
		sourceText = "#" + sourceText
	}
	return sourceText
}

func (m Model) renderNoteTagCells(n *model.Note) (age, source string) {
	source = dimBlueText.Render(noteSourceText(n))

	ageText := ""
	carriedOver := false
	switch {
	case n.Created.IsZero():
	case n.Status != model.StatusDone && !isSameDay(n.Created, m.currentDate) && n.Created.Before(m.currentDate):
		carriedOver = true
		ageText = fmt.Sprintf("%dd ago", daysAgo(n.Created, m.currentDate))
	case !isSameDay(n.Created, m.currentDate):
		ageText = n.Created.Format("Mon")
	default:
		ageText = n.Created.Format("15:04")
	}
	age = mutedStyle.Render(ageText)
	if carriedOver {
		age = yellowBadgeStyle.Render(ageText)
	}
	return age, source
}

func (m Model) inlineEditWidth(*model.Note) int {
	return max(m.width-3-3-4, 5)
}

func (m Model) renderRow(n *model.Note, selected bool, width int) string {
	if m.noteRows == nil || (selected && (m.mode == ViewInlineEdit || m.mode == ViewNotifyInput)) {
		return m.renderNoteRow(n, selected, width)
	}
	key := m.noteRowCacheKey(n, selected, width)
	if row, cached := m.noteRows.rows[key]; cached {
		return row
	}
	if len(m.noteRows.rows) >= maxCachedNoteRows {
		clear(m.noteRows.rows)
	}
	row := m.renderNoteRow(n, selected, width)
	m.noteRows.rows[key] = row
	return row
}

func (m Model) noteRowCacheKey(n *model.Note, selected bool, width int) noteRowKey {
	run, tracked := m.automationRuns[n.ID]
	notifyInterval := ""
	if entry, reminding := m.notifyEntries[n.ID]; reminding && noteIsActive(n) {
		notifyInterval = entry.Interval
	}
	return noteRowKey{
		tags:              m.noteTagKey(n),
		automationTracked: tracked,
		automationStatus:  run.Status,
		automated:         n.Automated,
		notifyInterval:    notifyInterval,
		summary:           n.Summary,
		selected:          selected,
		width:             width,
	}
}

func (m Model) noteRowTags(n *model.Note) []string {
	tags := m.noteRowTagsWithoutNotify(n)
	if notifyTag := m.notifyTag(n); notifyTag != "" {
		tags = append([]string{notifyTag}, tags...)
	}
	return tags
}

func (m Model) noteRowTagsWithoutNotify(n *model.Note) []string {
	age, source := m.noteTagCells(n)
	tags := []string{age, source}
	if automationTag := m.automationTag(n); automationTag != "" {
		tags = append([]string{automationTag}, tags...)
	}
	return tags
}

func (m Model) visibleNoteTags(n *model.Note, selected bool) []string {
	if selected || m.cfg.ShowAllTags() || n.Source == model.SourcePRReview || prReviewNoteURL(n) != "" {
		return m.noteRowTags(n)
	}
	if notifyTag := m.notifyTag(n); notifyTag != "" {
		return []string{notifyTag}
	}
	return nil
}

func (m Model) renderNoteRow(n *model.Note, selected bool, width int) string {
	return m.renderNoteRowWithTags(n, selected, width, m.visibleNoteTags(n, selected))
}

func (m Model) renderNoteRowWithTags(n *model.Note, selected bool, width int, tags []string) string {
	prefix := "   "

	boxChar := "☐"
	if n.Status == model.StatusDone {
		boxChar = "✔"
	}
	var leftBlock string
	switch {
	case selected && m.mode == ViewInlineEdit:
		leftBlock = fmt.Sprintf("%s%s %s", prefix, checkPending.Render(), m.inlineInput.View())
		tags = nil
	case selected && m.mode == ViewNotifyInput:
		leftBlock = fmt.Sprintf("%s%s %s", prefix, selectedPendingBox.Render(boxChar), selectedTitle(n.Summary))
		tags = append([]string{m.notifyInput.View()}, m.noteRowTagsWithoutNotify(n)...)
		if m.notifyNotice != "" {
			tags = []string{m.notifyInput.View(), staleStyle.Render(m.notifyNotice)}
		}
	case selected:
		boxStyle := selectedPendingBox
		if n.Status == model.StatusDone {
			boxStyle = selectedDoneBox
		}
		leftBlock = fmt.Sprintf("%s%s %s", prefix, boxStyle.Render(boxChar), selectedTitle(n.Summary))
	default:
		box := checkPending.Render()
		if n.Status == model.StatusDone {
			box = checkDone.Render()
		}
		leftBlock = fmt.Sprintf("%s%s %s", prefix, box, itemStyle.Render(n.Summary))
	}
	return alignRight(leftBlock, tags, width, selected) + "\n"
}

func (notesSection) ApplyMessage(m Model, msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case notesReloadedMsg:
		if !storeResultUsable(msg.err) {
			m.reportStoreError(msg.err)
			return messageHandled(m, nil)
		}
		selectedKey, selectedOccurrence := m.selectedNavKey()
		notesSection{}.replaceViewedDay(&m, msg.notes)
		m.restoreSelection(selectedKey, selectedOccurrence)
		m.postMessage(messageSourceNotes, messageSuccess, "reloaded")
		m.reportStoreError(msg.err)
		m.updateScrollOffset()
		return messageHandled(m, tea.Batch(refreshNotifyCmd(m.cfg.Root(), m.notes), m.closedThisWeekCmd()))

	case browsedNotesMsg:
		m.reportStoreError(msg.err)
		if !m.browsing() || !storeResultUsable(msg.err) {
			return messageHandled(m, nil)
		}
		m.browsedNotes = msg.notes
		if m.browsedNotes == nil {
			m.browsedNotes = []*model.Note{}
		}
		m.shareDashboardNotes()
		m.refreshBrowsedView()
		return messageHandled(m, nil)

	case closedThisWeekMsg:
		if !storeResultUsable(msg.err) {
			m.reportStoreError(msg.err)
			return messageHandled(m, nil)
		}
		m.closedThisWeekElsewhere = msg.count
		return messageHandled(m, nil)

	case notesChangedMsg:
		m.reportStoreError(msg.err)
		notesSection{}.mergeNotes(&m, msg.notes)
		m.updateScrollOffset()
		if m.mode == ViewPreview && m.previewingSearch() {
			m.updatePreviewViewport()
		}
		if m.mode == ViewArchived {
			m.refreshArchivedViewport()
		}
		return messageHandled(m, refreshNotifyCmd(m.cfg.Root(), m.notes))

	case notesSavedMsg:
		return messageHandled(m.applySavedNotes(msg))

	case notesDeletedMsg:
		return messageHandled(m.applyDeletedNotes(msg))

	case reminderSavedMsg:
		next, cmd := m.applyReminderSaved(msg)
		return messageHandled(next, cmd)

	case notifyEntriesMsg:
		if msg.err != nil {
			m.showError("NOTIFY ERROR", msg.err)
			return messageHandled(m, nil)
		}
		m.notifyEntries = msg.entries
		return messageHandled(m, nil)

	case appStateSavedMsg:
		if msg.err != nil {
			m.showError("STATE ERROR", msg.err)
		}
		return messageHandled(m, nil)
	}
	return m, nil, false
}

func (notesSection) storeLoadedNotes(m *Model, msg loadNotesMsg) {
	if storeResultUsable(msg.err) {
		m.notes = msg.notes
		m.shareDashboardNotes()
	}
	if m.initialSelectionPending {
		m.initialSelectionPending = false
		m.selectLaunchItem()
	}
	if m.selectAfterReload != "" {
		m.selectNoteByID(m.selectAfterReload)
		m.selectAfterReload = ""
	}
}

func (notesSection) finishLoadedNotes(m Model, msg loadNotesMsg, refetchPreviousDay tea.Cmd) (tea.Model, tea.Cmd) {
	m.updateScrollOffset()
	if m.mode == ViewPreview && m.previewingSearch() {
		m.updatePreviewViewport()
	}
	if m.mode == ViewArchived {
		m.refreshArchivedViewport()
	}
	m.reportStoreError(msg.err)
	if !storeResultUsable(msg.err) {
		return m, refetchPreviousDay
	}
	return m, tea.Batch(refetchPreviousDay, refreshNotifyCmd(m.cfg.Root(), m.notes), m.closedThisWeekCmd())
}

var notesKeystrokes sectionKeystrokes

func init() {
	notesKeystrokes = sectionKeystrokes{
		actionNewNote:                Model.newNote,
		actionInlineEdit:             Model.inlineEditSelected,
		actionToggleDone:             Model.toggleSelectedDone,
		actionOpenArchive:            Model.openArchive,
		actionCloseArchive:           Model.closeArchive,
		actionArchiveCursorDown:      Model.archiveCursorDown,
		actionArchiveCursorUp:        Model.archiveCursorUp,
		actionArchiveHalfPageDown:    Model.archiveHalfPageDown,
		actionArchiveHalfPageUp:      Model.archiveHalfPageUp,
		actionArchivePageDown:        Model.archivePageDown,
		actionArchivePageUp:          Model.archivePageUp,
		actionToggleArchiveSelection: Model.toggleArchiveSelection,
		actionRestoreArchived:        Model.restoreArchived,
		actionDeleteArchived:         Model.deleteArchived,
		actionCancelInlineEdit:       Model.cancelInlineEdit,
		actionSaveInlineEdit:         Model.saveInlineEdit,
		actionSaveNote:               Model.saveNote,
		actionCopyEditor:             Model.copyEditor,
		actionCancelEdit:             Model.cancelEdit,
		actionRecreateNote:           Model.confirmRecreateNote,
		actionDiscardMissingNote:     Model.discardMissingNote,
		actionConfirmNotify:          Model.confirmNotify,
		actionCancelNotify:           Model.cancelNotify,
		actionReloadNotes:            Model.reloadViewedDay,
	}
}

func (notesSection) ApplyKeystrokes(m Model, binding keyBinding, msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	return notesKeystrokes.apply(m, binding, msg)
}

func (section notesSection) Render(m Model, builder *dashboardBuilder, data dashboardData) {
	groups := data.groups
	builder.board.WriteString("\n " + renderSectionTitle(m.previousDayTitleFor(groups.previousDay), builder.selectedWithin(0, data.previousEnd)) + "\n")
	if len(groups.previousDone) > 0 {
		builder.board.WriteString("\n")
	}
	section.emitNotes(m, builder, groups.previousDone, "")

	builder.board.WriteString(sectionGap)
	jobBadge := fmt.Sprintf("%d jobs %s", len(data.drafts), amberDiamond.Render())
	todayActive := builder.selectedWithin(data.previousEnd, data.closedEnd) || builder.selectedWithin(data.pendingEnd, data.jobsEnd)
	todayTitle := renderSectionTitle(m.dayTitleText(m.currentDate, "T O D A Y"), todayActive)
	todayGap := builder.innerWidth - lipgloss.Width(todayTitle) - lipgloss.Width(jobBadge)
	builder.board.WriteString(fmt.Sprintf(" %s%s%s\n\n", todayTitle, safeRepeat(" ", todayGap), jobBadge))

	builder.board.WriteString("  " + m.renderSubSection("Pending, Carried Over", len(groups.carried), true, builder.selectedWithin(data.previousEnd, data.carriedEnd)) + "\n")
	section.emitNotes(m, builder, groups.carried, "   (no carried over notes)")
	builder.board.WriteString(sectionGap + "  " + m.renderSubSection("Added Today", len(groups.today), true, builder.selectedWithin(data.carriedEnd, data.addedEnd)) + "\n")
	section.emitNotes(m, builder, groups.today, "   (no notes added today)")
	builder.board.WriteString(sectionGap + "  " + m.renderSubSection("Closed Today", len(groups.todayDone), true, builder.selectedWithin(data.addedEnd, data.closedEnd)) + "\n")
	section.emitNotes(m, builder, groups.todayDone, "   (no notes closed today)")
}

func (notesSection) emitNotes(m Model, builder *dashboardBuilder, notes []*model.Note, emptyHint string) {
	if len(notes) == 0 && emptyHint != "" {
		builder.board.WriteString(mutedStyle.Render(emptyHint + "\n"))
	}
	for _, note := range notes {
		builder.emitRow(func(selected bool) string { return m.renderRow(note, selected, builder.rowWidth) })
	}
}

func (notesSection) appendNavItems(m Model, items []NavItem, data dashboardData) []NavItem {
	for _, n := range data.groups.previousDone {
		items = append(items, NavItem{Kind: KindYesterdayDone, Note: n})
	}
	for _, n := range data.groups.carried {
		items = append(items, NavItem{Kind: KindCarriedNote, Note: n})
	}
	for _, n := range data.groups.today {
		items = append(items, NavItem{Kind: KindTodayNote, Note: n})
	}
	for _, n := range data.groups.todayDone {
		items = append(items, NavItem{Kind: KindTodayDone, Note: n})
	}
	return items
}

type notesChangedMsg struct {
	notes []model.Note
	err   error
}

func indexOfNote(notes []*model.Note, target *model.Note, snapshot model.Note) int {
	if index := slices.Index(notes, target); target != nil && index >= 0 {
		return index
	}
	return slices.IndexFunc(notes, func(note *model.Note) bool {
		return (snapshot.ID != "" && note.ID == snapshot.ID) || (snapshot.FilePath != "" && note.FilePath == snapshot.FilePath)
	})
}

func (notesSection) mergeNotes(m *Model, notes []model.Note) {
	for _, changed := range notes {
		var merged *model.Note
		if index := indexOfNote(m.notes, nil, changed); index >= 0 {
			merged = m.notes[index]
			*merged = changed
		}
		if index := indexOfNote(m.browsedNotes, nil, changed); index >= 0 {
			if merged == nil {
				merged = m.browsedNotes[index]
			}
			*m.browsedNotes[index] = changed
			m.browsedNotes[index] = merged
		}
		if merged == nil {
			changedCopy := changed
			merged = &changedCopy
			if m.browsedNotes != nil {
				m.browsedNotes = append(m.browsedNotes, merged)
			}
		}
		if !slices.Contains(m.notes, merged) && m.belongsOnDashboard(merged) {
			m.notes = append(m.notes, merged)
		}
	}
}

type notesReloadedMsg struct {
	notes []*model.Note
	err   error
}

func (m Model) reloadViewedDay(tea.KeyMsg) (tea.Model, tea.Cmd) {
	noteStore, viewedDay, previousDay := m.store, m.currentDate, m.previousNoteDay()
	return m, func() tea.Msg {
		notes, err := noteStore.ListDashboard(viewedDay, previousDay)
		return notesReloadedMsg{notes: notes, err: err}
	}
}

func (notesSection) replaceViewedDay(m *Model, reloaded []*model.Note) {
	m.notes = reloaded
	m.shareDashboardNotes()
}
