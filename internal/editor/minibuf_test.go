package editor

import (
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/terminal"
)

// activateMinibuf sets up the minibuffer as if ReadMinibuffer was called.
func activateMinibuf(e *Editor) {
	e.ReadMinibuffer("test: ", func(string) {})
}

// mbKey builds a KeyEvent for dispatchMinibufKey tests.
func mbKey(key tcell.Key) terminal.KeyEvent {
	return terminal.KeyEvent{Key: key}
}

func mbRune(r rune) terminal.KeyEvent {
	return terminal.KeyEvent{Key: tcell.KeyRune, Rune: r}
}

func mbRuneMod(r rune, mod tcell.ModMask) terminal.KeyEvent {
	return terminal.KeyEvent{Key: tcell.KeyRune, Rune: r, Mod: mod}
}

// ---------------------------------------------------------------------------
// Enter / Escape / C-g
// ---------------------------------------------------------------------------

func TestDispatchMinibufKey_Enter_ClosesMinibuf(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.dispatchMinibufKey(mbKey(tcell.KeyEnter))
	if e.minibufActive {
		t.Error("Enter should close minibuffer")
	}
}

func TestDispatchMinibufKey_CtrlJ_ClosesMinibuf(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.dispatchMinibufKey(mbKey(tcell.KeyCtrlJ))
	if e.minibufActive {
		t.Error("C-j should close minibuffer")
	}
}

func TestDispatchMinibufKey_Escape_Cancels(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.dispatchMinibufKey(mbKey(tcell.KeyEscape))
	if e.minibufActive {
		t.Error("Escape should cancel minibuffer")
	}
}

func TestDispatchMinibufKey_CtrlG_Cancels(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.dispatchMinibufKey(mbKey(tcell.KeyCtrlG))
	if e.minibufActive {
		t.Error("C-g should cancel minibuffer")
	}
}

// ---------------------------------------------------------------------------
// Rune insertion
// ---------------------------------------------------------------------------

func TestDispatchMinibufKey_RuneInserts(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.dispatchMinibufKey(mbRune('h'))
	e.dispatchMinibufKey(mbRune('i'))
	if got := e.minibufBuf.String(); got != "hi" {
		t.Errorf("minibuf content = %q, want %q", got, "hi")
	}
}

// ---------------------------------------------------------------------------
// Backspace
// ---------------------------------------------------------------------------

func TestDispatchMinibufKey_Backspace_DeletesChar(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufBuf.InsertString(0, "hello")
	e.minibufBuf.SetPoint(5)
	e.dispatchMinibufKey(mbKey(tcell.KeyBackspace))
	if got := e.minibufBuf.String(); got != "hell" {
		t.Errorf("after backspace: %q, want %q", got, "hell")
	}
}

