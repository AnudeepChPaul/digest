package install

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"app/pkg/doctor"
)

type fakeRun struct {
	commands []string
}

func (run *fakeRun) run(name string, args ...string) error {
	run.commands = append(run.commands, strings.Join(append([]string{name}, args...), " "))
	return nil
}

func testInstaller(t *testing.T, answers []bool, keys [][]byte) (*Installer, *fakeRun, *bytes.Buffer, *int) {
	t.Helper()
	runner := &fakeRun{}
	output := &bytes.Buffer{}
	notificationInstalls := 0
	installer := &Installer{
		Out:     output,
		HomeDir: t.TempDir(),
		Confirm: func(string) bool {
			if len(answers) == 0 {
				t.Fatal("unexpected confirm")
			}
			answer := answers[0]
			answers = answers[1:]
			return answer
		},
		ReadKey: func() ([]byte, error) {
			if len(keys) == 0 {
				return nil, errors.New("no more keys")
			}
			key := keys[0]
			keys = keys[1:]
			return key, nil
		},
		LookPath: func(name string) (string, error) {
			if name == "brew" {
				return "/opt/homebrew/bin/brew", nil
			}
			return "", errors.New("missing")
		},
		RunCommand:           runner.run,
		TmuxServerRunning:    func() bool { return true },
		InstallNotifications: func() error { notificationInstalls++; return nil },
		MissingTools:         []string{"gh", "nvim", "terminal-notifier"},
	}
	return installer, runner, output, &notificationInstalls
}

func readHomeFile(t *testing.T, installer *Installer, name string) string {
	t.Helper()
	content, _ := os.ReadFile(filepath.Join(installer.HomeDir, name))
	return string(content)
}

func TestDecliningEveryStepChangesNothing(t *testing.T) {
	installer, runner, _, notificationInstalls := testInstaller(t, []bool{false, false, false}, nil)
	if err := installer.Run(); err != nil {
		t.Fatal(err)
	}
	if len(runner.commands) != 0 || *notificationInstalls != 0 || readHomeFile(t, installer, ".tmux.conf") != "" || readHomeFile(t, installer, ".zshrc") != "" {
		t.Errorf("declined steps should change nothing: commands %v installs %d", runner.commands, *notificationInstalls)
	}
}

func TestConfirmingEveryStepInstallsThemAll(t *testing.T) {
	installer, runner, output, notificationInstalls := testInstaller(t, []bool{true, true, true}, [][]byte{{0x07}, {'\r'}})
	if err := installer.Run(); err != nil {
		t.Fatal(err)
	}
	wantCommands := []string{"brew install gh neovim terminal-notifier", "tmux source-file " + filepath.Join(installer.HomeDir, ".tmux.conf")}
	if strings.Join(runner.commands, "\n") != strings.Join(wantCommands, "\n") {
		t.Errorf("commands = %q, want %q", runner.commands, wantCommands)
	}
	if *notificationInstalls != 1 {
		t.Errorf("notifications installed %d times", *notificationInstalls)
	}
	if conf := readHomeFile(t, installer, ".tmux.conf"); !strings.Contains(conf, "bind-key -n C-g display-popup -E -w 90% -h 90% digest") {
		t.Errorf("tmux.conf = %q", conf)
	}
	if !strings.Contains(output.String(), "ctrl+g") {
		t.Errorf("the captured key should be shown: %q", output.String())
	}
}

func TestMissingBrewSkipsDependenciesWithAHint(t *testing.T) {
	installer, runner, output, _ := testInstaller(t, []bool{false, false}, nil)
	installer.LookPath = func(string) (string, error) { return "", errors.New("missing") }
	if err := installer.Run(); err != nil {
		t.Fatal(err)
	}
	if len(runner.commands) != 0 || !strings.Contains(output.String(), "brew.sh") {
		t.Errorf("missing brew should skip with a hint: commands %v output %q", runner.commands, output.String())
	}
}

func TestNoMissingToolsSkipsTheDependencyQuestion(t *testing.T) {
	installer, runner, _, _ := testInstaller(t, []bool{false, false}, nil)
	installer.MissingTools = nil
	if err := installer.Run(); err != nil {
		t.Fatal(err)
	}
	if len(runner.commands) != 0 {
		t.Errorf("commands = %v", runner.commands)
	}
}

