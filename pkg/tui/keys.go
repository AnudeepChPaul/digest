package tui

import (
	"fmt"
	"slices"

	"github.com/charmbracelet/bubbles/key"
)

type keyAction int

const (
	actionNone keyAction = iota
	actionQuit
	actionDismissSyncErrors
	actionSync
	actionToggleSortField
	actionToggleSortOrder
	actionTogglePendingScope
	actionRunSelectedJob
	actionDryRunSelectedJob
	actionStopSelectedItem
	actionHalfPageDown
	actionHalfPageUp
	actionToday
	actionOpenArchive
	actionOpenPreview
	actionNewNote
	actionOpenItem
	actionInlineEdit
	actionDeleteItem
	actionToggleDone
	actionPreviousDay
	actionNextDay
	actionCursorDown
	actionCursorUp
	actionOpenSearch
	actionRefreshCommits
	actionOpenBrag
	actionOpenHelp
	actionSwitchGitColumn

	actionCloseHelp

	actionAutomate
	actionRunAutomationDraft
	actionConfirmAutomation
	actionCancelAutomation
	actionEditAutomation
	actionDeleteAutomationDraft
	actionSaveAutomationEdit
	actionCancelAutomationEdit

	actionOpenActions
	actionChooseAction
	actionActionMenuDown
	actionActionMenuUp
	actionCloseActionMenu
	actionConfirmNotify
	actionDashboardApprove
	actionSetupYes
	actionSetupNo
	actionSetupConfirm
	actionSetupSkip
	actionSetupSwitchTime
	actionSetupDayLeft
	actionSetupDayRight
	actionSetupToggleDay
	actionCloseSetup
	actionSetupFieldNext
	actionSetupFieldPrevious
	actionSetupFormToggle
	actionSetupFormSave
	actionSetupFormTab
	actionSetupFormEscape
	actionKeepEditingSetup
	actionDashboardReject
	actionCancelNotify
	actionOpenSetup
	actionOpenMessages
	actionEditorHalfPageUp
	actionEditorHalfPageDown

	actionCloseBrag
	actionBragCursorDown
	actionBragCursorUp
	actionBragListEnter
	actionCloseBragView
	actionEditBrag
	actionBragAgain
	actionCopyBrag
	actionConfirmBrag
	actionCancelBrag
	actionSaveBragEdit
	actionCopyBragEditor
	actionCancelBragEdit

	actionCloseGitDetails
	actionSwitchGitFilter
	actionGitCursorDown
	actionGitCursorUp
	actionOpenGitItem

	actionConfirmDelete
	actionCancelDelete

	actionConfirmReview
	actionCancelReview

	actionConfirmReviewRun
	actionCancelReviewRun

	actionSubmitRejectComment
	actionCancelRejectComment

	actionClosePreview
	actionPreviewPrevious
	actionPreviewNext
	actionCopyPreviewItem
	actionPreviewStop
	actionPreviewEnter
	actionOpenNotePR

	actionSwitchPreviewTab
	actionStartReview
	actionToggleFinding
	actionSelectAllFindings
	actionPostReview
	actionApprove
	actionRejectOrStopReview
	actionOpenClone
	actionFindingDown
	actionFindingUp
	actionIgnoreKey

	actionCloseArchive
	actionArchiveCursorDown
	actionArchiveCursorUp
	actionToggleArchiveSelection
	actionRestoreArchived
	actionDeleteArchived

	actionCancelInlineEdit
	actionSaveInlineEdit

	actionSaveNote
	actionCopyEditor
	actionCancelEdit

	actionCloseSearch
	actionSearchCursorUp
	actionSearchCursorDown
	actionSearchPageUp
	actionSearchPageDown
	actionSearchHalfPageUp
	actionSearchHalfPageDown
	actionOpenSearchPreview
	actionExportSearch

	actionCloseSearchPreview
	actionSearchPreviewNext
	actionSearchPreviewPrevious
	actionEditSearchResult
	actionCopySearchResult
	actionDeleteSearchResult

	actionDismissError
)

type keyBinding struct {
	binding key.Binding
	action  keyAction
	warn    bool
	hidden  bool
}

func newKeyBinding(action keyAction, keys []string, helpKey, helpText string) keyBinding {
	return keyBinding{
		binding: key.NewBinding(key.WithKeys(keys...), key.WithHelp(helpKey, helpText)),
		action:  action,
		hidden:  helpKey == "",
	}
}

