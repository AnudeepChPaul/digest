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
