package editor

import (
	"testing"

	"github.com/skybert/gomacs/internal/syntax"
)

// ---------------------------------------------------------------------------
// newVTScreen
// ---------------------------------------------------------------------------

func TestNewVTScreen_Dimensions(t *testing.T) {
	s := newVTScreen(24, 80)
	if s.rows != 24 || s.cols != 80 {
		t.Errorf("dims: want 24×80, got %d×%d", s.rows, s.cols)
	}
}

func TestNewVTScreen_CursorAtOrigin(t *testing.T) {
	s := newVTScreen(24, 80)
	if s.curRow != 0 || s.curCol != 0 {
		t.Errorf("cursor: want (0,0), got (%d,%d)", s.curRow, s.curCol)
	}
}

func TestNewVTScreen_AllCellsBlank(t *testing.T) {
	s := newVTScreen(5, 10)
	for r := range 5 {
		for c := range 10 {
			if got := s.cellAt(r, c).ch; got != ' ' {
				t.Errorf("cell(%d,%d) = %q, want space", r, c, got)
			}
		}
	}
}

func TestNewVTScreen_ScrollBotIsLastRow(t *testing.T) {
	s := newVTScreen(10, 40)
	if s.scrollBot != 9 {
		t.Errorf("scrollBot: want 9, got %d", s.scrollBot)
	}
}

// ---------------------------------------------------------------------------
// resize
// ---------------------------------------------------------------------------

func TestResize_PreservesContent(t *testing.T) {
	s := newVTScreen(5, 10)
	s.write([]byte("Hello"))
	s.resize(10, 20)
	if s.rows != 10 || s.cols != 20 {
		t.Fatalf("resize dims: got %d×%d", s.rows, s.cols)
	}
	// First row first 5 cells should still spell "Hello".
	want := "Hello"
	for i, ch := range want {
		if got := s.cellAt(0, i).ch; got != ch {
			t.Errorf("cell(0,%d) = %q, want %q", i, got, ch)
		}
	}
}

func TestResize_ClampsCursor(t *testing.T) {
	s := newVTScreen(5, 10)
	s.curRow = 4
	s.curCol = 9
	s.resize(3, 6)
	if s.curRow >= s.rows {
		t.Errorf("curRow %d >= rows %d after resize", s.curRow, s.rows)
	}
	if s.curCol >= s.cols {
		t.Errorf("curCol %d >= cols %d after resize", s.curCol, s.cols)
	}
}

// ---------------------------------------------------------------------------
// write — plain text
// ---------------------------------------------------------------------------

func TestWrite_PlainText(t *testing.T) {
	s := newVTScreen(5, 20)
	s.write([]byte("Hello"))
	for i, ch := range "Hello" {
		if got := s.cellAt(0, i).ch; got != ch {
			t.Errorf("cell(0,%d) = %q, want %q", i, got, ch)
		}
	}
	if s.curRow != 0 || s.curCol != 5 {
		t.Errorf("cursor after 'Hello': want (0,5), got (%d,%d)", s.curRow, s.curCol)
	}
}

func TestWrite_CR_ReturnsCursorToCol0(t *testing.T) {
	s := newVTScreen(5, 20)
	s.write([]byte("Hello\r"))
	if s.curCol != 0 {
		t.Errorf("after CR curCol = %d, want 0", s.curCol)
	}
}

func TestWrite_LF_AdvancesRow(t *testing.T) {
	s := newVTScreen(5, 20)
	s.write([]byte("A\nB"))
	if s.curRow != 1 {
		t.Errorf("after LF curRow = %d, want 1", s.curRow)
	}
}

func TestWrite_CRLF_NewLine(t *testing.T) {
	s := newVTScreen(5, 20)
	s.write([]byte("foo\r\nbar"))
	if s.curRow != 1 || s.curCol != 3 {
		t.Errorf("after CRLF cursor = (%d,%d), want (1,3)", s.curRow, s.curCol)
	}
	if got := s.cellAt(0, 0).ch; got != 'f' {
		t.Errorf("row0[0] = %q, want 'f'", got)
	}
	if got := s.cellAt(1, 0).ch; got != 'b' {
		t.Errorf("row1[0] = %q, want 'b'", got)
	}
}

func TestWrite_BS_MovesCursorBack(t *testing.T) {
	s := newVTScreen(5, 20)
	s.write([]byte("AB\x08"))
	if s.curCol != 1 {
		t.Errorf("after BS curCol = %d, want 1", s.curCol)
	}
}

func TestWrite_BS_DoesNotGoNegative(t *testing.T) {
	s := newVTScreen(5, 20)
	s.write([]byte("\x08\x08"))
	if s.curCol != 0 {
		t.Errorf("BS from col0: curCol = %d, want 0", s.curCol)
	}
}

func TestWrite_Tab_AlignsToNextTabStop(t *testing.T) {
	s := newVTScreen(5, 80)
	s.write([]byte("\t"))
	if s.curCol != 8 {
		t.Errorf("tab from col0: curCol = %d, want 8", s.curCol)
	}
	s.curCol = 3
	s.write([]byte("\t"))
	if s.curCol != 8 {
		t.Errorf("tab from col3: curCol = %d, want 8", s.curCol)
	}
}

// ---------------------------------------------------------------------------
// CSI cursor movement
// ---------------------------------------------------------------------------

func TestCSI_CursorUp(t *testing.T) {
	s := newVTScreen(10, 20)
	s.curRow = 5
	s.write([]byte("\x1b[3A")) // move up 3
	if s.curRow != 2 {
		t.Errorf("CursorUp 3 from row5: got row %d, want 2", s.curRow)
	}
}

