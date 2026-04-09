package editor

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/screenshot"
)

// cmdScreenshot renders the current editor state to a PNG file and saves it to
// the configured screenshot directory (default: working directory at startup).
func (e *Editor) cmdScreenshot() {
	dir := e.screenshotDir
	if dir == "" {
		dir = e.startDir
	}
	// Expand leading ~/ to the user's home directory.
	if strings.HasPrefix(dir, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, dir[2:])
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		e.Message("screenshot: mkdir %s: %v", dir, err)
		return
	}
	outPath := filepath.Join(dir, screenshot.TimestampedName(screenshotSlug(e.ActiveBuffer())))

	e.term.EnableCapture()
	e.Redraw()
	err := screenshot.TakeScreenshot(e.term, outPath, screenshot.DefaultFontSize)
	e.term.DisableCapture()
	// Restore the real screen now that capture mode is disabled.
	e.Redraw()

	if err != nil {
		e.Message("screenshot failed: %v", err)
		return
	}
	e.Message("screenshot saved: %s", outPath)
}

// screenshotSlug returns a filename-safe identifier for the buffer: the base
// name of its file with '.' replaced by '_', or the buffer name if no file.
func screenshotSlug(b *buffer.Buffer) string {
	name := filepath.Base(b.Filename())
	if name == "" || name == "." {
		name = b.Name()
	}
	return strings.NewReplacer(".", "_", "*", "_").Replace(name)
}
