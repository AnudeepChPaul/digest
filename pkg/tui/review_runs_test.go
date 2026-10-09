package tui

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/config"
	"github.com/AnudeepChPaul/digest/pkg/review"
	"github.com/AnudeepChPaul/digest/pkg/sourcecontrol"
)

func seedReviewState(t *testing.T, root string, ref review.PRRef, files map[string]string) {
	t.Helper()
	dir := review.StateDir(root, ref)
	if err := review.WriteMeta(dir, review.Meta{Ref: ref, Title: "t"}); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func prRef(repo string, number int) review.PRRef {
	return review.PRRef{Host: "github.com", Owner: "o", Repo: repo, Number: number, URL: "https://github.com/o/" + repo + "/pull/" + strconv.Itoa(number)}
}

func reviewRunsModel(t *testing.T) (Model, review.PRRef, review.PRRef) {
	t.Helper()
	cfg := &config.Config{DigestRoot: t.TempDir(), GreenOnly: true, Jobs: []config.JobSpec{{Name: "janitor"}}}
	root := cfg.ReviewRootDir()
	running, failed := prRef("console", 1), prRef("web-console", 2)
	seedReviewState(t, root, running, map[string]string{"review.pid": strconv.Itoa(os.Getpid())})
	seedReviewState(t, root, failed, map[string]string{"review.exit": "1", "review.log": "boom: install failed"})
	m := NewModel(cfg, nil)
	m.width, m.height = 120, 60
	return m, running, failed
}

func TestReviewRunsListedUnderJobsOnStartup(t *testing.T) {
	m, running, failed := reviewRunsModel(t)
	var keys []string
	for _, item := range m.allNavItems() {
		keys = append(keys, navItemKey(item))
	}
	joined := strings.Join(keys, ",")
	want := "job:janitor,review:" + running.URL + ",review:" + failed.URL
	if !strings.HasSuffix(joined, want) {
		t.Fatalf("nav keys = %s", joined)
	}
	body := m.renderDashboardBody()
	for _, text := range []string{"console:1", "web-console:2", "reviewing...", "failed"} {
		if !strings.Contains(body, text) {
			t.Errorf("dashboard missing %q", text)
		}
	}
}

func TestPulseStopsWhenOnlyAReviewRuns(t *testing.T) {
	m, _, _ := reviewRunsModel(t)
	m.git.loadingGit = false
	m.messages = nil
	if _, cmd := m.Update(syncPulseTickMsg{}); cmd != nil {
		t.Errorf("pulse kept ticking with no animated header icon")
	}
}

func TestPulseKeepsTickingWhileTheHeaderSyncSpinnerShows(t *testing.T) {
	m, _, _ := reviewRunsModel(t)
	m.messages, m.git.loadingGit = nil, true
	m.postMessage(messageSourceGit, messageProgress, "syncing")
	next, cmd := m.Update(syncPulseTickMsg{})
	if cmd == nil || next.(Model).syncPulseFrame != m.syncPulseFrame+1 {
		t.Errorf("pulse stopped while the header spinner shows")
	}
}

func TestReviewRunPreviewShowsLog(t *testing.T) {
	m, _, failed := reviewRunsModel(t)
	selectNavItem(t, &m, "review:"+failed.URL)
	m.mode = ViewPreview
	m.updatePreviewViewport()
	if !strings.Contains(m.previewViewport.View(), "boom: install failed") {
		t.Errorf("preview = %q", m.previewViewport.View())
	}
}

func TestPRRowTagsFollowReviewState(t *testing.T) {
	m, running, failed := reviewRunsModel(t)
	runningPR := review.QueuedPR{Ref: running, CIState: "SUCCESS"}
	failedPR := review.QueuedPR{Ref: failed, CIState: "SUCCESS"}
	reReviewPR := review.QueuedPR{Ref: prRef("console", 3), CIState: "SUCCESS", MyLastReviewState: "APPROVED", MyLastReviewAt: time.Now()}
	items := []GitPRItem{sourcecontrol.NewPRItem(runningPR, "Pending Review"), sourcecontrol.NewPRItem(failedPR, "Pending Review"), sourcecontrol.NewPRItem(reReviewPR, sourcecontrol.ReReviewKind)}
	if !strings.Contains(m.renderPRTag(&items[0], false), "reviewing...") {
		t.Errorf("running PR tag = %q", m.renderPRTag(&items[0], false))
	}
	if m.prState(&items[1]) != review.StateFailed || !strings.Contains(m.renderPRTag(&items[1], false), "Failed") {
		t.Errorf("failed PR tag = %q", m.renderPRTag(&items[1], false))
	}
	if m.prState(&items[2]) != review.StateApproved {
		t.Errorf("re-review state = %s, want Approved", m.prState(&items[2]))
	}
}

func TestReReviewsFollowPendingInRepoGroup(t *testing.T) {
	m := selectionTestModel(t)
	pending := sourcecontrol.NewPRItem(review.QueuedPR{Ref: prRef("console", 10), CIState: "SUCCESS"}, "Pending Review")
	otherRepo := sourcecontrol.NewPRItem(review.QueuedPR{Ref: prRef("web-console", 11), CIState: "SUCCESS"}, "Pending Review")
	reReview := sourcecontrol.NewPRItem(review.QueuedPR{Ref: prRef("console", 12), CIState: "SUCCESS"}, sourcecontrol.ReReviewKind)
	m.applyGitPending(gitPendingMsg{generation: m.git.fetchGeneration, pending: []GitPRItem{pending, otherRepo, reReview}})
	groups := m.getPendingGitGroups()
	if len(groups) != 2 || groups[0].Name != "console" || len(groups[0].Items) != 2 || groups[0].Items[1].URL != reReview.URL {
		t.Fatalf("groups = %+v", groups)
	}
}
