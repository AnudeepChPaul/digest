package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/achandrapaul/digest/pkg/config"
	"github.com/achandrapaul/digest/pkg/doctor"

	tea "github.com/charmbracelet/bubbletea"
)

func setupModel(t *testing.T, existing string) (Model, string, *int) {
	t.Helper()
	t.Setenv("__CFBundleIdentifier", "com.example.term")
	path := filepath.Join(t.TempDir(), "digest", "config.yaml")
	if existing != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(existing), 0644); err != nil {
			t.Fatal(err)
		}
	}
	installs := 0
	previousInstall, previousDoctor, previousInstalled, previousUninstall := installNotifications, runDoctor, notificationsInstalled, uninstallNotifications
	notificationsInstalled = func() bool { return false }
	uninstallNotifications = func() error { t.Error("unexpected uninstall"); return nil }
	installNotifications = func(*config.Config) error {
		installs++
		return nil
	}
	runDoctor = func(*config.Config) []doctor.Result {
		return []doctor.Result{{Name: "git", Found: true, Required: true, Path: "/bin/git"}, {Name: "gh", Required: true}}
	}
	t.Cleanup(func() {
		installNotifications, runDoctor, notificationsInstalled, uninstallNotifications = previousInstall, previousDoctor, previousInstalled, previousUninstall
	})
	m := syncTestModel(t)
	m = m.startSetup(path)
	return m, path, &installs
}

func TestSetupWalksThroughEveryQuestionAndWritesTheConfig(t *testing.T) {
	m, path, installs := setupModel(t, "")
	if m.mode != ViewSetup || !strings.Contains(m.View(), "Let's get digest ready") {
		t.Fatalf("setup should greet the user:\n%s", m.View())
	}
	m, _ = pressKey(t, m, "y")
	if !strings.Contains(m.View(), "~/Projects") {
		t.Errorf("roots step should suggest ~/Projects:\n%s", m.View())
	}
	m.setupInput.SetValue("~/code, ~/work")
	m, _ = pressKey(t, m, "enter")
	if view := m.View(); !strings.Contains(view, "Monday → Sunday") || !strings.Contains(view, "Mon") {
		t.Errorf("work days step should explain ISO weeks:\n%s", view)
	}
	m, _ = pressKey(t, m, "right")
	m, _ = pressKey(t, m, " ")
	m, _ = pressKey(t, m, "enter")
	m.setupInput.SetValue("08:15")
	m, _ = pressKey(t, m, "tab")
	m.setupInput.SetValue("")
	m, _ = pressKey(t, m, "enter")
	if !strings.Contains(m.View(), "key hints") {
		t.Errorf("hints step:\n%s", m.View())
	}
	m, _ = pressKey(t, m, "y")
	if view := m.View(); !strings.Contains(view, "✓ git") || !strings.Contains(view, "✗ gh") {
		t.Errorf("doctor step should list tools:\n%s", view)
	}
	m, _ = pressKey(t, m, "enter")
	if *installs != 1 || m.mode != ViewDashboard {
		t.Errorf("enter should install notifications and open the dashboard: installs %d mode %v", *installs, m.mode)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.GitEnabled() || strings.Join(cfg.GitRepositoryRoots, ",") != "~/code,~/work" || strings.Join(cfg.WorkDays, ",") != "mon,wed,thu,fri" ||
		cfg.DigestNotifications.Morning != "08:15" || cfg.DigestNotifications.Evening != "" || !cfg.ShowKeyHints || cfg.TerminalApp != "com.example.term" {
		t.Errorf("written config = %+v", cfg)
	}
}

func TestSetupRejectsABadTimeAndCannotSkipTheToolCheck(t *testing.T) {
	m, path, installs := setupModel(t, "digest_root: /mine\njobs: []\n")
	m, _ = pressKey(t, m, "n")
	m, _ = pressKey(t, m, "enter")
	m.setupInput.SetValue("25:99")
	m, _ = pressKey(t, m, "enter")
	if !strings.Contains(m.View(), "HH:MM") {
		t.Errorf("a bad time should explain the format:\n%s", m.View())
	}
	m.setupInput.SetValue("09:00")
	m, _ = pressKey(t, m, "enter")
	m, _ = pressKey(t, m, "n")
	if m, _ = pressKey(t, m, "n"); *installs != 0 || m.mode != ViewSetup || m.setup.step != setupStepDoctor {
		t.Fatalf("n should not skip the tool check: installs %d mode %v", *installs, m.mode)
	}
	if view := stripANSI(m.View()); strings.Contains(view, "skip") {
		t.Errorf("the tool check should not offer a skip:\n%s", view)
	}
	if m, _ = pressKey(t, m, "enter"); *installs != 1 || m.mode != ViewDashboard {
		t.Errorf("enter should finish the wizard: installs %d mode %v", *installs, m.mode)
	}
	written, _ := os.ReadFile(path)
	backup, _ := os.ReadFile(path + ".bak")
	if !strings.Contains(string(written), "digest_root: /mine") || !strings.Contains(string(written), "show_git: false") || string(backup) != "digest_root: /mine\njobs: []\n" {
		t.Errorf("setup should keep other keys and back up:\n%s\nbackup %q", written, backup)
	}
}

func setupAnswerLines(m Model) []string {
	var answered []string
	for _, line := range plainLines(m.View()) {
		line = strings.Trim(line, "│ ")
		if strings.HasPrefix(line, "✓ ") && strings.Contains(line, "  ") {
			answered = append(answered, strings.Join(strings.Fields(line), " "))
		}
	}
	return answered
}

func assertSetupAnswers(t *testing.T, m Model, want ...string) {
	t.Helper()
	if got := setupAnswerLines(m); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("answers so far = %q, want %q", got, want)
	}
}

