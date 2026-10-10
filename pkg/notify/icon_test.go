package notify

import (
	"bytes"
	"flag"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/achandrapaul/digest/pkg/brand"
)

var updateIcon = flag.Bool("update", false, "regenerate icon.png from the banner")

const (
	iconSize = 1024
	iconCell = 44
)

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

func renderIcon() []byte {
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

func TestEmbeddedIconMatchesTheBanner(t *testing.T) {
	if *updateIcon {
		if err := os.WriteFile("icon.png", renderIcon(), 0644); err != nil {
			t.Fatal(err)
		}
		t.Skip("icon.png regenerated; rebuild to embed it")
	}
	embedded, err := png.Decode(bytes.NewReader(IconPNG()))
	if err != nil {
		t.Fatalf("embedded icon.png does not decode: %v", err)
	}
	drawn, _ := png.Decode(bytes.NewReader(renderIcon()))
	if embedded.Bounds() != drawn.Bounds() {
		t.Fatalf("icon.png is %v, banner draws %v; run go test ./pkg/notify -run TestEmbeddedIcon -update", embedded.Bounds(), drawn.Bounds())
	}
	for y := range iconSize {
		for x := range iconSize {
			if embedded.At(x, y) != drawn.At(x, y) {
				t.Fatalf("icon.png differs from the banner at %d,%d; run go test ./pkg/notify -run TestEmbeddedIcon -update", x, y)
			}
		}
	}
}

func TestIconPNGDrawsTheBannerOnTheBase(t *testing.T) {
	image, err := png.Decode(bytes.NewReader(IconPNG()))
	if err != nil {
		t.Fatal(err)
	}
	if bounds := image.Bounds(); bounds.Dx() != iconSize || bounds.Dy() != iconSize {
		t.Fatalf("icon is %v", bounds)
	}
	colourAt := func(x, y int) [3]uint32 {
		red, green, blue, _ := image.At(x, y).RGBA()
		return [3]uint32{red >> 8, green >> 8, blue >> 8}
	}
	if corner := colourAt(0, 0); corner != [3]uint32{0x1E, 0x1E, 0x2E} {
		t.Errorf("corner = %x, want the base colour", corner)
	}
	left, top := iconOrigin()
	if ink := colourAt(left+iconCell/2, top+iconCell/2); ink != [3]uint32{0xCD, 0xD6, 0xF4} {
		t.Errorf("first banner cell = %x, want the text colour", ink)
	}
	if gap := colourAt(left+3*iconCell+iconCell/2, top+iconCell/2); gap != [3]uint32{0x1E, 0x1E, 0x2E} {
		t.Errorf("the space after D should be empty, got %x", gap)
	}
}

func TestBuildNotifierAppCopiesRebrandsAndSigns(t *testing.T) {
	var commands []string
	previous := runTool
	runTool = func(name string, args ...string) error {
		commands = append(commands, name+" "+strings.Join(args, " "))
		return nil
	}
	t.Cleanup(func() { runTool = previous })
	destination := filepath.Join(t.TempDir(), "Digest Notifier.app")
	if err := BuildNotifierApp("/brew/terminal-notifier.app", destination); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(commands, "\n")
	for _, want := range []string{
		"cp -R /brew/terminal-notifier.app " + destination,
		"iconutil -c icns",
		"Set :CFBundleIdentifier " + NotifierBundleID,
		"codesign --force --deep -s - " + destination,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("commands missing %q:\n%s", want, joined)
		}
	}
	if strings.Count(joined, "sips -z") != 10 {
		t.Errorf("iconset should have 10 sizes:\n%s", joined)
	}
}

func TestSenderUsesTheDigestNotifierWhenInstalled(t *testing.T) {
	appPath := filepath.Join(t.TempDir(), "Digest Notifier.app")
	previous := NotifierAppPath
	NotifierAppPath = func() string { return appPath }
	t.Cleanup(func() { NotifierAppPath = previous })
	if got := senderBinary(); got != "terminal-notifier" {
		t.Errorf("without the app, sender = %q", got)
	}
	binary := filepath.Join(appPath, "Contents", "MacOS", "terminal-notifier")
	if err := os.MkdirAll(filepath.Dir(binary), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, nil, 0755); err != nil {
		t.Fatal(err)
	}
	if got := senderBinary(); got != binary {
		t.Errorf("with the app, sender = %q, want %q", got, binary)
	}
}

func TestTerminalNotifierAppIsFoundFromTheBrewWrapper(t *testing.T) {
	cellar := t.TempDir()
	wrapper := filepath.Join(cellar, "bin", "terminal-notifier")
	app := filepath.Join(cellar, "terminal-notifier.app")
	for _, dir := range []string{filepath.Dir(wrapper), app} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(wrapper, []byte("#!/bin/bash\n"), 0755); err != nil {
		t.Fatal(err)
	}
	previous := lookPath
	lookPath = func(string) (string, error) { return wrapper, nil }
	t.Cleanup(func() { lookPath = previous })
	app, _ = filepath.EvalSymlinks(app)
	if got, err := terminalNotifierApp(); err != nil || got != app {
		t.Errorf("terminalNotifierApp = %q, %v; want %q", got, err, app)
	}
}
