package running

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/AnudeepChPaul/digest/pkg/paths"
)

func Mark(marker string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(marker), paths.PrivateDirMode); err != nil {
		return nil, err
	}
	if err := os.WriteFile(marker, []byte(strconv.Itoa(os.Getpid())), paths.PrivateFileMode); err != nil {
		return nil, err
	}
	return func() { os.Remove(marker) }, nil
}

func Alive(marker string) bool {
	data, err := os.ReadFile(marker)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	return err == nil && process.Signal(syscall.Signal(0)) == nil
}
