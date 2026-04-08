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
