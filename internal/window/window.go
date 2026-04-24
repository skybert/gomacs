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
	// wrapCol, when > 0, enables visual line wrapping: buffer lines longer
	// than wrapCol runes are split into multiple view rows.  The file
	// content is never changed.  0 means no wrapping.
	wrapCol int
	// Cached first-visible-line buffer position for fast scrolling.
	// Valid when cachedScrollLine == scrollLine and cachedChangeGen == buf.ChangeGen().
	cachedScrollLine int
	cachedScrollPos  int
	cachedChangeGen  int

	// gutterWidth is the number of columns reserved at the left edge for gutter
	// decorations such as breakpoint indicators (●) and execution position (→).
	// 0 means no gutter; the minimum meaningful value is 2.
	gutterWidth int
}

// ViewLine describes the content of one rendered row.
type ViewLine struct {
	Row      int // screen row
	Line     int // 1-based buffer line number (0 if past end of buffer)
	StartPos int // logical position in buffer of first rune on this row
	EndPos   int // logical position just past last rune (before newline)
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

// firstScrollPos returns the buffer position of the first visible line.
// It uses a per-window cache so repeated calls with the same scrollLine and
// buffer content cost O(1). The cache is keyed by (scrollLine, changeGen);
// on a miss it computes the position with a forward scan.
func (w *Window) firstScrollPos() int {
	gen := w.buf.ChangeGen()
	if w.cachedScrollLine == w.scrollLine && w.cachedChangeGen == gen {
		return w.cachedScrollPos
	}
	pos := w.buf.LineStart(w.scrollLine)
	w.cachedScrollLine = w.scrollLine
	w.cachedScrollPos = pos
	w.cachedChangeGen = gen
	return pos
}

// WrapCol returns the visual wrap column (0 = disabled).
func (w *Window) WrapCol() int { return w.wrapCol }

// SetWrapCol sets the visual wrap column.  0 disables wrapping.
func (w *Window) SetWrapCol(col int) { w.wrapCol = col }

// GutterWidth returns the number of columns reserved at the left for gutter
// decorations (0 = no gutter).
func (w *Window) GutterWidth() int { return w.gutterWidth }

// SetGutterWidth sets the number of gutter columns.  Use 0 to remove the gutter.
func (w *Window) SetGutterWidth(n int) { w.gutterWidth = n }

// ScrollUp scrolls the view down by n lines (scrollLine increases), revealing
// later content.
func (w *Window) ScrollUp(n int) {
	if n <= 0 {
		return
	}
	oldLine := w.scrollLine
	w.SetScrollLine(w.scrollLine + n)
	newLine := w.scrollLine
	delta := newLine - oldLine
	if delta <= 0 {
		return
	}
	// Update cached scroll position incrementally: scan forward by `delta`
	// newlines from the old position so the next ViewLines call is O(visible).
	gen := w.buf.ChangeGen()
	if w.cachedScrollLine == oldLine && w.cachedChangeGen == gen {
		pos := w.cachedScrollPos
		length := w.buf.Len()
		found := 0
		for i := pos; i < length && found < delta; i++ {
			if w.buf.RuneAt(i) == '\n' {
				found++
				pos = i + 1
			}
		}
		w.cachedScrollLine = newLine
		w.cachedScrollPos = pos
		// cachedChangeGen stays the same
	}
}

// ScrollDown scrolls the view up by n lines (scrollLine decreases), revealing
// earlier content.
func (w *Window) ScrollDown(n int) {
	w.SetScrollLine(w.scrollLine - n)
}

// textRows returns the number of rows available for buffer text content.
// For non-minibuffer windows the last row is reserved for the modeline, so
// textRows() == height-1 (minimum 1).  For the minibuffer window the full
// height is usable.
func (w *Window) textRows() int {
	if w.isMinibuffer || w.height <= 1 {
		return w.height
	}
	return w.height - 1
}

// visualRowsForLine returns how many visual rows buffer line bufLine occupies
// given the current wrapCol.  Returns 1 when wrapCol <= 0.
func (w *Window) visualRowsForLine(bufLine int) int {
	if w.wrapCol <= 0 {
		return 1
	}
	startPos := w.buf.LineStart(bufLine)
	endPos := w.buf.EndOfLine(startPos)
	lineLen := endPos - startPos // rune count (positions are rune indices)
	if lineLen == 0 {
		return 1
	}
	rows := lineLen / w.wrapCol
	if lineLen%w.wrapCol != 0 {
		rows++
	}
	return rows
}

// VisualRowForPoint returns the 0-based visual row (measured from the window
// top) of the current window point.  When wrapCol is 0 this equals
// pointLine − scrollLine.
func (w *Window) VisualRowForPoint() int {
	pointLine, _ := w.buf.LineCol(w.point)
	if w.wrapCol <= 0 {
		return pointLine - w.scrollLine
	}
	// Count visual rows from scrollLine up to (but not including) pointLine.
	visualRow := 0
	for bufLine := w.scrollLine; bufLine < pointLine; bufLine++ {
		visualRow += w.visualRowsForLine(bufLine)
	}
	// Add the visual segment offset within the cursor's own line.
	_, cursorCol := w.buf.LineCol(w.point)
	visualRow += cursorCol / w.wrapCol
	return visualRow
}

// EnsurePointVisible adjusts scrollLine so that the line containing Point is
// within the visible text area [scrollLine, scrollLine+textRows()).
// For non-minibuffer windows the last row is the modeline and is excluded
// from the text area, which fixes the "last line invisible" bug.
// When visual wrapping is enabled the check is performed in visual rows.
func (w *Window) EnsurePointVisible() {
	pointLine, _ := w.buf.LineCol(w.point)
	textH := w.textRows()

	if pointLine < w.scrollLine {
		w.SetScrollLine(pointLine)
		return
	}

	if w.wrapCol <= 0 {
		// No visual wrapping: simple line-count check.
		if pointLine >= w.scrollLine+textH {
			w.SetScrollLine(pointLine - textH + 1)
		}
		return
	}

	// Visual wrapping: count visual rows from scrollLine to the cursor.
	visualRow := w.VisualRowForPoint()
	if visualRow >= textH {
		// Cursor is past the bottom of the text area.  Set scrollLine to
		// pointLine so the cursor appears at the top of the window.
		// This is a safe, simple approach; future work could scroll minimally.
		w.SetScrollLine(pointLine)
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
// When wrapCol > 0, buffer lines longer than wrapCol runes are split across
// multiple consecutive ViewLine entries (each with the same Line number).
func (w *Window) ViewLines() []ViewLine {
	if w.wrapCol > 0 {
		return w.viewLinesWrapped()
	}
	return w.viewLinesNoWrap()
}

// viewLinesNoWrap is the original ViewLines implementation.
func (w *Window) viewLinesNoWrap() []ViewLine {
	totalLines := w.buf.LineCount()
	rows := make([]ViewLine, w.height)
	// Use the cached first-visible-line position for O(visible_lines) scan
	// instead of scanning from buffer start each frame.
	firstPos := w.firstScrollPos()
	startPositions := w.buf.LineStartsFromPos(w.scrollLine, firstPos, w.height)
	for i := range w.height {
		bufLine := w.scrollLine + i
		row := w.top + i
		if bufLine > totalLines {
			rows[i] = ViewLine{Row: row, Line: 0}
			continue
		}
		startPos := startPositions[i]
		endPos := w.buf.EndOfLine(startPos)
		rows[i] = ViewLine{
			Row:      row,
			Line:     bufLine,
			StartPos: startPos,
			EndPos:   endPos,
		}
	}
	return rows
}

// viewLinesWrapped produces ViewLine entries with long lines split into
// segments of at most wrapCol runes each.
func (w *Window) viewLinesWrapped() []ViewLine {
	totalLines := w.buf.LineCount()
	rows := make([]ViewLine, w.height)
	rowIdx := 0
	bufLine := w.scrollLine
	// Use the cached first-visible-line position for O(visible_lines) scan.
	firstPos := w.firstScrollPos()
	startPositions := w.buf.LineStartsFromPos(w.scrollLine, firstPos, w.height)
	spIdx := 0
	for rowIdx < w.height && bufLine <= totalLines {
		var startPos int
		if spIdx < len(startPositions) {
			startPos = startPositions[spIdx]
			spIdx++
		} else {
			startPos = w.buf.Len()
		}
		endPos := w.buf.EndOfLine(startPos)
		// Buffer positions are rune indices so the rune count is just the
		// difference — no string/rune-slice allocation needed.
		lineLen := endPos - startPos

		if lineLen <= w.wrapCol {
			rows[rowIdx] = ViewLine{
				Row:      w.top + rowIdx,
				Line:     bufLine,
				StartPos: startPos,
				EndPos:   endPos,
			}
			rowIdx++
		} else {
			segStart := startPos
			for rowIdx < w.height && segStart < endPos {
				segEnd := min(segStart+w.wrapCol, endPos)
				rows[rowIdx] = ViewLine{
					Row:      w.top + rowIdx,
					Line:     bufLine,
					StartPos: segStart,
					EndPos:   segEnd,
				}
				rowIdx++
				segStart = segEnd
			}
		}
		bufLine++
	}
	for ; rowIdx < w.height; rowIdx++ {
		rows[rowIdx] = ViewLine{Row: w.top + rowIdx, Line: 0}
	}
	return rows
}
