package buffer

// UndoRecord represents one atomic undo step.
type UndoRecord struct {
	Pos      int    // position in buffer where change happened
	Inserted string // text that was inserted (to undo: delete it)
	Deleted  string // text that was deleted (to undo: reinsert it)
}

// UndoRing is a ring buffer of undo records.
type UndoRing struct {
	records []UndoRecord
	head    int // next write position
	size    int // number of valid records
	cap     int // total capacity
	undoIdx int // current position for undo stepping (-1 = at head)
}

// NewUndoRing creates a new UndoRing with the given capacity.
func NewUndoRing(cap int) *UndoRing {
	if cap <= 0 {
		cap = 64
	}
	return &UndoRing{
		records: make([]UndoRecord, cap),
		cap:     cap,
		undoIdx: -1,
	}
}

// Push adds a new undo record and resets the undo index to -1 (head).
// If the ring is full the oldest record is overwritten.
func (u *UndoRing) Push(r UndoRecord) {
	u.records[u.head] = r
	u.head = (u.head + 1) % u.cap
	if u.size < u.cap {
		u.size++
	}
	u.undoIdx = -1
}

// Undo returns the next record to undo and advances the undo index.
// Returns (UndoRecord{}, false) if there is nothing left to undo.
//
// Stepping model:
//   - undoIdx == -1  → cursor is at the head (nothing undone yet, or after a Push)
//   - first Undo     → moves cursor to head-1 (most recent record)
//   - subsequent     → moves cursor one step older each time
//   - exhausted when cursor would move past the oldest record
func (u *UndoRing) Undo() (UndoRecord, bool) {
	if u.size == 0 {
		return UndoRecord{}, false
	}

	// oldest is the physical index of the oldest stored record.
	oldest := (u.head - u.size + u.cap) % u.cap

	if u.undoIdx == -1 {
		// First undo: step to the most-recently-pushed slot (head-1).
		u.undoIdx = (u.head - 1 + u.cap) % u.cap
		return u.records[u.undoIdx], true
	}

	// Already undoing: check whether we can step one further back.
	if u.undoIdx == oldest {
		// Already at the oldest record; nothing more to undo.
		return UndoRecord{}, false
	}

	u.undoIdx = (u.undoIdx - 1 + u.cap) % u.cap
	return u.records[u.undoIdx], true
}

// Redo returns the next record to redo (move forward after undo).
// Returns (UndoRecord{}, false) if there is nothing to redo.
func (u *UndoRing) Redo() (UndoRecord, bool) {
	// If undoIdx is -1 we are already at the head; nothing to redo.
	if u.undoIdx == -1 {
		return UndoRecord{}, false
	}

	prev := u.undoIdx
	u.undoIdx = (u.undoIdx + 1) % u.cap
	if u.undoIdx == u.head {
		// Back at the head; reset sentinel.
		u.undoIdx = -1
	}

	return u.records[prev], true
}

// Reset clears all undo records.
func (u *UndoRing) Reset() {
	u.head = 0
	u.size = 0
	u.undoIdx = -1
}
