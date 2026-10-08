package benchmark

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const goBenchOutput = `goos: darwin
goarch: arm64
pkg: github.com/AnudeepChPaul/digest/pkg/tui
cpu: Apple M1 Pro
BenchmarkScenarioStartup/notes=100-8      	      20	  10174767 ns/op	         0.5312 heap-MB	 2042562 B/op	   15548 allocs/op
BenchmarkScenarioStartup/notes=10000-8    	       2	 812000000 ns/op	        41.20 heap-MB	92042562 B/op	 1554800 allocs/op
BenchmarkScenarioTyping/lines=10000/cursor=bottom-8 	      20	  84907256 ns/op	38999629 B/op	  281458 allocs/op
BenchmarkScenarioNavigate/notes=10000-8   	     900	   1300000 ns/op	  309799 B/op	    3954 allocs/op
--- SKIP: BenchmarkScenarioOverlayLinkMenu/notes=1
PASS
ok  	github.com/AnudeepChPaul/digest/pkg/tui	27.221s
`

func TestParseGoBenchReadsEveryMetricAndTheCPU(t *testing.T) {
	results, cpu := ParseGoBench(goBenchOutput)
	if cpu != "Apple M1 Pro" {
		t.Errorf("cpu = %q", cpu)
	}
	if len(results) != 4 {
		t.Fatalf("results = %+v", results)
	}
	startup := results[0]
	if startup.Name != "Startup/notes=100" {
		t.Errorf("name = %q, want the scenario without prefix and CPU suffix", startup.Name)
	}
	want := map[string]float64{"ns/op": 10174767, "heap-MB": 0.5312, "B/op": 2042562, "allocs/op": 15548}
	for unit, value := range want {
		if startup.Metrics[unit] != value {
			t.Errorf("%s = %v, want %v", unit, startup.Metrics[unit], value)
		}
	}
	if results[2].Name != "Typing/lines=10000/cursor=bottom" {
		t.Errorf("typing name = %q", results[2].Name)
	}
}

func sampleRun(version, commit string, startupNanos float64) Run {
	return Run{
		Date:    time.Date(2026, 10, 8, 21, 0, 0, 0, time.UTC),
		Version: version,
		Commit:  commit,
		CPU:     "Apple M1 Pro",
		Results: []Result{
			{Name: "Startup/notes=100", Metrics: map[string]float64{"ns/op": 10e6, "B/op": 2e6, "allocs/op": 15548, "heap-MB": 0.5}},
			{Name: "Startup/notes=10000", Metrics: map[string]float64{"ns/op": startupNanos, "B/op": 92e6, "allocs/op": 1554800, "heap-MB": 41.2}},
			{Name: "Navigate/notes=10000", Metrics: map[string]float64{"ns/op": 1.3e6, "B/op": 309799, "allocs/op": 3954}},
			{Name: "Typing/lines=10000/cursor=bottom", Metrics: map[string]float64{"ns/op": 84.9e6, "B/op": 39e6, "allocs/op": 281458}},
			{Name: "Process/start", Metrics: map[string]float64{"ns/op": 9.5e6, "rss-MB": 14.2}},
		},
	}
}

func TestMarkdownShowsTheLatestRunAndTheGivenHistoryRows(t *testing.T) {
	newer, older := sampleRun("v1.2.6", "def5678", 700e6), sampleRun("v1.2.5", "abc1234", 812e6)
	markdown := Markdown(newer, []string{HistoryRow(newer), HistoryRow(older)})
	for _, want := range []string{
		"# Benchmarks",
		"v1.2.6 `def5678`",
		"Apple M1 Pro",
		"| Startup |",
		"700 ms · 87.7 MB",
		"10 ms · 1.9 MB",
		"| Startup heap |",
		"41.2 MB",
		"| 10000 lines |",
		"84.9 ms",
		"9.5 ms",
		"14.2 MB",
		"mise run bench",
	} {
		if !strings.Contains(markdown, want) {
			t.Errorf("markdown lacks %q:\n%s", want, markdown)
		}
	}
	history := markdown[strings.Index(markdown, "## History"):]
	if first, second := strings.Index(history, "def5678"), strings.Index(history, "abc1234"); first < 0 || second < 0 || first > second {
		t.Errorf("history should keep the given order, def5678 first:\n%s", history)
	}
	if !strings.Contains(history, "| v1.2.5 | `abc1234` | Apple M1 Pro |") || !strings.Contains(history, "812 ms") {
		t.Errorf("history row lacks version, commit, machine or headline numbers:\n%s", history)
	}
}

