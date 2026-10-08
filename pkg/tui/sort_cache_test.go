package tui

import (
	"testing"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/review"
	"github.com/AnudeepChPaul/digest/pkg/sourcecontrol"
)

func pendingCreatedOn(number int, created time.Time) GitPRItem {
	return sourcecontrol.NewPRItem(review.QueuedPR{Ref: prRef("console", number), CreatedAt: created, UpdatedAt: created}, "Pending Review")
}

func outOfOrderPending() []GitPRItem {
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	return []GitPRItem{
		pendingCreatedOn(1, base),
		pendingCreatedOn(3, base.AddDate(0, 0, 2)),
		pendingCreatedOn(2, base.AddDate(0, 0, 1)),
	}
}

func pendingNumbers(items []GitPRItem) []int {
	numbers := make([]int, 0, len(items))
	for _, item := range items {
		numbers = append(numbers, item.Number)
	}
	return numbers
}

func assertNumbers(t *testing.T, label string, items []GitPRItem, want []int) {
	t.Helper()
	got := pendingNumbers(items)
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s = %v, want %v", label, got, want)
		}
	}
}

func TestCachedPendingUsesSavedSort(t *testing.T) {
	m := syncTestModel(t)
	savedSort := sourcecontrol.Sort{ByCreated: true, Ascending: false}
	cache := gitSyncCache{Date: m.currentDate.Format("2006-01-02"), Pending: toCachedItems(outOfOrderPending()), PendingSort: &savedSort}
	if err := saveGitCache(cache); err != nil {
		t.Fatal(err)
	}
	fresh := NewModel(m.cfg, nil)
	assertNumbers(t, "displayed pending", fresh.pendingGitAction, []int{3, 2, 1})
}

func TestSyncedPendingUsesSavedSort(t *testing.T) {
	m := syncTestModel(t)
	m.pendingSort = sourcecontrol.Sort{ByCreated: true, Ascending: true}
	m.applyGitPending(gitPendingMsg{generation: m.fetchGeneration, pending: outOfOrderPending()})
	assertNumbers(t, "displayed pending", m.pendingGitAction, []int{1, 2, 3})
}
