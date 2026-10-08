package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckingAJobLeavesItsStalePIDFile(t *testing.T) {
	pidPath := filepath.Join(getLogsDir(), "stale-check.pid")
	if err := os.WriteFile(pidPath, []byte("999999"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(pidPath) })
	if isJobRunning("stale-check") {
		t.Fatal("a dead pid should not count as running")
	}
	if _, err := os.Stat(pidPath); err != nil {
		t.Error("checking whether a job runs deleted its pid file")
	}
}