func hiddenKeyBinding(action keyAction, keys ...string) keyBinding {
	return newKeyBinding(action, keys, "", "")
}

func (b keyBinding) warning() keyBinding {
	b.warn = true
	return b
}

func (b keyBinding) shownWhen(visible bool) keyBinding {
	b.hidden = b.hidden || !visible
	return b
}

func footerItemsFrom(bindings []keyBinding) []footerItem {
	var items []footerItem
	for _, binding := range bindings {
		if binding.hidden || !binding.binding.Enabled() {
			continue
		}
		help := binding.binding.Help()
		items = append(items, footerItem{help.Key, help.Desc, binding.warn})
	}
	return items
}

func (m Model) activeBindings() []keyBinding {
	switch m.mode {
	case ViewDashboard:
		return m.dashboardBindings()
	case ViewGitDetails:
		return gitDetailsBindings()
	case ViewDeleteConfirm:
		return deleteConfirmBindings()
	case ViewReviewConfirm:
		return reviewConfirmBindings()
	case ViewReviewRunConfirm:
		return reviewRunConfirmBindings()
	case ViewRejectComment:
		return rejectCommentBindings()
	case ViewPreview:
		return m.previewBindings()
	case ViewArchived:
		return archivedBindings()
	case ViewInlineEdit:
		return inlineEditBindings()
	case ViewEdit:
		return editBindings()
	case ViewSearch:
		return searchBindings()
	case ViewSearchPreview:
		return searchPreviewBindings()
	case ViewError:
		return errorBindings()
	case ViewHelp:
		return helpBindings()
	case ViewBragList:
		return bragListBindings()
	case ViewBragView:
		return bragViewBindings()
	case ViewBragConfirm:
		return bragConfirmBindings()
	case ViewBragEdit:
		return bragEditBindings()
	case ViewAutomationConfirm:
		return automationConfirmBindings()
	case ViewAutomationEdit:
		return automationEditBindings()
	case ViewActionMenu:
		return actionMenuBindings()
	case ViewNotifyInput:
		return notifyInputBindings()
	case ViewSetup:
		return m.setupKeyBindings()
	case ViewSetupDiscard:
		return setupDiscardBindings()
	}
	return nil
}

func (m Model) dashboardBindings() []keyBinding {
	quitLabel := "quit"
	if m.ctrlCCount > 0 {
		quitLabel = fmt.Sprintf("%d more", 3-m.ctrlCCount)
	}
	quit := newKeyBinding(actionQuit, []string{"ctrl+c"}, "ctrl+c", quitLabel)
	if m.ctrlCCount > 0 {
		quit = quit.warning()
	}
	bindings := append(hiddenCopies(m.selectedItemBindings(true)),
		newKeyBinding(actionOpenItem, []string{"enter"}, "↵", "open"),
		newKeyBinding(actionToggleDone, []string{" ", "space"}, "space", "done"),
		newKeyBinding(actionNewNote, []string{"a"}, "a", "new"),
		newKeyBinding(actionInlineEdit, []string{"i"}, "i", "inline"),
		hiddenKeyBinding(actionOpenActions, "@", "."),
		hiddenKeyBinding(actionOpenSetup, ","),
		hiddenKeyBinding(actionOpenMessages, "!"),
		newKeyBinding(actionOpenPreview, []string{"tab"}, "tab", "preview"),
		newKeyBinding(actionToday, []string{"t"}, "t", "today"),
		newKeyBinding(actionPreviousDay, []string{"p"}, "p|n", "date"),
		hiddenKeyBinding(actionNextDay, "n"),
		newKeyBinding(actionOpenArchive, []string{"ctrl+e"}, "ctrl+e", "open archive"),
		newKeyBinding(actionSync, []string{"g"}, "g", "run git"),
		newKeyBinding(actionRefreshCommits, []string{"c"}, "c", "refresh commits").shownWhen(m.cfg.DailyCommitsEnabled()),
		newKeyBinding(actionRunSelectedJob, []string{"r"}, "r", "run job"),
		newKeyBinding(actionDeleteItem, []string{"d"}, "d", "delete"),
		newKeyBinding(actionCursorDown, []string{"j", "down"}, "j|k", "nav"),
		hiddenKeyBinding(actionCursorUp, "k", "up"),
		newKeyBinding(actionSwitchGitColumn, []string{"h", "l", "left", "right"}, "h|l|←|→", "git column"),
		newKeyBinding(actionHalfPageDown, []string{"ctrl+d", "pgdown"}, "ctrl+d|u", "half page"),
		hiddenKeyBinding(actionHalfPageUp, "ctrl+u", "pgup"),
		newKeyBinding(actionOpenSearch, []string{"/"}, "/", "search"),
		newKeyBinding(actionOpenBrag, []string{"b"}, "b", "brag"),
		newKeyBinding(actionOpenHelp, []string{"?"}, "?", "shortcuts"),
		quit,
		hiddenKeyBinding(actionToggleSortField, "s"),
		hiddenKeyBinding(actionToggleSortOrder, "w"),
		hiddenKeyBinding(actionTogglePendingScope, "m"),
		hiddenKeyBinding(actionDismissSyncErrors, "esc"),
	)
	if m.cfg.GitEnabled() {
		return bindings
	}
	return slices.DeleteFunc(bindings, func(binding keyBinding) bool { return slices.Contains(gitActions, binding.action) })
}

