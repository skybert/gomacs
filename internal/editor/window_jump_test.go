package editor

import (
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/terminal"
)

// keCtrlG builds a KeyEvent for C-g.
func keCtrlG() terminal.KeyEvent {
	return terminal.KeyEvent{Key: tcell.KeyCtrlG}
}

// ---------------------------------------------------------------------------
// cmdWindowJump
// ---------------------------------------------------------------------------

func TestCmdWindowJump_OneWindow(t *testing.T) {
	e := newTestEditor("hello")
	e.cmdWindowJump()
	if e.message != "Only one window" {
		t.Errorf("expected 'Only one window' message, got %q", e.message)
	}
	if e.windowJumpActive {
		t.Error("windowJumpActive should not be set with one window")
	}
}

func TestCmdWindowJump_TwoWindows_InstantJump(t *testing.T) {
	e := newTestEditor("hello")
	e.cmdSplitWindowBelow()
	first := e.activeWin
	second := e.windows[1]

	e.cmdWindowJump()

	// Should jump instantly without entering overlay mode.
	if e.windowJumpActive {
		t.Error("windowJumpActive should not be set for two-window jump")
	}
	if e.activeWin != second {
		t.Errorf("expected active window to be second, got first=%p active=%p", first, e.activeWin)
	}
	// Jumping again should cycle back.
	e.cmdWindowJump()
	if e.activeWin != first {
		t.Errorf("expected active window to cycle back to first")
	}
}

func TestCmdWindowJump_ThreeWindows_EntersOverlayMode(t *testing.T) {
	e := newTestEditor("hello")
	e.cmdSplitWindowBelow()
	e.cmdSplitWindowBelow()
	if len(e.windows) != 3 {
		t.Fatalf("expected 3 windows, got %d", len(e.windows))
	}
	original := e.activeWin

	e.cmdWindowJump()

	if !e.windowJumpActive {
		t.Error("windowJumpActive should be true with 3+ windows")
	}
	if e.windowJumpMap == nil {
		t.Fatal("windowJumpMap should be non-nil")
	}
	// Active window should not be in the map.
	for r, w := range e.windowJumpMap {
		if w == original {
			t.Errorf("active window must not appear in jump map (key %q)", r)
		}
	}
	// The two non-active windows should each have an entry.
	if len(e.windowJumpMap) != 2 {
		t.Errorf("expected 2 entries in jump map, got %d", len(e.windowJumpMap))
	}
}

// ---------------------------------------------------------------------------
// windowJumpHandleKey
// ---------------------------------------------------------------------------

func TestWindowJumpHandleKey_ValidLetter(t *testing.T) {
	e := newTestEditor("hello")
	e.cmdSplitWindowBelow()
	e.cmdSplitWindowBelow()
	original := e.activeWin

	e.cmdWindowJump()
	if !e.windowJumpActive {
		t.Fatal("expected overlay mode to be active")
	}

	// Pick the first key in the map and press it.
	var targetKey rune
	for r := range e.windowJumpMap {
		targetKey = r
		break
	}

	e.windowJumpHandleKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: targetKey})

	if e.windowJumpActive {
		t.Error("windowJumpActive should be cleared after key press")
	}
	if e.activeWin == original {
		t.Error("expected active window to change after jump key")
	}
}

func TestWindowJumpHandleKey_CtrlG_Cancels(t *testing.T) {
	e := newTestEditor("hello")
	e.cmdSplitWindowBelow()
	e.cmdSplitWindowBelow()
	original := e.activeWin

	e.cmdWindowJump()
	e.windowJumpHandleKey(keCtrlG())

	if e.windowJumpActive {
		t.Error("windowJumpActive should be cleared after C-g")
	}
	if e.activeWin != original {
		t.Error("active window should not change after C-g cancel")
	}
}

func TestWindowJumpHandleKey_UnknownKey_CancelsSilently(t *testing.T) {
	e := newTestEditor("hello")
	e.cmdSplitWindowBelow()
	e.cmdSplitWindowBelow()
	original := e.activeWin

	e.cmdWindowJump()
	// Press a rune that is not in the map (e.g. 'z').
	e.windowJumpHandleKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'z'})

	if e.windowJumpActive {
		t.Error("windowJumpActive should be cleared after unknown key")
	}
	if e.activeWin != original {
		t.Error("active window should not change after unknown key")
	}
}

func TestWindowJumpHandleKey_ModifiedKey_CancelsSilently(t *testing.T) {
	e := newTestEditor("hello")
	e.cmdSplitWindowBelow()
	e.cmdSplitWindowBelow()
	original := e.activeWin

	e.cmdWindowJump()
	// The first home-row key is 'a', but with Alt modifier it should not jump.
	e.windowJumpHandleKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'a', Mod: tcell.ModAlt})

	if e.windowJumpActive {
		t.Error("windowJumpActive should be cleared")
	}
	if e.activeWin != original {
		t.Error("active window should not change for modified key")
	}
}

// ---------------------------------------------------------------------------
// relayoutWindows with mixed-orientation splits
// ---------------------------------------------------------------------------

// TestRelayout_HorizThenVert checks that C-x 2 followed by C-x 3 produces
// the correct window geometry (top window splits side-by-side, bottom stays).
func TestRelayout_HorizThenVert(t *testing.T) {
	e := newTestEditor("hello")
	// Window starts at 80×24.

	// C-x 2: split below.  win0 top=0, h=12; win1 top=12, h=12.
	e.cmdSplitWindowBelow()
	win0 := e.windows[0]
	win1 := e.windows[1]
	e.activeWin = win0

	// C-x 3 on win0: split right.  win0 left=0 w=39; win2 left=40 w=40; both top=0 h=12.
	e.cmdSplitWindowRight()
	win2 := e.windows[2]

	// Trigger a relayout at the original 80×24 area.
	e.relayoutWindows(80, 24)

	// win0 and win2 must share the same top (both in top half).
	if win0.Top() != win2.Top() {
		t.Errorf("win0.Top=%d win2.Top=%d: should share same row", win0.Top(), win2.Top())
	}
	// win1 must be below win0/win2.
	if win1.Top() <= win0.Top() {
		t.Errorf("win1.Top=%d should be below win0.Top=%d", win1.Top(), win0.Top())
	}
	// win0 and win2 must be side by side (different left offsets).
	if win0.Left() == win2.Left() {
		t.Errorf("win0 and win2 should be side-by-side, both at left=%d", win0.Left())
	}
	// win1 must start at left=0 (full width, stacked below).
	if win1.Left() != 0 {
		t.Errorf("win1.Left=%d, expected 0 (full-width stacked row)", win1.Left())
	}
}
