package tui

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/review"
	"github.com/achandrapaul/digest/pkg/sourcecontrol"
	"github.com/charmbracelet/bubbles/viewport"
)

func reviewStateDir(m Model) string {
	return review.StateDir(m.reviewRoot(), m.currentPRItem().PR.Ref)
}

func writeReviewStateFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func brokenURLItem() *GitPRItem {
	return &GitPRItem{Title: "Broken", URL: "not a pull request url"}
}

func reviewTabText(t *testing.T, m Model) string {
	t.Helper()
	content, _ := m.renderReviewContent(m.currentPRItem(), 100)
	return stripANSI(content)
}

func withLocalReview(m Model, state localReviewState) Model {
	m.localReviews = map[string]localReviewState{reviewStateDir(m): state}
	m.reviewReports = &reviewReportMemo{}
	return m
}

func TestReviewTabWithoutAReviewOffersToStartOne(t *testing.T) {
	m := withLocalReview(reviewTestModel(t), localReviewState{status: review.RunIdle})
	if text := reviewTabText(t, m); !strings.Contains(text, "No Claude review yet. Press r to clone the PR") {
		t.Fatalf("review tab:\n%s", text)
	}
}

func TestReviewTabWhileRunningShowsTheLogTail(t *testing.T) {
	m := reviewTestModel(t)
	var log strings.Builder
	for line := 1; line <= 20; line++ {
		log.WriteString("log line " + string(rune('a'+line)) + "\n")
	}
	writeReviewStateFile(t, reviewStateDir(m), review.LogFile, log.String())
	m = withLocalReview(m, localReviewState{status: review.RunRunning})
	text := reviewTabText(t, m)
	if !strings.Contains(text, "Claude is reviewing this PR in the background…") || !strings.Contains(text, "log line u") {
		t.Fatalf("review tab:\n%s", text)
	}
	if strings.Contains(text, "log line h") {
		t.Fatalf("only the last 12 log lines should show:\n%s", text)
	}
}

func TestReviewTabAfterAFailureOffersARetryWithTheLog(t *testing.T) {
	m := reviewTestModel(t)
	writeReviewStateFile(t, reviewStateDir(m), review.LogFile, "clone exploded\n")
	m = withLocalReview(m, localReviewState{status: review.RunFailed})
	text := reviewTabText(t, m)
	if !strings.Contains(text, "Review failed. Press r to retry.") || !strings.Contains(text, "clone exploded") {
		t.Fatalf("review tab:\n%s", text)
	}
}

func TestReviewTabShowsAnUnreadableReportError(t *testing.T) {
	m := reviewTestModel(t)
	writeReviewStateFile(t, reviewStateDir(m), review.FindingsFile, "{not json")
	m = withLocalReview(m, localReviewState{status: review.RunDone, finished: true, finishedAt: time.Now()})
	text := reviewTabText(t, m)
	if strings.Contains(text, "Recommendation:") || !strings.Contains(text, "State:") {
		t.Fatalf("an unreadable report should only show its error:\n%s", text)
	}
	if report, findings := m.loadFindings(m.currentPRItem()); report != nil || findings != nil {
		t.Fatal("an unreadable report should have no findings")
	}
}

func TestReviewTabWarnsWhenThePRMovedOnSinceTheReview(t *testing.T) {
	m := reviewTestModel(t)
	if err := review.WriteMeta(reviewStateDir(m), review.Meta{Ref: m.currentPRItem().PR.Ref, HeadSHA: "older"}); err != nil {
		t.Fatal(err)
	}
	m = withLocalReview(m, localReviewState{status: review.RunDone, finished: true, finishedAt: time.Now()})
	if text := reviewTabText(t, m); !strings.Contains(text, "PR has new commits since this review — press r to re-run.") {
		t.Fatalf("review tab:\n%s", text)
	}
}

