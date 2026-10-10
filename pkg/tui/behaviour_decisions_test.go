package tui

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/automation"
	"github.com/achandrapaul/digest/pkg/brag"
	"github.com/achandrapaul/digest/pkg/config"
	"github.com/achandrapaul/digest/pkg/model"
	"github.com/achandrapaul/digest/pkg/notify"
	"github.com/achandrapaul/digest/pkg/review"
	"github.com/achandrapaul/digest/pkg/store"
	"github.com/achandrapaul/digest/pkg/system"

	tea "github.com/charmbracelet/bubbletea"
)

func openNotifyInput(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = pressKey(t, m, "@")
	m, _ = pressKey(t, m, "enter")
	if m.mode != ViewNotifyInput {
		t.Fatalf("mode %v, want the notify input", m.mode)
	}
	return m
}

func TestNotifySaveFailureKeepsTheInputOpenWithTheError(t *testing.T) {
	m, _ := actionsTestModel(t)
	if err := os.MkdirAll(m.cfg.Root(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notify.Dir(m.cfg.Root()), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	m = openNotifyInput(t, m)
	m = typeText(t, m, "2")
	m, cmd := pressKey(t, m, "enter")
	m = applyMsgs(t, m, cmd)
	if m.mode != ViewNotifyInput || m.notifyNotice == "" {
		t.Fatalf("a failed save should keep the input open with the error: mode %v notice %q", m.mode, m.notifyNotice)
	}
	if _, tagged := m.notifyEntries["note-1"]; tagged {
		t.Error("a failed save should not add the tag")
	}
	m, _ = pressKey(t, m, "esc")
	if line := noteLine(m, "Flaky deploys"); strings.Contains(line, "@notify") {
		t.Errorf("row should have no reminder tag: %q", line)
	}
}

func TestNotifyOnANoteThatIsGoneSaysSoInTheHeader(t *testing.T) {
	m, _ := actionsTestModel(t)
	m = openNotifyInput(t, m)
	m.notes = slices.DeleteFunc(m.notes, func(note *model.Note) bool { return note.ID == "note-1" })
	m = typeText(t, m, "2")
	m, cmd := pressKey(t, m, "enter")
	m = applyMsgs(t, m, cmd)
	if m.mode != ViewDashboard || !strings.Contains(headerTopRow(m), "note no longer exists") {
		t.Errorf("mode %v header %q", m.mode, headerTopRow(m))
	}
	if _, found := notify.Load(m.cfg.Root(), "note-1"); found {
		t.Error("no reminder should be written for a missing note")
	}
}

func TestConfirmingStopOnAFinishedBragRunKeepsItListed(t *testing.T) {
	m, _ := bragTestModel(t, wednesday())
	writeBragState(t, m, "2026-W40", "brag.exit", "1")
	m.refreshBragRuns()
	selectNavItem(t, &m, "brag:2026-W40")
	run := m.bragRuns[0]
	m.bragRunToStop = &run
	m = m.beginStopConfirm()
	m = press(t, m, runes("y"))
	if len(m.bragRuns) != 1 || m.bragRuns[0].Status != brag.RunFailed || !system.Exists(brag.StateDir(m.cfg.BragDir(), "2026-W40")) {
		t.Errorf("a failed run stays listed until d dismisses it: runs %+v", m.bragRuns)
	}
}

func TestDOnAFailedReviewRunDismissesIt(t *testing.T) {
	m, _, failed := reviewRunsModel(t)
	selectNavItem(t, &m, "review:"+failed.URL)
	if hint := hintText(m); !strings.Contains(hint, "dismiss") {
		t.Errorf("a failed review run should offer dismiss: %q", hint)
	}
	m = press(t, m, runes("d"))
	if m.mode != ViewDashboard {
		t.Errorf("dismiss should not ask, mode %v", m.mode)
	}
	for _, run := range m.reviewRuns {
		if run.Meta.Ref.URL == failed.URL {
			t.Error("the failed run should be gone from Jobs")
		}
	}
	for _, run := range review.ListRuns(m.cfg.ReviewRootDir()) {
		if run.Meta.Ref.URL == failed.URL {
			t.Error("the failed run should stay dismissed after a refresh")
		}
	}
}

func TestDOnAFinishedReviewRunStillListedDismissesIt(t *testing.T) {
	m, running, _ := reviewRunsModel(t)
	if err := os.Remove(filepath.Join(review.StateDir(m.cfg.ReviewRootDir(), running), "review.pid")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(review.StateDir(m.cfg.ReviewRootDir(), running), "review.exit"), []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}
	selectNavItem(t, &m, "review:"+running.URL)
	m = press(t, m, runes("d"))
	if m.mode == ViewReviewRunConfirm || slices.ContainsFunc(m.reviewRuns, func(run review.ReviewRun) bool { return run.Meta.Ref.URL == running.URL }) {
		t.Errorf("a run that finished should be dismissed, mode %v", m.mode)
	}
}

func stubClipboard(t *testing.T, err error) *[]string {
	t.Helper()
	var copied []string
	original := copyToClipboard
	copyToClipboard = func(text string) error {
		copied = append(copied, text)
		return err
	}
	t.Cleanup(func() { copyToClipboard = original })
	return &copied
}

func TestCopyingShowsCopiedInTheHeader(t *testing.T) {
	stubClipboard(t, nil)
	m := noteSaveModel(t, dashboardNotes(time.Now())...)
	selectSummary(t, &m, "active")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlY})
	if top := headerTopRow(m); !strings.Contains(top, "✓ copied") {
		t.Errorf("header = %q", top)
	}
}

