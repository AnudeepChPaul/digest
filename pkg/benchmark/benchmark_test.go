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
pkg: github.com/achandrapaul/digest/pkg/tui
cpu: Apple M1 Pro
BenchmarkScenarioStartup/notes=100-8      	      20	  10174767 ns/op	         0.5312 heap-MB	 2042562 B/op	   15548 allocs/op
BenchmarkScenarioStartup/notes=10000-8    	       2	 812000000 ns/op	        41.20 heap-MB	92042562 B/op	 1554800 allocs/op
BenchmarkScenarioTyping/lines=10000/cursor=bottom-8 	      20	  84907256 ns/op	38999629 B/op	  281458 allocs/op
BenchmarkScenarioNavigate/notes=10000-8   	     900	   1300000 ns/op	  309799 B/op	    3954 allocs/op
--- SKIP: BenchmarkScenarioOverlayLinkMenu/notes=1
PASS
ok  	github.com/achandrapaul/digest/pkg/tui	27.221s
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

func TestMarkdownShowsTheLatestRun(t *testing.T) {
	markdown := Markdown(sampleRun("v1.2.6", "def5678", 700e6), "")
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
		"each reloads only that note",
	} {
		if !strings.Contains(markdown, want) {
			t.Errorf("markdown lacks %q:\n%s", want, markdown)
		}
	}
	if strings.Contains(markdown, "each reloads every note") {
		t.Errorf("markdown still says saving reloads every note")
	}
}

func historySection(markdown string) string {
	_, afterStart, _ := strings.Cut(markdown, historyStart)
	body, _, _ := strings.Cut(afterStart, historyEnd)
	return body
}

func historyLine(t *testing.T, markdown, title, label string) string {
	t.Helper()
	_, table, found := strings.Cut(historySection(markdown), "### "+title+"\n")
	if !found {
		t.Fatalf("history lacks %q table:\n%s", title, historySection(markdown))
	}
	table, _, _ = strings.Cut(table, "### ")
	for _, line := range strings.Split(table, "\n") {
		if strings.HasPrefix(line, "| "+label+" |") {
			return line
		}
	}
	return ""
}

func TestHistoryPivotsScenariosIntoRowsAndRunsIntoColumnsNewestFirst(t *testing.T) {
	markdown := Markdown(sampleRun("v1.2.6", "def5678", 700e6), Markdown(sampleRun("v1.2.5", "abc1234", 812e6), ""))
	header := historyLine(t, markdown, "10k notes", "Scenario")
	newer, older := strings.Index(header, "v1.2.6"), strings.Index(header, "v1.2.5")
	if newer < 0 || older < 0 || newer > older {
		t.Errorf("header should list v1.2.6 before v1.2.5: %q", header)
	}
	for _, want := range []string{"2026-10-08", "`def5678`", "Apple M1 Pro"} {
		if !strings.Contains(header, want) {
			t.Errorf("header lacks %q: %q", want, header)
		}
	}
	if got := historyLine(t, markdown, "10k notes", "Startup"); got != "| Startup | 700 ms · 87.7 MB | 812 ms · 87.7 MB |" {
		t.Errorf("startup row = %q", got)
	}
	if got := historyLine(t, markdown, "10k notes", "Delete note"); got != "| Delete note | – | – |" {
		t.Errorf("missing scenario row = %q", got)
	}
	section := historySection(markdown)
	for _, label := range []string{"Startup heap", "Load notes", "Navigate j/k", "Save note", "Archive", "Error"} {
		if !strings.Contains(section, "| "+label+" |") {
			t.Errorf("history lacks row %q", label)
		}
	}
}

func TestHistoryKeepsProcessAndTypingRowsOnlyInTheTenThousandTable(t *testing.T) {
	run := sampleRun("v1.2.6", "def5678", 700e6)
	run.Results = append(run.Results, Result{Name: "Startup/notes=1000", Metrics: map[string]float64{"ns/op": 95e6, "B/op": 13e6}})
	markdown := Markdown(run, "")
	if got := historyLine(t, markdown, "1k notes", "Startup"); got != "| Startup | 95 ms · 12.4 MB |" {
		t.Errorf("1k startup row = %q", got)
	}
	for _, label := range []string{"Process start", "Peak memory", "Typing 10000 lines, cursor bottom"} {
		if got := historyLine(t, markdown, "1k notes", label); got != "" {
			t.Errorf("1k table should not have %q: %q", label, got)
		}
	}
	for label, want := range map[string]string{
		"Process start":                     "| Process start | 9.5 ms |",
		"Peak memory":                       "| Peak memory | 14.2 MB |",
		"Typing 10000 lines, cursor bottom": "| Typing 10000 lines, cursor bottom | 84.9 ms · 37.2 MB |",
		"Typing 500 lines, cursor top":      "| Typing 500 lines, cursor top | – |",
	} {
		if got := historyLine(t, markdown, "10k notes", label); got != want {
			t.Errorf("10k %s = %q, want %q", label, got, want)
		}
	}
}

