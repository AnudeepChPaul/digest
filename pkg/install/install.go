package install

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/achandrapaul/digest/pkg/system"
)

const shortcutTag = "# digest shortcut"

var brewFormulas = map[string]string{"nvim": "neovim"}

type Installer struct {
	Out                  io.Writer
	HomeDir              string
	Confirm              func(question string) bool
	ReadKey              func() ([]byte, error)
	LookPath             func(name string) (string, error)
	RunCommand           func(name string, args ...string) error
	TmuxServerRunning    func() bool
	InstallNotifications func() error
	MissingTools         []string
}

func (installer *Installer) Run() error {
	if err := installer.InstallDependencies(); err != nil {
		return err
	}
	if installer.Confirm("Install the notifications launchd agent?") {
		if err := installer.InstallNotifications(); err != nil {
			return err
		}
		fmt.Fprintln(installer.Out, "✓ notifications installed")
	}
	return installer.InstallShortcut()
}

func (installer *Installer) InstallDependencies() error {
	if len(installer.MissingTools) == 0 {
		return nil
	}
	fmt.Fprintln(installer.Out, "Missing tools: "+strings.Join(installer.MissingTools, ", "))
	if _, err := installer.LookPath("brew"); err != nil {
		fmt.Fprintln(installer.Out, "Homebrew is not installed; get it from https://brew.sh and run digest install again.")
		return nil
	}
	formulas := make([]string, len(installer.MissingTools))
	for index, tool := range installer.MissingTools {
		formulas[index] = tool
		if formula, renamed := brewFormulas[tool]; renamed {
			formulas[index] = formula
		}
	}
	if !installer.Confirm("Install them with brew install " + strings.Join(formulas, " ") + "?") {
		return nil
	}
	return installer.RunCommand("brew", append([]string{"install"}, formulas...)...)
}

func (installer *Installer) InstallShortcut() error {
	if !installer.Confirm("Install a keyboard shortcut that opens digest?") {
		return nil
	}
	shortcut, err := installer.captureShortcut()
	if err != nil {
		return err
	}
	if installer.TmuxServerRunning() {
		confPath := filepath.Join(installer.HomeDir, ".tmux.conf")
		binding := fmt.Sprintf("bind-key -n %s display-popup -E -w 90%% -h 90%% digest %s", shortcut.Tmux(), shortcutTag)
		if err := replaceTaggedLines(confPath, []string{binding}); err != nil {
			return err
		}
		fmt.Fprintf(installer.Out, "✓ %s opens digest in a tmux popup\n", shortcut.Name())
		return installer.RunCommand("tmux", "source-file", confPath)
	}
	widget := []string{
		"digest-open() { BUFFER=digest; zle accept-line } " + shortcutTag,
		"zle -N digest-open " + shortcutTag,
		fmt.Sprintf("bindkey '%s' digest-open %s", shortcut.Zsh(), shortcutTag),
	}
	if err := replaceTaggedLines(filepath.Join(installer.HomeDir, ".zshrc"), widget); err != nil {
		return err
	}
	fmt.Fprintf(installer.Out, "✓ %s runs digest from the zsh prompt; open a new shell to use it\n", shortcut.Name())
	return nil
}

func (installer *Installer) captureShortcut() (Shortcut, error) {
	fmt.Fprintln(installer.Out, "Press the shortcut (ctrl+letter or alt+letter):")
	var chosen Shortcut
	picked := false
	for {
		raw, err := installer.ReadKey()
		if err != nil {
			return Shortcut{}, err
		}
		if picked && isEnter(raw) {
			return chosen, nil
		}
		shortcut, supported := ParseShortcut(raw)
		if !supported {
			fmt.Fprintln(installer.Out, "That key is not supported; press ctrl+letter or alt+letter.")
			continue
		}
		chosen, picked = shortcut, true
		fmt.Fprintf(installer.Out, "%s — press enter to confirm, or another shortcut to pick again\n", shortcut.Name())
	}
}

func replaceTaggedLines(path string, taggedLines []string) error {
	existing, err := system.Read(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	var kept []string
	if trimmed := strings.TrimRight(string(existing), "\n"); trimmed != "" {
		for _, line := range strings.Split(trimmed, "\n") {
			if !strings.HasSuffix(line, shortcutTag) {
				kept = append(kept, line)
			}
		}
	}
	content := strings.Join(append(kept, taggedLines...), "\n") + "\n"
	return system.WriteWithMode(path, []byte(content), 0o644)
}
