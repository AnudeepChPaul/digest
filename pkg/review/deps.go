package review

import (
	"os"
	"path/filepath"
	"strings"
)

func fileExists(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

func InstallCommand(dir string) []string {
	switch {
	case fileExists(dir, "pnpm-lock.yaml"):
		return []string{"pnpm", "install", "--frozen-lockfile"}
	case fileExists(dir, "yarn.lock") && fileExists(dir, ".yarnrc.yml"):
		return []string{"yarn", "install", "--immutable"}
	case fileExists(dir, "yarn.lock"):
		return []string{"yarn", "install", "--frozen-lockfile"}
	case fileExists(dir, "package-lock.json"):
		return []string{"npm", "ci"}
	case fileExists(dir, "package.json"):
		return []string{"npm", "install"}
	}
	return nil
}

func installScript(dir string) string {
	install := InstallCommand(dir)
	if len(install) == 0 {
		return ""
	}
	command := strings.Join(install, " ")
	if fileExists(dir, ".nvmrc") {
		return `export NVM_DIR="${NVM_DIR:-$HOME/.nvm}"; . "$NVM_DIR/nvm.sh" && nvm install && ` + command
	}
	return command
}
