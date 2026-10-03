package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"app/pkg/config"
	"app/pkg/model"
	"app/pkg/paths"
	"app/pkg/review"
	"app/pkg/sourcecontrol"
	"app/pkg/store"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
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
	Kind         NavItemKind
	Note         *model.Note
	GitRepo      *GitRepoStat
	Draft        *JobDraft
	PendingGitPR *GitPRItem
	ReviewRun    *review.ReviewRun
}

type PendingRepoGroup struct {
	Name  string
	Items []GitPRItem
}

type headerWaveTickMsg struct{}
type bannerWaveTickMsg struct{}
type syncPulseTickMsg struct{}
type ctrlCResetMsg struct{}
type autoSyncTickMsg time.Time
type jobLogTickMsg struct{}

type dryRunPollTickMsg struct{}

type jobAbortedMsg struct {
	jobName string
	err     error
}

type notesSavedMsg struct {
	errs []error
}

type gitDay = sourcecontrol.Day

const (
	gitDayToday     = sourcecontrol.Today
	gitDayYesterday = sourcecontrol.Yesterday
)

const gitSectionCount = 3

type gitDaySectionMsg struct {
	generation  int
	day         gitDay
	date        string
	reviewed    []GitPRItem
	commits     map[string][]GitPRItem
	reviews     []review.ActivityPR
	details     map[string]json.RawMessage
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

var (
	borderStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#45475A"))
	headerTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#CDD6F4"))

	// Section & Sub-section Title Styles
	sectionTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#CDD6F4"))
	subSectionStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#89B4FA"))
	mutedStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#6C7086"))
	itemStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("#CDD6F4"))

	selectedSummaryStyle = lipgloss.NewStyle().
				Underline(true).
				Bold(true).
				Foreground(lipgloss.Color("#F5E0DC"))

	selectedTagStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#A6E3A1"))

	tabActiveStyle = lipgloss.NewStyle().
			Bold(true).
			Underline(true).
			Foreground(lipgloss.Color("#89B4FA")).
			Padding(0, 1)

	tabInactiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#6C7086")).
				Padding(0, 1)

	checkDone        = lipgloss.NewStyle().Foreground(lipgloss.Color("#A6E3A1")).SetString("✔")
	checkPending     = lipgloss.NewStyle().Foreground(lipgloss.Color("#CDD6F4")).SetString("☐")
	amberDiamond     = lipgloss.NewStyle().Foreground(lipgloss.Color("#F9E2AF")).SetString("◆")
	yellowBadgeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#F9E2AF")).Bold(true)
	pendingPRIcon    = lipgloss.NewStyle().Foreground(lipgloss.Color("#74C7EC")).SetString("⊙")
	dimBlueText      = lipgloss.NewStyle().Foreground(lipgloss.Color("#74C7EC"))

	// Live status indicators
	dotSynced = lipgloss.NewStyle().Foreground(lipgloss.Color("#A6E3A1")).SetString("● synced")

	keyStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#CDD6F4"))
	actionStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#6C7086"))
	warnKeyStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F38BA8"))
	warnActionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#F9E2AF"))

	badgeActive = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Bold(true)
	badgeDone   = lipgloss.NewStyle().Foreground(lipgloss.Color("#A6E3A1")).Bold(true)
	tagStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#89B4FA")).Bold(true)

	// Popups strictly use terminal native background color
	modalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7D56F4")).
			Padding(1, 2)

	modalTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4"))

	deleteTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#F38BA8"))

	modalHelpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#A6ADC8")).Bold(true)
)

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
	return exec.Command(cmd, args...).Start()
}

func copyToClipboard(text string) error {
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

type footerItem struct {
	key    string
	action string
	isWarn bool
}

func capitaliseWord(word string) string {
	if word == "" {
		return word
	}
	return strings.ToUpper(word[:1]) + word[1:]
}

func splitFooterAction(action string) (firstLine, secondLine string) {
	words := strings.Fields(action)
	for index, word := range words {
		words[index] = capitaliseWord(word)
	}
	if len(words) == 0 {
		return "", ""
	}
	return words[0], strings.Join(words[1:], " ")
}

func footerLineCount(items []footerItem) int {
	for _, item := range items {
		if _, secondLine := splitFooterAction(item.action); secondLine != "" {
			return 3
		}
	}
	return 2
}

func renderFooterLines(items []footerItem) (keysLine, firstActionLine, secondActionLine string) {
	var keysParts, firstParts, secondParts []string
	for _, item := range items {
		firstWord, secondWord := splitFooterAction(item.action)
		width := max(lipgloss.Width(item.key), lipgloss.Width(firstWord), lipgloss.Width(secondWord))
		keyPadded := item.key + safeRepeat(" ", width-lipgloss.Width(item.key))
		firstPadded := firstWord + safeRepeat(" ", width-lipgloss.Width(firstWord))
		secondPadded := secondWord + safeRepeat(" ", width-lipgloss.Width(secondWord))
		currentKeyStyle, currentActionStyle := keyStyle, actionStyle
		if item.isWarn {
			currentKeyStyle, currentActionStyle = warnKeyStyle, warnActionStyle
		}
		keysParts = append(keysParts, currentKeyStyle.Render(keyPadded))
		firstParts = append(firstParts, currentActionStyle.Render(firstPadded))
		secondParts = append(secondParts, currentActionStyle.Render(secondPadded))
	}
	return strings.Join(keysParts, "   "), strings.Join(firstParts, "   "), strings.Join(secondParts, "   ")
}

func renderModalFooter(items []footerItem, maxWidth int) string {
	var renderedRows []string
	for _, row := range splitFooterRows(items, maxWidth) {
		keysLine, firstActionLine, secondActionLine := renderFooterLines(row)
		if footerLineCount(row) == 2 {
			renderedRows = append(renderedRows, keysLine+"\n"+firstActionLine)
		} else {
			renderedRows = append(renderedRows, keysLine+"\n"+firstActionLine+"\n"+secondActionLine)
		}
	}
	return strings.Join(renderedRows, "\n\n")
}

// --- Job Log & Execution Management ---

func getLogsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	dir := filepath.Join(home, "digest", "logs")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", "/c", fmt.Sprintf("tasklist /FI \"PID eq %d\"", pid))
		out, err := cmd.Output()
		if err != nil {
			return false
		}
		return strings.Contains(string(out), strconv.Itoa(pid))
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func runningJobPID(jobName string) (int, bool) {
	data, err := os.ReadFile(filepath.Join(getLogsDir(), fmt.Sprintf("%s.pid", jobName)))
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || !isProcessAlive(pid) {
		return 0, false
	}
	return pid, true
}

func isJobRunning(jobName string) bool {
	pidFile := filepath.Join(getLogsDir(), fmt.Sprintf("%s.pid", jobName))
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return false
	}
	if isProcessAlive(pid) {
		return true
	}
	_ = os.Remove(pidFile)
	return false
}

func dryRunFilePath(jobName string, suffix string) string {
	return filepath.Join(getLogsDir(), fmt.Sprintf("%s.dryrun.%s", jobName, suffix))
}

func isDryRunInFlight(jobName string) bool {
	pidFile := dryRunFilePath(jobName, "pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err == nil && isProcessAlive(pid) {
		return true
	}
	_ = os.Remove(pidFile)
	return false
}

func loadDryRunResult(jobName string) (string, int, bool) {
	exitData, err := os.ReadFile(dryRunFilePath(jobName, "exit"))
	if err != nil {
		return "", 0, false
	}
	exitCode, err := strconv.Atoi(strings.TrimSpace(string(exitData)))
	if err != nil {
		exitCode = 1
	}
	output, _ := os.ReadFile(dryRunFilePath(jobName, "log"))
	return string(output), exitCode, true
}

func writeDryRunFailure(jobName string, failure error) {
	_ = os.WriteFile(dryRunFilePath(jobName, "log"), []byte(failure.Error()+"\n"), 0644)
	_ = os.WriteFile(dryRunFilePath(jobName, "exit"), []byte("1"), 0644)
}

func startDryRunBackground(spec config.JobSpec) error {
	cmdStr, err := resolveJobCommand(spec, true)
	if err != nil {
		writeDryRunFailure(spec.Name, err)
		return err
	}

	logPath := dryRunFilePath(spec.Name, "log")
	pidPath := dryRunFilePath(spec.Name, "pid")
	exitPath := dryRunFilePath(spec.Name, "exit")
	_ = os.Remove(exitPath)

	f, err := os.Create(logPath)
	if err != nil {
		writeDryRunFailure(spec.Name, err)
		return err
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", cmdStr)
	} else {
		cmd = exec.Command("sh", "-c", `sh -c "$1"; echo $? > "$2"`, "digest-dry-run", cmdStr, exitPath)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setsid: true,
		}
	}
	cmd.Env = os.Environ()
	if home, err := os.UserHomeDir(); err == nil {
		cmd.Dir = home
	}
	cmd.Stdout = f
	cmd.Stderr = f

	if err := cmd.Start(); err != nil {
		f.Close()
		writeDryRunFailure(spec.Name, err)
		return err
	}

	ownPid := strconv.Itoa(cmd.Process.Pid)
	pidWriteErr := os.WriteFile(pidPath, []byte(ownPid), 0644)

	go func() {
		waitErr := cmd.Wait()
		_ = f.Close()
		if runtime.GOOS == "windows" {
			exitCode := 0
			if exitErr, ok := waitErr.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else if waitErr != nil {
				exitCode = 1
			}
			_ = os.WriteFile(exitPath, []byte(strconv.Itoa(exitCode)), 0644)
		}
		if data, err := os.ReadFile(pidPath); err == nil && strings.TrimSpace(string(data)) == ownPid {
			_ = os.Remove(pidPath)
		}
	}()

	if pidWriteErr != nil {
		return fmt.Errorf("dry run for %q started but its pid file could not be written: %w", spec.Name, pidWriteErr)
	}
	return nil
}

func tickDryRunPollCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return dryRunPollTickMsg{}
	})
}

func (m Model) isAnyDryRunInFlight() bool {
	if m.cfg == nil {
		return false
	}
	for _, j := range m.cfg.Jobs {
		if isDryRunInFlight(j.Name) {
			return true
		}
	}
	return false
}

func (m *Model) refreshDryRunResults() {
	if m.cfg == nil {
		return
	}
	if m.jobDryRunOutputs == nil {
		m.jobDryRunOutputs = make(map[string]string)
		m.jobDryRunExitCodes = make(map[string]int)
		m.jobDryRunHasRun = make(map[string]bool)
	}
	for _, j := range m.cfg.Jobs {
		if isDryRunInFlight(j.Name) {
			continue
		}
		output, exitCode, ok := loadDryRunResult(j.Name)
		if !ok {
			delete(m.jobDryRunOutputs, j.Name)
			delete(m.jobDryRunExitCodes, j.Name)
			delete(m.jobDryRunHasRun, j.Name)
			continue
		}
		m.jobDryRunOutputs[j.Name] = output
		m.jobDryRunExitCodes[j.Name] = exitCode
		m.jobDryRunHasRun[j.Name] = true
	}
}

func (m Model) isAnyJobRunning() bool {
	if m.cfg == nil || len(m.cfg.Jobs) == 0 {
		return false
	}
	for _, j := range m.cfg.Jobs {
		if isJobRunning(j.Name) {
			return true
		}
	}
	return false
}

var builtinJobNames = map[string]bool{
	"repo-sync":     true,
	"janitor":       true,
	"branch-reaper": true,
}

