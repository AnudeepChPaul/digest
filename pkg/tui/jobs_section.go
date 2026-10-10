package tui

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/achandrapaul/digest/pkg/automation"
	"github.com/achandrapaul/digest/pkg/brag"
	"github.com/achandrapaul/digest/pkg/review"
	"github.com/achandrapaul/digest/pkg/system"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type jobsSection struct{}

type JobDraft struct {
	Name           string
	DryRunCommand  string
	Command        string
	ExitCode       int
	HasRunDryRun   bool
	DryRunInFlight bool
}

type jobLogTickMsg struct{}

const jobLogTailLines = 400

type runStatePollTickMsg struct{}

type jobAbortedMsg struct {
	jobName string
	err     error
}

func (m Model) getJobDrafts() []*JobDraft {
	if m.cfg == nil || len(m.cfg.JobList()) == 0 {
		return nil
	}

	var drafts []*JobDraft
	for _, j := range m.cfg.JobList() {
		exitCode := 1
		hasRun := false
		if m.jobDryRunHasRun != nil && m.jobDryRunHasRun[j.Name] {
			hasRun = true
			exitCode = m.jobDryRunExitCodes[j.Name]
		}

		drafts = append(drafts, &JobDraft{
			Name:           j.Name,
			DryRunCommand:  j.DryRunCommand,
			Command:        j.Command,
			ExitCode:       exitCode,
			HasRunDryRun:   hasRun,
			DryRunInFlight: m.dryRunsInFlight[j.Name],
		})
	}
	return drafts
}

func tickRunStatePollCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return runStatePollTickMsg{}
	})
}

func (m *Model) ensureRunStatePoll() tea.Cmd {
	if m.runStatePolling {
		return nil
	}
	m.runStatePolling = true
	return tickRunStatePollCmd()
}

func (m *Model) refreshJobStates() {
	runningPIDs := make(map[string]int)
	inFlight := make(map[string]bool)
	if m.cfg != nil {
		for _, j := range m.cfg.JobList() {
			if pid, running := runningJobPID(j.Name); running {
				runningPIDs[j.Name] = pid
			}
			if isDryRunInFlight(j.Name) {
				inFlight[j.Name] = true
			}
		}
	}
	m.runningJobPIDs, m.dryRunsInFlight = runningPIDs, inFlight
}

func (m *Model) refreshDryRunResults() {
	m.refreshJobStates()
	if m.cfg == nil {
		return
	}
	if m.jobDryRunOutputs == nil {
		m.jobDryRunOutputs = make(map[string]string)
		m.jobDryRunExitCodes = make(map[string]int)
		m.jobDryRunHasRun = make(map[string]bool)
	}
	if m.dryRunLogStamps == nil {
		m.dryRunLogStamps = make(map[string]string)
	}
	for _, j := range m.cfg.JobList() {
		if m.dryRunsInFlight[j.Name] {
			continue
		}
		stamp := dryRunStamp(j.Name)
		if previous, seen := m.dryRunLogStamps[j.Name]; seen && previous == stamp {
			continue
		}
		m.dryRunLogStamps[j.Name] = stamp
		output, exitCode, ok := loadDryRunResult(j.Name)
		if !ok {
			delete(m.jobDryRunOutputs, j.Name)
			delete(m.jobDryRunExitCodes, j.Name)
			delete(m.jobDryRunHasRun, j.Name)
			continue
		}
		m.jobDryRunOutputs[j.Name] = output
		m.jobDryRunExitCodes[j.Name] = exitCode
		m.jobDryRunHasRun[j.Name] = true
	}
}

func tickJobLogCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
		return jobLogTickMsg{}
	})
}

func (m *Model) ensureJobLogRefresh() tea.Cmd {
	if m.jobLogRunning {
		return nil
	}
	m.jobLogRunning = true
	return tickJobLogCmd()
}

func (m Model) handleJobLogTick() (tea.Model, tea.Cmd) {
	jobName := m.previewedRunningJob()
	if jobName == "" {
		m.jobLogRunning = false
		return m, nil
	}
	m.refreshJobStates()
	running := m.jobRunning(jobName)
	if stamp := jobLogStampFor(jobName, running); stamp != m.jobLogStamp {
		m.jobLogStamp = stamp
		wasAtBottom := m.previewViewport.AtBottom()
		previousOffset := m.previewViewport.YOffset
		m.updatePreviewViewport()
		if wasAtBottom {
			m.previewViewport.GotoBottom()
		} else {
			m.previewViewport.SetYOffset(previousOffset)
		}
	}
	if !running {
		m.jobLogRunning = false
		return m, nil
	}
	return m, tickJobLogCmd()
}

