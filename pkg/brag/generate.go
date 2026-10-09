package brag

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/system"
)

var commandWaitDelay = 2 * time.Second

var bragCommandTimeout = 5 * time.Minute

var runBragCommand = func(ctx context.Context, command, input string) (string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.WaitDelay = commandWaitDelay
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.Stdin = strings.NewReader(input)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return "", fmt.Errorf("brag command: %s", message)
		}
		return "", fmt.Errorf("brag command: %w", err)
	}
	return string(out), nil
}

func GenerateSummary(ctx context.Context, command, prompt string, period Period, facts string) (string, error) {
	commandCtx, cancel := context.WithTimeout(ctx, bragCommandTimeout)
	defer cancel()
	output, err := runBragCommand(commandCtx, command, prompt+"\n\n"+factsHeading+"\n\n"+facts+"\n")
	if errors.Is(commandCtx.Err(), context.DeadlineExceeded) {
		return "", fmt.Errorf("brag command timed out after %s", bragCommandTimeout)
	}
	if err != nil {
		return "", err
	}
	heading := summaryHeading(period)
	if summaryStart := headingIndex(output, heading); summaryStart >= 0 {
		output = output[summaryStart+len(heading):]
	}
	summary := strings.TrimSpace(output)
	if summary == "" {
		return "", errors.New("brag command returned no summary")
	}
	return summary, nil
}

func Create(ctx context.Context, root, command, prompt string, period Period, facts string) (*Brag, error) {
	now := time.Now()
	entry := &Brag{Period: period, Created: now, Updated: now, Facts: facts}
	summary, summaryErr := GenerateSummary(ctx, command, prompt, period, facts)
	if summaryErr == nil {
		entry.Summary, entry.SummarizedAt = summary, now
	}
	if err := saveReportingLock(entry, root); err != nil {
		return nil, err
	}
	return entry, summaryErr
}

func saveReportingLock(entry *Brag, root string) error {
	err := entry.Save(root)
	if errors.Is(err, system.ErrLock) {
		fmt.Fprintf(os.Stderr, "%s: %v\n", entry.Period.Path(root), err)
		return nil
	}
	return err
}

func Regenerate(ctx context.Context, root, command, prompt string, period Period) (*Brag, error) {
	entry, err := Load(root, period)
	if err != nil {
		return nil, err
	}
	summary, err := GenerateSummary(ctx, command, prompt, period, entry.Facts)
	if err != nil {
		return entry, err
	}
	now := time.Now()
	entry.Summary, entry.Updated, entry.SummarizedAt = summary, now, now
	return entry, saveReportingLock(entry, root)
}