func TestHistoryTablesRoundTrip(t *testing.T) {
	run := sampleRun("v1.2.5", "abc1234", 812e6)
	run.CPU = "Apple | M1"
	markdown := Markdown(run, "")
	tables := historyTables(markdown)
	tenThousand := tables["10k notes"]
	if len(tenThousand.columns) != 1 {
		t.Fatalf("columns = %+v", tenThousand.columns)
	}
	column := tenThousand.columns[0]
	if column.version != "v1.2.5" || column.date != "2026-10-08" || column.commit != "abc1234" || column.machine != "Apple | M1" {
		t.Errorf("column = %+v", column)
	}
	if column.cells["Startup"] != "812 ms · 87.7 MB" || column.cells["Peak memory"] != "14.2 MB" {
		t.Errorf("cells = %v", column.cells)
	}
	if again := Markdown(run, markdown); historySection(again) != historySection(markdown) {
		t.Errorf("re-rendering the same version should not change history:\n%s\nvs\n%s", historySection(again), historySection(markdown))
	}
}

func TestHistoryTablesIgnoreRowsOutsideTheMarkers(t *testing.T) {
	markdown := Markdown(sampleRun("v1.2.5", "abc1234", 812e6), "") + "\n| Startup | 1 s |\n"
	if got := historyTables(markdown)["10k notes"].columns[0].cells["Startup"]; got != "812 ms · 87.7 MB" {
		t.Errorf("startup = %q", got)
	}
	if tables := historyTables("no markers here\n| a | b |\n"); len(tables) != 0 {
		t.Errorf("tables without markers = %+v", tables)
	}
}

const legacyHistory = `## History

<!-- history:start -->
| Date | Version | Commit | Machine | Process start | Peak memory | Startup | Typing | Navigate | Save |
|---|---|---|---|---|---|---|---|---|---|
| 2026-10-08 | v1.3.0 | ` + "`30db267`" + ` | Apple M1 (Virtual) | 30.4 ms | 17.5 MB | 273 ms | 121 ms | 42.9 ms | 367 ms |
| 2026-10-07 | v1.2.5 | working tree | Apple \| M1 Pro | 18.4 ms | 18.1 MB | 978 ms | 81.3 ms | 21.8 ms | 979 ms |
<!-- history:end -->
`

func TestHistoryTablesImportTheOldFlatHistoryIntoTheTenThousandTable(t *testing.T) {
	tables := historyTables(legacyHistory)
	if len(tables["1k notes"].columns) != 0 {
		t.Errorf("1k columns = %+v", tables["1k notes"].columns)
	}
	columns := tables["10k notes"].columns
	if len(columns) != 2 {
		t.Fatalf("columns = %+v", columns)
	}
	if columns[0].version != "v1.3.0" || columns[0].commit != "30db267" || columns[0].machine != "Apple M1 (Virtual)" {
		t.Errorf("first column = %+v", columns[0])
	}
	if columns[1].version != "v1.2.5" || columns[1].commit != "" || columns[1].machine != "Apple | M1 Pro" || columns[1].date != "2026-10-07" {
		t.Errorf("second column = %+v", columns[1])
	}
	want := map[string]string{
		"Process start":                     "30.4 ms",
		"Peak memory":                       "17.5 MB",
		"Startup":                           "273 ms",
		"Typing 10000 lines, cursor bottom": "121 ms",
		"Navigate j/k":                      "42.9 ms",
		"Save note":                         "367 ms",
	}
	for label, value := range want {
		if columns[0].cells[label] != value {
			t.Errorf("%s = %q, want %q", label, columns[0].cells[label], value)
		}
	}
	markdown := Markdown(sampleRun("v1.3.1", "fff0000", 700e6), legacyHistory)
	if got := historyLine(t, markdown, "10k notes", "Save note"); got != "| Save note | – | 367 ms | 979 ms |" {
		t.Errorf("save row = %q", got)
	}
	if strings.Contains(markdown, legacyHistoryHeader) {
		t.Errorf("old header should be gone:\n%s", markdown)
	}
}

