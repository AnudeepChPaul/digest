package jobs

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
)

func cloneUnder(t *testing.T, origin string) (root, clone string) {
	t.Helper()
	root = t.TempDir()
	clone = filepath.Join(root, "clone")
	runGit(t, root, "clone", "--quiet", origin, clone)
	return root, clone
}

func mergedPRJSON(number int, branch, head string, mergedAt time.Time) string {
	return `[{"number": ` + strconv.Itoa(number) + `, "headRefName": "` + branch + `", "headRefOid": "` + head + `", "mergedAt": "` + mergedAt.UTC().Format(time.RFC3339) + `"}]`
}

func containsAll(text string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}

func TestBranchReaperGraceDaysDecidesWhichMergedBranchesAreOldEnough(t *testing.T) {
	origin, _ := originWithSeed(t)
	reapRoot, clone := cloneUnder(t, origin)
	runGit(t, clone, "branch", "recent")
	head := gitOutput(t, clone, "rev-parse", "HEAD")
	fakeGitHubCLI(t, mergedPRJSON(1, "recent", head, time.Now().Add(-time.Hour)))

	if _, err := RunBranchReaper([]string{reapRoot}, 7, false); err != nil {
		t.Fatal(err)
	}
	if !localBranchExists(t, clone, "recent") {
		t.Fatal("a PR merged an hour ago is inside a 7 day grace period")
	}
	needsAction, err := RunBranchReaper([]string{reapRoot}, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if localBranchExists(t, clone, "recent") {
		t.Error("with grace_days 0 the merged branch should be deleted")
	}
	if needsAction {
		t.Error("a clean delete needs no user action")
	}
}

func TestBranchReaperReportsUnparseableGitHubOutputAsAFailedQuery(t *testing.T) {
	origin, _ := originWithSeed(t)
	reapRoot, _ := cloneUnder(t, origin)
	fakeGitHubCLI(t, "<html>rate limited</html>")

	job := &BranchReaperJob{Roots: []string{reapRoot}, MinAgeDays: 7, Logger: log.New(io.Discard)}
	result, err := job.Run(false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(result.Drafts, "clone: skipped, failed gh query") || !NeedsUserAction(result, false) {
		t.Errorf("result = %+v, want a failed gh query that needs action", result)
	}
}

func TestBranchReaperReportsAFailingGitHubCLI(t *testing.T) {
	origin, _ := originWithSeed(t)
	reapRoot, _ := cloneUnder(t, origin)
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "gh"), []byte("#!/bin/sh\nexit 4\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	needsAction, err := RunBranchReaper([]string{reapRoot}, 7, true)
	if err != nil || !needsAction {
		t.Errorf("needsAction = %v, err = %v; want a failed gh query", needsAction, err)
	}
}

func TestBranchReaperDryRunUsesGitsMergedIntoHeadOrUpstreamRule(t *testing.T) {
	origin, seed := originWithSeed(t)
	runGit(t, seed, "checkout", "--quiet", "-b", "pushed")
	commitFile(t, seed, "pushed.txt", "on the remote\n")
	runGit(t, seed, "push", "--quiet", "origin", "pushed")
	reapRoot, clone := cloneUnder(t, origin)
	runGit(t, clone, "branch", "--quiet", "--track", "pushed", "origin/pushed")
	runGit(t, clone, "branch", "in-head")
	runGit(t, clone, "checkout", "--quiet", "-b", "local-only")
	commitFile(t, clone, "local.txt", "nowhere else\n")
	runGit(t, clone, "checkout", "--quiet", "main")
	pushedHead := gitOutput(t, clone, "rev-parse", "pushed")
	merged := time.Now().AddDate(0, 0, -30).UTC().Format(time.RFC3339)
	fakeGitHubCLI(t, `[
		{"number": 1, "headRefName": "pushed", "headRefOid": "`+pushedHead+`", "mergedAt": "`+merged+`"},
		{"number": 2, "headRefName": "in-head", "headRefOid": "`+pushedHead+`", "mergedAt": "`+merged+`"},
		{"number": 3, "headRefName": "local-only", "headRefOid": "`+pushedHead+`", "mergedAt": "`+merged+`"},
		{"number": 4, "headRefName": "main", "headRefOid": "`+pushedHead+`", "mergedAt": "`+merged+`"}
	]`)

	job := &BranchReaperJob{Roots: []string{reapRoot}, MinAgeDays: 7, Logger: log.New(io.Discard)}
	result, err := job.Run(true)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"clone: would delete pushed", "clone: would delete in-head"} {
		if !slices.Contains(result.ActionsTaken, want) {
			t.Errorf("actions = %v, want %q", result.ActionsTaken, want)
		}
	}
	if !slices.Contains(result.Drafts, "clone: would skip local-only") {
		t.Errorf("drafts = %v, want local-only skipped", result.Drafts)
	}
	for _, branch := range []string{"pushed", "in-head", "local-only"} {
		if !localBranchExists(t, clone, branch) {
			t.Errorf("dry run deleted %s", branch)
		}
	}

	result, err = job.Run(false)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"clone: deleted pushed", "clone: deleted in-head"} {
		if !slices.Contains(result.ActionsTaken, want) {
			t.Errorf("real run actions = %v, want %q", result.ActionsTaken, want)
		}
	}
}

