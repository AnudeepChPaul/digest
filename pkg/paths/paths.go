package paths

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

func Expand(path string) (string, error) {
	var unsetVariables []string
	expanded := os.Expand(path, func(name string) string {
		value, found := os.LookupEnv(name)
		if !found {
			unsetVariables = append(unsetVariables, name)
		}
		return value
	})
	if len(unsetVariables) > 0 {
		return "", fmt.Errorf("expand %q: environment variable %s is not set", path, strings.Join(unsetVariables, ", "))
	}
	if !strings.HasPrefix(expanded, "~") {
		return expanded, nil
	}
	userName, rest, _ := strings.Cut(expanded[1:], "/")
	home, err := homeOf(userName)
	if err != nil {
		return "", fmt.Errorf("expand %q: %w", path, err)
	}
	return filepath.Join(home, rest), nil
}

func homeOf(userName string) (string, error) {
	if userName == "" {
		return os.UserHomeDir()
	}
	account, err := user.Lookup(userName)
	if err != nil {
		return "", err
	}
	return account.HomeDir, nil
}

const (
	PrivateFileMode os.FileMode = 0600
	PrivateDirMode  os.FileMode = 0700
)
