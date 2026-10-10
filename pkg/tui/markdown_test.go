package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func renderedLines(body string, width int) []string {
	return strings.Split(stripANSI(renderMarkdown(body, width)), "\n")
}

func TestMarkdownRendersBlocks(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       []string
	}{
		{"heading", "# Title\n\nBody text", []string{"Title", "", "Body text"}},
		{"sub heading", "### Deep", []string{"Deep"}},
		{"bullets", "- one\n* two", []string{"• one", "• two"}},
		{"nested bullet", "- one\n  - inner", []string{"• one", "  • inner"}},
		{"numbered", "1. first\n2. second", []string{"1. first", "2. second"}},
		{"task", "- [x] done\n- [ ] todo", []string{"✔ done", "☐ todo"}},
		{"quote", "> quoted", []string{"│ quoted"}},
		{"rule", "---", []string{strings.Repeat("─", 10)}},
		{"fence", "```go\nfunc main() {}\n```", []string{"  func main() {}"}},
		{"blank runs collapse", "a\n\n\n\nb", []string{"a", "", "b"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderedLines(tc.body, 10); strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
				t.Errorf("rendered = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMarkdownRendersInlineMarks(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{"a **bold** word", "a bold word"},
		{"a __bold__ word", "a bold word"},
		{"an *italic* word", "an italic word"},
		{"run `make test` now", "run make test now"},
		{"see [docs](https://x.io/d)", "see docs (https://x.io/d)"},
		{"[https://x.io](https://x.io)", "https://x.io"},
		{"keep `**raw**` inside code", "keep **raw** inside code"},
		{"snake_case_name stays", "snake_case_name stays"},
		{"a lone * star", "a lone * star"},
	} {
		if got := stripANSI(renderMarkdown(tc.body, 80)); got != tc.want {
			t.Errorf("render(%q) = %q, want %q", tc.body, got, tc.want)
		}
	}
}

func TestMarkdownWrapsToWidth(t *testing.T) {
	body := "# A heading that is long enough to wrap\n\nSome paragraph text that goes on for a while and must wrap.\n\n- a bullet item that is also long enough to wrap around\n\n> a quote that is long enough to wrap around too"
	for _, line := range renderedLines(body, 20) {
		if width := ansi.StringWidth(line); width > 20 {
			t.Errorf("line %q is %d wide", line, width)
		}
	}
	lines := renderedLines("- a bullet item that wraps around", 14)
	if len(lines) < 2 || !strings.HasPrefix(lines[1], "  ") {
		t.Errorf("bullet continuation should hang under the text: %q", lines)
	}
	lines = renderedLines("> a quote that wraps around", 12)
	if len(lines) < 2 || !strings.HasPrefix(lines[1], "│ ") {
		t.Errorf("every quote line keeps the bar: %q", lines)
	}
}

func TestMarkdownCodeFencesAreNotWrappedOrStyled(t *testing.T) {
	code := "a very long code line that should not wrap at all"
	lines := renderedLines("```\n"+code+"\n# not a heading\n```", 10)
	if len(lines) != 2 || lines[0] != "  "+code || lines[1] != "  # not a heading" {
		t.Errorf("fenced lines = %q", lines)
	}
	if got := renderedLines("```\nunclosed", 40); len(got) != 1 || got[0] != "  unclosed" {
		t.Errorf("an unclosed fence runs to the end: %q", got)
	}
}
