package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/automation"
	"github.com/AnudeepChPaul/digest/pkg/model"
	"github.com/AnudeepChPaul/digest/pkg/review"
	"github.com/AnudeepChPaul/digest/pkg/sourcecontrol"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (m Model) renderGitRepoRow(repo *GitRepoStat, selected bool, width int) string {
	var statParts []string
	if repo.Assigned > 0 {
		statParts = append(statParts, fmt.Sprintf("%d assigned", repo.Assigned))
	}
	if repo.Reviewed > 0 {
		statParts = append(statParts, fmt.Sprintf("%d reviewed", repo.Reviewed))
	}
	if m.cfg.DailyCommitsEnabled() {
		statParts = append(statParts, fmt.Sprintf("%d commits", repo.Commits))
	}
	stats := strings.Join(statParts, " · ")
	if room := width - 10; lipgloss.Width(stats) > room {
		stats = "…" + string([]rune(stats)[len([]rune(stats))-max(room-1, 1):])
	}

	nameStyle := subSectionStyle
	return alignRight("     "+underlinedWhen(selected, nameStyle.Render(repo.Name)), []string{mutedStyle.Render(stats)}, width, selected) + "\n"
}

const tagGap = "  "

func alignRight(left string, tags []string, width int, selected bool) string {
	tagBlock := joinTags(tags)
	room := width - lipgloss.Width(tagBlock) - 1
	if lipgloss.Width(left) > room {
		left = ansi.Truncate(left, max(room, 0), "…")
	}
	return left + safeRepeat(" ", width-lipgloss.Width(left)-lipgloss.Width(tagBlock)) + underlinedWhen(selected, tagBlock)
}

func joinTags(tags []string) string {
	var shown []string
	for _, tag := range tags {
		if tag != "" {
			shown = append(shown, tag)
		}
	}
	return strings.Join(shown, tagGap)
}

type rowTagCellCache struct {
	prs map[*GitPRItem][3]string
}

const maxCachedNoteRows = 4096

type noteTagKey struct {
	source      model.Source
	status      model.Status
	created     time.Time
	currentDate time.Time
}

type noteRowKey struct {
	tags              noteTagKey
	automationTracked bool
	automationStatus  automation.RunStatus
	automated         string
	notifyInterval    string
	summary           string
	selected          bool
	width             int
}

type noteRowCache struct {
	tags map[noteTagKey][2]string
	rows map[noteRowKey]string
}

func newNoteRowCache() *noteRowCache {
	return &noteRowCache{tags: map[noteTagKey][2]string{}, rows: map[noteRowKey]string{}}
}

func (m Model) noteTagKey(n *model.Note) noteTagKey {
	return noteTagKey{source: n.Source, status: n.Status, created: n.Created, currentDate: m.currentDate}
}

func (m Model) prTagCells(item *GitPRItem) (size, age, state string) {
	if m.tagCells == nil {
		return m.renderPRTagCells(item)
	}
	if cells, cached := m.tagCells.prs[item]; cached {
		return cells[0], cells[1], cells[2]
	}
	size, age, state = m.renderPRTagCells(item)
	m.tagCells.prs[item] = [3]string{size, age, state}
	return size, age, state
}

func (m Model) renderPRTagCells(item *GitPRItem) (size, age, state string) {
	if item.PR == nil {
		return "", "", mutedStyle.Render(fmt.Sprintf("[%s]", item.Kind))
	}
	now := time.Now()
	prState, stateLabel := m.prStateLabel(item)
	state = stateStyle(prState).Render(stateLabel)
	if prState == review.StateReviewing {
		state = m.renderReviewRunningIndicator()
	}
	ageText := shortAge(now.Sub(item.PR.RequestedAt))
	age = mutedStyle.Render(ageText)
	if item.PR.IsStale(now) {
		age = staleStyle.Render(ageText)
	}
	return mutedStyle.Render(fmt.Sprintf("±%d", item.PR.Size())), age, state
}

const (
	firstReviewIcon  = "\U000F0CA1"
	reReviewIcon     = "\U000F0458"
	teamReviewIcon   = "\U000F0849"
	directReviewIcon = "\U000F0065"

	reviewIconSlotWidth = 2
)

