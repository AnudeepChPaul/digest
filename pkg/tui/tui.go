package tui

import (
	"context"
	"fmt"
	"github.com/achandrapaul/digest/pkg/appstate"
	"os/exec"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/achandrapaul/digest/pkg/automation"

	"github.com/achandrapaul/digest/pkg/brag"
	"github.com/achandrapaul/digest/pkg/config"
	"github.com/achandrapaul/digest/pkg/model"
	"github.com/achandrapaul/digest/pkg/notify"
	"github.com/achandrapaul/digest/pkg/review"
	"github.com/achandrapaul/digest/pkg/running"
	"github.com/achandrapaul/digest/pkg/sourcecontrol"
	"github.com/achandrapaul/digest/pkg/store"
	"github.com/achandrapaul/digest/pkg/tui/textarea"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type ViewMode int

const (
	ViewDashboard ViewMode = iota
	ViewEdit
	ViewSearch
	ViewDeleteConfirm
	ViewPreview
	ViewInlineEdit
	ViewGitDetails
	ViewArchived
	ViewError
	ViewReviewConfirm
	ViewReviewRunConfirm
	ViewRejectComment
	ViewHelp
	ViewBragList
	ViewBragView
	ViewBragConfirm
	ViewBragEdit
	ViewAutomationEdit
	ViewAutomationConfirm
	ViewActionMenu
	ViewLinkMenu
	ViewNotifyInput
	ViewSetup
	ViewSetupDiscard
	ViewRecreateConfirm
	ViewRecreateRow
)

type NavItemKind int

const (
	KindGitRepo NavItemKind = iota
	KindYesterdayDone
	KindTodayNote
	KindTodayDone
	KindPendingGit
	KindCarriedNote
	KindJobDraft
	KindReviewRun
	KindBragRun
	KindAutomationRun
	KindMyPR
)

type GitPRItem = sourcecontrol.PRItem

type GitRepoStat struct {
	Name     string
	Path     string
	Commits  int
	Reviewed int
	Assigned int
	Items    []GitPRItem
}

type NavItem struct {
	Kind          NavItemKind
	Note          *model.Note
	GitRepo       *GitRepoStat
	Draft         *JobDraft
	PendingGitPR  *GitPRItem
	ReviewRun     *review.ReviewRun
	BragRun       *brag.Run
	AutomationRun *automation.Run
	MyPR          *review.QueuedPR
}

type bannerWaveTickMsg struct{}
type syncPulseTickMsg struct{}
type ctrlCResetMsg struct{ pressSequence int }
type localReviewState struct {
	status         review.RunStatus
	finishedAt     time.Time
	finished       bool
	pid            int
	recommendation string
	cloned         bool
}

func safeRepeat(s string, count int) string {
	if count <= 0 {
		return ""
	}
	return strings.Repeat(s, count)
}

var openURL = func(url string) error {
	if url == "" {
		return nil
	}
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	default:
		cmd = "xdg-open"
		args = []string{url}
	}
	_, err := startReaped(exec.Command(cmd, args...))
	return err
}

func (m *Model) openLink(url string) {
	if err := openURL(url); err != nil {
		m.showError("BROWSER", err)
	}
}

func (m *Model) copyText(text string) {
	if text == "" {
		return
	}
	if err := copyToClipboard(text); err != nil {
		m.postMessage(messageSourceClipboard, messageError, err.Error())
		return
	}
	m.postMessage(messageSourceClipboard, messageSuccess, "copied")
}

func startReaped(cmd *exec.Cmd) (<-chan error, error) {
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	return exited, nil
}

var copyToClipboard = func(text string) error {
	if text == "" {
		return nil
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "windows":
		cmd = exec.Command("cmd", "/c", "clip")
	default:
		if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd = exec.Command("wl-copy")
		} else if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else {
			return fmt.Errorf("no clipboard utility found")
		}
	}
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

func daysAgo(from, to time.Time) int {
	y1, m1, d1 := from.Local().Date()
	y2, m2, d2 := to.Local().Date()
	t1 := time.Date(y1, m1, d1, 0, 0, 0, 0, time.Local)
	t2 := time.Date(y2, m2, d2, 0, 0, 0, 0, time.Local)
	days := int(t2.Sub(t1).Hours() / 24)
	if days < 1 {
		days = 1
	}
	return days
}

