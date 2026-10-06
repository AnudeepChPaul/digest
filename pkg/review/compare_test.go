package review

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCompareSinceParsesCommitsAndFiles(t *testing.T) {
	var gotArgs []string
	original := runGHAPI
	runGHAPI = func(ctx context.Context, args ...string) ([]byte, error) {
		gotArgs = args
		return []byte(`{"commits":[{"sha":"abc1234def","message":"Fix nil\n\nlong body","author":"alice","date":"2026-10-05T10:00:00Z"}],"files":[{"filename":"a.go","status":"modified","additions":3,"deletions":1}]}`), nil
	}
	t.Cleanup(func() { runGHAPI = original })
	ref := PRRef{Host: "git.example.com", Owner: "o", Repo: "r", Number: 1}
	comparison, err := CompareSince(context.Background(), ref, "old", "new")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(gotArgs, " ")
	if !strings.Contains(joined, "--hostname git.example.com") || !strings.Contains(joined, "repos/o/r/compare/old...new") || !strings.Contains(joined, "--jq") {
		t.Errorf("args = %v", gotArgs)
	}
	if len(comparison.Commits) != 1 || comparison.Commits[0].Headline != "Fix nil" || comparison.Commits[0].Author != "alice" || !comparison.Commits[0].Date.Equal(time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("commits = %+v", comparison.Commits)
	}
	if len(comparison.Files) != 1 || comparison.Files[0] != (CompareFile{Path: "a.go", Status: "modified", Additions: 3, Deletions: 1}) {
		t.Errorf("files = %+v", comparison.Files)
	}
}

func TestCompareSinceMissingBaseIsReported(t *testing.T) {
	original := runGHAPI
	runGHAPI = func(ctx context.Context, args ...string) ([]byte, error) {
		return []byte("gh: Not Found (HTTP 404)"), errors.New("exit status 1")
	}
	t.Cleanup(func() { runGHAPI = original })
	if _, err := CompareSince(context.Background(), PRRef{Host: "github.com", Owner: "o", Repo: "r"}, "gone", "new"); !errors.Is(err, ErrReviewedCommitGone) {
		t.Errorf("err = %v, want ErrReviewedCommitGone", err)
	}
}
