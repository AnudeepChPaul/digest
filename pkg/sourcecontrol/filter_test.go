package sourcecontrol

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"app/pkg/config"
	"app/pkg/review"
)

func makeRepo(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(path, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConfiguredRepoNames(t *testing.T) {
	base := t.TempDir()
	oneConsole := makeRepo(t, filepath.Join(base, "github.com", "o", "web-console"))
	makeRepo(t, filepath.Join(base, "github.com", "o", "unlisted"))
	projects := filepath.Join(base, "projects")
	makeRepo(t, filepath.Join(projects, "monkey"))
	cfg := &config.Config{GitRepositoryRoots: []string{oneConsole + "/", projects}}
	names := ConfiguredRepoNames(cfg)
	if !names["web-console"] || !names["monkey"] || names["unlisted"] || len(names) != 2 {
		t.Errorf("names = %v", names)
	}
	if ConfiguredRepoNames(&config.Config{}) != nil {
		t.Errorf("no roots should mean no filter")
	}
}

func TestFilterPRItemsAndApprovals(t *testing.T) {
	allowed := map[string]bool{"console": true}
	items := []PRItem{{Repository: "console"}, {Repository: "monkey"}, {Repository: "acme/console"}}
	if kept := FilterPRItems(items, allowed); len(kept) != 2 || kept[0].Repository != "console" || kept[1].Repository != "acme/console" {
		t.Errorf("kept = %+v", kept)
	}
	if kept := FilterPRItems(items, nil); len(kept) != 3 {
		t.Errorf("nil filter dropped items: %d", len(kept))
	}
	approvals := []review.ActivityPR{{Repository: "console", Number: 1}, {Repository: "monkey", Number: 2}}
	if kept := FilterReviewRecords(approvals, allowed); len(kept) != 1 || kept[0].Number != 1 {
		t.Errorf("approvals = %+v", kept)
	}
	if kept := FilterReviewRecords(approvals, nil); len(kept) != 2 {
		t.Errorf("nil filter dropped approvals")
	}
}

func TestLocalRepoPathsUsesOnlyRoots(t *testing.T) {
	base := t.TempDir()
	listed := makeRepo(t, filepath.Join(base, "listed"))
	makeRepo(t, filepath.Join(base, "other"))
	cfg := &config.Config{GitRepositoryRoots: []string{listed}, NotesDir: base}
	paths := localRepoPaths(cfg)
	if len(paths) != 1 || filepath.Base(paths[0]) != "listed" {
		t.Errorf("paths = %v", paths)
	}
}

func TestDayResultOutsideRootsIsFiltered(t *testing.T) {
	cfg := &config.Config{GitRepositoryRoots: []string{makeRepo(t, filepath.Join(t.TempDir(), "console"))}}
	result := filterDayResult(DayResult{
		Day:      Today,
		Reviewed: []PRItem{{Repository: "console", Kind: ReviewedKind}, {Repository: "monkey", Kind: ReviewedKind}},
		Reviews:  []review.ActivityPR{{Number: 1, Repository: "console", State: "APPROVED", ReviewedAt: time.Now()}, {Number: 2, Repository: "monkey", State: "APPROVED", ReviewedAt: time.Now()}},
	}, ConfiguredRepoNames(cfg))
	if len(result.Reviewed) != 1 || len(result.Reviews) != 1 || result.Reviews[0].Repository != "console" {
		t.Errorf("filtered = %+v", result)
	}
}
