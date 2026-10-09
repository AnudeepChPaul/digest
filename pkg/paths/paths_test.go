package paths_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AnudeepChPaul/digest/pkg/paths"
	"github.com/AnudeepChPaul/digest/pkg/system"
)

func TestPrivateLogIsOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.log")
	if err := system.Write(path, nil); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != paths.PrivateFileMode {
		t.Errorf("mode = %v %v", info, err)
	}
}
