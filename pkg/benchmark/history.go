package benchmark

import (
	"fmt"
	"slices"
	"strings"
)

const (
	legacyHistoryHeader = "| Date | Version | Commit | Machine | Process start | Peak memory | Startup | Typing | Navigate | Save |"
	historyScenarioCell = "Scenario"
	workingTreeLabel    = "working tree"
	headerLineBreak     = "<br>"
	tenThousandTable    = "10k notes"
)

type historyColumn struct {
	version string
	date    string
	commit  string
	machine string
	cells   map[string]string
}

type historyTable struct {
	columns []historyColumn
}

type historyLayout struct {
	title string
	size  string
}

var historyLayouts = []historyLayout{{"1k notes", "1000"}, {tenThousandTable, "10000"}}

var processHistoryRows = []string{"Process start", "Peak memory"}

var typingLineCounts = []int{500, 2000, 10000}

func typingHistoryLabel(lines int, cursor string) string {
	return fmt.Sprintf("Typing %d lines, cursor %s", lines, cursor)
}

func historyRowLabels(title string) []string {
	var labels []string
	if title == tenThousandTable {
		labels = append(labels, processHistoryRows...)
	}
	for _, section := range sizedSections {
		for _, row := range section.rows {
			labels = append(labels, row.label)
		}
	}
	if title == tenThousandTable {
		for _, lines := range typingLineCounts {
			for _, cursor := range []string{"top", "bottom"} {
				labels = append(labels, typingHistoryLabel(lines, cursor))
			}
		}
	}
	return labels
}

func runColumn(run Run, layout historyLayout) (historyColumn, bool) {
	cells := map[string]string{}
	for _, section := range sizedSections {
		for _, row := range section.rows {
			result, found := sizedResult(run, row.scenario, layout.size)
			switch {
			case !found:
				cells[row.label] = missingCell
			case row.unit == "heap-MB":
				cells[row.label] = megabytesCell(result.Metrics["heap-MB"])
			default:
				cells[row.label] = resultCell(result)
			}
		}
	}
	if layout.title == tenThousandTable {
		cells[processHistoryRows[0]] = durationCell(run.metric(processStart, "ns/op"))
		cells[processHistoryRows[1]] = megabytesCell(run.metric(processStart, "rss-MB"))
		for _, lines := range typingLineCounts {
			for _, cursor := range []string{"top", "bottom"} {
				cells[typingHistoryLabel(lines, cursor)] = namedCell(run, fmt.Sprintf("Typing/lines=%d/cursor=%s", lines, cursor))
			}
		}
	}
	measured := false
	for _, cell := range cells {
		measured = measured || cell != missingCell
	}
	column := historyColumn{
		version: run.Version,
		date:    run.Date.UTC().Format("2006-01-02"),
		commit:  run.Commit,
		machine: run.CPU,
		cells:   cells,
	}
	return column, measured
}

func withRun(tables map[string]historyTable, run Run) map[string]historyTable {
	updated := map[string]historyTable{}
	for _, layout := range historyLayouts {
		columns := tables[layout.title].columns
		if column, measured := runColumn(run, layout); measured {
			if run.Version != "" {
				columns = slices.DeleteFunc(slices.Clone(columns), func(existing historyColumn) bool {
					return existing.version == run.Version
				})
			}
			columns = append([]historyColumn{column}, columns...)
		}
		updated[layout.title] = historyTable{columns: columns}
	}
	return updated
}

func historyBody(markdown string) (string, bool) {
	_, afterStart, found := strings.Cut(markdown, historyStart)
	if !found {
		return "", false
	}
	body, _, found := strings.Cut(afterStart, historyEnd)
	return body, found
}

