package review

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/charmbracelet/log"
)

func TestRunWritesFindingsAndNotifies(t *testing.T) {
	root := t.TempDir()
	ref := PRRef{Host: "github.com", Owner: "o", Repo: "console", Number: 4, URL: "https://github.com/o/console/pull/4"}

	originalPrepare, originalNotify := prepareClone, sendNotification
	prepareClone = func(ctx context.Context, ref PRRef, root string, logger *log.Logger, output io.Writer) (string, error) {
		dir := CloneDir(root, ref)
		return dir, os.MkdirAll(dir, 0755)
	}
	var notified []string
	sendNotification = func(title, message, openURL string) error {
		notified = append(notified, title)
		return nil
	}
	defer func() { prepareClone, sendNotification = originalPrepare, originalNotify }()

	t.Setenv("GH_TOKEN", "secret-gh")
	template := `test -z "$GH_TOKEN" && test "$(pwd -P)" = "$(cd {clone} && pwd -P)" && echo {url} && printf '{"recommendation":"COMMENT","findings":[{"severity":"high","title":"t"}]}'`
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
	prepareClone = func(ctx context.Context, ref PRRef, root string, logger *log.Logger, output io.Writer) (string, error) {
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

func TestReviewCommandStopsAtItsTimeout(t *testing.T) {
	root := t.TempDir()
	ref := PRRef{Repo: "console", Number: 6, URL: "u"}
	originalPrepare, originalNotify, originalTimeout := prepareClone, sendNotification, reviewCommandTimeout
	prepareClone = func(ctx context.Context, ref PRRef, root string, logger *log.Logger, output io.Writer) (string, error) {
		return root, nil
	}
	sendNotification = func(title, message, openURL string) error { return nil }
	reviewCommandTimeout = 200 * time.Millisecond
	defer func() {
		prepareClone, sendNotification, reviewCommandTimeout = originalPrepare, originalNotify, originalTimeout
	}()
	started := time.Now()
	err := Run(context.Background(), ref, root, "sleep 30", log.New(io.Discard))
	if err == nil || time.Since(started) > 10*time.Second {
		t.Errorf("err = %v after %v", err, time.Since(started))
	}
}

func TestCancelledRunStopsTheReviewCommand(t *testing.T) {
	root := t.TempDir()
	ref := PRRef{Repo: "console", Number: 7, URL: "u"}
	originalPrepare, originalNotify := prepareClone, sendNotification
	prepareClone = func(ctx context.Context, ref PRRef, root string, logger *log.Logger, output io.Writer) (string, error) {
		return root, nil
	}
	sendNotification = func(title, message, openURL string) error { return nil }
	defer func() { prepareClone, sendNotification = originalPrepare, originalNotify }()
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(200*time.Millisecond, cancel)
	started := time.Now()
	if err := Run(ctx, ref, root, "sleep 30", log.New(io.Discard)); err == nil || time.Since(started) > 10*time.Second {
		t.Errorf("err = %v after %v", err, time.Since(started))
	}
}

func TestCloneAndInstallStopAtTheirTimeout(t *testing.T) {
	root := t.TempDir()
	ref := PRRef{Repo: "console", Number: 8, URL: "u"}
	originalPrepare, originalNotify, originalTimeout := prepareClone, sendNotification, cloneTimeout
	prepareClone = func(ctx context.Context, ref PRRef, root string, logger *log.Logger, output io.Writer) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}
	sendNotification = func(title, message, openURL string) error { return nil }
	cloneTimeout = 200 * time.Millisecond
	defer func() {
		prepareClone, sendNotification, cloneTimeout = originalPrepare, originalNotify, originalTimeout
	}()
	started := time.Now()
	err := Run(context.Background(), ref, root, "true", log.New(io.Discard))
	if err == nil || time.Since(started) > 10*time.Second {
		t.Errorf("a hung clone should fail at its timeout: err = %v after %v", err, time.Since(started))
	}
}

func TestCancelledStepsKillTheirChildProcesses(t *testing.T) {
	for name, start := range map[string]func(ctx context.Context, dir, script string) error{
		"install": func(ctx context.Context, dir, script string) error { return runInstall(ctx, io.Discard, dir, script) },
		"step": func(ctx context.Context, dir, script string) error {
			return runStep(ctx, io.Discard, dir, "bash", "-c", script)
		},
	} {
		dir := t.TempDir()
		pidPath := filepath.Join(dir, "child.pid")
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- start(ctx, dir, "sleep 30 & echo $! > child.pid; wait") }()
		var childPID int
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
			if pid, ok := readInt(pidPath); ok {
				childPID = pid
				break
			}
		}
		if childPID == 0 {
			t.Fatalf("%s: child never started", name)
		}
		cancel()
		<-done
		for deadline := time.Now().Add(3 * time.Second); processAlive(childPID) && time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		}
		if processAlive(childPID) {
			_ = syscall.Kill(childPID, syscall.SIGKILL)
			t.Errorf("%s: cancelling left child process %d running", name, childPID)
		}
	}
}

