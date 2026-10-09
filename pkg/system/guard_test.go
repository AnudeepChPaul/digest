package system

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var directFileCalls = map[string]map[string]bool{
	"os": {
		"ReadFile": true, "WriteFile": true, "Create": true, "CreateTemp": true, "OpenFile": true, "Open": true,
		"Remove": true, "RemoveAll": true, "Rename": true, "MkdirAll": true, "Mkdir": true, "ReadDir": true,
		"Stat": true, "Lstat": true, "Chmod": true, "Symlink": true, "MkdirTemp": true, "Readlink": true, "Truncate": true,
	},
	"filepath": {"Glob": true, "WalkDir": true, "Walk": true},
	"ioutil":   {"ReadFile": true, "WriteFile": true, "ReadDir": true, "TempFile": true, "TempDir": true},
}

func TestFileAccessGoesThroughSystem(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	systemDir := filepath.Join(repoRoot, "pkg", "system")
	var violations []string
	for _, top := range []string{"cmd", "pkg"} {
		err := filepath.WalkDir(filepath.Join(repoRoot, top), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if path == systemDir {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			violations = append(violations, directFileAccess(t, repoRoot, path)...)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(violations) > 0 {
		t.Errorf("file access outside pkg/system:\n%s", strings.Join(violations, "\n"))
	}
}

func directFileAccess(t *testing.T, repoRoot, path string) []string {
	t.Helper()
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	relative, _ := filepath.Rel(repoRoot, path)
	var found []string
	ast.Inspect(file, func(node ast.Node) bool {
		selector, isSelector := node.(*ast.SelectorExpr)
		if !isSelector {
			return true
		}
		packageName, isIdent := selector.X.(*ast.Ident)
		if !isIdent {
			return true
		}
		if directFileCalls[packageName.Name][selector.Sel.Name] {
			found = append(found, relative+":"+strconv.Itoa(fileSet.Position(selector.Pos()).Line)+" "+packageName.Name+"."+selector.Sel.Name)
		}
		return true
	})
	return found
}
