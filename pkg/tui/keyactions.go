package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"app/pkg/model"
	"app/pkg/review"
	"app/pkg/sourcecontrol"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

type keyActionHandler func(Model, tea.KeyMsg) (tea.Model, tea.Cmd)

var keyActionHandlers map[keyAction]keyActionHandler

func init() {
	keyActionHandlers = map[keyAction]keyActionHandler{
		actionQuit:              Model.quitFromDashboard,
		actionDismissSyncErrors: Model.dismissSyncErrors,
		actionSync:              Model.syncFromKey,
		actionToggleSortField:   Model.toggleSortField,
		actionToggleSortOrder:   Model.toggleSortOrder,
		actionRunJobs:           Model.runJobsFromKey,
		actionHalfPageDown:      Model.dashboardHalfPageDown,
		actionHalfPageUp:        Model.dashboardHalfPageUp,
		actionToday:             Model.jumpToToday,
		actionOpenArchive:       Model.openArchive,
		actionOpenPreview:       Model.openSelectedPreview,
		actionNewNote:           Model.newNote,
		actionOpenItem:          Model.openSelectedItem,
		actionInlineEdit:        Model.inlineEditSelected,
		actionDeleteItem:        Model.deleteSelectedItem,
		actionToggleDone:        Model.toggleSelectedDone,
		actionPreviousDay:       Model.previousDay,
		actionNextDay:           Model.nextDay,
		actionCursorDown:        Model.dashboardCursorDown,
		actionCursorUp:          Model.dashboardCursorUp,
		actionOpenSearch:        Model.openSearch,

		actionCloseGitDetails: Model.closeGitDetails,
		actionSwitchGitFilter: Model.switchGitFilter,
		actionGitCursorDown:   Model.gitCursorDown,
		actionGitCursorUp:     Model.gitCursorUp,
		actionOpenGitItem:     Model.openGitItem,

		actionConfirmDelete: Model.confirmDelete,
		actionCancelDelete:  Model.cancelDelete,

		actionConfirmReview: Model.confirmReview,
		actionCancelReview:  Model.cancelReview,

		actionConfirmReviewRun: Model.confirmReviewRun,
		actionCancelReviewRun:  Model.cancelReviewRun,

		actionSubmitRejectComment: Model.submitRejectComment,
		actionCancelRejectComment: Model.cancelRejectComment,

		actionClosePreview:    Model.closePreview,
		actionPreviewPrevious: Model.previewPrevious,
		actionPreviewNext:     Model.previewNext,
		actionCopyPreviewItem: Model.copyPreviewItem,
		actionPreviewStop:     Model.previewStop,
		actionPreviewEnter:    Model.previewEnter,

		actionSwitchPreviewTab:   Model.switchPreviewTab,
		actionStartReview:        Model.startReviewFromKey,
		actionToggleFinding:      Model.toggleFinding,
		actionSelectAllFindings:  Model.selectAllFindings,
		actionPostReview:         Model.postReview,
		actionApprove:            Model.approvePR,
		actionRejectOrStopReview: Model.rejectOrStopReview,
		actionOpenClone:          Model.openCloneFromKey,
		actionFindingDown:        Model.findingDown,
		actionFindingUp:          Model.findingUp,
		actionIgnoreKey:          Model.ignoreKey,

		actionCloseArchive:           Model.closeArchive,
		actionArchiveCursorDown:      Model.archiveCursorDown,
		actionArchiveCursorUp:        Model.archiveCursorUp,
		actionToggleArchiveSelection: Model.toggleArchiveSelection,
		actionRestoreArchived:        Model.restoreArchived,
		actionDeleteArchived:         Model.deleteArchived,

		actionCancelInlineEdit: Model.cancelInlineEdit,
		actionSaveInlineEdit:   Model.saveInlineEdit,

		actionSaveNote:   Model.saveNote,
		actionCopyEditor: Model.copyEditor,
		actionCancelEdit: Model.cancelEdit,

		actionCloseSearch:        Model.closeSearch,
		actionSearchCursorUp:     Model.searchCursorUp,
		actionSearchCursorDown:   Model.searchCursorDown,
		actionSearchPageUp:       Model.searchPageUp,
		actionSearchPageDown:     Model.searchPageDown,
		actionSearchHalfPageUp:   Model.searchHalfPageUp,
		actionSearchHalfPageDown: Model.searchHalfPageDown,
		actionOpenSearchPreview:  Model.openSearchPreview,
		actionExportSearch:       Model.exportSearch,

		actionCloseSearchPreview:    Model.closeSearchPreview,
		actionSearchPreviewNext:     Model.searchPreviewNext,
		actionSearchPreviewPrevious: Model.searchPreviewPrevious,
		actionEditSearchResult:      Model.editSearchResult,
		actionCopySearchResult:      Model.copySearchResult,
		actionDeleteSearchResult:    Model.deleteSearchResult,

		actionDismissError: Model.dismissError,
	}
}

