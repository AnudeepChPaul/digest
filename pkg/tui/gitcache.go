package tui

import (
	"encoding/json"
	"maps"
	"path/filepath"
	"sync"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/review"
	"github.com/AnudeepChPaul/digest/pkg/sourcecontrol"
	"github.com/AnudeepChPaul/digest/pkg/system"

	tea "github.com/charmbracelet/bubbletea"
)

type cachedGitItem struct {
	Item GitPRItem        `json:"item"`
	PR   *review.QueuedPR `json:"pr,omitempty"`
}

type gitSyncCache struct {
	Date              string                     `json:"date"`
	PreviousDay       string                     `json:"previous_day,omitempty"`
	ReviewedToday     []cachedGitItem            `json:"reviewed_today"`
	ReviewedYesterday []cachedGitItem            `json:"reviewed_yesterday"`
	Pending           []cachedGitItem            `json:"pending"`
	CommitsToday      map[string][]GitPRItem     `json:"commits_today"`
	CommitsYesterday  map[string][]GitPRItem     `json:"commits_yesterday"`
	Details           map[string]json.RawMessage `json:"details,omitempty"`
	PendingSort       *sourcecontrol.Sort        `json:"pending_sort,omitempty"`
	MyPRs             []review.QueuedPR          `json:"my_prs,omitempty"`
}

var gitCachePath = func() string {
	return filepath.Join(digestRoot, "cache", "git-sync.json")
}

func toCachedItems(items []GitPRItem) []cachedGitItem {
	cached := make([]cachedGitItem, 0, len(items))
	for _, item := range items {
		cached = append(cached, cachedGitItem{Item: item, PR: item.PR})
	}
	return cached
}

func fromCachedItems(cached []cachedGitItem) []GitPRItem {
	items := make([]GitPRItem, 0, len(cached))
	for _, entry := range cached {
		item := entry.Item
		item.PR = entry.PR
		items = append(items, item)
	}
	return items
}

func saveGitCache(cache gitSyncCache) error {
	return system.WriteJSON(gitCachePath(), cache)
}

func loadGitCache() (gitSyncCache, bool) {
	var cache gitSyncCache
	found, err := system.ReadJSON(gitCachePath(), &cache)
	if !found || err != nil {
		return gitSyncCache{}, false
	}
	return cache, true
}

type gitCacheSavedMsg struct {
	err error
}

var writeGitCache = saveGitCache

var gitCacheWriteMu sync.Mutex

func (m *Model) gitCacheSaveCmd() tea.Cmd {
	if m.git.loadingGit || m.git.loadingCommits {
		return nil
	}
	m.git.prDetails = m.listedPRDetails()
	if !isSameDay(m.currentDate, time.Now()) {
		return nil
	}
	cache := m.gitCacheSnapshot()
	return func() tea.Msg {
		gitCacheWriteMu.Lock()
		defer gitCacheWriteMu.Unlock()
		return gitCacheSavedMsg{err: writeGitCache(cache)}
	}
}

func (m *Model) gitCacheSnapshot() gitSyncCache {
	cache := gitSyncCache{
		Date:              m.currentDate.Format("2006-01-02"),
		PreviousDay:       m.git.fetchedPreviousDay.Format("2006-01-02"),
		ReviewedToday:     toCachedItems(m.git.ghReviewedToday),
		ReviewedYesterday: toCachedItems(m.git.ghReviewedYesterday),
		Pending:           toCachedItems(m.git.ghPendingPRs),
		CommitsToday:      m.git.localCommitsToday,
		CommitsYesterday:  m.git.localCommitsYesterday,
		Details:           m.listedPRDetails(),
		MyPRs:             m.git.myPRs,
	}
	if m.git.pendingSortChosen {
		activeSort := m.git.pendingSort
		cache.PendingSort = &activeSort
	}
	return cache
}

func pendingSortSaveCmd(activeSort sourcecontrol.Sort) tea.Cmd {
	return func() tea.Msg {
		gitCacheWriteMu.Lock()
		defer gitCacheWriteMu.Unlock()
		cache, _ := loadGitCache()
		cache.PendingSort = &activeSort
		return gitCacheSavedMsg{err: writeGitCache(cache)}
	}
}

func (c gitSyncCache) hasDataFor(today time.Time) bool {
	if c.Date != today.Format("2006-01-02") {
		return false
	}
	return len(c.Pending)+len(c.MyPRs)+len(c.ReviewedToday)+len(c.ReviewedYesterday)+len(c.CommitsToday)+len(c.CommitsYesterday) > 0
}

func (m *Model) mergePRDetails(details map[string]json.RawMessage) {
	if len(details) == 0 {
		return
	}
	if m.git.prDetails == nil {
		m.git.prDetails = make(map[string]json.RawMessage, len(details))
	}
	maps.Copy(m.git.prDetails, details)
}

func (m Model) listedPRDetails() map[string]json.RawMessage {
	listed := map[string]json.RawMessage{}
	for _, items := range [][]GitPRItem{m.git.ghPendingPRs, m.git.ghReviewedToday, m.git.ghReviewedYesterday} {
		for _, item := range items {
			if raw, found := m.git.prDetails[item.URL]; found {
				listed[item.URL] = raw
			}
		}
	}
	return listed
}

func (m *Model) applyGitCache(cache gitSyncCache) {
	m.git.prDetails = cache.Details
	allowedRepos := sourcecontrol.ConfiguredRepoNames(m.cfg)
	m.git.ghPendingPRs = sourcecontrol.FilterPRItems(fromCachedItems(cache.Pending), allowedRepos)
	m.git.myPRs = sourcecontrol.FilterQueuedPRs(cache.MyPRs, allowedRepos)
	today := m.currentDate.Format("2006-01-02")
	if cache.Date == today && cache.PreviousDay == m.git.fetchedPreviousDay.Format("2006-01-02") {
		m.git.ghReviewedToday = sourcecontrol.FilterPRItems(fromCachedItems(cache.ReviewedToday), allowedRepos)
		m.git.ghReviewedYesterday = sourcecontrol.FilterPRItems(fromCachedItems(cache.ReviewedYesterday), allowedRepos)
		if m.cfg.DailyCommitsEnabled() {
			m.git.localCommitsToday = cache.CommitsToday
			m.git.localCommitsYesterday = cache.CommitsYesterday
		}
		m.git.gitSectionDates = map[string]string{
			sectionReviewedToday:     today,
			sectionReviewedYesterday: cache.PreviousDay,
		}
	}
	m.rebuildGitRepoStats()
}
