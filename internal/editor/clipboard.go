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
	argv := clipboardCmdFor(runtime.GOOS,
		os.Getenv("WAYLAND_DISPLAY"), os.Getenv("DISPLAY"), exec.LookPath)
	if argv == nil {
		return nil
	}
	return exec.Command(argv[0], argv[1:]...) //nolint:gosec
}

// clipboardCmdFor returns the argv of the clipboard tool to use for the given
// platform (goos) and display environment, or nil when none is available.
// Tool availability is probed through look (exec.LookPath in production) so the
// selection logic is testable on any platform.
//
// Candidates are tried in order until one is actually installed: Wayland
// (wl-copy), then X11 (xclip, then xsel).  Trying rather than switching matters
// on GNOME/Wayland without wl-copy, where an X11 tool still works via XWayland.
func clipboardCmdFor(goos, waylandDisplay, x11Display string, look func(string) (string, error)) []string {
	if goos == "darwin" {
		return []string{"pbcopy"}
	}

	installed := func(name string) bool {
		_, err := look(name)
		return err == nil
	}

	if waylandDisplay != "" && installed("wl-copy") {
		return []string{"wl-copy"}
	}
	if x11Display != "" {
		if installed("xclip") {
			return []string{"xclip", "-selection", "clipboard"}
		}
		if installed("xsel") {
			return []string{"xsel", "--clipboard", "--input"}
		}
	}
	return nil
}