func TestReviewTabWithNoFindingsSuggestsApproving(t *testing.T) {
	m := reviewTestModel(t)
	writeReviewStateFile(t, reviewStateDir(m), review.FindingsFile, `{"recommendation":"APPROVE","findings":[]}`)
	m = withLocalReview(m, localReviewState{status: review.RunDone, finished: true, finishedAt: time.Now()})
	text := reviewTabText(t, m)
	if !strings.Contains(text, "APPROVE · 0 findings · 0 selected") || !strings.Contains(text, "No actionable findings. Press a to approve.") {
		t.Fatalf("review tab:\n%s", text)
	}
}

func TestReviewTabShowsSuggestionsAndMediumFindings(t *testing.T) {
	m := reviewTestModel(t)
	writeReviewStateFile(t, reviewStateDir(m), review.FindingsFile, `{"recommendation":"COMMENT","findings":[
{"severity":"medium","title":"M1","body":"","suggestion":"use a map"}]}`)
	m = withLocalReview(m, localReviewState{status: review.RunDone, finished: true, finishedAt: time.Now()})
	text := reviewTabText(t, m)
	if !strings.Contains(text, "MEDIUM (1)") || !strings.Contains(text, "Suggestion: use a map") {
		t.Fatalf("review tab:\n%s", text)
	}
}

func TestReviewTabForAnUnparseablePRURLShowsTheError(t *testing.T) {
	m := reviewTestModel(t)
	content, cursorLine := m.renderReviewContent(brokenURLItem(), 80)
	if cursorLine != -1 || content == "" {
		t.Fatalf("content %q cursor %d", content, cursorLine)
	}
}

func TestDetailsTabShowsLastReviewMoreFilesAndLoading(t *testing.T) {
	m := detailedPRModel(t)
	pr := m.git.ghPendingPRs[0].PR
	pr.MyLastReviewAt = time.Now().Add(-30 * time.Minute)
	pr.Files = []string{"a.go", "b.go"}
	pr.ChangedFiles = 5
	m.historyRequested = map[string]bool{pr.Ref.URL: true}
	details := m.renderDetailsMarkdown(m.currentPRItem())
	for _, want := range []string{"**Your last review:** 30m ago", "- … and 3 more", "Loading…"} {
		if !strings.Contains(details, want) {
			t.Errorf("details missing %q:\n%s", want, details)
		}
	}
}

func TestDetailsTabListsHistoryAndHistoryFailures(t *testing.T) {
	m := detailedPRModel(t)
	url := m.currentPRItem().URL
	m.contextCache = map[string]string{url: "abc123 first\ndef456 second"}
	details := m.renderDetailsMarkdown(m.currentPRItem())
	if !strings.Contains(details, "- abc123 first\n- def456 second") {
		t.Fatalf("details:\n%s", details)
	}
	m.contextCache[url] = historyFailurePrefix + "no origin"
	if details := m.renderDetailsMarkdown(m.currentPRItem()); !strings.Contains(details, "Couldn't read history: no origin") {
		t.Fatalf("details:\n%s", details)
	}
}

func TestDetailsForAPRWithoutDetailsPointsToTheBrowser(t *testing.T) {
	m := reviewTestModel(t)
	details := m.renderDetailsMarkdown(&GitPRItem{Title: "Plain", Repository: "o/r", URL: "https://github.com/o/r/pull/1"})
	if !strings.Contains(details, "Press **[Enter]** on the dashboard to open this Pull Request in your browser.") {
		t.Fatalf("details:\n%s", details)
	}
}

func TestPRStateFallsBackForBrokenURLsAndGitHubReviews(t *testing.T) {
	m := reviewTestModel(t)
	if state := m.prState(brokenURLItem()); state != review.StatePending {
		t.Fatalf("broken URL state = %v", state)
	}
	pr := m.currentPRItem().PR
	pr.MyLastReviewState = "COMMENTED"
	pr.MyLastReviewAt = time.Now().Add(-time.Hour)
	m = withLocalReview(m, localReviewState{status: review.RunIdle, finished: true, finishedAt: time.Now()})
	if state := m.prState(m.currentPRItem()); state != review.StateCommented {
		t.Fatalf("state = %v, want commented", state)
	}
	m = withLocalReview(m, localReviewState{status: review.RunRunning})
	if state := m.prState(m.currentPRItem()); state != review.StateReviewing {
		t.Fatalf("state = %v, want reviewing", state)
	}
}

