package review

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const DefaultDetailsParallel = 8

type ActivityPR struct {
	Number     int
	Title      string
	URL        string
	Repository string
	State      string
	ReviewedAt time.Time
}

var runShell = func(ctx context.Context, command string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return nil, fmt.Errorf("%s", message)
		}
		return nil, err
	}
	return out, nil
}

func runGHCommand(ctx context.Context, command string, placeholders map[string]string) ([]byte, error) {
	for key, value := range placeholders {
		command = strings.ReplaceAll(command, "{"+key+"}", value)
	}
	return runShell(ctx, command)
}

func SearchPRs(ctx context.Context, command string, placeholders map[string]string) ([]PRRef, error) {
	out, err := runGHCommand(ctx, command, placeholders)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var refs []PRRef
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		ref, err := ParsePRURL(line)
		if err != nil {
			return nil, fmt.Errorf("pr search output: %w", err)
		}
		if seen[ref.URL] {
			continue
		}
		seen[ref.URL] = true
		refs = append(refs, ref)
	}
	return refs, nil
}

var (
	searchSortPattern  = regexp.MustCompile(`\bsort=(\w+)`)
	searchOrderPattern = regexp.MustCompile(`\border=(\w+)`)
)

func SearchSortOf(command string) (byCreated bool, ascending bool) {
	if match := searchSortPattern.FindStringSubmatch(command); match != nil {
		byCreated = match[1] == "created"
	}
	if match := searchOrderPattern.FindStringSubmatch(command); match != nil {
		ascending = match[1] == "asc"
	}
	return byCreated, ascending
}

func WithSearchSort(command string, byCreated bool, ascending bool) string {
	sortField, order := "updated", "desc"
	if byCreated {
		sortField = "created"
	}
	if ascending {
		order = "asc"
	}
	command = searchSortPattern.ReplaceAllString(command, "sort="+sortField)
	return searchOrderPattern.ReplaceAllString(command, "order="+order)
}

func FetchPRDetails(ctx context.Context, command string, ref PRRef) (json.RawMessage, error) {
	out, err := runGHCommand(ctx, command, map[string]string{"url": ref.URL})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ref.URL, err)
	}
	out = bytes.TrimSpace(out)
	if !json.Valid(out) {
		return nil, fmt.Errorf("%s: pr details are not JSON", ref.URL)
	}
	return json.RawMessage(out), nil
}

type checkRun struct {
	Typename   string `json:"__typename"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	State      string `json:"state"`
}

func ciStateFromChecks(checks []checkRun) string {
	if len(checks) == 0 {
		return ""
	}
	pending := false
	for _, check := range checks {
		outcome := check.State
		if check.Typename == "CheckRun" || check.Status != "" {
			if check.Status != "COMPLETED" {
				pending = true
				continue
			}
			outcome = check.Conclusion
		}
		switch outcome {
		case "FAILURE", "ERROR", "CANCELLED", "TIMED_OUT", "ACTION_REQUIRED", "STARTUP_FAILURE":
			return "FAILURE"
		case "PENDING", "EXPECTED", "QUEUED", "IN_PROGRESS", "WAITING", "":
			pending = true
		}
	}
	if pending {
		return "PENDING"
	}
	return "SUCCESS"
}

type prDetails struct {
	Number            int        `json:"number"`
	Title             string     `json:"title"`
	URL               string     `json:"url"`
	State             string     `json:"state"`
	IsDraft           bool       `json:"isDraft"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
	Additions         int        `json:"additions"`
	Deletions         int        `json:"deletions"`
	ChangedFiles      int        `json:"changedFiles"`
	HeadRefOid        string     `json:"headRefOid"`
	HeadRefName       string     `json:"headRefName"`
	ReviewDecision    string     `json:"reviewDecision"`
	Author            loginNode  `json:"author"`
	StatusCheckRollup []checkRun `json:"statusCheckRollup"`
	Commits           []struct {
		CommittedDate time.Time `json:"committedDate"`
	} `json:"commits"`
	ReviewRequests []struct {
		Typename string `json:"__typename"`
		Login    string `json:"login"`
		Name     string `json:"name"`
		Slug     string `json:"slug"`
	} `json:"reviewRequests"`
	LatestReviews []reviewNode  `json:"latestReviews"`
	Comments      []commentNode `json:"comments"`
	Files         []struct {
		Path string `json:"path"`
	} `json:"files"`
}

