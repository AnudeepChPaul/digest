package tui

import "github.com/charmbracelet/lipgloss"

var (
	boldTextStyle     = lipgloss.NewStyle().Bold(true).Foreground(colourText)
	headerTitleStyle  = boldTextStyle
	sectionTitleStyle = boldTextStyle
	keyStyle          = boldTextStyle

	borderStyle     = lipgloss.NewStyle().Foreground(colourSurface1)
	subSectionStyle = lipgloss.NewStyle().Bold(true).Foreground(colourBlue)
	mutedStyle      = lipgloss.NewStyle().Foreground(colourOverlay)
	actionStyle     = mutedStyle
	itemStyle       = lipgloss.NewStyle().Foreground(colourText)

	selectedSummaryStyle = itemStyle.Bold(true)
	jobActiveTagStyle    = lipgloss.NewStyle().Bold(true).Foreground(colourGreen)

	tabActiveStyle   = lipgloss.NewStyle().Bold(true).Underline(true).Foreground(colourBlue).Padding(0, 1)
	tabInactiveStyle = lipgloss.NewStyle().Foreground(colourOverlay).Padding(0, 1)

	checkDone        = lipgloss.NewStyle().Foreground(colourGreen).SetString("✔")
	checkPending     = lipgloss.NewStyle().Foreground(colourText).SetString("☐")
	amberDiamond     = lipgloss.NewStyle().Foreground(colourYellow).SetString("◆")
	yellowBadgeStyle = lipgloss.NewStyle().Foreground(colourYellow).Bold(true)
	pendingPRIcon    = lipgloss.NewStyle().Foreground(colourSapphire).SetString("⊙")
	dimBlueText      = lipgloss.NewStyle().Foreground(colourSapphire)

	warnKeyStyle    = lipgloss.NewStyle().Bold(true).Foreground(colourRed)
	warnActionStyle = lipgloss.NewStyle().Foreground(colourYellow)

	badgeActive = lipgloss.NewStyle().Foreground(colourBrightLime).Bold(true)
	badgeDone   = lipgloss.NewStyle().Foreground(colourGreen).Bold(true)
	tagStyle    = lipgloss.NewStyle().Foreground(colourBlue).Bold(true)

	modalStyle       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colourAccent).Padding(1, 2)
	modalTitleStyle  = lipgloss.NewStyle().Bold(true).Foreground(colourAccent)
	deleteTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(colourRed)

	staleStyle     = lipgloss.NewStyle().Foreground(colourRed).Bold(true)
	criticalStyle  = staleStyle
	highStyle      = lipgloss.NewStyle().Foreground(colourPeach).Bold(true)
	approvedStyle  = badgeDone
	reviewingStyle = yellowBadgeStyle
	reviewedStyle  = tagStyle
	cursorStyle    = lipgloss.NewStyle().Foreground(colourRosewater).Bold(true)
)
