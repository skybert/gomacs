package buffer

import (
	"sort"
	"strings"
)

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

	// Incremental line count: maintained by insertRunes/deleteRunes so that
	// LineCount() is O(1) after the first call instead of O(buffer_size).
	lineCountDelta int  // number of '\n' runes currently in the buffer
	lineCountReady bool // true after first LineCount() call seeds the delta

	// Line-start index: lineStarts[i] is the buffer position of the first rune
	// on 1-based line i+1 (so lineStarts[0] is always 0).  It is built lazily
	// by ensureLineStarts() and patched in place by insertRunes/deleteRunes for
	// every edit, wherever it lands, so a rebuild only ever happens once per
	// buffer.  Like lineCountDelta, this makes LineStart() O(1) after the first
	// call instead of O(buffer_size); LineCol() and PosForLineCol() binary-search
	// it.  The index is absolute: it ignores narrowing, matching LineStart().
	lineStarts      []int
	lineStartsReady bool
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
		// The InsertString above incremented lineCountDelta; reset so the
		// next LineCount() re-seeds from the actual buffer content.
		b.lineCountReady = false
		b.invalidateLineStarts()
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
	case "go", "markdown", "elisp", "python", "java", "bash", "perl", "gherkin", "json", "yaml", "makefile", "conf", "text", "diff", "dired", "vc-log", "vc-status", "vc-grep", "vc-commit", "vc-show", "vc-fixup-select", "vc-annotate", "buffer-list", "help", "compilation", "man", "lsp-refs", "shell", "debug-locals", "debug-stack", "debug-repl", modeFundamental:
		b.mode = mode
	default:
		// Modes carrying a language suffix (e.g. "vc-annotate+go",
		// "debug-repl+java") are accepted verbatim so the renderer can pick the
		// debugged/annotated language's highlighter.
		if strings.HasPrefix(mode, "vc-annotate+") || strings.HasPrefix(mode, "debug-repl+") {
			b.mode = mode
		} else {
			b.mode = modeFundamental
		}
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
	if b.lineCountReady {
		for _, r := range runes {
			if r == '\n' {
				b.lineCountDelta++
			}
		}
	}
	b.insertLineStarts(pos, runes)
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
	if b.lineCountReady {
		for i := pos; i < pos+count; i++ {
			if b.RuneAt(i) == '\n' {
				b.lineCountDelta--
			}
		}
	}
	b.deleteLineStarts(pos, count)
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

