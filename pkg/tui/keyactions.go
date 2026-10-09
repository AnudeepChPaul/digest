package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/model"
	"github.com/AnudeepChPaul/digest/pkg/review"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) clampScreenSelection() {
	switch m.mode {
	case ViewDashboard:
		navItems := m.allNavItems()
		if m.selected >= len(navItems) && len(navItems) > 0 {
			m.selected = len(navItems) - 1
		}
	case ViewGitDetails:
		gitItems := m.filteredGitItems()
		if m.git.gitPopupSelected >= len(gitItems) && len(gitItems) > 0 {
			m.git.gitPopupSelected = len(gitItems) - 1
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
		*m.inlineInput, cmd = m.inlineInput.Update(msg)
		return m, cmd
	case ViewEdit, ViewBragEdit, ViewAutomationEdit:
		*m.editor, cmd = m.editor.Update(msg)
		m.editorRevision++
		return m, cmd
	case ViewBragView:
		scrollViewport(&m.previewViewport, msg.String())
		return m, nil
	case ViewSearch:
		previousQuery := m.searchInput.Value()
		*m.searchInput, cmd = m.searchInput.Update(msg)
		if m.searchInput.Value() != previousQuery {
			m.searchSelected, m.searchScroll = 0, 0
			m.searchNotice = ""
		}
		return m, cmd
	case ViewSearchPreview:
		scrollViewport(&m.previewViewport, msg.String())
		return m, nil
	case ViewRejectComment:
		*m.rejectInput, cmd = m.rejectInput.Update(msg)
		return m, cmd
	case ViewSetup:
		if m.setup.saving {
			return m, nil
		}
		*m.setupInput, cmd = m.setupInput.Update(msg)
		m.setup.notice = ""
		return m, cmd
	case ViewNotifyInput:
		if !notifyInputAccepts(m.notifyInput.Value(), msg) {
			return m, nil
		}
		*m.notifyInput, cmd = m.notifyInput.Update(msg)
		m.notifyNotice = ""
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

func (m Model) countCtrlCToQuit(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.ctrlCCount++
	if m.ctrlCCount >= 3 {
		m.cancelGitSync()
		if m.cancelSession != nil {
			m.cancelSession()
		}
		return m, tea.Quit
	}
	return m, tickCtrlCResetCmd()
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
	if !isSameDay(m.currentDate, time.Now()) {
		m.resetCommitsForDate()
	}
	m.currentDate = time.Now()
	m.selected = 0
	m.scrollOffset = 0
	return m, tea.Batch(m.scheduleDaySync(), m.reloadNotesForDay())
}

func (m Model) openSelectedPreview(tea.KeyMsg) (tea.Model, tea.Cmd) {
	if item, ok := m.selectedNavItem(); ok {
		return m.openPreview(item)
	}
	return m, nil
}

func (m Model) openSelectedItem(tea.KeyMsg) (tea.Model, tea.Cmd) {
	item, ok := m.selectedNavItem()
	if !ok {
		return m, nil
	}
	if item.Kind == KindJobDraft || item.Kind == KindReviewRun || item.Kind == KindBragRun || item.Kind == KindAutomationRun {
		return m.openPreview(item)
	}
	if item.Kind == KindPendingGit && item.PendingGitPR != nil {
		_ = openURL(item.PendingGitPR.URL)
		return m, nil
	}
	if item.Kind == KindMyPR && item.MyPR != nil {
		_ = openURL(item.MyPR.Ref.URL)
		return m, nil
	}
	if item.Note != nil {
		previewed, _ := m.openPreview(item)
		return previewed.(Model).beginNoteEdit(item.Note, ViewPreview)
	}
	return m, nil
}

func (m Model) deleteSelectedItem(tea.KeyMsg) (tea.Model, tea.Cmd) {
	item, ok := m.selectedNavItem()
	if !ok {
		return m, nil
	}
	if item.Note != nil {
		m.beginNoteDelete(item.Note, ViewDashboard)
	}
	return m, nil
}

func (m Model) previousDay(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.currentDate = m.currentDate.AddDate(0, 0, -1)
	m.resetCommitsForDate()
	m.selected = 0
	m.scrollOffset = 0
	return m, tea.Batch(m.scheduleDaySync(), m.reloadNotesForDay())
}

func (m Model) nextDay(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.currentDate = m.currentDate.AddDate(0, 0, 1)
	m.resetCommitsForDate()
	m.selected = 0
	m.scrollOffset = 0
	return m, tea.Batch(m.scheduleDaySync(), m.reloadNotesForDay())
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
	return m, tea.Batch(textinput.Blink, m.ensureAllNotes())
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
		m.mode = m.deleteReturnMode
		if m.mode == ViewPreview {
			m.updatePreviewViewport()
		}
		if execErr != nil {
			m.showError("JOB ERROR", execErr)
		}
		m.refreshJobStates()
		return m, tea.Batch(m.ensureJobLogRefresh(), m.ensureSyncPulse(), m.ensureRunStatePoll())
	}

	if m.bragRunToStop != nil || m.automationRunToStop != nil {
		return m.confirmStopRun()
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
	if returnMode == ViewPreview || returnMode == ViewSearchPreview {
		return m, m.saveNotesFromPreviewCmd(targets...)
	}
	return m, m.saveNotesCmd(targets...)
}

func (m Model) cancelDelete(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = m.deleteReturnMode
	m.deleteTargetNotes = nil
	m.clearPendingConfirms()
	return m, nil
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
	return m, m.changesSinceReviewCmd(m.currentPRItem())
}

func (m Model) previewNext(tea.KeyMsg) (tea.Model, tea.Cmd) {
	navItems := m.allNavItems()
	if len(navItems) > 0 && m.selected < len(navItems)-1 {
		m.selected++
		m.updateScrollOffset()
		m.resetReviewView()
		m.updatePreviewViewport()
	}
	return m, m.changesSinceReviewCmd(m.currentPRItem())
}

func (m Model) copyPreviewItem(tea.KeyMsg) (tea.Model, tea.Cmd) {
	item, ok := m.selectedNavItem()
	if !ok {
		return m, nil
	}
	var textToCopy string
	if m.onDraftTab() {
		textToCopy = m.draftText()
	} else if item.Note != nil {
		textToCopy = fmt.Sprintf("%s\n\n%s", item.Note.Summary, item.Note.Body)
	} else if item.Draft != nil {
		if logText, _ := latestJobOutput(item.Draft.Name, m.jobDryRunOutputFor(item.Draft.Name)); logText != "" {
			textToCopy = logText
		} else {
			textToCopy = item.Draft.Name
		}
	} else if item.PendingGitPR != nil {
		_, findings := m.loadFindings(item.PendingGitPR)
		textToCopy = reviewCopyText(item.PendingGitPR, findings)
	} else if item.GitRepo != nil {
		textToCopy = item.GitRepo.Name
	}
	_ = copyToClipboard(strings.TrimSpace(textToCopy))
	return m, nil
}

func reviewCopyText(item *GitPRItem, findings []review.Finding) string {
	var text strings.Builder
	fmt.Fprintf(&text, "%s\n%s", item.Title, item.URL)
	if len(findings) > 0 {
		text.WriteString("\n\n## Review findings\n")
	}
	for _, group := range review.GroupBySeverity(findings) {
		fmt.Fprintf(&text, "\n### %s (%d)\n\n", strings.ToUpper(group.Severity), len(group.Findings))
		for _, finding := range group.Findings {
			fmt.Fprintf(&text, "- **%s**", finding.Title)
			if finding.Path != "" {
				fmt.Fprintf(&text, " — `%s:%d`", finding.Path, finding.Line)
			}
			text.WriteString("\n")
			if finding.Body != "" {
				text.WriteString(indent(strings.TrimSpace(finding.Body), "  ") + "\n")
			}
			if finding.Suggestion != "" {
				text.WriteString(indent("Suggestion: "+strings.TrimSpace(finding.Suggestion), "  ") + "\n")
			}
		}
	}
	return strings.TrimSpace(text.String())
}

func (m Model) previewStop(tea.KeyMsg) (tea.Model, tea.Cmd) {
	item, ok := m.selectedNavItem()
	if !ok {
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
	if item.Kind == KindJobDraft {
		return m.runSelectedJob(tea.KeyMsg{})
	}
	if item.Kind == KindPendingGit && item.PendingGitPR != nil {
		_ = openURL(item.PendingGitPR.URL)
		return m, nil
	}
	if item.Kind == KindMyPR && item.MyPR != nil {
		_ = openURL(item.MyPR.Ref.URL)
		return m, nil
	}
	if item.Note != nil {
		return m.beginNoteEdit(item.Note, ViewPreview)
	}
	return m, nil
}

func (m Model) switchPreviewTab(tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case m.currentPRItem() != nil:
		m.previewTab = (m.previewTab + 1) % 2
	case m.previewTab == previewTabDraft:
		m.previewTab = previewTabDetails
	default:
		m.previewTab = previewTabDraft
	}
	m.previewViewport = viewport.New(0, 0)
	return m.refreshPreview(), nil
}

func (m Model) ignoreKey(tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m, nil
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
	return m.beginNoteEdit(note, ViewSearchPreview)
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
