package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"app/pkg/config"
	"app/pkg/model"
	"app/pkg/review"
	"app/pkg/sourcecontrol"
	"app/pkg/store"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	previewTabDetails = 0
	previewTabReview  = 1
)

var (
	staleStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#F38BA8")).Bold(true)
	criticalStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#F38BA8")).Bold(true)
	highStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#FAB387")).Bold(true)
	approvedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#A6E3A1")).Bold(true)
	reviewingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#F9E2AF")).Bold(true)
	reviewedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#89B4FA")).Bold(true)
	cursorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#F5E0DC")).Bold(true)
)

type reviewPollTickMsg struct{}

type reviewSubmittedMsg struct {
	event   review.Event
	pr      review.QueuedPR
	payload review.Payload
	err     error
}

type reviewCloneReadyMsg struct {
	dir string
	ref review.PRRef
	err error
}

func tickReviewPollCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return reviewPollTickMsg{}
	})
}

func (m *Model) ensureReviewPoll() tea.Cmd {
	if m.reviewPolling {
		return nil
	}
	m.reviewPolling = true
	return tickReviewPollCmd()
}

func reviewRootFor(cfg *config.Config) string {
	return cfg.ReviewRootDir()
}

func (m Model) reviewRoot() string {
	return reviewRootFor(m.cfg)
}

func queuedFor(item *GitPRItem) (review.QueuedPR, error) {
	if item.PR != nil {
		return *item.PR, nil
	}
	ref, err := review.ParsePRURL(item.URL)
	if err != nil {
		return review.QueuedPR{}, err
	}
	return review.QueuedPR{Ref: ref, Title: item.Title}, nil
}

func (m Model) currentPRItem() *GitPRItem {
	navItems := m.allNavItems()
	if len(navItems) == 0 || m.selected >= len(navItems) {
		return nil
	}
	item := navItems[m.selected]
	if item.Kind == KindPendingGit && item.PendingGitPR != nil {
		return item.PendingGitPR
	}
	return nil
}

func (m *Model) resetReviewView() {
	m.previewTab = previewTabDetails
	m.reviewCursor = 0
	m.reviewSelected = make(map[int]bool)
	m.reviewNotice = ""
	m.previewViewport = viewport.New(0, 0)
}

func myReviewState(pr review.QueuedPR) (review.PRState, bool) {
	switch pr.MyLastReviewState {
	case "APPROVED":
		return review.StateApproved, true
	case "CHANGES_REQUESTED":
		return review.StateChangesRequested, true
	case "COMMENTED":
		return review.StateCommented, true
	}
	return "", false
}

var readLocalReview = func(stateDir string) localReviewState {
	pid, _ := review.RunningPID(stateDir)
	finishedAt, finished := review.LocalReviewFinishedAt(stateDir)
	return localReviewState{status: review.Status(stateDir), finishedAt: finishedAt, finished: finished, pid: pid}
}

func (m Model) localReviewFor(stateDir string) localReviewState {
	if state, cached := m.localReviews[stateDir]; cached {
		return state
	}
	return readLocalReview(stateDir)
}

func (m *Model) refreshLocalReviews() {
	root := m.reviewRoot()
	states := make(map[string]localReviewState)
	addState := func(ref review.PRRef) {
		stateDir := review.StateDir(root, ref)
		if _, seen := states[stateDir]; !seen {
			states[stateDir] = readLocalReview(stateDir)
		}
	}
	for _, items := range [][]GitPRItem{m.ghPendingPRs, m.ghReviewedToday, m.ghReviewedYesterday} {
		for i := range items {
			if queued, err := queuedFor(&items[i]); err == nil {
				addState(queued.Ref)
			}
		}
	}
	for _, run := range m.reviewRuns {
		addState(run.Meta.Ref)
	}
	m.localReviews = states
}

func (m Model) prState(item *GitPRItem) review.PRState {
	queued, err := queuedFor(item)
	if err != nil {
		return review.StatePending
	}
	local := m.localReviewFor(review.StateDir(m.reviewRoot(), queued.Ref))
	if local.status == review.RunRunning {
		return review.StateReviewing
	}
	myState, reviewedOnGitHub := myReviewState(queued)
	actedSinceLocalReview := reviewedOnGitHub && (!local.finished || queued.MyLastReviewAt.After(local.finishedAt))
	switch {
	case actedSinceLocalReview:
		return myState
	case local.status == review.RunDone:
		return review.StateReviewed
	case local.status == review.RunFailed:
		return review.StateFailed
	case reviewedOnGitHub:
		return myState
	}
	return review.StatePending
}