type Model struct {
	git   gitState
	mode  ViewMode
	cfg   *config.Config
	store *store.NoteStore
	notes []*model.Note

	sessionCtx         context.Context
	startupNotesErr    error
	appState           appstate.State
	appStateKnown      bool
	cancelSession      context.CancelFunc
	searchCache        *searchMemo
	reviewReports      *reviewReportMemo
	contentVersion     int
	scrollPending      bool
	frames             *dashboardFrameCache
	bragSaved          *bragSavedMemo
	dryRunLogStamps    map[string]string
	selected           int
	scrollOffset       int
	archivedSelected   int
	ctrlCCount         int
	ctrlCPressSequence int
	currentNote        *model.Note
	currentDate        time.Time

	jobDryRunOutputs   map[string]string
	jobDryRunExitCodes map[string]int
	jobDryRunHasRun    map[string]bool

	archivedSelectedMap  map[int]bool
	deleteTargetNotes    []*model.Note
	deleteReturnMode     ViewMode
	jobToExecute         string
	jobToAbort           string
	bragRunToStop        *brag.Run
	automationRunToStop  *automation.Run
	jobRunStopsDryRun    bool
	jobDryRunToStart     string
	staleJobToClear      string
	runToRetry           *NavItem
	draftToDelete        string
	discardingEdit       bool
	jobConfirmError      string
	previewNotice        string
	editorStartText      string
	editorNotice         string
	confirmTitle         string
	confirmPrompt        string
	restoreOnConfirm     bool
	actionMenuReturnMode ViewMode
	linkMenuItems        []string
	linkMenuSelected     int
	linkMenuReturnMode   ViewMode

	syncPulseFrame   int
	bannerWaveActive bool
	bannerWaveFrame  int

	syncPulseRunning      bool
	jobLogRunning         bool
	jobLogStamp           string
	runStatePolling       bool
	runningJobPIDs        map[string]int
	dryRunsInFlight       map[string]bool
	localReviews          map[string]localReviewState
	previewPollStamp      previewStamp
	previewJobLogFinished bool
	previewFindingsCount  int
	bragStates            map[string]bragRowState
	historyRequested      map[string]bool

	initialSelectionPending bool
	selectAfterReload       string
	awaitingNewNoteSave     bool
	browsedNotes            []*model.Note
	closedThisWeekElsewhere int
	missingSave             *failedNoteSave
	missingSaveReturnMode   ViewMode
	helpReturnMode          ViewMode
	reviewRuns              []review.ReviewRun
	reviewRunAction         string
	reviewRunTarget         review.PRRef
	reviewRunReturnMode     ViewMode
	tagCells                *rowTagCellCache
	noteRows                *noteRowCache
	popupMemo               *framedPopupMemo
	bragRuns                []brag.Run
	bragSelected            int
	bragExpanded            map[int]bool
	bragPeriod              brag.Period
	bragEntry               *brag.Brag
	bragRegenerate          bool
	bragConfirmReturn       ViewMode
	bragNotice              string
	automationRuns          map[string]automation.Run
	automationNoteID        string
	automationReturnMode    ViewMode
	actionMenuNoteID        string
	actionMenuItems         []noteAction
	actionMenuSelected      int
	notifyInput             *textinput.Model
	notifyNotice            string
	notifyEntries           map[string]notify.Entry
	hintGeneration          int
	hintVisible             bool
	editorRevision          int
	editorCache             *editorViewCache
	prActionFromDashboard   bool
	setup                   *setupState
	setupInput              *textinput.Model
	configPath              string
	automationName          string
	automationPhase         automation.Phase
	automationNotice        string

	previewViewport  viewport.Model
	previewPlainText string
	archivedViewport viewport.Model

	editor      *textarea.Model
	searchInput *textinput.Model

	searchSelected   int
	searchScroll     int
	searchPreviewID  string
	searchPreviewing bool
	searchNotice     string
	editReturnMode   ViewMode
	inlineInput      *textinput.Model

	previewTab     int
	reviewCursor   int
	reviewSelected map[int]bool
	reviewEvent    review.Event
	reviewBody     string
	reviewNotice   string
	rejectInput    *textarea.Model
	contextCache   map[string]string
	reviewPolling  bool

	errorTitle           string
	messages             []appMessage
	messageExpiryPending bool
	errorLines           []string
	errorReturnMode      ViewMode
	messageLogExpanded   bool
	messageLogViewport   viewport.Model
	screenError          string
	screenErrorMode      ViewMode

	width  int
	height int
}

