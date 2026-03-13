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
		if vl[i].Text != tc.text {
			t.Errorf("row %d: expected Text=%q, got %q", i, tc.text, vl[i].Text)
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
		if vl[i].Text != "" {
			t.Errorf("row %d beyond buffer: expected empty Text, got %q", i, vl[i].Text)
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

// TestRecenterTopPutsPointAtFirstRow verifies that RecenterTop makes the
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