var gitActions = []keyAction{actionSync, actionRefreshCommits, actionSwitchGitColumn, actionToggleSortField, actionToggleSortOrder, actionTogglePendingScope}

func gitDetailsBindings() []keyBinding {
	return []keyBinding{
		newKeyBinding(actionSwitchGitFilter, []string{"tab"}, "tab", "switch filter"),
		newKeyBinding(actionOpenGitItem, []string{"enter"}, "enter", "open browser"),
		newKeyBinding(actionCloseGitDetails, []string{"esc"}, "esc", "close"),
		hiddenKeyBinding(actionGitCursorDown, "j", "down"),
		hiddenKeyBinding(actionGitCursorUp, "k", "up"),
	}
}

func deleteConfirmBindings() []keyBinding {
	return []keyBinding{
		newKeyBinding(actionConfirmDelete, []string{"y", "Y", "enter"}, "y|enter", "confirm"),
		newKeyBinding(actionCancelDelete, []string{"esc"}, "esc", "cancel"),
	}
}

func reviewConfirmBindings() []keyBinding {
	return []keyBinding{
		newKeyBinding(actionConfirmReview, []string{"y", "Y", "enter"}, "y|enter", "confirm"),
		newKeyBinding(actionCancelReview, []string{"esc", "n", "N"}, "esc", "cancel"),
	}
}

func reviewRunConfirmBindings() []keyBinding {
	return []keyBinding{
		newKeyBinding(actionConfirmReviewRun, []string{"y", "Y", "enter"}, "y|enter", "confirm"),
		newKeyBinding(actionCancelReviewRun, []string{"esc", "n", "N"}, "esc", "cancel"),
	}
}

func rejectCommentBindings() []keyBinding {
	return append([]keyBinding{
		newKeyBinding(actionSubmitRejectComment, []string{"ctrl+s"}, "ctrl+s", "continue"),
		newKeyBinding(actionCancelRejectComment, []string{"esc"}, "esc", "cancel"),
	}, editorHalfPageBindings()...)
}

func (m Model) previewBindings() []keyBinding {
	item, found := m.selectedNavItem()
	if !found {
		return previewTailBindings()
	}
	switch {
	case item.Kind == KindPendingGit && item.PendingGitPR != nil:
		return m.prPreviewBindings(item.PendingGitPR)
	case item.Kind == KindReviewRun && item.ReviewRun != nil, item.Kind == KindBragRun && item.BragRun != nil, item.Kind == KindAutomationRun && item.AutomationRun != nil:
		return m.runPreviewBindings(item)
	case item.Kind == KindMyPR && item.MyPR != nil:
		return append([]keyBinding{
			newKeyBinding(actionPreviewEnter, []string{"enter"}, "enter", "open PR"),
		}, previewTailBindings()...)
	case item.Kind == KindJobDraft && item.Draft != nil:
		return append(append(m.itemBindings(item, false), hiddenKeyBinding(actionPreviewEnter, "enter")), previewTailBindings()...)
	}
	if m.onDraftTab() {
		return m.noteDraftTabBindings()
	}
	var draftTab []keyBinding
	tail := previewTailBindings()
	if m.hasDraftTab() {
		draftTab = []keyBinding{newKeyBinding(actionSwitchPreviewTab, []string{"tab"}, "tab", "tabs")}
		tail[0] = newKeyBinding(actionClosePreview, []string{"esc"}, "esc", "close")
	}
	bindings := append(draftTab,
		newKeyBinding(actionPreviewEnter, []string{"enter"}, "enter", "edit").shownWhen(item.Note != nil),
		newKeyBinding(actionOpenNotePR, []string{"o"}, "o", "open PR").shownWhen(prReviewNoteURL(item.Note) != ""),
	)
	bindings = append(bindings, m.itemBindings(item, false)...)
	bindings = append(bindings, newKeyBinding(actionPreviewStop, []string{"d"}, "d", "delete").warning().shownWhen(item.Note != nil && item.Note.FilePath != ""))
	return append(bindings, tail...)
}

