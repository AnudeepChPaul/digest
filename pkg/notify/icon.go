package notify

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/achandrapaul/digest/pkg/system"
)

//go:embed icon.png
var iconPNG []byte

const (
	NotifierBundleID = "com.digest.notifier"
	notifierAppName  = "Digest Notifier.app"
)

var iconSizes = []int{16, 32, 128, 256, 512}

var NotifierAppPath = func() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Application Support", "digest", notifierAppName)
}

var runTool = func(name string, args ...string) error {
	output, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", name, err, output)
	}
	return nil
}

func IconPNG() []byte {
	return iconPNG
}

func terminalNotifierApp() (string, error) {
	wrapper, err := lookPath("terminal-notifier")
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(wrapper); err == nil {
		wrapper = resolved
	}
	app := filepath.Join(filepath.Dir(filepath.Dir(wrapper)), "terminal-notifier.app")
	if _, err := system.Stat(app); err != nil {
		return "", errors.New("terminal-notifier.app not found next to " + wrapper)
	}
	return app, nil
}

func BuildNotifierApp(sourceApp, destination string) error {
	if err := system.MkdirAllWithMode(filepath.Dir(destination), 0755); err != nil {
		return err
	}
	if err := system.RemoveAll(destination); err != nil {
		return err
	}
	if err := runTool("cp", "-R", sourceApp, destination); err != nil {
		return err
	}
	workDir, err := system.MkdirTemp("", "digest-icon")
	if err != nil {
		return err
	}
	defer system.RemoveAll(workDir)
	iconPath, iconset := filepath.Join(workDir, "icon.png"), filepath.Join(workDir, "digest.iconset")
	if err := system.MkdirAllWithMode(iconset, 0755); err != nil {
		return err
	}
	if err := system.WriteWithMode(iconPath, IconPNG(), 0644); err != nil {
		return err
	}
	for _, size := range iconSizes {
		for scale, suffix := range map[int]string{1: "", 2: "@2x"} {
			pixels := strconv.Itoa(size * scale)
			output := filepath.Join(iconset, fmt.Sprintf("icon_%dx%d%s.png", size, size, suffix))
			if err := runTool("sips", "-z", pixels, pixels, iconPath, "--out", output); err != nil {
				return err
			}
		}
	}
	contents := filepath.Join(destination, "Contents")
	if err := runTool("iconutil", "-c", "icns", iconset, "-o", filepath.Join(contents, "Resources", "Terminal.icns")); err != nil {
		return err
	}
	if err := runTool("/usr/libexec/PlistBuddy", "-c", "Set :CFBundleIdentifier "+NotifierBundleID, "-c", "Set :CFBundleName Digest", filepath.Join(contents, "Info.plist")); err != nil {
		return err
	}
	return runTool("codesign", "--force", "--deep", "-s", "-", destination)
}

func senderBinary() string {
	binary := filepath.Join(NotifierAppPath(), "Contents", "MacOS", "terminal-notifier")
	if system.Exists(binary) {
		return binary
	}
	return "terminal-notifier"
}