func TestStateStylesAndShortAges(t *testing.T) {
	for _, state := range []review.PRState{review.StateApproved, review.StateReviewing, review.StateCommented, review.StatePending} {
		if stateStyle(state).Render("x") == "" {
			t.Fatalf("no style for %v", state)
		}
	}
	for duration, want := range map[time.Duration]string{5 * time.Minute: "5m", 3 * time.Hour: "3h", 50 * time.Hour: "2d"} {
		if got := shortAge(duration); got != want {
			t.Fatalf("shortAge(%v) = %q, want %q", duration, got, want)
		}
	}
	if severityStyle("low").Render("x") == "" {
		t.Fatal("low severity should use the default style")
	}
}

func TestEventLabelsNameEachReviewAction(t *testing.T) {
	for event, want := range map[review.Event]string{
		review.EventApprove:        "Approve",
		review.EventRequestChanges: "Request changes on",
		review.EventComment:        "Post review comments on",
	} {
		if got := eventLabel(event); got != want {
			t.Fatalf("eventLabel(%v) = %q, want %q", event, got, want)
		}
	}
}

func TestBrokenURLHelpersReturnEmptyResults(t *testing.T) {
	m := reviewTestModel(t)
	item := brokenURLItem()
	if report, findings := m.loadFindings(item); report != nil || findings != nil {
		t.Fatal("loadFindings should be empty")
	}
	if _, _, err := m.buildPayload(item, review.EventApprove, ""); err == nil {
		t.Fatal("buildPayload should fail")
	}
	if ref := m.refFor(item); ref.URL != "" {
		t.Fatalf("refFor = %+v", ref)
	}
	if _, ok := m.reviewPIDFor(item); ok {
		t.Fatal("reviewPIDFor should report no pid")
	}
	if next, cmd := m.beginReviewRunConfirm(reviewActionStart, review.PRRef{}); next.(Model).mode != m.mode || cmd != nil {
		t.Fatal("an empty ref should not open the confirm")
	}
	_, next, _ := m.beginConfirm(item, review.EventApprove, "")
	if got := next.(Model); got.mode != ViewPreview || got.reviewNotice == "" {
		t.Fatalf("beginConfirm mode %v notice %q", got.mode, got.reviewNotice)
	}
	_, next, _ = m.startReview(item)
	if next.(Model).reviewNotice == "" {
		t.Fatal("startReview should report the URL error")
	}
	_, next, _ = m.openClone(item)
	if next.(Model).reviewNotice == "" {
		t.Fatal("openClone should report the URL error")
	}
}

func TestStartReviewReportsABackgroundStartFailure(t *testing.T) {
	previous := startBackground
	startBackground = func(review.QueuedPR, string) error { return errors.New("cannot spawn") }
	t.Cleanup(func() { startBackground = previous })
	m := reviewTestModel(t)
	_, next, cmd := m.startReview(m.currentPRItem())
	if next.(Model).reviewNotice != "cannot spawn" || cmd != nil {
		t.Fatalf("notice %q", next.(Model).reviewNotice)
	}
}

func TestOpenCloneWithoutASessionContextStillClones(t *testing.T) {
	stubOpenInNvim(t)
	t.Setenv("TMUX", "/tmp/tmux-test,1,0")
	m, _ := clonedPRModel(t)
	m.sessionCtx = nil
	_, next, cmd := m.openClone(m.currentPRItem())
	if next.(Model).reviewNotice != "Opening PR clone in nvim…" || cmd == nil {
		t.Fatalf("notice %q", next.(Model).reviewNotice)
	}
}