func stateStyle(state review.PRState) lipgloss.Style {
	switch state {
	case review.StateApproved:
		return approvedStyle
	case review.StateReviewing:
		return reviewingStyle
	case review.StateReviewed:
		return reviewedStyle
	case review.StateFailed, review.StateChangesRequested:
		return staleStyle
	case review.StateCommented:
		return yellowBadgeStyle
	}
	return mutedStyle
}

func (m Model) prStateBadge(item *GitPRItem) string {
	state := m.prState(item)
	return stateStyle(state).Render(strings.ToUpper(string(state)))
}

func shortAge(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func (m Model) renderPRTag(item *GitPRItem, selected bool) string {
	return joinTags(m.prTags(item, selected))
}

func (m Model) renderPreviewTabs() string {
	names := []string{"Details", "Review"}
	var rendered []string
	for i, name := range names {
		if i == m.previewTab {
			rendered = append(rendered, tabActiveStyle.Render(name))
		} else {
			rendered = append(rendered, tabInactiveStyle.Render(name))
		}
	}
	return strings.Join(rendered, " ")
}

func (m Model) reviewFooterItems(item *GitPRItem) []footerItem {
	return footerItemsFrom(m.prPreviewBindings(item))
}

func (m Model) selectedCount() int {
	count := 0
	for _, picked := range m.reviewSelected {
		if picked {
			count++
		}
	}
	return count
}

var loadReviewReport = review.Load

type reviewReportMemo struct {
	dir        string
	finishedAt time.Time
	report     *review.Report
	findings   []review.Finding
	meta       review.Meta
	metaErr    error
	err        error
}

func (m Model) doneReview(dir string) (*reviewReportMemo, bool) {
	local := m.localReviewFor(dir)
	if local.status != review.RunDone {
		return nil, false
	}
	memo := m.reviewReports
	if memo == nil {
		memo = &reviewReportMemo{}
	}
	if memo.dir != dir || !memo.finishedAt.Equal(local.finishedAt) || (memo.report == nil && memo.err == nil) {
		*memo = reviewReportMemo{dir: dir, finishedAt: local.finishedAt}
		memo.report, memo.err = loadReviewReport(dir)
		if memo.err == nil {
			memo.findings = review.Flatten(review.GroupBySeverity(memo.report.Findings))
		}
		memo.meta, memo.metaErr = review.ReadMeta(dir)
	}
	return memo, true
}

func (m Model) loadFindings(item *GitPRItem) (*review.Report, []review.Finding) {
	queued, err := queuedFor(item)
	if err != nil {
		return nil, nil
	}
	done, ok := m.doneReview(review.StateDir(m.reviewRoot(), queued.Ref))
	if !ok || done.err != nil {
		return nil, nil
	}
	return done.report, done.findings
}

func (m Model) selectedFindings(item *GitPRItem) []review.Finding {
	_, findings := m.loadFindings(item)
	var selected []review.Finding
	for i, finding := range findings {
		if m.reviewSelected[i] {
			selected = append(selected, finding)
		}
	}
	return selected
}

func (m *Model) setPRPreviewContent(item *GitPRItem, width, height int) {
	previousOffset := m.previewViewport.YOffset
	height -= 2
	if m.reviewNotice != "" {
		height -= 2
	}
	if height < 3 {
		height = 3
	}
	m.previewViewport = viewport.New(width, height)
	if m.previewTab == previewTabReview {
		_, findings := m.loadFindings(item)
		m.previewFindingsCount = len(findings)
		content, cursorLine := m.renderReviewContent(item, width)
		m.previewViewport.SetContent(content)
		m.previewViewport.SetYOffset(previousOffset)
		if cursorLine >= 0 {
			if cursorLine < m.previewViewport.YOffset {
				m.previewViewport.SetYOffset(cursorLine)
			} else if cursorLine >= m.previewViewport.YOffset+height-2 {
				m.previewViewport.SetYOffset(cursorLine - height + 3)
			}
		}
		return
	}
	m.previewViewport.SetContent(renderMarkdown(m.renderDetailsMarkdown(item), width))
	m.previewViewport.SetYOffset(previousOffset)
}

type relatedHistoryMsg struct {
	url     string
	history string
}

var gitLogForFiles = func(dir string, files []string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	args := append([]string{"-C", dir, "log", "-n", "5", "--format=%h %s", "origin/HEAD", "--"}, files...)
	out, err := exec.CommandContext(ctx, "git", args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (m *Model) relatedHistoryCmd(item *GitPRItem) tea.Cmd {
	if item == nil || item.PR == nil {
		return nil
	}
	pr := item.PR
	if _, cached := m.contextCache[pr.Ref.URL]; cached || m.historyRequested[pr.Ref.URL] {
		return nil
	}
	dir := review.CloneDir(m.reviewRoot(), pr.Ref)
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil || len(pr.Files) == 0 {
		return nil
	}
	files := pr.Files
	if len(files) > 20 {
		files = files[:20]
	}
	if m.historyRequested == nil {
		m.historyRequested = make(map[string]bool)
	}
	m.historyRequested[pr.Ref.URL] = true
	url := pr.Ref.URL
	return func() tea.Msg {
		return relatedHistoryMsg{url: url, history: gitLogForFiles(dir, files)}
	}
}

func (m Model) handleRelatedHistory(msg relatedHistoryMsg) (tea.Model, tea.Cmd) {
	delete(m.historyRequested, msg.url)
	if m.contextCache == nil {
		m.contextCache = make(map[string]string)
	}
	m.contextCache[msg.url] = msg.history
	if item := m.currentPRItem(); m.mode == ViewPreview && item != nil && item.URL == msg.url {
		m.updatePreviewViewport()
	}
	return m, nil
}

func (m Model) relatedHistory(pr review.QueuedPR) (string, bool) {
	history, cached := m.contextCache[pr.Ref.URL]
	return history, m.historyRequested[pr.Ref.URL] && !cached
}

func (m Model) renderDetailsMarkdown(item *GitPRItem) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", item.Title)
	fmt.Fprintf(&b, "- **Repository:** %s\n- **URL:** %s\n", item.Repository, item.URL)
	pr := item.PR
	if pr == nil {
		b.WriteString("\nPress **[Enter]** on the dashboard to open this Pull Request in your browser.\n")
		return b.String()
	}
	now := time.Now()
	wait := shortAge(now.Sub(pr.RequestedAt))
	if pr.IsStale(now) {
		wait += " (stale)"
	}
	ci := pr.CIState
	if ci == "" {
		ci = "unknown"
	}
	fmt.Fprintf(&b, "- **Author:** %s\n", pr.Author)
	fmt.Fprintf(&b, "- **State:** %s · **CI:** %s · **Waiting:** %s\n", m.prState(item), ci, wait)
	fmt.Fprintf(&b, "- **Size:** +%d / -%d in %d files\n", pr.Additions, pr.Deletions, pr.ChangedFiles)
	if pr.JiraKey != "" {
		if m.cfg != nil && m.cfg.JiraBaseURL != "" {
			fmt.Fprintf(&b, "- **Jira:** [%s](%s%s)\n", pr.JiraKey, m.cfg.JiraBaseURL, pr.JiraKey)
		} else {
			fmt.Fprintf(&b, "- **Jira:** %s\n", pr.JiraKey)
		}
	}
	if pr.CodeOwner {
		owners := "yes"
		if len(pr.OwnerTeams) > 0 {
			owners = strings.Join(pr.OwnerTeams, ", ")
		}
		fmt.Fprintf(&b, "- **Code owners requested:** %s\n", owners)
	}
	if !pr.MyLastReviewAt.IsZero() {
		fmt.Fprintf(&b, "- **Your last review:** %s ago\n", shortAge(now.Sub(pr.MyLastReviewAt)))
	}
	if len(pr.Files) > 0 {
		b.WriteString("\n## Changed files\n\n")
		for _, file := range pr.Files {
			fmt.Fprintf(&b, "- %s\n", file)
		}
		if pr.ChangedFiles > len(pr.Files) {
			fmt.Fprintf(&b, "- … and %d more\n", pr.ChangedFiles-len(pr.Files))
		}
	}
	b.WriteString("\n## Recent changes to these files\n\n")
	history, loading := m.relatedHistory(*pr)
	if history != "" {
		for _, line := range strings.Split(history, "\n") {
			fmt.Fprintf(&b, "- %s\n", line)
		}
	} else if loading {
		b.WriteString("Loading…\n")
	} else {
		b.WriteString("Available once a review clone exists (press **o** or run a review).\n")
	}
	return b.String()
}

func lastLines(text string, count int) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) > count {
		lines = lines[len(lines)-count:]
	}
	return strings.Join(lines, "\n")
}