func TestShortcutFallsBackToZshWithoutATmuxServer(t *testing.T) {
	installer, runner, _, _ := testInstaller(t, []bool{true}, [][]byte{{0x1b, 'd'}, {'\r'}})
	installer.MissingTools = nil
	installer.TmuxServerRunning = func() bool { return false }
	if err := installer.InstallShortcut(); err != nil {
		t.Fatal(err)
	}
	zshrc := readHomeFile(t, installer, ".zshrc")
	for _, want := range []string{"zle -N digest-open", "bindkey '^[d' digest-open"} {
		if !strings.Contains(zshrc, want) {
			t.Errorf("zshrc should contain %q:\n%s", want, zshrc)
		}
	}
	if len(runner.commands) != 0 || readHomeFile(t, installer, ".tmux.conf") != "" {
		t.Errorf("zsh install should not touch tmux: %v", runner.commands)
	}
}

func TestAnotherKeyBeforeEnterPicksAgain(t *testing.T) {
	installer, _, _, _ := testInstaller(t, []bool{true}, [][]byte{{0x07}, {0x1b, 'x'}, {'\r'}})
	if err := installer.InstallShortcut(); err != nil {
		t.Fatal(err)
	}
	if conf := readHomeFile(t, installer, ".tmux.conf"); !strings.Contains(conf, "bind-key -n M-x ") || strings.Contains(conf, "C-g") {
		t.Errorf("the last key before enter should win: %q", conf)
	}
}

func TestUnsupportedKeysAreRejected(t *testing.T) {
	installer, _, output, _ := testInstaller(t, []bool{true}, [][]byte{{0x1b, '[', 'A'}, {0x09}, {0x05}, {'\r'}})
	if err := installer.InstallShortcut(); err != nil {
		t.Fatal(err)
	}
	if conf := readHomeFile(t, installer, ".tmux.conf"); !strings.Contains(conf, "C-e") || !strings.Contains(output.String(), "not supported") {
		t.Errorf("unsupported keys should be refused: conf %q output %q", conf, output.String())
	}
}

func TestParseShortcut(t *testing.T) {
	cases := []struct {
		raw             []byte
		name, tmux, zsh string
		supported       bool
	}{
		{[]byte{0x07}, "ctrl+g", "C-g", "^G", true},
		{[]byte{0x1b, 'g'}, "alt+g", "M-g", "^[g", true},
		{[]byte{'\r'}, "", "", "", false},
		{[]byte{0x0a}, "", "", "", false},
		{[]byte{0x09}, "", "", "", false},
		{[]byte{'g'}, "", "", "", false},
		{[]byte{0x1b, '[', 'A'}, "", "", "", false},
	}
	for _, c := range cases {
		shortcut, ok := ParseShortcut(c.raw)
		if ok != c.supported || (ok && (shortcut.Name() != c.name || shortcut.Tmux() != c.tmux || shortcut.Zsh() != c.zsh)) {
			t.Errorf("%q: got %+v %v", c.raw, shortcut, ok)
		}
	}
}

func TestReinstallingReplacesTheEarlierShortcut(t *testing.T) {
	installer, _, _, _ := testInstaller(t, []bool{true, true}, [][]byte{{0x07}, {'\r'}, {0x05}, {'\r'}})
	confPath := filepath.Join(installer.HomeDir, ".tmux.conf")
	if err := os.WriteFile(confPath, []byte("set -g mouse on\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := installer.InstallShortcut(); err != nil {
			t.Fatal(err)
		}
	}
	conf := readHomeFile(t, installer, ".tmux.conf")
	if !strings.HasPrefix(conf, "set -g mouse on\n") || strings.Count(conf, shortcutTag) != 1 || !strings.Contains(conf, "C-e") {
		t.Errorf("reinstall should keep other lines and replace the old binding:\n%s", conf)
	}
}

func TestMissingToolsKeepsOnlyBrewInstallableTools(t *testing.T) {
	results := []doctor.Result{
		{Name: "git", Found: true},
		{Name: "gh"},
		{Name: "nvim"},
		{Name: "git_repository_roots"},
		{Name: "claude"},
		{Name: "tmux", Note: "not needed (show_git: false)"},
	}
	if got := strings.Join(MissingTools(results), ","); got != "gh,nvim" {
		t.Errorf("missing tools = %q", got)
	}
}
