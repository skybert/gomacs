package terminal

import (
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/color"
	"github.com/gdamore/tcell/v3/vt"
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

// ---- Close -------------------------------------------------------------------

// TestClose_CaptureNoop verifies that Close on a capture-mode Terminal (whose
// screen field is nil) does not panic, and that the terminal remains usable
// (and its capture grid untouched) afterwards — Close is documented as safe
// to call more than once, and a headless capture terminal is the only way to
// exercise its nil-screen guard without a real TTY.
func TestClose_CaptureNoop(t *testing.T) {
	term := NewCapture(10, 5)
	term.SetCell(1, 1, 'A', syntax.Face{Fg: "red"})

	term.Close()

	// Terminal is still usable after Close: capture state is untouched...
	ch, f := term.CaptureCell(1, 1)
	if ch != 'A' || f.Fg != "red" {
		t.Errorf("after Close: CaptureCell(1,1) = (%q, %+v), want ('A', Fg=red)", ch, f)
	}
	w, h := term.CaptureSize()
	if w != 10 || h != 5 {
		t.Errorf("after Close: CaptureSize() = (%d,%d), want (10,5)", w, h)
	}

	// ...and calling Close again must still not panic.
	term.Close()
}

// ---- InvalidateStyleCache --------------------------------------------------

func TestInvalidateStyleCacheDoesNotPanic(t *testing.T) {
	term := NewCapture(10, 5)
	term.InvalidateStyleCache()
}

// ---- styleFor: one-entry memo -----------------------------------------------

// realScreenTerminal builds a Terminal backed by a real (headless) tcell
// screen using tcell/v3's vt mock terminal, so the non-capture SetCell/
// DrawString path (the one styleFor actually optimizes) can be exercised
// hermetically, with no real TTY involved.
func realScreenTerminal(t *testing.T, width, height int) *Terminal {
	t.Helper()
	mt := vt.NewMockTerm()
	mt.Backend().SetSize(vt.Coord{X: vt.Col(width), Y: vt.Row(height)})
	s, err := tcell.NewTerminfoScreenFromTty(mt)
	if err != nil {
		t.Fatalf("NewTerminfoScreenFromTty: %v", err)
	}
	if err := s.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(s.Fini)
	return &Terminal{screen: s, styleCache: make(map[syntax.Face]tcell.Style, 32)}
}

func TestStyleForMemoHitsSameFace(t *testing.T) {
	term := realScreenTerminal(t, 10, 5)
	face := syntax.Face{Fg: "red", Bold: true}

	// First call resolves via styleCache (memo miss); record the style.
	first := term.styleFor(face)
	if !term.lastFaceValid || term.lastFace != face {
		t.Fatal("styleFor: memo not populated after first call")
	}

	// Mutate the underlying cache entry to a distinguishable style and
	// confirm the second call for the *same* face comes from the memo
	// rather than re-consulting styleCache.
	term.styleCache[face] = tcell.StyleDefault.Reverse(true)
	second := term.styleFor(face)
	if second != first {
		t.Errorf("styleFor: second call for identical face = %v, want memoized %v (should not have re-read styleCache)", second, first)
	}
}

func TestStyleForMemoMissOnDifferentFace(t *testing.T) {
	term := realScreenTerminal(t, 10, 5)
	red := syntax.Face{Fg: "red"}
	blue := syntax.Face{Fg: "blue"}

	term.styleFor(red)
	got := term.styleFor(blue)
	want := faceToStyle(blue)
	if got != want {
		t.Errorf("styleFor(blue) after styleFor(red) = %v, want %v", got, want)
	}
	if term.lastFace != blue {
		t.Errorf("styleFor: memo face = %v, want %v", term.lastFace, blue)
	}
}

// TestInvalidateStyleCacheClearsMemo is the regression test for the one
// thing most likely to break the memo: a theme change (which calls
// InvalidateStyleCache) must take effect on the very next SetCell, even if
// that SetCell uses the same Face value as the one currently memoized (a
// theme swap can change what a given Face resolves to via faceToStyle only
// if faceToStyle's inputs change — here we simulate the more common case of
// re-registering a Face under the same key with a stale cached Style, which
// is exactly what a bare `clear(styleCache)` without memo invalidation would
// have left behind).
func TestInvalidateStyleCacheClearsMemo(t *testing.T) {
	term := realScreenTerminal(t, 10, 5)
	face := syntax.Face{Fg: "red"}

	term.SetCell(0, 0, 'A', face)
	if !term.lastFaceValid {
		t.Fatal("SetCell: memo not populated")
	}

	// Simulate a theme change: the style that this Face should now resolve to
	// is different (e.g. LoadTheme mutated global Face vars, but the Face
	// struct value passed to SetCell happens to be unchanged, or simply: the
	// cache must not be trusted at all after invalidation).
	term.InvalidateStyleCache()
	if term.lastFaceValid {
		t.Fatal("InvalidateStyleCache: one-entry memo still marked valid")
	}
	if len(term.styleCache) != 0 {
		t.Fatalf("InvalidateStyleCache: styleCache not cleared, len=%d", len(term.styleCache))
	}

	// The very next SetCell for the same face must recompute rather than
	// reuse anything left over from before invalidation.
	term.SetCell(0, 0, 'A', face)
	want := faceToStyle(face)
	if term.lastStyle != want {
		t.Errorf("after InvalidateStyleCache: resolved style = %v, want freshly computed %v", term.lastStyle, want)
	}
	if len(term.styleCache) != 1 {
		t.Errorf("after InvalidateStyleCache + SetCell: styleCache len = %d, want 1", len(term.styleCache))
	}
}

// TestSetCellRealScreenAppliesFace exercises the non-capture SetCell path
// end-to-end against a real (headless) tcell screen: the cell content that
// lands in the screen's buffer must reflect the face passed in.
func TestSetCellRealScreenAppliesFace(t *testing.T) {
	term := realScreenTerminal(t, 10, 5)
	face := syntax.Face{Fg: "red", Bold: true}
	term.SetCell(2, 1, 'Z', face)

	str, style, _ := term.screen.Get(2, 1)
	if str != "Z" {
		t.Errorf("Get rune = %q, want %q", str, "Z")
	}
	wantFg := parseColor("red")
	if fg := style.GetForeground(); fg != wantFg {
		t.Errorf("Get style fg = %v, want %v", fg, wantFg)
	}
	attrs := style.GetAttributes()
	if attrs&tcell.AttrBold == 0 {
		t.Error("GetContent style: AttrBold not set")
	}
}

// ---- benchmarks -------------------------------------------------------------

// BenchmarkSetCellSameFaceRun measures SetCell for a long run of identical
// faces against a real (headless) screen — the realistic case of a syntax
// span, a padded modeline, or a run of spaces, where the one-entry memo
// should eliminate the styleCache map lookup for every cell after the first.
func BenchmarkSetCellSameFaceRun(b *testing.B) {
	mt := vt.NewMockTerm()
	mt.Backend().SetSize(vt.Coord{X: 200, Y: 50})
	s, err := tcell.NewTerminfoScreenFromTty(mt)
	if err != nil {
		b.Fatalf("NewTerminfoScreenFromTty: %v", err)
	}
	if err := s.Init(); err != nil {
		b.Fatalf("screen.Init: %v", err)
	}
	defer s.Fini()
	term := &Terminal{screen: s, styleCache: make(map[syntax.Face]tcell.Style, 32)}
	face := syntax.Face{Fg: "#e17df3", Bg: "#1a1a1a", Bold: true}

	for b.Loop() {
		for col := range 200 {
			term.SetCell(col, 10, 'x', face)
		}
	}
}

// BenchmarkStyleForMemoHit measures styleFor for a run of identical faces —
// the case the one-entry memo targets. Compare against
// BenchmarkStyleForCacheHitNoMemo (the old styleCache-only lookup) for the
// direct before/after of this change.
func BenchmarkStyleForMemoHit(b *testing.B) {
	term := &Terminal{styleCache: make(map[syntax.Face]tcell.Style, 32)}
	face := syntax.Face{Fg: "#e17df3", Bg: "#1a1a1a", Bold: true}
	var style tcell.Style
	for b.Loop() {
		style = term.styleFor(face)
	}
	_ = style
}

// BenchmarkStyleForCacheHitNoMemo measures a bare styleCache lookup (what
// SetCell cost before the one-entry memo) for comparison against
// BenchmarkSetCellSameFaceRun.
func BenchmarkStyleForCacheHitNoMemo(b *testing.B) {
	cache := make(map[syntax.Face]tcell.Style, 32)
	face := syntax.Face{Fg: "#e17df3", Bg: "#1a1a1a", Bold: true}
	cache[face] = faceToStyle(face)
	var style tcell.Style
	for b.Loop() {
		style = cache[face]
	}
	_ = style
}

// BenchmarkFaceToStyleMiss measures the full faceToStyle conversion (a cache
// miss), for comparison.
func BenchmarkFaceToStyleMiss(b *testing.B) {
	face := syntax.Face{Fg: "#e17df3", Bg: "#1a1a1a", Bold: true}
	var style tcell.Style
	for b.Loop() {
		style = faceToStyle(face)
	}
	_ = style
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

// ---- faceToStyle: underline with color -------------------------------------

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
