package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/automation"

	"github.com/AnudeepChPaul/digest/pkg/brag"
	"github.com/AnudeepChPaul/digest/pkg/config"
	"github.com/AnudeepChPaul/digest/pkg/habit"
	"github.com/AnudeepChPaul/digest/pkg/model"
	"github.com/AnudeepChPaul/digest/pkg/notify"
	"github.com/AnudeepChPaul/digest/pkg/review"
	"github.com/AnudeepChPaul/digest/pkg/sourcecontrol"
	"github.com/AnudeepChPaul/digest/pkg/store"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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
	ViewSearchPreview
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

type JobDraft struct {
	Name           string
	DryRunCommand  string
	Command        string
	ExitCode       int
	HasRunDryRun   bool
	DryRunInFlight bool
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

type PendingRepoGroup struct {
	Name  string
	Items []GitPRItem
}

type bannerWaveTickMsg struct{}
type syncPulseTickMsg struct{}
type ctrlCResetMsg struct{}
type autoSyncTickMsg time.Time

type daySyncDueMsg struct {
	generation int
}

const daySyncDelay = 2 * time.Second

type jobLogTickMsg struct{}

const jobLogTailLines = 400

type runStatePollTickMsg struct{}

type localReviewState struct {
	status         review.RunStatus
	finishedAt     time.Time
	finished       bool
	pid            int
	recommendation string
	cloned         bool
}

type jobAbortedMsg struct {
	jobName string
	err     error
}

type notesSavedMsg struct {
	errs     []error
	savedIDs []string
}

type gitDay = sourcecontrol.Day

const (
	gitDayToday     = sourcecontrol.Today
	gitDayYesterday = sourcecontrol.Yesterday
)

const gitSectionCount = sourcecontrol.SectionCount

type gitDaySectionMsg struct {
	generation  int
	day         gitDay
	date        string
	reviewed    []GitPRItem
	reviews     []review.ActivityPR
	details     map[string]json.RawMessage
	failedHosts []string
	err         error
	sections    <-chan sourcecontrol.Section
}

type gitMyPRsMsg struct {
	generation  int
	partOfSync  bool
	prs         []review.QueuedPR
	closed      map[string]string
	failedHosts []string
	err         error
	sections    <-chan sourcecontrol.Section
}

type gitPendingMsg struct {
	generation  int
	startedAt   time.Time
	pending     []GitPRItem
	details     map[string]json.RawMessage
	failedHosts []string
	err         error
	sections    <-chan sourcecontrol.Section
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
	mode             ViewMode
	cfg              *config.Config
	store            *store.NoteStore
	notes            []*model.Note
	todayGitRepos    []*GitRepoStat
	yesterdayGitRepo []*GitRepoStat
	pendingGitAction []GitPRItem
	loadingGit       bool
	fetchGeneration  int

	gitSectionsPending int
	gitSectionDates    map[string]string
	syncErrors         map[string]string
	sessionCtx         context.Context
	startupNotesErr    error
	cancelSession      context.CancelFunc
	searchCache        *searchMemo
	reviewReports      *reviewReportMemo
	changesSince       map[string]changesSinceReview
	contentVersion     int
	scrollPending      bool
	frames             *dashboardFrameCache
	bragSaved          *bragSavedMemo
	dryRunLogStamps    map[string]string
	gitCancel          context.CancelFunc
	gitFetchCtx        context.Context
	commitsCtx         context.Context
	commitsCancel      context.CancelFunc
	daySyncGeneration  int
	selected           int
	scrollOffset       int
	archivedSelected   int
	ctrlCCount         int
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

	ghReviewedToday         []GitPRItem
	ghReviewedYesterday     []GitPRItem
	ghPendingPRs            []GitPRItem
	myPRs                   []review.QueuedPR
	knownMyPRs              []review.PRRef
	loadingMyPRs            bool
	prDetails               map[string]json.RawMessage
	pendingSort             sourcecontrol.Sort
	pendingMeOnly           bool
	initialSelectionPending bool
	selectAfterReload       string
	awaitingNewNoteSave     bool
	pendingSortChosen       bool
	syncOnLoad              bool
	reviewRuns              []review.ReviewRun
	reviewRunAction         string
	reviewRunTarget         review.PRRef
	reviewRunReturnMode     ViewMode
	localCommitsToday       map[string][]GitPRItem
	tagCells                *rowTagCellCache
	noteRows                *noteRowCache
	popupMemo               *framedPopupMemo
	loadingCommits          bool
	localCommitsYesterday   map[string][]GitPRItem
	fetchedPreviousDay      time.Time
	commitsGeneration       int
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
	actionUsage             map[string]int
	notifyInput             *textinput.Model
	notifyNotice            string
	notifyEntries           map[string]notify.Entry
	hintGeneration          int
	hintVisible             bool
	prActionFromDashboard   bool
	setup                   *setupState
	setupInput              *textinput.Model
	configPath              string
	automationName          string
	automationPhase         automation.Phase
	automationNotice        string

	gitPopupRepo     *GitRepoStat
	gitPopupTab      int
	gitPopupSelected int

	previewViewport  viewport.Model
	archivedViewport viewport.Model

	editor      *textarea.Model
	searchInput *textinput.Model

	searchSelected  int
	searchScroll    int
	searchPreviewID string
	searchNotice    string
	editReturnMode  ViewMode
	inlineInput     *textinput.Model

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
	closedMyPRs          map[string]string
	prAlertsDue          bool
	errorLines           []string
	errorReturnMode      ViewMode

	width  int
	height int
}

type loadNotesMsg struct {
	notes []*model.Note
	err   error
}

func autoSyncTickCmd(intervalSecs int) tea.Cmd {
	if intervalSecs <= 0 {
		return nil
	}
	return tea.Tick(time.Duration(intervalSecs)*time.Second, func(t time.Time) tea.Msg {
		return autoSyncTickMsg(t)
	})
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
		searchCache:             &searchMemo{},
		reviewReports:           &reviewReportMemo{},
		loadingCommits:          cfg != nil && cfg.DailyCommitsEnabled(),
		currentDate:             time.Now(),
		archivedSelectedMap:     make(map[int]bool),
		jobDryRunOutputs:        make(map[string]string),
		jobDryRunExitCodes:      make(map[string]int),
		jobDryRunHasRun:         make(map[string]bool),
		bannerWaveActive:        true,
	}
	m.sessionCtx, m.cancelSession = context.WithCancel(context.Background())
	m.notes, m.startupNotesErr = m.store.List()
	m.actionUsage = loadActionUsage(cfg.CacheDir())
	m.notifyEntries = listNotifyEntries(cfg.Root()).entries
	m.fetchedPreviousDay = m.previousNoteDay()
	m.loadingGit = false
	if cfg.GitEnabled() {
		m.loadGitOnStartup()
	}
	m.refreshDryRunResults()
	m.refreshReviewRuns()
	m.commitsCtx, m.commitsCancel = context.WithCancel(context.Background())
	m.refreshBragRuns()
	m.applyAutomationRuns(automation.ListRuns(m.automationRoot()))
	m.reviewPolling = m.anyReviewRunning() || m.anyBragRunning() || m.anyAutomationRunning()
	m.syncPulseRunning = m.anythingBusy()
	m.runStatePolling = m.isAnyDryRunInFlight() || m.isAnyJobRunning()
	if startupErr != nil {
		m.showError("CONFIG ERROR", startupErr)
	}
	return m
}

