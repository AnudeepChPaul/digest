package tui

import (
	"errors"
	"fmt"
	"maps"
	"sort"
	"strings"

	"github.com/AnudeepChPaul/digest/pkg/automation"
	"github.com/AnudeepChPaul/digest/pkg/config"
	"github.com/AnudeepChPaul/digest/pkg/model"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"
)

var (
	startAutomation = automation.StartBackground
	stopAutomation  = automation.Stop
)

const automationFailureLines = 8

const automationNavKeyPrefix = "automation:"

const reauthRetryHint = "Re-authenticate, then press r on the Draft tab to run it again."

func (m Model) automationRoot() string {
	return m.cfg.AutomationDir()
}

func (m Model) noteAutomation(n *model.Note) (config.AutomationSpec, bool) {
	if n == nil || n.ID == "" {
		return config.AutomationSpec{}, false
	}
	return automation.Match(m.cfg.AutomationList(), n)
}

func (m Model) automationTag(n *model.Note) string {
	if run, tracked := m.automationRuns[n.ID]; tracked && run.Status != automation.RunCreated {
		switch run.Status {
		case automation.RunRunning:
			return yellowBadgeStyle.Render("#draft")
		case automation.RunNeedsReauth, automation.RunFailed:
			return staleStyle.Render("#draft")
		}
		return tagStyle.Render("#draft")
	}
	if n.Automated != "" {
		return dimBlueText.Render("#" + strings.ToLower(n.Automated))
	}
	return ""
}

func (m Model) previewNote() (*model.Note, bool) {
	item, found := m.selectedNavItem()
	if !found || item.Note == nil || item.Note.ID == "" || item.Kind == KindPendingGit {
		return nil, false
	}
	return item.Note, true
}

func (m Model) noteRun(n *model.Note) (automation.Run, bool) {
	if n == nil {
		return automation.Run{}, false
	}
	run, tracked := m.automationRuns[n.ID]
	return run, tracked && run.Status != automation.RunCreated
}

func (m Model) hasDraftTab() bool {
	note, found := m.previewNote()
	if !found {
		return false
	}
	_, tracked := m.noteRun(note)
	return tracked
}

func (m Model) onDraftTab() bool {
	return m.previewTab == previewTabDraft && m.hasDraftTab()
}

func (m Model) noteCanStartAutomation(n *model.Note) bool {
	if n == nil || n.Automated != "" {
		return false
	}
	run, tracked := m.noteRun(n)
	return !tracked || run.Status != automation.RunRunning
}

func (m Model) canAutomate() bool {
	note, found := m.previewNote()
	if !found || note.Automated != "" {
		return false
	}
	if _, matched := m.noteAutomation(note); !matched {
		return false
	}
	run, tracked := m.noteRun(note)
	return !tracked || run.Status != automation.RunRunning
}

func (m Model) canRunDraft() bool {
	note, found := m.previewNote()
	if !found {
		return false
	}
	run, tracked := m.noteRun(note)
	return tracked && run.HasDraft && run.Status != automation.RunRunning
}

func (m Model) canDeleteDraft() bool {
	note, found := m.previewNote()
	if !found {
		return false
	}
	run, tracked := m.noteRun(note)
	return tracked && run.Status != automation.RunRunning
}

func (m Model) anyAutomationRunning() bool {
	for _, run := range m.automationRuns {
		if run.Status == automation.RunRunning {
			return true
		}
	}
	return false
}

func (m Model) automateFromPreview(tea.KeyMsg) (tea.Model, tea.Cmd) {
	note, found := m.previewNote()
	if !found || !m.canAutomate() {
		return m, nil
	}
	spec, _ := m.noteAutomation(note)
	return m.askAutomationConfirm(note.ID, spec.Name, automation.PhaseDraft), nil
}

func (m Model) runAutomationDraft(tea.KeyMsg) (tea.Model, tea.Cmd) {
	note, found := m.previewNote()
	if !found || !m.onDraftTab() || !m.canRunDraft() {
		return m, nil
	}
	run, _ := m.noteRun(note)
	return m.askAutomationConfirm(note.ID, run.Meta.Automation, automation.PhaseCreate), nil
}

func (m Model) askAutomationConfirm(noteID, automationName string, phase automation.Phase) Model {
	m.automationNoteID, m.automationName, m.automationPhase = noteID, automationName, phase
	m.automationReturnMode = ViewPreview
	m.mode = ViewAutomationConfirm
	return m
}

