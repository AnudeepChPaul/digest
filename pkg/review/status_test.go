package review

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestStateOf(t *testing.T) {
	root := t.TempDir()
	ref := PRRef{Repo: "console", Number: 1, URL: "u1"}
	pr := QueuedPR{Ref: ref}
	if StateOf(pr, root) != StatePending {
		t.Errorf("want pending")
	}
	state := StateDir(root, ref)
	writeFile(t, filepath.Join(state, exitFile), "0")
	writeFile(t, filepath.Join(state, FindingsFile), `{"findings":[]}`)
	if StateOf(pr, root) != StateReviewed {
		t.Errorf("want reviewed")
	}
	pr.Approved = true
	if StateOf(pr, root) != StateApproved {
		t.Errorf("want approved")
	}
	writeFile(t, filepath.Join(state, pidFile), strconv.Itoa(os.Getpid()))
	if StateOf(pr, root) != StateReviewing {
		t.Errorf("want reviewing")
	}
}

func TestNotifyApprovedTransitions(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "seen.json")
	var sent []string
	original := sendNotification
	sendNotification = func(title, message, openURL string) error {
		sent = append(sent, openURL)
		return nil
	}
	defer func() { sendNotification = original }()

	first := []QueuedPR{{Ref: PRRef{Repo: "a", Number: 1, URL: "u1"}, Approved: true}, {Ref: PRRef{Repo: "b", Number: 2, URL: "u2"}}}
	if err := NotifyTransitions(first, root, statePath); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 0 {
		t.Errorf("first run should seed silently, sent %v", sent)
	}
	first[1].Approved = true
	if err := NotifyTransitions(first, root, statePath); err != nil {
		t.Fatal(err)
	}
	if err := NotifyTransitions(first, root, statePath); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 1 || sent[0] != "u2" {
		t.Errorf("sent = %v, want [u2]", sent)
	}
}

func TestStateOfFailed(t *testing.T) {
	root := t.TempDir()
	ref := PRRef{Repo: "console", Number: 2, URL: "u2"}
	pr := QueuedPR{Ref: ref}
	writeFile(t, filepath.Join(StateDir(root, ref), exitFile), "1")
	if StateOf(pr, root) != StateFailed {
		t.Errorf("want failed, got %s", StateOf(pr, root))
	}
	pr.Approved = true
	if StateOf(pr, root) != StateApproved {
		t.Errorf("approved should win over failed")
	}
}
