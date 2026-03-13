package buffer

import (
	"strings"
	"testing"
)

// ---- helpers ---------------------------------------------------------------

func mustString(b *Buffer, want string, t *testing.T) {
	t.Helper()
	if got := b.String(); got != want {
		t.Errorf("buffer content = %q, want %q", got, want)
	}
}

// ---- Insert ----------------------------------------------------------------

func TestInsert(t *testing.T) {
	t.Run("insert into empty buffer", func(t *testing.T) {
		b := New("test")
		b.Insert(0, 'A')
		mustString(b, "A", t)
	})

	t.Run("insert at start", func(t *testing.T) {
		b := NewWithContent("test", "ello")
		b.Insert(0, 'H')
		mustString(b, "Hello", t)
	})

	t.Run("insert at end", func(t *testing.T) {
		b := NewWithContent("test", "Hell")
		b.Insert(b.Len(), 'o')
		mustString(b, "Hello", t)
	})

	t.Run("insert in middle", func(t *testing.T) {
		b := NewWithContent("test", "Hllo")
		b.Insert(1, 'e')
		mustString(b, "Hello", t)
	})

	t.Run("InsertString at start", func(t *testing.T) {
		b := NewWithContent("test", "world")
		b.InsertString(0, "hello ")
		mustString(b, "hello world", t)
	})

	t.Run("InsertString at end", func(t *testing.T) {
		b := NewWithContent("test", "hello")
		b.InsertString(b.Len(), " world")
		mustString(b, "hello world", t)
	})

	t.Run("InsertString in middle", func(t *testing.T) {
		b := NewWithContent("test", "helloworld")
		b.InsertString(5, " ")
		mustString(b, "hello world", t)
	})

	t.Run("multiple inserts force gap growth", func(t *testing.T) {
		b := New("test")
		s := strings.Repeat("x", 200)
		b.InsertString(0, s)
		if b.Len() != 200 {
			t.Errorf("Len = %d, want 200", b.Len())
		}
		mustString(b, s, t)
	})
}

// ---- Delete ----------------------------------------------------------------

func TestDelete(t *testing.T) {
	t.Run("delete single rune at start", func(t *testing.T) {
		b := NewWithContent("test", "Hello")
		got := b.Delete(0, 1)
		if got != "H" {
			t.Errorf("deleted = %q, want %q", got, "H")
		}
		mustString(b, "ello", t)
	})

	t.Run("delete single rune at end", func(t *testing.T) {
		b := NewWithContent("test", "Hello")
		b.Delete(4, 1)
		mustString(b, "Hell", t)
	})

	t.Run("delete multiple runes in middle", func(t *testing.T) {
		b := NewWithContent("test", "Hello, world!")
		b.Delete(5, 7)
		mustString(b, "Hello!", t)
	})

	t.Run("delete all", func(t *testing.T) {
		b := NewWithContent("test", "Hello")
		b.Delete(0, b.Len())
		mustString(b, "", t)
	})

	t.Run("delete past end is clamped", func(t *testing.T) {
		b := NewWithContent("test", "Hi")
		b.Delete(0, 100)
		mustString(b, "", t)
	})

	t.Run("delete count 0 is no-op", func(t *testing.T) {
		b := NewWithContent("test", "Hi")
		b.Delete(0, 0)
		mustString(b, "Hi", t)
	})
}

// ---- Gap movement ----------------------------------------------------------

func TestGapMovement(t *testing.T) {
	t.Run("inserts at different positions", func(t *testing.T) {
		b := NewWithContent("test", "ace")
		b.Insert(1, 'b') // a b c e  → "abce"
		mustString(b, "abce", t)
		b.Insert(3, 'd') // a b c d e → "abcde"
		mustString(b, "abcde", t)
	})

	t.Run("interleaved inserts and deletes", func(t *testing.T) {
		b := NewWithContent("test", "Hello World")
		b.Delete(5, 6)         // "Hello"
		b.InsertString(5, "!") // "Hello!"
		mustString(b, "Hello!", t)
	})
}

// ---- Substring -------------------------------------------------------------