func TestCopyFailuresShowAsHeaderErrors(t *testing.T) {
	stubClipboard(t, errors.New("pbcopy missing"))
	m := noteSaveModel(t, dashboardNotes(time.Now())...)
	selectSummary(t, &m, "active")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlY})
	if top := headerTopRow(m); !strings.Contains(top, "✗") || !strings.Contains(top, "pbcopy missing") {
		t.Errorf("header = %q", top)
	}
	if footer := lastPopupLines(m, 4); !strings.Contains(footer, "✗") || !strings.Contains(footer, "pbcopy missing") {
		t.Errorf("the preview footer should show the error:\n%s", footer)
	}
	if footer := lastPopupLines(press(t, m, runes("j")), 4); strings.Contains(footer, "pbcopy missing") {
		t.Errorf("the next action should clear the footer error:\n%s", footer)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m = press(t, m, tea.KeyMsg{Type: tea.KeyTab}); strings.Contains(stripANSI(m.View()), "pbcopy missing") {
		t.Error("leaving the preview should clear its footer error")
	}
}

func lastPopupLines(m Model, count int) string {
	var popupLines []string
	for _, line := range strings.Split(stripANSI(m.View()), "\n") {
		if strings.TrimSpace(line) != "" {
			popupLines = append(popupLines, line)
		}
	}
	return strings.Join(popupLines[max(0, len(popupLines)-count):], "\n")
}

func TestOpenLinkFailuresShowAsHeaderErrors(t *testing.T) {
	original := openURL
	openURL = func(string) error { return errors.New("no browser") }
	t.Cleanup(func() { openURL = original })
	m := noteSaveModel(t, &model.Note{Summary: "with link", Body: "see https://example.com/a", Status: model.StatusActive, Source: model.SourceManual, Created: time.Now()})
	selectSummary(t, &m, "with link")
	m = press(t, m, runes("o"))
	if top := headerTopRow(m); !strings.Contains(top, "✗") || !strings.Contains(top, "no browser") {
		t.Errorf("header = %q", top)
	}
	m = press(t, press(t, m, tea.KeyMsg{Type: tea.KeyTab}), runes("o"))
	if footer := lastPopupLines(m, 4); m.mode != ViewPreview || !strings.Contains(footer, "✗") || !strings.Contains(footer, "no browser") {
		t.Errorf("the preview footer should show the error, mode %v:\n%s", m.mode, footer)
	}
}

