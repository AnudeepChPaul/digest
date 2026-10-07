package paths

import (
	"os"
	"path/filepath"
	"strings"
)

func Expand(path string) string {
	expanded := os.ExpandEnv(path)
	if expanded != "~" && !strings.HasPrefix(expanded, "~/") {
		return expanded
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return expanded
	}
	if expanded == "~" {
		return home
	}
	return filepath.Join(home, expanded[2:])
}

const (
	PrivateFileMode os.FileMode = 0600
	PrivateDirMode  os.FileMode = 0700
)

func CreatePrivate(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, PrivateFileMode)
}
