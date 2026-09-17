package buffer

import "testing"

func TestReplaceStringUndoRedo(t *testing.T) {
	b := NewWithContent("test", "Hello World")
	b.ReplaceString(0, 5, "hello")

	// Undo should restore "Hello World".
	if !b.ApplyUndo() {
		t.Fatal("ApplyUndo returned false")
	}
	if got := b.String(); got != "Hello World" {
		t.Fatalf("after undo: want %q, got %q", "Hello World", got)
	}

	// Redo should re-apply the downcase.
	if !b.ApplyRedo() {
		t.Fatal("ApplyRedo returned false")
	}
	if got := b.String(); got != "hello World" {
		t.Fatalf("after redo: want %q, got %q", "hello World", got)
	}
}

func TestReplaceStringSingleUndoRecord(t *testing.T) {
	b := NewWithContent("test", "Hello")
	b.ReplaceString(0, 5, "hello")
	// Should take exactly one undo step.
	b.ApplyUndo()
	if got := b.String(); got != "Hello" {
		t.Fatalf("want %q after 1 undo, got %q", "Hello", got)
	}
	if b.ApplyUndo() {
		t.Fatal("second ApplyUndo should return false — only one record expected")
	}
}

// ---- ApplyUndo / ApplyRedo (buffer API) ------------------------------------

func TestApplyUndoInsert(t *testing.T) {
	b := NewWithContent("test", "hello")
	b.InsertString(5, " world")
	if b.String() != "hello world" {
		t.Fatalf("setup: want %q, got %q", "hello world", b.String())
	}
	if !b.ApplyUndo() {
		t.Fatal("ApplyUndo returned false")
	}
	if got := b.String(); got != "hello" {
		t.Fatalf("after undo: want %q, got %q", "hello", got)
	}
}

func TestApplyUndoDelete(t *testing.T) {
	b := NewWithContent("test", "hello world")
	b.Delete(5, 6) // delete " world"
	if b.String() != "hello" {
		t.Fatalf("setup: want %q, got %q", "hello", b.String())
	}
	if !b.ApplyUndo() {
		t.Fatal("ApplyUndo returned false")
	}
	if got := b.String(); got != "hello world" {
		t.Fatalf("after undo: want %q, got %q", "hello world", got)
	}
}

func TestApplyUndoEmptyReturnsFalse(t *testing.T) {
	b := NewWithContent("test", "hi")
	if b.ApplyUndo() {
		t.Fatal("ApplyUndo on fresh buffer should return false")
	}
}

func TestApplyRedoAfterUndo(t *testing.T) {
	b := NewWithContent("test", "")
	b.InsertString(0, "abc")
	b.ApplyUndo()
	if b.String() != "" {
		t.Fatalf("after undo: want empty, got %q", b.String())
	}
	if !b.ApplyRedo() {
		t.Fatal("ApplyRedo returned false")
	}
	if got := b.String(); got != "abc" {
		t.Fatalf("after redo: want %q, got %q", "abc", got)
	}
}

func TestApplyRedoEmptyReturnsFalse(t *testing.T) {
	b := NewWithContent("test", "hi")
	if b.ApplyRedo() {
		t.Fatal("ApplyRedo with nothing to redo should return false")
	}
}

func TestApplyRedoReplaceString(t *testing.T) {
	b := NewWithContent("test", "hello")
	b.ReplaceString(0, 5, "world")
	b.ApplyUndo()
	if got := b.String(); got != "hello" {
		t.Fatalf("after undo of replace: want %q, got %q", "hello", got)
	}
	if !b.ApplyRedo() {
		t.Fatal("ApplyRedo of replace returned false")
	}
	if got := b.String(); got != "world" {
		t.Fatalf("after redo of replace: want %q, got %q", "world", got)
	}
	if b.Point() != 5 {
		t.Errorf("point after redo of replace = %d, want 5", b.Point())
	}
}

func TestApplyRedoDelete(t *testing.T) {
	b := NewWithContent("test", "hello world")
	b.Delete(5, 6)
	b.ApplyUndo()
	if got := b.String(); got != "hello world" {
		t.Fatalf("after undo of delete: want %q, got %q", "hello world", got)
	}
	if !b.ApplyRedo() {
		t.Fatal("ApplyRedo of delete returned false")
	}
	if got := b.String(); got != "hello" {
		t.Fatalf("after redo of delete: want %q, got %q", "hello", got)
	}
}

func TestApplyUndoMultipleSteps(t *testing.T) {
	b := NewWithContent("test", "")
	b.Insert(0, 'a')
	b.Insert(1, 'b')
	b.Insert(2, 'c')

	b.ApplyUndo()
	if got := b.String(); got != "ab" {
		t.Fatalf("after 1 undo: want %q, got %q", "ab", got)
	}
	b.ApplyUndo()
	if got := b.String(); got != "a" {
		t.Fatalf("after 2 undos: want %q, got %q", "a", got)
	}
	b.ApplyUndo()
	if got := b.String(); got != "" {
		t.Fatalf("after 3 undos: want empty, got %q", got)
	}
	if b.ApplyUndo() {
		t.Fatal("4th ApplyUndo should return false (nothing left)")
	}
}

// TestUndoAfterPartialUndo exercises the critical scenario: undo N steps,
// make a new edit, then undo again.  Without truncating future records in
// Push, the stale undone records would be re-visited, corrupting the buffer.
func TestUndoAfterPartialUndo(t *testing.T) {
	b := NewWithContent("test", "")
	b.Insert(0, 'a') // records: [{0,"a",""}]
	b.Insert(1, 'b') // records: [{0,"a",""},{1,"b",""}]

	b.ApplyUndo() // undo 'b' → "a"
	b.ApplyUndo() // undo 'a' → ""
	if got := b.String(); got != "" {
		t.Fatalf("after 2 undos: want %q, got %q", "", got)
	}

	b.Insert(0, 'c') // new edit; old records must be discarded

	b.ApplyUndo() // undo 'c' → ""
	if got := b.String(); got != "" {
		t.Fatalf("after undo of new edit: want %q, got %q", "", got)
	}

	// No more history — must not touch the (empty) buffer again.
	if b.ApplyUndo() {
		t.Fatal("expected no further undo information")
	}
}

// TestRedoInvalidatedByNewEdit verifies that redo is unavailable after a new
// edit is made following an undo.
func TestRedoInvalidatedByNewEdit(t *testing.T) {
	b := NewWithContent("test", "")
	b.Insert(0, 'a')
	b.Insert(1, 'b')

	b.ApplyUndo() // undo 'b' → "a"

	b.Insert(1, 'c') // new edit after undo; redo of 'b' must not be possible

	if b.ApplyRedo() {
		t.Fatal("redo should not be available after a new edit post-undo")
	}
	if got := b.String(); got != "ac" {
		t.Fatalf("want %q, got %q", "ac", got)
	}
}