func (m Model) confirmAutomation(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = m.automationReturnMode
	if err := startAutomation(m.automationRoot(), m.automationNoteID, m.automationName, m.automationPhase); err != nil {
		m.showError("AUTOMATION ERROR", err)
		return m, nil
	}
	runs := maps.Clone(m.automationRuns)
	if runs == nil {
		runs = map[string]automation.Run{}
	}
	hasDraft := m.automationPhase == automation.PhaseCreate
	runs[m.automationNoteID] = automation.Run{Meta: automation.RunMeta{NoteID: m.automationNoteID, Automation: m.automationName, Phase: m.automationPhase}, Status: automation.RunRunning, HasDraft: hasDraft}
	m.automationRuns = runs
	if m.mode != ViewPreview {
		return m, m.ensureReviewPoll()
	}
	m.previewTab = previewTabDraft
	return m.refreshPreview(), m.ensureReviewPoll()
}

func (m Model) cancelAutomation(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = m.automationReturnMode
	return m, nil
}

func (m *Model) applyAutomationRuns(runs map[string]automation.Run) tea.Cmd {
	var reloadNotes bool
	selectedKey, selectedOccurrence := m.selectedNavKey()
	if runs == nil {
		runs = map[string]automation.Run{}
	}
	for noteID, previous := range m.automationRuns {
		if _, listed := runs[noteID]; !listed && previous.Status == automation.RunRunning {
			runs[noteID] = previous
		}
	}
	changed := len(runs) != len(m.automationRuns)
	for noteID, run := range runs {
		previous, tracked := m.automationRuns[noteID]
		changed = changed || !tracked || previous != run
		finished := tracked && previous.Status == automation.RunRunning && run.Status != automation.RunRunning
		switch {
		case run.Status == automation.RunCreated:
			if err := automation.Dismiss(m.automationRoot(), noteID); err != nil {
				m.showError("AUTOMATION ERROR", err)
			}
			delete(runs, noteID)
			reloadNotes = true
		case finished && run.Status == automation.RunNeedsReauth:
			m.showError("RE-AUTH NEEDED", errors.New(m.reauthHint(run)+"\n\n"+reauthRetryHint))
		case finished && run.Status == automation.RunFailed:
			m.showError("AUTOMATION FAILED", errors.New(m.automationFailure(noteID)))
		}
	}
	m.automationRuns = runs
	if changed {
		m.restoreSelection(selectedKey, selectedOccurrence)
		if currentKey, _ := m.selectedNavKey(); m.mode == ViewPreview && strings.HasPrefix(selectedKey, automationNavKeyPrefix) && currentKey != selectedKey {
			m.mode = ViewDashboard
		}
	}
	if changed && m.mode == ViewPreview {
		if m.previewTab == previewTabDraft && !m.hasDraftTab() {
			m.previewTab = previewTabDetails
		}
		m.updatePreviewViewport()
	}
	if reloadNotes {
		return m.loadNotesCmd
	}
	return nil
}

func (m Model) reauthHint(run automation.Run) string {
	if spec, found := automation.Find(m.cfg.AutomationList(), run.Meta.Automation); found && spec.ReauthHint != "" {
		return spec.ReauthHint
	}
	return fmt.Sprintf("%s could not sign in to its tools.", run.Meta.Automation)
}

