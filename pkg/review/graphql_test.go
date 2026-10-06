package review

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func stubGraphQL(t *testing.T, respond func(host, query string) string) *[]string {
	t.Helper()
	var queries []string
	var queriesMu sync.Mutex
	original := runGraphQL
	runGraphQL = func(ctx context.Context, host, query string) ([]byte, error) {
		queriesMu.Lock()
		queries = append(queries, query)
		queriesMu.Unlock()
		return []byte(respond(host, query)), nil
	}
	t.Cleanup(func() { runGraphQL = original })
	return &queries
}

const samplePRNode = `{
	"number": 6, "title": "PROJ-12 Fix date picker", "url": "https://github.com/o/console/pull/6", "state": "OPEN", "isDraft": false,
	"createdAt": "2026-10-01T10:00:00Z", "updatedAt": "2026-10-02T10:00:00Z", "additions": 3, "deletions": 1, "changedFiles": 1,
	"headRefOid": "abc", "headRefName": "fix", "reviewDecision": "REVIEW_REQUIRED", "author": {"login": "dev"},
	"commits": {"nodes": [{"commit": {"committedDate": "2026-10-02T09:00:00Z", "statusCheckRollup": {"contexts": {"nodes": [
		{"__typename": "CheckRun", "status": "COMPLETED", "conclusion": "SUCCESS"},
		{"__typename": "StatusContext", "state": "SUCCESS"}
	]}}}}]},
	"reviewRequests": {"nodes": [{"requestedReviewer": {"__typename": "Team", "name": "Platform Team", "slug": "platform-team"}}]},
	"latestReviews": {"nodes": [{"author": {"login": "me"}, "state": "COMMENTED", "submittedAt": "2026-10-01T12:00:00Z"}]},
	"comments": {"nodes": [{"author": {"login": "dev"}, "createdAt": "2026-10-01T13:00:00Z"}]},
	"files": {"nodes": [{"path": "src/picker.ts"}]}
}`

func TestBuildSearchQuery(t *testing.T) {
	query := buildSearchQuery([]PRSearch{
		{Key: "pending0", Query: "is:pr review-requested:@me repo:o/console", First: 15, Details: true},
		{Key: "direct0", Query: "is:pr user-review-requested:@me repo:o/console", First: 15},
	})
	for _, want := range []string{
		"fragment PRFields on PullRequest",
		`pending0: search(type: ISSUE, first: 15, query: "is:pr review-requested:@me repo:o/console") { nodes { ... on PullRequest { ...PRFields } } }`,
		`direct0: search(type: ISSUE, first: 15, query: "is:pr user-review-requested:@me repo:o/console") { nodes { ... on PullRequest { url } } }`,
		"viewer { login }",
	} {
		if !strings.Contains(query, want) {
			t.Errorf("query missing %q:\n%s", want, query)
		}
	}
	urlsOnly := buildSearchQuery([]PRSearch{{Key: "direct0", Query: "is:pr", First: 5}})
	if strings.Contains(urlsOnly, "fragment PRFields") {
		t.Errorf("unused fragment makes GitHub reject the query:\n%s", urlsOnly)
	}
}

func TestRunPRSearchesMapsDetails(t *testing.T) {
	stubGraphQL(t, func(host, query string) string {
		return `{"data": {"viewer": {"login": "me"},
			"pending0": {"nodes": [` + samplePRNode + `]},
			"direct0": {"nodes": [{"url": "https://github.com/o/console/pull/6"}, {}]}}}`
	})
	results, err := RunPRSearches(context.Background(), "github.com", []PRSearch{
		{Key: "pending0", Kind: "pending", Query: "q", First: 15, Details: true},
		{Key: "direct0", Kind: "direct", Query: "q", First: 15},
	})
	if err != nil || len(results) != 2 {
		t.Fatalf("results=%+v err=%v", results, err)
	}
	pending, direct := results[0], results[1]
	if pending.Kind != "pending" || len(pending.PRs) != 1 || len(pending.Refs) != 1 {
		t.Fatalf("pending = %+v", pending)
	}
	pr := pending.PRs[0]
	if pr.Ref.URL != "https://github.com/o/console/pull/6" || pr.CIState != "SUCCESS" || pr.JiraKey != "PROJ-12" || pr.Author != "dev" {
		t.Errorf("pr = %+v", pr)
	}
	if pr.MyLastReviewState != "COMMENTED" || pr.LastReplyAt.IsZero() || pr.LastCommitAt.IsZero() {
		t.Errorf("review fields = %+v", pr)
	}
	if len(pr.OwnerTeams) != 1 || pr.OwnerTeams[0] != "Platform Team" || len(pr.Files) != 1 {
		t.Errorf("teams=%v files=%v", pr.OwnerTeams, pr.Files)
	}
	raw := pending.Details[pr.Ref.URL]
	reparsed, err := ParsePRDetails(raw, "me")
	if err != nil || reparsed.CIState != "SUCCESS" || reparsed.MyLastReviewState != "COMMENTED" {
		t.Errorf("cached details do not round-trip: %v %+v", err, reparsed)
	}
	if direct.Kind != "direct" || len(direct.Refs) != 1 || len(direct.PRs) != 0 {
		t.Errorf("direct = %+v", direct)
	}
}