func NewModel(cfg *config.Config, startupErr error) Model {
	ti := textinput.New()
	ti.Placeholder = "Search notes… (tag:pr-review, date:7d, date:2w, date:3m, date:24-12-2026)"

	ii := textinput.New()
	ii.Prompt = ""

	notifyInput := textinput.New()
	notifyInput.Prompt = "@notify:"
	notifyInput.Placeholder = "1h"
	notifyInput.CharLimit = 16
	notifyInput.Width = lipgloss.Width(notifyInput.Prompt + notifyInput.Placeholder)

	ta := textarea.New()
	ta.Placeholder = "First line: Summary\n\nRemaining lines: Body..."
	ta.ShowLineNumbers = false
	ta.MaxHeight = 0
	ta.Cursor.SetMode(cursor.CursorStatic)

	textStyle := lipgloss.NewStyle().Foreground(colourText).UnsetBackground()
	placeholderStyle := lipgloss.NewStyle().UnsetBackground()

	ta.FocusedStyle.Base = textStyle
	ta.FocusedStyle.Text = textStyle
	ta.FocusedStyle.Placeholder = placeholderStyle
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.CursorLineNumber = lipgloss.NewStyle()
	ta.FocusedStyle.LineNumber = lipgloss.NewStyle()
	ta.FocusedStyle.Prompt = lipgloss.NewStyle()
	ta.FocusedStyle.EndOfBuffer = lipgloss.NewStyle()

	ta.BlurredStyle = ta.FocusedStyle
	disableTextareaDeleteShortcuts(&ta)
	for _, input := range []*textinput.Model{&ti, &ii, &notifyInput} {
		disableInputDeleteShortcuts(input)
	}

	storePath := cfg.NotesDir()
	digestRoot = cfg.Root()

	rejectArea := textarea.New()
	rejectArea.Placeholder = "Explain what needs to change..."
	rejectArea.ShowLineNumbers = false
	rejectArea.MaxHeight = 0
	rejectArea.FocusedStyle = ta.FocusedStyle
	rejectArea.BlurredStyle = ta.FocusedStyle
	disableTextareaDeleteShortcuts(&rejectArea)

	m := Model{
		initialSelectionPending: true,
		rejectInput:             &rejectArea,
		reviewSelected:          make(map[int]bool),
		contextCache:            make(map[string]string),
		mode:                    ViewDashboard,
		cfg:                     cfg,
		store:                   store.New(storePath),
		searchInput:             &ti,
		inlineInput:             &ii,
		notifyInput:             &notifyInput,
		frames:                  newDashboardFrameCache(),
		noteRows:                newNoteRowCache(),
		popupMemo:               &framedPopupMemo{},
		editor:                  &ta,
		editorCache:             &editorViewCache{},
		searchCache:             &searchMemo{},
		reviewReports:           &reviewReportMemo{},
		git:                     gitState{loadingCommits: cfg != nil && cfg.DailyCommitsEnabled()},
		currentDate:             time.Now(),
		archivedSelectedMap:     make(map[int]bool),
		jobDryRunOutputs:        make(map[string]string),
		jobDryRunExitCodes:      make(map[string]int),
		jobDryRunHasRun:         make(map[string]bool),
		bannerWaveActive:        true,
	}
	m.sessionCtx, m.cancelSession = context.WithCancel(context.Background())
	m.notes, m.startupNotesErr = m.store.ListDashboard(m.currentDate, m.previousNoteDay())
	m.loadAppState()
	m.notifyEntries = listNotifyEntries(cfg.Root()).entries
	m.git.fetchedPreviousDay = m.previousNoteDay()
	m.git.loadingGit = false
	if cfg.GitEnabled() {
		m.loadGitOnStartup()
	}
	m.refreshDryRunResults()
	m.refreshReviewRuns()
	m.git.commitsCtx, m.git.commitsCancel = context.WithCancel(context.Background())
	m.refreshBragRuns()
	m.applyAutomationRuns(automation.ListRuns(m.automationRoot()))
	m.reviewPolling = m.anyReviewRunning() || m.anyBragRunning() || m.anyAutomationRunning()
	m.syncPulseRunning = m.headerAnimating()
	m.runStatePolling = m.isAnyDryRunInFlight() || m.isAnyJobRunning()
	if startupErr != nil {
		m.showError("CONFIG ERROR", startupErr)
	}
	return m
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.startupNotesCmd(), m.rebuildAppStateCmd(), tickBannerWaveCmd(), m.loadCommitsCmd(), m.startupHintCmd()}
	if m.syncPulseRunning {
		cmds = append(cmds, tickSyncPulseCmd())
	}
	if m.git.syncOnLoad {
		cmds = append(cmds, m.gitFetchCmd())
	} else if m.git.loadingMyPRs {
		cmds = append(cmds, m.myPRsFetchCmd())
	}
	if m.runStatePolling {
		cmds = append(cmds, tickRunStatePollCmd())
	}
	if m.reviewPolling {
		cmds = append(cmds, m.tickReviewPollCmd())
	}
	if m.cfg.GitEnabled() && m.cfg.GitAutoSyncInterval > 0 {
		cmds = append(cmds, autoSyncTickCmd(m.cfg.GitAutoSyncInterval))
	}
	return tea.Batch(cmds...)
}

