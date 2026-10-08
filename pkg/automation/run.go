package automation

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/AnudeepChPaul/digest/pkg/paths"
)

const (
	runPIDFile  = "automation.pid"
	runExitFile = "automation.exit"
	runLogFile  = "automation.log"
	runMetaFile = "meta.json"
	draftFile   = "draft.yaml"
)

const ExitNeedsReauth = 3

type RunStatus int

const (
	RunIdle RunStatus = iota
	RunRunning
	RunFailed
	RunNeedsReauth
	RunDraftReady
	RunCreated
)

type RunMeta struct {
	NoteID     string `json:"note_id"`
	Automation string `json:"automation"`
	Phase      Phase  `json:"phase"`
}

type Run struct {
	Meta     RunMeta
	Status   RunStatus
	HasDraft bool
}

var ErrRunning = errors.New("this note's automation is already running")

var executablePath = os.Executable

func StateDir(root, noteID string) string {
	return filepath.Join(root, noteID)
}

func LogPath(root, noteID string) string {
	return filepath.Join(StateDir(root, noteID), runLogFile)
}

func readInt(path string) (int, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	value, err := strconv.Atoi(strings.TrimSpace(string(data)))
	return value, err == nil
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	return err == nil && process.Signal(syscall.Signal(0)) == nil
}

func running(root, noteID string) bool {
	pid, found := readInt(filepath.Join(StateDir(root, noteID), runPIDFile))
	return found && processAlive(pid)
}

func writeMeta(dir string, meta RunMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, runMetaFile), data, paths.PrivateFileMode)
}

func Status(root, noteID string) Run {
	dir := StateDir(root, noteID)
	run := Run{Meta: RunMeta{NoteID: noteID}}
	if data, err := os.ReadFile(filepath.Join(dir, runMetaFile)); err == nil {
		_ = json.Unmarshal(data, &run.Meta)
	}
	if _, err := os.Stat(filepath.Join(dir, draftFile)); err == nil {
		run.HasDraft = true
	}
	if running(root, noteID) {
		run.Status = RunRunning
		return run
	}
	exitCode, found := readInt(filepath.Join(dir, runExitFile))
	switch {
	case !found:
		run.Status = RunIdle
		if run.HasDraft {
			run.Status = RunDraftReady
		}
		if _, err := os.Stat(filepath.Join(dir, runPIDFile)); err == nil {
			run.Status = RunFailed
		}
	case exitCode == ExitNeedsReauth:
		run.Status = RunNeedsReauth
	case exitCode != 0:
		run.Status = RunFailed
	case run.Meta.Phase == PhaseCreate:
		run.Status = RunCreated
	default:
		run.Status = RunDraftReady
	}
	return run
}

func ListRuns(root string) map[string]Run {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	runs := map[string]Run{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if run := Status(root, entry.Name()); run.Status != RunIdle {
			runs[entry.Name()] = run
		}
	}
	return runs
}

func StartBackground(root, noteID, automationName string, phase Phase) error {
	if running(root, noteID) {
		return ErrRunning
	}
	dir := StateDir(root, noteID)
	if err := os.MkdirAll(dir, paths.PrivateDirMode); err != nil {
		return err
	}
	stale := []string{runExitFile, runPIDFile}
	if phase == PhaseDraft {
		stale = append(stale, draftFile)
	}
	for _, staleFile := range stale {
		if err := os.Remove(filepath.Join(dir, staleFile)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := writeMeta(dir, RunMeta{NoteID: noteID, Automation: automationName, Phase: phase}); err != nil {
		return err
	}
	executable, err := executablePath()
	if err != nil {
		return fmt.Errorf("locate digest binary: %w", err)
	}
	logOutput, err := paths.CreatePrivate(LogPath(root, noteID))
	if err != nil {
		return err
	}
	pidPath := filepath.Join(dir, runPIDFile)
	script := `"$3" automation --note "$4" --name "$5" --phase "$6"; echo $? > "$1"; rm -f "$2"`
	cmd := exec.Command("sh", "-c", script, "digest-automation", filepath.Join(dir, runExitFile), pidPath, executable, noteID, automationName, string(phase))
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Dir = dir
	cmd.Stdout = logOutput
	cmd.Stderr = logOutput
	if err := cmd.Start(); err != nil {
		logOutput.Close()
		return err
	}
	pidErr := os.WriteFile(pidPath, []byte(strconv.Itoa(cmd.Process.Pid)), paths.PrivateFileMode)
	go func() {
		_ = cmd.Wait()
		_ = logOutput.Close()
	}()
	if pidErr != nil {
		return fmt.Errorf("automation started but its pid file could not be written: %w", pidErr)
	}
	return nil
}

func Stop(root, noteID string) error {
	dir := StateDir(root, noteID)
	pidPath := filepath.Join(dir, runPIDFile)
	pid, found := readInt(pidPath)
	if !found || !processAlive(pid) {
		return nil
	}
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("stop automation for %s: %w", noteID, err)
	}
	if err := os.Remove(pidPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(filepath.Join(dir, runExitFile), []byte("143"), paths.PrivateFileMode)
}

var ErrInvalidNoteID = errors.New("note id does not name a folder inside the automations root")

func Dismiss(root, noteID string) error {
	if noteID == "" || filepath.Dir(StateDir(root, noteID)) != filepath.Clean(root) || filepath.Base(StateDir(root, noteID)) != noteID {
		return ErrInvalidNoteID
	}
	if running(root, noteID) {
		return ErrRunning
	}
	return os.RemoveAll(StateDir(root, noteID))
}

func AnyRunning(root string) bool {
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() && running(root, entry.Name()) {
			return true
		}
	}
	return false
}
