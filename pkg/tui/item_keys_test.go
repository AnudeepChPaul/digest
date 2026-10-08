package tui

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"app/pkg/config"
	"app/pkg/model"
	"app/pkg/review"

	tea "github.com/charmbracelet/bubbletea"
)

var dashboardOnlyActions = map[keyAction]keyAction{actionDashboardApprove: actionApprove, actionDashboardReject: actionRejectOrStopReview}

func hintKeyMsg(hintKey string) tea.KeyMsg {
	switch hintKey {
	case "␣":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
	case ".|@":
		return runes(".")
	}
	return runes(hintKey)
}

func assertRowMatchesPreview(t *testing.T, name string, m Model) {
	t.Helper()
	m.mode = ViewDashboard
	hints := m.selectedRowHints()
	preview := m
	preview.mode = ViewPreview
	previewHints := map[string]string{}
	for _, hint := range hintsFromBindings(preview.previewBindings()) {
		previewHints[hint.key] = hint.label
	}
	checked := 0
	for _, hint := range hints {
		if hint.key == "↵" || hint.key == "i" {
			continue
		}
		checked++
		if previewHints[hint.key] != hint.label {
			t.Errorf("%s: row shows (%s)%s but preview shows %q", name, hint.key, hint.label, previewHints[hint.key])
		}
		rowBinding, rowFound := m.resolveKey(hintKeyMsg(hint.key))
		previewBinding, previewFound := preview.resolveKey(hintKeyMsg(hint.key))
		rowAction := rowBinding.action
		if mapped, dashboardOnly := dashboardOnlyActions[rowAction]; dashboardOnly {
			rowAction = mapped
		}
		if !rowFound || !previewFound || rowAction != previewBinding.action {
			t.Errorf("%s: %s does different things on the row (%v) and in the preview (%v)", name, hint.key, rowBinding.action, previewBinding.action)
		}
	}
	if checked == 0 {
		t.Errorf("%s: row hint has no shared keys: %+v", name, hints)
	}
}

func jobTestModel(t *testing.T) (Model, *[]string, *[]string) {
	t.Helper()
	m := syncTestModel(t)
	m.cfg.ShowKeyHints = true
	m.cfg.Jobs = []config.JobSpec{{Name: "nightly", Command: "true", DryRunCommand: "true"}}
	var executed, dryRun []string
	previousExecute, previousDryRun := executeJobBackground, startDryRunBackground
	executeJobBackground = func(cfg *config.Config, jobName string) error {
		executed = append(executed, jobName)
		return nil
	}
	startDryRunBackground = func(spec config.JobSpec) error {
		dryRun = append(dryRun, spec.Name)
		return nil
	}
	t.Cleanup(func() { executeJobBackground, startDryRunBackground = previousExecute, previousDryRun })
	m.contentVersion++
	selectNavItem(t, &m, "job:nightly")
	return m, &executed, &dryRun
}

func markJobRunning(t *testing.T, m *Model, jobName string) {
	t.Helper()
	pid := os.Getpid()
	if err := os.WriteFile(filepath.Join(getLogsDir(), jobName+".pid"), []byte(strconv.Itoa(pid)), 0644); err != nil {
		t.Fatal(err)
	}
	m.runningJobPIDs = map[string]int{jobName: pid}
	m.contentVersion++
}

func hintText(m Model) string {
	return stripANSI(renderHintPill(m.selectedRowHints()))
}

func TestRowHintsMatchPreviewKeys(t *testing.T) {
	note, _ := automationTestModel(t)
	note.cfg.ShowKeyHints = true
	assertRowMatchesPreview(t, "note", note)

	pr := reviewTestModel(t)
	pr.cfg.ShowKeyHints = true
	assertRowMatchesPreview(t, "pending PR", pr)
	markRunning(t, pr, pr.currentPRItem().PR.Ref)
	assertRowMatchesPreview(t, "reviewing PR", pr)

	job, _, _ := jobTestModel(t)
	assertRowMatchesPreview(t, "idle job", job)
	markJobRunning(t, &job, "nightly")
	assertRowMatchesPreview(t, "running job", job)

	automationJob := runningJobsModel(t)
	selectAutomationJob(t, &automationJob)
	assertRowMatchesPreview(t, "automation run", automationJob)

	bragJob, _ := bragTestModel(t, wednesday())
	writeBragState(t, bragJob, "2026-W40", "brag.pid", strconv.Itoa(os.Getpid()))
	bragJob.refreshBragRuns()
	selectNavItem(t, &bragJob, "brag:2026-W40")
	assertRowMatchesPreview(t, "brag run", bragJob)
}

