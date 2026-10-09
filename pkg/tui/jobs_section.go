package tui

import (
	"fmt"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/brag"
	"github.com/AnudeepChPaul/digest/pkg/review"

	tea "github.com/charmbracelet/bubbletea"
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
	if !found || item.Kind != KindJobDraft || item.Draft == nil || isJobRunning(item.Draft.Name) {
		return m, nil
	}
	m.jobToExecute = item.Draft.Name
	m.deleteTargetNotes = nil
	m.deleteReturnMode = m.mode
	m.mode = ViewDeleteConfirm
	return m, nil
}

func (m Model) dryRunSelectedJob(tea.KeyMsg) (tea.Model, tea.Cmd) {
	item, found := m.selectedNavItem()
	if !found || item.Kind != KindJobDraft || item.Draft == nil || isJobRunning(item.Draft.Name) || !jobHasDryRun(item.Draft.Name, item.Draft.DryRunCommand) {
		return m, nil
	}
	return m, m.startJobDryRunCmd(item.Draft.Name)
}

func (m Model) stopSelectedItem(tea.KeyMsg) (tea.Model, tea.Cmd) {
	item, found := m.selectedNavItem()
	if !found {
		return m, nil
	}
	switch {
	case item.Kind == KindReviewRun && item.ReviewRun != nil && item.ReviewRun.Status == review.RunRunning:
		return m.beginReviewRunConfirm(reviewActionStop, item.ReviewRun.Meta.Ref)
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
	case item.Kind == KindJobDraft && item.Draft != nil && isJobRunning(item.Draft.Name):
		m.jobToAbort = item.Draft.Name
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
		statusText := "need to act"
		if draft.HasRunDryRun && draft.ExitCode == 0 {
			statusText = "success"
		}

		rightBlock = fmt.Sprintf("%s   %s", dimBlueText.Render("#job"), mutedStyle.Render(statusText))
	}
	return renderJobStyleRow(icon, draft.Name, rightBlock, selected, width)
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
