package install

import (
	"fmt"
	"strings"
)

type Shortcut struct {
	alt    bool
	letter byte
}

func isEnter(raw []byte) bool {
	return len(raw) == 1 && (raw[0] == '\r' || raw[0] == '\n')
}

func ParseShortcut(raw []byte) (Shortcut, bool) {
	switch {
	case len(raw) == 1 && raw[0] >= 1 && raw[0] <= 26 && raw[0] != '\t' && raw[0] != '\n' && raw[0] != '\r':
		return Shortcut{letter: 'a' + raw[0] - 1}, true
	case len(raw) == 2 && raw[0] == 0x1b && raw[1] >= 'a' && raw[1] <= 'z':
		return Shortcut{alt: true, letter: raw[1]}, true
	}
	return Shortcut{}, false
}

func (shortcut Shortcut) Name() string {
	if shortcut.alt {
		return fmt.Sprintf("alt+%c", shortcut.letter)
	}
	return fmt.Sprintf("ctrl+%c", shortcut.letter)
}

func (shortcut Shortcut) Tmux() string {
	if shortcut.alt {
		return fmt.Sprintf("M-%c", shortcut.letter)
	}
	return fmt.Sprintf("C-%c", shortcut.letter)
}

func (shortcut Shortcut) Zsh() string {
	if shortcut.alt {
		return fmt.Sprintf("^[%c", shortcut.letter)
	}
	return "^" + strings.ToUpper(string(shortcut.letter))
}
