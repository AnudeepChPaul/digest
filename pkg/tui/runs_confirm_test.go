package tui

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/achandrapaul/digest/pkg/automation"
	"github.com/achandrapaul/digest/pkg/brag"
	"github.com/achandrapaul/digest/pkg/config"
	"github.com/achandrapaul/digest/pkg/model"

	tea "github.com/charmbracelet/bubbletea"
)

func stubDryRunStop(t *testing.T) *[]string {
	t.Helper()
	var stopped []string
	previous := stopJobDryRun
	stopJobDryRun = func(jobName string) error {
		stopped = append(stopped, jobName)
		return os.Remove(dryRunFilePath(jobName, "pid"))
	}
	t.Cleanup(func() { stopJobDryRun = previous })
	return &stopped
}

func exitedPID(t *testing.T) int {
	t.Helper()
	exited := exec.Command("true")
	if err := exited.Run(); err != nil {
		t.Fatal(err)
	}
	return exited.Process.Pid
}

func confirmView(m Model) string {
	return strings.Join(strings.Fields(stripANSI(m.View())), " ")
}

func TestRunWhileTheDryRunIsInFlightStopsItAfterConfirm(t *testing.T) {
	for _, start := range []struct {
		name        string
		fromPreview bool
		key         tea.KeyMsg
	}{{"row r", false, runes("r")}, {"preview r", true, runes("r")}, {"preview enter", true, tea.KeyMsg{Type: tea.KeyEnter}}} {
		m, executed, _ := jobTestModel(t)
		stopped := stubDryRunStop(t)
		markDryRunInFlight(t, &m, "nightly")
		selectNavItem(t, &m, "job:nightly")
		if start.fromPreview {
			m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
		}
		m = press(t, m, start.key)
		if m.mode != ViewDeleteConfirm || !strings.Contains(confirmView(m), "Dry run in progress. Stop it and run the job?") || len(*executed) != 0 || len(*stopped) != 0 {
			t.Fatalf("%s: mode = %v executed = %v stopped = %v view:\n%s", start.name, m.mode, *executed, *stopped, confirmView(m))
		}
		m = press(t, m, runes("y"))
		if len(*stopped) != 1 || (*stopped)[0] != "nightly" || len(*executed) != 1 || (*executed)[0] != "nightly" {
			t.Errorf("%s: stopped = %v executed = %v", start.name, *stopped, *executed)
		}
		if m.dryRunsInFlight["nightly"] {
			t.Errorf("%s: the dry run still shows as in flight", start.name)
		}
	}
}

func TestDOnAStaleRunningJobClearsItsPIDAfterConfirm(t *testing.T) {
	for _, fromPreview := range []bool{false, true} {
		m, executed, dryRun := jobTestModel(t)
		pidPath := filepath.Join(getLogsDir(), "nightly.pid")
		pid := exitedPID(t)
		if err := os.WriteFile(pidPath, []byte(strconv.Itoa(pid)), 0644); err != nil {
			t.Fatal(err)
		}
		if fromPreview {
			m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
		}
		m.runningJobPIDs = map[string]int{"nightly": pid}
		m.contentVersion++
		m = press(t, m, runes("d"))
		if m.mode != ViewDeleteConfirm || !strings.Contains(confirmView(m), "no longer running") {
			t.Fatalf("preview %v: d on a stale job should ask, mode = %v view:\n%s", fromPreview, m.mode, confirmView(m))
		}
		m = press(t, m, runes("y"))
		if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
			t.Errorf("preview %v: stale pid file kept, err = %v", fromPreview, err)
		}
		if m.jobRunning("nightly") || len(*executed) != 0 || len(*dryRun) != 0 {
			t.Errorf("preview %v: running = %v executed = %v dryRun = %v", fromPreview, m.jobRunning("nightly"), *executed, *dryRun)
		}
	}
}

