package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/model"
	"github.com/achandrapaul/digest/pkg/notify"
	"github.com/achandrapaul/digest/pkg/review"
	tea "github.com/charmbracelet/bubbletea"
)

func TestChooseActionPastTheMenuDoesNothing(t *testing.T) {
	m := syncTestModel(t)
	m.actionMenuItems, m.actionMenuSelected, m.mode = nil, 3, ViewActionMenu
	if next, cmd := m.chooseAction(tea.KeyMsg{}); cmd != nil || next.(Model).mode != ViewActionMenu {
		t.Fatal("an out of range choice should do nothing")
	}
}

func TestReminderSaveErrorsOutsideTheInputGoToMessages(t *testing.T) {
	m := syncTestModel(t)
	next, _ := m.applyReminderSaved(reminderSavedMsg{noteID: "n", err: errors.New("write failed")})
	if latestMessageText(next.(Model)) != "write failed" {
		t.Fatalf("message = %q", latestMessageText(next.(Model)))
	}
	next, _ = m.applyReminderSaved(reminderSavedMsg{noteID: "n", listed: notifyEntriesMsg{err: errors.New("list failed")}})
	if latestMessageText(next.(Model)) != "list failed" {
		t.Fatalf("message = %q", latestMessageText(next.(Model)))
	}
	if !notifyInputAccepts("5", tea.KeyMsg{Type: tea.KeyBackspace}) {
		t.Fatal("non-rune keys are always accepted")
	}
}

