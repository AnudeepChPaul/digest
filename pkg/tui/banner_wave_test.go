package tui

import (
	"strings"
	"testing"

	"app/pkg/config"
)

func bannerTestModel(t *testing.T) Model {
	t.Helper()
	m := NewModel(&config.Config{ReviewRoot: t.TempDir()}, nil)
	m.width, m.height = 120, 40
	return m
}

func TestBannerWaveStartsOnNewModel(t *testing.T) {
	m := bannerTestModel(t)
	if !m.bannerWaveActive || m.bannerWaveFrame != 0 {
		t.Errorf("banner wave active=%v frame=%d, want active at frame 0", m.bannerWaveActive, m.bannerWaveFrame)
	}
}

func TestBannerWaveTickAdvancesThenStops(t *testing.T) {
	m := bannerTestModel(t)
	next, cmd := m.Update(bannerWaveTickMsg{})
	advanced := next.(Model)
	if cmd == nil || advanced.bannerWaveFrame != 1 {
		t.Fatalf("tick frame=%d cmd=%v, want frame 1 and another tick", advanced.bannerWaveFrame, cmd)
	}

	advanced.bannerWaveFrame = bannerWaveLastFrame()
	next, cmd = advanced.Update(bannerWaveTickMsg{})
	finished := next.(Model)
	if cmd != nil || finished.bannerWaveActive {
		t.Errorf("wave still running after last frame: active=%v cmd=%v", finished.bannerWaveActive, cmd)
	}
}

func TestBannerWaveIgnoresGitSyncWave(t *testing.T) {
	m := bannerTestModel(t)
	m.bannerWaveActive = false
	m.beginGitFetch(true)
	if m.bannerWaveActive {
		t.Errorf("git sync replayed the banner wave")
	}
}

func TestHeaderShowsDigestBanner(t *testing.T) {
	m := bannerTestModel(t)
	m.bannerWaveActive = false
	header := m.renderHeader()
	for _, row := range []string{bannerTopRow, bannerBottomRow} {
		if !strings.Contains(header, row) {
			t.Errorf("header missing banner row %q", row)
		}
	}
	if strings.Contains(header, "█▄░█ █▀█ ▀█▀ █▀▀") {
		t.Errorf("header still shows NOTE banner")
	}
}
