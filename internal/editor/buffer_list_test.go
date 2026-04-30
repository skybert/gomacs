package editor

import (
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/terminal"
)

// makeBufferListEditor builds a test editor that has called cmdListBuffers so
// the *Buffer List* buffer has the real, correctly-formatted content.
func makeBufferListEditor(t *testing.T) *Editor {
	t.Helper()
	e := newTestEditor("hello")
	// Add a second buffer so the list has something to switch to.
	e.buffers = append(e.buffers, newTestEditor("world").buffers[0])
	e.cmdListBuffers()
	// Verify the buffer list was created and is now active.
	if e.ActiveBuffer().Mode() != "buffer-list" {
		t.Fatal("cmdListBuffers did not switch active buffer to buffer-list mode")
	}
	return e
}

func TestBufferListDispatch_N_MovesDown(t *testing.T) {
	e := makeBufferListEditor(t)
	before := e.ActiveBuffer().Point()
	e.bufferListDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'n'})
	if e.ActiveBuffer().Point() <= before {
		t.Error("'n' should advance point to next line")
	}
}

func TestBufferListDispatch_P_MovesUp(t *testing.T) {
	e := makeBufferListEditor(t)
	// Move down first, then back up.
	e.bufferListDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'n'})
	after := e.ActiveBuffer().Point()
	e.bufferListDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'p'})
	if e.ActiveBuffer().Point() >= after {
		t.Error("'p' should retreat point to previous line")
	}
}

func TestBufferListDispatch_Q_SwitchesBuffer(t *testing.T) {
	e := makeBufferListEditor(t)
	e.bufferListDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'q'})
	if e.ActiveBuffer().Mode() == "buffer-list" {
		t.Error("'q' should switch away from *Buffer List*")
	}
}

func TestBufferListDispatch_Enter_OpensBuffer(t *testing.T) {
	e := makeBufferListEditor(t)
	// The first data line is the first buffer (*test*); press Enter on it.
	e.bufferListDispatch(terminal.KeyEvent{Key: tcell.KeyEnter})
	if e.ActiveBuffer().Mode() == "buffer-list" {
		t.Error("Enter should open the buffer on the current line, not stay in buffer-list")
	}
}

func TestBufferListDispatch_Space_MovesDown(t *testing.T) {
	e := makeBufferListEditor(t)
	before := e.ActiveBuffer().Point()
	e.bufferListDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: ' '})
	if e.ActiveBuffer().Point() <= before {
		t.Error("space should advance point (same as n)")
	}
}

func TestBufferListDispatch_IgnoresUnknownKey(t *testing.T) {
	e := makeBufferListEditor(t)
	consumed := e.bufferListDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'z'})
	if consumed {
		t.Error("unknown key 'z' should not be consumed by bufferListDispatch")
	}
}

func TestBufferListDispatch_IgnoresNonRune(t *testing.T) {
	e := makeBufferListEditor(t)
	consumed := e.bufferListDispatch(terminal.KeyEvent{Key: tcell.KeyCtrlG})
	if consumed {
		t.Error("non-rune key should not be consumed by bufferListDispatch")
	}
}
