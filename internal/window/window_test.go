package window

import (
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
