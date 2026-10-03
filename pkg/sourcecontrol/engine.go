package sourcecontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"sync"
	"time"

	"app/pkg/config"
	"app/pkg/review"
)

const ghTimeout = 2 * time.Minute

type Engine struct {
	cfg        *config.Config
	fetcher    *review.DetailsFetcher
	hostsOnce  sync.Once
	hosts      []string
	hostsErr   error
	scopesOnce sync.Once
	scopes     map[string][]string
}

func NewEngine(cfg *config.Config, previousDetails map[string]json.RawMessage) *Engine {
	return &Engine{cfg: cfg, fetcher: review.NewDetailsFetcher(cfg.PRDetailsCommand(), review.DefaultDetailsParallel, previousDetails)}
}

func (e *Engine) ghHosts(ctx context.Context) ([]string, error) {
	e.hostsOnce.Do(func() { e.hosts, e.hostsErr = review.GHHosts(ctx) })
	return e.hosts, e.hostsErr
}

func (e *Engine) repoScopes() map[string][]string {
	e.scopesOnce.Do(func() { e.scopes = RepoScopes(e.cfg) })
	return e.scopes
}

func searchAllHosts(ctx context.Context, hosts []string, command string, placeholders map[string]string, scopes map[string][]string) ([]review.PRRef, []string, error) {
	type search struct {
		host  string
		repos string
		refs  []review.PRRef
		err   error
	}
	var searches []*search
	for _, host := range hosts {
		for _, repos := range hostQueries(command, scopes, host) {
			searches = append(searches, &search{host: host, repos: repos})
		}
	}
	var wg sync.WaitGroup
	for _, current := range searches {
		wg.Add(1)
		go func() {
			defer wg.Done()
			searchPlaceholders := maps.Clone(placeholders)
			if searchPlaceholders == nil {
				searchPlaceholders = map[string]string{}
			}
			searchPlaceholders["host"] = current.host
			searchPlaceholders["repos"] = current.repos
			current.refs, current.err = review.SearchPRs(ctx, command, searchPlaceholders)
		}()
	}
	wg.Wait()

	var refs []review.PRRef
	var failedHosts []string
	var errs []error
	seenURLs, failed := map[string]bool{}, map[string]bool{}
	for _, current := range searches {
		if current.err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", current.host, current.err))
			if !failed[current.host] {
				failed[current.host] = true
				failedHosts = append(failedHosts, current.host)
			}
		}
		for _, ref := range current.refs {
			if !seenURLs[ref.URL] {
				seenURLs[ref.URL] = true
				refs = append(refs, ref)
			}
		}
	}
	return refs, failedHosts, errors.Join(errs...)
}

func unloadedHosts(refs []review.PRRef, loaded []review.QueuedPR) []string {
	loadedURLs := map[string]bool{}
	for _, pr := range loaded {
		loadedURLs[pr.Ref.URL] = true
	}
	var hosts []string
	seen := map[string]bool{}
	for _, ref := range refs {
		if !loadedURLs[ref.URL] && !seen[ref.Host] {
			seen[ref.Host] = true
			hosts = append(hosts, ref.Host)
		}
	}
	return hosts
}

func mergeHosts(hostLists ...[]string) []string {
	var merged []string
	seen := map[string]bool{}
	for _, hosts := range hostLists {
		for _, host := range hosts {
			if !seen[host] {
				seen[host] = true
				merged = append(merged, host)
			}
		}
	}
	return merged
}

func urlSet(refs []review.PRRef) map[string]bool {
	urls := make(map[string]bool, len(refs))
	for _, ref := range refs {
		urls[ref.URL] = true
	}
	return urls
}

