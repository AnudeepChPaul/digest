package tui

import (
	"strings"
	"testing"

	"github.com/achandrapaul/digest/pkg/config"
)

func bannerTestModel(t *testing.T) Model {
	t.Helper()
	m := NewModel(&config.Config{DigestRoot: t.TempDir()}, nil)
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
	m.beginGitFetch()
	if m.bannerWaveActive {
		t.Errorf("git sync replayed the banner wave")
	}
}

func TestHeaderShowsDigestBanner(t *testing.T) {
	m := bannerTestModel(t)
	m.bannerWaveActive = false
	header := headerSection{}.Render(m)
	for _, row := range []string{bannerTopRow, bannerMiddleRow, bannerBottomRow} {
		if !strings.Contains(header, row) {
			t.Errorf("header missing banner row %q", row)
		}
	}
	if strings.Contains(header, "█▄░█ █▀█ ▀█▀ █▀▀") {
		t.Errorf("header still shows NOTE banner")
	}
}

func TestHeaderTextSitsOnMiddleLogoRow(t *testing.T) {
	m := bannerTestModel(t)
	m.bannerWaveActive = false
	lines := plainLines(headerSection{}.Render(m))
	if !strings.Contains(lines[2], strings.TrimSpace(bannerMiddleRow)) || !strings.Contains(lines[2], "— ") {
		t.Errorf("date should share the middle logo row:\n%s", strings.Join(lines, "\n"))
	}
	if strings.Contains(lines[1], "— ") || strings.Contains(lines[3], "— ") {
		t.Errorf("date should not sit on the top or bottom logo row:\n%s", strings.Join(lines, "\n"))
	}
}
