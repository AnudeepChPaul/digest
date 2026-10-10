package tui

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/achandrapaul/digest/pkg/automation"
	"github.com/achandrapaul/digest/pkg/model"
	tea "github.com/charmbracelet/bubbletea"
)

func draftTabModel(t *testing.T, status automation.RunStatus) Model {
	t.Helper()
	m, _ := automationTestModel(t)
	m = withDraft(t, m, status, automation.PhaseCreate)
	return openDraftTab(t, m)
}

func readOnlyAutomationRoot(t *testing.T, m Model) {
	t.Helper()
	root := m.cfg.AutomationDir()
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(root, 0o700) })
}

func TestAutomationDraftEditorScreenShowsTheDraftAndNotice(t *testing.T) {
	m := draftTabModel(t, automation.RunFailed)
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != ViewAutomationEdit {
		t.Fatalf("mode = %v", m.mode)
	}
	m.automationNotice = "could not save"
	view := stripANSI(m.View())
	for _, want := range []string{"EDIT JIRA DRAFT", "summary: Flaky deploys", "could not save"} {
		if !strings.Contains(view, want) {
			t.Fatalf("editor screen missing %q:\n%s", want, view)
		}
	}
}

func TestCreateStepConfirmAsksToRunTheDraft(t *testing.T) {
	m := draftTabModel(t, automation.RunFailed)
	m = press(t, m, runes("r"))
	if m.mode != ViewAutomationConfirm || m.automationPhase != automation.PhaseCreate {
		t.Fatalf("mode %v phase %v", m.mode, m.automationPhase)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Run jira with this draft?") || !strings.Contains(view, "Claude creates it in the background.") {
		t.Fatalf("confirm:\n%s", view)
	}
}