func severityStyle(severity string) lipgloss.Style {
	switch severity {
	case "critical":
		return criticalStyle
	case "high":
		return highStyle
	}
	return reviewedStyle
}

func (m Model) renderReviewContent(item *GitPRItem, width int) (string, int) {
	queued, err := queuedFor(item)
	if err != nil {
		return staleStyle.Render(err.Error()), -1
	}
	dir := review.StateDir(m.reviewRoot(), queued.Ref)
	var b strings.Builder
	cursorLine := -1
	lineCount := func() int { return strings.Count(b.String(), "\n") }

	fmt.Fprintf(&b, "%s %s\n\n", sectionTitleStyle.Render("State:"), m.prStateBadge(item))

	switch m.localReviewFor(dir).status {
	case review.RunIdle:
		b.WriteString(mutedStyle.Render("No Claude review yet. Press r to clone the PR, install dependencies and run review-toolkit.") + "\n")
	case review.RunRunning:
		b.WriteString(reviewingStyle.Render("Claude is reviewing this PR in the background…") + "\n\n")
		b.WriteString(mutedStyle.Render(lastLines(review.ReadLog(dir), 12)) + "\n")
	case review.RunFailed:
		b.WriteString(staleStyle.Render("Review failed. Press r to retry.") + "\n\n")
		b.WriteString(mutedStyle.Render(lastLines(review.ReadLog(dir), 12)) + "\n")
	case review.RunDone:
		done, _ := m.doneReview(dir)
		if done.err != nil {
			b.WriteString(staleStyle.Render(done.err.Error()) + "\n")
			break
		}
		report, meta := done.report, done.meta
		if done.metaErr == nil && queued.HeadSHA != "" && meta.HeadSHA != "" && meta.HeadSHA != queued.HeadSHA {
			b.WriteString(staleStyle.Render("PR has new commits since this review — press r to re-run.") + "\n\n")
		}
		fmt.Fprintf(&b, "%s %s · %d findings · %d selected\n", sectionTitleStyle.Render("Recommendation:"), report.Recommendation, len(report.Findings), m.selectedCount())
		if len(report.Findings) == 0 {
			b.WriteString("\n" + approvedStyle.Render("No actionable findings. Press a to approve.") + "\n")
			break
		}
		bodyStyle := lipgloss.NewStyle().Width(width - 8).Foreground(lipgloss.Color("#A6ADC8"))
		index := 0
		for _, group := range review.GroupBySeverity(report.Findings) {
			fmt.Fprintf(&b, "\n%s\n", severityStyle(group.Severity).Render(fmt.Sprintf("%s (%d)", strings.ToUpper(group.Severity), len(group.Findings))))
			for _, finding := range group.Findings {
				box := "☐"
				if m.reviewSelected[index] {
					box = checkDone.String()
				}
				pointer := "  "
				title := finding.Title
				if index == m.reviewCursor {
					pointer = cursorStyle.Render("▸ ")
					title = selectedSummaryStyle.Render(title)
					cursorLine = lineCount()
				}
				location := ""
				if finding.Path != "" {
					location = fmt.Sprintf("%s:%d", finding.Path, finding.Line)
				}
				fmt.Fprintf(&b, "%s%s %s\n", pointer, box, title)
				if location != "" {
					fmt.Fprintf(&b, "      %s\n", dimBlueText.Render(location))
				}
				if finding.Body != "" {
					b.WriteString(indent(bodyStyle.Render(finding.Body), "      ") + "\n")
				}
				if finding.Suggestion != "" {
					b.WriteString(indent(bodyStyle.Render("Suggestion: "+finding.Suggestion), "      ") + "\n")
				}
				index++
			}
		}
	}
	return b.String(), cursorLine
}