func (m *Model) showError(title string, errs ...error) {
	var lines []string
	for _, err := range errs {
		if err != nil {
			lines = append(lines, err.Error())
		}
	}
	if len(lines) == 0 {
		return
	}
	m.postMessage(title, messageError, strings.Join(lines, "\n"))
}

func navItemKey(item NavItem) string {
	switch {
	case item.Kind == KindJobDraft && item.Draft != nil:
		return "job:" + item.Draft.Name
	case item.Kind == KindReviewRun && item.ReviewRun != nil:
		return "review:" + item.ReviewRun.Meta.Ref.URL
	case item.Kind == KindBragRun && item.BragRun != nil:
		return "brag:" + item.BragRun.Meta.ID
	case item.Kind == KindAutomationRun && item.AutomationRun != nil:
		return automationNavKeyPrefix + item.AutomationRun.Meta.NoteID
	case item.MyPR != nil:
		return "mine:" + item.MyPR.Ref.URL
	case item.PendingGitPR != nil:
		return "pr:" + item.PendingGitPR.URL
	case item.GitRepo != nil:
		return "repo:" + item.GitRepo.Name
	case item.Note != nil:
		return fmt.Sprintf("note:%d:%s", item.Kind, item.Note.ID)
	}
	return ""
}

func (m Model) selectedNavKey() (string, int) {
	items := m.allNavItems()
	if m.selected < 0 || m.selected >= len(items) {
		return "", 0
	}
	key := navItemKey(items[m.selected])
	occurrence := 0
	for _, item := range items[:m.selected] {
		if navItemKey(item) == key {
			occurrence++
		}
	}
	return key, occurrence
}

var launchKinds = map[string][]NavItemKind{
	config.SelectionNotesToday:     {KindTodayNote, KindCarriedNote},
	config.SelectionNotesYesterday: {KindYesterdayDone},
}

func (m *Model) selectLaunchItem() {
	items := m.allNavItems()
	for _, preferredKind := range launchKinds[m.cfg.SelectionDefault] {
		if index := slices.IndexFunc(items, func(item NavItem) bool { return item.Kind == preferredKind }); index >= 0 {
			m.selected = index
			return
		}
	}
	m.selected = 0
}

func (m *Model) selectNoteByID(noteID string) {
	items := m.allNavItems()
	if index := slices.IndexFunc(items, func(item NavItem) bool { return item.Note != nil && item.Note.ID == noteID }); index >= 0 {
		m.selected = index
	}
}

func (m *Model) restoreSelection(key string, occurrence int) {
	items := m.allNavItems()
	if key != "" {
		seen := 0
		for index, item := range items {
			if navItemKey(item) != key {
				continue
			}
			if seen == occurrence {
				m.selected = index
				return
			}
			seen++
		}
	}
	if m.selected >= len(items) {
		m.selected = max(len(items)-1, 0)
	}
}