func TestHistoryRowsReadsOnlyTheRowsBetweenTheMarkers(t *testing.T) {
	run := sampleRun("v1.2.5", "abc1234", 812e6)
	markdown := Markdown(run, []string{HistoryRow(run)}) + "\n| stray | row |\n"
	rows := HistoryRows(markdown)
	if len(rows) != 1 || rows[0] != HistoryRow(run) {
		t.Errorf("rows = %q", rows)
	}
	if rows := HistoryRows("no markers here\n| a | b |\n"); len(rows) != 0 {
		t.Errorf("rows without markers = %q", rows)
	}
}

func TestRecordAddsEachRunOnTopAndShowsOnlyTheLatestTables(t *testing.T) {
	root := t.TempDir()
	if err := Record(root, sampleRun("v1.2.5", "abc1234", 812e6)); err != nil {
		t.Fatal(err)
	}
	if err := Record(root, sampleRun("v1.2.6", "def5678", 700e6)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, MarkdownFile))
	if err != nil {
		t.Fatal(err)
	}
	markdown := string(data)
	rows := HistoryRows(markdown)
	if len(rows) != 2 || !strings.Contains(rows[0], "def5678") || !strings.Contains(rows[1], "abc1234") {
		t.Fatalf("rows = %q", rows)
	}
	latest := markdown[:strings.Index(markdown, "## History")]
	if !strings.Contains(latest, "700 ms") || strings.Contains(latest, "812 ms") {
		t.Errorf("latest tables should show only the newest run:\n%s", latest)
	}
	if _, err := os.Stat(filepath.Join(root, "benchmarks", "history.csv")); !os.IsNotExist(err) {
		t.Errorf("history.csv should not be written, stat err = %v", err)
	}
}

func TestMarkdownEscapesPipes(t *testing.T) {
	run := sampleRun("v1.2.5", "abc1234", 1e6)
	run.CPU = "Apple | M1"
	markdown := Markdown(run, []string{HistoryRow(run)})
	if strings.Contains(markdown, "Apple | M1") || !strings.Contains(markdown, `Apple \| M1`) {
		t.Errorf("pipe not escaped:\n%s", markdown)
	}
}

func TestMarkdownNamesAnUncommittedRunTheWorkingTree(t *testing.T) {
	run := sampleRun("v1.2.5", "", 1e6)
	markdown := Markdown(run, []string{HistoryRow(run)})
	if strings.Contains(markdown, "``") || !strings.Contains(markdown, "working tree") {
		t.Errorf("uncommitted run should say working tree:\n%s", markdown)
	}
}

func TestFormatting(t *testing.T) {
	cases := map[float64]string{850: "850 ns", 12_300: "12.3 µs", 1_510_000: "1.51 ms", 1_200_000_000: "1.2 s"}
	for nanos, want := range cases {
		if got := formatDuration(nanos); got != want {
			t.Errorf("formatDuration(%v) = %q, want %q", nanos, got, want)
		}
	}
	bytes := map[float64]string{512: "512 B", 340_000: "332 KB", 2_097_152: "2.0 MB"}
	for size, want := range bytes {
		if got := formatBytes(size); got != want {
			t.Errorf("formatBytes(%v) = %q, want %q", size, got, want)
		}
	}
}
