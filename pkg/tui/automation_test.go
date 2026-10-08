package tui

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"app/pkg/automation"
	"app/pkg/config"
	"app/pkg/model"

	tea "github.com/charmbracelet/bubbletea"
)

type startedAutomation struct {
	root, noteID, name string
	phase              automation.Phase
}

const testDraft = "project: PROJ\nsummary: Flaky deploys\ndescription: |\n  Deploys fail on Mondays\n"

func automationTestModel(t *testing.T) (Model, *[]startedAutomation) {
	t.Helper()
	m := gitStripTestModel(t)
	m.cfg.Automations = []config.AutomationSpec{{Name: "jira", Match: []string{"create a ticket"}}}
	m.notes = append(m.notes, &model.Note{ID: "note-1", Summary: "Flaky deploys", Body: "We should create a ticket for this.", Created: m.currentDate, Source: model.SourceManual})
	var started []startedAutomation
	previous := startAutomation
	startAutomation = func(root, noteID, name string, phase automation.Phase) error {
		started = append(started, startedAutomation{root, noteID, name, phase})
		return nil
	}
	t.Cleanup(func() { startAutomation = previous })
	selectNote(t, &m, "note-1")
	return m, &started
}

func selectNote(t *testing.T, m *Model, noteID string) {
	t.Helper()
	for index, item := range m.allNavItems() {
		if item.Note != nil && item.Note.ID == noteID {
			m.selected = index
			return
		}
	}
	t.Fatalf("note %s not on the dashboard", noteID)
}

func noteLine(m Model, summary string) string {
	for _, line := range plainLines(m.renderDashboardBody()) {
		if strings.Contains(line, summary) {
			return line
		}
	}
	return ""
}

func footerHas(m Model, label string) bool {
	for _, item := range footerItemsFrom(m.activeBindings()) {
		if strings.EqualFold(item.action, label) {
			return true
		}
	}
	return false
}

func withDraft(t *testing.T, m Model, status automation.RunStatus, phase automation.Phase) Model {
	t.Helper()
	if err := automation.SaveDraft(m.cfg.AutomationDir(), "note-1", testDraft); err != nil {
		t.Fatal(err)
	}
	m.automationRuns = map[string]automation.Run{"note-1": {Meta: automation.RunMeta{NoteID: "note-1", Automation: "jira", Phase: phase}, Status: status, HasDraft: true}}
	return m
}

func openDraftTab(t *testing.T, m Model) Model {
	t.Helper()
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.mode != ViewPreview || m.previewTab != previewTabDraft {
		t.Fatalf("mode = %v tab = %v", m.mode, m.previewTab)
	}
	return m
}

func TestNoAutomationTagBeforeADraft(t *testing.T) {
	m, _ := automationTestModel(t)
	if line := noteLine(m, "Flaky deploys"); strings.Contains(line, "#draft") {
		t.Errorf("row tagged before any draft: %q", line)
	}
	if line := noteLine(m, "Flaky deploys"); strings.Contains(line, "#jira") {
		t.Errorf("row shows the automation name: %q", line)
	}
}

func TestXOnTheDashboardDoesNothing(t *testing.T) {
	m, started := automationTestModel(t)
	m = press(t, m, runes("x"))
	if len(*started) != 0 || m.mode != ViewDashboard {
		t.Errorf("mode = %v started = %+v", m.mode, *started)
	}
}

func TestPreviewOffersAutomationsThroughActionsOnly(t *testing.T) {
	m, _ := automationTestModel(t)
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if footerHas(m, "automate") || !footerHas(m, "actions") {
		t.Errorf("preview should offer actions, not automate: %+v", footerItemsFrom(m.activeBindings()))
	}
	m = press(t, m, runes("x"))
	if m.mode != ViewPreview {
		t.Errorf("x should do nothing in the details preview, mode = %v", m.mode)
	}
	if !menuHasAutomation(m, "jira") {
		t.Errorf("matching note menu misses the automation: %+v", m.noteActions(m.notes[len(m.notes)-1]))
	}
}

