package tui

import (
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strings"

	"github.com/achandrapaul/digest/pkg/review"
	"github.com/achandrapaul/digest/pkg/sourcecontrol"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	sectionReviewedToday     = "Reviewed today"
	sectionReviewedYesterday = "Reviewed yesterday"
	sectionPending           = "Pending"
	sectionMyPRs             = "My PRs"
)

func (m *Model) finishGitSection() {
	if m.git.gitSectionsPending > 0 {
		m.git.gitSectionsPending--
	}
	m.git.loadingGit = m.git.gitSectionsPending > 0
	m.noteGitSyncDone()
}

func (m *Model) recordSectionError(section string, err error) {
	if err == nil {
		delete(m.git.syncErrors, section)
		return
	}
	if m.git.syncErrors == nil {
		m.git.syncErrors = make(map[string]string)
	}
	m.git.syncErrors[section] = err.Error()
}

func (m *Model) applyGitDay(msg gitDaySectionMsg) {
	if msg.generation != m.git.fetchGeneration {
		return
	}
	m.finishGitSection()
	selectedKey, selectedOccurrence := m.selectedNavKey()

	section, reviewed := sectionReviewedToday, &m.git.ghReviewedToday
	if msg.day == gitDayYesterday {
		section, reviewed = sectionReviewedYesterday, &m.git.ghReviewedYesterday
	}
	if m.git.gitSectionDates == nil {
		m.git.gitSectionDates = make(map[string]string)
	}
	m.recordSectionError(section, msg.err)
	m.mergePRDetails(msg.details)
	if msg.err == nil || len(msg.reviewed) > 0 || m.git.gitSectionDates[section] != msg.date {
		var previous []GitPRItem
		if m.git.gitSectionDates[section] == msg.date {
			previous = *reviewed
		}
		*reviewed = keepFailedHostItems(previous, msg.reviewed, msg.failedHosts)
		sourcecontrol.SortItems(*reviewed, sourcecontrol.Sort{})
		m.git.gitSectionDates[section] = msg.date
	}

	m.refreshReviewRuns()
	m.rebuildGitRepoStats()
	m.restoreSelection(selectedKey, selectedOccurrence)
	m.updateScrollOffset()
}

func (m *Model) applyGitPending(msg gitPendingMsg) {
	if msg.generation != m.git.fetchGeneration {
		return
	}
	m.finishGitSection()
	selectedKey, selectedOccurrence := m.selectedNavKey()

	m.recordSectionError(sectionPending, msg.err)
	m.mergePRDetails(msg.details)
	if msg.err == nil || len(msg.pending) > 0 {
		m.git.ghPendingPRs = keepFailedHostItems(m.git.ghPendingPRs, msg.pending, msg.failedHosts)
	}

	m.refreshReviewRuns()
	m.rebuildGitRepoStats()
	m.restoreSelection(selectedKey, selectedOccurrence)
	m.updateScrollOffset()
}

func (m *Model) applyMyPRs(msg gitMyPRsMsg) {
	if msg.partOfSync {
		if msg.generation != m.git.fetchGeneration {
			return
		}
		m.finishGitSection()
	}
	m.git.loadingMyPRs = false
	selectedKey, selectedOccurrence := m.selectedNavKey()
	m.recordSectionError(sectionMyPRs, msg.err)
	if msg.err == nil || len(msg.prs) > 0 {
		m.git.myPRs = keepFailedHostPRs(m.git.myPRs, msg.prs, msg.failedHosts)
		m.git.closedMyPRs = msg.closed
		sortMyPRs(m.git.myPRs)
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

	commitsToday, commitsYesterday := m.git.localCommitsToday, m.git.localCommitsYesterday
	if !m.cfg.DailyCommitsEnabled() {
		commitsToday, commitsYesterday = nil, nil
	}
	m.git.todayGitRepos = buildStats(m.git.ghReviewedToday, commitsToday)
	m.git.yesterdayGitRepo = buildStats(m.git.ghReviewedYesterday, commitsYesterday)
	sourcecontrol.SortItems(m.git.ghPendingPRs, m.git.pendingSort)
	m.git.pendingGitAction = m.git.ghPendingPRs
	if m.git.pendingMeOnly {
		m.git.pendingGitAction = slices.DeleteFunc(slices.Clone(m.git.ghPendingPRs), func(item GitPRItem) bool {
			return item.PR == nil || !item.PR.DirectRequest
		})
	}
}

func (m *Model) applyPendingSort() {
	m.git.pendingSortChosen = true
	selectedKey, selectedOccurrence := m.selectedNavKey()
	m.rebuildGitRepoStats()
	m.restoreSelection(selectedKey, selectedOccurrence)
	m.updateScrollOffset()
}

func (m *Model) changePendingSort(toggle func(*sourcecontrol.Sort)) tea.Cmd {
	toggle(&m.git.pendingSort)
	m.applyPendingSort()
	save := pendingSortSaveCmd(m.git.pendingSort)
	if !m.git.loadingGit {
		return save
	}
	return tea.Batch(save, m.startLoadGitStatsCmd())
}

func (m Model) renderPendingSortHint() string {
	field, direction := "Updated", "↓ Desc"
	if m.git.pendingSort.ByCreated {
		field = "Created"
	}
	if m.git.pendingSort.Ascending {
		direction = "↑ Asc"
	}
	scope := directReviewIcon + " " + teamReviewIcon
	if m.git.pendingMeOnly {
		scope = directReviewIcon
	}
	return fmt.Sprintf("%s %s  %s %s  %s %s", keyStyle.Render("s"), mutedStyle.Render(field), keyStyle.Render("w"), mutedStyle.Render(direction), keyStyle.Render("m"), dimBlueText.Render(scope))
}