func TestDispatchMinibufKey_Backspace_AtStart_NoOp(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufBuf.InsertString(0, "x")
	e.minibufBuf.SetPoint(0)
	e.dispatchMinibufKey(mbKey(tcell.KeyBackspace))
	if got := e.minibufBuf.String(); got != "x" {
		t.Errorf("backspace at start changed content to %q", got)
	}
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

func TestDispatchMinibufKey_Delete_DeletesForward(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufBuf.InsertString(0, "hello")
	e.minibufBuf.SetPoint(0)
	e.dispatchMinibufKey(mbKey(tcell.KeyDelete))
	if got := e.minibufBuf.String(); got != "ello" {
		t.Errorf("after delete: %q, want %q", got, "ello")
	}
}

// ---------------------------------------------------------------------------
// Navigation: Left / Right / Home / End / C-a / C-e / C-f / C-b
// ---------------------------------------------------------------------------

func TestDispatchMinibufKey_Left_MovesBack(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufBuf.InsertString(0, "abc")
	e.minibufBuf.SetPoint(3)
	e.dispatchMinibufKey(mbKey(tcell.KeyLeft))
	if got := e.minibufBuf.Point(); got != 2 {
		t.Errorf("Left: point = %d, want 2", got)
	}
}

func TestDispatchMinibufKey_Right_MovesForward(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufBuf.InsertString(0, "abc")
	e.minibufBuf.SetPoint(0)
	e.dispatchMinibufKey(mbKey(tcell.KeyRight))
	if got := e.minibufBuf.Point(); got != 1 {
		t.Errorf("Right: point = %d, want 1", got)
	}
}

func TestDispatchMinibufKey_Home_MovesToStart(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufBuf.InsertString(0, "hello")
	e.minibufBuf.SetPoint(5)
	e.dispatchMinibufKey(mbKey(tcell.KeyHome))
	if got := e.minibufBuf.Point(); got != 0 {
		t.Errorf("Home: point = %d, want 0", got)
	}
}

func TestDispatchMinibufKey_End_MovesToEnd(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufBuf.InsertString(0, "hello")
	e.minibufBuf.SetPoint(0)
	e.dispatchMinibufKey(mbKey(tcell.KeyEnd))
	if got := e.minibufBuf.Point(); got != 5 {
		t.Errorf("End: point = %d, want 5", got)
	}
}

func TestDispatchMinibufKey_CtrlA_MovesToStart(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufBuf.InsertString(0, "hello")
	e.minibufBuf.SetPoint(5)
	e.dispatchMinibufKey(mbKey(tcell.KeyCtrlA))
	if got := e.minibufBuf.Point(); got != 0 {
		t.Errorf("C-a: point = %d, want 0", got)
	}
}

func TestDispatchMinibufKey_CtrlE_MovesToEnd(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufBuf.InsertString(0, "hello")
	e.minibufBuf.SetPoint(0)
	e.dispatchMinibufKey(mbKey(tcell.KeyCtrlE))
	if got := e.minibufBuf.Point(); got != 5 {
		t.Errorf("C-e: point = %d, want 5", got)
	}
}

func TestDispatchMinibufKey_CtrlF_MovesForward(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufBuf.InsertString(0, "abc")
	e.minibufBuf.SetPoint(1)
	e.dispatchMinibufKey(mbKey(tcell.KeyCtrlF))
	if got := e.minibufBuf.Point(); got != 2 {
		t.Errorf("C-f: point = %d, want 2", got)
	}
}

func TestDispatchMinibufKey_CtrlB_MovesBack(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufBuf.InsertString(0, "abc")
	e.minibufBuf.SetPoint(2)
	e.dispatchMinibufKey(mbKey(tcell.KeyCtrlB))
	if got := e.minibufBuf.Point(); got != 1 {
		t.Errorf("C-b: point = %d, want 1", got)
	}
}

// ---------------------------------------------------------------------------
// C-k (kill to end of line)
// ---------------------------------------------------------------------------

func TestDispatchMinibufKey_CtrlK_KillsToEnd(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufBuf.InsertString(0, "hello world")
	e.minibufBuf.SetPoint(5)
	e.dispatchMinibufKey(mbKey(tcell.KeyCtrlK))
	if got := e.minibufBuf.String(); got != "hello" {
		t.Errorf("C-k: content = %q, want %q", got, "hello")
	}
}

// ---------------------------------------------------------------------------
// C-d (delete forward)
// ---------------------------------------------------------------------------

func TestDispatchMinibufKey_CtrlD_DeletesForward(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufBuf.InsertString(0, "hello")
	e.minibufBuf.SetPoint(1)
	e.dispatchMinibufKey(mbKey(tcell.KeyCtrlD))
	if got := e.minibufBuf.String(); got != "hllo" {
		t.Errorf("C-d: content = %q, want %q", got, "hllo")
	}
}

// ---------------------------------------------------------------------------
// M-n / M-p (candidate navigation)
// ---------------------------------------------------------------------------

func TestDispatchMinibufKey_MetaN_SelectsNext(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufCandidates = []string{"alpha", "beta", "gamma"}
	e.minibufSelectedIdx = 0
	e.dispatchMinibufKey(mbRuneMod('n', tcell.ModAlt))
	if e.minibufSelectedIdx != 1 {
		t.Errorf("M-n: selectedIdx = %d, want 1", e.minibufSelectedIdx)
	}
}

func TestDispatchMinibufKey_MetaP_SelectsPrev(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufCandidates = []string{"alpha", "beta", "gamma"}
	e.minibufSelectedIdx = 2
	e.dispatchMinibufKey(mbRuneMod('p', tcell.ModAlt))
	if e.minibufSelectedIdx != 1 {
		t.Errorf("M-p: selectedIdx = %d, want 1", e.minibufSelectedIdx)
	}
}

// ---------------------------------------------------------------------------
// Down / Up with candidates
// ---------------------------------------------------------------------------

func TestDispatchMinibufKey_Down_WithCandidates_SelectsNext(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufCandidates = []string{"a", "b"}
	e.minibufSelectedIdx = 0
	e.dispatchMinibufKey(mbKey(tcell.KeyDown))
	if e.minibufSelectedIdx != 1 {
		t.Errorf("Down: selectedIdx = %d, want 1", e.minibufSelectedIdx)
	}
}

func TestDispatchMinibufKey_Up_WithCandidates_SelectsPrev(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufCandidates = []string{"a", "b"}
	e.minibufSelectedIdx = 1
	e.dispatchMinibufKey(mbKey(tcell.KeyUp))
	if e.minibufSelectedIdx != 0 {
		t.Errorf("Up: selectedIdx = %d, want 0", e.minibufSelectedIdx)
	}
}

// ---------------------------------------------------------------------------
// C-n / C-p
// ---------------------------------------------------------------------------

func TestDispatchMinibufKey_CtrlN_SelectsNext(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufCandidates = []string{"a", "b", "c"}
	e.minibufSelectedIdx = 1
	e.dispatchMinibufKey(mbKey(tcell.KeyCtrlN))
	if e.minibufSelectedIdx != 2 {
		t.Errorf("C-n: selectedIdx = %d, want 2", e.minibufSelectedIdx)
	}
}

func TestDispatchMinibufKey_CtrlP_SelectsPrev(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufCandidates = []string{"a", "b", "c"}
	e.minibufSelectedIdx = 2
	e.dispatchMinibufKey(mbKey(tcell.KeyCtrlP))
	if e.minibufSelectedIdx != 1 {
		t.Errorf("C-p: selectedIdx = %d, want 1", e.minibufSelectedIdx)
	}
}

// ---------------------------------------------------------------------------
// C-w (kill word backward)
// ---------------------------------------------------------------------------

func TestDispatchMinibufKey_CtrlW_KillsWordBackward(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufBuf.InsertString(0, "hello world")
	e.minibufBuf.SetPoint(11)
	e.dispatchMinibufKey(mbKey(tcell.KeyCtrlW))
	// "world" should be killed.
	got := e.minibufBuf.String()
	if len(got) >= 11 {
		t.Errorf("C-w did not kill word, content = %q", got)
	}
}

// ---------------------------------------------------------------------------
// Unknown key (not a rune, not handled) — no-op
// ---------------------------------------------------------------------------

func TestDispatchMinibufKey_UnknownKey_NoOp(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufBuf.InsertString(0, "abc")
	e.minibufBuf.SetPoint(3)
	// KeyF1 is not handled → default branch, returns early because not KeyRune.
	e.dispatchMinibufKey(terminal.KeyEvent{Key: tcell.KeyF1})
	if got := e.minibufBuf.String(); got != "abc" {
		t.Errorf("unknown key changed minibuf to %q", got)
	}
	if !e.minibufActive {
		t.Error("unknown key should not close minibuffer")
	}
}

// ---------------------------------------------------------------------------
// Hint cleared on non-TAB keypresses
// ---------------------------------------------------------------------------

func TestDispatchMinibufKey_HintClearedOnNonTab(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufHint = "some hint"
	e.dispatchMinibufKey(mbRune('a'))
	if e.minibufHint != "" {
		t.Errorf("minibufHint = %q, want empty after non-TAB key", e.minibufHint)
	}
}
