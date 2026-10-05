package brag

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

var commandWaitDelay = 2 * time.Second

var runBragCommand = func(ctx context.Context, command, input string) (string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.WaitDelay = commandWaitDelay
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
	output, err := runBragCommand(ctx, command, prompt+"\n\n"+factsHeading+"\n\n"+facts+"\n")
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
	if err := entry.Save(root); err != nil {
		return nil, err
	}
	return entry, summaryErr
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
	return entry, entry.Save(root)
}