func TestHistoryReplacesTheColumnOfARecordedVersion(t *testing.T) {
	markdown := Markdown(sampleRun("v1.2.5", "abc1234", 812e6), "")
	markdown = Markdown(sampleRun("v1.2.6", "def5678", 700e6), markdown)
	markdown = Markdown(sampleRun("v1.2.5", "abc9999", 600e6), markdown)
	columns := historyTables(markdown)["10k notes"].columns
	if len(columns) != 2 || columns[0].commit != "abc9999" || columns[1].version != "v1.2.6" {
		t.Fatalf("columns = %+v", columns)
	}
	if columns[0].cells["Startup"] != "600 ms · 87.7 MB" {
		t.Errorf("startup = %q", columns[0].cells["Startup"])
	}
}

func TestHistoryAddsAColumnForEveryWorkingTreeRun(t *testing.T) {
	markdown := Markdown(sampleRun("", "", 812e6), "")
	markdown = Markdown(sampleRun("", "", 700e6), markdown)
	columns := historyTables(markdown)["10k notes"].columns
	if len(columns) != 2 || columns[0].cells["Startup"] != "700 ms · 87.7 MB" || columns[1].cells["Startup"] != "812 ms · 87.7 MB" {
		t.Fatalf("columns = %+v", columns)
	}
	if header := historyLine(t, markdown, "10k notes", "Scenario"); strings.Count(header, "working tree") < 2 {
		t.Errorf("header should label both runs working tree: %q", header)
	}
}

func TestRecordWritesTheDocsFileAndAddsEachRunAsANewColumn(t *testing.T) {
	root := t.TempDir()
	if err := Record(root, sampleRun("v1.2.5", "abc1234", 812e6)); err != nil {
		t.Fatal(err)
	}
	if err := Record(root, sampleRun("v1.2.6", "def5678", 700e6)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "docs", "benchmark", "benchmark.md"))
	if err != nil {
		t.Fatal(err)
	}
	markdown := string(data)
	columns := historyTables(markdown)["10k notes"].columns
	if len(columns) != 2 || columns[0].commit != "def5678" || columns[1].commit != "abc1234" {
		t.Fatalf("columns = %+v", columns)
	}
	latest := markdown[:strings.Index(markdown, "## History")]
	if !strings.Contains(latest, "700 ms") || strings.Contains(latest, "812 ms") {
		t.Errorf("latest tables should show only the newest run:\n%s", latest)
	}
}

func TestRecordIntoAPageWithoutHistoryStartsAFreshColumn(t *testing.T) {
	root := t.TempDir()
	pagePath := filepath.Join(root, "docs", "benchmark", "benchmark.md")
	if err := os.MkdirAll(filepath.Dir(pagePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pagePath, []byte("# Benchmarks\n\nNo release recorded yet.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Record(root, sampleRun("v1.4.1", "abc1234", 700e6)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(pagePath)
	if err != nil {
		t.Fatal(err)
	}
	columns := historyTables(string(data))["10k notes"].columns
	if len(columns) != 1 || columns[0].commit != "abc1234" {
		t.Fatalf("columns = %+v", columns)
	}
	if strings.Contains(string(data), "No release recorded yet.") {
		t.Errorf("the placeholder should be replaced by the recorded page:\n%s", data)
	}
	if !strings.Contains(string(data), "every version bump you push") {
		t.Errorf("intro should describe the pushed version bump releases:\n%s", data)
	}
}

func TestMarkdownEscapesPipes(t *testing.T) {
	run := sampleRun("v1.2.5", "abc1234", 1e6)
	run.CPU = "Apple | M1"
	markdown := Markdown(run, "")
	if strings.Contains(markdown, "Apple | M1") || !strings.Contains(markdown, `Apple \| M1`) {
		t.Errorf("pipe not escaped:\n%s", markdown)
	}
}

func TestMarkdownNamesAnUncommittedRunTheWorkingTree(t *testing.T) {
	run := sampleRun("v1.2.5", "", 1e6)
	markdown := Markdown(run, "")
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
