package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

func TestWizardDoctorStepYesInstallsNotificationsAndFinishes(t *testing.T) {
	m, path, installs := setupModel(t, "")
	m = walkWizardToTheLastStep(t, m)
	if m.setup.step != setupStepDoctor {
		t.Fatalf("step = %v", m.setup.step)
	}
	m, _ = pressKey(t, m, "y")
	if *installs != 1 || m.mode == ViewSetup || m.setup != nil {
		t.Fatalf("installs %d mode %v", *installs, m.mode)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config should be written: %v", err)
	}
}

func TestWizardIgnoresKeysThatDoNotApplyToTheStep(t *testing.T) {
	m, _, _ := setupModel(t, "")
	m.setup.step = setupStepRoots
	for name, action := range map[string]func(tea.KeyMsg) (tea.Model, tea.Cmd){
		"yes":    m.setupAnswerYes,
		"switch": m.setupSwitchTime,
	} {
		if next, _ := action(tea.KeyMsg{}); next.(Model).setup.step != setupStepRoots {
			t.Fatalf("%s should not move the wizard", name)
		}
	}
	m.setup.step = setupStepGit
	if next, _ := m.setupConfirm(tea.KeyMsg{}); next.(Model).setup.step != setupStepGit {
		t.Fatal("enter on the git question does nothing")
	}
}

func TestWizardDefaultsEmptyRootsAndNeedsAWorkDay(t *testing.T) {
	m, _, _ := setupModel(t, "")
	m, _ = pressKey(t, m, "y")
	m.setupInput.SetValue("  ")
	m, _ = pressKey(t, m, "enter")
	if strings.Join(m.setup.answers.RepositoryRoots, ",") != "~/Projects" || m.setup.step != setupStepWorkDays {
		t.Fatalf("roots %v step %v", m.setup.answers.RepositoryRoots, m.setup.step)
	}
	m.setup.answers.WorkDays = nil
	m, _ = pressKey(t, m, "enter")
	if m.setup.notice != "Pick at least one day 🙂" || m.setup.step != setupStepWorkDays {
		t.Fatalf("notice %q step %v", m.setup.notice, m.setup.step)
	}
	m, _ = pressKey(t, m, " ")
	if len(m.setup.answers.WorkDays) != 1 || m.setup.notice != "" {
		t.Fatalf("space should add the day under the cursor: %v", m.setup.answers.WorkDays)
	}
}

func TestWizardTimesStepShowsTheEveningInput(t *testing.T) {
	m, _, _ := setupModel(t, "")
	m.setup.step = setupStepTimes
	m.setupInput.SetValue("09:00")
	m, _ = pressKey(t, m, "tab")
	if !m.setup.editingEvening {
		t.Fatal("tab should switch to the evening time")
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "☀️  morning 09:00") || !strings.Contains(view, "🌙 evening ›") {
		t.Fatalf("view:\n%s", view)
	}
}

func TestWizardSaveErrorsStayInTheWizard(t *testing.T) {
	m, path, _ := setupModel(t, "digest_root: /mine\n")
	if err := os.Mkdir(path+".bak", 0755); err != nil {
		t.Fatal(err)
	}
	m = walkWizardToTheLastStep(t, m)
	m, _ = pressKey(t, m, "enter")
	if m.mode != ViewSetup || !strings.HasPrefix(m.setup.notice, "Couldn't save the config: ") {
		t.Fatalf("mode %v notice %q", m.mode, m.setup.notice)
	}
}

func TestWritingTheConfigReportsAnUnwritableFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })
	m, _, _ := setupModel(t, "")
	if _, backedUp, err := writeSetupConfig(path, m.setup.answers, true); err == nil || !backedUp {
		t.Fatalf("err %v backedUp %v", err, backedUp)
	}
}

func TestSettingsEditTheEveningTimeAndToggleHints(t *testing.T) {
	m, _, _ := setupFormModel(t, "")
	m = m.focusSetupField(setupFieldEvening)
	m, _ = pressKey(t, m, "tab")
	if m.setupInput.Value() != m.setup.answers.Evening || !m.setup.editing {
		t.Fatalf("editing the evening should start from its value, got %q", m.setupInput.Value())
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "tab done · enter save · esc undo") {
		t.Fatalf("the editing hint should show:\n%s", view)
	}
	m.setupInput.SetValue("19:15")
	m, _ = pressKey(t, m, "tab")
	if m.setup.answers.Evening != "19:15" {
		t.Fatalf("evening = %q", m.setup.answers.Evening)
	}
	m = m.focusSetupField(setupFieldHints)
	hintsWereOn := m.setup.answers.ShowKeyHints
	if m, _ = pressKey(t, m, " "); m.setup.answers.ShowKeyHints == hintsWereOn {
		t.Fatal("space should toggle key hints")
	}
}

func TestSettingsShowAnEmptyTimeAsOff(t *testing.T) {
	m, _, _ := setupFormModel(t, "")
	m.setup.answers.Morning = ""
	if view := stripANSI(m.View()); !strings.Contains(view, "Morning") || !strings.Contains(view, "off") {
		t.Fatalf("view:\n%s", view)
	}
}

func TestSettingsSaveNeedsAWorkDayAndDefaultsRoots(t *testing.T) {
	m, _, _ := setupFormModel(t, "")
	m.setup.answers.WorkDays = nil
	m, _ = pressKey(t, m, "enter")
	if m.setup.field != setupFieldWorkDays || m.setup.notice != "Pick at least one day 🙂" || m.setup.saving {
		t.Fatalf("field %v notice %q saving %v", m.setup.field, m.setup.notice, m.setup.saving)
	}
	m.setup.answers.WorkDays = []string{"mon"}
	m.setup.answers.RepositoryRoots = nil
	m, _ = pressKey(t, m, "enter")
	if strings.Join(m.setup.answers.RepositoryRoots, ",") != "~/Projects" || !m.setup.saving {
		t.Fatalf("roots %v saving %v", m.setup.answers.RepositoryRoots, m.setup.saving)
	}
	if _, cmd := m.tickSetupSpinner(spinner.TickMsg{ID: m.setup.spinner.ID()}); cmd == nil {
		t.Fatal("the spinner should keep ticking while saving")
	}
}

func TestSetupSavedAfterSetupClosedIsIgnored(t *testing.T) {
	m := syncTestModel(t)
	m.setup = nil
	if next, cmd := m.applySetupSaved(setupSavedMsg{}); cmd != nil || next.(Model).setup != nil {
		t.Fatal("a late save result should be ignored")
	}
}

func TestDoctorStepOnlyYesFinishesTheWizard(t *testing.T) {
	m, _, installs := setupModel(t, "")
	m.setup.step = setupStepDoctor
	if next, _ := m.setupYesNo(false); next.(Model).setup == nil || next.(Model).setup.step != setupStepDoctor || *installs != 0 {
		t.Fatalf("no should not skip the tool check: installs %d", *installs)
	}
	if next, _ := m.setupYesNo(true); next.(Model).setup != nil || *installs != 1 {
		t.Fatalf("yes should finish the wizard: installs %d", *installs)
	}
}