func reviewRequestIcon(item *GitPRItem) string {
	icons := firstReviewIcon
	if item.Kind == sourcecontrol.ReReviewKind {
		icons = reReviewIcon
	}
	switch {
	case item.PR.DirectRequest:
		icons += directReviewIcon
	case item.PR.CodeOwner:
		icons += teamReviewIcon
	}
	return dimBlueText.Render(icons)
}

func (m Model) prTags(item *GitPRItem, selected bool) []string {
	size, age, state := m.prTagCells(item)
	if item.PR == nil {
		return []string{state}
	}
	icons := reviewRequestIcon(item)
	stateWithIcons := state + " " + safeRepeat(" ", reviewIconSlotWidth-lipgloss.Width(icons)) + icons
	if !selected && !m.cfg.ShowAllTags() {
		return []string{stateWithIcons}
	}
	return []string{size, age, stateWithIcons}
}

func (m Model) renderPendingGitRow(item *GitPRItem, selected bool, width int) string {
	titleStyle := itemStyle
	if selected {
		titleStyle = selectedSummaryStyle
	}
	leftBlock := fmt.Sprintf("      %s %s", pendingPRIcon.Render(), underlinedWhen(selected, titleStyle.Render(item.Title)))
	return alignRight(leftBlock, m.prTags(item, selected), width, selected) + "\n"
}

func renderJobStyleRow(icon, label, rightBlock string, selected bool, width int) string {
	rightColWidth := 26
	if width < 60 {
		rightColWidth = 20
	}
	leftWidth := max(width-rightColWidth, 15)
	label = ansi.Truncate(label, max(leftWidth-5, 5), "…")
	labelText := itemStyle.Render(label)
	if selected {
		labelText = selectedTitle(label)
	}
	leftBlock := "   " + icon + " " + labelText
	leftPadding := max(leftWidth-lipgloss.Width(leftBlock), 0)
	return leftBlock + safeRepeat(" ", leftPadding) + underlinedWhen(selected, rightBlock) + "\n"
}

func (m Model) renderDraftRow(draft *JobDraft, selected bool, width int) string {
	icon := amberDiamond.Render()
	if draft.HasRunDryRun && draft.ExitCode == 0 {
		icon = checkDone.Render()
	}
	var rightBlock string
	if m.jobRunning(draft.Name) {
		runningIndicator := m.renderJobRunningIndicator()
		rightBlock = fmt.Sprintf("%s   %s", jobActiveTagStyle.Render("#job"), runningIndicator)
	} else if draft.DryRunInFlight {
		rightBlock = fmt.Sprintf("%s   %s", jobActiveTagStyle.Render("#job"), m.renderDryRunIndicator())
	} else {
		statusText := "need to act"
		if draft.HasRunDryRun && draft.ExitCode == 0 {
			statusText = "success"
		}

		rightBlock = fmt.Sprintf("%s   %s", dimBlueText.Render("#job"), mutedStyle.Render(statusText))
	}
	return renderJobStyleRow(icon, draft.Name, rightBlock, selected, width)
}

func (m Model) noteTagCells(n *model.Note) (age, source string) {
	if m.noteRows == nil {
		return m.renderNoteTagCells(n)
	}
	key := m.noteTagKey(n)
	if cells, cached := m.noteRows.tags[key]; cached {
		return cells[0], cells[1]
	}
	if len(m.noteRows.tags) >= maxCachedNoteRows {
		clear(m.noteRows.tags)
	}
	age, source = m.renderNoteTagCells(n)
	m.noteRows.tags[key] = [2]string{age, source}
	return age, source
}

func noteSourceText(n *model.Note) string {
	sourceText := string(n.Source)
	if sourceText == "" {
		sourceText = "manual"
	}
	if !strings.HasPrefix(sourceText, "#") {
		sourceText = "#" + sourceText
	}
	return sourceText
}

func (m Model) renderNoteTagCells(n *model.Note) (age, source string) {
	source = dimBlueText.Render(noteSourceText(n))

	ageText := ""
	carriedOver := false
	switch {
	case n.Created.IsZero():
	case n.Status != model.StatusDone && !isSameDay(n.Created, m.currentDate) && n.Created.Before(m.currentDate):
		carriedOver = true
		ageText = fmt.Sprintf("%dd ago", daysAgo(n.Created, m.currentDate))
	case !isSameDay(n.Created, m.currentDate):
		ageText = n.Created.Format("Mon")
	default:
		ageText = n.Created.Format("15:04")
	}
	age = mutedStyle.Render(ageText)
	if carriedOver {
		age = yellowBadgeStyle.Render(ageText)
	}
	return age, source
}

