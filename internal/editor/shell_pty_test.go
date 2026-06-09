//go:build linux || darwin

package editor

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/terminal"
)

func TestKeyToShellBytes_AllSpecialKeys(t *testing.T) {
	cases := []struct {
		key  tcell.Key
		want []byte
	}{
		{tcell.KeyF2, []byte{0x1b, 'O', 'Q'}},
		{tcell.KeyF3, []byte{0x1b, 'O', 'R'}},
		{tcell.KeyF4, []byte{0x1b, 'O', 'S'}},
		{tcell.KeyF5, []byte{0x1b, '[', '1', '5', '~'}},
		{tcell.KeyF6, []byte{0x1b, '[', '1', '7', '~'}},
		{tcell.KeyF7, []byte{0x1b, '[', '1', '8', '~'}},
		{tcell.KeyF8, []byte{0x1b, '[', '1', '9', '~'}},
		{tcell.KeyF9, []byte{0x1b, '[', '2', '0', '~'}},
		{tcell.KeyF10, []byte{0x1b, '[', '2', '1', '~'}},
		{tcell.KeyF11, []byte{0x1b, '[', '2', '3', '~'}},
		{tcell.KeyF12, []byte{0x1b, '[', '2', '4', '~'}},
		{tcell.KeyCtrlB, []byte{2}},
		{tcell.KeyCtrlC, []byte{3}},
		{tcell.KeyCtrlD, []byte{4}},
		{tcell.KeyCtrlE, []byte{5}},
		{tcell.KeyCtrlF, []byte{6}},
		{tcell.KeyCtrlG, []byte{7}},
		{tcell.KeyCtrlH, []byte{8}},
		{tcell.KeyCtrlK, []byte{11}},
		{tcell.KeyCtrlL, []byte{12}},
		{tcell.KeyCtrlN, []byte{14}},
		{tcell.KeyCtrlO, []byte{15}},
		{tcell.KeyCtrlP, []byte{16}},
		{tcell.KeyCtrlQ, []byte{17}},
		{tcell.KeyCtrlR, []byte{18}},
		{tcell.KeyCtrlS, []byte{19}},
		{tcell.KeyCtrlT, []byte{20}},
		{tcell.KeyCtrlU, []byte{21}},
		{tcell.KeyCtrlW, []byte{23}},
		{tcell.KeyCtrlY, []byte{25}},
		{tcell.KeyCtrlZ, []byte{26}},
	}
	for _, tc := range cases {
		got := keyToShellBytes(terminal.KeyEvent{Key: tc.key})
		if !bytes.Equal(got, tc.want) {
			t.Errorf("key %v: got %v, want %v", tc.key, got, tc.want)
		}
	}
}

func TestKeyToShellBytes_UnknownReturnsNil(t *testing.T) {
	if got := keyToShellBytes(terminal.KeyEvent{Key: tcell.KeyF20}); got != nil {
		t.Errorf("unmapped key should return nil, got %v", got)
	}
}

func TestKeyToShellBytes_PrintableRune(t *testing.T) {
	ke := terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'A', Mod: 0}
	got := keyToShellBytes(ke)
	if string(got) != "A" {
		t.Errorf("rune 'A': got %q, want \"A\"", got)
	}
}

func TestKeyToShellBytes_Unicode(t *testing.T) {
	ke := terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'ø', Mod: 0}
	got := keyToShellBytes(ke)
	if string(got) != "ø" {
		t.Errorf("rune 'ø': got %q, want \"ø\"", got)
	}
}

func TestKeyToShellBytes_CtrlRune_Lowercase(t *testing.T) {
	ke := terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'c', Mod: tcell.ModCtrl}
	got := keyToShellBytes(ke)
	if len(got) != 1 || got[0] != 3 {
		t.Errorf("C-c: got %v, want [3]", got)
	}
}

func TestKeyToShellBytes_CtrlRune_Uppercase(t *testing.T) {
	ke := terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'C', Mod: tcell.ModCtrl}
	got := keyToShellBytes(ke)
	if len(got) != 1 || got[0] != 3 {
		t.Errorf("C-C: got %v, want [3]", got)
	}
}

func TestKeyToShellBytes_MetaRune(t *testing.T) {
	ke := terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'b', Mod: tcell.ModAlt}
	got := keyToShellBytes(ke)
	if len(got) != 2 || got[0] != 0x1b || got[1] != 'b' {
		t.Errorf("M-b: got %v, want [ESC 'b']", got)
	}
}

func TestKeyToShellBytes_Enter(t *testing.T) {
	ke := terminal.KeyEvent{Key: tcell.KeyEnter}
	got := keyToShellBytes(ke)
	if len(got) != 1 || got[0] != '\r' {
		t.Errorf("Enter: got %v, want [\\r]", got)
	}
}

func TestKeyToShellBytes_Tab(t *testing.T) {
	ke := terminal.KeyEvent{Key: tcell.KeyTab}
	got := keyToShellBytes(ke)
	if len(got) != 1 || got[0] != '\t' {
		t.Errorf("Tab: got %v, want [\\t]", got)
	}
}

