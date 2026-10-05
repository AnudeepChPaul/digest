package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"app/pkg/review"
	"app/pkg/sourcecontrol"
)

func locallyReviewedItem(t *testing.T, m Model, number int, finishedAt time.Time, kind string, pr review.QueuedPR) GitPRItem {
	t.Helper()
	pr.Ref = prRef("console", number)
	pr.CIState = "SUCCESS"
	if !finishedAt.IsZero() {
		dir := review.StateDir(m.reviewRoot(), pr.Ref)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		findingsPath := filepath.Join(dir, review.FindingsFile)
		if err := os.WriteFile(findingsPath, []byte(`{"findings":[]}`), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "review.exit"), []byte("0"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(findingsPath, finishedAt, finishedAt); err != nil {
			t.Fatal(err)
		}
	}
	return sourcecontrol.NewPRItem(pr, kind)
}

func TestPRStatusFollowsMyProgress(t *testing.T) {
	m := selectionTestModel(t)
	localDone := time.Now().Add(-2 * time.Hour)
	later := localDone.Add(time.Hour)
	earlier := localDone.Add(-time.Hour)
	cases := []struct {
		name       string
		finishedAt time.Time
		kind       string
		pr         review.QueuedPR
		want       review.PRState
	}{
		{"others approved, I have not acted", localDone, "Pending Review", review.QueuedPR{Approved: true}, review.StateReviewed},
		{"I approved after local review", localDone, "Pending Review", review.QueuedPR{MyLastReviewState: "APPROVED", MyLastReviewAt: later}, review.StateApproved},
		{"I requested changes after local review", localDone, "Pending Review", review.QueuedPR{MyLastReviewState: "CHANGES_REQUESTED", MyLastReviewAt: later}, review.StateChangesRequested},
		{"I commented after local review", localDone, "Pending Review", review.QueuedPR{MyLastReviewState: "COMMENTED", MyLastReviewAt: later}, review.StateCommented},
		{"my review is older than local review", localDone, "Pending Review", review.QueuedPR{MyLastReviewState: "APPROVED", MyLastReviewAt: earlier}, review.StateReviewed},
		{"no local review, I commented", time.Time{}, "Pending Review", review.QueuedPR{MyLastReviewState: "COMMENTED", MyLastReviewAt: earlier}, review.StateCommented},
		{"nothing at all", time.Time{}, "Pending Review", review.QueuedPR{}, review.StatePending},
		{"others approved, no local review", time.Time{}, "Pending Review", review.QueuedPR{Approved: true}, review.StatePending},
		{"dismissed review", time.Time{}, "Pending Review", review.QueuedPR{MyLastReviewState: "DISMISSED", MyLastReviewAt: earlier}, review.StatePending},
		{"re-review with changes requested", time.Time{}, sourcecontrol.ReReviewKind, review.QueuedPR{MyLastReviewState: "CHANGES_REQUESTED", MyLastReviewAt: earlier}, review.StateChangesRequested},
	}
	for index, c := range cases {
		item := locallyReviewedItem(t, m, 100+index, c.finishedAt, c.kind, c.pr)
		if got := m.prState(&item); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

func TestPRStatusTagText(t *testing.T) {
	m := selectionTestModel(t)
	item := locallyReviewedItem(t, m, 200, time.Time{}, "Pending Review", review.QueuedPR{MyLastReviewState: "CHANGES_REQUESTED", MyLastReviewAt: time.Now()})
	if tag := m.renderPRTag(&item, false); !strings.Contains(tag, "Changes requested") {
		t.Errorf("row tag = %q", tag)
	}
	if badge := m.prStateBadge(&item); !strings.Contains(badge, "CHANGES REQUESTED") {
		t.Errorf("badge = %q", badge)
	}
}

func TestCachedPRKeepsMyReviewState(t *testing.T) {
	m := syncTestModel(t)
	pending := sourcecontrol.NewPRItem(review.QueuedPR{Ref: prRef("console", 5), CIState: "SUCCESS", MyLastReviewState: "COMMENTED", MyLastReviewAt: time.Now()}, "Pending Review")
	m.applyGitPending(gitPendingMsg{generation: m.fetchGeneration, pending: []GitPRItem{pending}})
	saveCacheNow(t, m)
	fresh := NewModel(m.cfg, nil)
	if len(fresh.ghPendingPRs) != 1 || fresh.ghPendingPRs[0].PR.MyLastReviewState != "COMMENTED" {
		t.Errorf("cached = %+v", fresh.ghPendingPRs)
	}
}

func TestFailedLocalReviewThenMyGitHubReview(t *testing.T) {
	m := selectionTestModel(t)
	failedAt := time.Now().Add(-2 * time.Hour)
	for _, c := range []struct {
		reviewedAt time.Time
		want       review.PRState
	}{
		{time.Time{}, review.StateFailed},
		{failedAt.Add(-time.Hour), review.StateFailed},
		{failedAt.Add(time.Hour), review.StateApproved},
	} {
		pr := review.QueuedPR{Ref: prRef("console", 300), CIState: "SUCCESS"}
		if !c.reviewedAt.IsZero() {
			pr.MyLastReviewState, pr.MyLastReviewAt = "APPROVED", c.reviewedAt
		}
		dir := review.StateDir(m.reviewRoot(), pr.Ref)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		exitPath := filepath.Join(dir, "review.exit")
		if err := os.WriteFile(exitPath, []byte("1"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(exitPath, failedAt, failedAt); err != nil {
			t.Fatal(err)
		}
		item := sourcecontrol.NewPRItem(pr, "Pending Review")
		if got := m.prState(&item); got != c.want {
			t.Errorf("reviewed at %v: %s, want %s", c.reviewedAt, got, c.want)
		}
	}
}
