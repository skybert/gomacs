package terminal

import (
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/color"
	"github.com/skybert/gomacs/internal/syntax"
)

// ---- parseColor ------------------------------------------------------------

func TestParseColorEmpty(t *testing.T) {
	if got := parseColor(""); got != color.Default {
		t.Errorf("parseColor(%q) = %v, want ColorDefault", "", got)
	}
}

func TestParseColorDefault(t *testing.T) {
	if got := parseColor("default"); got != color.Default {
		t.Errorf("parseColor(%q) = %v, want ColorDefault", "default", got)
	}
}

func TestParseColorNamedBlack(t *testing.T) {
	if got := parseColor("black"); got != color.Black {
		t.Errorf("parseColor(%q) = %v, want ColorBlack", "black", got)
	}
}

func TestParseColorNamedCaseInsensitive(t *testing.T) {
	if got := parseColor("WHITE"); got != color.White {
		t.Errorf("parseColor(%q) = %v, want ColorWhite", "WHITE", got)
	}
}

func TestParseColorNamedGray(t *testing.T) {
	// Both "gray" and "grey" should map to the same color.
	gray := parseColor("gray")
	grey := parseColor("grey")
	if gray != grey {
		t.Errorf("gray(%v) != grey(%v)", gray, grey)
	}
}

func TestParseColorHex(t *testing.T) {
	// #ff0000 should parse to an RGB red.
	got := parseColor("#ff0000")
	want := tcell.NewRGBColor(0xff, 0x00, 0x00)
	if got != want {
		t.Errorf("parseColor(%q) = %v, want %v", "#ff0000", got, want)
	}
}

func TestParseColorHexVariants(t *testing.T) {
	tests := []struct {
		input   string
		r, g, b int32
	}{
		{"#000000", 0, 0, 0},
		{"#ffffff", 255, 255, 255},
		{"#1a2b3c", 0x1a, 0x2b, 0x3c},
	}
	for _, tc := range tests {
		want := tcell.NewRGBColor(tc.r, tc.g, tc.b)
		if got := parseColor(tc.input); got != want {
			t.Errorf("parseColor(%q) = %v, want %v", tc.input, got, want)
		}
	}
}

func TestParseColorANSIIndex(t *testing.T) {
	got := parseColor("0")
	want := tcell.PaletteColor(0)
	if got != want {
		t.Errorf("parseColor(%q) = %v, want %v", "0", got, want)
	}
	got = parseColor("255")
	want = tcell.PaletteColor(255)
	if got != want {
		t.Errorf("parseColor(%q) = %v, want %v", "255", got, want)
	}
}

func TestParseColorUnknownFallback(t *testing.T) {
	if got := parseColor("notacolor"); got != color.Default {
		t.Errorf("parseColor(%q) = %v, want ColorDefault", "notacolor", got)
	}
}

// ---- faceToStyle -----------------------------------------------------------

func TestFaceToStyleDefault(t *testing.T) {
	face := syntax.Face{}
	style := faceToStyle(face)
	fg := style.GetForeground()
	bg := style.GetBackground()
	if fg != color.Default {
		t.Errorf("default face: fg = %v, want ColorDefault", fg)
	}
	if bg != color.Default {
		t.Errorf("default face: bg = %v, want ColorDefault", bg)
	}
}

func TestFaceToStyleColors(t *testing.T) {
	face := syntax.Face{Fg: "red", Bg: "black"}
	style := faceToStyle(face)
	fg := style.GetForeground()
	bg := style.GetBackground()
	if fg != color.Red {
		t.Errorf("fg = %v, want ColorRed", fg)
	}
	if bg != color.Black {
		t.Errorf("bg = %v, want ColorBlack", bg)
	}
}

func TestFaceToStyleBold(t *testing.T) {
	face := syntax.Face{Bold: true}
	style := faceToStyle(face)
	attrs := style.GetAttributes()
	if attrs&tcell.AttrBold == 0 {
		t.Error("bold face: AttrBold not set")
	}
}

func TestFaceToStyleItalic(t *testing.T) {
	face := syntax.Face{Italic: true}
	style := faceToStyle(face)
	attrs := style.GetAttributes()
	if attrs&tcell.AttrItalic == 0 {
		t.Error("italic face: AttrItalic not set")
	}
}

func TestFaceToStyleUnderline(t *testing.T) {
	face := syntax.Face{Underline: true}
	style := faceToStyle(face)
	if style.GetUnderlineStyle() == tcell.UnderlineStyleNone {
		t.Error("underline face: underline not set")
	}
}

// ---- ParseKey --------------------------------------------------------------

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
