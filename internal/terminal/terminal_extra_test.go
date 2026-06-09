package terminal

import (
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/syntax"
)

// ---- NewCapture / Size / CaptureSize ---------------------------------------

func TestNewCaptureSize(t *testing.T) {
	term := NewCapture(80, 24)
	w, h := term.Size()
	if w != 80 || h != 24 {
		t.Errorf("Size() = (%d,%d), want (80,24)", w, h)
	}
}

func TestCaptureSizeReturnsWidthHeight(t *testing.T) {
	term := NewCapture(120, 40)
	w, h := term.CaptureSize()
	if w != 120 || h != 40 {
		t.Errorf("CaptureSize() = (%d,%d), want (120,40)", w, h)
	}
}

func TestCaptureSizeNonCaptureReturnsZero(t *testing.T) {
	// A Terminal with no capture cells should return (0,0) from CaptureSize.
	term := &Terminal{styleCache: make(map[syntax.Face]tcell.Style)}
	w, h := term.CaptureSize()
	if w != 0 || h != 0 {
		t.Errorf("CaptureSize() on non-capture terminal = (%d,%d), want (0,0)", w, h)
	}
}

// ---- SetCell / CaptureCell -------------------------------------------------

func TestSetCellBasic(t *testing.T) {
	term := NewCapture(10, 5)
	face := syntax.Face{Fg: "red"}
	term.SetCell(3, 2, 'A', face)
	ch, f := term.CaptureCell(3, 2)
	if ch != 'A' {
		t.Errorf("SetCell/CaptureCell rune = %q, want 'A'", ch)
	}
	if f.Fg != "red" {
		t.Errorf("SetCell/CaptureCell face.Fg = %q, want %q", f.Fg, "red")
	}
}

func TestSetCellOverwritesPreviousValue(t *testing.T) {
	term := NewCapture(10, 5)
	term.SetCell(0, 0, 'X', syntax.Face{})
	term.SetCell(0, 0, 'Y', syntax.Face{Fg: "blue"})
	ch, f := term.CaptureCell(0, 0)
	if ch != 'Y' {
		t.Errorf("overwrite: rune = %q, want 'Y'", ch)
	}
	if f.Fg != "blue" {
		t.Errorf("overwrite: face.Fg = %q, want %q", f.Fg, "blue")
	}
}

func TestSetCellOutOfBoundsNoOp(t *testing.T) {
	term := NewCapture(10, 5)
	// These calls should not panic.
	term.SetCell(-1, 0, 'A', syntax.Face{})
	term.SetCell(0, -1, 'A', syntax.Face{})
	term.SetCell(10, 0, 'A', syntax.Face{})
	term.SetCell(0, 5, 'A', syntax.Face{})
}

func TestCaptureCellDefaultForUnwritten(t *testing.T) {
	term := NewCapture(10, 5)
	ch, face := term.CaptureCell(5, 3)
	if ch != ' ' {
		t.Errorf("unwritten cell rune = %q, want ' '", ch)
	}
	if face != (syntax.Face{}) {
		t.Errorf("unwritten cell face = %v, want zero Face", face)
	}
}

func TestCaptureCellOutOfBoundsReturnsDefault(t *testing.T) {
	term := NewCapture(10, 5)
	ch, face := term.CaptureCell(100, 100)
	if ch != ' ' {
		t.Errorf("out-of-bounds CaptureCell rune = %q, want ' '", ch)
	}
	if face != syntax.FaceDefault {
		t.Errorf("out-of-bounds CaptureCell face = %v, want FaceDefault", face)
	}
}

func TestCaptureCellNegativeColRow(t *testing.T) {
	term := NewCapture(10, 5)
	ch, _ := term.CaptureCell(-1, -1)
	if ch != ' ' {
		t.Errorf("negative CaptureCell = %q, want ' '", ch)
	}
}

// ---- DrawString ------------------------------------------------------------

func TestDrawStringBasic(t *testing.T) {
	term := NewCapture(20, 5)
	face := syntax.Face{Fg: "green"}
	term.DrawString(2, 1, "Hello", face)
	for i, want := range "Hello" {
		ch, f := term.CaptureCell(2+i, 1)
		if ch != want {
			t.Errorf("DrawString col %d: rune = %q, want %q", 2+i, ch, want)
		}
		if f.Fg != "green" {
			t.Errorf("DrawString col %d: face.Fg = %q, want %q", 2+i, f.Fg, "green")
		}
	}
}