func (m Model) resolveKey(msg tea.KeyMsg) (keyBinding, bool) {
	for _, binding := range m.activeBindings() {
		if key.Matches(msg, binding.binding) {
			return binding, true
		}
	}
	return keyBinding{}, false
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.clampScreenSelection()
	if binding, found := m.resolveKey(msg); found {
		if handler, ok := keyActionHandlers[binding.action]; ok {
			return handler(m, msg)
		}
		return m, nil
	}
	if msg.String() == "ctrl+c" {
		return m, nil
	}
	return m.forwardUnboundKey(msg)
}

func (m *Model) clampScreenSelection() {
	switch m.mode {
	case ViewDashboard:
		navItems := m.allNavItems()
		if m.selected >= len(navItems) && len(navItems) > 0 {
			m.selected = len(navItems) - 1
		}
	case ViewGitDetails:
		gitItems := m.filteredGitItems()
		if m.gitPopupSelected >= len(gitItems) && len(gitItems) > 0 {
			m.gitPopupSelected = len(gitItems) - 1
		}
	case ViewArchived:
		archivedNotes := m.getArchivedNotes()
		if m.archivedSelected >= len(archivedNotes) && len(archivedNotes) > 0 {
			m.archivedSelected = len(archivedNotes) - 1
		}
		if m.archivedSelected < 0 {
			m.archivedSelected = 0
		}
	}
}

func (m Model) forwardUnboundKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.mode {
	case ViewPreview:
		scrollViewport(&m.previewViewport, msg.String())
		return m, nil
	case ViewArchived:
		scrollViewport(&m.archivedViewport, msg.String())
		return m, nil
	case ViewInlineEdit:
		m.inlineInput, cmd = m.inlineInput.Update(msg)
		return m, cmd
	case ViewEdit:
		m.editor, cmd = m.editor.Update(msg)
		return m, cmd
	case ViewSearch:
		previousQuery := m.searchInput.Value()
		m.searchInput, cmd = m.searchInput.Update(msg)
		if m.searchInput.Value() != previousQuery {
			m.searchSelected, m.searchScroll = 0, 0
			m.searchNotice = ""
		}
		return m, cmd
	case ViewSearchPreview:
		scrollViewport(&m.previewViewport, msg.String())
		return m, nil
	case ViewRejectComment:
		m.rejectInput, cmd = m.rejectInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) selectedNavItem() (NavItem, bool) {
	navItems := m.allNavItems()
	if len(navItems) > 0 && m.selected < len(navItems) {
		return navItems[m.selected], true
	}
	return NavItem{}, false
}

func (m Model) quitFromDashboard(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.ctrlCCount++
	if m.ctrlCCount >= 3 {
		if m.gitCancel != nil {
			m.gitCancel()
		}
		return m, tea.Quit
	}
	return m, tickCtrlCResetCmd()
}

func (m Model) dismissSyncErrors(tea.KeyMsg) (tea.Model, tea.Cmd) {
	if len(m.syncErrors) > 0 {
		m.syncErrors = nil
	}
	return m, nil
}

func (m Model) syncFromKey(tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m, m.startLoadGitStatsCmd(false)
}

