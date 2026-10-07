package review

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	sendNotification = func(string, string, string) error { return nil }
	os.Exit(m.Run())
}