func TestDrawStringUnicode(t *testing.T) {
	term := NewCapture(20, 5)
	term.DrawString(0, 0, "日本語", syntax.Face{})
	runes := []rune("日本語")
	for i, want := range runes {
		ch, _ := term.CaptureCell(i, 0)
		if ch != want {
			t.Errorf("DrawString unicode col %d: rune = %q, want %q", i, ch, want)
		}
	}
}

func TestDrawStringClipsAtEdge(t *testing.T) {
	term := NewCapture(5, 3)
	// Writing starting at col 3 with a 5-char string: only 2 chars fit.
	term.DrawString(3, 0, "ABCDE", syntax.Face{})
	chA, _ := term.CaptureCell(3, 0)
	chB, _ := term.CaptureCell(4, 0)
	if chA != 'A' || chB != 'B' {
		t.Errorf("DrawString clip: got %q%q, want AB", chA, chB)
	}
}

func TestDrawStringFaceAttributes(t *testing.T) {
	term := NewCapture(20, 5)
	bold := syntax.Face{Bold: true, Fg: "#ff0000"}
	term.DrawString(0, 0, "X", bold)
	_, f := term.CaptureCell(0, 0)
	if !f.Bold {
		t.Error("DrawString: Bold not stored in captured face")
	}
	if f.Fg != "#ff0000" {
		t.Errorf("DrawString: Fg = %q, want %q", f.Fg, "#ff0000")
	}
}

func TestDrawStringItalicFace(t *testing.T) {
	term := NewCapture(10, 3)
	italic := syntax.Face{Italic: true}
	term.DrawString(0, 0, "X", italic)
	_, f := term.CaptureCell(0, 0)
	if !f.Italic {
		t.Error("DrawString: Italic not stored in captured face")
	}
}

func TestDrawStringUnderlineFace(t *testing.T) {
	term := NewCapture(10, 3)
	ul := syntax.Face{Underline: true, UnderlineColor: "red"}
	term.DrawString(0, 0, "X", ul)
	_, f := term.CaptureCell(0, 0)
	if !f.Underline {
		t.Error("DrawString: Underline not stored in captured face")
	}
	if f.UnderlineColor != "red" {
		t.Errorf("DrawString: UnderlineColor = %q, want %q", f.UnderlineColor, "red")
	}
}

// ---- Clear -----------------------------------------------------------------

func TestClearResetsAllCells(t *testing.T) {
	term := NewCapture(5, 3)
	term.SetCell(0, 0, 'Z', syntax.Face{Fg: "red"})
	term.Clear()
	ch, f := term.CaptureCell(0, 0)
	if ch != ' ' {
		t.Errorf("after Clear: cell = %q, want ' '", ch)
	}
	if f != (syntax.Face{}) {
		t.Errorf("after Clear: face = %v, want zero", f)
	}
}

// ---- Show (no-op in capture mode) -----------------------------------------

func TestShowNopanicInCaptureMode(t *testing.T) {
	term := NewCapture(10, 5)
	term.Show()
}

// ---- ShowCursor (no-op in capture mode) ------------------------------------

func TestShowCursorNopanicInCaptureMode(t *testing.T) {
	term := NewCapture(10, 5)
	term.ShowCursor(3, 2)
}

// ---- DisableCapture --------------------------------------------------------

func TestDisableCaptureStopsCapture(t *testing.T) {
	term := NewCapture(10, 5)
	term.DisableCapture()
	w, h := term.CaptureSize()
	if w != 0 || h != 0 {
		t.Errorf("after DisableCapture: CaptureSize = (%d,%d), want (0,0)", w, h)
	}
}

// ---- InvalidateStyleCache --------------------------------------------------

func TestInvalidateStyleCacheDoesNotPanic(t *testing.T) {
	term := NewCapture(10, 5)
	term.InvalidateStyleCache()
}

// ---- ParseColorRGB (exported helper) ---------------------------------------

func TestParseColorRGBHex(t *testing.T) {
	r, g, b, ok := ParseColorRGB("#1a2b3c")
	if !ok {
		t.Fatal("ParseColorRGB: ok=false for valid hex")
	}
	if r != 0x1a || g != 0x2b || b != 0x3c {
		t.Errorf("ParseColorRGB = (%d,%d,%d), want (26,43,60)", r, g, b)
	}
}

func TestParseColorRGBNamedRed(t *testing.T) {
	r, g, b, ok := ParseColorRGB("red")
	if !ok {
		t.Fatal("ParseColorRGB: ok=false for named 'red'")
	}
	if r == 0 && g == 0 && b == 0 {
		t.Error("ParseColorRGB: red should not be (0,0,0)")
	}
}

func TestParseColorRGBEmpty(t *testing.T) {
	_, _, _, ok := ParseColorRGB("")
	if ok {
		t.Error("ParseColorRGB: ok=true for empty string, want false")
	}
}

