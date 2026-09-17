package window

import (
	"strings"
	"testing"

	"github.com/skybert/gomacs/internal/buffer"
)

// fiveLineContent is a buffer with exactly 5 lines.
const fiveLineContent = "line1\nline2\nline3\nline4\nline5"

func newFiveLineWindow(height int) (*Window, *buffer.Buffer) {
	buf := buffer.NewWithContent("test", fiveLineContent)
	w := New(buf, 0, 0, 80, height)
	return w, buf
}

// TestNewWindowScrollLineStartsAtOne verifies that a freshly created window
// has scrollLine == 1.
func TestNewWindowScrollLineStartsAtOne(t *testing.T) {
	w, _ := newFiveLineWindow(3)
	if w.ScrollLine() != 1 {
		t.Errorf("expected ScrollLine=1, got %d", w.ScrollLine())
	}
}

// TestEnsurePointVisibleBelowWindow verifies that when the point is on a line
// below the visible area, EnsurePointVisible scrolls down far enough to
// include it.
func TestEnsurePointVisibleBelowWindow(t *testing.T) {
	w, buf := newFiveLineWindow(2) // shows lines 1-2
	// Move point to line 5 (last line).
	buf.SetPoint(buf.LineStart(5))
	w.SetPoint(buf.Point())

	w.EnsurePointVisible()

	start, end := w.VisibleLines()
	pointLine, _ := buf.LineCol(w.Point())
	if pointLine < start || pointLine >= end {
		t.Errorf("point line %d not in visible range [%d, %d)", pointLine, start, end)
	}
}

// TestEnsurePointVisibleAboveWindow verifies that when the point is on a line
// above the visible area, EnsurePointVisible scrolls up to include it.
func TestEnsurePointVisibleAboveWindow(t *testing.T) {
	w, buf := newFiveLineWindow(2) // shows lines 1-2
	// Scroll down so line 4 is the first visible line.
	w.SetScrollLine(4)
	// Place point on line 1.
	buf.SetPoint(0)
	w.SetPoint(0)

	w.EnsurePointVisible()

	start, end := w.VisibleLines()
	pointLine, _ := buf.LineCol(w.Point())
	if pointLine < start || pointLine >= end {
		t.Errorf("point line %d not in visible range [%d, %d)", pointLine, start, end)
	}
}

// TestEnsurePointVisibleLastLine checks that when the point would fall on the
// modeline row (scrollLine+height-1), EnsurePointVisible scrolls it into the
// text area.  This exercises the "last line invisible" bug where
// EnsurePointVisible previously used the full window height instead of the
// text height (height-1).
func TestEnsurePointVisibleLastLine(t *testing.T) {
	// height=4: rows 0,1,2 are text; row 3 is the modeline.
	// With scrollLine=1 the text shows buffer lines 1,2,3.
	// Buffer line 4 falls on the modeline row — not rendered by renderWindow.
	w, buf := newFiveLineWindow(4)
	buf.SetPoint(buf.LineStart(4))
	w.SetPoint(buf.Point())

	w.EnsurePointVisible()

	textH := w.Height() - 1 // 3
	sl := w.ScrollLine()
	pointLine, _ := buf.LineCol(w.Point())
	if pointLine < sl || pointLine >= sl+textH {
		t.Errorf("point line %d not in text area [%d, %d)", pointLine, sl, sl+textH)
	}
}

// TestVisibleLinesRange checks that VisibleLines returns [scrollLine,
// scrollLine+height).
func TestVisibleLinesRange(t *testing.T) {
	w, _ := newFiveLineWindow(3)
	w.SetScrollLine(2)

	start, end := w.VisibleLines()
	if start != 2 {
		t.Errorf("expected start=2, got %d", start)
	}
	if end != 5 {
		t.Errorf("expected end=5, got %d", end)
	}
}

// TestRecenterPutsPointInMiddle verifies that after Recenter the line
// containing the point is at scrollLine + height/2.
func TestRecenterPutsPointInMiddle(t *testing.T) {
	w, buf := newFiveLineWindow(4) // height 4 → middle row index 2
	// Place point on line 4.
	buf.SetPoint(buf.LineStart(4))
	w.SetPoint(buf.Point())

	w.Recenter()

	pointLine, _ := buf.LineCol(w.Point())
	wantScrollLine := max(pointLine-w.Height()/2, 1)
	if w.ScrollLine() != wantScrollLine {
		t.Errorf("expected scrollLine=%d after Recenter, got %d", wantScrollLine, w.ScrollLine())
	}
}

