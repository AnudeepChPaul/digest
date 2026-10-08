package brag

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/model"
	"github.com/AnudeepChPaul/digest/pkg/review"
	"github.com/AnudeepChPaul/digest/pkg/sourcecontrol"
)

func localDate(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 15, 30, 0, 0, time.Local)
}

func TestWeekOfStartsOnMondayAndUsesISOYear(t *testing.T) {
	cases := []struct {
		day        time.Time
		id, start  string
		file       string
		labelStart string
	}{
		{localDate(2026, 10, 5), "2026-W41", "2026-10-05", "2026/week-41.md", "05 Oct"},
		{localDate(2026, 10, 11), "2026-W41", "2026-10-05", "2026/week-41.md", "05 Oct"},
		{localDate(2026, 10, 4), "2026-W40", "2026-09-28", "2026/week-40.md", "28 Sep"},
		{localDate(2027, 1, 1), "2026-W53", "2026-12-28", "2026/week-53.md", "28 Dec"},
		{localDate(2025, 12, 29), "2026-W01", "2025-12-29", "2026/week-01.md", "29 Dec"},
	}
	for _, c := range cases {
		week := WeekOf(c.day)
		if week.ID() != c.id || week.Start.Format("2006-01-02") != c.start || week.Start.Weekday() != time.Monday {
			t.Errorf("%s: id=%s start=%s", c.day.Format("2006-01-02"), week.ID(), week.Start.Format("2006-01-02"))
		}
		if got := week.Path("/root"); got != filepath.Join("/root", c.file) {
			t.Errorf("%s path = %s", c.id, got)
		}
		if !strings.Contains(week.Label(), c.labelStart) {
			t.Errorf("%s label = %s", c.id, week.Label())
		}
		if !week.End().Equal(week.Start.AddDate(0, 0, 7)) {
			t.Errorf("%s end = %s", c.id, week.End())
		}
	}
}

func TestWeeksSinceNewestFirst(t *testing.T) {
	weeks := WeeksSince(localDate(2026, 9, 17), localDate(2026, 10, 6))
	var ids []string
	for _, week := range weeks {
		ids = append(ids, week.ID())
	}
	if !reflect.DeepEqual(ids, []string{"2026-W41", "2026-W40", "2026-W39", "2026-W38"}) {
		t.Errorf("weeks = %v", ids)
	}
	if len(WeeksSince(time.Time{}, localDate(2026, 10, 6))) != 1 {
		t.Errorf("no notes should still offer the current week")
	}
}

func TestBragFileRoundTrip(t *testing.T) {
	root := t.TempDir()
	week := WeekOf(localDate(2026, 10, 1))
	created := localDate(2026, 10, 5)
	entry := &Brag{Period: week, Created: created, Updated: created, SummarizedAt: created, Facts: "### PR reviews\n- Approved:console:6 Fix", Summary: "- Unblocked the date picker"}
	if err := entry.Save(root); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(root, week)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Facts != entry.Facts || loaded.Summary != entry.Summary || !loaded.Created.Equal(created) || loaded.Period.ID() != "2026-W40" {
		t.Errorf("loaded = %+v", loaded)
	}
	edited, err := ParseBody(week, loaded.Body())
	if err != nil || edited.Facts != entry.Facts {
		t.Errorf("body does not parse back: %v %+v", err, edited)
	}
	if !Exists(root, week) || Exists(root, week.Previous()) {
		t.Errorf("exists check wrong")
	}
}

func TestParseBodyKeepsManualFactsWithoutSummary(t *testing.T) {
	week := WeekOf(localDate(2026, 10, 1))
	parsed, err := ParseBody(week, "## Facts\n\n- shipped thing\n- my manual point\n")
	if err != nil || parsed.Facts != "- shipped thing\n- my manual point" || parsed.Summary != "" {
		t.Errorf("parsed = %+v err = %v", parsed, err)
	}
	if _, err := ParseBody(week, "just text"); err == nil {
		t.Errorf("body without Facts heading should be rejected")
	}
}

