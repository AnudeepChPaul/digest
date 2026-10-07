package sourcecontrol

import (
	"testing"

	"app/pkg/review"
)

func TestOutdatedCheckRunsOncePerPR(t *testing.T) {
	calls := map[string]int{}
	isOutdated := memoizeByURL(func(pr review.QueuedPR) bool {
		calls[pr.Ref.URL]++
		return pr.Ref.URL == "u/1"
	})
	first, second := review.QueuedPR{Ref: review.PRRef{URL: "u/1"}}, review.QueuedPR{Ref: review.PRRef{URL: "u/2"}}
	for range 3 {
		if !isOutdated(first) || isOutdated(second) {
			t.Fatalf("memoized answers changed")
		}
	}
	if calls["u/1"] != 1 || calls["u/2"] != 1 {
		t.Errorf("calls = %v", calls)
	}
}