var errNoJobCommand = errors.New("no command configured")

func resolveJobCommand(spec config.JobSpec, dryRun bool) (string, error) {
	configured := spec.Command
	if dryRun {
		configured = spec.DryRunCommand
	}
	if configured != "" {
		return configured, nil
	}
	if !builtinJobNames[spec.Name] {
		return "", fmt.Errorf("job %q: %w", spec.Name, errNoJobCommand)
	}
	execPath, err := os.Executable()
	if err != nil {
		execPath = "digest"
	}
	commandStr := fmt.Sprintf("%s %s", execPath, spec.Name)
	if dryRun {
		commandStr += " --dry-run"
	}
	return commandStr, nil
}

func findJobSpec(cfg *config.Config, jobName string) config.JobSpec {
	if cfg != nil {
		for _, j := range cfg.Jobs {
			if j.Name == jobName {
				return j
			}
		}
	}
	return config.JobSpec{Name: jobName}
}

func executeJobBackground(cfg *config.Config, jobName string) error {
	commandStr, err := resolveJobCommand(findJobSpec(cfg, jobName), false)
	if err != nil {
		return err
	}

	logsDir := getLogsDir()
	activeLog := filepath.Join(logsDir, fmt.Sprintf("%s.log", jobName))
	pidFile := filepath.Join(logsDir, fmt.Sprintf("%s.pid", jobName))

	if _, err := os.Stat(activeLog); err == nil {
		timestamp := time.Now().Format("20060102-150405")
		archivedLog := filepath.Join(logsDir, fmt.Sprintf("%s-%s.log", jobName, timestamp))
		_ = os.Rename(activeLog, archivedLog)
	}

	f, err := os.Create(activeLog)
	if err != nil {
		return err
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", commandStr)
	} else {
		cmd = exec.Command("sh", "-c", commandStr)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setsid: true,
		}
	}

	cmd.Env = os.Environ()

	if home, err := os.UserHomeDir(); err == nil {
		cmd.Dir = home
	}
	if cfg != nil && cfg.NotesDir != "" {
		cmd.Dir = paths.Expand(cfg.NotesDir)
	}

	cmd.Stdout = f
	cmd.Stderr = f

	if err := cmd.Start(); err != nil {
		f.Close()
		return err
	}

	ownPid := strconv.Itoa(cmd.Process.Pid)
	pidWriteErr := os.WriteFile(pidFile, []byte(ownPid), 0644)

	go func() {
		_ = cmd.Wait()
		_ = f.Close()
		if data, err := os.ReadFile(pidFile); err == nil && strings.TrimSpace(string(data)) == ownPid {
			_ = os.Remove(pidFile)
		}
	}()

	if pidWriteErr != nil {
		return fmt.Errorf("job %q started but its pid file could not be written (abort unavailable): %w", jobName, pidWriteErr)
	}
	return nil
}

func signalJobGroup(pid int, sig syscall.Signal) error {
	if runtime.GOOS == "windows" {
		return exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid)).Run()
	}
	return syscall.Kill(-pid, sig)
}

func abortJobCmd(jobName string) tea.Cmd {
	return func() tea.Msg {
		pidFile := filepath.Join(getLogsDir(), fmt.Sprintf("%s.pid", jobName))
		data, err := os.ReadFile(pidFile)
		if err != nil {
			return jobAbortedMsg{jobName: jobName, err: fmt.Errorf("failed to read pid file for %q: %w", jobName, err)}
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			return jobAbortedMsg{jobName: jobName, err: fmt.Errorf("invalid pid file for %q: %w", jobName, err)}
		}

		var abortErr error
		if isProcessAlive(pid) {
			if err := signalJobGroup(pid, syscall.SIGTERM); err != nil {
				abortErr = fmt.Errorf("failed to terminate job %q: %w", jobName, err)
			}
			deadline := time.Now().Add(3 * time.Second)
			for isProcessAlive(pid) && time.Now().Before(deadline) {
				time.Sleep(100 * time.Millisecond)
			}
			if isProcessAlive(pid) {
				if err := signalJobGroup(pid, syscall.SIGKILL); err != nil {
					abortErr = fmt.Errorf("failed to kill job %q: %w", jobName, err)
				}
			}
		}

		activeLog := filepath.Join(getLogsDir(), fmt.Sprintf("%s.log", jobName))
		if f, err := os.OpenFile(activeLog, os.O_WRONLY|os.O_APPEND, 0644); err == nil {
			_, _ = f.WriteString("\n[JOB ABORTED BY USER]\n")
			f.Close()
		}

		if current, err := os.ReadFile(pidFile); err == nil && strings.TrimSpace(string(current)) == strconv.Itoa(pid) {
			_ = os.Remove(pidFile)
		}
		return jobAbortedMsg{jobName: jobName, err: abortErr}
	}
}

func readJobLog(jobName string) string {
	activeLog := filepath.Join(getLogsDir(), fmt.Sprintf("%s.log", jobName))
	data, err := os.ReadFile(activeLog)
	if err != nil || len(data) == 0 {
		return ""
	}
	return string(data)
}

func jobLogModTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

func latestJobOutput(jobName string, dryRunOutput string) (string, bool) {
	realLog := readJobLog(jobName)
	if isJobRunning(jobName) || strings.TrimSpace(dryRunOutput) == "" {
		return realLog, false
	}
	if realLog == "" {
		return dryRunOutput, true
	}
	realModTime := jobLogModTime(filepath.Join(getLogsDir(), fmt.Sprintf("%s.log", jobName)))
	dryRunModTime := jobLogModTime(dryRunFilePath(jobName, "log"))
	if dryRunModTime.After(realModTime) {
		return dryRunOutput, true
	}
	return realLog, false
}

func (m Model) jobDryRunOutputFor(jobName string) string {
	if m.jobDryRunHasRun == nil || !m.jobDryRunHasRun[jobName] {
		return ""
	}
	return m.jobDryRunOutputs[jobName]
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
	gitSpinner         spinner.Model
	gitCancel          context.CancelFunc
	gitFetchCtx        context.Context
	selected           int
	scrollOffset       int
	archivedSelected   int
	ctrlCCount         int
	currentNote        *model.Note
	currentDate        time.Time

	// Dynamic Job Dry-Run & Execution state
	jobDryRunOutputs   map[string]string
	jobDryRunExitCodes map[string]int
	jobDryRunHasRun    map[string]bool

	// Multi-select, Delete, and Job execution state
	archivedSelectedMap map[int]bool
	deleteTargetNotes   []*model.Note
	deleteReturnMode    ViewMode
	jobToExecute        string
	jobToAbort          string

	// Wave and Sync Pulse Animation State
	waveActive       bool
	waveFrame        int
	syncPulseFrame   int
	bannerWaveActive bool
	bannerWaveFrame  int

	// Active git items rendered on screen
	ghReviewedToday       []GitPRItem
	ghReviewedYesterday   []GitPRItem
	ghPendingPRs          []GitPRItem
	prDetails             map[string]json.RawMessage
	pendingSort           sourcecontrol.Sort
	pendingSortChosen     bool
	syncOnLoad            bool
	reviewRuns            []review.ReviewRun
	reviewRunAction       string
	reviewRunTarget       review.PRRef
	reviewRunReturnMode   ViewMode
	localCommitsToday     map[string][]GitPRItem
	localCommitsYesterday map[string][]GitPRItem

	gitPopupRepo     *GitRepoStat
	gitPopupTab      int
	gitPopupSelected int

	previewViewport  viewport.Model
	archivedViewport viewport.Model

	editor      textarea.Model
	searchInput textinput.Model

	searchSelected  int
	searchScroll    int
	searchPreviewID string
	searchNotice    string
	editReturnMode  ViewMode
	inlineInput     textinput.Model

	previewTab     int
	reviewCursor   int
	reviewSelected map[int]bool
	reviewEvent    review.Event
	reviewBody     string
	reviewNotice   string
	rejectInput    textarea.Model
	contextCache   map[string]string
	reviewPolling  bool

	errorTitle      string
	errorLines      []string
	errorReturnMode ViewMode

	width  int
	height int
}

type loadNotesMsg struct {
	notes []*model.Note
	err   error
}

func tickHeaderWaveCmd() tea.Cmd {
	return tea.Tick(35*time.Millisecond, func(t time.Time) tea.Msg {
		return headerWaveTickMsg{}
	})
}

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

func tickCtrlCResetCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return ctrlCResetMsg{}
	})
}

func tickJobLogCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
		return jobLogTickMsg{}
	})
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

	ta := textarea.New()
	ta.Placeholder = "First line: Summary\n\nRemaining lines: Body..."
	ta.ShowLineNumbers = false

	textStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#CDD6F4")).UnsetBackground()
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

	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#F9E2AF"))

	storePath := "~/digest/notes"
	if cfg != nil && cfg.NotesDir != "" {
		storePath = cfg.NotesDir
	}

	rejectArea := textarea.New()
	rejectArea.Placeholder = "Explain what needs to change..."
	rejectArea.ShowLineNumbers = false
	rejectArea.FocusedStyle = ta.FocusedStyle
	rejectArea.BlurredStyle = ta.FocusedStyle

	m := Model{
		rejectInput:         rejectArea,
		reviewSelected:      make(map[int]bool),
		contextCache:        make(map[string]string),
		mode:                ViewDashboard,
		cfg:                 cfg,
		store:               store.New(storePath),
		searchInput:         ti,
		inlineInput:         ii,
		editor:              ta,
		gitSpinner:          sp,
		loadingGit:          true,
		currentDate:         time.Now(),
		archivedSelectedMap: make(map[int]bool),
		jobDryRunOutputs:    make(map[string]string),
		jobDryRunExitCodes:  make(map[string]int),
		jobDryRunHasRun:     make(map[string]bool),
		bannerWaveActive:    true,
	}
	m.pendingSort = sourcecontrol.SortFromCommand(cfg.PendingPRsCommand())
	cache, cacheLoaded := loadGitCache()
	if cacheLoaded {
		if cache.PendingSort != nil {
			m.pendingSort, m.pendingSortChosen = *cache.PendingSort, true
		}
		m.applyGitCache(cache)
	}
	m.syncOnLoad = !cacheLoaded || !cache.hasDataFor(m.currentDate)
	if m.syncOnLoad {
		m.beginGitFetch(true)
	} else {
		m.loadingGit = false
	}
	m.refreshDryRunResults()
	m.refreshReviewRuns()
	m.reviewPolling = m.anyReviewRunning()
	if startupErr != nil {
		m.showError("CONFIG ERROR", startupErr)
	}
	return m
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.loadNotesCmd, tickBannerWaveCmd()}
	if m.syncOnLoad {
		cmds = append(cmds, m.gitFetchCmd(true))
	}
	if m.isAnyDryRunInFlight() {
		cmds = append(cmds, tickDryRunPollCmd())
	}
	if m.reviewPolling {
		cmds = append(cmds, tickReviewPollCmd())
	}
	if m.cfg != nil && m.cfg.GitAutoSyncInterval > 0 {
		cmds = append(cmds, autoSyncTickCmd(m.cfg.GitAutoSyncInterval))
	}
	return tea.Batch(cmds...)
}

func (m Model) loadNotesCmd() tea.Msg {
	notes, err := m.store.List()
	return loadNotesMsg{notes: notes, err: err}
}

