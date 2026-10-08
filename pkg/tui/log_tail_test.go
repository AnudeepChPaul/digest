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
	for name, preview := range map[string]string{"review": reviewPreview, "brag": bragPreview} {
		if !strings.Contains(preview, "LASTLINE") || strings.Contains(preview, "FIRSTLINE") {
			t.Errorf("%s preview should show only the end of a large log", name)
		}
		if len(preview) > int(jobLogTailBytes)+1024 {
			t.Errorf("%s preview holds %d bytes", name, len(preview))
		}
	}
}
