// Package screenshot renders the gomacs terminal state to PNG image files.
// It is used both by the cmd/shotgen documentation tool and by the interactive
// M-x screenshot command.
package screenshot

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"time"

	"github.com/skybert/gomacs/internal/syntax"
	"github.com/skybert/gomacs/internal/terminal"
)

// Layout constants for the rendered image.
const (
	PadX = 8 // horizontal padding in device pixels
	PadY = 6 // vertical padding in device pixels
)

// DefaultFontSize is the point size used when none is specified.
const DefaultFontSize = 16.0

// Default sweet-theme terminal colors used when Face.Fg/Bg are empty.
const (
	DefaultBg = "#222235"
	DefaultFg = "#b8c0d4"
)

// FontHandle holds font metrics and an opaque platform-specific font reference
// returned by LoadFont. Pass it directly to RenderToImage or TakeScreenshot.
type FontHandle struct {
	CharW int // character cell width in device pixels
	LineH int // line height in device pixels
	impl  any // platform-specific data (darwinFont or otherFont)
}

// TakeScreenshot renders the terminal state to a PNG file at outPath, creating
// any missing parent directories. fontSize controls the font point size used
// for rendering.
func TakeScreenshot(t *terminal.Terminal, outPath string, fontSize float64) error {
	h, err := LoadFont(fontSize)
	if err != nil {
		return fmt.Errorf("load font: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	img := RenderToImage(t, h)
	return WritePNG(img, outPath)
}

// TimestampedName returns a filename of the form "gomacs-<mode>-20060102-150405.png"
// using the current local time.
func TimestampedName(mode string) string {
	return "gomacs-" + mode + time.Now().Format("-20060102-150405.png")
}

// WritePNG encodes img as PNG and writes it to path, creating any missing
// parent directories. If the file already exists with identical content the
// write is skipped, keeping the file's mtime unchanged (so make and git do
// not see a spurious change).
func WritePNG(img image.Image, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, buf.Bytes()) {
		return nil // identical — skip write
	}
	return os.WriteFile(path, buf.Bytes(), 0o644) //nolint:gosec
}

// ResolveColor converts a hex color string to color.RGBA, falling back to the
// supplied default RGB values if parsing fails.
func ResolveColor(s string, defaultR, defaultG, defaultB uint8) color.RGBA {
	if r, g, b, ok := terminal.ParseColorRGB(s); ok {
		return color.RGBA{R: r, G: g, B: b, A: 255}
	}
	return color.RGBA{R: defaultR, G: defaultG, B: defaultB, A: 255}
}

// CellColors resolves the foreground and background colors for a terminal cell,
// applying reverse-video if set.
func CellColors(f syntax.Face, defBg, defFg color.RGBA) (bg, fg color.RGBA) {
	bg = defBg
	fg = defFg
	if f.Bg != "" {
		if r, g, b, ok := terminal.ParseColorRGB(f.Bg); ok {
			bg = color.RGBA{R: r, G: g, B: b, A: 255}
		}
	}
	if f.Fg != "" {
		if r, g, b, ok := terminal.ParseColorRGB(f.Fg); ok {
			fg = color.RGBA{R: r, G: g, B: b, A: 255}
		}
	}
	if f.Reverse {
		bg, fg = fg, bg
	}
	return bg, fg
}