func (m Model) saveNotesCmd(notes ...*model.Note) tea.Cmd {
	noteStore := m.store
	copies := make([]model.Note, 0, len(notes))
	for _, n := range notes {
		if n != nil {
			copies = append(copies, *n)
		}
	}
	return func() tea.Msg {
		var errs []error
		for i := range copies {
			if err := noteStore.Save(&copies[i]); err != nil {
				errs = append(errs, fmt.Errorf("save %q: %w", copies[i].Summary, err))
			}
		}
		return notesSavedMsg{errs: errs}
	}
}

func (m Model) deleteNotesCmd(notes ...*model.Note) tea.Cmd {
	noteStore := m.store
	copies := make([]model.Note, 0, len(notes))
	for _, n := range notes {
		if n != nil {
			copies = append(copies, *n)
		}
	}
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
	if m.mode != ViewError {
		m.errorReturnMode = m.mode
		m.errorTitle = title
		m.errorLines = nil
	} else if title != m.errorTitle {
		for i, line := range lines {
			lines[i] = title + ": " + line
		}
	}
	m.errorLines = append(m.errorLines, lines...)
	m.mode = ViewError
}

func (m *Model) startJobDryRunsCmd() tea.Cmd {
	if m.cfg == nil || len(m.cfg.Jobs) == 0 {
		return nil
	}
	started := 0
	var errs []error
	for _, j := range m.cfg.Jobs {
		if isDryRunInFlight(j.Name) {
			continue
		}
		if err := startDryRunBackground(j); err != nil {
			errs = append(errs, err)
		} else {
			started++
		}
	}
	m.refreshDryRunResults()
	if m.mode == ViewPreview {
		m.updatePreviewViewport()
	}
	if len(errs) > 0 {
		m.showError("JOB ERROR", errs...)
	}
	if started == 0 {
		return nil
	}
	return tea.Batch(tickDryRunPollCmd(), tickSyncPulseCmd())
}

func waitForGitSection(sections <-chan sourcecontrol.Section, generation int) tea.Cmd {
	return func() tea.Msg {
		section, open := <-sections
		switch {
		case !open:
			return nil
		case section.Day != nil:
			day := section.Day
			return gitDaySectionMsg{generation: generation, day: day.Day, date: day.Date, reviewed: day.Reviewed, commits: day.Commits, reviews: day.Reviews, details: day.Details, failedHosts: day.FailedHosts, err: day.Err, sections: sections}
		default:
			pending := section.Pending
			return gitPendingMsg{generation: generation, startedAt: pending.StartedAt, pending: pending.Items, details: pending.Details, failedHosts: pending.FailedHosts, err: pending.Err, sections: sections}
		}
	}
}