func (m Model) allNavItems() []NavItem {
	data := m.dashboardData()
	items := make([]NavItem, 0, data.jobsEnd)
	items = notesSection{}.appendNavItems(m, items, data)
	items = gitSection{}.appendNavItems(m, items, data)
	return jobsSection{}.appendNavItems(m, items, data)
}

func (m *Model) updatePreviewViewport() {
	item, found := m.selectedNavItem()
	if !found {
		return
	}

	_, innerWidth, innerHeight := previewModalSize(m.width, m.height)

	var mdContent string
	if item.Kind == KindGitRepo {
		commitsLine := ""
		if m.cfg.DailyCommitsEnabled() {
			commitsLine = fmt.Sprintf("- Commits: %d\n", item.GitRepo.Commits)
		}
		mdContent = renderMarkdown(fmt.Sprintf("# Repository Activity: %s\n%s- PRs Reviewed: %d\n- PRs Assigned: %d\n\nPress **[Tab]** on this repository item to view and open PRs directly in your browser.", item.GitRepo.Name, commitsLine, item.GitRepo.Reviewed, item.GitRepo.Assigned), innerWidth)
	} else if item.Kind == KindPendingGit && item.PendingGitPR != nil {
		m.setPRPreviewContent(item.PendingGitPR, innerWidth, innerHeight)
		return
	} else if item.Kind == KindJobDraft && item.Draft != nil {
		m.refreshJobStates()
		running := m.jobRunning(item.Draft.Name)
		logText, isDryRunOutput := latestJobOutput(item.Draft.Name, m.jobDryRunOutputFor(item.Draft.Name))
		m.previewJobLogFinished = logText != "" && !isDryRunOutput
		if !running && item.Draft.DryRunInFlight {
			mdContent = renderMarkdown(fmt.Sprintf("# Job: %s (DRY RUN IN PROGRESS)\n\nDry run in progress...", item.Draft.Name), innerWidth)
		} else if logText != "" {
			statusHeader := "FINISHED / LOG OUTPUT"
			if running {
				statusHeader = "LIVE EXECUTION LOG"
			} else if isDryRunOutput {
				statusHeader = "DRY RUN ANALYSIS"
			}
			mdContent = runPreview{heading: fmt.Sprintf("# Job: %s (%s)", item.Draft.Name, statusHeader), log: logText}.render(innerWidth)
		} else {
			keyGuide := "(r) run"
			if jobHasDryRun(item.Draft.DryRunCommand) {
				keyGuide = "(d) dry-run " + keyGuide
			}
			mdContent = renderMarkdown(fmt.Sprintf("# Job: %s\n\nNo dry-run analysis or log output available.\n%s", item.Draft.Name, keyGuide), innerWidth)
		}
	} else if item.Kind == KindReviewRun && item.ReviewRun != nil {
		mdContent = reviewRunPreview(m.reviewRoot(), *item.ReviewRun).render(innerWidth)
	} else if item.Kind == KindBragRun && item.BragRun != nil {
		mdContent = bragRunPreview(m.bragRoot(), *item.BragRun).render(innerWidth)
	} else if item.Kind == KindAutomationRun && item.AutomationRun != nil {
		mdContent = m.automationRunPreview(*item.AutomationRun).render(innerWidth)
	} else if item.Kind == KindMyPR && item.MyPR != nil {
		mdContent = renderMarkdown(myPRDetailsMarkdown(*item.MyPR), innerWidth)
	} else if item.Note != nil && m.onDraftTab() {
		mdContent = renderMarkdown(m.draftTabMarkdown(item.Note), innerWidth)
	} else if item.Note != nil {
		fullText := fmt.Sprintf("# %s", item.Note.Summary)
		if strings.TrimSpace(item.Note.Body) != "" {
			fullText += fmt.Sprintf("\n\n%s", item.Note.Body)
		}
		mdContent = renderMarkdown(fullText, innerWidth)
	}

	m.previewViewport = viewport.New(innerWidth, innerHeight)
	m.previewViewport.SetContent(mdContent)
	m.previewPlainText = plainText(mdContent)
	if item.Kind == KindJobDraft || item.Kind == KindReviewRun || item.Kind == KindBragRun || item.Kind == KindAutomationRun {
		m.previewViewport.GotoBottom()
	}
}

type runPreview struct {
	heading string
	log     string
}

