package tui

import (
	"slices"

	"github.com/achandrapaul/digest/pkg/brag"
	"github.com/achandrapaul/digest/pkg/model"
	"github.com/achandrapaul/digest/pkg/review"

	tea "github.com/charmbracelet/bubbletea"
)

var hintKeySymbols = map[string]string{"enter": "↵", "space": "␣"}

func (m Model) itemBindings(item NavItem, onDashboard bool) []keyBinding {
	switch item.Kind {
	case KindJobDraft:
		if item.Draft == nil {
			return nil
		}
		if m.jobRunning(item.Draft.Name) {
			return []keyBinding{newKeyBinding(actionStopSelectedItem, []string{"d"}, "d", "stop").warning()}
		}
		runKeyLabel := "r"
		if !onDashboard {
			runKeyLabel = "r|⏎"
		}
		bindings := []keyBinding{
			newKeyBinding(actionRunSelectedJob, []string{"r"}, runKeyLabel, "run"),
		}
		if jobHasDryRun(item.Draft.DryRunCommand) {
			bindings = append(bindings, newKeyBinding(actionDryRunSelectedJob, []string{"d"}, "d", "dry run").shownWhen(!item.Draft.DryRunInFlight))
		}
		return bindings
	case KindReviewRun, KindBragRun, KindAutomationRun:
		label := m.runStopLabel(item)
		if label == "" {
			return nil
		}
		bindings := []keyBinding{newKeyBinding(actionStopSelectedItem, []string{"d"}, "d", label).warning()}
		if label == "dismiss" {
			bindings = append(bindings, newKeyBinding(actionRunSelectedJob, []string{"r"}, "r", "retry"))
		}
		return bindings
	case KindPendingGit:
		if item.PendingGitPR == nil {
			return nil
		}
		approve, reject := actionApprove, actionRejectOrStopReview
		if onDashboard {
			approve, reject = actionDashboardApprove, actionDashboardReject
		}
		rejectLabel := "reject"
		if _, running := m.reviewPIDFor(item.PendingGitPR); running {
			rejectLabel = "stop"
		}
		bindings := []keyBinding{
			newKeyBinding(approve, []string{"y"}, "y", "approve"),
			newKeyBinding(reject, []string{"d"}, "d", rejectLabel).warning(),
			newKeyBinding(actionStartReview, []string{"r"}, "r", "review"),
		}
		if m.cloneReady(item.PendingGitPR) {
			bindings = append(bindings, newKeyBinding(actionOpenClone, []string{"o"}, "o", "nvim"))
		}
		return bindings
	case KindTodayNote, KindCarriedNote, KindYesterdayDone, KindTodayDone:
		if item.Note == nil {
			return nil
		}
		links := noteLinks(item.Note)
		toggleLabel := "done"
		if item.Note.Status == model.StatusDone {
			toggleLabel = "active"
		}
		return []keyBinding{
			newKeyBinding(actionToggleDone, []string{" ", "space"}, "space", toggleLabel),
			newKeyBinding(actionOpenActions, []string{".", "@"}, ".|@", "actions").shownWhen(len(m.noteActions(item.Note)) > 0),
			newKeyBinding(actionOpenNoteLinks, []string{"o"}, "o", noteLinksLabel(links, onDashboard)).shownWhen(len(links) > 0),
		}
	}
	return nil
}

func (m Model) runStopLabel(item NavItem) string {
	switch {
	case item.Kind == KindReviewRun && item.ReviewRun != nil && item.ReviewRun.Status == review.RunRunning:
		return "stop"
	case item.Kind == KindReviewRun && item.ReviewRun != nil:
		return "dismiss"
	case item.Kind == KindBragRun && item.BragRun != nil && item.BragRun.Status == brag.RunRunning:
		return "stop"
	case item.Kind == KindBragRun && item.BragRun != nil:
		return "dismiss"
	case item.Kind == KindAutomationRun && item.AutomationRun != nil && m.automationJobRunning(item.AutomationRun):
		return "stop"
	case item.Kind == KindAutomationRun && item.AutomationRun != nil:
		return "dismiss"
	}
	return ""
}

func (m Model) selectedItemBindings(onDashboard bool) []keyBinding {
	item, found := m.selectedNavItem()
	if !found {
		return nil
	}
	bindings := m.itemBindings(item, onDashboard)
	if onDashboard && item.Kind == KindPendingGit && !m.cfg.ShowKeyHints {
		return slices.DeleteFunc(bindings, func(binding keyBinding) bool { return binding.action != actionStartReview })
	}
	return bindings
}

func hintsFromBindings(bindings []keyBinding) []keyHint {
	var hints []keyHint
	for _, binding := range bindings {
		if binding.hidden {
			continue
		}
		help := binding.binding.Help()
		hintKey := help.Key
		if symbol, mapped := hintKeySymbols[hintKey]; mapped {
			hintKey = symbol
		}
		hints = append(hints, keyHint{key: hintKey, label: help.Desc})
	}
	return hints
}

func hiddenCopies(bindings []keyBinding) []keyBinding {
	copies := make([]keyBinding, len(bindings))
	for index, binding := range bindings {
		copies[index] = binding.shownWhen(false)
	}
	return copies
}

func (m Model) beginStopConfirm() Model {
	m.deleteTargetNotes = nil
	m.deleteReturnMode = m.mode
	m.mode = ViewDeleteConfirm
	return m
}

func (m *Model) clearPendingConfirms() {
	m.jobToExecute = ""
	m.jobToAbort = ""
	m.bragRunToStop = nil
	m.automationRunToStop = nil
	m.jobRunStopsDryRun = false
	m.jobDryRunToStart = ""
	m.staleJobToClear = ""
	m.runToRetry = nil
	m.draftToDelete = ""
	m.discardingEdit = false
	m.jobConfirmError = ""
	m.confirmTitle, m.confirmPrompt = "", ""
	m.restoreOnConfirm = false
}

func (m Model) stopTargetName() string {
	switch {
	case m.bragRunToStop != nil:
		return "brag " + m.bragRunToStop.Meta.ID
	case m.automationRunToStop != nil:
		return m.automationRunToStop.Meta.Automation
	}
	return ""
}

func (m Model) confirmStopRun() (tea.Model, tea.Cmd) {
	bragRun, automationRun := m.bragRunToStop, m.automationRunToStop
	m.clearPendingConfirms()
	m.mode = m.deleteReturnMode
	if m.mode == ViewPreview {
		m.updatePreviewViewport()
	}
	if bragRun != nil {
		return m.stopRunningBragRuns()
	}
	if automationRun != nil {
		return m.stopAutomationRun(automationRun)
	}
	return m, nil
}
