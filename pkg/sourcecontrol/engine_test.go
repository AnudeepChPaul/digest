package sourcecontrol

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"app/pkg/config"
	"app/pkg/review"
)

func TestSortQualifier(t *testing.T) {
	cases := map[Sort]string{
		{}:                                 "sort:updated-desc",
		{Ascending: true}:                  "sort:updated-asc",
		{ByCreated: true}:                  "sort:created-desc",
		{ByCreated: true, Ascending: true}: "sort:created-asc",
	}
	for activeSort, want := range cases {
		if got := activeSort.qualifier(); got != want {
			t.Errorf("%+v qualifier = %q, want %q", activeSort, got, want)
		}
	}
}

type recordedSearch struct {
	host     string
	searches []review.PRSearch
}

func stubEngineGitHub(t *testing.T, hosts []string, respond func(host string, search review.PRSearch) review.SearchResult) *[]recordedSearch {
	t.Helper()
	var recorded []recordedSearch
	originalHosts, originalSearches := listGHHosts, runPRSearches
	listGHHosts = func(ctx context.Context) ([]string, error) { return hosts, nil }
	runPRSearches = func(ctx context.Context, host string, searches []review.PRSearch) ([]review.SearchResult, error) {
		recorded = append(recorded, recordedSearch{host: host, searches: searches})
		var results []review.SearchResult
		for _, search := range searches {
			result := respond(host, search)
			result.Kind = search.Kind
			results = append(results, result)
		}
		return results, nil
	}
	t.Cleanup(func() { listGHHosts, runPRSearches = originalHosts, originalSearches })
	return &recorded
}

func engineConfig(t *testing.T, quantity int) *config.Config {
	t.Helper()
	base := t.TempDir()
	service := gitRepoWithOrigin(t, filepath.Join(base, "service"), "https://git.example.com/team/service.git")
	tool := gitRepoWithOrigin(t, filepath.Join(base, "tool"), "https://git.example.com/team/tool.git")
	return &config.Config{GitRepositoryRoots: []string{service, tool}, PRQuantityPerRepo: quantity, ReviewRoot: t.TempDir()}
}

func queued(number int, mutate func(*review.QueuedPR)) review.QueuedPR {
	pr := review.QueuedPR{Ref: prRefOn("git.example.com", number), Title: "PR", CIState: "SUCCESS", CreatedAt: time.Now().Add(-time.Hour)}
	if mutate != nil {
		mutate(&pr)
	}
	return pr
}

func TestLoadPendingSearchesEachRepoWithCachedSort(t *testing.T) {
	cfg := engineConfig(t, 20)
	reviewedAt := time.Now().Add(-2 * time.Hour)
	recorded := stubEngineGitHub(t, []string{"git.example.com", "unscoped.example.com"}, func(host string, search review.PRSearch) review.SearchResult {
		if !strings.Contains(search.Query, "repo:team/service") {
			return review.SearchResult{}
		}
		switch search.Kind {
		case "pending":
			team, direct := queued(1, nil), queued(2, nil)
			return review.SearchResult{Refs: []review.PRRef{team.Ref, direct.Ref}, PRs: []review.QueuedPR{team, direct}}
		case "direct":
			return review.SearchResult{Refs: []review.PRRef{prRefOn("git.example.com", 2)}}
		default:
			reReview := queued(3, func(pr *review.QueuedPR) {
				pr.MyLastReviewAt, pr.MyLastReviewState, pr.LastCommitAt = reviewedAt, "COMMENTED", time.Now()
			})
			return review.SearchResult{Refs: []review.PRRef{reReview.Ref}, PRs: []review.QueuedPR{reReview}}
		}
	})

	items, _, failedHosts, err := NewEngine(cfg).loadPending(context.Background(), Sort{ByCreated: true, Ascending: true})
	if err != nil || len(failedHosts) != 0 {
		t.Fatalf("err=%v failed=%v", err, failedHosts)
	}
	if len(*recorded) != 1 || (*recorded)[0].host != "git.example.com" {
		t.Fatalf("hosts searched = %+v; hosts without configured repos must be skipped", *recorded)
	}
	searches := (*recorded)[0].searches
	if len(searches) != 6 {
		t.Fatalf("searches = %d, want 3 kinds x 2 repos", len(searches))
	}
	for _, search := range searches {
		if search.First != 20 || !strings.Contains(search.Query, "sort:created-asc") || !strings.Contains(search.Query, "repo:team/") {
			t.Errorf("search = %+v", search)
		}
		if search.Details == (search.Kind == "direct") {
			t.Errorf("details flag wrong for %+v", search)
		}
	}
	codeOwner := map[int]bool{}
	var kinds []string
	for _, item := range items {
		kinds = append(kinds, item.Kind)
		codeOwner[item.Number] = item.PR.CodeOwner
	}
	if !reflect.DeepEqual(kinds, []string{PendingReviewKind, PendingReviewKind, ReReviewKind}) {
		t.Errorf("kinds = %v", kinds)
	}
	if !codeOwner[1] || codeOwner[2] {
		t.Errorf("code owner flags = %v; team request is code owner, direct request is not", codeOwner)
	}
}

