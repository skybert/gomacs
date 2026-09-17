//go:build !linux && !darwin

package editor

import (
	"testing"

	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/keymap"
	"github.com/skybert/gomacs/internal/terminal"
	"github.com/skybert/gomacs/internal/window"
)

// newStubShellTestEditor builds a minimal Editor backed by a headless
// capture terminal (terminal.NewCapture), so renderShellWindow's fallback to
// renderWindow has something safe to draw into. Kept local to this file
// rather than reusing a helper from another _test.go, since this file
// targets a platform (!linux && !darwin) nobody else's tests need to run on.
func newStubShellTestEditor(content string) *Editor {
	term := terminal.NewCapture(80, 24)
	buf := buffer.NewWithContent("*shell*", content)
	win := window.New(buf, 0, 0, 80, 23)
	return &Editor{
		term:         term,
		buffers:      []*buffer.Buffer{buf},
		windows:      []*window.Window{win},
		activeWin:    win,
		layoutRoot:   leafNode(win),
		minibufBuf:   buffer.New(" *minibuf*"),
		globalKeymap: keymap.New("global"),
		ctrlXKeymap:  keymap.New("C-x"),
		universalArg: 1,
	}
}

// TestCmdShell_Stub verifies M-x shell reports itself unavailable rather
// than silently doing nothing on platforms without a native PTY.
func TestCmdShell_Stub(t *testing.T) {
	e := newStubShellTestEditor("")
	e.cmdShell()

	const want = "M-x shell is not supported on this platform"
	if e.message != want {
		t.Errorf("message = %q, want %q", e.message, want)
	}
}

// TestShellDispatch_Stub verifies key events are never claimed by the shell
// dispatcher on this platform, so normal editor dispatch always runs.
func TestShellDispatch_Stub(t *testing.T) {
	e := newStubShellTestEditor("")
	if got := e.shellDispatch(terminal.KeyEvent{}); got {
		t.Error("shellDispatch = true, want false on unsupported platform")
	}
}

// TestRenderShellWindow_Stub verifies the stub delegates straight to
// renderWindow instead of panicking or drawing anything shell-specific.
func TestRenderShellWindow_Stub(t *testing.T) {
	e := newStubShellTestEditor("hello")
	e.renderShellWindow(e.activeWin)

	ch, _ := e.term.CaptureCell(0, 0)
	if ch != 'h' {
		t.Errorf("cell (0,0) = %q, want 'h' (plain renderWindow output)", ch)
	}
}

// TestShellCursorPos_Stub verifies the stub always reports "no cursor",
// since there is never a live shellState to look up on this platform.
func TestShellCursorPos_Stub(t *testing.T) {
	e := newStubShellTestEditor("")
	col, row := e.shellCursorPos(e.ActiveBuffer())
	if col != -1 || row != -1 {
		t.Errorf("shellCursorPos = (%d, %d), want (-1, -1)", col, row)
	}
}

// TestShellState_Stub verifies the placeholder shellState.close is callable
// (a no-op) and does not panic.
func TestShellState_Stub(t *testing.T) {
	st := &shellState{}
	st.close()
}
