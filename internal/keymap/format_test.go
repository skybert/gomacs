package keymap

import (
	"testing"

	"github.com/gdamore/tcell/v3"
)

func TestFormatKeyRune(t *testing.T) {
	tests := []struct {
		key  ModKey
		want string
	}{
		{PlainKey('a'), "a"},
		{PlainKey('Z'), "Z"},
		{PlainKey(' '), " "},
	}
	for _, tc := range tests {
		if got := FormatKey(tc.key); got != tc.want {
			t.Errorf("FormatKey(%v) = %q, want %q", tc.key, got, tc.want)
		}
	}
}

func TestFormatKeyMeta(t *testing.T) {
	mk := MetaKey('f')
	if got := FormatKey(mk); got != "M-f" {
		t.Errorf("FormatKey(M-f) = %q, want %q", got, "M-f")
	}
}

func TestFormatKeyCtrlLetter(t *testing.T) {
	tests := []struct {
		key  ModKey
		want string
	}{
		{CtrlKey('a'), "C-a"},
		{CtrlKey('x'), "C-x"},
		{CtrlKey('g'), "C-g"},
	}
	for _, tc := range tests {
		if got := FormatKey(tc.key); got != tc.want {
			t.Errorf("FormatKey(%v) = %q, want %q", tc.key, got, tc.want)
		}
	}
}

func TestFormatKeyCtrlSpace(t *testing.T) {
	mk := CtrlKey(' ')
	if got := FormatKey(mk); got != "C-SPC" {
		t.Errorf("FormatKey(C-SPC) = %q, want %q", got, "C-SPC")
	}
}

func TestFormatKeyCtrlSlash(t *testing.T) {
	mk := CtrlKey('/')
	if got := FormatKey(mk); got != "C-/" {
		t.Errorf("FormatKey(C-/) = %q, want %q", got, "C-/")
	}
}

func TestFormatKeySpecialKeys(t *testing.T) {
	tests := []struct {
		key  ModKey
		want string
	}{
		{ModKey{Key: tcell.KeyEnter}, "RET"},
		{ModKey{Key: tcell.KeyTab}, "TAB"},
		{ModKey{Key: tcell.KeyEscape}, "ESC"},
		{ModKey{Key: tcell.KeyBackspace}, "DEL"},
		{ModKey{Key: tcell.KeyDelete}, "<delete>"},
		{ModKey{Key: tcell.KeyUp}, "<up>"},
		{ModKey{Key: tcell.KeyDown}, "<down>"},
		{ModKey{Key: tcell.KeyLeft}, "<left>"},
		{ModKey{Key: tcell.KeyRight}, "<right>"},
		{ModKey{Key: tcell.KeyHome}, "<home>"},
		{ModKey{Key: tcell.KeyEnd}, "<end>"},
		{ModKey{Key: tcell.KeyPgUp}, "<prior>"},
		{ModKey{Key: tcell.KeyPgDn}, "<next>"},
		{ModKey{Key: tcell.KeyF1}, "<f1>"},
		{ModKey{Key: tcell.KeyF12}, "<f12>"},
	}
	for _, tc := range tests {
		if got := FormatKey(tc.key); got != tc.want {
			t.Errorf("FormatKey(%v) = %q, want %q", tc.key, got, tc.want)
		}
	}
}