func TestKeyToShellBytes_Backspace(t *testing.T) {
	ke := terminal.KeyEvent{Key: tcell.KeyBackspace}
	got := keyToShellBytes(ke)
	if len(got) != 1 || got[0] != 0x7f {
		t.Errorf("Backspace: got %v, want [0x7f]", got)
	}
}

func TestKeyToShellBytes_Escape(t *testing.T) {
	ke := terminal.KeyEvent{Key: tcell.KeyEscape}
	got := keyToShellBytes(ke)
	if len(got) != 1 || got[0] != 0x1b {
		t.Errorf("Escape: got %v, want [0x1b]", got)
	}
}

func TestKeyToShellBytes_Delete(t *testing.T) {
	ke := terminal.KeyEvent{Key: tcell.KeyDelete}
	got := keyToShellBytes(ke)
	if string(got) != "\x1b[3~" {
		t.Errorf("Delete: got %q, want ESC[3~", got)
	}
}

func TestKeyToShellBytes_ArrowUp(t *testing.T) {
	ke := terminal.KeyEvent{Key: tcell.KeyUp}
	got := keyToShellBytes(ke)
	if string(got) != "\x1b[A" {
		t.Errorf("Up: got %q, want ESC[A", got)
	}
}

func TestKeyToShellBytes_ArrowDown(t *testing.T) {
	ke := terminal.KeyEvent{Key: tcell.KeyDown}
	got := keyToShellBytes(ke)
	if string(got) != "\x1b[B" {
		t.Errorf("Down: got %q, want ESC[B", got)
	}
}

func TestKeyToShellBytes_ArrowRight(t *testing.T) {
	ke := terminal.KeyEvent{Key: tcell.KeyRight}
	got := keyToShellBytes(ke)
	if string(got) != "\x1b[C" {
		t.Errorf("Right: got %q, want ESC[C", got)
	}
}

func TestKeyToShellBytes_ArrowLeft(t *testing.T) {
	ke := terminal.KeyEvent{Key: tcell.KeyLeft}
	got := keyToShellBytes(ke)
	if string(got) != "\x1b[D" {
		t.Errorf("Left: got %q, want ESC[D", got)
	}
}

func TestKeyToShellBytes_Home(t *testing.T) {
	ke := terminal.KeyEvent{Key: tcell.KeyHome}
	got := keyToShellBytes(ke)
	if string(got) != "\x1b[H" {
		t.Errorf("Home: got %q, want ESC[H", got)
	}
}

func TestKeyToShellBytes_End(t *testing.T) {
	ke := terminal.KeyEvent{Key: tcell.KeyEnd}
	got := keyToShellBytes(ke)
	if string(got) != "\x1b[F" {
		t.Errorf("End: got %q, want ESC[F", got)
	}
}

func TestKeyToShellBytes_PageUp(t *testing.T) {
	ke := terminal.KeyEvent{Key: tcell.KeyPgUp}
	got := keyToShellBytes(ke)
	if string(got) != "\x1b[5~" {
		t.Errorf("PageUp: got %q, want ESC[5~", got)
	}
}

func TestKeyToShellBytes_PageDown(t *testing.T) {
	ke := terminal.KeyEvent{Key: tcell.KeyPgDn}
	got := keyToShellBytes(ke)
	if string(got) != "\x1b[6~" {
		t.Errorf("PageDown: got %q, want ESC[6~", got)
	}
}

func TestKeyToShellBytes_F1(t *testing.T) {
	ke := terminal.KeyEvent{Key: tcell.KeyF1}
	got := keyToShellBytes(ke)
	if string(got) != "\x1bOP" {
		t.Errorf("F1: got %q, want ESC OP", got)
	}
}

func TestKeyToShellBytes_CtrlA(t *testing.T) {
	ke := terminal.KeyEvent{Key: tcell.KeyCtrlA}
	got := keyToShellBytes(ke)
	if len(got) != 1 || got[0] != 1 {
		t.Errorf("C-a: got %v, want [1]", got)
	}
}

func TestKeyToShellBytes_CtrlC(t *testing.T) {
	ke := terminal.KeyEvent{Key: tcell.KeyCtrlC}
	got := keyToShellBytes(ke)
	if len(got) != 1 || got[0] != 3 {
		t.Errorf("C-c: got %v, want [3]", got)
	}
}

func TestKeyToShellBytes_CtrlD(t *testing.T) {
	ke := terminal.KeyEvent{Key: tcell.KeyCtrlD}
	got := keyToShellBytes(ke)
	if len(got) != 1 || got[0] != 4 {
		t.Errorf("C-d: got %v, want [4]", got)
	}
}

func TestKeyToShellBytes_CtrlZ(t *testing.T) {
	ke := terminal.KeyEvent{Key: tcell.KeyCtrlZ}
	got := keyToShellBytes(ke)
	if len(got) != 1 || got[0] != 26 {
		t.Errorf("C-z: got %v, want [26]", got)
	}
}

