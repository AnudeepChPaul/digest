package tui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"app/pkg/notify"
	"app/pkg/paths"
	"app/pkg/review"
	"app/pkg/sourcecontrol"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	prKindMine    = "mine"
	prKindRequest = "request"

	prStateApproved         = "approved"
	prStateChangesRequested = "changes requested"
	prStateMerged           = "merged"
	prStateClosed           = "closed"
	prStateRequested        = "requested"
	prStateCommentedPrefix  = "commented "
	prStateReReviewPrefix   = "re-review "
)

type prStatus struct {
	Kind   string `json:"kind"`
	State  string `json:"state"`
	Title  string `json:"title"`
	Author string `json:"author,omitempty"`
}

type prAlertsMsg struct {
	err error
}

func prStatusPath(root string) string {
	return filepath.Join(root, "cache", "pr-status.json")
}

func myPRState(pr review.QueuedPR) string {
	switch pr.ReviewDecision {
	case "APPROVED":
		return prStateApproved
	case "CHANGES_REQUESTED":
		return prStateChangesRequested
	}
	var latestComment time.Time
	for _, prReview := range pr.Reviews {
		if prReview.State == "COMMENTED" && prReview.SubmittedAt.After(latestComment) {
			latestComment = prReview.SubmittedAt
		}
	}
	if !latestComment.IsZero() {
		return prStateCommentedPrefix + latestComment.UTC().Format(time.RFC3339)
	}
	return "open"
}

func prSnapshot(myPRs []review.QueuedPR, closed map[string]string, pending []GitPRItem) map[string]prStatus {
	snapshot := map[string]prStatus{}
	for _, pr := range myPRs {
		snapshot[pr.Ref.URL] = prStatus{Kind: prKindMine, State: myPRState(pr), Title: pr.Title}
	}
	for url, state := range closed {
		status := snapshot[url]
		status.Kind, status.State = prKindMine, prStateClosed
		if strings.EqualFold(state, "MERGED") {
			status.State = prStateMerged
		}
		snapshot[url] = status
	}
	for _, item := range pending {
		status := prStatus{Kind: prKindRequest, State: prStateRequested, Title: item.Title}
		if item.PR != nil {
			status.Title, status.Author = item.PR.Title, item.PR.Author
			if item.Kind == sourcecontrol.ReReviewKind {
				status.State = prStateReReviewPrefix + item.PR.RequestedAt.UTC().Format(time.RFC3339)
			}
		}
		snapshot[item.URL] = status
	}
	return snapshot
}

var prAlertLines = map[string][]string{
	prStateApproved:         {"🎉 %s got the green light!", "✅ Approved! %s is ready to ship.", "🥳 Someone loves %s. Approved!"},
	prStateChangesRequested: {"🛠️ Changes requested on %s. Back to the workbench!", "🔧 %s needs a few tweaks.", "📝 Reviewers left homework on %s."},
	prStateMerged:           {"🚀 %s merged. Ship it!", "🎊 %s is in! Merged.", "🏁 %s crossed the finish line."},
	prStateClosed:           {"📪 %s was closed.", "🧹 %s got closed.", "👋 %s is closed."},
	prStateCommentedPrefix:  {"💬 New comments on %s.", "🗨️ %s has fresh feedback.", "👂 People are talking about %s."},
	prStateRequested:        {"👀 %s wants your eyes.", "🔍 Fresh review: %s.", "☕ Grab a coffee, %s needs a review."},
	prStateReReviewPrefix:   {"🔁 %s is back for another look.", "♻️ Round two for %s.", "👀 %s wants a re-review."},
}

var prAlertTitles = map[string]string{
	prStateApproved:         "PR approved",
	prStateChangesRequested: "Changes requested",
	prStateMerged:           "PR merged",
	prStateClosed:           "PR closed",
	prStateCommentedPrefix:  "New comments",
	prStateRequested:        "Review requested",
	prStateReReviewPrefix:   "Re-review requested",
}

func alertKey(state string) string {
	for _, prefix := range []string{prStateCommentedPrefix, prStateReReviewPrefix} {
		if strings.HasPrefix(state, prefix) {
			return prefix
		}
	}
	return state
}

func prAlert(url string, status prStatus) (notify.Notification, bool) {
	key := alertKey(status.State)
	lines, alertable := prAlertLines[key]
	if !alertable {
		return notify.Notification{}, false
	}
	line := fmt.Sprintf(lines[len(status.Title)%len(lines)], status.Title)
	if status.Author != "" && key == prStateRequested {
		line += " (from " + status.Author + ")"
	}
	return notify.Notification{Title: prAlertTitles[key], Message: line, OpenURL: url, Group: "digest-pr-" + url}, true
}

func changedPRAlerts(previous, current map[string]prStatus) []notify.Notification {
	var urls []string
	for url := range current {
		urls = append(urls, url)
	}
	slices.Sort(urls)
	var alerts []notify.Notification
	for _, url := range urls {
		status := current[url]
		before, existed := previous[url]
		if existed && before.State == status.State {
			continue
		}
		if !existed && status.Kind == prKindMine {
			continue
		}
		if status.Title == "" {
			status.Title = before.Title
		}
		if alert, alertable := prAlert(url, status); alertable {
			alerts = append(alerts, alert)
		}
	}
	return alerts
}

func sendPRAlerts(path string, current map[string]prStatus) error {
	data, err := os.ReadFile(path)
	firstSync := errors.Is(err, os.ErrNotExist)
	if err != nil && !firstSync {
		return err
	}
	previous := map[string]prStatus{}
	if !firstSync {
		if err := json.Unmarshal(data, &previous); err != nil {
			return err
		}
	}
	var sendErrs []error
	if !firstSync {
		for _, alert := range changedPRAlerts(previous, current) {
			sendErrs = append(sendErrs, notify.Send(alert))
		}
	}
	encoded, err := json.MarshalIndent(current, "", "  ")
	if err == nil {
		err = os.MkdirAll(filepath.Dir(path), paths.PrivateDirMode)
	}
	if err == nil {
		err = os.WriteFile(path, encoded, paths.PrivateFileMode)
	}
	return errors.Join(append(sendErrs, err)...)
}

func (m Model) prAlertsCmd() tea.Cmd {
	snapshot := prSnapshot(m.myPRs, m.closedMyPRs, m.ghPendingPRs)
	path := prStatusPath(m.cfg.Root())
	return func() tea.Msg {
		return prAlertsMsg{err: sendPRAlerts(path, snapshot)}
	}
}
