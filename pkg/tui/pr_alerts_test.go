package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/notify"
	"github.com/achandrapaul/digest/pkg/paths"
	"github.com/achandrapaul/digest/pkg/review"
	"github.com/achandrapaul/digest/pkg/sourcecontrol"
)

func alertPR(url, title, decision string) review.QueuedPR {
	return review.QueuedPR{Ref: review.PRRef{URL: url}, Title: title, ReviewDecision: decision}
}

func requestItem(url, title, kind string, requestedAt time.Time) GitPRItem {
	return GitPRItem{URL: url, Title: title, Kind: kind, PR: &review.QueuedPR{Ref: review.PRRef{URL: url}, Title: title, Author: "sam", RequestedAt: requestedAt}}
}

func captureAlerts(t *testing.T) *[]notify.Notification {
	t.Helper()
	var sent []notify.Notification
	previous := notify.Send
	notify.Send = func(notification notify.Notification) error {
		sent = append(sent, notification)
		return nil
	}
	t.Cleanup(func() { notify.Send = previous })
	return &sent
}

func TestPRAlertsFireOnTheFirstSyncThenOncePerChange(t *testing.T) {
	sent := captureAlerts(t)
	path := filepath.Join(t.TempDir(), "pr-status.json")
	requested := time.Now().Add(-time.Hour)
	baseline := prSnapshot([]review.QueuedPR{alertPR("u/1", "Add trial", "REVIEW_REQUIRED"), alertPR("u/2", "Fix login", "")}, nil, []GitPRItem{requestItem("u/9", "Old ask", sourcecontrol.PendingReviewKind, requested)})
	if err := sendPRAlerts(path, baseline); err != nil || len(*sent) != 3 {
		t.Fatalf("first sync should alert on every PR, mine included: err %v sent %v", err, *sent)
	}
	var firstTitles []string
	for _, alert := range *sent {
		firstTitles = append(firstTitles, alert.Title)
	}
	if joined := strings.Join(firstTitles, " | "); strings.Count(joined, "PR opened") != 2 || !strings.Contains(joined, "Review requested") {
		t.Errorf("first sync alerts = %q", joined)
	}
	*sent = nil
	next := prSnapshot(
		[]review.QueuedPR{alertPR("u/1", "Add trial", "APPROVED")},
		map[string]string{"u/2": "MERGED"},
		[]GitPRItem{requestItem("u/9", "Old ask", sourcecontrol.ReReviewKind, time.Now()), requestItem("u/10", "New ask", sourcecontrol.PendingReviewKind, time.Now())},
	)
	if err := sendPRAlerts(path, next); err != nil {
		t.Fatal(err)
	}
	var subtitles []string
	for _, alert := range *sent {
		subtitles = append(subtitles, alert.Title)
		if alert.OpenURL == "" || alert.Message == "" {
			t.Errorf("alert should open the PR and carry a line: %+v", alert)
		}
	}
	joined := strings.Join(subtitles, " | ")
	for _, want := range []string{"PR approved", "PR merged", "Re-review requested", "Review requested"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %q", want, joined)
		}
	}
	if len(*sent) != 4 {
		t.Errorf("want 4 alerts, got %d: %q", len(*sent), joined)
	}
	*sent = nil
	if err := sendPRAlerts(path, next); err != nil || len(*sent) != 0 {
		t.Errorf("the same state should not alert again: %v", *sent)
	}
}

func TestMyPRReviewStatesMapToAlerts(t *testing.T) {
	sent := captureAlerts(t)
	path := filepath.Join(t.TempDir(), "pr-status.json")
	prs := []review.QueuedPR{alertPR("u/1", "One", ""), alertPR("u/2", "Two", ""), alertPR("u/3", "Three", "")}
	if err := sendPRAlerts(path, prSnapshot(prs, nil, nil)); err != nil {
		t.Fatal(err)
	}
	commented := alertPR("u/3", "Three", "")
	commented.Reviews = []review.PRReview{{Author: "kim", State: "COMMENTED", SubmittedAt: time.Now()}}
	changed := []review.QueuedPR{alertPR("u/1", "One", "CHANGES_REQUESTED"), commented}
	if err := sendPRAlerts(path, prSnapshot(changed, map[string]string{"u/2": "CLOSED"}, nil)); err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, alert := range *sent {
		titles = append(titles, alert.Title)
	}
	joined := strings.Join(titles, " | ")
	for _, want := range []string{"Changes requested", "PR closed", "New comments"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %q", want, joined)
		}
	}
}

func TestPRStatusCacheIsOwnerOnly(t *testing.T) {
	captureAlerts(t)
	statusPath := filepath.Join(t.TempDir(), "cache", "pr-status.json")
	if err := sendPRAlerts(statusPath, prSnapshot([]review.QueuedPR{alertPR("u/1", "Add trial", "")}, nil, nil)); err != nil {
		t.Fatal(err)
	}
	fileInfo, fileErr := os.Stat(statusPath)
	dirInfo, dirErr := os.Stat(filepath.Dir(statusPath))
	if fileErr != nil || dirErr != nil || fileInfo.Mode().Perm() != paths.PrivateFileMode || dirInfo.Mode().Perm() != paths.PrivateDirMode {
		t.Errorf("cache modes = %v %v (%v %v)", fileInfo, dirInfo, fileErr, dirErr)
	}
}
