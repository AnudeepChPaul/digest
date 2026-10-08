package tui

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/review"
	"github.com/AnudeepChPaul/digest/pkg/sourcecontrol"

	tea "github.com/charmbracelet/bubbletea"
)

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
	if m.gitCancel != nil {
		m.gitCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.gitFetchCtx = ctx
	m.gitCancel = cancel
	m.fetchGeneration++
	m.fetchedPreviousDay = m.previousNoteDay()
	m.loadingGit = true
	m.gitSectionsPending = gitSectionCount
	m.postMessage(messageSourceGit, messageProgress, "syncing")
}

func (m Model) gitFetchCmd() tea.Cmd {
	if !m.cfg.GitEnabled() {
		return nil
	}
	ctx := m.gitFetchCtx
	if ctx == nil {
		ctx = context.Background()
	}
	sections := sourcecontrol.Sync(ctx, sourcecontrol.SyncParams{
		Config:      m.cfg,
		Today:       m.currentDate,
		PreviousDay: m.previousNoteDay(),
		Sort:        m.pendingSort,
		KnownMyPRs:  m.knownMyPRs,
	})

	return waitForGitSection(sections, m.fetchGeneration)
}

func (m Model) myPRsFetchCmd() tea.Cmd {
	if !m.cfg.GitEnabled() {
		return nil
	}
	cfg, known, generation, ctx := m.cfg, m.knownMyPRs, m.fetchGeneration, m.sessionCtx
	return func() tea.Msg {
		result := sourcecontrol.NewEngine(cfg).FetchMyPRs(ctx, known)
		return gitMyPRsMsg{generation: generation, prs: result.PRs, closed: result.Closed, failedHosts: result.FailedHosts, err: result.Err}
	}
}

func (m *Model) cancelGitSync() {
	if m.gitCancel != nil {
		m.gitCancel()
	}
	m.fetchGeneration++
	if m.commitsCancel != nil {
		m.commitsCancel()
	}
	m.commitsGeneration++
}

func (m *Model) scheduleDaySync() tea.Cmd {
	if !m.cfg.GitEnabled() {
		return nil
	}
	m.cancelGitSync()
	m.loadingGit = true
	m.loadingCommits = m.cfg.DailyCommitsEnabled()
	m.postMessage(messageSourceGit, messageProgress, "syncing")
	m.daySyncGeneration++
	generation := m.daySyncGeneration
	return tea.Batch(tea.Tick(daySyncDelay, func(time.Time) tea.Msg { return daySyncDueMsg{generation: generation} }), m.ensureSyncPulse())
}

func (m *Model) startLoadGitStatsCmd() tea.Cmd {
	if !m.cfg.GitEnabled() {
		return nil
	}
	m.daySyncGeneration++
	m.beginGitFetch()
	return tea.Batch(m.gitFetchCmd(), m.refreshCommitsCmd(), m.ensureSyncPulse())
}

const (
	sectionReviewedToday     = "Reviewed today"
	sectionReviewedYesterday = "Reviewed yesterday"
	sectionPending           = "Pending"
	sectionMyPRs             = "My PRs"
)

func (m *Model) finishGitSection() {
	if m.gitSectionsPending > 0 {
		m.gitSectionsPending--
	}
	m.loadingGit = m.gitSectionsPending > 0
	m.noteGitSyncDone()
}

func (m *Model) recordSectionError(section string, err error) {
	if err == nil {
		delete(m.syncErrors, section)
		return
	}
	if m.syncErrors == nil {
		m.syncErrors = make(map[string]string)
	}
	m.syncErrors[section] = err.Error()
}

func (m *Model) applyGitDay(msg gitDaySectionMsg) {
	if msg.generation != m.fetchGeneration {
		return
	}
	m.finishGitSection()
	selectedKey, selectedOccurrence := m.selectedNavKey()

	section, reviewed := sectionReviewedToday, &m.ghReviewedToday
	if msg.day == gitDayYesterday {
		section, reviewed = sectionReviewedYesterday, &m.ghReviewedYesterday
	}
	if m.gitSectionDates == nil {
		m.gitSectionDates = make(map[string]string)
	}
	m.recordSectionError(section, msg.err)
	m.mergePRDetails(msg.details)
	if msg.err == nil || len(msg.reviewed) > 0 || m.gitSectionDates[section] != msg.date {
		var previous []GitPRItem
		if m.gitSectionDates[section] == msg.date {
			previous = *reviewed
		}
		*reviewed = keepFailedHostItems(previous, msg.reviewed, msg.failedHosts)
		sourcecontrol.SortItems(*reviewed, sourcecontrol.Sort{})
		m.gitSectionDates[section] = msg.date
	}

	m.refreshReviewRuns()
	m.rebuildGitRepoStats()
	m.restoreSelection(selectedKey, selectedOccurrence)
	m.updateScrollOffset()
}

func (m *Model) applyGitPending(msg gitPendingMsg) {
	if msg.generation != m.fetchGeneration {
		return
	}
	m.finishGitSection()
	selectedKey, selectedOccurrence := m.selectedNavKey()

	m.recordSectionError(sectionPending, msg.err)
	m.mergePRDetails(msg.details)
	if msg.err == nil || len(msg.pending) > 0 {
		m.ghPendingPRs = keepFailedHostItems(m.ghPendingPRs, msg.pending, msg.failedHosts)
	}

	m.refreshReviewRuns()
	m.rebuildGitRepoStats()
	m.restoreSelection(selectedKey, selectedOccurrence)
	m.updateScrollOffset()
}

