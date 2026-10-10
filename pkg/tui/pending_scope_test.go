package tui

import (
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/review"
	"github.com/achandrapaul/digest/pkg/sourcecontrol"

	tea "github.com/charmbracelet/bubbletea"
)

func scopeTestModel(t *testing.T) Model {
	t.Helper()
	m := syncTestModel(t)
	base := time.Now().Add(-time.Hour)
	pr := func(number int, direct bool, age time.Duration) GitPRItem {
		queued := review.QueuedPR{Ref: prRef("console", number), Title: "PR", CIState: "SUCCESS", DirectRequest: direct, CreatedAt: base.Add(-age), UpdatedAt: base.Add(-age)}
		return sourcecontrol.NewPRItem(queued, "Pending Review")
	}
	m.git.ghPendingPRs = []GitPRItem{pr(1, false, 1*time.Hour), pr(2, true, 2*time.Hour), pr(3, true, 3*time.Hour)}
	m.rebuildGitRepoStats()
	return m
}

func shownPending(model tea.Model) []int {
	return pendingNumbers(model.(Model).git.pendingGitAction)
}

func TestMeKeyTogglesExplicitlyRequestedPRs(t *testing.T) {
	m := scopeTestModel(t)
	next, cmd := m.Update(runes("m"))
	meOnly := next.(Model)
	if cmd != nil {
		t.Errorf("toggling me-only should not start a fetch")
	}
	if got := shownPending(meOnly); !slices.Equal(got, []int{2, 3}) {
		t.Errorf("me-only should keep explicitly requested PRs, got %v", got)
	}
	next, _ = meOnly.Update(runes("m"))
	if got := shownPending(next); len(got) != 3 {
		t.Errorf("toggling back should restore all PRs, got %v", got)
	}
}

func TestMeOnlyHonoursSortOrder(t *testing.T) {
	m := scopeTestModel(t)
	next, _ := m.Update(runes("m"))
	descending := shownPending(next)
	next, _ = next.(Model).Update(runes("w"))
	ascending := shownPending(next)
	if len(descending) != 2 || len(ascending) != 2 || descending[0] != ascending[1] || descending[1] != ascending[0] {
		t.Errorf("sort order should flip within me-only: desc=%v asc=%v", descending, ascending)
	}
}

func TestMeOnlyIsNeitherCachedNorSaved(t *testing.T) {
	m := scopeTestModel(t)
	next, _ := m.Update(runes("m"))
	if _, err := os.Stat(gitCachePath()); err == nil {
		raw, _ := os.ReadFile(gitCachePath())
		if strings.Contains(strings.ToLower(string(raw)), "meonly") {
			t.Errorf("me-only should not be cached: %s", raw)
		}
	}
	fresh := NewModel(next.(Model).cfg, nil)
	if fresh.git.pendingMeOnly {
		t.Errorf("a new launch should start in me + team")
	}
}

func TestPendingHeaderShowsScopeIcons(t *testing.T) {
	m := scopeTestModel(t)
	hint := stripANSI(m.renderPendingSortHint())
	if !strings.Contains(hint, "m "+directReviewIcon+" "+teamReviewIcon) {
		t.Errorf("default hint should show me + team: %q", hint)
	}
	next, _ := m.Update(runes("m"))
	hint = stripANSI(next.(Model).renderPendingSortHint())
	if !strings.HasSuffix(hint, "m "+directReviewIcon) {
		t.Errorf("me-only hint should show only the user icon: %q", hint)
	}
}

func TestMeOnlyEmptyStateMessage(t *testing.T) {
	m := scopeTestModel(t)
	m.git.loadingGit = false
	m.git.ghPendingPRs = m.git.ghPendingPRs[:1]
	m.rebuildGitRepoStats()
	next, _ := m.Update(runes("m"))
	content, _ := next.(Model).dashboardContent()
	if !strings.Contains(stripANSI(content), "(no PRs asking you by name)") {
		t.Errorf("empty me-only list should say so:\n%s", stripANSI(content))
	}
}