func reviewRoot(cfg *config.Config) string {
	if cfg == nil {
		return (&config.Config{}).ReviewRootDir()
	}
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
	hosts, err := e.ghHosts(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	scopes := e.repoScopes()
	var pendingRefs, directRefs, reReviewRefs []review.PRRef
	var pendingFailed, reReviewFailed []string
	var pendingErr, directErr, reReviewErr error
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		pendingRefs, pendingFailed, pendingErr = searchAllHosts(ctx, hosts, activeSort.apply(cfg.PendingPRsCommand()), nil, scopes)
	}()
	go func() {
		defer wg.Done()
		directRefs, _, directErr = searchAllHosts(ctx, hosts, activeSort.apply(cfg.DirectRequestedPRsCommand()), nil, scopes)
	}()
	go func() {
		defer wg.Done()
		reReviewRefs, reReviewFailed, reReviewErr = searchAllHosts(ctx, hosts, activeSort.apply(cfg.RereviewPRsCommand()), nil, scopes)
	}()
	wg.Wait()
	searchErr := errors.Join(pendingErr, directErr, reReviewErr)
	if len(pendingRefs)+len(reReviewRefs) == 0 && searchErr != nil {
		return nil, nil, mergeHosts(pendingFailed, reReviewFailed), searchErr
	}
	allowedRepos := ConfiguredRepoNames(cfg)
	pendingRefs, reReviewRefs = FilterRefs(pendingRefs, allowedRepos), FilterRefs(reReviewRefs, allowedRepos)

	requestedRefs := append(append([]review.PRRef(nil), pendingRefs...), reReviewRefs...)
	loaded, details, loadErr := e.fetcher.Load(ctx, requestedRefs)
	failedHosts := mergeHosts(pendingFailed, reReviewFailed, unloadedHosts(requestedRefs, loaded))
	pendingURLs, directURLs := urlSet(pendingRefs), urlSet(directRefs)
	var queue, reviewed []review.QueuedPR
	for _, pr := range loaded {
		if pendingURLs[pr.Ref.URL] {
			pr.CodeOwner = !directURLs[pr.Ref.URL]
			queue = append(queue, pr)
			continue
		}
		reviewed = append(reviewed, pr)
	}
	reReviews := review.SelectReReviews(reviewed, queue)

	root := reviewRoot(cfg)
	_ = review.NotifyTransitions(append(append([]review.QueuedPR(nil), queue...), reReviews...), root, filepath.Join(root, ".state", "approved-seen.json"))

	var items []PRItem
	for _, pr := range review.VisibleRanked(queue, cfg.GreenOnly) {
		items = append(items, NewPRItem(pr, PendingReviewKind))
	}
	for _, pr := range review.VisibleRanked(reReviews, false) {
		items = append(items, NewPRItem(pr, ReReviewKind))
	}
	return items, details, failedHosts, errors.Join(searchErr, loadErr)
}

func (e *Engine) FetchDay(ctx context.Context, day Day, date time.Time) DayResult {
	result := DayResult{Day: day, Date: date.Format("2006-01-02")}
	if e.cfg == nil {
		return result
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		fetchCtx, cancel := context.WithTimeout(ctx, ghTimeout)
		defer cancel()
		result.Reviewed, result.Reviews, result.Details, result.FailedHosts, result.Err = e.loadReviewed(fetchCtx, date)
	}()
	go func() {
		defer wg.Done()
		result.Commits = FetchLocalCommits(ctx, e.cfg, date)
	}()
	wg.Wait()
	return filterDayResult(result, ConfiguredRepoNames(e.cfg))
}

func (e *Engine) loadReviewed(ctx context.Context, date time.Time) ([]PRItem, []review.ActivityPR, map[string]json.RawMessage, []string, error) {
	hosts, err := e.ghHosts(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	refs, searchFailed, searchErr := searchAllHosts(ctx, hosts, e.cfg.ReviewedPRsCommand(), map[string]string{"date": date.Format("2006-01-02")}, e.repoScopes())
	refs = FilterRefs(refs, ConfiguredRepoNames(e.cfg))
	loaded, details, loadErr := e.fetcher.Load(ctx, refs)
	failedHosts := mergeHosts(searchFailed, unloadedHosts(refs, loaded))
	var items []PRItem
	var reviews []review.ActivityPR
	for _, pr := range loaded {
		if pr.MyLastReviewAt.IsZero() || !sameLocalDay(pr.MyLastReviewAt, date) {
			continue
		}
		items = append(items, NewPRItem(pr, ReviewedKind))
		if ReviewNoteStates[pr.MyLastReviewState] {
			reviews = append(reviews, ReviewRecord(pr, pr.MyLastReviewState, pr.MyLastReviewAt))
		}
	}
	return items, reviews, details, failedHosts, errors.Join(searchErr, loadErr)
}

func sameLocalDay(first, second time.Time) bool {
	firstYear, firstMonth, firstDay := first.Local().Date()
	secondYear, secondMonth, secondDay := second.Local().Date()
	return firstYear == secondYear && firstMonth == secondMonth && firstDay == secondDay
}