func ParsePRDetails(raw json.RawMessage, viewer string) (QueuedPR, error) {
	var details prDetails
	if err := json.Unmarshal(raw, &details); err != nil {
		return QueuedPR{}, fmt.Errorf("decode pr details: %w", err)
	}
	ref, err := ParsePRURL(details.URL)
	if err != nil {
		return QueuedPR{}, err
	}
	pr := QueuedPR{
		Ref:            ref,
		Title:          details.Title,
		Author:         details.Author.Login,
		State:          details.State,
		HeadSHA:        details.HeadRefOid,
		HeadRef:        details.HeadRefName,
		IsDraft:        details.IsDraft,
		Additions:      details.Additions,
		Deletions:      details.Deletions,
		ChangedFiles:   details.ChangedFiles,
		CIState:        ciStateFromChecks(details.StatusCheckRollup),
		ReviewDecision: details.ReviewDecision,
		CreatedAt:      details.CreatedAt,
		UpdatedAt:      details.UpdatedAt,
		RequestedAt:    details.CreatedAt,
		Approved:       details.ReviewDecision == "APPROVED",
	}
	if key := jiraKeyPattern.FindString(details.Title + " " + details.HeadRefName); key != "" {
		pr.JiraKey = key
	}
	for _, commit := range details.Commits {
		if commit.CommittedDate.After(pr.LastCommitAt) {
			pr.LastCommitAt = commit.CommittedDate
		}
	}
	for _, request := range details.ReviewRequests {
		if request.Typename == "Team" {
			pr.OwnerTeams = append(pr.OwnerTeams, request.Name)
		}
	}
	pr.applyReviews(viewer, details.LatestReviews)
	for _, comment := range details.Comments {
		pr.applyReply(viewer, comment)
	}
	for _, file := range details.Files {
		pr.Files = append(pr.Files, file.Path)
	}
	return pr, nil
}

var (
	viewerMu     sync.Mutex
	viewerByHost = map[string]string{}
)

func ViewerLogin(ctx context.Context, host string) (string, error) {
	viewerMu.Lock()
	defer viewerMu.Unlock()
	if login, found := viewerByHost[host]; found {
		return login, nil
	}
	out, err := runGHCommand(ctx, "gh api user --hostname {host} --jq .login", map[string]string{"host": host})
	if err != nil {
		return "", fmt.Errorf("login on %s: %w", host, err)
	}
	login := strings.TrimSpace(string(out))
	if login == "" {
		return "", fmt.Errorf("login on %s: empty", host)
	}
	viewerByHost[host] = login
	return login, nil
}

var runAuthStatus = func(ctx context.Context) ([]byte, error) {
	return exec.CommandContext(ctx, "gh", "auth", "status").CombinedOutput()
}

var loggedInHostPattern = regexp.MustCompile(`(?:Logged|log) in to (\S+)`)

func parseAuthHosts(output string) []string {
	seen := map[string]bool{}
	var hosts []string
	for _, match := range loggedInHostPattern.FindAllStringSubmatch(output, -1) {
		if host := match[1]; !seen[host] {
			seen[host] = true
			hosts = append(hosts, host)
		}
	}
	sort.Strings(hosts)
	return hosts
}

