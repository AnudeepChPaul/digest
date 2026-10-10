package system

import (
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/achandrapaul/digest/pkg/paths"
)

func TestWriteReplacesTheFileInPlace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "file.txt")
	if err := Write(path, []byte("first")); err != nil {
		t.Fatal(err)
	}
	before, _ := Stat(path)
	if err := Write(path, []byte("second")); err != nil {
		t.Fatal(err)
	}
	if after, _ := Stat(path); !os.SameFile(before, after) {
		t.Error("write should change the same file in place")
	}
	if content, _ := Read(path); string(content) != "second" {
		t.Errorf("content = %q", content)
	}
	if info, _ := Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v", info.Mode().Perm())
	}
	entries, _ := List(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("extra files left behind: %v", entries)
	}
}

func TestFailedWriteLeavesNoExtraFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "child"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Write(target, []byte("x")); err == nil {
		t.Fatal("writing over a non-empty folder should fail")
	}
	entries, _ := List(dir)
	if len(entries) != 1 {
		t.Errorf("extra files left behind: %v", entries)
	}
}

func TestJSONRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	var missing map[string]int
	if found, err := ReadJSON(path, &missing); found || err != nil {
		t.Fatalf("missing file = %v, %v", found, err)
	}
	if err := WriteJSON(path, map[string]int{"streak": 3}); err != nil {
		t.Fatal(err)
	}
	var loaded map[string]int
	if found, err := ReadJSON(path, &loaded); !found || err != nil || loaded["streak"] != 3 {
		t.Fatalf("loaded = %v, %v, %v", loaded, found, err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadJSON(path, &loaded); err == nil {
		t.Error("corrupt JSON should be an error")
	}
}

func TestAppendAndLogWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "run.log")
	if err := Append(path, []byte("one\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := LogWriter(path).Write([]byte("two\n")); err != nil {
		t.Fatal(err)
	}
	if content, _ := Read(path); string(content) != "one\ntwo\n" {
		t.Errorf("log = %q", content)
	}
}

func TestWriteExistingNeedsTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gone.md")
	if err := WriteExisting(path, []byte("x")); !os.IsNotExist(err) {
		t.Errorf("err = %v, want not exist", err)
	}
	if Exists(path) {
		t.Error("WriteExisting should not create the file")
	}
}

func TestRemoveAllListsEveryFileItCannotRemove(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "folder")
	for _, name := range []string{"a", "b"} {
		if err := Write(filepath.Join(folder, name), nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(folder, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(folder, 0o700) })
	err := RemoveAll(folder)
	for _, name := range []string{"a", "b"} {
		if err == nil || !strings.Contains(err.Error(), filepath.Join(folder, name)) {
			t.Errorf("err = %v, want %s named", err, name)
		}
	}
	if !strings.Contains(err.Error(), "2 failed") {
		t.Errorf("err = %v, want the failure count", err)
	}
}

func TestRemoveAllReportsAnUnreadableFolder(t *testing.T) {
	parent := t.TempDir()
	sealed := filepath.Join(parent, "sealed")
	if err := Write(filepath.Join(sealed, "inside"), nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sealed, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(sealed, 0o700) })
	if err := RemoveAll(sealed); err == nil || !strings.Contains(err.Error(), "permission denied") || !strings.Contains(err.Error(), sealed) {
		t.Errorf("err = %v, want the unreadable folder named", err)
	}
	emptySealed := filepath.Join(parent, "empty-sealed")
	if err := os.Mkdir(emptySealed, 0); err != nil {
		t.Fatal(err)
	}
	if err := RemoveAll(emptySealed); err != nil || Exists(emptySealed) {
		t.Errorf("an empty unreadable folder can still be removed: %v", err)
	}
}

func TestRemoveAllUnderAFileFails(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := Write(file, nil); err != nil {
		t.Fatal(err)
	}
	if err := RemoveAll(filepath.Join(file, "child")); err == nil {
		t.Error("removing below a file should fail")
	}
}

func TestRemoveAllOfNothingSucceeds(t *testing.T) {
	if err := RemoveAll(filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Errorf("err = %v", err)
	}
}

func TestRemoveAllRemovesTreesAndLinks(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"tree/a", "tree/deep/b"} {
		if err := Write(filepath.Join(root, name), nil); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(root, "link")
	if err := Symlink(filepath.Join(root, "tree"), link); err != nil {
		t.Fatal(err)
	}
	if err := RemoveAll(link); err != nil || !Exists(filepath.Join(root, "tree", "a")) {
		t.Fatalf("removing a link should keep its target: %v", err)
	}
	if err := RemoveAll(filepath.Join(root, "tree")); err != nil || Exists(filepath.Join(root, "tree")) {
		t.Errorf("tree should be gone: %v", err)
	}
}

func TestPrivateLogIsOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.log")
	if err := Write(path, nil); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != paths.PrivateFileMode {
		t.Errorf("mode = %v %v", info, err)
	}
}

func fileInTheWay(t *testing.T) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "file")
	if err := Write(file, nil); err != nil {
		t.Fatal(err)
	}
	return file
}

func brokenPipePath(t *testing.T) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	reader.Close()
	t.Cleanup(func() { writer.Close() })
	return "/dev/fd/" + strconv.Itoa(int(writer.Fd()))
}

func TestWritesBelowAFileFail(t *testing.T) {
	below := filepath.Join(fileInTheWay(t), "child", "x")
	if err := Write(below, nil); err == nil {
		t.Error("Write below a file should fail")
	}
	if err := Append(below, nil); err == nil {
		t.Error("Append below a file should fail")
	}
	if _, err := LogWriter(below).Write([]byte("x")); err == nil {
		t.Error("LogWriter below a file should fail")
	}
	if _, err := OpenLog(below); err == nil {
		t.Error("OpenLog below a file should fail")
	}
	if err := MkdirAll(below); err == nil {
		t.Error("MkdirAll below a file should fail")
	}
}

func TestAppendReportsFailures(t *testing.T) {
	if err := Append(t.TempDir(), []byte("x")); err == nil {
		t.Error("appending to a folder should fail")
	}
	if err := Append(brokenPipePath(t), []byte("x")); err == nil {
		t.Error("appending to a closed pipe should fail")
	}
}

func TestWriteExistingReplacesAnExistingFile(t *testing.T) {
	path := fileInTheWay(t)
	if err := WriteExisting(path, []byte("replaced")); err != nil {
		t.Fatal(err)
	}
	if content, _ := Read(path); string(content) != "replaced" {
		t.Errorf("content = %q", content)
	}
}

func TestWriteJSONRejectsValuesThatCannotBeEncoded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := WriteJSON(path, make(chan int)); err == nil || Exists(path) {
		t.Errorf("err = %v, file written = %v", err, Exists(path))
	}
}

func TestReadJSONReportsAnUnreadablePath(t *testing.T) {
	var value map[string]int
	if found, err := ReadJSON(t.TempDir(), &value); found || err == nil {
		t.Errorf("reading a folder = %v, %v; want an error", found, err)
	}
}

func TestFolderHelpers(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b")
	if err := MkdirAll(nested); err != nil {
		t.Fatal(err)
	}
	if info, err := Stat(nested); err != nil || info.Mode().Perm() != paths.PrivateDirMode {
		t.Errorf("MkdirAll mode = %v, %v", info, err)
	}
	shared := filepath.Join(root, "shared")
	if err := MkdirAllWithMode(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	if info, err := Stat(shared); err != nil || info.Mode().Perm() != 0o755 {
		t.Errorf("MkdirAllWithMode mode = %v, %v", info, err)
	}
	temporary, err := MkdirTemp(root, "work-*")
	if err != nil || !strings.HasPrefix(filepath.Base(temporary), "work-") || !Exists(temporary) {
		t.Errorf("MkdirTemp = %q, %v", temporary, err)
	}
}

func TestReadHelpers(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "notes", "a.md")
	if err := Write(path, []byte("content")); err != nil {
		t.Fatal(err)
	}
	file, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	link := filepath.Join(root, "link")
	if err := Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if info, err := Lstat(link); err != nil || info.Mode()&fs.ModeSymlink == 0 {
		t.Errorf("Lstat = %v, %v; want the link itself", info, err)
	}
	if target, err := ReadLink(link); err != nil || target != path {
		t.Errorf("ReadLink = %q, %v", target, err)
	}
	if matches, err := Glob(filepath.Join(root, "notes", "*.md")); err != nil || len(matches) != 1 || matches[0] != path {
		t.Errorf("Glob = %v, %v", matches, err)
	}
	var walked []string
	if err := Walk(root, func(visited string, entry fs.DirEntry, err error) error {
		walked = append(walked, visited)
		return err
	}); err != nil || len(walked) != 4 {
		t.Errorf("Walk visited %v, %v", walked, err)
	}
}

func TestRenameRemoveAndChmodReportMissingFiles(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if err := Rename(missing, missing+".moved"); !os.IsNotExist(err) {
		t.Errorf("Rename err = %v", err)
	}
	if err := Remove(missing); !os.IsNotExist(err) {
		t.Errorf("Remove err = %v", err)
	}
	if err := Chmod(missing, 0o600); !os.IsNotExist(err) {
		t.Errorf("Chmod err = %v", err)
	}
}

func TestChmodChangesTheMode(t *testing.T) {
	path := fileInTheWay(t)
	if err := Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	if info, _ := Stat(path); info.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v", info.Mode().Perm())
	}
}
