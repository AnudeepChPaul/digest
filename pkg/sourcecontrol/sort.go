package sourcecontrol

import (
	"sort"

	"app/pkg/review"
)

type Sort struct {
	ByCreated bool `json:"by_created"`
	Ascending bool `json:"ascending"`
}

func SortFromCommand(command string) Sort {
	byCreated, ascending := review.SearchSortOf(command)
	return Sort{ByCreated: byCreated, Ascending: ascending}
}

func (s Sort) apply(command string) string {
	return review.WithSearchSort(command, s.ByCreated, s.Ascending)
}

func SortItems(items []PRItem, activeSort Sort) {
	sort.SliceStable(items, func(i, j int) bool {
		left, right := items[i].PR, items[j].PR
		if left == nil || right == nil {
			return left != nil
		}
		leftAt, rightAt := left.ActivityAt(activeSort.ByCreated), right.ActivityAt(activeSort.ByCreated)
		if activeSort.Ascending {
			return leftAt.Before(rightAt)
		}
		return leftAt.After(rightAt)
	})
}
