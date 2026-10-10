package tui

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/review"
	tea "github.com/charmbracelet/bubbletea"
)

func TestJobTicksProduceTheirMessages(t *testing.T) {
	if _, ok := tickRunStatePollCmd()().(runStatePollTickMsg); !ok {
		t.Fatal("expected a run state poll tick")
	}
	if _, ok := tickJobLogCmd()().(jobLogTickMsg); !ok {
		t.Fatal("expected a job log tick")
	}
}

func TestRefreshDryRunResultsWithoutConfigOrMaps(t *testing.T) {
	tempDigestRoot(t)
	m, _, _ := jobTestModel(t)
	m.jobDryRunOutputs, m.jobDryRunExitCodes, m.jobDryRunHasRun = nil, nil, nil
	writeLogsFile(t, "nightly.dryrun.exit", "0")
	m.refreshDryRunResults()
	if !m.jobDryRunHasRun["nightly"] {
		t.Fatal("the finished dry run should be recorded")
	}
	writeLogsFile(t, "nightly.dryrun.pid", strconv.Itoa(os.Getpid()))
	delete(m.dryRunLogStamps, "nightly")
	m.refreshDryRunResults()
	if !m.dryRunsInFlight["nightly"] {
		t.Fatal("a live dry run pid should be in flight")
	}
	if view := stripANSI(m.renderDashboardBody()); !strings.Contains(view, "nightly") {
		t.Fatalf("dashboard:\n%s", view)
	}
	m.cfg = nil
	m.refreshDryRunResults()
}

func TestRunningJobRowShowsTheRunningIndicator(t *testing.T) {
	tempDigestRoot(t)
	m, _, _ := jobTestModel(t)
	m.runningJobPIDs = map[string]int{"nightly": os.Getpid()}
	m.contentVersion++
	if row := stripANSI(m.renderDraftRow(m.allNavItems()[m.selected].Draft, false, 100)); !strings.Contains(row, "#job") {
		t.Fatalf("row = %q", row)
	}
}

func TestJobLogTickStopsWithoutAJobPreview(t *testing.T) {
	m := syncTestModel(t)
	m.jobLogRunning = true
	next, cmd := m.handleJobLogTick()
	if cmd != nil || next.(Model).jobLogRunning {
		t.Fatal("no previewed job should stop the log ticks")
	}
}

func TestJobLogTickFollowsTheTailWhenAtTheBottom(t *testing.T) {
	tempDigestRoot(t)
	m, _, _ := jobTestModel(t)
	writeLogsFile(t, "nightly.pid", strconv.Itoa(os.Getpid()))
	writeLogsFile(t, "nightly.log", strings.Repeat("line\n", 200))
	m.mode = ViewPreview
	m.updatePreviewViewport()
	m.previewViewport.GotoBottom()
	m.jobLogStamp = "stale"
	next, cmd := m.handleJobLogTick()
	if cmd == nil || !next.(Model).previewViewport.AtBottom() {
		t.Fatal("a running job at the bottom should keep following the log")
	}
}

func TestJobKeysOnNonJobRowsDoNothing(t *testing.T) {
	m := noteSaveModel(t, dashboardNotes(time.Now())...)
	selectNavKind(t, &m, KindCarriedNote)
	for name, action := range map[string]func(tea.KeyMsg) (tea.Model, tea.Cmd){
		"run":  m.runSelectedJob,
		"dry":  m.dryRunSelectedJob,
		"stop": m.stopSelectedItem,
	} {
		if next, cmd := action(tea.KeyMsg{}); cmd != nil || next.(Model).mode != m.mode {
			t.Fatalf("%s should do nothing on a note row", name)
		}
	}
}

func TestRunStatePollRefreshesAChangedJobPreview(t *testing.T) {
	tempDigestRoot(t)
	m, _, _ := jobTestModel(t)
	m.mode = ViewPreview
	writeLogsFile(t, "nightly.dryrun.pid", strconv.Itoa(os.Getpid()))
	_, cmd := m.Update(runStatePollTickMsg{})
	if cmd == nil {
		t.Fatal("a dry run in flight should keep polling")
	}
}

func TestDismissingAReviewRunReportsErrorsAndLeavesThePreview(t *testing.T) {
	m := reviewTestModel(t)
	ref := review.PRRef{Host: "github.com", Owner: "o", Repo: "api", Number: 9, URL: "https://github.com/o/api/pull/9"}
	dir := review.StateDir(m.reviewRoot(), ref)
	if err := review.WriteMeta(dir, review.Meta{Ref: ref}); err != nil {
		t.Fatal(err)
	}
	writeReviewStateFile(t, dir, "review.exit", "1")
	m.refreshReviewRuns()
	m.mode = ViewPreview
	selectNavKind(t, &m, KindReviewRun)
	next, _ := m.dismissReviewRun(ref)
	if got := next.(Model); got.mode != ViewPreview || len(got.reviewRuns) != 0 {
		t.Fatalf("mode %v runs %d", got.mode, len(got.reviewRuns))
	}
	m.git.ghPendingPRs = nil
	m.rebuildGitRepoStats()
	writeReviewStateFile(t, dir, "review.exit", "1")
	if err := review.WriteMeta(dir, review.Meta{Ref: ref}); err != nil {
		t.Fatal(err)
	}
	m.refreshReviewRuns()
	selectNavKind(t, &m, KindReviewRun)
	if next, _ = m.dismissReviewRun(ref); next.(Model).mode != ViewDashboard {
		t.Fatalf("with no rows left the preview should close, mode %v", next.(Model).mode)
	}
	writeLivePID(t, dir+"/review.pid")
	m.dismissReviewRun(ref)
}

func TestStopWithoutASelectedRowDoesNothing(t *testing.T) {
	m := syncTestModel(t)
	m.selected = 999
	if next, cmd := m.stopSelectedItem(tea.KeyMsg{}); cmd != nil || next.(Model).mode != m.mode {
		t.Fatal("no row means nothing to stop")
	}
}