func TestJobPreviewBindsEnterOnlyWhileTheJobIsIdle(t *testing.T) {
	m, _, _ := jobTestModel(t)
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if binding, found := m.resolveKey(tea.KeyMsg{Type: tea.KeyEnter}); !found || binding.action != actionPreviewEnter {
		t.Errorf("idle job preview should bind enter, found %v action %v", found, binding.action)
	}
	markJobRunning(t, &m, "nightly")
	if binding, found := m.resolveKey(tea.KeyMsg{Type: tea.KeyEnter}); found {
		t.Errorf("running job preview binds enter to %v", binding.action)
	}
}

func TestCopyWithNothingToCopySaysNothing(t *testing.T) {
	copied := captureClipboard(t)
	m, _, _ := jobTestModel(t)
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m.messages = nil
	m = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlY})
	if len(*copied) != 0 || len(m.messages) != 0 {
		t.Errorf("a job without output copied %q and posted %+v", *copied, m.messages)
	}
	m.copyText("")
	if len(m.messages) != 0 {
		t.Errorf("copying empty text posted %+v", m.messages)
	}
}

func TestRunJobSpawnErrorShowsInTheConfirmAndPreview(t *testing.T) {
	for _, fromPreview := range []bool{false, true} {
		m, _, _ := jobTestModel(t)
		executeJobBackground = func(cfg *config.Config, jobName string) error { return errors.New("sh: not found") }
		if fromPreview {
			m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
		}
		m = press(t, m, runes("r"))
		m = press(t, m, runes("y"))
		if m.mode != ViewDeleteConfirm || !strings.Contains(confirmView(m), "sh: not found") {
			t.Fatalf("preview %v: the confirm should show the error, mode = %v view:\n%s", fromPreview, m.mode, confirmView(m))
		}
		if !strings.Contains(headerTopRow(m), "sh: not found") {
			t.Errorf("preview %v: header = %q", fromPreview, headerTopRow(m))
		}
		m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
		if fromPreview && (m.mode != ViewPreview || !strings.Contains(confirmView(m), "sh: not found")) {
			t.Errorf("the preview should keep the error, mode = %v view:\n%s", m.mode, confirmView(m))
		}
	}
}

func runningBragRunsModel(t *testing.T) (Model, *[]string) {
	t.Helper()
	m, _ := bragTestModel(t, wednesday())
	for _, id := range []string{"2026-W40", "2026-W39"} {
		writeBragState(t, m, id, "meta.json", `{"id":"`+id+`"}`)
		writeBragState(t, m, id, "brag.pid", strconv.Itoa(os.Getpid()))
	}
	var stopped []string
	previous := stopBragRun
	stopBragRun = func(root, id string) error {
		stopped = append(stopped, id)
		if err := os.Remove(filepath.Join(brag.StateDir(root, id), "brag.pid")); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(brag.StateDir(root, id), "brag.exit"), []byte("143"), 0644)
	}
	t.Cleanup(func() { stopBragRun = previous })
	m.refreshBragRuns()
	m.contentVersion++
	selectNavItem(t, &m, "brag:2026-W40")
	return m, &stopped
}

func TestStoppingABragStopsEveryRunningBragAndKeepsThemAsFailed(t *testing.T) {
	for _, fromPreview := range []bool{false, true} {
		m, stopped := runningBragRunsModel(t)
		startMode := ViewDashboard
		if fromPreview {
			m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
			startMode = ViewPreview
		}
		m = press(t, m, runes("d"))
		m = press(t, m, runes("y"))
		if len(*stopped) != 2 {
			t.Errorf("preview %v: stopped = %v, want both brag runs", fromPreview, *stopped)
		}
		if len(m.bragRuns) != 2 || m.bragRuns[0].Status != brag.RunFailed || m.bragRuns[1].Status != brag.RunFailed {
			t.Errorf("preview %v: runs = %+v, want both listed as failed", fromPreview, m.bragRuns)
		}
		if key, _ := m.selectedNavKey(); m.mode != startMode || key != "brag:2026-W40" {
			t.Errorf("preview %v: mode = %v selected = %q", fromPreview, m.mode, key)
		}
		if fromPreview && !strings.Contains(confirmView(m), "FAILED") {
			t.Errorf("the preview should show the failed run:\n%s", confirmView(m))
		}
	}
}