func (preview runPreview) render(width int) string {
	logTail := lipgloss.NewStyle().Width(width).Render(lastLines(strings.TrimSpace(preview.log), jobLogTailLines))
	return renderMarkdown(preview.heading, width) + "\n\n" + logTail
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if !isAnimationTick(msg) {
		m.contentVersion++
	}
	next, cmd := m.handleMsg(msg)
	updated, isModel := next.(Model)
	if !isModel {
		return next, cmd
	}
	updated.clearScreenErrorOffScreen()
	if updated.scrollPending {
		updated.settleScroll()
	}
	if updated.browsedNotes != nil && !updated.browsing() {
		updated.browsedNotes = nil
		cmd = tea.Batch(cmd, updated.closedThisWeekCmd())
	}
	if updated.git.prAlertsDue {
		updated.git.prAlertsDue = false
		cmd = tea.Batch(cmd, updated.prAlertsCmd())
	}
	if !updated.messageExpiryPending {
		if expiry := updated.messageExpiryCmd(); expiry != nil {
			updated.messageExpiryPending = true
			cmd = tea.Batch(cmd, expiry)
		}
	}
	return updated, cmd
}

func isAnimationTick(msg tea.Msg) bool {
	switch msg.(type) {
	case bannerWaveTickMsg, syncPulseTickMsg:
		return true
	}
	return false
}

type messageApplier func(Model, tea.Msg) (tea.Model, tea.Cmd, bool)

var messageAppliers []messageApplier

func init() {
	messageAppliers = []messageApplier{
		headerSection{}.ApplyMessage,
		notesSection{}.ApplyMessage,
		gitSection{}.ApplyMessage,
		jobsSection{}.ApplyMessage,
	}
}

func messageHandled(next tea.Model, cmd tea.Cmd) (tea.Model, tea.Cmd, bool) {
	return next, cmd, true
}

func (m Model) handleMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	for _, applyMessage := range messageAppliers {
		if next, cmd, handled := applyMessage(m, msg); handled {
			return next, cmd
		}
	}
	switch msg := msg.(type) {
	case searchExportedMsg:
		return m.handleSearchExported(msg)

	case ctrlCResetMsg:
		if msg.pressSequence == m.ctrlCPressSequence {
			m.ctrlCCount = 0
		}
		return m, nil

	case setupSavedMsg:
		return m.applySetupSaved(msg)

	case spinner.TickMsg:
		return m.tickSetupSpinner(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		_, editorWidth, editorHeight := previewModalSize(msg.Width, msg.Height)
		m.editor.SetWidth(editorWidth)
		m.editor.SetHeight(editorHeight)
		if m.mode == ViewInlineEdit && m.currentNote != nil {
			m.inlineInput.Width = m.inlineEditWidth(m.currentNote)
		}
		m.updateScrollOffset()
		if m.mode == ViewPreview {
			m.updatePreviewViewport()
		}

	case loadNotesMsg:
		notesSection{}.storeLoadedNotes(&m, msg)
		refetchPreviousDay := gitSection{}.refreshPreviousDay(&m)
		return notesSection{}.finishLoadedNotes(m, msg, refetchPreviousDay)

	case appStateRebuiltMsg:
		return m.applyRebuiltAppState(msg)

	case tea.KeyMsg:
		next, cmd := m.handleKey(msg)
		updated, isModel := next.(Model)
		if !isModel {
			return next, cmd
		}
		return updated, tea.Batch(cmd, updated.restartHintTimer())

	case hintIdleMsg:
		m.hintVisible = msg.generation == m.hintGeneration && m.mode == ViewDashboard && m.keyHintPill() != ""
		return m, nil
	}

	return m, nil
}