func TestSetupShowsTheAnswersSoFar(t *testing.T) {
	m, _, _ := setupModel(t, "")
	assertSetupAnswers(t, m)
	m, _ = pressKey(t, m, "y")
	assertSetupAnswers(t, m, "✓ git yes")
	m.setupInput.SetValue("~/code, ~/work")
	m, _ = pressKey(t, m, "enter")
	assertSetupAnswers(t, m, "✓ git yes", "✓ repos ~/code, ~/work")
	m, _ = pressKey(t, m, "right")
	m, _ = pressKey(t, m, " ")
	m, _ = pressKey(t, m, "enter")
	assertSetupAnswers(t, m, "✓ git yes", "✓ repos ~/code, ~/work", "✓ work days Mon Wed Thu Fri")
	m.setupInput.SetValue("08:15")
	m, _ = pressKey(t, m, "tab")
	m.setupInput.SetValue("")
	m, _ = pressKey(t, m, "enter")
	assertSetupAnswers(t, m, "✓ git yes", "✓ repos ~/code, ~/work", "✓ work days Mon Wed Thu Fri", "✓ summaries 08:15 · off")
	m, _ = pressKey(t, m, "y")
	assertSetupAnswers(t, m, "✓ git yes", "✓ repos ~/code, ~/work", "✓ work days Mon Wed Thu Fri", "✓ summaries 08:15 · off", "✓ key hints on")
}

func TestSetupWithoutGitSkipsTheReposAnswer(t *testing.T) {
	m, _, _ := setupModel(t, "")
	m, _ = pressKey(t, m, "n")
	assertSetupAnswers(t, m, "✓ git no")
}

func TestSetupTakesAPastedListOfManyRoots(t *testing.T) {
	m, _, _ := setupModel(t, "")
	m, _ = pressKey(t, m, "y")
	m.setupInput.SetValue("")
	var pastedLines, want []string
	for _, repo := range []string{"team/admin-api", "team/console-e2e-tests", "team/monorepo", "team/monkey-service", "acme/php-service", "acme/console-gateway", "acme/accounts-service", "acme/recovery-service"} {
		root := "~/Projects/git.example.com/" + repo + "/"
		want = append(want, root)
		pastedLines = append(pastedLines, "- "+root)
	}
	pasted := strings.Join(pastedLines, "\n")
	if len(pasted) < 300 {
		t.Fatalf("pasted list too short to prove the limit: %d", len(pasted))
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(pasted), Paste: true})
	m = next.(Model)
	m, _ = pressKey(t, m, "enter")
	if got := strings.Join(m.setup.answers.RepositoryRoots, ","); got != strings.Join(want, ",") {
		t.Errorf("roots = %q\nwant %q", got, strings.Join(want, ","))
	}
}