func TestRRetriesAFailedRunAfterConfirm(t *testing.T) {
	for _, fromPreview := range []bool{false, true} {
		started, _ := stubReviewControl(t)
		reviews, _, failed := reviewRunsModel(t)
		selectNavItem(t, &reviews, "review:"+failed.URL)
		reviews = retryAfterConfirm(t, reviews, fromPreview)
		if len(*started) != 1 || (*started)[0] != failed.URL {
			t.Errorf("preview %v: review started = %v", fromPreview, *started)
		}

		brags, bragsStarted := bragTestModel(t, wednesday())
		writeBragState(t, brags, "2026-W40", "meta.json", `{"id":"2026-W40"}`)
		writeBragState(t, brags, "2026-W40", "brag.exit", "1")
		brags.refreshBragRuns()
		brags.contentVersion++
		selectNavItem(t, &brags, "brag:2026-W40")
		brags = retryAfterConfirm(t, brags, fromPreview)
		if len(*bragsStarted) != 1 || (*bragsStarted)[0] != (startedBrag{"2026-W40", false}) {
			t.Errorf("preview %v: brag started = %+v", fromPreview, *bragsStarted)
		}

		automations := failedJobsModel(t, automation.RunFailed)
		var automationsStarted []startedAutomation
		startAutomation = func(root, noteID, name string, phase automation.Phase) error {
			automationsStarted = append(automationsStarted, startedAutomation{root, noteID, name, phase})
			return nil
		}
		selectAutomationJob(t, &automations)
		automations = retryAfterConfirm(t, automations, fromPreview)
		if len(automationsStarted) != 1 || automationsStarted[0].noteID != "note-1" || automationsStarted[0].phase != automation.PhaseCreate {
			t.Errorf("preview %v: automation started = %+v", fromPreview, automationsStarted)
		}
	}
}

func retryAfterConfirm(t *testing.T, m Model, fromPreview bool) Model {
	t.Helper()
	m.cfg.ShowKeyHints = true
	if hint := hintText(m); !strings.Contains(hint, "(r)retry") {
		t.Errorf("failed run hint = %q", hint)
	}
	startMode := ViewDashboard
	if fromPreview {
		m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
		startMode = ViewPreview
		if binding, found := m.resolveKey(tea.KeyMsg{Type: tea.KeyEnter}); found {
			t.Errorf("run preview binds enter to %v", binding.action)
		}
	}
	m = press(t, m, runes("r"))
	if m.mode != ViewDeleteConfirm || !strings.Contains(confirmView(m), "Run again?") {
		t.Fatalf("preview %v: r should ask first, mode = %v view:\n%s", fromPreview, m.mode, confirmView(m))
	}
	m = press(t, m, runes("y"))
	if m.mode != startMode {
		t.Errorf("preview %v: returned to %v", fromPreview, m.mode)
	}
	return m
}

func TestPreviewNextOntoAGitRepoShowsItsDetails(t *testing.T) {
	m := gitStripTestModel(t)
	for index, item := range m.allNavItems() {
		if item.Kind == KindGitRepo {
			m.selected = index - 1
			break
		}
	}
	m.mode = ViewPreview
	m.updatePreviewViewport()
	m = press(t, m, runes("n"))
	if m.mode != ViewGitDetails || m.git.gitPopupRepo == nil || m.git.gitPopupRepo.Name != selectedRepoName(m) {
		t.Errorf("mode = %v repo = %+v", m.mode, m.git.gitPopupRepo)
	}
}

func TestPreviewNextOntoARunningJobFollowsItsLog(t *testing.T) {
	m, _, _ := jobTestModel(t)
	m.cfg.Jobs = append(m.cfg.Jobs, config.JobSpec{Name: "weekly", Command: "true"})
	m.contentVersion++
	markJobRunning(t, &m, "weekly")
	selectNavItem(t, &m, "job:nightly")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m.jobLogRunning, m.jobLogStamp = false, ""
	next, cmd := m.Update(runes("n"))
	m = next.(Model)
	if key, _ := m.selectedNavKey(); key != "job:weekly" || !m.jobLogRunning || m.jobLogStamp == "" || cmd == nil {
		t.Errorf("selected = %q logRunning = %v stamp = %q cmd = %v", key, m.jobLogRunning, m.jobLogStamp, cmd != nil)
	}
}

