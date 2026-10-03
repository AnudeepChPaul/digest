package review

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/log"
)

func TestRunWritesFindingsAndNotifies(t *testing.T) {
	root := t.TempDir()
	ref := PRRef{Host: "github.com", Owner: "o", Repo: "console", Number: 4, URL: "https://github.com/o/console/pull/4"}

	originalPrepare, originalNotify := prepareClone, sendNotification
	prepareClone = func(ctx context.Context, ref PRRef, root string, logger *log.Logger) (string, error) {
		dir := CloneDir(root, ref)
		return dir, os.MkdirAll(dir, 0755)
	}
	var notified []string
	sendNotification = func(title, message, openURL string) error {
		notified = append(notified, title)
		return nil
	}
	defer func() { prepareClone, sendNotification = originalPrepare, originalNotify }()

	template := `test "$(pwd -P)" = "$(cd {clone} && pwd -P)" && printf '{"recommendation":"COMMENT","findings":[{"severity":"high","title":"t"}]}' > {findings} && echo {url}`
	if err := Run(context.Background(), ref, root, template, log.New(os.Stderr)); err != nil {
		t.Fatal(err)
	}
	report, err := Load(StateDir(root, ref))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Findings) != 1 {
		t.Errorf("findings = %+v", report.Findings)
	}
	if len(notified) != 1 || notified[0] != "PR reviewed" {
		t.Errorf("notified = %v", notified)
	}
	if meta, err := ReadMeta(StateDir(root, ref)); err != nil || meta.Ref.URL != ref.URL {
		t.Errorf("meta = %+v err = %v", meta, err)
	}
}

func TestRunFailsWithoutFindings(t *testing.T) {
	root := t.TempDir()
	ref := PRRef{Repo: "console", Number: 5, URL: "u"}
	originalPrepare, originalNotify := prepareClone, sendNotification
	prepareClone = func(ctx context.Context, ref PRRef, root string, logger *log.Logger) (string, error) {
		return root, nil
	}
	var notified []string
	sendNotification = func(title, message, openURL string) error {
		notified = append(notified, title)
		return nil
	}
	defer func() { prepareClone, sendNotification = originalPrepare, originalNotify }()

	if err := Run(context.Background(), ref, root, "true", log.New(os.Stderr)); err == nil {
		t.Errorf("expected error when findings are missing")
	}
	if _, err := os.Stat(filepath.Join(StateDir(root, ref), FindingsFile)); err == nil {
		t.Errorf("findings should not exist")
	}
	if len(notified) != 1 || notified[0] != "PR review failed" {
		t.Errorf("notified = %v", notified)
	}
}
