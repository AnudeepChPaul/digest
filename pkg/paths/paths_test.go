package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreatePrivateIsOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.log")
	file, err := CreatePrivate(path)
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != PrivateFileMode {
		t.Errorf("mode = %v %v", info, err)
	}
}
