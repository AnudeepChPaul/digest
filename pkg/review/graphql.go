package review

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	maxDetailSearchesPerRequest = 4
	maxURLSearchesPerRequest    = 20
	maxStatesPerRequest         = 50
	searchBatchesInFlight       = 2
)

const prFieldsFragment = `fragment PRFields on PullRequest {
  number title url state isDraft createdAt updatedAt additions deletions changedFiles
  headRefOid headRefName reviewDecision
  author { login }
  commits(last: 1) { nodes { commit { committedDate statusCheckRollup { contexts(first: 100) { nodes { __typename ... on CheckRun { status conclusion } ... on StatusContext { state } } } } } } }
  reviewRequests(first: 100) { nodes { requestedReviewer { __typename ... on User { login } ... on Team { name slug } } } }
  latestReviews(first: 100) { nodes { author { login } state submittedAt } }
  comments(first: 100) { nodes { author { login } createdAt } }
  files(first: 100) { nodes { path } }
}
`

var runGraphQL = func(ctx context.Context, host, query string) ([]byte, error) {
	body, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "gh", "api", "graphql", "--hostname", host, "--input", "-")
	cmd.Stdin = bytes.NewReader(body)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil && !json.Valid(bytes.TrimSpace(out)) {
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return nil, fmt.Errorf("%s", message)
		}
		return nil, err
	}
	return out, nil
}

type graphQLResponse struct {
	Data    map[string]json.RawMessage `json:"data"`
	Message string                     `json:"message"`
	Errors  []struct {
		Message string `json:"message"`
		Path    []any  `json:"path"`
	} `json:"errors"`
}

func queryGraphQL(ctx context.Context, host, query string) (map[string]json.RawMessage, map[string]error, error) {
	out, err := runGraphQL(ctx, host, query)
	if err != nil {
		return nil, nil, err
	}
	var response graphQLResponse
	if err := json.Unmarshal(out, &response); err != nil {
		return nil, nil, fmt.Errorf("decode graphql response: %w", err)
	}
	aliasErrors := map[string]error{}
	var requestErrors []error
	for _, graphErr := range response.Errors {
		if alias, ok := firstPathElement(graphErr.Path); ok {
			aliasErrors[alias] = errors.Join(aliasErrors[alias], errors.New(graphErr.Message))
			continue
		}
		requestErrors = append(requestErrors, errors.New(graphErr.Message))
	}
	if response.Data == nil {
		if response.Message != "" {
			requestErrors = append(requestErrors, errors.New(response.Message))
		}
		if len(requestErrors) == 0 {
			requestErrors = append(requestErrors, errors.New("graphql response has no data"))
		}
		return nil, nil, errors.Join(requestErrors...)
	}
	return response.Data, aliasErrors, errors.Join(requestErrors...)
}

func firstPathElement(path []any) (string, bool) {
	if len(path) == 0 {
		return "", false
	}
	alias, ok := path[0].(string)
	return alias, ok
}