func (m Model) automationLogTail(noteID string) string {
	var kept []string
	for _, line := range strings.Split(readFileTail(automation.LogPath(m.automationRoot(), noteID), logPeekBytes), "\n") {
		if strings.TrimSpace(line) != "" {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept[max(len(kept)-automationFailureLines, 0):], "\n")
}

func (m Model) automationFailure(noteID string) string {
	if tail := m.automationLogTail(noteID); tail != "" {
		return tail
	}
	return "The automation stopped without output. Press r on the Draft tab to run it again."
}

func draftMarkdown(draft string) string {
	var fields map[string]any
	if yaml.Unmarshal([]byte(draft), &fields) != nil {
		return "```yaml\n" + draft + "\n```"
	}
	var keys []string
	for key := range fields {
		if key != "summary" && key != "description" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	var text strings.Builder
	fmt.Fprintf(&text, "# %v\n\n", fields["summary"])
	for _, key := range keys {
		value := fields[key]
		if list, isList := value.([]any); isList {
			var items []string
			for _, item := range list {
				items = append(items, fmt.Sprint(item))
			}
			value = strings.Join(items, ", ")
		}
		fmt.Fprintf(&text, "- **%s:** %v\n", strings.ReplaceAll(key, "_", " "), value)
	}
	if description, _ := fields["description"].(string); description != "" {
		fmt.Fprintf(&text, "\n## Description\n\n%s\n", description)
	}
	return text.String()
}

func (m Model) draftTabMarkdown(note *model.Note) string {
	run, _ := m.noteRun(note)
	draft, draftErr := automation.LoadDraft(m.automationRoot(), note.ID)
	if run.Status == automation.RunRunning && run.Meta.Phase == automation.PhaseDraft {
		return fmt.Sprintf("# Drafting…\n\nClaude is drafting with **%s**. This tab updates when it finishes.", run.Meta.Automation)
	}
	if draftErr != nil {
		tail := m.automationLogTail(note.ID)
		if tail == "" {
			tail = "No output."
		}
		return "# Draft failed\n\nPress **x** to draft again.\n\n```\n" + tail + "\n```"
	}
	var status string
	switch run.Status {
	case automation.RunRunning:
		status = fmt.Sprintf("> Running **%s**… the draft is read-only until it finishes.\n\n", run.Meta.Automation)
	case automation.RunFailed:
		status = "> The last run failed. Press **r** to run it again, or **x** to draft again.\n\n"
	case automation.RunNeedsReauth:
		status = "> The last run needs re-auth. Re-authenticate, then press **r** to run it again.\n\n"
	}
	return status + draftMarkdown(draft)
}

func (m Model) draftText() string {
	note, found := m.previewNote()
	if !found {
		return ""
	}
	draft, _ := automation.LoadDraft(m.automationRoot(), note.ID)
	return draft
}

func (m Model) deleteAutomationDraft(tea.KeyMsg) (tea.Model, tea.Cmd) {
	note, found := m.previewNote()
	if !found || !m.onDraftTab() || !m.canDeleteDraft() {
		return m, nil
	}
	if err := automation.Dismiss(m.automationRoot(), note.ID); err != nil {
		m.showError("AUTOMATION ERROR", err)
		return m, nil
	}
	runs := maps.Clone(m.automationRuns)
	delete(runs, note.ID)
	m.automationRuns = runs
	m.previewTab = previewTabDetails
	return m.refreshPreview(), nil
}

func (m Model) editAutomationDraft(tea.KeyMsg) (tea.Model, tea.Cmd) {
	note, found := m.previewNote()
	if !found || !m.onDraftTab() || !m.canRunDraft() {
		return m, nil
	}
	draft, err := automation.LoadDraft(m.automationRoot(), note.ID)
	if err != nil {
		m.showError("AUTOMATION ERROR", fmt.Errorf("draft missing: %w", err))
		return m, nil
	}
	run, _ := m.noteRun(note)
	m.automationNoteID, m.automationName, m.automationNotice = note.ID, run.Meta.Automation, ""
	m.editor.SetValue(draft)
	m.editor.Focus()
	startEditorAtTop(m.editor)
	m.mode = ViewAutomationEdit
	return m, textarea.Blink
}

func (m Model) saveAutomationEdit(tea.KeyMsg) (tea.Model, tea.Cmd) {
	if err := automation.SaveDraft(m.automationRoot(), m.automationNoteID, m.editor.Value()); err != nil {
		m.automationNotice = err.Error()
		return m, nil
	}
	m.editor.Blur()
	m.mode = ViewPreview
	return m.refreshPreview(), nil
}

func (m Model) cancelAutomationEdit(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.editor.Blur()
	m.automationNotice = ""
	m.mode = ViewPreview
	return m, nil
}

func (m Model) renderAutomationConfirm(modalWidth int) string {
	prompt := fmt.Sprintf("Draft %s for this note?\n\nClaude writes the draft in the background.", m.automationName)
	if run, tracked := m.automationRuns[m.automationNoteID]; tracked && run.HasDraft && m.automationPhase == automation.PhaseDraft {
		prompt += "\nThis replaces the current draft."
	}
	if m.automationPhase == automation.PhaseCreate {
		prompt = fmt.Sprintf("Run %s with this draft?\n\nClaude creates it in the background.", m.automationName)
	}
	content := lipgloss.JoinVertical(lipgloss.Left,
		modalTitleStyle.Render(" AUTOMATE "),
		"",
		prompt,
		"",
		renderModalFooter(footerItemsFrom(automationConfirmBindings()), modalWidth-6),
	)
	return m.framedPopup(content, modalWidth)
}

func (m Model) renderAutomationEdit() string {
	modalWidth, _ := m.bragModalSize()
	title := modalTitleStyle.Render(fmt.Sprintf(" EDIT %s DRAFT ", strings.ToUpper(m.automationName)))
	parts := []string{title, "", m.editor.View(), ""}
	if m.automationNotice != "" {
		parts = append(parts, yellowBadgeStyle.Render(m.automationNotice), "")
	}
	parts = append(parts, renderModalFooter(footerItemsFrom(automationEditBindings()), modalWidth-6))
	return m.framedPopup(lipgloss.JoinVertical(lipgloss.Left, parts...), modalWidth)
}

func (m Model) noteDraftTabBindings() []keyBinding {
	return []keyBinding{
		newKeyBinding(actionSwitchPreviewTab, []string{"tab"}, "tab", "tabs"),
		newKeyBinding(actionEditAutomation, []string{"enter"}, "enter", "edit").shownWhen(m.canRunDraft()),
		newKeyBinding(actionAutomate, []string{"x"}, "x", "draft again").shownWhen(m.canAutomate()),
		newKeyBinding(actionRunAutomationDraft, []string{"r"}, "r", "run").shownWhen(m.canRunDraft()),
		newKeyBinding(actionDeleteAutomationDraft, []string{"d"}, "d", "delete draft").warning().shownWhen(m.canDeleteDraft()),
		newKeyBinding(actionCopyPreviewItem, []string{"ctrl+y"}, "ctrl+y", "copy").shownWhen(m.draftText() != ""),
		newKeyBinding(actionClosePreview, []string{"esc"}, "esc", "close"),
		newKeyBinding(actionPreviewPrevious, []string{"p"}, "p|n", "prev/next"),
		hiddenKeyBinding(actionPreviewNext, "n"),
	}
}

func automationConfirmBindings() []keyBinding {
	return []keyBinding{
		newKeyBinding(actionConfirmAutomation, []string{"y", "Y", "enter"}, "y|enter", "confirm"),
		newKeyBinding(actionCancelAutomation, []string{"esc", "n", "N"}, "esc", "cancel"),
	}
}

func automationEditBindings() []keyBinding {
	return append([]keyBinding{
		newKeyBinding(actionSaveAutomationEdit, []string{"ctrl+o"}, "ctrl+o", "save"),
		newKeyBinding(actionCancelAutomationEdit, []string{"esc"}, "esc", "cancel"),
	}, editorHalfPageBindings()...)
}

func (m Model) runningAutomations() []automation.Run {
	var running []automation.Run
	for _, run := range m.automationRuns {
		if run.Status == automation.RunRunning {
			running = append(running, run)
		}
	}
	sort.Slice(running, func(i, j int) bool { return running[i].Meta.NoteID < running[j].Meta.NoteID })
	return running
}

func (m Model) automationJobLabel(run automation.Run) string {
	summary := run.Meta.NoteID
	for _, note := range m.notes {
		if note.ID == run.Meta.NoteID {
			summary = note.Summary
			break
		}
	}
	return fmt.Sprintf("%s %s: %s", run.Meta.Automation, run.Meta.Phase, summary)
}

func (m Model) automationJobRunning(run *automation.Run) bool {
	return m.automationRuns[run.Meta.NoteID].Status == automation.RunRunning
}

func (m Model) renderAutomationRunRow(run automation.Run, selected bool, width int) string {
	label := "running..."
	if run.Meta.Phase == automation.PhaseDraft {
		label = "drafting..."
	}
	return renderJobStyleRow(amberDiamond.Render(), m.automationJobLabel(run), m.renderPulseIndicator(label), selected, width)
}

func (m Model) automationRunPreview(run automation.Run) string {
	status := "FAILED"
	if m.automationJobRunning(&run) {
		status = "RUNNING"
	}
	logText := m.automationLogTail(run.Meta.NoteID)
	if logText == "" {
		logText = "(no log output yet)"
	}
	return fmt.Sprintf("# Automation: %s (%s)\n\n```\n%s\n```", m.automationJobLabel(run), status, logText)
}

func (m Model) stopAutomationRun(run *automation.Run) (tea.Model, tea.Cmd) {
	if !m.automationJobRunning(run) {
		return m, nil
	}
	if err := stopAutomation(m.automationRoot(), run.Meta.NoteID); err != nil {
		m.showError("AUTOMATION ERROR", err)
		return m, nil
	}
	stopped := *run
	stopped.Status = automation.RunFailed
	runs := maps.Clone(m.automationRuns)
	runs[stopped.Meta.NoteID] = stopped
	m.automationRuns = runs
	if m.mode == ViewPreview {
		m.mode = ViewDashboard
	}
	m.clampScreenSelection()
	return m, nil
}
