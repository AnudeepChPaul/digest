package tui

import (
	"fmt"
	"testing"

	"github.com/achandrapaul/digest/pkg/sourcecontrol"
)

func pendingOnHost(host string, number int) GitPRItem {
	return GitPRItem{Title: "PR", URL: fmt.Sprintf("https://%s/team/service/pull/%d", host, number), Kind: sourcecontrol.PendingReviewKind, Number: number, Repository: "service"}
}

func TestKeepFailedHostItems(t *testing.T) {
	previous := []GitPRItem{pendingOnHost("git.example.com", 1), pendingOnHost("other.example.com", 2), pendingOnHost("other.example.com", 3)}
	fresh := []GitPRItem{pendingOnHost("git.example.com", 4), pendingOnHost("other.example.com", 3)}

	merged := keepFailedHostItems(previous, fresh, []string{"other.example.com"})
	assertNumbers(t, "merged", merged, []int{4, 3, 2})

	unchanged := keepFailedHostItems(previous, fresh, nil)
	assertNumbers(t, "no failures", unchanged, []int{4, 3})
}

func TestPendingSyncKeepsFailedHostPRs(t *testing.T) {
	m := syncTestModel(t)
	m.git.ghPendingPRs = []GitPRItem{pendingOnHost("git.example.com", 1), pendingOnHost("other.example.com", 2)}
	m.applyGitPending(gitPendingMsg{
		generation:  m.git.fetchGeneration,
		pending:     []GitPRItem{pendingOnHost("git.example.com", 5)},
		failedHosts: []string{"other.example.com"},
		err:         fmt.Errorf("other.example.com: TLS handshake timeout"),
	})
	assertNumbers(t, "pending", m.git.ghPendingPRs, []int{5, 2})
	if m.git.syncErrors[sectionPending] == "" {
		t.Errorf("failed host should leave a sync warning")
	}
}

func TestReviewedSyncKeepsFailedHostPRs(t *testing.T) {
	m := syncTestModel(t)
	today := m.currentDate.Format("2006-01-02")
	m.git.gitSectionDates = map[string]string{sectionReviewedToday: today}
	m.git.ghReviewedToday = []GitPRItem{pendingOnHost("other.example.com", 7)}
	m.applyGitDay(gitDaySectionMsg{
		generation:  m.git.fetchGeneration,
		day:         gitDayToday,
		date:        today,
		reviewed:    []GitPRItem{pendingOnHost("git.example.com", 8)},
		failedHosts: []string{"other.example.com"},
		err:         fmt.Errorf("other.example.com: TLS handshake timeout"),
	})
	if len(m.git.ghReviewedToday) != 2 {
		t.Errorf("reviewed today = %v", pendingNumbers(m.git.ghReviewedToday))
	}
}