func (m Model) runSelectedJob(tea.KeyMsg) (tea.Model, tea.Cmd) {
	item, found := m.selectedNavItem()
	if !found {
		return m, nil
	}
	if m.runStopLabel(item) == "dismiss" {
		retried := item
		m.runToRetry = &retried
		return m.beginStopConfirm(), nil
	}
	if item.Kind != KindJobDraft || item.Draft == nil || isJobRunning(item.Draft.Name) {
		return m, nil
	}
	m.jobToExecute = item.Draft.Name
	m.jobRunStopsDryRun = isDryRunInFlight(item.Draft.Name)
	m.jobConfirmError = ""
	return m.beginStopConfirm(), nil
}

func (m Model) dryRunSelectedJob(tea.KeyMsg) (tea.Model, tea.Cmd) {
	item, found := m.selectedNavItem()
	if !found || item.Kind != KindJobDraft || item.Draft == nil || isJobRunning(item.Draft.Name) || !jobHasDryRun(item.Draft.DryRunCommand) || m.dryRunsInFlight[item.Draft.Name] {
		return m, nil
	}
	m.jobDryRunToStart = item.Draft.Name
	return m.beginStopConfirm(), nil
}

func (m Model) stopSelectedItem(tea.KeyMsg) (tea.Model, tea.Cmd) {
	item, found := m.selectedNavItem()
	if !found {
		return m, nil
	}
	switch {
	case item.Kind == KindReviewRun && item.ReviewRun != nil && review.Status(review.StateDir(m.reviewRoot(), item.ReviewRun.Meta.Ref)) == review.RunRunning:
		return m.beginReviewRunConfirm(reviewActionStop, item.ReviewRun.Meta.Ref)
	case item.Kind == KindReviewRun && item.ReviewRun != nil:
		return m.dismissReviewRun(item.ReviewRun.Meta.Ref)
	case item.Kind == KindBragRun && item.BragRun != nil && brag.IsRunning(m.bragRoot(), item.BragRun.Meta.ID):
		run := *item.BragRun
		m.bragRunToStop = &run
		return m.beginStopConfirm(), nil
	case item.Kind == KindBragRun && item.BragRun != nil:
		return m.stopOrDismissBragRun(item.BragRun)
	case item.Kind == KindAutomationRun && item.AutomationRun != nil && m.automationJobRunning(item.AutomationRun):
		run := *item.AutomationRun
		m.automationRunToStop = &run
		return m.beginStopConfirm(), nil
	case item.Kind == KindAutomationRun && item.AutomationRun != nil:
		return m.dismissAutomationRun(item.AutomationRun)
	case item.Kind == KindJobDraft && item.Draft != nil && isJobRunning(item.Draft.Name):
		m.jobToAbort = item.Draft.Name
		return m.beginStopConfirm(), nil
	case item.Kind == KindJobDraft && item.Draft != nil && m.jobRunning(item.Draft.Name):
		m.staleJobToClear = item.Draft.Name
		return m.beginStopConfirm(), nil
	}
	return m, nil
}

func (m Model) renderDraftRow(draft *JobDraft, selected bool, width int) string {
	icon := amberDiamond.Render()
	if draft.HasRunDryRun && draft.ExitCode == 0 {
		icon = checkDone.Render()
	}
	var rightBlock string
	if m.jobRunning(draft.Name) {
		runningIndicator := m.renderJobRunningIndicator()
		rightBlock = fmt.Sprintf("%s   %s", jobActiveTagStyle.Render("#job"), runningIndicator)
	} else if draft.DryRunInFlight {
		rightBlock = fmt.Sprintf("%s   %s", jobActiveTagStyle.Render("#job"), m.renderDryRunIndicator())
	} else {
		statusText := "act"
		if draft.HasRunDryRun && draft.ExitCode == 0 {
			statusText = "done"
		}

		rightBlock = fmt.Sprintf("%s   %s", dimBlueText.Render("#job"), mutedStyle.Render(statusText))
	}
	return renderJobStyleRow(icon, draft.Name, rightBlock, selected, width)
}

func (m Model) renderReviewRunRow(run review.ReviewRun, selected bool, width int) string {
	rightBlock := stateStyle(review.StateFailed).Render("failed")
	if run.Status == review.RunRunning {
		rightBlock = m.renderReviewRunningIndicator()
	}
	return renderJobStyleRow(amberDiamond.Render(), reviewRunLabel(run), rightBlock, selected, width)
}