func previewTailBindings() []keyBinding {
	return []keyBinding{
		newKeyBinding(actionClosePreview, []string{"esc", "tab"}, "esc|tab", "close"),
		newKeyBinding(actionPreviewPrevious, []string{"p"}, "p|n", "prev/next"),
		hiddenKeyBinding(actionPreviewNext, "n"),
		newKeyBinding(actionCopyPreviewItem, []string{"ctrl+y"}, "ctrl+y", "copy"),
	}
}

func (m Model) runPreviewBindings(item NavItem) []keyBinding {
	return append(append(m.itemBindings(item, false), hiddenKeyBinding(actionPreviewEnter, "enter")), previewTailBindings()...)
}

func (m Model) prPreviewBindings(item *GitPRItem) []keyBinding {
	onReviewTab := m.previewTab == previewTabReview
	hasFindings := onReviewTab && m.previewFindingsCount > 0
	bindings := append([]keyBinding{
		newKeyBinding(actionSwitchPreviewTab, []string{"tab"}, "tab", "tabs"),
	}, m.itemBindings(NavItem{Kind: KindPendingGit, PendingGitPR: item}, false)...)
	if hasFindings {
		bindings = append(bindings,
			newKeyBinding(actionToggleFinding, []string{" ", "space"}, "space", "select"),
			newKeyBinding(actionSelectAllFindings, []string{"ctrl+a"}, "ctrl+a", "all"),
		)
	}
	if onReviewTab && m.selectedCount() > 0 {
		bindings = append(bindings, newKeyBinding(actionPostReview, []string{"enter"}, "enter", "post review"))
	} else {
		bindings = append(bindings, newKeyBinding(actionPreviewEnter, []string{"enter"}, "enter", "open PR"))
	}
	bindings = append(bindings,
		hiddenKeyBinding(actionApprove, "a"),
		newKeyBinding(actionPreviewPrevious, []string{"p"}, "p|n", "prev/next"),
		hiddenKeyBinding(actionPreviewNext, "n"),
		newKeyBinding(actionCopyPreviewItem, []string{"ctrl+y"}, "ctrl+y", "copy"),
		newKeyBinding(actionClosePreview, []string{"esc"}, "esc", "close"),
		hiddenKeyBinding(actionIgnoreKey, "h", "l", "left", "right"),
	)
	if hasFindings {
		bindings = append(bindings,
			hiddenKeyBinding(actionFindingDown, "j", "down"),
			hiddenKeyBinding(actionFindingUp, "k", "up"),
		)
	}
	return bindings
}

func archivedBindings() []keyBinding {
	return []keyBinding{
		newKeyBinding(actionCloseArchive, []string{"esc", "ctrl+e"}, "esc|ctrl+e", "close"),
		newKeyBinding(actionArchiveCursorDown, []string{"j", "down"}, "j|k", "nav"),
		hiddenKeyBinding(actionArchiveCursorUp, "k", "up"),
		newKeyBinding(actionToggleArchiveSelection, []string{" ", "space"}, "space", "select"),
		newKeyBinding(actionRestoreArchived, []string{"u", "enter"}, "u|enter", "unarchive"),
		newKeyBinding(actionDeleteArchived, []string{"d"}, "d", "delete"),
	}
}

func inlineEditBindings() []keyBinding {
	return []keyBinding{
		hiddenKeyBinding(actionCancelInlineEdit, "esc"),
		hiddenKeyBinding(actionSaveInlineEdit, "enter"),
	}
}

