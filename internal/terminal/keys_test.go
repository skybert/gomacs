package terminal

import (
	"testing"

	"github.com/gdamore/tcell/v3"
)

// ---- ParseKey: Ctrl+letter -------------------------------------------------

func TestParseKeyCtrlA(t *testing.T) {
	ev := tcell.NewEventKey(tcell.KeyCtrlA, "a", tcell.ModCtrl)
	ke := ParseKey(ev)
	if ke.Key != tcell.KeyCtrlA {
		t.Errorf("ParseKey CtrlA: key = %v, want KeyCtrlA", ke.Key)
	}
	// ModCtrl should be stripped from Mod (it's encoded in Key).
	if ke.Mod&tcell.ModCtrl != 0 {
		t.Error("ParseKey CtrlA: ModCtrl should be stripped from Mod")
	}
	if ke.Rune != 0 {
		t.Errorf("ParseKey CtrlA: rune = %v, want 0", ke.Rune)
	}
}

func TestParseKeyCtrlLetterAll(t *testing.T) {
	// Every Ctrl+letter constant should have Rune=0 and ModCtrl stripped.
	ctrlKeys := []tcell.Key{
		tcell.KeyCtrlA, tcell.KeyCtrlB, tcell.KeyCtrlC, tcell.KeyCtrlD,
		tcell.KeyCtrlE, tcell.KeyCtrlF, tcell.KeyCtrlG, tcell.KeyCtrlH,
		tcell.KeyCtrlI, tcell.KeyCtrlJ, tcell.KeyCtrlK, tcell.KeyCtrlL,
		tcell.KeyCtrlM, tcell.KeyCtrlN, tcell.KeyCtrlO, tcell.KeyCtrlP,
		tcell.KeyCtrlQ, tcell.KeyCtrlR, tcell.KeyCtrlS, tcell.KeyCtrlT,
		tcell.KeyCtrlU, tcell.KeyCtrlV, tcell.KeyCtrlW, tcell.KeyCtrlX,
		tcell.KeyCtrlY, tcell.KeyCtrlZ,
	}
	for _, k := range ctrlKeys {
		ev := tcell.NewEventKey(k, "", tcell.ModCtrl)
		ke := ParseKey(ev)
		if ke.Key != k {
			t.Errorf("ParseKey %v: key = %v, want %v", k, ke.Key, k)
		}
		if ke.Rune != 0 {
			t.Errorf("ParseKey %v: rune = %v, want 0", k, ke.Rune)
		}
		if ke.Mod&tcell.ModCtrl != 0 {
			t.Errorf("ParseKey %v: ModCtrl should be stripped from Mod", k)
		}
	}
}

func TestParseKeyCtrlSlash(t *testing.T) {
	// In tcell v3, C-/ is delivered as {KeyRune, "/", ModCtrl}.
	ev := tcell.NewEventKey(tcell.KeyRune, "/", tcell.ModCtrl)
	ke := ParseKey(ev)
	if ke.Key != tcell.KeyRune {
		t.Errorf("ParseKey C-/: key = %v, want KeyRune", ke.Key)
	}
	if ke.Rune != '/' {
		t.Errorf("ParseKey C-/: rune = %v, want '/'", ke.Rune)
	}
	if ke.Mod&tcell.ModCtrl == 0 {
		t.Error("ParseKey C-/: ModCtrl should be present")
	}
}

func TestParseKeyCtrlSpace(t *testing.T) {
	// In tcell v3, C-SPC is delivered as {KeyRune, " ", ModCtrl}.
	ev := tcell.NewEventKey(tcell.KeyRune, " ", tcell.ModCtrl)
	ke := ParseKey(ev)
	if ke.Key != tcell.KeyRune {
		t.Errorf("ParseKey C-SPC: key = %v, want KeyRune", ke.Key)
	}
	if ke.Rune != ' ' {
		t.Errorf("ParseKey C-SPC: rune = %v, want ' '", ke.Rune)
	}
	if ke.Mod&tcell.ModCtrl == 0 {
		t.Error("ParseKey C-SPC: ModCtrl should be present")
	}
}

func TestParseKeyCtrlBackslash(t *testing.T) {
	// In tcell v3, C-\ is delivered as {KeyRune, "\\", ModCtrl}.
	ev := tcell.NewEventKey(tcell.KeyRune, "\\", tcell.ModCtrl)
	ke := ParseKey(ev)
	if ke.Key != tcell.KeyRune {
		t.Errorf("ParseKey C-\\: key = %v, want KeyRune", ke.Key)
	}
	if ke.Rune != '\\' {
		t.Errorf("ParseKey C-\\: rune = %v, want '\\\\'", ke.Rune)
	}
	if ke.Mod&tcell.ModCtrl == 0 {
		t.Error("ParseKey C-\\: ModCtrl should be present")
	}
}

