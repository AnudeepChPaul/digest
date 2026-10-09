package system

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func Read(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func ReadJSON(path string, value any) (bool, error) {
	encoded, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(encoded, value); err != nil {
		return false, fmt.Errorf("%s: %w", path, err)
	}
	return true, nil
}

func Open(path string) (*os.File, error) {
	return os.Open(path)
}

func Stat(path string) (fs.FileInfo, error) {
	return os.Stat(path)
}

func Lstat(path string) (fs.FileInfo, error) {
	return os.Lstat(path)
}

func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func List(dir string) ([]fs.DirEntry, error) {
	return os.ReadDir(dir)
}

func Glob(pattern string) ([]string, error) {
	return filepath.Glob(pattern)
}

func Walk(root string, visit fs.WalkDirFunc) error {
	return filepath.WalkDir(root, visit)
}

func ReadLink(path string) (string, error) {
	return os.Readlink(path)
}
