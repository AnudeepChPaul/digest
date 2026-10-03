package tui

import (
	"fmt"

	"app/pkg/review"

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
	actionRunJobs
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
	return []keyBinding{
		newKeyBinding(actionOpenItem, []string{"enter"}, "↵", "open"),
		newKeyBinding(actionToggleDone, []string{" ", "space"}, "space", "done"),
		newKeyBinding(actionNewNote, []string{"a"}, "a", "new"),
		newKeyBinding(actionInlineEdit, []string{"i"}, "i", "inline"),
		newKeyBinding(actionOpenPreview, []string{"tab"}, "tab", "preview"),
		newKeyBinding(actionToday, []string{"t"}, "t", "today"),
		newKeyBinding(actionPreviousDay, []string{"p", "left"}, "p|n|←|→", "date"),
		hiddenKeyBinding(actionNextDay, "n", "right"),
		newKeyBinding(actionOpenArchive, []string{"ctrl+e"}, "ctrl+e", "open archive"),
		newKeyBinding(actionSync, []string{"g"}, "g", "run git"),
		newKeyBinding(actionRunJobs, []string{"r"}, "r", "run jobs"),
		newKeyBinding(actionDeleteItem, []string{"d"}, "d", "delete"),
		newKeyBinding(actionCursorDown, []string{"j", "down"}, "j|k", "nav"),
		hiddenKeyBinding(actionCursorUp, "k", "up"),
		newKeyBinding(actionHalfPageDown, []string{"ctrl+d", "pgdown"}, "ctrl+d|u", "half page"),
		hiddenKeyBinding(actionHalfPageUp, "ctrl+u", "pgup"),
		newKeyBinding(actionOpenSearch, []string{"/"}, "/", "search"),
		quit,
		hiddenKeyBinding(actionToggleSortField, "s"),
		hiddenKeyBinding(actionToggleSortOrder, "w"),
		hiddenKeyBinding(actionDismissSyncErrors, "esc"),
	}
}

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
	return []keyBinding{
		newKeyBinding(actionSubmitRejectComment, []string{"ctrl+s"}, "ctrl+s", "continue"),
		newKeyBinding(actionCancelRejectComment, []string{"esc"}, "esc", "cancel"),
	}
}

func (m Model) previewBindings() []keyBinding {
	if prItem := m.currentPRItem(); prItem != nil {
		return m.prPreviewBindings(prItem)
	}
	navItems := m.allNavItems()
	if len(navItems) == 0 || m.selected >= len(navItems) {
		return previewTailBindings()
	}
	item := navItems[m.selected]
	switch {
	case item.Kind == KindReviewRun && item.ReviewRun != nil:
		_, running := review.RunningPID(review.StateDir(m.reviewRoot(), item.ReviewRun.Meta.Ref))
		return reviewRunPreviewBindings(running)
	case item.Kind == KindJobDraft && item.Draft != nil:
		running := isJobRunning(item.Draft.Name)
		return append([]keyBinding{
			newKeyBinding(actionPreviewStop, []string{"d"}, "d", "abort job").warning().shownWhen(running),
			newKeyBinding(actionPreviewEnter, []string{"enter"}, "enter", "run job").shownWhen(!running),
		}, previewTailBindings()...)
	}
	return append([]keyBinding{
		newKeyBinding(actionPreviewEnter, []string{"enter"}, "enter", "open PR").shownWhen(prReviewNoteURL(item.Note) != ""),
		newKeyBinding(actionPreviewStop, []string{"d"}, "d", "delete").warning().shownWhen(item.Note != nil && item.Note.FilePath != ""),
	}, previewTailBindings()...)
}

func previewTailBindings() []keyBinding {
	return []keyBinding{
		newKeyBinding(actionClosePreview, []string{"esc", "tab"}, "esc|tab", "close"),
		newKeyBinding(actionPreviewPrevious, []string{"p"}, "p|n", "prev/next"),
		hiddenKeyBinding(actionPreviewNext, "n"),
		newKeyBinding(actionCopyPreviewItem, []string{"ctrl+y"}, "ctrl+y", "copy"),
	}
}

func reviewRunPreviewBindings(running bool) []keyBinding {
	return append([]keyBinding{
		newKeyBinding(actionPreviewStop, []string{"d"}, "d", "stop review").warning().shownWhen(running),
		hiddenKeyBinding(actionPreviewEnter, "enter"),
	}, previewTailBindings()...)
}

func (m Model) prPreviewBindings(item *GitPRItem) []keyBinding {
	rejectOrStop := "reject"
	if _, running := m.reviewPIDFor(item); running {
		rejectOrStop = "stop review"
	}
	onReviewTab := m.previewTab == previewTabReview
	hasFindings := false
	if onReviewTab {
		_, findings := m.loadFindings(item)
		hasFindings = len(findings) > 0
	}
	bindings := []keyBinding{
		newKeyBinding(actionSwitchPreviewTab, []string{"tab"}, "tab", "tabs"),
		newKeyBinding(actionStartReview, []string{"r"}, "r", "review"),
	}
	if hasFindings {
		bindings = append(bindings,
			newKeyBinding(actionToggleFinding, []string{" ", "space"}, "space", "select"),
			newKeyBinding(actionSelectAllFindings, []string{"ctrl+a"}, "ctrl+a", "all"),
		)
	}
	if onReviewTab {
		bindings = append(bindings, newKeyBinding(actionPostReview, []string{"enter"}, "enter", "post review").shownWhen(m.selectedCount() > 0))
	}
	bindings = append(bindings,
		newKeyBinding(actionApprove, []string{"a", "y"}, "a|y", "approve"),
		newKeyBinding(actionRejectOrStopReview, []string{"d"}, "d", rejectOrStop).warning(),
		newKeyBinding(actionOpenClone, []string{"o"}, "o", "nvim"),
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
	return []keyBinding{
		newKeyBinding(actionSaveNote, []string{"ctrl+o"}, "ctrl+o", "save"),
		newKeyBinding(actionCopyEditor, []string{"ctrl+y"}, "ctrl+y", "copy"),
		newKeyBinding(actionCancelEdit, []string{"esc"}, "esc", "cancel"),
	}
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
		newKeyBinding(actionCloseSearch, []string{"esc", "ctrl+c"}, "esc|ctrl+c", "close"),
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

func errorBindings() []keyBinding {
	return []keyBinding{
		newKeyBinding(actionDismissError, []string{"esc"}, "esc", "dismiss"),
	}
}