func indent(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}

func (m Model) refreshPreview() Model {
	m.updatePreviewViewport()
	return m
}

func (m Model) reviewedHeadSHA(queued review.QueuedPR) string {
	if m.selectedCount() > 0 {
		if meta, err := review.ReadMeta(review.StateDir(m.reviewRoot(), queued.Ref)); err == nil && meta.HeadSHA != "" {
			return meta.HeadSHA
		}
	}
	return queued.HeadSHA
}

func (m Model) buildPayload(item *GitPRItem, event review.Event, body string) (review.QueuedPR, review.Payload, error) {
	queued, err := queuedFor(item)
	if err != nil {
		return queued, review.Payload{}, err
	}
	payload, err := review.BuildPayload(event, m.selectedFindings(item), body, m.reviewedHeadSHA(queued))
	return queued, payload, err
}

func (m Model) beginConfirm(item *GitPRItem, event review.Event, body string) (bool, tea.Model, tea.Cmd) {
	if _, _, err := m.buildPayload(item, event, body); err != nil {
		m.reviewNotice = err.Error()
		m.mode = ViewPreview
		return true, m.refreshPreview(), nil
	}
	m.reviewEvent = event
	m.reviewBody = body
	m.mode = ViewReviewConfirm
	return true, m, nil
}