func (m Model) renderBragRunRow(run brag.Run, selected bool, width int) string {
	rightBlock := stateStyle(review.StateFailed).Render("failed")
	if run.Status == brag.RunRunning {
		rightBlock = m.renderPulseIndicator("bragging...")
	}
	return renderJobStyleRow(amberDiamond.Render(), bragRunLabel(run), rightBlock, selected, width)
}

func (m Model) renderAutomationRunRow(run automation.Run, selected bool, width int) string {
	var rightBlock string
	switch {
	case run.Status == automation.RunFailed:
		rightBlock = stateStyle(review.StateFailed).Render("failed")
	case run.Status == automation.RunNeedsReauth:
		rightBlock = stateStyle(review.StateFailed).Render("needs re-auth")
	case run.Meta.Phase == automation.PhaseDraft:
		rightBlock = m.renderPulseIndicator("drafting...")
	default:
		rightBlock = m.renderPulseIndicator("running...")
	}
	return renderJobStyleRow(amberDiamond.Render(), m.automationJobLabel(run), rightBlock, selected, width)
}

func renderJobStyleRow(icon, label, rightBlock string, selected bool, width int) string {
	label = ansi.Truncate(label, max(width-lipgloss.Width(rightBlock)-6, 5), "…")
	labelText := itemStyle.Render(label)
	if selected {
		labelText = selectedTitle(label)
	}
	return alignRight("   "+icon+" "+labelText, []string{rightBlock}, width, selected) + "\n"
}

func (jobsSection) ApplyMessage(m Model, msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case jobLogTickMsg:
		return messageHandled(m.handleJobLogTick())

	case runStatePollTickMsg:
		previousJobState := m.previewedJobState()
		m.refreshDryRunResults()
		stillBusy := m.isAnyDryRunInFlight() || m.isAnyJobRunning()
		if m.mode == ViewPreview && !m.jobLogRunning && m.previewedJobState() != previousJobState {
			m.updatePreviewViewport()
		}
		if stillBusy {
			return messageHandled(m, tickRunStatePollCmd())
		}
		m.runStatePolling = false
		return messageHandled(m, nil)

	case jobAbortedMsg:
		m.refreshJobStates()
		if m.mode == ViewPreview {
			m.updatePreviewViewport()
		}
		if msg.err != nil {
			m.showError("JOB ERROR", msg.err)
		}
		return messageHandled(m, nil)

	case reviewPollTickMsg:
		return messageHandled(m.handleReviewPoll(msg.snapshot))
	}
	return m, nil, false
}

var jobsKeystrokes sectionKeystrokes

func init() {
	jobsKeystrokes = sectionKeystrokes{
		actionRunSelectedJob:    Model.runSelectedJob,
		actionDryRunSelectedJob: Model.dryRunSelectedJob,
		actionStopSelectedItem:  Model.stopSelectedItem,
	}
}

func (jobsSection) ApplyKeystrokes(m Model, binding keyBinding, msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	return jobsKeystrokes.apply(m, binding, msg)
}

func (jobsSection) Render(m Model, builder *dashboardBuilder, data dashboardData) {
	builder.board.WriteString(sectionGap + "  " + m.renderSubSection("Jobs", 0, false, builder.selectedWithin(data.pendingEnd, data.jobsEnd)) + "\n")
	if data.jobsCount == 0 {
		builder.board.WriteString(mutedStyle.Render("   (no jobs configured)\n"))
	}
	for _, draft := range data.drafts {
		builder.emitRow(func(selected bool) string { return m.renderDraftRow(draft, selected, builder.innerWidth) })
	}
	for _, run := range m.reviewRuns {
		builder.emitRow(func(selected bool) string { return m.renderReviewRunRow(run, selected, builder.innerWidth) })
	}
	for _, run := range m.bragRuns {
		builder.emitRow(func(selected bool) string { return m.renderBragRunRow(run, selected, builder.innerWidth) })
	}
	for _, run := range data.automationRuns {
		builder.emitRow(func(selected bool) string { return m.renderAutomationRunRow(run, selected, builder.innerWidth) })
	}
}