func TestGitDetailsPAndNMoveToTheNeighbouringItems(t *testing.T) {
	m := gitStripTestModel(t)
	var repoIndexes []int
	for index, item := range m.allNavItems() {
		if item.Kind == KindGitRepo {
			repoIndexes = append(repoIndexes, index)
		}
	}
	m.selected = repoIndexes[0]
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.mode != ViewGitDetails {
		t.Fatalf("mode = %v", m.mode)
	}
	first := m.git.gitPopupRepo.Name
	m = press(t, m, runes("n"))
	if m.selected != repoIndexes[0]+1 || m.mode != ViewGitDetails || m.git.gitPopupRepo.Name == first {
		t.Errorf("n: selected = %d mode = %v repo = %s", m.selected, m.mode, m.git.gitPopupRepo.Name)
	}
	m = press(t, m, runes("p"))
	if m.selected != repoIndexes[0] || m.git.gitPopupRepo.Name != first {
		t.Errorf("p: selected = %d repo = %s", m.selected, m.git.gitPopupRepo.Name)
	}
}

func TestDraftTabActsOnTheNoteLikeTheDetailsTab(t *testing.T) {
	m, started := automationTestModel(t)
	m = withDraft(t, m, automation.RunFailed, automation.PhaseCreate)
	m = openDraftTab(t, m)
	if binding, found := m.resolveKey(runes("x")); found {
		t.Errorf("x is bound on the draft tab to %v", binding.action)
	}
	m.updatePreviewViewport()
	if view := confirmView(m); strings.Contains(view, "x to draft again") || strings.Contains(view, "Press x") {
		t.Errorf("draft tab still names x:\n%s", view)
	}
	details := m
	details.previewTab = previewTabDetails
	for _, key := range []tea.KeyMsg{{Type: tea.KeySpace, Runes: []rune(" ")}, runes("@"), runes(".")} {
		binding, found := m.resolveKey(key)
		want, _ := details.resolveKey(key)
		if !found || binding.action != want.action {
			t.Errorf("%q on the draft tab = %v, details tab = %v", key.String(), binding.action, want.action)
		}
	}
	m, _ = pressKey(t, m, " ")
	if note := m.noteByID("note-1"); note == nil || note.Status != model.StatusDone {
		t.Errorf("space on the draft tab should mark the note done: %+v", note)
	}
	if len(*started) != 0 {
		t.Errorf("started = %+v", *started)
	}
}

func TestDraftTabDeletesTheDraftOnlyAfterConfirm(t *testing.T) {
	m, _ := automationTestModel(t)
	m = withDraft(t, m, automation.RunDraftReady, automation.PhaseDraft)
	m = openDraftTab(t, m)
	m = press(t, m, runes("d"))
	if m.mode != ViewDeleteConfirm {
		t.Fatalf("d should ask first, mode = %v", m.mode)
	}
	if _, err := automation.LoadDraft(m.cfg.AutomationDir(), "note-1"); err != nil {
		t.Fatalf("the draft went before the confirm: %v", err)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != ViewPreview || m.previewTab != previewTabDraft {
		t.Fatalf("esc should return to the draft tab, mode = %v tab = %v", m.mode, m.previewTab)
	}
	m = press(t, m, runes("d"))
	m = press(t, m, runes("y"))
	if _, err := automation.LoadDraft(m.cfg.AutomationDir(), "note-1"); err == nil {
		t.Error("the draft should be deleted after the confirm")
	}
	if m.mode != ViewPreview || m.previewTab != previewTabDetails {
		t.Errorf("mode = %v tab = %v", m.mode, m.previewTab)
	}
}

func TestDraftTabCopyWithoutADraftCopiesNothing(t *testing.T) {
	copied := captureClipboard(t)
	m, _ := automationTestModel(t)
	m.automationRuns = map[string]automation.Run{"note-1": {Meta: automation.RunMeta{NoteID: "note-1", Automation: "jira", Phase: automation.PhaseDraft}, Status: automation.RunFailed}}
	m = openDraftTab(t, m)
	m.messages = nil
	m = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlY})
	if len(*copied) != 0 || len(m.messages) != 0 {
		t.Errorf("copied %q messages %+v", *copied, m.messages)
	}
}