func (m Model) startReview(item *GitPRItem) (bool, tea.Model, tea.Cmd) {
	queued, err := queuedFor(item)
	if err != nil {
		m.reviewNotice = err.Error()
		return true, m.refreshPreview(), nil
	}
	if err := startBackground(queued, m.reviewRoot()); err != nil {
		m.reviewNotice = err.Error()
		return true, m.refreshPreview(), nil
	}
	m.reviewSelected = make(map[int]bool)
	m.reviewCursor = 0
	m.reviewNotice = "Review started in the background"
	m.refreshReviewRuns()
	pollCmds := tea.Batch(m.ensureReviewPoll(), m.ensureSyncPulse())
	return true, m.refreshPreview(), pollCmds
}

func (m Model) openClone(item *GitPRItem) (bool, tea.Model, tea.Cmd) {
	queued, err := queuedFor(item)
	if err != nil {
		m.reviewNotice = err.Error()
		return true, m.refreshPreview(), nil
	}
	if os.Getenv("TMUX") == "" {
		m.reviewNotice = "Not inside tmux; cannot open a new nvim window"
		return true, m.refreshPreview(), nil
	}
	root, sessionCtx := m.reviewRoot(), m.sessionCtx
	if sessionCtx == nil {
		sessionCtx = context.Background()
	}
	m.reviewNotice = "Opening PR clone in nvim…"
	return true, m.refreshPreview(), func() tea.Msg {
		cloneDir, err := sourcecontrol.ClonePR(sessionCtx, root, queued)
		return reviewCloneReadyMsg{dir: cloneDir, ref: queued.Ref, err: err}
	}
}

func openInNvim(dir string, ref review.PRRef) error {
	windowName := fmt.Sprintf("%s#%d", ref.Repo, ref.Number)
	out, err := exec.Command("tmux", "new-window", "-c", dir, "-n", windowName, "nvim", ".").CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux new-window: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func (m Model) handleCloneReady(msg reviewCloneReadyMsg) (tea.Model, tea.Cmd) {
	if errors.Is(msg.err, sourcecontrol.ErrCloneInProgress) {
		m.reviewNotice = "The running review is still cloning; try again shortly"
	} else if msg.err != nil {
		m.reviewNotice = "Clone failed: " + msg.err.Error()
	} else if err := openInNvim(msg.dir, msg.ref); err != nil {
		m.reviewNotice = err.Error()
	} else {
		m.reviewNotice = "Opened clone in a new tmux window"
	}
	delete(m.contextCache, msg.ref.URL)
	var historyCmd tea.Cmd
	if m.mode == ViewPreview {
		historyCmd = m.relatedHistoryCmd(m.currentPRItem())
		m.updatePreviewViewport()
	}
	return m, historyCmd
}

func eventLabel(event review.Event) string {
	switch event {
	case review.EventApprove:
		return "Approve"
	case review.EventRequestChanges:
		return "Request changes on"
	}
	return "Post review comments on"
}

func (m Model) handleReviewSubmitted(msg reviewSubmittedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.reviewNotice = ""
		m.showError("REVIEW ERROR", msg.err)
		return m, nil
	}
	switch msg.event {
	case review.EventApprove:
		m.reviewNotice = "Approved ✓"
	case review.EventRequestChanges:
		m.reviewNotice = "Changes requested ✓"
	default:
		m.reviewNotice = "Comments posted ✓"
	}
	m.reviewSelected = make(map[int]bool)
	m.reviewBody = ""
	if m.mode == ViewPreview {
		m.updatePreviewViewport()
	}
	var notesCmd tea.Cmd
	if msg.pr.Ref.URL != "" {
		notesCmd = reviewNotesCmd(m.store, []review.ActivityPR{submittedReviewRecord(msg, time.Now())})
	}
	if msg.event == review.EventApprove {
		return m, tea.Batch(notesCmd, tea.Tick(approvalSyncDelay, func(time.Time) tea.Msg { return approvalSyncMsg{} }))
	}
	return m, tea.Batch(notesCmd, m.startLoadGitStatsCmd(false))
}

type approvalSyncMsg struct{}

var approvalSyncDelay = 3 * time.Second

func submittedReviewRecord(msg reviewSubmittedMsg, reviewedAt time.Time) review.ActivityPR {
	record := sourcecontrol.ReviewRecord(msg.pr, submittedReviewStates[msg.event], reviewedAt)
	if msg.event == review.EventRequestChanges {
		record.Comments = strings.TrimSpace(review.FoldIntoBody(msg.payload).Body)
	}
	return record
}

var submittedReviewStates = map[review.Event]string{review.EventApprove: "APPROVED", review.EventRequestChanges: "CHANGES_REQUESTED", review.EventComment: "COMMENTED"}