func (jobsSection) appendNavItems(m Model, items []NavItem, data dashboardData) []NavItem {
	for _, d := range data.drafts {
		items = append(items, NavItem{Kind: KindJobDraft, Draft: d})
	}
	for i := range m.reviewRuns {
		items = append(items, NavItem{Kind: KindReviewRun, ReviewRun: &m.reviewRuns[i]})
	}
	for i := range m.bragRuns {
		items = append(items, NavItem{Kind: KindBragRun, BragRun: &m.bragRuns[i]})
	}
	for i := range data.automationRuns {
		items = append(items, NavItem{Kind: KindAutomationRun, AutomationRun: &data.automationRuns[i]})
	}
	return items
}

func (m Model) dismissReviewRun(ref review.PRRef) (tea.Model, tea.Cmd) {
	if err := review.Dismiss(m.reviewRoot(), ref); err != nil {
		m.showError("REVIEW ERROR", err)
		return m, nil
	}
	selectedKey, selectedOccurrence := m.selectedNavKey()
	m.refreshReviewRuns()
	m.restoreSelection(selectedKey, selectedOccurrence)
	if m.mode == ViewPreview {
		if _, ok := m.selectedNavItem(); ok {
			m.updatePreviewViewport()
		} else {
			m.mode = ViewDashboard
		}
	}
	m.clampScreenSelection()
	return m, nil
}

func (m Model) jobConfirmPrompt() (title, prompt string, found bool) {
	switch {
	case m.jobToAbort != "":
		return deleteTitleStyle.Render(" ABORT JOB "), fmt.Sprintf("Are you sure you want to abort running job '%s'?", m.jobToAbort), true
	case m.jobToExecute != "" && m.jobRunStopsDryRun:
		prompt = fmt.Sprintf("Dry run in progress. Stop it and run the job?\n\nThe dry run of '%s' is killed first.", m.jobToExecute)
		return modalTitleStyle.Render(" EXECUTE JOB "), m.withJobConfirmError(prompt), true
	case m.jobToExecute != "":
		return modalTitleStyle.Render(" EXECUTE JOB "), m.withJobConfirmError(fmt.Sprintf("Are you sure you want to run '%s'?", m.jobToExecute)), true
	case m.jobDryRunToStart != "":
		return modalTitleStyle.Render(" DRY RUN "), fmt.Sprintf("Dry run '%s'?", m.jobDryRunToStart), true
	case m.staleJobToClear != "":
		return deleteTitleStyle.Render(" STALE JOB "), fmt.Sprintf("'%s' is no longer running.\n\nClear its stale state?", m.staleJobToClear), true
	case m.runToRetry != nil:
		return modalTitleStyle.Render(" RETRY JOB "), fmt.Sprintf("Run again?\n\n%s", m.runLabel(*m.runToRetry)), true
	case m.draftToDelete != "":
		return deleteTitleStyle.Render(" DELETE DRAFT "), fmt.Sprintf("Delete the %s draft for this note?", m.automationRuns[m.draftToDelete].Meta.Automation), true
	case m.discardingEdit:
		return deleteTitleStyle.Render(" DISCARD "), "Discard changes?", true
	}
	if target := m.stopTargetName(); target != "" {
		return deleteTitleStyle.Render(" STOP JOB "), fmt.Sprintf("Are you sure you want to stop '%s'?", target), true
	}
	return "", "", false
}

func (m Model) withJobConfirmError(prompt string) string {
	if m.jobConfirmError == "" {
		return prompt
	}
	return prompt + "\n\n" + stateStyle(review.StateFailed).Render(m.jobConfirmError)
}

func (m Model) runLabel(item NavItem) string {
	switch {
	case item.ReviewRun != nil:
		return "review " + reviewRunLabel(*item.ReviewRun)
	case item.BragRun != nil:
		return bragRunLabel(*item.BragRun)
	case item.AutomationRun != nil:
		return m.automationJobLabel(*item.AutomationRun)
	}
	return ""
}

func (m Model) confirmJobPrompt() (tea.Model, tea.Cmd, bool) {
	switch {
	case m.discardingEdit:
		next, cmd := m.confirmDiscardEdit()
		return next, cmd, true
	case m.jobToAbort != "":
		jobName := m.jobToAbort
		m.jobToAbort = ""
		m.returnFromJobConfirm()
		return m, abortJobCmd(jobName), true
	case m.jobToExecute != "":
		next, cmd := m.confirmRunJob()
		return next, cmd, true
	case m.jobDryRunToStart != "":
		jobName := m.jobDryRunToStart
		m.jobDryRunToStart = ""
		m.returnFromJobConfirm()
		return m, m.startJobDryRunCmd(jobName), true
	case m.staleJobToClear != "":
		jobName := m.staleJobToClear
		m.staleJobToClear = ""
		clearStaleJobPID(jobName)
		m.refreshJobStates()
		m.returnFromJobConfirm()
		return m, nil, true
	case m.runToRetry != nil:
		item := *m.runToRetry
		m.runToRetry = nil
		m.returnFromJobConfirm()
		next, cmd := m.retryRun(item)
		return next, cmd, true
	case m.draftToDelete != "":
		noteID := m.draftToDelete
		m.draftToDelete = ""
		m.mode = m.deleteReturnMode
		next, cmd := m.deleteDraft(noteID)
		return next, cmd, true
	case m.bragRunToStop != nil || m.automationRunToStop != nil:
		next, cmd := m.confirmStopRun()
		return next, cmd, true
	}
	return m, nil, false
}