func TestRejectBoxCopiesItsText(t *testing.T) {
	copied := captureClipboard(t)
	m := rejectCommentModel(t)
	m = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlY})
	if len(*copied) != 0 {
		t.Errorf("an empty reject box copied %q", *copied)
	}
	m.rejectInput.SetValue("please add tests")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlY})
	if len(*copied) != 1 || (*copied)[0] != "please add tests" || m.mode != ViewRejectComment {
		t.Errorf("copied %q mode %v", *copied, m.mode)
	}
}

func TestEditorsAskBeforeDiscardingChanges(t *testing.T) {
	editors := map[string]func(t *testing.T) Model{
		"note": func(t *testing.T) Model {
			m, _ := automationTestModel(t)
			return press(t, press(t, m, tea.KeyMsg{Type: tea.KeyTab}), tea.KeyMsg{Type: tea.KeyEnter})
		},
		"brag": func(t *testing.T) Model { return press(t, savedBragModel(t), tea.KeyMsg{Type: tea.KeyEnter}) },
		"automation draft": func(t *testing.T) Model {
			m, _ := automationTestModel(t)
			m = openDraftTab(t, withDraft(t, m, automation.RunDraftReady, automation.PhaseDraft))
			return press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
		},
	}
	for name, open := range editors {
		m := open(t)
		editorMode := m.mode
		returnMode := press(t, m, tea.KeyMsg{Type: tea.KeyEsc}).mode
		if returnMode == editorMode || returnMode == ViewDeleteConfirm {
			t.Errorf("%s: esc without changes should leave at once, mode = %v", name, returnMode)
		}
		m.editor.SetValue(m.editor.Value() + "\nunsaved line")
		m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
		if m.mode != ViewDeleteConfirm || !strings.Contains(confirmView(m), "Discard changes?") {
			t.Fatalf("%s: esc with changes should ask, mode = %v", name, m.mode)
		}
		m = press(t, m, runes("n"))
		if m.mode != editorMode || !strings.Contains(m.editor.Value(), "unsaved line") {
			t.Fatalf("%s: n should keep editing, mode = %v", name, m.mode)
		}
		m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
		m = press(t, m, runes("y"))
		if m.mode != returnMode {
			t.Errorf("%s: y should discard and leave to %v, mode = %v", name, returnMode, m.mode)
		}
	}
}

func TestAutomationDraftEditorCopiesItsText(t *testing.T) {
	copied := captureClipboard(t)
	m, _ := automationTestModel(t)
	m = openDraftTab(t, withDraft(t, m, automation.RunDraftReady, automation.PhaseDraft))
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlY})
	if len(*copied) != 1 || (*copied)[0] != strings.TrimSpace(testDraft) || m.mode != ViewAutomationEdit {
		t.Fatalf("copied %q mode %v", *copied, m.mode)
	}
	m.editor.SetValue("")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlY})
	if len(*copied) != 1 {
		t.Errorf("an empty editor copied %q", *copied)
	}
}

func TestAutomationStartFailureShowsInTheHeaderAndPreviewUntilASuccess(t *testing.T) {
	m, started := automationTestModel(t)
	m = withDraft(t, m, automation.RunFailed, automation.PhaseCreate)
	m = openDraftTab(t, m)
	startAutomation = func(root, noteID, name string, phase automation.Phase) error { return errors.New("claude not found") }
	m = press(t, m, runes("r"))
	m = press(t, m, runes("y"))
	if m.mode != ViewPreview || !strings.Contains(confirmView(m), "claude not found") {
		t.Errorf("the preview should show the failure, mode = %v view:\n%s", m.mode, confirmView(m))
	}
	if header := headerTopRow(m); !strings.Contains(header, "automation error") || !strings.Contains(header, "claude not found") {
		t.Errorf("header = %q", header)
	}
	startAutomation = func(root, noteID, name string, phase automation.Phase) error {
		*started = append(*started, startedAutomation{root, noteID, name, phase})
		return nil
	}
	m = press(t, m, runes("r"))
	m = press(t, m, runes("y"))
	if len(*started) != 1 || strings.Contains(confirmView(m), "claude not found") || strings.Contains(headerTopRow(m), "claude not found") {
		t.Errorf("a successful start should clear the failure, started = %+v header = %q", *started, headerTopRow(m))
	}
}