func TestCloneReadyNoticesForEachOutcome(t *testing.T) {
	m := reviewTestModel(t)
	cases := map[string]reviewCloneReadyMsg{
		"The running review is still cloning; try again shortly": {err: sourcecontrol.ErrCloneInProgress},
		"Clone failed: disk full":                                {err: errors.New("disk full")},
		"no tmux":                                                {openErr: errors.New("no tmux")},
		"Opened clone in a new tmux window":                      {},
	}
	for want, msg := range cases {
		next, _ := m.handleCloneReady(msg)
		if got := next.(Model).reviewNotice; got != want {
			t.Fatalf("notice = %q, want %q", got, want)
		}
	}
	m.mode = ViewDashboard
	if _, cmd := m.handleCloneReady(reviewCloneReadyMsg{}); cmd != nil {
		t.Fatal("outside the preview no history should load")
	}
}

var realOpenInNvim = openInNvim

func fakeTmuxOnPath(t *testing.T, script string) {
	t.Helper()
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "tmux"), []byte("#!/bin/sh\n"+script+"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestOpenInNvimRunsTmuxNewWindow(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	fakeTmuxOnPath(t, `echo "$@" > `+argsFile)
	if err := realOpenInNvim("/clone/dir", review.PRRef{Repo: "console", Number: 7}); err != nil {
		t.Fatal(err)
	}
	args, _ := os.ReadFile(argsFile)
	if strings.TrimSpace(string(args)) != "new-window -c /clone/dir -n console#7 nvim ." {
		t.Fatalf("tmux args = %q", args)
	}
}

func TestOpenInNvimReportsTmuxOutputOnFailure(t *testing.T) {
	fakeTmuxOnPath(t, `echo "no server running"; exit 1`)
	err := realOpenInNvim("/clone/dir", review.PRRef{Repo: "console", Number: 7})
	if err == nil || err.Error() != "tmux new-window: no server running" {
		t.Fatalf("err = %v", err)
	}
}

func TestReviewSubmittedNoticesAndFollowUps(t *testing.T) {
	m := reviewTestModel(t)
	next, cmd := m.handleReviewSubmitted(reviewSubmittedMsg{err: errors.New("403")})
	if got := next.(Model); got.reviewNotice != "" || cmd != nil {
		t.Fatalf("an error should clear the notice, got %q", got.reviewNotice)
	}
	pr := *m.currentPRItem().PR
	for event, want := range map[review.Event]string{
		review.EventRequestChanges: "Changes requested ✓",
		review.EventComment:        "Comments posted ✓",
	} {
		next, cmd := m.handleReviewSubmitted(reviewSubmittedMsg{event: event, pr: pr})
		if got := next.(Model); got.reviewNotice != want || cmd == nil {
			t.Fatalf("notice = %q, want %q", got.reviewNotice, want)
		}
	}
	m.mode = ViewDashboard
	next, _ = m.handleReviewSubmitted(reviewSubmittedMsg{event: review.EventApprove})
	if next.(Model).reviewNotice != "Approved ✓" {
		t.Fatalf("notice = %q", next.(Model).reviewNotice)
	}
}

func TestReviewedHeadSHAUsesTheReviewedCommitWhenFindingsArePicked(t *testing.T) {
	m := reviewTestModel(t)
	queued := *m.currentPRItem().PR
	if err := review.WriteMeta(reviewStateDir(m), review.Meta{Ref: queued.Ref, HeadSHA: "reviewed-sha"}); err != nil {
		t.Fatal(err)
	}
	if got := m.reviewedHeadSHA(queued); got != "sha" {
		t.Fatalf("without picks = %q", got)
	}
	m.reviewSelected = map[int]bool{0: true}
	if got := m.reviewedHeadSHA(queued); got != "reviewed-sha" {
		t.Fatalf("with picks = %q", got)
	}
}