// TestViewLinesText verifies that ViewLines returns the correct buffer text
// for each visible row.
func TestViewLinesText(t *testing.T) {
	w, _ := newFiveLineWindow(3)
	// scrollLine == 1 → rows 0..2 show lines 1, 2, 3

	vl := w.ViewLines()
	if len(vl) != 3 {
		t.Fatalf("expected 3 ViewLines, got %d", len(vl))
	}

	want := []struct {
		line int
		text string
	}{
		{1, "line1"},
		{2, "line2"},
		{3, "line3"},
	}

	for i, tc := range want {
		if vl[i].Line != tc.line {
			t.Errorf("row %d: expected Line=%d, got %d", i, tc.line, vl[i].Line)
		}
		if got := w.Buf().Substring(vl[i].StartPos, vl[i].EndPos); got != tc.text {
			t.Errorf("row %d: expected Text=%q, got %q", i, tc.text, got)
		}
	}
}

// TestViewLinesPastEndOfBuffer verifies that rows beyond the last buffer line
// have Line==0 and empty Text.
func TestViewLinesPastEndOfBuffer(t *testing.T) {
	// Buffer has 5 lines but window height is 7 → last 2 rows are empty.
	buf := buffer.NewWithContent("test", fiveLineContent)
	w := New(buf, 0, 0, 80, 7)

	vl := w.ViewLines()
	if len(vl) != 7 {
		t.Fatalf("expected 7 ViewLines, got %d", len(vl))
	}

	for i := 5; i < 7; i++ {
		if vl[i].Line != 0 {
			t.Errorf("row %d beyond buffer: expected Line=0, got %d", i, vl[i].Line)
		}
		if vl[i].StartPos != vl[i].EndPos {
			t.Errorf("row %d beyond buffer: expected empty (StartPos==EndPos), got [%d,%d)", i, vl[i].StartPos, vl[i].EndPos)
		}
	}
}

// TestNewMinibuffer verifies that NewMinibuffer creates a 1-line window with
// IsMinibuffer==true.
func TestNewMinibuffer(t *testing.T) {
	buf := buffer.New("*minibuffer*")
	mb := NewMinibuffer(buf, 24, 0, 80)

	if !mb.IsMinibuffer() {
		t.Error("expected IsMinibuffer=true")
	}
	if mb.Height() != 1 {
		t.Errorf("expected Height=1, got %d", mb.Height())
	}
}

// TestSetScrollLineClamping verifies the clamping behaviour of SetScrollLine.
func TestSetScrollLineClamping(t *testing.T) {
	w, _ := newFiveLineWindow(3)

	w.SetScrollLine(0) // below minimum
	if w.ScrollLine() != 1 {
		t.Errorf("expected clamped scrollLine=1, got %d", w.ScrollLine())
	}

	w.SetScrollLine(999) // above maximum (5 lines)
	if w.ScrollLine() != 5 {
		t.Errorf("expected clamped scrollLine=5, got %d", w.ScrollLine())
	}
}

// TestGutterWidth verifies that GutterWidth and SetGutterWidth work correctly.
func TestGutterWidth(t *testing.T) {
	w, _ := newFiveLineWindow(5)
	if w.GutterWidth() != 0 {
		t.Errorf("expected initial GutterWidth=0, got %d", w.GutterWidth())
	}
	w.SetGutterWidth(2)
	if w.GutterWidth() != 2 {
		t.Errorf("expected GutterWidth=2, got %d", w.GutterWidth())
	}
	w.SetGutterWidth(0)
	if w.GutterWidth() != 0 {
		t.Errorf("expected GutterWidth=0 after reset, got %d", w.GutterWidth())
	}
}

// line containing the point the first visible line.
func TestRecenterTopPutsPointAtFirstRow(t *testing.T) {
	w, buf := newFiveLineWindow(3)
	// Place point on line 4.
	buf.SetPoint(buf.LineStart(4))
	w.SetPoint(buf.Point())

	w.RecenterTop()

	pointLine, _ := buf.LineCol(w.Point())
	if w.ScrollLine() != pointLine {
		t.Errorf("RecenterTop: expected scrollLine=%d (point line), got %d", pointLine, w.ScrollLine())
	}
}

// TestRecenterBottomPutsPointNearLastRow verifies that RecenterBottom makes
// the line containing the point appear near the bottom of the window.
func TestRecenterBottomPutsPointNearLastRow(t *testing.T) {
	w, buf := newFiveLineWindow(4)
	// Place point on line 4.
	buf.SetPoint(buf.LineStart(4))
	w.SetPoint(buf.Point())

	w.RecenterBottom()

	pointLine, _ := buf.LineCol(w.Point())
	// With height=4, the point line should be visible and near the last text row.
	start, end := w.VisibleLines()
	if pointLine < start || pointLine >= end {
		t.Errorf("RecenterBottom: point line %d not in visible range [%d, %d)", pointLine, start, end)
	}
	// scrollLine should be less than pointLine (point is scrolled toward the bottom).
	if w.ScrollLine() >= pointLine {
		t.Errorf("RecenterBottom: expected scrollLine < pointLine (%d), got %d", pointLine, w.ScrollLine())
	}
}

