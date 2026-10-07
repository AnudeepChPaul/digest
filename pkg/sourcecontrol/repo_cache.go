package sourcecontrol

import (
	"strings"
	"sync"
	"time"

	"app/pkg/jobs"
)

var repoCacheTTL = time.Minute

type expiringCache[V any] struct {
	mu       sync.Mutex
	key      string
	loadedAt time.Time
	value    V
	loaded   bool
}

func (cache *expiringCache[V]) get(key string, load func() V) V {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.loaded && cache.key == key && time.Since(cache.loadedAt) < repoCacheTTL {
		return cache.value
	}
	cache.key, cache.loadedAt, cache.value, cache.loaded = key, time.Now(), load(), true
	return cache.value
}

var (
	discoveredRepos expiringCache[[]string]
	repoScopes      expiringCache[map[string][]string]
)

func rootsKey(roots []string) string {
	return strings.Join(roots, "\x00")
}

func discoverReposCached(roots []string) []string {
	return discoveredRepos.get(rootsKey(roots), func() []string {
		repoPaths, _ := jobs.DiscoverRepos(roots)
		return repoPaths
	})
}
