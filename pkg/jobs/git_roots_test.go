package jobs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckGitRoots(t *testing.T) {
	if err := CheckGitRoots(false, nil); err != nil {
		t.Errorf("git off needs no roots: %v", err)
	}
	if err := CheckGitRoots(true, nil); err == nil || !strings.Contains(err.Error(), "git_repository_roots") || !strings.Contains(err.Error(), "show_git: false") {
		t.Errorf("empty roots should name the fix: %v", err)
	}
	empty := t.TempDir()
	if err := CheckGitRoots(true, []string{empty}); err == nil || !strings.Contains(err.Error(), empty) {
		t.Errorf("roots without a repo should be reported: %v", err)
	}
	withRepo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(withRepo, "app", ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := CheckGitRoots(true, []string{empty, withRepo}); err != nil {
		t.Errorf("a repo under any root is enough: %v", err)
	}
}