func (m Model) afterGitSection(cmds []tea.Cmd) (tea.Model, tea.Cmd) {
	if m.mode == ViewPreview {
		m.updatePreviewViewport()
	}
	if m.anyReviewRunning() {
		cmds = append(cmds, m.ensureReviewPoll())
	}
	return m, tea.Batch(cmds...)
}

func (m Model) anyReviewRunning() bool {
	for _, run := range m.reviewRuns {
		if run.Status == review.RunRunning {
			return true
		}
	}
	for _, state := range m.localReviews {
		if state.status == review.RunRunning {
			return true
		}
	}
	return false
}

func (m Model) handleReviewPoll() (tea.Model, tea.Cmd) {
	selectedKey, selectedOccurrence := m.selectedNavKey()
	m.refreshReviewRuns()
	m.refreshBragRuns()
	m.restoreSelection(selectedKey, selectedOccurrence)
	inPRPreview := m.mode == ViewPreview && m.currentPRItem() != nil
	if inPRPreview {
		m.updatePreviewViewport()
	}
	if m.mode == ViewBragView {
		m.reloadBragViewIfChanged()
	}
	if m.anyReviewRunning() || m.anyBragRunning() {
		return m, tickReviewPollCmd()
	}
	m.reviewPolling = false
	return m, nil
}

func (m Model) renderReviewConfirm(modalWidth int) string {
	item := m.currentPRItem()
	title := modalTitleStyle.Render(" CONFIRM REVIEW ")
	if m.reviewEvent == review.EventRequestChanges {
		title = deleteTitleStyle.Render(" CONFIRM REQUEST CHANGES ")
	}
	var prompt strings.Builder
	if item != nil {
		fmt.Fprintf(&prompt, "%s %s?\n\n%s\n", eventLabel(m.reviewEvent), item.Repository, item.Title)
	}
	if count := m.selectedCount(); count > 0 {
		fmt.Fprintf(&prompt, "\nIncludes %d selected comment(s).\n", count)
	}
	if m.reviewBody != "" {
		fmt.Fprintf(&prompt, "\nComment:\n%s\n", mutedStyle.Render(m.reviewBody))
	}
	footer := renderModalFooter(footerItemsFrom(reviewConfirmBindings()), modalWidth-6)
	content := lipgloss.JoinVertical(lipgloss.Left, title, "", strings.TrimRight(prompt.String(), "\n"), "", footer)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, fitPopup(modalStyle.Width(modalWidth).Render(content), m.width, m.height))
}

func (m Model) renderRejectComment(modalWidth int) string {
	title := deleteTitleStyle.Render(" REQUEST CHANGES ")
	hint := mutedStyle.Render("No comments selected. Write a comment for the author.")
	m.rejectInput.SetWidth(modalWidth - 6)
	m.rejectInput.SetHeight(8)
	parts := []string{title, "", hint, "", m.rejectInput.View(), ""}
	if m.reviewNotice != "" {
		parts = append(parts, staleStyle.Render(m.reviewNotice), "")
	}
	parts = append(parts, renderModalFooter(footerItemsFrom(rejectCommentBindings()), modalWidth-6))
	content := lipgloss.JoinVertical(lipgloss.Left, parts...)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, fitPopup(modalStyle.Width(modalWidth).Render(content), m.width, m.height))
}

func (m *Model) refreshReviewRuns() {
	m.reviewRuns = review.ListRuns(m.reviewRoot())
	m.refreshLocalReviews()
}

func reviewRunLabel(run review.ReviewRun) string {
	return fmt.Sprintf("%s:%d", run.Meta.Ref.Repo, run.Meta.Ref.Number)
}

func (m Model) renderReviewRunningIndicator() string {
	return m.renderPulseIndicator("reviewing...")
}

func (m Model) renderPulseIndicator(label string) string {
	return m.renderPulseDot() + " " + reviewingStyle.Render(label)
}

func (m Model) renderPulseDot() string {
	pulseColors := []string{
		"#F9E2AF", "#EED49F", "#F5BDE6", "#C6A0F6",
		"#89B4FA", "#74C7EC", "#8BD5CA", "#A6E3A1",
	}
	idx := m.syncPulseFrame % len(pulseColors)
	return lipgloss.NewStyle().Foreground(lipgloss.Color(pulseColors[idx])).Bold(true).Render("●")
}

