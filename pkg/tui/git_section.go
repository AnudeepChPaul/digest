package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/achandrapaul/digest/pkg/review"
	"github.com/achandrapaul/digest/pkg/sourcecontrol"
	"github.com/achandrapaul/digest/pkg/tui/textarea"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type gitSection struct{}

type PendingRepoGroup struct {
	Name  string
	Items []GitPRItem
}

type autoSyncTickMsg time.Time

type daySyncDueMsg struct {
	generation int
}

const daySyncDelay = 2 * time.Second

type gitDay = sourcecontrol.Day

const (
	gitDayToday     = sourcecontrol.Today
	gitDayYesterday = sourcecontrol.Yesterday
)

const gitSectionCount = sourcecontrol.SectionCount

type gitDaySectionMsg struct {
	generation  int
	day         gitDay
	date        string
	reviewed    []GitPRItem
	reviews     []review.ActivityPR
	details     map[string]json.RawMessage
	failedHosts []string
	err         error
	sections    <-chan sourcecontrol.Section
}

type gitMyPRsMsg struct {
	generation  int
	partOfSync  bool
	prs         []review.QueuedPR
	closed      map[string]string
	failedHosts []string
	err         error
	sections    <-chan sourcecontrol.Section
}

type gitPendingMsg struct {
	generation  int
	startedAt   time.Time
	pending     []GitPRItem
	details     map[string]json.RawMessage
	failedHosts []string
	err         error
	sections    <-chan sourcecontrol.Section
}

func autoSyncTickCmd(intervalSecs int) tea.Cmd {
	if intervalSecs <= 0 {
		return nil
	}
	return tea.Tick(time.Duration(intervalSecs)*time.Second, func(t time.Time) tea.Msg {
		return autoSyncTickMsg(t)
	})
}

func (m *Model) loadGitOnStartup() {
	m.git.loadingGit = true
	cache, cacheLoaded := loadGitCache()
	if cacheLoaded {
		if cache.PendingSort != nil {
			m.git.pendingSort, m.git.pendingSortChosen = *cache.PendingSort, true
		}
		m.applyGitCache(cache)
	}
	if seen, _, err := loadMyPRsSeen(myPRsSeenPath(m.cfg.Root())); err == nil {
		m.git.knownMyPRs = knownMyPRRefs(seen)
	}
	m.git.syncOnLoad = !cacheLoaded || !cache.hasDataFor(m.currentDate) || cache.PreviousDay != m.git.fetchedPreviousDay.Format("2006-01-02")
	m.git.loadingMyPRs = true
	if m.git.syncOnLoad {
		m.beginGitFetch()
	} else {
		m.git.loadingGit = false
	}
}

func (m Model) getPendingGitGroups() []PendingRepoGroup {
	if !m.cfg.GitEnabled() {
		return nil
	}
	var groups []PendingRepoGroup
	groupMap := make(map[string]int)

	for _, item := range m.git.pendingGitAction {
		rName := item.Repository
		if rName == "" {
			rName = "general"
		}
		idx, exists := groupMap[rName]
		if !exists {
			groupMap[rName] = len(groups)
			groups = append(groups, PendingRepoGroup{Name: rName, Items: []GitPRItem{item}})
		} else {
			groups[idx].Items = append(groups[idx].Items, item)
		}
	}

	return groups
}

