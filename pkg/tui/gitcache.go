package tui

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"time"

	"app/pkg/review"
	"app/pkg/sourcecontrol"
)

type cachedGitItem struct {
	Item GitPRItem        `json:"item"`
	PR   *review.QueuedPR `json:"pr,omitempty"`
}

type gitSyncCache struct {
	Date              string                     `json:"date"`
	ReviewedToday     []cachedGitItem            `json:"reviewed_today"`
	ReviewedYesterday []cachedGitItem            `json:"reviewed_yesterday"`
	Pending           []cachedGitItem            `json:"pending"`
	CommitsToday      map[string][]GitPRItem     `json:"commits_today"`
	CommitsYesterday  map[string][]GitPRItem     `json:"commits_yesterday"`
	Details           map[string]json.RawMessage `json:"details,omitempty"`
	PendingSort       *sourcecontrol.Sort        `json:"pending_sort,omitempty"`
}

var gitCachePath = func() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, "digest", "cache", "git-sync.json")
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
	cachePath := gitCachePath()
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		return err
	}
	encoded, err := json.Marshal(cache)
	if err != nil {
		return err
	}
	tempFile, err := os.CreateTemp(filepath.Dir(cachePath), ".git-sync-*.json")
	if err != nil {
		return err
	}
	if _, err := tempFile.Write(encoded); err != nil {
		tempFile.Close()
		os.Remove(tempFile.Name())
		return err
	}
	if err := tempFile.Close(); err != nil {
		os.Remove(tempFile.Name())
		return err
	}
	return os.Rename(tempFile.Name(), cachePath)
}

func loadGitCache() (gitSyncCache, bool) {
	var cache gitSyncCache
	encoded, err := os.ReadFile(gitCachePath())
	if err != nil {
		return cache, false
	}
	if err := json.Unmarshal(encoded, &cache); err != nil {
		return gitSyncCache{}, false
	}
	return cache, true
}

func (m *Model) saveGitCacheIfToday() {
	if !isSameDay(m.currentDate, time.Now()) {
		return
	}
	cache := gitSyncCache{
		Date:              m.currentDate.Format("2006-01-02"),
		ReviewedToday:     toCachedItems(m.ghReviewedToday),
		ReviewedYesterday: toCachedItems(m.ghReviewedYesterday),
		Pending:           toCachedItems(m.ghPendingPRs),
		CommitsToday:      m.localCommitsToday,
		CommitsYesterday:  m.localCommitsYesterday,
		Details:           m.listedPRDetails(),
	}
	if m.pendingSortChosen {
		cache.PendingSort = &m.pendingSort
	}
	if err := saveGitCache(cache); err != nil {
		m.recordSectionError("Cache", err)
	}
}

func (m *Model) savePendingSort() {
	cache, _ := loadGitCache()
	activeSort := m.pendingSort
	cache.PendingSort = &activeSort
	if err := saveGitCache(cache); err != nil {
		m.recordSectionError("Cache", err)
	}
}

func (c gitSyncCache) hasDataFor(today time.Time) bool {
	if c.Date != today.Format("2006-01-02") {
		return false
	}
	return len(c.Pending)+len(c.ReviewedToday)+len(c.ReviewedYesterday)+len(c.CommitsToday)+len(c.CommitsYesterday) > 0
}

func (m *Model) mergePRDetails(details map[string]json.RawMessage) {
	if len(details) == 0 {
		return
	}
	if m.prDetails == nil {
		m.prDetails = make(map[string]json.RawMessage, len(details))
	}
	maps.Copy(m.prDetails, details)
}

func (m Model) listedPRDetails() map[string]json.RawMessage {
	listed := map[string]json.RawMessage{}
	for _, items := range [][]GitPRItem{m.ghPendingPRs, m.ghReviewedToday, m.ghReviewedYesterday} {
		for _, item := range items {
			if raw, found := m.prDetails[item.URL]; found {
				listed[item.URL] = raw
			}
		}
	}
	return listed
}

func (m *Model) applyGitCache(cache gitSyncCache) {
	m.prDetails = cache.Details
	allowedRepos := sourcecontrol.ConfiguredRepoNames(m.cfg)
	m.ghPendingPRs = sourcecontrol.FilterPRItems(fromCachedItems(cache.Pending), allowedRepos)
	today := m.currentDate.Format("2006-01-02")
	if cache.Date == today {
		m.ghReviewedToday = sourcecontrol.FilterPRItems(fromCachedItems(cache.ReviewedToday), allowedRepos)
		m.ghReviewedYesterday = sourcecontrol.FilterPRItems(fromCachedItems(cache.ReviewedYesterday), allowedRepos)
		m.localCommitsToday = cache.CommitsToday
		m.localCommitsYesterday = cache.CommitsYesterday
		m.gitSectionDates = map[string]string{
			sectionReviewedToday:     today,
			sectionReviewedYesterday: m.currentDate.AddDate(0, 0, -1).Format("2006-01-02"),
		}
	}
	m.rebuildGitRepoStats()
}