func TestJobRowHintFollowsRunState(t *testing.T) {
	m, _, _ := jobTestModel(t)
	if hint := hintText(m); !strings.Contains(hint, "(↵)open (r)run (d)dry run") {
		t.Errorf("idle job hint = %q", hint)
	}
	markJobRunning(t, &m, "nightly")
	if hint := hintText(m); !strings.Contains(hint, "(↵)open (d)stop") || strings.Contains(hint, "(r)") {
		t.Errorf("running job hint = %q", hint)
	}
}

func TestROnJobRunsItAfterConfirmFromRowAndPreview(t *testing.T) {
	for _, fromPreview := range []bool{false, true} {
		m, executed, dryRun := jobTestModel(t)
		startMode := ViewDashboard
		if fromPreview {
			m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
			startMode = ViewPreview
		}
		m = press(t, m, runes("r"))
		if m.mode != ViewDeleteConfirm || m.jobToExecute != "nightly" || len(*executed) != 0 {
			t.Fatalf("preview %v: r should ask first, mode = %v executed = %v", fromPreview, m.mode, *executed)
		}
		m = press(t, m, runes("y"))
		if len(*executed) != 1 || (*executed)[0] != "nightly" || len(*dryRun) != 0 {
			t.Errorf("preview %v: executed = %v dryRun = %v", fromPreview, *executed, *dryRun)
		}
		if m.mode != startMode {
			t.Errorf("preview %v: returned to %v, want %v", fromPreview, m.mode, startMode)
		}
	}
}

func TestEnterInJobPreviewStillRuns(t *testing.T) {
	m, executed, _ := jobTestModel(t)
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = press(t, m, runes("y"))
	if len(*executed) != 1 || m.mode != ViewPreview {
		t.Errorf("executed = %v mode = %v", *executed, m.mode)
	}
}

func TestDOnIdleJobDryRunsOnlyThatJob(t *testing.T) {
	for _, fromPreview := range []bool{false, true} {
		m, executed, dryRun := jobTestModel(t)
		m.cfg.Jobs = append(m.cfg.Jobs, config.JobSpec{Name: "weekly", Command: "true", DryRunCommand: "true"})
		m.contentVersion++
		selectNavItem(t, &m, "job:nightly")
		if fromPreview {
			m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
		}
		m = press(t, m, runes("d"))
		if len(*dryRun) != 1 || (*dryRun)[0] != "nightly" || len(*executed) != 0 || m.mode == ViewDeleteConfirm {
			t.Errorf("preview %v: dryRun = %v executed = %v mode = %v", fromPreview, *dryRun, *executed, m.mode)
		}
	}
}

func TestROnRunningJobOrRunRowsDoesNothing(t *testing.T) {
	m, executed, _ := jobTestModel(t)
	markJobRunning(t, &m, "nightly")
	m = press(t, m, runes("r"))
	if m.mode != ViewDashboard || len(*executed) != 0 {
		t.Errorf("running job: mode = %v executed = %v", m.mode, *executed)
	}
	automationJob := runningJobsModel(t)
	selectAutomationJob(t, &automationJob)
	if hint := hintText(automationJob); strings.Contains(hint, "(r)") || !strings.Contains(hint, "(d)stop") {
		t.Errorf("automation run hint = %q", hint)
	}
	if next := press(t, automationJob, runes("r")); next.mode != ViewDashboard {
		t.Errorf("r on automation run changed mode to %v", next.mode)
	}
}

func TestDOnRunningJobAsksBeforeStopping(t *testing.T) {
	m, _, _ := jobTestModel(t)
	markJobRunning(t, &m, "nightly")
	m = press(t, m, runes("d"))
	if m.mode != ViewDeleteConfirm || m.jobToAbort != "nightly" {
		t.Errorf("mode = %v abort = %q", m.mode, m.jobToAbort)
	}
}