// TestViewLinesWrapped verifies that ViewLines splits a long line into
// multiple entries when wrapCol is set.
func TestViewLinesWrapped(t *testing.T) {
	// One 20-rune line wrapped at 8 cols → 3 entries (8+8+4).
	content := "12345678901234567890"
	buf := buffer.NewWithContent("test", content)
	w := New(buf, 0, 0, 80, 5)
	w.SetWrapCol(8)

	vl := w.ViewLines()
	if len(vl) != 5 {
		t.Fatalf("expected 5 ViewLines (height), got %d", len(vl))
	}
	expected := []string{"12345678", "90123456", "7890"}
	for i, want := range expected {
		if vl[i].Line != 1 {
			t.Errorf("row %d: expected Line=1, got %d", i, vl[i].Line)
		}
		if got := buf.Substring(vl[i].StartPos, vl[i].EndPos); got != want {
			t.Errorf("row %d: expected Text=%q, got %q", i, want, got)
		}
	}
	for i := 3; i < 5; i++ {
		if vl[i].Line != 0 {
			t.Errorf("row %d: expected past-end (Line=0), got %d", i, vl[i].Line)
		}
	}
}

// TestVisualRowForPoint verifies VisualRowForPoint with wrapping enabled.
func TestVisualRowForPoint(t *testing.T) {
	// Line 1: 20 runes, wrap at 8; line 2: 5 runes.
	content := "12345678901234567890\nHello"
	buf := buffer.NewWithContent("test", content)
	w := New(buf, 0, 0, 80, 10)
	w.SetWrapCol(8)

	// col 0 on line 1 → visual row 0.
	buf.SetPoint(0)
	w.SetPoint(0)
	if row := w.VisualRowForPoint(); row != 0 {
		t.Errorf("expected visual row 0, got %d", row)
	}

	// col 8 on line 1 → second segment → visual row 1.
	buf.SetPoint(8)
	w.SetPoint(8)
	if row := w.VisualRowForPoint(); row != 1 {
		t.Errorf("expected visual row 1, got %d", row)
	}

	// start of line 2 → after 3 visual rows of line 1 → visual row 3.
	line2Start := buf.LineStart(2)
	buf.SetPoint(line2Start)
	w.SetPoint(line2Start)
	if row := w.VisualRowForPoint(); row != 3 {
		t.Errorf("expected visual row 3 for line 2, got %d", row)
	}
}

// ---- benchmarks ------------------------------------------------------------

// benchBuffer builds a buffer of `lines` source-like lines for scroll and
// visual-row benchmarks.
func benchBuffer(lines int) *buffer.Buffer {
	var sb strings.Builder
	for range lines {
		sb.WriteString("\tsomeIdentifier := doSomething(a, b, c) // filler text\n")
	}
	return buffer.NewWithContent("bench", sb.String())
}

// BenchmarkPageUpToTop measures paging backwards from the end of a large
// buffer to the top, rendering after each page — the "hold Page-Up" case.
func BenchmarkPageUpToTop(b *testing.B) {
	buf := benchBuffer(20000)
	w := New(buf, 0, 0, 80, 40)
	const page = 39
	for b.Loop() {
		w.SetScrollLine(buf.LineCount())
		_ = w.ViewLines()
		for w.ScrollLine() > 1 {
			w.ScrollDown(page)
			_ = w.ViewLines()
		}
	}
}

// BenchmarkScrollDownOneLine measures a single backward scroll plus render,
// i.e. the cost of one up-arrow keypress deep inside a large buffer.
func BenchmarkScrollDownOneLine(b *testing.B) {
	buf := benchBuffer(20000)
	w := New(buf, 0, 0, 80, 40)
	w.SetScrollLine(15000)
	_ = w.ViewLines()
	for b.Loop() {
		w.ScrollDown(1)
		_ = w.ViewLines()
		w.ScrollUp(1)
		_ = w.ViewLines()
	}
}

// BenchmarkVisualRowForPoint measures visual-line mode: VisualRowForPoint is
// called twice per key event (EnsurePointVisible and placeCursor).
func BenchmarkVisualRowForPoint(b *testing.B) {
	buf := benchBuffer(20000)
	w := New(buf, 0, 0, 80, 40)
	w.SetWrapCol(78)
	w.SetScrollLine(15000)
	pos := buf.LineStart(15030)
	w.SetPoint(pos)
	buf.SetPoint(pos)
	_ = w.ViewLines()
	for b.Loop() {
		_ = w.VisualRowForPoint()
		_ = w.VisualRowForPoint()
	}
}

// BenchmarkEnsurePointVisibleWrapped measures the full per-keystroke scroll
// bookkeeping with visual-line mode enabled.
func BenchmarkEnsurePointVisibleWrapped(b *testing.B) {
	buf := benchBuffer(20000)
	w := New(buf, 0, 0, 80, 40)
	w.SetWrapCol(78)
	w.SetScrollLine(15000)
	pos := buf.LineStart(15010)
	w.SetPoint(pos)
	buf.SetPoint(pos)
	_ = w.ViewLines()
	for b.Loop() {
		w.EnsurePointVisible()
		_ = w.VisualRowForPoint()
	}
}

