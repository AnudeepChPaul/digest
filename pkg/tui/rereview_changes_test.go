package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/review"
	"github.com/AnudeepChPaul/digest/pkg/sourcecontrol"

	tea "github.com/charmbracelet/bubbletea"
)

func reReviewModel(t *testing.T, compare func(ctx context.Context, ref review.PRRef, baseSHA, headSHA string) (review.Comparison, error)) Model {
	t.Helper()
	original := compareSince
	compareSince = compare
	t.Cleanup(func() { compareSince = original })
	m := reviewTestModel(t)
	pr := *m.ghPendingPRs[0].PR
	pr.HeadSHA = "def5678aaaa"
	if err := review.WriteMeta(review.StateDir(m.reviewRoot(), pr.Ref), review.Meta{Ref: pr.Ref, HeadSHA: "abc1234bbbb"}); err != nil {
		t.Fatal(err)
	}
	m.ghPendingPRs = []GitPRItem{sourcecontrol.NewPRItem(pr, sourcecontrol.ReReviewKind)}
	m.rebuildGitRepoStats()
	m.mode = ViewDashboard
	return m
}

func openAndSettle(t *testing.T, m Model) Model {
	t.Helper()
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	for _, msg := range collectMsgs(cmd) {
		if changes, ok := msg.(changesSinceReviewMsg); ok {
			next, _ = m.Update(changes)
			m = next.(Model)
		}
	}
	return m
}

func TestReReviewModalShowsChangesSinceLocalReview(t *testing.T) {
	var compared []string
	m := reReviewModel(t, func(ctx context.Context, ref review.PRRef, baseSHA, headSHA string) (review.Comparison, error) {
		compared = append(compared, baseSHA+"..."+headSHA)
		return review.Comparison{
			Commits: []review.CompareCommit{{SHA: "1111111abc", Headline: "Address feedback", Author: "dev", Date: time.Now().Add(-2 * time.Hour)}},
			Files:   []review.CompareFile{{Path: "a.ts", Status: "modified", Additions: 4, Deletions: 2}},
		}, nil
	})
	m = openAndSettle(t, m)
	if len(compared) != 1 || compared[0] != "abc1234bbbb...def5678aaaa" {
		t.Fatalf("compared = %v", compared)
	}
	content := stripANSI(m.previewViewport.View())
	for _, want := range []string{"Changes since your review", "abc1234", "def5678", "1 commit", "1111111", "Address feedback", "a.ts", "+4", "-2"} {
		if !strings.Contains(content, want) {
			t.Errorf("modal missing %q:\n%s", want, content)
		}
	}
	m.mode = ViewDashboard
	if openAndSettle(t, m); len(compared) != 1 {
		t.Errorf("same head should reuse the cached comparison, compared %v", compared)
	}
}

func TestReReviewModalShowsLoadingWhileFetching(t *testing.T) {
	m := reReviewModel(t, func(ctx context.Context, ref review.PRRef, baseSHA, headSHA string) (review.Comparison, error) {
		return review.Comparison{}, nil
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if content := stripANSI(next.(Model).previewViewport.View()); !strings.Contains(content, "Changes since your review") || !strings.Contains(content, "Loading") {
		t.Errorf("modal should show loading while fetching:\n%s", content)
	}
}

func TestReReviewModalExplainsMissingReviewedCommit(t *testing.T) {
	m := reReviewModel(t, func(ctx context.Context, ref review.PRRef, baseSHA, headSHA string) (review.Comparison, error) {
		return review.Comparison{}, review.ErrReviewedCommitGone
	})
	m = openAndSettle(t, m)
	if content := stripANSI(m.previewViewport.View()); !strings.Contains(content, "force-push") {
		t.Errorf("modal should explain the missing commit:\n%s", content)
	}
}