func (m Model) toggleSortField(tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m, m.changePendingSort(func(activeSort *sourcecontrol.Sort) { activeSort.ByCreated = !activeSort.ByCreated })
}

func (m Model) toggleSortOrder(tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m, m.changePendingSort(func(activeSort *sourcecontrol.Sort) { activeSort.Ascending = !activeSort.Ascending })
}

func (m Model) runJobsFromKey(tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m, m.startJobDryRunsCmd()
}

func (m Model) dashboardHalfPageDown(tea.KeyMsg) (tea.Model, tea.Cmd) {
	navItems := m.allNavItems()
	bodyHeight := m.dashboardBodyHeight()
	if bodyHeight < 5 {
		bodyHeight = 5
	}
	m.selected += bodyHeight / 2
	if len(navItems) > 0 && m.selected >= len(navItems) {
		m.selected = len(navItems) - 1
	}
	m.updateScrollOffset()
	return m, nil
}

func (m Model) dashboardHalfPageUp(tea.KeyMsg) (tea.Model, tea.Cmd) {
	bodyHeight := m.dashboardBodyHeight()
	if bodyHeight < 5 {
		bodyHeight = 5
	}
	m.selected -= bodyHeight / 2
	if m.selected < 0 {
		m.selected = 0
	}
	m.updateScrollOffset()
	return m, nil
}

func (m Model) jumpToToday(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.currentDate = time.Now()
	m.selected = 0
	m.scrollOffset = 0
	return m, m.startLoadGitStatsCmd(true)
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
	return m, nil
}

func (m Model) openSelectedPreview(tea.KeyMsg) (tea.Model, tea.Cmd) {
	if item, ok := m.selectedNavItem(); ok {
		return m.openPreview(item)
	}
	return m, nil
}

func (m Model) newNote(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = ViewEdit
	m.currentNote = &model.Note{
		Status:  model.StatusActive,
		Source:  model.SourceManual,
		Created: m.currentDate,
	}
	m.editor.Reset()
	m.editor.Focus()
	return m, textarea.Blink
}

func (m Model) openSelectedItem(tea.KeyMsg) (tea.Model, tea.Cmd) {
	item, ok := m.selectedNavItem()
	if !ok {
		return m, nil
	}
	if item.Kind == KindJobDraft || item.Kind == KindReviewRun {
		return m.openPreview(item)
	}
	if item.Kind == KindPendingGit && item.PendingGitPR != nil {
		_ = openURL(item.PendingGitPR.URL)
		return m, nil
	}
	if item.Note != nil {
		m.currentNote = item.Note
		m.mode = ViewEdit
		m.editor.SetValue(fmt.Sprintf("%s\n\n%s", item.Note.Summary, item.Note.Body))
		m.editor.Focus()
		return m, textarea.Blink
	}
	return m, nil
}