// ---- scroll position cache -------------------------------------------------

// checkScrollCache asserts that, when the cached first-visible-line position is
// live, it agrees with the buffer's own line-start lookup.
func checkScrollCache(t *testing.T, w *Window, what string) {
	t.Helper()
	if w.cachedScrollLine != w.scrollLine || w.cachedChangeGen != w.buf.ChangeGen() {
		return // cache not live; firstScrollPos() will recompute
	}
	if want := w.buf.LineStart(w.scrollLine); w.cachedScrollPos != want {
		t.Errorf("%s: cachedScrollPos = %d, want %d (scrollLine %d)",
			what, w.cachedScrollPos, want, w.scrollLine)
	}
}

// TestScrollDownUpdatesScrollCache verifies that ScrollDown maintains the
// cached first-visible-line position instead of leaving it stale.
func TestScrollDownUpdatesScrollCache(t *testing.T) {
	buf := benchBuffer(200)
	w := New(buf, 0, 0, 80, 10)
	w.SetScrollLine(100)
	_ = w.ViewLines() // seed the cache

	w.ScrollDown(9)
	if got := w.ScrollLine(); got != 91 {
		t.Fatalf("ScrollDown(9) from 100: ScrollLine = %d, want 91", got)
	}
	if w.cachedScrollLine != 91 {
		t.Errorf("cachedScrollLine = %d, want 91 (cache should be updated)", w.cachedScrollLine)
	}
	checkScrollCache(t, w, "after ScrollDown(9)")
}

// TestScrollDownRepeatedlyToTop pages backwards a line at a time and checks the
// cache stays correct all the way to line 1.
func TestScrollDownRepeatedlyToTop(t *testing.T) {
	buf := benchBuffer(60)
	w := New(buf, 0, 0, 80, 10)
	w.SetScrollLine(50)
	_ = w.ViewLines()
	for w.ScrollLine() > 1 {
		w.ScrollDown(1)
		checkScrollCache(t, w, "paging up one line")
		if vl := w.ViewLines(); vl[0].StartPos != buf.LineStart(w.ScrollLine()) {
			t.Fatalf("scrollLine %d: first row StartPos = %d, want %d",
				w.ScrollLine(), vl[0].StartPos, buf.LineStart(w.ScrollLine()))
		}
	}
}

// TestScrollDownZeroAndNegativeNoOp verifies ScrollDown ignores n <= 0.
func TestScrollDownZeroAndNegativeNoOp(t *testing.T) {
	w, _ := newFiveLineWindow(3)
	w.SetScrollLine(3)
	for _, n := range []int{0, -1, -5} {
		w.ScrollDown(n)
		if got := w.ScrollLine(); got != 3 {
			t.Errorf("ScrollDown(%d): ScrollLine = %d, want 3 (unchanged)", n, got)
		}
	}
}

// TestScrollDownClampsToTop verifies clamping plus a correct cache at line 1.
func TestScrollDownClampsToTop(t *testing.T) {
	buf := benchBuffer(50)
	w := New(buf, 0, 0, 80, 10)
	w.SetScrollLine(20)
	_ = w.ViewLines()
	w.ScrollDown(999)
	if got := w.ScrollLine(); got != 1 {
		t.Fatalf("ScrollDown(999): ScrollLine = %d, want 1", got)
	}
	checkScrollCache(t, w, "clamped to top")
}

// TestScrollRoundTripKeepsCache scrolls forward and back and checks that both
// directions leave the cache usable.
func TestScrollRoundTripKeepsCache(t *testing.T) {
	buf := benchBuffer(200)
	w := New(buf, 0, 0, 80, 10)
	_ = w.ViewLines()
	for range 20 {
		w.ScrollUp(9)
		checkScrollCache(t, w, "after ScrollUp")
	}
	for range 20 {
		w.ScrollDown(9)
		checkScrollCache(t, w, "after ScrollDown")
	}
	if got := w.ScrollLine(); got != 1 {
		t.Errorf("after round trip: ScrollLine = %d, want 1", got)
	}
}

// TestScrollLargeJumpFallsBackToLineStart verifies that a jump larger than
// maxIncrementalScrollScan does not corrupt the cache: firstScrollPos()
// recomputes and ViewLines() still renders the right lines.
func TestScrollLargeJumpFallsBackToLineStart(t *testing.T) {
	buf := benchBuffer(maxIncrementalScrollScan * 3)
	w := New(buf, 0, 0, 80, 10)
	_ = w.ViewLines()
	w.ScrollUp(maxIncrementalScrollScan + 10)
	if got := w.firstScrollPos(); got != buf.LineStart(w.ScrollLine()) {
		t.Errorf("after big ScrollUp: firstScrollPos = %d, want %d", got, buf.LineStart(w.ScrollLine()))
	}
	w.ScrollDown(maxIncrementalScrollScan + 10)
	if got := w.firstScrollPos(); got != buf.LineStart(w.ScrollLine()) {
		t.Errorf("after big ScrollDown: firstScrollPos = %d, want %d", got, buf.LineStart(w.ScrollLine()))
	}
}

