package review

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallCommand(t *testing.T) {
	cases := []struct {
		files []string
		want  string
	}{
		{nil, ""},
		{[]string{"pnpm-lock.yaml"}, "pnpm install --frozen-lockfile"},
		{[]string{"yarn.lock"}, "yarn install --frozen-lockfile"},
		{[]string{"yarn.lock", ".yarnrc.yml"}, "yarn install --immutable"},
		{[]string{"package-lock.json"}, "npm ci"},
		{[]string{"package.json"}, "npm install"},
	}
	for _, c := range cases {
		dir := t.TempDir()
		for _, f := range c.files {
			writeFile(t, filepath.Join(dir, f), "")
		}
		if got := strings.Join(InstallCommand(dir), " "); got != c.want {
			t.Errorf("%v: got %q want %q", c.files, got, c.want)
		}
	}
}

func TestInstallScriptUsesNvm(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package-lock.json"), "")
	if got := installScript(dir); got != "npm ci" {
		t.Errorf("without nvmrc got %q", got)
	}
	writeFile(t, filepath.Join(dir, ".nvmrc"), "20")
	got := installScript(dir)
	if !strings.Contains(got, "nvm.sh") || !strings.Contains(got, "nvm install") || !strings.HasSuffix(got, "npm ci") {
		t.Errorf("with nvmrc got %q", got)
	}
	if installScript(t.TempDir()) != "" {
		t.Errorf("no lockfile should give empty script")
	}
}