func editBindings() []keyBinding {
	return append([]keyBinding{
		newKeyBinding(actionSaveNote, []string{"ctrl+o"}, "ctrl+o", "save"),
		newKeyBinding(actionCopyEditor, []string{"ctrl+y"}, "ctrl+y", "copy"),
		newKeyBinding(actionCancelEdit, []string{"esc"}, "esc", "cancel"),
	}, editorHalfPageBindings()...)
}

func searchBindings() []keyBinding {
	return []keyBinding{
		newKeyBinding(actionSearchCursorUp, []string{"up"}, "↑|↓", "nav"),
		hiddenKeyBinding(actionSearchCursorDown, "down"),
		newKeyBinding(actionSearchPageUp, []string{"pgup"}, "pgup|pgdn", "page"),
		hiddenKeyBinding(actionSearchPageDown, "pgdown"),
		newKeyBinding(actionSearchHalfPageDown, []string{"ctrl+d"}, "ctrl+d|u", "half page"),
		hiddenKeyBinding(actionSearchHalfPageUp, "ctrl+u"),
		newKeyBinding(actionOpenSearchPreview, []string{"tab"}, "tab", "preview"),
		newKeyBinding(actionExportSearch, []string{"ctrl+e"}, "ctrl+e", "export"),
		newKeyBinding(actionCloseSearch, []string{"esc"}, "esc", "close"),
	}
}

func searchPreviewBindings() []keyBinding {
	return []keyBinding{
		newKeyBinding(actionSearchPreviewNext, []string{"n"}, "n|p", "next/prev"),
		hiddenKeyBinding(actionSearchPreviewPrevious, "p"),
		newKeyBinding(actionEditSearchResult, []string{"enter"}, "enter", "edit"),
		newKeyBinding(actionCopySearchResult, []string{"ctrl+y"}, "ctrl+y", "copy"),
		newKeyBinding(actionDeleteSearchResult, []string{"d"}, "d", "delete").warning(),
		newKeyBinding(actionCloseSearchPreview, []string{"esc", "tab"}, "esc|tab", "back"),
	}
}

func helpBindings() []keyBinding {
	return []keyBinding{
		newKeyBinding(actionCloseHelp, []string{"esc", "?", "q"}, "esc|?", "close"),
	}
}

func bragListBindings() []keyBinding {
	return []keyBinding{
		newKeyBinding(actionBragListEnter, []string{"enter"}, "enter", "open"),
		newKeyBinding(actionBragCursorDown, []string{"j", "down"}, "j|k", "nav"),
		hiddenKeyBinding(actionBragCursorUp, "k", "up"),
		newKeyBinding(actionCloseBrag, []string{"esc"}, "esc", "close"),
	}
}

func bragViewBindings() []keyBinding {
	return []keyBinding{
		newKeyBinding(actionEditBrag, []string{"enter"}, "enter", "edit"),
		newKeyBinding(actionBragAgain, []string{"b"}, "b", "brag again"),
		newKeyBinding(actionCopyBrag, []string{"ctrl+y"}, "ctrl+y", "copy"),
		newKeyBinding(actionCloseBragView, []string{"esc"}, "esc", "back"),
	}
}

func bragConfirmBindings() []keyBinding {
	return []keyBinding{
		newKeyBinding(actionConfirmBrag, []string{"y", "Y", "enter"}, "y|enter", "confirm"),
		newKeyBinding(actionCancelBrag, []string{"esc", "n", "N"}, "esc", "cancel"),
	}
}

func bragEditBindings() []keyBinding {
	return append([]keyBinding{
		newKeyBinding(actionSaveBragEdit, []string{"ctrl+o"}, "ctrl+o", "save"),
		newKeyBinding(actionCopyBragEditor, []string{"ctrl+y"}, "ctrl+y", "copy"),
		newKeyBinding(actionCancelBragEdit, []string{"esc"}, "esc", "cancel"),
	}, editorHalfPageBindings()...)
}

func errorBindings() []keyBinding {
	return []keyBinding{
		newKeyBinding(actionDismissError, []string{"esc"}, "esc", "dismiss"),
	}
}

func editorHalfPageBindings() []keyBinding {
	return []keyBinding{
		newKeyBinding(actionEditorHalfPageDown, []string{"ctrl+d"}, "ctrl+d|u", "half page"),
		hiddenKeyBinding(actionEditorHalfPageUp, "ctrl+u"),
	}
}
