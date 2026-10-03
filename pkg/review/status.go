package review

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
)

type PRState string

const (
	StatePending          PRState = "Pending"
	StateApproved         PRState = "Approved"
	StateReviewing        PRState = "Reviewing"
	StateReviewed         PRState = "Reviewed"
	StateFailed           PRState = "Failed"
	StateChangesRequested PRState = "Changes requested"
	StateCommented        PRState = "Commented"
)

func StateOf(pr QueuedPR, root string) PRState {
	status := Status(StateDir(root, pr.Ref))
	switch {
	case status == RunRunning:
		return StateReviewing
	case pr.Approved:
		return StateApproved
	case status == RunDone:
		return StateReviewed
	case status == RunFailed:
		return StateFailed
	default:
		return StatePending
	}
}

var sendNotification = func(title, message, openURL string) error {
	args := []string{"-title", title, "-message", message, "-group", openURL}
	if openURL != "" {
		args = append(args, "-open", openURL)
	}
	return exec.Command("terminal-notifier", args...).Run()
}

func Notify(title, message, openURL string) error {
	return sendNotification(title, message, openURL)
}

func NotifyTransitions(prs []QueuedPR, root, statePath string) error {
	seen := map[string]bool{}
	data, err := os.ReadFile(statePath)
	firstRun := errors.Is(err, fs.ErrNotExist)
	if err == nil {
		if err := json.Unmarshal(data, &seen); err != nil {
			seen = map[string]bool{}
		}
	} else if !firstRun {
		return err
	}

	var notifyErr error
	for _, pr := range prs {
		if StateOf(pr, root) != StateApproved || seen[pr.Ref.URL] {
			continue
		}
		seen[pr.Ref.URL] = true
		if firstRun {
			continue
		}
		message := fmt.Sprintf("%s #%d approved: %s", pr.Ref.Repo, pr.Ref.Number, pr.Title)
		if err := sendNotification("PR approved", message, pr.Ref.URL); err != nil && notifyErr == nil {
			notifyErr = err
		}
	}

	if err := os.MkdirAll(filepath.Dir(statePath), 0755); err != nil {
		return err
	}
	encoded, err := json.Marshal(seen)
	if err != nil {
		return err
	}
	if err := os.WriteFile(statePath, encoded, 0644); err != nil {
		return err
	}
	return notifyErr
}