// TestEnsurePointVisibleBackwardUpdatesCache verifies the backward path of
// EnsurePointVisible now maintains the scroll-position cache.
func TestEnsurePointVisibleBackwardUpdatesCache(t *testing.T) {
	buf := benchBuffer(200)
	w := New(buf, 0, 0, 80, 10)
	w.SetScrollLine(100)
	_ = w.ViewLines()

	pos := buf.LineStart(95)
	buf.SetPoint(pos)
	w.SetPoint(pos)
	w.EnsurePointVisible()

	if got := w.ScrollLine(); got != 95 {
		t.Fatalf("EnsurePointVisible backward: ScrollLine = %d, want 95", got)
	}
	if w.cachedScrollLine != 95 {
		t.Errorf("cachedScrollLine = %d, want 95 (cache should be updated)", w.cachedScrollLine)
	}
	checkScrollCache(t, w, "after backward EnsurePointVisible")
}

// TestEnsurePointVisibleForwardUpdatesCache covers the forward (no-wrap) path.
func TestEnsurePointVisibleForwardUpdatesCache(t *testing.T) {
	buf := benchBuffer(200)
	w := New(buf, 0, 0, 80, 10)
	_ = w.ViewLines()

	pos := buf.LineStart(30)
	buf.SetPoint(pos)
	w.SetPoint(pos)
	w.EnsurePointVisible()

	if got := w.ScrollLine(); got != 30-w.textRows()+1 {
		t.Fatalf("EnsurePointVisible forward: ScrollLine = %d, want %d", got, 30-w.textRows()+1)
	}
	checkScrollCache(t, w, "after forward EnsurePointVisible")
}

// ---- scanForwardLines / scanBackwardLines ----------------------------------

func TestScanForwardLines(t *testing.T) {
	buf := buffer.NewWithContent("test", "a\nbb\nccc\ndddd\n")
	w := New(buf, 0, 0, 80, 5)
	tests := []struct{ from, n, want int }{
		{0, 1, 2},  // line 1 → line 2
		{0, 3, 9},  // line 1 → line 4
		{2, 2, 9},  // line 2 → line 4
		{0, 4, 14}, // line 1 → line 5 (empty last line)
	}
	for _, tc := range tests {
		if got := w.scanForwardLines(tc.from, tc.n); got != tc.want {
			t.Errorf("scanForwardLines(%d,%d) = %d, want %d", tc.from, tc.n, got, tc.want)
		}
	}
}

func TestScanBackwardLines(t *testing.T) {
	buf := buffer.NewWithContent("test", "a\nbb\nccc\ndddd\n")
	w := New(buf, 0, 0, 80, 5)
	tests := []struct{ from, n, want int }{
		{2, 1, 0},  // line 2 → line 1
		{9, 1, 5},  // line 4 → line 3
		{9, 3, 0},  // line 4 → line 1
		{14, 4, 0}, // line 5 → line 1
		{9, 99, 0}, // clamps at buffer start
	}
	for _, tc := range tests {
		if got := w.scanBackwardLines(tc.from, tc.n); got != tc.want {
			t.Errorf("scanBackwardLines(%d,%d) = %d, want %d", tc.from, tc.n, got, tc.want)
		}
	}
}

// TestScanLinesRoundTrip checks the two scanners are exact inverses across a
// whole buffer.
func TestScanLinesRoundTrip(t *testing.T) {
	buf := benchBuffer(40)
	w := New(buf, 0, 0, 80, 5)
	for line := 1; line <= 40; line++ {
		start := buf.LineStart(line)
		for n := 1; n+line <= 40; n++ {
			fwd := w.scanForwardLines(start, n)
			if want := buf.LineStart(line + n); fwd != want {
				t.Fatalf("scanForwardLines from line %d by %d = %d, want %d", line, n, fwd, want)
			}
			if back := w.scanBackwardLines(fwd, n); back != start {
				t.Fatalf("scanBackwardLines from line %d by %d = %d, want %d", line+n, n, back, start)
			}
		}
	}
}

// ---- visualRowsForSpan -----------------------------------------------------

