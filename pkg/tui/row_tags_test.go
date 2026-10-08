package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/config"
	"github.com/AnudeepChPaul/digest/pkg/model"
	"github.com/AnudeepChPaul/digest/pkg/review"
	"github.com/AnudeepChPaul/digest/pkg/sourcecontrol"

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
	note := plainRow(m.renderRow(&model.Note{Summary: "Fix login", Created: m.currentDate.AddDate(0, 0, -3), Source: model.SourceManual}, true, 100))
	if !strings.HasSuffix(note, "#manual") {
		t.Errorf("note should end with #source: %q", note)
	}
	assertOrder(t, note, "Fix login", "3d ago", "#manual")

	item := tagTestPR()
	pr := plainRow(m.renderPendingGitRow(item, true, 100))
	if !strings.HasSuffix(pr, teamReviewIcon) {
		t.Errorf("PR row should end with the request icons: %q", pr)
	}
	assertOrder(t, pr, "Add trial state", "±120", "2d", string(m.prState(item)), teamReviewIcon)

	repo := plainRow(m.renderGitRepoRow(&GitRepoStat{Name: "digest", Commits: 5, Reviewed: 1, Assigned: 2}, false, 70))
	if !strings.HasSuffix(repo, "5 commits") {
		t.Errorf("repo row should end with commits: %q", repo)
	}
	assertOrder(t, repo, "digest", "2 assigned", "1 reviewed", "5 commits")
}

func TestLongSummaryTruncatesInsteadOfPushingTags(t *testing.T) {
	m := syncTestModel(t)
	row := m.renderRow(&model.Note{Summary: strings.Repeat("very long summary ", 20), Created: m.currentDate, Source: model.SourcePRReview}, true, 80)
	plain := plainRow(row)
	if lipgloss.Width(strings.TrimSuffix(row, "\n")) != 80 || !strings.HasSuffix(plain, "#pr-review") || !strings.Contains(plain, "…") {
		t.Errorf("summary should be cut short with tags intact: %q", plain)
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
		{"both", true, true, []string{directReviewIcon}, []string{teamReviewIcon}},
		{"neither", false, false, nil, []string{directReviewIcon, teamReviewIcon}},
	}
	for _, c := range cases {
		item := tagTestPR()
		item.PR.CodeOwner, item.PR.DirectRequest = c.codeOwner, c.directRequest
		row := plainRow(m.renderPendingGitRow(item, true, 100))
		assertOrder(t, row, append([]string{"Add trial state", "±120", string(m.prState(item))}, c.icons...)...)
		for _, icon := range c.absent {
			if strings.Contains(row, icon) {
				t.Errorf("%s: unexpected %q in %q", c.name, icon, row)
			}
		}
	}
}

func TestPRStateColumnAlignsAcrossIconCounts(t *testing.T) {
	m := syncTestModel(t)
	stateEnd := func(item *GitPRItem) int {
		row := plainRow(m.renderPendingGitRow(item, true, 100))
		state := string(m.prState(item))
		index := strings.LastIndex(row, state)
		if index < 0 {
			t.Fatalf("state %q missing from %q", state, row)
		}
		return lipgloss.Width(row[:index+len(state)])
	}
	oneIcon := tagTestPR()
	oneIcon.PR.CodeOwner = false
	twoIcons := tagTestPR()
	twoIcons.Kind, twoIcons.PR.DirectRequest = sourcecontrol.ReReviewKind, true
	if one, two := stateEnd(oneIcon), stateEnd(twoIcons); one != two {
		t.Errorf("state column should not shift with icon count: %d vs %d", one, two)
	}
}