func TestParseColorRGBUnknown(t *testing.T) {
	_, _, _, ok := ParseColorRGB("notarealcolor")
	if ok {
		t.Error("ParseColorRGB: ok=true for unknown color, want false")
	}
}

// ---- faceToStyle: underline with color ------------------------------------

func TestFaceToStyleUnderlineWithColor(t *testing.T) {
	face := syntax.Face{Underline: true, UnderlineColor: "red"}
	style := faceToStyle(face)
	if style.GetUnderlineStyle() == tcell.UnderlineStyleNone {
		t.Error("underline with color: underline not set in style")
	}
}

func TestFaceToStyleReverse(t *testing.T) {
	face := syntax.Face{Reverse: true}
	style := faceToStyle(face)
	attrs := style.GetAttributes()
	if attrs&tcell.AttrReverse == 0 {
		t.Error("reverse face: AttrReverse not set")
	}
}

func TestFaceToStyleHexColor(t *testing.T) {
	face := syntax.Face{Fg: "#ff0000", Bg: "#0000ff"}
	style := faceToStyle(face)
	fg := style.GetForeground()
	bg := style.GetBackground()
	wantFg := tcell.NewRGBColor(0xff, 0x00, 0x00)
	wantBg := tcell.NewRGBColor(0x00, 0x00, 0xff)
	if fg != wantFg {
		t.Errorf("faceToStyle hex fg = %v, want %v", fg, wantFg)
	}
	if bg != wantBg {
		t.Errorf("faceToStyle hex bg = %v, want %v", bg, wantBg)
	}
}

// ---- parseColor: named aliases ---------------------------------------------

func TestParseColorMagentaAlias(t *testing.T) {
	magenta := parseColor("magenta")
	fuchsia := parseColor("fuchsia")
	if magenta != fuchsia {
		t.Errorf("magenta(%v) != fuchsia(%v)", magenta, fuchsia)
	}
}

func TestParseColorCyanAlias(t *testing.T) {
	cyan := parseColor("cyan")
	aqua := parseColor("aqua")
	if cyan != aqua {
		t.Errorf("cyan(%v) != aqua(%v)", cyan, aqua)
	}
}

func TestParseColorANSIBounds(t *testing.T) {
	// 256 is out of range; should fall back to default.
	got := parseColor("256")
	if got != parseColor("default") {
		t.Errorf("parseColor(256) should fall back to default, got %v", got)
	}
}

func TestParseColorHexUppercase(t *testing.T) {
	lower := parseColor("#ff8800")
	upper := parseColor("#FF8800")
	if lower != upper {
		t.Errorf("hex case mismatch: lower=%v upper=%v", lower, upper)
	}
}

func TestParseColorHexWrongLength(t *testing.T) {
	// Short hex is not the expected #rrggbb format — falls back to default.
	got := parseColor("#f00")
	if got != parseColor("default") {
		t.Errorf("short hex #f00 should fall back to default, got %v", got)
	}
}

func TestParseColorANSIMin(t *testing.T) {
	got := parseColor("1")
	want := tcell.PaletteColor(1)
	if got != want {
		t.Errorf("parseColor(1) = %v, want PaletteColor(1)=%v", got, want)
	}
}

// ---- TryPollEvent / PostWakeup nil-screen guards ---------------------------

func TestTryPollEventNilScreen(t *testing.T) {
	// A capture-mode terminal has no real screen; TryPollEvent returns nil.
	term := NewCapture(10, 5)
	if ev := term.TryPollEvent(); ev != nil {
		t.Errorf("TryPollEvent with nil screen = %v, want nil", ev)
	}
}

func TestPostWakeupNilScreenNoPanic(t *testing.T) {
	// PostWakeup is a no-op when there is no real screen.
	term := NewCapture(10, 5)
	term.PostWakeup()
}

// ---- faceToStyle: underline without explicit color -------------------------

func TestFaceToStyleUnderlineNoColor(t *testing.T) {
	face := syntax.Face{Underline: true}
	style := faceToStyle(face)
	if style.GetUnderlineStyle() == tcell.UnderlineStyleNone {
		t.Error("underline without color: underline not set in style")
	}
}

func TestFaceToStyleBoldItalic(t *testing.T) {
	face := syntax.Face{Bold: true, Italic: true}
	style := faceToStyle(face)
	attrs := style.GetAttributes()
	if attrs&tcell.AttrBold == 0 {
		t.Error("bold face: AttrBold not set")
	}
	if attrs&tcell.AttrItalic == 0 {
		t.Error("italic face: AttrItalic not set")
	}
}
