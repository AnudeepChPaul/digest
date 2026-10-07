package sourcecontrol

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestShellCommandTimeoutKillsChildProcesses(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	executeShellCommand(ctx, "", "sleep 30 & echo $! > "+pidPath+"; wait")
	data, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatal(err)
	}
	childPID, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	deadline := time.Now().Add(2 * time.Second)
	for syscall.Kill(childPID, 0) == nil {
		if time.Now().After(deadline) {
			_ = syscall.Kill(childPID, syscall.SIGKILL)
			t.Fatal("the child process outlived the timeout")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
