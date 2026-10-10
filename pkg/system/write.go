package system

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/achandrapaul/digest/pkg/paths"
)

const logExtension = ".log"

var (
	ErrUnlock     = errors.New("could not unlock file")
	ErrLock       = errors.New("saved but not locked")
	ErrLockedPath = errors.New("path is locked by digest")
)

var protection struct {
	sync.RWMutex
	root   string
	except []string
}

func Protect(root string, except ...string) {
	protection.Lock()
	defer protection.Unlock()
	protection.root = cleanRoot(root)
	protection.except = nil
	for _, folder := range except {
		protection.except = append(protection.except, cleanRoot(folder))
	}
}

func cleanRoot(path string) string {
	if path == "" {
		return ""
	}
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	return filepath.Clean(path)
}

func within(path, root string) bool {
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}

func Protected(path string) bool {
	protection.RLock()
	defer protection.RUnlock()
	if protection.root == "" {
		return false
	}
	path = cleanRoot(path)
	if !within(path, protection.root) || strings.HasSuffix(path, logExtension) {
		return false
	}
	for _, folder := range protection.except {
		if within(path, folder) {
			return false
		}
	}
	return true
}

func unlockBefore(path string) error {
	if !Protected(path) {
		return nil
	}
	if err := setLocked(path, false); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: %w", ErrUnlock, err)
	}
	return nil
}

func lockAfter(path string) error {
	if !Protected(path) {
		return nil
	}
	if err := setLocked(path, true); err != nil {
		return fmt.Errorf("%w: %w", ErrLock, err)
	}
	return nil
}

func relock(path string) {
	if Protected(path) {
		_ = setLocked(path, true)
	}
}

func Write(path string, content []byte) error {
	return WriteWithMode(path, content, paths.PrivateFileMode)
}

func WriteWithMode(path string, content []byte, mode fs.FileMode) error {
	if err := unlockBefore(path); err != nil {
		return err
	}
	if err := replaceFile(path, content, mode); err != nil {
		relock(path)
		return err
	}
	return lockAfter(path)
}

func WriteExisting(path string, content []byte) error {
	if !Exists(path) {
		return &fs.PathError{Op: "write", Path: path, Err: fs.ErrNotExist}
	}
	return Write(path, content)
}

func replaceFile(path string, content []byte, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), paths.PrivateDirMode); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(content)
	closeErr := file.Close()
	return errors.Join(writeErr, closeErr)
}

func WriteJSON(path string, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return Write(path, append(encoded, '\n'))
}

func Append(path string, chunk []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), paths.PrivateDirMode); err != nil {
		return err
	}
	if err := unlockBefore(path); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, paths.PrivateFileMode)
	if err != nil {
		relock(path)
		return err
	}
	_, writeErr := file.Write(chunk)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		relock(path)
		return err
	}
	return lockAfter(path)
}

func OpenLog(path string) (*os.File, error) {
	if Protected(path) {
		return nil, &fs.PathError{Op: "open log", Path: path, Err: ErrLockedPath}
	}
	if err := os.MkdirAll(filepath.Dir(path), paths.PrivateDirMode); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, paths.PrivateFileMode)
}

type logWriter struct {
	path string
}

func (writer logWriter) Write(chunk []byte) (int, error) {
	if err := Append(writer.path, chunk); err != nil {
		return 0, err
	}
	return len(chunk), nil
}

func LogWriter(path string) io.Writer {
	return logWriter{path: path}
}

func MkdirAll(path string) error {
	return os.MkdirAll(path, paths.PrivateDirMode)
}

func MkdirAllWithMode(path string, mode fs.FileMode) error {
	return os.MkdirAll(path, mode)
}

func MkdirTemp(dir, pattern string) (string, error) {
	return os.MkdirTemp(dir, pattern)
}

func Rename(from, to string) error {
	if err := unlockBefore(from); err != nil {
		return err
	}
	if err := os.Rename(from, to); err != nil {
		relock(from)
		return err
	}
	return lockAfter(to)
}

func Remove(path string) error {
	if err := unlockBefore(path); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		relock(path)
		return err
	}
	return nil
}

func RemoveAll(path string) error {
	var failures []error
	if Protected(path) {
		for _, err := range changeTree(path, false) {
			failures = append(failures, fmt.Errorf("%w: %w", ErrUnlock, err))
		}
	}
	failures = append(failures, removeTree(path)...)
	if len(failures) > 0 {
		return fmt.Errorf("remove %s: %d failed: %w", path, len(failures), errors.Join(failures...))
	}
	return nil
}

func removeTree(path string) []error {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return []error{err}
	}
	var readErr error
	if info.IsDir() {
		var entries []fs.DirEntry
		entries, readErr = os.ReadDir(path)
		var childFailures []error
		for _, entry := range entries {
			childFailures = append(childFailures, removeTree(filepath.Join(path, entry.Name()))...)
		}
		if len(childFailures) > 0 {
			return childFailures
		}
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return []error{errors.Join(readErr, err)}
	}
	return nil
}

func Chmod(path string, mode fs.FileMode) error {
	if err := unlockBefore(path); err != nil {
		return err
	}
	if err := os.Chmod(path, mode); err != nil {
		relock(path)
		return err
	}
	return lockAfter(path)
}

func Symlink(target, link string) error {
	return os.Symlink(target, link)
}

func LockTree(root string) (int, error) {
	locked := 0
	failures := walkFiles(root, func(path string) error {
		if !Protected(path) {
			return nil
		}
		if err := setLocked(path, true); err != nil {
			return err
		}
		locked++
		return nil
	})
	if len(failures) > 0 {
		return locked, fmt.Errorf("%d locked, %d failed: %w", locked, len(failures), errors.Join(failures...))
	}
	return locked, nil
}

func UnlockTree(root string) error {
	return errors.Join(changeTree(root, false)...)
}

func changeTree(root string, locked bool) []error {
	return walkFiles(root, func(path string) error {
		return setLocked(path, locked)
	})
}

func walkFiles(root string, visit func(path string) error) []error {
	var failures []error
	record := func(err error) {
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			failures = append(failures, err)
		}
	}
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			record(err)
		} else if entry.Type().IsRegular() {
			record(visit(path))
		}
		return nil
	})
	record(walkErr)
	return failures
}