func TestReReviewIconComesFirst(t *testing.T) {
	m := syncTestModel(t)
	item := tagTestPR()
	item.Kind, item.PR.DirectRequest = sourcecontrol.ReReviewKind, true
	reReviewRow := plainRow(m.renderPendingGitRow(item, true, 100))
	assertOrder(t, reReviewRow, "Add trial state", "±120", string(m.prState(item)), reReviewIcon, directReviewIcon)
	if strings.Contains(reReviewRow, teamReviewIcon) {
		t.Errorf("only two icons fit, so the code-owner icon should yield to the direct request: %q", reReviewRow)
	}

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

func TestPRRowShowsSizeAndAgeOnlyWhenSelected(t *testing.T) {
	m := syncTestModel(t)
	item := tagTestPR()
	state := string(m.prState(item))
	resting := plainRow(m.renderPendingGitRow(item, false, 100))
	assertOrder(t, resting, "Add trial state", state, teamReviewIcon)
	if strings.Contains(resting, "±120") || strings.Contains(resting, "2d") {
		t.Errorf("unselected PR row should hide size and age: %q", resting)
	}
	assertOrder(t, plainRow(m.renderPendingGitRow(item, true, 100)), "Add trial state", "±120", "2d", state, teamReviewIcon)
	m.cfg.ShowTags = config.ShowTagsAlways
	assertOrder(t, plainRow(m.renderPendingGitRow(item, false, 100)), "Add trial state", "±120", "2d", state, teamReviewIcon)
}

func TestPRTagsHugTheRightEdgeWithoutPadding(t *testing.T) {
	m := selectionTestModel(t)
	oneIcon, threeIcons, noIcons := pendingItem(1), pendingItem(2), pendingItem(3)
	oneIcon.Title, threeIcons.Title, noIcons.Title = "One icon PR", "Three icons PR", "No icons PR"
	oneIcon.PR.CodeOwner = true
	threeIcons.Kind, threeIcons.PR.DirectRequest, threeIcons.PR.CodeOwner = sourcecontrol.ReReviewKind, true, true
	m.ghPendingPRs = []GitPRItem{oneIcon, threeIcons, noIcons}
	m.rebuildGitRepoStats()
	selectNavItem(t, &m, "pr:"+threeIcons.URL)
	lines := plainLines(m.View())
	for _, title := range []string{"One icon PR", "Three icons PR", "No icons PR"} {
		row := slicesIndex(lines, title)
		if row < 0 {
			t.Fatalf("%s missing:\n%s", title, strings.Join(lines, "\n"))
		}
		content := strings.TrimSuffix(strings.TrimSuffix(lines[row], "│"), " ")
		if strings.HasSuffix(content, " ") {
			t.Errorf("%s: tags should end at the right edge without padding: %q", title, lines[row])
		}
	}
	if row := lines[slicesIndex(lines, "No icons PR")]; !strings.HasSuffix(strings.TrimSuffix(strings.TrimSuffix(row, "│"), " "), string(m.prState(&m.ghPendingPRs[2]))+"  "+firstReviewIcon) {
		t.Errorf("a row with no audience should end with status then the first-review marker: %q", row)
	}
}

func TestReviewRequestIconsPairPassWithAudience(t *testing.T) {
	cases := []struct {
		name              string
		kind              string
		direct, codeOwner bool
		want              string
	}{
		{"first review for me", sourcecontrol.PendingReviewKind, true, false, "\U000F0CA1\U000F0065"},
		{"first review for my team", sourcecontrol.PendingReviewKind, false, true, "\U000F0CA1\U000F0849"},
		{"first review for both", sourcecontrol.PendingReviewKind, true, true, "\U000F0CA1\U000F0065"},
		{"re-review for me", sourcecontrol.ReReviewKind, true, false, "\U000F0458\U000F0065"},
		{"re-review for my team", sourcecontrol.ReReviewKind, false, true, "\U000F0458\U000F0849"},
		{"re-review with no audience", sourcecontrol.ReReviewKind, false, false, "\U000F0458"},
	}
	for _, c := range cases {
		item := tagTestPR()
		item.Kind, item.PR.DirectRequest, item.PR.CodeOwner = c.kind, c.direct, c.codeOwner
		if got := stripANSI(reviewRequestIcon(item)); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}
