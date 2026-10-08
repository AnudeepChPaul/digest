package review

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Finding struct {
	Severity   string   `json:"severity"`
	Category   string   `json:"category"`
	Title      string   `json:"title"`
	Path       string   `json:"path"`
	Line       int      `json:"line"`
	Body       string   `json:"body"`
	Suggestion string   `json:"suggestion"`
	Confidence string   `json:"confidence"`
	FoundBy    []string `json:"found_by"`
}

type Report struct {
	Recommendation string    `json:"recommendation"`
	ToolVersion    string    `json:"tool_version"`
	Findings       []Finding `json:"findings"`
}

type SeverityGroup struct {
	Severity string
	Findings []Finding
}

var severityOrder = []string{"critical", "high", "medium", "low"}

func Load(dir string) (*Report, error) {
	data, err := os.ReadFile(filepath.Join(dir, FindingsFile))
	if err != nil {
		return nil, err
	}
	var report Report
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("decode %s: %w", FindingsFile, err)
	}
	plainTextFields(&report)
	return &report, nil
}

func GroupBySeverity(findings []Finding) []SeverityGroup {
	bySeverity := make(map[string][]Finding)
	var extra []string
	for _, finding := range findings {
		if _, seen := bySeverity[finding.Severity]; !seen && !isKnownSeverity(finding.Severity) {
			extra = append(extra, finding.Severity)
		}
		bySeverity[finding.Severity] = append(bySeverity[finding.Severity], finding)
	}
	var groups []SeverityGroup
	for _, severity := range append(append([]string(nil), severityOrder...), extra...) {
		if items := bySeverity[severity]; len(items) > 0 {
			groups = append(groups, SeverityGroup{Severity: severity, Findings: items})
		}
	}
	return groups
}

func isKnownSeverity(severity string) bool {
	for _, known := range severityOrder {
		if known == severity {
			return true
		}
	}
	return false
}

func Flatten(groups []SeverityGroup) []Finding {
	var flat []Finding
	for _, group := range groups {
		flat = append(flat, group.Findings...)
	}
	return flat
}
