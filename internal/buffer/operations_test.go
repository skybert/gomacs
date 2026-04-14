package buffer

import "testing"

func TestReplaceStringBasic(t *testing.T) {
	b := NewWithContent("test", "Hello World")
	b.ReplaceString(0, 5, "hello")
	if got := b.String(); got != "hello World" {
		t.Fatalf("after replace: want %q, got %q", "hello World", got)
	}
}

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
