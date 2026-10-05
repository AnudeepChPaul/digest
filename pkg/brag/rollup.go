package brag

import (
	"fmt"
	"strings"
	"time"
)

func WeeksInMonth(month Month) []Week {
	_, end := month.Range()
	var weeks []Week
	for week := WeekOf(month.Start); week.Start.Before(end); week = WeekOf(week.Start.AddDate(0, 0, 7)) {
		if MonthOfWeek(week).ID() == month.ID() {
			weeks = append(weeks, week)
		}
	}
	return weeks
}

func demoteHeadings(markdown string, levels int) string {
	lines := strings.Split(markdown, "\n")
	for i, line := range lines {
		depth := len(line) - len(strings.TrimLeft(line, "#"))
		if depth == 0 || !strings.HasPrefix(line[depth:], " ") {
			continue
		}
		lines[i] = strings.Repeat("#", min(depth+levels, 6)) + line[depth:]
	}
	return strings.Join(lines, "\n")
}

func MonthFacts(root string, month Month) (string, error) {
	var sections []string
	for _, week := range WeeksInMonth(month) {
		if !Exists(root, week) {
			continue
		}
		entry, err := Load(root, week)
		if err != nil {
			return "", err
		}
		section := fmt.Sprintf("### Week %02d · %s", week.Number, week.Days())
		if entry.Summary != "" {
			section += "\n\n#### Summary\n\n" + demoteHeadings(entry.Summary, 2)
		}
		section += "\n\n#### Facts\n\n" + demoteHeadings(entry.Facts, 2)
		sections = append(sections, section)
	}
	if len(sections) == 0 {
		return "", fmt.Errorf("no weekly brags saved for %s", month.Label())
	}
	return strings.Join(sections, "\n\n"), nil
}

func YearFacts(root string, year Year) (string, error) {
	var sections []string
	for monthNumber := time.January; monthNumber <= time.December; monthNumber++ {
		month := MonthOf(time.Date(year.Start.Year(), monthNumber, 1, 0, 0, 0, 0, year.Start.Location()))
		if !Exists(root, month) {
			continue
		}
		entry, err := Load(root, month)
		if err != nil {
			return "", err
		}
		summary := entry.Summary
		if summary == "" {
			summary = entry.Facts
		}
		sections = append(sections, "### "+month.Label()+"\n\n"+demoteHeadings(summary, 2))
	}
	if len(sections) == 0 {
		return "", fmt.Errorf("no monthly brags saved for %s", year.ID())
	}
	return strings.Join(sections, "\n\n"), nil
}
