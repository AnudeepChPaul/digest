package install

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"app/pkg/doctor"

	"golang.org/x/term"
)

var brewInstallable = []string{"go", "git", "gh", "tmux", "nvim", "terminal-notifier"}

func MissingTools(results []doctor.Result) []string {
	var missing []string
	for _, result := range results {
		if !result.Found && result.Note == "" && slices.Contains(brewInstallable, result.Name) {
			missing = append(missing, result.Name)
		}
	}
	return missing
}

func ConfirmFromStdin(reader *bufio.Reader) func(string) bool {
	return func(question string) bool {
		fmt.Print(question + " [y/N] ")
		answer, _ := reader.ReadString('\n')
		answer = strings.ToLower(strings.TrimSpace(answer))
		return answer == "y" || answer == "yes"
	}
}

func ReadRawKey() ([]byte, error) {
	stdinFD := int(os.Stdin.Fd())
	previousState, err := term.MakeRaw(stdinFD)
	if err != nil {
		return nil, err
	}
	defer func() { _ = term.Restore(stdinFD, previousState) }()
	buffer := make([]byte, 8)
	count, err := os.Stdin.Read(buffer)
	if err != nil {
		return nil, err
	}
	return buffer[:count], nil
}

func RunInTerminal(name string, args ...string) error {
	command := exec.Command(name, args...)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	return command.Run()
}

func TmuxServerRunning() bool {
	if _, err := exec.LookPath("tmux"); err != nil {
		return false
	}
	return exec.Command("tmux", "list-sessions").Run() == nil
}
