//go:build linux || darwin

package editor

import (
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/terminal"
)

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
