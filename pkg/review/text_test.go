package review

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const hostileText = "fix \x1b]52;c;ZXZpbA==\x07login \x1b[31mred\x1b[0m \x1b]8;;https://evil\x1b\\link\x1b]8;;\x1b\\ \u009b2J\x00bell\x07 tab\tok\r\nnext line ✅"

func TestPlainTextRemovesTerminalControlSequences(t *testing.T) {
	got := PlainText(hostileText)
	if want := "fix login red link 2Jbell tab\tok\nnext line ✅"; got != want {
		t.Errorf("PlainText = %q, want %q", got, want)
	}
	if got := PlainText("npm progress\r[####]\rdone\n"); strings.Contains(got, "\r") {
		t.Errorf("carriage returns should become line breaks: %q", got)
	}
}

func TestParsePRDetailsStripsControlSequences(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"number": 5, "title": hostileText, "body": hostileText, "url": "https://github.com/o/console/pull/5",
		"headRefName": "feat/\x1b[2Jx", "author": map[string]any{"login": "a"},
	})
	pr, err := ParsePRDetails(raw, "me")
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{"title": pr.Title, "body": pr.Body, "head": pr.HeadRef} {
		if strings.ContainsAny(value, "\x1b\x07\u009b\x00") {
			t.Errorf("%s keeps control sequences: %q", name, value)
		}
	}
}

func TestLoadStripsControlSequencesFromFindings(t *testing.T) {
	dir := t.TempDir()
	report, _ := json.Marshal(Report{Recommendation: hostileText, Findings: []Finding{{Title: hostileText, Body: hostileText, Suggestion: hostileText, Path: "a\x1b[2J.go"}}})
	if err := os.WriteFile(filepath.Join(dir, FindingsFile), report, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	finding := loaded.Findings[0]
	for _, value := range []string{loaded.Recommendation, finding.Title, finding.Body, finding.Suggestion, finding.Path} {
		if strings.ContainsAny(value, "\x1b\x07\u009b\x00") {
			t.Errorf("finding keeps control sequences: %q", value)
		}
	}
}