func (m *Model) beginGitFetch(triggerWave bool) {
	if m.gitCancel != nil {
		m.gitCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.gitFetchCtx = ctx
	m.gitCancel = cancel
	m.fetchGeneration++
	m.loadingGit = true
	m.gitSectionsPending = gitSectionCount

	if triggerWave {
		m.waveActive = true
		m.waveFrame = 0
	}
}

func (m Model) gitFetchCmd(triggerWave bool) tea.Cmd {
	ctx := m.gitFetchCtx
	if ctx == nil {
		ctx = context.Background()
	}
	sections := sourcecontrol.Sync(ctx, sourcecontrol.SyncParams{
		Config:          m.cfg,
		PreviousDetails: maps.Clone(m.prDetails),
		Today:           m.currentDate,
		Sort:            m.pendingSort,
	})

	cmds := []tea.Cmd{
		waitForGitSection(sections, m.fetchGeneration),
		m.gitSpinner.Tick,
		tickSyncPulseCmd(),
	}

	if triggerWave {
		cmds = append(cmds, tickHeaderWaveCmd())
	}

	return tea.Batch(cmds...)
}

func (m *Model) startLoadGitStatsCmd(triggerWave bool) tea.Cmd {
	m.beginGitFetch(triggerWave)
	return m.gitFetchCmd(triggerWave)
}

func navItemKey(item NavItem) string {
	switch {
	case item.Kind == KindJobDraft && item.Draft != nil:
		return "job:" + item.Draft.Name
	case item.Kind == KindReviewRun && item.ReviewRun != nil:
		return "review:" + item.ReviewRun.Meta.Ref.URL
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

const (
	sectionReviewedToday     = "Reviewed today"
	sectionReviewedYesterday = "Reviewed yesterday"
	sectionPending           = "Pending"
)

func (m *Model) finishGitSection() {
	if m.gitSectionsPending > 0 {
		m.gitSectionsPending--
	}
	m.loadingGit = m.gitSectionsPending > 0
}

func (m *Model) recordSectionError(section string, err error) {
	if err == nil {
		delete(m.syncErrors, section)
		return
	}
	if m.syncErrors == nil {
		m.syncErrors = make(map[string]string)
	}
	m.syncErrors[section] = err.Error()
}

func (m *Model) applyGitDay(msg gitDaySectionMsg) {
	if msg.generation != m.fetchGeneration {
		return
	}
	m.finishGitSection()
	selectedKey, selectedOccurrence := m.selectedNavKey()
	m.refreshReviewRuns()

	section, reviewed, commits := sectionReviewedToday, &m.ghReviewedToday, &m.localCommitsToday
	if msg.day == gitDayYesterday {
		section, reviewed, commits = sectionReviewedYesterday, &m.ghReviewedYesterday, &m.localCommitsYesterday
	}
	if m.gitSectionDates == nil {
		m.gitSectionDates = make(map[string]string)
	}
	m.recordSectionError(section, msg.err)
	m.mergePRDetails(msg.details)
	if msg.err == nil || len(msg.reviewed) > 0 || m.gitSectionDates[section] != msg.date {
		var previous []GitPRItem
		if m.gitSectionDates[section] == msg.date {
			previous = *reviewed
		}
		*reviewed = keepFailedHostItems(previous, msg.reviewed, msg.failedHosts)
		sourcecontrol.SortItems(*reviewed, sourcecontrol.Sort{})
		m.gitSectionDates[section] = msg.date
	}
	*commits = msg.commits

	m.rebuildGitRepoStats()
	m.restoreSelection(selectedKey, selectedOccurrence)
	m.updateScrollOffset()
	m.saveGitCacheIfToday()
}

func (m *Model) applyGitPending(msg gitPendingMsg) {
	if msg.generation != m.fetchGeneration {
		return
	}
	m.finishGitSection()
	selectedKey, selectedOccurrence := m.selectedNavKey()
	m.refreshReviewRuns()

	m.recordSectionError(sectionPending, msg.err)
	m.mergePRDetails(msg.details)
	if msg.err == nil || len(msg.pending) > 0 {
		m.ghPendingPRs = keepFailedHostItems(m.ghPendingPRs, msg.pending, msg.failedHosts)
	}

	m.rebuildGitRepoStats()
	m.restoreSelection(selectedKey, selectedOccurrence)
	m.updateScrollOffset()
	m.saveGitCacheIfToday()
}

func keepFailedHostItems(previous, fresh []GitPRItem, failedHosts []string) []GitPRItem {
	if len(failedHosts) == 0 {
		return fresh
	}
	failed := make(map[string]bool, len(failedHosts))
	for _, host := range failedHosts {
		failed[host] = true
	}
	freshURLs := make(map[string]bool, len(fresh))
	for _, item := range fresh {
		freshURLs[item.URL] = true
	}
	kept := append([]GitPRItem(nil), fresh...)
	for _, item := range previous {
		parsed, err := url.Parse(item.URL)
		if err != nil || !failed[parsed.Host] || freshURLs[item.URL] {
			continue
		}
		kept = append(kept, item)
	}
	return kept
}

func (m *Model) rebuildGitRepoStats() {
	buildStats := func(reviewed []GitPRItem, commitMap map[string][]GitPRItem) []*GitRepoStat {
		repoMap := make(map[string]*GitRepoStat)

		getOrCreate := func(name string) *GitRepoStat {
			stat, ok := repoMap[name]
			if !ok {
				stat = &GitRepoStat{Name: name}
				repoMap[name] = stat
			}
			return stat
		}

		for _, item := range reviewed {
			rName := item.Repository
			if rName == "" {
				rName = "general"
			}
			st := getOrCreate(rName)
			st.Items = append(st.Items, item)
			st.Reviewed++
		}

		for rName, cItems := range commitMap {
			st := getOrCreate(rName)
			st.Items = append(st.Items, cItems...)
			st.Commits += len(cItems)
		}

		var stats []*GitRepoStat
		for _, stat := range repoMap {
			stats = append(stats, stat)
		}
		sort.SliceStable(stats, func(i, j int) bool {
			return stats[i].Name < stats[j].Name
		})
		return stats
	}

	m.todayGitRepos = buildStats(m.ghReviewedToday, m.localCommitsToday)
	m.yesterdayGitRepo = buildStats(m.ghReviewedYesterday, m.localCommitsYesterday)
	sourcecontrol.SortItems(m.ghPendingPRs, m.pendingSort)
	m.pendingGitAction = m.ghPendingPRs
}

func (m Model) getJobDrafts() []*JobDraft {
	if m.cfg == nil || len(m.cfg.Jobs) == 0 {
		return nil
	}

	var drafts []*JobDraft
	for _, j := range m.cfg.Jobs {
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
			DryRunInFlight: isDryRunInFlight(j.Name),
		})
	}
	return drafts
}

func (m Model) getYesterdayDoneNotes() []*model.Note {
	yesterday := m.currentDate.AddDate(0, 0, -1)
	var list []*model.Note
	for _, n := range m.notes {
		if n.Status == model.StatusDone && isSameDay(n.Updated, yesterday) {
			list = append(list, n)
		}
	}
	return list
}

func (m Model) getTodayDoneNotes() []*model.Note {
	var list []*model.Note
	for _, n := range m.notes {
		if n.Status == model.StatusDone && isSameDay(n.Updated, m.currentDate) {
			list = append(list, n)
		}
	}
	return list
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

func (m Model) partitionActiveNotes() (todayNotes []*model.Note, olderNotes []*model.Note) {
	for _, n := range m.notes {
		if n.Status == model.StatusDone || n.Status == model.StatusArchived {
			continue
		}

		if isSameDay(n.Created, m.currentDate) {
			todayNotes = append(todayNotes, n)
		} else if n.Created.Before(m.currentDate) {
			olderNotes = append(olderNotes, n)
		}
	}
	return todayNotes, olderNotes
}

func (m Model) getPendingGitGroups() []PendingRepoGroup {
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
	var items []NavItem

	for _, repo := range m.yesterdayGitRepo {
		items = append(items, NavItem{Kind: KindGitRepo, GitRepo: repo})
	}

	for _, n := range m.getYesterdayDoneNotes() {
		items = append(items, NavItem{Kind: KindYesterdayDone, Note: n})
	}

	todayNotes, olderNotes := m.partitionActiveNotes()

	for _, n := range olderNotes {
		items = append(items, NavItem{Kind: KindCarriedNote, Note: n})
	}

	for _, n := range todayNotes {
		items = append(items, NavItem{Kind: KindTodayNote, Note: n})
	}

	for _, n := range m.getTodayDoneNotes() {
		items = append(items, NavItem{Kind: KindTodayDone, Note: n})
	}

	groups := m.getPendingGitGroups()
	for _, g := range groups {
		for i := range g.Items {
			items = append(items, NavItem{Kind: KindPendingGit, PendingGitPR: &g.Items[i]})
		}
	}

	for _, repo := range m.todayGitRepos {
		items = append(items, NavItem{Kind: KindGitRepo, GitRepo: repo})
	}

	for _, d := range m.getJobDrafts() {
		items = append(items, NavItem{Kind: KindJobDraft, Draft: d})
	}

	for i := range m.reviewRuns {
		items = append(items, NavItem{Kind: KindReviewRun, ReviewRun: &m.reviewRuns[i]})
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

func (m Model) computeDashboardLayout() (selectedLineIdx int, totalLines int) {
	var b strings.Builder
	yesterdayDate := m.currentDate.AddDate(0, 0, -1)
	globalIdx := 0

	realToday := time.Now()
	isRealToday := isSameDay(m.currentDate, realToday)

	// --- YESTERDAY SECTION ---
	var yTitleText string
	if isRealToday {
		yTitleText = fmt.Sprintf("Y E S T E R D A Y  ·  %s", strings.ToUpper(yesterdayDate.Format("02 Jan")))
	} else {
		yTitleText = fmt.Sprintf("%s . %s", strings.ToUpper(yesterdayDate.Format("Monday")), strings.ToUpper(yesterdayDate.Format("02 Jan")))
	}
	b.WriteString("\n " + yTitleText + "\n\n")

	gitSummary := "  / Git   commits"
	b.WriteString(gitSummary + "\n")

	for _, repo := range m.yesterdayGitRepo {
		if globalIdx == m.selected {
			selectedLineIdx = strings.Count(b.String(), "\n")
		}
		b.WriteString(m.renderGitRepoRow(repo, false, 80))
		globalIdx++
	}

	b.WriteString("\n")
	yNotes := m.getYesterdayDoneNotes()
	for _, n := range yNotes {
		if globalIdx == m.selected {
			selectedLineIdx = strings.Count(b.String(), "\n")
		}
		b.WriteString(m.renderRow(n, false, 80))
		globalIdx++
	}

	b.WriteString("\n\n")

	var todayTitleText string
	if isRealToday {
		todayTitleText = fmt.Sprintf("T O D A Y  ·  %s", strings.ToUpper(m.currentDate.Format("02 Jan")))
	} else {
		todayTitleText = fmt.Sprintf("%s . %s", strings.ToUpper(m.currentDate.Format("Monday")), strings.ToUpper(m.currentDate.Format("02 Jan")))
	}

	b.WriteString(" " + todayTitleText + "\n\n")

	todayNotes, olderNotes := m.partitionActiveNotes()

	b.WriteString("  / Pending, Carried Over (0)\n")
	if len(olderNotes) == 0 {
		b.WriteString("   (no carried over notes)\n")
	} else {
		for _, n := range olderNotes {
			if globalIdx == m.selected {
				selectedLineIdx = strings.Count(b.String(), "\n")
			}
			b.WriteString(m.renderRow(n, false, 80))
			globalIdx++
		}
	}

	b.WriteString("\n  / Added Today (0)\n")
	if len(todayNotes) == 0 {
		b.WriteString("   (no notes added today)\n")
	} else {
		for _, n := range todayNotes {
			if globalIdx == m.selected {
				selectedLineIdx = strings.Count(b.String(), "\n")
			}
			b.WriteString(m.renderRow(n, false, 80))
			globalIdx++
		}
	}

	b.WriteString("\n  / Closed Today (0)\n")
	todayDoneNotes := m.getTodayDoneNotes()
	if len(todayDoneNotes) == 0 {
		b.WriteString("   (no notes closed today)\n")
	} else {
		for _, n := range todayDoneNotes {
			if globalIdx == m.selected {
				selectedLineIdx = strings.Count(b.String(), "\n")
			}
			b.WriteString(m.renderRow(n, false, 80))
			globalIdx++
		}
	}

	b.WriteString("\n  / Pending Git Actions\n")
	pendingGroups := m.getPendingGitGroups()
	if len(pendingGroups) == 0 {
		b.WriteString("   (no PRs requiring review)\n")
	} else {
		for _, g := range pendingGroups {
			b.WriteString("    " + dimBlueText.Bold(true).Render(g.Name) + "\n")
			for i := range g.Items {
				if globalIdx == m.selected {
					selectedLineIdx = strings.Count(b.String(), "\n")
				}
				b.WriteString(m.renderPendingGitRow(&g.Items[i], false, 80))
				globalIdx++
			}
		}
	}

	if len(m.todayGitRepos) > 0 || m.loadingGit {
		b.WriteString("\n  / Git Updates\n")
		if len(m.todayGitRepos) == 0 {
			b.WriteString("   (no git updates today)\n")
		} else {
			for _, repo := range m.todayGitRepos {
				if globalIdx == m.selected {
					selectedLineIdx = strings.Count(b.String(), "\n")
				}
				b.WriteString(m.renderGitRepoRow(repo, false, 80))
				globalIdx++
			}
		}
	}

	b.WriteString("\n  / Jobs\n")
	drafts := m.getJobDrafts()
	for _, d := range drafts {
		if globalIdx == m.selected {
			selectedLineIdx = strings.Count(b.String(), "\n")
		}
		b.WriteString(m.renderDraftRow(d, false, 80))
		globalIdx++
	}
	for i := range m.reviewRuns {
		if globalIdx == m.selected {
			selectedLineIdx = strings.Count(b.String(), "\n")
		}
		b.WriteString(m.renderReviewRunRow(m.reviewRuns[i], false, 80))
		globalIdx++
	}

	lines := strings.Split(b.String(), "\n")
	return selectedLineIdx, len(lines)
}

func (m *Model) updateScrollOffset() {
	bodyHeight := m.dashboardBodyHeight()
	if bodyHeight < 10 {
		bodyHeight = 10
	}

	if m.selected == 0 {
		m.scrollOffset = 0
		return
	}

	selectedLineIdx, totalLines := m.computeDashboardLayout()

	lookaheadTop := selectedLineIdx - 2
	if lookaheadTop < 0 {
		lookaheadTop = 0
	}

	if lookaheadTop < m.scrollOffset {
		m.scrollOffset = lookaheadTop
	} else if selectedLineIdx >= m.scrollOffset+bodyHeight {
		m.scrollOffset = selectedLineIdx - bodyHeight + 1
	}

	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
	if m.scrollOffset > totalLines-bodyHeight && totalLines > bodyHeight {
		m.scrollOffset = totalLines - bodyHeight
	}
}

func previewModalSize(width, height int) (int, int, int) {
	modalWidth := modalWidthFor(width)
	innerHeight := height - 14
	if innerHeight < 4 {
		innerHeight = 4
	}
	return modalWidth, modalWidth - 6, innerHeight
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
		mdContent = renderMarkdown(fmt.Sprintf("# Repository Activity: %s\n- Commits: %d\n- PRs Reviewed: %d\n- PRs Assigned: %d\n\nPress **[Tab]** on this repository item to view and open PRs directly in your browser.", item.GitRepo.Name, item.GitRepo.Commits, item.GitRepo.Reviewed, item.GitRepo.Assigned), innerWidth)
	} else if item.Kind == KindPendingGit && item.PendingGitPR != nil {
		m.setPRPreviewContent(item.PendingGitPR, innerWidth, innerHeight)
		return
	} else if item.Kind == KindJobDraft && item.Draft != nil {
		logText, isDryRunOutput := latestJobOutput(item.Draft.Name, m.jobDryRunOutputFor(item.Draft.Name))
		if !isJobRunning(item.Draft.Name) && item.Draft.DryRunInFlight {
			mdContent = renderMarkdown(fmt.Sprintf("# Job: %s (DRY RUN IN PROGRESS)\n\nDry run in progress...", item.Draft.Name), innerWidth)
		} else if logText != "" {
			statusHeader := "FINISHED / LOG OUTPUT"
			if isJobRunning(item.Draft.Name) {
				statusHeader = "LIVE EXECUTION LOG"
			} else if isDryRunOutput {
				statusHeader = "DRY RUN ANALYSIS"
			}
			mdContent = renderMarkdown(fmt.Sprintf("# Job: %s (%s)\n\n```\n%s\n```", item.Draft.Name, statusHeader, strings.TrimSpace(logText)), innerWidth)
		} else {
			mdContent = renderMarkdown(fmt.Sprintf("# Job: %s\n\nNo dry-run analysis or log output available.\nPress **[r]** on dashboard to run dry-run check, or press **[Enter]** to execute job.", item.Draft.Name), innerWidth)
		}
	} else if item.Kind == KindReviewRun && item.ReviewRun != nil {
		mdContent = renderMarkdown(reviewRunPreview(m.reviewRoot(), *item.ReviewRun), innerWidth)
	} else if item.Note != nil {
		fullText := fmt.Sprintf("# %s", item.Note.Summary)
		if strings.TrimSpace(item.Note.Body) != "" {
			fullText += fmt.Sprintf("\n\n%s", item.Note.Body)
		}
		mdContent = renderMarkdown(fullText, innerWidth)
	}

	m.previewViewport = viewport.New(innerWidth, innerHeight)
	m.previewViewport.SetContent(mdContent)
	if item.Kind == KindJobDraft || item.Kind == KindReviewRun {
		m.previewViewport.GotoBottom()
	}
}

func (m Model) renderArchivedContent(width int, selectedIdx int) string {
	archived := m.getArchivedNotes()
	if len(archived) == 0 {
		return mutedStyle.Render("(No archived notes)")
	}

	var b strings.Builder
	currentGroupDate := ""

	for i, n := range archived {
		dateStr := n.Updated.Local().Format("Monday 02 Jan 2006")
		if dateStr != currentGroupDate {
			if currentGroupDate != "" {
				b.WriteString("\n")
			}
			b.WriteString(subSectionStyle.Render(dateStr) + "\n")
			currentGroupDate = dateStr
		}

		prefix := "  "
		if m.archivedSelectedMap != nil && m.archivedSelectedMap[i] {
			prefix = amberDiamond.Render() + " "
		}

		box := checkDone.Render()
		if n.Status != model.StatusDone {
			box = checkPending.Render()
		}

		age := n.Updated.Local().Format("15:04")
		src := string(n.Source)
		if src == "" {
			src = "manual"
		}
		if !strings.HasPrefix(src, "#") {
			src = "#" + src
		}

		summaryWidth := width - len(prefix) - 2 - len(src) - len(age) - 6
		if summaryWidth < 10 {
			summaryWidth = 10
		}

		summary := n.Summary
		if len(summary) > summaryWidth {
			summary = summary[:summaryWidth-3] + "..."
		}

		gap := summaryWidth - len(summary)

		var line string
		if i == selectedIdx {
			renderedSummary := selectedSummaryStyle.Render(summary)
			line = fmt.Sprintf("%s%s %s%s%s   %s\n",
				prefix,
				box,
				renderedSummary,
				safeRepeat(" ", gap),
				selectedTagStyle.Render(src),
				mutedStyle.Bold(true).Render(age),
			)
		} else {
			line = fmt.Sprintf("%s%s %s%s%s   %s\n",
				prefix,
				box,
				summary,
				safeRepeat(" ", gap),
				dimBlueText.Render(src),
				mutedStyle.Render(age),
			)
		}
		b.WriteString(line)
	}

	return b.String()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case headerWaveTickMsg:
		if m.waveActive {
			m.waveFrame++
			if m.waveFrame > 30 {
				m.waveActive = false
				return m, nil
			}
			return m, tickHeaderWaveCmd()
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
		if m.loadingGit || m.isAnyJobRunning() || m.isAnyDryRunInFlight() || m.anyReviewRunning() {
			m.syncPulseFrame++
			return m, tickSyncPulseCmd()
		}
		return m, nil

	case jobLogTickMsg:
		if m.mode == ViewPreview {
			wasAtBottom := m.previewViewport.AtBottom()
			m.updatePreviewViewport()
			if wasAtBottom {
				m.previewViewport.GotoBottom()
			}
			return m, tickJobLogCmd()
		}
		return m, nil

	case dryRunPollTickMsg:
		m.refreshDryRunResults()
		if m.mode == ViewPreview {
			m.updatePreviewViewport()
		}
		if m.isAnyDryRunInFlight() {
			return m, tickDryRunPollCmd()
		}
		return m, nil

	case jobAbortedMsg:
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
		return m, m.loadNotesCmd

	case autoSyncTickMsg:
		if m.cfg != nil && m.cfg.GitAutoSyncInterval > 0 {
			return m, tea.Batch(
				m.startLoadGitStatsCmd(false),
				autoSyncTickCmd(m.cfg.GitAutoSyncInterval),
			)
		}
		return m, nil

	case ctrlCResetMsg:
		m.ctrlCCount = 0
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		_, editorWidth, editorHeight := previewModalSize(msg.Width, msg.Height)
		m.editor.SetWidth(editorWidth)
		m.editor.SetHeight(editorHeight)
		m.updateScrollOffset()
		if m.mode == ViewPreview {
			m.updatePreviewViewport()
		}
		if m.mode == ViewSearchPreview {
			m.updateSearchPreviewViewport()
		}

	case loadNotesMsg:
		m.notes = msg.notes
		m.updateScrollOffset()
		if m.mode == ViewSearchPreview {
			m.updateSearchPreviewViewport()
		}
		if m.mode == ViewArchived {
			m.refreshArchivedViewport()
		}
		if msg.err != nil {
			m.showError("STORE ERROR", msg.err)
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
		m.applyGitDay(msg)
		return m.afterGitSection(cmds)

	case gitPendingMsg:
		var cmds []tea.Cmd
		if msg.generation == m.fetchGeneration && msg.err == nil {
			cmds = append(cmds, reopenApprovedNotesCmd(m.store, msg.pending, msg.startedAt))
		}
		if msg.sections != nil {
			cmds = append(cmds, waitForGitSection(msg.sections, msg.generation))
		}
		m.applyGitPending(msg)
		return m.afterGitSection(cmds)

	case approvalSyncMsg:
		return m, m.startLoadGitStatsCmd(false)

	case reviewPollTickMsg:
		return m.handleReviewPoll()

	case reviewSubmittedMsg:
		return m.handleReviewSubmitted(msg)

	case reviewCloneReadyMsg:
		return m.handleCloneReady(msg)

	case spinner.TickMsg:
		m.gitSpinner, cmd = m.gitSpinner.Update(msg)
		if m.loadingGit {
			return m, cmd
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func isSameDay(t1, t2 time.Time) bool {
	y1, m1, d1 := t1.Local().Date()
	y2, m2, d2 := t2.Local().Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

func renderMarkdown(body string, width int) string {
	if strings.TrimSpace(body) == "" {
		return mutedStyle.Render("(No note body text)")
	}

	style := styles.DarkStyleConfig
	style.Document.StylePrimitive.BackgroundColor = nil
	style.Paragraph.StylePrimitive.BackgroundColor = nil
	style.Heading.StylePrimitive.BackgroundColor = nil
	style.H1.StylePrimitive.BackgroundColor = nil
	style.H2.StylePrimitive.BackgroundColor = nil
	style.H3.StylePrimitive.BackgroundColor = nil
	style.H4.StylePrimitive.BackgroundColor = nil
	style.H5.StylePrimitive.BackgroundColor = nil
	style.H6.StylePrimitive.BackgroundColor = nil
	style.BlockQuote.StylePrimitive.BackgroundColor = nil
	style.Code.StylePrimitive.BackgroundColor = nil
	style.CodeBlock.StylePrimitive.BackgroundColor = nil
	if style.CodeBlock.Chroma != nil {
		style.CodeBlock.Chroma.Background.BackgroundColor = nil
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return body
	}

	out, err := r.Render(body)
	if err != nil {
		return body
	}
	return strings.TrimSpace(out)
}

func (m Model) renderWaveTitle(title string, isActive bool) string {
	baseStyle := sectionTitleStyle.Copy()
	if isActive {
		baseStyle = baseStyle.Underline(true)
	}

	if !m.waveActive {
		return baseStyle.Render(title)
	}
	return renderWave(title, m.waveFrame, baseStyle)
}

func renderWave(text string, frame int, baseStyle lipgloss.Style) string {
	var sb strings.Builder
	runes := []rune(text)

	dimStyle := baseStyle.Copy().Foreground(lipgloss.Color("#585B70"))
	centerStyle := baseStyle.Copy().Foreground(lipgloss.Color("#00FFFF")).Bold(true)
	innerGlowStyle := baseStyle.Copy().Foreground(lipgloss.Color("#89B4FA")).Bold(true)
	outerGlowStyle := baseStyle.Copy().Foreground(lipgloss.Color("#74C7EC"))

	for i, r := range runes {
		diff := i - frame
		if diff < 0 {
			diff = -diff
		}

		switch diff {
		case 0:
			sb.WriteString(centerStyle.Render(string(r)))
		case 1:
			sb.WriteString(innerGlowStyle.Render(string(r)))
		case 2:
			sb.WriteString(outerGlowStyle.Render(string(r)))
		default:
			sb.WriteString(dimStyle.Render(string(r)))
		}
	}
	return sb.String()
}

func (m Model) renderLiveSyncDot() string {
	if !m.loadingGit {
		return dotSynced.Render()
	}

	pulseColors := []string{
		"#45475A", "#585B70", "#6C7086", "#74C7EC",
		"#89B4FA", "#B4BEFE", "#C6A0F6", "#F5C2E7",
		"#F9E2AF", "#F5C2E7", "#B4BEFE", "#74C7EC",
	}

	idx := m.syncPulseFrame % len(pulseColors)
	dotStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(pulseColors[idx])).Bold(true)
	textStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#A6ADC8"))

	return fmt.Sprintf("%s %s", dotStyle.Render("●"), textStyle.Render("syncing..."))
}

func (m Model) renderDryRunIndicator() string {
	pulseColors := []string{
		"#F9E2AF", "#EED49F", "#F5BDE6", "#C6A0F6",
		"#89B4FA", "#74C7EC", "#8BD5CA", "#A6E3A1",
	}
	idx := m.syncPulseFrame % len(pulseColors)
	dotStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(pulseColors[idx])).Bold(true)
	textStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#F9E2AF")).Bold(true)
	return fmt.Sprintf("%s %s", dotStyle.Render("●"), textStyle.Render("dry run..."))
}

func (m Model) renderJobRunningIndicator() string {
	pulseColors := []string{
		"#F9E2AF", "#EED49F", "#F5BDE6", "#C6A0F6",
		"#89B4FA", "#74C7EC", "#8BD5CA", "#A6E3A1",
	}
	idx := m.syncPulseFrame % len(pulseColors)
	dotStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(pulseColors[idx])).Bold(true)
	textStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#F9E2AF")).Bold(true)
	return fmt.Sprintf("%s %s", dotStyle.Render("●"), textStyle.Render("running..."))
}

func (m Model) renderSubSection(title string, count int, showCount bool, isActive bool) string {
	baseStyle := subSectionStyle.Copy()
	if isActive {
		baseStyle = baseStyle.Underline(true)
	}
	if showCount {
		countStr := mutedStyle.Render(fmt.Sprintf("(%d)", count))
		return fmt.Sprintf("%s %s", baseStyle.Render("/ "+title), countStr)
	}
	return baseStyle.Render("/ " + title)
}

func (m Model) renderErrorModal(modalWidth int) string {
	title := m.errorTitle
	if title == "" {
		title = "ERROR"
	}
	titleText := deleteTitleStyle.Render(" " + title + " ")
	errorText := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#F38BA8")).
		Width(modalWidth - 6).
		Render(strings.Join(m.errorLines, "\n"))

	footerText := renderModalFooter(footerItemsFrom(errorBindings()), modalWidth-6)

	popupContent := lipgloss.JoinVertical(
		lipgloss.Left,
		titleText,
		"",
		errorText,
		"",
		footerText,
	)

	modal := modalStyle.Width(modalWidth).Render(popupContent)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, fitPopup(modal, m.width, m.height))
}

func (m Model) View() string {
	if m.width < 40 {
		return "Terminal window is too small."
	}

	dashboardView := lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderHeader(),
		m.renderDashboardBody(),
		m.renderFooter(),
	)

	modalWidth := modalWidthFor(m.width)
	innerWidth := modalWidth - 6

	switch m.mode {

	case ViewGitDetails:
		if m.gitPopupRepo == nil {
			return dashboardView
		}

		titleText := modalTitleStyle.Render(fmt.Sprintf(" GIT DETAILS: %s ", m.gitPopupRepo.Name))

		tabNames := []string{"All", "Reviewed", "Assigned", "Commits"}
		var renderedTabs []string
		for i, name := range tabNames {
			if i == m.gitPopupTab {
				renderedTabs = append(renderedTabs, tabActiveStyle.Render(name))
			} else {
				renderedTabs = append(renderedTabs, tabInactiveStyle.Render(name))
			}
		}
		tabsRow := strings.Join(renderedTabs, " ")

		footerText := renderModalFooter(footerItemsFrom(gitDetailsBindings()), modalWidth-6)
		fixedHeight := lipgloss.Height(lipgloss.JoinVertical(lipgloss.Left, titleText, "\n"+tabsRow, "", "", footerText))
		listRows := max(1, previewContentHeight(m.height)-fixedHeight)

		var listLines []string
		items := m.filteredGitItems()

		if len(items) == 0 {
			listLines = append(listLines, "  "+mutedStyle.Render("(no items in this tab)"))
		} else {
			firstRow, lastRow := visibleGitRows(m.gitPopupSelected, len(items), listRows)
			for i := firstRow; i < lastRow; i++ {
				item := items[i]
				prefix := "  "
				kindTag := fmt.Sprintf("[%s]", item.Kind)

				titleWidth := innerWidth - len(kindTag) - 6
				title := item.Title
				if len(title) > titleWidth && titleWidth > 5 {
					title = title[:titleWidth-3] + "..."
				}

				gap := titleWidth - len(title)
				if gap < 1 {
					gap = 1
				}

				if i == m.gitPopupSelected {
					renderedTitle := selectedSummaryStyle.Render(title)
					listLines = append(listLines, fmt.Sprintf("%s%s%s %s", prefix, renderedTitle, safeRepeat(" ", gap), selectedTagStyle.Render(kindTag)))
				} else {
					listLines = append(listLines, fmt.Sprintf("%s%s%s %s", prefix, itemStyle.Render(title), safeRepeat(" ", gap), mutedStyle.Render(kindTag)))
				}
			}
		}
		for len(listLines) < listRows {
			listLines = append(listLines, "")
		}

		popupContent := lipgloss.JoinVertical(
			lipgloss.Left,
			titleText,
			"\n"+tabsRow,
			"",
			strings.Join(listLines, "\n"),
			"",
			footerText,
		)

		modal := modalStyle.Width(modalWidth).Render(popupContent)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, fitPopup(modal, m.width, m.height))

	case ViewDeleteConfirm:
		var titleText string
		var prompt string

		if m.jobToAbort != "" {
			titleText = deleteTitleStyle.Render(" ABORT JOB ")
			prompt = fmt.Sprintf("Are you sure you want to abort running job '%s'?", m.jobToAbort)
		} else if m.jobToExecute != "" {
			titleText = modalTitleStyle.Render(" EXECUTE JOB ")
			prompt = fmt.Sprintf("Are you sure you want to run '%s'?", m.jobToExecute)
		} else {
			titleText = deleteTitleStyle.Render(" DELETE CONFIRMATION ")
			if len(m.deleteTargetNotes) > 1 {
				prompt = fmt.Sprintf("Are you sure you want to permanently delete these %d selected notes?", len(m.deleteTargetNotes))
			} else if len(m.deleteTargetNotes) == 1 {
				if m.deleteReturnMode == ViewArchived {
					prompt = fmt.Sprintf("Are you sure you want to permanently delete this note?\n\n\"%s\"", m.deleteTargetNotes[0].Summary)
				} else {
					prompt = fmt.Sprintf("Are you sure you want to archive this note?\n\n\"%s\"", m.deleteTargetNotes[0].Summary)
				}
			} else {
				prompt = "No notes selected for deletion."
			}
		}

		footerText := renderModalFooter(footerItemsFrom(deleteConfirmBindings()), modalWidth-6)

		popupContent := lipgloss.JoinVertical(
			lipgloss.Left,
			titleText,
			"",
			prompt,
			"",
			footerText,
		)

		modal := modalStyle.Width(modalWidth).Render(popupContent)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, fitPopup(modal, m.width, m.height))

	case ViewError:
		return m.renderErrorModal(modalWidth)

	case ViewReviewConfirm:
		return m.renderReviewConfirm(modalWidth)

	case ViewReviewRunConfirm:
		return m.renderReviewRunConfirm(modalWidth)

	case ViewRejectComment:
		return m.renderRejectComment(modalWidth)

	case ViewArchived:
		titleText := modalTitleStyle.Render(" ARCHIVED NOTES ")

		innerHeight := m.height - 10 - footerLineCount(archiveFooterItems)
		if innerHeight < 4 {
			innerHeight = 4
		}
		m.archivedViewport.Height = innerHeight

		footerText := renderModalFooter(archiveFooterItems, modalWidth-6)

		popupContent := lipgloss.JoinVertical(
			lipgloss.Left,
			titleText,
			"",
			m.archivedViewport.View(),
			"",
			footerText,
		)

		modal := modalStyle.Width(modalWidth).Render(popupContent)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, fitPopup(modal, m.width, m.height))

	case ViewPreview:
		navItems := m.allNavItems()
		headerTitle := " PREVIEW "
		statusBadge := badgeActive.Render("IDLE")
		tagBadge := tagStyle.Render("#general")

		if len(navItems) > 0 && m.selected < len(navItems) {
			item := navItems[m.selected]
			switch item.Kind {
			case KindGitRepo:
				headerTitle = fmt.Sprintf(" GIT REPO: %s ", item.GitRepo.Name)
				statusBadge = badgeActive.Render("SYNCED")
				tagBadge = tagStyle.Render("#git")
			case KindPendingGit:
				if item.PendingGitPR != nil {
					headerTitle = fmt.Sprintf(" PENDING PR REVIEW: %s ", item.PendingGitPR.Repository)
					if item.PendingGitPR.Kind == sourcecontrol.ReReviewKind {
						headerTitle = fmt.Sprintf(" RE-REVIEW: %s ", item.PendingGitPR.Repository)
					}
					statusBadge = m.prStateBadge(item.PendingGitPR)
					if pid, running := m.reviewPIDFor(item.PendingGitPR); running {
						statusBadge = badgeActive.Render(fmt.Sprintf("RUNNING · PID %d", pid))
					}
					tagBadge = tagStyle.Render("#github")
				}
			case KindReviewRun:
				headerTitle = fmt.Sprintf(" REVIEW JOB: %s ", reviewRunLabel(*item.ReviewRun))
				statusBadge = stateStyle(review.StateFailed).Render("FAILED")
				if pid, running := review.RunningPID(review.StateDir(m.reviewRoot(), item.ReviewRun.Meta.Ref)); running {
					statusBadge = badgeActive.Render(fmt.Sprintf("RUNNING · PID %d", pid))
				}
				tagBadge = tagStyle.Render("#github")
			case KindJobDraft:
				headerTitle = fmt.Sprintf(" JOB: %s ", item.Draft.Name)
				if pid, running := runningJobPID(item.Draft.Name); running {
					statusBadge = badgeActive.Render(fmt.Sprintf("RUNNING · PID %d", pid))
				} else if item.Draft.DryRunInFlight {
					statusBadge = badgeActive.Render("DRY RUNNING")
				} else if logText, isDryRunOutput := latestJobOutput(item.Draft.Name, m.jobDryRunOutputFor(item.Draft.Name)); logText != "" && !isDryRunOutput {
					statusBadge = badgeDone.Render("FINISHED")
				} else if item.Draft.HasRunDryRun {
					if item.Draft.ExitCode == 0 {
						statusBadge = badgeDone.Render("SUCCESS")
					} else {
						statusBadge = lipgloss.NewStyle().Foreground(lipgloss.Color("#F38BA8")).Bold(true).Render("NEED ACT")
					}
				} else {
					statusBadge = badgeActive.Render("IDLE")
				}
				tagBadge = tagStyle.Render("#job")
			default:
				if item.Note != nil {
					headerTitle = " PREVIEW NOTE "
					if item.Note.Status == model.StatusDone {
						statusBadge = badgeDone.Render("DONE")
					}
					if item.Note.Subject != "" {
						tagBadge = tagStyle.Render("#" + item.Note.Subject)
					}
				}
			}
		}

		headerLeft := modalTitleStyle.Render(headerTitle)
		rightCol := lipgloss.JoinVertical(lipgloss.Right, statusBadge, tagBadge)
		gap := innerWidth - lipgloss.Width(headerLeft) - lipgloss.Width(rightCol)

		topLine := lipgloss.JoinHorizontal(lipgloss.Top, headerLeft, safeRepeat(" ", gap), rightCol)

		prItem := m.currentPRItem()
		items := footerItemsFrom(m.previewBindings())

		footerText := renderModalFooter(items, modalWidth-6)

		partsAbove := []string{topLine, ""}
		if prItem != nil {
			partsAbove = append(partsAbove, m.renderPreviewTabs(), "")
		}
		partsBelow := []string{""}
		if prItem != nil && m.reviewNotice != "" {
			partsBelow = append(partsBelow, yellowBadgeStyle.Render(m.reviewNotice), "")
		}
		partsBelow = append(partsBelow, footerText)
		fixedHeight := lipgloss.Height(lipgloss.JoinVertical(lipgloss.Left, partsAbove...)) + lipgloss.Height(lipgloss.JoinVertical(lipgloss.Left, partsBelow...))
		m.previewViewport.Height = max(3, previewContentHeight(m.height)-fixedHeight)
		previewParts := append(append(partsAbove, m.previewViewport.View()), partsBelow...)
		popupContent := lipgloss.JoinVertical(lipgloss.Left, previewParts...)

		modal := modalStyle.Width(modalWidth).Render(popupContent)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, fitPopup(modal, m.width, m.height))

	case ViewEdit:
		titleText := modalTitleStyle.Render(" ADD / EDIT NOTE ")

		footerText := renderModalFooter(footerItemsFrom(editBindings()), modalWidth-6)

		popupContent := lipgloss.JoinVertical(
			lipgloss.Left,
			titleText,
			"",
			m.editor.View(),
			"",
			footerText,
		)

		modal := modalStyle.Width(modalWidth).Render(popupContent)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, fitPopup(modal, m.width, m.height))

	case ViewSearch:
		return m.renderSearchModal(modalWidth)

	case ViewSearchPreview:
		return m.renderSearchPreview(modalWidth)
	}

	if toast := m.renderSyncErrorToast(); toast != "" {
		return overlayTopRight(dashboardView, toast, m.width)
	}
	return dashboardView
}

