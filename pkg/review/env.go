package review

import (
	"os"
	"strings"
)

var allowedEnvNames = map[string]bool{
	"PATH": true, "HOME": true, "USER": true, "LOGNAME": true, "SHELL": true, "LANG": true, "TMPDIR": true, "TERM": true, "NVM_DIR": true,
	"HTTP_PROXY": true, "HTTPS_PROXY": true, "NO_PROXY": true, "http_proxy": true, "https_proxy": true, "no_proxy": true,
	"NODE_EXTRA_CA_CERTS": true, "SSL_CERT_FILE": true, "SSL_CERT_DIR": true, "REQUESTS_CA_BUNDLE": true, "CURL_CA_BUNDLE": true,
}

var installEnvNames = map[string]bool{
	"VOLTA_HOME": true, "PNPM_HOME": true, "COREPACK_HOME": true,
}

var installEnvPrefixes = []string{"npm_config_", "NPM_CONFIG_", "ASDF_"}

var reviewEnvNames = map[string]bool{
	"GH_HOST": true, "GH_CONFIG_DIR": true, "AWS_PROFILE": true, "AWS_REGION": true, "AWS_DEFAULT_REGION": true, "AWS_CONFIG_FILE": true,
	"AWS_SHARED_CREDENTIALS_FILE": true, "CLOUD_ML_REGION": true, "GOOGLE_APPLICATION_CREDENTIALS": true, "GOOGLE_CLOUD_PROJECT": true,
}

var reviewEnvPrefixes = []string{"ANTHROPIC_", "CLAUDE_"}

var gitSafetyEnv = []string{
	"GIT_CONFIG_COUNT=2",
	"GIT_CONFIG_KEY_0=core.hooksPath", "GIT_CONFIG_VALUE_0=/dev/null",
	"GIT_CONFIG_KEY_1=core.fsmonitor", "GIT_CONFIG_VALUE_1=false",
	"GIT_TERMINAL_PROMPT=0", "CI=true",
}

func allowlistedEnv(extraNames map[string]bool, extraPrefixes []string) []string {
	var kept []string
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if allowedEnvNames[name] || extraNames[name] || strings.HasPrefix(name, "LC_") || strings.HasPrefix(name, "XDG_") || hasAnyPrefix(name, extraPrefixes) {
			kept = append(kept, entry)
		}
	}
	return kept
}

func hasAnyPrefix(name string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func stepEnv() []string {
	return append(os.Environ(), gitSafetyEnv...)
}

func installEnv() []string {
	return append(append(allowlistedEnv(installEnvNames, installEnvPrefixes), gitSafetyEnv...), "YARN_IGNORE_PATH=1")
}

func ReviewEnv() []string {
	return append(allowlistedEnv(reviewEnvNames, reviewEnvPrefixes), gitSafetyEnv...)
}
