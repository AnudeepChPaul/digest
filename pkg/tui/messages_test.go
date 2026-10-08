package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/config"
)

func latestMessageText(m Model) string {
	if len(m.messages) == 0 {
		return ""
	}
	return m.messages[len(m.messages)-1].text
}

func headerTopRow(m Model) string {
	return stripANSI(strings.Split(m.renderHeader(), "\n")[1])
}

func TestErrorsShowInTheHeaderAndStayUntilEsc(t *testing.T) {
	m := syncTestModel(t)
	m.showError("JOB ERROR", errors.New("janitor exited 1"))
	if m.mode != ViewDashboard || !strings.Contains(headerTopRow(m), "janitor exited 1") {
		t.Fatalf("errors should show in the header without a popup, mode %v: %q", m.mode, headerTopRow(m))
	}
	later := m
	messageNow = func() time.Time { return time.Now().Add(time.Hour) }
	t.Cleanup(func() { messageNow = time.Now })
	if !strings.Contains(headerTopRow(later), "janitor exited 1") {
		t.Errorf("errors should not fade")
	}
	m, _ = pressKey(t, m, "esc")
	if strings.Contains(headerTopRow(m), "janitor") {
		t.Errorf("esc should dismiss the error: %q", headerTopRow(m))
	}
}

func TestSuccessFadesAndClearsThatSourcesError(t *testing.T) {
	m := syncTestModel(t)
	m.postMessage("git", messageError, "sync failed")
	m.postMessage("git", messageSuccess, "synced")
	if row := headerTopRow(m); !strings.Contains(row, "synced") || strings.Contains(row, "failed") {
		t.Errorf("success should replace that source's error: %q", row)
	}
	messageNow = func() time.Time { return time.Now().Add(5 * time.Second) }
	t.Cleanup(func() { messageNow = time.Now })
	if row := headerTopRow(m); strings.Contains(row, "synced") {
		t.Errorf("success should fade after 4s: %q", row)
	}
}

func TestGitSyncShowsProgressThenSyncedInTheHeader(t *testing.T) {
	m := syncTestModel(t)
	showGit := true
	m.cfg.ShowGit = &showGit
	m.cfg.GitRepositoryRoots = []string{t.TempDir()}
	m.loadingCommits = false
	m.beginGitFetch()
	if row := headerTopRow(m); !strings.Contains(row, "syncing") {
		t.Errorf("sync should show progress in the header: %q", row)
	}
	m.gitSectionsPending = 1
	m.finishGitSection()
	if row := headerTopRow(m); !strings.Contains(row, "synced") {
		t.Errorf("finished sync should say synced: %q", row)
	}
	m.beginGitFetch()
	m.recordSectionError(sectionPending, errors.New("gh: rate limited"))
	m.gitSectionsPending = 1
	m.finishGitSection()
	if row := headerTopRow(m); !strings.Contains(row, "rate limited") {
		t.Errorf("failed sync should show the error: %q", row)
	}
	if body := stripANSI(m.renderDashboardBody()); strings.Contains(body, "syncing...") || strings.Contains(body, "● synced") {
		t.Errorf("sync status should only be in the header")
	}
}

func TestBangOpensTheFullMessageLog(t *testing.T) {
	m := syncTestModel(t)
	long := strings.Repeat("very long automation failure ", 10) + "END"
	m.showError("AUTOMATION ERROR", errors.New(long))
	m, _ = pressKey(t, m, "!")
	if m.mode != ViewError || !strings.Contains(strings.Join(m.errorLines, " "), "END") {
		t.Fatalf("! should open the message log with the full text, mode %v", m.mode)
	}
	if m, _ = pressKey(t, m, "esc"); m.mode != ViewDashboard {
		t.Errorf("esc should close the log, mode %v", m.mode)
	}
	var helpKeys []string
	for _, entry := range m.helpEntries() {
		helpKeys = append(helpKeys, entry.key)
	}
	if !strings.Contains(strings.Join(helpKeys, " "), "!") {
		t.Errorf("help should list !")
	}
}

func TestStartupErrorGoesToTheHeader(t *testing.T) {
	cfg := &config.Config{DigestRoot: t.TempDir()}
	m := NewModel(cfg, errors.New("show_git is on but git_repository_roots is empty"))
	m.width, m.height = 120, 40
	if m.mode == ViewError || !strings.Contains(headerTopRow(m), "git_repository_roots") {
		t.Errorf("startup errors should be a header message: mode %v %q", m.mode, headerTopRow(m))
	}
}
