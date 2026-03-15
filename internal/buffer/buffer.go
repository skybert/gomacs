package buffer

const (
	initialGapSize  = 64
	modeFundamental = "fundamental"
)

// Buffer is a gap-buffer backed text buffer.
//
// The underlying slice is organised as:
//
//	data[0 .. gapStart)          – text before the gap
//	data[gapStart .. gapEnd)     – the gap (unused space)
//	data[gapEnd .. len(data))    – text after the gap
//
// Logical position p maps to:
//
//	p < gapStart  →  data[p]
//	p >= gapStart →  data[p + (gapEnd-gapStart)]
type Buffer struct {
	name       string
	data       []rune
	gapStart   int
	gapEnd     int
	point      int // cursor position (logical index)
	mark       int // mark position (-1 if not set)
	markActive bool
	modified   bool
	modCount   int // incremented on every mutation; used by LSP change detection
	readOnly   bool
	filename   string // associated file path, empty if none
	undo       *UndoRing
	mode       string // modeFundamental, "go", "markdown", etc.

	// changeGen tracks the net number of edits relative to the saved state.
	// It increments on Insert/Delete and decrements on ApplyUndo, increments
	// on ApplyRedo.  Modified() compares it to savedChangeGen.
	changeGen      int
	savedChangeGen int

	// Mark ring: previous mark positions for C-u C-SPC to cycle through.
	markRing []int

	// Narrowing: restricts the accessible portion of the buffer.
	narrowed  bool
	narrowMin int
	narrowMax int // end of accessible region (exclusive)
}

// New creates an empty buffer with the given name.
func New(name string) *Buffer {
	data := make([]rune, initialGapSize)
	return &Buffer{
		name:     name,
		data:     data,
		gapStart: 0,
		gapEnd:   initialGapSize,
		mark:     -1,
		undo:     NewUndoRing(256),
		mode:     modeFundamental,
	}
}

// NewWithContent creates a buffer pre-populated with content.
func NewWithContent(name, content string) *Buffer {
	b := New(name)
	if len(content) > 0 {
		b.InsertString(0, content)
		b.modified = false // initial load is not a modification
		b.changeGen = 0
		b.savedChangeGen = 0
		b.undo.Reset()
	}
	return b
}

// ---- metadata accessors ----------------------------------------------------

func (b *Buffer) Name() string         { return b.name }
func (b *Buffer) SetName(name string)  { b.name = name }
func (b *Buffer) Filename() string     { return b.filename }
func (b *Buffer) SetFilename(f string) { b.filename = f }
func (b *Buffer) Mode() string         { return b.mode }
func (b *Buffer) ChangeGen() int       { return b.changeGen }
func (b *Buffer) Modified() bool {
	return b.changeGen != b.savedChangeGen
}
func (b *Buffer) SetModified(v bool) {
	if !v {
		// Mark the current generation as clean.
		b.savedChangeGen = b.changeGen
	} else {
		// Force dirty by making changeGen diverge from savedChangeGen.
		if b.changeGen == b.savedChangeGen {
			b.changeGen++
		}
	}
	b.modified = v
}
func (b *Buffer) ModCount() int      { return b.modCount }
func (b *Buffer) ReadOnly() bool     { return b.readOnly }
func (b *Buffer) SetReadOnly(v bool) { b.readOnly = v }

// SetMode sets the buffer's major mode.
func (b *Buffer) SetMode(mode string) {
	switch mode {
	case "go", "markdown", "elisp", "python", "java", "bash", "json", "makefile", "diff", "dired", "vc-log", "vc-status", "vc-grep", "buffer-list", modeFundamental:
		b.mode = mode
	default:
		b.mode = modeFundamental
	}
}

// ---- gap-buffer internals --------------------------------------------------

// gapSize returns the current gap size.
func (b *Buffer) gapSize() int { return b.gapEnd - b.gapStart }

// Len returns the total number of runes in the buffer (excluding the gap).
func (b *Buffer) Len() int { return len(b.data) - b.gapSize() }

// rawIndex converts a logical position to a physical index in b.data.
func (b *Buffer) rawIndex(pos int) int {
	if pos < b.gapStart {
		return pos
	}
	return pos + b.gapSize()
}

