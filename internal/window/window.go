package window

import "github.com/skybert/gomacs/internal/buffer"

// Window represents a rectangular view into a buffer.
//
// Multiple windows can display the same buffer (e.g., split view).
// Each window has its own point and scroll offset independent of the buffer's
// own point.
type Window struct {
	buf *buffer.Buffer
	// Screen region
	top    int // first row on screen (0-based)
	left   int // first column on screen (0-based)
	width  int
	height int
	// Scroll position: first visible line (1-based)
	scrollLine int
	// Per-window cursor (mirrors buf.Point for the active window;
	// independent when the window is not active)
	point int
	// goalCol is the target column for vertical motion (-1 = unset).
	goalCol int
	// Window-local overlay for isearch highlights etc.
	isMinibuffer bool
}

// ViewLine describes the content of one rendered row.
type ViewLine struct {
	Row      int    // screen row
	Line     int    // 1-based buffer line number (0 if past end of buffer)
	StartPos int    // logical position in buffer of first rune on this row
	EndPos   int    // logical position just past last rune (before newline)
	Text     string // rune string of the line content
}

// New creates a window displaying buf occupying the given screen region.
// scrollLine starts at 1 and point mirrors buf.Point().
func New(buf *buffer.Buffer, top, left, width, height int) *Window {
	return &Window{
		buf:        buf,
		top:        top,
		left:       left,
		width:      width,
		height:     height,
		scrollLine: 1,
		point:      buf.Point(),
		goalCol:    -1,
	}
}

// NewMinibuffer creates a single-row window flagged as the minibuffer.
func NewMinibuffer(buf *buffer.Buffer, top, left, width int) *Window {
	w := New(buf, top, left, width, 1)
	w.isMinibuffer = true
	return w
}

// Buf returns the buffer this window is displaying.
func (w *Window) Buf() *buffer.Buffer { return w.buf }

// SetBuf replaces the buffer being displayed.
// point and scrollLine are reset to safe defaults.
func (w *Window) SetBuf(b *buffer.Buffer) {
	w.buf = b
	w.point = b.Point()
	w.scrollLine = 1
}

// Top returns the first screen row (0-based).
func (w *Window) Top() int { return w.top }

// Left returns the first screen column (0-based).
func (w *Window) Left() int { return w.left }

// Width returns the window width in columns.
func (w *Window) Width() int { return w.width }

// Height returns the window height in rows.
func (w *Window) Height() int { return w.height }

// SetRegion updates the screen region for this window.
func (w *Window) SetRegion(top, left, width, height int) {
	w.top = top
	w.left = left
	w.width = width
	w.height = height
}

// IsMinibuffer reports whether this window is the minibuffer.
func (w *Window) IsMinibuffer() bool { return w.isMinibuffer }

// GoalCol returns the goal column used for vertical motion (-1 = unset).
func (w *Window) GoalCol() int { return w.goalCol }

// SetGoalCol sets the goal column.
func (w *Window) SetGoalCol(col int) { w.goalCol = col }

// ClearGoalCol resets the goal column to -1 (unset).
func (w *Window) ClearGoalCol() { w.goalCol = -1 }

// Point returns the window-local cursor position.
func (w *Window) Point() int { return w.point }

// SetPoint sets the window-local cursor position, clamped to [0, buf.Len()].
func (w *Window) SetPoint(p int) {
	n := w.buf.Len()
	if p < 0 {
		p = 0
	}
	if p > n {
		p = n
	}
	w.point = p
}

// ScrollLine returns the 1-based index of the first visible buffer line.
func (w *Window) ScrollLine() int { return w.scrollLine }

// SetScrollLine sets the first visible buffer line, clamped to
// [1, buf.LineCount()].
func (w *Window) SetScrollLine(l int) {
	max := w.buf.LineCount()
	if max < 1 {
		max = 1
	}
	if l < 1 {
		l = 1
	}
	if l > max {
		l = max
	}
	w.scrollLine = l
}

// ScrollUp scrolls the view down by n lines (scrollLine increases), revealing
// later content.
func (w *Window) ScrollUp(n int) {
	w.SetScrollLine(w.scrollLine + n)
}

// ScrollDown scrolls the view up by n lines (scrollLine decreases), revealing
// earlier content.
func (w *Window) ScrollDown(n int) {
	w.SetScrollLine(w.scrollLine - n)
}

// EnsurePointVisible adjusts scrollLine so that the line containing Point is
// within the visible area [scrollLine, scrollLine+height).
func (w *Window) EnsurePointVisible() {
	pointLine, _ := w.buf.LineCol(w.point)
	start, end := w.VisibleLines()
	if pointLine < start {
		w.SetScrollLine(pointLine)
	} else if pointLine >= end {
		w.SetScrollLine(pointLine - w.height + 1)
	}
}

// VisibleLines returns the 1-based range [start, end) of lines currently
// visible: start == scrollLine, end == scrollLine + height.
func (w *Window) VisibleLines() (start, end int) {
	return w.scrollLine, w.scrollLine + w.height
}

// Recenter adjusts scrollLine so that the line containing Point is roughly in
// the middle of the window.
func (w *Window) Recenter() {
	pointLine, _ := w.buf.LineCol(w.point)
	w.SetScrollLine(pointLine - w.height/2)
}

// RecenterTop scrolls so the line containing Point is at the top of the window.
func (w *Window) RecenterTop() {
	pointLine, _ := w.buf.LineCol(w.point)
	w.SetScrollLine(pointLine)
}

// RecenterBottom scrolls so the line containing Point is near the bottom of
// the window (leaving one line of context above the modeline).
func (w *Window) RecenterBottom() {
	pointLine, _ := w.buf.LineCol(w.point)
	w.SetScrollLine(pointLine - w.height + 2)
}

// ViewLines computes the ViewLine descriptor for every row in the window.
// Rows that fall beyond the last buffer line have Line == 0 and empty Text.
func (w *Window) ViewLines() []ViewLine {
	totalLines := w.buf.LineCount()
	rows := make([]ViewLine, w.height)
	for i := range w.height {
		bufLine := w.scrollLine + i
		row := w.top + i
		if bufLine > totalLines {
			rows[i] = ViewLine{Row: row, Line: 0}
			continue
		}
		startPos := w.buf.LineStart(bufLine)
		endPos := w.buf.EndOfLine(startPos)
		text := w.buf.Substring(startPos, endPos)
		rows[i] = ViewLine{
			Row:      row,
			Line:     bufLine,
			StartPos: startPos,
			EndPos:   endPos,
			Text:     text,
		}
	}
	return rows
}