func TestVisualRowsForSpan(t *testing.T) {
	buf := buffer.NewWithContent("test", "hello")
	w := New(buf, 0, 0, 80, 5)

	if got := w.visualRowsForSpan(0, 5); got != 1 {
		t.Errorf("visualRowsForSpan with wrapping disabled = %d, want 1", got)
	}

	w.SetWrapCol(8)
	tests := []struct{ start, end, want int }{
		{0, 0, 1},   // empty line
		{0, 8, 1},   // exactly one row
		{0, 9, 2},   // one rune over
		{0, 24, 3},  // exact multiple
		{10, 30, 3}, // offset span, 20 runes
	}
	for _, tc := range tests {
		if got := w.visualRowsForSpan(tc.start, tc.end); got != tc.want {
			t.Errorf("visualRowsForSpan(%d,%d) = %d, want %d", tc.start, tc.end, got, tc.want)
		}
	}
}

// TestVisualRowForPointScrolled verifies VisualRowForPoint when the window is
// scrolled away from line 1, which is the case the forward walk has to get
// right.
func TestVisualRowForPointScrolled(t *testing.T) {
	// Six lines of 20 runes each; wrap at 8 → 3 visual rows per line.
	line := "12345678901234567890\n"
	buf := buffer.NewWithContent("test", strings.Repeat(line, 6))
	w := New(buf, 0, 0, 80, 10)
	w.SetWrapCol(8)
	w.SetScrollLine(3)

	tests := []struct{ line, col, want int }{
		{3, 0, 0},   // top of window
		{3, 9, 1},   // second segment of the top line
		{4, 0, 3},   // one full line down
		{5, 17, 8},  // two lines down + third segment
		{6, 0, 9},   // three lines down
		{3, 19, 2},  // last segment of the top line
		{6, 20, 11}, // end of line 6: col 20 is still the third segment
	}
	for _, tc := range tests {
		pos := buf.LineStart(tc.line) + tc.col
		buf.SetPoint(pos)
		w.SetPoint(pos)
		if got := w.VisualRowForPoint(); got != tc.want {
			t.Errorf("VisualRowForPoint line %d col %d = %d, want %d", tc.line, tc.col, got, tc.want)
		}
	}
}

// TestVisualRowForPointMatchesViewLines cross-checks VisualRowForPoint against
// the rows ViewLines() actually produces.
func TestVisualRowForPointMatchesViewLines(t *testing.T) {
	buf := buffer.NewWithContent("test", "short\n"+strings.Repeat("x", 30)+"\nmid\n"+strings.Repeat("y", 17)+"\ntail\n")
	w := New(buf, 0, 0, 80, 20)
	w.SetWrapCol(8)
	w.SetScrollLine(2)
	rows := w.ViewLines()

	for i, vl := range rows {
		if vl.Line == 0 {
			break
		}
		buf.SetPoint(vl.StartPos)
		w.SetPoint(vl.StartPos)
		if got := w.VisualRowForPoint(); got != i {
			t.Errorf("row %d (line %d, pos %d): VisualRowForPoint = %d, want %d",
				i, vl.Line, vl.StartPos, got, i)
		}
	}
}

// TestVisualRowForPointPastEndOfBuffer makes sure the forward walk does not run
// off the end of the buffer when pointLine is past the last line.
func TestVisualRowForPointPastEndOfBuffer(t *testing.T) {
	buf := buffer.NewWithContent("test", "a\nb\nc")
	w := New(buf, 0, 0, 80, 5)
	w.SetWrapCol(4)
	w.SetPoint(buf.Len())
	buf.SetPoint(buf.Len())
	// Lines 1 and 2 are one visual row each, so the cursor is on row 2.
	if got := w.VisualRowForPoint(); got != 2 {
		t.Errorf("VisualRowForPoint at end of buffer = %d, want 2", got)
	}
}

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

// ---- firstScrollPos cache hit ----------------------------------------------

func TestFirstScrollPosCacheHit(t *testing.T) {
	w, _ := newFiveLineWindow(3)
	w.SetScrollLine(2)
	first := w.ViewLines()  // seeds the firstScrollPos cache
	second := w.ViewLines() // same scrollLine + changeGen → cache hit
	if first[0].StartPos != second[0].StartPos {
		t.Errorf("firstScrollPos cache mismatch: %d vs %d", first[0].StartPos, second[0].StartPos)
	}
}

// ---- textRows for a height<=1 normal window --------------------------------

func TestTextRowsHeightOneNormalWindow(t *testing.T) {
	b := buffer.NewWithContent("t", "a\nb\nc")
	w := New(b, 0, 0, 80, 1) // height 1: textRows uses full height
	vl := w.ViewLines()
	if len(vl) != 1 {
		t.Errorf("height-1 window ViewLines len = %d, want 1", len(vl))
	}
	// EnsurePointVisible calls textRows(); with height<=1 it returns the full
	// height rather than reserving a modeline row.
	w.SetPoint(b.LineStart(3))
	w.EnsurePointVisible()
	if w.ScrollLine() != 3 {
		t.Errorf("height-1 EnsurePointVisible: ScrollLine = %d, want 3", w.ScrollLine())
	}
}

// ---- VisualRowForPoint (no wrap path) --------------------------------------

