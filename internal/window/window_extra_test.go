package window

import (
	"testing"

	"github.com/skybert/gomacs/internal/buffer"
)

// ---- ScrollLine getter/setter ----------------------------------------------

func TestScrollLineGetterSetter(t *testing.T) {
	w, _ := newFiveLineWindow(5)
	w.SetScrollLine(3)
	if got := w.ScrollLine(); got != 3 {
		t.Errorf("ScrollLine() = %d, want 3", got)
	}
}

// ---- Left() for non-zero left offset ---------------------------------------

func TestLeftNonZero(t *testing.T) {
	buf := buffer.New("t")
	w := New(buf, 0, 40, 80, 10)
	if got := w.Left(); got != 40 {
		t.Errorf("Left() = %d, want 40", got)
	}
}

func TestTopNonZero(t *testing.T) {
	buf := buffer.New("t")
	w := New(buf, 5, 0, 80, 10)
	if got := w.Top(); got != 5 {
		t.Errorf("Top() = %d, want 5", got)
	}
}

// ---- Width / Height --------------------------------------------------------

func TestWidthHeight(t *testing.T) {
	buf := buffer.New("t")
	w := New(buf, 0, 0, 120, 30)
	if got := w.Width(); got != 120 {
		t.Errorf("Width() = %d, want 120", got)
	}
	if got := w.Height(); got != 30 {
		t.Errorf("Height() = %d, want 30", got)
	}
}

// ---- Point -----------------------------------------------------------------

func TestWindowPoint(t *testing.T) {
	buf := buffer.NewWithContent("t", "hello")
	w := New(buf, 0, 0, 80, 5)
	w.SetPoint(3)
	if got := w.Point(); got != 3 {
		t.Errorf("Point() = %d, want 3", got)
	}
}

// ---- SetBuf ----------------------------------------------------------------

func TestSetBufResetsPoint(t *testing.T) {
	buf1 := buffer.NewWithContent("t1", "hello world")
	buf2 := buffer.NewWithContent("t2", "foo")
	w := New(buf1, 0, 0, 80, 5)
	w.SetPoint(5)
	w.SetScrollLine(1)
	w.SetBuf(buf2)
	if w.Buf() != buf2 {
		t.Error("SetBuf: Buf() did not change")
	}
	if w.Point() != buf2.Point() {
		t.Errorf("SetBuf: Point() = %d, want %d", w.Point(), buf2.Point())
	}
	if w.ScrollLine() != 1 {
		t.Errorf("SetBuf: ScrollLine() = %d, want 1", w.ScrollLine())
	}
}

// ---- IsMinibuffer ----------------------------------------------------------

func TestIsMinibufferFalseForNormal(t *testing.T) {
	w, _ := newFiveLineWindow(5)
	if w.IsMinibuffer() {
		t.Error("normal window should not be minibuffer")
	}
}

// ---- WrapCol ---------------------------------------------------------------

func TestWrapColDefaultZero(t *testing.T) {
	w, _ := newFiveLineWindow(5)
	if got := w.WrapCol(); got != 0 {
		t.Errorf("WrapCol() = %d, want 0", got)
	}
}

func TestSetWrapCol(t *testing.T) {
	w, _ := newFiveLineWindow(5)
	w.SetWrapCol(40)
	if got := w.WrapCol(); got != 40 {
		t.Errorf("WrapCol() = %d, want 40", got)
	}
	w.SetWrapCol(0) // disable
	if got := w.WrapCol(); got != 0 {
		t.Errorf("WrapCol() after disable = %d, want 0", got)
	}
}

// ---- ViewLines with scroll position ----------------------------------------

func TestViewLinesScrolled(t *testing.T) {
	w, buf := newFiveLineWindow(3) // height 3, scrollLine starts at 1
	w.SetScrollLine(3)             // show lines 3, 4, 5

	vl := w.ViewLines()
	if len(vl) != 3 {
		t.Fatalf("expected 3 ViewLines, got %d", len(vl))
	}
	for i, want := range []int{3, 4, 5} {
		if vl[i].Line != want {
			t.Errorf("row %d: Line = %d, want %d", i, vl[i].Line, want)
		}
		text := buf.Substring(vl[i].StartPos, vl[i].EndPos)
		_ = text // content correctness already covered by TestViewLinesText
	}
}