func TestBuildFacts(t *testing.T) {
	week := WeekOf(localDate(2026, 9, 30))
	inWeek, before := localDate(2026, 10, 1), localDate(2026, 9, 20)
	notes := []*model.Note{
		{Summary: "Ship brag", Status: model.StatusDone, Source: model.SourceManual, Updated: inWeek},
		{Summary: "Old done", Status: model.StatusDone, Source: model.SourceManual, Updated: before},
		{Summary: "Draft RFC", Status: model.StatusActive, Source: model.SourceManual, Created: inWeek, Updated: inWeek},
		{Summary: "Synced repos", Status: model.StatusDone, Source: model.SourceRepoSync, Updated: inWeek},
		{
			Summary: "Approved:console:6 Fix date picker", Status: model.StatusDone, Source: model.SourcePRReview, Updated: inWeek,
			Body: "2026-09-29 10:00 Reviewed: Commented:console:6 Fix date picker\n2026-09-10 09:00 Reviewed: Commented:console:6 Old\nhttps://github.com/o/console/pull/6",
		},
	}
	opened := []review.QueuedPR{{Ref: review.PRRef{Repo: "console", Number: 12}, Title: "Add brag", State: "OPEN"}}
	merged := []review.QueuedPR{{Ref: review.PRRef{Repo: "web-console", Number: 3}, Title: "Fix nav", State: "MERGED"}}
	commits := map[string][]sourcecontrol.PRItem{"console": {{Title: "commit abc123: Add brag"}}}

	facts := BuildFacts(week, Sources{Notes: notes, Opened: opened, Merged: merged, Commits: commits})
	for _, want := range []string{"### Completed", "- Ship brag", "### Started", "- Draft RFC", "### PR reviews", "Approved:console:6 Fix date picker", "Reviewed: Commented:console:6 Fix date picker (Tue 29 Sep 10:00)", "### PRs opened", "- console#12 Add brag (OPEN)", "### PRs merged", "- web-console#3 Fix nav", "### Commits", "#### console", "- abc123: Add brag"} {
		if !strings.Contains(facts, want) {
			t.Errorf("facts missing %q:\n%s", want, facts)
		}
	}
	for _, unwanted := range []string{"Old done", "Synced repos", "Commented:console:6 Old"} {
		if strings.Contains(facts, unwanted) {
			t.Errorf("facts include %q:\n%s", unwanted, facts)
		}
	}
	if BuildFacts(week, Sources{}) != noActivityFacts {
		t.Errorf("empty week facts = %q", BuildFacts(week, Sources{}))
	}
}

func TestBuildFactsIncludesNoteBodies(t *testing.T) {
	week := WeekOf(localDate(2026, 9, 30))
	inWeek := localDate(2026, 10, 1)
	notes := []*model.Note{
		{Summary: "Debugged WFM", Status: model.StatusDone, Source: model.SourceManual, Updated: inWeek, Body: "\nhttps://jira/PROJ-1\n\n  Findings in slack thread  \n"},
		{Summary: "Draft RFC", Status: model.StatusActive, Source: model.SourceManual, Created: inWeek, Updated: inWeek, Body: "Outline ready"},
		{Summary: "No details", Status: model.StatusDone, Source: model.SourceManual, Updated: inWeek},
	}
	facts := BuildFacts(week, Sources{Notes: notes})
	for _, want := range []string{
		"- Debugged WFM\n  https://jira/PROJ-1\n  Findings in slack thread\n",
		"- Draft RFC\n  Outline ready",
		"- No details",
	} {
		if !strings.Contains(facts, want) {
			t.Errorf("facts missing %q:\n%s", want, facts)
		}
	}
	if strings.Contains(facts, "\n  \n") || strings.Contains(facts, "- No details\n  ") {
		t.Errorf("facts include blank body lines:\n%s", facts)
	}
}

func TestGenerateSummaryKeepsOurFacts(t *testing.T) {
	original := runBragCommand
	var stdin string
	runBragCommand = func(ctx context.Context, command, input string) (string, error) {
		stdin = input
		return "## Facts\n\n- rewritten by model\n\n## Summary\n\n- Shipped brag\n", nil
	}
	defer func() { runBragCommand = original }()
	week := WeekOf(localDate(2026, 10, 1))
	summary, err := GenerateSummary(context.Background(), "claude -p", "Summarise my week.", week, "- shipped brag")
	if err != nil || summary != "- Shipped brag" || !strings.HasPrefix(stdin, "Summarise my week.\n\n## Facts") || !strings.Contains(stdin, "- shipped brag") {
		t.Errorf("summary=%q err=%v stdin=%q", summary, err, stdin)
	}
	runBragCommand = func(ctx context.Context, command, input string) (string, error) { return "  plain summary  ", nil }
	if summary, _ := GenerateSummary(context.Background(), "x", "p", week, "f"); summary != "plain summary" {
		t.Errorf("summary without heading = %q", summary)
	}
	runBragCommand = func(ctx context.Context, command, input string) (string, error) { return "   ", nil }
	if _, err := GenerateSummary(context.Background(), "x", "p", week, "f"); err == nil {
		t.Errorf("empty output should fail")
	}
}