// AppendRunes appends the buffer's runes to dst and returns the extended slice.
// The two gap-buffer segments are copied in bulk, so callers that want runes
// avoid the UTF-8 encode/decode round trip of String() followed by []rune(...),
// and can reuse a scratch slice across calls.  Like String(), the whole buffer
// is returned: narrowing is not taken into account.
func (b *Buffer) AppendRunes(dst []rune) []rune {
	pre := b.data[:b.gapStart]
	post := b.data[b.gapEnd:]
	start := len(dst)
	need := start + len(pre) + len(post)
	if cap(dst) < need {
		grown := make([]rune, start, need)
		copy(grown, dst)
		dst = grown
	}
	dst = dst[:need]
	n := copy(dst[start:], pre)
	copy(dst[start+n:], post)
	return dst
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

// ---- line-start index ------------------------------------------------------

// invalidateLineStarts drops the line-start index, keeping the backing array so
// the next rebuild does not have to reallocate.
func (b *Buffer) invalidateLineStarts() {
	b.lineStarts = b.lineStarts[:0]
	b.lineStartsReady = false
}

// ensureLineStarts builds the line-start index if it is not currently valid.
// The scan walks the two contiguous gap-buffer segments in bulk rather than
// calling RuneAt() per rune (see LineStartsFromPos for the same technique).
// Because the index records every '\n', it also seeds the incremental line
// count for free.
func (b *Buffer) ensureLineStarts() {
	if b.lineStartsReady {
		return
	}
	b.lineStarts = append(b.lineStarts[:0], 0)
	logPos := 0
	for _, r := range b.data[:b.gapStart] {
		logPos++
		if r == '\n' {
			b.lineStarts = append(b.lineStarts, logPos)
		}
	}
	for _, r := range b.data[b.gapEnd:] {
		logPos++
		if r == '\n' {
			b.lineStarts = append(b.lineStarts, logPos)
		}
	}
	b.lineStartsReady = true
	// One entry per '\n' plus the always-present line 1 entry.
	b.lineCountDelta = len(b.lineStarts) - 1
	b.lineCountReady = true
}

// insertLineStarts keeps the line-start index in step with an insertion of
// `runes` at pos.  Entries recording a position after pos shift right by the
// number of runes inserted, and one new entry is spliced in for each newline
// inside the inserted text.  The cost is O(log lines + entries after pos),
// which beats dropping the index and paying an O(buffer_size) rebuild on the
// next LineStart().
func (b *Buffer) insertLineStarts(pos int, runes []rune) {
	if !b.lineStartsReady || len(runes) == 0 {
		return
	}
	n := len(runes)
	// First entry lying strictly after the insertion point; everything from
	// here on moves right.  Entry 0 (position 0) is never in this range.
	at := sort.SearchInts(b.lineStarts, pos+1)
	newlines := 0
	for _, r := range runes {
		if r == '\n' {
			newlines++
		}
	}
	if newlines > 0 {
		// Open a gap of `newlines` entries at `at` by extending the slice and
		// sliding the tail up.
		old := len(b.lineStarts)
		for range newlines {
			b.lineStarts = append(b.lineStarts, 0)
		}
		copy(b.lineStarts[at+newlines:], b.lineStarts[at:old])
	}
	for i := at + newlines; i < len(b.lineStarts); i++ {
		b.lineStarts[i] += n
	}
	// A newline at offset i starts a new line at pos+i+1.  These positions all
	// fall in (pos, pos+n], i.e. before every shifted entry, so they land in
	// the gap in ascending order.
	next := at
	for i, r := range runes {
		if r == '\n' {
			b.lineStarts[next] = pos + i + 1
			next++
		}
	}
	b.lineCountDelta = len(b.lineStarts) - 1
}

// deleteLineStarts keeps the line-start index in step with a deletion of count
// runes at pos.  An entry at position v records a newline at v-1, so entries in
// (pos, pos+count] vanish with the removed range; entries beyond it shift left
// by count.  Like insertLineStarts this patches the index in place rather than
// dropping it.
func (b *Buffer) deleteLineStarts(pos, count int) {
	if !b.lineStartsReady || count <= 0 {
		return
	}
	// [first, past) is the half-open range of entries the deletion swallows.
	// Entry 0 (position 0) is never in it.
	first := sort.SearchInts(b.lineStarts, pos+1)
	past := sort.SearchInts(b.lineStarts, pos+count+1)
	for i := past; i < len(b.lineStarts); i++ {
		b.lineStarts[i] -= count
	}
	if past > first {
		b.lineStarts = append(b.lineStarts[:first], b.lineStarts[past:]...)
	}
	b.lineCountDelta = len(b.lineStarts) - 1
}

// ---- line / column helpers -------------------------------------------------

// LineCount returns the number of lines (newlines + 1).
// The result is maintained incrementally by insertRunes/deleteRunes so that
// after the first call (which seeds the delta with an O(n) scan) all
// subsequent calls are O(1) regardless of edits.
func (b *Buffer) LineCount() int {
	if b.lineCountReady {
		return b.lineCountDelta + 1
	}
	// First call: O(n) scan to seed the incremental delta.
	n := 0
	for i := range b.Len() {
		if b.RuneAt(i) == '\n' {
			n++
		}
	}
	b.lineCountDelta = n
	b.lineCountReady = true
	return n + 1
}

// LineCol returns the 1-based line number and 0-based column for pos.
// The position is absolute: narrowing is not taken into account, matching
// LineStart().  pos is clamped into [0, Len()].
//
// The lookup binary-searches the lazily built line-start index, so it is
// O(log lines) rather than an O(pos) scan.  The result is additionally cached by
// (changeGen, pos) so a repeated call with an unchanged cursor costs nothing.
func (b *Buffer) LineCol(pos int) (line, col int) {
	if pos > b.Len() {
		pos = b.Len()
	}
	if pos < 0 {
		pos = 0
	}
	if b.lcacheValid && b.lcacheGen == b.changeGen && b.lcachePos == pos {
		return b.lcacheLine, b.lcacheCol
	}
	b.ensureLineStarts()
	// lineStarts[0] is 0 and pos >= 0, so the search never lands on index 0.
	idx := sort.SearchInts(b.lineStarts, pos+1) - 1
	line = idx + 1
	col = pos - b.lineStarts[idx]
	b.lcacheValid = true
	b.lcachePos = pos
	b.lcacheLine = line
	b.lcacheCol = col
	b.lcacheGen = b.changeGen
	return line, col
}

// LineStart returns the logical position of the first rune on the given
// 1-based line number.  Returns 0 for line <= 1 and Len() for lines beyond
// the last line.  The position is absolute: narrowing is not taken into
// account.
//
// The lookup goes through the lazily built line-start index, so it is O(1)
// after the first call rather than an O(buffer_size) scan every time.
func (b *Buffer) LineStart(line int) int {
	if line <= 1 {
		return 0
	}
	b.ensureLineStarts()
	if line-1 >= len(b.lineStarts) {
		return b.Len()
	}
	return b.lineStarts[line-1]
}

// LineStartsFrom returns the buffer start positions for `count` consecutive
// lines beginning at 1-based line `from`.  Each entry is an index lookup, so
// the cost is O(count) once the line-start index exists.
// Positions beyond the last buffer line are set to b.Len().
func (b *Buffer) LineStartsFrom(from, count int) []int {
	if count <= 0 {
		return nil
	}
	if from < 1 {
		from = 1
	}
	b.ensureLineStarts()
	out := make([]int, count)
	length := b.Len()
	for i := range count {
		if idx := from + i - 1; idx < len(b.lineStarts) {
			out[i] = b.lineStarts[idx]
		} else {
			out[i] = length
		}
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

	// Iterate directly over the two contiguous segments of the gap buffer,
	// avoiding the per-character gap-index arithmetic of RuneAt(i).
	//
	// Physical layout: data[0..gapStart) | gap[gapStart..gapEnd) | data[gapEnd..len(data))
	// Logical mapping: pos < gapStart → data[pos]; pos >= gapStart → data[pos+gapSize]
	gapSize := b.gapEnd - b.gapStart
	logPos := bufPos

	// Segment 1: data before the gap (logical range [bufPos, gapStart)).
	if bufPos < b.gapStart {
		for phys := bufPos; phys < b.gapStart && filled < count; phys++ {
			if b.data[phys] == '\n' {
				out[filled] = logPos + 1
				filled++
			}
			logPos++
		}
	}

	// Segment 2: data after the gap.
	// Physical start: gapEnd (when bufPos < gapStart) or bufPos+gapSize (when bufPos >= gapStart).
	seg2Start := b.gapEnd
	if bufPos >= b.gapStart {
		seg2Start = bufPos + gapSize
	}
	for phys := seg2Start; phys < len(b.data) && filled < count; phys++ {
		if b.data[phys] == '\n' {
			out[filled] = logPos + 1
			filled++
		}
		logPos++
	}

	for filled < count {
		out[filled] = length
		filled++
	}
	return out
}

// PosForLineCol returns the logical position of the given 1-based line and
// 0-based column.  The column is clamped to the line length.  Like LineCol it
// reads the line-start index, so both directions cost O(1) and stay mutually
// consistent.
func (b *Buffer) PosForLineCol(line, col int) int {
	b.ensureLineStarts()
	if line < 1 {
		line = 1
	}
	// Positions past the last line collapse onto the end of the buffer.
	pos := b.Len()
	if line-1 < len(b.lineStarts) {
		pos = b.lineStarts[line-1]
	}
	// The line ends just before the newline that starts the next one, or at the
	// end of the buffer for the last line.
	end := b.Len()
	if line < len(b.lineStarts) {
		end = b.lineStarts[line] - 1
	}
	return min(pos+col, end)
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
