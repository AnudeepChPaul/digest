package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/review"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	ciPassingIcon = lipgloss.NewStyle().Foreground(colourGreen).Render("✓")
	ciFailingIcon = lipgloss.NewStyle().Foreground(colourRed).Render("✗")
	ciRunningIcon = lipgloss.NewStyle().Foreground(colourYellow).Render("◌")
	ciUnknownIcon = mutedStyle.Render("·")
)

const myPRApprovedGlyph = "\U000F0A50"

var myPRApprovedIcon = lipgloss.NewStyle().Foreground(colourGreen).Render(myPRApprovedGlyph)

func myPRApprovalCell(decision string) string {
	if decision == "APPROVED" {
		return myPRApprovedIcon
	}
	return safeRepeat(" ", lipgloss.Width(myPRApprovedGlyph))
}

func myPRCIIcon(state string) string {
	switch state {
	case "SUCCESS":
		return ciPassingIcon
	case "FAILURE":
		return ciFailingIcon
	case "PENDING":
		return ciRunningIcon
	}
	return ciUnknownIcon
}

func myPRCIText(state string) string {
	switch state {
	case "SUCCESS":
		return "passing"
	case "FAILURE":
		return "failing"
	case "PENDING":
		return "running"
	}
	return "no checks"
}

func myPRFilesText(changedFiles int) string {
	if changedFiles == 1 {
		return "1 file"
	}
	return fmt.Sprintf("%d files", changedFiles)
}

type myPRColumnLine struct {
	text     string
	navIndex int
}

func (m Model) renderMyPRBlock(firstIndex, width int) []myPRColumnLine {
	if len(m.myPRs) == 0 {
		hint := "     (no open PRs)"
		if m.loadingMyPRs {
			hint = "     (fetching...)"
		}
		return []myPRColumnLine{{text: mutedStyle.Render(hint), navIndex: -1}}
	}
	var lines []myPRColumnLine
	currentRepo := ""
	for index, pr := range m.myPRs {
		if pr.Ref.Repo != currentRepo || index == 0 {
			currentRepo = pr.Ref.Repo
			lines = append(lines, myPRColumnLine{text: "     " + subSectionStyle.Render(currentRepo), navIndex: -1})
		}
		navIndex := firstIndex + index
		selected := navIndex == m.selected
		numberStyle := itemStyle
		if selected {
			numberStyle = selectedSummaryStyle
		}
		branch := pr.HeadRef
		if pr.IsDraft {
			branch += " (draft)"
		}
		left := fmt.Sprintf("       %s %s", underlinedWhen(selected, numberStyle.Render(fmt.Sprintf("#%d", pr.Ref.Number))), mutedStyle.Render(branch))
		facts := mutedStyle.Render(fmt.Sprintf("%s · %s ago", myPRFilesText(pr.ChangedFiles), shortAge(time.Since(pr.CreatedAt))))
		icon := facts + "  " + myPRApprovalCell(pr.ReviewDecision) + " " + myPRCIIcon(pr.CIState)
		left = ansi.Truncate(left, max(width-lipgloss.Width(icon)-2, 1), "…")
		lines = append(lines, myPRColumnLine{text: left + safeRepeat(" ", width-lipgloss.Width(left)-lipgloss.Width(icon)-1) + underlinedWhen(selected, icon), navIndex: navIndex})
	}
	return lines
}

var myPRReviewWords = map[string]string{
	"APPROVED":          "approved",
	"CHANGES_REQUESTED": "changes requested",
	"COMMENTED":         "commented",
	"DISMISSED":         "dismissed",
}

func myPRDetailsMarkdown(pr review.QueuedPR) string {
	var details strings.Builder
	fmt.Fprintf(&details, "# %s\n\n", pr.Title)
	fmt.Fprintf(&details, "- **PR:** %s/%s#%d\n", pr.Ref.Owner, pr.Ref.Repo, pr.Ref.Number)
	fmt.Fprintf(&details, "- **Branch:** `%s`\n", pr.HeadRef)
	fmt.Fprintf(&details, "- **Files:** %s (+%d / -%d)\n", myPRFilesText(pr.ChangedFiles), pr.Additions, pr.Deletions)
	fmt.Fprintf(&details, "- **Created:** %s (%s ago)\n", pr.CreatedAt.Local().Format("2006-01-02 15:04"), shortAge(time.Since(pr.CreatedAt)))
	fmt.Fprintf(&details, "- **CI:** %s\n", myPRCIText(pr.CIState))
	decision := strings.ToLower(strings.ReplaceAll(pr.ReviewDecision, "_", " "))
	if decision == "" {
		decision = "none"
	}
	if pr.ReviewDecision == "APPROVED" {
		decision = myPRApprovedGlyph + " " + decision
	}
	if pr.IsDraft {
		decision += " (draft)"
	}
	fmt.Fprintf(&details, "- **Review decision:** %s\n", decision)
	var reviewers []string
	for _, prReview := range pr.Reviews {
		word, known := myPRReviewWords[prReview.State]
		if !known || prReview.Author == pr.Author || review.IsBot(prReview.Author) {
			continue
		}
		reviewers = append(reviewers, fmt.Sprintf("@%s (%s)", prReview.Author, word))
	}
	if len(reviewers) > 0 {
		fmt.Fprintf(&details, "- **Reviewers:** %s\n", strings.Join(reviewers, ", "))
	}
	description := normalizedDescription(pr.Body)
	if description == "" {
		description = "_No description._"
	}
	fmt.Fprintf(&details, "\n## Description\n\n%s\n\nPress **[Enter]** on the row to open it in the browser.", description)
	return details.String()
}
