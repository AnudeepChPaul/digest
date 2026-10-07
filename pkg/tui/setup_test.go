package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"app/pkg/config"
	"app/pkg/doctor"

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
	previousInstall, previousDoctor := installNotifications, runDoctor
	installNotifications = func(*config.Config) error {
		installs++
		return nil
	}
	runDoctor = func(*config.Config) []doctor.Result {
		return []doctor.Result{{Name: "git", Found: true, Required: true, Path: "/bin/git"}, {Name: "gh", Required: true}}
	}
	t.Cleanup(func() { installNotifications, runDoctor = previousInstall, previousDoctor })
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

func TestSetupRejectsABadTimeAndSkipsInstallOnEsc(t *testing.T) {
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
	m, _ = pressKey(t, m, "esc")
	if *installs != 0 || m.mode != ViewDashboard {
		t.Errorf("esc should skip the install: installs %d mode %v", *installs, m.mode)
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