func scrollViewport(view *viewport.Model, key string) bool {
	switch key {
	case "j", "down":
		view.ScrollDown(1)
	case "k", "up":
		view.ScrollUp(1)
	case "pgdown":
		view.PageDown()
	case "pgup":
		view.PageUp()
	case "ctrl+d":
		view.HalfPageDown()
	case "ctrl+u":
		view.HalfPageUp()
	default:
		return false
	}
	return true
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
		return m, m.ensureReviewPoll()
	}
	if item.Kind == KindJobDraft && item.Draft != nil && isJobRunning(item.Draft.Name) {
		return m, tickJobLogCmd()
	}
	return m, nil
}

func (m Model) renderSyncErrorToast() string {
	if len(m.syncErrors) == 0 {
		return ""
	}
	sections := make([]string, 0, len(m.syncErrors))
	for section := range m.syncErrors {
		sections = append(sections, section)
	}
	sort.Strings(sections)
	toastWidth := min(max(m.width/3, 36)*11/10, m.width-2)
	lines := []string{staleStyle.Render("GIT SYNC FAILED")}
	for _, section := range sections {
		lines = append(lines, ansi.Truncate(section+": "+m.syncErrors[section], toastWidth-4, "…"))
	}
	lines = append(lines, mutedStyle.Render("esc dismiss"))
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#F38BA8")).
		Padding(0, 1).
		Width(toastWidth).
		Render(strings.Join(lines, "\n"))
}

