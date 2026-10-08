package automation

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDismissRefusesNoteIDsOutsideTheRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "automations")
	outside := filepath.Join(parent, "keep")
	if err := os.MkdirAll(outside, 0700); err != nil {
		t.Fatal(err)
	}
	for _, noteID := range []string{"../keep", "..", "", "a/../../keep"} {
		if err := Dismiss(root, noteID); err == nil {
			t.Errorf("Dismiss(%q) should refuse", noteID)
		}
	}
	if _, err := os.Stat(outside); err != nil {
		t.Error("Dismiss removed a folder outside the automations root")
	}
}