func (m Model) filteredGitItems() []GitPRItem {
	if m.git.gitPopupRepo == nil {
		return nil
	}
	var filtered []GitPRItem
	for _, item := range m.git.gitPopupRepo.Items {
		switch m.git.gitPopupTab {
		case 1:
			if item.Kind == "Reviewed" {
				filtered = append(filtered, item)
			}
		case 2:
			if item.Kind == "Assigned" {
				filtered = append(filtered, item)
			}
		case 3:
			if item.Kind == "Commit" {
				filtered = append(filtered, item)
			}
		default:
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func (m Model) gitNavStart() int {
	return gitNavStartFor(m.groupNotes())
}

func (m Model) renderGitStrip(width int, active bool, groups noteGroups) (lines []string, selectedRow int) {
	separator := mutedStyle.Render(" │")
	leftWidth := max((width-lipgloss.Width(separator))/2, 16)
	rightWidth := max(width-lipgloss.Width(separator)-leftWidth, 16)
	columnWidths := [2]int{leftWidth, rightWidth}
	header := " " + renderSectionTitle("G I T", active)
	repoColumns := [2][]*GitRepoStat{m.git.yesterdayGitRepo, m.git.todayGitRepos}
	columnTitles := [2]string{
		m.previousDayTitleFor(groups.previousDay),
		m.dayTitleText(m.currentDate, "T O D A Y"),
	}
	navStart := gitNavStartFor(groups)
	columnStarts := [2]int{navStart, navStart + len(m.git.yesterdayGitRepo)}
	var captionCells [2]string
	for side, repos := range repoColumns {
		columnActive := m.selected >= columnStarts[side] && m.selected < columnStarts[side]+len(repos)
		caption := " " + renderSectionTitle(columnTitles[side], columnActive)
		if m.cfg.DailyCommitsEnabled() {
			commits := 0
			for _, repo := range repos {
				commits += repo.Commits
			}
			caption += mutedStyle.Render(fmt.Sprintf("  %d commits", commits))
		}
		captionCells[side] = ansi.Truncate(caption, columnWidths[side], "…")
	}
	joinCells := func(cells [2]string) string {
		return cells[0] + safeRepeat(" ", columnWidths[0]-lipgloss.Width(cells[0])) + separator + cells[1]
	}
	lines = []string{header, "", joinCells(captionCells)}
	selectedRow = -1
	rowCount := max(len(m.git.yesterdayGitRepo), len(m.git.todayGitRepos), 1)
	for row := range rowCount {
		var cells [2]string
		for side, repos := range repoColumns {
			globalIndex := columnStarts[side] + row
			switch {
			case row < len(repos):
				selected := globalIndex == m.selected
				if selected {
					selectedRow = len(lines)
				}
				cells[side] = strings.TrimSuffix(m.renderGitRepoRow(repos[row], selected, columnWidths[side]), "\n")
			case row == 0 && m.git.loadingCommits:
				cells[side] = mutedStyle.Render("     (checking...)")
			case row == 0:
				cells[side] = mutedStyle.Render("     (no git activity)")
			}
		}
		lines = append(lines, joinCells(cells))
	}
	myPRsStart := columnStarts[1] + len(m.git.todayGitRepos)
	myPRsActive := m.selected >= myPRsStart && m.selected < myPRsStart+len(m.git.myPRs)
	lines = append(lines, "", ansi.Truncate(" "+renderSectionTitle("M Y   P R ( S )", myPRsActive)+mutedStyle.Render(fmt.Sprintf("  %d open", len(m.git.myPRs))), width, "…"))
	for _, line := range m.renderMyPRBlock(myPRsStart, width) {
		if line.navIndex >= 0 && line.navIndex == m.selected {
			selectedRow = len(lines)
		}
		lines = append(lines, line.text)
	}
	return lines, selectedRow
}

func waitForGitSection(sections <-chan sourcecontrol.Section, generation int) tea.Cmd {
	return func() tea.Msg {
		section, open := <-sections
		switch {
		case !open:
			return nil
		case section.MyPRs != nil:
			mine := section.MyPRs
			return gitMyPRsMsg{generation: generation, partOfSync: true, prs: mine.PRs, closed: mine.Closed, failedHosts: mine.FailedHosts, err: mine.Err, sections: sections}
		case section.Day != nil:
			day := section.Day
			return gitDaySectionMsg{generation: generation, day: day.Day, date: day.Date, reviewed: day.Reviewed, reviews: day.Reviews, details: day.Details, failedHosts: day.FailedHosts, err: day.Err, sections: sections}
		default:
			pending := section.Pending
			return gitPendingMsg{generation: generation, startedAt: pending.StartedAt, pending: pending.Items, details: pending.Details, failedHosts: pending.FailedHosts, err: pending.Err, sections: sections}
		}
	}
}

func (m *Model) beginGitFetch() {
	if m.git.gitCancel != nil {
		m.git.gitCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.git.gitFetchCtx = ctx
	m.git.gitCancel = cancel
	m.git.fetchGeneration++
	m.git.fetchedPreviousDay = m.previousNoteDay()
	m.git.loadingGit = true
	m.git.gitSectionsPending = gitSectionCount
	m.postMessage(messageSourceGit, messageProgress, "syncing")
}

func (m Model) gitFetchCmd() tea.Cmd {
	if !m.cfg.GitEnabled() {
		return nil
	}
	ctx := m.git.gitFetchCtx
	if ctx == nil {
		ctx = context.Background()
	}
	sections := sourcecontrol.Sync(ctx, sourcecontrol.SyncParams{
		Config:      m.cfg,
		Today:       m.currentDate,
		PreviousDay: m.previousNoteDay(),
		Sort:        m.git.pendingSort,
		KnownMyPRs:  m.git.knownMyPRs,
	})

	return waitForGitSection(sections, m.git.fetchGeneration)
}

func (m Model) myPRsFetchCmd() tea.Cmd {
	if !m.cfg.GitEnabled() {
		return nil
	}
	cfg, known, generation, ctx := m.cfg, m.git.knownMyPRs, m.git.fetchGeneration, m.sessionCtx
	return func() tea.Msg {
		result := sourcecontrol.NewEngine(cfg).FetchMyPRs(ctx, known)
		return gitMyPRsMsg{generation: generation, prs: result.PRs, closed: result.Closed, failedHosts: result.FailedHosts, err: result.Err}
	}
}

func (m *Model) cancelGitSync() {
	if m.git.gitCancel != nil {
		m.git.gitCancel()
	}
	m.git.fetchGeneration++
	if m.git.commitsCancel != nil {
		m.git.commitsCancel()
	}
	m.git.commitsGeneration++
}

func (m *Model) scheduleDaySync() tea.Cmd {
	if !m.cfg.GitEnabled() {
		return nil
	}
	m.cancelGitSync()
	m.git.loadingGit = true
	m.git.loadingCommits = m.cfg.DailyCommitsEnabled()
	m.postMessage(messageSourceGit, messageProgress, "syncing")
	m.git.daySyncGeneration++
	generation := m.git.daySyncGeneration
	return tea.Batch(tea.Tick(daySyncDelay, func(time.Time) tea.Msg { return daySyncDueMsg{generation: generation} }), m.ensureSyncPulse())
}

func (m *Model) startLoadGitStatsCmd() tea.Cmd {
	if !m.cfg.GitEnabled() {
		return nil
	}
	m.git.daySyncGeneration++
	m.beginGitFetch()
	return tea.Batch(m.gitFetchCmd(), m.refreshCommitsCmd(), m.ensureSyncPulse())
}

var fetchDaysCommits = sourcecontrol.FetchLocalCommitsForDays

func (m Model) loadCommitsCmd() tea.Cmd {
	if m.cfg == nil || !m.cfg.DailyCommitsEnabled() {
		return nil
	}
	cfg, date, previousDay, generation := m.cfg, m.currentDate, m.previousNoteDay(), m.git.commitsGeneration
	ctx := m.git.commitsCtx
	return func() tea.Msg {
		days := fetchDaysCommits(ctx, cfg, date, previousDay)
		return commitsLoadedMsg{generation: generation, today: days[0], yesterday: days[1]}
	}
}

func (m *Model) refreshCommitsCmd() tea.Cmd {
	if m.git.commitsCancel != nil {
		m.git.commitsCancel()
	}
	m.git.commitsCtx, m.git.commitsCancel = context.WithCancel(context.Background())
	m.git.commitsGeneration++
	load := m.loadCommitsCmd()
	if load == nil {
		return nil
	}
	m.git.loadingCommits = true
	if !m.gitSyncInProgress() {
		m.postMessage(messageSourceGit, messageProgress, "syncing")
	}
	return tea.Batch(load, m.ensureSyncPulse())
}

func (m Model) renderGitDetailsModal(modalWidth, innerWidth int) string {
	if m.git.gitPopupRepo == nil {
		return composeDashboard(m)
	}

	titleText := modalTitleStyle.Render(fmt.Sprintf(" GIT DETAILS: %s ", m.git.gitPopupRepo.Name))

	tabNames := []string{"All", "Reviewed", "Assigned", "Commits"}
	var renderedTabs []string
	for i, name := range tabNames {
		if i == m.git.gitPopupTab {
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
		firstRow, lastRow := visibleGitRows(m.git.gitPopupSelected, len(items), listRows)
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

			if i == m.git.gitPopupSelected {
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

func (m Model) renderGitRepoRow(repo *GitRepoStat, selected bool, width int) string {
	var statParts []string
	if repo.Assigned > 0 {
		statParts = append(statParts, fmt.Sprintf("%d assigned", repo.Assigned))
	}
	if repo.Reviewed > 0 {
		statParts = append(statParts, fmt.Sprintf("%d reviewed", repo.Reviewed))
	}
	if m.cfg.DailyCommitsEnabled() {
		statParts = append(statParts, fmt.Sprintf("%d commits", repo.Commits))
	}
	stats := strings.Join(statParts, " · ")
	if room := width - 10; lipgloss.Width(stats) > room {
		stats = "…" + string([]rune(stats)[len([]rune(stats))-max(room-1, 1):])
	}

	nameStyle := subSectionStyle
	return alignRight("     "+underlinedWhen(selected, nameStyle.Render(repo.Name)), []string{mutedStyle.Render(stats)}, width, selected) + "\n"
}

type rowTagCellCache struct {
	prs map[*GitPRItem][3]string
}

func (m Model) prTagCells(item *GitPRItem) (size, age, state string) {
	if m.tagCells == nil {
		return m.renderPRTagCells(item)
	}
	if cells, cached := m.tagCells.prs[item]; cached {
		return cells[0], cells[1], cells[2]
	}
	size, age, state = m.renderPRTagCells(item)
	m.tagCells.prs[item] = [3]string{size, age, state}
	return size, age, state
}

func (m Model) renderPRTagCells(item *GitPRItem) (size, age, state string) {
	if item.PR == nil {
		return "", "", mutedStyle.Render(fmt.Sprintf("[%s]", item.Kind))
	}
	now := time.Now()
	prState, stateLabel := m.prStateLabel(item)
	state = stateStyle(prState).Render(stateLabel)
	if prState == review.StateReviewing {
		state = m.renderReviewRunningIndicator()
	}
	ageText := shortAge(now.Sub(item.PR.RequestedAt))
	age = mutedStyle.Render(ageText)
	if item.PR.IsStale(now) {
		age = staleStyle.Render(ageText)
	}
	return mutedStyle.Render(fmt.Sprintf("±%d", item.PR.Size())), age, state
}

const (
	firstReviewIcon  = "\U000F0CA1"
	reReviewIcon     = "\U000F0458"
	teamReviewIcon   = "\U000F0849"
	directReviewIcon = "\U000F0065"

	reviewIconSlotWidth = 2
)

func reviewRequestIcon(item *GitPRItem) string {
	icons := firstReviewIcon
	if item.Kind == sourcecontrol.ReReviewKind {
		icons = reReviewIcon
	}
	switch {
	case item.PR.DirectRequest:
		icons += directReviewIcon
	case item.PR.CodeOwner:
		icons += teamReviewIcon
	}
	return dimBlueText.Render(icons)
}

func (m Model) prTags(item *GitPRItem, selected bool) []string {
	size, age, state := m.prTagCells(item)
	if item.PR == nil {
		return []string{state}
	}
	icons := reviewRequestIcon(item)
	stateWithIcons := state + " " + safeRepeat(" ", reviewIconSlotWidth-lipgloss.Width(icons)) + icons
	if !selected && !m.cfg.ShowAllTags() {
		return []string{stateWithIcons}
	}
	return []string{size, age, stateWithIcons}
}

func (m Model) renderPendingGitRow(item *GitPRItem, selected bool, width int) string {
	titleStyle := itemStyle
	if selected {
		titleStyle = selectedSummaryStyle
	}
	leftBlock := fmt.Sprintf("      %s %s", pendingPRIcon.Render(), underlinedWhen(selected, titleStyle.Render(item.Title)))
	return alignRight(leftBlock, m.prTags(item, selected), width, selected) + "\n"
}

func (m Model) syncFromKey(tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m, m.startLoadGitStatsCmd()
}

func (m Model) toggleSortField(tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m, m.changePendingSort(func(activeSort *sourcecontrol.Sort) { activeSort.ByCreated = !activeSort.ByCreated })
}

func (m Model) toggleSortOrder(tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m, m.changePendingSort(func(activeSort *sourcecontrol.Sort) { activeSort.Ascending = !activeSort.Ascending })
}

func (m Model) togglePendingScope(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.git.pendingMeOnly = !m.git.pendingMeOnly
	selectedKey, selectedOccurrence := m.selectedNavKey()
	m.rebuildGitRepoStats()
	m.restoreSelection(selectedKey, selectedOccurrence)
	m.updateScrollOffset()
	return m, nil
}

func (m Model) closeGitDetails(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = ViewDashboard
	return m, nil
}

func (m Model) switchGitFilter(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.git.gitPopupTab = (m.git.gitPopupTab + 1) % 4
	m.git.gitPopupSelected = 0
	return m, nil
}

func (m Model) gitCursorDown(tea.KeyMsg) (tea.Model, tea.Cmd) {
	gitItems := m.filteredGitItems()
	if len(gitItems) > 0 && m.git.gitPopupSelected < len(gitItems)-1 {
		m.git.gitPopupSelected++
	}
	return m, nil
}

func (m Model) gitCursorUp(tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.git.gitPopupSelected > 0 {
		m.git.gitPopupSelected--
	}
	return m, nil
}

func (m Model) openGitItem(tea.KeyMsg) (tea.Model, tea.Cmd) {
	gitItems := m.filteredGitItems()
	if len(gitItems) > 0 && m.git.gitPopupSelected < len(gitItems) {
		m.openLink(gitItems[m.git.gitPopupSelected].URL)
	}
	return m, nil
}

func (m Model) confirmReview(tea.KeyMsg) (tea.Model, tea.Cmd) {
	item := m.currentPRItem()
	m.mode = m.prActionReturnMode()
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
		return reviewSubmittedMsg{event: event, pr: queued, payload: payload, err: review.Submit(ctx, queued.Ref, payload)}
	}
}

func (m Model) cancelReview(tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.mode = m.prActionReturnMode(); m.mode == ViewDashboard {
		return m, nil
	}
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
	m.reviewEvent, m.reviewBody = review.EventRequestChanges, body
	return m.confirmReview(tea.KeyMsg{})
}

func (m Model) copyRejectComment(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.copyText(strings.TrimSpace(m.rejectInput.Value()))
	return m, nil
}

func (m Model) cancelRejectComment(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.rejectInput.Blur()
	if m.mode = m.prActionReturnMode(); m.mode == ViewDashboard {
		return m, nil
	}
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

func (gitSection) ApplyMessage(m Model, msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case prAlertsMsg:
		if msg.err != nil {
			m.showError("PR ALERTS", msg.err)
		}
		return messageHandled(m, nil)

	case autoSyncTickMsg:
		if m.cfg.GitEnabled() && m.cfg.GitAutoSyncInterval > 0 {
			return messageHandled(m, tea.Batch(
				m.startLoadGitStatsCmd(),
				autoSyncTickCmd(m.cfg.GitAutoSyncInterval),
			))
		}
		return messageHandled(m, nil)

	case commitsLoadedMsg:
		wasSyncing := m.git.loadingGit || m.git.loadingCommits
		m.applyCommits(msg)
		if wasSyncing {
			save := m.gitCacheSaveCmd()
			return messageHandled(m, save)
		}
		return messageHandled(m, nil)

	case gitDaySectionMsg:
		var cmds []tea.Cmd
		if msg.generation == m.git.fetchGeneration {
			cmds = append(cmds, m.reviewNotesCmd(msg.reviews))
		}
		if msg.sections != nil {
			cmds = append(cmds, waitForGitSection(msg.sections, msg.generation))
		}
		wasSyncing := m.git.loadingGit || m.git.loadingCommits
		m.applyGitDay(msg)
		if wasSyncing {
			cmds = append(cmds, m.gitCacheSaveCmd())
		}
		return messageHandled(m.afterGitSection(cmds))

	case gitMyPRsMsg:
		var cmds []tea.Cmd
		current := !msg.partOfSync || msg.generation == m.git.fetchGeneration
		if current && (len(msg.prs) > 0 || len(msg.closed) > 0) {
			cmds = append(cmds, m.myPRNotesCmd(msg.prs, msg.closed, msg.failedHosts, time.Now()))
		}
		if msg.sections != nil {
			cmds = append(cmds, waitForGitSection(msg.sections, msg.generation))
		}
		wasSyncing := m.git.loadingGit || m.git.loadingCommits
		m.applyMyPRs(msg)
		if wasSyncing || !msg.partOfSync {
			cmds = append(cmds, m.gitCacheSaveCmd())
		}
		return messageHandled(m.afterGitSection(cmds))

	case myPRNotesMsg:
		if msg.known != nil || msg.err == nil {
			m.git.knownMyPRs = msg.known
		}
		if msg.err != nil {
			m.recordSectionError(sectionMyPRs, msg.err)
		}
		if !msg.saved && msg.missing == nil {
			return messageHandled(m, nil)
		}
		return messageHandled(m.Update(notesChangedMsg{notes: msg.changed, err: msg.missing}))

	case gitPendingMsg:
		var cmds []tea.Cmd
		if msg.generation == m.git.fetchGeneration && msg.err == nil {
			cmds = append(cmds, m.reopenApprovedNotesCmd(slices.Clone(msg.pending), msg.startedAt))
		}
		if msg.sections != nil {
			cmds = append(cmds, waitForGitSection(msg.sections, msg.generation))
		}
		wasSyncing := m.git.loadingGit || m.git.loadingCommits
		m.applyGitPending(msg)
		if wasSyncing {
			cmds = append(cmds, m.gitCacheSaveCmd())
		}
		return messageHandled(m.afterGitSection(cmds))

	case approvalSyncMsg:
		return messageHandled(m, m.startLoadGitStatsCmd())

	case daySyncDueMsg:
		if msg.generation != m.git.daySyncGeneration {
			return messageHandled(m, nil)
		}
		return messageHandled(m, m.startLoadGitStatsCmd())

	case changesSinceReviewMsg:
		return messageHandled(m.handleChangesSinceReview(msg))

	case relatedHistoryMsg:
		return messageHandled(m.handleRelatedHistory(msg))

	case gitCacheSavedMsg:
		m.recordSectionError("Cache", msg.err)
		return messageHandled(m, nil)

	case reviewSubmittedMsg:
		return messageHandled(m.handleReviewSubmitted(msg))

	case reviewCloneReadyMsg:
		return messageHandled(m.handleCloneReady(msg))
	}
	return m, nil, false
}

func (gitSection) refreshPreviousDay(m *Model) tea.Cmd {
	if isSameDay(m.previousNoteDay(), m.git.fetchedPreviousDay) {
		return nil
	}
	m.git.ghReviewedYesterday, m.git.localCommitsYesterday = nil, nil
	m.rebuildGitRepoStats()
	return m.startLoadGitStatsCmd()
}

var gitKeystrokes sectionKeystrokes

func init() {
	gitKeystrokes = sectionKeystrokes{
		actionSync:                Model.syncFromKey,
		actionToggleSortField:     Model.toggleSortField,
		actionToggleSortOrder:     Model.toggleSortOrder,
		actionTogglePendingScope:  Model.togglePendingScope,
		actionRefreshCommits:      Model.refreshCommitsFromKey,
		actionDashboardApprove:    Model.dashboardApprove,
		actionDashboardReject:     Model.dashboardReject,
		actionCloseGitDetails:     Model.closeGitDetails,
		actionSwitchGitFilter:     Model.switchGitFilter,
		actionGitCursorDown:       Model.gitCursorDown,
		actionGitCursorUp:         Model.gitCursorUp,
		actionOpenGitItem:         Model.openGitItem,
		actionConfirmReview:       Model.confirmReview,
		actionCancelReview:        Model.cancelReview,
		actionConfirmReviewRun:    Model.confirmReviewRun,
		actionCancelReviewRun:     Model.cancelReviewRun,
		actionSubmitRejectComment: Model.submitRejectComment,
		actionCopyRejectComment:   Model.copyRejectComment,
		actionCancelRejectComment: Model.cancelRejectComment,
		actionStartReview:         Model.startReviewFromKey,
		actionToggleFinding:       Model.toggleFinding,
		actionSelectAllFindings:   Model.selectAllFindings,
		actionPostReview:          Model.postReview,
		actionApprove:             Model.approvePR,
		actionRejectOrStopReview:  Model.rejectOrStopReview,
		actionOpenClone:           Model.openCloneFromKey,
		actionFindingDown:         Model.findingDown,
		actionFindingUp:           Model.findingUp,
	}
}

func (gitSection) ApplyKeystrokes(m Model, binding keyBinding, msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	return gitKeystrokes.apply(m, binding, msg)
}

func (gitSection) Render(m Model, builder *dashboardBuilder, data dashboardData) {
	stripLines, stripSelectedRow := m.renderGitStrip(builder.rowWidth, builder.selectedWithin(data.closedEnd, data.pendingEnd), data.groups)
	builder.board.WriteString(sectionGap)
	if stripSelectedRow >= 0 {
		builder.selectedLine = builder.lineCount() + stripSelectedRow
	}
	builder.board.WriteString(strings.Join(stripLines, "\n") + "\n")
	builder.navIndex = data.gitStripEnd

	pendingHeader := m.renderSubSection("Pending Git Actions", 0, false, builder.selectedWithin(data.gitStripEnd, data.pendingEnd))
	builder.board.WriteString(fmt.Sprintf("%s  %s  %s\n", sectionGap, pendingHeader, m.renderPendingSortHint()))
	switch {
	case len(data.pendingGroups) > 0:
		for groupIndex, group := range data.pendingGroups {
			if groupIndex > 0 {
				builder.board.WriteString("\n")
			}
			builder.board.WriteString("    " + dimBlueText.Bold(true).Render(group.Name) + "\n")
			for itemIndex := range group.Items {
				item := &group.Items[itemIndex]
				builder.emitRow(func(selected bool) string { return m.renderPendingGitRow(item, selected, builder.rowWidth) })
			}
		}
	case m.git.loadingGit:
		builder.board.WriteString(mutedStyle.Render("   (checking pending PR reviews...)\n"))
	case m.git.pendingMeOnly:
		builder.board.WriteString(mutedStyle.Render("   (no PRs asking you by name)\n"))
	default:
		builder.board.WriteString(mutedStyle.Render("   (no PRs requiring review)\n"))
	}
}

func (gitSection) appendNavItems(m Model, items []NavItem, data dashboardData) []NavItem {
	if m.cfg.GitEnabled() {
		for _, repo := range m.git.yesterdayGitRepo {
			items = append(items, NavItem{Kind: KindGitRepo, GitRepo: repo})
		}
		for _, repo := range m.git.todayGitRepos {
			items = append(items, NavItem{Kind: KindGitRepo, GitRepo: repo})
		}
		for index := range m.git.myPRs {
			items = append(items, NavItem{Kind: KindMyPR, MyPR: &m.git.myPRs[index]})
		}
	}
	for _, g := range data.pendingGroups {
		for i := range g.Items {
			items = append(items, NavItem{Kind: KindPendingGit, PendingGitPR: &g.Items[i]})
		}
	}
	return items
}

func gitNavStartFor(groups noteGroups) int {
	return len(groups.previousDone) + len(groups.carried) + len(groups.today) + len(groups.todayDone)
}
