package editor

import (
	"context"
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/keymap"
	"github.com/skybert/gomacs/internal/terminal"
	"github.com/skybert/gomacs/internal/window"
)

// newPrefixTestEditor builds an Editor with a three-level keymap tree:
//
//		C-x      → prefix (cx)
//		C-x v    → prefix (cxv)
//		C-x v s  → command "forward-char"   (three-deep, hint shown at C-x v)
//	 C-x f    → command "forward-char"   (two-deep, no hint shown)
//
// "forward-char" and "keyboard-quit" are already registered in the global
// command table via the editor package's init().
func newPrefixTestEditor(t *testing.T) *Editor {
	t.Helper()

	b := buffer.NewWithContent("*test*", "hello")
	win := window.New(b, 0, 0, 80, 24)

	gk := keymap.New("global")
	cx := keymap.New("C-x")
	cxv := keymap.New("C-x v")

	cxv.Bind(keymap.PlainKey('s'), "forward-char")
	cx.BindPrefix(keymap.PlainKey('v'), cxv)
	cx.Bind(keymap.PlainKey('f'), "forward-char")
	gk.BindPrefix(keymap.CtrlKey('x'), cx)
	gk.Bind(keymap.CtrlKey('g'), "keyboard-quit")

	_, nopCancel := context.WithCancel(context.Background())
	e := &Editor{
		buffers:      []*buffer.Buffer{b},
		windows:      []*window.Window{win},
		activeWin:    win,
		minibufBuf:   buffer.New(" *minibuf*"),
		globalKeymap: gk,
		ctrlXKeymap:  cx,
		ctrlCKeymap:  keymap.New("C-c"),
		universalArg: 1,
		lspOpCancel:  nopCancel,
	}
	e.minibufWin = window.New(e.minibufBuf, 23, 0, 80, 1)
	return e
}

func pressCtrlX(e *Editor) {
	e.dispatchParsedKey(terminal.KeyEvent{Key: tcell.KeyCtrlX})
}

func pressRune(e *Editor, r rune) {
	e.dispatchParsedKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: r})
}

// TestPrefixHint_FirstChordSilent: pressing C-x alone must not show a hint.
func TestPrefixHint_FirstChordSilent(t *testing.T) {
	e := newPrefixTestEditor(t)
	pressCtrlX(e)

	if e.prefixKeymap == nil {
		t.Fatal("expected prefixKeymap to be set after C-x")
	}
	if e.message != "" {
		t.Errorf("expected no message after first chord C-x, got %q", e.message)
	}
}

// TestPrefixHint_SecondChordShowsHint: C-x v must show "C-x v" in the minibuffer.
func TestPrefixHint_SecondChordShowsHint(t *testing.T) {
	e := newPrefixTestEditor(t)
	pressCtrlX(e)
	pressRune(e, 'v')

	if e.message != "C-x v" {
		t.Errorf("expected minibuffer to show \"C-x v\", got %q", e.message)
	}
	if e.prefixKeySeq != "C-x v" {
		t.Errorf("prefixKeySeq = %q, want \"C-x v\"", e.prefixKeySeq)
	}
}

// TestPrefixHint_CommandClearsHint: completing C-x v s must clear the hint.
func TestPrefixHint_CommandClearsHint(t *testing.T) {
	e := newPrefixTestEditor(t)
	pressCtrlX(e)
	pressRune(e, 'v')
	pressRune(e, 's')

	if e.prefixKeymap != nil {
		t.Error("prefixKeymap should be nil after command executes")
	}
	if e.prefixKeySeq != "" {
		t.Errorf("prefixKeySeq should be empty after command, got %q", e.prefixKeySeq)
	}
	if e.message != "" {
		t.Errorf("message should be cleared after command, got %q", e.message)
	}
}

// TestPrefixHint_SingleDepthNoHint: C-x f must not show a hint (only one prefix level).
func TestPrefixHint_SingleDepthNoHint(t *testing.T) {
	e := newPrefixTestEditor(t)
	pressCtrlX(e)
	pressRune(e, 'f')

	if e.prefixKeymap != nil {
		t.Error("prefixKeymap should be nil after command executes")
	}
	if e.prefixKeySeq != "" {
		t.Errorf("prefixKeySeq should be empty after command, got %q", e.prefixKeySeq)
	}
	if e.message != "" {
		t.Errorf("no hint was shown, so message should be empty, got %q", e.message)
	}
}

// TestPrefixHint_UnknownKeyCancels: unbound key in a prefix clears the hint.
func TestPrefixHint_UnknownKeyCancels(t *testing.T) {
	e := newPrefixTestEditor(t)
	pressCtrlX(e)
	pressRune(e, 'v')
	pressRune(e, 'z') // not bound under C-x v

	if e.prefixKeymap != nil {
		t.Error("prefixKeymap should be nil after unknown key")
	}
	if e.prefixKeySeq != "" {
		t.Errorf("prefixKeySeq should be empty after cancel, got %q", e.prefixKeySeq)
	}
}

// TestPrefixHint_KeyboardQuitCancels: C-g clears the prefix and the hint.
func TestPrefixHint_KeyboardQuitCancels(t *testing.T) {
	e := newPrefixTestEditor(t)
	pressCtrlX(e)
	pressRune(e, 'v')
	e.dispatchParsedKey(terminal.KeyEvent{Key: tcell.KeyCtrlG})

	if e.prefixKeymap != nil {
		t.Error("prefixKeymap should be nil after C-g")
	}
	if e.prefixKeySeq != "" {
		t.Errorf("prefixKeySeq should be empty after C-g, got %q", e.prefixKeySeq)
	}
}
