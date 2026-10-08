package jobs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/review"
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
	lookupPRStates = func(ctx context.Context, refs []review.PRRef) (map[string]string, error) {
		lookups++
		if len(refs) != 3 {
			t.Errorf("refs=%d", len(refs))
		}
		return map[string]string{"u-merged": "MERGED", "u-closed": "CLOSED", "u-open": "OPEN"}, nil
	}
	defer func() { lookupPRStates = original }()

	actions, failures := reapReviewClones(root, true)
	if lookups != 1 {
		t.Errorf("lookups = %d, want one batched call", lookups)
	}
	if len(actions) != 2 || len(failures) != 0 {
		t.Fatalf("dry run actions=%v failures=%v", actions, failures)
	}
	if _, err := os.Stat(review.CloneDir(root, merged)); err != nil {
		t.Errorf("dry run removed clone")
	}

	actions, failures = reapReviewClones(root, false)
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
	actions, failures := reapReviewClones(filepath.Join(t.TempDir(), "nope"), false)
	if len(actions) != 0 || len(failures) != 0 {
		t.Errorf("actions=%v failures=%v", actions, failures)
	}
}

func TestReapReviewClonesKeepsClonesWhenLookupFails(t *testing.T) {
	root := t.TempDir()
	ref := review.PRRef{Repo: "console", Number: 9, URL: "u-9"}
	seedReviewClone(t, root, ref)
	original := lookupPRStates
	lookupPRStates = func(ctx context.Context, refs []review.PRRef) (map[string]string, error) {
		return nil, errors.New("boom")
	}
	defer func() { lookupPRStates = original }()
	actions, failures := reapReviewClones(root, false)
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
	lookupPRStates = func(ctx context.Context, refs []review.PRRef) (map[string]string, error) {
		return map[string]string{}, nil
	}
	defer func() { lookupPRStates = original }()
	actions, failures := reapReviewClones(root, false)
	if len(actions) != 0 || len(failures) != 1 {
		t.Fatalf("actions=%v failures=%v", actions, failures)
	}
}

func TestReapReviewClonesSkipsRunningReviews(t *testing.T) {
	root := t.TempDir()
	ref := review.PRRef{Repo: "console", Number: 3, URL: "u-running"}
	seedReviewClone(t, root, ref)
	if err := os.WriteFile(filepath.Join(review.StateDir(root, ref), "review.pid"), []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		t.Fatal(err)
	}
	original := lookupPRStates
	lookupPRStates = func(ctx context.Context, refs []review.PRRef) (map[string]string, error) {
		return map[string]string{"u-running": "MERGED"}, nil
	}
	defer func() { lookupPRStates = original }()
	reapReviewClones(root, false)
	if _, err := os.Stat(review.CloneDir(root, ref)); err != nil {
		t.Error("a clone in use by a running review should be kept")
	}
}

func ageReviewClone(t *testing.T, root string, ref review.PRRef, age time.Duration) {
	t.Helper()
	stamp := time.Now().Add(-age)
	stateDir := review.StateDir(root, ref)
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{stateDir, review.CloneDir(root, ref)}
	for _, entry := range entries {
		paths = append(paths, filepath.Join(stateDir, entry.Name()))
	}
	for _, path := range paths {
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReapReviewClonesRemovesIdleClonesWhateverTheirState(t *testing.T) {
	root := t.TempDir()
	idle := review.PRRef{Repo: "console", Number: 4, URL: "u-idle"}
	fresh := review.PRRef{Repo: "console", Number: 5, URL: "u-fresh"}
	running := review.PRRef{Repo: "console", Number: 6, URL: "u-running"}
	for _, ref := range []review.PRRef{idle, fresh, running} {
		seedReviewClone(t, root, ref)
	}
	if err := os.WriteFile(filepath.Join(review.StateDir(root, running), "review.pid"), []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		t.Fatal(err)
	}
	ageReviewClone(t, root, idle, 8*24*time.Hour)
	ageReviewClone(t, root, running, 8*24*time.Hour)
	original := lookupPRStates
	lookupPRStates = func(ctx context.Context, refs []review.PRRef) (map[string]string, error) {
		return nil, errors.New("gh offline")
	}
	defer func() { lookupPRStates = original }()

	actions, _ := reapReviewClones(root, true)
	if len(actions) != 1 || !strings.Contains(actions[0], "idle") {
		t.Fatalf("dry run should list only the idle clone: %v", actions)
	}
	reapReviewClones(root, false)
	if _, err := os.Stat(review.CloneDir(root, idle)); err == nil {
		t.Error("idle clone should be removed even when the gh lookup fails")
	}
	if _, err := os.Stat(review.StateDir(root, idle)); err == nil {
		t.Error("idle clone state should be removed")
	}
	for _, ref := range []review.PRRef{fresh, running} {
		if _, err := os.Stat(review.CloneDir(root, ref)); err != nil {
			t.Errorf("%s clone should be kept", ref.DirName())
		}
	}
}
