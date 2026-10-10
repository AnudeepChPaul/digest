package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const unsetVariablePath = "$DIGEST_TEST_UNSET_CONFIG_VARIABLE/digest.yaml"

func TestAnUnexpandableDigestRootIsAConfigError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "digest.yaml")
	if err := os.WriteFile(path, []byte("digest_root: $DIGEST_TEST_UNSET_CONFIG_VARIABLE/digest\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadOrDefault(path)
	if cfg == nil || cfg.DigestRoot != DefaultConfig().DigestRoot {
		t.Errorf("cfg = %+v, want the defaults", cfg)
	}
	if err == nil || !strings.Contains(err.Error(), "digest_root") || !strings.Contains(err.Error(), "DIGEST_TEST_UNSET_CONFIG_VARIABLE") {
		t.Errorf("err = %v, want digest_root and the variable named", err)
	}
}

func TestAnUnexpandableConfigPathIsReported(t *testing.T) {
	if _, err := LoadOrCreate(unsetVariablePath); err == nil || !strings.Contains(err.Error(), "DIGEST_TEST_UNSET_CONFIG_VARIABLE") {
		t.Errorf("LoadOrCreate err = %v", err)
	}
	if _, err := LoadOrDefault(unsetVariablePath); err == nil || !strings.Contains(err.Error(), "DIGEST_TEST_UNSET_CONFIG_VARIABLE") {
		t.Errorf("LoadOrDefault err = %v", err)
	}
	if _, _, err := Init(unsetVariablePath, true); err == nil {
		t.Error("Init should fail")
	}
	if err := DefaultConfig().TightenPermissions(unsetVariablePath); err == nil {
		t.Error("TightenPermissions should fail")
	}
	if Exists(unsetVariablePath) {
		t.Error("an unexpandable path cannot exist")
	}
	if got := Path(unsetVariablePath); got != unsetVariablePath {
		t.Errorf("Path = %q, want it unchanged", got)
	}
}

func TestWithoutAHomeTheDefaultConfigPathIsReported(t *testing.T) {
	defaults := DefaultConfig()
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	if _, err := resolveConfigPath(""); err == nil {
		t.Error("the config folder should be unknown without a home")
	}
	if got := defaults.Root(); got != DefaultDigestRoot {
		t.Errorf("Root = %q, want the default kept as written", got)
	}
	if got := (&Config{DigestRoot: "~/digest"}).Root(); got != "~/digest" {
		t.Errorf("Root = %q, want the configured value kept", got)
	}
}

func TestXDGConfigHomeIsUsedWhenAbsolute(t *testing.T) {
	xdgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgHome)
	if err := os.WriteFile(filepath.Join(xdgHome, "digest.yaml"), []byte("digest_root: /tmp/d\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Path(""); got != filepath.Join(xdgHome, "digest.yaml") {
		t.Errorf("Path = %q", got)
	}
}
