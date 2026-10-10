package jobs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/review"

	"github.com/charmbracelet/log"
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

func seedPartialClone(t *testing.T, root string, ref review.PRRef) string {
	t.Helper()
	partialDir := review.CloneDir(root, ref) + review.PartialCloneSuffix
	if err := os.MkdirAll(filepath.Join(partialDir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	return partialDir
}

func TestReapReviewClonesRemovesLeftoverPartialClones(t *testing.T) {
	root := t.TempDir()
	withoutState := seedPartialClone(t, root, review.PRRef{Repo: "console", Number: 7})
	stoppedRef := review.PRRef{Repo: "console", Number: 8, URL: "u-stopped"}
	if err := review.WriteMeta(review.StateDir(root, stoppedRef), review.Meta{Ref: stoppedRef}); err != nil {
		t.Fatal(err)
	}
	stopped := seedPartialClone(t, root, stoppedRef)
	original := lookupPRStates
	lookupPRStates = func(ctx context.Context, refs []review.PRRef) (map[string]string, error) {
		return map[string]string{"u-stopped": "OPEN"}, nil
	}
	defer func() { lookupPRStates = original }()

	actions, _ := reapReviewClones(root, true)
	if len(actions) != 2 || !strings.Contains(strings.Join(actions, "\n"), "would remove partial clone") {
		t.Errorf("dry run actions = %v", actions)
	}
	for _, partialDir := range []string{withoutState, stopped} {
		if _, err := os.Stat(partialDir); err != nil {
			t.Errorf("dry run removed %s", partialDir)
		}
	}

	actions, failures := reapReviewClones(root, false)
	if len(actions) != 2 || len(failures) != 0 {
		t.Fatalf("actions=%v failures=%v", actions, failures)
	}
	for _, partialDir := range []string{withoutState, stopped} {
		if _, err := os.Stat(partialDir); err == nil {
			t.Errorf("leftover %s still present", partialDir)
		}
	}
}

func TestReapReviewClonesKeepsThePartialCloneOfARunningReview(t *testing.T) {
	root := t.TempDir()
	ref := review.PRRef{Repo: "console", Number: 3, URL: "u-running"}
	if err := review.WriteMeta(review.StateDir(root, ref), review.Meta{Ref: ref}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(review.StateDir(root, ref), "review.pid"), []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		t.Fatal(err)
	}
	partialDir := seedPartialClone(t, root, ref)
	reapReviewClones(root, false)
	if _, err := os.Stat(partialDir); err != nil {
		t.Error("a review that is still cloning should keep its partial folder")
	}
}

func TestReapingAReviewCloneRemovesItsPartialFolder(t *testing.T) {
	root := t.TempDir()
	ref := review.PRRef{Repo: "console", Number: 1, URL: "u-merged"}
	seedReviewClone(t, root, ref)
	partialDir := seedPartialClone(t, root, ref)
	if action, failure := (reviewFolders{ref: ref, stateDir: review.StateDir(root, ref), cloneDir: review.CloneDir(root, ref)}).remove("merged", false); action == "" || failure != "" {
		t.Fatalf("action=%q failure=%q", action, failure)
	}
	if _, err := os.Stat(partialDir); err == nil {
		t.Error("partial folder left after reaping the clone")
	}
}

func TestReapReviewClonesHandlesLegacyFolderNames(t *testing.T) {
	root := t.TempDir()
	ref := review.PRRef{Owner: "o", Repo: "console", Number: 1, URL: "u-merged-legacy"}
	legacyClone := filepath.Join(root, "console_1")
	if err := os.MkdirAll(filepath.Join(legacyClone, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := review.WriteMeta(filepath.Join(root, ".state", "console_1"), review.Meta{Ref: ref}); err != nil {
		t.Fatal(err)
	}
	original := lookupPRStates
	lookupPRStates = func(ctx context.Context, refs []review.PRRef) (map[string]string, error) {
		return map[string]string{"u-merged-legacy": "MERGED"}, nil
	}
	defer func() { lookupPRStates = original }()
	if actions, failures := reapReviewClones(root, false); len(actions) != 1 || len(failures) != 0 {
		t.Fatalf("actions=%v failures=%v", actions, failures)
	}
	for _, leftover := range []string{legacyClone, review.CloneDir(root, ref), filepath.Join(root, ".state", "console_1")} {
		if _, err := os.Stat(leftover); err == nil {
			t.Errorf("%s left behind", leftover)
		}
	}
}

func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		snapshot[path] = fmt.Sprintf("%v %d %d", info.Mode(), info.Size(), info.ModTime().UnixNano())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestJanitorDryRunLeavesReviewFoldersUntouched(t *testing.T) {
	root := t.TempDir()
	legacy := review.PRRef{Owner: "o", Repo: "console", Number: 1, URL: "u-legacy"}
	if err := os.MkdirAll(filepath.Join(root, "console_1", ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := review.WriteMeta(filepath.Join(root, ".state", "console_1"), review.Meta{Ref: legacy}); err != nil {
		t.Fatal(err)
	}
	stale := review.PRRef{Owner: "o", Repo: "api", Number: 2, URL: "u-stale"}
	seedReviewClone(t, root, stale)
	if err := os.WriteFile(filepath.Join(review.StateDir(root, stale), "review.pid"), []byte("999999"), 0600); err != nil {
		t.Fatal(err)
	}
	seedPartialClone(t, root, review.PRRef{Owner: "o", Repo: "web", Number: 3})
	original := lookupPRStates
	lookupPRStates = func(ctx context.Context, refs []review.PRRef) (map[string]string, error) {
		return map[string]string{"u-legacy": "MERGED", "u-stale": "MERGED"}, nil
	}
	defer func() { lookupPRStates = original }()

	before := snapshotTree(t, root)
	actions, failures := reapReviewClones(root, true)
	after := snapshotTree(t, root)
	if len(failures) != 0 {
		t.Fatalf("failures = %v", failures)
	}
	for path, state := range before {
		if after[path] != state {
			t.Errorf("dry run changed %s: %q -> %q", path, state, after[path])
		}
	}
	for path := range after {
		if _, existed := before[path]; !existed {
			t.Errorf("dry run created %s", path)
		}
	}
	report := strings.Join(actions, "\n")
	for _, want := range []string{filepath.Join(root, "console_1"), review.CloneDir(root, stale)} {
		if !strings.Contains(report, "would remove review clone "+want+" (merged)") {
			t.Errorf("dry run should report %s as merged:\n%s", want, report)
		}
	}
}

func TestJanitorDryRunSummarySaysWhatWouldHappen(t *testing.T) {
	job := &JanitorJob{Roots: []string{t.TempDir()}, QuarantineRoot: filepath.Join(t.TempDir(), "q"), ReviewRoot: t.TempDir(), Logger: log.New(io.Discard)}
	result, err := job.Run(true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Summary, "removed") || strings.Contains(result.Summary, "reclaimed") || !strings.Contains(result.Summary, "to remove") {
		t.Errorf("dry-run summary = %q", result.Summary)
	}
}

func TestJanitorRealRunQuarantinesOnlyMatchingFiles(t *testing.T) {
	scanRoot := t.TempDir()
	quarantineRoot := filepath.Join(t.TempDir(), "quarantine")
	for _, name := range []string{"build.log", "notes.tmp", "keep.txt", ".hidden.tmp"} {
		if err := os.WriteFile(filepath.Join(scanRoot, name), []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(scanRoot, "folder.tmp"), 0755); err != nil {
		t.Fatal(err)
	}

	job := &JanitorJob{Roots: []string{scanRoot}, Patterns: []string{"*.tmp", "*.log"}, QuarantineRoot: quarantineRoot, GraceDays: 7, Logger: log.New(io.Discard)}
	result, err := job.Run(false)
	if err != nil {
		t.Fatal(err)
	}
	dateDir := filepath.Join(quarantineRoot, time.Now().Format("2006-01-02"))
	for _, name := range []string{"build.log", "notes.tmp"} {
		if _, err := os.Stat(filepath.Join(scanRoot, name)); err == nil {
			t.Errorf("%s left in the scanned root", name)
		}
		if content, err := os.ReadFile(filepath.Join(dateDir, name)); err != nil || string(content) != name {
			t.Errorf("%s not quarantined into %s: %q %v", name, dateDir, content, err)
		}
	}
	for _, name := range []string{"keep.txt", ".hidden.tmp", "folder.tmp"} {
		if _, err := os.Stat(filepath.Join(scanRoot, name)); err != nil {
			t.Errorf("%s should stay in place: %v", name, err)
		}
	}
	if len(result.ActionsTaken) != 2 || !strings.HasPrefix(result.Summary, "2 quarantined") {
		t.Errorf("result = %+v, want two quarantined files", result)
	}
	if NeedsUserAction(result, false) {
		t.Errorf("a clean quarantine needs no user action: %+v", result)
	}
}