func stubReviewSideEffects(t *testing.T) {
	t.Helper()
	originalPrepare, originalNotify := prepareClone, sendNotification
	prepareClone = func(ctx context.Context, ref PRRef, root string, logger *log.Logger, output io.Writer) (string, error) {
		return root, nil
	}
	sendNotification = func(title, message, openURL string) error { return nil }
	t.Cleanup(func() { prepareClone, sendNotification = originalPrepare, originalNotify })
}

func TestRunSavesTheFindingsJSONPrintedOnStdout(t *testing.T) {
	stubReviewSideEffects(t)
	root := t.TempDir()
	ref := PRRef{Repo: "console", Number: 12, URL: "u"}
	var streamed strings.Builder
	template := `printf '\n  {"recommendation":"APPROVE","findings":[{"severity":"low","title":"nit"}]}\n'`
	if err := run(context.Background(), ref, root, template, log.New(io.Discard), &streamed, io.Discard); err != nil {
		t.Fatal(err)
	}
	report, err := Load(StateDir(root, ref))
	if err != nil {
		t.Fatal(err)
	}
	if report.Recommendation != "APPROVE" || len(report.Findings) != 1 || report.Findings[0].Title != "nit" {
		t.Errorf("report = %+v", report)
	}
	if !strings.Contains(streamed.String(), `"recommendation":"APPROVE"`) {
		t.Errorf("stdout should still stream to the log, got %q", streamed.String())
	}
}

func TestRunTakesTheOutermostObjectFromChattyStdout(t *testing.T) {
	stubReviewSideEffects(t)
	root := t.TempDir()
	ref := PRRef{Repo: "console", Number: 13, URL: "u"}
	template := `echo 'Here are the findings:'; printf '{"recommendation":"COMMENT","findings":[{"severity":"high","title":"t","body":"{x}"}]}'; echo ' done'`
	if err := run(context.Background(), ref, root, template, log.New(io.Discard), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	report, err := Load(StateDir(root, ref))
	if err != nil || len(report.Findings) != 1 || report.Findings[0].Body != "{x}" {
		t.Errorf("report = %+v err = %v", report, err)
	}
}

func TestRunRejectsStdoutWithoutFindingsJSON(t *testing.T) {
	stubReviewSideEffects(t)
	root := t.TempDir()
	ref := PRRef{Repo: "console", Number: 14, URL: "u"}
	err := run(context.Background(), ref, root, "echo 'review complete, no json here {oops'", log.New(io.Discard), io.Discard, io.Discard)
	if !errors.Is(err, ErrNoFindingsJSON) || err.Error() != "review produced no findings JSON" {
		t.Errorf("err = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(StateDir(root, ref), FindingsFile)); statErr == nil {
		t.Errorf("findings should not exist")
	}
	if Status(StateDir(root, ref)) != RunFailed {
		t.Errorf("status = %v, want failed", Status(StateDir(root, ref)))
	}
}

func TestReviewCommandDropsTheLegacyFindingsTarget(t *testing.T) {
	cases := map[string]string{
		"claude -p '/review-toolkit:review {url} --emit findings-json --emit-to {findings}' --strict-mcp-config": "claude -p '/review-toolkit:review https://github.com/o/r/pull/1 --emit findings-json' --strict-mcp-config",
		"claude -p '/review {url} --emit-to={findings} --emit findings-json'":                                    "claude -p '/review https://github.com/o/r/pull/1 --emit findings-json'",
		"claude -p '/review {url}' {findings}":                                                                   "claude -p '/review https://github.com/o/r/pull/1' ",
	}
	for template, want := range cases {
		command, err := expandReviewCommand(template, map[string]string{"url": "https://github.com/o/r/pull/1"})
		if err != nil || command != want {
			t.Errorf("%q:\n got %q, %v\nwant %q", template, command, err, want)
		}
	}
}

func TestRunWithLegacyTemplateStillSavesStdoutFindings(t *testing.T) {
	stubReviewSideEffects(t)
	root := t.TempDir()
	ref := PRRef{Repo: "console", Number: 15, URL: "u"}
	template := `test -z "$(echo --emit-to {findings})" && printf '{"recommendation":"COMMENT","findings":[]}'`
	if err := run(context.Background(), ref, root, template, log.New(io.Discard), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if report, err := Load(StateDir(root, ref)); err != nil || report.Recommendation != "COMMENT" {
		t.Errorf("report = %+v err = %v", report, err)
	}
}