func TestGenerateSummaryTimesOutHungCommand(t *testing.T) {
	originalCommand, originalTimeout := runBragCommand, bragCommandTimeout
	defer func() { runBragCommand, bragCommandTimeout = originalCommand, originalTimeout }()
	bragCommandTimeout = 50 * time.Millisecond
	runBragCommand = func(ctx context.Context, command, input string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}
	started := time.Now()
	_, err := GenerateSummary(context.Background(), "x", "p", WeekOf(localDate(2026, 10, 1)), "f")
	if err == nil || !strings.Contains(err.Error(), "timed out after") {
		t.Errorf("hung command err = %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Errorf("timeout took %v", elapsed)
	}
}

func TestRunBragCommandKillsChildrenOnCancel(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if _, err := runBragCommand(ctx, `sleep 30 & echo $! > "`+pidFile+`"; wait`, ""); err == nil {
		t.Fatal("cancelled command should fail")
	}
	childPID, ok := readInt(pidFile)
	if !ok {
		t.Fatal("child pid not recorded")
	}
	deadline := time.Now().Add(2 * time.Second)
	for processAlive(childPID) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if processAlive(childPID) {
		t.Errorf("child %d still running after cancel", childPID)
	}
}

func TestDefaultBragTimeoutIsFiveMinutes(t *testing.T) {
	if bragCommandTimeout != 5*time.Minute {
		t.Errorf("brag timeout = %v", bragCommandTimeout)
	}
}

func TestCreateAndRegenerate(t *testing.T) {
	root := t.TempDir()
	week := WeekOf(localDate(2026, 9, 30))
	original := runBragCommand
	summaries := []string{"## Summary\n\n- first", "## Summary\n\n- second"}
	var inputs []string
	runBragCommand = func(ctx context.Context, command, input string) (string, error) {
		inputs = append(inputs, input)
		next := summaries[0]
		summaries = summaries[1:]
		return next, nil
	}
	defer func() { runBragCommand = original }()

	created, err := Create(context.Background(), root, "cmd", "prompt", week, "- collected fact")
	if err != nil || created.Summary != "- first" || created.Facts != "- collected fact" {
		t.Fatalf("created = %+v err = %v", created, err)
	}
	edited, _ := Load(root, week)
	edited.Facts += "\n- my manual point"
	if err := edited.Save(root); err != nil {
		t.Fatal(err)
	}
	regenerated, err := Regenerate(context.Background(), root, "cmd", "prompt", week)
	if err != nil || regenerated.Summary != "- second" || !strings.Contains(regenerated.Facts, "- my manual point") {
		t.Fatalf("regenerated = %+v err = %v", regenerated, err)
	}
	if !strings.Contains(inputs[1], "- my manual point") {
		t.Errorf("regenerate did not send edited facts: %q", inputs[1])
	}
	if !regenerated.Created.Equal(created.Created) {
		t.Errorf("created time changed on regenerate")
	}
}

func TestMonthAndYearPeriods(t *testing.T) {
	month := MonthOf(localDate(2026, 10, 17))
	if month.ID() != "2026-10" || month.Label() != "October 2026" || month.Path("/r") != filepath.Join("/r", "2026", "month-10.md") {
		t.Errorf("month = %s %s %s", month.ID(), month.Label(), month.Path("/r"))
	}
	start, end := month.Range()
	if start.Format("2006-01-02") != "2026-10-01" || end.Format("2006-01-02") != "2026-11-01" {
		t.Errorf("month range = %s..%s", start, end)
	}
	year := YearOf(localDate(2026, 10, 17))
	if year.ID() != "2026" || year.Path("/r") != filepath.Join("/r", "2026", "performance-review.md") {
		t.Errorf("year = %s %s", year.ID(), year.Path("/r"))
	}
	if got := MonthOfWeek(WeekOf(localDate(2026, 9, 30))); got.ID() != "2026-10" {
		t.Errorf("week ending Sun 4 Oct belongs to %s", got.ID())
	}
	if got := MonthOfWeek(WeekOf(localDate(2027, 1, 1))); got.ID() != "2027-01" {
		t.Errorf("2026-W53 ends in January 2027, got %s", got.ID())
	}
}

func TestParsePeriod(t *testing.T) {
	for id, kind := range map[string]string{"2026-W40": "week", "2026-W01": "week", "2026-W53": "week", "2026-10": "month", "2026": "year"} {
		period, err := ParsePeriod(id, time.Local)
		if err != nil || period.ID() != id || string(period.Kind()) != kind {
			t.Errorf("%s: %v %v", id, period, err)
		}
	}
	if week, _ := ParsePeriod("2026-W01", time.Local); week.(Week).Start.Format("2006-01-02") != "2025-12-29" {
		t.Errorf("2026-W01 start = %v", week.(Week).Start)
	}
	for _, bad := range []string{"", "2026-W54", "2025-W53", "2026-13", "26", "2026-W4"} {
		if _, err := ParsePeriod(bad, time.Local); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestBraggable(t *testing.T) {
	now := localDate(2026, 10, 5)
	if Braggable(WeekOf(now), now) || !Braggable(WeekOf(now).Previous(), now) {
		t.Errorf("only ended weeks are braggable")
	}
	if Braggable(MonthOf(now), now) || !Braggable(MonthOf(localDate(2026, 9, 1)), now) {
		t.Errorf("only ended months are braggable")
	}
	if !Braggable(YearOf(now), now) {
		t.Errorf("the year review is braggable any time")
	}
}

func TestMonthFactsRollUpSavedWeeks(t *testing.T) {
	root := t.TempDir()
	october := MonthOf(localDate(2026, 10, 1))
	if _, err := MonthFacts(root, october); err == nil {
		t.Errorf("month without weekly brags should fail")
	}
	inMonth := WeekOf(localDate(2026, 9, 30))
	notInMonth := WeekOf(localDate(2026, 9, 23))
	for _, entry := range []*Brag{
		{Period: inMonth, Facts: "### Completed\n- Ship brag", Summary: "## Highlights\n- Shipped brag"},
		{Period: notInMonth, Facts: "- September only", Summary: "- Sep"},
	} {
		if err := entry.Save(root); err != nil {
			t.Fatal(err)
		}
	}
	facts, err := MonthFacts(root, october)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"### Week 40", "#### Summary", "#### Highlights", "#### Facts", "##### Completed", "- Ship brag"} {
		if !strings.Contains(facts, want) {
			t.Errorf("month facts missing %q:\n%s", want, facts)
		}
	}
	if strings.Contains(facts, "September only") {
		t.Errorf("week ending in September leaked into October:\n%s", facts)
	}
}

func TestYearFactsUseMonthSummaries(t *testing.T) {
	root := t.TempDir()
	year := YearOf(localDate(2026, 1, 1))
	if _, err := YearFacts(root, year); err == nil {
		t.Errorf("year without month brags should fail")
	}
	month := &Brag{Period: MonthOf(localDate(2026, 9, 1)), Facts: "- long weekly detail", Summary: "- Led the migration"}
	if err := month.Save(root); err != nil {
		t.Fatal(err)
	}
	facts, err := YearFacts(root, year)
	if err != nil || !strings.Contains(facts, "### September 2026") || !strings.Contains(facts, "- Led the migration") || strings.Contains(facts, "long weekly detail") {
		t.Errorf("year facts = %q err = %v", facts, err)
	}
}

func TestPerformanceReviewKeepsWholeOutput(t *testing.T) {
	original := runBragCommand
	runBragCommand = func(ctx context.Context, command, input string) (string, error) {
		return "## Summary\nGreat year.\n\n## Growth areas\n- Delegate more", nil
	}
	defer func() { runBragCommand = original }()
	root := t.TempDir()
	year := YearOf(localDate(2026, 1, 1))
	created, err := Create(context.Background(), root, "cmd", "prompt", year, "### September 2026\n- Led")
	if err != nil || !strings.Contains(created.Summary, "## Summary\nGreat year.") || !strings.Contains(created.Summary, "## Growth areas") {
		t.Fatalf("created = %+v err = %v", created, err)
	}
	loaded, err := Load(root, year)
	if err != nil || loaded.Summary != created.Summary || loaded.Facts != "### September 2026\n- Led" {
		t.Errorf("loaded = %+v err = %v", loaded, err)
	}
}

func TestHeadingsMatchWholeLinesOnly(t *testing.T) {
	month := MonthOf(localDate(2026, 10, 1))
	parsed, err := ParseBody(month, "## Facts\n\n### Week 40\n\n#### Summary\n\n- weekly\n\n## Summary\n\n- monthly\n")
	if err != nil || parsed.Summary != "- monthly" || !strings.Contains(parsed.Facts, "#### Summary") {
		t.Errorf("parsed = %+v err = %v", parsed, err)
	}
}