func TestRunPRSearchesReportsAliasErrors(t *testing.T) {
	stubGraphQL(t, func(host, query string) string {
		return `{"data": {"viewer": {"login": "me"}, "pending0": null, "pending1": {"nodes": []}},
			"errors": [{"path": ["pending0"], "message": "Could not resolve to a Repository"}]}`
	})
	results, err := RunPRSearches(context.Background(), "github.com", []PRSearch{
		{Key: "pending0", Kind: "pending", Query: "q", First: 1},
		{Key: "pending1", Kind: "pending", Query: "q", First: 1},
	})
	if err != nil {
		t.Fatalf("partial failure should not fail the request: %v", err)
	}
	if results[0].Err == nil || !strings.Contains(results[0].Err.Error(), "Could not resolve") {
		t.Errorf("alias error = %v", results[0].Err)
	}
	if results[1].Err != nil {
		t.Errorf("healthy alias error = %v", results[1].Err)
	}
}

func TestRunPRSearchesFailsWithoutData(t *testing.T) {
	stubGraphQL(t, func(host, query string) string {
		return `{"message": "API rate limit exceeded"}`
	})
	_, err := RunPRSearches(context.Background(), "github.com", []PRSearch{{Key: "pending0", Query: "q", First: 1}})
	if err == nil || !strings.Contains(err.Error(), "rate limit") {
		t.Errorf("err = %v", err)
	}
}

func emptySearchResponse(query string) string {
	data := map[string]any{"viewer": map[string]string{"login": "me"}}
	for _, line := range strings.Split(query, "\n") {
		if alias, _, found := strings.Cut(strings.TrimSpace(line), ": search("); found {
			data[alias] = map[string]any{"nodes": []any{}}
		}
	}
	body, _ := json.Marshal(map[string]any{"data": data})
	return string(body)
}

func searchesOf(count int, details bool) []PRSearch {
	var searches []PRSearch
	for index := range count {
		searches = append(searches, PRSearch{Key: fmt.Sprintf("s%d_%v", index, details), Kind: "k", Query: "q", First: 1, Details: details})
	}
	return searches
}

func TestRunPRSearchesBatchesDetailAndURLSearchesSeparately(t *testing.T) {
	queries := stubGraphQL(t, func(host, query string) string { return emptySearchResponse(query) })
	searches := append(searchesOf(maxDetailSearchesPerRequest+1, true), searchesOf(maxURLSearchesPerRequest, false)...)
	results, err := RunPRSearches(context.Background(), "github.com", searches)
	if err != nil || len(results) != len(searches) {
		t.Fatalf("results=%d err=%v", len(results), err)
	}
	if len(*queries) != 3 {
		t.Errorf("requests = %d, want 2 detail batches + 1 url batch", len(*queries))
	}
	for _, query := range *queries {
		if strings.Contains(query, "...PRFields") && strings.Contains(query, "{ url }") {
			t.Errorf("detail and url searches share a request:\n%s", query)
		}
	}
}

func TestRunPRSearchesSplitsBatchesThatTimeOut(t *testing.T) {
	queries := stubGraphQL(t, func(host, query string) string {
		if strings.Count(query, ": search(") > 1 {
			return `{"data": null, "errors": [{"message": "We couldn't respond to your request in time. Sorry about that."}]}`
		}
		return emptySearchResponse(query)
	})
	searches := searchesOf(3, true)
	results, err := RunPRSearches(context.Background(), "github.com", searches)
	if err != nil || len(results) != 3 {
		t.Fatalf("results=%d err=%v", len(results), err)
	}
	for index, result := range results {
		if result.Err != nil {
			t.Errorf("result %d err = %v", index, result.Err)
		}
	}
	if len(*queries) < 4 {
		t.Errorf("requests = %d, want the timed-out batch split and retried", len(*queries))
	}
}

func TestRunPRSearchesDoesNotSplitOnRateLimit(t *testing.T) {
	queries := stubGraphQL(t, func(host, query string) string {
		return `{"message": "API rate limit exceeded"}`
	})
	_, err := RunPRSearches(context.Background(), "github.com", searchesOf(3, true))
	if err == nil || len(*queries) != 1 {
		t.Errorf("err=%v requests=%d; rate limits must not trigger extra requests", err, len(*queries))
	}
}

