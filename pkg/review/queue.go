package review

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

const StaleAfter = 48 * time.Hour

var jiraKeyPattern = regexp.MustCompile(`\b[A-Z][A-Z0-9]+-[0-9]+\b`)

type QueuedPR struct {
	Ref               PRRef
	Title             string
	Author            string
	State             string
	HeadSHA           string
	HeadRef           string
	IsDraft           bool
	Additions         int
	Deletions         int
	ChangedFiles      int
	CIState           string
	ReviewDecision    string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	RequestedAt       time.Time
	LastCommitAt      time.Time
	MyLastReviewAt    time.Time
	MyLastReviewState string
	LastReplyAt       time.Time
	CodeOwner         bool
	OwnerTeams        []string
	Files             []string
	Approved          bool
	JiraKey           string
}

func (p QueuedPR) Size() int {
	return p.Additions + p.Deletions
}

func (p QueuedPR) IsStale(now time.Time) bool {
	return now.Sub(p.RequestedAt) > StaleAfter
}

type loginNode struct {
	Login string `json:"login"`
}

type reviewNode struct {
	State       string    `json:"state"`
	Author      loginNode `json:"author"`
	SubmittedAt time.Time `json:"submittedAt"`
}

type commentNode struct {
	CreatedAt time.Time `json:"createdAt"`
	Author    loginNode `json:"author"`
}

func isBot(login string) bool {
	return strings.HasSuffix(login, "[bot]") || strings.HasPrefix(login, "copilot") || strings.HasPrefix(login, "svc-")
}

func (p *QueuedPR) applyReviews(viewer string, reviews []reviewNode) {
	for _, review := range reviews {
		if review.Author.Login == viewer && review.SubmittedAt.After(p.MyLastReviewAt) {
			p.MyLastReviewAt = review.SubmittedAt
			p.MyLastReviewState = review.State
		}
		if review.State == "APPROVED" && !isBot(review.Author.Login) {
			p.Approved = true
		}
	}
}

func (p *QueuedPR) applyReply(viewer string, comment commentNode) {
	if comment.Author.Login == viewer || isBot(comment.Author.Login) {
		return
	}
	if comment.CreatedAt.After(p.LastReplyAt) {
		p.LastReplyAt = comment.CreatedAt
	}
}

func NeedsReReview(pr QueuedPR) bool {
	if pr.MyLastReviewAt.IsZero() {
		return false
	}
	return pr.LastCommitAt.After(pr.MyLastReviewAt) || pr.LastReplyAt.After(pr.MyLastReviewAt)
}

func SelectReReviews(reviewed, pending []QueuedPR) []QueuedPR {
	pendingURLs := make(map[string]bool, len(pending))
	for _, pr := range pending {
		pendingURLs[pr.Ref.URL] = true
	}
	var selected []QueuedPR
	for _, pr := range reviewed {
		if !pendingURLs[pr.Ref.URL] && NeedsReReview(pr) {
			selected = append(selected, pr)
		}
	}
	return selected
}

func (p QueuedPR) ActivityAt(byCreated bool) time.Time {
	if byCreated || p.UpdatedAt.IsZero() {
		return p.CreatedAt
	}
	return p.UpdatedAt
}

func SortPRs(prs []QueuedPR, byCreated bool, ascending bool) {
	sort.SliceStable(prs, func(i, j int) bool {
		left, right := prs[i].ActivityAt(byCreated), prs[j].ActivityAt(byCreated)
		if ascending {
			return left.Before(right)
		}
		return left.After(right)
	})
}

func VisibleRanked(prs []QueuedPR, greenOnly bool) []QueuedPR {
	var visible []QueuedPR
	for _, pr := range prs {
		if pr.IsDraft {
			continue
		}
		if greenOnly && pr.CIState != "SUCCESS" {
			continue
		}
		visible = append(visible, pr)
	}
	return visible
}
