package tui

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/review"
)

func detailedPRModel(t *testing.T) Model {
	t.Helper()
	m := reviewTestModel(t)
	pr := m.git.ghPendingPRs[0].PR
	pr.Author = "octocat"
	pr.CIState = "FAILURE"
	pr.JiraKey = "PROJ-42"
	pr.CodeOwner = true
	pr.OwnerTeams = []string{"@o/console-team", "@o/platform"}
	pr.Additions, pr.Deletions, pr.ChangedFiles = 12, 3, 2
	pr.RequestedAt = time.Now().Add(-2 * time.Hour)
	m.cfg.JiraBaseURL = "https://jira.example.com/browse/"
	m.contentVersion++
	m.previewTab = previewTabDetails
	m.updatePreviewViewport()
	return m
}

func TestPRDetailsTabShowsAuthorCIJiraAndOwners(t *testing.T) {
	m := detailedPRModel(t)
	details := m.renderDetailsMarkdown(m.currentPRItem())
	for _, want := range []string{
		"**Author:** octocat",
		"**CI:** FAILURE",
		"**Jira:** [PROJ-42](https://jira.example.com/browse/PROJ-42)",
		"**Code owners requested:** @o/console-team, @o/platform",
		"+12 / -3 in 2 files",
	} {
		if !strings.Contains(details, want) {
			t.Errorf("details missing %q:\n%s", want, details)
		}
	}
	view := stripANSI(m.View())
	for _, want := range []string{"octocat", "FAILURE", "PROJ-42", "@o/console-team"} {
		if !strings.Contains(view, want) {
			t.Errorf("rendered Details tab missing %q", want)
		}
	}
}

func TestPRDetailsJiraWithoutBaseURLIsPlainAndOwnersFallBackToYes(t *testing.T) {
	m := detailedPRModel(t)
	m.cfg.JiraBaseURL = ""
	m.git.ghPendingPRs[0].PR.OwnerTeams = nil
	details := m.renderDetailsMarkdown(m.currentPRItem())
	if !strings.Contains(details, "**Jira:** PROJ-42\n") || !strings.Contains(details, "**Code owners requested:** yes") {
		t.Errorf("details:\n%s", details)
	}
}

func TestPRDetailsHideJiraAndOwnersWhenAbsentAndShowUnknownCI(t *testing.T) {
	m := detailedPRModel(t)
	pr := m.git.ghPendingPRs[0].PR
	pr.JiraKey, pr.CodeOwner, pr.CIState = "", false, ""
	details := m.renderDetailsMarkdown(m.currentPRItem())
	if strings.Contains(details, "Jira") || strings.Contains(details, "Code owners") || !strings.Contains(details, "**CI:** unknown") {
		t.Errorf("details:\n%s", details)
	}
}

type openedClone struct {
	dir string
	ref review.PRRef
}

func stubOpenInNvim(t *testing.T) *[]openedClone {
	t.Helper()
	var opened []openedClone
	previous := openInNvim
	openInNvim = func(dir string, ref review.PRRef) error {
		opened = append(opened, openedClone{dir, ref})
		return nil
	}
	t.Cleanup(func() { openInNvim = previous })
	return &opened
}

func clonedPRModel(t *testing.T) (Model, string) {
	t.Helper()
	m := reviewTestModel(t)
	ref := m.currentPRItem().PR.Ref
	cloneDir := review.CloneDir(m.reviewRoot(), ref)
	if err := os.MkdirAll(cloneDir+"/.git", 0755); err != nil {
		t.Fatal(err)
	}
	m.refreshLocalReviews()
	if !m.cloneReady(m.currentPRItem()) {
		t.Fatal("clone should be ready")
	}
	return m, cloneDir
}

func TestOOnAPRPreviewOpensTheCloneInTmux(t *testing.T) {
	opened := stubOpenInNvim(t)
	t.Setenv("TMUX", "/tmp/tmux-test,1,0")
	m, cloneDir := clonedPRModel(t)
	next, cmd := m.Update(runes("o"))
	m = next.(Model)
	if m.reviewNotice != "Opening PR clone in nvim…" || cmd == nil {
		t.Fatalf("notice %q cmd %v", m.reviewNotice, cmd != nil)
	}
	if len(*opened) != 0 {
		t.Fatalf("tmux ran on the UI loop")
	}
	ready, isReady := cmd().(reviewCloneReadyMsg)
	if !isReady || ready.err != nil || ready.openErr != nil {
		t.Fatalf("msg = %+v", ready)
	}
	ref := m.currentPRItem().PR.Ref
	if len(*opened) != 1 || (*opened)[0].dir != cloneDir || (*opened)[0].ref.URL != ref.URL {
		t.Fatalf("opened = %+v", *opened)
	}
	m = update(m, ready)
	if m.reviewNotice != "Opened clone in a new tmux window" {
		t.Errorf("notice %q", m.reviewNotice)
	}
}

func TestOOutsideTmuxSaysSoAndOpensTheCloneFolder(t *testing.T) {
	opened := stubOpenInNvim(t)
	folders := stubOpenURL(t)
	t.Setenv("TMUX", "")
	m, cloneDir := clonedPRModel(t)
	next, cmd := m.Update(runes("o"))
	m = next.(Model)
	if cmd != nil || len(*opened) != 0 || m.reviewNotice != "Not inside tmux; cannot open a new nvim window" {
		t.Errorf("notice %q opened %+v", m.reviewNotice, *opened)
	}
	if len(*folders) != 1 || (*folders)[0] != cloneDir {
		t.Errorf("preview should open the clone folder, opened %v", *folders)
	}
	dashboard, dashboardCloneDir := clonedPRModel(t)
	dashboard.mode = ViewDashboard
	dashboard.cfg.ShowKeyHints = true
	dashboard = press(t, dashboard, runes("o"))
	if len(*folders) != 2 || (*folders)[1] != dashboardCloneDir || !strings.Contains(latestMessageText(dashboard), "Not inside tmux") {
		t.Errorf("dashboard: opened %v message %q", *folders, latestMessageText(dashboard))
	}
}

func TestOWithoutACloneDoesNothing(t *testing.T) {
	opened := stubOpenInNvim(t)
	t.Setenv("TMUX", "/tmp/tmux-test,1,0")
	m := reviewTestModel(t)
	next, cmd := m.Update(runes("o"))
	m = next.(Model)
	if cmd != nil || len(*opened) != 0 || m.reviewNotice != "" || m.mode != ViewPreview {
		t.Errorf("o without a clone: notice %q opened %+v mode %v", m.reviewNotice, *opened, m.mode)
	}
	if footer := footerText(m.reviewFooterItems(m.currentPRItem())); strings.Contains(footer, "o nvim") {
		t.Errorf("footer offers o without a clone: %s", footer)
	}
}

func TestPRFooterOffersOOnceACloneExists(t *testing.T) {
	m, _ := clonedPRModel(t)
	if footer := footerText(m.reviewFooterItems(m.currentPRItem())); !strings.Contains(footer, "o nvim") {
		t.Errorf("footer = %s", footer)
	}
}

func TestOpenCloneErrorShowsInTheNotice(t *testing.T) {
	m, _ := clonedPRModel(t)
	m = update(m, reviewCloneReadyMsg{ref: m.currentPRItem().PR.Ref, err: os.ErrPermission})
	if !strings.HasPrefix(m.reviewNotice, "Clone failed: ") {
		t.Errorf("notice %q", m.reviewNotice)
	}
}