func overlayTopRight(base, box string, width int) string {
	baseLines := strings.Split(base, "\n")
	boxLines := strings.Split(box, "\n")
	boxWidth := lipgloss.Width(box)
	leftWidth := width - boxWidth - 1
	if leftWidth < 0 {
		leftWidth = 0
	}
	for index, boxLine := range boxLines {
		row := index + 1
		if row >= len(baseLines) {
			break
		}
		left := ansi.Truncate(baseLines[row], leftWidth, "")
		if gap := leftWidth - lipgloss.Width(left); gap > 0 {
			left += strings.Repeat(" ", gap)
		}
		baseLines[row] = left + "\x1b[0m" + boxLine
	}
	return strings.Join(baseLines, "\n")
}

const (
	bannerTopRow    = "█▀▄ █ █▀▀ █▀▀ █▀ ▀█▀"
	bannerBottomRow = "█▄▀ █ █▄█ ██▄ ▄█ ░█░"
)

func bannerWaveLastFrame() int {
	return len([]rune(bannerTopRow)) + 2
}

func (m Model) renderBannerLine(line string) string {
	if !m.bannerWaveActive {
		return headerTitleStyle.Render(line)
	}
	return renderWave(line, m.bannerWaveFrame, headerTitleStyle)
}

func (m Model) renderHeader() string {
	nowStr := m.currentDate.Format("Monday 02 Jan")

	renderedAscii1 := m.renderBannerLine(bannerTopRow)
	renderedAscii2 := m.renderBannerLine(bannerBottomRow)
	renderedDate := mutedStyle.Bold(true).Render("— " + nowStr)

	topLine := fmt.Sprintf("%s   %s", renderedAscii1, renderedDate)
	bottomLine := renderedAscii2

	gapTop := m.width - lipgloss.Width(topLine) - 4
	gapBottom := m.width - lipgloss.Width(bottomLine) - 4

	topBorder := borderStyle.Render("┌" + safeRepeat("─", m.width-2) + "┐")
	line1 := fmt.Sprintf("│ %s%s │", topLine, safeRepeat(" ", gapTop))
	line2 := fmt.Sprintf("│ %s%s │", bottomLine, safeRepeat(" ", gapBottom))
	divider := borderStyle.Render("├" + safeRepeat("─", m.width-2) + "┤")

	return fmt.Sprintf("%s\n%s\n%s\n%s", topBorder, line1, line2, divider)
}