func notifyRoot(t *testing.T, noteIDs ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, noteID := range noteIDs {
		if err := notify.Save(root, notify.Entry{NoteID: noteID, Summary: noteID, Interval: "1h"}); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestRemovingAReminderReportsAFailedRemove(t *testing.T) {
	root := notifyRoot(t)
	if err := os.MkdirAll(filepath.Join(notify.Dir(root), "stuck.yaml", "child"), 0755); err != nil {
		t.Fatal(err)
	}
	m := syncTestModel(t)
	m.cfg.DigestRoot = root
	m.actionMenuNoteID = "stuck"
	m.cfg.DigestRoot = root
	if m.cfg.Root() != root {
		t.Skipf("config root %q is not the digest root", m.cfg.Root())
	}
	_, cmd := m.removeReminder()
	if msg, ok := cmd().(notifyEntriesMsg); !ok || msg.err == nil {
		t.Fatalf("msg = %#v", msg)
	}
}

func TestRefreshingRemindersReportsListAndRemoveFailures(t *testing.T) {
	brokenRoot := t.TempDir()
	if err := os.WriteFile(notify.Dir(brokenRoot), []byte("not a dir"), 0644); err != nil {
		t.Fatal(err)
	}
	if msg, ok := refreshNotifyCmd(brokenRoot, nil)().(notifyEntriesMsg); !ok || msg.err == nil {
		t.Fatalf("msg = %#v", msg)
	}
	root := notifyRoot(t, "gone-note")
	if err := os.Chmod(notify.Dir(root), 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(notify.Dir(root), 0o700) })
	if msg, ok := refreshNotifyCmd(root, nil)().(notifyEntriesMsg); !ok || msg.err == nil {
		t.Fatalf("msg = %#v", msg)
	}
}

func TestStaleGitSectionsAreIgnored(t *testing.T) {
	m := syncTestModel(t)
	m.git.ghReviewedToday = nil
	m.applyGitDay(gitDaySectionMsg{generation: m.git.fetchGeneration + 1, reviewed: []GitPRItem{reviewedItem("console", 1)}})
	m.applyMyPRs(gitMyPRsMsg{generation: m.git.fetchGeneration + 1, partOfSync: true, prs: []review.QueuedPR{myPR(4, nil)}})
	if len(m.git.ghReviewedToday) != 0 || len(m.git.myPRs) != 0 {
		t.Fatal("stale sections should not apply")
	}
}

func TestMyPRsFromAFailedHostAreKept(t *testing.T) {
	previous := []review.QueuedPR{myPR(4, nil)}
	kept := keepFailedHostPRs(previous, nil, []string{"github.com"})
	if len(kept) != 1 {
		t.Fatalf("kept = %+v", kept)
	}
}

func TestReviewedItemsWithoutARepoGoUnderGeneral(t *testing.T) {
	m := syncTestModel(t)
	m.git.ghReviewedToday = []GitPRItem{{Title: "x", Kind: "Reviewed", URL: "https://github.com/o/r/pull/1"}, reviewedItem("console", 2)}
	m.rebuildGitRepoStats()
	var names []string
	for _, stat := range m.git.todayGitRepos {
		names = append(names, stat.Name)
	}
	if strings.Join(names, ",") != "console,general" {
		t.Fatalf("repos = %v", names)
	}
}

func TestRecreatingANoteReportsAFailure(t *testing.T) {
	missing := failedNoteSave{attempted: model.Note{Summary: "gone", Status: model.StatusActive, Source: model.SourceManual, Created: time.Now()}}
	msg, ok := recreateNoteCmd(readOnlyNoteStore(t), missing)().(notesSavedMsg)
	if !ok || len(msg.failed) != 1 || msg.failed[0].err == nil {
		t.Fatalf("msg = %#v", msg)
	}
}

func TestSavedFieldsMergeTheCreatedTime(t *testing.T) {
	note := &model.Note{}
	created := time.Now()
	mergeSavedFields(note, model.Note{}, model.Note{Created: created})
	if !note.Created.Equal(created) {
		t.Fatal("a changed created time should be merged")
	}
}

func TestMissingNotePromptWithoutAMissingNote(t *testing.T) {
	m := syncTestModel(t)
	m.missingSave = nil
	if _, cmd := m.confirmRecreateNote(tea.KeyMsg{}); cmd != nil {
		t.Fatal("nothing to recreate")
	}
	if _, cmd := m.discardMissingNote(tea.KeyMsg{}); cmd != nil {
		t.Fatal("nothing to discard")
	}
	if m.missingNoteSummary() != "" {
		t.Fatal("no summary without a missing note")
	}
}

func TestMyPRDetailsForFailingRunningAndDraftPRs(t *testing.T) {
	for state, want := range map[string]string{"FAILURE": "failing", "PENDING": "running", "": "no checks"} {
		if got := myPRCIText(state); got != want {
			t.Fatalf("ci %q = %q", state, got)
		}
	}
	pr := myPR(4, func(pr *review.QueuedPR) {
		pr.IsDraft, pr.Body = true, ""
		pr.Reviews = []review.PRReview{{Author: "me", State: "COMMENTED"}, {Author: "bob", State: "UNKNOWN"}}
	})
	details := myPRDetailsMarkdown(pr)
	if !strings.Contains(details, "(draft)") || !strings.Contains(details, "_No description._") || strings.Contains(details, "Reviewers") {
		t.Fatalf("details:\n%s", details)
	}
}

func TestItemBindingsForIncompleteRows(t *testing.T) {
	m := syncTestModel(t)
	for _, item := range []NavItem{{Kind: KindJobDraft}, {Kind: KindPendingGit}, {Kind: KindTodayNote}, {Kind: KindReviewRun}} {
		if bindings := m.itemBindings(item, true); bindings != nil {
			t.Fatalf("kind %v bindings = %+v", item.Kind, bindings)
		}
	}
}

func TestStopConfirmNamesTheAutomationAndEndsWithNothingToStop(t *testing.T) {
	m := runningJobsModel(t)
	run := m.automationRuns["note-1"]
	m.automationRunToStop = &run
	if m.stopTargetName() != "jira" {
		t.Fatalf("target = %q", m.stopTargetName())
	}
	m.automationRunToStop = nil
	if next, cmd := m.confirmStopRun(); cmd != nil || next.(Model).mode != m.deleteReturnMode {
		t.Fatal("nothing to stop")
	}
}

func TestDashboardFrameWithoutACacheAndScrollClamps(t *testing.T) {
	m := manyTodayNotesModel(t, 80)
	m.frames = nil
	if frame := m.currentDashboardFrame(); frame.content == "" {
		t.Fatal("an uncached frame should still render")
	}
	m.selected = 1
	m.scrollOffset = 50
	m.settleScroll()
	if want := max(m.currentDashboardFrame().selectedLine-2, 0); m.scrollOffset != want {
		t.Fatalf("scrolling up should keep two lines above the row, offset %d want %d", m.scrollOffset, want)
	}
	m.selected = len(m.allNavItems()) - 1
	m.scrollOffset = 10000
	m.settleScroll()
	frame := m.currentDashboardFrame()
	if m.scrollOffset > strings.Count(frame.content, "\n")+1 {
		t.Fatalf("offset = %d", m.scrollOffset)
	}
	m.scrollOffset = -5
	m.selected = 3
	m.settleScroll()
	if m.scrollOffset < 0 {
		t.Fatalf("offset = %d", m.scrollOffset)
	}
}