// ---- ParseKey: plain / shifted runes ---------------------------------------

func TestParseKeyRune(t *testing.T) {
	ev := tcell.NewEventKey(tcell.KeyRune, "a", 0)
	ke := ParseKey(ev)
	if ke.Key != tcell.KeyRune {
		t.Errorf("ParseKey 'a': key = %v, want KeyRune", ke.Key)
	}
	if ke.Rune != 'a' {
		t.Errorf("ParseKey 'a': rune = %v, want 'a'", ke.Rune)
	}
}

func TestParseKeyRuneStripsModShift(t *testing.T) {
	ev := tcell.NewEventKey(tcell.KeyRune, "<", tcell.ModShift|tcell.ModAlt)
	ke := ParseKey(ev)
	if ke.Mod&tcell.ModShift != 0 {
		t.Error("ParseKey rune with ModShift: ModShift should be stripped")
	}
	if ke.Mod&tcell.ModAlt == 0 {
		t.Error("ParseKey rune with ModAlt: ModAlt should be preserved")
	}
}

func TestParseKeyRuneShiftOnlyStripped(t *testing.T) {
	// A plain shifted rune (e.g. 'A' from Shift+a) should have ModShift
	// stripped entirely, since the rune itself already encodes the shift.
	ev := tcell.NewEventKey(tcell.KeyRune, "A", tcell.ModShift)
	ke := ParseKey(ev)
	if ke.Rune != 'A' {
		t.Errorf("ParseKey 'A': rune = %v, want 'A'", ke.Rune)
	}
	if ke.Mod != 0 {
		t.Errorf("ParseKey 'A': mod = %v, want 0 (ModShift stripped)", ke.Mod)
	}
}

// ---- ParseKey: Meta/Alt normalisation --------------------------------------

func TestParseKeyNormalisesModMeta(t *testing.T) {
	ev := tcell.NewEventKey(tcell.KeyRune, "f", tcell.ModMeta)
	ke := ParseKey(ev)
	if ke.Mod&tcell.ModAlt == 0 {
		t.Error("ParseKey ModMeta: should be normalised to ModAlt")
	}
	if ke.Mod&tcell.ModMeta != 0 {
		t.Error("ParseKey ModMeta: ModMeta should be cleared")
	}
}

func TestParseKeyModAltPreserved(t *testing.T) {
	ev := tcell.NewEventKey(tcell.KeyRune, "f", tcell.ModAlt)
	ke := ParseKey(ev)
	if ke.Mod&tcell.ModAlt == 0 {
		t.Error("ParseKey ModAlt: ModAlt should be preserved")
	}
}

func TestParseKeyModAltAndModMetaBothSet(t *testing.T) {
	// Some terminals report both ModAlt and ModMeta for the same Meta chord;
	// the result should still be exactly ModAlt (no duplicate bits, no panic).
	ev := tcell.NewEventKey(tcell.KeyRune, "x", tcell.ModAlt|tcell.ModMeta)
	ke := ParseKey(ev)
	if ke.Mod&tcell.ModMeta != 0 {
		t.Error("ParseKey ModAlt|ModMeta: ModMeta should be cleared")
	}
	if ke.Mod&tcell.ModAlt == 0 {
		t.Error("ParseKey ModAlt|ModMeta: ModAlt should remain set")
	}
}

func TestParseKeyMetaShiftCombo(t *testing.T) {
	// M-< arrives as ModMeta|ModShift with rune '<'; after normalisation it
	// should carry only ModAlt so MetaKey('<') bindings match.
	ev := tcell.NewEventKey(tcell.KeyRune, "<", tcell.ModMeta|tcell.ModShift)
	ke := ParseKey(ev)
	if ke.Mod != tcell.ModAlt {
		t.Errorf("ParseKey M-<: mod = %v, want ModAlt only", ke.Mod)
	}
	if ke.Rune != '<' {
		t.Errorf("ParseKey M-<: rune = %v, want '<'", ke.Rune)
	}
}

// ---- ParseKey: special named keys (no rune) --------------------------------

