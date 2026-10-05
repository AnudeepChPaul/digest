package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"app/pkg/brag"
	"app/pkg/config"
	"app/pkg/model"
	"app/pkg/review"
	"app/pkg/sourcecontrol"
	"app/pkg/store"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	glamouransi "github.com/charmbracelet/glamour/ansi"
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
	ViewHelp
	ViewBragList
	ViewBragView
	ViewBragConfirm
	ViewBragEdit
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
	BragRun      *brag.Run
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

type daySyncDueMsg struct {
	generation int
}

const daySyncDelay = 2 * time.Second

type jobLogTickMsg struct{}

const jobLogTailLines = 400

type runStatePollTickMsg struct{}

type localReviewState struct {
	status     review.RunStatus
	finishedAt time.Time
	finished   bool
	pid        int
}

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

var digestRoot = (*config.Config)(nil).Root()

var createdLogsDirs sync.Map

func getLogsDir() string {
	dir := filepath.Join(digestRoot, "logs")
	if _, created := createdLogsDirs.Load(dir); !created {
		if os.MkdirAll(dir, 0755) == nil {
			createdLogsDirs.Store(dir, true)
		}
	}
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
	output, _ := readDryRunLog(dryRunFilePath(jobName, "log"))
	return string(output), exitCode, true
}

var readDryRunLog = os.ReadFile

func dryRunStamp(jobName string) string {
	var stamp strings.Builder
	for _, suffix := range []string{"exit", "log"} {
		if info, err := os.Stat(dryRunFilePath(jobName, suffix)); err == nil {
			fmt.Fprintf(&stamp, "%s:%d:%d|", suffix, info.Size(), info.ModTime().UnixNano())
		}
	}
	return stamp.String()
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

func tickRunStatePollCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return runStatePollTickMsg{}
	})
}

func (m *Model) ensureRunStatePoll() tea.Cmd {
	if m.runStatePolling {
		return nil
	}
	m.runStatePolling = true
	return tickRunStatePollCmd()
}

func (m *Model) refreshJobStates() {
	runningPIDs := make(map[string]int)
	inFlight := make(map[string]bool)
	if m.cfg != nil {
		for _, j := range m.cfg.Jobs {
			if pid, running := runningJobPID(j.Name); running {
				runningPIDs[j.Name] = pid
			}
			if isDryRunInFlight(j.Name) {
				inFlight[j.Name] = true
			}
		}
	}
	m.runningJobPIDs, m.dryRunsInFlight = runningPIDs, inFlight
}

func (m Model) jobRunning(jobName string) bool {
	return m.runningJobPIDs[jobName] > 0
}

func (m Model) isAnyDryRunInFlight() bool {
	return len(m.dryRunsInFlight) > 0
}

func (m *Model) refreshDryRunResults() {
	m.refreshJobStates()
	if m.cfg == nil {
		return
	}
	if m.jobDryRunOutputs == nil {
		m.jobDryRunOutputs = make(map[string]string)
		m.jobDryRunExitCodes = make(map[string]int)
		m.jobDryRunHasRun = make(map[string]bool)
	}
	if m.dryRunLogStamps == nil {
		m.dryRunLogStamps = make(map[string]string)
	}
	for _, j := range m.cfg.Jobs {
		if m.dryRunsInFlight[j.Name] {
			continue
		}
		stamp := dryRunStamp(j.Name)
		if previous, seen := m.dryRunLogStamps[j.Name]; seen && previous == stamp {
			continue
		}
		m.dryRunLogStamps[j.Name] = stamp
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
	return len(m.runningJobPIDs) > 0
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

const jobLogArchiveFormat = "20060102-150405"

func pruneJobLogArchives(logsDir, jobName string, retentionDays int, now time.Time) {
	archives, _ := filepath.Glob(filepath.Join(logsDir, jobName+"-*.log"))
	cutoff := now.AddDate(0, 0, -retentionDays)
	for _, archive := range archives {
		stamp := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(archive), jobName+"-"), ".log")
		if _, err := time.Parse(jobLogArchiveFormat, stamp); err != nil {
			continue
		}
		if info, err := os.Stat(archive); err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(archive)
		}
	}
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
		timestamp := time.Now().Format(jobLogArchiveFormat)
		archivedLog := filepath.Join(logsDir, fmt.Sprintf("%s-%s.log", jobName, timestamp))
		_ = os.Rename(activeLog, archivedLog)
	}
	pruneJobLogArchives(logsDir, jobName, cfg.Retention(), time.Now())

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
	if cfg != nil {
		cmd.Dir = cfg.NotesDir()
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

const jobLogTailBytes = 256 * 1024

func readFileTail(path string, maxBytes int64) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() == 0 {
		return ""
	}
	offset := max(info.Size()-maxBytes, 0)
	data := make([]byte, info.Size()-offset)
	if _, err := file.ReadAt(data, offset); err != nil && !errors.Is(err, io.EOF) {
		return ""
	}
	if offset > 0 {
		if newline := bytes.IndexByte(data, '\n'); newline >= 0 {
			data = data[newline+1:]
		}
	}
	return string(data)
}

