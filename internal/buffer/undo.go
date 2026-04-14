package buffer

// UndoRecord represents one atomic undo step.
type UndoRecord struct {
	Pos      int    // position in buffer where change happened
	Inserted string // text that was inserted (to undo: delete it)
	Deleted  string // text that was deleted (to undo: reinsert it)
}

// UndoRing is an unbounded list of undo records.
// The name is kept for compatibility; it is no longer a ring.
type UndoRing struct {
	records []UndoRecord
	undoIdx int // index of current undo position, or -1 (at head)
}

// NewUndoRing creates a new UndoRing.  The cap argument is ignored and
// kept only for call-site compatibility.
func NewUndoRing(_ int) *UndoRing {
	return &UndoRing{undoIdx: -1}
}

// Push adds a new undo record and resets the undo index to -1 (head).
// If we are mid-undo (undoIdx != -1), the "future" records that were
// undone are discarded so that the new edit starts a fresh branch.
func (u *UndoRing) Push(r UndoRecord) {
	if u.undoIdx != -1 {
		// Discard undone records: undoIdx points to the most-recently-undone
		// record, so everything from undoIdx onward is gone.
		u.records = u.records[:u.undoIdx]
	}
	u.records = append(u.records, r)
	u.undoIdx = -1
}

// Undo returns the next record to undo and advances the undo index.
// Returns (UndoRecord{}, false) if there is nothing left to undo.
func (u *UndoRing) Undo() (UndoRecord, bool) {
	if len(u.records) == 0 {
		return UndoRecord{}, false
	}

	if u.undoIdx == -1 {
		// First undo: step to the most-recently-pushed record.
		u.undoIdx = len(u.records) - 1
		return u.records[u.undoIdx], true
	}

	if u.undoIdx == 0 {
		// Already at the oldest record; nothing more to undo.
		return UndoRecord{}, false
	}

	u.undoIdx--
	return u.records[u.undoIdx], true
}

// Redo returns the next record to redo (move forward after undo).
// Returns (UndoRecord{}, false) if there is nothing to redo.
func (u *UndoRing) Redo() (UndoRecord, bool) {
	if u.undoIdx == -1 {
		return UndoRecord{}, false
	}

	prev := u.undoIdx
	u.undoIdx++
	if u.undoIdx >= len(u.records) {
		u.undoIdx = -1
	}

	return u.records[prev], true
}

// Reset clears all undo records.
func (u *UndoRing) Reset() {
	u.records = nil
	u.undoIdx = -1
}