func TestCSI_CursorDown(t *testing.T) {
	s := newVTScreen(10, 20)
	s.curRow = 2
	s.write([]byte("\x1b[4B")) // move down 4
	if s.curRow != 6 {
		t.Errorf("CursorDown 4 from row2: got row %d, want 6", s.curRow)
	}
}

func TestCSI_CursorRight(t *testing.T) {
	s := newVTScreen(10, 20)
	s.write([]byte("\x1b[5C")) // move right 5
	if s.curCol != 5 {
		t.Errorf("CursorRight 5: got col %d, want 5", s.curCol)
	}
}

func TestCSI_CursorLeft(t *testing.T) {
	s := newVTScreen(10, 20)
	s.curCol = 8
	s.write([]byte("\x1b[3D")) // move left 3
	if s.curCol != 5 {
		t.Errorf("CursorLeft 3 from col8: got col %d, want 5", s.curCol)
	}
}

func TestCSI_CursorPosition(t *testing.T) {
	s := newVTScreen(24, 80)
	s.write([]byte("\x1b[5;10H")) // row 5, col 10 (1-based)
	if s.curRow != 4 || s.curCol != 9 {
		t.Errorf("CursorPosition(5,10): got (%d,%d), want (4,9)", s.curRow, s.curCol)
	}
}

func TestCSI_CursorPosition_DefaultsToOrigin(t *testing.T) {
	s := newVTScreen(24, 80)
	s.curRow, s.curCol = 5, 5
	s.write([]byte("\x1b[H")) // no params → (1,1)
	if s.curRow != 0 || s.curCol != 0 {
		t.Errorf("CursorPosition(): got (%d,%d), want (0,0)", s.curRow, s.curCol)
	}
}

func TestCSI_CursorHorizontalAbsolute(t *testing.T) {
	s := newVTScreen(10, 80)
	s.write([]byte("\x1b[15G")) // col 15 (1-based)
	if s.curCol != 14 {
		t.Errorf("CHA 15: got col %d, want 14", s.curCol)
	}
}

// ---------------------------------------------------------------------------
// CSI erase
// ---------------------------------------------------------------------------

func TestCSI_EraseInLine_ToEnd(t *testing.T) {
	s := newVTScreen(5, 10)
	s.write([]byte("Hello"))
	s.curCol = 2
	s.write([]byte("\x1b[K")) // erase from col2 to end of line
	if got := s.cellAt(0, 2).ch; got != ' ' {
		t.Errorf("cell(0,2) after EL-ToEnd = %q, want space", got)
	}
	// cols 0..1 should be untouched
	if got := s.cellAt(0, 0).ch; got != 'H' {
		t.Errorf("cell(0,0) unexpectedly changed to %q", got)
	}
}

func TestCSI_EraseDisplay_ToEnd(t *testing.T) {
	s := newVTScreen(3, 5)
	s.write([]byte("AAAAA\r\nBBBBB\r\nCCCCC"))
	s.curRow, s.curCol = 1, 2
	s.write([]byte("\x1b[J")) // erase from cursor to end of display
	// row1 cols 2..4 should be blank
	if got := s.cellAt(1, 2).ch; got != ' ' {
		t.Errorf("cell(1,2) = %q, want space", got)
	}
	// row1 col 0..1 unchanged
	if got := s.cellAt(1, 0).ch; got != 'B' {
		t.Errorf("cell(1,0) = %q, want 'B'", got)
	}
	// entire row 2 blank
	if got := s.cellAt(2, 0).ch; got != ' ' {
		t.Errorf("cell(2,0) = %q, want space", got)
	}
}