func (m Model) inlineEditSelected(tea.KeyMsg) (tea.Model, tea.Cmd) {
	if item, ok := m.selectedNavItem(); ok && item.Note != nil {
		m.currentNote = item.Note
		m.mode = ViewInlineEdit
		m.inlineInput.SetValue(item.Note.Summary)
		m.inlineInput.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

func (m Model) deleteSelectedItem(tea.KeyMsg) (tea.Model, tea.Cmd) {
	item, ok := m.selectedNavItem()
	if !ok {
		return m, nil
	}
	if item.Kind == KindReviewRun && item.ReviewRun != nil && item.ReviewRun.Status == review.RunRunning {
		return m.beginReviewRunConfirm(reviewActionStop, item.ReviewRun.Meta.Ref)
	}
	if item.Kind == KindJobDraft && item.Draft != nil && isJobRunning(item.Draft.Name) {
		m.jobToAbort = item.Draft.Name
		m.deleteTargetNotes = nil
		m.deleteReturnMode = ViewDashboard
		m.mode = ViewDeleteConfirm
		return m, nil
	}
	if item.Note != nil {
		m.beginNoteDelete(item.Note, ViewDashboard)
	}
	return m, nil
}

func (m *Model) beginNoteDelete(note *model.Note, returnMode ViewMode) {
	if note == nil || note.FilePath == "" {
		return
	}
	m.deleteTargetNotes = []*model.Note{note}
	m.deleteReturnMode = returnMode
	m.jobToExecute = ""
	m.jobToAbort = ""
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

func (m Model) previousDay(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.currentDate = m.currentDate.AddDate(0, 0, -1)
	m.selected = 0
	m.scrollOffset = 0
	return m, m.startLoadGitStatsCmd(true)
}

func (m Model) nextDay(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.currentDate = m.currentDate.AddDate(0, 0, 1)
	m.selected = 0
	m.scrollOffset = 0
	return m, m.startLoadGitStatsCmd(true)
}

func (m Model) dashboardCursorDown(tea.KeyMsg) (tea.Model, tea.Cmd) {
	navItems := m.allNavItems()
	if len(navItems) > 0 && m.selected < len(navItems)-1 {
		m.selected++
		m.updateScrollOffset()
	}
	return m, nil
}

func (m Model) dashboardCursorUp(tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.selected > 0 {
		m.selected--
		m.updateScrollOffset()
	}
	return m, nil
}

func (m Model) openSearch(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = ViewSearch
	m.keepSearchSelectionVisible()
	m.searchInput.Focus()
	return m, textinput.Blink
}

func (m Model) closeGitDetails(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = ViewDashboard
	return m, nil
}

func (m Model) switchGitFilter(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.gitPopupTab = (m.gitPopupTab + 1) % 4
	m.gitPopupSelected = 0
	return m, nil
}

func (m Model) gitCursorDown(tea.KeyMsg) (tea.Model, tea.Cmd) {
	gitItems := m.filteredGitItems()
	if len(gitItems) > 0 && m.gitPopupSelected < len(gitItems)-1 {
		m.gitPopupSelected++
	}
	return m, nil
}

func (m Model) gitCursorUp(tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.gitPopupSelected > 0 {
		m.gitPopupSelected--
	}
	return m, nil
}

func (m Model) openGitItem(tea.KeyMsg) (tea.Model, tea.Cmd) {
	gitItems := m.filteredGitItems()
	if len(gitItems) > 0 && m.gitPopupSelected < len(gitItems) {
		_ = openURL(gitItems[m.gitPopupSelected].URL)
	}
	return m, nil
}

func (m Model) confirmDelete(tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.jobToAbort != "" {
		jobName := m.jobToAbort
		m.jobToAbort = ""
		m.mode = m.deleteReturnMode
		if m.mode == ViewPreview {
			m.updatePreviewViewport()
		}
		return m, abortJobCmd(jobName)
	}

	if m.jobToExecute != "" {
		jobName := m.jobToExecute
		m.jobToExecute = ""
		execErr := executeJobBackground(m.cfg, jobName)
		m.mode = ViewPreview
		m.updatePreviewViewport()
		if execErr != nil {
			m.showError("JOB ERROR", execErr)
		}
		return m, tea.Batch(tickJobLogCmd(), tickSyncPulseCmd())
	}

	targets := m.deleteTargetNotes
	returnMode := m.deleteReturnMode
	m.archivedSelectedMap = make(map[int]bool)
	m.deleteTargetNotes = nil
	m.mode = returnMode
	if returnMode == ViewArchived {
		return m, m.deleteNotesCmd(targets...)
	}
	for _, n := range targets {
		n.Status = model.StatusArchived
		n.Updated = m.currentDate
	}
	m.afterPreviewArchive(returnMode)
	return m, m.saveNotesCmd(targets...)
}

func (m Model) cancelDelete(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = m.deleteReturnMode
	m.deleteTargetNotes = nil
	m.jobToExecute = ""
	m.jobToAbort = ""
	return m, nil
}

func (m Model) confirmReview(tea.KeyMsg) (tea.Model, tea.Cmd) {
	item := m.currentPRItem()
	m.mode = ViewPreview
	if item == nil {
		return m, nil
	}
	queued, payload, err := m.buildPayload(item, m.reviewEvent, m.reviewBody)
	if err != nil {
		m.reviewNotice = err.Error()
		return m.refreshPreview(), nil
	}
	m.reviewNotice = "Submitting to GitHub…"
	event := m.reviewEvent
	return m.refreshPreview(), func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		return reviewSubmittedMsg{event: event, pr: queued, err: review.Submit(ctx, queued.Ref, payload)}
	}
}

func (m Model) cancelReview(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = ViewPreview
	return m.refreshPreview(), nil
}

func (m Model) confirmReviewRun(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = m.reviewRunReturnMode
	if m.reviewRunAction == reviewActionStart {
		item := m.currentPRItem()
		if item == nil || m.refFor(item).URL != m.reviewRunTarget.URL {
			return m, nil
		}
		_, next, cmd := m.startReview(item)
		return next, cmd
	}
	if err := stopReview(m.reviewRoot(), m.reviewRunTarget); err != nil {
		m.showError("REVIEW ERROR", err)
		return m, nil
	}
	m.reviewNotice = "Review stopped"
	selectedKey, selectedOccurrence := m.selectedNavKey()
	m.refreshReviewRuns()
	m.restoreSelection(selectedKey, selectedOccurrence)
	if m.mode == ViewPreview {
		if currentKey, _ := m.selectedNavKey(); currentKey != selectedKey {
			m.mode = ViewDashboard
			return m, nil
		}
		m.updatePreviewViewport()
	}
	return m, nil
}

func (m Model) cancelReviewRun(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = m.reviewRunReturnMode
	if m.mode == ViewPreview {
		m.updatePreviewViewport()
	}
	return m, nil
}

func (m Model) submitRejectComment(tea.KeyMsg) (tea.Model, tea.Cmd) {
	body := strings.TrimSpace(m.rejectInput.Value())
	if body == "" {
		m.reviewNotice = review.ErrRejectNeedsComment.Error()
		return m, nil
	}
	item := m.currentPRItem()
	if item == nil {
		m.mode = ViewPreview
		return m, nil
	}
	m.rejectInput.Blur()
	m.reviewNotice = ""
	_, next, cmd := m.beginConfirm(item, review.EventRequestChanges, body)
	return next, cmd
}

func (m Model) cancelRejectComment(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.rejectInput.Blur()
	m.mode = ViewPreview
	return m.refreshPreview(), nil
}

func (m Model) closePreview(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = ViewDashboard
	return m, nil
}

func (m Model) previewPrevious(tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.selected > 0 {
		m.selected--
		m.updateScrollOffset()
		m.resetReviewView()
		m.updatePreviewViewport()
	}
	return m, nil
}

func (m Model) previewNext(tea.KeyMsg) (tea.Model, tea.Cmd) {
	navItems := m.allNavItems()
	if len(navItems) > 0 && m.selected < len(navItems)-1 {
		m.selected++
		m.updateScrollOffset()
		m.resetReviewView()
		m.updatePreviewViewport()
	}
	return m, nil
}

func (m Model) copyPreviewItem(tea.KeyMsg) (tea.Model, tea.Cmd) {
	item, ok := m.selectedNavItem()
	if !ok {
		return m, nil
	}
	var textToCopy string
	if item.Note != nil {
		textToCopy = fmt.Sprintf("%s\n\n%s", item.Note.Summary, item.Note.Body)
	} else if item.Draft != nil {
		if logText, _ := latestJobOutput(item.Draft.Name, m.jobDryRunOutputFor(item.Draft.Name)); logText != "" {
			textToCopy = logText
		} else {
			textToCopy = item.Draft.Name
		}
	} else if item.PendingGitPR != nil {
		textToCopy = fmt.Sprintf("%s\n%s", item.PendingGitPR.Title, item.PendingGitPR.URL)
	} else if item.GitRepo != nil {
		textToCopy = item.GitRepo.Name
	}
	_ = copyToClipboard(strings.TrimSpace(textToCopy))
	return m, nil
}

func (m Model) previewStop(tea.KeyMsg) (tea.Model, tea.Cmd) {
	item, ok := m.selectedNavItem()
	if !ok {
		return m, nil
	}
	if item.Kind == KindReviewRun && item.ReviewRun != nil && item.ReviewRun.Status == review.RunRunning {
		return m.beginReviewRunConfirm(reviewActionStop, item.ReviewRun.Meta.Ref)
	}
	if item.Kind == KindJobDraft && item.Draft != nil && isJobRunning(item.Draft.Name) {
		m.jobToAbort = item.Draft.Name
		m.deleteTargetNotes = nil
		m.deleteReturnMode = ViewPreview
		m.mode = ViewDeleteConfirm
		return m, nil
	}
	if item.Note != nil {
		m.beginNoteDelete(item.Note, ViewPreview)
	}
	return m, nil
}

func (m Model) previewEnter(tea.KeyMsg) (tea.Model, tea.Cmd) {
	item, ok := m.selectedNavItem()
	if !ok {
		return m, nil
	}
	if item.Kind == KindJobDraft && item.Draft != nil && !isJobRunning(item.Draft.Name) {
		m.jobToExecute = item.Draft.Name
		m.deleteTargetNotes = nil
		m.deleteReturnMode = ViewPreview
		m.mode = ViewDeleteConfirm
		return m, nil
	}
	if prURL := prReviewNoteURL(item.Note); prURL != "" {
		_ = openURL(prURL)
		return m, nil
	}
	return m, nil
}

func (m Model) switchPreviewTab(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.previewTab = (m.previewTab + 1) % 2
	m.previewViewport = viewport.New(0, 0)
	return m.refreshPreview(), nil
}

func (m Model) startReviewFromKey(tea.KeyMsg) (tea.Model, tea.Cmd) {
	item := m.currentPRItem()
	if item == nil {
		return m, nil
	}
	return m.beginReviewRunConfirm(reviewActionStart, m.refFor(item))
}

func (m Model) toggleFinding(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.reviewSelected[m.reviewCursor] = !m.reviewSelected[m.reviewCursor]
	return m.refreshPreview(), nil
}

func (m Model) selectAllFindings(tea.KeyMsg) (tea.Model, tea.Cmd) {
	item := m.currentPRItem()
	if item == nil {
		return m, nil
	}
	_, findings := m.loadFindings(item)
	selectAll := m.selectedCount() < len(findings)
	m.reviewSelected = make(map[int]bool)
	if selectAll {
		for i := range findings {
			m.reviewSelected[i] = true
		}
	}
	return m.refreshPreview(), nil
}

func (m Model) postReview(tea.KeyMsg) (tea.Model, tea.Cmd) {
	item := m.currentPRItem()
	if item == nil || m.selectedCount() == 0 {
		return m, nil
	}
	_, next, cmd := m.beginConfirm(item, review.EventComment, "")
	return next, cmd
}

func (m Model) approvePR(tea.KeyMsg) (tea.Model, tea.Cmd) {
	item := m.currentPRItem()
	if item == nil {
		return m, nil
	}
	_, next, cmd := m.beginConfirm(item, review.EventApprove, "")
	return next, cmd
}

func (m Model) rejectOrStopReview(tea.KeyMsg) (tea.Model, tea.Cmd) {
	item := m.currentPRItem()
	if item == nil {
		return m, nil
	}
	if _, running := m.reviewPIDFor(item); running {
		return m.beginReviewRunConfirm(reviewActionStop, m.refFor(item))
	}
	if m.selectedCount() > 0 {
		_, next, cmd := m.beginConfirm(item, review.EventRequestChanges, "")
		return next, cmd
	}
	m.rejectInput.Reset()
	m.rejectInput.Focus()
	m.reviewNotice = ""
	m.mode = ViewRejectComment
	return m, textarea.Blink
}

func (m Model) openCloneFromKey(tea.KeyMsg) (tea.Model, tea.Cmd) {
	item := m.currentPRItem()
	if item == nil {
		return m, nil
	}
	_, next, cmd := m.openClone(item)
	return next, cmd
}

func (m Model) findingDown(tea.KeyMsg) (tea.Model, tea.Cmd) {
	item := m.currentPRItem()
	if item == nil {
		return m, nil
	}
	_, findings := m.loadFindings(item)
	if m.reviewCursor < len(findings)-1 {
		m.reviewCursor++
	}
	return m.refreshPreview(), nil
}

func (m Model) findingUp(tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.reviewCursor > 0 {
		m.reviewCursor--
	}
	return m.refreshPreview(), nil
}

func (m Model) ignoreKey(tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		m.jobToExecute = ""
		m.jobToAbort = ""
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

	m.mode = m.returnFromEdit()
	return m, m.saveNotesCmd(m.currentNote)
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
	if returnMode == ViewSearchPreview {
		m.updateSearchPreviewViewport()
	}
	return returnMode
}

func (m Model) exportSearch(tea.KeyMsg) (tea.Model, tea.Cmd) {
	results := m.searchResults()
	if len(results) == 0 {
		m.searchNotice = nothingToExportNotice
		return m, nil
	}
	return m, m.exportSearchCmd(results)
}

func (m Model) closeSearch(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.searchNotice = ""
	m.searchInput.Blur()
	m.mode = ViewDashboard
	return m, nil
}

func (m Model) searchCursorUp(tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.moveSearchSelection(-1), nil
}

func (m Model) searchCursorDown(tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.moveSearchSelection(1), nil
}

func (m Model) searchPageUp(tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.moveSearchSelection(-m.searchPageSize()), nil
}

func (m Model) searchPageDown(tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.moveSearchSelection(m.searchPageSize()), nil
}

func (m Model) searchHalfPageUp(tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.moveSearchSelection(-max(1, m.searchPageSize()/2)), nil
}

func (m Model) searchHalfPageDown(tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.moveSearchSelection(max(1, m.searchPageSize()/2)), nil
}

func (m Model) openSearchPreview(tea.KeyMsg) (tea.Model, tea.Cmd) {
	if len(m.searchResults()) == 0 {
		return m, nil
	}
	m.searchInput.Blur()
	m.mode = ViewSearchPreview
	m.showSearchPreviewAt(m.searchSelected)
	return m, nil
}

func (m Model) closeSearchPreview(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = ViewSearch
	m.keepSearchSelectionVisible()
	m.searchInput.Focus()
	return m, textinput.Blink
}

func (m Model) searchPreviewNext(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.showSearchPreviewAt(m.searchPreviewIndex(m.searchResults()) + 1)
	return m, nil
}

func (m Model) searchPreviewPrevious(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.showSearchPreviewAt(m.searchPreviewIndex(m.searchResults()) - 1)
	return m, nil
}

func (m Model) editSearchResult(tea.KeyMsg) (tea.Model, tea.Cmd) {
	note := m.searchPreviewNote()
	if note == nil {
		return m, nil
	}
	m.currentNote = note
	m.editReturnMode = ViewSearchPreview
	m.mode = ViewEdit
	m.editor.SetValue(fmt.Sprintf("%s\n\n%s", note.Summary, note.Body))
	m.editor.Focus()
	return m, textarea.Blink
}

func (m Model) deleteSearchResult(tea.KeyMsg) (tea.Model, tea.Cmd) {
	if note := m.searchPreviewNote(); note != nil {
		m.searchSelected = m.searchPreviewIndex(m.searchResults())
		m.beginNoteDelete(note, ViewSearchPreview)
	}
	return m, nil
}

func (m Model) copySearchResult(tea.KeyMsg) (tea.Model, tea.Cmd) {
	if note := m.searchPreviewNote(); note != nil {
		_ = copyToClipboard(fmt.Sprintf("%s\n\n%s", note.Summary, note.Body))
	}
	return m, nil
}

func (m Model) dismissError(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = m.errorReturnMode
	m.errorTitle = ""
	m.errorLines = nil
	if m.mode == ViewArchived {
		m.refreshArchivedViewport()
	}
	return m, nil
}
