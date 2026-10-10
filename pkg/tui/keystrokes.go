package tui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

type keyActionHandler func(Model, tea.KeyMsg) (tea.Model, tea.Cmd)

type sectionKeystrokes map[keyAction]keyActionHandler

type keystrokeApplier func(Model, keyBinding, tea.KeyMsg) (tea.Model, tea.Cmd, bool)

var appKeystrokes sectionKeystrokes

var keystrokeAppliers []keystrokeApplier

func init() {
	appKeystrokes = sectionKeystrokes{
		actionQuit:         Model.countCtrlCToQuit,
		actionHalfPageDown: Model.dashboardHalfPageDown,
		actionHalfPageUp:   Model.dashboardHalfPageUp,
		actionPageDown:     Model.dashboardPageDown,
		actionPageUp:       Model.dashboardPageUp,
		actionToday:        Model.jumpToToday,
		actionOpenPreview:  Model.openSelectedPreview,
		actionOpenItem:     Model.openSelectedItem,
		actionDeleteItem:   Model.deleteSelectedItem,
		actionPreviousDay:  Model.previousDay,
		actionNextDay:      Model.nextDay,
		actionCursorDown:   Model.dashboardCursorDown,
		actionCursorUp:     Model.dashboardCursorUp,
		actionOpenSearch:   Model.openSearch,
		actionOpenBrag:     Model.openBrag,
		actionOpenHelp:     Model.openHelp,

		actionCloseHelp: Model.closeHelp,

		actionRunAutomationDraft:    Model.runAutomationDraft,
		actionConfirmAutomation:     Model.confirmAutomation,
		actionCancelAutomation:      Model.cancelAutomation,
		actionEditAutomation:        Model.editAutomationDraft,
		actionDeleteAutomationDraft: Model.deleteAutomationDraft,
		actionSaveAutomationEdit:    Model.saveAutomationEdit,
		actionCancelAutomationEdit:  Model.cancelAutomationEdit,
		actionCopyAutomationEditor:  Model.copyAutomationEditor,

		actionOpenActions:        Model.openActionMenu,
		actionChooseAction:       Model.chooseAction,
		actionActionMenuDown:     Model.actionMenuDown,
		actionActionMenuUp:       Model.actionMenuUp,
		actionCloseActionMenu:    Model.closeActionMenu,
		actionSetupYes:           Model.setupAnswerYes,
		actionSetupNo:            Model.setupAnswerNo,
		actionSetupConfirm:       Model.setupConfirm,
		actionSetupSwitchTime:    Model.setupSwitchTime,
		actionSetupDayLeft:       Model.setupDayLeft,
		actionSetupDayRight:      Model.setupDayRight,
		actionSetupToggleDay:     Model.setupToggleDay,
		actionSetupFieldNext:     Model.setupFieldNext,
		actionSetupFieldPrevious: Model.setupFieldPrevious,
		actionSetupFormToggle:    Model.setupFormToggle,
		actionSetupFormSave:      Model.setupFormSave,
		actionSetupFormTab:       Model.setupFormTab,
		actionSetupFormEscape:    Model.setupFormEscape,
		actionKeepEditingSetup:   Model.keepEditingSetup,
		actionCloseSetup:         Model.closeSetup,
		actionOpenSetup:          Model.openSetup,
		actionEditorHalfPageUp:   Model.editorHalfPageUp,
		actionEditorHalfPageDown: Model.editorHalfPageDown,
		actionEditorPageUp:       Model.editorPageUp,
		actionEditorPageDown:     Model.editorPageDown,

		actionCloseBrag:      Model.closeBrag,
		actionBragCursorDown: Model.bragCursorDown,
		actionBragCursorUp:   Model.bragCursorUp,
		actionBragListEnter:  Model.bragListEnter,
		actionCloseBragView:  Model.closeBragView,
		actionEditBrag:       Model.editBrag,
		actionBragAgain:      Model.bragAgain,
		actionCopyBrag:       Model.copyBrag,
		actionConfirmBrag:    Model.confirmBrag,
		actionCancelBrag:     Model.cancelBrag,
		actionSaveBragEdit:   Model.saveBragEdit,
		actionCopyBragEditor: Model.copyBragEditor,
		actionCancelBragEdit: Model.cancelBragEdit,

		actionConfirmDelete: Model.confirmDelete,
		actionCancelDelete:  Model.cancelDelete,

		actionClosePreview:     Model.closePreview,
		actionPreviewPrevious:  Model.previewPrevious,
		actionPreviewNext:      Model.previewNext,
		actionCopyPreviewItem:  Model.copyPreviewItem,
		actionPreviewStop:      Model.previewStop,
		actionPreviewEnter:     Model.previewEnter,
		actionOpenNoteLinks:    Model.openNoteLinks,
		actionLinkMenuDown:     Model.linkMenuDown,
		actionLinkMenuUp:       Model.linkMenuUp,
		actionChooseLink:       Model.chooseLink,
		actionCloseLinkMenu:    Model.closeLinkMenu,
		actionSwitchPreviewTab: Model.switchPreviewTab,

		actionCloseSearch:        Model.closeSearch,
		actionSearchCursorUp:     Model.searchCursorUp,
		actionSearchCursorDown:   Model.searchCursorDown,
		actionSearchPageUp:       Model.searchPageUp,
		actionSearchPageDown:     Model.searchPageDown,
		actionSearchHalfPageUp:   Model.searchHalfPageUp,
		actionSearchHalfPageDown: Model.searchHalfPageDown,
		actionOpenSearchPreview:  Model.openSearchPreview,
		actionOpenSearchResult:   Model.openSearchResult,
		actionExportSearch:       Model.exportSearch,

		actionDismissError: Model.dismissError,
	}
	keystrokeAppliers = []keystrokeApplier{
		headerSection{}.ApplyKeystrokes,
		appKeystrokes.apply,
		notesSection{}.ApplyKeystrokes,
		gitSection{}.ApplyKeystrokes,
		jobsSection{}.ApplyKeystrokes,
	}
}

func (handlers sectionKeystrokes) apply(m Model, binding keyBinding, msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	handler, owned := handlers[binding.action]
	if !owned {
		return m, nil, false
	}
	next, cmd := handler(m, msg)
	return next, cmd, true
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
	if m.setupSaving() {
		return m, nil
	}
	m.screenError = ""
	if msg.String() == "ctrl+c" {
		return m.countCtrlCToQuit(msg)
	}
	m.ctrlCCount = 0
	if binding, found := m.resolveKey(msg); found {
		for _, applyKeystroke := range keystrokeAppliers {
			if next, cmd, handled := applyKeystroke(m, binding, msg); handled {
				return next, cmd
			}
		}
		return m, nil
	}
	return m.forwardUnboundKey(msg)
}
