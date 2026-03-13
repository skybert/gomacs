// Package terminal bridges raw tcell key events and our internal
// representation.  All key-event parsing lives here so that the rest of the
// editor never needs to import tcell directly for input handling.
package terminal

import "github.com/gdamore/tcell/v2"

// KeyEvent is our canonical, tcell-independent description of a key chord.
type KeyEvent struct {
	Key  tcell.Key
	Rune rune
	Mod  tcell.ModMask
}

// ParseKey converts a tcell.EventKey into a KeyEvent.
//
// The mapping rules are:
//
//   - Special named keys (arrows, function keys, Backspace, Delete, Enter,
//     Tab, Escape, …) are forwarded with their tcell.Key constant and Rune=0.
//   - Ctrl+letter keys arrive from tcell with Key=tcell.KeyCtrlA…Z; they are
//     forwarded with ModCtrl stripped (the Key constant already encodes ctrl).
//   - Meta/Alt combos: tcell may set ModAlt, ModMeta, or both; we normalise
//     to ModAlt only so that MetaKey() bindings always match.
//   - KeyRune events: the rune already encodes Shift (e.g. '<' vs ','), so
//     ModShift is stripped to allow bindings like MetaKey('<') to work.
//   - All other printable runes: Key=tcell.KeyRune, Rune=<char>.
func ParseKey(ev *tcell.EventKey) KeyEvent {
	ke := KeyEvent{
		Key:  ev.Key(),
		Rune: ev.Rune(),
		Mod:  ev.Modifiers(),
	}

	//nolint:exhaustive // external enum; default case handles unknowns
	switch ev.Key() {
	// --- special named keys that carry no rune ---
	case tcell.KeyBackspace, tcell.KeyBackspace2,
		tcell.KeyDelete,
		tcell.KeyEnter,
		tcell.KeyTab,
		tcell.KeyEscape,
		tcell.KeyUp, tcell.KeyDown, tcell.KeyLeft, tcell.KeyRight,
		tcell.KeyHome, tcell.KeyEnd,
		tcell.KeyPgUp, tcell.KeyPgDn,
		tcell.KeyInsert,
		tcell.KeyF1, tcell.KeyF2, tcell.KeyF3, tcell.KeyF4,
		tcell.KeyF5, tcell.KeyF6, tcell.KeyF7, tcell.KeyF8,
		tcell.KeyF9, tcell.KeyF10, tcell.KeyF11, tcell.KeyF12:
		ke.Rune = 0

	// --- Ctrl+letter (tcell.KeyCtrlA…Z and related) ---
	// The Key constant already encodes the ctrl modifier, so strip ModCtrl
	// from the Mod mask to ensure keymap lookups succeed.
	case tcell.KeyCtrlA, tcell.KeyCtrlB, tcell.KeyCtrlC, tcell.KeyCtrlD,
		tcell.KeyCtrlE, tcell.KeyCtrlF, tcell.KeyCtrlG, tcell.KeyCtrlH,
		tcell.KeyCtrlI, tcell.KeyCtrlJ, tcell.KeyCtrlK, tcell.KeyCtrlL,
		tcell.KeyCtrlM, tcell.KeyCtrlN, tcell.KeyCtrlO, tcell.KeyCtrlP,
		tcell.KeyCtrlQ, tcell.KeyCtrlR, tcell.KeyCtrlS, tcell.KeyCtrlT,
		tcell.KeyCtrlU, tcell.KeyCtrlV, tcell.KeyCtrlW, tcell.KeyCtrlX,
		tcell.KeyCtrlY, tcell.KeyCtrlZ,
		tcell.KeyCtrlSpace,
		tcell.KeyCtrlUnderscore:
		ke.Rune = 0
		ke.Mod &^= tcell.ModCtrl
	}

	// Some terminals (xterm modifyOtherKeys, kitty keyboard protocol) deliver
	// C-/ as {KeyRune, '/', ModCtrl} rather than KeyCtrlUnderscore.
	// Normalise to the canonical form so bindings and FormatKey work correctly.
	if ke.Key == tcell.KeyRune && ke.Mod&tcell.ModCtrl != 0 && ke.Rune == '/' {
		ke.Key = tcell.KeyCtrlUnderscore
		ke.Rune = 0
		ke.Mod &^= tcell.ModCtrl
	}

	// For printable-rune events the rune already encodes any Shift (e.g. '<'
	// vs ',').  Strip ModShift so Meta+Shift combos such as M-< match the
	// MetaKey('<') binding, which carries only ModAlt.
	if ke.Key == tcell.KeyRune {
		ke.Mod &^= tcell.ModShift
	}

	// Normalise ModMeta → ModAlt: Emacs treats both as Meta, but some
	// terminals report ModMeta instead of (or in addition to) ModAlt.
	if ke.Mod&tcell.ModMeta != 0 {
		ke.Mod = (ke.Mod &^ tcell.ModMeta) | tcell.ModAlt
	}

	return ke
}
