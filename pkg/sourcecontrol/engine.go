package sourcecontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/achandrapaul/digest/pkg/config"
	"github.com/achandrapaul/digest/pkg/review"
)

const ghTimeout = 2 * time.Minute

type Engine struct {
	cfg        *config.Config
	hostsOnce  sync.Once
	hosts      []string
	hostsErr   error
	scopesOnce sync.Once
	scopes     map[string][]string
}

var (
	listGHHosts   = review.GHHosts
	runPRSearches = review.RunPRSearches
	fetchPRStates = review.FetchPRStates
)

func NewEngine(cfg *config.Config) *Engine {
	return &Engine{cfg: cfg}
}

func (e *Engine) ghHosts(ctx context.Context) ([]string, error) {
	e.hostsOnce.Do(func() { e.hosts, e.hostsErr = listGHHosts(ctx) })
	return e.hosts, e.hostsErr
}

func (e *Engine) repoScopes() map[string][]string {
	e.scopesOnce.Do(func() { e.scopes = RepoScopes(e.cfg) })
	return e.scopes
}

const (
	pendingSearch  = "pending"
	directSearch   = "direct"
	reReviewSearch = "rereview"
	reviewedSearch = "reviewed"
	openedSearch   = "opened"
	mergedSearch   = "merged"
	myPRsSearch    = "mine"
)

type searchKind struct {
	key     string
	query   string
	details bool
	body    bool
}

type hostSearches struct {
	host     string
	searches []review.PRSearch
}

func (e *Engine) planSearches(hosts []string, kinds []searchKind) []hostSearches {
	scopes := e.repoScopes()
	perRepo := e.cfg.PRsPerRepo()
	var planned []hostSearches
	for _, host := range hosts {
		repos := []string{""}
		if scopes != nil {
			repos = scopes[host]
		}
		var searches []review.PRSearch
		for _, kind := range kinds {
			for index, repo := range repos {
				query := kind.query
				if repo != "" {
					query += " repo:" + repo
				}
				searches = append(searches, review.PRSearch{Key: fmt.Sprintf("%s%d", kind.key, index), Kind: kind.key, Query: query, First: perRepo, Details: kind.details, Body: kind.body})
			}
		}
		if len(searches) > 0 {
			planned = append(planned, hostSearches{host: host, searches: searches})
		}
	}
	return planned
}

type searchOutcome struct {
	prs         map[string][]review.QueuedPR
	urls        map[string]map[string]bool
	details     map[string]json.RawMessage
	failedHosts []string
	err         error
}

func (e *Engine) search(ctx context.Context, kinds []searchKind) searchOutcome {
	outcome := searchOutcome{prs: map[string][]review.QueuedPR{}, urls: map[string]map[string]bool{}, details: map[string]json.RawMessage{}}
	hosts, err := e.ghHosts(ctx)
	if err != nil {
		outcome.err = err
		return outcome
	}
	planned := e.planSearches(hosts, kinds)
	results := make([][]review.SearchResult, len(planned))
	requestErrs := make([]error, len(planned))
	var wg sync.WaitGroup
	for index, current := range planned {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[index], requestErrs[index] = runPRSearches(ctx, current.host, current.searches)
		}()
	}
	wg.Wait()

	seenPRs := map[string]map[string]bool{}
	var errs []error
	for index, current := range planned {
		failed := requestErrs[index] != nil
		if failed {
			errs = append(errs, fmt.Errorf("%s: %w", current.host, requestErrs[index]))
		}
		for _, result := range results[index] {
			if result.Err != nil {
				failed = true
				errs = append(errs, fmt.Errorf("%s: %w", current.host, result.Err))
				continue
			}
			if outcome.urls[result.Kind] == nil {
				outcome.urls[result.Kind], seenPRs[result.Kind] = map[string]bool{}, map[string]bool{}
			}
			for _, ref := range result.Refs {
				outcome.urls[result.Kind][ref.URL] = true
			}
			for _, pr := range result.PRs {
				if seenPRs[result.Kind][pr.Ref.URL] {
					continue
				}
				seenPRs[result.Kind][pr.Ref.URL] = true
				outcome.prs[result.Kind] = append(outcome.prs[result.Kind], pr)
				if raw, found := result.Details[pr.Ref.URL]; found {
					outcome.details[pr.Ref.URL] = raw
				}
			}
		}
		if failed {
			outcome.failedHosts = append(outcome.failedHosts, current.host)
		}
	}
	outcome.err = errors.Join(errs...)
	return outcome
}

