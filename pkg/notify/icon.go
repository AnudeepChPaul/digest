package notify

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/AnudeepChPaul/digest/pkg/brand"
)

const (
	iconSize         = 1024
	iconCell         = 44
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

func hexColour(hex string) color.RGBA {
	value, _ := strconv.ParseUint(hex[1:], 16, 32)
	return color.RGBA{R: uint8(value >> 16), G: uint8(value >> 8), B: uint8(value), A: 0xFF}
}

func bannerPixels() [][]bool {
	var pixels [][]bool
	for _, row := range brand.BannerRows {
		var upper, lower []bool
		for _, glyph := range row {
			upper = append(upper, glyph == '█' || glyph == '▀')
			lower = append(lower, glyph == '█' || glyph == '▄')
		}
		pixels = append(pixels, upper, lower)
	}
	return pixels
}

func iconOrigin() (left, top int) {
	pixels := bannerPixels()
	return (iconSize - len(pixels[0])*iconCell) / 2, (iconSize - len(pixels)*iconCell) / 2
}

func IconPNG() []byte {
	pixels := bannerPixels()
	left, top := iconOrigin()
	base, ink := hexColour(brand.BaseColour), hexColour(brand.TextColour)
	canvas := image.NewRGBA(image.Rect(0, 0, iconSize, iconSize))
	for y := range iconSize {
		for x := range iconSize {
			column, row := (x-left)/iconCell, (y-top)/iconCell
			inside := x >= left && y >= top && row < len(pixels) && column < len(pixels[row])
			if inside && pixels[row][column] {
				canvas.SetRGBA(x, y, ink)
			} else {
				canvas.SetRGBA(x, y, base)
			}
		}
	}
	var encoded bytes.Buffer
	_ = png.Encode(&encoded, canvas)
	return encoded.Bytes()
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
	if _, err := os.Stat(app); err != nil {
		return "", errors.New("terminal-notifier.app not found next to " + wrapper)
	}
	return app, nil
}

func BuildNotifierApp(sourceApp, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return err
	}
	if err := os.RemoveAll(destination); err != nil {
		return err
	}
	if err := runTool("cp", "-R", sourceApp, destination); err != nil {
		return err
	}
	workDir, err := os.MkdirTemp("", "digest-icon")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workDir)
	iconPath, iconset := filepath.Join(workDir, "icon.png"), filepath.Join(workDir, "digest.iconset")
	if err := os.MkdirAll(iconset, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(iconPath, IconPNG(), 0644); err != nil {
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
	if _, err := os.Stat(binary); err == nil {
		return binary
	}
	return "terminal-notifier"
}