func TestBragStopAsksButDismissDoesNot(t *testing.T) {
	m, _ := bragTestModel(t, wednesday())
	writeBragState(t, m, "2026-W40", "brag.pid", strconv.Itoa(os.Getpid()))
	m.refreshBragRuns()
	selectNavItem(t, &m, "brag:2026-W40")
	var stopped []string
	previousStop := stopBragRun
	stopBragRun = func(root, id string) error {
		stopped = append(stopped, id)
		return nil
	}
	t.Cleanup(func() { stopBragRun = previousStop })
	if hint := hintText(m); !strings.Contains(hint, "(d)stop") {
		t.Errorf("running brag hint = %q", hint)
	}
	m = press(t, m, runes("d"))
	if m.mode != ViewDeleteConfirm || len(stopped) != 0 || !strings.Contains(stripANSI(m.View()), "stop 'brag 2026-W40'") {
		t.Fatalf("d should ask before stopping, mode = %v stopped = %v", m.mode, stopped)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != ViewDashboard || len(stopped) != 0 {
		t.Fatalf("esc should cancel, mode = %v stopped = %v", m.mode, stopped)
	}
	m = press(t, m, runes("d"))
	m = press(t, m, runes("y"))
	if len(stopped) != 1 || stopped[0] != "2026-W40" {
		t.Errorf("stopped = %v", stopped)
	}

	finished, _ := bragTestModel(t, wednesday())
	writeBragState(t, finished, "2026-W40", "meta.json", `{"id":"2026-W40"}`)
	writeBragState(t, finished, "2026-W40", "brag.exit", "1")
	finished.refreshBragRuns()
	selectNavItem(t, &finished, "brag:2026-W40")
	if hint := hintText(finished); !strings.Contains(hint, "(d)dismiss") {
		t.Errorf("finished brag hint = %q", hint)
	}
	if next := press(t, finished, runes("d")); next.mode == ViewDeleteConfirm {
		t.Error("dismiss should not ask")
	}
}

func TestSpaceAndActionsWorkInNotePreview(t *testing.T) {
	m, _ := automationTestModel(t)
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m, _ = pressKey(t, m, " ")
	if note := m.noteByID("note-1"); note == nil || note.Status != model.StatusDone {
		t.Fatalf("space in preview should mark the note done: %+v", note)
	}
	m, _ = pressKey(t, m, " ")
	m = press(t, m, runes("@"))
	if m.mode != ViewActionMenu {
		t.Fatalf("@ in preview should open actions, mode = %v", m.mode)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != ViewPreview {
		t.Errorf("closing the menu should return to the preview, mode = %v", m.mode)
	}
}

func TestPRRowOffersNvimAndStopLabel(t *testing.T) {
	m := reviewTestModel(t)
	m.cfg.ShowKeyHints = true
	if hint := hintText(m); !strings.Contains(hint, "(d)reject") || !strings.Contains(hint, "(o)nvim") {
		t.Errorf("idle PR hint = %q", hint)
	}
	markRunning(t, m, m.currentPRItem().PR.Ref)
	if hint := hintText(m); !strings.Contains(hint, "(d)stop") {
		t.Errorf("reviewing PR hint = %q", hint)
	}
	if binding, found := m.resolveKey(runes("o")); !found || binding.action != actionOpenClone {
		t.Errorf("o on the PR row resolves to %v", binding.action)
	}
}

func TestMyPRRowHintsOpen(t *testing.T) {
	m := syncTestModel(t)
	m.cfg.ShowKeyHints = true
	m.myPRs = []review.QueuedPR{myOpenPR("console", 4, "fix/a", "SUCCESS")}
	m.contentVersion++
	for index, item := range m.allNavItems() {
		if item.Kind == KindMyPR {
			m.selected = index
			if hint := hintText(m); !strings.Contains(hint, "(↵)open") {
				t.Errorf("my PR hint = %q", hint)
			}
			return
		}
	}
	t.Fatal("no my PR row")
}