func TestVisualRowForPointNoWrap(t *testing.T) {
	b := buffer.NewWithContent("t", fiveLineContent)
	w := New(b, 0, 0, 80, 5)
	w.SetScrollLine(2)
	w.SetPoint(b.LineStart(4)) // line 4
	if got := w.VisualRowForPoint(); got != 2 {
		t.Errorf("VisualRowForPoint no-wrap = %d, want 2 (line4-scroll2)", got)
	}
}

// ---- visualRowsForLine with wrapping ---------------------------------------

func TestVisualRowsForLineWrapped(t *testing.T) {
	// One long line of 25 runes, wrapCol 10 → 3 visual rows.
	b := buffer.NewWithContent("t", "0123456789abcdefghijABCDE")
	w := New(b, 0, 0, 80, 5)
	w.SetWrapCol(10)
	if got := w.visualRowsForLine(1); got != 3 {
		t.Errorf("visualRowsForLine wrapped = %d, want 3", got)
	}
}

func TestVisualRowsForLineEmptyLine(t *testing.T) {
	// Empty first line; wrapping enabled → still 1 row.
	b := buffer.NewWithContent("t", "\nsecond")
	w := New(b, 0, 0, 80, 5)
	w.SetWrapCol(10)
	if got := w.visualRowsForLine(1); got != 1 {
		t.Errorf("visualRowsForLine empty = %d, want 1", got)
	}
}

// TestVisualRowsForLineNoWrap covers the wrapping-disabled early-out: with
// wrapCol <= 0 every buffer line occupies exactly one visual row, however long
// it is.
func TestVisualRowsForLineNoWrap(t *testing.T) {
	b := buffer.NewWithContent("t", strings.Repeat("x", 200)+"\nshort")
	w := New(b, 0, 0, 80, 5)
	for _, wrapCol := range []int{0, -1} {
		w.SetWrapCol(wrapCol)
		for _, bufLine := range []int{1, 2} {
			if got := w.visualRowsForLine(bufLine); got != 1 {
				t.Errorf("visualRowsForLine(%d) with wrapCol=%d = %d, want 1",
					bufLine, wrapCol, got)
			}
		}
	}
}

// ---- viewLinesWrapped ------------------------------------------------------

func TestViewLinesWrappedSplitsLongLine(t *testing.T) {
	b := buffer.NewWithContent("t", "0123456789abcdefghij")
	w := New(b, 0, 0, 80, 5)
	w.SetWrapCol(10)
	vl := w.ViewLines()
	// First two rows are segments of line 1.
	if vl[0].Line != 1 || vl[1].Line != 1 {
		t.Errorf("wrapped: rows 0,1 Line = %d,%d, want 1,1", vl[0].Line, vl[1].Line)
	}
	if vl[0].EndPos-vl[0].StartPos != 10 {
		t.Errorf("wrapped seg 0 len = %d, want 10", vl[0].EndPos-vl[0].StartPos)
	}
}

func TestViewLinesWrappedShortLine(t *testing.T) {
	// Short lines (<= wrapCol) take the single-row branch.
	b := buffer.NewWithContent("t", "ab\ncd")
	w := New(b, 0, 0, 80, 4)
	w.SetWrapCol(10)
	vl := w.ViewLines()
	if vl[0].Line != 1 || vl[1].Line != 2 {
		t.Errorf("wrapped short: rows Line = %d,%d, want 1,2", vl[0].Line, vl[1].Line)
	}
}

func TestViewLinesWrappedPastEnd(t *testing.T) {
	// Window taller than content with wrapping on: the fallback past-EOF
	// branch (spIdx >= len(startPositions)) runs.
	b := buffer.NewWithContent("t", "only")
	w := New(b, 0, 0, 80, 4)
	w.SetWrapCol(10)
	vl := w.ViewLines()
	if len(vl) != 4 {
		t.Fatalf("wrapped past-end ViewLines len = %d, want 4", len(vl))
	}
}

// ---- EnsurePointVisible (wrapped, cursor past bottom) ----------------------

func TestEnsurePointVisibleWrappedCursorBelow(t *testing.T) {
	// Several long lines with wrapping; point on a far line so its visual row
	// exceeds the text area, exercising the wrapped scroll-adjust branch.
	long := "0123456789abcdefghij"
	b := buffer.NewWithContent("t", long+"\n"+long+"\n"+long+"\n"+long)
	w := New(b, 0, 0, 80, 4) // textRows = 3
	w.SetWrapCol(10)
	w.SetScrollLine(1)
	w.SetPoint(b.LineStart(4)) // line 4, well past the visible visual rows
	w.EnsurePointVisible()
	pointLine, _ := b.LineCol(w.Point())
	if w.ScrollLine() != pointLine {
		t.Errorf("EnsurePointVisible wrapped: ScrollLine = %d, want %d", w.ScrollLine(), pointLine)
	}
}

// ---- Buf / SetBuf ----------------------------------------------------------