func menuHasAutomation(m Model, name string) bool {
	note, _ := m.previewNote()
	for _, action := range m.noteActions(note) {
		if action.automation && action.name == name {
			return true
		}
	}
	return false
}

func chooseMenuAutomation(t *testing.T, m Model, name string) Model {
	t.Helper()
	m = press(t, m, runes("."))
	if m.mode != ViewActionMenu {
		t.Fatalf(". did not open the actions menu, mode = %v", m.mode)
	}
	for m.actionMenuItems[m.actionMenuSelected].name != name {
		if m.actionMenuSelected == len(m.actionMenuItems)-1 {
			t.Fatalf("%s missing from menu %+v", name, m.actionMenuItems)
		}
		m = press(t, m, runes("j"))
	}
	return press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
}

func TestPreviewActionsConfirmThenStartTheDraft(t *testing.T) {
	m, started := automationTestModel(t)
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = chooseMenuAutomation(t, m, "jira")
	if m.mode != ViewAutomationConfirm || len(*started) != 0 {
		t.Fatalf("mode = %v started = %+v", m.mode, *started)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != ViewPreview || len(*started) != 0 {
		t.Fatalf("esc did not cancel: mode = %v started = %+v", m.mode, *started)
	}
	m = chooseMenuAutomation(t, m, "jira")
	m = press(t, m, runes("y"))
	if len(*started) != 1 || (*started)[0] != (startedAutomation{m.cfg.AutomationDir(), "note-1", "jira", automation.PhaseDraft}) {
		t.Fatalf("started = %+v", *started)
	}
	if m.mode != ViewPreview || m.previewTab != previewTabDraft {
		t.Errorf("mode = %v tab = %v", m.mode, m.previewTab)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Drafting") {
		t.Errorf("draft tab does not show the running draft:\n%s", view)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if line := noteLine(m, "Flaky deploys"); !strings.Contains(line, "#draft") {
		t.Errorf("row = %q", line)
	}
}

func TestRedraftWarnsAboutReplacingTheDraft(t *testing.T) {
	m, _ := automationTestModel(t)
	m = withDraft(t, m, automation.RunDraftReady, automation.PhaseDraft)
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = chooseMenuAutomation(t, m, "jira")
	if view := stripANSI(m.View()); m.mode != ViewAutomationConfirm || !strings.Contains(view, "replaces") {
		t.Errorf("mode = %v view:\n%s", m.mode, view)
	}
}

func TestTabWithoutADraftClosesThePreview(t *testing.T) {
	m, _ := automationTestModel(t)
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.mode != ViewDashboard {
		t.Errorf("mode = %v", m.mode)
	}
}

func TestDraftTabShowsTheDraftAndCycles(t *testing.T) {
	m, _ := automationTestModel(t)
	m = withDraft(t, m, automation.RunDraftReady, automation.PhaseDraft)
	if line := noteLine(m, "Flaky deploys"); !strings.Contains(line, "#draft") {
		t.Errorf("row = %q", line)
	}
	m = openDraftTab(t, m)
	view := stripANSI(m.View())
	for _, want := range []string{"Details", "Draft", "Flaky deploys", "Deploys fail on Mondays"} {
		if !strings.Contains(view, want) {
			t.Errorf("draft tab misses %q:\n%s", want, view)
		}
	}
	for _, label := range []string{"edit", "draft again", "run", "delete draft", "copy"} {
		if !footerHas(m, label) {
			t.Errorf("draft tab footer misses %q: %+v", label, footerItemsFrom(m.activeBindings()))
		}
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.mode != ViewPreview || m.previewTab != previewTabDetails {
		t.Errorf("tab did not return to details: mode = %v tab = %v", m.mode, m.previewTab)
	}
}

func TestDraftTabRConfirmsThenRunsTheAutomation(t *testing.T) {
	m, started := automationTestModel(t)
	m = withDraft(t, m, automation.RunDraftReady, automation.PhaseDraft)
	m = openDraftTab(t, m)
	m = press(t, m, runes("r"))
	if m.mode != ViewAutomationConfirm || len(*started) != 0 {
		t.Fatalf("mode = %v started = %+v", m.mode, *started)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(*started) != 1 || (*started)[0].phase != automation.PhaseCreate {
		t.Fatalf("started = %+v", *started)
	}
	if m.mode != ViewPreview || m.previewTab != previewTabDraft {
		t.Errorf("mode = %v tab = %v", m.mode, m.previewTab)
	}
}

func TestDraftTabXConfirmsThenDraftsAgain(t *testing.T) {
	m, started := automationTestModel(t)
	m = withDraft(t, m, automation.RunFailed, automation.PhaseCreate)
	m = openDraftTab(t, m)
	m = press(t, m, runes("x"))
	if view := stripANSI(m.View()); m.mode != ViewAutomationConfirm || !strings.Contains(view, "replaces") {
		t.Fatalf("mode = %v view:\n%s", m.mode, view)
	}
	m = press(t, m, runes("y"))
	if len(*started) != 1 || (*started)[0].phase != automation.PhaseDraft {
		t.Fatalf("started = %+v", *started)
	}
	if m.mode != ViewPreview || m.previewTab != previewTabDraft {
		t.Errorf("mode = %v tab = %v", m.mode, m.previewTab)
	}
}

func TestEveryConfirmAcceptsYAndEnter(t *testing.T) {
	m, _ := automationTestModel(t)
	for mode := ViewDashboard; mode <= ViewAutomationConfirm; mode++ {
		m.mode = mode
		for _, binding := range m.activeBindings() {
			if binding.binding.Help().Desc != "confirm" {
				continue
			}
			keys := binding.binding.Keys()
			if !slices.Contains(keys, "y") || !slices.Contains(keys, "enter") {
				t.Errorf("mode %d confirm keys = %v", mode, keys)
			}
		}
	}
}

func TestRunningDraftIsReadOnly(t *testing.T) {
	m, started := automationTestModel(t)
	m = withDraft(t, m, automation.RunRunning, automation.PhaseCreate)
	m = openDraftTab(t, m)
	for _, label := range []string{"edit", "draft again", "run", "delete draft"} {
		if footerHas(m, label) {
			t.Errorf("running draft offers %q", label)
		}
	}
	for _, key := range []tea.KeyMsg{runes("x"), runes("r"), {Type: tea.KeyEnter}, runes("d")} {
		m = press(t, m, key)
		if m.mode != ViewPreview || len(*started) != 0 {
			t.Fatalf("%v acted on a running draft: mode = %v started = %+v", key, m.mode, *started)
		}
	}
	if _, err := automation.LoadDraft(m.cfg.AutomationDir(), "note-1"); err != nil {
		t.Errorf("running draft deleted: %v", err)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Running") {
		t.Errorf("draft tab does not show the run:\n%s", view)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if footerHas(m, "automate") {
		t.Error("details tab offers automate while running")
	}
}

func TestEnterEditsTheDraft(t *testing.T) {
	m, _ := automationTestModel(t)
	m = withDraft(t, m, automation.RunDraftReady, automation.PhaseDraft)
	m = openDraftTab(t, m)
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != ViewAutomationEdit || !strings.Contains(m.editor.Value(), "summary: Flaky deploys") {
		t.Fatalf("mode = %v editor = %q", m.mode, m.editor.Value())
	}
	assertEditorAtTop(t, "automation", m, strings.SplitN(m.editor.Value(), "\n", 2)[0])
	m.editor.SetValue("project: PROJ\n")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlO})
	if m.mode != ViewAutomationEdit || !strings.Contains(m.automationNotice, "summary") {
		t.Fatalf("invalid draft accepted: mode = %v notice = %q", m.mode, m.automationNotice)
	}
	m.editor.SetValue("project: PROJ\nsummary: Deploys break on Mondays\n")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlO})
	if m.mode != ViewPreview || m.previewTab != previewTabDraft {
		t.Fatalf("mode = %v tab = %v", m.mode, m.previewTab)
	}
	if saved, _ := automation.LoadDraft(m.cfg.AutomationDir(), "note-1"); !strings.Contains(saved, "Deploys break on Mondays") {
		t.Errorf("saved draft = %q", saved)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Deploys break on Mondays") {
		t.Errorf("draft tab shows the old draft:\n%s", view)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != ViewPreview || m.previewTab != previewTabDraft {
		t.Errorf("esc did not return to the draft tab: mode = %v tab = %v", m.mode, m.previewTab)
	}
}

func TestDDeletesTheDraft(t *testing.T) {
	m, _ := automationTestModel(t)
	m = withDraft(t, m, automation.RunDraftReady, automation.PhaseDraft)
	m = openDraftTab(t, m)
	m = press(t, m, runes("d"))
	if _, err := os.Stat(automation.StateDir(m.cfg.AutomationDir(), "note-1")); !os.IsNotExist(err) {
		t.Errorf("draft kept: %v", err)
	}
	if m.mode != ViewPreview || m.previewTab != previewTabDetails {
		t.Errorf("mode = %v tab = %v", m.mode, m.previewTab)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if line := noteLine(m, "Flaky deploys"); strings.Contains(line, "#draft") {
		t.Errorf("row still tagged: %q", line)
	}
}

func TestCtrlYCopiesTheDraft(t *testing.T) {
	m, _ := automationTestModel(t)
	var copied []string
	previous := copyToClipboard
	copyToClipboard = func(text string) error { copied = append(copied, text); return nil }
	t.Cleanup(func() { copyToClipboard = previous })
	m = withDraft(t, m, automation.RunDraftReady, automation.PhaseDraft)
	m = openDraftTab(t, m)
	press(t, m, tea.KeyMsg{Type: tea.KeyCtrlY})
	if len(copied) != 1 || copied[0] != strings.TrimSpace(testDraft) {
		t.Errorf("copied = %q", copied)
	}
}

func TestFailedRunCanBeRunAgain(t *testing.T) {
	m, started := automationTestModel(t)
	if err := automation.SaveDraft(m.cfg.AutomationDir(), "note-1", testDraft); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(automation.LogPath(m.cfg.AutomationDir(), "note-1"), []byte("Error running automation: claude exploded\n"), 0644); err != nil {
		t.Fatal(err)
	}
	meta := automation.RunMeta{NoteID: "note-1", Automation: "jira", Phase: automation.PhaseCreate}
	m.automationRuns = map[string]automation.Run{"note-1": {Meta: meta, Status: automation.RunRunning, HasDraft: true}}
	next, _ := m.handleReviewPoll(reviewPollSnapshot{automationRuns: map[string]automation.Run{"note-1": {Meta: meta, Status: automation.RunFailed, HasDraft: true}}})
	m = next.(Model)
	if m.mode == ViewError || !strings.Contains(latestMessageText(m), "claude exploded") {
		t.Fatalf("mode = %v message = %q", m.mode, latestMessageText(m))
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if line := noteLine(m, "Flaky deploys"); !strings.Contains(line, "#draft") {
		t.Errorf("row = %q", line)
	}
	m = openDraftTab(t, m)
	m = press(t, m, runes("r"))
	m = press(t, m, runes("y"))
	if len(*started) != 1 || (*started)[0].phase != automation.PhaseCreate {
		t.Errorf("retry started = %+v", *started)
	}
}

func TestNeedsReauthShowsTheHint(t *testing.T) {
	m, _ := automationTestModel(t)
	m.cfg.Automations[0].ReauthHint = "store a fresh token"
	meta := automation.RunMeta{NoteID: "note-1", Automation: "jira", Phase: automation.PhaseCreate}
	m.automationRuns = map[string]automation.Run{"note-1": {Meta: meta, Status: automation.RunRunning, HasDraft: true}}
	next, _ := m.handleReviewPoll(reviewPollSnapshot{automationRuns: map[string]automation.Run{"note-1": {Meta: meta, Status: automation.RunNeedsReauth, HasDraft: true}}})
	m = next.(Model)
	if m.mode == ViewError || !strings.Contains(latestMessageText(m), "store a fresh token") {
		t.Fatalf("mode = %v message = %q", m.mode, latestMessageText(m))
	}
}

func TestSucceededRunReloadsNotesAndTagsTheKind(t *testing.T) {
	m, _ := automationTestModel(t)
	if err := os.MkdirAll(automation.StateDir(m.cfg.AutomationDir(), "note-1"), 0755); err != nil {
		t.Fatal(err)
	}
	meta := automation.RunMeta{NoteID: "note-1", Automation: "jira", Phase: automation.PhaseCreate}
	m.automationRuns = map[string]automation.Run{"note-1": {Meta: meta, Status: automation.RunRunning, HasDraft: true}}
	next, cmd := m.handleReviewPoll(reviewPollSnapshot{automationRuns: map[string]automation.Run{"note-1": {Meta: meta, Status: automation.RunCreated}}})
	m = next.(Model)
	if _, err := os.Stat(automation.StateDir(m.cfg.AutomationDir(), "note-1")); !os.IsNotExist(err) {
		t.Errorf("state kept after success: %v", err)
	}
	if _, tracked := m.automationRuns["note-1"]; tracked {
		t.Error("finished run still tracked")
	}
	reloads := false
	for _, msg := range collectMsgs(cmd) {
		if _, ok := msg.(loadNotesMsg); ok {
			reloads = true
		}
	}
	if !reloads {
		t.Error("notes not reloaded after the automation succeeded")
	}
	for kind, tag := range map[string]string{"ticket": "#ticket", "Doc": "#doc"} {
		automated := *m.notes[len(m.notes)-1]
		automated.Automated = kind
		m.notes = append(append([]*model.Note{}, m.notes[:len(m.notes)-1]...), &automated)
		m.contentVersion++
		line := noteLine(m, "Flaky deploys")
		if !strings.Contains(line, tag) || strings.Contains(line, "#draft") {
			t.Errorf("kind %q row = %q", kind, line)
		}
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if footerHas(m, "automate") {
		t.Error("automated note still offers automate")
	}
}

func TestDraftsSurviveARestart(t *testing.T) {
	m, _ := automationTestModel(t)
	if err := automation.SaveDraft(m.cfg.AutomationDir(), "note-1", testDraft); err != nil {
		t.Fatal(err)
	}
	restarted := NewModel(m.cfg, nil)
	if run, tracked := restarted.automationRuns["note-1"]; !tracked || !run.HasDraft {
		t.Errorf("draft not loaded at startup: %+v", restarted.automationRuns)
	}
}

func TestNoBindingUsesE(t *testing.T) {
	m, _ := automationTestModel(t)
	m = withDraft(t, m, automation.RunDraftReady, automation.PhaseDraft)
	check := func(label string, bindings []keyBinding) {
		for _, binding := range bindings {
			for _, key := range binding.binding.Keys() {
				if key == "e" || key == "E" {
					t.Errorf("%s binds %q to %v", label, key, binding.action)
				}
			}
		}
	}
	for mode := ViewDashboard; mode <= ViewAutomationConfirm; mode++ {
		m.mode = mode
		check(fmt.Sprintf("mode %d", mode), m.activeBindings())
	}
	m.mode = ViewPreview
	m.previewTab = previewTabDraft
	check("draft tab", m.activeBindings())
}

func runningJobsModel(t *testing.T) Model {
	t.Helper()
	m, _ := automationTestModel(t)
	if err := os.MkdirAll(automation.StateDir(m.cfg.AutomationDir(), "note-1"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(automation.LogPath(m.cfg.AutomationDir(), "note-1"), []byte("reading create_meta for PROJ\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m = withDraft(t, m, automation.RunRunning, automation.PhaseCreate)
	m.contentVersion++
	return m
}

func selectAutomationJob(t *testing.T, m *Model) {
	t.Helper()
	for index, item := range m.allNavItems() {
		if item.Kind == KindAutomationRun {
			m.selected = index
			return
		}
	}
	t.Fatal("no automation row in Jobs")
}

func TestRunningAutomationShowsInJobs(t *testing.T) {
	m := runningJobsModel(t)
	lines := plainLines(m.renderDashboardBody())
	jobsLine, rowLine := -1, -1
	for index, line := range lines {
		if strings.Contains(line, "Jobs") && jobsLine < 0 {
			jobsLine = index
		}
		if strings.Contains(line, "jira create: Flaky deploys") {
			rowLine = index
		}
	}
	if rowLine < 0 || rowLine < jobsLine {
		t.Fatalf("jobs = %d row = %d:\n%s", jobsLine, rowLine, strings.Join(lines, "\n"))
	}
	m.automationRuns["note-1"] = automation.Run{Meta: m.automationRuns["note-1"].Meta, Status: automation.RunDraftReady, HasDraft: true}
	m.contentVersion++
	for _, item := range m.allNavItems() {
		if item.Kind == KindAutomationRun {
			t.Error("finished run still listed in Jobs")
		}
	}
}

func TestAutomationJobPreviewShowsTheLog(t *testing.T) {
	m := runningJobsModel(t)
	selectAutomationJob(t, &m)
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	view := stripANSI(m.View())
	if m.mode != ViewPreview || !strings.Contains(view, "RUNNING") || !strings.Contains(view, "reading create_meta for PROJ") {
		t.Errorf("mode = %v view:\n%s", m.mode, view)
	}
	if !footerHas(m, "stop") {
		t.Errorf("preview footer misses stop: %+v", footerItemsFrom(m.activeBindings()))
	}
}

func TestDStopsTheAutomationJob(t *testing.T) {
	for _, fromPreview := range []bool{false, true} {
		m := runningJobsModel(t)
		var stopped []string
		previous := stopAutomation
		stopAutomation = func(root, noteID string) error {
			stopped = append(stopped, noteID)
			return nil
		}
		t.Cleanup(func() { stopAutomation = previous })
		selectAutomationJob(t, &m)
		if fromPreview {
			m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
		}
		m = press(t, m, runes("d"))
		if m.mode != ViewDeleteConfirm || len(stopped) != 0 {
			t.Fatalf("preview %v: d should ask before stopping, mode = %v stopped = %v", fromPreview, m.mode, stopped)
		}
		m = press(t, m, runes("y"))
		if len(stopped) != 1 || stopped[0] != "note-1" {
			t.Fatalf("preview %v stopped = %v", fromPreview, stopped)
		}
		if m.automationRuns["note-1"].Status != automation.RunFailed || m.mode == ViewError {
			t.Errorf("preview %v run = %+v mode = %v", fromPreview, m.automationRuns["note-1"], m.mode)
		}
		if _, err := automation.LoadDraft(m.cfg.AutomationDir(), "note-1"); err != nil {
			t.Errorf("stop removed the draft: %v", err)
		}
	}
}

func TestTypingEditsTheDraft(t *testing.T) {
	m, _ := automationTestModel(t)
	m = withDraft(t, m, automation.RunDraftReady, automation.PhaseDraft)
	m = openDraftTab(t, m)
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m.editor.SetValue("summary: Flaky deploy")
	m = press(t, m, runes("s"))
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	for _, key := range "labels: [ops]x" {
		m = press(t, m, runes(string(key)))
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
	if m.mode != ViewAutomationEdit || m.editor.Value() != "summary: Flaky deploys\nlabels: [ops]" {
		t.Fatalf("mode = %v editor = %q", m.mode, m.editor.Value())
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlO})
	if saved, _ := automation.LoadDraft(m.cfg.AutomationDir(), "note-1"); saved != "summary: Flaky deploys\nlabels: [ops]" {
		t.Errorf("saved draft = %q", saved)
	}
}
