package review

import (
	"errors"
	"io/fs"
	"path/filepath"

	"github.com/achandrapaul/digest/pkg/system"
)

func Dismiss(root string, ref PRRef) error {
	dir := StateDir(root, ref)
	if _, running := RunningPID(dir); running {
		return ErrReviewRunning
	}
	if err := system.Remove(filepath.Join(dir, exitFile)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
