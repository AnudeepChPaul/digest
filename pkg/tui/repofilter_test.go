package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func makeRepo(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(path, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOldCacheIsFilteredOnLoad(t *testing.T) {
	m := syncTestModel(t)
	m.cfg.GitRepositoryRoots = []string{makeRepo(t, filepath.Join(t.TempDir(), "console"))}
	cache := gitSyncCache{Date: m.currentDate.Format("2006-01-02"), Pending: []cachedGitItem{{Item: GitPRItem{Repository: "console"}}, {Item: GitPRItem{Repository: "monkey"}}}}
	if err := saveGitCache(cache); err != nil {
		t.Fatal(err)
	}
	fresh := NewModel(m.cfg, nil)
	if len(fresh.git.ghPendingPRs) != 1 || fresh.git.ghPendingPRs[0].Repository != "console" {
		t.Errorf("pending = %+v", fresh.git.ghPendingPRs)
	}
}
