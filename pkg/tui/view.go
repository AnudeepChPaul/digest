package tui

import (
	"fmt"
	"strings"

	"github.com/AnudeepChPaul/digest/pkg/brag"
	"github.com/AnudeepChPaul/digest/pkg/model"
	"github.com/AnudeepChPaul/digest/pkg/review"
	"github.com/AnudeepChPaul/digest/pkg/sourcecontrol"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (m Model) renderErrorModal(modalWidth int) string {
	title := m.errorTitle
	if title == "" {
		title = "ERROR"
	}
	titleText := deleteTitleStyle.Render(" " + title + " ")
	errorText := lipgloss.NewStyle().
		Foreground(colourRed).
		Width(modalWidth - 6).
		Render(strings.Join(m.errorLines, "\n"))

	footerText := renderModalFooter(footerItemsFrom(errorBindings()), modalWidth-6)

	popupContent := lipgloss.JoinVertical(
		lipgloss.Left,
		titleText,
		"",
		errorText,
		"",
		footerText,
	)

	return m.framedPopup(popupContent, modalWidth)
}

var composeDashboard = func(m Model) string {
	frame := m.currentDashboardFrame()
	return lipgloss.JoinVertical(lipgloss.Left, frame.header, m.renderFrameBody(frame), frame.footer)
}

func (m Model) View() string {
	if m.width < 40 {
		return "Terminal window is too small."
	}
	screen := m.renderScreen()
	if m.ctrlCCount == 0 || m.mode == ViewDashboard || m.mode == ViewInlineEdit || m.mode == ViewNotifyInput {
		return screen
	}
	return withQuitCountdown(screen, fmt.Sprintf("ctrl+c %d more to quit", 3-m.ctrlCCount), m.width)
}

func withQuitCountdown(screen, warning string, width int) string {
	lines := strings.Split(screen, "\n")
	lines[len(lines)-1] = lipgloss.PlaceHorizontal(width, lipgloss.Center, yellowBadgeStyle.Render(warning))
	return strings.Join(lines, "\n")
}

func (m Model) renderScreen() string {
	if m.scrollPending {
		m.settleScroll()
	}

	modalWidth := modalWidthFor(m.width)
	innerWidth := modalWidth - 6

	switch m.mode {

	case ViewGitDetails:
		return m.renderGitDetailsModal(modalWidth, innerWidth)

	case ViewDeleteConfirm:
		return m.renderDeleteConfirmModal(modalWidth)

	case ViewError:
		return m.renderErrorModal(modalWidth)

	case ViewReviewConfirm:
		return m.renderReviewConfirm(modalWidth)

	case ViewReviewRunConfirm:
		return m.renderReviewRunConfirm(modalWidth)

	case ViewRejectComment:
		return m.renderRejectComment(modalWidth)

	case ViewArchived:
		return m.renderArchivedModal(modalWidth)

	case ViewPreview:
		return m.renderPreviewModal(modalWidth, innerWidth)

	case ViewEdit:
		return m.renderEditModal(modalWidth)

	case ViewSearch:
		return m.renderSearchModal(modalWidth)

	case ViewSearchPreview:
		return m.renderSearchPreview(modalWidth)

	case ViewHelp:
		return m.renderHelp(modalWidth)

	case ViewBragList:
		return m.renderBragList()

	case ViewBragView:
		return m.renderBragView()

	case ViewBragConfirm:
		return m.renderBragConfirm(modalWidth)

	case ViewBragEdit:
		return m.renderBragEdit()

	case ViewAutomationConfirm:
		return m.renderAutomationConfirm(modalWidth)

	case ViewAutomationEdit:
		return m.renderAutomationEdit()

	case ViewSetup:
		return m.renderSetup(modalWidth)

	case ViewSetupDiscard:
		return m.renderSetupDiscard(modalWidth)

	case ViewActionMenu:
		return m.renderActionMenu()

	}

	if m.mode == ViewNotifyInput {
		pill := m.notifyInfoPill()
		return m.overlayUnderSelectedRow(pill, m.rightAlignedColumn(pill))
	}
	if m.mode == ViewInlineEdit {
		pill := m.inlineEditPill()
		return m.overlayUnderSelectedRow(pill, m.rightAlignedColumn(pill))
	}
	if m.mode == ViewDashboard && m.hintVisible {
		if pill := m.keyHintPill(); pill != "" {
			return m.overlayUnderSelectedRow(pill, m.rightAlignedColumn(pill))
		}
	}
	return composeDashboard(m)
}

func (m Model) renderGitDetailsModal(modalWidth, innerWidth int) string {
	if m.gitPopupRepo == nil {
		return composeDashboard(m)
	}

	titleText := modalTitleStyle.Render(fmt.Sprintf(" GIT DETAILS: %s ", m.gitPopupRepo.Name))

	tabNames := []string{"All", "Reviewed", "Assigned", "Commits"}
	var renderedTabs []string
	for i, name := range tabNames {
		if i == m.gitPopupTab {
			renderedTabs = append(renderedTabs, tabActiveStyle.Render(name))
		} else {
			renderedTabs = append(renderedTabs, tabInactiveStyle.Render(name))
		}
	}
	tabsRow := strings.Join(renderedTabs, " ")

	footerText := renderModalFooter(footerItemsFrom(gitDetailsBindings()), modalWidth-6)
	fixedHeight := lipgloss.Height(lipgloss.JoinVertical(lipgloss.Left, titleText, "\n"+tabsRow, "", "", footerText))
	listRows := max(1, previewContentHeight(m.height)-fixedHeight)

	var listLines []string
	items := m.filteredGitItems()

	if len(items) == 0 {
		listLines = append(listLines, "  "+mutedStyle.Render("(no items in this tab)"))
	} else {
		firstRow, lastRow := visibleGitRows(m.gitPopupSelected, len(items), listRows)
		for i := firstRow; i < lastRow; i++ {
			item := items[i]
			prefix := "  "
			kindTag := fmt.Sprintf("[%s]", item.Kind)

			titleWidth := innerWidth - len(kindTag) - 6
			title := item.Title
			if titleWidth > 5 {
				title = ansi.Truncate(title, titleWidth, "…")
			}
			gap := max(titleWidth-ansi.StringWidth(title), 1)

			if i == m.gitPopupSelected {
				renderedTitle := selectedTitle(title)
				listLines = append(listLines, fmt.Sprintf("%s%s%s %s", prefix, renderedTitle, safeRepeat(" ", gap), underlined(mutedStyle.Render(kindTag))))
			} else {
				listLines = append(listLines, fmt.Sprintf("%s%s%s %s", prefix, itemStyle.Render(title), safeRepeat(" ", gap), mutedStyle.Render(kindTag)))
			}
		}
	}
	for len(listLines) < listRows {
		listLines = append(listLines, "")
	}

	popupContent := lipgloss.JoinVertical(
		lipgloss.Left,
		titleText,
		"\n"+tabsRow,
		"",
		strings.Join(listLines, "\n"),
		"",
		footerText,
	)

	return m.framedPopup(popupContent, modalWidth)
}

func (m Model) renderDeleteConfirmModal(modalWidth int) string {
	var titleText string
	var prompt string

	if m.jobToAbort != "" {
		titleText = deleteTitleStyle.Render(" ABORT JOB ")
		prompt = fmt.Sprintf("Are you sure you want to abort running job '%s'?", m.jobToAbort)
	} else if m.jobToExecute != "" {
		titleText = modalTitleStyle.Render(" EXECUTE JOB ")
		prompt = fmt.Sprintf("Are you sure you want to run '%s'?", m.jobToExecute)
	} else if target := m.stopTargetName(); target != "" {
		titleText = deleteTitleStyle.Render(" STOP JOB ")
		prompt = fmt.Sprintf("Are you sure you want to stop '%s'?", target)
	} else {
		titleText = deleteTitleStyle.Render(" DELETE CONFIRMATION ")
		if len(m.deleteTargetNotes) > 1 {
			prompt = fmt.Sprintf("Are you sure you want to permanently delete these %d selected notes?", len(m.deleteTargetNotes))
		} else if len(m.deleteTargetNotes) == 1 {
			if m.deleteReturnMode == ViewArchived {
				prompt = fmt.Sprintf("Are you sure you want to permanently delete this note?\n\n\"%s\"", m.deleteTargetNotes[0].Summary)
			} else {
				prompt = fmt.Sprintf("Are you sure you want to archive this note?\n\n\"%s\"", m.deleteTargetNotes[0].Summary)
			}
		} else {
			prompt = "No notes selected for deletion."
		}
	}

	footerText := renderModalFooter(footerItemsFrom(deleteConfirmBindings()), modalWidth-6)

	popupContent := lipgloss.JoinVertical(
		lipgloss.Left,
		titleText,
		"",
		prompt,
		"",
		footerText,
	)

	return m.framedPopup(popupContent, modalWidth)
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

func (m Model) renderPreviewModal(modalWidth, innerWidth int) string {
	navItems := m.allNavItems()
	headerTitle := " PREVIEW "
	statusBadge := badgeActive.Render("IDLE")
	tagBadge := tagStyle.Render("#general")

	if len(navItems) > 0 && m.selected < len(navItems) {
		item := navItems[m.selected]
		switch item.Kind {
		case KindGitRepo:
			headerTitle = fmt.Sprintf(" GIT REPO: %s ", item.GitRepo.Name)
			statusBadge = badgeActive.Render("SYNCED")
			tagBadge = tagStyle.Render("#git")
		case KindPendingGit:
			if item.PendingGitPR != nil {
				headerTitle = fmt.Sprintf(" PENDING PR REVIEW: %s ", item.PendingGitPR.Repository)
				if item.PendingGitPR.Kind == sourcecontrol.ReReviewKind {
					headerTitle = fmt.Sprintf(" RE-REVIEW: %s ", item.PendingGitPR.Repository)
				}
				statusBadge = m.prStateBadge(item.PendingGitPR)
				if pid, running := m.reviewPIDFor(item.PendingGitPR); running {
					statusBadge = badgeActive.Render(fmt.Sprintf("RUNNING · PID %d", pid))
				}
				tagBadge = tagStyle.Render("#github")
			}
		case KindMyPR:
			if item.MyPR != nil {
				headerTitle = fmt.Sprintf(" MY PR: %s #%d ", item.MyPR.Ref.Repo, item.MyPR.Ref.Number)
				statusBadge = badgeActive.Render(strings.ToUpper(myPRCIText(item.MyPR.CIState)))
				tagBadge = tagStyle.Render("#my-pr")
			}
		case KindReviewRun:
			headerTitle = fmt.Sprintf(" REVIEW JOB: %s ", reviewRunLabel(*item.ReviewRun))
			statusBadge = stateStyle(review.StateFailed).Render("FAILED")
			if state := m.localReviews[review.StateDir(m.reviewRoot(), item.ReviewRun.Meta.Ref)]; state.pid > 0 {
				statusBadge = badgeActive.Render(fmt.Sprintf("RUNNING · PID %d", state.pid))
			}
			tagBadge = tagStyle.Render("#github")
		case KindBragRun:
			headerTitle = fmt.Sprintf(" BRAG JOB: %s ", item.BragRun.Meta.ID)
			statusBadge = stateStyle(review.StateFailed).Render("FAILED")
			if item.BragRun.Status == brag.RunRunning {
				statusBadge = badgeActive.Render("RUNNING")
			}
			tagBadge = tagStyle.Render("#brag")
		case KindAutomationRun:
			headerTitle = fmt.Sprintf(" AUTOMATION JOB: %s ", item.AutomationRun.Meta.Automation)
			statusBadge = stateStyle(review.StateFailed).Render("FAILED")
			if m.automationJobRunning(item.AutomationRun) {
				statusBadge = badgeActive.Render("RUNNING")
			}
			tagBadge = tagStyle.Render("#" + string(item.AutomationRun.Meta.Phase))
		case KindJobDraft:
			headerTitle = fmt.Sprintf(" JOB: %s ", item.Draft.Name)
			if pid := m.runningJobPIDs[item.Draft.Name]; pid > 0 {
				statusBadge = badgeActive.Render(fmt.Sprintf("RUNNING · PID %d", pid))
			} else if item.Draft.DryRunInFlight {
				statusBadge = badgeActive.Render("DRY RUNNING")
			} else if m.previewJobLogFinished {
				statusBadge = badgeDone.Render("FINISHED")
			} else if item.Draft.HasRunDryRun {
				if item.Draft.ExitCode == 0 {
					statusBadge = badgeDone.Render("SUCCESS")
				} else {
					statusBadge = staleStyle.Render("NEED ACT")
				}
			} else {
				statusBadge = badgeActive.Render("IDLE")
			}
			tagBadge = tagStyle.Render("#job")
		default:
			if item.Note != nil {
				headerTitle = " PREVIEW NOTE "
				if item.Note.Status == model.StatusDone {
					statusBadge = badgeDone.Render("DONE")
				}
				if item.Note.Subject != "" {
					tagBadge = tagStyle.Render("#" + item.Note.Subject)
				}
			}
		}
	}

	headerLeft := modalTitleStyle.Render(headerTitle)
	rightCol := lipgloss.JoinVertical(lipgloss.Right, statusBadge, tagBadge)
	gap := innerWidth - lipgloss.Width(headerLeft) - lipgloss.Width(rightCol)

	topLine := lipgloss.JoinHorizontal(lipgloss.Top, headerLeft, safeRepeat(" ", gap), rightCol)

	prItem := m.currentPRItem()
	items := footerItemsFrom(m.previewBindings())

	footerText := renderModalFooter(items, modalWidth-6)

	partsAbove := []string{topLine, ""}
	if prItem != nil || m.hasDraftTab() {
		partsAbove = append(partsAbove, m.renderPreviewTabs(), "")
	}
	partsBelow := []string{""}
	if prItem != nil && m.reviewNotice != "" {
		partsBelow = append(partsBelow, yellowBadgeStyle.Render(m.reviewNotice), "")
	}
	partsBelow = append(partsBelow, footerText)
	fixedHeight := lipgloss.Height(lipgloss.JoinVertical(lipgloss.Left, partsAbove...)) + lipgloss.Height(lipgloss.JoinVertical(lipgloss.Left, partsBelow...))
	m.previewViewport.Height = max(3, previewContentHeight(m.height)-fixedHeight)
	previewParts := append(append(partsAbove, m.previewViewport.View()), partsBelow...)
	popupContent := lipgloss.JoinVertical(lipgloss.Left, previewParts...)

	return m.framedPopup(popupContent, modalWidth)
}

func (m Model) renderEditModal(modalWidth int) string {
	titleText := modalTitleStyle.Render(" ADD / EDIT NOTE ")

	footerText := renderModalFooter(footerItemsFrom(editBindings()), modalWidth-6)

	popupContent := lipgloss.JoinVertical(
		lipgloss.Left,
		titleText,
		"",
		m.editor.View(),
		"",
		footerText,
	)

	return m.framedPopup(popupContent, modalWidth)
}

func scrollViewport(view *viewport.Model, key string) bool {
	switch key {
	case "j", "down":
		view.ScrollDown(1)
	case "k", "up":
		view.ScrollUp(1)
	case "pgdown":
		view.PageDown()
	case "pgup":
		view.PageUp()
	case "ctrl+d":
		view.HalfPageDown()
	case "ctrl+u":
		view.HalfPageUp()
	default:
		return false
	}
	return true
}

func overlayAt(base, box string, top, left int) string {
	baseLines := strings.Split(base, "\n")
	boxWidth := lipgloss.Width(box)
	for index, boxLine := range strings.Split(box, "\n") {
		row := top + index
		if row < 0 || row >= len(baseLines) {
			continue
		}
		line := baseLines[row]
		before := ansi.Truncate(line, left, "")
		if gap := left - lipgloss.Width(before); gap > 0 {
			before += strings.Repeat(" ", gap)
		}
		after := ansi.TruncateLeft(line, left+boxWidth, "")
		baseLines[row] = before + "\x1b[0m" + boxLine + "\x1b[0m" + after
	}
	return strings.Join(baseLines, "\n")
}
