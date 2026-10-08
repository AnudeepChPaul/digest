package tui

import (
	"testing"

	"github.com/AnudeepChPaul/digest/pkg/review"
)

func TestReviewCopyTextAddsFindingsBySeverity(t *testing.T) {
	item := &GitPRItem{Title: "Fix picker", URL: "https://github.com/o/console/pull/4"}
	findings := []review.Finding{
		{Severity: "low", Title: "Rename var", Path: "a.go", Line: 3},
		{Severity: "high", Title: "Nil deref", Path: "b.go", Line: 10, Body: "x can be nil", Suggestion: "guard it"},
		{Severity: "high", Title: "Leak"},
	}
	want := "Fix picker\nhttps://github.com/o/console/pull/4\n\n" +
		"## Review findings\n\n" +
		"### HIGH (2)\n\n" +
		"- **Nil deref** — `b.go:10`\n  x can be nil\n  Suggestion: guard it\n" +
		"- **Leak**\n\n" +
		"### LOW (1)\n\n" +
		"- **Rename var** — `a.go:3`"
	if got := reviewCopyText(item, findings); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestReviewCopyTextWithoutFindingsIsTitleAndURL(t *testing.T) {
	item := &GitPRItem{Title: "Fix picker", URL: "https://github.com/o/console/pull/4"}
	if got := reviewCopyText(item, nil); got != "Fix picker\nhttps://github.com/o/console/pull/4" {
		t.Errorf("got %q", got)
	}
}
