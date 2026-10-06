package review

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestDirs(t *testing.T) {
	ref := PRRef{Repo: "console", Number: 5}
	if CloneDir("/r", ref) != "/r/console_5" {
		t.Errorf("CloneDir = %q", CloneDir("/r", ref))
	}
	if StateDir("/r", ref) != "/r/.state/console_5" {
		t.Errorf("StateDir = %q", StateDir("/r", ref))
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestStatus(t *testing.T) {
	dir := t.TempDir()
	if Status(dir) != RunIdle {
		t.Errorf("empty dir should be idle")
	}
	writeFile(t, filepath.Join(dir, pidFile), strconv.Itoa(os.Getpid()))
	if Status(dir) != RunRunning {
		t.Errorf("live pid should be running")
	}
	writeFile(t, filepath.Join(dir, pidFile), "999999")
	writeFile(t, filepath.Join(dir, exitFile), "1")
	if Status(dir) != RunFailed {
		t.Errorf("exit 1 should be failed")
	}
	writeFile(t, filepath.Join(dir, exitFile), "0")
	if Status(dir) != RunFailed {
		t.Errorf("exit 0 without findings should be failed")
	}
	writeFile(t, filepath.Join(dir, FindingsFile), `{"findings":[]}`)
	if Status(dir) != RunDone {
		t.Errorf("exit 0 with findings should be done")
	}
}

func TestStatusDoneWithoutExitFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, FindingsFile), `{"findings":[]}`)
	if Status(dir) != RunDone {
		t.Errorf("findings from a direct CLI run should count as done")
	}
}

func TestMetaRoundTrip(t *testing.T) {
	dir := t.TempDir()
	meta := Meta{Ref: PRRef{Repo: "console", Number: 3, URL: "u"}, HeadSHA: "abc"}
	if err := WriteMeta(dir, meta); err != nil {
		t.Fatal(err)
	}
	got, err := ReadMeta(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Ref != meta.Ref || got.HeadSHA != "abc" {
		t.Errorf("got %+v", got)
	}
}

func seedRun(t *testing.T, root string, ref PRRef, files map[string]string, startedAt time.Time) {
	t.Helper()
	dir := StateDir(root, ref)
	if err := WriteMeta(dir, Meta{Ref: ref, Title: ref.Repo}); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		writeFile(t, filepath.Join(dir, name), content)
	}
	if err := os.Chtimes(filepath.Join(dir, metaFile), startedAt, startedAt); err != nil {
		t.Fatal(err)
	}
}

func TestListRunsRunningAndFailedOnly(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	running := PRRef{Repo: "console", Number: 1, URL: "u1"}
	failedOld := PRRef{Repo: "console", Number: 2, URL: "u2"}
	failedNew := PRRef{Repo: "web-console", Number: 3, URL: "u3"}
	done := PRRef{Repo: "console", Number: 4, URL: "u4"}
	seedRun(t, root, failedOld, map[string]string{exitFile: "1"}, base)
	seedRun(t, root, running, map[string]string{pidFile: strconv.Itoa(os.Getpid())}, base)
	seedRun(t, root, failedNew, map[string]string{exitFile: "2"}, base.Add(time.Hour))
	seedRun(t, root, done, map[string]string{exitFile: "0", FindingsFile: `{"findings":[]}`}, base)
	writeFile(t, filepath.Join(root, ".state", "approved-seen.json"), "{}")

	runs := ListRuns(root)
	var got []string
	for _, run := range runs {
		got = append(got, run.Meta.Ref.URL)
	}
	want := []string{"u1", "u3", "u2"}
	if len(got) != len(want) {
		t.Fatalf("runs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("runs = %v, want %v", got, want)
		}
	}
	if runs[0].Status != RunRunning || runs[1].Status != RunFailed || !runs[1].StartedAt.Equal(base.Add(time.Hour)) {
		t.Errorf("runs = %+v", runs)
	}
}

func TestListRunsMissingRoot(t *testing.T) {
	if runs := ListRuns(filepath.Join(t.TempDir(), "nope")); len(runs) != 0 {
		t.Errorf("runs = %v", runs)
	}
}

func writeLocalReview(t *testing.T, root string, ref PRRef, headSHA string) {
	t.Helper()
	dir := StateDir(root, ref)
	if err := WriteMeta(dir, Meta{Ref: ref, HeadSHA: headSHA}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, FindingsFile), []byte(`{"findings":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, exitFile), []byte("0"), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLocalReviewOutdated(t *testing.T) {
	root := t.TempDir()
	ref := PRRef{Host: "github.com", Owner: "o", Repo: "r", Number: 1, URL: "https://github.com/o/r/pull/1"}
	if LocalReviewOutdated(root, QueuedPR{Ref: ref, HeadSHA: "new"}) {
		t.Error("no local review must not be outdated")
	}
	writeLocalReview(t, root, ref, "old")
	if !LocalReviewOutdated(root, QueuedPR{Ref: ref, HeadSHA: "new"}) {
		t.Error("head moved since the local review")
	}
	if LocalReviewOutdated(root, QueuedPR{Ref: ref, HeadSHA: "old"}) || LocalReviewOutdated(root, QueuedPR{Ref: ref}) {
		t.Error("same or unknown head must not be outdated")
	}
}