func (m Model) inlineEditWidth(*model.Note) int {
	return max(m.width-3-3-4, 5)
}

func (m Model) renderRow(n *model.Note, selected bool, width int) string {
	if m.noteRows == nil || (selected && (m.mode == ViewInlineEdit || m.mode == ViewNotifyInput)) {
		return m.renderNoteRow(n, selected, width)
	}
	key := m.noteRowCacheKey(n, selected, width)
	if row, cached := m.noteRows.rows[key]; cached {
		return row
	}
	if len(m.noteRows.rows) >= maxCachedNoteRows {
		clear(m.noteRows.rows)
	}
	row := m.renderNoteRow(n, selected, width)
	m.noteRows.rows[key] = row
	return row
}

func (m Model) noteRowCacheKey(n *model.Note, selected bool, width int) noteRowKey {
	run, tracked := m.automationRuns[n.ID]
	notifyInterval := ""
	if entry, reminding := m.notifyEntries[n.ID]; reminding && noteIsActive(n) {
		notifyInterval = entry.Interval
	}
	return noteRowKey{
		tags:              m.noteTagKey(n),
		automationTracked: tracked,
		automationStatus:  run.Status,
		automated:         n.Automated,
		notifyInterval:    notifyInterval,
		summary:           n.Summary,
		selected:          selected,
		width:             width,
	}
}

func (m Model) noteRowTags(n *model.Note) []string {
	tags := m.noteRowTagsWithoutNotify(n)
	if notifyTag := m.notifyTag(n); notifyTag != "" {
		tags = append([]string{notifyTag}, tags...)
	}
	return tags
}

func (m Model) noteRowTagsWithoutNotify(n *model.Note) []string {
	age, source := m.noteTagCells(n)
	tags := []string{age, source}
	if automationTag := m.automationTag(n); automationTag != "" {
		tags = append([]string{automationTag}, tags...)
	}
	return tags
}

func (m Model) visibleNoteTags(n *model.Note, selected bool) []string {
	if selected || m.cfg.ShowAllTags() || n.Source == model.SourcePRReview || prReviewNoteURL(n) != "" {
		return m.noteRowTags(n)
	}
	if notifyTag := m.notifyTag(n); notifyTag != "" {
		return []string{notifyTag}
	}
	return nil
}

func (m Model) renderNoteRow(n *model.Note, selected bool, width int) string {
	return m.renderNoteRowWithTags(n, selected, width, m.visibleNoteTags(n, selected))
}

func (m Model) renderNoteRowWithTags(n *model.Note, selected bool, width int, tags []string) string {
	prefix := "   "

	boxChar := "☐"
	if n.Status == model.StatusDone {
		boxChar = "✔"
	}
	var leftBlock string
	switch {
	case selected && m.mode == ViewInlineEdit:
		leftBlock = fmt.Sprintf("%s%s %s", prefix, checkPending.Render(), m.inlineInput.View())
		tags = nil
	case selected && m.mode == ViewNotifyInput:
		leftBlock = fmt.Sprintf("%s%s %s", prefix, selectedPendingBox.Render(boxChar), selectedTitle(n.Summary))
		tags = append([]string{m.notifyInput.View()}, m.noteRowTagsWithoutNotify(n)...)
		if m.notifyNotice != "" {
			tags = []string{m.notifyInput.View(), staleStyle.Render(m.notifyNotice)}
		}
	case selected:
		boxStyle := selectedPendingBox
		if n.Status == model.StatusDone {
			boxStyle = selectedDoneBox
		}
		leftBlock = fmt.Sprintf("%s%s %s", prefix, boxStyle.Render(boxChar), selectedTitle(n.Summary))
	default:
		box := checkPending.Render()
		if n.Status == model.StatusDone {
			box = checkDone.Render()
		}
		leftBlock = fmt.Sprintf("%s%s %s", prefix, box, itemStyle.Render(n.Summary))
	}
	return alignRight(leftBlock, tags, width, selected) + "\n"
}
