package tui

import (
	"strings"
	"testing"

	"github.com/achandrapaul/digest/pkg/brag"
	"github.com/achandrapaul/digest/pkg/model"
	"github.com/achandrapaul/digest/pkg/sourcecontrol"
)

func TestDeleteConfirmTextsForJobsAndNoteCounts(t *testing.T) {
	m := syncTestModel(t)
	m.mode = ViewDeleteConfirm
	m.jobToExecute = "nightly"
	if view := stripANSI(m.View()); !strings.Contains(view, "EXECUTE JOB") || !strings.Contains(view, "Are you sure you want to run 'nightly'?") {
		t.Fatalf("execute confirm:\n%s", view)
	}
	m.jobToExecute = ""
	m.deleteTargetNotes = []*model.Note{{Summary: "a"}, {Summary: "b"}}
	if view := stripANSI(m.View()); !strings.Contains(view, "permanently delete these 2 selected notes?") {
		t.Fatalf("multi delete confirm:\n%s", view)
	}
	m.deleteTargetNotes = nil
	if view := stripANSI(m.View()); !strings.Contains(view, "No notes selected for deletion.") {
		t.Fatalf("empty delete confirm:\n%s", view)
	}
}

func previewHeader(t *testing.T, m Model) string {
	t.Helper()
	m.mode = ViewPreview
	return stripANSI(m.View())
}

func TestPreviewHeaderForARepoRow(t *testing.T) {
	m := gitStripTestModel(t)
	selectNavKind(t, &m, KindGitRepo)
	m.updatePreviewViewport()
	view := previewHeader(t, m)
	if !strings.Contains(view, "GIT REPO:") || !strings.Contains(view, "SYNCED") || !strings.Contains(view, "#git") {
		t.Fatalf("preview:\n%s", view)
	}
	if !strings.Contains(view, "Repository Activity:") || !strings.Contains(view, "Commits:") {
		t.Fatalf("repo preview body:\n%s", view)
	}
}

func TestPreviewHeaderForAReReviewAndItsNotice(t *testing.T) {
	m := reReviewModel(t, nil)
	m.git.ghPendingPRs[0].Kind = sourcecontrol.ReReviewKind
	m.reviewNotice = "Review started in the background"
	view := previewHeader(t, m)
	if !strings.Contains(view, "RE-REVIEW: ") || !strings.Contains(view, "Review started in the background") {
		t.Fatalf("preview:\n%s", view)
	}
}

func TestPreviewHeaderForBragJobs(t *testing.T) {
	m, _ := bragTestModel(t, wednesday())
	m.bragRuns = []brag.Run{{Meta: brag.RunMeta{ID: "2026-W40"}, Status: brag.RunFailed}}
	selectNavKind(t, &m, KindBragRun)
	if view := previewHeader(t, m); !strings.Contains(view, "BRAG JOB: 2026-W40") || !strings.Contains(view, "FAILED") || !strings.Contains(view, "#brag") {
		t.Fatalf("preview:\n%s", view)
	}
	m.bragRuns[0].Status = brag.RunRunning
	if view := previewHeader(t, m); !strings.Contains(view, "RUNNING") {
		t.Fatalf("running preview:\n%s", view)
	}
}

func TestPreviewHeaderForJobDryRunStates(t *testing.T) {
	tempDigestRoot(t)
	m, _, _ := jobTestModel(t)
	m.dryRunsInFlight = map[string]bool{"nightly": true}
	if view := previewHeader(t, m); !strings.Contains(view, "DRY RUNNING") {
		t.Fatalf("in flight:\n%s", view)
	}
	m.dryRunsInFlight = nil
	m.previewJobLogFinished = true
	if view := previewHeader(t, m); !strings.Contains(view, "FINISHED") {
		t.Fatalf("finished:\n%s", view)
	}
	m.previewJobLogFinished = false
	m.jobDryRunHasRun = map[string]bool{"nightly": true}
	m.jobDryRunExitCodes = map[string]int{"nightly": 0}
	if view := previewHeader(t, m); !strings.Contains(view, "SUCCESS") {
		t.Fatalf("success:\n%s", view)
	}
	m.jobDryRunExitCodes["nightly"] = 2
	if view := previewHeader(t, m); !strings.Contains(view, "NEED ACT") {
		t.Fatalf("needs action:\n%s", view)
	}
}

func TestJobPreviewLabelsANewerDryRunAsAnalysis(t *testing.T) {
	tempDigestRoot(t)
	m, _, _ := jobTestModel(t)
	writeLogsFile(t, "nightly.log", "real output\n")
	writeLogsFile(t, "nightly.dryrun.log", "dry output\n")
	writeLogsFile(t, "nightly.dryrun.exit", "0")
	m.refreshDryRunResults()
	m.mode = ViewPreview
	m.updatePreviewViewport()
	if content := stripANSI(m.previewViewport.View()); !strings.Contains(content, "DRY RUN ANALYSIS") {
		t.Fatalf("preview:\n%s", content)
	}
}

func TestOverlayAtSkipsRowsOutsideAndPadsShortLines(t *testing.T) {
	got := overlayAt("ab\ncd", "XY\nZW", -1, 4)
	lines := strings.Split(stripANSI(got), "\n")
	if lines[0] != "abZW" && lines[0] != "ab  ZW" {
		t.Fatalf("lines = %q", lines)
	}
	if lines[1] != "cd" {
		t.Fatalf("the second row should be untouched, got %q", lines[1])
	}
}

func TestErrorPopupDefaultsItsTitle(t *testing.T) {
	m := syncTestModel(t)
	m.mode, m.errorTitle, m.errorLines = ViewError, "", []string{"boom"}
	if view := stripANSI(m.View()); !strings.Contains(view, " ERROR ") || !strings.Contains(view, "boom") {
		t.Fatalf("view:\n%s", view)
	}
}
