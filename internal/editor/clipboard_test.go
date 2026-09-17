package editor

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestClipboardCmd(t *testing.T) {
	cmd := clipboardCmd()
	if runtime.GOOS == "darwin" {
		if cmd == nil {
			t.Fatal("clipboardCmd should return a pbcopy command on darwin")
		}
		if cmd.Args[0] != "pbcopy" {
			t.Errorf("expected pbcopy, got %q", cmd.Args[0])
		}
	}
	// On other platforms the result depends on the environment; just ensure
	// the call does not panic.
}

func TestClipboardWriteNoPanic(t *testing.T) {
	// Runs the clipboard command in a background goroutine; must not panic
	// even when no clipboard tool is available.
	clipboardWrite("hello clipboard")
}

// TestClipboardCmd_Darwin verifies that on macOS the function returns pbcopy.
func TestClipboardCmd_Darwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only test")
	}
	cmd := clipboardCmd()
	if cmd == nil {
		t.Fatal("clipboardCmd on darwin: expected non-nil command")
	}
	if !strings.HasSuffix(cmd.Path, "pbcopy") {
		t.Errorf("clipboardCmd darwin: want pbcopy, got %q", cmd.Path)
	}
}

// TestClipboardCmd_NoneOnNonDarwinWithoutDisplay verifies that when
// WAYLAND_DISPLAY and DISPLAY are both unset (and we are not on macOS),
// clipboardCmd returns nil.
func TestClipboardCmd_NoneOnNonDarwinWithoutDisplay(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("macOS always returns pbcopy; skip nil-return test")
	}
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")

	cmd := clipboardCmd()
	if cmd != nil {
		t.Errorf("clipboardCmd with no display env: expected nil, got %v", cmd.Path)
	}
}

// TestClipboardCmd_WaylandWhenWlCopyPresent checks the Wayland branch when
// wl-copy is available.  On CI/macOS, wl-copy is absent so the test is skipped.
func TestClipboardCmd_WaylandWhenWlCopyPresent(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("darwin: always returns pbcopy, not wl-copy")
	}
	// Locate wl-copy; skip if not installed.
	wlCopy, err := findInPATH("wl-copy")
	if err != nil || wlCopy == "" {
		t.Skip("wl-copy not found in PATH")
	}

	t.Setenv("WAYLAND_DISPLAY", ":0")

	cmd := clipboardCmd()
	if cmd == nil {
		t.Fatal("clipboardCmd with WAYLAND_DISPLAY and wl-copy: expected non-nil")
	}
	if !strings.HasSuffix(cmd.Path, "wl-copy") {
		t.Errorf("want wl-copy, got %q", cmd.Path)
	}
}

// TestClipboardCmd_X11WhenXclipPresent checks the X11/xclip branch.
func TestClipboardCmd_X11WhenXclipPresent(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("darwin: always returns pbcopy")
	}
	xclip, err := findInPATH("xclip")
	if err != nil || xclip == "" {
		t.Skip("xclip not found in PATH")
	}

	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", ":0")

	cmd := clipboardCmd()
	if cmd == nil {
		t.Fatal("clipboardCmd with DISPLAY and xclip: expected non-nil")
	}
	if !strings.HasSuffix(cmd.Path, "xclip") {
		t.Errorf("want xclip, got %q", cmd.Path)
	}
}

// findInPATH looks up a binary in PATH; returns "" if not found (no error).
func findInPATH(name string) (string, error) {
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		full := filepath.Join(dir, name)
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			return full, nil
		}
	}
	return "", nil
}