func TestViewLinesAllPastEnd(t *testing.T) {
	// Buffer has 2 lines. Start scroll at line 3 → all rows are past EOF.
	// Line 3 doesn't exist (buffer only has lines 1 and 2), so all ViewLine
	// entries should have Line==0.
	buf := buffer.NewWithContent("t", "line1\nline2")
	w := New(buf, 0, 0, 80, 3)
	// 2 lines total; clamp to max (2); then scroll to line 2+1 manually via
	// direct field access is not available — instead verify that rows beyond
	// the last buffer line are Line==0. We use height=5 and scroll to line 2:
	// row 0 shows line 2, rows 1..4 are past EOF.
	w.SetScrollLine(2)
	w2 := New(buf, 0, 0, 80, 5)
	w2.SetScrollLine(2)
	vl := w2.ViewLines()
	if len(vl) != 5 {
		t.Fatalf("expected 5 ViewLines, got %d", len(vl))
	}
	// row 0 is line 2 (valid)
	if vl[0].Line != 2 {
		t.Errorf("row 0: Line = %d, want 2", vl[0].Line)
	}
	// rows 1..4 are past EOF
	for i := 1; i < 5; i++ {
		if vl[i].Line != 0 {
			t.Errorf("row %d: Line = %d, want 0 (past EOF)", i, vl[i].Line)
		}
	}
}

// ---- ScrollUp zero is no-op ------------------------------------------------

func TestScrollUpZeroNoOp(t *testing.T) {
	w, _ := newFiveLineWindow(3)
	w.ScrollUp(0)
	if got := w.ScrollLine(); got != 1 {
		t.Errorf("ScrollUp(0): ScrollLine = %d, want 1", got)
	}
}

// ---- Row fields in ViewLines -----------------------------------------------

func TestViewLinesRowFieldMatchesTopPlusIndex(t *testing.T) {
	buf := buffer.NewWithContent("t", fiveLineContent)
	// top=5, so rows should be 5, 6, 7.
	w := New(buf, 5, 0, 80, 3)

	vl := w.ViewLines()
	for i, v := range vl {
		wantRow := 5 + i
		if v.Row != wantRow {
			t.Errorf("ViewLine[%d].Row = %d, want %d", i, v.Row, wantRow)
		}
	}
}

// ---- ViewLines (no-wrap) StartPos/EndPos correctness -----------------------

func TestViewLinesStartEndPos(t *testing.T) {
	// "hello\nworld" — line1: [0,5), line2: [6,11)
	buf := buffer.NewWithContent("t", "hello\nworld")
	w := New(buf, 0, 0, 80, 2)
	vl := w.ViewLines()

	if len(vl) < 2 {
		t.Fatalf("expected 2 ViewLines, got %d", len(vl))
	}
	if vl[0].StartPos != 0 || vl[0].EndPos != 5 {
		t.Errorf("line1: StartPos=%d EndPos=%d, want 0,5", vl[0].StartPos, vl[0].EndPos)
	}
	if vl[1].StartPos != 6 || vl[1].EndPos != 11 {
		t.Errorf("line2: StartPos=%d EndPos=%d, want 6,11", vl[1].StartPos, vl[1].EndPos)
	}
}

// ---- ScrollUp with positive n ----------------------------------------------

func TestScrollUpPositive(t *testing.T) {
	// 10 lines of content, window height 3
	content := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10"
	b := buffer.NewWithContent("t", content)
	w := New(b, 0, 0, 80, 3)
	// scrollLine starts at 1 (minimum); ScrollUp(3) → 1+3=4
	w.ScrollUp(3)
	if got := w.ScrollLine(); got != 4 {
		t.Errorf("ScrollUp(3): ScrollLine = %d, want 4", got)
	}
}

func TestScrollUpClampsToMax(t *testing.T) {
	// 3-line buffer, height 3. ScrollLine can't go past lineCount-1.
	b := buffer.NewWithContent("t", "a\nb\nc")
	w := New(b, 0, 0, 80, 3)
	w.SetScrollLine(0)
	w.ScrollUp(100)
	// Should clamp to 3 (lineCount) rather than 100.
	if got := w.ScrollLine(); got > 3 {
		t.Errorf("ScrollUp(100): ScrollLine = %d, want <= 3", got)
	}
}

func TestScrollUpNegativeNoOp(t *testing.T) {
	w, _ := newFiveLineWindow(3)
	initial := w.ScrollLine()
	w.ScrollUp(-1)
	if got := w.ScrollLine(); got != initial {
		t.Errorf("ScrollUp(-1): ScrollLine = %d, want %d (unchanged)", got, initial)
	}
}

func TestScrollUpUpdatesScrollCache(t *testing.T) {
	// Seed the scroll cache by calling ViewLines(), then ScrollUp() should
	// take the fast cache-update path.
	content := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10"
	b := buffer.NewWithContent("t", content)
	w := New(b, 0, 0, 80, 4)
	w.SetScrollLine(1)
	// Seed the cache.
	_ = w.ViewLines()
	// Now scroll up: cachedScrollLine==1==scrollLine, so cache update runs.
	w.ScrollUp(2)
	if got := w.ScrollLine(); got != 3 {
		t.Errorf("ScrollUp(2) from 1: ScrollLine = %d, want 3", got)
	}
}

