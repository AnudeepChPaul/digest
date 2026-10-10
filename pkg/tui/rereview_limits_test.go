package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/review"
)

func changesMarkdownFor(t *testing.T, comparison review.Comparison, err error) string {
	t.Helper()
	m := reReviewModel(t, nil)
	item := &m.git.ghPendingPRs[0]
	m.git.changesSince = map[string]changesSinceReview{item.PR.Ref.URL: {baseSHA: "abc", headSHA: item.PR.HeadSHA, comparison: comparison, err: err}}
	return m.changesSinceReviewMarkdown(item)
}

func TestChangesSinceReviewShowsTheLoadError(t *testing.T) {
	markdown := changesMarkdownFor(t, review.Comparison{}, errors.New("rate limited"))
	if !strings.Contains(markdown, "Could not load the changes: rate limited") || !strings.Contains(markdown, "(abc → def5678)") {
		t.Fatalf("markdown:\n%s", markdown)
	}
}

func TestChangesSinceReviewCapsCommitsAndFilesWithPlurals(t *testing.T) {
	var comparison review.Comparison
	for index := 0; index < 52; index++ {
		comparison.Commits = append(comparison.Commits, review.CompareCommit{SHA: "c", Headline: "commit", Date: time.Now()})
	}
	for index := 0; index < 103; index++ {
		comparison.Files = append(comparison.Files, review.CompareFile{Path: "f.go", Status: "modified", Additions: 1})
	}
	markdown := changesMarkdownFor(t, comparison, nil)
	for _, want := range []string{"**52 commits · 103 files · +103 / -0**", "- … and 2 more\n", "- … and 3 more\n"} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown missing %q:\n%s", want, markdown)
		}
	}
	if strings.Count(markdown, "- `c` commit") != 50 || strings.Count(markdown, "- `f.go`") != 100 {
		t.Fatalf("expected 50 commits and 100 files listed:\n%s", markdown)
	}
}

func TestChangesSinceReviewSingularCounts(t *testing.T) {
	markdown := changesMarkdownFor(t, review.Comparison{Commits: []review.CompareCommit{{SHA: "c", Date: time.Now()}}, Files: []review.CompareFile{{Path: "a"}}}, nil)
	if !strings.Contains(markdown, "**1 commit · 1 file · +0 / -0**") {
		t.Fatalf("markdown:\n%s", markdown)
	}
	if plural(0, "file") != "0 files" {
		t.Fatalf("plural(0) = %q", plural(0, "file"))
	}
}

func TestNoChangesSectionWhenTheHeadIsTheReviewedCommit(t *testing.T) {
	m := reReviewModel(t, func(context.Context, review.PRRef, string, string) (review.Comparison, error) {
		t.Fatal("nothing to compare")
		return review.Comparison{}, nil
	})
	item := &m.git.ghPendingPRs[0]
	if err := review.WriteMeta(review.StateDir(m.reviewRoot(), item.PR.Ref), review.Meta{Ref: item.PR.Ref, HeadSHA: item.PR.HeadSHA}); err != nil {
		t.Fatal(err)
	}
	if m.changesSinceReviewCmd(item) != nil || m.changesSinceReviewMarkdown(item) != "" {
		t.Fatal("no changes section when the head is the reviewed commit")
	}
	if shortSHA("abc") != "abc" {
		t.Fatal("short SHAs stay as they are")
	}
}

func TestStaleChangesResultsAreIgnoredAndUnlistedOnesDropped(t *testing.T) {
	m := reReviewModel(t, nil)
	url := m.git.ghPendingPRs[0].PR.Ref.URL
	m.git.changesSince = map[string]changesSinceReview{
		url:                       {baseSHA: "abc", headSHA: "def", loading: true},
		"https://gone/o/r/pull/1": {baseSHA: "x", headSHA: "y"},
	}
	next, _ := m.handleChangesSinceReview(changesSinceReviewMsg{url: url, baseSHA: "abc", headSHA: "other"})
	if next.(Model).git.changesSince[url].loading != true {
		t.Fatal("a result for an older head should be ignored")
	}
	next, _ = m.handleChangesSinceReview(changesSinceReviewMsg{url: url, baseSHA: "abc", headSHA: "def"})
	changes := next.(Model).git.changesSince
	if _, kept := changes["https://gone/o/r/pull/1"]; kept || changes[url].loading {
		t.Fatalf("changes = %+v", changes)
	}
}
