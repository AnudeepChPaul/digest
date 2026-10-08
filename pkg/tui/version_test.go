package tui

import (
	"runtime/debug"
	"strings"
	"testing"
)

func withVersionSources(t *testing.T, stamped string, moduleVersion string, haveBuildInfo bool) {
	t.Helper()
	originalVersion, originalReader := appVersion, readBuildInfo
	appVersion = stamped
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		if !haveBuildInfo {
			return nil, false
		}
		return &debug.BuildInfo{Main: debug.Module{Version: moduleVersion}}, true
	}
	t.Cleanup(func() { appVersion, readBuildInfo = originalVersion, originalReader })
}

func TestStampedVersionWins(t *testing.T) {
	withVersionSources(t, "1.2.3+abc1234", "v9.9.9", true)
	if got := displayVersion(); got != "v1.2.3+abc1234" {
		t.Errorf("displayVersion() = %q", got)
	}
}

func TestUnstampedBuildShowsTheGoModuleVersion(t *testing.T) {
	for moduleVersion, want := range map[string]string{"v1.1.55": "v1.1.55", "v1.1.54+dirty": "v1.1.54"} {
		withVersionSources(t, "", moduleVersion, true)
		if got := displayVersion(); got != want {
			t.Errorf("module version %q: displayVersion() = %q, want %q", moduleVersion, got, want)
		}
	}
}

func TestUnknownVersionIsHiddenFromTheHeader(t *testing.T) {
	for _, moduleVersion := range []string{"(devel)", ""} {
		withVersionSources(t, "", moduleVersion, true)
		if got := displayVersion(); got != "" {
			t.Errorf("module version %q: displayVersion() = %q", moduleVersion, got)
		}
		m := syncTestModel(t)
		middle := plainLines(m.renderHeader())[2]
		if strings.Contains(middle, " v") || strings.Contains(middle, "dev") || !strings.Contains(middle, "— ") {
			t.Errorf("header should show only the date: %q", middle)
		}
	}
	withVersionSources(t, "", "", false)
	if got := displayVersion(); got != "" {
		t.Errorf("no build info: displayVersion() = %q", got)
	}
}