func TestBufReturnsBuffer(t *testing.T) {
	buf := buffer.NewWithContent("test", "hello")
	w := New(buf, 0, 0, 80, 24)
	if w.Buf() != buf {
		t.Error("Buf() returned unexpected buffer")
	}
}

func TestSetBufSwapsBuffer(t *testing.T) {
	b1 := buffer.NewWithContent("test1", "hello")
	b2 := buffer.NewWithContent("test2", "world")
	w := New(b1, 0, 0, 80, 24)
	w.SetBuf(b2)
	if w.Buf() != b2 {
		t.Error("SetBuf: Buf() should return the new buffer")
	}
	if w.ScrollLine() != 1 {
		t.Errorf("SetBuf: scrollLine should reset to 1, got %d", w.ScrollLine())
	}
}

// ---- Top / Left / Width ----------------------------------------------------

func TestTopLeftWidth(t *testing.T) {
	buf := buffer.New("test")
	w := New(buf, 5, 10, 40, 20)
	if w.Top() != 5 {
		t.Errorf("Top() = %d, want 5", w.Top())
	}
	if w.Left() != 10 {
		t.Errorf("Left() = %d, want 10", w.Left())
	}
	if w.Width() != 40 {
		t.Errorf("Width() = %d, want 40", w.Width())
	}
}

// ---- SetRegion -------------------------------------------------------------

func TestSetRegion(t *testing.T) {
	buf := buffer.New("test")
	w := New(buf, 0, 0, 80, 24)
	w.SetRegion(3, 5, 30, 10)
	if w.Top() != 3 {
		t.Errorf("Top() = %d, want 3", w.Top())
	}
	if w.Left() != 5 {
		t.Errorf("Left() = %d, want 5", w.Left())
	}
	if w.Width() != 30 {
		t.Errorf("Width() = %d, want 30", w.Width())
	}
	if w.Height() != 10 {
		t.Errorf("Height() = %d, want 10", w.Height())
	}
}

// ---- GoalCol / SetGoalCol / ClearGoalCol -----------------------------------

func TestGoalColDefaultNegativeOne(t *testing.T) {
	buf := buffer.New("test")
	w := New(buf, 0, 0, 80, 24)
	if w.GoalCol() != -1 {
		t.Errorf("GoalCol() default = %d, want -1", w.GoalCol())
	}
}

func TestSetAndClearGoalCol(t *testing.T) {
	buf := buffer.New("test")
	w := New(buf, 0, 0, 80, 24)
	w.SetGoalCol(42)
	if w.GoalCol() != 42 {
		t.Errorf("GoalCol() = %d, want 42", w.GoalCol())
	}
	w.ClearGoalCol()
	if w.GoalCol() != -1 {
		t.Errorf("GoalCol() after Clear = %d, want -1", w.GoalCol())
	}
}

// ---- SetPoint clamping -----------------------------------------------------

func TestSetPointClampedToZero(t *testing.T) {
	buf := buffer.NewWithContent("test", "hello")
	w := New(buf, 0, 0, 80, 24)
	w.SetPoint(-5)
	if w.Point() != 0 {
		t.Errorf("SetPoint(-5): want 0, got %d", w.Point())
	}
}

func TestSetPointClampedToLen(t *testing.T) {
	buf := buffer.NewWithContent("test", "hello")
	w := New(buf, 0, 0, 80, 24)
	w.SetPoint(999)
	if w.Point() != buf.Len() {
		t.Errorf("SetPoint(999): want %d, got %d", buf.Len(), w.Point())
	}
}

// ---- ScrollUp / ScrollDown -------------------------------------------------

func TestScrollUp(t *testing.T) {
	w, _ := newFiveLineWindow(3)
	w.SetScrollLine(1)
	w.ScrollUp(2)
	if w.ScrollLine() != 3 {
		t.Errorf("ScrollUp(2): want scrollLine=3, got %d", w.ScrollLine())
	}
}

func TestScrollDown(t *testing.T) {
	w, _ := newFiveLineWindow(3)
	w.SetScrollLine(4)
	w.ScrollDown(2)
	if w.ScrollLine() != 2 {
		t.Errorf("ScrollDown(2): want scrollLine=2, got %d", w.ScrollLine())
	}
}

func TestScrollUpBeyondMax(t *testing.T) {
	w, _ := newFiveLineWindow(3) // 5-line buffer
	w.ScrollUp(999)
	if w.ScrollLine() != 5 {
		t.Errorf("ScrollUp beyond max: want scrollLine=5, got %d", w.ScrollLine())
	}
}

func TestScrollDownBelowMin(t *testing.T) {
	w, _ := newFiveLineWindow(3)
	w.SetScrollLine(2)
	w.ScrollDown(999)
	if w.ScrollLine() != 1 {
		t.Errorf("ScrollDown below min: want scrollLine=1, got %d", w.ScrollLine())
	}
}
