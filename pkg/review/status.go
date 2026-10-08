package review

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/AnudeepChPaul/digest/pkg/notify"
	"github.com/AnudeepChPaul/digest/pkg/paths"
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
	return notify.Send(notify.Notification{Title: title, Message: message, OpenURL: openURL})
}

func NotifyTransitions(prs []QueuedPR, root, statePath string) error {
	seen := map[string]bool{}
	data, err := os.ReadFile(statePath)
	firstRun := errors.Is(err, fs.ErrNotExist)
	if err == nil {
		if err := json.Unmarshal(data, &seen); err != nil {
			seen, firstRun = map[string]bool{}, true
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

	if err := os.MkdirAll(filepath.Dir(statePath), paths.PrivateDirMode); err != nil {
		return err
	}
	encoded, err := json.Marshal(seen)
	if err != nil {
		return err
	}
	if err := writeFileAtomically(statePath, encoded); err != nil {
		return err
	}
	return notifyErr
}

func writeFileAtomically(path string, data []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	_, writeErr := temporary.Write(data)
	closeErr := temporary.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		_ = os.Remove(temporary.Name())
		return err
	}
	if err := os.Rename(temporary.Name(), path); err != nil {
		_ = os.Remove(temporary.Name())
		return err
	}
	return nil
}