func TestReviewConfirmShowsTheComment(t *testing.T) {
	m := reviewTestModel(t)
	m.reviewEvent = review.EventComment
	m.reviewBody = "please rename"
	if text := stripANSI(m.renderReviewConfirm(80)); !strings.Contains(text, "Comment:") || !strings.Contains(text, "please rename") {
		t.Fatalf("confirm:\n%s", text)
	}
}

func TestRejectCommentPopupShowsTheHintAndNotice(t *testing.T) {
	m := rejectCommentModel(t)
	m.reviewNotice = "comment required"
	text := stripANSI(m.View())
	for _, want := range []string{"REQUEST CHANGES", "No comments selected. Write a comment for the author.", "comment required"} {
		if !strings.Contains(text, want) {
			t.Fatalf("popup missing %q:\n%s", want, text)
		}
	}
}

func TestReviewPollHelpers(t *testing.T) {
	m := reviewTestModel(t)
	m.reviewPolling = true
	if m.ensureReviewPoll() != nil {
		t.Fatal("an active poll should not start another")
	}
	m.selected = 99
	if m.currentPRItem() != nil {
		t.Fatal("an out of range selection has no PR")
	}
}

func TestPollSnapshotIncludesFailedReviewRuns(t *testing.T) {
	m := reviewTestModel(t)
	ref := review.PRRef{Host: "github.com", Owner: "o", Repo: "api", Number: 9, URL: "https://github.com/o/api/pull/9"}
	dir := review.StateDir(m.reviewRoot(), ref)
	if err := review.WriteMeta(dir, review.Meta{Ref: ref}); err != nil {
		t.Fatal(err)
	}
	writeReviewStateFile(t, dir, "review.exit", "1")
	snapshot := m.loadReviewPollSnapshot()
	if len(snapshot.reviewRuns) != 1 {
		t.Fatalf("runs = %+v", snapshot.reviewRuns)
	}
	if _, tracked := snapshot.localReviews[dir]; !tracked {
		t.Fatal("the failed run should be tracked as a local review")
	}
}

func TestLoadRecommendationIsEmptyWithoutAReport(t *testing.T) {
	if got := loadRecommendation(t.TempDir()); got != "" {
		t.Fatalf("recommendation = %q", got)
	}
}

func TestDoneReviewBuildsAMemoWhenNoneIsShared(t *testing.T) {
	m := reviewTestModel(t)
	m.refreshLocalReviews()
	m.reviewReports = nil
	if memo, done := m.doneReview(reviewStateDir(m)); !done || memo.report == nil {
		t.Fatalf("memo %+v done %v", memo, done)
	}
}

func TestPreviewContentKeepsAMinimumHeightAndScrollsUpToTheCursor(t *testing.T) {
	m := reviewTestModel(t)
	m.previewTab = previewTabReview
	m.reviewNotice = "notice"
	m.setPRPreviewContent(m.currentPRItem(), 80, 4)
	if m.previewViewport.Height != 3 {
		t.Fatalf("height = %d", m.previewViewport.Height)
	}
	m.reviewNotice = ""
	m.previewViewport = viewport.New(80, 3)
	m.previewViewport.SetContent(strings.Repeat("line\n", 40))
	m.previewViewport.SetYOffset(20)
	m.reviewCursor = 0
	m.setPRPreviewContent(m.currentPRItem(), 80, 5)
	if m.previewViewport.YOffset >= 20 {
		t.Fatalf("the view should scroll up to the cursor, offset = %d", m.previewViewport.YOffset)
	}
}

func gitRepoWithOriginHead(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"-c", "user.email=a@b", "-c", "user.name=a", "commit", "-q", "--allow-empty", "-m", "touch a.go"},
		{"update-ref", "refs/remotes/origin/HEAD", "HEAD"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return dir
}

