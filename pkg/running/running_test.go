package running

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestMarkIsAliveUntilReleased(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "state", "tui.pid")
	release, err := Mark(marker)
	if err != nil {
		t.Fatal(err)
	}
	if !Alive(marker) {
		t.Fatal("marker of this process should be alive")
	}
	release()
	if Alive(marker) {
		t.Error("released marker should not be alive")
	}
}

func TestMarkerOfAGoneProcessIsNotAlive(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "tui.pid")
	if err := os.WriteFile(marker, []byte(strconv.Itoa(1<<22)), 0o600); err != nil {
		t.Fatal(err)
	}
	if Alive(marker) {
		t.Error("a crashed TUI's marker should be ignored")
	}
}
