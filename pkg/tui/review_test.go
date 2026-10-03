package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"app/pkg/config"
	"app/pkg/review"
	"app/pkg/sourcecontrol"

	tea "github.com/charmbracelet/bubbletea"
)

func reviewTestModel(t *testing.T) Model {
	t.Helper()
	root := t.TempDir()
	cfg := &config.Config{ReviewRoot: root, GreenOnly: true}
	ref := review.PRRef{Host: "github.com", Owner: "o", Repo: "console", Number: 7, URL: "https://github.com/o/console/pull/7"}
	stateDir := review.StateDir(root, ref)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	findings := `{"recommendation":"REQUEST_CHANGES","findings":[
{"severity":"high","title":"H1","path":"a.ts","line":2,"body":"b"},
{"severity":"critical","title":"C1","path":"b.ts","line":5,"body":"c"}]}`
	for name, content := range map[string]string{review.FindingsFile: findings, "review.exit": "0"} {
		if err := os.WriteFile(filepath.Join(stateDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	m := NewModel(cfg, nil)
	m.width, m.height = 120, 40
	m.ghPendingPRs = []GitPRItem{sourcecontrol.NewPRItem(review.QueuedPR{Ref: ref, Title: "Fix", HeadSHA: "sha", CIState: "SUCCESS"}, "Pending Review")}
	m.rebuildGitRepoStats()
	m.selected = 0
	m.mode = ViewPreview
	m.resetReviewView()
	return m
}

func press(t *testing.T, m Model, msg tea.KeyMsg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(Model)
}

func runes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestReviewTabSelection(t *testing.T) {
	m := reviewTestModel(t)
	if m.currentPRItem() == nil {
		t.Fatal("expected PR item selected")
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.previewTab != previewTabReview {
		t.Fatalf("previewTab = %d", m.previewTab)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeySpace})
	if m.selectedCount() != 1 {
		t.Errorf("after space selected = %d", m.selectedCount())
	}
	if got := m.selectedFindings(m.currentPRItem()); len(got) != 1 || got[0].Title != "C1" {
		t.Errorf("first finding should be critical C1, got %+v", got)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlA})
	if m.selectedCount() != 2 {
		t.Errorf("after ctrl+a selected = %d", m.selectedCount())
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlA})
	if m.selectedCount() != 0 {
		t.Errorf("second ctrl+a should clear, selected = %d", m.selectedCount())
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != ViewPreview || m.reviewNotice != "" {
		t.Errorf("enter without selection should do nothing, mode=%d notice=%q", m.mode, m.reviewNotice)
	}
	m = press(t, m, runes("j"))
	m = press(t, m, tea.KeyMsg{Type: tea.KeySpace})
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != ViewReviewConfirm || m.reviewEvent != review.EventComment {
		t.Errorf("enter with selection should confirm COMMENT, mode=%d event=%s", m.mode, m.reviewEvent)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != ViewPreview || m.selectedCount() != 1 {
		t.Errorf("esc should cancel and keep selection, mode=%d selected=%d", m.mode, m.selectedCount())
	}
}

func TestRejectNeedsComment(t *testing.T) {
	m := reviewTestModel(t)
	m = press(t, m, runes("d"))
	if m.mode != ViewRejectComment {
		t.Fatalf("d without selection should open comment box, mode=%d", m.mode)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if m.mode != ViewRejectComment || m.reviewNotice == "" {
		t.Errorf("empty comment should be blocked, mode=%d", m.mode)
	}
	m.rejectInput.SetValue("please add tests")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if m.mode != ViewReviewConfirm || m.reviewEvent != review.EventRequestChanges || m.reviewBody != "please add tests" {
		t.Errorf("mode=%d event=%s body=%q", m.mode, m.reviewEvent, m.reviewBody)
	}
}

func TestRejectWithSelectionSkipsCommentBox(t *testing.T) {
	m := reviewTestModel(t)
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = press(t, m, tea.KeyMsg{Type: tea.KeySpace})
	m = press(t, m, runes("d"))
	if m.mode != ViewReviewConfirm || m.reviewEvent != review.EventRequestChanges {
		t.Errorf("mode=%d event=%s", m.mode, m.reviewEvent)
	}
}

func TestApproveAlwaysConfirms(t *testing.T) {
	for _, key := range []string{"a", "y"} {
		m := reviewTestModel(t)
		m = press(t, m, runes(key))
		if m.mode != ViewReviewConfirm || m.reviewEvent != review.EventApprove {
			t.Errorf("%s: mode=%d event=%s", key, m.mode, m.reviewEvent)
		}
	}
}

func TestReviewTabRendersGroupedFindings(t *testing.T) {
	m := reviewTestModel(t)
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	content, cursorLine := m.renderReviewContent(m.currentPRItem(), 80)
	if cursorLine < 0 {
		t.Errorf("cursor line not tracked")
	}
	for _, want := range []string{"CRITICAL (1)", "HIGH (1)", "C1", "H1", "b.ts:5", "REVIEWED"} {
		if !strings.Contains(content, want) {
			t.Errorf("review content missing %q", want)
		}
	}
	if m.prState(m.currentPRItem()) != review.StateReviewed {
		t.Errorf("state = %s", m.prState(m.currentPRItem()))
	}
}