func TestLoadPendingReportsFailedHosts(t *testing.T) {
	cfg := engineConfig(t, 0)
	stubEngineGitHub(t, []string{"git.example.com"}, func(host string, search review.PRSearch) review.SearchResult {
		if search.First != config.DefaultPRQuantityPerRepo {
			t.Errorf("first = %d, want default", search.First)
		}
		if strings.Contains(search.Query, "repo:team/tool") {
			return review.SearchResult{Err: errors.New("Could not resolve")}
		}
		return review.SearchResult{}
	})
	_, _, failedHosts, err := NewEngine(cfg).loadPending(context.Background(), Sort{})
	if err == nil || !reflect.DeepEqual(failedHosts, []string{"git.example.com"}) {
		t.Errorf("err=%v failed=%v", err, failedHosts)
	}
}

func TestLoadReviewedUsesDateAndUpdatedOrder(t *testing.T) {
	cfg := engineConfig(t, 20)
	day := time.Date(2026, 10, 5, 12, 0, 0, 0, time.Local)
	recorded := stubEngineGitHub(t, []string{"git.example.com"}, func(host string, search review.PRSearch) review.SearchResult {
		reviewed := queued(4, func(pr *review.QueuedPR) { pr.MyLastReviewAt, pr.MyLastReviewState = day, "APPROVED" })
		older := queued(5, func(pr *review.QueuedPR) { pr.MyLastReviewAt, pr.MyLastReviewState = day.AddDate(0, 0, -2), "APPROVED" })
		return review.SearchResult{Refs: []review.PRRef{reviewed.Ref, older.Ref}, PRs: []review.QueuedPR{reviewed, older}}
	})
	items, reviews, _, failedHosts, err := NewEngine(cfg).loadReviewed(context.Background(), day)
	if err != nil || len(failedHosts) != 0 {
		t.Fatalf("err=%v failed=%v", err, failedHosts)
	}
	for _, search := range (*recorded)[0].searches {
		if search.Kind != "reviewed" || !search.Details || !strings.Contains(search.Query, "updated:>=2026-10-05") || !strings.Contains(search.Query, "sort:updated-desc") {
			t.Errorf("search = %+v", search)
		}
	}
	if len(items) != 1 || items[0].Number != 4 || len(reviews) != 1 {
		t.Errorf("items=%+v reviews=%+v", items, reviews)
	}
}

func TestFetchDaySkipsCommitsWhenDisabled(t *testing.T) {
	cfg := engineConfig(t, 0)
	disabled := false
	cfg.ShowDailyCommits = &disabled
	stubEngineGitHub(t, []string{"git.example.com"}, func(host string, search review.PRSearch) review.SearchResult { return review.SearchResult{} })
	original := fetchLocalCommits
	fetchLocalCommits = func(ctx context.Context, cfg *config.Config, date time.Time) map[string][]PRItem {
		t.Errorf("local commits fetched while show_daily_commits is false")
		return nil
	}
	defer func() { fetchLocalCommits = original }()
	if result := NewEngine(cfg).FetchDay(context.Background(), Today, time.Now()); len(result.Commits) != 0 {
		t.Errorf("commits = %v", result.Commits)
	}
}