func graphQLString(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

type PRSearch struct {
	Key     string
	Kind    string
	Query   string
	First   int
	Details bool
}

type SearchResult struct {
	Kind    string
	Refs    []PRRef
	PRs     []QueuedPR
	Details map[string]json.RawMessage
	Err     error
}

func buildSearchQuery(searches []PRSearch) string {
	var query strings.Builder
	for _, search := range searches {
		if search.Details {
			query.WriteString(prFieldsFragment)
			break
		}
	}
	query.WriteString("query {\n  viewer { login }\n")
	for _, search := range searches {
		selection := "url"
		if search.Details {
			selection = "...PRFields"
		}
		fmt.Fprintf(&query, "  %s: search(type: ISSUE, first: %d, query: %s) { nodes { ... on PullRequest { %s } } }\n", search.Key, search.First, graphQLString(search.Query), selection)
	}
	query.WriteString("}\n")
	return query.String()
}

func RunPRSearches(ctx context.Context, host string, searches []PRSearch) ([]SearchResult, error) {
	batches := batchSearches(searches)
	batchResults := make([][]SearchResult, len(batches))
	batchErrs := make([]error, len(batches))
	slots := make(chan struct{}, searchBatchesInFlight)
	var wg sync.WaitGroup
	for index, batch := range batches {
		wg.Add(1)
		go func() {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			batchResults[index], batchErrs[index] = runSearchBatchSplitting(ctx, host, batch)
		}()
	}
	wg.Wait()
	var results []SearchResult
	for _, batch := range batchResults {
		results = append(results, batch...)
	}
	return results, errors.Join(batchErrs...)
}

func batchSearches(searches []PRSearch) [][]PRSearch {
	var detailSearches, urlSearches []PRSearch
	for _, search := range searches {
		if search.Details {
			detailSearches = append(detailSearches, search)
		} else {
			urlSearches = append(urlSearches, search)
		}
	}
	return append(chunkSearches(detailSearches, maxDetailSearchesPerRequest), chunkSearches(urlSearches, maxURLSearchesPerRequest)...)
}

func chunkSearches(searches []PRSearch, size int) [][]PRSearch {
	var chunks [][]PRSearch
	for start := 0; start < len(searches); start += size {
		chunks = append(chunks, searches[start:min(start+size, len(searches))])
	}
	return chunks
}

func runSearchBatchSplitting(ctx context.Context, host string, batch []PRSearch) ([]SearchResult, error) {
	results, err := runSearchBatch(ctx, host, batch)
	if err == nil || len(batch) == 1 || !isServerTimeout(err) || ctx.Err() != nil {
		return results, err
	}
	half := len(batch) / 2
	firstResults, firstErr := runSearchBatchSplitting(ctx, host, batch[:half])
	secondResults, secondErr := runSearchBatchSplitting(ctx, host, batch[half:])
	return append(firstResults, secondResults...), errors.Join(firstErr, secondErr)
}

func isServerTimeout(err error) bool {
	message := strings.ToLower(err.Error())
	for _, marker := range []string{"respond to your request in time", "http 502", "http 504", "timeout"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func runSearchBatch(ctx context.Context, host string, searches []PRSearch) ([]SearchResult, error) {
	data, aliasErrors, err := queryGraphQL(ctx, host, buildSearchQuery(searches))
	if data == nil {
		return nil, err
	}
	var viewer loginNode
	if err := json.Unmarshal(data["viewer"], &viewer); err != nil || viewer.Login == "" {
		return nil, fmt.Errorf("graphql viewer login missing: %w", errors.Join(err, aliasErrors["viewer"]))
	}
	results := make([]SearchResult, 0, len(searches))
	for _, search := range searches {
		result := SearchResult{Kind: search.Kind, Details: map[string]json.RawMessage{}}
		if aliasErr := aliasErrors[search.Key]; aliasErr != nil {
			result.Err = fmt.Errorf("%s: %w", search.Query, aliasErr)
		} else if decodeErr := result.decode(data[search.Key], search.Details, viewer.Login); decodeErr != nil {
			result.Err = fmt.Errorf("%s: %w", search.Query, decodeErr)
		}
		results = append(results, result)
	}
	return results, nil
}

func (result *SearchResult) decode(raw json.RawMessage, details bool, viewer string) error {
	var connection struct {
		Nodes []json.RawMessage `json:"nodes"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &connection); err != nil {
			return err
		}
	}
	for _, node := range connection.Nodes {
		var pr graphPR
		if err := json.Unmarshal(node, &pr); err != nil {
			return err
		}
		if pr.URL == "" {
			continue
		}
		ref, err := ParsePRURL(pr.URL)
		if err != nil {
			return err
		}
		result.Refs = append(result.Refs, ref)
		if !details {
			continue
		}
		detailsJSON, err := json.Marshal(pr.toDetails())
		if err != nil {
			return err
		}
		parsed, err := ParsePRDetails(detailsJSON, viewer)
		if err != nil {
			return err
		}
		result.PRs = append(result.PRs, parsed)
		result.Details[ref.URL] = detailsJSON
	}
	return nil
}

type graphPR struct {
	Number         int       `json:"number"`
	Title          string    `json:"title"`
	URL            string    `json:"url"`
	State          string    `json:"state"`
	IsDraft        bool      `json:"isDraft"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
	Additions      int       `json:"additions"`
	Deletions      int       `json:"deletions"`
	ChangedFiles   int       `json:"changedFiles"`
	HeadRefOid     string    `json:"headRefOid"`
	HeadRefName    string    `json:"headRefName"`
	ReviewDecision string    `json:"reviewDecision"`
	Author         loginNode `json:"author"`
	Commits        struct {
		Nodes []struct {
			Commit struct {
				CommittedDate     time.Time `json:"committedDate"`
				StatusCheckRollup *struct {
					Contexts struct {
						Nodes []checkRun `json:"nodes"`
					} `json:"contexts"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
	ReviewRequests struct {
		Nodes []struct {
			RequestedReviewer reviewRequestNode `json:"requestedReviewer"`
		} `json:"nodes"`
	} `json:"reviewRequests"`
	LatestReviews struct {
		Nodes []reviewNode `json:"nodes"`
	} `json:"latestReviews"`
	Comments struct {
		Nodes []commentNode `json:"nodes"`
	} `json:"comments"`
	Files struct {
		Nodes []fileNode `json:"nodes"`
	} `json:"files"`
}

func (pr graphPR) toDetails() prDetails {
	details := prDetails{
		Number:         pr.Number,
		Title:          pr.Title,
		URL:            pr.URL,
		State:          pr.State,
		IsDraft:        pr.IsDraft,
		CreatedAt:      pr.CreatedAt,
		UpdatedAt:      pr.UpdatedAt,
		Additions:      pr.Additions,
		Deletions:      pr.Deletions,
		ChangedFiles:   pr.ChangedFiles,
		HeadRefOid:     pr.HeadRefOid,
		HeadRefName:    pr.HeadRefName,
		ReviewDecision: pr.ReviewDecision,
		Author:         pr.Author,
		LatestReviews:  pr.LatestReviews.Nodes,
		Comments:       pr.Comments.Nodes,
		Files:          pr.Files.Nodes,
	}
	for _, node := range pr.Commits.Nodes {
		details.Commits = append(details.Commits, commitNode{CommittedDate: node.Commit.CommittedDate})
		if node.Commit.StatusCheckRollup != nil {
			details.StatusCheckRollup = append(details.StatusCheckRollup, node.Commit.StatusCheckRollup.Contexts.Nodes...)
		}
	}
	for _, node := range pr.ReviewRequests.Nodes {
		details.ReviewRequests = append(details.ReviewRequests, node.RequestedReviewer)
	}
	return details
}

func FetchPRStates(ctx context.Context, refs []PRRef) (map[string]string, error) {
	refsByHost := map[string][]PRRef{}
	var hosts []string
	for _, ref := range refs {
		if _, found := refsByHost[ref.Host]; !found {
			hosts = append(hosts, ref.Host)
		}
		refsByHost[ref.Host] = append(refsByHost[ref.Host], ref)
	}
	states := map[string]string{}
	var errs []error
	for _, host := range hosts {
		hostRefs := refsByHost[host]
		for start := 0; start < len(hostRefs); start += maxStatesPerRequest {
			batch := hostRefs[start:min(start+maxStatesPerRequest, len(hostRefs))]
			if err := fetchStateBatch(ctx, host, batch, states); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", host, err))
			}
		}
	}
	if len(states) == 0 && len(errs) > 0 {
		return nil, summarizeErrors(errs)
	}
	return states, nil
}

func fetchStateBatch(ctx context.Context, host string, refs []PRRef, states map[string]string) error {
	var query strings.Builder
	query.WriteString("query {\n")
	for index, ref := range refs {
		fmt.Fprintf(&query, "  pr%d: repository(owner: %s, name: %s) { pullRequest(number: %d) { state } }\n", index, graphQLString(ref.Owner), graphQLString(ref.Repo), ref.Number)
	}
	query.WriteString("}\n")
	data, _, err := queryGraphQL(ctx, host, query.String())
	if data == nil {
		return err
	}
	for index, ref := range refs {
		var repository struct {
			PullRequest *struct {
				State string `json:"state"`
			} `json:"pullRequest"`
		}
		if raw := data[fmt.Sprintf("pr%d", index)]; len(raw) > 0 && json.Unmarshal(raw, &repository) == nil && repository.PullRequest != nil && repository.PullRequest.State != "" {
			states[ref.URL] = repository.PullRequest.State
		}
	}
	return err
}
