package notify

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

var lookPath = exec.LookPath

func shellQuote(text string) string {
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}

func clickTarget(entry Entry, notification Notification) Notification {
	switch {
	case entry.OpenURL != "":
		notification.OpenURL = entry.OpenURL
	case entry.Terminal != "":
		notification.Execute = "open -b " + shellQuote(entry.Terminal)
	}
	return notification
}

func RunDue(root string, now time.Time) error {
	entries, err := List(root)
	if err != nil {
		return err
	}
	var errs []error
	for _, entry := range entries {
		interval, err := parseStoredInterval(entry.Interval)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", entry.NoteID, err))
			continue
		}
		if now.Sub(entry.NotifiedAt) < interval {
			continue
		}
		if err := Send(clickTarget(entry, Notification{Title: "Reminder · every " + IntervalLabel(interval), Message: entry.Summary, Group: entry.NoteID})); err != nil {
			errs = append(errs, fmt.Errorf("notify %s: %w", entry.NoteID, err))
			continue
		}
		if _, stillActive := Load(root, entry.NoteID); !stillActive {
			continue
		}
		entry.NotifiedAt = now
		if err := Save(root, entry); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
