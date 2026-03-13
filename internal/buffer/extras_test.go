package buffer

import "testing"

// ---- ReadOnly / SetReadOnly ------------------------------------------------

func TestReadOnlyDefault(t *testing.T) {
	b := NewWithContent("test", "hello")
	if b.ReadOnly() {
		t.Error("new buffer should not be read-only")
	}
}

func TestSetReadOnly(t *testing.T) {
	b := NewWithContent("test", "hello")
	b.SetReadOnly(true)
	if !b.ReadOnly() {
		t.Error("expected read-only after SetReadOnly(true)")
	}
	b.SetReadOnly(false)
	if b.ReadOnly() {
		t.Error("expected writable after SetReadOnly(false)")
	}
}

// ---- PushMarkRing / PopMarkRing --------------------------------------------

func TestPushPopMarkRing(t *testing.T) {
	b := NewWithContent("test", "hello world")
	b.PushMarkRing(3)
	b.PushMarkRing(7)

	if got := b.PopMarkRing(); got != 7 {
		t.Fatalf("PopMarkRing: want 7, got %d", got)
	}
	if got := b.PopMarkRing(); got != 3 {
		t.Fatalf("PopMarkRing: want 3, got %d", got)
	}
}

func TestPopMarkRingEmpty(t *testing.T) {
	b := NewWithContent("test", "hello")
	if got := b.PopMarkRing(); got != -1 {
		t.Fatalf("PopMarkRing on empty ring: want -1, got %d", got)
	}
}

func TestMarkRingCapCapped(t *testing.T) {
	b := NewWithContent("test", "hello")
	for i := range markRingMax + 5 {
		b.PushMarkRing(i)
	}
	// Pop markRingMax times — the ring should never grow beyond markRingMax.
	count := 0
	for b.PopMarkRing() != -1 {
		count++
	}
	if count != markRingMax {
		t.Fatalf("mark ring should hold at most %d entries, got %d", markRingMax, count)
	}
}

// ---- Narrow / Widen / Narrowed / NarrowMin / NarrowMax --------------------

func TestNarrowAndWiden(t *testing.T) {
	b := NewWithContent("test", "hello world")

	if b.Narrowed() {
		t.Error("new buffer should not be narrowed")
	}
	if b.NarrowMin() != 0 {
		t.Errorf("NarrowMin when not narrowed: want 0, got %d", b.NarrowMin())
	}
	if b.NarrowMax() != b.Len() {
		t.Errorf("NarrowMax when not narrowed: want %d, got %d", b.Len(), b.NarrowMax())
	}

	b.Narrow(6, 11) // "world"
	if !b.Narrowed() {
		t.Error("expected Narrowed() to be true after Narrow()")
	}
	if b.NarrowMin() != 6 {
		t.Errorf("NarrowMin: want 6, got %d", b.NarrowMin())
	}
	if b.NarrowMax() != 11 {
		t.Errorf("NarrowMax: want 11, got %d", b.NarrowMax())
	}

	b.Widen()
	if b.Narrowed() {
		t.Error("expected Narrowed() to be false after Widen()")
	}
	if b.NarrowMin() != 0 {
		t.Errorf("NarrowMin after Widen: want 0, got %d", b.NarrowMin())
	}
}

// ---- PosForLineCol ---------------------------------------------------------

func TestPosForLineCol(t *testing.T) {
	b := NewWithContent("test", "hello\nworld\nfoo")
	tests := []struct {
		line, col, want int
	}{
		{1, 0, 0},
		{1, 3, 3},
		{2, 0, 6},
		{2, 3, 9},
		{3, 0, 12},
		{1, 99, 5}, // clamped to end of line (before '\n')
	}
	for _, tc := range tests {
		got := b.PosForLineCol(tc.line, tc.col)
		if got != tc.want {
			t.Errorf("PosForLineCol(%d,%d) = %d, want %d", tc.line, tc.col, got, tc.want)
		}
	}
}

// ---- UndoRing.Redo ---------------------------------------------------------

func TestUndoRingRedo(t *testing.T) {
	u := NewUndoRing(0)
	u.Push(UndoRecord{Pos: 0, Inserted: "a"})
	u.Push(UndoRecord{Pos: 1, Inserted: "b"})

	// Undo both.
	u.Undo()
	u.Undo()

	// Redo the first (oldest-undone = "a").
	rec, ok := u.Redo()
	if !ok {
		t.Fatal("Redo returned false")
	}
	if rec.Inserted != "a" {
		t.Fatalf("Redo: want Inserted=%q, got %q", "a", rec.Inserted)
	}

	// Redo the second.
	rec, ok = u.Redo()
	if !ok {
		t.Fatal("second Redo returned false")
	}
	if rec.Inserted != "b" {
		t.Fatalf("second Redo: want Inserted=%q, got %q", "b", rec.Inserted)
	}

	// Nothing more to redo.
	_, ok = u.Redo()
	if ok {
		t.Fatal("third Redo should return false")
	}
}

func TestUndoRingRedoAtHead(t *testing.T) {
	u := NewUndoRing(0)
	u.Push(UndoRecord{Pos: 0, Inserted: "x"})
	// No undo done — nothing to redo.
	_, ok := u.Redo()
	if ok {
		t.Fatal("Redo without prior Undo should return false")
	}
}
