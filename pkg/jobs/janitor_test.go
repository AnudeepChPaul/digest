package jobs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"app/pkg/review"
)

func seedReviewClone(t *testing.T, root string, ref review.PRRef) {
	t.Helper()
	if err := os.MkdirAll(review.CloneDir(root, ref), 0755); err != nil {
		t.Fatal(err)
	}
	if err := review.WriteMeta(review.StateDir(root, ref), review.Meta{Ref: ref}); err != nil {
		t.Fatal(err)
	}
}

func TestReapReviewClones(t *testing.T) {
	root := t.TempDir()
	merged := review.PRRef{Repo: "console", Number: 1, URL: "u-merged"}
	closed := review.PRRef{Repo: "console", Number: 2, URL: "u-closed"}
	open := review.PRRef{Repo: "web-console", Number: 1, URL: "u-open"}
	for _, ref := range []review.PRRef{merged, closed, open} {
		seedReviewClone(t, root, ref)
	}
	orphan := filepath.Join(root, "random_dir")
	_ = os.MkdirAll(orphan, 0755)

	original := lookupPRStates
	var lookups int
	lookupPRStates = func(ctx context.Context, detailsCommand string, refs []review.PRRef) (map[string]string, error) {
		lookups++
		if len(refs) != 3 {
			t.Errorf("refs=%d", len(refs))
		}
		return map[string]string{"u-merged": "MERGED", "u-closed": "CLOSED", "u-open": "OPEN"}, nil
	}
	defer func() { lookupPRStates = original }()

	actions, failures := reapReviewClones(root, "", true)
	if lookups != 1 {
		t.Errorf("lookups = %d, want one batched call", lookups)
	}
	if len(actions) != 2 || len(failures) != 0 {
		t.Fatalf("dry run actions=%v failures=%v", actions, failures)
	}
	if _, err := os.Stat(review.CloneDir(root, merged)); err != nil {
		t.Errorf("dry run removed clone")
	}

	actions, failures = reapReviewClones(root, "", false)
	if len(actions) != 2 || len(failures) != 0 {
		t.Fatalf("actions=%v failures=%v", actions, failures)
	}
	for _, ref := range []review.PRRef{merged, closed} {
		if _, err := os.Stat(review.CloneDir(root, ref)); err == nil {
			t.Errorf("%s clone still present", ref.DirName())
		}
		if _, err := os.Stat(review.StateDir(root, ref)); err == nil {
			t.Errorf("%s state still present", ref.DirName())
		}
	}
	if _, err := os.Stat(review.CloneDir(root, open)); err != nil {
		t.Errorf("open PR clone removed")
	}
	if _, err := os.Stat(orphan); err != nil {
		t.Errorf("unrelated dir removed")
	}
}

func TestReapReviewClonesMissingRoot(t *testing.T) {
	actions, failures := reapReviewClones(filepath.Join(t.TempDir(), "nope"), "", false)
	if len(actions) != 0 || len(failures) != 0 {
		t.Errorf("actions=%v failures=%v", actions, failures)
	}
}

func TestReapReviewClonesKeepsClonesWhenLookupFails(t *testing.T) {
	root := t.TempDir()
	ref := review.PRRef{Repo: "console", Number: 9, URL: "u-9"}
	seedReviewClone(t, root, ref)
	original := lookupPRStates
	lookupPRStates = func(ctx context.Context, detailsCommand string, refs []review.PRRef) (map[string]string, error) {
		return nil, errors.New("boom")
	}
	defer func() { lookupPRStates = original }()
	actions, failures := reapReviewClones(root, "", false)
	if len(actions) != 0 || len(failures) != 1 {
		t.Fatalf("actions=%v failures=%v", actions, failures)
	}
	if _, err := os.Stat(review.CloneDir(root, ref)); err != nil {
		t.Errorf("clone removed after failed lookup")
	}
}

func TestReapReviewClonesReportsMissingState(t *testing.T) {
	root := t.TempDir()
	ref := review.PRRef{Repo: "console", Number: 9, URL: "u-9"}
	seedReviewClone(t, root, ref)
	original := lookupPRStates
	lookupPRStates = func(ctx context.Context, detailsCommand string, refs []review.PRRef) (map[string]string, error) {
		return map[string]string{}, nil
	}
	defer func() { lookupPRStates = original }()
	actions, failures := reapReviewClones(root, "", false)
	if len(actions) != 0 || len(failures) != 1 {
		t.Fatalf("actions=%v failures=%v", actions, failures)
	}
}