func readJobLog(jobName string) string {
	return readFileTail(filepath.Join(getLogsDir(), fmt.Sprintf("%s.log", jobName)), jobLogTailBytes)
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
	sessionCtx         context.Context
	startupNotesErr    error
	cancelSession      context.CancelFunc
	searchCache        *searchMemo
	reviewReports      *reviewReportMemo
	updateSeq          int
	scrollPending      bool
	settledFrame       *dashboardFrame
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

	syncPulseRunning      bool
	headerWaveRunning     bool
	jobLogRunning         bool
	jobLogStamp           string
	runStatePolling       bool
	runningJobPIDs        map[string]int
	dryRunsInFlight       map[string]bool
	localReviews          map[string]localReviewState
	previewJobLogFinished bool
	previewFindingsCount  int
	bragStates            map[string]bragRowState
	historyRequested      map[string]bool

	// Active git items rendered on screen
	ghReviewedToday         []GitPRItem
	ghReviewedYesterday     []GitPRItem
	ghPendingPRs            []GitPRItem
	prDetails               map[string]json.RawMessage
	pendingSort             sourcecontrol.Sort
	pendingMeOnly           bool
	initialSelectionPending bool
	pendingSortChosen       bool
	syncOnLoad              bool
	reviewRuns              []review.ReviewRun
	reviewRunAction         string
	reviewRunTarget         review.PRRef
	reviewRunReturnMode     ViewMode
	localCommitsToday       map[string][]GitPRItem
	tagSlots                rowTagSlots
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

func (m *Model) ensureSyncPulse() tea.Cmd {
	if m.syncPulseRunning {
		return nil
	}
	m.syncPulseRunning = true
	return tickSyncPulseCmd()
}

func (m Model) anythingBusy() bool {
	return m.loadingGit || m.loadingCommits || m.isAnyJobRunning() || m.isAnyDryRunInFlight() || m.anyReviewRunning() || m.anyBragRunning()
}

func (m *Model) restartHeaderWave() tea.Cmd {
	m.waveActive = true
	m.waveFrame = 0
	if m.headerWaveRunning {
		return nil
	}
	m.headerWaveRunning = true
	return tickHeaderWaveCmd()
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

func (m *Model) ensureJobLogRefresh() tea.Cmd {
	if m.jobLogRunning {
		return nil
	}
	m.jobLogRunning = true
	return tickJobLogCmd()
}

func jobLogStampFor(jobName string, running bool) string {
	stamp := strconv.FormatBool(running)
	for _, path := range []string{filepath.Join(getLogsDir(), jobName+".log"), dryRunFilePath(jobName, "log")} {
		if info, err := os.Stat(path); err == nil {
			stamp += fmt.Sprintf("|%d:%d", info.Size(), info.ModTime().UnixNano())
		}
	}
	return stamp
}

func (m Model) previewedRunningJob() string {
	if m.mode != ViewPreview {
		return ""
	}
	navItems := m.allNavItems()
	if m.selected >= len(navItems) {
		return ""
	}
	if item := navItems[m.selected]; item.Kind == KindJobDraft && item.Draft != nil {
		return item.Draft.Name
	}
	return ""
}

func (m Model) handleJobLogTick() (tea.Model, tea.Cmd) {
	jobName := m.previewedRunningJob()
	if jobName == "" {
		m.jobLogRunning = false
		return m, nil
	}
	m.refreshJobStates()
	running := m.jobRunning(jobName)
	if stamp := jobLogStampFor(jobName, running); stamp != m.jobLogStamp {
		m.jobLogStamp = stamp
		wasAtBottom := m.previewViewport.AtBottom()
		previousOffset := m.previewViewport.YOffset
		m.updatePreviewViewport()
		if wasAtBottom {
			m.previewViewport.GotoBottom()
		} else {
			m.previewViewport.SetYOffset(previousOffset)
		}
	}
	if !running {
		m.jobLogRunning = false
		return m, nil
	}
	return m, tickJobLogCmd()
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

	storePath := cfg.NotesDir()
	digestRoot = cfg.Root()

	rejectArea := textarea.New()
	rejectArea.Placeholder = "Explain what needs to change..."
	rejectArea.ShowLineNumbers = false
	rejectArea.FocusedStyle = ta.FocusedStyle
	rejectArea.BlurredStyle = ta.FocusedStyle

	m := Model{
		initialSelectionPending: true,
		rejectInput:             rejectArea,
		reviewSelected:          make(map[int]bool),
		contextCache:            make(map[string]string),
		mode:                    ViewDashboard,
		cfg:                     cfg,
		store:                   store.New(storePath),
		searchInput:             ti,
		inlineInput:             ii,
		editor:                  ta,
		searchCache:             &searchMemo{},
		reviewReports:           &reviewReportMemo{},
		loadingGit:              true,
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
	m.fetchedPreviousDay = m.previousNoteDay()
	cache, cacheLoaded := loadGitCache()
	if cacheLoaded {
		if cache.PendingSort != nil {
			m.pendingSort, m.pendingSortChosen = *cache.PendingSort, true
		}
		m.applyGitCache(cache)
	}
	m.syncOnLoad = !cacheLoaded || !cache.hasDataFor(m.currentDate) || cache.PreviousDay != m.fetchedPreviousDay.Format("2006-01-02")
	if m.syncOnLoad {
		m.beginGitFetch(true)
	} else {
		m.loadingGit = false
	}
	m.refreshDryRunResults()
	m.refreshReviewRuns()
	m.refreshBragRuns()
	m.reviewPolling = m.anyReviewRunning() || m.anyBragRunning()
	m.syncPulseRunning = m.anythingBusy()
	m.headerWaveRunning = m.waveActive
	m.runStatePolling = m.isAnyDryRunInFlight() || m.isAnyJobRunning()
	if startupErr != nil {
		m.showError("CONFIG ERROR", startupErr)
	}
	return m
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.startupNotesCmd(), tickBannerWaveCmd(), m.loadCommitsCmd()}
	if m.syncPulseRunning {
		cmds = append(cmds, tickSyncPulseCmd())
	}
	if m.headerWaveRunning {
		cmds = append(cmds, tickHeaderWaveCmd())
	}
	if m.syncOnLoad {
		cmds = append(cmds, m.gitFetchCmd())
	}
	if m.runStatePolling {
		cmds = append(cmds, tickRunStatePollCmd())
	}
	if m.reviewPolling {
		cmds = append(cmds, tickReviewPollCmd())
	}
	if m.cfg != nil && m.cfg.GitAutoSyncInterval > 0 {
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
		if m.dryRunsInFlight[j.Name] {
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
	return tea.Batch(m.ensureRunStatePoll(), m.ensureSyncPulse())
}

func waitForGitSection(sections <-chan sourcecontrol.Section, generation int) tea.Cmd {
	return func() tea.Msg {
		section, open := <-sections
		switch {
		case !open:
			return nil
		case section.Day != nil:
			day := section.Day
			return gitDaySectionMsg{generation: generation, day: day.Day, date: day.Date, reviewed: day.Reviewed, reviews: day.Reviews, details: day.Details, failedHosts: day.FailedHosts, err: day.Err, sections: sections}
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
	m.fetchedPreviousDay = m.previousNoteDay()
	m.loadingGit = true
	m.gitSectionsPending = gitSectionCount

	if triggerWave {
		m.waveActive = true
		m.waveFrame = 0
	}
}

func (m Model) gitFetchCmd() tea.Cmd {
	ctx := m.gitFetchCtx
	if ctx == nil {
		ctx = context.Background()
	}
	sections := sourcecontrol.Sync(ctx, sourcecontrol.SyncParams{
		Config:      m.cfg,
		Today:       m.currentDate,
		PreviousDay: m.previousNoteDay(),
		Sort:        m.pendingSort,
	})

	return waitForGitSection(sections, m.fetchGeneration)
}

func (m *Model) cancelGitSync() {
	if m.gitCancel != nil {
		m.gitCancel()
	}
	m.fetchGeneration++
	if m.commitsCancel != nil {
		m.commitsCancel()
	}
	m.commitsGeneration++
}

func (m *Model) scheduleDaySync() tea.Cmd {
	m.cancelGitSync()
	m.loadingGit = true
	m.loadingCommits = m.cfg.DailyCommitsEnabled()
	m.daySyncGeneration++
	generation := m.daySyncGeneration
	return tea.Batch(tea.Tick(daySyncDelay, func(time.Time) tea.Msg { return daySyncDueMsg{generation: generation} }), m.ensureSyncPulse())
}

func (m *Model) startLoadGitStatsCmd(triggerWave bool) tea.Cmd {
	m.daySyncGeneration++
	m.beginGitFetch(triggerWave)
	cmds := []tea.Cmd{m.gitFetchCmd(), m.refreshCommitsCmd(), m.ensureSyncPulse()}
	if triggerWave {
		cmds = append(cmds, m.restartHeaderWave())
	}
	return tea.Batch(cmds...)
}

func navItemKey(item NavItem) string {
	switch {
	case item.Kind == KindJobDraft && item.Draft != nil:
		return "job:" + item.Draft.Name
	case item.Kind == KindReviewRun && item.ReviewRun != nil:
		return "review:" + item.ReviewRun.Meta.Ref.URL
	case item.Kind == KindBragRun && item.BragRun != nil:
		return "brag:" + item.BragRun.Meta.ID
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

func (m *Model) selectLaunchItem() {
	items := m.allNavItems()
	for _, preferredKind := range []NavItemKind{KindTodayNote, KindCarriedNote} {
		if index := slices.IndexFunc(items, func(item NavItem) bool { return item.Kind == preferredKind }); index >= 0 {
			m.selected = index
			return
		}
	}
	m.selected = 0
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

	section, reviewed := sectionReviewedToday, &m.ghReviewedToday
	if msg.day == gitDayYesterday {
		section, reviewed = sectionReviewedYesterday, &m.ghReviewedYesterday
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

	m.refreshReviewRuns()
	m.rebuildGitRepoStats()
	m.restoreSelection(selectedKey, selectedOccurrence)
	m.updateScrollOffset()
}

func (m *Model) applyGitPending(msg gitPendingMsg) {
	if msg.generation != m.fetchGeneration {
		return
	}
	m.finishGitSection()
	selectedKey, selectedOccurrence := m.selectedNavKey()

	m.recordSectionError(sectionPending, msg.err)
	m.mergePRDetails(msg.details)
	if msg.err == nil || len(msg.pending) > 0 {
		m.ghPendingPRs = keepFailedHostItems(m.ghPendingPRs, msg.pending, msg.failedHosts)
	}

	m.refreshReviewRuns()
	m.rebuildGitRepoStats()
	m.restoreSelection(selectedKey, selectedOccurrence)
	m.updateScrollOffset()
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

	commitsToday, commitsYesterday := m.localCommitsToday, m.localCommitsYesterday
	if !m.cfg.DailyCommitsEnabled() {
		commitsToday, commitsYesterday = nil, nil
	}
	m.todayGitRepos = buildStats(m.ghReviewedToday, commitsToday)
	m.yesterdayGitRepo = buildStats(m.ghReviewedYesterday, commitsYesterday)
	sourcecontrol.SortItems(m.ghPendingPRs, m.pendingSort)
	m.pendingGitAction = m.ghPendingPRs
	if m.pendingMeOnly {
		m.pendingGitAction = slices.DeleteFunc(slices.Clone(m.ghPendingPRs), func(item GitPRItem) bool {
			return item.PR == nil || !item.PR.DirectRequest
		})
	}
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

func (m Model) previousDayTitle() string {
	return m.previousDayTitleFor(m.previousNoteDay())
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

func (m Model) getYesterdayDoneNotes() []*model.Note {
	return m.groupNotes().previousDone
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

	for _, repo := range m.todayGitRepos {
		items = append(items, NavItem{Kind: KindGitRepo, GitRepo: repo})
	}

	groups := m.groupNotes()
	for _, n := range groups.previousDone {
		items = append(items, NavItem{Kind: KindYesterdayDone, Note: n})
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

	for _, g := range m.getPendingGitGroups() {
		for i := range g.Items {
			items = append(items, NavItem{Kind: KindPendingGit, PendingGitPR: &g.Items[i]})
		}
	}

	for _, d := range m.getJobDrafts() {
		items = append(items, NavItem{Kind: KindJobDraft, Draft: d})
	}

	for i := range m.reviewRuns {
		items = append(items, NavItem{Kind: KindReviewRun, ReviewRun: &m.reviewRuns[i]})
	}

	for i := range m.bragRuns {
		items = append(items, NavItem{Kind: KindBragRun, BragRun: &m.bragRuns[i]})
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

func (m Model) renderGitStrip(width int, active bool) (lines []string, selectedRow int) {
	leftColumnWidth := max((width-2)/2, 20)
	columnWidths := [2]int{leftColumnWidth, max(width-2-leftColumnWidth, 20)}
	header := " " + m.renderWaveTitle("G I T", active) + "  " + m.renderSyncDot(m.loadingCommits)
	columns := [2][]*GitRepoStat{m.yesterdayGitRepo, m.todayGitRepos}
	columnTitles := [2]string{
		m.previousDayTitle(),
		m.dayTitleText(m.currentDate, "T O D A Y"),
	}
	var captionCells [2]string
	for side, repos := range columns {
		firstIndex := 0
		if side == 1 {
			firstIndex = len(m.yesterdayGitRepo)
		}
		columnActive := m.selected >= firstIndex && m.selected < firstIndex+len(repos)
		caption := " " + m.renderWaveTitle(columnTitles[side], columnActive)
		if m.cfg.DailyCommitsEnabled() {
			commits := 0
			for _, repo := range repos {
				commits += repo.Commits
			}
			caption += mutedStyle.Render(fmt.Sprintf("  %d commits", commits))
		}
		captionCells[side] = ansi.Truncate(caption, columnWidths[side], "…")
	}
	separator := mutedStyle.Render(" │")
	joinCells := func(left, right string) string {
		return left + safeRepeat(" ", leftColumnWidth-lipgloss.Width(left)) + separator + right
	}
	lines = []string{header, "", joinCells(captionCells[0], captionCells[1])}
	selectedRow = -1
	rowCount := max(len(m.yesterdayGitRepo), len(m.todayGitRepos), 1)
	for row := range rowCount {
		var cells [2]string
		for side, repos := range columns {
			globalIndex := row
			if side == 1 {
				globalIndex += len(m.yesterdayGitRepo)
			}
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
		lines = append(lines, joinCells(cells[0], cells[1]))
	}
	return lines, selectedRow
}

func (m Model) gitStripColumnSwitch(towardsToday bool) (target int, ok bool) {
	yesterdayCount, todayCount := len(m.yesterdayGitRepo), len(m.todayGitRepos)
	switch {
	case towardsToday && m.selected < yesterdayCount && todayCount > 0:
		return yesterdayCount + min(m.selected, todayCount-1), true
	case !towardsToday && m.selected >= yesterdayCount && m.selected < yesterdayCount+todayCount && yesterdayCount > 0:
		return min(m.selected-yesterdayCount, yesterdayCount-1), true
	}
	return 0, false
}

func (m *Model) updateScrollOffset() {
	m.scrollPending = true
}

type dashboardFrame struct {
	updateSeq    int
	width        int
	height       int
	selected     int
	mode         ViewMode
	header       string
	footer       string
	content      string
	selectedLine int
}

func (m Model) buildDashboardFrame() *dashboardFrame {
	frame := &dashboardFrame{updateSeq: m.updateSeq, width: m.width, height: m.height, selected: m.selected, mode: m.mode, header: m.renderHeader(), footer: m.renderFooter()}
	frame.content, frame.selectedLine = m.dashboardContent()
	return frame
}

func (m Model) currentDashboardFrame() *dashboardFrame {
	if frame := m.settledFrame; frame != nil && frame.updateSeq == m.updateSeq && frame.width == m.width && frame.height == m.height && frame.selected == m.selected && frame.mode == m.mode {
		return frame
	}
	return m.buildDashboardFrame()
}

func (frame *dashboardFrame) bodyHeight() int {
	return max(frame.height-lipgloss.Height(frame.header)-lipgloss.Height(frame.footer), 10)
}

func (m *Model) settleScroll() {
	m.scrollPending = false
	if m.selected == 0 {
		m.scrollOffset = 0
		return
	}
	frame := m.buildDashboardFrame()
	m.settledFrame = frame
	bodyHeight := frame.bodyHeight()
	selectedLineIdx := frame.selectedLine
	totalLines := strings.Count(frame.content, "\n") + 1

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
	} else if item.Note != nil {
		fullText := fmt.Sprintf("# %s", item.Note.Summary)
		if strings.TrimSpace(item.Note.Body) != "" {
			fullText += fmt.Sprintf("\n\n%s", item.Note.Body)
		}
		mdContent = renderMarkdown(fullText, innerWidth)
	}

	m.previewViewport = viewport.New(innerWidth, innerHeight)
	m.previewViewport.SetContent(mdContent)
	if item.Kind == KindJobDraft || item.Kind == KindReviewRun || item.Kind == KindBragRun {
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
	m.updateSeq++
	next, cmd := m.handleMsg(msg)
	updated, isModel := next.(Model)
	if !isModel {
		return next, cmd
	}
	if updated.scrollPending {
		updated.settleScroll()
	}
	return updated, cmd
}

func (m Model) handleMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case headerWaveTickMsg:
		if m.waveActive {
			m.waveFrame++
			if m.waveFrame <= 30 {
				return m, tickHeaderWaveCmd()
			}
		}
		m.waveActive = false
		m.headerWaveRunning = false
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
		wasBusy := m.isAnyDryRunInFlight() || m.isAnyJobRunning()
		m.refreshDryRunResults()
		stillBusy := m.isAnyDryRunInFlight() || m.isAnyJobRunning()
		if m.mode == ViewPreview && (wasBusy || stillBusy) && !m.jobLogRunning {
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
		if m.initialSelectionPending {
			m.initialSelectionPending = false
			m.selectLaunchItem()
		}
		var refetchPreviousDay tea.Cmd
		if !isSameDay(m.previousNoteDay(), m.fetchedPreviousDay) {
			m.ghReviewedYesterday, m.localCommitsYesterday = nil, nil
			m.rebuildGitRepoStats()
			refetchPreviousDay = m.startLoadGitStatsCmd(false)
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
		}
		return m, refetchPreviousDay

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

	case gitPendingMsg:
		var cmds []tea.Cmd
		if msg.generation == m.fetchGeneration && msg.err == nil {
			cmds = append(cmds, reopenApprovedNotesCmd(m.store, msg.pending, msg.startedAt))
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
		return m, m.startLoadGitStatsCmd(false)

	case daySyncDueMsg:
		if msg.generation != m.daySyncGeneration {
			return m, nil
		}
		return m, m.startLoadGitStatsCmd(true)

	case relatedHistoryMsg:
		return m.handleRelatedHistory(msg)

	case gitCacheSavedMsg:
		m.recordSectionError("Cache", msg.err)
		return m, nil

	case reviewPollTickMsg:
		return m.handleReviewPoll()

	case reviewSubmittedMsg:
		return m.handleReviewSubmitted(msg)

	case reviewCloneReadyMsg:
		return m.handleCloneReady(msg)

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

var (
	markdownRenderersMu sync.Mutex
	markdownRenderers   = map[int]*glamour.TermRenderer{}
)

func markdownStyle() glamouransi.StyleConfig {
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
		chroma := *style.CodeBlock.Chroma
		chroma.Background.BackgroundColor = nil
		style.CodeBlock.Chroma = &chroma
	}
	return style
}

func markdownRendererFor(width int) *glamour.TermRenderer {
	markdownRenderersMu.Lock()
	defer markdownRenderersMu.Unlock()
	if renderer, cached := markdownRenderers[width]; cached {
		return renderer
	}
	renderer, err := glamour.NewTermRenderer(glamour.WithStyles(markdownStyle()), glamour.WithWordWrap(width))
	if err != nil {
		return nil
	}
	markdownRenderers[width] = renderer
	return renderer
}

var markdownRenderMu sync.Mutex

func renderMarkdown(body string, width int) string {
	if strings.TrimSpace(body) == "" {
		return mutedStyle.Render("(No note body text)")
	}
	renderer := markdownRendererFor(width)
	if renderer == nil {
		return body
	}
	markdownRenderMu.Lock()
	out, err := renderer.Render(body)
	markdownRenderMu.Unlock()
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
	return m.renderSyncDot(m.loadingGit)
}

func (m Model) renderSyncDot(syncing bool) string {
	if !syncing {
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

var composeDashboard = func(m Model) string {
	frame := m.currentDashboardFrame()
	return lipgloss.JoinVertical(lipgloss.Left, frame.header, m.renderFrameBody(frame), frame.footer)
}

func (m Model) View() string {
	if m.width < 40 {
		return "Terminal window is too small."
	}
	if m.scrollPending {
		m.settleScroll()
	}

	modalWidth := modalWidthFor(m.width)
	innerWidth := modalWidth - 6

	switch m.mode {

	case ViewGitDetails:
		if m.gitPopupRepo == nil {
			return composeDashboard(m)
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
				if state := m.localReviews[review.StateDir(m.reviewRoot(), item.ReviewRun.Meta.Ref)]; state.pid > 0 {
					statusBadge = badgeActive.Render(fmt.Sprintf("RUNNING · PID %d", state.pid))
				}
				tagBadge = tagStyle.Render("#github")
			case KindBragRun:
				headerTitle = fmt.Sprintf(" BRAG JOB: %s ", item.BragRun.Meta.ID)
				statusBadge = stateStyle(review.StateFailed).Render("FAILED")
				if item.BragRun.Status == brag.RunRunning {
					statusBadge = badgeActive.Render("RUNNING")
				}
				tagBadge = tagStyle.Render("#brag")
			case KindJobDraft:
				headerTitle = fmt.Sprintf(" JOB: %s ", item.Draft.Name)
				if pid := m.runningJobPIDs[item.Draft.Name]; pid > 0 {
					statusBadge = badgeActive.Render(fmt.Sprintf("RUNNING · PID %d", pid))
				} else if item.Draft.DryRunInFlight {
					statusBadge = badgeActive.Render("DRY RUNNING")
				} else if m.previewJobLogFinished {
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

	case ViewHelp:
		return m.renderHelp(modalWidth)

	case ViewBragList:
		return m.renderBragList()

	case ViewBragView:
		return m.renderBragView()

	case ViewBragConfirm:
		return m.renderBragConfirm(modalWidth)

	case ViewBragEdit:
		return m.renderBragEdit()
	}

	dashboardView := composeDashboard(m)
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
		cmds := []tea.Cmd{m.relatedHistoryCmd(item.PendingGitPR)}
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

var (
	appVersion   = "dev"
	versionStyle = lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("#6C7086"))
)

const (
	bannerTopRow    = "█▀▄ █ █▀▀ █▀▀ █▀▀ ▀█▀"
	bannerMiddleRow = "█ █ █ █ ▄ █▀  ▀▀█  █ "
	bannerBottomRow = "█▄▀ █ █▄█ █▄▄ ▄▄█  █ "
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
	renderedDate := mutedStyle.Bold(true).Render("— " + time.Now().Format("Monday 02 Jan"))
	renderedVersion := versionStyle.Render("v" + appVersion)
	middleLine := fmt.Sprintf("%s  %s  %s", m.renderBannerLine(bannerMiddleRow), renderedVersion, renderedDate)
	if notice := m.unbraggedWeekNotice(); notice != "" {
		noticeRoom := m.width - lipgloss.Width(middleLine) - 7
		if noticeRoom > 0 {
			noticeText := yellowBadgeStyle.Render(ansi.Truncate(notice, noticeRoom, "…"))
			middleLine += safeRepeat(" ", m.width-4-lipgloss.Width(middleLine)-lipgloss.Width(noticeText)) + noticeText
		}
	}

	headerLines := []string{borderStyle.Render("┌" + safeRepeat("─", m.width-2) + "┐")}
	for _, content := range []string{m.renderBannerLine(bannerTopRow), middleLine, m.renderBannerLine(bannerBottomRow)} {
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
	var b strings.Builder
	innerWidth := m.width - 4
	rowWidth := innerWidth + 1

	// Calculate counts to determine globalIdx ranges
	yGitCount := len(m.yesterdayGitRepo)
	groups := m.groupNotes()
	yNotes := groups.previousDone
	yNotesCount := len(yNotes)
	todayNotes, olderNotes := groups.today, groups.carried
	carriedCount := len(olderNotes)
	addedCount := len(todayNotes)
	todayDoneNotes := groups.todayDone
	closedCount := len(todayDoneNotes)

	pendingGroups := m.getPendingGitGroups()
	m.tagSlots = m.computeTagSlots(slices.Concat(yNotes, olderNotes, todayNotes, todayDoneNotes), pendingGroups)
	pendingGitCount := 0
	for _, g := range pendingGroups {
		pendingGitCount += len(g.Items)
	}

	gitUpdatesCount := len(m.todayGitRepos)
	drafts := m.getJobDrafts()
	draftsCount := len(drafts) + len(m.reviewRuns) + len(m.bragRuns)

	// Calculate index boundaries
	yGitStart := 0
	yGitEnd := yGitStart + yGitCount

	gitUpdatesStart := yGitEnd
	gitUpdatesEnd := gitUpdatesStart + gitUpdatesCount

	yNotesStart := gitUpdatesEnd
	yNotesEnd := yNotesStart + yNotesCount

	carriedStart := yNotesEnd
	carriedEnd := carriedStart + carriedCount

	addedStart := carriedEnd
	addedEnd := addedStart + addedCount

	closedStart := addedEnd
	closedEnd := closedStart + closedCount

	pendingGitStart := closedEnd
	pendingGitEnd := pendingGitStart + pendingGitCount

	draftsStart := pendingGitEnd

	// Evaluate active header flags
	isGitStripActive := m.selected >= yGitStart && m.selected < gitUpdatesEnd
	isYesterdayActive := m.selected >= yNotesStart && m.selected < yNotesEnd

	isCarriedActive := m.selected >= carriedStart && m.selected < carriedEnd
	isAddedActive := m.selected >= addedStart && m.selected < addedEnd
	isClosedActive := m.selected >= closedStart && m.selected < closedEnd
	isPendingGitActive := m.selected >= pendingGitStart && m.selected < pendingGitEnd
	isDraftsActive := m.selected >= draftsStart && m.selected < draftsStart+draftsCount
	isTodayActive := m.selected >= carriedStart && m.selected < draftsStart+draftsCount

	globalIdx := 0
	markSelection := func() {
		if globalIdx == m.selected {
			selectedLine = strings.Count(b.String(), "\n")
		}
	}

	stripLines, stripSelectedRow := m.renderGitStrip(rowWidth, isGitStripActive)
	b.WriteString("\n")
	if stripSelectedRow >= 0 {
		selectedLine = strings.Count(b.String(), "\n") + stripSelectedRow
	}
	b.WriteString(strings.Join(stripLines, "\n") + "\n")
	globalIdx = gitUpdatesEnd

	// --- YESTERDAY SECTION ---
	yTitleText := m.previousDayTitleFor(groups.previousDay)
	b.WriteString(sectionGap + " " + m.renderWaveTitle(yTitleText, isYesterdayActive) + "\n")
	if len(yNotes) > 0 {
		b.WriteString("\n")
	}
	for _, n := range yNotes {
		markSelection()
		isSel := (globalIdx == m.selected)
		b.WriteString(m.renderRow(n, isSel, rowWidth))
		globalIdx++
	}

	// --- TODAY SECTION ---
	b.WriteString(sectionGap)
	jobBadge := fmt.Sprintf("%d jobs %s", len(drafts), amberDiamond.Render())

	todayTitleText := m.dayTitleText(m.currentDate, "T O D A Y")

	renderedTodayTitle := m.renderWaveTitle(todayTitleText, isTodayActive)
	todayGap := innerWidth - lipgloss.Width(renderedTodayTitle) - lipgloss.Width(jobBadge)
	b.WriteString(fmt.Sprintf(" %s%s%s\n\n", renderedTodayTitle, safeRepeat(" ", todayGap), jobBadge))

	// 1. TODAY: Pending, Carried Over
	b.WriteString("  " + m.renderSubSection("Pending, Carried Over", carriedCount, true, isCarriedActive) + "\n")
	if len(olderNotes) == 0 {
		b.WriteString(mutedStyle.Render("   (no carried over notes)\n"))
	} else {
		for _, n := range olderNotes {
			markSelection()
			isSel := (globalIdx == m.selected)
			b.WriteString(m.renderRow(n, isSel, rowWidth))
			globalIdx++
		}
	}

	// 2. TODAY: Added Today
	b.WriteString(sectionGap + "  " + m.renderSubSection("Added Today", addedCount, true, isAddedActive) + "\n")
	if len(todayNotes) == 0 {
		b.WriteString(mutedStyle.Render("   (no notes added today)\n"))
	} else {
		for _, n := range todayNotes {
			markSelection()
			isSel := (globalIdx == m.selected)
			b.WriteString(m.renderRow(n, isSel, rowWidth))
			globalIdx++
		}
	}

	// 3. TODAY: Closed Today
	b.WriteString(sectionGap + "  " + m.renderSubSection("Closed Today", closedCount, true, isClosedActive) + "\n")
	if len(todayDoneNotes) == 0 {
		b.WriteString(mutedStyle.Render("   (no notes closed today)\n"))
	} else {
		for _, n := range todayDoneNotes {
			markSelection()
			isSel := (globalIdx == m.selected)
			b.WriteString(m.renderRow(n, isSel, rowWidth))
			globalIdx++
		}
	}

	// 4. TODAY: Pending Git Actions
	pendingGitHeader := m.renderSubSection("Pending Git Actions", 0, false, isPendingGitActive)
	pendingStatus := m.renderLiveSyncDot()
	b.WriteString(fmt.Sprintf("%s  %s  %s  %s\n", sectionGap, pendingGitHeader, m.renderPendingSortHint(), pendingStatus))

	if len(pendingGroups) == 0 {
		if m.loadingGit {
			b.WriteString(mutedStyle.Render("   (checking pending PR reviews...)\n"))
		} else if m.pendingMeOnly {
			b.WriteString(mutedStyle.Render("   (no PRs asking you by name)\n"))
		} else {
			b.WriteString(mutedStyle.Render("   (no PRs requiring review)\n"))
		}
	} else {
		for groupIndex, g := range pendingGroups {
			if groupIndex > 0 {
				b.WriteString("\n")
			}
			b.WriteString("    " + dimBlueText.Bold(true).Render(g.Name) + "\n")
			for i := range g.Items {
				markSelection()
				isSel := (globalIdx == m.selected)
				b.WriteString(m.renderPendingGitRow(&g.Items[i], isSel, rowWidth))
				globalIdx++
			}
		}
	}

	// 5. TODAY: Jobs
	b.WriteString(sectionGap + "  " + m.renderSubSection("Jobs", 0, false, isDraftsActive) + "\n")
	if draftsCount == 0 {
		b.WriteString(mutedStyle.Render("   (no jobs configured)\n"))
	} else {
		for _, d := range drafts {
			markSelection()
			isSel := (globalIdx == m.selected)
			b.WriteString(m.renderDraftRow(d, isSel, innerWidth))
			globalIdx++
		}
		for i := range m.reviewRuns {
			markSelection()
			isSel := (globalIdx == m.selected)
			b.WriteString(m.renderReviewRunRow(m.reviewRuns[i], isSel, innerWidth))
			globalIdx++
		}
		for i := range m.bragRuns {
			markSelection()
			isSel := (globalIdx == m.selected)
			b.WriteString(m.renderBragRunRow(m.bragRuns[i], isSel, innerWidth))
			globalIdx++
		}
	}

	return b.String(), selectedLine
}

func (m Model) renderDashboardBody() string {
	if m.scrollPending {
		m.settleScroll()
	}
	return m.renderFrameBody(m.currentDashboardFrame())
}

func (m Model) renderFrameBody(frame *dashboardFrame) string {
	bodyHeight := frame.bodyHeight()
	lines := strings.Split(frame.content, "\n")

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

	nameStyle, statsStyle := subSectionStyle, mutedStyle
	if selected {
		nameStyle, statsStyle = selectedSummaryStyle, selectedTagStyle
	}
	return alignRight("     "+nameStyle.Render(repo.Name), []string{statsStyle.Render(stats)}, width) + "\n"
}

const tagGap = "  "

type rowTagSlots struct {
	noteAge, noteSource, prSize, prAge, prState int
}

func slotted(cell string, slotWidth int) string {
	return safeRepeat(" ", slotWidth-lipgloss.Width(cell)) + cell
}

func alignRight(left string, tags []string, width int) string {
	tagBlock := joinTags(tags)
	room := width - lipgloss.Width(tagBlock) - 1
	if lipgloss.Width(left) > room {
		left = ansi.Truncate(left, max(room, 0), "…")
	}
	return left + safeRepeat(" ", width-lipgloss.Width(left)-lipgloss.Width(tagBlock)) + tagBlock
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

func (m Model) prTagCells(item *GitPRItem, selected bool) (size, age, state string) {
	if item.PR == nil {
		kindTag := fmt.Sprintf("[%s]", item.Kind)
		if selected {
			return "", "", selectedTagStyle.Render(kindTag)
		}
		return "", "", mutedStyle.Render(kindTag)
	}
	now := time.Now()
	prState := m.prState(item)
	state = stateStyle(prState).Render(string(prState))
	if selected {
		state = selectedTagStyle.Render(string(prState))
	}
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
	reReviewIcon     = "\U000F02DA"
	teamReviewIcon   = "\U000F0849"
	directReviewIcon = "\U000F0004"
)

func reviewRequestIcon(item *GitPRItem) string {
	var icons []string
	if item.Kind == sourcecontrol.ReReviewKind {
		icons = append(icons, reReviewIcon)
	}
	if item.PR.DirectRequest {
		icons = append(icons, directReviewIcon)
	}
	if item.PR.CodeOwner {
		icons = append(icons, teamReviewIcon)
	}
	if len(icons) == 0 {
		return ""
	}
	return dimBlueText.Render(strings.Join(icons, " "))
}

func (m Model) prTags(item *GitPRItem, selected bool) []string {
	size, age, state := m.prTagCells(item, selected)
	if item.PR == nil {
		return []string{slotted(state, m.tagSlots.prState)}
	}
	return []string{reviewRequestIcon(item), slotted(size, m.tagSlots.prSize), slotted(age, m.tagSlots.prAge), slotted(state, m.tagSlots.prState)}
}

func (m Model) computeTagSlots(notes []*model.Note, pendingGroups []PendingRepoGroup) rowTagSlots {
	var slots rowTagSlots
	for _, n := range notes {
		age, source := m.noteTagCells(n, false)
		slots.noteAge = max(slots.noteAge, lipgloss.Width(age))
		slots.noteSource = max(slots.noteSource, lipgloss.Width(source))
	}
	for _, group := range pendingGroups {
		for i := range group.Items {
			size, age, state := m.prTagCells(&group.Items[i], false)
			slots.prSize = max(slots.prSize, lipgloss.Width(size))
			slots.prAge = max(slots.prAge, lipgloss.Width(age))
			slots.prState = max(slots.prState, lipgloss.Width(state))
		}
	}
	return slots
}

func (m Model) renderPendingGitRow(item *GitPRItem, selected bool, width int) string {
	titleStyle := itemStyle
	if selected {
		titleStyle = selectedSummaryStyle
	}
	leftBlock := fmt.Sprintf("      %s %s", pendingPRIcon.Render(), titleStyle.Render(item.Title))
	return alignRight(leftBlock, m.prTags(item, selected), width) + "\n"
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
	if m.jobRunning(draft.Name) {
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

func (m Model) noteTagCells(n *model.Note, selected bool) (age, source string) {
	sourceText := string(n.Source)
	if sourceText == "" {
		sourceText = "manual"
	}
	if !strings.HasPrefix(sourceText, "#") {
		sourceText = "#" + sourceText
	}
	source = dimBlueText.Render(sourceText)
	if selected {
		source = selectedTagStyle.Render(sourceText)
	}

	ageText := "08:40"
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
	switch {
	case carriedOver:
		age = yellowBadgeStyle.Render(ageText)
	case selected:
		age = mutedStyle.Bold(true).Render(ageText)
	default:
		age = mutedStyle.Render(ageText)
	}
	return age, source
}

func (m Model) renderRow(n *model.Note, selected bool, width int) string {
	age, source := m.noteTagCells(n, selected)
	tags := []string{slotted(age, m.tagSlots.noteAge), slotted(source, m.tagSlots.noteSource)}
	prefix := "   "

	boxChar := "☐"
	if n.Status == model.StatusDone {
		boxChar = "✔"
	}
	isTodayDone := n.Status == model.StatusDone && isSameDay(n.Updated, m.currentDate)

	var leftBlock string
	switch {
	case selected && m.mode == ViewInlineEdit:
		m.inlineInput.Width = max(width-lipgloss.Width(strings.Join(tags, tagGap))-len(prefix)-4, 5)
		leftBlock = fmt.Sprintf("%s%s %s", prefix, checkPending.Render(), m.inlineInput.View())
	case selected:
		boxStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#CDD6F4"))
		if n.Status == model.StatusDone {
			boxStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A6E3A1"))
		}
		summaryStyle := selectedSummaryStyle.Copy()
		if isTodayDone {
			summaryStyle = summaryStyle.Strikethrough(true)
		}
		leftBlock = fmt.Sprintf("%s%s %s", prefix, boxStyle.Render(boxChar), summaryStyle.Render(n.Summary))
	default:
		box := checkPending.Render()
		if n.Status == model.StatusDone {
			box = checkDone.Render()
		}
		summary := n.Summary
		if isTodayDone {
			summary = itemStyle.Copy().Strikethrough(true).Render(summary)
		}
		leftBlock = fmt.Sprintf("%s%s %s", prefix, box, summary)
	}
	return alignRight(leftBlock, tags, width) + "\n"
}

var archiveFooterItems = footerItemsFrom(archivedBindings())

func (m Model) dashboardBodyHeight() int {
	return m.height - lipgloss.Height(m.renderHeader()) - lipgloss.Height(m.renderFooter())
}

func (m Model) footerLines() []string {
	var lines []string
	if m.ctrlCCount > 0 {
		var warnings []footerItem
		for _, binding := range m.dashboardBindings() {
			if binding.action == actionQuit {
				warnings = footerItemsFrom([]keyBinding{binding})
			}
		}
		keysLine, firstActionLine, secondActionLine := renderFooterLines(warnings)
		lines = append(lines, keysLine, firstActionLine)
		if footerLineCount(warnings) == 3 {
			lines = append(lines, secondActionLine)
		}
	}
	return lines
}

func (m Model) renderFooter() string {
	lines := m.footerLines()
	bottomBorder := borderStyle.Render("└" + safeRepeat("─", m.width-2) + "┘")
	if len(lines) == 0 {
		return bottomBorder
	}
	topBorder := borderStyle.Render("├" + safeRepeat("─", m.width-2) + "┤")

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
	links := noteLinkPattern.FindAllString(note.Body, -1)
	for i := len(links) - 1; i >= 0; i-- {
		if pullRequestPathPattern.MatchString(links[i]) {
			return links[i]
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
}

func (m *Model) changePendingSort(toggle func(*sourcecontrol.Sort)) tea.Cmd {
	toggle(&m.pendingSort)
	m.applyPendingSort()
	save := pendingSortSaveCmd(m.pendingSort)
	if !m.loadingGit {
		return save
	}
	return tea.Batch(save, m.startLoadGitStatsCmd(false))
}

func (m Model) renderPendingSortHint() string {
	field, direction := "Updated", "↓ Desc"
	if m.pendingSort.ByCreated {
		field = "Created"
	}
	if m.pendingSort.Ascending {
		direction = "↑ Asc"
	}
	scope := directReviewIcon + " " + teamReviewIcon
	if m.pendingMeOnly {
		scope = directReviewIcon
	}
	return fmt.Sprintf("%s %s  %s %s  %s %s", keyStyle.Render("s"), mutedStyle.Render(field), keyStyle.Render("w"), mutedStyle.Render(direction), keyStyle.Render("m"), dimBlueText.Render(scope))
}