func TestSetupWorkDaysRenderSideBySide(t *testing.T) {
	m, _, _ := setupModel(t, "")
	m, _ = pressKey(t, m, "y")
	m, _ = pressKey(t, m, "enter")
	for _, line := range plainLines(m.View()) {
		if strings.Contains(line, "Mon") && !strings.Contains(line, "Monday") {
			for _, day := range []string{"Tue", "Wed", "Thu", "Fri", "Sat", "Sun"} {
				if !strings.Contains(line, day) {
					t.Errorf("%s should sit on the same line as Mon: %q", day, line)
				}
			}
			return
		}
	}
	t.Fatalf("no day chips:\n%s", m.View())
}

func TestEscClosesSetupOnEveryStepWithoutSaving(t *testing.T) {
	advance := [][]string{{}, {"y"}, {"y", "enter"}, {"y", "enter", "enter"}, {"y", "enter", "enter", "enter"}, {"y", "enter", "enter", "enter", "n"}}
	for step, keys := range advance {
		m, path, installs := setupModel(t, "digest_root: /mine\n")
		for _, key := range keys {
			m, _ = pressKey(t, m, key)
		}
		if m.mode != ViewSetup || m.setup.step != setupStep(step) {
			t.Fatalf("step %d: did not reach it, mode %v", step, m.mode)
		}
		if !strings.Contains(m.View(), "esc close") {
			t.Errorf("step %d: hint should mention esc close:\n%s", step, m.View())
		}
		m, _ = pressKey(t, m, "esc")
		if m.mode != ViewDashboard || *installs != 0 {
			t.Errorf("step %d: esc should close setup, mode %v installs %d", step, m.mode, *installs)
		}
		written, _ := os.ReadFile(path)
		if string(written) != "digest_root: /mine\n" {
			t.Errorf("step %d: esc should not save, config = %q", step, written)
		}
		if _, err := os.Stat(path + ".bak"); err == nil {
			t.Errorf("step %d: esc should not write a backup", step)
		}
	}
}

func TestCommaReopensSetupAfterFinishing(t *testing.T) {
	m, path, _ := setupModel(t, "")
	m.configPath = path
	for _, key := range []string{"n", "enter", "enter", "n", "enter"} {
		m, _ = pressKey(t, m, key)
	}
	if m.mode != ViewDashboard {
		t.Fatalf("setup should finish, mode %v", m.mode)
	}
	if m, _ = pressKey(t, m, ","); m.mode != ViewSetup {
		t.Errorf(", should reopen setup after finishing it, mode %v", m.mode)
	}
}

func setupFormModel(t *testing.T, existing string) (Model, string, *int) {
	t.Helper()
	m, path, installs := setupModel(t, existing)
	return openSetupForm(t, m, path), path, installs
}

func openSetupForm(t *testing.T, m Model, path string) Model {
	t.Helper()
	m, _ = pressKey(t, m, "esc")
	m.configPath = path
	m, _ = pressKey(t, m, ",")
	if m.mode != ViewSetup || !m.setup.form {
		t.Fatalf(", should open the setup form, mode %v", m.mode)
	}
	return m
}