func (m Model) renderReviewRunRow(run review.ReviewRun, selected bool, width int) string {
	rightColWidth := 26
	if width < 60 {
		rightColWidth = 20
	}
	leftWidth := max(width-rightColWidth, 15)
	prefix := "   "
	icon := amberDiamond.Render()
	label := reviewRunLabel(run)
	labelText := itemStyle.Render(label)
	if selected {
		labelText = selectedSummaryStyle.Render(label)
	}
	leftBlock := fmt.Sprintf("%s%s %s", prefix, icon, labelText)
	leftPadding := max(leftWidth-lipgloss.Width(leftBlock), 0)
	rightBlock := stateStyle(review.StateFailed).Render("failed")
	if run.Status == review.RunRunning {
		rightBlock = m.renderReviewRunningIndicator()
	}
	return fmt.Sprintf("%s%s%s\n", leftBlock, safeRepeat(" ", leftPadding), rightBlock)
}

func reviewRunPreview(root string, run review.ReviewRun) string {
	status := "FAILED"
	if run.Status == review.RunRunning {
		status = "RUNNING"
	}
	logText := strings.TrimSpace(review.ReadLog(review.StateDir(root, run.Meta.Ref)))
	if logText == "" {
		logText = "(no log output yet)"
	}
	return fmt.Sprintf("# Review: %s (%s)\n\n%s\n\n```\n%s\n```", reviewRunLabel(run), status, run.Meta.Ref.URL, logText)
}

func reviewNoteID(repository string, number int) string {
	return fmt.Sprintf("%s:%d", repository, number)
}

func reviewNoteSummary(state, repository string, number int, title string) string {
	prefix := "Reviewed: Commented"
	switch state {
	case "APPROVED":
		prefix = approvedSummaryPrefix
	case "CHANGES_REQUESTED":
		prefix = "Reviewed: RequestedChanges"
	}
	return fmt.Sprintf("%s:%s %s", prefix, reviewNoteID(repository, number), title)
}

const approvedSummaryPrefix = "Approved"

func isApprovedSummary(summary string) bool {
	return strings.HasPrefix(summary, approvedSummaryPrefix+":")
}

func reviewNoteStatus(state string) model.Status {
	if state == "APPROVED" {
		return model.StatusDone
	}
	return model.StatusActive
}

const reviewHistoryTimeFormat = "2006-01-02 15:04"

const requestedChangesHeading = "### Requested changes "

func requestedChangesBlock(comments string, reviewedAt time.Time) string {
	return requestedChangesHeading + reviewedAt.Local().Format(reviewHistoryTimeFormat) + "\n\n" + comments
}

func insertAbovePRURL(body, prURL, block string) string {
	at := strings.Index(body, requestedChangesHeading)
	if at < 0 {
		at = strings.LastIndex(body, prURL)
	}
	if at < 0 {
		return block + "\n\n" + strings.TrimLeft(body, "\n")
	}
	above := body[:at]
	if above != "" && !strings.HasSuffix(above, "\n") {
		above += "\n\n"
	}
	return above + block + "\n\n" + body[at:]
}

func pushReviewLine(note *model.Note, summary string, previousAt time.Time) {
	note.Body = previousAt.Local().Format(reviewHistoryTimeFormat) + " " + note.Summary + "\n" + note.Body
	note.Summary = summary
}

var duplicateReviewWindow = 2 * time.Minute

func isRepeatOfLatest(note *model.Note, summary string, reviewedAt time.Time) bool {
	if note.Summary != summary {
		return false
	}
	gap := reviewedAt.Sub(note.Updated)
	return gap < duplicateReviewWindow && gap > -duplicateReviewWindow
}

var reviewNotesMu sync.Mutex

func prReviewNotesByID(noteStore *store.NoteStore) (map[string]*model.Note, error) {
	notes, err := noteStore.List()
	if err != nil {
		return nil, err
	}
	byID := make(map[string]*model.Note, len(notes))
	for _, note := range notes {
		byID[note.ID] = note
	}
	return byID, nil
}

