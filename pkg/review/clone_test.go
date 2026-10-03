package review

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
)

func TestPrepareFreshClone(t *testing.T) {
	root := t.TempDir()
	ref := PRRef{Host: "github.com", Owner: "o", Repo: "console", Number: 9}
	stale := filepath.Join(CloneDir(root, ref), "stale.txt")
	writeFile(t, stale, "old")

	var calls []string
	original := runStep
	runStep = func(ctx context.Context, output io.Writer, dir string, name string, args ...string) error {
		calls = append(calls, dir+"|"+name+" "+strings.Join(args, " "))
		if name == "gh" && args[0] == "repo" {
			return os.MkdirAll(args[3], 0755)
		}
		return nil
	}
	defer func() { runStep = original }()

	dir, err := Prepare(context.Background(), ref, root, log.New(os.Stderr))
	if err != nil {
		t.Fatal(err)
	}
	if dir != CloneDir(root, ref) {
		t.Errorf("dir = %q", dir)
	}
	if _, err := os.Stat(stale); err == nil {
		t.Errorf("stale clone was not removed")
	}
	if len(calls) < 2 {
		t.Fatalf("calls = %v", calls)
	}
	if !strings.HasPrefix(calls[0], root+"|gh repo clone github.com/o/console "+dir) {
		t.Errorf("clone call = %q", calls[0])
	}
	if calls[1] != dir+"|gh pr checkout 9" {
		t.Errorf("checkout call = %q", calls[1])
	}
}