// moveGap repositions the gap so that gapStart == pos.
// The gap size is preserved.
func (b *Buffer) moveGap(pos int) {
	if pos == b.gapStart {
		return
	}
	gs := b.gapSize()
	if pos < b.gapStart {
		// Move gap left: shift data[pos..gapStart) rightward by gs.
		copy(b.data[pos+gs:b.gapEnd], b.data[pos:b.gapStart])
		b.gapStart = pos
		b.gapEnd = pos + gs
	} else {
		// Move gap right: shift data[gapEnd..gapEnd+(pos-gapStart)) leftward by gs.
		n := pos - b.gapStart
		copy(b.data[b.gapStart:b.gapStart+n], b.data[b.gapEnd:b.gapEnd+n])
		b.gapStart = pos
		b.gapEnd = pos + gs
	}
}

// growGap ensures the gap is at least `needed` runes wide.
func (b *Buffer) growGap(needed int) {
	if b.gapSize() >= needed {
		return
	}
	extra := needed - b.gapSize()
	grow := extra
	if q := len(b.data) / 4; q > grow {
		grow = q
	}
	if grow < initialGapSize {
		grow = initialGapSize
	}
	// Build new slice.
	newData := make([]rune, len(b.data)+grow)
	copy(newData, b.data[:b.gapStart])
	newGapEnd := b.gapEnd + grow
	copy(newData[newGapEnd:], b.data[b.gapEnd:])
	b.data = newData
	b.gapEnd = newGapEnd
}

// ---- rune access -----------------------------------------------------------

// RuneAt returns the rune at logical position pos.
func (b *Buffer) RuneAt(pos int) rune {
	if pos < 0 || pos >= b.Len() {
		return 0
	}
	return b.data[b.rawIndex(pos)]
}

// ---- insertion / deletion --------------------------------------------------

// Insert inserts a single rune at logical position pos, records undo.
func (b *Buffer) Insert(pos int, r rune) {
	b.insertRunes(pos, []rune{r})
	b.undo.Push(UndoRecord{Pos: pos, Inserted: string(r)})
	b.modified = true
	b.modCount++
	b.changeGen++
}

// InsertString inserts a string at logical position pos, records undo.
func (b *Buffer) InsertString(pos int, s string) {
	if s == "" {
		return
	}
	runes := []rune(s)
	b.insertRunes(pos, runes)
	b.undo.Push(UndoRecord{Pos: pos, Inserted: s})
	b.modified = true
	b.modCount++
	b.changeGen++
}

// insertRunes is the raw (no undo) insertion primitive.
func (b *Buffer) insertRunes(pos int, runes []rune) {
	n := len(runes)
	b.growGap(n)
	b.moveGap(pos)
	copy(b.data[b.gapStart:], runes)
	b.gapStart += n
	// Adjust point and mark.
	if b.point >= pos {
		b.point += n
	}
	if b.mark >= pos {
		b.mark += n
	}
}

// Delete removes count runes starting at logical position pos.
// Returns the deleted string and records an undo entry.
func (b *Buffer) Delete(pos, count int) string {
	length := b.Len()
	if pos < 0 {
		pos = 0
	}
	if pos >= length {
		return ""
	}
	if count <= 0 {
		return ""
	}
	if pos+count > length {
		count = length - pos
	}

	deleted := b.Substring(pos, pos+count)
	b.deleteRunes(pos, count)
	b.undo.Push(UndoRecord{Pos: pos, Deleted: deleted})
	b.modified = true
	b.modCount++
	b.changeGen++
	return deleted
}

// deleteRunes is the raw (no undo) deletion primitive.
func (b *Buffer) deleteRunes(pos, count int) {
	b.moveGap(pos)
	b.gapEnd += count
	// Adjust point and mark.
	if b.point > pos+count {
		b.point -= count
	} else if b.point > pos {
		b.point = pos
	}
	if b.mark > pos+count {
		b.mark -= count
	} else if b.mark > pos {
		b.mark = pos
	}
}

// ---- string extraction -----------------------------------------------------

// Substring returns the runes in [start, end) as a string.
func (b *Buffer) Substring(start, end int) string {
	length := b.Len()
	if start < 0 {
		start = 0
	}
	if end > length {
		end = length
	}
	if start >= end {
		return ""
	}
	runes := make([]rune, end-start)
	for i := start; i < end; i++ {
		runes[i-start] = b.RuneAt(i)
	}
	return string(runes)
}

// String returns the entire buffer content as a string.
func (b *Buffer) String() string {
	return b.Substring(0, b.Len())
}

// ---- cursor / mark ---------------------------------------------------------

func (b *Buffer) Point() int { return b.point }

