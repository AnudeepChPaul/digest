//go:build !darwin

package system

var ChangeFileFlags = func(string, int) error { return nil }

func setLocked(string, bool) error {
	return nil
}
