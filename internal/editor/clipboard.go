package editor

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// clipboardWrite sends text to the OS clipboard asynchronously.
// Supports macOS (pbcopy), Linux/Wayland (wl-copy), and Linux/X11
// (xclip -selection clipboard, falling back to xsel --clipboard --input).
// Errors are silently ignored so a missing clipboard tool never blocks editing.
func clipboardWrite(text string) {
	go func() {
		cmd := clipboardCmd()
		if cmd == nil {
			return
		}
		cmd.Stdin = strings.NewReader(text)
		_ = cmd.Run()
	}()
}

// clipboardCmd returns an exec.Cmd for writing to the clipboard, or nil if no
// suitable tool is available for the current platform/environment.
func clipboardCmd() *exec.Cmd {
	switch {
	case runtime.GOOS == "darwin":
		return exec.Command("pbcopy") //nolint:gosec
	case os.Getenv("WAYLAND_DISPLAY") != "":
		if _, err := exec.LookPath("wl-copy"); err == nil {
			return exec.Command("wl-copy") //nolint:gosec
		}
	case os.Getenv("DISPLAY") != "":
		if _, err := exec.LookPath("xclip"); err == nil {
			return exec.Command("xclip", "-selection", "clipboard") //nolint:gosec
		}
		if _, err := exec.LookPath("xsel"); err == nil {
			return exec.Command("xsel", "--clipboard", "--input") //nolint:gosec
		}
	}
	return nil
}
