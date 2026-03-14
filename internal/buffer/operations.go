package buffer

// ApplyUndo pops the most recent UndoRecord from the undo ring and applies
// its inverse operation to the buffer.  Returns false if there is nothing to
// undo.
func (b *Buffer) ApplyUndo() bool {
	rec, ok := b.undo.Undo()
	if !ok {
		return false
	}
	if rec.Inserted != "" {
		// Undo the insertion: delete the text that was inserted.
		b.deleteRunes(rec.Pos, len([]rune(rec.Inserted)))
	}
	if rec.Deleted != "" {
		// Undo the deletion: re-insert the text that was removed.
		b.insertRunes(rec.Pos, []rune(rec.Deleted))
	}
	b.SetPoint(rec.Pos)
	b.changeGen--
	return true
}

// ApplyRedo re-applies the most recently undone operation.
// Returns false if there is nothing to redo.
func (b *Buffer) ApplyRedo() bool {
	rec, ok := b.undo.Redo()
	if !ok {
		return false
	}
	// Redo is the inverse of undo: re-apply the original operation.
	if rec.Inserted != "" {
		// The original operation was an insertion; re-insert.
		b.insertRunes(rec.Pos, []rune(rec.Inserted))
		b.SetPoint(rec.Pos + len([]rune(rec.Inserted)))
	}
	if rec.Deleted != "" {
		// The original operation was a deletion; re-delete.
		b.deleteRunes(rec.Pos, len([]rune(rec.Deleted)))
		b.SetPoint(rec.Pos)
	}
	b.changeGen++
	return true
}
