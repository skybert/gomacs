package editor

import (
	"runtime"
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