func TestEditingAMissingDraftShowsAnError(t *testing.T) {
	m := draftTabModel(t, automation.RunFailed)
	if err := os.RemoveAll(automation.StateDir(m.cfg.AutomationDir(), "note-1")); err != nil {
		t.Fatal(err)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode == ViewAutomationEdit || !strings.Contains(latestMessageText(m), "draft missing") {
		t.Fatalf("mode %v message %q", m.mode, latestMessageText(m))
	}
}

func TestDraftTabFailureAndReauthBanners(t *testing.T) {
	m := draftTabModel(t, automation.RunNeedsReauth)
	if markdown := m.draftTabMarkdown(m.notes[len(m.notes)-1]); !strings.Contains(markdown, "The last run needs re-auth. Re-authenticate, then press **r** to run it again.") {
		t.Fatalf("markdown:\n%s", markdown)
	}
	if err := os.Remove(automation.LogPath(m.cfg.AutomationDir(), "note-1")); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.RemoveAll(automation.StateDir(m.cfg.AutomationDir(), "note-1")); err != nil {
		t.Fatal(err)
	}
	if markdown := m.draftTabMarkdown(m.notes[len(m.notes)-1]); !strings.Contains(markdown, "# Draft failed") || !strings.Contains(markdown, "No output.") {
		t.Fatalf("markdown:\n%s", markdown)
	}
}

func TestDraftThatIsNotYAMLShowsAsARawBlock(t *testing.T) {
	if got := draftMarkdown("not: [valid"); got != "```yaml\nnot: [valid\n```" {
		t.Fatalf("markdown = %q", got)
	}
}

func TestAutomationFailureFallbacks(t *testing.T) {
	m, _ := automationTestModel(t)
	if got := m.automationFailure("none"); got != "The automation stopped without output. Press r on the Draft tab to run it again." {
		t.Fatalf("failure = %q", got)
	}
	if got := m.reauthHint(automation.Run{Meta: automation.RunMeta{Automation: "unknown"}}); got != "unknown could not sign in to its tools." {
		t.Fatalf("hint = %q", got)
	}
	preview := m.automationRunPreview(automation.Run{Meta: automation.RunMeta{NoteID: "none", Automation: "jira", Phase: automation.PhaseDraft}})
	if preview.log != "(no log output yet)" {
		t.Fatalf("preview log = %q", preview.log)
	}
}

func TestConfirmAutomationReportsAStartFailure(t *testing.T) {
	m, _ := automationTestModel(t)
	startAutomation = func(string, string, string, automation.Phase) error { return errors.New("no claude") }
	m.automationReturnMode = ViewPreview
	next, cmd := m.confirmAutomation(tea.KeyMsg{})
	if cmd != nil || len(next.(Model).automationRuns) != 0 {
		t.Fatal("a failed start should not track a run")
	}
}

func TestConfirmAutomationWithoutTrackedRunsStartsAMap(t *testing.T) {
	m, started := automationTestModel(t)
	m.automationRuns = nil
	m.automationNoteID, m.automationName, m.automationPhase = "note-1", "jira", automation.PhaseDraft
	m.automationReturnMode = ViewDashboard
	next, _ := m.confirmAutomation(tea.KeyMsg{})
	if run := next.(Model).automationRuns["note-1"]; run.Status != automation.RunRunning || len(*started) != 1 {
		t.Fatalf("run %+v started %v", run, *started)
	}
}

func TestAutomationGuardsWithoutAPreviewNote(t *testing.T) {
	m, _ := automationTestModel(t)
	m.selected = 999
	if m.canAutomate() || m.canRunDraft() || m.canDeleteDraft() || m.draftText() != "" {
		t.Fatal("nothing applies without a selected note")
	}
	if _, matched := m.noteAutomation(&model.Note{}); matched {
		t.Fatal("a note without an ID never matches")
	}
	if _, tracked := m.noteRun(nil); tracked {
		t.Fatal("no note has no run")
	}
	selectNote(t, &m, "note-1")
	m.notes[len(m.notes)-1].Body = "nothing to automate"
	if m.canAutomate() {
		t.Fatal("an unmatched note cannot automate")
	}
}

func TestAutomatedNotesCannotAutomateAgain(t *testing.T) {
	m, _ := automationTestModel(t)
	m.notes[len(m.notes)-1].Automated = "jira"
	if m.automatedKindKnown("jira") && m.canAutomate() {
		t.Fatal("an automated note should not offer automation")
	}
}

func TestAnyAutomationRunningSeesRunningRuns(t *testing.T) {
	m := runningJobsModel(t)
	if !m.anyAutomationRunning() {
		t.Fatal("a running run should count")
	}
}

func TestDeletingADraftReportsADismissFailure(t *testing.T) {
	m := draftTabModel(t, automation.RunFailed)
	readOnlyAutomationRoot(t, m)
	m = press(t, m, runes("d"))
	if _, tracked := m.automationRuns["note-1"]; !tracked {
		t.Fatal("a failed delete should keep the run")
	}
}

func TestStopAndDismissFailuresKeepTheRun(t *testing.T) {
	m := runningJobsModel(t)
	previous := stopAutomation
	stopAutomation = func(string, string) error { return errors.New("kill failed") }
	t.Cleanup(func() { stopAutomation = previous })
	run := m.automationRuns["note-1"]
	next, _ := m.stopAutomationRun(&run)
	if next.(Model).automationRuns["note-1"].Status != automation.RunRunning {
		t.Fatal("a failed stop should leave the run running")
	}
	stopped := automation.Run{Meta: automation.RunMeta{NoteID: "other"}}
	if next, _ := m.stopAutomationRun(&stopped); len(next.(Model).automationRuns) != len(m.automationRuns) {
		t.Fatal("stopping a run that is not running does nothing")
	}
	readOnlyAutomationRoot(t, m)
	next, _ = m.dismissAutomationRun(&run)
	if _, tracked := next.(Model).automationRuns["note-1"]; !tracked {
		t.Fatal("a failed dismiss should keep the run")
	}
}

func TestApplyingRunsKeepsUnlistedRunningRunsAndSortsJobs(t *testing.T) {
	m := runningJobsModel(t)
	m.automationRuns["note-0"] = automation.Run{Meta: automation.RunMeta{NoteID: "note-0", Automation: "jira"}, Status: automation.RunFailed}
	m.applyAutomationRuns(nil)
	if _, kept := m.automationRuns["note-1"]; !kept {
		t.Fatal("a running run missing from disk should be kept")
	}
	jobs := m.automationJobRuns()
	if len(jobs) != 1 || jobs[0].Meta.NoteID != "note-1" {
		t.Fatalf("jobs = %+v", jobs)
	}
	m.automationRuns["note-0"] = automation.Run{Meta: automation.RunMeta{NoteID: "note-0"}, Status: automation.RunFailed}
	if jobs := m.automationJobRuns(); len(jobs) != 2 || jobs[0].Meta.NoteID != "note-0" {
		t.Fatalf("jobs should be sorted by note, got %+v", jobs)
	}
}

func TestAppliedRunsLeaveTheDraftTabWhenItsRunIsGone(t *testing.T) {
	m := draftTabModel(t, automation.RunFailed)
	m.applyAutomationRuns(map[string]automation.Run{})
	if m.previewTab != previewTabDetails {
		t.Fatalf("tab = %v", m.previewTab)
	}
}

func TestAppliedRunsCloseTheJobPreviewWhenItsRunIsGone(t *testing.T) {
	m := failedJobsModel(t, automation.RunFailed)
	selectAutomationJob(t, &m)
	m.mode = ViewPreview
	m.applyAutomationRuns(map[string]automation.Run{})
	if m.mode != ViewDashboard {
		t.Fatalf("mode = %v", m.mode)
	}
}

func TestCreatedRunWhoseDismissFailsShowsAnError(t *testing.T) {
	m := runningJobsModel(t)
	readOnlyAutomationRoot(t, m)
	cmd := m.applyAutomationRuns(map[string]automation.Run{"note-1": {Meta: automation.RunMeta{NoteID: "note-1"}, Status: automation.RunCreated}})
	if cmd == nil || latestMessageText(m) == "" {
		t.Fatalf("message %q", latestMessageText(m))
	}
}