func TestAutomationDraftSaveFailureShowsInTheHeaderUntilASave(t *testing.T) {
	m, _ := automationTestModel(t)
	m = openDraftTab(t, withDraft(t, m, automation.RunDraftReady, automation.PhaseDraft))
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m.editor.SetValue("summary: [unterminated")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlO})
	if m.mode != ViewAutomationEdit || m.automationNotice == "" || !strings.Contains(headerTopRow(m), "automation error") {
		t.Fatalf("mode = %v notice = %q header = %q", m.mode, m.automationNotice, headerTopRow(m))
	}
	m.editor.SetValue(testDraft)
	m = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlO})
	if m.mode != ViewPreview || strings.Contains(headerTopRow(m), "automation error") || m.previewNotice != "" {
		t.Errorf("a save should clear the failure, mode = %v header = %q notice = %q", m.mode, headerTopRow(m), m.previewNotice)
	}
}

func TestUnreadableBragOffersAFreshBrag(t *testing.T) {
	m, started := bragTestModel(t, wednesday())
	week40 := brag.WeekOf(wednesday()).Previous()
	if err := os.MkdirAll(filepath.Dir(week40.Path(m.cfg.BragDir())), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(week40.Path(m.cfg.BragDir()), []byte("not a brag"), 0644); err != nil {
		t.Fatal(err)
	}
	m = press(t, m, runes("b"))
	m = selectBragRow(t, m, "Week 40 · 28 Sep – 04 Oct")
	if label := m.bragStateLabel(bragRow{period: week40}); label == "View your brag" {
		t.Errorf("an unreadable brag should not offer a view, label = %q", label)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != ViewBragConfirm || m.bragRegenerate || !strings.Contains(m.bragNotice, "missing frontmatter") {
		t.Fatalf("mode = %v regenerate = %v notice = %q", m.mode, m.bragRegenerate, m.bragNotice)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != ViewBragList || !strings.Contains(stripANSI(m.View()), "missing frontmatter") {
		t.Fatalf("the list should keep the load error, mode = %v", m.mode)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = press(t, m, runes("y"))
	if len(*started) != 1 || (*started)[0] != (startedBrag{week40.ID(), false}) {
		t.Errorf("started = %+v", *started)
	}
	if err := (&brag.Brag{Period: week40, Facts: "- shipped", Summary: "- Shipped"}).Save(m.cfg.BragDir()); err != nil {
		t.Fatal(err)
	}
	m.refreshBragRuns()
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != ViewBragView || m.bragNotice != "" {
		t.Errorf("a readable brag should open, mode = %v notice = %q", m.mode, m.bragNotice)
	}
}

func TestBOpensTheBragListOnTheUnbraggedWeekWithoutRefreshing(t *testing.T) {
	m, _ := bragTestModel(t, wednesday())
	m.appState.FirstNoteCreated, m.appStateKnown = m.notes[0].Created, true
	if notice := m.unbraggedWeekNotice(); notice == "" {
		t.Fatal("expected the unbragged week notice")
	}
	m.bragSelected = 0
	next, cmd := m.Update(runes("b"))
	m = next.(Model)
	row, ok := m.selectedBragRow()
	if m.mode != ViewBragList || !ok || row.title() != "Week 40 · 28 Sep – 04 Oct" {
		t.Errorf("mode = %v row = %q", m.mode, row.title())
	}
	if cmd != nil {
		t.Error("opening the brag list should not refresh anything")
	}
}
