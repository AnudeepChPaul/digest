package benchmark

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

const (
	processStart = "Process/start"
	missingCell  = "–"
)

type tableRow struct {
	label    string
	scenario string
	unit     string
}

type tableSection struct {
	title string
	rows  []tableRow
}

var sizedSections = []tableSection{
	{"Startup and loading", []tableRow{
		{"Startup", "Startup", "ns/op"},
		{"Startup heap", "Startup", "heap-MB"},
		{"Load notes", "LoadNotes", "ns/op"},
		{"Load reviews", "LoadReviews", "ns/op"},
	}},
	{"Redraw and navigation", []tableRow{
		{"Navigate j/k", "Navigate", "ns/op"},
		{"Pulse tick", "Pulse", "ns/op"},
		{"Header", "Header", "ns/op"},
		{"Preview open", "PreviewOpen", "ns/op"},
		{"Preview next", "PreviewNext", "ns/op"},
	}},
	{"Actions", []tableRow{
		{"Quick actions open", "QuickActionsOpen", "ns/op"},
		{"Quick actions move", "QuickActionsMove", "ns/op"},
		{"Search keystroke", "Search", "ns/op"},
		{"Save note", "SaveNote", "ns/op"},
		{"Delete note", "DeleteNote", "ns/op"},
	}},
	{"Screens and overlays", []tableRow{
		{"Archive", "ScreenArchive", "ns/op"},
		{"Help", "ScreenHelp", "ns/op"},
		{"Settings", "ScreenSettings", "ns/op"},
		{"Brag view", "ScreenBrag", "ns/op"},
		{"Review details", "ScreenReviewDetails", "ns/op"},
		{"Link menu", "OverlayLinkMenu", "ns/op"},
		{"Delete confirm", "OverlayDeleteConfirm", "ns/op"},
		{"Error", "OverlayError", "ns/op"},
	}},
}

func Markdown(latest Run, previous string) string {
	var page strings.Builder
	page.WriteString(`# Benchmarks

How fast digest is, measured in CI on every version bump you push, beside the release build. Each cell is time per operation · memory allocated per operation, so lower is better. A 60 fps frame is 16.7 ms.

- The release workflow records each run here in its own ` + "`chore: benchmark`" + ` commit.
- ` + "`mise run bench`" + ` prints the same tables locally without recording them; ` + "`mise run bench:full`" + ` adds 100k notes.
- History columns from different machines aren't directly comparable.

What runs, at 1, 100, 1k and 10k notes on disk:

- **Process**: starting the binary (` + "`digest --help`" + `) and its peak memory.
- **Startup and loading**: opening the dashboard (config, notes from disk, first frame), the heap it keeps, listing notes, reading local review state.
- **Redraw and navigation**: moving with j/k, the sync pulse, the header, opening a preview and stepping through previews.
- **Actions**: quick actions, a search keystroke, saving and deleting a note (each reloads only that note).
- **Screens and overlays**: archive, help, settings, brag view, review details, link menu, delete confirm, error.
- **Typing in a long note**: a keystroke in a 500, 2k and 10k-line note with the cursor at the top and at the bottom.

`)
	fmt.Fprintf(&page, "## Latest\n\n%s %s · %s · %s\n\n", latest.Version, commitLabel(latest.Commit), latest.Date.UTC().Format("2006-01-02 15:04 UTC"), escapeCell(latest.CPU))
	writeProcessTable(&page, latest)
	writeSizedTables(&page, latest)
	writeTypingTable(&page, latest)
	writeHistory(&page, withRun(historyTables(previous), latest))
	return page.String()
}

func writeProcessTable(page *strings.Builder, run Run) {
	page.WriteString("### Process\n\n| Process start (`digest --help`) | Peak memory |\n|---|---|\n")
	fmt.Fprintf(page, "| %s | %s |\n\n", durationCell(run.metric(processStart, "ns/op")), megabytesCell(run.metric(processStart, "rss-MB")))
}

func sizeKeys(run Run) []string {
	var sizes []int
	for _, result := range run.Results {
		_, parameter, found := strings.Cut(result.Name, "/")
		if !found || strings.Contains(parameter, "/") {
			continue
		}
		_, value, found := strings.Cut(parameter, "=")
		size, err := strconv.Atoi(value)
		if found && err == nil && !slices.Contains(sizes, size) {
			sizes = append(sizes, size)
		}
	}
	slices.Sort(sizes)
	keys := make([]string, len(sizes))
	for index, size := range sizes {
		keys[index] = strconv.Itoa(size)
	}
	return keys
}

