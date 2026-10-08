package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AnudeepChPaul/digest/pkg/brag"
	"github.com/AnudeepChPaul/digest/pkg/review"
)

func writeHugeLog(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	content := "FIRSTLINE\n" + strings.Repeat("filler line of log output\n", 40000) + "LASTLINE\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestRunPreviewsReadOnlyTheLogTail(t *testing.T) {
	root := t.TempDir()
	ref := prRef("console", 7)
	writeHugeLog(t, filepath.Join(review.StateDir(root, ref), review.LogFile))
	reviewPreview := reviewRunPreview(root, review.ReviewRun{Meta: review.Meta{Ref: ref}, Status: review.RunRunning})
	writeHugeLog(t, filepath.Join(brag.StateDir(root, "2026-W40"), brag.RunLogFile))
	bragPreview := bragRunPreview(root, brag.Run{Meta: brag.RunMeta{ID: "2026-W40"}, Status: brag.RunRunning})
	for name, parts := range map[string]runPreview{"review": reviewPreview, "brag": bragPreview} {
		preview := parts.heading + "\n" + parts.log
		if !strings.Contains(preview, "LASTLINE") || strings.Contains(preview, "FIRSTLINE") {
			t.Errorf("%s preview should show only the end of a large log", name)
		}
		if len(preview) > int(jobLogTailBytes)+1024 {
			t.Errorf("%s preview holds %d bytes", name, len(preview))
		}
	}
}

func TestRunLogsSkipTheMarkdownRenderer(t *testing.T) {
	m, running, _ := reviewRunsModel(t)
	logPath := filepath.Join(review.StateDir(m.reviewRoot(), running), review.LogFile)
	if err := os.WriteFile(logPath, []byte("npm warn deprecated thing\nUNIQUE-LOG-LINE\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var markdownInputs []string
	original := renderMarkdown
	renderMarkdown = func(body string, width int) string {
		markdownInputs = append(markdownInputs, body)
		return original(body, width)
	}
	t.Cleanup(func() { renderMarkdown = original })
	selectNavItem(t, &m, "review:"+running.URL)
	m.mode = ViewPreview
	m.updatePreviewViewport()
	if !strings.Contains(m.previewViewport.View(), "UNIQUE-LOG-LINE") {
		t.Fatalf("log missing from the preview:\n%s", m.previewViewport.View())
	}
	for _, input := range markdownInputs {
		if strings.Contains(input, "UNIQUE-LOG-LINE") {
			t.Errorf("the run log went through the markdown renderer")
		}
	}
}

func TestLogTailsDropTerminalControlSequences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review.log")
	if err := os.WriteFile(path, []byte("install \x1b]52;c;ZXZpbA==\x07ok\n\x1b[2Jcleared\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := readFileTail(path, jobLogTailBytes); got != "install ok\ncleared\n" {
		t.Errorf("log tail = %q", got)
	}
}