func isSameDay(t1, t2 time.Time) bool {
	y1, m1, d1 := t1.Local().Date()
	y2, m2, d2 := t2.Local().Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

func (m Model) renderSubSection(title string, count int, showCount bool, isActive bool) string {
	baseStyle := subSectionStyle
	if isActive {
		baseStyle = baseStyle.Underline(true)
	}
	if showCount {
		countStr := mutedStyle.Render(fmt.Sprintf("(%d)", count))
		return fmt.Sprintf("%s %s", baseStyle.Render("/ "+title), countStr)
	}
	return baseStyle.Render("/ " + title)
}

func (m Model) openPreview(item NavItem) (tea.Model, tea.Cmd) {
	m.previewNotice = ""
	if item.Kind == KindGitRepo && item.GitRepo != nil {
		m.git.gitPopupRepo = item.GitRepo
		m.git.gitPopupTab = 0
		m.git.gitPopupSelected = 0
		m.mode = ViewGitDetails
		return m, nil
	}
	m.searchPreviewing = false
	m.resetReviewView()
	m.updatePreviewViewport()
	m.mode = ViewPreview
	if item.PendingGitPR != nil {
		cmds := []tea.Cmd{m.relatedHistoryCmd(item.PendingGitPR), m.changesSinceReviewCmd(item.PendingGitPR)}
		if m.anyReviewRunning() {
			cmds = append(cmds, m.ensureReviewPoll())
		}
		return m, tea.Batch(cmds...)
	}
	if item.Kind == KindJobDraft && item.Draft != nil && m.jobRunning(item.Draft.Name) {
		m.jobLogStamp = jobLogStampFor(item.Draft.Name, true)
		return m, m.ensureJobLogRefresh()
	}
	return m, nil
}

const sectionGap = "\n\n"

var dashboardContentBuilder = Model.buildDashboardContent

func (m Model) dashboardContent() (content string, selectedLine int) {
	return dashboardContentBuilder(m)
}

func (m Model) buildDashboardContent() (content string, selectedLine int) {
	builder := &dashboardBuilder{selected: m.selected, innerWidth: m.width - 4, rowWidth: m.width - 3}
	if m.frames != nil {
		builder.board.Grow(m.frames.lastContentLength + 1024)
	}
	m.tagCells = &rowTagCellCache{prs: map[*GitPRItem][3]string{}}
	data := m.dashboardData()

	notesSection{}.Render(m, builder, data)
	if m.cfg.GitEnabled() {
		gitSection{}.Render(m, builder, data)
	}
	jobsSection{}.Render(m, builder, data)

	if m.frames != nil {
		m.frames.lastContentLength = builder.board.Len()
	}
	return builder.board.String(), builder.selectedLine
}

func (m Model) visibleScrollOffset(frame *dashboardFrame) int {
	bodyHeight := m.frameBodyHeight(frame)
	lineCount := strings.Count(frame.content, "\n") + 1
	scrollOffset := max(m.scrollOffset, 0)
	if scrollOffset > lineCount-bodyHeight && lineCount > bodyHeight {
		scrollOffset = lineCount - bodyHeight
	}
	return scrollOffset
}

func (m Model) renderFrameBody(frame *dashboardFrame) string {
	bodyHeight := m.frameBodyHeight(frame)
	lines := strings.Split(frame.content, "\n")
	scrollOffset := m.visibleScrollOffset(frame)

	endIdx := scrollOffset + bodyHeight
	if endIdx > len(lines) {
		endIdx = len(lines)
	}

	visibleLines := lines[scrollOffset:endIdx]
	for i := len(visibleLines); i < bodyHeight; i++ {
		visibleLines = append(visibleLines, "")
	}

	var framed []string
	for _, l := range visibleLines[:bodyHeight] {
		pad := m.width - lipgloss.Width(l) - 2
		framed = append(framed, fmt.Sprintf("│%s%s│", l, safeRepeat(" ", pad)))
	}

	return strings.Join(framed, "\n")
}

func Run(cfg *config.Config, startupErr error, configPath string, startInSetup bool) error {
	model := NewModel(cfg, startupErr)
	model.configPath = configPath
	if startInSetup {
		model = model.startSetup(configPath)
	}
	release, err := running.Mark(cfg.TUIMarkerPath())
	if err != nil {
		return err
	}
	defer release()
	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

var noteLinkPattern = regexp.MustCompile(`https?://[^\s<>()"']+`)
var pullRequestPathPattern = regexp.MustCompile(`/[^/\s]+/pull/\d+`)

func prReviewNoteURL(note *model.Note) string {
	if note == nil || note.Source != model.SourcePRReview {
		return ""
	}
	return notePullRequestURL(note)
}

func notePullRequestURL(note *model.Note) string {
	links := noteLinkPattern.FindAllString(note.Body, -1)
	for i := len(links) - 1; i >= 0; i-- {
		if pullRequestPathPattern.MatchString(links[i]) {
			return links[i]
		}
	}
	return ""
}