func GHHosts(ctx context.Context) ([]string, error) {
	output, statusErr := runAuthStatus(ctx)
	hosts := parseAuthHosts(string(output))
	if len(hosts) == 0 {
		return nil, fmt.Errorf("gh auth status lists no logged-in hosts: %v %s", statusErr, strings.TrimSpace(string(output)))
	}
	return hosts, nil
}

type detailsCall struct {
	done chan struct{}
	raw  json.RawMessage
	err  error
}

type DetailsFetcher struct {
	command  string
	previous map[string]json.RawMessage
	slots    chan struct{}
	mu       sync.Mutex
	calls    map[string]*detailsCall
}

func NewDetailsFetcher(command string, parallel int, previous map[string]json.RawMessage) *DetailsFetcher {
	return &DetailsFetcher{
		command:  command,
		previous: previous,
		slots:    make(chan struct{}, max(parallel, 1)),
		calls:    map[string]*detailsCall{},
	}
}

func (f *DetailsFetcher) Fetch(ctx context.Context, ref PRRef) (json.RawMessage, error) {
	f.mu.Lock()
	call, found := f.calls[ref.URL]
	if !found {
		call = &detailsCall{done: make(chan struct{})}
		f.calls[ref.URL] = call
	}
	f.mu.Unlock()
	if !found {
		select {
		case f.slots <- struct{}{}:
			call.raw, call.err = FetchPRDetails(ctx, f.command, ref)
			<-f.slots
		case <-ctx.Done():
			call.err = ctx.Err()
		}
		close(call.done)
	} else {
		select {
		case <-call.done:
		case <-ctx.Done():
			return f.previous[ref.URL], ctx.Err()
		}
	}
	if call.err != nil {
		return f.previous[ref.URL], call.err
	}
	return call.raw, nil
}

func (f *DetailsFetcher) Load(ctx context.Context, refs []PRRef) ([]QueuedPR, map[string]json.RawMessage, error) {
	type result struct {
		pr  QueuedPR
		raw json.RawMessage
		err error
		ok  bool
	}
	results := make([]result, len(refs))
	var wg sync.WaitGroup
	for index, ref := range refs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			raw, fetchErr := f.Fetch(ctx, ref)
			results[index].err = fetchErr
			if raw == nil {
				return
			}
			viewer, viewerErr := ViewerLogin(ctx, ref.Host)
			if viewerErr != nil {
				results[index].err = errors.Join(fetchErr, viewerErr)
				return
			}
			pr, parseErr := ParsePRDetails(raw, viewer)
			if parseErr != nil {
				results[index].err = errors.Join(fetchErr, parseErr)
				return
			}
			results[index] = result{pr: pr, raw: raw, err: fetchErr, ok: true}
		}()
	}
	wg.Wait()
	var prs []QueuedPR
	details := map[string]json.RawMessage{}
	var errs []error
	for index, res := range results {
		if res.err != nil {
			errs = append(errs, res.err)
		}
		if !res.ok {
			continue
		}
		prs = append(prs, res.pr)
		details[refs[index].URL] = res.raw
	}
	return prs, details, summarizeErrors(errs)
}

func summarizeErrors(errs []error) error {
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	}
	return fmt.Errorf("%d pr details failed, first: %w", len(errs), errs[0])
}

func FetchPRStates(ctx context.Context, detailsCommand string, refs []PRRef) (map[string]string, error) {
	fetcher := NewDetailsFetcher(detailsCommand, DefaultDetailsParallel, nil)
	states := map[string]string{}
	var mu sync.Mutex
	var errs []error
	var wg sync.WaitGroup
	for _, ref := range refs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			raw, err := fetcher.Fetch(ctx, ref)
			var details struct {
				State string `json:"state"`
			}
			if err == nil {
				err = json.Unmarshal(raw, &details)
			}
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			if details.State != "" {
				states[ref.URL] = details.State
			}
		}()
	}
	wg.Wait()
	if len(states) == 0 && len(errs) > 0 {
		return nil, summarizeErrors(errs)
	}
	return states, nil
}
