package buffer

import "testing"

// ---- UndoRing (raw ring) ---------------------------------------------------

func TestUndoInsert(t *testing.T) {
	t.Run("undo single insert restores original", func(t *testing.T) {
		b := NewWithContent("test", "Hello")
		b.Insert(5, '!')
		mustString(b, "Hello!", t)

		rec, ok := b.undo.Undo()
		if !ok {
			t.Fatal("Undo returned false")
		}
		// Apply undo manually: delete what was inserted.
		b.deleteRunes(rec.Pos, len([]rune(rec.Inserted)))
		mustString(b, "Hello", t)
	})

	t.Run("undo InsertString restores original", func(t *testing.T) {
		b := NewWithContent("test", "world")
		b.InsertString(0, "hello ")
		mustString(b, "hello world", t)

		rec, ok := b.undo.Undo()
		if !ok {
			t.Fatal("Undo returned false")
		}
		b.deleteRunes(rec.Pos, len([]rune(rec.Inserted)))
		mustString(b, "world", t)
	})
}

func TestUndoDelete(t *testing.T) {
	t.Run("undo delete restores original", func(t *testing.T) {
		b := NewWithContent("test", "Hello!")
		b.Delete(5, 1)
		mustString(b, "Hello", t)

		rec, ok := b.undo.Undo()
		if !ok {
			t.Fatal("Undo returned false")
		}
		// Apply undo: re-insert the deleted text.
		b.insertRunes(rec.Pos, []rune(rec.Deleted))
		mustString(b, "Hello!", t)
	})
}

func TestUndoMultipleSteps(t *testing.T) {
	b := NewWithContent("test", "")
	b.InsertString(0, "a")
	b.InsertString(1, "b")
	b.InsertString(2, "c")
	mustString(b, "abc", t)

	// Undo "c"
	rec, ok := b.undo.Undo()
	if !ok {
		t.Fatal("step 1 Undo returned false")
	}
	b.deleteRunes(rec.Pos, len([]rune(rec.Inserted)))
	mustString(b, "ab", t)

	// Undo "b"
	rec, ok = b.undo.Undo()
	if !ok {
		t.Fatal("step 2 Undo returned false")
	}
	b.deleteRunes(rec.Pos, len([]rune(rec.Inserted)))
	mustString(b, "a", t)

	// Undo "a"
	rec, ok = b.undo.Undo()
	if !ok {
		t.Fatal("step 3 Undo returned false")
	}
	b.deleteRunes(rec.Pos, len([]rune(rec.Inserted)))
	mustString(b, "", t)

	// No more undos.
	_, ok = b.undo.Undo()
	if ok {
		t.Error("expected Undo to return false when history exhausted")
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
