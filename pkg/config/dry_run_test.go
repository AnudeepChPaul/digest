package config

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func changeTime(t *testing.T, path string) time.Time {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat := info.Sys().(*syscall.Stat_t)
	return time.Unix(stat.Ctimespec.Sec, stat.Ctimespec.Nsec)
}

func TestTightenPermissionsLeavesCorrectModesAlone(t *testing.T) {
	base := t.TempDir()
	digestRoot := filepath.Join(base, "digest")
	notesDir := filepath.Join(digestRoot, "notes")
	configPath := filepath.Join(base, "config.yaml")
	if err := os.MkdirAll(notesDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("digest_root: "+digestRoot+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before := map[string]time.Time{notesDir: changeTime(t, notesDir), configPath: changeTime(t, configPath)}
	time.Sleep(20 * time.Millisecond)
	if err := (&Config{DigestRoot: digestRoot}).TightenPermissions(configPath); err != nil {
		t.Fatal(err)
	}
	for path, changed := range before {
		if !changeTime(t, path).Equal(changed) {
			t.Errorf("%s was chmodded although its mode was already private", path)
		}
	}
}

func TestLoadOrDefaultNeverWritesAConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "digest", "config.yaml")
	cfg, err := LoadOrDefault(configPath)
	if err != nil || cfg == nil {
		t.Fatalf("cfg=%v err=%v", cfg, err)
	}
	if _, err := os.Stat(filepath.Dir(configPath)); err == nil {
		t.Error("LoadOrDefault created the config directory")
	}
}