func reviewNotesCmd(noteStore *store.NoteStore, reviews []review.ActivityPR) tea.Cmd {
	if len(reviews) == 0 {
		return nil
	}
	ordered := slices.Clone(reviews)
	slices.SortStableFunc(ordered, func(a, b review.ActivityPR) int { return a.ReviewedAt.Compare(b.ReviewedAt) })
	return func() tea.Msg {
		reviewNotesMu.Lock()
		defer reviewNotesMu.Unlock()
		byID, err := prReviewNotesByID(noteStore)
		if err != nil {
			return loadNotesMsg{err: err}
		}
		saved := false
		for _, pr := range ordered {
			noteID := reviewNoteID(pr.Repository, pr.Number)
			reviewedAt := pr.ReviewedAt.Local()
			summary := reviewNoteSummary(pr.State, pr.Repository, pr.Number, pr.Title)
			note, exists := byID[noteID]
			switch {
			case !exists:
				note = &model.Note{
					ID:      noteID,
					Created: reviewedAt,
					Source:  model.SourcePRReview,
					Summary: summary,
					Repo:    pr.Repository,
					Body:    pr.URL,
				}
			case note.Source != model.SourcePRReview, !reviewedAt.After(note.Updated):
				continue
			case isRepeatOfLatest(note, summary, reviewedAt):
				if pr.Comments == "" || strings.Contains(note.Body, pr.Comments) {
					continue
				}
			default:
				pushReviewLine(note, summary, note.Updated)
			}
			if pr.Comments != "" {
				note.Body = insertAbovePRURL(note.Body, pr.URL, requestedChangesBlock(pr.Comments, reviewedAt))
			}
			note.Status = reviewNoteStatus(pr.State)
			note.Updated = reviewedAt
			if err := noteStore.Save(note); err != nil {
				return loadNotesMsg{err: err}
			}
			byID[noteID] = note
			saved = true
		}
		if !saved {
			return nil
		}
		notes, err := noteStore.List()
		return loadNotesMsg{notes: notes, err: err}
	}
}

func reopenApprovedNotesCmd(noteStore *store.NoteStore, pending []GitPRItem, requestedSince time.Time) tea.Cmd {
	if len(pending) == 0 {
		return nil
	}
	return func() tea.Msg {
		reviewNotesMu.Lock()
		defer reviewNotesMu.Unlock()
		byID, err := prReviewNotesByID(noteStore)
		if err != nil {
			return loadNotesMsg{err: err}
		}
		reopened := false
		for _, item := range pending {
			note, exists := byID[reviewNoteID(item.Repository, item.Number)]
			if !exists || note.Source != model.SourcePRReview || note.Status != model.StatusDone || !isApprovedSummary(note.Summary) || !note.Updated.Before(requestedSince) {
				continue
			}
			note.Status = model.StatusActive
			note.Updated = time.Now()
			if err := noteStore.Save(note); err != nil {
				return loadNotesMsg{err: err}
			}
			reopened = true
		}
		if !reopened {
			return nil
		}
		notes, err := noteStore.List()
		return loadNotesMsg{notes: notes, err: err}
	}
}

const (
	reviewActionStart = "start"
	reviewActionStop  = "stop"
)

var (
	startBackground = review.StartBackground
	stopReview      = review.Stop
)

func (m Model) refFor(item *GitPRItem) review.PRRef {
	queued, err := queuedFor(item)
	if err != nil {
		return review.PRRef{}
	}
	return queued.Ref
}

func (m Model) reviewPIDFor(item *GitPRItem) (int, bool) {
	ref := m.refFor(item)
	if ref.URL == "" {
		return 0, false
	}
	pid := m.localReviewFor(review.StateDir(m.reviewRoot(), ref)).pid
	return pid, pid > 0
}

func (m Model) beginReviewRunConfirm(action string, ref review.PRRef) (tea.Model, tea.Cmd) {
	if ref.URL == "" {
		return m, nil
	}
	m.reviewRunAction = action
	m.reviewRunTarget = ref
	m.reviewRunReturnMode = m.mode
	m.mode = ViewReviewRunConfirm
	return m, nil
}

func (m Model) renderReviewRunConfirm(modalWidth int) string {
	label := fmt.Sprintf("%s:%d", m.reviewRunTarget.Repo, m.reviewRunTarget.Number)
	title := modalTitleStyle.Render(" START REVIEW ")
	prompt := fmt.Sprintf("Start a Claude review of %s?\n\nFresh clone, install dependencies, then review.", label)
	if m.reviewRunAction == reviewActionStop {
		title = deleteTitleStyle.Render(" STOP REVIEW ")
		prompt = fmt.Sprintf("Stop the running review of %s?", label)
		if pid := m.localReviewFor(review.StateDir(m.reviewRoot(), m.reviewRunTarget)).pid; pid > 0 {
			prompt += fmt.Sprintf("\n\nPID %d and its child processes will be terminated.", pid)
		}
	}
	footer := renderModalFooter(footerItemsFrom(reviewRunConfirmBindings()), modalWidth-6)
	content := lipgloss.JoinVertical(lipgloss.Left, title, "", prompt, "", footer)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, fitPopup(modalStyle.Width(modalWidth).Render(content), m.width, m.height))
}

func reviewRunFooterItems(running bool) []footerItem {
	return footerItemsFrom(reviewRunPreviewBindings(running))
}