func TestBranchReaperDryRunMatchesAFetchedPRHead(t *testing.T) {
	origin, _ := originWithSeed(t)
	reapRoot, clone := cloneUnder(t, origin)
	runGit(t, clone, "checkout", "--quiet", "-b", "squashed")
	commitFile(t, clone, "squashed.txt", "done\n")
	squashedHead := gitOutput(t, clone, "rev-parse", "HEAD")
	runGit(t, clone, "push", "--quiet", "origin", "squashed:refs/pull/1/head")
	runGit(t, clone, "checkout", "--quiet", "main")
	fakeGitHubCLI(t, mergedPRJSON(1, "squashed", squashedHead, time.Now().AddDate(0, 0, -30)))

	job := &BranchReaperJob{Roots: []string{reapRoot}, MinAgeDays: 7, Logger: log.New(io.Discard)}
	result, err := job.Run(true)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(result.ActionsTaken, "clone: would delete squashed (PR #1)") || !localBranchExists(t, clone, "squashed") {
		t.Errorf("actions = %v, want squashed matched by its PR head", result.ActionsTaken)
	}
}

func TestBranchReaperReportsPRHeadsItCannotFetchOrCompare(t *testing.T) {
	origin, _ := originWithSeed(t)
	reapRoot, clone := cloneUnder(t, origin)
	runGit(t, clone, "checkout", "--quiet", "-b", "orphaned")
	commitFile(t, clone, "orphaned.txt", "work\n")
	runGit(t, clone, "checkout", "--quiet", "main")
	merged := time.Now().AddDate(0, 0, -30).UTC().Format(time.RFC3339)
	runGit(t, clone, "push", "--quiet", "origin", "main:refs/pull/2/head")
	fakeGitHubCLI(t, `[
		{"number": 1, "headRefName": "orphaned", "headRefOid": "0000000000000000000000000000000000000000", "mergedAt": "`+merged+`"},
		{"number": 2, "headRefName": "orphaned", "headRefOid": "1111111111111111111111111111111111111111", "mergedAt": "`+merged+`"}
	]`)

	matched, report := matchMergedPR(clone, "orphaned", []MergedPR{
		{Number: 1, HeadRefName: "orphaned", HeadRefOid: "0000000000000000000000000000000000000000"},
		{Number: 2, HeadRefName: "orphaned", HeadRefOid: "1111111111111111111111111111111111111111"},
	}, false)
	if matched != nil {
		t.Fatalf("matched %+v, want none", matched)
	}
	if !containsAll(report, "#1: ", "#2: ") {
		t.Errorf("report = %q, want both PRs explained", report)
	}

	job := &BranchReaperJob{Roots: []string{reapRoot}, MinAgeDays: 7, Logger: log.New(io.Discard)}
	result, err := job.Run(false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(result.Drafts, "clone: orphaned - git refused delete") || !localBranchExists(t, clone, "orphaned") {
		t.Errorf("drafts = %v, want orphaned kept and flagged", result.Drafts)
	}
}

func TestBranchReaperReportsAForceDeleteGitRefuses(t *testing.T) {
	origin, _ := originWithSeed(t)
	reapRoot, clone := cloneUnder(t, origin)
	runGit(t, clone, "checkout", "--quiet", "-b", "squashed")
	commitFile(t, clone, "squashed.txt", "done\n")
	squashedHead := gitOutput(t, clone, "rev-parse", "HEAD")
	runGit(t, clone, "push", "--quiet", "origin", "squashed:refs/pull/1/head")
	runGit(t, clone, "checkout", "--quiet", "main")
	runGit(t, clone, "worktree", "add", "--quiet", filepath.Join(t.TempDir(), "elsewhere"), "squashed")
	fakeGitHubCLI(t, mergedPRJSON(1, "squashed", squashedHead, time.Now().AddDate(0, 0, -30)))

	job := &BranchReaperJob{Roots: []string{reapRoot}, MinAgeDays: 7, Logger: log.New(io.Discard)}
	result, err := job.Run(false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(result.Drafts, "clone: squashed - git refused delete") || !localBranchExists(t, clone, "squashed") {
		t.Errorf("drafts = %v, want a refused force delete", result.Drafts)
	}
}

func TestBranchReaperIgnoresUntrackedFilesButSkipsTrackedChanges(t *testing.T) {
	origin, _ := originWithSeed(t)
	reapRoot, clone := cloneUnder(t, origin)
	runGit(t, clone, "branch", "done")
	head := gitOutput(t, clone, "rev-parse", "HEAD")
	fakeGitHubCLI(t, mergedPRJSON(1, "done", head, time.Now().AddDate(0, 0, -30)))
	if err := os.WriteFile(filepath.Join(clone, "scratch.txt"), []byte("untracked\n"), 0644); err != nil {
		t.Fatal(err)
	}

	job := &BranchReaperJob{Roots: []string{reapRoot}, MinAgeDays: 7, Logger: log.New(io.Discard)}
	result, err := job.Run(false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(result.ActionsTaken, "clone: deleted done") {
		t.Errorf("untracked files should not block the reaper: %+v", result)
	}

	runGit(t, clone, "branch", "done")
	if err := os.WriteFile(filepath.Join(clone, "README"), []byte("edited\n"), 0644); err != nil {
		t.Fatal(err)
	}
	result, err = job.Run(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ActionsTaken) != 0 || len(result.Drafts) != 0 || !localBranchExists(t, clone, "done") {
		t.Errorf("tracked changes keep the repo log-only: %+v", result)
	}
}

func TestBranchReaperMissingRootIsAnError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	if _, err := RunBranchReaper([]string{missing}, 7, false); err == nil {
		t.Error("a missing --root should be an error")
	}
}

func TestBranchReaperWithoutALoggerLogsToStderr(t *testing.T) {
	job := &BranchReaperJob{Roots: []string{t.TempDir()}}
	result, err := job.Run(true)
	if err != nil || result.Summary != "0 repositories · 0 branches processed" {
		t.Errorf("result = %+v, err = %v", result, err)
	}
}