func TestSubstring(t *testing.T) {
	b := NewWithContent("test", "Hello, World!")

	t.Run("full string", func(t *testing.T) {
		if got := b.Substring(0, b.Len()); got != "Hello, World!" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("partial", func(t *testing.T) {
		if got := b.Substring(7, 12); got != "World" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("empty range", func(t *testing.T) {
		if got := b.Substring(3, 3); got != "" {
			t.Errorf("got %q", got)
		}
	})
}

// ---- Line helpers ----------------------------------------------------------

func TestLineCount(t *testing.T) {
	tests := []struct {
		content string
		want    int
	}{
		{"", 1},
		{"hello", 1},
		{"hello\nworld", 2},
		{"a\nb\nc", 3},
		{"a\nb\n", 3},
	}
	for _, tc := range tests {
		b := NewWithContent("test", tc.content)
		if got := b.LineCount(); got != tc.want {
			t.Errorf("LineCount(%q) = %d, want %d", tc.content, got, tc.want)
		}
	}
}

func TestLineCol(t *testing.T) {
	b := NewWithContent("test", "abc\nde\nfghi")
	// positions:  0123 4 56 7 8901
	tests := []struct {
		pos      int
		wantLine int
		wantCol  int
	}{
		{0, 1, 0},
		{2, 1, 2},
		{3, 1, 3}, // the '\n' itself counts as col 3
		{4, 2, 0},
		{6, 2, 2},
		{7, 3, 0},
		{10, 3, 3},
	}
	for _, tc := range tests {
		line, col := b.LineCol(tc.pos)
		if line != tc.wantLine || col != tc.wantCol {
			t.Errorf("LineCol(%d) = (%d,%d), want (%d,%d)", tc.pos, line, col, tc.wantLine, tc.wantCol)
		}
	}
}

func TestLineStart(t *testing.T) {
	b := NewWithContent("test", "abc\nde\nfghi")
	tests := []struct{ line, want int }{
		{1, 0},
		{2, 4},
		{3, 7},
		{4, b.Len()}, // beyond last line
	}
	for _, tc := range tests {
		if got := b.LineStart(tc.line); got != tc.want {
			t.Errorf("LineStart(%d) = %d, want %d", tc.line, got, tc.want)
		}
	}
}

func TestBeginningOfLine(t *testing.T) {
	b := NewWithContent("test", "abc\nde\nfghi")
	tests := []struct{ pos, want int }{
		{0, 0},
		{2, 0},
		{4, 4},
		{5, 4},
		{7, 7},
		{10, 7},
	}
	for _, tc := range tests {
		if got := b.BeginningOfLine(tc.pos); got != tc.want {
			t.Errorf("BeginningOfLine(%d) = %d, want %d", tc.pos, got, tc.want)
		}
	}
}

func TestEndOfLine(t *testing.T) {
	b := NewWithContent("test", "abc\nde\nfghi")
	// "abc" ends at 3, "de" ends at 6, "fghi" ends at 11 (Len)
	tests := []struct{ pos, want int }{
		{0, 3},
		{2, 3},
		{4, 6},
		{7, 11},
		{10, 11},
	}
	for _, tc := range tests {
		if got := b.EndOfLine(tc.pos); got != tc.want {
			t.Errorf("EndOfLine(%d) = %d, want %d", tc.pos, got, tc.want)
		}
	}
}

// ---- Mark ------------------------------------------------------------------

func TestMark(t *testing.T) {
	b := NewWithContent("test", "Hello")

	t.Run("mark not set initially", func(t *testing.T) {
		if b.Mark() != -1 {
			t.Errorf("expected mark == -1, got %d", b.Mark())
		}
		if b.MarkActive() {
			t.Error("expected markActive == false")
		}
	})

	t.Run("set and read mark", func(t *testing.T) {
		b.SetMark(3)
		if b.Mark() != 3 {
			t.Errorf("Mark() = %d, want 3", b.Mark())
		}
	})

	t.Run("activate mark", func(t *testing.T) {
		b.SetMarkActive(true)
		if !b.MarkActive() {
			t.Error("expected markActive == true")
		}
	})

	t.Run("deactivate mark", func(t *testing.T) {
		b.SetMarkActive(false)
		if b.MarkActive() {
			t.Error("expected markActive == false")
		}
	})
}

// ---- Undo ------------------------------------------------------------------

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

// ---- Point -----------------------------------------------------------------

func TestPoint(t *testing.T) {
	b := NewWithContent("test", "Hello")

	b.SetPoint(3)
	if b.Point() != 3 {
		t.Errorf("Point() = %d, want 3", b.Point())
	}

	b.SetPoint(-10) // clamped to 0
	if b.Point() != 0 {
		t.Errorf("Point() = %d, want 0 after negative clamp", b.Point())
	}

	b.SetPoint(1000) // clamped to Len()
	if b.Point() != b.Len() {
		t.Errorf("Point() = %d, want %d after upper clamp", b.Point(), b.Len())
	}
}

// ---- Metadata --------------------------------------------------------------

func TestMetadata(t *testing.T) {
	b := New("scratch")

	if b.Name() != "scratch" {
		t.Errorf("Name() = %q", b.Name())
	}
	b.SetName("new-name")
	if b.Name() != "new-name" {
		t.Errorf("SetName failed")
	}

	b.SetFilename("/tmp/foo.go")
	if b.Filename() != "/tmp/foo.go" {
		t.Errorf("Filename() = %q", b.Filename())
	}

	b.SetMode("go")
	if b.Mode() != "go" {
		t.Errorf("Mode() = %q", b.Mode())
	}
	b.SetMode("unknown") // falls back to fundamental
	if b.Mode() != "fundamental" {
		t.Errorf("SetMode(unknown) should set fundamental, got %q", b.Mode())
	}

	if b.Modified() {
		t.Error("new buffer should not be modified")
	}
	b.SetModified(true)
	if !b.Modified() {
		t.Error("Modified() should be true")
	}
}

// ---- RuneAt ----------------------------------------------------------------

func TestRuneAt(t *testing.T) {
	b := NewWithContent("test", "Hello")
	if r := b.RuneAt(0); r != 'H' {
		t.Errorf("RuneAt(0) = %q, want 'H'", r)
	}
	if r := b.RuneAt(4); r != 'o' {
		t.Errorf("RuneAt(4) = %q, want 'o'", r)
	}
	if r := b.RuneAt(100); r != 0 {
		t.Errorf("RuneAt(100) out of range = %q, want 0", r)
	}
}

// ---- Unicode ---------------------------------------------------------------

func TestUnicode(t *testing.T) {
	b := NewWithContent("test", "日本語")
	if b.Len() != 3 {
		t.Errorf("Len() = %d, want 3 runes", b.Len())
	}
	if r := b.RuneAt(1); r != '本' {
		t.Errorf("RuneAt(1) = %q, want '本'", r)
	}
	b.Insert(3, '！')
	mustString(b, "日本語！", t)
}
