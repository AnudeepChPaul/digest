package sourcecontrol

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/review"
)

const (
	PendingReviewKind = "Pending Review"
	ReReviewKind      = "Re-review"
	ReviewedKind      = "Reviewed"
	CommitKind        = "Commit"
)

type PRItem struct {
	Title      string `json:"title"`
	URL        string `json:"url"`
	Kind       string `json:"kind"`
	Number     int    `json:"number"`
	Repository string `json:"repository,omitempty"`

	PR *review.QueuedPR `json:"-"`
}

func NewPRItem(pr review.QueuedPR, kind string) PRItem {
	queued := pr
	return PRItem{
		Title:      fmt.Sprintf("PR #%d: %s", pr.Ref.Number, pr.Title),
		URL:        pr.Ref.URL,
		Kind:       kind,
		Number:     pr.Ref.Number,
		Repository: pr.Ref.Repo,
		PR:         &queued,
	}
}

type Day int

const (
	Today Day = iota
	Yesterday
)

type DayResult struct {
	Day         Day
	Date        string
	Reviewed    []PRItem
	Reviews     []review.ActivityPR
	Details     map[string]json.RawMessage
	FailedHosts []string
	Err         error
}

type PendingResult struct {
	Items       []PRItem
	Details     map[string]json.RawMessage
	StartedAt   time.Time
	FailedHosts []string
	Err         error
}

type MyPRsResult struct {
	PRs         []review.QueuedPR
	Closed      map[string]string
	StartedAt   time.Time
	FailedHosts []string
	Err         error
}

type Section struct {
	Day     *DayResult
	Pending *PendingResult
	MyPRs   *MyPRsResult
}