func TestGitLogForFilesReadsHistoryAndErrors(t *testing.T) {
	dir := gitRepoWithOriginHead(t)
	history, err := gitLogForFiles(dir, nil)
	if err != nil || !strings.HasSuffix(history, " touch a.go") {
		t.Fatalf("history %q err %v", history, err)
	}
	if _, err := gitLogForFiles(t.TempDir(), nil); err == nil || strings.Contains(err.Error(), "exit status") {
		t.Fatalf("a non-repo should report git's stderr, got %v", err)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := gitLogForFiles(dir, nil); err == nil {
		t.Fatal("a missing git binary should fail")
	}
}

func TestRelatedHistoryCmdSkipsAndCapsFiles(t *testing.T) {
	m, _ := clonedPRModel(t)
	if m.relatedHistoryCmd(nil) != nil {
		t.Fatal("no item should have no command")
	}
	item := m.currentPRItem()
	files := make([]string, 25)
	for index := range files {
		files[index] = "file.go"
	}
	item.PR.Files = files
	var requestedFiles []string
	previous := gitLogForFiles
	gitLogForFiles = func(_ string, files []string) (string, error) {
		requestedFiles = files
		return "", nil
	}
	t.Cleanup(func() { gitLogForFiles = previous })
	m.historyRequested = nil
	cmd := m.relatedHistoryCmd(item)
	if cmd == nil {
		t.Fatal("expected a history command")
	}
	cmd()
	if len(requestedFiles) != 20 {
		t.Fatalf("files = %d, want 20", len(requestedFiles))
	}
	m.contextCache = map[string]string{item.PR.Ref.URL: ""}
	if m.relatedHistoryCmd(item) != nil {
		t.Fatal("cached history should not reload")
	}
}

func TestRelatedHistoryCreatesTheCacheAndRecordsFailures(t *testing.T) {
	m := reviewTestModel(t)
	m.contextCache = nil
	url := m.currentPRItem().URL
	next, _ := m.handleRelatedHistory(relatedHistoryMsg{url: url, err: errors.New("boom")})
	if got := next.(Model).contextCache[url]; got != historyFailurePrefix+"boom" {
		t.Fatalf("cache = %q", got)
	}
}

func TestAfterGitSectionRefreshesThePreviewAndPollsRunningReviews(t *testing.T) {
	m := withLocalReview(reviewTestModel(t), localReviewState{status: review.RunRunning})
	next, cmd := m.afterGitSection(nil)
	if cmd == nil || !next.(Model).reviewPolling {
		t.Fatal("a running review should start the poll")
	}
}

func TestPreviewStampCoversRunRows(t *testing.T) {
	m := reviewTestModel(t)
	m.mode = ViewDashboard
	if m.previewStamp() != (previewStamp{}) {
		t.Fatal("outside the preview the stamp is empty")
	}
	if _, found := m.previewedLocalReview(); found {
		t.Fatal("outside the preview nothing is previewed")
	}
	m.mode = ViewPreview
	m.git.ghPendingPRs = []GitPRItem{*brokenURLItem()}
	m.git.ghPendingPRs[0].PR = nil
	m.rebuildGitRepoStats()
	if _, found := m.previewedLocalReview(); found {
		t.Fatal("a broken URL is never previewed")
	}
}

func TestInsertAbovePRURLPlacements(t *testing.T) {
	if got := insertAbovePRURL("\n\nbody", "https://x/pull/1", "BLOCK"); got != "BLOCK\n\nbody" {
		t.Fatalf("no url = %q", got)
	}
	if got := insertAbovePRURL("intro https://x/pull/1", "https://x/pull/1", "BLOCK"); got != "intro \n\nBLOCK\n\nhttps://x/pull/1" {
		t.Fatalf("inline url = %q", got)
	}
}

func TestPollInBragViewReloadsTheBrag(t *testing.T) {
	m := reviewTestModel(t)
	m.mode = ViewBragView
	next, _ := m.handleReviewPoll(reviewPollSnapshot{})
	if next.(Model).mode != ViewBragView {
		t.Fatal("the brag view should stay open")
	}
}