func TestScrollUpAlreadyAtMax(t *testing.T) {
	// 2-line buffer: max scrollLine=2. If already at 2, ScrollUp(1) clamps
	// to 2 and delta==0, so the cache-update path is skipped.
	b := buffer.NewWithContent("t", "a\nb")
	w := New(b, 0, 0, 80, 3)
	w.SetScrollLine(2) // already at max
	_ = w.ViewLines()  // seed cache
	w.ScrollUp(1)
	if got := w.ScrollLine(); got != 2 {
		t.Errorf("ScrollUp(1) at max: ScrollLine = %d, want 2", got)
	}
}

// ---- EnsurePointVisible ----------------------------------------------------

func TestEnsurePointVisiblePointBelow(t *testing.T) {
	// 5-line buffer, height 3. Scroll to line 3, put point on line 1.
	// EnsurePointVisible should scroll back to show line 1.
	content := fiveLineContent
	b := buffer.NewWithContent("t", content)
	w := New(b, 0, 0, 80, 4) // height 4: 3 text rows + 1 modeline
	w.SetScrollLine(3)
	w.SetPoint(0) // line 1
	w.EnsurePointVisible()
	if w.ScrollLine() > 1 {
		t.Errorf("EnsurePointVisible: ScrollLine = %d, want <= 1 so line 1 is visible", w.ScrollLine())
	}
}

func TestEnsurePointVisiblePointAbove(t *testing.T) {
	// 5-line buffer, height 3. Point is on line 5 but window shows lines 1-2.
	b := buffer.NewWithContent("t", fiveLineContent)
	w := New(b, 0, 0, 80, 4) // height 4: 3 text rows + 1 modeline
	w.SetScrollLine(1)
	// Put point on last character (line 5).
	w.SetPoint(b.Len() - 1)
	w.EnsurePointVisible()
	// After ensuring visibility, scrollLine should be set so line 5 is visible.
	line, _ := b.LineCol(w.Point())
	textH := 3 // textRows = height - 1
	if w.ScrollLine()+textH <= line {
		t.Errorf("EnsurePointVisible: point line %d not visible with scrollLine=%d, textH=%d", line, w.ScrollLine(), textH)
	}
}

// ---- textRows for minibuffer (full height, no modeline row) ----------------

func TestTextRowsMinibuffer(t *testing.T) {
	b := buffer.New("t")
	w := NewMinibuffer(b, 0, 0, 80) // height=1 always for minibuffer
	// Minibuffer uses full height for text (no modeline reserved).
	vl := w.ViewLines()
	if len(vl) != 1 {
		t.Errorf("minibuffer ViewLines: len = %d, want 1 (full height, no modeline)", len(vl))
	}
}

func TestTextRowsNormalWindowReservesModeline(t *testing.T) {
	// For EnsurePointVisible, a height-5 normal window should only use
	// 4 rows for text (the last row is for the modeline).
	// We verify this by placing point on a line just past the 4-row view
	// and checking EnsurePointVisible scrolls to accommodate it.
	b := buffer.NewWithContent("t", "a\nb\nc\nd\ne\nf\ng\nh")
	w := New(b, 0, 0, 80, 5) // height=5 → textRows=4
	w.SetScrollLine(1)
	// Point on line 6 (beyond the 4-row view starting at line 1).
	w.SetPoint(b.LineStart(6))
	w.EnsurePointVisible()
	// scrollLine should have been updated so line 6 is within rows [scrollLine, scrollLine+4).
	line, _ := b.LineCol(w.Point())
	textH := 4
	start := w.ScrollLine()
	if line < start || line >= start+textH {
		t.Errorf("point line %d not visible in [%d,%d)", line, start, start+textH)
	}
}

// ---- Scroll cache correctness after mutations ------------------------------

func TestScrollCacheInvalidatedAfterEdit(t *testing.T) {
	w, buf := newFiveLineWindow(3)
	// Seed the cache.
	_ = w.ViewLines()
	// Now mutate the buffer (insert a newline to shift line positions).
	buf.InsertString(0, "new\n")
	// ViewLines should reflect the updated content.
	vl := w.ViewLines()
	if vl[0].Line != 1 {
		t.Errorf("after insert: ViewLine[0].Line = %d, want 1", vl[0].Line)
	}
	text := buf.Substring(vl[0].StartPos, vl[0].EndPos)
	if text != "new" {
		t.Errorf("after insert: ViewLine[0] text = %q, want %q", text, "new")
	}
}
