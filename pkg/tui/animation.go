package tui

import (
	"runtime/debug"
	"strings"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/brand"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func tickBannerWaveCmd() tea.Cmd {
	return tea.Tick(35*time.Millisecond, func(t time.Time) tea.Msg {
		return bannerWaveTickMsg{}
	})
}

func tickSyncPulseCmd() tea.Cmd {
	return tea.Tick(90*time.Millisecond, func(t time.Time) tea.Msg {
		return syncPulseTickMsg{}
	})
}

func (m *Model) ensureSyncPulse() tea.Cmd {
	if m.syncPulseRunning || !m.headerAnimating() {
		return nil
	}
	m.syncPulseRunning = true
	return tickSyncPulseCmd()
}

func (m Model) headerAnimating() bool {
	if !m.loadingGit && !m.loadingCommits {
		return false
	}
	message, found := m.activeMessage()
	return found && message.kind == messageProgress
}

type waveStyles struct {
	base, dim, center, innerGlow, outerGlow lipgloss.Style
}

func newWaveStyles(baseStyle lipgloss.Style) waveStyles {
	return waveStyles{
		base:      baseStyle,
		dim:       baseStyle.Foreground(colourSurface2),
		center:    baseStyle.Foreground(colourCyan).Bold(true),
		innerGlow: baseStyle.Foreground(colourBlue).Bold(true),
		outerGlow: baseStyle.Foreground(colourSapphire),
	}
}

var (
	bannerWaveStyles = newWaveStyles(headerTitleStyle)
)

func renderSectionTitle(title string, isActive bool) string {
	if isActive {
		return sectionTitleStyle.Underline(true).Render(title)
	}
	return sectionTitleStyle.Render(title)
}

func renderWave(text string, frame int, styles waveStyles) string {
	var sb strings.Builder
	for i, r := range []rune(text) {
		switch abs(i - frame) {
		case 0:
			sb.WriteString(styles.center.Render(string(r)))
		case 1:
			sb.WriteString(styles.innerGlow.Render(string(r)))
		case 2:
			sb.WriteString(styles.outerGlow.Render(string(r)))
		default:
			sb.WriteString(styles.dim.Render(string(r)))
		}
	}
	return sb.String()
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func pulseStyles(colours ...string) []lipgloss.Style {
	styles := make([]lipgloss.Style, len(colours))
	for index, colour := range colours {
		styles[index] = lipgloss.NewStyle().Foreground(lipgloss.Color(colour)).Bold(true)
	}
	return styles
}

var (
	syncPulseStyles = pulseStyles(
		"#45475A", "#585B70", "#6C7086", "#74C7EC",
		"#89B4FA", "#B4BEFE", "#C6A0F6", "#F5C2E7",
		"#F9E2AF", "#F5C2E7", "#B4BEFE", "#74C7EC",
	)
	jobPulseTextStyle  = lipgloss.NewStyle().Foreground(colourYellow).Bold(true)
	selectedPendingBox = lipgloss.NewStyle().Bold(true).Foreground(colourText)
	selectedDoneBox    = lipgloss.NewStyle().Bold(true).Foreground(colourGreen)
)

func (m Model) renderDryRunIndicator() string {
	return jobPulseTextStyle.Render("dry run...")
}

func (m Model) renderJobRunningIndicator() string {
	return jobPulseTextStyle.Render("running...")
}

var readBuildInfo = debug.ReadBuildInfo

func displayVersion() string {
	if appVersion != "" {
		return "v" + appVersion
	}
	if buildInfo, ok := readBuildInfo(); ok && buildInfo.Main.Version != "" && buildInfo.Main.Version != "(devel)" {
		releaseVersion, _, _ := strings.Cut(buildInfo.Main.Version, "+")
		return releaseVersion
	}
	return ""
}

var (
	appVersion   = ""
	versionStyle = lipgloss.NewStyle().Faint(true).Foreground(colourOverlay)
)

const (
	bannerTopRow    = brand.BannerTopRow
	bannerMiddleRow = brand.BannerMiddleRow
	bannerBottomRow = brand.BannerBottomRow
)

func bannerWaveLastFrame() int {
	return len([]rune(bannerTopRow)) + 2
}

func (m Model) renderBannerLine(line string) string {
	if !m.bannerWaveActive {
		return headerTitleStyle.Render(line)
	}
	return renderWave(line, m.bannerWaveFrame, bannerWaveStyles)
}