func readConfigFile(t *testing.T, path string) *config.Config {
	t.Helper()
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestSetupFormShowsEveryField(t *testing.T) {
	m, _, _ := setupFormModel(t, "")
	view := stripANSI(m.View())
	for _, want := range []string{"Git", "Repos", "Work days", "Mon", "Sun", "Morning", "Evening", "Key hints", "Notifications", "✓ git", "✗ gh", "enter save", "esc close"} {
		if !strings.Contains(view, want) {
			t.Errorf("form should show %q:\n%s", want, view)
		}
	}
}

func TestSetupFormMovesPastTextFieldsWithJKAndArrows(t *testing.T) {
	m, _, _ := setupFormModel(t, "")
	moves := []struct {
		key  string
		want setupField
	}{{"j", setupFieldRoots}, {"x", setupFieldRoots}, {"j", setupFieldWorkDays}, {"k", setupFieldRoots}, {"down", setupFieldWorkDays}, {"up", setupFieldRoots}, {"up", setupFieldGit}}
	for _, move := range moves {
		if m, _ = pressKey(t, m, move.key); m.setup.field != move.want {
			t.Fatalf("after %q field = %d, want %d", move.key, m.setup.field, move.want)
		}
	}
	if strings.Join(m.setup.answers.RepositoryRoots, ",") != "~/Projects" || m.setupInput.Focused() {
		t.Errorf("text fields should stay read-only until tab: roots %q focused %v", m.setup.answers.RepositoryRoots, m.setupInput.Focused())
	}
}

func TestSetupFormTabAndSpaceToggleValues(t *testing.T) {
	m, _, _ := setupFormModel(t, "")
	gitWasOn := m.setup.answers.ShowGit
	if m, _ = pressKey(t, m, "tab"); m.setup.answers.ShowGit == gitWasOn {
		t.Errorf("tab should toggle git")
	}
	if m, _ = pressKey(t, m, " "); m.setup.answers.ShowGit != gitWasOn {
		t.Errorf("space should toggle git back")
	}
	m, _ = pressKey(t, m, "j")
	m, _ = pressKey(t, m, "j")
	m, _ = pressKey(t, m, "right")
	if m, _ = pressKey(t, m, "tab"); strings.Join(m.setup.answers.WorkDays, ",") != "mon,wed,thu,fri" {
		t.Errorf("tab should toggle the day under the cursor: %v", m.setup.answers.WorkDays)
	}
	for range 4 {
		m, _ = pressKey(t, m, "j")
	}
	if m, _ = pressKey(t, m, "tab"); !m.setup.notificationsOn {
		t.Errorf("tab should toggle notifications")
	}
}

func TestSetupFormTabEditsATextFieldAndEscReverts(t *testing.T) {
	m, _, _ := setupFormModel(t, "")
	m, _ = pressKey(t, m, "j")
	if m, _ = pressKey(t, m, "tab"); !m.setup.editing || !m.setupInput.Focused() {
		t.Fatalf("tab on a text field should start editing")
	}
	m.setupInput.SetValue("~/cod")
	m, _ = pressKey(t, m, "e")
	if m, _ = pressKey(t, m, "j"); m.setupInput.Value() != "~/codej" || m.setup.field != setupFieldRoots {
		t.Errorf("keys should type while editing: %q field %d", m.setupInput.Value(), m.setup.field)
	}
	m.setupInput.SetValue("~/code")
	if m, _ = pressKey(t, m, "tab"); m.setup.editing || strings.Join(m.setup.answers.RepositoryRoots, ",") != "~/code" {
		t.Errorf("tab should leave editing and keep the value: editing %v roots %v", m.setup.editing, m.setup.answers.RepositoryRoots)
	}
	m, _ = pressKey(t, m, "tab")
	m.setupInput.SetValue("~/oops")
	m, _ = pressKey(t, m, "esc")
	if m.mode != ViewSetupDiscard || !strings.Contains(stripANSI(m.View()), "Undo") {
		t.Fatalf("esc on a changed field should ask before undoing, mode %v:\n%s", m.mode, stripANSI(m.View()))
	}
	m, _ = pressKey(t, m, "y")
	if m.mode != ViewSetup || m.setup.editing || strings.Join(m.setup.answers.RepositoryRoots, ",") != "~/code" {
		t.Errorf("y should revert the field and keep the form open: mode %v editing %v roots %v", m.mode, m.setup.editing, m.setup.answers.RepositoryRoots)
	}
	if !strings.Contains(stripANSI(m.View()), "~/code") {
		t.Errorf("the reverted value should show:\n%s", stripANSI(m.View()))
	}
}

func TestSetupFormEnterWhileEditingSaves(t *testing.T) {
	m, path, _ := setupFormModel(t, "")
	for range 3 {
		m, _ = pressKey(t, m, "j")
	}
	m, _ = pressKey(t, m, "tab")
	m.setupInput.SetValue("07:45")
	m, cmd := pressKey(t, m, "enter")
	m = applyMsgs(t, m, cmd)
	if cfg := readConfigFile(t, path); cfg.DigestNotifications.Morning != "07:45" {
		t.Errorf("enter while editing should save the form, morning = %q", cfg.DigestNotifications.Morning)
	}
}

func TestSetupFormBadTimeBlocksSave(t *testing.T) {
	m, path, installs := setupFormModel(t, "digest_root: /mine\n")
	for range 3 {
		m, _ = pressKey(t, m, "j")
	}
	m, _ = pressKey(t, m, "tab")
	m.setupInput.SetValue("25:99")
	m, cmd := pressKey(t, m, "enter")
	m = applyMsgs(t, m, cmd)
	written, _ := os.ReadFile(path)
	if m.mode != ViewSetup || m.setup.saving || !strings.Contains(stripANSI(m.View()), "HH:MM") || *installs != 0 || string(written) != "digest_root: /mine\n" {
		t.Errorf("a bad time should block saving with a notice, mode %v config %q:\n%s", m.mode, written, stripANSI(m.View()))
	}
}

func TestSetupFormEscClosesACleanFormAndAsksOnADirtyOne(t *testing.T) {
	m, _, _ := setupFormModel(t, "")
	if closed, _ := pressKey(t, m, "esc"); closed.mode != ViewDashboard {
		t.Errorf("esc on an unchanged form should close it, mode %v", closed.mode)
	}
	m, path, _ := setupFormModel(t, "digest_root: /mine\n")
	m, _ = pressKey(t, m, "tab")
	if m, _ = pressKey(t, m, "esc"); m.mode != ViewSetupDiscard || !strings.Contains(stripANSI(m.View()), "Discard") {
		t.Fatalf("esc on a changed form should ask first, mode %v:\n%s", m.mode, stripANSI(m.View()))
	}
	gitAnswer := m.setup.answers.ShowGit
	if m, _ = pressKey(t, m, "n"); m.mode != ViewSetup || m.setup.answers.ShowGit != gitAnswer {
		t.Fatalf("n should return to the form with the changes, mode %v", m.mode)
	}
	m, _ = pressKey(t, m, "esc")
	m, _ = pressKey(t, m, "y")
	written, _ := os.ReadFile(path)
	if m.mode != ViewDashboard || m.setup != nil || string(written) != "digest_root: /mine\n" {
		t.Errorf("y should discard and close, mode %v config %q", m.mode, written)
	}
}

func TestSetupFormSavesInTheBackgroundThenCloses(t *testing.T) {
	m, path, installs := setupFormModel(t, "")
	for range 6 {
		m, _ = pressKey(t, m, "j")
	}
	m, _ = pressKey(t, m, "tab")
	m, cmd := pressKey(t, m, "enter")
	if !m.setup.saving || !strings.Contains(stripANSI(m.View()), "Saving") {
		t.Fatalf("enter should show the saving state:\n%s", stripANSI(m.View()))
	}
	for _, key := range []string{"esc", "j", "tab", "ctrl+c", "ctrl+c", "ctrl+c"} {
		if m, _ = pressKey(t, m, key); m.mode != ViewSetup || !m.setup.saving || m.setup.field != setupFieldNotifications || m.ctrlCCount != 0 {
			t.Fatalf("%q should be ignored while saving: mode %v", key, m.mode)
		}
	}
	m = applyMsgs(t, m, cmd)
	if m.mode != ViewDashboard || m.setup != nil || strings.Contains(stripANSI(m.View()), "Config saved successfully") {
		t.Fatalf("after saving the setup window should close without a message, mode %v:\n%s", m.mode, stripANSI(m.View()))
	}
	if *installs != 1 || m.configPath != path || readConfigFile(t, path) == nil {
		t.Errorf("save should install notifications and keep the config path: installs %d path %q", *installs, m.configPath)
	}
}

func setupFormWithAgent(t *testing.T, installed bool) (Model, *int, *int) {
	t.Helper()
	m, path, installs := setupModel(t, "")
	uninstalls := 0
	notificationsInstalled = func() bool { return installed }
	uninstallNotifications = func() error { uninstalls++; return nil }
	return openSetupForm(t, m, path), installs, &uninstalls
}

func notificationsRow(t *testing.T, m Model) string {
	t.Helper()
	for _, line := range strings.Split(stripANSI(m.View()), "\n") {
		if strings.Contains(line, "Notifications") {
			return line
		}
	}
	t.Fatalf("no Notifications row:\n%s", stripANSI(m.View()))
	return ""
}

func TestSetupFormShowsWhetherNotificationsAreInstalled(t *testing.T) {
	for _, installed := range []bool{true, false} {
		m, _, _ := setupFormWithAgent(t, installed)
		want := map[bool]string{true: "on", false: "off"}[installed]
		if row := notificationsRow(t, m); !strings.HasSuffix(strings.TrimRight(strings.TrimRight(row, " │|"), " "), want) {
			t.Errorf("installed %v: row = %q, want %q", installed, row, want)
		}
		if closed, _ := pressKey(t, m, "esc"); closed.mode != ViewDashboard {
			t.Errorf("installed %v: an untouched form should not be dirty, mode %v", installed, closed.mode)
		}
	}
}

func TestSetupFormSaveInstallsOrUninstallsOnlyOnChange(t *testing.T) {
	cases := []struct {
		installed, toggle         bool
		wantInstalls, wantRemoves int
	}{
		{false, true, 1, 0},
		{true, true, 0, 1},
		{true, false, 0, 0},
		{false, false, 0, 0},
	}
	for _, c := range cases {
		m, installs, uninstalls := setupFormWithAgent(t, c.installed)
		for range 6 {
			m, _ = pressKey(t, m, "j")
		}
		if c.toggle {
			m, _ = pressKey(t, m, "tab")
		}
		m, cmd := pressKey(t, m, "enter")
		applyMsgs(t, m, cmd)
		if *installs != c.wantInstalls || *uninstalls != c.wantRemoves {
			t.Errorf("installed %v toggle %v: installs %d uninstalls %d, want %d %d", c.installed, c.toggle, *installs, *uninstalls, c.wantInstalls, c.wantRemoves)
		}
	}
}

func TestSetupFormScrollsToKeepTheFocusedFieldVisible(t *testing.T) {
	m, _, _ := setupFormModel(t, "")
	m.height = 16
	for range 6 {
		m, _ = pressKey(t, m, "j")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Notifications") {
		t.Errorf("the focused field should stay on screen:\n%s", view)
	}
}

func TestSetupFormEscWhileEditingAsksOnlyWhenTheValueChanged(t *testing.T) {
	m, _, _ := setupFormModel(t, "")
	m, _ = pressKey(t, m, "j")
	m, _ = pressKey(t, m, "tab")
	if m, _ = pressKey(t, m, "esc"); m.mode != ViewSetup || m.setup.editing {
		t.Fatalf("esc on an untouched field should just stop editing, mode %v editing %v", m.mode, m.setup.editing)
	}
	m, _ = pressKey(t, m, "tab")
	m.setupInput.SetValue("~/elsewhere")
	m, _ = pressKey(t, m, "esc")
	if m, _ = pressKey(t, m, "n"); m.mode != ViewSetup || !m.setup.editing || m.setupInput.Value() != "~/elsewhere" {
		t.Errorf("n should keep editing the changed value: mode %v editing %v value %q", m.mode, m.setup.editing, m.setupInput.Value())
	}
}

func TestSetupFormEnterWithNothingChangedDoesNothing(t *testing.T) {
	m, path, installs := setupFormModel(t, "")
	if next, cmd := pressKey(t, m, "enter"); cmd != nil || next.setup.saving || next.mode != ViewSetup {
		t.Errorf("enter on an unchanged form should do nothing: saving %v mode %v", next.setup.saving, next.mode)
	}
	m, _ = pressKey(t, m, "j")
	m, _ = pressKey(t, m, "tab")
	if next, cmd := pressKey(t, m, "enter"); cmd != nil || next.setup.saving {
		t.Errorf("enter while editing an unchanged value should not save")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) || *installs != 0 {
		t.Errorf("nothing should be written: stat err %v installs %d", err, *installs)
	}
}

func TestWizardEscAsksBeforeDiscardingChangedAnswers(t *testing.T) {
	m, path, _ := setupModel(t, "")
	m, _ = pressKey(t, m, "n")
	if m, _ = pressKey(t, m, "esc"); m.mode != ViewSetupDiscard || !strings.Contains(stripANSI(m.View()), "Discard") {
		t.Fatalf("esc after changing an answer should ask first, mode %v:\n%s", m.mode, stripANSI(m.View()))
	}
	if m, _ = pressKey(t, m, "n"); m.mode != ViewSetup || m.setup.step != setupStepWorkDays || m.setup.answers.ShowGit {
		t.Fatalf("n should go back to the wizard with the answers, mode %v step %v", m.mode, m.setup.step)
	}
	m, _ = pressKey(t, m, "esc")
	if m, _ = pressKey(t, m, "y"); m.mode != ViewDashboard || m.setup != nil {
		t.Errorf("y should discard and close, mode %v", m.mode)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("discarding the first-run wizard should leave no config, stat err %v", err)
	}

	m, _, _ = setupModel(t, "")
	m, _ = pressKey(t, m, "y")
	m.setupInput.SetValue("~/typed")
	if m, _ = pressKey(t, m, "esc"); m.mode != ViewSetupDiscard {
		t.Errorf("esc with a typed but unconfirmed answer should ask first, mode %v", m.mode)
	}
}

func TestFirstRunWizardEscWithoutChangesLeavesNoConfig(t *testing.T) {
	m, path, _ := setupModel(t, "")
	if m, _ = pressKey(t, m, "esc"); m.mode != ViewDashboard {
		t.Fatalf("esc on an untouched wizard should close at once, mode %v", m.mode)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("esc should leave no config so the wizard returns, stat err %v", err)
	}
}

func TestWizardArrowsMoveBetweenTheStepsAlreadyReached(t *testing.T) {
	m, _, _ := setupModel(t, "")
	if m, _ = pressKey(t, m, "right"); m.setup.step != setupStepGit {
		t.Fatalf("→ should not go past the furthest step, step %v", m.setup.step)
	}
	for _, key := range []string{"y", "enter", "enter", "enter", "n"} {
		m, _ = pressKey(t, m, key)
	}
	if m.setup.step != setupStepDoctor {
		t.Fatalf("walk: step %v", m.setup.step)
	}
	moves := []struct {
		key  string
		want setupStep
	}{{"right", setupStepDoctor}, {"left", setupStepHints}, {"right", setupStepDoctor}, {"left", setupStepHints}, {"left", setupStepTimes}}
	for _, move := range moves {
		if m, _ = pressKey(t, m, move.key); m.setup.step != move.want {
			t.Fatalf("after %q step = %v, want %v", move.key, m.setup.step, move.want)
		}
	}
	for _, arrow := range []tea.KeyType{tea.KeyLeft, tea.KeyRight} {
		next, _ := m.Update(tea.KeyMsg{Type: arrow})
		if m = next.(Model); m.setup.step != setupStepTimes {
			t.Errorf("arrows should stay in the time field while typing, step %v", m.setup.step)
		}
	}
	if m, _ = pressKey(t, m, "enter"); m.setup.step != setupStepHints {
		t.Fatalf("enter should leave the time step, step %v", m.setup.step)
	}
	if m, _ = pressKey(t, m, "y"); m.setup.step != setupStepDoctor || !m.setup.answers.ShowKeyHints {
		t.Errorf("y should stay the answer on a y/n step, step %v hints %v", m.setup.step, m.setup.answers.ShowKeyHints)
	}

	m, _, _ = setupModel(t, "")
	for _, key := range []string{"n", "enter", "enter"} {
		m, _ = pressKey(t, m, key)
	}
	m, _ = pressKey(t, m, "y")
	if m, _ = pressKey(t, m, "left"); m.setup.step != setupStepHints {
		t.Fatalf("← from the tool check: step %v", m.setup.step)
	}
	m.setup.step = setupStepGit
	if m, _ = pressKey(t, m, "right"); m.setup.step != setupStepWorkDays {
		t.Errorf("→ from git without git should skip the repos step, step %v", m.setup.step)
	}
}