func TestArchiveDeleteReportsADraftThatCouldNotBeRemoved(t *testing.T) {
	m := noteSaveModel(t, dashboardNotes(time.Now())...)
	next, cmd := m.openArchive(tea.KeyMsg{})
	m, _ = applyNoteMessages(t, next.(Model), cmd)
	archived := m.getArchivedNotes()[0]
	draftDir := automation.StateDir(m.automationRoot(), archived.ID)
	if err := os.MkdirAll(draftDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(draftDir, "draft.md"), []byte("draft"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(draftDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(draftDir, 0o755) })
	next, _ = m.Update(runes("d"))
	next, cmd = next.(Model).Update(runes("y"))
	m, _ = applyNoteMessages(t, next.(Model), cmd)
	if _, err := os.Stat(archived.FilePath); !os.IsNotExist(err) {
		t.Fatalf("the note should be deleted: %v", err)
	}
	if top := headerTopRow(m); !strings.Contains(top, "deleted, but couldn't remove draft") {
		t.Errorf("header = %q", top)
	}
}

func TestCorruptMyPRSeenIsRebuiltFromTheNotesUpdates(t *testing.T) {
	noteStore := store.New(t.TempDir())
	seenPath := filepath.Join(t.TempDir(), "seen.json")
	now := time.Date(2026, 10, 6, 10, 0, 0, 0, time.Local)
	pr := myPR(4, func(pr *review.QueuedPR) {
		pr.CIState = "SUCCESS"
		pr.Reviews = []review.PRReview{{Author: "alice", State: "APPROVED", SubmittedAt: now.Add(-time.Minute)}}
	})
	runMyPRNotes(t, noteStore, seenPath, []review.QueuedPR{pr}, nil, now)
	before := onlyMyPRNote(t, noteStore).Body
	if err := os.WriteFile(seenPath, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	runMyPRNotes(t, noteStore, seenPath, []review.QueuedPR{pr}, nil, now.Add(time.Hour))
	if after := onlyMyPRNote(t, noteStore).Body; after != before {
		t.Errorf("a reset seen file should not add updates twice:\n%s\nwant\n%s", after, before)
	}
	later := now.Add(2 * time.Hour)
	pr.Reviews = append(pr.Reviews, review.PRReview{Author: "bob", State: "CHANGES_REQUESTED", SubmittedAt: later})
	runMyPRNotes(t, noteStore, seenPath, []review.QueuedPR{pr}, nil, later)
	if body := onlyMyPRNote(t, noteStore).Body; strings.Count(body, "Changes requested by @bob") != 1 || strings.Count(body, "Approved by @alice") != 1 {
		t.Errorf("new events should still be logged once:\n%s", body)
	}
}

func TestCorruptMyPRSeenStillClosesKnownPRs(t *testing.T) {
	noteStore := store.New(t.TempDir())
	seenPath := filepath.Join(t.TempDir(), "seen.json")
	now := time.Date(2026, 10, 6, 10, 0, 0, 0, time.Local)
	pr := myPR(4, nil)
	runMyPRNotes(t, noteStore, seenPath, []review.QueuedPR{pr}, nil, now)
	if err := os.WriteFile(seenPath, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	msg := runMyPRNotes(t, noteStore, seenPath, nil, nil, now.Add(time.Hour))
	if len(msg.known) != 1 || msg.known[0].URL != pr.Ref.URL {
		t.Fatalf("the open PR note should stay known after a reset: %+v", msg.known)
	}
	runMyPRNotes(t, noteStore, seenPath, nil, map[string]string{pr.Ref.URL: "MERGED"}, now.Add(2*time.Hour))
	if note := onlyMyPRNote(t, noteStore); note.Status != model.StatusDone {
		t.Errorf("merged PR should close its note: %s", note.Status)
	}
}

func TestCorruptPRStatusFileReportsOnceThenResets(t *testing.T) {
	sent := captureAlerts(t)
	path := filepath.Join(t.TempDir(), "pr-status.json")
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	approved := prSnapshot([]review.QueuedPR{alertPR("u/1", "Add trial", "APPROVED")}, nil, nil)
	if err := sendPRAlerts(path, approved); err == nil || len(*sent) != 1 {
		t.Fatalf("a corrupt file should report once and alert like a first sync: err %v sent %d", err, len(*sent))
	}
	*sent = nil
	if err := sendPRAlerts(path, approved); err != nil || len(*sent) != 0 {
		t.Errorf("after the reset the next sync should be quiet: err %v sent %d", err, len(*sent))
	}
	merged := prSnapshot(nil, map[string]string{"u/1": "MERGED"}, nil)
	if err := sendPRAlerts(path, merged); err != nil || len(*sent) != 1 {
		t.Errorf("changes after the reset should alert: err %v sent %d", err, len(*sent))
	}
}

func TestCorruptPRStatusShowsPRAlertsInTheHeader(t *testing.T) {
	m := syncTestModel(t)
	m = update(m, prAlertsMsg{err: errors.New("pr-status.json: invalid")})
	if top := headerTopRow(m); !strings.Contains(top, "✗ pr alerts") {
		t.Errorf("header = %q", top)
	}
}

func TestHistoryFailureShowsOnTheDetailsTab(t *testing.T) {
	original := gitLogForFiles
	gitLogForFiles = func(string, []string) (string, error) { return "", errors.New("bad object origin/HEAD") }
	t.Cleanup(func() { gitLogForFiles = original })
	m := reviewTestModel(t)
	pr := m.git.ghPendingPRs[0].PR
	pr.Files = []string{"a.ts"}
	if err := os.MkdirAll(filepath.Join(review.CloneDir(m.reviewRoot(), pr.Ref), ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := m.relatedHistoryCmd(&m.git.ghPendingPRs[0])
	for _, msg := range collectMsgs(cmd) {
		m = update(m, msg)
	}
	if details := m.renderDetailsMarkdown(&m.git.ghPendingPRs[0]); !strings.Contains(details, "Couldn't read history: bad object origin/HEAD") {
		t.Errorf("details:\n%s", details)
	}
}

func walkWizardToTheLastStep(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = pressKey(t, m, "n")
	m, _ = pressKey(t, m, "enter")
	m.setupInput.SetValue("09:00")
	m, _ = pressKey(t, m, "enter")
	m, _ = pressKey(t, m, "n")
	return m
}

func TestWizardErrorsShowInsideTheWizardAndRetryKeepsTheBackup(t *testing.T) {
	original := "digest_root: /mine\njobs: []\n"
	m, path, installs := setupModel(t, original)
	failing := true
	installNotifications = func(*config.Config) error {
		*installs++
		if failing {
			return errors.New("launchctl refused")
		}
		return nil
	}
	m = walkWizardToTheLastStep(t, m)
	m, _ = pressKey(t, m, "enter")
	if m.mode != ViewSetup || !strings.Contains(stripANSI(m.View()), "launchctl refused") {
		t.Fatalf("the error should show inside the wizard, mode %v:\n%s", m.mode, stripANSI(m.View()))
	}
	if strings.Contains(headerTopRow(syncTestModelFrom(m)), "launchctl") {
		t.Error("wizard errors should not go to the header")
	}
	failing = false
	m, _ = pressKey(t, m, "enter")
	if m.mode != ViewDashboard || *installs != 2 {
		t.Fatalf("retry should finish: mode %v installs %d", m.mode, *installs)
	}
	if backup, _ := os.ReadFile(path + ".bak"); string(backup) != original {
		t.Errorf("a retry should keep the first backup, got %q", backup)
	}
}

func syncTestModelFrom(m Model) Model {
	m.mode = ViewDashboard
	return m
}

func TestSettingsRetryKeepsTheFirstBackup(t *testing.T) {
	original := "digest_root: /mine\njobs: []\n"
	m, path, installs := setupFormModel(t, original)
	failing := true
	installNotifications = func(*config.Config) error {
		*installs++
		if failing {
			return errors.New("launchctl refused")
		}
		return nil
	}
	for range 6 {
		m, _ = pressKey(t, m, "j")
	}
	m, _ = pressKey(t, m, "tab")
	m, cmd := pressKey(t, m, "enter")
	m = applyMsgs(t, m, cmd)
	if m.mode != ViewSetup || !strings.Contains(m.setup.notice, "launchctl refused") {
		t.Fatalf("mode %v notice %q", m.mode, m.setup.notice)
	}
	failing = false
	m, cmd = pressKey(t, m, "enter")
	m = applyMsgs(t, m, cmd)
	if m.mode != ViewDashboard {
		t.Fatalf("retry should save, mode %v", m.mode)
	}
	if backup, _ := os.ReadFile(path + ".bak"); string(backup) != original {
		t.Errorf("a retry should keep the first backup, got %q", backup)
	}
}

func TestRejectCommentBoxHasNoLineLimit(t *testing.T) {
	m := reviewTestModel(t)
	m.rejectInput.Focus()
	for range 150 {
		m.rejectInput.InsertString("line")
		*m.rejectInput, _ = m.rejectInput.Update(tea.KeyMsg{Type: tea.KeyEnter})
	}
	m.rejectInput.InsertString("end")
	if lines := strings.Count(m.rejectInput.Value(), "\n") + 1; lines != 151 {
		t.Errorf("reject comment kept %d lines, want 151", lines)
	}
}

func TestBragAndReviewStartErrorsStayInlineNotices(t *testing.T) {
	originalStart := startBackground
	startBackground = func(review.QueuedPR, string) error { return errors.New("clone failed") }
	t.Cleanup(func() { startBackground = originalStart })
	m := reviewTestModel(t)
	m.previewTab = previewTabDetails
	m = press(t, m, runes("r"))
	m = press(t, m, runes("y"))
	if !strings.Contains(m.reviewNotice, "clone failed") || strings.Contains(headerTopRow(syncTestModelFrom(m)), "clone failed") {
		t.Errorf("review start errors should be an inline notice: notice %q", m.reviewNotice)
	}

	bragModel, _ := bragTestModel(t, wednesday())
	startBragRun = func(string, brag.Period, bool) error { return errors.New("claude missing") }
	bragModel.bragPeriod, bragModel.bragConfirmReturn, bragModel.mode = brag.WeekOf(wednesday()).Previous(), ViewBragList, ViewBragConfirm
	bragModel = press(t, bragModel, runes("y"))
	if bragModel.bragNotice != "claude missing" || strings.Contains(headerTopRow(syncTestModelFrom(bragModel)), "claude missing") {
		t.Errorf("brag start errors should be an inline notice: notice %q", bragModel.bragNotice)
	}
}

func TestUnknownAutomatedKindsShowNoTagAndCanStillAutomate(t *testing.T) {
	m, _ := automationTestModel(t)
	note := m.notes[len(m.notes)-1]
	note.Automated = "made-up"
	m.contentVersion++
	if line := noteLine(m, "Flaky deploys"); strings.Contains(line, "#made-up") {
		t.Errorf("an unknown kind should not show a tag: %q", line)
	}
	if !m.noteCanStartAutomation(note) {
		t.Error("an unknown kind should still allow automating")
	}
	m, _ = pressKey(t, m, "@")
	if !slices.Contains(actionNames(m), "jira") {
		t.Errorf("actions = %v, want the automation offered", actionNames(m))
	}
	note.Automated = "ticket"
	m, _ = pressKey(t, m, "esc")
	m.contentVersion++
	if line := noteLine(m, "Flaky deploys"); !strings.Contains(line, "#ticket") || m.noteCanStartAutomation(note) {
		t.Errorf("a configured kind keeps its tag and blocks automating: %q", line)
	}
}

func TestReloadByIDKeepsANoteFoundNextToACorruptOne(t *testing.T) {
	m, _ := automationTestModel(t)
	note := &model.Note{Summary: "automated", Status: model.StatusDone, Source: model.SourceManual, Created: time.Now(), Updated: time.Now()}
	if err := m.store.Save(note); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(note.FilePath), note.ID+".md"), []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	msg, ok := m.reloadNotesByIDCmd([]string{note.ID})().(notesChangedMsg)
	if !ok || len(msg.notes) != 1 || msg.notes[0].ID != note.ID {
		t.Fatalf("the good note should still reload: %#v", msg)
	}
	m = update(m, msg)
	if top := headerTopRow(m); msg.err == nil || !strings.Contains(top, "✗ store: ") || !strings.Contains(latestMessageText(m), "no front matter") {
		t.Errorf("a skipped note should show as a store error: %q %q", top, latestMessageText(m))
	}
}
