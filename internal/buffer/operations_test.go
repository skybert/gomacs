package buffer

import "testing"

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
