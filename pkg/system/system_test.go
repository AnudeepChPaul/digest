package system

import (
	"os"
	"path/filepath"
	"testing"
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