func historyTables(markdown string) map[string]historyTable {
	tables := map[string]historyTable{}
	body, found := historyBody(markdown)
	if !found {
		return tables
	}
	lines := strings.Split(body, "\n")
	if slices.Contains(lines, legacyHistoryHeader) {
		tables[tenThousandTable] = legacyHistoryTable(lines)
		return tables
	}
	var title string
	for _, line := range lines {
		if heading, isHeading := strings.CutPrefix(line, "### "); isHeading {
			title = strings.TrimSpace(heading)
			continue
		}
		if !strings.HasPrefix(line, "| ") || title == "" {
			continue
		}
		cells := tableCells(line)
		table := tables[title]
		if cells[0] == historyScenarioCell {
			for _, header := range cells[1:] {
				table.columns = append(table.columns, headerColumn(header))
			}
		} else {
			for index, cell := range cells[1:] {
				if index < len(table.columns) {
					table.columns[index].cells[cells[0]] = cell
				}
			}
		}
		tables[title] = table
	}
	return tables
}

func legacyHistoryTable(lines []string) historyTable {
	var table historyTable
	for _, line := range lines {
		if !strings.HasPrefix(line, "| ") || line == legacyHistoryHeader {
			continue
		}
		cells := tableCells(line)
		if len(cells) != 10 {
			continue
		}
		column := historyColumn{
			version: cells[1],
			date:    cells[0],
			commit:  parsedCommit(cells[2]),
			machine: cells[3],
			cells: map[string]string{
				processHistoryRows[0]:               cells[4],
				processHistoryRows[1]:               cells[5],
				"Startup":                           cells[6],
				typingHistoryLabel(10000, "bottom"): cells[7],
				"Navigate j/k":                      cells[8],
				"Save note":                         cells[9],
			},
		}
		if column.version == workingTreeLabel {
			column.version = ""
		}
		table.columns = append(table.columns, column)
	}
	return table
}

func tableCells(line string) []string {
	var cells []string
	var cell strings.Builder
	trimmed := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(line), "|"), "|")
	for index := 0; index < len(trimmed); index++ {
		switch {
		case trimmed[index] == '\\' && index+1 < len(trimmed) && trimmed[index+1] == '|':
			cell.WriteByte('|')
			index++
		case trimmed[index] == '|':
			cells = append(cells, strings.TrimSpace(cell.String()))
			cell.Reset()
		default:
			cell.WriteByte(trimmed[index])
		}
	}
	return append(cells, strings.TrimSpace(cell.String()))
}

func parsedCommit(label string) string {
	if label == workingTreeLabel || label == missingCell {
		return ""
	}
	return strings.Trim(label, "`")
}

func headerColumn(header string) historyColumn {
	parts := strings.Split(header, headerLineBreak)
	for len(parts) < 4 {
		parts = append(parts, "")
	}
	column := historyColumn{version: parts[0], date: parts[1], commit: parsedCommit(parts[2]), machine: parts[3], cells: map[string]string{}}
	if column.version == workingTreeLabel {
		column.version = ""
	}
	return column
}

func columnHeader(column historyColumn) string {
	label := column.version
	if label == "" {
		label = workingTreeLabel
	}
	return strings.Join([]string{escapeCell(label), column.date, commitLabel(column.commit), escapeCell(column.machine)}, headerLineBreak)
}

func writeHistory(page *strings.Builder, tables map[string]historyTable) {
	page.WriteString("## History\n\nOne column per run, newest first; re-recording a version replaces its column. Rows are every scenario at 1k and at 10k notes; the 10k table adds process start, peak memory and typing in a long note.\n\n")
	page.WriteString(historyStart + "\n")
	for _, layout := range historyLayouts {
		columns := tables[layout.title].columns
		if len(columns) == 0 {
			continue
		}
		fmt.Fprintf(page, "### %s\n\n| %s |", layout.title, historyScenarioCell)
		for _, column := range columns {
			page.WriteString(" " + columnHeader(column) + " |")
		}
		page.WriteString("\n|---|" + strings.Repeat("---|", len(columns)) + "\n")
		for _, label := range historyRowLabels(layout.title) {
			fmt.Fprintf(page, "| %s |", label)
			for _, column := range columns {
				cell, found := column.cells[label]
				if !found || cell == "" {
					cell = missingCell
				}
				page.WriteString(" " + escapeCell(cell) + " |")
			}
			page.WriteString("\n")
		}
		page.WriteString("\n")
	}
	page.WriteString(historyEnd + "\n")
}
