package sourcecontrol

import (
	"sort"
)

type Sort struct {
	ByCreated bool `json:"by_created"`
	Ascending bool `json:"ascending"`
}

func (s Sort) qualifier() string {
	field, order := "updated", "desc"
	if s.ByCreated {
		field = "created"
	}
	if s.Ascending {
		order = "asc"
	}
	return "sort:" + field + "-" + order
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
