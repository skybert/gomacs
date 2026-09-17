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

func TestLineStartsFrom(t *testing.T) {
	b := NewWithContent("test", "abc\nde\nfghi")
	// "abc" line 1: pos 0, "de" line 2: pos 4, "fghi" line 3: pos 7
	tests := []struct {
		from, count int
		want        []int
	}{
		{1, 3, []int{0, 4, 7}},
		{2, 2, []int{4, 7}},
		{3, 1, []int{7}},
		{1, 5, []int{0, 4, 7, b.Len(), b.Len()}}, // beyond last line → Len()
		{4, 2, []int{b.Len(), b.Len()}},          // all beyond EOF
	}
	for _, tc := range tests {
		got := b.LineStartsFrom(tc.from, tc.count)
		if len(got) != len(tc.want) {
			t.Errorf("LineStartsFrom(%d,%d) len=%d, want %d", tc.from, tc.count, len(got), len(tc.want))
			continue
		}
		for i, w := range tc.want {
			if got[i] != w {
				t.Errorf("LineStartsFrom(%d,%d)[%d] = %d, want %d", tc.from, tc.count, i, got[i], w)
			}
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

// ---- changeGen / Modified --------------------------------------------------

func TestModifiedFalseAfterNewWithContent(t *testing.T) {
	b := NewWithContent("test", "hello")
	if b.Modified() {
		t.Error("NewWithContent buffer should not be Modified()")
	}
}

func TestModifiedTrueAfterInsert(t *testing.T) {
	b := NewWithContent("test", "hello")
	b.Insert(0, 'X')
	if !b.Modified() {
		t.Error("buffer should be Modified() after Insert")
	}
}

func TestModifiedClearedAfterUndoToOriginal(t *testing.T) {
	b := NewWithContent("test", "hello")
	b.Insert(0, 'X')
	if !b.Modified() {
		t.Error("expected Modified() after Insert")
	}
	b.ApplyUndo()
	if b.Modified() {
		t.Error("Modified() should be false after undoing back to saved state")
	}
}

func TestModifiedAfterUndoAndRedo(t *testing.T) {
	b := NewWithContent("test", "hello")
	b.Insert(0, 'X')
	b.ApplyUndo()
	b.ApplyRedo()
	if !b.Modified() {
		t.Error("Modified() should be true after redo")
	}
}

func TestSetModifiedFalseSnapshotsGen(t *testing.T) {
	b := NewWithContent("test", "hello")
	b.Insert(0, 'X')
	b.SetModified(false) // simulate save
	if b.Modified() {
		t.Error("Modified() should be false after SetModified(false)")
	}
	// Further undo should make it modified again.
	b.ApplyUndo()
	if !b.Modified() {
		t.Error("Modified() should be true after undoing past the save point")
	}
}

// ---- benchmarks ------------------------------------------------------------

// benchContent builds a buffer body of `lines` lines of roughly source-file
// width, which is what the line-start index has to cope with in practice.
func benchContent(lines int) string {
	var sb strings.Builder
	for i := range lines {
		sb.WriteString("\tsomeIdentifier := doSomething(a, b, c) // line ")
		sb.WriteString(string(rune('0' + i%10)))
		sb.WriteByte('\n')
	}
	return sb.String()
}

// BenchmarkLineStartLastLine measures LineStart() for the last line of a large
// buffer — the worst case for a scan-from-zero implementation.
func BenchmarkLineStartLastLine(b *testing.B) {
	buf := NewWithContent("bench", benchContent(20000))
	last := buf.LineCount()
	for b.Loop() {
		_ = buf.LineStart(last)
	}
}

// BenchmarkLineStartWindow measures the access pattern of one render frame:
// the line starts of 40 consecutive visible lines near the end of the buffer.
func BenchmarkLineStartWindow(b *testing.B) {
	buf := NewWithContent("bench", benchContent(20000))
	first := buf.LineCount() - 40
	for b.Loop() {
		for i := range 40 {
			_ = buf.LineStart(first + i)
		}
	}
}

// BenchmarkLineStartAfterAppend measures the append-then-query pattern used by
// output buffers (compilation, shell): one line appended at end of buffer
// followed by a line-start query.
func BenchmarkLineStartAfterAppend(b *testing.B) {
	buf := NewWithContent("bench", benchContent(20000))
	for b.Loop() {
		buf.InsertString(buf.Len(), "another line of output\n")
		_ = buf.LineStart(buf.LineCount())
	}
}

// BenchmarkLineStartAfterMidEdit measures the worst case for an incrementally
// maintained index: an edit in the middle of the buffer followed by a
// line-start query, i.e. a full index rebuild per iteration.
func BenchmarkLineStartAfterMidEdit(b *testing.B) {
	buf := NewWithContent("bench", benchContent(20000))
	mid := buf.Len() / 2
	last := buf.LineCount()
	for b.Loop() {
		buf.Insert(mid, 'x')
		_ = buf.LineStart(last)
	}
}

// ---- line-start index ------------------------------------------------------

// lineStartRef is a deliberately naive reference implementation of LineStart,
// used to cross-check the incrementally maintained index.
func lineStartRef(b *Buffer, line int) int {
	if line <= 1 {
		return 0
	}
	current := 1
	for i := range b.Len() {
		if b.RuneAt(i) == '\n' {
			current++
			if current == line {
				return i + 1
			}
		}
	}
	return b.Len()
}

// checkLineStarts asserts LineStart agrees with the reference scan for every
// line number from 1 to LineCount()+2 (i.e. including past-EOF lines).
func checkLineStarts(t *testing.T, b *Buffer, what string) {
	t.Helper()
	for line := 1; line <= b.LineCount()+2; line++ {
		if got, want := b.LineStart(line), lineStartRef(b, line); got != want {
			t.Errorf("%s: LineStart(%d) = %d, want %d (content %q)", what, line, got, want, b.String())
		}
	}
}

func TestLineStartIndexIsStableAcrossEdits(t *testing.T) {
	t.Run("append at end", func(t *testing.T) {
		b := NewWithContent("test", "abc\nde\n")
		checkLineStarts(t, b, "initial")
		b.InsertString(b.Len(), "fghi\njkl")
		checkLineStarts(t, b, "after append")
	})

	t.Run("append newline only", func(t *testing.T) {
		b := NewWithContent("test", "abc")
		checkLineStarts(t, b, "initial")
		b.Insert(b.Len(), '\n')
		if got := b.LineCount(); got != 2 {
			t.Errorf("LineCount after appending newline = %d, want 2", got)
		}
		checkLineStarts(t, b, "after newline append")
	})

	t.Run("insert in middle", func(t *testing.T) {
		b := NewWithContent("test", "abc\nde\nfghi")
		checkLineStarts(t, b, "initial")
		b.InsertString(2, "XX\nYY")
		checkLineStarts(t, b, "after middle insert")
	})

	t.Run("insert at line start", func(t *testing.T) {
		b := NewWithContent("test", "abc\nde")
		checkLineStarts(t, b, "initial")
		b.InsertString(4, "Z")
		if got := b.LineStart(2); got != 4 {
			t.Errorf("LineStart(2) after insert at line start = %d, want 4", got)
		}
		checkLineStarts(t, b, "after insert at line start")
	})

	t.Run("delete at end", func(t *testing.T) {
		b := NewWithContent("test", "abc\nde\nfghi")
		checkLineStarts(t, b, "initial")
		b.Delete(6, b.Len()-6) // drops the trailing newline and "fghi"
		checkLineStarts(t, b, "after delete at end")
	})

	t.Run("delete in middle", func(t *testing.T) {
		b := NewWithContent("test", "abc\nde\nfghi")
		checkLineStarts(t, b, "initial")
		b.Delete(1, 4)
		checkLineStarts(t, b, "after delete in middle")
	})

	t.Run("delete whole buffer", func(t *testing.T) {
		b := NewWithContent("test", "abc\nde\n")
		checkLineStarts(t, b, "initial")
		b.Delete(0, b.Len())
		if got := b.LineStart(1); got != 0 {
			t.Errorf("LineStart(1) on empty buffer = %d, want 0", got)
		}
		checkLineStarts(t, b, "after deleting everything")
	})

	t.Run("replace string", func(t *testing.T) {
		b := NewWithContent("test", "abc\nde\nfghi")
		checkLineStarts(t, b, "initial")
		b.ReplaceString(4, 2, "x\ny\nz")
		checkLineStarts(t, b, "after replace")
	})

	t.Run("replace at end of buffer", func(t *testing.T) {
		b := NewWithContent("test", "abc\nde\nfghi")
		checkLineStarts(t, b, "initial")
		// Deletion reaches the end (index truncates) and the insertion then
		// appends (index extends) — both incremental paths in one call.
		b.ReplaceString(7, 4, "x\ny\n")
		checkLineStarts(t, b, "after replace at end")
	})

	t.Run("undo and redo", func(t *testing.T) {
		b := NewWithContent("test", "abc\nde")
		checkLineStarts(t, b, "initial")
		b.InsertString(2, "1\n2\n3")
		checkLineStarts(t, b, "after insert")
		b.ApplyUndo()
		checkLineStarts(t, b, "after undo")
		b.ApplyRedo()
		checkLineStarts(t, b, "after redo")
	})
}

// TestLineStartIndexAfterRepeatedAppends exercises the incremental append path
// many times in a row, which is what output buffers do.
func TestLineStartIndexAfterRepeatedAppends(t *testing.T) {
	b := New("test")
	_ = b.LineStart(1) // build the index while the buffer is empty
	for i := range 20 {
		b.InsertString(b.Len(), "line\n")
		if got, want := b.LineCount(), i+2; got != want {
			t.Fatalf("after %d appends: LineCount = %d, want %d", i+1, got, want)
		}
		if got, want := b.LineStart(i+2), (i+1)*5; got != want {
			t.Fatalf("after %d appends: LineStart(%d) = %d, want %d", i+1, i+2, got, want)
		}
	}
	checkLineStarts(t, b, "after 20 appends")
}

// TestLineStartIndexAfterTrailingDeletes exercises the truncation path.
func TestLineStartIndexAfterTrailingDeletes(t *testing.T) {
	b := NewWithContent("test", "a\nb\nc\nd\n")
	checkLineStarts(t, b, "initial")
	for b.Len() > 0 {
		b.Delete(b.Len()-1, 1)
		checkLineStarts(t, b, "after trailing delete")
	}
}

// TestLineStartIgnoresNarrowing documents that LineStart positions are absolute
// and unaffected by narrowing, matching the pre-index behaviour.
func TestLineStartIgnoresNarrowing(t *testing.T) {
	b := NewWithContent("test", "abc\nde\nfghi")
	want := b.LineStart(3)
	b.Narrow(4, 6)
	if got := b.LineStart(3); got != want {
		t.Errorf("LineStart(3) while narrowed = %d, want %d (absolute)", got, want)
	}
	b.Widen()
	if got := b.LineStart(3); got != want {
		t.Errorf("LineStart(3) after widen = %d, want %d", got, want)
	}
}

// TestLineStartSeedsLineCount verifies that building the line-start index also
// seeds the incremental line count.
func TestLineStartSeedsLineCount(t *testing.T) {
	b := NewWithContent("test", "a\nb\nc")
	if b.lineCountReady {
		t.Fatal("lineCountReady should start false")
	}
	_ = b.LineStart(2)
	if !b.lineCountReady {
		t.Error("LineStart should seed the incremental line count")
	}
	if got := b.LineCount(); got != 3 {
		t.Errorf("LineCount = %d, want 3", got)
	}
}

// TestLineStartsFromAfterEdits checks LineStartsFrom against the reference
// implementation once the index has been through a few edits.
func TestLineStartsFromAfterEdits(t *testing.T) {
	b := NewWithContent("test", "abc\nde\nfghi\n")
	_ = b.LineStart(2)
	b.InsertString(2, "Z\nZ")
	b.Delete(0, 1)
	b.InsertString(b.Len(), "tail\n")
	got := b.LineStartsFrom(1, b.LineCount()+2)
	for i, pos := range got {
		if want := lineStartRef(b, i+1); pos != want {
			t.Errorf("LineStartsFrom[%d] = %d, want %d (content %q)", i, pos, want, b.String())
		}
	}
}

// TestLineStartsFromClampsLowLine keeps the documented behaviour that `from`
// below 1 is clamped to line 1.
func TestLineStartsFromClampsLowLine(t *testing.T) {
	b := NewWithContent("test", "abc\nde")
	got := b.LineStartsFrom(0, 2)
	want := []int{0, 4}
	for i, wv := range want {
		if got[i] != wv {
			t.Errorf("LineStartsFrom(0,2)[%d] = %d, want %d", i, got[i], wv)
		}
	}
}

// TestLineStartsFromNonPositiveCount covers the early-out for a non-positive
// count: no slice is allocated at all.
func TestLineStartsFromNonPositiveCount(t *testing.T) {
	b := NewWithContent("test", "abc\nde\nfgh")
	for _, count := range []int{0, -1, -7} {
		if got := b.LineStartsFrom(1, count); got != nil {
			t.Errorf("LineStartsFrom(1,%d) = %v, want nil", count, got)
		}
	}
}

// TestLineStartsFromPosNonPositiveCount covers the same early-out on the
// position-seeded variant.
func TestLineStartsFromPosNonPositiveCount(t *testing.T) {
	b := NewWithContent("test", "abc\nde\nfgh")
	for _, count := range []int{0, -1, -7} {
		if got := b.LineStartsFromPos(2, 4, count); got != nil {
			t.Errorf("LineStartsFromPos(2,4,%d) = %v, want nil", count, got)
		}
	}
}

// TestLineStartsFromPosClampsLowLine documents that a `from` below 1 restarts
// the scan at the top of the buffer, ignoring the supplied bufPos hint.
func TestLineStartsFromPosClampsLowLine(t *testing.T) {
	b := NewWithContent("test", "abc\nde\nfgh")
	// from = 0 with a bogus bufPos of 4: the hint must be discarded and the
	// scan must restart at position 0, yielding the first three line starts.
	got := b.LineStartsFromPos(0, 4, 3)
	want := []int{0, 4, 7}
	if len(got) != len(want) {
		t.Fatalf("LineStartsFromPos(0,4,3) len = %d, want %d", len(got), len(want))
	}
	for i, wv := range want {
		if got[i] != wv {
			t.Errorf("LineStartsFromPos(0,4,3)[%d] = %d, want %d", i, got[i], wv)
		}
	}
}

// TestLineStartIndexSpansGap covers the two-segment scan: the gap is left in
// the middle of the buffer by an edit, then the index is rebuilt.
func TestLineStartIndexSpansGap(t *testing.T) {
	b := NewWithContent("test", "aaa\nbbb\nccc\nddd")
	b.InsertString(5, "\n") // leaves the gap mid-buffer
	checkLineStarts(t, b, "with gap mid-buffer")
}

// ---- SetMode language-suffixed modes ---------------------------------------

// TestSetModeLanguageSuffix verifies that modes carrying a language suffix are
// accepted verbatim, while an unknown prefix still falls back to fundamental.
func TestSetModeLanguageSuffix(t *testing.T) {
	tests := []struct{ set, want string }{
		{"vc-annotate+go", "vc-annotate+go"},
		{"debug-repl+java", "debug-repl+java"},
		{"debug-repl+python", "debug-repl+python"},
		{"bogus+go", modeFundamental},
		{"debug-repl", "debug-repl"},
	}
	for _, tc := range tests {
		b := New("test")
		b.SetMode(tc.set)
		if got := b.Mode(); got != tc.want {
			t.Errorf("SetMode(%q) → Mode() = %q, want %q", tc.set, got, tc.want)
		}
	}
}

// ---- Substring edge cases --------------------------------------------------

func TestSubstringNegativeStart(t *testing.T) {
	b := NewWithContent("t", "Hello")
	// Negative start should be clamped to 0.
	got := b.Substring(-5, 3)
	if got != "Hel" {
		t.Errorf("Substring(-5,3) = %q, want %q", got, "Hel")
	}
}

func TestSubstringEndBeyondLen(t *testing.T) {
	b := NewWithContent("t", "Hello")
	// end > Len() should be clamped to Len().
	got := b.Substring(3, 100)
	if got != "lo" {
		t.Errorf("Substring(3,100) = %q, want %q", got, "lo")
	}
}

func TestSubstringStartEqualsEnd(t *testing.T) {
	b := NewWithContent("t", "Hello")
	if got := b.Substring(2, 2); got != "" {
		t.Errorf("Substring(2,2) = %q, want empty", got)
	}
}

func TestSubstringStartAfterEnd(t *testing.T) {
	b := NewWithContent("t", "Hello")
	if got := b.Substring(4, 2); got != "" {
		t.Errorf("Substring(4,2) = %q, want empty", got)
	}
}

// Force the gap to be in the middle of the extracted range so the
// "spans the gap" branch of Substring is exercised.
func TestSubstringSpansGap(t *testing.T) {
	b := NewWithContent("t", "abc")
	// Insert at position 1 to move the gap mid-buffer.
	b.Insert(1, 'X')
	// b now contains "aXbc", gap is after position 2.
	got := b.String()
	if got != "aXbc" {
		t.Fatalf("unexpected buffer content %q after insert", got)
	}
	// Extract a range that spans the gap: positions 0..4.
	if sub := b.Substring(0, 4); sub != "aXbc" {
		t.Errorf("Substring(0,4) across gap = %q, want %q", sub, "aXbc")
	}
}

// Range entirely after the gap.
func TestSubstringAfterGap(t *testing.T) {
	b := NewWithContent("t", "Hello World")
	// Move the gap to position 5 by inserting then deleting.
	b.Insert(5, 'X')
	b.Delete(5, 1)
	// Gap is now around position 5; extract a range after it.
	got := b.Substring(6, 11)
	if got != "World" {
		t.Errorf("Substring(6,11) after gap = %q, want %q", got, "World")
	}
}

// Narrowed buffer: Substring still returns raw logical positions.
func TestSubstringNarrowedBuffer(t *testing.T) {
	b := NewWithContent("t", "0123456789")
	b.Narrow(3, 7)
	// Substring uses logical positions, not narrowed ones.
	got := b.Substring(3, 7)
	if got != "3456" {
		t.Errorf("Substring(3,7) in narrowed buffer = %q, want %q", got, "3456")
	}
}

// ---- LineStartsFromPos -----------------------------------------------------

func TestLineStartsFromPosSingleLine(t *testing.T) {
	b := NewWithContent("t", "hello")
	got := b.LineStartsFromPos(1, 0, 1)
	if len(got) != 1 || got[0] != 0 {
		t.Errorf("LineStartsFromPos single line: %v, want [0]", got)
	}
}

func TestLineStartsFromPosMultiLine(t *testing.T) {
	// "abc\nde\nfghi" — line 1 at 0, line 2 at 4, line 3 at 7.
	b := NewWithContent("t", "abc\nde\nfghi")
	got := b.LineStartsFromPos(1, 0, 3)
	want := []int{0, 4, 7}
	for i, w := range want {
		if i >= len(got) || got[i] != w {
			t.Errorf("LineStartsFromPos[%d] = %v, want %d", i, got, w)
		}
	}
}

func TestLineStartsFromPosMidBuffer(t *testing.T) {
	b := NewWithContent("t", "abc\nde\nfghi")
	// Start from line 2 whose known position is 4.
	got := b.LineStartsFromPos(2, 4, 2)
	want := []int{4, 7}
	for i, w := range want {
		if i >= len(got) || got[i] != w {
			t.Errorf("LineStartsFromPos from mid[%d] = %v, want %d", i, got, w)
		}
	}
}

func TestLineStartsFromPosCountZero(t *testing.T) {
	b := NewWithContent("t", "abc\nde")
	got := b.LineStartsFromPos(1, 0, 0)
	if got != nil {
		t.Errorf("LineStartsFromPos count=0 should return nil, got %v", got)
	}
}

func TestLineStartsFromPosCountBeyondLines(t *testing.T) {
	b := NewWithContent("t", "abc\nde")
	// Only 2 lines; request 5 — extras should be Len().
	got := b.LineStartsFromPos(1, 0, 5)
	if len(got) != 5 {
		t.Fatalf("expected length 5, got %d", len(got))
	}
	n := b.Len()
	for _, v := range got[2:] {
		if v != n {
			t.Errorf("past-EOF slot = %d, want %d (Len)", v, n)
		}
	}
}

// Start position is after the gap.
func TestLineStartsFromPosAfterGap(t *testing.T) {
	b := NewWithContent("t", "abc\nde\nfghi")
	// Force gap near position 4 by inserting then deleting.
	b.Insert(4, 'X')
	b.Delete(4, 1)
	// Now request from line 2 (buf pos 4).
	got := b.LineStartsFromPos(2, 4, 2)
	want := []int{4, 7}
	for i, w := range want {
		if i >= len(got) || got[i] != w {
			t.Errorf("after gap: LineStartsFromPos[%d] = %v, want %d", i, got, w)
		}
	}
}

// ---- ModCount --------------------------------------------------------------

func TestModCountInitiallyZero(t *testing.T) {
	b := New("t")
	if b.ModCount() != 0 {
		t.Errorf("initial ModCount = %d, want 0", b.ModCount())
	}
}

func TestModCountIncreasesOnInsert(t *testing.T) {
	b := New("t")
	b.Insert(0, 'A')
	if b.ModCount() != 1 {
		t.Errorf("ModCount after Insert = %d, want 1", b.ModCount())
	}
	b.Insert(1, 'B')
	if b.ModCount() != 2 {
		t.Errorf("ModCount after 2nd Insert = %d, want 2", b.ModCount())
	}
}

func TestModCountIncreasesOnDelete(t *testing.T) {
	b := NewWithContent("t", "Hello")
	initial := b.ModCount()
	b.Delete(0, 1)
	if b.ModCount() != initial+1 {
		t.Errorf("ModCount after Delete = %d, want %d", b.ModCount(), initial+1)
	}
}

func TestModCountIncreasesOnInsertString(t *testing.T) {
	b := New("t")
	b.InsertString(0, "hi")
	if b.ModCount() != 1 {
		t.Errorf("ModCount after InsertString = %d, want 1", b.ModCount())
	}
}

func TestModCountNotAffectedByNewWithContent(t *testing.T) {
	// NewWithContent resets the undo ring but does NOT reset modCount — internal
	// detail. What matters is that subsequent edits increment it.
	b := NewWithContent("t", "abc")
	mc := b.ModCount()
	b.Insert(0, 'X')
	if b.ModCount() != mc+1 {
		t.Errorf("ModCount after Insert on NewWithContent = %d, want %d", b.ModCount(), mc+1)
	}
}

// ---- ChangeGen -------------------------------------------------------------

func TestChangeGenInitiallyZero(t *testing.T) {
	b := NewWithContent("t", "hello")
	if b.ChangeGen() != 0 {
		t.Errorf("initial ChangeGen = %d, want 0", b.ChangeGen())
	}
}

func TestChangeGenIncreasesOnInsert(t *testing.T) {
	b := NewWithContent("t", "hello")
	g0 := b.ChangeGen()
	b.Insert(0, 'X')
	if b.ChangeGen() != g0+1 {
		t.Errorf("ChangeGen after Insert = %d, want %d", b.ChangeGen(), g0+1)
	}
}

func TestChangeGenIncreasesOnDelete(t *testing.T) {
	b := NewWithContent("t", "hello")
	g0 := b.ChangeGen()
	b.Delete(0, 1)
	if b.ChangeGen() != g0+1 {
		t.Errorf("ChangeGen after Delete = %d, want %d", b.ChangeGen(), g0+1)
	}
}

func TestChangeGenDecreasesOnUndo(t *testing.T) {
	b := NewWithContent("t", "hello")
	b.Insert(0, 'X')
	g1 := b.ChangeGen()
	b.ApplyUndo()
	if b.ChangeGen() == g1 {
		t.Errorf("ChangeGen should change after ApplyUndo")
	}
}

func TestChangeGenIncreasesOnReplaceString(t *testing.T) {
	b := NewWithContent("t", "hello")
	g0 := b.ChangeGen()
	b.ReplaceString(0, 2, "HE")
	if b.ChangeGen() != g0+1 {
		t.Errorf("ChangeGen after ReplaceString = %d, want %d", b.ChangeGen(), g0+1)
	}
}

// ---- BeginningOfLine -------------------------------------------------------

func TestBeginningOfLineAtStart(t *testing.T) {
	b := NewWithContent("t", "hello\nworld")
	if got := b.BeginningOfLine(0); got != 0 {
		t.Errorf("BeginningOfLine(0) = %d, want 0", got)
	}
}

func TestBeginningOfLineMidLine(t *testing.T) {
	b := NewWithContent("t", "hello\nworld")
	// Position 3 is 'l' on line 1; beginning of line is 0.
	if got := b.BeginningOfLine(3); got != 0 {
		t.Errorf("BeginningOfLine(3) = %d, want 0", got)
	}
}

func TestBeginningOfLineSecondLine(t *testing.T) {
	b := NewWithContent("t", "hello\nworld")
	// Position 7 is 'o' on line 2; beginning is 6 (after the \n at 5).
	if got := b.BeginningOfLine(7); got != 6 {
		t.Errorf("BeginningOfLine(7) = %d, want 6", got)
	}
}

func TestBeginningOfLineAtNewline(t *testing.T) {
	b := NewWithContent("t", "abc\ndef")
	// Position 3 is the newline; BeginningOfLine should return 0 (start of line 1).
	if got := b.BeginningOfLine(3); got != 0 {
		t.Errorf("BeginningOfLine(3) at newline = %d, want 0", got)
	}
}

func TestBeginningOfLinePastEnd(t *testing.T) {
	b := NewWithContent("t", "abc\ndef")
	// Position beyond Len() should be clamped.
	got := b.BeginningOfLine(b.Len() + 5)
	if got != 4 {
		t.Errorf("BeginningOfLine past end = %d, want 4", got)
	}
}

// ---- EndOfLine -------------------------------------------------------------

func TestEndOfLineAtStart(t *testing.T) {
	b := NewWithContent("t", "hello\nworld")
	// Position 0: EndOfLine is at the newline position 5.
	if got := b.EndOfLine(0); got != 5 {
		t.Errorf("EndOfLine(0) = %d, want 5", got)
	}
}

func TestEndOfLineMidLine(t *testing.T) {
	b := NewWithContent("t", "hello\nworld")
	if got := b.EndOfLine(2); got != 5 {
		t.Errorf("EndOfLine(2) = %d, want 5", got)
	}
}

func TestEndOfLineLastLine(t *testing.T) {
	b := NewWithContent("t", "hello\nworld")
	// Line 2 has no trailing newline; EndOfLine returns Len().
	if got := b.EndOfLine(6); got != b.Len() {
		t.Errorf("EndOfLine(6) = %d, want %d (Len)", got, b.Len())
	}
}

func TestEndOfLineSingleLine(t *testing.T) {
	b := NewWithContent("t", "noNewline")
	if got := b.EndOfLine(0); got != b.Len() {
		t.Errorf("EndOfLine(0) single-line = %d, want %d", got, b.Len())
	}
}

func TestEndOfLinePastEnd(t *testing.T) {
	b := NewWithContent("t", "abc")
	got := b.EndOfLine(b.Len() + 10)
	if got != b.Len() {
		t.Errorf("EndOfLine past end = %d, want %d", got, b.Len())
	}
}

// ---- ReplaceString ---------------------------------------------------------

func TestReplaceStringSameLength(t *testing.T) {
	b := NewWithContent("t", "Hello World")
	b.ReplaceString(6, 5, "Earth")
	got := b.String()
	if got != "Hello Earth" {
		t.Errorf("ReplaceString same length = %q, want %q", got, "Hello Earth")
	}
}

func TestReplaceStringShorter(t *testing.T) {
	b := NewWithContent("t", "Hello World")
	b.ReplaceString(6, 5, "Go")
	got := b.String()
	if got != "Hello Go" {
		t.Errorf("ReplaceString shorter = %q, want %q", got, "Hello Go")
	}
}

func TestReplaceStringLonger(t *testing.T) {
	b := NewWithContent("t", "Hello X")
	b.ReplaceString(6, 1, "World")
	got := b.String()
	if got != "Hello World" {
		t.Errorf("ReplaceString longer = %q, want %q", got, "Hello World")
	}
}

func TestReplaceStringAtStart(t *testing.T) {
	b := NewWithContent("t", "Hello World")
	b.ReplaceString(0, 5, "Hi")
	got := b.String()
	if got != "Hi World" {
		t.Errorf("ReplaceString at start = %q, want %q", got, "Hi World")
	}
}

func TestReplaceStringCountClamped(t *testing.T) {
	b := NewWithContent("t", "abcde")
	// count goes past end; should be clamped.
	b.ReplaceString(3, 100, "XY")
	got := b.String()
	if got != "abcXY" {
		t.Errorf("ReplaceString count clamped = %q, want %q", got, "abcXY")
	}
}

func TestReplaceStringNegativePosNoOp(t *testing.T) {
	b := NewWithContent("t", "hello")
	b.ReplaceString(-1, 2, "ZZ")
	// pos < 0 gets clamped to 0, not a no-op; verify content changed correctly.
	got := b.String()
	if got != "ZZllo" {
		t.Errorf("ReplaceString negative pos = %q, want %q", got, "ZZllo")
	}
}

func TestReplaceStringZeroCountNoOp(t *testing.T) {
	b := NewWithContent("t", "hello")
	b.ReplaceString(2, 0, "ZZ")
	// count <= 0 → no-op.
	if got := b.String(); got != "hello" {
		t.Errorf("ReplaceString count=0 should be no-op, got %q", got)
	}
}

func TestReplaceStringModCount(t *testing.T) {
	b := NewWithContent("t", "hello")
	mc := b.ModCount()
	b.ReplaceString(0, 2, "HE")
	if b.ModCount() != mc+1 {
		t.Errorf("ModCount after ReplaceString = %d, want %d", b.ModCount(), mc+1)
	}
}

func TestReplaceStringUndoable(t *testing.T) {
	b := NewWithContent("t", "hello")
	b.ReplaceString(0, 5, "world")
	if got := b.String(); got != "world" {
		t.Fatalf("after ReplaceString = %q, want %q", got, "world")
	}
	b.ApplyUndo()
	if got := b.String(); got != "hello" {
		t.Errorf("after undo of ReplaceString = %q, want %q", got, "hello")
	}
}

// ---- growGap (indirect via large insertion) --------------------------------

func TestGrowGapForcedByLargeInsert(t *testing.T) {
	b := New("t")
	// The initial gap is initialGapSize (64) runes. Insert more than that in
	// one shot so growGap is called.
	large := strings.Repeat("x", initialGapSize*3)
	b.InsertString(0, large)
	if b.Len() != initialGapSize*3 {
		t.Errorf("Len after large insert = %d, want %d", b.Len(), initialGapSize*3)
	}
	if b.String() != large {
		t.Error("buffer content corrupted after large insert forcing growGap")
	}
}

func TestGrowGapPreservesContent(t *testing.T) {
	b := NewWithContent("t", "start")
	// Now append enough to exceed the gap twice over.
	extra := strings.Repeat("y", initialGapSize*4)
	b.InsertString(b.Len(), extra)
	want := "start" + extra
	if b.String() != want {
		t.Errorf("content mismatch after forced growGap (len=%d, want %d)", b.Len(), len([]rune(want)))
	}
}

// growGap: when the buffer is large, grow is driven by len(data)/4 rather than
// the small requested extra.
func TestGrowGapQuarterDriven(t *testing.T) {
	b := New("t")
	base := strings.Repeat("z", initialGapSize*8)
	b.InsertString(0, base)
	// Buffer now large with a small remaining gap; insert one rune at a time
	// to force a growGap where len(data)/4 exceeds the requested extra.
	b.InsertString(b.Len(), "tail")
	if !strings.HasSuffix(b.String(), "tail") {
		t.Errorf("growGap quarter-driven: content corrupted, suffix=%q", b.String()[len(b.String())-4:])
	}
}

// ---- SetMode ---------------------------------------------------------------

func TestSetModeValid(t *testing.T) {
	b := New("t")
	b.SetMode("go")
	if b.Mode() != "go" {
		t.Errorf("SetMode go = %q, want go", b.Mode())
	}
}

func TestSetModeVcAnnotatePrefix(t *testing.T) {
	b := New("t")
	b.SetMode("vc-annotate+go")
	if b.Mode() != "vc-annotate+go" {
		t.Errorf("SetMode vc-annotate+go = %q, want vc-annotate+go", b.Mode())
	}
}

func TestSetModeInvalidFallsBackToFundamental(t *testing.T) {
	b := New("t")
	b.SetMode("go") // start from a non-default mode
	b.SetMode("nonsense-mode")
	if b.Mode() != modeFundamental {
		t.Errorf("SetMode invalid = %q, want %q", b.Mode(), modeFundamental)
	}
}

// ---- rawIndex (post-gap branch) --------------------------------------------

func TestRuneAtAfterGap(t *testing.T) {
	b := NewWithContent("t", "abcdef")
	// Move gap to the middle by inserting mid-buffer; positions >= gapStart
	// exercise the post-gap branch of rawIndex.
	b.Insert(2, 'X')
	// Now "abXcdef"; position 5 (>= gapStart) exercises the post-gap branch.
	if got := b.RuneAt(5); got != 'e' {
		t.Errorf("RuneAt(5) after gap = %q, want 'e'", got)
	}
}

// ---- InsertString empty ----------------------------------------------------

func TestInsertStringEmptyNoOp(t *testing.T) {
	b := NewWithContent("t", "hello")
	mc := b.ModCount()
	b.InsertString(2, "")
	if b.String() != "hello" {
		t.Errorf("InsertString empty changed content: %q", b.String())
	}
	if b.ModCount() != mc {
		t.Errorf("InsertString empty bumped ModCount: %d, want %d", b.ModCount(), mc)
	}
}

// ---- insertRunes / deleteRunes incremental line count ----------------------

func TestInsertUpdatesLineCountDelta(t *testing.T) {
	b := NewWithContent("t", "a\nb")
	if b.LineCount() != 2 { // seeds lineCountReady
		t.Fatalf("initial LineCount = %d, want 2", b.LineCount())
	}
	b.InsertString(b.Len(), "\nc\nd")
	if b.LineCount() != 4 {
		t.Errorf("LineCount after inserting newlines = %d, want 4", b.LineCount())
	}
}

func TestDeleteUpdatesLineCountDelta(t *testing.T) {
	b := NewWithContent("t", "a\nb\nc")
	if b.LineCount() != 3 { // seeds lineCountReady
		t.Fatalf("initial LineCount = %d, want 3", b.LineCount())
	}
	b.Delete(1, 2) // remove "\nb"
	if b.LineCount() != 2 {
		t.Errorf("LineCount after deleting a newline = %d, want 2", b.LineCount())
	}
}

// ---- insertRunes / deleteRunes mark adjustment -----------------------------

func TestInsertShiftsMark(t *testing.T) {
	b := NewWithContent("t", "hello")
	b.SetMark(3)
	b.InsertString(0, "AB")
	if b.Mark() != 5 {
		t.Errorf("mark after insert before it = %d, want 5", b.Mark())
	}
}

func TestDeleteShiftsMarkBeyondRange(t *testing.T) {
	b := NewWithContent("t", "hello world")
	b.SetMark(9)
	b.Delete(0, 6) // delete "hello "
	if b.Mark() != 3 {
		t.Errorf("mark after delete before it = %d, want 3", b.Mark())
	}
}

func TestDeleteClampsMarkInsideRange(t *testing.T) {
	b := NewWithContent("t", "hello world")
	b.SetMark(3) // inside the deleted range
	b.Delete(1, 5)
	if b.Mark() != 1 {
		t.Errorf("mark inside deleted range = %d, want 1 (clamped to pos)", b.Mark())
	}
}

// ---- Delete edge cases -----------------------------------------------------

func TestDeleteNegativePosClamped(t *testing.T) {
	b := NewWithContent("t", "hello")
	got := b.Delete(-3, 2)
	if got != "he" {
		t.Errorf("Delete(-3,2) = %q, want %q", got, "he")
	}
}

func TestDeletePosBeyondLen(t *testing.T) {
	b := NewWithContent("t", "hello")
	if got := b.Delete(100, 2); got != "" {
		t.Errorf("Delete past end = %q, want empty", got)
	}
}

// ---- Narrow clamping -------------------------------------------------------

func TestNarrowClampsNegativeMin(t *testing.T) {
	b := NewWithContent("t", "0123456789")
	b.Narrow(-5, 4)
	if b.NarrowMin() != 0 {
		t.Errorf("Narrow negative min = %d, want 0", b.NarrowMin())
	}
}

func TestNarrowClampsMaxBeyondLen(t *testing.T) {
	b := NewWithContent("t", "0123456789")
	b.Narrow(2, 100)
	if b.NarrowMax() != b.Len() {
		t.Errorf("Narrow max beyond len = %d, want %d", b.NarrowMax(), b.Len())
	}
}

// ---- LineCount cached path -------------------------------------------------

func TestLineCountCachedSecondCall(t *testing.T) {
	b := NewWithContent("t", "a\nb\nc")
	first := b.LineCount()
	second := b.LineCount() // hits the lineCountReady fast path
	if first != second || second != 3 {
		t.Errorf("LineCount cached = %d/%d, want 3", first, second)
	}
}

// ---- LineCol cache hit + clamp ---------------------------------------------

func TestLineColCacheHit(t *testing.T) {
	b := NewWithContent("t", "ab\ncd")
	l1, c1 := b.LineCol(4)
	l2, c2 := b.LineCol(4) // same gen + pos → cached
	if l1 != l2 || c1 != c2 {
		t.Errorf("LineCol cache mismatch: (%d,%d) vs (%d,%d)", l1, c1, l2, c2)
	}
	if l1 != 2 || c1 != 1 {
		t.Errorf("LineCol(4) = (%d,%d), want (2,1)", l1, c1)
	}
}

func TestLineColClampsPastEnd(t *testing.T) {
	b := NewWithContent("t", "ab\ncd")
	line, col := b.LineCol(b.Len() + 10)
	wantLine, wantCol := b.LineCol(b.Len())
	if line != wantLine || col != wantCol {
		t.Errorf("LineCol past end = (%d,%d), want (%d,%d)", line, col, wantLine, wantCol)
	}
}

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

func TestReplaceStringBasic(t *testing.T) {
	b := NewWithContent("test", "Hello World")
	b.ReplaceString(0, 5, "hello")
	if got := b.String(); got != "hello World" {
		t.Fatalf("after replace: want %q, got %q", "hello World", got)
	}
}
