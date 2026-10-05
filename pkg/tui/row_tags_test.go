package tui

import (
	"strings"
	"testing"
	"time"

	"app/pkg/model"
	"app/pkg/review"
	"app/pkg/sourcecontrol"

	"github.com/charmbracelet/lipgloss"
)

func plainRow(row string) string {
	return stripANSI(strings.TrimSuffix(row, "\n"))
}

func assertOrder(t *testing.T, row string, parts ...string) {
	t.Helper()
	last := -1
	for _, part := range parts {
		at := strings.Index(row, part)
		if at <= last {
			t.Errorf("%q should come after the previous tag in %q", part, row)
			return
		}
		last = at
	}
}

func tagTestPR() *GitPRItem {
	return &GitPRItem{Title: "Add trial state", Repository: "console", Number: 7, PR: &review.QueuedPR{
		RequestedAt: time.Now().Add(-49 * time.Hour), Additions: 100, Deletions: 20, CodeOwner: true,
	}}
}

func TestRowsEndAtTheSameRightColumn(t *testing.T) {
	m := syncTestModel(t)
	note := &model.Note{Summary: "Fix login redirect", Created: m.currentDate, Source: model.SourceManual}
	rows := map[string]string{
		"note": m.renderRow(note, false, 100),
		"pr":   m.renderPendingGitRow(tagTestPR(), false, 100),
		"repo": m.renderGitRepoRow(&GitRepoStat{Name: "digest", Commits: 5}, false, 100),
	}
	for kind, row := range rows {
		if width := lipgloss.Width(strings.TrimSuffix(row, "\n")); width != 100 {
			t.Errorf("%s row width %d, want 100: %q", kind, width, plainRow(row))
		}
	}
}

func TestRowTagOrder(t *testing.T) {
	m := syncTestModel(t)
	note := plainRow(m.renderRow(&model.Note{Summary: "Fix login", Created: m.currentDate.AddDate(0, 0, -3), Source: model.SourceManual}, false, 100))
	if !strings.HasSuffix(note, "#manual") {
		t.Errorf("note should end with #source: %q", note)
	}
	assertOrder(t, note, "Fix login", "3d ago", "#manual")

	item := tagTestPR()
	pr := plainRow(m.renderPendingGitRow(item, false, 100))
	if state := string(m.prState(item)); !strings.HasSuffix(pr, state) {
		t.Errorf("PR row should end with state %q: %q", state, pr)
	}
	assertOrder(t, pr, "Add trial state", teamReviewIcon, "±120", "2d", string(m.prState(item)))

	repo := plainRow(m.renderGitRepoRow(&GitRepoStat{Name: "digest", Commits: 5, Reviewed: 1, Assigned: 2}, false, 70))
	if !strings.HasSuffix(repo, "5 commits") {
		t.Errorf("repo row should end with commits: %q", repo)
	}
	assertOrder(t, repo, "digest", "2 assigned", "1 reviewed", "5 commits")
}

func TestLongSummaryTruncatesInsteadOfPushingTags(t *testing.T) {
	m := syncTestModel(t)
	row := m.renderRow(&model.Note{Summary: strings.Repeat("very long summary ", 20), Created: m.currentDate, Source: model.SourcePRReview}, false, 80)
	plain := plainRow(row)
	if lipgloss.Width(strings.TrimSuffix(row, "\n")) != 80 || !strings.HasSuffix(plain, "#pr-review") || !strings.Contains(plain, "…") {
		t.Errorf("summary should be cut short with tags intact: %q", plain)
	}
}

func TestNoteAgesLineUpAcrossSources(t *testing.T) {
	m := syncTestModel(t)
	m.notes = []*model.Note{
		{Summary: "carried-note", Created: m.currentDate.AddDate(0, 0, -12), Source: model.SourceManual},
		{Summary: "review-note", Created: m.currentDate.AddDate(0, 0, -2), Source: model.SourcePRReview},
	}
	content, _ := m.dashboardContent()
	ageEnds := map[int]bool{}
	for _, line := range strings.Split(stripANSI(content), "\n") {
		for _, age := range []string{"12d ago", "2d ago"} {
			if strings.Contains(line, "-note") && strings.Contains(line, age) {
				ageEnds[strings.Index(line, age)+len(age)] = true
			}
		}
	}
	if len(ageEnds) != 1 {
		t.Errorf("ages should end in one column, got %v:\n%s", ageEnds, stripANSI(content))
	}
}

func TestHeaderShowsVersionBetweenLogoAndDate(t *testing.T) {
	original := appVersion
	appVersion = "9.9.9"
	t.Cleanup(func() { appVersion = original })
	m := syncTestModel(t)
	middle := plainLines(m.renderHeader())[2]
	assertOrder(t, middle, strings.TrimSpace(bannerMiddleRow), "v9.9.9", "— ")
}

func TestPRRowShowsWhoReviewWasRequestedFrom(t *testing.T) {
	m := syncTestModel(t)
	cases := []struct {
		name          string
		codeOwner     bool
		directRequest bool
		icons         []string
		absent        []string
	}{
		{"code owner only", true, false, []string{teamReviewIcon}, []string{directReviewIcon}},
		{"by name only", false, true, []string{directReviewIcon}, []string{teamReviewIcon}},
		{"both", true, true, []string{directReviewIcon, teamReviewIcon}, nil},
		{"neither", false, false, nil, []string{directReviewIcon, teamReviewIcon}},
	}
	for _, c := range cases {
		item := tagTestPR()
		item.PR.CodeOwner, item.PR.DirectRequest = c.codeOwner, c.directRequest
		row := plainRow(m.renderPendingGitRow(item, false, 100))
		assertOrder(t, row, append(append([]string{"Add trial state"}, c.icons...), "±120")...)
		for _, icon := range c.absent {
			if strings.Contains(row, icon) {
				t.Errorf("%s: unexpected %q in %q", c.name, icon, row)
			}
		}
	}
}

func TestReReviewIconComesFirst(t *testing.T) {
	m := syncTestModel(t)
	item := tagTestPR()
	item.Kind, item.PR.DirectRequest = sourcecontrol.ReReviewKind, true
	assertOrder(t, plainRow(m.renderPendingGitRow(item, false, 100)), "Add trial state", reReviewIcon, directReviewIcon, teamReviewIcon, "±120")

	item.Kind = sourcecontrol.PendingReviewKind
	if row := plainRow(m.renderPendingGitRow(item, false, 100)); strings.Contains(row, reReviewIcon) {
		t.Errorf("pending PR should not show the re-review icon: %q", row)
	}
}

func TestHeaderDateStaysOnTodayWhileBrowsing(t *testing.T) {
	m := syncTestModel(t)
	m.currentDate = m.currentDate.AddDate(0, 0, -3)
	middle := plainLines(m.renderHeader())[2]
	if today := time.Now().Format("Monday 02 Jan"); !strings.Contains(middle, today) {
		t.Errorf("header should show today %q: %q", today, middle)
	}
}