func TestCSI_EraseDisplay_Entire(t *testing.T) {
	s := newVTScreen(3, 5)
	s.write([]byte("Hello"))
	s.write([]byte("\x1b[2J"))
	for r := range 3 {
		for c := range 5 {
			if got := s.cellAt(r, c).ch; got != ' ' {
				t.Errorf("cell(%d,%d) = %q, want space after ED-Entire", r, c, got)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// CSI save/restore cursor
// ---------------------------------------------------------------------------

func TestCSI_SaveRestoreCursor(t *testing.T) {
	s := newVTScreen(10, 80)
	s.curRow, s.curCol = 3, 7
	s.write([]byte("\x1b[s")) // save
	s.curRow, s.curCol = 0, 0
	s.write([]byte("\x1b[u")) // restore
	if s.curRow != 3 || s.curCol != 7 {
		t.Errorf("restore cursor: got (%d,%d), want (3,7)", s.curRow, s.curCol)
	}
}

// ---------------------------------------------------------------------------
// SGR
// ---------------------------------------------------------------------------

func TestSGR_Reset(t *testing.T) {
	s := newVTScreen(5, 20)
	s.write([]byte("\x1b[1mA")) // bold A
	s.write([]byte("\x1b[0m"))  // reset
	if s.curFace.Bold {
		t.Error("after SGR reset, Bold should be false")
	}
}

func TestSGR_Bold(t *testing.T) {
	s := newVTScreen(5, 20)
	s.write([]byte("\x1b[1mX"))
	if !s.cellAt(0, 0).face.Bold {
		t.Error("SGR 1: cell should be bold")
	}
}

func TestSGR_Foreground_ANSI(t *testing.T) {
	s := newVTScreen(5, 20)
	s.write([]byte("\x1b[31mR")) // red foreground
	face := s.cellAt(0, 0).face
	if face.Fg != "red" {
		t.Errorf("SGR 31: Fg = %q, want \"red\"", face.Fg)
	}
}

func TestSGR_Background_ANSI(t *testing.T) {
	s := newVTScreen(5, 20)
	s.write([]byte("\x1b[42mG")) // green background
	face := s.cellAt(0, 0).face
	if face.Bg != "green" {
		t.Errorf("SGR 42: Bg = %q, want \"green\"", face.Bg)
	}
}

func TestSGR_BrightColor(t *testing.T) {
	s := newVTScreen(5, 20)
	s.write([]byte("\x1b[90mX")) // bright black (dark grey)
	face := s.cellAt(0, 0).face
	if face.Fg != "bright-black" {
		t.Errorf("SGR 90: Fg = %q, want \"bright-black\"", face.Fg)
	}
}

func TestSGR_TrueColor_Foreground(t *testing.T) {
	s := newVTScreen(5, 20)
	s.write([]byte("\x1b[38;2;255;128;0mX")) // orange
	face := s.cellAt(0, 0).face
	if face.Fg != "#ff8000" {
		t.Errorf("SGR 38;2;255;128;0: Fg = %q, want \"#ff8000\"", face.Fg)
	}
}

func TestSGR_Underline(t *testing.T) {
	s := newVTScreen(5, 20)
	s.write([]byte("\x1b[4mU"))
	if !s.cellAt(0, 0).face.Underline {
		t.Error("SGR 4: cell should be underlined")
	}
}

func TestSGR_Reverse(t *testing.T) {
	s := newVTScreen(5, 20)
	s.write([]byte("\x1b[7mV"))
	if !s.cellAt(0, 0).face.Reverse {
		t.Error("SGR 7: cell should be reverse")
	}
}

func TestSGR_FaceAppliesToCell(t *testing.T) {
	s := newVTScreen(5, 20)
	s.write([]byte("\x1b[1;32mok")) // bold + green
	face := s.cellAt(0, 0).face
	if !face.Bold || face.Fg != "green" {
		t.Errorf("SGR 1;32: Bold=%v Fg=%q, want true/green", face.Bold, face.Fg)
	}
}

// ---------------------------------------------------------------------------
// ESC M — reverse index
// ---------------------------------------------------------------------------

func TestESC_ReverseIndex_AtScrollTop(t *testing.T) {
	s := newVTScreen(5, 10)
	s.write([]byte("Line0\r\nLine1\r\nLine2\r\nLine3\r\nLine4"))
	s.curRow = 0 // at scroll top
	s.write([]byte("\x1bM"))
	// Should scroll down (insert blank line at top).
	if got := s.cellAt(0, 0).ch; got != ' ' {
		t.Errorf("ESC M at top: row0 should be blank, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Alternate screen
// ---------------------------------------------------------------------------

func TestPrivateMode_AltScreen_1049(t *testing.T) {
	s := newVTScreen(5, 20)
	s.write([]byte("main"))
	s.write([]byte("\x1b[?1049h")) // enter alt screen
	if !s.useAlt {
		t.Fatal("useAlt should be true after ?1049h")
	}
	// Alt screen should be blank.
	if got := s.cellAt(0, 0).ch; got != ' ' {
		t.Errorf("alt screen row0col0 = %q, want space", got)
	}
	s.write([]byte("\x1b[?1049l")) // leave alt screen
	if s.useAlt {
		t.Fatal("useAlt should be false after ?1049l")
	}
	// Main screen content restored.
	if got := s.cellAt(0, 0).ch; got != 'm' {
		t.Errorf("main screen after alt exit row0col0 = %q, want 'm'", got)
	}
}

// ---------------------------------------------------------------------------
// Scroll region
// ---------------------------------------------------------------------------

func TestCSI_ScrollUp(t *testing.T) {
	s := newVTScreen(5, 5)
	s.write([]byte("AAAAA\r\nBBBBB\r\nCCCCC\r\nDDDDD\r\nEEEEE"))
	s.write([]byte("\x1b[2S")) // scroll up 2
	// Row 0 should now hold what was row 2 ("CCCCC").
	if got := s.cellAt(0, 0).ch; got != 'C' {
		t.Errorf("after scroll-up 2, row0[0] = %q, want 'C'", got)
	}
	// Last 2 rows should be blank.
	if got := s.cellAt(3, 0).ch; got != ' ' {
		t.Errorf("after scroll-up 2, row3[0] = %q, want space", got)
	}
}

func TestCSI_ScrollDown(t *testing.T) {
	s := newVTScreen(5, 5)
	s.write([]byte("AAAAA\r\nBBBBB\r\nCCCCC\r\nDDDDD\r\nEEEEE"))
	s.write([]byte("\x1b[1T")) // scroll down 1
	// Row 0 should now be blank.
	if got := s.cellAt(0, 0).ch; got != ' ' {
		t.Errorf("after scroll-down 1, row0[0] = %q, want space", got)
	}
	// Row 1 should hold old row 0.
	if got := s.cellAt(1, 0).ch; got != 'A' {
		t.Errorf("after scroll-down 1, row1[0] = %q, want 'A'", got)
	}
}

// ---------------------------------------------------------------------------
// Auto-wrap / line feed at bottom
// ---------------------------------------------------------------------------

func TestWrite_LineFeed_AtBottom_Scrolls(t *testing.T) {
	s := newVTScreen(3, 5)
	s.write([]byte("AAA\r\nBBB\r\nCCC\r\n")) // 3 lines + one more LF
	// scroll should have happened; row 0 should now hold BBB
	if got := s.cellAt(0, 0).ch; got != 'B' {
		t.Errorf("after scroll, row0[0] = %q, want 'B'", got)
	}
}

// ---------------------------------------------------------------------------
// vtParseParams
// ---------------------------------------------------------------------------

func TestVtParseParams_Empty(t *testing.T) {
	if got := vtParseParams(nil); got != nil {
		t.Errorf("nil input: want nil, got %v", got)
	}
}

func TestVtParseParams_Single(t *testing.T) {
	got := vtParseParams([]byte("5"))
	if len(got) != 1 || got[0] != 5 {
		t.Errorf("'5': want [5], got %v", got)
	}
}

func TestVtParseParams_Multiple(t *testing.T) {
	got := vtParseParams([]byte("1;2;3"))
	want := []int{1, 2, 3}
	if len(got) != len(want) {
		t.Fatalf("'1;2;3': len=%d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("[%d]=%d, want %d", i, got[i], w)
		}
	}
}

func TestVtParseParams_EmptySegment(t *testing.T) {
	got := vtParseParams([]byte("1;;3"))
	if len(got) != 3 || got[1] != 0 {
		t.Errorf("'1;;3': want [1 0 3], got %v", got)
	}
}

// ---------------------------------------------------------------------------
// vtAnsi256
// ---------------------------------------------------------------------------

func TestVtAnsi256_BasicColor(t *testing.T) {
	if got := vtAnsi256(1); got != "red" {
		t.Errorf("ansi256(1) = %q, want \"red\"", got)
	}
}

func TestVtAnsi256_Grayscale(t *testing.T) {
	got := vtAnsi256(232)
	if got != "#080808" {
		t.Errorf("ansi256(232) = %q, want \"#080808\"", got)
	}
}

func TestVtAnsi256_256Color(t *testing.T) {
	// n=16: first 6×6×6 cube entry = black (#000000)
	got := vtAnsi256(16)
	if got != "#000000" {
		t.Errorf("ansi256(16) = %q, want \"#000000\"", got)
	}
}

// ---------------------------------------------------------------------------
// vtClamp
// ---------------------------------------------------------------------------

func TestVtClamp(t *testing.T) {
	tests := []struct{ v, lo, hi, want int }{
		{5, 0, 10, 5},
		{-1, 0, 10, 0},
		{15, 0, 10, 10},
		{0, 0, 0, 0},
	}
	for _, tc := range tests {
		if got := vtClamp(tc.v, tc.lo, tc.hi); got != tc.want {
			t.Errorf("vtClamp(%d,%d,%d) = %d, want %d", tc.v, tc.lo, tc.hi, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// vtColorName
// ---------------------------------------------------------------------------

func TestVtColorName_Valid(t *testing.T) {
	if got := vtColorName(0); got != "black" {
		t.Errorf("vtColorName(0) = %q, want \"black\"", got)
	}
	if got := vtColorName(7); got != "white" {
		t.Errorf("vtColorName(7) = %q, want \"white\"", got)
	}
	if got := vtColorName(15); got != "bright-white" {
		t.Errorf("vtColorName(15) = %q, want \"bright-white\"", got)
	}
}

func TestVtColorName_OOB(t *testing.T) {
	if got := vtColorName(16); got != "" {
		t.Errorf("vtColorName(16) = %q, want \"\"", got)
	}
}

// ---------------------------------------------------------------------------
// OSC — must not corrupt normal parsing
// ---------------------------------------------------------------------------

func TestOSC_Ignored(t *testing.T) {
	s := newVTScreen(5, 20)
	// OSC 0 ; title BEL — set window title (ignored by vtScreen)
	s.write([]byte("\x1b]0;My Title\x07Hello"))
	// "Hello" should appear at cursor position.
	if got := s.cellAt(0, 0).ch; got != 'H' {
		t.Errorf("after OSC: cell(0,0) = %q, want 'H'", got)
	}
}

// ---------------------------------------------------------------------------
// cellAt — out-of-bounds returns blank
// ---------------------------------------------------------------------------

func TestCellAt_OutOfBounds(t *testing.T) {
	s := newVTScreen(5, 10)
	c := s.cellAt(-1, 0)
	if c.ch != ' ' {
		t.Errorf("cellAt(-1,0) = %q, want space", c.ch)
	}
	c = s.cellAt(100, 0)
	if c.ch != ' ' {
		t.Errorf("cellAt(100,0) = %q, want space", c.ch)
	}
}

// ---------------------------------------------------------------------------
// full reset ESC c
// ---------------------------------------------------------------------------

func TestESC_FullReset(t *testing.T) {
	s := newVTScreen(5, 20)
	s.write([]byte("\x1b[1mBold\x1bc"))
	if s.curFace != (syntax.Face{}) {
		t.Errorf("after ESC c: curFace should be zero, got %+v", s.curFace)
	}
	if s.curRow != 0 || s.curCol != 0 {
		t.Errorf("after ESC c: cursor should be (0,0), got (%d,%d)", s.curRow, s.curCol)
	}
}

// ---------------------------------------------------------------------------
// vtParseParams — zero param
// ---------------------------------------------------------------------------

func TestVtParseParams_Zero(t *testing.T) {
	got := vtParseParams([]byte("0"))
	if len(got) != 1 || got[0] != 0 {
		t.Errorf("'0': want [0], got %v", got)
	}
}

// ---------------------------------------------------------------------------
// vtAnsi256 — bright system colors and RGB cube mid-range
// ---------------------------------------------------------------------------

func TestVtAnsi256_BrightColor(t *testing.T) {
	// Index 8 is bright-black.
	if got := vtAnsi256(8); got != "bright-black" {
		t.Errorf("ansi256(8) = %q, want \"bright-black\"", got)
	}
	// Index 15 is bright-white.
	if got := vtAnsi256(15); got != "bright-white" {
		t.Errorf("ansi256(15) = %q, want \"bright-white\"", got)
	}
}

func TestVtAnsi256_RGBCubeMidRange(t *testing.T) {
	// n=52 → index 52-16=36; r=36/36=1→51, g=(36/6)%6=0→0, b=36%6=0→0 → #330000
	if got := vtAnsi256(52); got != "#330000" {
		t.Errorf("ansi256(52) = %q, want \"#330000\"", got)
	}
}

func TestVtAnsi256_GrayscaleMax(t *testing.T) {
	// n=255: v=(255-232)*10+8=238; all channels 0xee
	if got := vtAnsi256(255); got != "#eeeeee" {
		t.Errorf("ansi256(255) = %q, want \"#eeeeee\"", got)
	}
}

// ---------------------------------------------------------------------------
// vtColorName — all 16 entries
// ---------------------------------------------------------------------------

func TestVtColorName_AllSixteen(t *testing.T) {
	want := []string{
		"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white",
		"bright-black", "bright-red", "bright-green", "bright-yellow",
		"bright-blue", "bright-magenta", "bright-cyan", "bright-white",
	}
	for i, w := range want {
		if got := vtColorName(i); got != w {
			t.Errorf("vtColorName(%d) = %q, want %q", i, got, w)
		}
	}
}

func TestVtColorName_Negative(t *testing.T) {
	if got := vtColorName(-1); got != "" {
		t.Errorf("vtColorName(-1) = %q, want \"\"", got)
	}
}

// ---------------------------------------------------------------------------
// active() — reports altCells when useAlt is true
// ---------------------------------------------------------------------------

func TestActive_MainScreenByDefault(t *testing.T) {
	s := newVTScreen(5, 10)
	s.write([]byte("X"))
	// active() on main screen must return cells, not altCells.
	if &s.active()[0] == &s.altCells[0] {
		t.Error("active() returned altCells when useAlt is false")
	}
}

func TestActive_AltScreenWhenEnabled(t *testing.T) {
	s := newVTScreen(5, 10)
	s.write([]byte("\x1b[?1049h")) // enter alt screen
	if &s.active()[0] != &s.altCells[0] {
		t.Error("active() should return altCells when useAlt is true")
	}
}

// ---------------------------------------------------------------------------
// SGR — italic, 256-color, true-color bg, reset-fg/bg, bright-bg, attr-off
// ---------------------------------------------------------------------------

func TestSGR_Italic(t *testing.T) {
	s := newVTScreen(5, 20)
	s.write([]byte("\x1b[3mI"))
	if !s.cellAt(0, 0).face.Italic {
		t.Error("SGR 3: cell should be italic")
	}
}

func TestSGR_256Color_Foreground(t *testing.T) {
	s := newVTScreen(5, 20)
	s.write([]byte("\x1b[38;5;196mR")) // 256-color red
	face := s.cellAt(0, 0).face
	// Index 196 in 6x6x6 cube: 196-16=180; r=180/36=5→255, g=(180/6)%6=0, b=0 → #ff0000
	if face.Fg != "#ff0000" {
		t.Errorf("SGR 38;5;196: Fg = %q, want \"#ff0000\"", face.Fg)
	}
}

func TestSGR_256Color_Background(t *testing.T) {
	s := newVTScreen(5, 20)
	s.write([]byte("\x1b[48;5;21mB")) // 256-color blue (index 21)
	face := s.cellAt(0, 0).face
	// Index 21-16=5; r=5/36=0, g=(5/6)%6=0, b=5%6=5→255 → #0000ff
	if face.Bg != "#0000ff" {
		t.Errorf("SGR 48;5;21: Bg = %q, want \"#0000ff\"", face.Bg)
	}
}

func TestSGR_TrueColor_Background(t *testing.T) {
	s := newVTScreen(5, 20)
	s.write([]byte("\x1b[48;2;0;255;128mG"))
	face := s.cellAt(0, 0).face
	if face.Bg != "#00ff80" {
		t.Errorf("SGR 48;2;0;255;128: Bg = %q, want \"#00ff80\"", face.Bg)
	}
}

func TestSGR_ResetForeground(t *testing.T) {
	s := newVTScreen(5, 20)
	s.write([]byte("\x1b[31m")) // set red
	s.write([]byte("\x1b[39m")) // reset fg
	if s.curFace.Fg != "" {
		t.Errorf("SGR 39: Fg = %q, want \"\"", s.curFace.Fg)
	}
}

func TestSGR_ResetBackground(t *testing.T) {
	s := newVTScreen(5, 20)
	s.write([]byte("\x1b[42m")) // set green bg
	s.write([]byte("\x1b[49m")) // reset bg
	if s.curFace.Bg != "" {
		t.Errorf("SGR 49: Bg = %q, want \"\"", s.curFace.Bg)
	}
}

func TestSGR_BrightBackground(t *testing.T) {
	s := newVTScreen(5, 20)
	s.write([]byte("\x1b[101mX")) // bright red background
	face := s.cellAt(0, 0).face
	if face.Bg != "bright-red" {
		t.Errorf("SGR 101: Bg = %q, want \"bright-red\"", face.Bg)
	}
}

func TestSGR_TurnOffBold(t *testing.T) {
	s := newVTScreen(5, 20)
	s.write([]byte("\x1b[1m\x1b[22m")) // bold on, then intensity normal
	if s.curFace.Bold {
		t.Error("SGR 22: Bold should be false")
	}
}

func TestSGR_TurnOffItalic(t *testing.T) {
	s := newVTScreen(5, 20)
	s.write([]byte("\x1b[3m\x1b[23m"))
	if s.curFace.Italic {
		t.Error("SGR 23: Italic should be false")
	}
}

func TestSGR_TurnOffUnderline(t *testing.T) {
	s := newVTScreen(5, 20)
	s.write([]byte("\x1b[4m\x1b[24m"))
	if s.curFace.Underline {
		t.Error("SGR 24: Underline should be false")
	}
}

func TestSGR_TurnOffReverse(t *testing.T) {
	s := newVTScreen(5, 20)
	s.write([]byte("\x1b[7m\x1b[27m"))
	if s.curFace.Reverse {
		t.Error("SGR 27: Reverse should be false")
	}
}

// ---------------------------------------------------------------------------
// ESC 7/8 — save and restore cursor (separate from CSI s/u)
// ---------------------------------------------------------------------------

func TestESC_SaveRestoreCursor(t *testing.T) {
	s := newVTScreen(10, 40)
	s.curRow, s.curCol = 4, 12
	s.write([]byte("\x1b7"))  // ESC 7 = save
	s.curRow, s.curCol = 0, 0 // move away
	s.write([]byte("\x1b8"))  // ESC 8 = restore
	if s.curRow != 4 || s.curCol != 12 {
		t.Errorf("ESC 7/8: restored cursor = (%d,%d), want (4,12)", s.curRow, s.curCol)
	}
}

// ---------------------------------------------------------------------------
// CSI EraseInLine modes 1 and 2
// ---------------------------------------------------------------------------

func TestCSI_EraseInLine_ToStart(t *testing.T) {
	s := newVTScreen(5, 10)
	s.write([]byte("ABCDE"))
	s.curCol = 2
	s.write([]byte("\x1b[1K")) // erase from start to cursor (inclusive)
	// Cols 0-2 should be blank.
	for c := 0; c <= 2; c++ {
		if got := s.cellAt(0, c).ch; got != ' ' {
			t.Errorf("cell(0,%d) = %q after EL-ToStart, want space", c, got)
		}
	}
	// Cols 3-4 unchanged.
	if got := s.cellAt(0, 3).ch; got != 'D' {
		t.Errorf("cell(0,3) = %q, want 'D'", got)
	}
}

func TestCSI_EraseInLine_Entire(t *testing.T) {
	s := newVTScreen(5, 10)
	s.write([]byte("ABCDE"))
	s.curCol = 2
	s.write([]byte("\x1b[2K")) // erase entire line
	for c := 0; c < 10; c++ {
		if got := s.cellAt(0, c).ch; got != ' ' {
			t.Errorf("cell(0,%d) = %q after EL-Entire, want space", c, got)
		}
	}
}

// ---------------------------------------------------------------------------
// CSI EraseDisplay mode 1 (erase to start of display)
// ---------------------------------------------------------------------------

func TestCSI_EraseDisplay_ToStart(t *testing.T) {
	s := newVTScreen(3, 5)
	s.write([]byte("AAAAA\r\nBBBBB\r\nCCCCC"))
	s.curRow, s.curCol = 1, 2
	s.write([]byte("\x1b[1J")) // erase from start to cursor
	// row 0 entirely blank.
	for c := range 5 {
		if got := s.cellAt(0, c).ch; got != ' ' {
			t.Errorf("cell(0,%d) = %q after ED-ToStart, want space", c, got)
		}
	}
	// row1 cols 0-2 blank.
	for c := 0; c <= 2; c++ {
		if got := s.cellAt(1, c).ch; got != ' ' {
			t.Errorf("cell(1,%d) = %q after ED-ToStart, want space", c, got)
		}
	}
	// row 2 untouched.
	if got := s.cellAt(2, 0).ch; got != 'C' {
		t.Errorf("cell(2,0) = %q, want 'C'", got)
	}
}

// ---------------------------------------------------------------------------
// CSI cursor next / previous line (E / F)
// ---------------------------------------------------------------------------

func TestCSI_CursorNextLine(t *testing.T) {
	s := newVTScreen(10, 20)
	s.curRow, s.curCol = 2, 5
	s.write([]byte("\x1b[2E")) // down 2 lines, col 0
	if s.curRow != 4 || s.curCol != 0 {
		t.Errorf("CursorNextLine 2: got (%d,%d), want (4,0)", s.curRow, s.curCol)
	}
}

func TestCSI_CursorPrevLine(t *testing.T) {
	s := newVTScreen(10, 20)
	s.curRow, s.curCol = 5, 8
	s.write([]byte("\x1b[3F")) // up 3 lines, col 0
	if s.curRow != 2 || s.curCol != 0 {
		t.Errorf("CursorPrevLine 3: got (%d,%d), want (2,0)", s.curRow, s.curCol)
	}
}

// ---------------------------------------------------------------------------
// CSI vertical position absolute (d)
// ---------------------------------------------------------------------------

func TestCSI_VerticalPositionAbsolute(t *testing.T) {
	s := newVTScreen(10, 20)
	s.write([]byte("\x1b[5d")) // row 5 (1-based)
	if s.curRow != 4 {
		t.Errorf("VPA 5: curRow = %d, want 4", s.curRow)
	}
}

// ---------------------------------------------------------------------------
// CSI delete characters (P), insert characters (@), erase characters (X)
// ---------------------------------------------------------------------------

func TestCSI_DeleteChars(t *testing.T) {
	s := newVTScreen(3, 10)
	s.write([]byte("ABCDE"))
	s.curCol = 1
	s.write([]byte("\x1b[2P")) // delete 2 chars at col 1
	// "ABCDE" → delete B,C → shift D,E left → "ADE  "
	want := []rune{'A', 'D', 'E', ' ', ' '}
	for i, w := range want {
		if got := s.cellAt(0, i).ch; got != w {
			t.Errorf("cell(0,%d) after P: got %q, want %q", i, got, w)
		}
	}
}

func TestCSI_InsertChars(t *testing.T) {
	s := newVTScreen(3, 10)
	s.write([]byte("ABCDE"))
	s.curCol = 1
	s.write([]byte("\x1b[2@")) // insert 2 blank chars at col 1
	// "ABCDE" → insert 2 spaces at col1 → "A  BCDE" (last 2 chars pushed off)
	if got := s.cellAt(0, 0).ch; got != 'A' {
		t.Errorf("cell(0,0) = %q, want 'A'", got)
	}
	if got := s.cellAt(0, 1).ch; got != ' ' {
		t.Errorf("cell(0,1) = %q, want space", got)
	}
	if got := s.cellAt(0, 2).ch; got != ' ' {
		t.Errorf("cell(0,2) = %q, want space", got)
	}
	if got := s.cellAt(0, 3).ch; got != 'B' {
		t.Errorf("cell(0,3) = %q, want 'B'", got)
	}
}

func TestCSI_EraseChars(t *testing.T) {
	s := newVTScreen(3, 10)
	s.write([]byte("ABCDE"))
	s.curCol = 1
	s.write([]byte("\x1b[3X")) // erase 3 chars starting at col 1
	if got := s.cellAt(0, 0).ch; got != 'A' {
		t.Errorf("cell(0,0) = %q, want 'A'", got)
	}
	for c := 1; c <= 3; c++ {
		if got := s.cellAt(0, c).ch; got != ' ' {
			t.Errorf("cell(0,%d) = %q, want space", c, got)
		}
	}
	if got := s.cellAt(0, 4).ch; got != 'E' {
		t.Errorf("cell(0,4) = %q, want 'E'", got)
	}
}

// ---------------------------------------------------------------------------
// CSI insert / delete lines (L / M)
// ---------------------------------------------------------------------------

func TestCSI_InsertLines(t *testing.T) {
	s := newVTScreen(5, 5)
	s.write([]byte("AAAAA\r\nBBBBB\r\nCCCCC\r\nDDDDD\r\nEEEEE"))
	s.curRow = 1
	s.write([]byte("\x1b[1L")) // insert 1 blank line at row 1
	// Row 1 should be blank, old row 1 shifts to row 2.
	if got := s.cellAt(1, 0).ch; got != ' ' {
		t.Errorf("row1[0] after L = %q, want space", got)
	}
	if got := s.cellAt(2, 0).ch; got != 'B' {
		t.Errorf("row2[0] after L = %q, want 'B'", got)
	}
}

func TestCSI_DeleteLines(t *testing.T) {
	s := newVTScreen(5, 5)
	s.write([]byte("AAAAA\r\nBBBBB\r\nCCCCC\r\nDDDDD\r\nEEEEE"))
	s.curRow = 1
	s.write([]byte("\x1b[1M")) // delete 1 line at row 1
	// Old row 2 should now be row 1.
	if got := s.cellAt(1, 0).ch; got != 'C' {
		t.Errorf("row1[0] after M = %q, want 'C'", got)
	}
	// Last row should be blank.
	if got := s.cellAt(4, 0).ch; got != ' ' {
		t.Errorf("row4[0] after M = %q, want space", got)
	}
}

// ---------------------------------------------------------------------------
// CSI set scroll region (r)
// ---------------------------------------------------------------------------

func TestCSI_SetScrollRegion(t *testing.T) {
	s := newVTScreen(10, 10)
	s.write([]byte("\x1b[3;7r")) // rows 3-7 (1-based)
	if s.scrollTop != 2 || s.scrollBot != 6 {
		t.Errorf("scroll region: got top=%d bot=%d, want 2/6", s.scrollTop, s.scrollBot)
	}
	// Cursor should be at origin after setting scroll region.
	if s.curRow != 0 || s.curCol != 0 {
		t.Errorf("cursor after set-scroll-region = (%d,%d), want (0,0)", s.curRow, s.curCol)
	}
}

// ---------------------------------------------------------------------------
// Auto-wrap past last column
// ---------------------------------------------------------------------------

func TestWrite_AutoWrap(t *testing.T) {
	s := newVTScreen(3, 5)
	s.write([]byte("ABCDE")) // fills row 0 cols 0-4; cursor now at col 5
	s.write([]byte("X"))     // should wrap to row 1 col 0
	if s.curRow != 1 || s.curCol != 1 {
		t.Errorf("after auto-wrap: cursor = (%d,%d), want (1,1)", s.curRow, s.curCol)
	}
	if got := s.cellAt(1, 0).ch; got != 'X' {
		t.Errorf("wrapped char at (1,0) = %q, want 'X'", got)
	}
}

// ---------------------------------------------------------------------------
// UTF-8 multibyte characters
// ---------------------------------------------------------------------------

func TestWrite_UTF8_Multibyte(t *testing.T) {
	s := newVTScreen(5, 20)
	s.write([]byte("A\xc3\xa9B")) // A é B  (é = U+00E9)
	if got := s.cellAt(0, 0).ch; got != 'A' {
		t.Errorf("cell(0,0) = %q, want 'A'", got)
	}
	if got := s.cellAt(0, 1).ch; got != 'é' {
		t.Errorf("cell(0,1) = %q, want 'é'", got)
	}
	if got := s.cellAt(0, 2).ch; got != 'B' {
		t.Errorf("cell(0,2) = %q, want 'B'", got)
	}
}

// ---------------------------------------------------------------------------
// resize — shrinking the screen
// ---------------------------------------------------------------------------

func TestResize_Shrink(t *testing.T) {
	s := newVTScreen(10, 20)
	s.write([]byte("Hello"))
	s.curRow, s.curCol = 9, 19
	s.resize(5, 10)
	if s.rows != 5 || s.cols != 10 {
		t.Fatalf("shrink dims: got %d×%d, want 5×10", s.rows, s.cols)
	}
	// Cursor must be clamped within new bounds.
	if s.curRow >= s.rows || s.curCol >= s.cols {
		t.Errorf("cursor out of bounds after shrink: (%d,%d)", s.curRow, s.curCol)
	}
	// First row content preserved up to new width.
	if got := s.cellAt(0, 0).ch; got != 'H' {
		t.Errorf("cell(0,0) after shrink = %q, want 'H'", got)
	}
}

// ---------------------------------------------------------------------------
// Mode 1047 — alternate screen without cursor save
// ---------------------------------------------------------------------------

func TestPrivateMode_AltScreen_1047(t *testing.T) {
	s := newVTScreen(5, 20)
	s.write([]byte("main"))
	s.curRow, s.curCol = 2, 3
	s.write([]byte("\x1b[?1047h")) // enter alt screen (no cursor save)
	if !s.useAlt {
		t.Fatal("useAlt should be true after ?1047h")
	}
	// Alt screen blank.
	if got := s.cellAt(0, 0).ch; got != ' ' {
		t.Errorf("alt screen (0,0) = %q, want space", got)
	}
	s.write([]byte("\x1b[?1047l")) // leave alt screen
	if s.useAlt {
		t.Fatal("useAlt should be false after ?1047l")
	}
	// Main screen content still intact.
	if got := s.cellAt(0, 0).ch; got != 'm' {
		t.Errorf("main screen after ?1047l: (0,0) = %q, want 'm'", got)
	}
}

// ---------------------------------------------------------------------------
// Mode 1048 — save/restore cursor only
// ---------------------------------------------------------------------------

func TestPrivateMode_1048_SaveRestore(t *testing.T) {
	s := newVTScreen(10, 40)
	s.curRow, s.curCol = 5, 15
	s.write([]byte("\x1b[?1048h")) // save cursor
	s.curRow, s.curCol = 0, 0
	s.write([]byte("\x1b[?1048l")) // restore cursor
	if s.curRow != 5 || s.curCol != 15 {
		t.Errorf("mode 1048 restore: got (%d,%d), want (5,15)", s.curRow, s.curCol)
	}
}

// ---------------------------------------------------------------------------
// ESC D (Index) and ESC E (Next Line)
// ---------------------------------------------------------------------------

func TestESC_Index(t *testing.T) {
	s := newVTScreen(5, 10)
	s.curRow = 2
	s.write([]byte("\x1bD")) // ESC D = same as LF
	if s.curRow != 3 {
		t.Errorf("ESC D: curRow = %d, want 3", s.curRow)
	}
}

func TestESC_NextLine(t *testing.T) {
	s := newVTScreen(5, 10)
	s.curRow, s.curCol = 2, 5
	s.write([]byte("\x1bE")) // ESC E = CR + LF
	if s.curRow != 3 || s.curCol != 0 {
		t.Errorf("ESC E: got (%d,%d), want (3,0)", s.curRow, s.curCol)
	}
}

// ---------------------------------------------------------------------------
// OSC terminated by ESC backslash (ST)
// ---------------------------------------------------------------------------

func TestOSC_TerminatedByESC(t *testing.T) {
	s := newVTScreen(5, 20)
	// OSC terminated by ESC (which vtScreen treats as closing OSC).
	s.write([]byte("\x1b]0;title\x1bHi"))
	// After the OSC, "Hi" should be written normally.
	if got := s.cellAt(0, 0).ch; got != 'H' {
		t.Errorf("after OSC+ESC terminator: cell(0,0) = %q, want 'H'", got)
	}
}

// ---------------------------------------------------------------------------
// Tab — clamped to last column when tab stop exceeds screen width
// ---------------------------------------------------------------------------

func TestWrite_Tab_ClampedAtLastCol(t *testing.T) {
	s := newVTScreen(5, 10) // 10 columns: last col = 9
	s.curCol = 7
	s.write([]byte("\t")) // next tab stop = 8; 8 < 10, so curCol = 8
	if s.curCol != 8 {
		t.Errorf("tab from col7 in 10-col screen: curCol = %d, want 8", s.curCol)
	}
	// A second tab from col8 would go to 16, clamped to 9.
	s.write([]byte("\t"))
	if s.curCol != 9 {
		t.Errorf("tab from col8 clamped: curCol = %d, want 9", s.curCol)
	}
}

// ---------------------------------------------------------------------------
// Resize resets scroll region to full screen
// ---------------------------------------------------------------------------

func TestResize_ResetsScrollRegion(t *testing.T) {
	s := newVTScreen(10, 20)
	s.write([]byte("\x1b[3;7r")) // restrict to rows 3-7
	s.resize(12, 20)
	if s.scrollTop != 0 || s.scrollBot != 11 {
		t.Errorf("after resize: scrollTop=%d scrollBot=%d, want 0/11", s.scrollTop, s.scrollBot)
	}
}

// ---------------------------------------------------------------------------
// ESC ReverseIndex — not at scroll top just moves cursor up
// ---------------------------------------------------------------------------

func TestESC_ReverseIndex_NotAtScrollTop(t *testing.T) {
	s := newVTScreen(10, 10)
	s.curRow = 4
	s.write([]byte("\x1bM"))
	if s.curRow != 3 {
		t.Errorf("ESC M not at scrollTop: curRow = %d, want 3", s.curRow)
	}
}