func (m *Model) applyMyPRs(msg gitMyPRsMsg) {
	if msg.partOfSync {
		if msg.generation != m.fetchGeneration {
			return
		}
		m.finishGitSection()
	}
	m.loadingMyPRs = false
	selectedKey, selectedOccurrence := m.selectedNavKey()
	m.recordSectionError(sectionMyPRs, msg.err)
	if msg.err == nil || len(msg.prs) > 0 {
		m.myPRs = keepFailedHostPRs(m.myPRs, msg.prs, msg.failedHosts)
		m.closedMyPRs = msg.closed
		sortMyPRs(m.myPRs)
	}
	m.restoreSelection(selectedKey, selectedOccurrence)
	m.updateScrollOffset()
}

func keepFailedHostPRs(previous, fresh []review.QueuedPR, failedHosts []string) []review.QueuedPR {
	kept := append([]review.QueuedPR(nil), fresh...)
	for _, pr := range previous {
		if slices.Contains(failedHosts, pr.Ref.Host) && !slices.ContainsFunc(fresh, func(candidate review.QueuedPR) bool { return candidate.Ref.URL == pr.Ref.URL }) {
			kept = append(kept, pr)
		}
	}
	return kept
}

func sortMyPRs(prs []review.QueuedPR) {
	slices.SortStableFunc(prs, func(a, b review.QueuedPR) int {
		if byRepo := strings.Compare(a.Ref.Repo, b.Ref.Repo); byRepo != 0 {
			return byRepo
		}
		return b.Ref.Number - a.Ref.Number
	})
}

func keepFailedHostItems(previous, fresh []GitPRItem, failedHosts []string) []GitPRItem {
	if len(failedHosts) == 0 {
		return fresh
	}
	failed := make(map[string]bool, len(failedHosts))
	for _, host := range failedHosts {
		failed[host] = true
	}
	freshURLs := make(map[string]bool, len(fresh))
	for _, item := range fresh {
		freshURLs[item.URL] = true
	}
	kept := append([]GitPRItem(nil), fresh...)
	for _, item := range previous {
		parsed, err := url.Parse(item.URL)
		if err != nil || !failed[parsed.Host] || freshURLs[item.URL] {
			continue
		}
		kept = append(kept, item)
	}
	return kept
}

func (m *Model) rebuildGitRepoStats() {
	buildStats := func(reviewed []GitPRItem, commitMap map[string][]GitPRItem) []*GitRepoStat {
		repoMap := make(map[string]*GitRepoStat)

		getOrCreate := func(name string) *GitRepoStat {
			stat, ok := repoMap[name]
			if !ok {
				stat = &GitRepoStat{Name: name}
				repoMap[name] = stat
			}
			return stat
		}

		for _, item := range reviewed {
			rName := item.Repository
			if rName == "" {
				rName = "general"
			}
			st := getOrCreate(rName)
			st.Items = append(st.Items, item)
			st.Reviewed++
		}

		for rName, cItems := range commitMap {
			st := getOrCreate(rName)
			st.Items = append(st.Items, cItems...)
			st.Commits += len(cItems)
		}

		var stats []*GitRepoStat
		for _, stat := range repoMap {
			stats = append(stats, stat)
		}
		sort.SliceStable(stats, func(i, j int) bool {
			return stats[i].Name < stats[j].Name
		})
		return stats
	}

	commitsToday, commitsYesterday := m.localCommitsToday, m.localCommitsYesterday
	if !m.cfg.DailyCommitsEnabled() {
		commitsToday, commitsYesterday = nil, nil
	}
	m.todayGitRepos = buildStats(m.ghReviewedToday, commitsToday)
	m.yesterdayGitRepo = buildStats(m.ghReviewedYesterday, commitsYesterday)
	sourcecontrol.SortItems(m.ghPendingPRs, m.pendingSort)
	m.pendingGitAction = m.ghPendingPRs
	if m.pendingMeOnly {
		m.pendingGitAction = slices.DeleteFunc(slices.Clone(m.ghPendingPRs), func(item GitPRItem) bool {
			return item.PR == nil || !item.PR.DirectRequest
		})
	}
}

func (m *Model) applyPendingSort() {
	m.pendingSortChosen = true
	selectedKey, selectedOccurrence := m.selectedNavKey()
	m.rebuildGitRepoStats()
	m.restoreSelection(selectedKey, selectedOccurrence)
	m.updateScrollOffset()
}

func (m *Model) changePendingSort(toggle func(*sourcecontrol.Sort)) tea.Cmd {
	toggle(&m.pendingSort)
	m.applyPendingSort()
	save := pendingSortSaveCmd(m.pendingSort)
	if !m.loadingGit {
		return save
	}
	return tea.Batch(save, m.startLoadGitStatsCmd())
}

func (m Model) renderPendingSortHint() string {
	field, direction := "Updated", "↓ Desc"
	if m.pendingSort.ByCreated {
		field = "Created"
	}
	if m.pendingSort.Ascending {
		direction = "↑ Asc"
	}
	scope := directReviewIcon + " " + teamReviewIcon
	if m.pendingMeOnly {
		scope = directReviewIcon
	}
	return fmt.Sprintf("%s %s  %s %s  %s %s", keyStyle.Render("s"), mutedStyle.Render(field), keyStyle.Render("w"), mutedStyle.Render(direction), keyStyle.Render("m"), dimBlueText.Render(scope))
}
