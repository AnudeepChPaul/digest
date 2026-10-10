package jobs

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/review"

	"github.com/charmbracelet/log"
)

func quarantineBatch(t *testing.T, quarantineRoot string, age time.Duration) string {
	t.Helper()
	batch := filepath.Join(quarantineRoot, time.Now().Add(-age).Format("2006-01-02"))
	if err := os.MkdirAll(batch, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(batch, "old.hprof"), []byte("heap"), 0644); err != nil {
		t.Fatal(err)
	}
	return batch
}

func TestJanitorGraceDaysDecidesWhichBatchesArePurged(t *testing.T) {
	quarantineRoot := t.TempDir()
	tenDaysOld := quarantineBatch(t, quarantineRoot, 10*24*time.Hour)
	twentyDaysOld := quarantineBatch(t, quarantineRoot, 20*24*time.Hour)
	if err := os.WriteFile(filepath.Join(quarantineRoot, "stray.txt"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(quarantineRoot, "not-a-date"), 0755); err != nil {
		t.Fatal(err)
	}

	needsAction, err := RunJanitor([]string{t.TempDir()}, nil, "", quarantineRoot, 14, true)
	if err != nil || !needsAction {
		t.Fatalf("dry run: needsAction = %v, err = %v; a purge to do needs a look", needsAction, err)
	}
	if _, err := os.Stat(twentyDaysOld); err != nil {
		t.Error("dry run purged a batch")
	}
	if _, err := RunJanitor([]string{t.TempDir()}, nil, "", quarantineRoot, 14, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(twentyDaysOld); err == nil {
		t.Error("a batch older than grace_days should be purged")
	}
	if _, err := os.Stat(tenDaysOld); err != nil {
		t.Error("a batch inside grace_days should be kept")
	}
	job := &JanitorJob{Roots: []string{t.TempDir()}, QuarantineRoot: quarantineRoot, GraceDays: 7, Logger: log.New(io.Discard)}
	if _, err := job.Run(false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tenDaysOld); err == nil {
		t.Error("grace_days 7 should purge the ten day old batch")
	}
}

func TestJanitorCountsAFailedPurgeAndRetriesItNextRun(t *testing.T) {
	quarantineRoot := t.TempDir()
	batch := quarantineBatch(t, quarantineRoot, 30*24*time.Hour)
	stuck := filepath.Join(batch, "stuck")
	if err := os.MkdirAll(stuck, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stuck, "file"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(stuck, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(stuck, 0755) })

	job := &JanitorJob{Roots: []string{t.TempDir()}, QuarantineRoot: quarantineRoot, GraceDays: 14, Logger: log.New(io.Discard)}
	result, err := job.Run(false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(result.Summary, "· 0 expired batches purged · 0 review clones removed · 1 failed") || !result.Degraded {
		t.Errorf("result = %+v, want the failed purge counted", result)
	}
	if !strings.Contains(result.Details, "purge "+batch) {
		t.Errorf("details = %q, want the batch named", result.Details)
	}

	if err := os.Chmod(stuck, 0755); err != nil {
		t.Fatal(err)
	}
	result, err = job.Run(false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(batch); err == nil || result.Degraded || !strings.Contains(result.Summary, "1 expired batches purged") {
		t.Errorf("next run should retry the purge: %+v", result)
	}
}

func TestJanitorMissingOrUnreadableRootIsAnError(t *testing.T) {
	quarantineRoot := t.TempDir()
	scanned := t.TempDir()
	if err := os.WriteFile(filepath.Join(scanned, "x.hprof"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := RunJanitor([]string{scanned, filepath.Join(t.TempDir(), "nope")}, []string{"*.hprof"}, "", quarantineRoot, 14, false); err == nil {
		t.Error("a missing --root should be an error")
	}
	if _, err := os.Stat(filepath.Join(scanned, "x.hprof")); err != nil {
		t.Error("nothing should be quarantined when a root is missing")
	}
	unreadable := t.TempDir()
	if err := os.Chmod(unreadable, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0755) })
	if _, err := RunJanitor([]string{unreadable}, []string{"*.hprof"}, "", quarantineRoot, 14, false); err == nil {
		t.Error("an unreadable --root should be an error")
	}
}

func TestJanitorUnexpandableRootsAreErrors(t *testing.T) {
	unset := "$DIGEST_TEST_UNSET_VARIABLE/x"
	if _, err := RunJanitor([]string{unset}, nil, "", t.TempDir(), 14, false); err == nil {
		t.Error("an unexpandable --root should be an error")
	}
	if _, err := RunJanitor([]string{t.TempDir()}, nil, "", unset, 14, false); err == nil {
		t.Error("an unexpandable quarantine root should be an error")
	}
	job := &JanitorJob{Roots: []string{t.TempDir()}, QuarantineRoot: t.TempDir(), ReviewRoot: unset, Logger: log.New(io.Discard)}
	result, err := job.Run(false)
	if err != nil || !result.Degraded {
		t.Errorf("an unexpandable review root is a failure: %+v %v", result, err)
	}
}

func TestJanitorReportsFilesItCannotQuarantine(t *testing.T) {
	scanned := t.TempDir()
	if err := os.WriteFile(filepath.Join(scanned, "dump.hprof"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(t.TempDir(), "quarantine")
	if err := os.WriteFile(blocker, nil, 0644); err != nil {
		t.Fatal(err)
	}
	job := &JanitorJob{Roots: []string{scanned}, Patterns: []string{"*.hprof"}, QuarantineRoot: blocker, Logger: log.New(io.Discard)}
	result, err := job.Run(false)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Degraded || !strings.Contains(result.Details, "mkdir ") {
		t.Errorf("result = %+v, want a failed mkdir", result)
	}

	quarantineRoot := t.TempDir()
	dateDir := filepath.Join(quarantineRoot, time.Now().Format("2006-01-02"))
	if err := os.MkdirAll(dateDir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dateDir, 0755) })
	job.QuarantineRoot = quarantineRoot
	result, err = job.Run(false)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Degraded || !strings.Contains(result.Details, "move ") {
		t.Errorf("result = %+v, want a failed move", result)
	}

	job.QuarantineRoot = t.TempDir()
	result, err = job.Run(true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(result.Summary, "1 to quarantine") {
		t.Errorf("dry run = %+v", result)
	}
	if _, err := os.Stat(filepath.Join(scanned, "dump.hprof")); err != nil {
		t.Error("dry run moved the file")
	}
}

func TestReapReviewClonesRemovesIdleStateWithoutMetadata(t *testing.T) {
	root := t.TempDir()
	stateRoot := filepath.Join(root, ".state")
	idleState := filepath.Join(stateRoot, "lost_1")
	freshState := filepath.Join(stateRoot, "lost_2")
	unreadable := filepath.Join(stateRoot, "lost_3")
	for _, dir := range []string{idleState, freshState, unreadable, filepath.Join(root, "lost_1")} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(unreadable, "meta.json"), []byte("{broken"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateRoot, "stray-file"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().Add(-8 * 24 * time.Hour)
	for _, path := range []string{idleState, filepath.Join(root, "lost_1"), unreadable, filepath.Join(unreadable, "meta.json")} {
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	original := lookupPRStates
	lookupPRStates = func(ctx context.Context, refs []review.PRRef) (map[string]string, error) {
		t.Errorf("no metadata means nothing to look up: %v", refs)
		return nil, nil
	}
	defer func() { lookupPRStates = original }()

	actions, failures := reapReviewClones(root, false)
	if len(actions) != 2 || len(failures) != 0 {
		t.Fatalf("actions = %v, failures = %v", actions, failures)
	}
	for _, removed := range []string{idleState, filepath.Join(root, "lost_1"), unreadable} {
		if _, err := os.Stat(removed); err == nil {
			t.Errorf("%s should be removed after 7 idle days", removed)
		}
	}
	if _, err := os.Stat(freshState); err != nil {
		t.Error("a recently touched state folder should be kept")
	}
}

func TestReviewFolderRemovalReportsFailures(t *testing.T) {
	parent := t.TempDir()
	cloneDir := filepath.Join(parent, "clone")
	if err := os.MkdirAll(filepath.Join(cloneDir, "inner"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cloneDir, "inner", "file"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(cloneDir, "inner"), 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(cloneDir, "inner"), 0755) })
	if action, failure := (reviewFolders{cloneDir: cloneDir, stateDir: t.TempDir()}).remove("idle 7d", false); action != "" || !strings.HasPrefix(failure, "remove "+cloneDir) {
		t.Errorf("clone removal: action %q failure %q", action, failure)
	}

	stateParent := t.TempDir()
	stateDir := filepath.Join(stateParent, "state")
	if err := os.MkdirAll(filepath.Join(stateDir, "inner"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "inner", "file"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(stateDir, "inner"), 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(stateDir, "inner"), 0755) })
	if action, failure := (reviewFolders{cloneDir: filepath.Join(t.TempDir(), "gone"), stateDir: stateDir}).remove("merged", false); action != "" || !strings.HasPrefix(failure, "remove state "+stateDir) {
		t.Errorf("state removal: action %q failure %q", action, failure)
	}
}

func TestReapPartialClonesReportsFailures(t *testing.T) {
	root := t.TempDir()
	partial := filepath.Join(root, "console_1"+review.PartialCloneSuffix)
	if err := os.MkdirAll(filepath.Join(partial, "inner"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(partial, "inner", "file"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(partial, "inner"), 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(partial, "inner"), 0755) })
	var failures []string
	reapPartialClones(root, filepath.Join(root, ".state"), false, func(action, failure string) {
		if failure != "" {
			failures = append(failures, failure)
		}
	})
	if len(failures) != 1 || !strings.HasPrefix(failures[0], "remove "+partial) {
		t.Errorf("failures = %v", failures)
	}
}

func TestNeedsUserActionWithoutAResult(t *testing.T) {
	if NeedsUserAction(nil, true) {
		t.Error("no result needs no action")
	}
}

func TestJanitorReportsAFileThatVanishesBeforeItIsQuarantined(t *testing.T) {
	scanned := t.TempDir()
	if err := os.WriteFile(filepath.Join(scanned, "dump.hprof"), []byte("heap"), 0644); err != nil {
		t.Fatal(err)
	}
	job := &JanitorJob{Roots: []string{scanned, scanned}, Patterns: []string{"*.hprof"}, QuarantineRoot: t.TempDir(), Logger: log.New(io.Discard)}
	result, err := job.Run(false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(result.Summary, "1 quarantined") || !strings.Contains(result.Details, "stat ") {
		t.Errorf("result = %+v, want one quarantined and one failed stat", result)
	}
}

func TestJanitorRunLogsReviewClonesItRemoves(t *testing.T) {
	reviewRoot := t.TempDir()
	merged := review.PRRef{Repo: "console", Number: 1, URL: "u-merged"}
	seedReviewClone(t, reviewRoot, merged)
	original := lookupPRStates
	lookupPRStates = func(ctx context.Context, refs []review.PRRef) (map[string]string, error) {
		return map[string]string{"u-merged": "MERGED"}, nil
	}
	defer func() { lookupPRStates = original }()

	job := &JanitorJob{Roots: []string{t.TempDir()}, QuarantineRoot: t.TempDir(), ReviewRoot: reviewRoot}
	result, err := job.Run(true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(result.Summary, "1 review clones to remove") {
		t.Errorf("dry run = %+v", result)
	}
	result, err = job.Run(false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(result.Summary, "1 review clones removed") || !result.Changed {
		t.Errorf("real run = %+v", result)
	}
}

func TestReapReviewClonesReportsAnIdleCloneItCannotRemove(t *testing.T) {
	root := t.TempDir()
	ref := review.PRRef{Repo: "console", Number: 4, URL: "u-idle"}
	seedReviewClone(t, root, ref)
	inner := filepath.Join(review.CloneDir(root, ref), "inner")
	if err := os.MkdirAll(inner, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inner, "file"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	ageReviewClone(t, root, ref, 8*24*time.Hour)
	if err := os.Chmod(inner, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(inner, 0755) })

	actions, failures := reapReviewClones(root, false)
	if len(actions) != 0 || len(failures) != 1 {
		t.Errorf("actions = %v, failures = %v", actions, failures)
	}
}

func TestJanitorNoQuarantineDeletesMatchesForGood(t *testing.T) {
	scanned := t.TempDir()
	heapDump := filepath.Join(scanned, "java.hprof")
	if err := os.WriteFile(heapDump, []byte("heap"), 0644); err != nil {
		t.Fatal(err)
	}
	quarantineRoot := filepath.Join(t.TempDir(), "q")
	job := &JanitorJob{Roots: []string{scanned}, Patterns: []string{"*.hprof"}, QuarantineRoot: quarantineRoot, NoQuarantine: true, Logger: log.New(io.Discard)}

	result, err := job.Run(true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(heapDump); err != nil || !strings.HasPrefix(result.Summary, "1 to delete") {
		t.Errorf("dry run: summary %q, file kept err %v", result.Summary, err)
	}

	if result, err = job.Run(false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(heapDump); !os.IsNotExist(err) {
		t.Errorf("the match should be deleted, stat err = %v", err)
	}
	if _, err := os.Stat(quarantineRoot); !os.IsNotExist(err) {
		t.Errorf("nothing should go to quarantine, stat err = %v", err)
	}
	if !strings.HasPrefix(result.Summary, "1 deleted") || !strings.Contains(strings.Join(result.ActionsTaken, "\n"), "deleted "+heapDump) {
		t.Errorf("result = %+v, want the deletion reported", result)
	}
}