func sizedResult(run Run, scenario, size string) (Result, bool) {
	for _, result := range run.Results {
		name, parameter, found := strings.Cut(result.Name, "/")
		if !found || name != scenario {
			continue
		}
		if _, value, found := strings.Cut(parameter, "="); found && value == size {
			return result, true
		}
	}
	return Result{}, false
}

func writeSizedTables(page *strings.Builder, run Run) {
	sizes := sizeKeys(run)
	if len(sizes) == 0 {
		return
	}
	for _, section := range sizedSections {
		fmt.Fprintf(page, "### %s\n\nColumns are notes on disk (reviews for Load reviews, at most 1000).\n\n| Scenario |", section.title)
		for _, size := range sizes {
			fmt.Fprintf(page, " %s |", size)
		}
		page.WriteString("\n|---|" + strings.Repeat("---|", len(sizes)) + "\n")
		for _, row := range section.rows {
			fmt.Fprintf(page, "| %s |", row.label)
			for _, size := range sizes {
				result, found := sizedResult(run, row.scenario, size)
				switch {
				case !found:
					page.WriteString(" " + missingCell + " |")
				case row.unit == "heap-MB":
					page.WriteString(" " + megabytesCell(result.Metrics["heap-MB"]) + " |")
				default:
					page.WriteString(" " + resultCell(result) + " |")
				}
			}
			page.WriteString("\n")
		}
		page.WriteString("\n")
	}
}

func writeTypingTable(page *strings.Builder, run Run) {
	var lineCounts []int
	for _, result := range run.Results {
		var lines int
		var cursor string
		if _, err := fmt.Sscanf(result.Name, "Typing/lines=%d/cursor=%s", &lines, &cursor); err == nil && !slices.Contains(lineCounts, lines) {
			lineCounts = append(lineCounts, lines)
		}
	}
	if len(lineCounts) == 0 {
		return
	}
	slices.Sort(lineCounts)
	page.WriteString("### Typing in a long note\n\nOne keystroke plus redraw.\n\n| Note length | Cursor at top | Cursor at bottom |\n|---|---|---|\n")
	for _, lines := range lineCounts {
		fmt.Fprintf(page, "| %d lines |", lines)
		for _, cursor := range []string{"top", "bottom"} {
			page.WriteString(" " + namedCell(run, fmt.Sprintf("Typing/lines=%d/cursor=%s", lines, cursor)) + " |")
		}
		page.WriteString("\n")
	}
	page.WriteString("\n")
}

func namedCell(run Run, name string) string {
	for _, result := range run.Results {
		if result.Name == name {
			return resultCell(result)
		}
	}
	return missingCell
}

func resultCell(result Result) string {
	nanos, hasTime := result.Metrics["ns/op"]
	if !hasTime {
		return missingCell
	}
	if bytes, hasBytes := result.Metrics["B/op"]; hasBytes {
		return formatDuration(nanos) + " · " + formatBytes(bytes)
	}
	return formatDuration(nanos)
}

func durationCell(nanos float64) string {
	if nanos == 0 {
		return missingCell
	}
	return formatDuration(nanos)
}

func megabytesCell(megabytes float64) string {
	if megabytes == 0 {
		return missingCell
	}
	return strconv.FormatFloat(megabytes, 'f', 1, 64) + " MB"
}

func commitLabel(commit string) string {
	if commit == "" {
		return "working tree"
	}
	return "`" + commit + "`"
}

func escapeCell(text string) string {
	return strings.ReplaceAll(text, "|", `\|`)
}

func trimmedNumber(value float64) string {
	decimals := 0
	switch {
	case value < 10:
		decimals = 2
	case value < 100:
		decimals = 1
	}
	formatted := strconv.FormatFloat(value, 'f', decimals, 64)
	if strings.Contains(formatted, ".") {
		formatted = strings.TrimRight(strings.TrimRight(formatted, "0"), ".")
	}
	return formatted
}

func formatDuration(nanos float64) string {
	switch {
	case nanos < 1e3:
		return trimmedNumber(nanos) + " ns"
	case nanos < 1e6:
		return trimmedNumber(nanos/1e3) + " µs"
	case nanos < 1e9:
		return trimmedNumber(nanos/1e6) + " ms"
	default:
		return trimmedNumber(nanos/1e9) + " s"
	}
}

func formatBytes(bytes float64) string {
	switch {
	case bytes < 1<<10:
		return strconv.FormatFloat(bytes, 'f', 0, 64) + " B"
	case bytes < 1<<20:
		return strconv.FormatFloat(bytes/(1<<10), 'f', 0, 64) + " KB"
	default:
		return strconv.FormatFloat(bytes/(1<<20), 'f', 1, 64) + " MB"
	}
}