func (m *Model) loadGitOnStartup() {
	m.loadingGit = true
	cache, cacheLoaded := loadGitCache()
	if cacheLoaded {
		if cache.PendingSort != nil {
			m.pendingSort, m.pendingSortChosen = *cache.PendingSort, true
		}
		m.applyGitCache(cache)
	}
	if seen, err := loadMyPRsSeen(myPRsSeenPath(m.cfg.Root())); err == nil {
		m.knownMyPRs = knownMyPRRefs(seen)
	}
	m.syncOnLoad = !cacheLoaded || !cache.hasDataFor(m.currentDate) || cache.PreviousDay != m.fetchedPreviousDay.Format("2006-01-02")
	m.loadingMyPRs = true
	if m.syncOnLoad {
		m.beginGitFetch()
	} else {
		m.loadingGit = false
	}
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.startupNotesCmd(), tickBannerWaveCmd(), m.loadCommitsCmd(), m.startupHintCmd()}
	if m.syncPulseRunning {
		cmds = append(cmds, tickSyncPulseCmd())
	}
	if m.syncOnLoad {
		cmds = append(cmds, m.gitFetchCmd())
	} else if m.loadingMyPRs {
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

func (m Model) startupNotesCmd() tea.Cmd {
	notes, err := m.notes, m.startupNotesErr
	return func() tea.Msg {
		return loadNotesMsg{notes: notes, err: err}
	}
}

func (m Model) loadNotesCmd() tea.Msg {
	notes, err := m.store.List()
	return loadNotesMsg{notes: notes, err: err}
}

func copyNotes(notes []*model.Note) []model.Note {
	copies := make([]model.Note, 0, len(notes))
	for _, note := range notes {
		if note != nil {
			copies = append(copies, *note)
		}
	}
	return copies
}

func (m Model) saveNotesCmd(notes ...*model.Note) tea.Cmd {
	noteStore := m.store
	copies := copyNotes(notes)
	return func() tea.Msg {
		var errs []error
		var savedIDs []string
		for i := range copies {
			if err := noteStore.Save(&copies[i]); err != nil {
				errs = append(errs, fmt.Errorf("save %q: %w", copies[i].Summary, err))
				continue
			}
			savedIDs = append(savedIDs, copies[i].ID)
		}
		return notesSavedMsg{errs: errs, savedIDs: savedIDs}
	}
}

func (m Model) deleteNotesCmd(notes ...*model.Note) tea.Cmd {
	noteStore := m.store
	copies := copyNotes(notes)
	return func() tea.Msg {
		var errs []error
		for i := range copies {
			if err := noteStore.Delete(&copies[i]); err != nil {
				errs = append(errs, fmt.Errorf("delete %q: %w", copies[i].Summary, err))
			}
		}
		return notesSavedMsg{errs: errs}
	}
}

func (m *Model) refreshArchivedViewport() {
	archivedNotes := m.getArchivedNotes()
	if m.archivedSelected >= len(archivedNotes) && len(archivedNotes) > 0 {
		m.archivedSelected = len(archivedNotes) - 1
	}
	if m.archivedSelected < 0 {
		m.archivedSelected = 0
	}
	modalWidth := modalWidthFor(m.width)
	m.archivedViewport.SetContent(m.renderArchivedContent(modalWidth-6, m.archivedSelected))
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

func (m Model) getJobDrafts() []*JobDraft {
	if m.cfg == nil || len(m.cfg.JobList()) == 0 {
		return nil
	}

	var drafts []*JobDraft
	for _, j := range m.cfg.JobList() {
		exitCode := 1
		hasRun := false
		if m.jobDryRunHasRun != nil && m.jobDryRunHasRun[j.Name] {
			hasRun = true
			exitCode = m.jobDryRunExitCodes[j.Name]
		}

		drafts = append(drafts, &JobDraft{
			Name:           j.Name,
			DryRunCommand:  j.DryRunCommand,
			Command:        j.Command,
			ExitCode:       exitCode,
			HasRunDryRun:   hasRun,
			DryRunInFlight: m.dryRunsInFlight[j.Name],
		})
	}
	return drafts
}

func (m Model) previousNoteDay() time.Time {
	viewedDayStart := time.Date(m.currentDate.Year(), m.currentDate.Month(), m.currentDate.Day(), 0, 0, 0, 0, m.currentDate.Location())
	var latest time.Time
	for _, note := range m.notes {
		if note.Status == model.StatusArchived || (note.Source != model.SourceManual && note.Source != "") {
			continue
		}
		for _, stamp := range []time.Time{note.Created, note.Updated} {
			if stamp.Before(viewedDayStart) && stamp.After(latest) {
				latest = stamp
			}
		}
	}
	if latest.IsZero() {
		return m.currentDate.AddDate(0, 0, -1)
	}
	return latest
}

type noteGroups struct {
	previousDay  time.Time
	previousDone []*model.Note
	carried      []*model.Note
	today        []*model.Note
	todayDone    []*model.Note
}

func (m Model) groupNotes() noteGroups {
	groups := noteGroups{previousDay: m.previousNoteDay()}
	for _, n := range m.notes {
		switch n.Status {
		case model.StatusArchived:
		case model.StatusDone:
			if isSameDay(n.Updated, groups.previousDay) {
				groups.previousDone = append(groups.previousDone, n)
			}
			if isSameDay(n.Updated, m.currentDate) {
				groups.todayDone = append(groups.todayDone, n)
			}
		default:
			if isSameDay(n.Created, m.currentDate) {
				groups.today = append(groups.today, n)
			} else if n.Created.Before(m.currentDate) {
				groups.carried = append(groups.carried, n)
			}
		}
	}
	slices.SortStableFunc(groups.previousDone, func(a, b *model.Note) int {
		if bySource := strings.Compare(string(a.Source), string(b.Source)); bySource != 0 {
			return bySource
		}
		return a.Updated.Compare(b.Updated)
	})
	return groups
}

func (m Model) previousDayTitleFor(previousDay time.Time) string {
	if isSameDay(previousDay, m.currentDate.AddDate(0, 0, -1)) {
		return m.dayTitleText(previousDay, "Y E S T E R D A Y")
	}
	return m.dayTitleText(previousDay, letterSpaced(strings.ToUpper(previousDay.Format("Monday"))))
}

func letterSpaced(word string) string {
	return strings.Join(strings.Split(word, ""), " ")
}

func (m Model) getArchivedNotes() []*model.Note {
	var list []*model.Note
	for _, n := range m.notes {
		if n.Status == model.StatusArchived {
			list = append(list, n)
		}
	}
	sort.SliceStable(list, func(i, j int) bool {
		return list[i].Updated.After(list[j].Updated)
	})
	return list
}

func (m Model) getPendingGitGroups() []PendingRepoGroup {
	if !m.cfg.GitEnabled() {
		return nil
	}
	var groups []PendingRepoGroup
	groupMap := make(map[string]int)

	for _, item := range m.pendingGitAction {
		rName := item.Repository
		if rName == "" {
			rName = "general"
		}
		idx, exists := groupMap[rName]
		if !exists {
			groupMap[rName] = len(groups)
			groups = append(groups, PendingRepoGroup{Name: rName, Items: []GitPRItem{item}})
		} else {
			groups[idx].Items = append(groups[idx].Items, item)
		}
	}

	return groups
}

func (m Model) allNavItems() []NavItem {
	groups := m.groupNotes()
	pendingGroups := m.getPendingGitGroups()
	drafts := m.getJobDrafts()
	automationRuns := m.runningAutomations()
	capacity := len(groups.previousDone) + len(groups.carried) + len(groups.today) + len(groups.todayDone) + len(drafts) + len(m.reviewRuns) + len(m.bragRuns) + len(automationRuns)
	if m.cfg.GitEnabled() {
		capacity += len(m.yesterdayGitRepo) + len(m.todayGitRepos) + len(m.myPRs)
	}
	for _, group := range pendingGroups {
		capacity += len(group.Items)
	}
	items := make([]NavItem, 0, capacity)

	for _, n := range groups.previousDone {
		items = append(items, NavItem{Kind: KindYesterdayDone, Note: n})
	}

	if m.cfg.GitEnabled() {
		for _, repo := range m.yesterdayGitRepo {
			items = append(items, NavItem{Kind: KindGitRepo, GitRepo: repo})
		}
		for _, repo := range m.todayGitRepos {
			items = append(items, NavItem{Kind: KindGitRepo, GitRepo: repo})
		}
		for index := range m.myPRs {
			items = append(items, NavItem{Kind: KindMyPR, MyPR: &m.myPRs[index]})
		}
	}

	for _, n := range groups.carried {
		items = append(items, NavItem{Kind: KindCarriedNote, Note: n})
	}

	for _, n := range groups.today {
		items = append(items, NavItem{Kind: KindTodayNote, Note: n})
	}

	for _, n := range groups.todayDone {
		items = append(items, NavItem{Kind: KindTodayDone, Note: n})
	}

	for _, g := range pendingGroups {
		for i := range g.Items {
			items = append(items, NavItem{Kind: KindPendingGit, PendingGitPR: &g.Items[i]})
		}
	}

	for _, d := range drafts {
		items = append(items, NavItem{Kind: KindJobDraft, Draft: d})
	}

	for i := range m.reviewRuns {
		items = append(items, NavItem{Kind: KindReviewRun, ReviewRun: &m.reviewRuns[i]})
	}

	for i := range m.bragRuns {
		items = append(items, NavItem{Kind: KindBragRun, BragRun: &m.bragRuns[i]})
	}

	for i := range automationRuns {
		items = append(items, NavItem{Kind: KindAutomationRun, AutomationRun: &automationRuns[i]})
	}

	return items
}

func (m Model) filteredGitItems() []GitPRItem {
	if m.gitPopupRepo == nil {
		return nil
	}
	var filtered []GitPRItem
	for _, item := range m.gitPopupRepo.Items {
		switch m.gitPopupTab {
		case 1:
			if item.Kind == "Reviewed" {
				filtered = append(filtered, item)
			}
		case 2:
			if item.Kind == "Assigned" {
				filtered = append(filtered, item)
			}
		case 3:
			if item.Kind == "Commit" {
				filtered = append(filtered, item)
			}
		default:
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func (m Model) dayTitleText(date time.Time, spacedLabel string) string {
	if isSameDay(m.currentDate, time.Now()) {
		return fmt.Sprintf("%s  ·  %s", spacedLabel, strings.ToUpper(date.Format("02 Jan")))
	}
	return fmt.Sprintf("%s . %s", strings.ToUpper(date.Format("Monday")), strings.ToUpper(date.Format("02 Jan")))
}

func (m Model) gitNavStart() int {
	return len(m.groupNotes().previousDone)
}

func (m Model) renderGitStrip(width int, active bool, groups noteGroups) (lines []string, selectedRow int) {
	separator := mutedStyle.Render(" │")
	leftWidth := max((width-lipgloss.Width(separator))/2, 16)
	rightWidth := max(width-lipgloss.Width(separator)-leftWidth, 16)
	columnWidths := [2]int{leftWidth, rightWidth}
	header := " " + renderSectionTitle("G I T", active)
	repoColumns := [2][]*GitRepoStat{m.yesterdayGitRepo, m.todayGitRepos}
	columnTitles := [2]string{
		m.previousDayTitleFor(groups.previousDay),
		m.dayTitleText(m.currentDate, "T O D A Y"),
	}
	navStart := len(groups.previousDone)
	columnStarts := [2]int{navStart, navStart + len(m.yesterdayGitRepo)}
	var captionCells [2]string
	for side, repos := range repoColumns {
		columnActive := m.selected >= columnStarts[side] && m.selected < columnStarts[side]+len(repos)
		caption := " " + renderSectionTitle(columnTitles[side], columnActive)
		if m.cfg.DailyCommitsEnabled() {
			commits := 0
			for _, repo := range repos {
				commits += repo.Commits
			}
			caption += mutedStyle.Render(fmt.Sprintf("  %d commits", commits))
		}
		captionCells[side] = ansi.Truncate(caption, columnWidths[side], "…")
	}
	joinCells := func(cells [2]string) string {
		return cells[0] + safeRepeat(" ", columnWidths[0]-lipgloss.Width(cells[0])) + separator + cells[1]
	}
	lines = []string{header, "", joinCells(captionCells)}
	selectedRow = -1
	rowCount := max(len(m.yesterdayGitRepo), len(m.todayGitRepos), 1)
	for row := range rowCount {
		var cells [2]string
		for side, repos := range repoColumns {
			globalIndex := columnStarts[side] + row
			switch {
			case row < len(repos):
				selected := globalIndex == m.selected
				if selected {
					selectedRow = len(lines)
				}
				cells[side] = strings.TrimSuffix(m.renderGitRepoRow(repos[row], selected, columnWidths[side]), "\n")
			case row == 0 && m.loadingCommits:
				cells[side] = mutedStyle.Render("     (checking...)")
			case row == 0:
				cells[side] = mutedStyle.Render("     (no git activity)")
			}
		}
		lines = append(lines, joinCells(cells))
	}
	myPRsStart := columnStarts[1] + len(m.todayGitRepos)
	myPRsActive := m.selected >= myPRsStart && m.selected < myPRsStart+len(m.myPRs)
	lines = append(lines, "", ansi.Truncate(" "+renderSectionTitle("M Y   P R ( S )", myPRsActive)+mutedStyle.Render(fmt.Sprintf("  %d open", len(m.myPRs))), width, "…"))
	for _, line := range m.renderMyPRBlock(myPRsStart, width) {
		if line.navIndex >= 0 && line.navIndex == m.selected {
			selectedRow = len(lines)
		}
		lines = append(lines, line.text)
	}
	return lines, selectedRow
}

func (m Model) gitStripColumnSwitch(towardsRight bool) (target int, ok bool) {
	navStart := m.gitNavStart()
	counts := [2]int{len(m.yesterdayGitRepo), len(m.todayGitRepos)}
	starts := [2]int{navStart, navStart + counts[0]}
	current := -1
	for column := range counts {
		if m.selected >= starts[column] && m.selected < starts[column]+counts[column] {
			current = column
		}
	}
	if current < 0 {
		return 0, false
	}
	step := -1
	if towardsRight {
		step = 1
	}
	row := m.selected - starts[current]
	for column := current + step; column >= 0 && column < len(counts); column += step {
		if counts[column] > 0 {
			return starts[column] + min(row, counts[column]-1), true
		}
	}
	return 0, false
}

func (m *Model) updatePreviewViewport() {
	navItems := m.allNavItems()
	if len(navItems) == 0 || m.selected >= len(navItems) {
		return
	}

	_, innerWidth, innerHeight := previewModalSize(m.width, m.height)

	item := navItems[m.selected]
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
			heading := renderMarkdown(fmt.Sprintf("# Job: %s (%s)", item.Draft.Name, statusHeader), innerWidth)
			logTail := lipgloss.NewStyle().Width(innerWidth).Render(lastLines(strings.TrimSpace(logText), jobLogTailLines))
			mdContent = heading + "\n\n" + logTail
		} else {
			mdContent = renderMarkdown(fmt.Sprintf("# Job: %s\n\nNo dry-run analysis or log output available.\nPress **[r]** on dashboard to run dry-run check, or press **[Enter]** to execute job.", item.Draft.Name), innerWidth)
		}
	} else if item.Kind == KindReviewRun && item.ReviewRun != nil {
		mdContent = renderMarkdown(reviewRunPreview(m.reviewRoot(), *item.ReviewRun), innerWidth)
	} else if item.Kind == KindBragRun && item.BragRun != nil {
		mdContent = renderMarkdown(bragRunPreview(m.bragRoot(), *item.BragRun), innerWidth)
	} else if item.Kind == KindAutomationRun && item.AutomationRun != nil {
		mdContent = renderMarkdown(m.automationRunPreview(*item.AutomationRun), innerWidth)
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
	if item.Kind == KindJobDraft || item.Kind == KindReviewRun || item.Kind == KindBragRun || item.Kind == KindAutomationRun {
		m.previewViewport.GotoBottom()
	}
}

func (m Model) renderArchivedContent(width int, selectedIndex int) string {
	archived := m.getArchivedNotes()
	if len(archived) == 0 {
		return mutedStyle.Render("(No archived notes)")
	}

	var listing strings.Builder
	currentGroupDate := ""

	for index, note := range archived {
		dateLabel := note.Updated.Local().Format("Monday 02 Jan 2006")
		if dateLabel != currentGroupDate {
			if currentGroupDate != "" {
				listing.WriteString("\n")
			}
			listing.WriteString(subSectionStyle.Render(dateLabel) + "\n")
			currentGroupDate = dateLabel
		}

		prefix := "  "
		if m.archivedSelectedMap != nil && m.archivedSelectedMap[index] {
			prefix = amberDiamond.Render() + " "
		}

		box := checkDone.Render()
		if note.Status != model.StatusDone {
			box = checkPending.Render()
		}

		age := note.Updated.Local().Format("15:04")
		sourceText := noteSourceText(note)
		summaryWidth := max(width-ansi.StringWidth(prefix)-2-ansi.StringWidth(sourceText)-len(age)-6, 10)
		summary := ansi.Truncate(note.Summary, summaryWidth, "…")
		gap := summaryWidth - ansi.StringWidth(summary)

		selected := index == selectedIndex
		if selected {
			summary = selectedTitle(summary)
		} else {
			summary = itemStyle.Render(summary)
		}
		rightBlock := fmt.Sprintf("%s   %s", dimBlueText.Render(sourceText), mutedStyle.Render(age))
		listing.WriteString(fmt.Sprintf("%s%s %s%s%s\n", prefix, box, summary, safeRepeat(" ", gap), underlinedWhen(selected, rightBlock)))
	}

	return listing.String()
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
	if updated.scrollPending {
		updated.settleScroll()
	}
	if updated.prAlertsDue {
		updated.prAlertsDue = false
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

func (m Model) handleMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case messageExpiryMsg:
		m.messageExpiryPending = false
		return m, nil

	case prAlertsMsg:
		if msg.err != nil {
			m.showError("PR ALERTS", msg.err)
		}
		return m, nil

	case bannerWaveTickMsg:
		if !m.bannerWaveActive {
			return m, nil
		}
		m.bannerWaveFrame++
		if m.bannerWaveFrame > bannerWaveLastFrame() {
			m.bannerWaveActive = false
			return m, nil
		}
		return m, tickBannerWaveCmd()

	case syncPulseTickMsg:
		if m.anythingBusy() {
			m.syncPulseFrame++
			return m, tickSyncPulseCmd()
		}
		m.syncPulseRunning = false
		return m, nil

	case jobLogTickMsg:
		return m.handleJobLogTick()

	case runStatePollTickMsg:
		previousJobState := m.previewedJobState()
		m.refreshDryRunResults()
		stillBusy := m.isAnyDryRunInFlight() || m.isAnyJobRunning()
		if m.mode == ViewPreview && !m.jobLogRunning && m.previewedJobState() != previousJobState {
			m.updatePreviewViewport()
		}
		if stillBusy {
			return m, tickRunStatePollCmd()
		}
		m.runStatePolling = false
		return m, nil

	case jobAbortedMsg:
		m.refreshJobStates()
		if m.mode == ViewPreview {
			m.updatePreviewViewport()
		}
		if msg.err != nil {
			m.showError("JOB ERROR", msg.err)
		}
		return m, nil

	case searchExportedMsg:
		return m.handleSearchExported(msg)

	case notesSavedMsg:
		if len(msg.errs) > 0 {
			m.showError("STORE ERROR", msg.errs...)
		}
		if m.awaitingNewNoteSave && len(msg.savedIDs) == 1 {
			m.selectAfterReload = msg.savedIDs[0]
		}
		m.awaitingNewNoteSave = false
		return m, m.loadNotesCmd

	case autoSyncTickMsg:
		if m.cfg.GitEnabled() && m.cfg.GitAutoSyncInterval > 0 {
			return m, tea.Batch(
				m.startLoadGitStatsCmd(),
				autoSyncTickCmd(m.cfg.GitAutoSyncInterval),
			)
		}
		return m, nil

	case ctrlCResetMsg:
		m.ctrlCCount = 0
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
		if m.mode == ViewSearchPreview {
			m.updateSearchPreviewViewport()
		}

	case loadNotesMsg:
		m.notes = msg.notes
		if m.initialSelectionPending {
			m.initialSelectionPending = false
			m.selectLaunchItem()
		}
		if m.selectAfterReload != "" {
			m.selectNoteByID(m.selectAfterReload)
			m.selectAfterReload = ""
		}
		var refetchPreviousDay tea.Cmd
		if !isSameDay(m.previousNoteDay(), m.fetchedPreviousDay) {
			m.ghReviewedYesterday, m.localCommitsYesterday = nil, nil
			m.rebuildGitRepoStats()
			refetchPreviousDay = m.startLoadGitStatsCmd()
		}
		m.updateScrollOffset()
		if m.mode == ViewSearchPreview {
			m.updateSearchPreviewViewport()
		}
		if m.mode == ViewArchived {
			m.refreshArchivedViewport()
		}
		if msg.err != nil {
			m.showError("STORE ERROR", msg.err)
			return m, refetchPreviousDay
		}
		return m, tea.Batch(refetchPreviousDay, refreshNotifyCmd(m.cfg.Root(), m.notes))

	case notifyEntriesMsg:
		if msg.err != nil {
			m.showError("NOTIFY ERROR", msg.err)
			return m, nil
		}
		m.notifyEntries = msg.entries
		return m, nil

	case actionUsageSavedMsg:
		if msg.err != nil {
			m.showError("ACTION USAGE ERROR", msg.err)
		}
		return m, nil

	case commitsLoadedMsg:
		wasSyncing := m.loadingGit || m.loadingCommits
		m.applyCommits(msg)
		if wasSyncing {
			save := m.gitCacheSaveCmd()
			return m, save
		}
		return m, nil

	case gitDaySectionMsg:
		var cmds []tea.Cmd
		if msg.generation == m.fetchGeneration {
			cmds = append(cmds, reviewNotesCmd(m.store, msg.reviews))
		}
		if msg.sections != nil {
			cmds = append(cmds, waitForGitSection(msg.sections, msg.generation))
		}
		wasSyncing := m.loadingGit || m.loadingCommits
		m.applyGitDay(msg)
		if wasSyncing {
			cmds = append(cmds, m.gitCacheSaveCmd())
		}
		return m.afterGitSection(cmds)

	case gitMyPRsMsg:
		var cmds []tea.Cmd
		current := !msg.partOfSync || msg.generation == m.fetchGeneration
		if current && (len(msg.prs) > 0 || len(msg.closed) > 0) {
			cmds = append(cmds, myPRNotesCmd(m.store, myPRsSeenPath(m.cfg.Root()), msg.prs, msg.closed, msg.failedHosts, time.Now()))
		}
		if msg.sections != nil {
			cmds = append(cmds, waitForGitSection(msg.sections, msg.generation))
		}
		wasSyncing := m.loadingGit || m.loadingCommits
		m.applyMyPRs(msg)
		if wasSyncing || !msg.partOfSync {
			cmds = append(cmds, m.gitCacheSaveCmd())
		}
		return m.afterGitSection(cmds)

	case myPRNotesMsg:
		if msg.known != nil || msg.err == nil {
			m.knownMyPRs = msg.known
		}
		if msg.err != nil {
			m.recordSectionError(sectionMyPRs, msg.err)
			return m, nil
		}
		if !msg.saved {
			return m, nil
		}
		return m.Update(loadNotesMsg{notes: msg.notes})

	case gitPendingMsg:
		var cmds []tea.Cmd
		if msg.generation == m.fetchGeneration && msg.err == nil {
			cmds = append(cmds, reopenApprovedNotesCmd(m.store, slices.Clone(msg.pending), msg.startedAt))
		}
		if msg.sections != nil {
			cmds = append(cmds, waitForGitSection(msg.sections, msg.generation))
		}
		wasSyncing := m.loadingGit || m.loadingCommits
		m.applyGitPending(msg)
		if wasSyncing {
			cmds = append(cmds, m.gitCacheSaveCmd())
		}
		return m.afterGitSection(cmds)

	case approvalSyncMsg:
		return m, m.startLoadGitStatsCmd()

	case daySyncDueMsg:
		if msg.generation != m.daySyncGeneration {
			return m, nil
		}
		return m, m.startLoadGitStatsCmd()

	case changesSinceReviewMsg:
		return m.handleChangesSinceReview(msg)
	case relatedHistoryMsg:
		return m.handleRelatedHistory(msg)

	case gitCacheSavedMsg:
		m.recordSectionError("Cache", msg.err)
		return m, nil

	case reviewPollTickMsg:
		return m.handleReviewPoll(msg.snapshot)

	case reviewSubmittedMsg:
		return m.handleReviewSubmitted(msg)

	case reviewCloneReadyMsg:
		return m.handleCloneReady(msg)

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
	if item.Kind == KindGitRepo && item.GitRepo != nil {
		m.gitPopupRepo = item.GitRepo
		m.gitPopupTab = 0
		m.gitPopupSelected = 0
		m.mode = ViewGitDetails
		return m, nil
	}
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

func (m Model) progressText() string {
	facts := habit.Gather(m.notes, time.Now(), m.cfg.IsWorkDay)
	var parts []string
	if facts.Streak > 0 {
		parts = append(parts, yellowBadgeStyle.Render(fmt.Sprintf("🔥 %d-day streak", facts.Streak)))
	}
	if facts.ClosedThisWeek > 0 {
		parts = append(parts, mutedStyle.Render(fmt.Sprintf("%d closed this week", facts.ClosedThisWeek)))
	}
	return strings.Join(parts, mutedStyle.Render(" · "))
}

func (m Model) renderHeader() string {
	renderedDate := mutedStyle.Bold(true).Render("— " + time.Now().Format("Monday 02 Jan"))
	middleLine := m.renderBannerLine(bannerMiddleRow) + "  "
	if version := displayVersion(); version != "" {
		middleLine += versionStyle.Render(version) + "  "
	}
	middleLine += renderedDate
	if notice := m.unbraggedWeekNotice(); notice != "" {
		noticeRoom := m.width - lipgloss.Width(middleLine) - 7
		if noticeRoom > 0 {
			noticeText := yellowBadgeStyle.Render(ansi.Truncate(notice, noticeRoom, "…"))
			middleLine += safeRepeat(" ", m.width-4-lipgloss.Width(middleLine)-lipgloss.Width(noticeText)) + noticeText
		}
	}

	bottomLine := m.renderBannerLine(bannerBottomRow)

	topLine := m.renderBannerLine(bannerTopRow)
	if progress := m.progressText(); progress != "" {
		versionColumn := lipgloss.Width(m.renderBannerLine(bannerMiddleRow)) + 2
		if paddedLine := topLine + safeRepeat(" ", versionColumn-lipgloss.Width(topLine)) + progress; lipgloss.Width(paddedLine) <= m.width-4 {
			topLine = paddedLine
		}
	}
	if message := m.renderActiveMessage(m.width - 4 - lipgloss.Width(topLine) - 4); message != "" {
		topLine += safeRepeat(" ", m.width-4-lipgloss.Width(topLine)-lipgloss.Width(message)) + message
	}

	headerLines := []string{borderStyle.Render("┌" + safeRepeat("─", m.width-2) + "┐")}
	for _, content := range []string{topLine, middleLine, bottomLine} {
		headerLines = append(headerLines, fmt.Sprintf("│ %s%s │", content, safeRepeat(" ", m.width-lipgloss.Width(content)-4)))
	}
	headerLines = append(headerLines, borderStyle.Render("├"+safeRepeat("─", m.width-2)+"┤"))
	return strings.Join(headerLines, "\n")
}

const sectionGap = "\n\n"

var dashboardContentBuilder = Model.buildDashboardContent

func (m Model) dashboardContent() (content string, selectedLine int) {
	return dashboardContentBuilder(m)
}

func (m Model) buildDashboardContent() (content string, selectedLine int) {
	var board strings.Builder
	if m.frames != nil {
		board.Grow(m.frames.lastContentLength + 1024)
	}
	innerWidth := m.width - 4
	rowWidth := innerWidth + 1

	groups := m.groupNotes()
	pendingGroups := m.getPendingGitGroups()
	m.tagCells = &rowTagCellCache{prs: map[*GitPRItem][3]string{}}
	pendingCount := 0
	for _, group := range pendingGroups {
		pendingCount += len(group.Items)
	}
	drafts := m.getJobDrafts()
	automationRuns := m.runningAutomations()
	jobsCount := len(drafts) + len(m.reviewRuns) + len(m.bragRuns) + len(automationRuns)

	previousNotesEnd := len(groups.previousDone)
	gitEnd := previousNotesEnd
	if m.cfg.GitEnabled() {
		gitEnd += len(m.yesterdayGitRepo) + len(m.todayGitRepos) + len(m.myPRs)
	}
	carriedEnd := gitEnd + len(groups.carried)
	addedEnd := carriedEnd + len(groups.today)
	closedEnd := addedEnd + len(groups.todayDone)
	pendingEnd := closedEnd + pendingCount
	jobsEnd := pendingEnd + jobsCount
	selectedWithin := func(start, end int) bool { return m.selected >= start && m.selected < end }

	navIndex := 0
	emitRow := func(render func(selected bool) string) {
		selected := navIndex == m.selected
		if selected {
			selectedLine = strings.Count(board.String(), "\n")
		}
		board.WriteString(render(selected))
		navIndex++
	}
	emitNotes := func(notes []*model.Note, emptyHint string) {
		if len(notes) == 0 && emptyHint != "" {
			board.WriteString(mutedStyle.Render(emptyHint + "\n"))
		}
		for _, note := range notes {
			emitRow(func(selected bool) string { return m.renderRow(note, selected, rowWidth) })
		}
	}

	board.WriteString("\n " + renderSectionTitle(m.previousDayTitleFor(groups.previousDay), selectedWithin(0, previousNotesEnd)) + "\n")
	if len(groups.previousDone) > 0 {
		board.WriteString("\n")
	}
	emitNotes(groups.previousDone, "")

	if m.cfg.GitEnabled() {
		stripLines, stripSelectedRow := m.renderGitStrip(rowWidth, selectedWithin(previousNotesEnd, gitEnd), groups)
		board.WriteString(sectionGap)
		if stripSelectedRow >= 0 {
			selectedLine = strings.Count(board.String(), "\n") + stripSelectedRow
		}
		board.WriteString(strings.Join(stripLines, "\n") + "\n")
		navIndex = gitEnd
	}

	board.WriteString(sectionGap)
	jobBadge := fmt.Sprintf("%d jobs %s", len(drafts), amberDiamond.Render())
	todayTitle := renderSectionTitle(m.dayTitleText(m.currentDate, "T O D A Y"), selectedWithin(gitEnd, jobsEnd))
	todayGap := innerWidth - lipgloss.Width(todayTitle) - lipgloss.Width(jobBadge)
	board.WriteString(fmt.Sprintf(" %s%s%s\n\n", todayTitle, safeRepeat(" ", todayGap), jobBadge))

	board.WriteString("  " + m.renderSubSection("Pending, Carried Over", len(groups.carried), true, selectedWithin(gitEnd, carriedEnd)) + "\n")
	emitNotes(groups.carried, "   (no carried over notes)")
	board.WriteString(sectionGap + "  " + m.renderSubSection("Added Today", len(groups.today), true, selectedWithin(carriedEnd, addedEnd)) + "\n")
	emitNotes(groups.today, "   (no notes added today)")
	board.WriteString(sectionGap + "  " + m.renderSubSection("Closed Today", len(groups.todayDone), true, selectedWithin(addedEnd, closedEnd)) + "\n")
	emitNotes(groups.todayDone, "   (no notes closed today)")

	if m.cfg.GitEnabled() {
		pendingHeader := m.renderSubSection("Pending Git Actions", 0, false, selectedWithin(closedEnd, pendingEnd))
		board.WriteString(fmt.Sprintf("%s  %s  %s\n", sectionGap, pendingHeader, m.renderPendingSortHint()))
		switch {
		case len(pendingGroups) > 0:
			for groupIndex, group := range pendingGroups {
				if groupIndex > 0 {
					board.WriteString("\n")
				}
				board.WriteString("    " + dimBlueText.Bold(true).Render(group.Name) + "\n")
				for itemIndex := range group.Items {
					item := &group.Items[itemIndex]
					emitRow(func(selected bool) string { return m.renderPendingGitRow(item, selected, rowWidth) })
				}
			}
		case m.loadingGit:
			board.WriteString(mutedStyle.Render("   (checking pending PR reviews...)\n"))
		case m.pendingMeOnly:
			board.WriteString(mutedStyle.Render("   (no PRs asking you by name)\n"))
		default:
			board.WriteString(mutedStyle.Render("   (no PRs requiring review)\n"))
		}
	}

	board.WriteString(sectionGap + "  " + m.renderSubSection("Jobs", 0, false, selectedWithin(pendingEnd, jobsEnd)) + "\n")
	if jobsCount == 0 {
		board.WriteString(mutedStyle.Render("   (no jobs configured)\n"))
	}
	for _, draft := range drafts {
		emitRow(func(selected bool) string { return m.renderDraftRow(draft, selected, innerWidth) })
	}
	for _, run := range m.reviewRuns {
		emitRow(func(selected bool) string { return m.renderReviewRunRow(run, selected, innerWidth) })
	}
	for _, run := range m.bragRuns {
		emitRow(func(selected bool) string { return m.renderBragRunRow(run, selected, innerWidth) })
	}
	for _, run := range automationRuns {
		emitRow(func(selected bool) string { return m.renderAutomationRunRow(run, selected, innerWidth) })
	}

	if m.frames != nil {
		m.frames.lastContentLength = board.Len()
	}
	return board.String(), selectedLine
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
	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err := p.Run()
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