func reviewRoot(cfg *config.Config) string {
	return cfg.ReviewRootDir()
}

func (e *Engine) FetchPendingPRs(ctx context.Context, activeSort Sort) PendingResult {
	result := PendingResult{StartedAt: time.Now()}
	if e.cfg == nil {
		return result
	}
	fetchCtx, cancel := context.WithTimeout(ctx, ghTimeout)
	defer cancel()
	result.Items, result.Details, result.FailedHosts, result.Err = e.loadPending(fetchCtx, activeSort)
	result.Items = FilterPRItems(result.Items, ConfiguredRepoNames(e.cfg))
	return result
}

func (e *Engine) loadPending(ctx context.Context, activeSort Sort) ([]PRItem, map[string]json.RawMessage, []string, error) {
	cfg := e.cfg
	order := " " + activeSort.qualifier()
	outcome := e.search(ctx, []searchKind{
		{key: pendingSearch, query: "is:pr is:open draft:false review-requested:@me" + order, details: true},
		{key: directSearch, query: "is:pr is:open draft:false user-review-requested:@me" + order},
		{key: reReviewSearch, query: "is:pr is:open draft:false reviewed-by:@me -review-requested:@me" + order, details: true},
	})
	if len(outcome.prs[pendingSearch])+len(outcome.prs[reReviewSearch]) == 0 && outcome.err != nil {
		return nil, nil, outcome.failedHosts, outcome.err
	}
	allowedRepos := ConfiguredRepoNames(cfg)
	var queue []review.QueuedPR
	for _, pr := range FilterQueuedPRs(outcome.prs[pendingSearch], allowedRepos) {
		pr.CodeOwner = !outcome.urls[directSearch][pr.Ref.URL]
		queue = append(queue, pr)
	}
	root := reviewRoot(cfg)
	localReviewOutdated := memoizeByURL(func(pr review.QueuedPR) bool { return review.LocalReviewOutdated(root, pr) })
	reReviews := review.SelectReReviews(FilterQueuedPRs(outcome.prs[reReviewSearch], allowedRepos), queue, func(pr review.QueuedPR) bool {
		return review.NeedsReReview(pr) || localReviewOutdated(pr)
	})
	_ = review.NotifyTransitions(append(append([]review.QueuedPR(nil), queue...), reReviews...), root, filepath.Join(root, ".state", "approved-seen.json"))

	var items []PRItem
	for _, pr := range review.VisibleRanked(queue, cfg.GreenOnly) {
		kind := PendingReviewKind
		if localReviewOutdated(pr) {
			kind = ReReviewKind
		}
		items = append(items, NewPRItem(pr, kind))
	}
	for _, pr := range review.VisibleRanked(reReviews, false) {
		items = append(items, NewPRItem(pr, ReReviewKind))
	}
	return items, outcome.details, outcome.failedHosts, outcome.err
}

func (e *Engine) FetchDay(ctx context.Context, day Day, date time.Time) DayResult {
	result := DayResult{Day: day, Date: date.Format("2006-01-02")}
	if e.cfg == nil {
		return result
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		fetchCtx, cancel := context.WithTimeout(ctx, ghTimeout)
		defer cancel()
		result.Reviewed, result.Reviews, result.Details, result.FailedHosts, result.Err = e.loadReviewed(fetchCtx, date)
	}()
	wg.Wait()
	return filterDayResult(result, ConfiguredRepoNames(e.cfg))
}