func TestParseKeySpecialNamedKeysCarryNoRune(t *testing.T) {
	tests := []struct {
		name string
		key  tcell.Key
	}{
		{"Backspace", tcell.KeyBackspace},
		{"Delete", tcell.KeyDelete},
		{"Enter", tcell.KeyEnter},
		{"Tab", tcell.KeyTab},
		{"Escape", tcell.KeyEscape},
		{"Up", tcell.KeyUp},
		{"Down", tcell.KeyDown},
		{"Left", tcell.KeyLeft},
		{"Right", tcell.KeyRight},
		{"Home", tcell.KeyHome},
		{"End", tcell.KeyEnd},
		{"PgUp", tcell.KeyPgUp},
		{"PgDn", tcell.KeyPgDn},
		{"Insert", tcell.KeyInsert},
		{"F1", tcell.KeyF1},
		{"F2", tcell.KeyF2},
		{"F3", tcell.KeyF3},
		{"F4", tcell.KeyF4},
		{"F5", tcell.KeyF5},
		{"F6", tcell.KeyF6},
		{"F7", tcell.KeyF7},
		{"F8", tcell.KeyF8},
		{"F9", tcell.KeyF9},
		{"F10", tcell.KeyF10},
		{"F11", tcell.KeyF11},
		{"F12", tcell.KeyF12},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ev := tcell.NewEventKey(tc.key, "", 0)
			ke := ParseKey(ev)
			if ke.Key != tc.key {
				t.Errorf("ParseKey %s: key = %v, want %v", tc.name, ke.Key, tc.key)
			}
			if ke.Rune != 0 {
				t.Errorf("ParseKey %s: rune = %v, want 0", tc.name, ke.Rune)
			}
		})
	}
}

func TestParseKeyArrow(t *testing.T) {
	ev := tcell.NewEventKey(tcell.KeyUp, "", 0)
	ke := ParseKey(ev)
	if ke.Key != tcell.KeyUp {
		t.Errorf("ParseKey Up: key = %v, want KeyUp", ke.Key)
	}
	if ke.Rune != 0 {
		t.Errorf("ParseKey Up: rune = %v, want 0", ke.Rune)
	}
}

func TestParseKeyF1(t *testing.T) {
	ev := tcell.NewEventKey(tcell.KeyF1, "", 0)
	ke := ParseKey(ev)
	if ke.Key != tcell.KeyF1 {
		t.Errorf("ParseKey F1: key = %v, want KeyF1", ke.Key)
	}
	if ke.Rune != 0 {
		t.Errorf("ParseKey F1: rune = %v, want 0", ke.Rune)
	}
}

// ---- ParseKey: special key + modifier edge cases ---------------------------

func TestParseKeyBackspaceWithModAlt(t *testing.T) {
	// M-Backspace (backward-kill-word) must retain ModAlt while still
	// clearing any stray rune.
	ev := tcell.NewEventKey(tcell.KeyBackspace, "", tcell.ModAlt)
	ke := ParseKey(ev)
	if ke.Rune != 0 {
		t.Errorf("ParseKey M-Backspace: rune = %v, want 0", ke.Rune)
	}
	if ke.Mod&tcell.ModAlt == 0 {
		t.Error("ParseKey M-Backspace: ModAlt should be preserved")
	}
}

func TestParseKeyCtrlArrow(t *testing.T) {
	// C-Right (forward-word in some terminals) is delivered as KeyRight with
	// ModCtrl set; the named-key branch clears the rune but leaves Mod alone
	// since arrows aren't in the Ctrl-letter branch.
	ev := tcell.NewEventKey(tcell.KeyRight, "", tcell.ModCtrl)
	ke := ParseKey(ev)
	if ke.Key != tcell.KeyRight {
		t.Errorf("ParseKey C-Right: key = %v, want KeyRight", ke.Key)
	}
	if ke.Rune != 0 {
		t.Errorf("ParseKey C-Right: rune = %v, want 0", ke.Rune)
	}
	if ke.Mod&tcell.ModCtrl == 0 {
		t.Error("ParseKey C-Right: ModCtrl should still be present (not stripped for arrows)")
	}
}

// ---- firstRune --------------------------------------------------------------

func TestFirstRuneEmpty(t *testing.T) {
	if got := firstRune(""); got != 0 {
		t.Errorf("firstRune(\"\") = %v, want 0", got)
	}
}

func TestFirstRuneSingle(t *testing.T) {
	if got := firstRune("a"); got != 'a' {
		t.Errorf("firstRune(%q) = %v, want 'a'", "a", got)
	}
}

func TestFirstRuneMultiByte(t *testing.T) {
	// firstRune must decode UTF-8 correctly and return only the first rune.
	if got := firstRune("日本語"); got != '日' {
		t.Errorf("firstRune(%q) = %v, want '日'", "日本語", got)
	}
}

func TestFirstRuneMultiCharString(t *testing.T) {
	// Only the first rune of a multi-character string is returned, even
	// though ev.Str() could in principle carry more than one rune.
	if got := firstRune("ab"); got != 'a' {
		t.Errorf("firstRune(%q) = %v, want 'a'", "ab", got)
	}
}
