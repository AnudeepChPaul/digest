package sourcecontrol

import (
	"path/filepath"
	"strings"

	"app/pkg/config"
	"app/pkg/jobs"
	"app/pkg/review"
)

func ConfiguredRepoNames(cfg *config.Config) map[string]bool {
	if cfg == nil || len(cfg.GitRepositoryRoots) == 0 {
		return nil
	}
	repoPaths, _ := jobs.DiscoverRepos(cfg.GitRepositoryRoots)
	names := make(map[string]bool, len(repoPaths))
	for _, repoPath := range repoPaths {
		names[filepath.Base(filepath.Clean(repoPath))] = true
	}
	return names
}

func RepoNameAllowed(repository string, allowed map[string]bool) bool {
	if allowed == nil {
		return true
	}
	return allowed[repository[strings.LastIndex(repository, "/")+1:]]
}

func FilterPRItems(items []PRItem, allowed map[string]bool) []PRItem {
	if allowed == nil {
		return items
	}
	var kept []PRItem
	for _, item := range items {
		if RepoNameAllowed(item.Repository, allowed) {
			kept = append(kept, item)
		}
	}
	return kept
}

func FilterRefs(refs []review.PRRef, allowed map[string]bool) []review.PRRef {
	if allowed == nil {
		return refs
	}
	var kept []review.PRRef
	for _, ref := range refs {
		if RepoNameAllowed(ref.Repo, allowed) {
			kept = append(kept, ref)
		}
	}
	return kept
}

func FilterReviewRecords(reviews []review.ActivityPR, allowed map[string]bool) []review.ActivityPR {
	if allowed == nil {
		return reviews
	}
	var kept []review.ActivityPR
	for _, record := range reviews {
		if RepoNameAllowed(record.Repository, allowed) {
			kept = append(kept, record)
		}
	}
	return kept
}

func filterDayResult(result DayResult, allowed map[string]bool) DayResult {
	result.Reviewed = FilterPRItems(result.Reviewed, allowed)
	result.Reviews = FilterReviewRecords(result.Reviews, allowed)
	return result
}