func (m Model) renderDashboardBody() string {
	bodyHeight := m.dashboardBodyHeight()
	if bodyHeight < 10 {
		bodyHeight = 10
	}

	var b strings.Builder
	innerWidth := m.width - 4

	yesterdayDate := m.currentDate.AddDate(0, 0, -1)

	// Calculate counts to determine globalIdx ranges
	yGitCount := len(m.yesterdayGitRepo)
	yNotesCount := len(m.getYesterdayDoneNotes())
	todayNotes, olderNotes := m.partitionActiveNotes()
	carriedCount := len(olderNotes)
	addedCount := len(todayNotes)
	todayDoneNotes := m.getTodayDoneNotes()
	closedCount := len(todayDoneNotes)

	pendingGroups := m.getPendingGitGroups()
	pendingGitCount := 0
	for _, g := range pendingGroups {
		pendingGitCount += len(g.Items)
	}

	gitUpdatesCount := len(m.todayGitRepos)
	drafts := m.getJobDrafts()
	draftsCount := len(drafts) + len(m.reviewRuns)

	// Calculate index boundaries
	yGitStart := 0
	yGitEnd := yGitStart + yGitCount

	yNotesStart := yGitEnd
	yNotesEnd := yNotesStart + yNotesCount

	carriedStart := yNotesEnd
	carriedEnd := carriedStart + carriedCount

	addedStart := carriedEnd
	addedEnd := addedStart + addedCount

	closedStart := addedEnd
	closedEnd := closedStart + closedCount

	pendingGitStart := closedEnd
	pendingGitEnd := pendingGitStart + pendingGitCount

	gitUpdatesStart := pendingGitEnd
	gitUpdatesEnd := gitUpdatesStart + gitUpdatesCount

	draftsStart := gitUpdatesEnd

	// Evaluate active header flags
	isYGitActive := m.selected >= yGitStart && m.selected < yGitEnd
	isYesterdayActive := m.selected >= yGitStart && m.selected < yNotesEnd

	isCarriedActive := m.selected >= carriedStart && m.selected < carriedEnd
	isAddedActive := m.selected >= addedStart && m.selected < addedEnd
	isClosedActive := m.selected >= closedStart && m.selected < closedEnd
	isPendingGitActive := m.selected >= pendingGitStart && m.selected < pendingGitEnd
	isGitUpdatesActive := m.selected >= gitUpdatesStart && m.selected < gitUpdatesEnd
	isDraftsActive := m.selected >= draftsStart && m.selected < draftsStart+draftsCount
	isTodayActive := m.selected >= carriedStart && m.selected < draftsStart+draftsCount

	globalIdx := 0

	realToday := time.Now()
	isRealToday := isSameDay(m.currentDate, realToday)

	// --- YESTERDAY SECTION ---
	var yTitleText string
	if isRealToday {
		yTitleText = fmt.Sprintf("Y E S T E R D A Y  ·  %s", strings.ToUpper(yesterdayDate.Format("02 Jan")))
	} else {
		yTitleText = fmt.Sprintf("%s . %s", strings.ToUpper(yesterdayDate.Format("Monday")), strings.ToUpper(yesterdayDate.Format("02 Jan")))
	}
	b.WriteString("\n " + m.renderWaveTitle(yTitleText, isYesterdayActive) + "\n\n")

	totalCommits := 0
	for _, r := range m.yesterdayGitRepo {
		totalCommits += r.Commits
	}

	gitLabel := m.renderSubSection("Git", 0, false, isYGitActive)
	gitStatus := m.renderLiveSyncDot()

	gitSummary := fmt.Sprintf("  %s %s   %d commits", gitLabel, gitStatus, totalCommits)

	b.WriteString(gitSummary + "\n")

	for _, repo := range m.yesterdayGitRepo {
		isSel := (globalIdx == m.selected)
		b.WriteString(m.renderGitRepoRow(repo, isSel, innerWidth))
		globalIdx++
	}

	b.WriteString("\n")
	yNotes := m.getYesterdayDoneNotes()
	for _, n := range yNotes {
		isSel := (globalIdx == m.selected)
		b.WriteString(m.renderRow(n, isSel, innerWidth))
		globalIdx++
	}

	// --- TODAY SECTION ---
	b.WriteString("\n\n")
	jobBadge := fmt.Sprintf("%d jobs %s", len(drafts), amberDiamond.Render())

	var todayTitleText string
	if isRealToday {
		todayTitleText = fmt.Sprintf("T O D A Y  ·  %s", strings.ToUpper(m.currentDate.Format("02 Jan")))
	} else {
		todayTitleText = fmt.Sprintf("%s . %s", strings.ToUpper(m.currentDate.Format("Monday")), strings.ToUpper(m.currentDate.Format("02 Jan")))
	}

	renderedTodayTitle := m.renderWaveTitle(todayTitleText, isTodayActive)
	todayGap := innerWidth - lipgloss.Width(renderedTodayTitle) - lipgloss.Width(jobBadge)
	b.WriteString(fmt.Sprintf(" %s%s%s\n\n", renderedTodayTitle, safeRepeat(" ", todayGap), jobBadge))

	// 1. TODAY: Pending, Carried Over
	b.WriteString("  " + m.renderSubSection("Pending, Carried Over", carriedCount, true, isCarriedActive) + "\n")
	if len(olderNotes) == 0 {
		b.WriteString(mutedStyle.Render("   (no carried over notes)\n"))
	} else {
		for _, n := range olderNotes {
			isSel := (globalIdx == m.selected)
			b.WriteString(m.renderRow(n, isSel, innerWidth))
			globalIdx++
		}
	}

	// 2. TODAY: Added Today
	b.WriteString("\n  " + m.renderSubSection("Added Today", addedCount, true, isAddedActive) + "\n")
	if len(todayNotes) == 0 {
		b.WriteString(mutedStyle.Render("   (no notes added today)\n"))
	} else {
		for _, n := range todayNotes {
			isSel := (globalIdx == m.selected)
			b.WriteString(m.renderRow(n, isSel, innerWidth))
			globalIdx++
		}
	}

	// 3. TODAY: Closed Today
	b.WriteString("\n  " + m.renderSubSection("Closed Today", closedCount, true, isClosedActive) + "\n")
	if len(todayDoneNotes) == 0 {
		b.WriteString(mutedStyle.Render("   (no notes closed today)\n"))
	} else {
		for _, n := range todayDoneNotes {
			isSel := (globalIdx == m.selected)
			b.WriteString(m.renderRow(n, isSel, innerWidth))
			globalIdx++
		}
	}

	// 4. TODAY: Pending Git Actions
	pendingGitHeader := m.renderSubSection("Pending Git Actions", 0, false, isPendingGitActive)
	pendingStatus := m.renderLiveSyncDot()
	b.WriteString(fmt.Sprintf("\n  %s  %s  %s\n", pendingGitHeader, m.renderPendingSortHint(), pendingStatus))

	if len(pendingGroups) == 0 {
		if m.loadingGit {
			b.WriteString(mutedStyle.Render("   (checking pending PR reviews...)\n"))
		} else {
			b.WriteString(mutedStyle.Render("   (no PRs requiring review)\n"))
		}
	} else {
		for _, g := range pendingGroups {
			b.WriteString("    " + dimBlueText.Bold(true).Render(g.Name) + "\n")
			for i := range g.Items {
				isSel := (globalIdx == m.selected)
				b.WriteString(m.renderPendingGitRow(&g.Items[i], isSel, innerWidth))
				globalIdx++
			}
		}
	}

	// 5. TODAY: Git Updates
	if len(m.todayGitRepos) > 0 || m.loadingGit {
		gitUpdatesHeader := m.renderSubSection("Git Updates", 0, false, isGitUpdatesActive)
		gitUpdatesStatus := m.renderLiveSyncDot()
		b.WriteString(fmt.Sprintf("\n  %s %s\n", gitUpdatesHeader, gitUpdatesStatus))

		if len(m.todayGitRepos) == 0 && m.loadingGit {
			b.WriteString(mutedStyle.Render("   (checking git updates...)\n"))
		} else if len(m.todayGitRepos) == 0 {
			b.WriteString(mutedStyle.Render("   (no git updates today)\n"))
		} else {
			for _, repo := range m.todayGitRepos {
				isSel := (globalIdx == m.selected)
				b.WriteString(m.renderGitRepoRow(repo, isSel, innerWidth))
				globalIdx++
			}
		}
	}

	// 6. TODAY: Jobs
	b.WriteString("\n  " + m.renderSubSection("Jobs", 0, false, isDraftsActive) + "\n")
	if draftsCount == 0 {
		b.WriteString(mutedStyle.Render("   (no jobs configured)\n"))
	} else {
		for _, d := range drafts {
			isSel := (globalIdx == m.selected)
			b.WriteString(m.renderDraftRow(d, isSel, innerWidth))
			globalIdx++
		}
		for i := range m.reviewRuns {
			isSel := (globalIdx == m.selected)
			b.WriteString(m.renderReviewRunRow(m.reviewRuns[i], isSel, innerWidth))
			globalIdx++
		}
	}

	lines := strings.Split(b.String(), "\n")

	scrollOffset := m.scrollOffset
	if scrollOffset < 0 {
		scrollOffset = 0
	}
	if scrollOffset > len(lines)-bodyHeight && len(lines) > bodyHeight {
		scrollOffset = len(lines) - bodyHeight
	}

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

func (m Model) renderGitRepoRow(repo *GitRepoStat, selected bool, width int) string {
	rightColWidth := 26
	if width < 60 {
		rightColWidth = 20
	}
	leftWidth := width - rightColWidth
	if leftWidth < 15 {
		leftWidth = 15
	}

	prefix := "     "
	maxNameWidth := leftWidth - len(prefix)
	if maxNameWidth < 5 {
		maxNameWidth = 5
	}

	name := repo.Name
	if lipgloss.Width(name) > maxNameWidth {
		name = name[:maxNameWidth-3] + "..."
	}

	var leftBlock string
	if selected {
		leftBlock = prefix + selectedSummaryStyle.Render(name)
	} else {
		leftBlock = prefix + subSectionStyle.Render(name)
	}

	leftPadding := leftWidth - lipgloss.Width(leftBlock)
	if leftPadding < 0 {
		leftPadding = 0
	}

	var statParts []string
	statParts = append(statParts, fmt.Sprintf("%d commits", repo.Commits))
	if repo.Reviewed > 0 {
		statParts = append(statParts, fmt.Sprintf("%d reviewed", repo.Reviewed))
	}
	if repo.Assigned > 0 {
		statParts = append(statParts, fmt.Sprintf("%d assigned", repo.Assigned))
	}
	stats := strings.Join(statParts, " · ")

	var rightBlock string
	if selected {
		rightBlock = selectedTagStyle.Render(stats)
	} else {
		rightBlock = mutedStyle.Render(stats)
	}

	return fmt.Sprintf("%s%s%s\n", leftBlock, safeRepeat(" ", leftPadding), rightBlock)
}

func (m Model) renderPendingGitRow(item *GitPRItem, selected bool, width int) string {
	rightColWidth := 26
	if width < 60 {
		rightColWidth = 20
	}
	leftWidth := width - rightColWidth
	if leftWidth < 15 {
		leftWidth = 15
	}

	prefix := "      "
	icon := pendingPRIcon.Render()
	maxTitleWidth := leftWidth - len(prefix) - 2
	if maxTitleWidth < 5 {
		maxTitleWidth = 5
	}

	title := item.Title
	if lipgloss.Width(title) > maxTitleWidth {
		title = title[:maxTitleWidth-3] + "..."
	}

	var leftBlock string
	if selected {
		leftBlock = fmt.Sprintf("%s%s %s", prefix, icon, selectedSummaryStyle.Render(title))
	} else {
		leftBlock = fmt.Sprintf("%s%s %s", prefix, icon, itemStyle.Render(title))
	}

	leftPadding := leftWidth - lipgloss.Width(leftBlock)
	if leftPadding < 0 {
		leftPadding = 0
	}

	kindTag := fmt.Sprintf("[%s]", item.Kind)
	var rightBlock string
	if selected {
		rightBlock = selectedTagStyle.Render(kindTag)
	} else {
		rightBlock = mutedStyle.Render(kindTag)
	}
	if item.PR != nil {
		rightBlock = m.renderPRTag(item, selected)
	}

	return fmt.Sprintf("%s%s%s\n", leftBlock, safeRepeat(" ", leftPadding), rightBlock)
}

func (m Model) renderDraftRow(draft *JobDraft, selected bool, width int) string {
	rightColWidth := 26
	if width < 60 {
		rightColWidth = 20
	}
	leftWidth := width - rightColWidth
	if leftWidth < 15 {
		leftWidth = 15
	}

	prefix := "   "

	var icon string
	if draft.HasRunDryRun && draft.ExitCode == 0 {
		icon = checkDone.Render()
	} else {
		icon = amberDiamond.Render()
	}

	label := draft.Name
	maxLabelWidth := leftWidth - len(prefix) - 2
	if maxLabelWidth < 5 {
		maxLabelWidth = 5
	}

	if lipgloss.Width(label) > maxLabelWidth {
		label = label[:maxLabelWidth-3] + "..."
	}

	var leftBlock string
	if selected {
		leftBlock = fmt.Sprintf("%s%s %s", prefix, icon, selectedSummaryStyle.Render(label))
	} else {
		leftBlock = fmt.Sprintf("%s%s %s", prefix, icon, itemStyle.Render(label))
	}

	leftPadding := leftWidth - lipgloss.Width(leftBlock)
	if leftPadding < 0 {
		leftPadding = 0
	}

	var rightBlock string
	if isJobRunning(draft.Name) {
		runningIndicator := m.renderJobRunningIndicator()
		rightBlock = fmt.Sprintf("%s   %s", selectedTagStyle.Render("#job"), runningIndicator)
	} else if draft.DryRunInFlight {
		rightBlock = fmt.Sprintf("%s   %s", selectedTagStyle.Render("#job"), m.renderDryRunIndicator())
	} else {
		statusText := "need to act"
		if draft.HasRunDryRun && draft.ExitCode == 0 {
			statusText = "success"
		}

		if selected {
			rightBlock = fmt.Sprintf("%s   %s", selectedTagStyle.Render("#job"), mutedStyle.Bold(true).Render(statusText))
		} else {
			rightBlock = fmt.Sprintf("%s   %s", dimBlueText.Render("#job"), mutedStyle.Render(statusText))
		}
	}

	return fmt.Sprintf("%s%s%s\n", leftBlock, safeRepeat(" ", leftPadding), rightBlock)
}

func (m Model) renderRow(n *model.Note, selected bool, width int) string {
	rightColWidth := 26
	if width < 60 {
		rightColWidth = 20
	}

	leftWidth := width - rightColWidth
	if leftWidth < 15 {
		leftWidth = 15
	}

	boxChar := "☐"
	if n.Status == model.StatusDone {
		boxChar = "✔"
	}

	prefix := "   "

	src := string(n.Source)
	if src == "" {
		src = "manual"
	}
	if !strings.HasPrefix(src, "#") {
		src = "#" + src
	}

	var age string
	var isCarriedOver bool
	if !n.Created.IsZero() {
		if n.Status != model.StatusDone && !isSameDay(n.Created, m.currentDate) && n.Created.Before(m.currentDate) {
			isCarriedOver = true
			days := daysAgo(n.Created, m.currentDate)
			age = fmt.Sprintf("%dd ago", days)
		} else {
			if !isSameDay(n.Created, m.currentDate) {
				age = n.Created.Format("Mon")
			} else {
				age = n.Created.Format("15:04")
			}
		}
	} else {
		age = "08:40"
	}

	maxSummaryWidth := leftWidth - len(prefix) - 2
	if maxSummaryWidth < 5 {
		maxSummaryWidth = 5
	}

	summary := n.Summary
	if lipgloss.Width(summary) > maxSummaryWidth {
		summary = summary[:maxSummaryWidth-3] + "..."
	}

	isTodayDone := (n.Status == model.StatusDone && isSameDay(n.Updated, m.currentDate))

	var leftBlock string
	if selected && m.mode == ViewInlineEdit {
		m.inlineInput.Width = maxSummaryWidth
		leftBlock = fmt.Sprintf("%s%s %s", prefix, checkPending.Render(), m.inlineInput.View())
	} else if selected {
		boxStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#CDD6F4"))
		if n.Status == model.StatusDone {
			boxStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A6E3A1"))
		}

		sumStyle := selectedSummaryStyle.Copy()
		if isTodayDone {
			sumStyle = sumStyle.Strikethrough(true)
		}
		leftBlock = fmt.Sprintf("%s%s %s", prefix, boxStyle.Render(boxChar), sumStyle.Render(summary))
	} else {
		box := checkPending.Render()
		if n.Status == model.StatusDone {
			box = checkDone.Render()
		}

		renderedSummary := summary
		if isTodayDone {
			renderedSummary = itemStyle.Copy().Strikethrough(true).Render(summary)
		}
		leftBlock = fmt.Sprintf("%s%s %s", prefix, box, renderedSummary)
	}

	leftPadding := leftWidth - lipgloss.Width(leftBlock)
	if leftPadding < 0 {
		leftPadding = 0
	}

	var srcRendered string
	if selected {
		srcRendered = selectedTagStyle.Render(src)
	} else {
		srcRendered = dimBlueText.Render(src)
	}

	var ageRendered string
	if isCarriedOver {
		ageRendered = yellowBadgeStyle.Render(age)
	} else if selected {
		ageRendered = mutedStyle.Bold(true).Render(age)
	} else {
		ageRendered = mutedStyle.Render(age)
	}

	rightBlock := fmt.Sprintf("%s   %s", srcRendered, ageRendered)

	return fmt.Sprintf("%s%s%s\n", leftBlock, safeRepeat(" ", leftPadding), rightBlock)
}

var archiveFooterItems = footerItemsFrom(archivedBindings())

func (m Model) dashboardBodyHeight() int {
	return m.height - 6 - footerLineCount(footerItemsFrom(m.dashboardBindings()))
}

func (m Model) renderFooter() string {
	items := footerItemsFrom(m.dashboardBindings())
	keysLine, firstActionLine, secondActionLine := renderFooterLines(items)
	lines := []string{keysLine, firstActionLine}
	if footerLineCount(items) == 3 {
		lines = append(lines, secondActionLine)
	}

	topBorder := borderStyle.Render("├" + safeRepeat("─", m.width-2) + "┤")
	bottomBorder := borderStyle.Render("└" + safeRepeat("─", m.width-2) + "┘")

	rows := []string{topBorder}
	for _, line := range lines {
		padded := " " + line + " "
		rows = append(rows, "│"+padded+safeRepeat(" ", m.width-lipgloss.Width(padded)-2)+"│")
	}
	rows = append(rows, bottomBorder)
	return strings.Join(rows, "\n")
}

func Run(cfg *config.Config, startupErr error) error {
	p := tea.NewProgram(NewModel(cfg, startupErr), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

var noteLinkPattern = regexp.MustCompile(`https?://[^\s<>()"']+`)
var pullRequestPathPattern = regexp.MustCompile(`/[^/\s]+/pull/\d+`)

func prReviewNoteURL(note *model.Note) string {
	if note == nil || note.Source != model.SourcePRReview {
		return ""
	}
	for _, link := range noteLinkPattern.FindAllString(note.Body, -1) {
		if pullRequestPathPattern.MatchString(link) {
			return link
		}
	}
	return ""
}

func previewModalHeight(terminalHeight int) int {
	return max(12, terminalHeight-4)
}

func previewContentHeight(terminalHeight int) int {
	return previewModalHeight(terminalHeight) - modalStyle.GetVerticalFrameSize()
}

func visibleGitRows(selected, total, rowsAvailable int) (first, last int) {
	if rowsAvailable < 1 {
		rowsAvailable = 1
	}
	if total <= rowsAvailable {
		return 0, total
	}
	first = max(0, selected-rowsAvailable+1)
	return first, first + rowsAvailable
}

func modalWidthFor(digestWidth int) int {
	return min(digestWidth*90/100, digestWidth-modalStyle.GetHorizontalBorderSize())
}

func fitPopup(popup string, terminalWidth, terminalHeight int) string {
	lines := strings.Split(popup, "\n")
	if len(lines) > terminalHeight {
		keptBottom := terminalHeight / 2
		lines = append(lines[:terminalHeight-keptBottom], lines[len(lines)-keptBottom:]...)
	}
	for index, line := range lines {
		if lipgloss.Width(line) > terminalWidth {
			lines[index] = ansi.Truncate(line, terminalWidth, "")
		}
	}
	return strings.Join(lines, "\n")
}

func footerColumnWidth(item footerItem) int {
	firstWord, secondWord := splitFooterAction(item.action)
	return max(lipgloss.Width(item.key), lipgloss.Width(firstWord), lipgloss.Width(secondWord))
}

func splitFooterRows(items []footerItem, maxWidth int) [][]footerItem {
	var rows [][]footerItem
	var currentRow []footerItem
	rowWidth := 0
	for _, item := range items {
		columnWidth := footerColumnWidth(item)
		if len(currentRow) > 0 && rowWidth+3+columnWidth > maxWidth {
			rows = append(rows, currentRow)
			currentRow, rowWidth = nil, 0
		}
		if len(currentRow) > 0 {
			rowWidth += 3
		}
		currentRow = append(currentRow, item)
		rowWidth += columnWidth
	}
	if len(currentRow) > 0 {
		rows = append(rows, currentRow)
	}
	return rows
}

func (m *Model) applyPendingSort() {
	m.pendingSortChosen = true
	selectedKey, selectedOccurrence := m.selectedNavKey()
	m.rebuildGitRepoStats()
	m.restoreSelection(selectedKey, selectedOccurrence)
	m.updateScrollOffset()
	m.savePendingSort()
}

func (m *Model) changePendingSort(toggle func(*sourcecontrol.Sort)) tea.Cmd {
	toggle(&m.pendingSort)
	m.applyPendingSort()
	if !m.loadingGit {
		return nil
	}
	return m.startLoadGitStatsCmd(false)
}

func (m Model) renderPendingSortHint() string {
	field, direction := "Updated", "↓ Desc"
	if m.pendingSort.ByCreated {
		field = "Created"
	}
	if m.pendingSort.Ascending {
		direction = "↑ Asc"
	}
	return fmt.Sprintf("%s %s  %s %s", keyStyle.Render("s"), mutedStyle.Render(field), keyStyle.Render("w"), mutedStyle.Render(direction))
}
