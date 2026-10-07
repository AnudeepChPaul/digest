package review

import (
	"sort"
	"testing"
	"time"
)

func TestApprovedIgnoresBots(t *testing.T) {
	pr := QueuedPR{}
	pr.applyReviews("me", []reviewNode{{State: "APPROVED", Author: loginNode{Login: "copilot-pull-request-reviewer[bot]"}}})
	if pr.Approved {
		t.Errorf("bot approval counted")
	}
}

func TestVisibleRanked(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	prs := []QueuedPR{
		{Ref: PRRef{Number: 1}, CIState: "SUCCESS", CreatedAt: now.Add(-1 * time.Hour), Additions: 10},
		{Ref: PRRef{Number: 2}, CIState: "FAILURE", CreatedAt: now.Add(-50 * time.Hour)},
		{Ref: PRRef{Number: 3}, CIState: "SUCCESS", IsDraft: true, CreatedAt: now.Add(-50 * time.Hour)},
		{Ref: PRRef{Number: 4}, CIState: "SUCCESS", CreatedAt: now.Add(-30 * time.Hour), Additions: 500},
		{Ref: PRRef{Number: 5}, CIState: "SUCCESS", CreatedAt: now.Add(-2 * time.Hour), CodeOwner: true},
		{Ref: PRRef{Number: 6}, CIState: "PENDING", CreatedAt: now.Add(-2 * time.Hour)},
		{Ref: PRRef{Number: 7}, CIState: "SUCCESS", CreatedAt: now.Add(-30 * time.Hour), Additions: 20},
	}
	got := VisibleRanked(prs, true)
	sortPRs(got, false, false)
	want := []int{1, 5, 4, 7}
	if len(got) != len(want) {
		t.Fatalf("got %d prs, want %d", len(got), len(want))
	}
	for i, n := range want {
		if got[i].Ref.Number != n {
			t.Errorf("pos %d = #%d, want #%d", i, got[i].Ref.Number, n)
		}
	}
	sortPRs(got, false, true)
	if got[0].Ref.Number != 4 || got[len(got)-1].Ref.Number != 1 {
		t.Errorf("ascending order = %v", got)
	}
	all := VisibleRanked(prs, false)
	if len(all) != 6 {
		t.Errorf("green_only=false kept %d, want 6 (drafts still hidden)", len(all))
	}
}

func TestIsStale(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	if !(QueuedPR{RequestedAt: now.Add(-49 * time.Hour)}).IsStale(now) {
		t.Errorf("49h should be stale")
	}
	if (QueuedPR{RequestedAt: now.Add(-47 * time.Hour)}).IsStale(now) {
		t.Errorf("47h should not be stale")
	}
}

func TestNeedsReReview(t *testing.T) {
	review := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		pr   QueuedPR
		want bool
	}{
		{"never reviewed", QueuedPR{LastCommitAt: review}, false},
		{"commit after review", QueuedPR{MyLastReviewAt: review, LastCommitAt: review.Add(time.Minute)}, true},
		{"reply after review", QueuedPR{MyLastReviewAt: review, LastCommitAt: review.Add(-time.Hour), LastReplyAt: review.Add(time.Minute)}, true},
		{"nothing new", QueuedPR{MyLastReviewAt: review, LastCommitAt: review.Add(-time.Hour), LastReplyAt: review.Add(-time.Minute)}, false},
	}
	for _, c := range cases {
		if got := NeedsReReview(c.pr); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestReReviewsExcludesPending(t *testing.T) {
	review := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	reviewed := []QueuedPR{
		{Ref: PRRef{URL: "u1"}, MyLastReviewAt: review, LastCommitAt: review.Add(time.Hour)},
		{Ref: PRRef{URL: "u2"}, MyLastReviewAt: review, LastCommitAt: review.Add(time.Hour)},
		{Ref: PRRef{URL: "u3"}, MyLastReviewAt: review, LastCommitAt: review.Add(-time.Hour)},
	}
	pending := []QueuedPR{{Ref: PRRef{URL: "u2"}}}
	got := SelectReReviews(reviewed, pending, NeedsReReview)
	if len(got) != 1 || got[0].Ref.URL != "u1" {
		t.Errorf("got %+v", got)
	}
}

func TestApplyReviewsKeepsMyLatestState(t *testing.T) {
	older := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	for _, state := range []string{"APPROVED", "CHANGES_REQUESTED", "COMMENTED"} {
		pr := QueuedPR{}
		pr.applyReviews("me", []reviewNode{
			{State: "COMMENTED", Author: loginNode{Login: "me"}, SubmittedAt: older},
			{State: state, Author: loginNode{Login: "me"}, SubmittedAt: newer},
			{State: "APPROVED", Author: loginNode{Login: "someone"}, SubmittedAt: newer.Add(time.Hour)},
		})
		if pr.MyLastReviewState != state {
			t.Errorf("MyLastReviewState = %q, want %q", pr.MyLastReviewState, state)
		}
	}
	other := QueuedPR{}
	other.applyReviews("me", []reviewNode{{State: "APPROVED", Author: loginNode{Login: "someone"}, SubmittedAt: newer}})
	if other.MyLastReviewState != "" {
		t.Errorf("someone else's review set %q", other.MyLastReviewState)
	}
}

func sortPRs(prs []QueuedPR, byCreated bool, ascending bool) {
	sort.SliceStable(prs, func(i, j int) bool {
		left, right := prs[i].ActivityAt(byCreated), prs[j].ActivityAt(byCreated)
		if ascending {
			return left.Before(right)
		}
		return left.After(right)
	})
}