func (e *Engine) loadReviewed(ctx context.Context, date time.Time) ([]PRItem, []review.ActivityPR, map[string]json.RawMessage, []string, error) {
	outcome := e.search(ctx, []searchKind{
		{key: reviewedSearch, query: "is:pr reviewed-by:@me updated:>=" + date.Format("2006-01-02") + " " + Sort{}.qualifier(), details: true},
	})
	loaded := FilterQueuedPRs(outcome.prs[reviewedSearch], ConfiguredRepoNames(e.cfg))
	var items []PRItem
	var reviews []review.ActivityPR
	dayStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	for _, pr := range loaded {
		if pr.MyLastReviewAt.IsZero() || pr.MyLastReviewAt.Before(dayStart) {
			continue
		}
		if sameLocalDay(pr.MyLastReviewAt, date) {
			items = append(items, NewPRItem(pr, ReviewedKind))
		}
		if ReviewNoteStates[pr.MyLastReviewState] {
			reviews = append(reviews, ReviewRecord(pr, pr.MyLastReviewState, pr.MyLastReviewAt))
		}
	}
	return items, reviews, outcome.details, outcome.failedHosts, outcome.err
}

func (e *Engine) AuthoredPRs(ctx context.Context, start, end time.Time) ([]review.QueuedPR, []review.QueuedPR, error) {
	dateRange := start.Format("2006-01-02") + ".." + end.AddDate(0, 0, -1).Format("2006-01-02")
	outcome := e.search(ctx, []searchKind{
		{key: openedSearch, query: "is:pr author:@me created:" + dateRange, details: true},
		{key: mergedSearch, query: "is:pr author:@me merged:" + dateRange, details: true},
	})
	allowedRepos := ConfiguredRepoNames(e.cfg)
	return FilterQueuedPRs(outcome.prs[openedSearch], allowedRepos), FilterQueuedPRs(outcome.prs[mergedSearch], allowedRepos), outcome.err
}

func (e *Engine) FetchMyPRs(ctx context.Context, known []review.PRRef) MyPRsResult {
	result := MyPRsResult{StartedAt: time.Now()}
	if e.cfg == nil {
		return result
	}
	fetchCtx, cancel := context.WithTimeout(ctx, ghTimeout)
	defer cancel()
	outcome := e.search(fetchCtx, []searchKind{
		{key: myPRsSearch, query: "is:pr is:open author:@me " + Sort{}.qualifier(), details: true, body: true},
	})
	result.FailedHosts, result.Err = outcome.failedHosts, outcome.err
	allowedRepos := ConfiguredRepoNames(e.cfg)
	result.PRs = FilterQueuedPRs(outcome.prs[myPRsSearch], allowedRepos)
	if len(outcome.urls[myPRsSearch]) == 0 && outcome.err != nil {
		return result
	}
	failedHosts := map[string]bool{}
	for _, host := range outcome.failedHosts {
		failedHosts[host] = true
	}
	var gone []review.PRRef
	for _, ref := range known {
		if !outcome.urls[myPRsSearch][ref.URL] && !failedHosts[ref.Host] && RepoNameAllowed(ref.Repo, allowedRepos) {
			gone = append(gone, ref)
		}
	}
	if len(gone) == 0 {
		return result
	}
	states, err := fetchPRStates(fetchCtx, gone)
	result.Err = errors.Join(result.Err, err)
	for url, state := range states {
		if state == "MERGED" || state == "CLOSED" {
			if result.Closed == nil {
				result.Closed = map[string]string{}
			}
			result.Closed[url] = state
		}
	}
	return result
}

func sameLocalDay(first, second time.Time) bool {
	firstYear, firstMonth, firstDay := first.Local().Date()
	secondYear, secondMonth, secondDay := second.Local().Date()
	return firstYear == secondYear && firstMonth == secondMonth && firstDay == secondDay
}

func memoizeByURL(check func(review.QueuedPR) bool) func(review.QueuedPR) bool {
	answers := map[string]bool{}
	return func(pr review.QueuedPR) bool {
		if answer, known := answers[pr.Ref.URL]; known {
			return answer
		}
		answers[pr.Ref.URL] = check(pr)
		return answers[pr.Ref.URL]
	}
}