func (b *Buffer) SetPoint(p int) {
	lo := b.NarrowMin()
	hi := b.NarrowMax()
	if p < lo {
		p = lo
	}
	if p > hi {
		p = hi
	}
	b.point = p
}

func (b *Buffer) Mark() int            { return b.mark }
func (b *Buffer) SetMark(m int)        { b.mark = m }
func (b *Buffer) MarkActive() bool     { return b.markActive }
func (b *Buffer) SetMarkActive(v bool) { b.markActive = v }

// ---- mark ring -------------------------------------------------------------

const markRingMax = 16

// PushMarkRing pushes pos onto the mark ring, capping at markRingMax.
func (b *Buffer) PushMarkRing(pos int) {
	b.markRing = append([]int{pos}, b.markRing...)
	if len(b.markRing) > markRingMax {
		b.markRing = b.markRing[:markRingMax]
	}
}

// PopMarkRing removes and returns the most recent mark ring entry.
// Returns -1 if the ring is empty.
func (b *Buffer) PopMarkRing() int {
	if len(b.markRing) == 0 {
		return -1
	}
	pos := b.markRing[0]
	b.markRing = b.markRing[1:]
	return pos
}

// ---- narrowing -------------------------------------------------------------

// Narrow restricts the buffer's accessible region to [min, max).
// Point is clamped into the new accessible region.
func (b *Buffer) Narrow(min, max int) {
	n := b.Len()
	if min < 0 {
		min = 0
	}
	if max > n {
		max = n
	}
	b.narrowed = true
	b.narrowMin = min
	b.narrowMax = max
	b.SetPoint(b.point) // clamp
}

// Widen cancels any narrowing, making the entire buffer accessible.
func (b *Buffer) Widen() {
	b.narrowed = false
	b.narrowMin = 0
	b.narrowMax = 0
}

// Narrowed reports whether the buffer is currently narrowed.
func (b *Buffer) Narrowed() bool { return b.narrowed }

// NarrowMin returns the start of the accessible region (0 when not narrowed).
func (b *Buffer) NarrowMin() int {
	if b.narrowed {
		return b.narrowMin
	}
	return 0
}

// NarrowMax returns the end of the accessible region (Len() when not narrowed).
func (b *Buffer) NarrowMax() int {
	if b.narrowed {
		return b.narrowMax
	}
	return b.Len()
}

// ---- line / column helpers -------------------------------------------------

// LineCount returns the number of lines (newlines + 1).
func (b *Buffer) LineCount() int {
	n := 1
	length := b.Len()
	for i := range length {
		if b.RuneAt(i) == '\n' {
			n++
		}
	}
	return n
}

// LineCol returns the 1-based line number and 0-based column for pos.
func (b *Buffer) LineCol(pos int) (line, col int) {
	if pos > b.Len() {
		pos = b.Len()
	}
	line = 1
	col = 0
	for i := range pos {
		if b.RuneAt(i) == '\n' {
			line++
			col = 0
		} else {
			col++
		}
	}
	return line, col
}

// LineStart returns the logical position of the first rune on the given
// 1-based line number.  Returns 0 for line <= 1 and Len() for lines beyond
// the last line.
func (b *Buffer) LineStart(line int) int {
	if line <= 1 {
		return 0
	}
	current := 1
	length := b.Len()
	for i := range length {
		if b.RuneAt(i) == '\n' {
			current++
			if current == line {
				return i + 1
			}
		}
	}
	return b.Len()
}

// PosForLineCol returns the logical position for the given 1-based line and
// 0-based column.  The column is clamped to the line length.
func (b *Buffer) PosForLineCol(line, col int) int {
	pos := b.LineStart(line)
	end := b.EndOfLine(pos)
	if pos+col < end {
		return pos + col
	}
	return end
}

// BeginningOfLine returns the logical position of the first rune on the line
// that contains pos.
func (b *Buffer) BeginningOfLine(pos int) int {
	if pos > b.Len() {
		pos = b.Len()
	}
	for i := pos - 1; i >= 0; i-- {
		if b.RuneAt(i) == '\n' {
			return i + 1
		}
	}
	return 0
}

// EndOfLine returns the logical position just before the newline that ends the
// line containing pos (or Len() if on the last line).
func (b *Buffer) EndOfLine(pos int) int {
	n := b.Len()
	if pos > n {
		pos = n
	}
	for i := pos; i < n; i++ {
		if b.RuneAt(i) == '\n' {
			return i
		}
	}
	return n
}
