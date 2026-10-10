package running

import (
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/achandrapaul/digest/pkg/system"
)

func Mark(marker string) (func(), error) {
	if err := system.Write(marker, []byte(strconv.Itoa(os.Getpid()))); err != nil {
		return nil, err
	}
	return func() { _ = system.Remove(marker) }, nil
}

func Alive(marker string) bool {
	data, err := system.Read(marker)
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