func TestFetchPRStatesBatchesPerHost(t *testing.T) {
	queries := stubGraphQL(t, func(host, query string) string {
		if host == "github.com" {
			return `{"data": {"pr0": {"pullRequest": {"state": "MERGED"}}, "pr1": {"pullRequest": null}},
				"errors": [{"path": ["pr1", "pullRequest"], "message": "Could not resolve"}]}`
		}
		return `{"data": {"pr0": {"pullRequest": {"state": "OPEN"}}}}`
	})
	merged, _ := ParsePRURL("https://github.com/o/console/pull/1")
	missing, _ := ParsePRURL("https://github.com/o/console/pull/2")
	open, _ := ParsePRURL("https://git.example.com/team/svc/pull/3")
	states, err := FetchPRStates(context.Background(), []PRRef{merged, missing, open})
	if err != nil {
		t.Fatal(err)
	}
	if states[merged.URL] != "MERGED" || states[open.URL] != "OPEN" || states[missing.URL] != "" {
		t.Errorf("states = %v", states)
	}
	if len(*queries) != 2 {
		t.Errorf("requests = %d, want one per host", len(*queries))
	}
	if !strings.Contains(strings.Join(*queries, "\n"), `pr0: repository(owner: "o", name: "console") { pullRequest(number: 1) { state } }`) {
		t.Errorf("queries = %v", *queries)
	}
}

func TestParsePRDetailsFlagsExplicitViewerRequest(t *testing.T) {
	team := reviewRequestNode{Typename: "Team", Name: "Platform Team", Slug: "platform-team"}
	cases := []struct {
		name     string
		requests []reviewRequestNode
		want     bool
	}{
		{"viewer listed", []reviewRequestNode{team, {Typename: "User", Login: "Me"}}, true},
		{"team only", []reviewRequestNode{team}, false},
		{"someone else", []reviewRequestNode{{Typename: "User", Login: "mate"}}, false},
	}
	for _, c := range cases {
		raw, _ := json.Marshal(prDetails{URL: "https://github.com/o/console/pull/6", ReviewRequests: c.requests})
		pr, err := ParsePRDetails(raw, "me")
		if err != nil || pr.DirectRequest != c.want {
			t.Errorf("%s: DirectRequest=%v want %v err=%v", c.name, pr.DirectRequest, c.want, err)
		}
	}
}

func TestBodySearchUsesMyPRFragmentOnly(t *testing.T) {
	query := buildSearchQuery([]PRSearch{
		{Key: "mine0", Query: "is:pr author:@me", First: 5, Details: true, Body: true},
		{Key: "pending0", Query: "is:pr review-requested:@me", First: 5, Details: true},
	})
	for _, want := range []string{"fragment PRFields on PullRequest", "fragment MyPRFields on PullRequest { ...PRFields body }", "{ ...MyPRFields }", "{ ...PRFields }"} {
		if !strings.Contains(query, want) {
			t.Errorf("query missing %q:\n%s", want, query)
		}
	}
	if withoutBody := buildSearchQuery([]PRSearch{{Key: "pending0", Query: "q", First: 5, Details: true}}); strings.Contains(withoutBody, "MyPRFields") {
		t.Errorf("unused MyPRFields fragment makes GitHub reject the query:\n%s", withoutBody)
	}
}

func TestRunPRSearchesDecodesBodyAndReviews(t *testing.T) {
	node := strings.Replace(samplePRNode, `"number": 6,`, `"number": 6, "body": "Fixes the picker",`, 1)
	node = strings.Replace(node, `{"author": {"login": "me"}, "state": "COMMENTED", "submittedAt": "2026-10-01T12:00:00Z"}`,
		`{"author": {"login": "me"}, "state": "COMMENTED", "submittedAt": "2026-10-01T12:00:00Z"}, {"author": {"login": "alice"}, "state": "APPROVED", "submittedAt": "2026-10-02T08:00:00Z"}`, 1)
	stubGraphQL(t, func(host, query string) string {
		return `{"data": {"viewer": {"login": "me"}, "mine0": {"nodes": [` + node + `]}}}`
	})
	results, err := RunPRSearches(context.Background(), "github.com", []PRSearch{{Key: "mine0", Kind: "mine", Query: "q", First: 5, Details: true, Body: true}})
	if err != nil || len(results) != 1 || len(results[0].PRs) != 1 {
		t.Fatalf("results=%+v err=%v", results, err)
	}
	pr := results[0].PRs[0]
	if pr.Body != "Fixes the picker" {
		t.Errorf("body = %q", pr.Body)
	}
	if len(pr.Reviews) != 2 || pr.Reviews[1].Author != "alice" || pr.Reviews[1].State != "APPROVED" || pr.Reviews[1].SubmittedAt.IsZero() {
		t.Errorf("reviews = %+v", pr.Reviews)
	}
}
