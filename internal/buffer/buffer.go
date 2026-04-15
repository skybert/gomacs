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

	// LineCol cache: avoids re-scanning the buffer every frame for the modeline.
	lcacheValid bool
	lcachePos   int
	lcacheLine  int
	lcacheCol   int
	lcacheGen   int

	// LineCount cache: avoids O(n) scan every frame from ViewLines.
	lcountValid bool
	lcountGen   int
	lcountValue int
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
	case "go", "markdown", "elisp", "python", "java", "bash", "json", "yaml", "makefile", "conf", "text", "diff", "dired", "vc-log", "vc-status", "vc-grep", "vc-commit", "vc-show", "vc-fixup-select", "buffer-list", "help", "compilation", "man", "lsp-refs", "shell", modeFundamental:
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

// ReplaceString replaces count runes at pos with s.  It performs both the
// deletion and insertion with a single gap movement (faster than Delete +
// InsertString) and records them as one combined undo record.
func (b *Buffer) ReplaceString(pos, count int, s string) {
	length := b.Len()
	if pos < 0 {
		pos = 0
	}
	if pos >= length || count <= 0 {
		return
	}
	if pos+count > length {
		count = length - pos
	}
	deleted := b.Substring(pos, pos+count)
	b.deleteRunes(pos, count)
	b.insertRunes(pos, []rune(s))
	b.undo.Push(UndoRecord{Pos: pos, Deleted: deleted, Inserted: s})
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
// It uses bulk copies of the gap-buffer segments instead of per-rune RuneAt
// calls, which is significantly faster for large ranges.
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

	// Entire range is before the gap: direct slice.
	if end <= b.gapStart {
		return string(b.data[start:end])
	}
	// Entire range is after the gap: offset by gap size.
	gapSize := b.gapEnd - b.gapStart
	if start >= b.gapStart {
		return string(b.data[start+gapSize : end+gapSize])
	}
	// Range spans the gap: two bulk copies.
	preLen := b.gapStart - start
	postLen := end - b.gapStart
	result := make([]rune, preLen+postLen)
	copy(result, b.data[start:b.gapStart])
	copy(result[preLen:], b.data[b.gapEnd:b.gapEnd+postLen])
	return string(result)
}

// String returns the entire buffer content as a string.
// It copies the two gap-buffer segments directly without per-rune overhead.
func (b *Buffer) String() string {
	pre := b.data[:b.gapStart]
	post := b.data[b.gapEnd:]
	if len(post) == 0 {
		return string(pre)
	}
	if len(pre) == 0 {
		return string(post)
	}
	result := make([]rune, len(pre)+len(post))
	n := copy(result, pre)
	copy(result[n:], post)
	return string(result)
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
// The result is cached by changeGen so repeated calls within the same frame
// are O(1) instead of O(n).
func (b *Buffer) LineCount() int {
	if b.lcountValid && b.lcountGen == b.changeGen {
		return b.lcountValue
	}
	n := 1
	length := b.Len()
	for i := range length {
		if b.RuneAt(i) == '\n' {
			n++
		}
	}
	b.lcountValid = true
	b.lcountGen = b.changeGen
	b.lcountValue = n
	return n
}

// LineCol returns the 1-based line number and 0-based column for pos.
// The result is cached by (changeGen, pos) so repeated calls with the same
// cursor position cost O(1) instead of O(pos).
func (b *Buffer) LineCol(pos int) (line, col int) {
	if pos > b.Len() {
		pos = b.Len()
	}
	if b.lcacheValid && b.lcacheGen == b.changeGen && b.lcachePos == pos {
		return b.lcacheLine, b.lcacheCol
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
	b.lcacheValid = true
	b.lcachePos = pos
	b.lcacheLine = line
	b.lcacheCol = col
	b.lcacheGen = b.changeGen
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

// LineStartsFrom returns the buffer start positions for `count` consecutive
// lines beginning at 1-based line `from`. It does a single forward scan so
// the cost is O(pos_of_last_line) rather than O(count × pos_of_first_line).
// Positions beyond the last buffer line are set to b.Len().
func (b *Buffer) LineStartsFrom(from, count int) []int {
	if count <= 0 {
		return nil
	}
	if from < 1 {
		from = 1
	}
	out := make([]int, count)
	length := b.Len()
	cur := 1    // 1-based line counter
	filled := 0 // how many entries in out[] have been filled

	if from == 1 {
		out[0] = 0
		filled = 1
	}

	for i := range length {
		if filled >= count {
			break
		}
		if b.RuneAt(i) == '\n' {
			cur++
			if cur >= from && cur < from+count {
				out[cur-from] = i + 1
				filled++
			}
		}
	}
	// Fill any remaining entries (lines beyond EOF) with Len().
	for filled < count {
		out[filled] = length
		filled++
	}
	return out
}

// LineStartsFromPos is like LineStartsFrom but begins the forward scan at
// bufPos (the known buffer position of 1-based line `from`) instead of 0.
// This makes repeated ViewLines calls O(visible_lines) rather than
// O(scrollLine) when the caller caches the first-visible-line position.
func (b *Buffer) LineStartsFromPos(from int, bufPos int, count int) []int {
	if count <= 0 {
		return nil
	}
	if from < 1 {
		bufPos = 0
	}
	out := make([]int, count)
	out[0] = bufPos
	length := b.Len()
	filled := 1
	for i := bufPos; i < length && filled < count; i++ {
		if b.RuneAt(i) == '\n' {
			out[filled] = i + 1
			filled++
		}
	}
	for filled < count {
		out[filled] = length
		filled++
	}
	return out
}

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
