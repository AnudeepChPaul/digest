package review

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

type ActivityPR struct {
	Number     int
	Title      string
	URL        string
	Repository string
	State      string
	ReviewedAt time.Time
	Comments   string
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
	Number            int                 `json:"number"`
	Title             string              `json:"title"`
	Body              string              `json:"body,omitempty"`
	URL               string              `json:"url"`
	State             string              `json:"state"`
	IsDraft           bool                `json:"isDraft"`
	CreatedAt         time.Time           `json:"createdAt"`
	UpdatedAt         time.Time           `json:"updatedAt"`
	Additions         int                 `json:"additions"`
	Deletions         int                 `json:"deletions"`
	ChangedFiles      int                 `json:"changedFiles"`
	HeadRefOid        string              `json:"headRefOid"`
	HeadRefName       string              `json:"headRefName"`
	ReviewDecision    string              `json:"reviewDecision"`
	Author            loginNode           `json:"author"`
	StatusCheckRollup []checkRun          `json:"statusCheckRollup"`
	Commits           []commitNode        `json:"commits"`
	ReviewRequests    []reviewRequestNode `json:"reviewRequests"`
	LatestReviews     []reviewNode        `json:"latestReviews"`
	Comments          []commentNode       `json:"comments"`
	Files             []fileNode          `json:"files"`
}

type commitNode struct {
	CommittedDate time.Time `json:"committedDate"`
}

type reviewRequestNode struct {
	Typename string `json:"__typename"`
	Login    string `json:"login"`
	Name     string `json:"name"`
	Slug     string `json:"slug"`
}

type fileNode struct {
	Path string `json:"path"`
}

func ParsePRDetails(raw json.RawMessage, viewer string) (QueuedPR, error) {
	var details prDetails
	if err := json.Unmarshal(raw, &details); err != nil {
		return QueuedPR{}, fmt.Errorf("decode pr details: %w", err)
	}
	plainTextFields(&details)
	ref, err := ParsePRURL(details.URL)
	if err != nil {
		return QueuedPR{}, err
	}
	pr := QueuedPR{
		Ref:            ref,
		Title:          details.Title,
		Body:           details.Body,
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
		if request.Typename == "User" && viewer != "" && strings.EqualFold(request.Login, viewer) {
			pr.DirectRequest = true
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

var runAuthStatus = func(ctx context.Context) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", "auth", "status")
	cmd.WaitDelay = commandWaitDelay
	return cmd.CombinedOutput()
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
		return nil, fmt.Errorf("gh auth status lists no logged-in hosts (%v); run gh auth login", statusErr)
	}
	return hosts, nil
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