func TestKeyToShellBytes_Unknown_ReturnsNil(t *testing.T) {
	// An unrecognised key should produce nil (nothing to send).
	ke := terminal.KeyEvent{Key: tcell.KeyF12 + 100}
	got := keyToShellBytes(ke)
	if got != nil {
		t.Errorf("unknown key: got %v, want nil", got)
	}
}

// ---------------------------------------------------------------------------
// shellState / shellDispatch / renderShellWindow / shellCursorPos
// ---------------------------------------------------------------------------

// newShellStateEditor builds a capture editor with a synthetic shell buffer
// whose PTY master is the write end of an os.Pipe (so Write succeeds without a
// real shell process).
func newShellStateEditor(t *testing.T) (*Editor, *buffer.Buffer, *shellState, *os.File) {
	t.Helper()
	e := newCapTestEditor("")
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	st := &shellState{master: pw, vt: newVTScreen(23, 80)}
	sb := buffer.New("*shell*")
	sb.SetMode("shell")
	e.buffers = append(e.buffers, sb)
	e.activeWin.SetBuf(sb)
	e.shellStates = map[*buffer.Buffer]*shellState{sb: st}
	return e, sb, st, pr
}

func TestShellDispatch_WritesRune(t *testing.T) {
	e, _, _, pr := newShellStateEditor(t)
	defer func() { _ = pr.Close() }()
	done := make(chan []byte, 1)
	go func() {
		b := make([]byte, 8)
		n, _ := pr.Read(b)
		done <- b[:n]
	}()
	if !e.shellDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'a'}) {
		t.Fatal("a rune should be written to the PTY and handled")
	}
	got := <-done
	if string(got) != "a" {
		t.Fatalf("expected 'a' written to PTY, got %q", got)
	}
}

func TestShellDispatch_ReservedKeys(t *testing.T) {
	e, _, _, pr := newShellStateEditor(t)
	defer func() { _ = pr.Close() }()
	// M-x falls through to normal dispatch.
	if e.shellDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'x', Mod: tcell.ModAlt}) {
		t.Fatal("M-x should not be consumed by the shell")
	}
	// C-v (scroll) falls through.
	if e.shellDispatch(terminal.KeyEvent{Key: tcell.KeyCtrlV}) {
		t.Fatal("C-v should not be consumed by the shell")
	}
}

func TestShellDispatch_NoState(t *testing.T) {
	e := newCapTestEditor("")
	if e.shellDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'a'}) {
		t.Fatal("dispatch on a non-shell buffer should return false")
	}
}

func TestRenderShellWindow(t *testing.T) {
	e, _, st, pr := newShellStateEditor(t)
	defer func() { _ = pr.Close() }()
	// Feed some output into the vt screen.
	st.vt.write([]byte("hello"))
	e.renderShellWindow(e.activeWin)
	// The first cell should render the first character.
	ch, _ := e.term.CaptureCell(0, 0)
	if ch != 'h' {
		t.Fatalf("expected 'h' rendered at (0,0), got %q", ch)
	}
}

func TestRenderShellWindow_NoStateFallsBack(t *testing.T) {
	e := newCapTestEditor("plain text")
	// Active buffer has no shell state → falls back to renderWindow (no panic).
	e.renderShellWindow(e.activeWin)
}

func TestShellCursorPos(t *testing.T) {
	e, sb, _, pr := newShellStateEditor(t)
	defer func() { _ = pr.Close() }()
	col, row := e.shellCursorPos(sb)
	if col < 0 || row < 0 {
		t.Fatalf("shellCursorPos should return a valid position, got (%d,%d)", col, row)
	}
	// Buffer with no shell state → (-1,-1).
	other := buffer.New("*other*")
	if c, r := e.shellCursorPos(other); c != -1 || r != -1 {
		t.Fatalf("expected (-1,-1) for non-shell buffer, got (%d,%d)", c, r)
	}
}

func TestShellState_Close(t *testing.T) {
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pr.Close() }()
	st := &shellState{master: pw, vt: newVTScreen(2, 4)}
	st.close() // should close the master without panic
}

func TestCmdShell_SwitchesToExisting(t *testing.T) {
	e := newCapTestEditor("")
	// Pre-create a shell buffer so cmdShell switches to it instead of spawning.
	_, vcRoot := vcFind(e.bufferDir(e.ActiveBuffer()))
	name := "*shell*"
	if vcRoot != "" {
		name = "*shell/" + filepath.Base(vcRoot) + "*"
	}
	sb := buffer.New(name)
	sb.SetMode("shell")
	e.buffers = append(e.buffers, sb)
	e.shellStates = map[*buffer.Buffer]*shellState{}
	e.cmdShell()
	if e.ActiveBuffer().Name() != name {
		t.Fatalf("cmdShell should switch to the existing shell buffer %q, got %q", name, e.ActiveBuffer().Name())
	}
}
