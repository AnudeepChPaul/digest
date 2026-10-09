package tui

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/automation"
	"github.com/AnudeepChPaul/digest/pkg/model"
	"github.com/AnudeepChPaul/digest/pkg/store"
	"github.com/AnudeepChPaul/digest/pkg/tui/textarea"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type notesSection struct{}

type loadNotesMsg struct {
	notes    []*model.Note
	err      error
	complete bool
}

func (m Model) startupNotesCmd() tea.Cmd {
	notes, err := m.notes, m.startupNotesErr
	return func() tea.Msg {
		return loadNotesMsg{notes: notes, err: err}
	}
}

func (m Model) loadNotesCmd() tea.Msg {
	if m.notesComplete {
		notes, err := m.store.List()
		return loadNotesMsg{notes: notes, err: err, complete: true}
	}
	notes, err := m.store.ListDashboard(m.currentDate)
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

func (m Model) previousNoteDay() time.Time {
	viewedDayStart := time.Date(m.currentDate.Year(), m.currentDate.Month(), m.currentDate.Day(), 0, 0, 0, 0, m.currentDate.Location())
	var latest time.Time
	for _, note := range m.notes {
		if note.Status == model.StatusArchived || (note.Source != model.SourceManual && note.Source != "") {
			continue
		}
		for _, stamp := range []time.Time{note.Created, note.Updated} {
			if stamp.Before(viewedDayStart) && stamp.After(latest) {
				latest = stamp
			}
		}
	}
	if latest.IsZero() {
		return m.currentDate.AddDate(0, 0, -1)
	}
	return latest
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
			if isSameDay(n.Updated, groups.previousDay) {
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
	if isSameDay(previousDay, m.currentDate.AddDate(0, 0, -1)) {
		return m.dayTitleText(previousDay, "Y E S T E R D A Y")
	}
	return m.dayTitleText(previousDay, letterSpaced(strings.ToUpper(previousDay.Format("Monday"))))
}

func letterSpaced(word string) string {
	return strings.Join(strings.Split(word, ""), " ")
}

func (m Model) getArchivedNotes() []*model.Note {
	var list []*model.Note
	for _, n := range m.notes {
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
	if isSameDay(m.currentDate, time.Now()) {
		return fmt.Sprintf("%s  ·  %s", spacedLabel, strings.ToUpper(date.Format("02 Jan")))
	}
	return fmt.Sprintf("%s . %s", strings.ToUpper(date.Format("Monday")), strings.ToUpper(date.Format("02 Jan")))
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

func loadAllNotesCmd(noteStore *store.NoteStore) tea.Cmd {
	return func() tea.Msg {
		notes, err := noteStore.List()
		return loadNotesMsg{notes: notes, err: err, complete: true}
	}
}

func (m *Model) ensureAllNotes() tea.Cmd {
	if m.notesComplete || m.loadingAllNotes {
		return nil
	}
	m.loadingAllNotes = true
	return loadAllNotesCmd(m.store)
}

func (m Model) reloadNotesForDay() tea.Cmd {
	if m.notesComplete {
		return nil
	}
	return m.loadNotesCmd
}

func (m Model) renderArchivedModal(modalWidth int) string {
	titleText := modalTitleStyle.Render(" ARCHIVED NOTES ")

	innerHeight := m.height - 10 - footerLineCount(archiveFooterItems)
	if innerHeight < 4 {
		innerHeight = 4
	}
	m.archivedViewport.Height = innerHeight

	footerText := renderModalFooter(archiveFooterItems, modalWidth-6)

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
		"",
		footerText,
	)

	return m.framedPopup(popupContent, modalWidth)
}

func (m Model) openArchive(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.archivedSelected = 0
	m.archivedSelectedMap = make(map[int]bool)
	modalWidth := modalWidthFor(m.width)
	innerWidth := modalWidth - 6
	innerHeight := m.height - 10 - footerLineCount(archiveFooterItems)
	if innerHeight < 4 {
		innerHeight = 4
	}

	m.archivedViewport = viewport.New(innerWidth, innerHeight)
	m.archivedViewport.SetContent(m.renderArchivedContent(innerWidth, m.archivedSelected))
	m.mode = ViewArchived
	return m, m.ensureAllNotes()
}

func (m Model) newNote(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = ViewEdit
	m.currentNote = &model.Note{
		Status:  model.StatusActive,
		Source:  model.SourceManual,
		Created: m.currentDate,
	}
	m.editor.Reset()
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
	if note == nil || note.FilePath == "" {
		return
	}
	m.deleteTargetNotes = []*model.Note{note}
	m.deleteReturnMode = returnMode
	m.clearPendingConfirms()
	m.mode = ViewDeleteConfirm
}

func (m *Model) afterPreviewArchive(returnMode ViewMode) {
	switch returnMode {
	case ViewPreview:
		navItems := m.allNavItems()
		if m.selected >= len(navItems) {
			m.selected = max(len(navItems)-1, 0)
			m.mode = ViewDashboard
		} else {
			m.resetReviewView()
			m.updatePreviewViewport()
		}
		m.updateScrollOffset()
	case ViewSearchPreview:
		results := m.searchResults()
		if m.searchSelected >= len(results) {
			m.searchSelected = max(len(results)-1, 0)
			m.mode = ViewSearch
			m.keepSearchSelectionVisible()
			m.searchInput.Focus()
		} else {
			m.showSearchPreviewAt(m.searchSelected)
		}
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

func (m Model) archiveCursorDown(tea.KeyMsg) (tea.Model, tea.Cmd) {
	archivedNotes := m.getArchivedNotes()
	if len(archivedNotes) > 0 && m.archivedSelected < len(archivedNotes)-1 {
		m.archivedSelected++
		m.archivedViewport.SetContent(m.renderArchivedContent(m.archivedInnerWidth(), m.archivedSelected))
	}
	return m, nil
}

func (m Model) archiveCursorUp(tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.archivedSelected > 0 {
		m.archivedSelected--
		m.archivedViewport.SetContent(m.renderArchivedContent(m.archivedInnerWidth(), m.archivedSelected))
	}
	return m, nil
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
	lines := strings.SplitN(strings.TrimSpace(text), "\n", 2)

	summary := "Untitled Note"
	body := ""
	if len(lines) > 0 && strings.TrimSpace(lines[0]) != "" {
		summary = strings.TrimSpace(lines[0])
	}
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
	_ = copyToClipboard(m.editor.Value())
	return m, nil
}

func (m Model) cancelEdit(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = m.returnFromEdit()
	return m, nil
}

func (m *Model) returnFromEdit() ViewMode {
	returnMode := m.editReturnMode
	m.editReturnMode = ViewDashboard
	switch returnMode {
	case ViewSearchPreview:
		m.updateSearchPreviewViewport()
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
		if msg.err != nil {
			m.showError("STORE ERROR", msg.err)
			return messageHandled(m, nil)
		}
		selectedKey, selectedOccurrence := m.selectedNavKey()
		notesSection{}.replaceViewedDay(&m, msg.notes)
		m.restoreSelection(selectedKey, selectedOccurrence)
		m.postMessage(messageSourceNotes, messageSuccess, "reloaded")
		m.updateScrollOffset()
		return messageHandled(m, refreshNotifyCmd(m.cfg.Root(), m.notes))

	case notesChangedMsg:
		if msg.err != nil {
			m.showError("STORE ERROR", msg.err)
		}
		notesSection{}.mergeNotes(&m, msg.notes)
		m.updateScrollOffset()
		if m.mode == ViewSearchPreview {
			m.updateSearchPreviewViewport()
		}
		if m.mode == ViewArchived {
			m.refreshArchivedViewport()
		}
		return messageHandled(m, refreshNotifyCmd(m.cfg.Root(), m.notes))

	case notesSavedMsg:
		return messageHandled(m.applySavedNotes(msg))

	case notesDeletedMsg:
		return messageHandled(m.applyDeletedNotes(msg))

	case notifyEntriesMsg:
		if msg.err != nil {
			m.showError("NOTIFY ERROR", msg.err)
			return messageHandled(m, nil)
		}
		m.notifyEntries = msg.entries
		return messageHandled(m, nil)

	case actionUsageSavedMsg:
		if msg.err != nil {
			m.showError("ACTION USAGE ERROR", msg.err)
		}
		return messageHandled(m, nil)
	}
	return m, nil, false
}

func (notesSection) storeLoadedNotes(m *Model, msg loadNotesMsg) {
	if msg.complete {
		m.loadingAllNotes = false
	}
	if msg.err == nil && (msg.complete || !m.notesComplete) {
		m.notes = msg.notes
		m.notesComplete = m.notesComplete || msg.complete
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
	if m.mode == ViewSearchPreview {
		m.updateSearchPreviewViewport()
	}
	if m.mode == ViewArchived {
		m.refreshArchivedViewport()
	}
	if msg.err != nil {
		m.showError("STORE ERROR", msg.err)
		return m, refetchPreviousDay
	}
	return m, tea.Batch(refetchPreviousDay, refreshNotifyCmd(m.cfg.Root(), m.notes))
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

func (notesSection) mergeNotes(m *Model, notes []model.Note) {
	for _, changed := range notes {
		index := slices.IndexFunc(m.notes, func(note *model.Note) bool {
			return (changed.ID != "" && note.ID == changed.ID) || (changed.FilePath != "" && note.FilePath == changed.FilePath)
		})
		if index < 0 {
			changedCopy := changed
			m.notes = append(m.notes, &changedCopy)
			continue
		}
		*m.notes[index] = changed
	}
}

type notesReloadedMsg struct {
	notes []*model.Note
	err   error
}

func (m Model) reloadViewedDay(tea.KeyMsg) (tea.Model, tea.Cmd) {
	noteStore, viewedDay := m.store, m.currentDate
	return m, func() tea.Msg {
		notes, err := noteStore.ListDashboard(viewedDay)
		return notesReloadedMsg{notes: notes, err: err}
	}
}

func (section notesSection) replaceViewedDay(m *Model, reloaded []*model.Note) {
	if !m.notesComplete {
		m.notes = reloaded
		return
	}
	reloadedPaths := make(map[string]bool, len(reloaded))
	for _, note := range reloaded {
		reloadedPaths[note.FilePath] = true
	}
	m.notes = slices.DeleteFunc(m.notes, func(note *model.Note) bool {
		active := note.Status != model.StatusDone && note.Status != model.StatusArchived
		return active && !reloadedPaths[note.FilePath]
	})
	copies := make([]model.Note, len(reloaded))
	for index, note := range reloaded {
		copies[index] = *note
	}
	section.mergeNotes(m, copies)
}