func (m *Model) returnFromJobConfirm() {
	m.mode = m.deleteReturnMode
	if m.mode == ViewPreview {
		m.updatePreviewViewport()
	}
}

func (m Model) confirmRunJob() (tea.Model, tea.Cmd) {
	jobName := m.jobToExecute
	if m.jobRunStopsDryRun {
		if err := stopJobDryRun(jobName); err != nil {
			return m.failJobConfirm(err), nil
		}
		m.jobRunStopsDryRun = false
		m.refreshDryRunResults()
	}
	if err := executeJobBackground(m.cfg, jobName); err != nil {
		return m.failJobConfirm(err), nil
	}
	m.jobToExecute, m.jobConfirmError = "", ""
	if m.deleteReturnMode == ViewPreview {
		m.previewNotice = ""
	}
	m.returnFromJobConfirm()
	m.refreshJobStates()
	return m, tea.Batch(m.ensureJobLogRefresh(), m.ensureSyncPulse(), m.ensureRunStatePoll())
}

func (m Model) failJobConfirm(err error) Model {
	m.showError("JOB ERROR", err)
	m.screenError = ""
	m.jobConfirmError = err.Error()
	if m.deleteReturnMode == ViewPreview {
		m.previewNotice = "JOB ERROR: " + err.Error()
	}
	m.refreshJobStates()
	return m
}

func clearStaleJobPID(jobName string) {
	pidFile := filepath.Join(getLogsDir(), fmt.Sprintf("%s.pid", jobName))
	if !isJobRunning(jobName) {
		_ = system.Remove(pidFile)
	}
}

func (m Model) retryRun(item NavItem) (tea.Model, tea.Cmd) {
	switch {
	case item.ReviewRun != nil:
		meta := item.ReviewRun.Meta
		if err := startBackground(review.QueuedPR{Ref: meta.Ref, Title: meta.Title, HeadSHA: meta.HeadSHA}, m.reviewRoot()); err != nil {
			m.showError("REVIEW ERROR", err)
			return m, nil
		}
		m.refreshReviewRuns()
	case item.BragRun != nil:
		period, err := brag.ParsePeriod(item.BragRun.Meta.ID, bragClock().Location())
		if err == nil {
			err = startBragRun(m.bragRoot(), period, item.BragRun.Meta.Regenerate)
		}
		if err != nil {
			m.showError("BRAG ERROR", err)
			return m, nil
		}
		m.refreshBragRuns()
	case item.AutomationRun != nil:
		meta := item.AutomationRun.Meta
		m.automationNoteID, m.automationName, m.automationPhase, m.automationReturnMode = meta.NoteID, meta.Automation, meta.Phase, m.mode
		next, cmd := m.confirmAutomation(tea.KeyMsg{})
		return next, cmd
	default:
		return m, nil
	}
	if m.mode == ViewPreview {
		m.updatePreviewViewport()
	}
	return m, tea.Batch(m.ensureReviewPoll(), m.ensureSyncPulse())
}

func (m Model) landPreviewOnSelection() (tea.Model, tea.Cmd) {
	m.updateScrollOffset()
	m.previewNotice = ""
	item, found := m.selectedNavItem()
	if found && item.Kind == KindGitRepo && item.GitRepo != nil {
		return m.openPreview(item)
	}
	m.resetReviewView()
	m.updatePreviewViewport()
	m.mode = ViewPreview
	cmds := []tea.Cmd{m.changesSinceReviewCmd(m.currentPRItem())}
	if found && item.Kind == KindJobDraft && item.Draft != nil && m.jobRunning(item.Draft.Name) {
		m.jobLogStamp = jobLogStampFor(item.Draft.Name, true)
		cmds = append(cmds, m.ensureJobLogRefresh())
	}
	return m, tea.Batch(cmds...)
}
