//go:build darwin

package system

import (
	"os"

	"golang.org/x/sys/unix"
)

var ChangeFileFlags = unix.Chflags

func setLocked(path string, locked bool) error {
	var stat unix.Stat_t
	if err := unix.Lstat(path, &stat); err != nil {
		return &os.PathError{Op: "lstat", Path: path, Err: err}
	}
	flags := stat.Flags &^ unix.UF_IMMUTABLE
	if locked {
		flags |= unix.UF_IMMUTABLE
	}
	if flags == stat.Flags {
		return nil
	}
	if err := ChangeFileFlags(path, int(flags)); err != nil {
		return &os.PathError{Op: "chflags", Path: path, Err: err}
	}
	return nil
}
