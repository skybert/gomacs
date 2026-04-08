//go:build !linux && !darwin

package editor

import (
	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/terminal"
	"github.com/skybert/gomacs/internal/window"
)

// shellState is an empty placeholder on unsupported platforms.
type shellState struct{}

func (s *shellState) close() {}

// cmdShell is not supported on this platform.
func (e *Editor) cmdShell() {
	e.clearArg()
	e.Message("M-x shell is not supported on this platform")
}

// shellDispatch always returns false on unsupported platforms.
func (e *Editor) shellDispatch(_ terminal.KeyEvent) bool { return false }

// renderShellWindow falls back to renderWindow on unsupported platforms.
func (e *Editor) renderShellWindow(w *window.Window) { e.renderWindow(w) }

// shellCursorPos returns -1, -1 on unsupported platforms.
func (e *Editor) shellCursorPos(_ *buffer.Buffer) (int, int) { return -1, -1 }
