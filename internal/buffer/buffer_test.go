package buffer

import (
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
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

// lineStartsRef rebuilds the whole line-start index from scratch with a naive
// scan, for cross-checking the incrementally patched one.
func lineStartsRef(b *Buffer) []int {
	out := []int{0}
	for i := range b.Len() {
		if b.RuneAt(i) == '\n' {
			out = append(out, i+1)
		}
	}
	return out
}

// lineColRef is a naive reference implementation of LineCol.
func lineColRef(b *Buffer, pos int) (line, col int) {
	pos = min(max(pos, 0), b.Len())
	line, col = 1, 0
	for i := range pos {
		if b.RuneAt(i) == '\n' {
			line++
			col = 0
		} else {
			col++
		}
	}
	return line, col
}

// checkLineStarts cross-checks the line-start index and everything derived from
// it against naive from-scratch scans: the raw index itself (before anything can
// trigger a rebuild that would paper over a bad patch), the incremental line
// count, LineStart for every line from 1 to LineCount()+2 (i.e. including
// past-EOF lines), LineCol for every position, and the PosForLineCol round trip.
func checkLineStarts(t *testing.T, b *Buffer, what string) {
	t.Helper()
	want := lineStartsRef(b)
	if b.lineStartsReady {
		if !slices.Equal(b.lineStarts, want) {
			t.Errorf("%s: lineStarts = %v, want %v (content %q)", what, b.lineStarts, want, b.String())
		}
		if got := b.lineCountDelta; got != len(want)-1 {
			t.Errorf("%s: lineCountDelta = %d, want %d", what, got, len(want)-1)
		}
	}
	for line := 1; line <= b.LineCount()+2; line++ {
		if got, want := b.LineStart(line), lineStartRef(b, line); got != want {
			t.Errorf("%s: LineStart(%d) = %d, want %d (content %q)", what, line, got, want, b.String())
		}
	}
	for pos := 0; pos <= b.Len(); pos++ {
		line, col := b.LineCol(pos)
		wantLine, wantCol := lineColRef(b, pos)
		if line != wantLine || col != wantCol {
			t.Errorf("%s: LineCol(%d) = (%d,%d), want (%d,%d) (content %q)",
				what, pos, line, col, wantLine, wantCol, b.String())
		}
		if got := b.PosForLineCol(line, col); got != pos {
			t.Errorf("%s: PosForLineCol(%d,%d) = %d, want %d (round trip from pos %d)",
				what, line, col, got, pos, pos)
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

// ---- insert/delete point & mark boundary conditions ------------------------
//
// insertRunes adjusts point and mark with `>= pos`, i.e. text inserted at
// exactly the point (or the mark) pushes it forward.  That is the single most
// executed path in the editor (self-insert types at the point) and flipping
// either comparison to `> pos` is otherwise invisible to the suite: every
// other test inserts strictly before or strictly after point/mark.  The tables
// below therefore hard-code the resulting Point()/Mark() for insertion one
// rune before, exactly at, and one rune after each of them.

func TestInsertAdjustsPointAtBoundaries(t *testing.T) {
	const content = "hello" // 5 runes
	tests := []struct {
		name      string
		point     int
		insertAt  int
		insert    string
		wantText  string
		wantPoint int
	}{
		{"insert one rune before point", 2, 1, "AB", "hABello", 4},
		{"insert exactly at point", 2, 2, "AB", "heABllo", 4},
		{"insert one rune after point", 2, 3, "AB", "helABlo", 2},
		{"insert at point 0", 0, 0, "AB", "ABhello", 2},
		{"insert one rune after point 0", 0, 1, "AB", "hABello", 0},
		{"insert at end with point at end", 5, 5, "AB", "helloAB", 7},
		{"insert before point at end", 5, 4, "AB", "hellABo", 7},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := NewWithContent("t", content)
			b.SetPoint(tc.point)
			if b.Point() != tc.point {
				t.Fatalf("SetPoint(%d) gave Point() = %d", tc.point, b.Point())
			}
			b.InsertString(tc.insertAt, tc.insert)
			mustString(b, tc.wantText, t)
			if got := b.Point(); got != tc.wantPoint {
				t.Errorf("Point() after inserting %q at %d with point %d = %d, want %d",
					tc.insert, tc.insertAt, tc.point, got, tc.wantPoint)
			}
		})
	}
}

func TestInsertAdjustsMarkAtBoundaries(t *testing.T) {
	const content = "hello"
	tests := []struct {
		name     string
		mark     int
		insertAt int
		insert   string
		wantMark int
	}{
		{"insert one rune before mark", 3, 2, "AB", 5},
		{"insert exactly at mark", 3, 3, "AB", 5},
		{"insert one rune after mark", 3, 4, "AB", 3},
		{"insert at mark 0", 0, 0, "AB", 2},
		{"insert one rune after mark 0", 0, 1, "AB", 0},
		{"insert at mark at end", 5, 5, "AB", 7},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := NewWithContent("t", content)
			b.SetMark(tc.mark)
			b.InsertString(tc.insertAt, tc.insert)
			if got := b.Mark(); got != tc.wantMark {
				t.Errorf("Mark() after inserting %q at %d with mark %d = %d, want %d",
					tc.insert, tc.insertAt, tc.mark, got, tc.wantMark)
			}
		})
	}
}

func TestInsertAtPointEqualToMarkMovesBoth(t *testing.T) {
	b := NewWithContent("t", "hello")
	b.SetPoint(2)
	b.SetMark(2)
	b.InsertString(2, "XYZ")
	mustString(b, "heXYZllo", t)
	if b.Point() != 5 {
		t.Errorf("Point() = %d, want 5 (point rides over text inserted at it)", b.Point())
	}
	if b.Mark() != 5 {
		t.Errorf("Mark() = %d, want 5 (mark rides over text inserted at it)", b.Mark())
	}
	// The region between point and mark stays empty, so a region command
	// operates on nothing rather than on the freshly typed text.
	if b.Point() != b.Mark() {
		t.Errorf("point (%d) and mark (%d) should stay equal", b.Point(), b.Mark())
	}
}

func TestInsertLeavesUnsetMarkAlone(t *testing.T) {
	b := NewWithContent("t", "hello")
	if b.Mark() != -1 {
		t.Fatalf("fresh buffer Mark() = %d, want -1", b.Mark())
	}
	b.InsertString(0, "AB")
	if b.Mark() != -1 {
		t.Errorf("Mark() = %d after insert at 0, want -1 (unset mark must not move)", b.Mark())
	}
}

// TestDeleteAdjustsPointAndMarkAtBoundaries pins point/mark for a deletion of
// [pos, pos+count) with point/mark before, at pos, inside, at pos+count and
// after.
//
// Note: unlike insertRunes, the boundary comparisons in deleteRunes cannot be
// pinned by any test.  Flipping `b.point > pos+count` to `>=` (or `b.point >
// pos` to `>=`) is semantically *equivalent*: at point == pos+count both
// branches produce pos (pos+count-count), and at point == pos both leave the
// point at pos.  Those mutants are therefore untestable rather than untested.
func TestDeleteAdjustsPointAndMarkAtBoundaries(t *testing.T) {
	const content = "hello world" // 11 runes
	// Delete 5 runes starting at 3: "lo wo" -> "helrld"
	const pos, count = 3, 5
	tests := []struct {
		name string
		at   int
		want int
	}{
		{"before deleted range", 1, 1},
		{"exactly at pos", pos, pos},
		{"inside deleted range", pos + 2, pos},
		{"at last rune of deleted range", pos + count - 1, pos},
		{"exactly at pos+count", pos + count, pos},
		{"one rune past deleted range", pos + count + 1, pos + 1},
		{"at end of buffer", 11, 11 - count},
	}
	for _, tc := range tests {
		t.Run(tc.name+" (point)", func(t *testing.T) {
			b := NewWithContent("t", content)
			b.SetPoint(tc.at)
			b.Delete(pos, count)
			mustString(b, "helrld", t)
			if got := b.Point(); got != tc.want {
				t.Errorf("Point() with point %d after Delete(%d,%d) = %d, want %d",
					tc.at, pos, count, got, tc.want)
			}
		})
		t.Run(tc.name+" (mark)", func(t *testing.T) {
			b := NewWithContent("t", content)
			b.SetMark(tc.at)
			b.Delete(pos, count)
			if got := b.Mark(); got != tc.want {
				t.Errorf("Mark() with mark %d after Delete(%d,%d) = %d, want %d",
					tc.at, pos, count, got, tc.want)
			}
		})
	}
}

func TestDeleteLeavesUnsetMarkAlone(t *testing.T) {
	b := NewWithContent("t", "hello world")
	b.Delete(0, 6)
	if b.Mark() != -1 {
		t.Errorf("Mark() = %d after delete, want -1 (unset mark must not move)", b.Mark())
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

// ---- performance benchmarks -------------------------------------------------

// benchSizes are the buffer sizes every performance benchmark is measured at.
var benchSizes = []int{1000, 10000, 50000}

// BenchmarkLineCol measures the uncached LineCol cost — the price the modeline
// pays on the first call of a frame — for a position half way through and at
// the very end of the buffer.
func BenchmarkLineCol(b *testing.B) {
	for _, lines := range benchSizes {
		buf := NewWithContent("bench", benchContent(lines))
		for _, frac := range []struct {
			name string
			num  int
		}{{"50%", 1}, {"100%", 2}} {
			pos := buf.Len() * frac.num / 2
			b.Run(sizeName(lines)+"/pos"+frac.name, func(b *testing.B) {
				for b.Loop() {
					buf.lcacheValid = false // defeat the (changeGen,pos) memo
					_, _ = buf.LineCol(pos)
				}
			})
		}
	}
}

// BenchmarkLineColCached measures the memoised path, which must stay O(1).
func BenchmarkLineColCached(b *testing.B) {
	buf := NewWithContent("bench", benchContent(50000))
	pos := buf.Len() / 2
	for b.Loop() {
		_, _ = buf.LineCol(pos)
	}
}

// BenchmarkPosForLineCol measures the inverse mapping for the last line.
func BenchmarkPosForLineCol(b *testing.B) {
	for _, lines := range benchSizes {
		buf := NewWithContent("bench", benchContent(lines))
		last := buf.LineCount()
		b.Run(sizeName(lines), func(b *testing.B) {
			for b.Loop() {
				_ = buf.PosForLineCol(last, 10)
			}
		})
	}
}

// BenchmarkEnsureLineStartsRebuild measures a full index rebuild from scratch —
// the cost that a dropped index imposes on the next LineStart() call.
func BenchmarkEnsureLineStartsRebuild(b *testing.B) {
	for _, lines := range benchSizes {
		buf := NewWithContent("bench", benchContent(lines))
		b.Run(sizeName(lines), func(b *testing.B) {
			for b.Loop() {
				buf.invalidateLineStarts()
				buf.ensureLineStarts()
			}
		})
	}
}

// BenchmarkInsertMidThenLineStart measures typing in the middle of a buffer
// followed by a line-start query — the pattern behind every keystroke.
func BenchmarkInsertMidThenLineStart(b *testing.B) {
	for _, lines := range benchSizes {
		buf := NewWithContent("bench", benchContent(lines))
		mid := buf.Len() / 2
		last := buf.LineCount()
		b.Run(sizeName(lines), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				buf.Insert(mid, 'x')
				_ = buf.LineStart(last)
			}
		})
	}
}

// BenchmarkDeleteMidThenLineStart is the deletion counterpart.
func BenchmarkDeleteMidThenLineStart(b *testing.B) {
	for _, lines := range benchSizes {
		buf := NewWithContent("bench", benchContent(lines))
		last := buf.LineCount()
		b.Run(sizeName(lines), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				mid := buf.Len() / 2
				buf.Insert(mid, 'x')
				buf.Delete(mid, 1)
				_ = buf.LineStart(last)
			}
		})
	}
}

// BenchmarkStringRunes measures the UTF-8 round trip callers currently pay when
// they need the buffer as runes.
func BenchmarkStringRunes(b *testing.B) {
	for _, lines := range benchSizes {
		buf := NewWithContent("bench", benchContent(lines))
		b.Run(sizeName(lines), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = []rune(buf.String())
			}
		})
	}
}

// sizeName labels a benchmark sub-test with its buffer size in lines.
func sizeName(lines int) string {
	switch {
	case lines >= 1000:
		return strconv.Itoa(lines/1000) + "k"
	default:
		return strconv.Itoa(lines)
	}
}

// ---- line-start index: in-place patching ------------------------------------

// TestLineStartIndexPatchedNotRebuilt pins down the point of the patching path:
// a mid-buffer edit must leave the index valid instead of dropping it, so the
// next LineStart() does not pay for a full rebuild.
func TestLineStartIndexPatchedNotRebuilt(t *testing.T) {
	b := NewWithContent("test", "aaa\nbbb\nccc\nddd\n")
	_ = b.LineStart(2) // build the index
	if !b.lineStartsReady {
		t.Fatal("index should be ready after LineStart")
	}

	steps := []struct {
		what string
		edit func()
	}{
		{"insert mid-line", func() { b.Insert(5, 'x') }},
		{"insert newline mid-buffer", func() { b.InsertString(6, "\n") }},
		{"insert multi-line mid-buffer", func() { b.InsertString(2, "1\n2\n3") }},
		{"insert at position 0", func() { b.InsertString(0, "Z\n") }},
		{"delete mid-line", func() { b.Delete(3, 1) }},
		{"delete spanning lines", func() { b.Delete(1, 6) }},
		{"replace mid-buffer", func() { b.ReplaceString(2, 3, "q\nr") }},
		{"undo", func() { b.ApplyUndo() }},
		{"redo", func() { b.ApplyRedo() }},
	}
	for _, s := range steps {
		s.edit()
		if !b.lineStartsReady {
			t.Errorf("%s: index was dropped, want it patched in place", s.what)
			_ = b.LineStart(1) // rebuild so the remaining steps stay meaningful
		}
		checkLineStarts(t, b, s.what)
	}
}

// TestLineStartIndexPatchedAcrossEditKinds walks every mutation shape the buffer
// supports and cross-checks the whole index after each one.
func TestLineStartIndexPatchedAcrossEditKinds(t *testing.T) {
	t.Run("insert at start", func(t *testing.T) {
		b := NewWithContent("test", "abc\nde\nfghi")
		checkLineStarts(t, b, "initial")
		b.InsertString(0, "Z\nY\n")
		checkLineStarts(t, b, "after insert at start")
	})

	t.Run("multi-line insert in middle", func(t *testing.T) {
		b := NewWithContent("test", "abc\nde\nfghi")
		checkLineStarts(t, b, "initial")
		b.InsertString(5, "\n\n\nQ\n")
		checkLineStarts(t, b, "after multi-line insert")
	})

	t.Run("insert consecutive newlines at end", func(t *testing.T) {
		b := NewWithContent("test", "abc")
		checkLineStarts(t, b, "initial")
		b.InsertString(b.Len(), "\n\n\n")
		checkLineStarts(t, b, "after newline run")
	})

	t.Run("delete spanning several lines", func(t *testing.T) {
		b := NewWithContent("test", "a\nb\nc\nd\ne\nf\n")
		checkLineStarts(t, b, "initial")
		b.Delete(2, 6) // removes "b\nc\nd\n"
		checkLineStarts(t, b, "after multi-line delete")
	})

	t.Run("delete exactly one newline", func(t *testing.T) {
		b := NewWithContent("test", "a\nb\nc")
		checkLineStarts(t, b, "initial")
		b.Delete(1, 1) // just the first '\n'
		checkLineStarts(t, b, "after newline delete")
	})

	t.Run("delete everything then refill", func(t *testing.T) {
		b := NewWithContent("test", "a\nb\nc\n")
		checkLineStarts(t, b, "initial")
		b.Delete(0, b.Len())
		checkLineStarts(t, b, "after delete all")
		b.InsertString(0, "x\ny\n")
		checkLineStarts(t, b, "after refill")
	})

	t.Run("edits either side of the gap", func(t *testing.T) {
		b := NewWithContent("test", "aaa\nbbb\nccc\nddd\neee\n")
		checkLineStarts(t, b, "initial")
		b.InsertString(10, "\nmid") // leaves the gap around position 14
		checkLineStarts(t, b, "after edit leaving gap mid-buffer")
		b.InsertString(2, "\nbefore") // insert before the gap
		checkLineStarts(t, b, "after insert before gap")
		b.InsertString(b.Len()-2, "after\n") // insert after the gap
		checkLineStarts(t, b, "after insert after gap")
		b.Delete(1, 5) // delete before the gap
		checkLineStarts(t, b, "after delete before gap")
	})

	t.Run("replace shorter and longer", func(t *testing.T) {
		b := NewWithContent("test", "one\ntwo\nthree\n")
		checkLineStarts(t, b, "initial")
		b.ReplaceString(4, 3, "x") // shorter, no newlines
		checkLineStarts(t, b, "after shorter replace")
		b.ReplaceString(4, 1, "a\nb\nc\n") // longer, adds newlines
		checkLineStarts(t, b, "after longer replace")
		b.ReplaceString(0, b.Len(), "single line") // collapses the buffer
		checkLineStarts(t, b, "after whole-buffer replace")
	})

	t.Run("undo and redo a multi-line insert", func(t *testing.T) {
		b := NewWithContent("test", "abc\nde")
		checkLineStarts(t, b, "initial")
		b.InsertString(2, "1\n2\n3")
		checkLineStarts(t, b, "after insert")
		b.ApplyUndo()
		checkLineStarts(t, b, "after undo")
		b.ApplyRedo()
		checkLineStarts(t, b, "after redo")
		b.ApplyUndo()
		checkLineStarts(t, b, "after second undo")
	})

	t.Run("undo and redo a multi-line delete", func(t *testing.T) {
		b := NewWithContent("test", "a\nb\nc\nd\n")
		checkLineStarts(t, b, "initial")
		b.Delete(2, 4)
		checkLineStarts(t, b, "after delete")
		b.ApplyUndo()
		checkLineStarts(t, b, "after undo")
		b.ApplyRedo()
		checkLineStarts(t, b, "after redo")
	})

	t.Run("undo and redo a replace", func(t *testing.T) {
		b := NewWithContent("test", "a\nb\nc\n")
		checkLineStarts(t, b, "initial")
		b.ReplaceString(2, 2, "X\nY\nZ\n")
		checkLineStarts(t, b, "after replace")
		b.ApplyUndo()
		checkLineStarts(t, b, "after undo")
		b.ApplyRedo()
		checkLineStarts(t, b, "after redo")
	})

	t.Run("with narrowing active", func(t *testing.T) {
		b := NewWithContent("test", "aaa\nbbb\nccc\nddd\n")
		checkLineStarts(t, b, "initial")
		b.Narrow(4, 12)
		checkLineStarts(t, b, "narrowed")
		b.InsertString(6, "X\nY")
		checkLineStarts(t, b, "narrowed after insert")
		b.Delete(5, 3)
		checkLineStarts(t, b, "narrowed after delete")
		b.Widen()
		checkLineStarts(t, b, "after widen")
	})

	t.Run("unicode content", func(t *testing.T) {
		b := NewWithContent("test", "héllo\nwörld\n日本語\n")
		checkLineStarts(t, b, "initial")
		b.InsertString(3, "æø\nå")
		checkLineStarts(t, b, "after unicode insert")
		b.Delete(2, 5)
		checkLineStarts(t, b, "after unicode delete")
	})
}

// TestLineStartIndexRandomEdits hammers the patching paths with a deterministic
// pseudo-random edit stream, cross-checking the index after every mutation.
func TestLineStartIndexRandomEdits(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	b := NewWithContent("test", "alpha\nbeta\ngamma\ndelta\n")
	_ = b.LineStart(2) // build the index (line 1 short-circuits without it)
	inserts := []string{"x", "\n", "a\nb", "\n\n", "long piece of text", "q\nr\ns\n", "é\n"}
	for step := range 300 {
		switch rng.IntN(4) {
		case 0:
			b.InsertString(rng.IntN(b.Len()+1), inserts[rng.IntN(len(inserts))])
		case 1:
			if b.Len() > 0 {
				pos := rng.IntN(b.Len())
				b.Delete(pos, 1+rng.IntN(b.Len()-pos))
			}
		case 2:
			if b.Len() > 0 {
				pos := rng.IntN(b.Len())
				b.ReplaceString(pos, 1+rng.IntN(b.Len()-pos), inserts[rng.IntN(len(inserts))])
			}
		case 3:
			if rng.IntN(2) == 0 {
				b.ApplyUndo()
			} else {
				b.ApplyRedo()
			}
		}
		if !b.lineStartsReady {
			t.Fatalf("step %d: index was dropped", step)
		}
		if want := lineStartsRef(b); !slices.Equal(b.lineStarts, want) {
			t.Fatalf("step %d: lineStarts = %v, want %v (content %q)", step, b.lineStarts, want, b.String())
		}
	}
	checkLineStarts(t, b, "after random edit stream")
}

// ---- LineCol edge cases -----------------------------------------------------

func TestLineColEdgeCases(t *testing.T) {
	t.Run("empty buffer", func(t *testing.T) {
		b := New("test")
		if line, col := b.LineCol(0); line != 1 || col != 0 {
			t.Errorf("LineCol(0) on empty buffer = (%d,%d), want (1,0)", line, col)
		}
		if line, col := b.LineCol(5); line != 1 || col != 0 {
			t.Errorf("LineCol(5) on empty buffer = (%d,%d), want (1,0)", line, col)
		}
	})

	t.Run("negative pos clamps to start", func(t *testing.T) {
		b := NewWithContent("test", "ab\ncd")
		for _, pos := range []int{-1, -100} {
			if line, col := b.LineCol(pos); line != 1 || col != 0 {
				t.Errorf("LineCol(%d) = (%d,%d), want (1,0)", pos, line, col)
			}
		}
	})

	t.Run("trailing newline", func(t *testing.T) {
		b := NewWithContent("test", "ab\ncd\n")
		// Len() sits on the empty last line.
		if line, col := b.LineCol(b.Len()); line != 3 || col != 0 {
			t.Errorf("LineCol(Len) = (%d,%d), want (3,0)", line, col)
		}
	})

	t.Run("no trailing newline", func(t *testing.T) {
		b := NewWithContent("test", "ab\ncd")
		if line, col := b.LineCol(b.Len()); line != 2 || col != 2 {
			t.Errorf("LineCol(Len) = (%d,%d), want (2,2)", line, col)
		}
	})

	t.Run("position on a newline", func(t *testing.T) {
		b := NewWithContent("test", "abc\nde")
		// The '\n' at position 3 belongs to line 1 as its last column.
		if line, col := b.LineCol(3); line != 1 || col != 3 {
			t.Errorf("LineCol(3) = (%d,%d), want (1,3)", line, col)
		}
		if line, col := b.LineCol(4); line != 2 || col != 0 {
			t.Errorf("LineCol(4) = (%d,%d), want (2,0)", line, col)
		}
	})

	t.Run("consecutive newlines", func(t *testing.T) {
		b := NewWithContent("test", "a\n\n\nb")
		for pos, want := range map[int][2]int{0: {1, 0}, 1: {1, 1}, 2: {2, 0}, 3: {3, 0}, 4: {4, 0}, 5: {4, 1}} {
			if line, col := b.LineCol(pos); line != want[0] || col != want[1] {
				t.Errorf("LineCol(%d) = (%d,%d), want (%d,%d)", pos, line, col, want[0], want[1])
			}
		}
	})

	t.Run("spanning the gap", func(t *testing.T) {
		b := NewWithContent("test", "aaa\nbbb\nccc\nddd")
		b.Insert(5, 'x') // leaves the gap at position 6
		checkLineStarts(t, b, "gap mid-buffer")
	})
}

// TestLineColIgnoresNarrowing documents that LineCol, like LineStart, reports
// absolute positions unaffected by narrowing.
func TestLineColIgnoresNarrowing(t *testing.T) {
	b := NewWithContent("test", "abc\nde\nfghi")
	wantLine, wantCol := b.LineCol(8)
	if wantLine != 3 || wantCol != 1 {
		t.Fatalf("LineCol(8) = (%d,%d), want (3,1)", wantLine, wantCol)
	}
	b.Narrow(4, 6)
	if line, col := b.LineCol(8); line != wantLine || col != wantCol {
		t.Errorf("LineCol(8) while narrowed = (%d,%d), want (%d,%d)", line, col, wantLine, wantCol)
	}
	b.Widen()
	if line, col := b.LineCol(8); line != wantLine || col != wantCol {
		t.Errorf("LineCol(8) after widen = (%d,%d), want (%d,%d)", line, col, wantLine, wantCol)
	}
}

// TestLineColCacheInvalidatedByEdit makes sure the memo keys off changeGen, so
// an edit is never served a stale line/column.
func TestLineColCacheInvalidatedByEdit(t *testing.T) {
	b := NewWithContent("test", "ab\ncd")
	if line, col := b.LineCol(4); line != 2 || col != 1 {
		t.Fatalf("LineCol(4) = (%d,%d), want (2,1)", line, col)
	}
	b.InsertString(0, "\n") // pushes everything down one line
	if line, col := b.LineCol(4); line != 3 || col != 0 {
		t.Errorf("LineCol(4) after insert = (%d,%d), want (3,0)", line, col)
	}
}

// TestLineColSeedsLineStarts verifies LineCol builds the shared index, so the
// first modeline draw pays for it once and LineStart() is free afterwards.
func TestLineColSeedsLineStarts(t *testing.T) {
	b := NewWithContent("test", "a\nb\nc")
	if b.lineStartsReady {
		t.Fatal("index should start invalid")
	}
	_, _ = b.LineCol(2)
	if !b.lineStartsReady {
		t.Error("LineCol should build the line-start index")
	}
	if got := b.LineCount(); got != 3 {
		t.Errorf("LineCount = %d, want 3", got)
	}
}

// ---- PosForLineCol edge cases ----------------------------------------------

func TestPosForLineColEdgeCases(t *testing.T) {
	b := NewWithContent("test", "hello\nworld\nfoo")
	tests := []struct {
		line, col, want int
		what            string
	}{
		{0, 2, 2, "line 0 clamps to line 1"},
		{-5, 0, 0, "negative line clamps to line 1"},
		{4, 0, b.Len(), "line past EOF collapses to Len"},
		{99, 7, b.Len(), "far past EOF collapses to Len"},
		{3, 99, b.Len(), "col past end of last line clamps to Len"},
		{2, 5, 11, "col at the newline of its line"},
		{2, 6, 11, "col past the newline clamps to it"},
	}
	for _, tc := range tests {
		if got := b.PosForLineCol(tc.line, tc.col); got != tc.want {
			t.Errorf("%s: PosForLineCol(%d,%d) = %d, want %d", tc.what, tc.line, tc.col, got, tc.want)
		}
	}

	t.Run("empty buffer", func(t *testing.T) {
		e := New("test")
		for _, tc := range []struct{ line, col int }{{1, 0}, {1, 5}, {2, 0}, {0, 0}} {
			if got := e.PosForLineCol(tc.line, tc.col); got != 0 {
				t.Errorf("PosForLineCol(%d,%d) on empty buffer = %d, want 0", tc.line, tc.col, got)
			}
		}
	})

	t.Run("empty line between newlines", func(t *testing.T) {
		e := NewWithContent("test", "a\n\nb")
		if got := e.PosForLineCol(2, 3); got != 2 {
			t.Errorf("PosForLineCol(2,3) on empty line = %d, want 2", got)
		}
	})
}

// TestPosForLineColRoundTripsLineCol checks the two directions agree for every
// position in a buffer with mixed line shapes.
func TestPosForLineColRoundTripsLineCol(t *testing.T) {
	b := NewWithContent("test", "abc\n\ndefgh\n\n\nij")
	for pos := 0; pos <= b.Len(); pos++ {
		line, col := b.LineCol(pos)
		if got := b.PosForLineCol(line, col); got != pos {
			t.Errorf("round trip pos %d → (%d,%d) → %d", pos, line, col, got)
		}
	}
}

// ---- AppendRunes ------------------------------------------------------------

func TestAppendRunes(t *testing.T) {
	t.Run("empty buffer", func(t *testing.T) {
		b := New("test")
		if got := b.AppendRunes(nil); len(got) != 0 {
			t.Errorf("AppendRunes(nil) on empty buffer = %v, want empty", got)
		}
	})

	t.Run("nil destination", func(t *testing.T) {
		b := NewWithContent("test", "hello\nworld")
		if got := string(b.AppendRunes(nil)); got != "hello\nworld" {
			t.Errorf("AppendRunes(nil) = %q, want %q", got, "hello\nworld")
		}
	})

	t.Run("appends to existing content", func(t *testing.T) {
		b := NewWithContent("test", "world")
		got := string(b.AppendRunes([]rune("hello ")))
		if got != "hello world" {
			t.Errorf("AppendRunes = %q, want %q", got, "hello world")
		}
	})

	t.Run("spans the gap", func(t *testing.T) {
		b := NewWithContent("test", "Hello World")
		b.Insert(5, ',') // leaves the gap mid-buffer
		if got := string(b.AppendRunes(nil)); got != "Hello, World" {
			t.Errorf("AppendRunes across gap = %q, want %q", got, "Hello, World")
		}
	})

	t.Run("gap at the start", func(t *testing.T) {
		b := NewWithContent("test", "abcdef")
		b.Delete(0, 1) // gap sits at position 0
		if got := string(b.AppendRunes(nil)); got != "bcdef" {
			t.Errorf("AppendRunes with leading gap = %q, want %q", got, "bcdef")
		}
	})

	t.Run("unicode", func(t *testing.T) {
		const content = "héllo\n日本語\næøå"
		b := NewWithContent("test", content)
		got := b.AppendRunes(nil)
		if want := []rune(content); !slices.Equal(got, want) {
			t.Errorf("AppendRunes unicode = %v, want %v", got, want)
		}
		if len(got) != len([]rune(content)) {
			t.Errorf("AppendRunes returned %d runes, want %d", len(got), len([]rune(content)))
		}
	})

	t.Run("matches String", func(t *testing.T) {
		b := NewWithContent("test", "one\ntwo\nthree")
		b.InsertString(4, "X\nY")
		b.Delete(1, 2)
		if got, want := string(b.AppendRunes(nil)), b.String(); got != want {
			t.Errorf("AppendRunes = %q, String = %q", got, want)
		}
	})

	t.Run("ignores narrowing like String", func(t *testing.T) {
		b := NewWithContent("test", "0123456789")
		b.Narrow(3, 7)
		if got, want := string(b.AppendRunes(nil)), b.String(); got != want {
			t.Errorf("AppendRunes while narrowed = %q, want %q (same as String)", got, want)
		}
	})

	t.Run("reuses spare capacity", func(t *testing.T) {
		b := NewWithContent("test", "hello world")
		scratch := make([]rune, 0, 64)
		got := b.AppendRunes(scratch)
		if &got[:1][0] != &scratch[:1:1][0] {
			t.Error("AppendRunes reallocated despite sufficient capacity")
		}
		// A second pass over the reset slice must produce the same content.
		again := b.AppendRunes(got[:0])
		if string(again) != b.String() {
			t.Errorf("reused AppendRunes = %q, want %q", string(again), b.String())
		}
	})

	t.Run("grows an undersized destination", func(t *testing.T) {
		b := NewWithContent("test", strings.Repeat("ab\n", 50))
		dst := make([]rune, 0, 1)
		got := b.AppendRunes(dst)
		if string(got) != b.String() {
			t.Errorf("AppendRunes into small slice = %q, want %q", string(got), b.String())
		}
	})
}

// ---- AppendRunes benchmark --------------------------------------------------

// BenchmarkAppendRunes measures the direct rune copy that replaces the
// String()+[]rune() round trip, reusing a scratch slice across iterations.
func BenchmarkAppendRunes(b *testing.B) {
	for _, lines := range benchSizes {
		buf := NewWithContent("bench", benchContent(lines))
		b.Run(sizeName(lines), func(b *testing.B) {
			b.ReportAllocs()
			scratch := make([]rune, 0, buf.Len())
			for b.Loop() {
				scratch = buf.AppendRunes(scratch[:0])
			}
			_ = scratch
		})
	}
}

// BenchmarkLineStartsPatch isolates the cost of keeping the line-start index in
// step with a mid-buffer edit: a binary search plus a tail shift, with no
// allocation and no rebuild.  The insert/delete pair is self-cancelling, so the
// index stays correct across iterations.
func BenchmarkLineStartsPatch(b *testing.B) {
	for _, lines := range benchSizes {
		buf := NewWithContent("bench", benchContent(lines))
		buf.ensureLineStarts()
		mid := buf.Len() / 2
		one := []rune{'x'}
		b.Run(sizeName(lines), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				buf.insertLineStarts(mid, one)
				buf.deleteLineStarts(mid, 1)
			}
		})
		if want := lineStartsRef(buf); !slices.Equal(buf.lineStarts, want) {
			b.Fatalf("index drifted during benchmark at %d lines", lines)
		}
	}
}

// BenchmarkEndOfLineLongLine measures the boundary lookups on a buffer whose
// lines are pathologically long (minified JSON/JS, long log lines), the worst
// case for a rune-by-rune scan.  Each iteration probes a different position so
// the LineCol cache cannot answer every call.
func BenchmarkEndOfLineLongLine(b *testing.B) {
	const lineLen = 200000
	long := strings.Repeat("x", lineLen)
	buf := NewWithContent("bench", "short\n"+long+"\n"+long)
	mid := make([]int, 64)
	for i := range mid {
		mid[i] = 6 + lineLen/2 + i
	}
	last := make([]int, 64)
	for i := range last {
		last[i] = buf.Len() - lineLen/2 + i
	}
	run := func(name string, probes []int, fn func(int) int) {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				_ = fn(probes[i%len(probes)])
				i++
			}
		})
	}
	run("EndOfLine/mid", mid, buf.EndOfLine)
	run("EndOfLine/lastLine", last, buf.EndOfLine)
	run("BeginningOfLine/mid", mid, buf.BeginningOfLine)
	// The scan the index lookups replace, for comparison.
	run("scan/EndOfLine/mid", mid, func(pos int) int { return endOfLineRef(buf, pos) })
	run("scan/BeginningOfLine/mid", mid, func(pos int) int { return beginningOfLineRef(buf, pos) })
}

// ---- line boundaries: equivalence with a brute-force scan -------------------

// beginningOfLineRef is the naive backwards-scan reference implementation that
// BeginningOfLine's line-start-index lookup has to agree with exactly.
func beginningOfLineRef(b *Buffer, pos int) int {
	if pos > b.Len() {
		pos = b.Len()
	}
	for i := pos - 1; i >= 0; i-- {
		if b.RuneAt(i) == '\n' {
			return i + 1
		}
	}
	return 0
}

// endOfLineRef is the naive forwards-scan reference implementation for
// EndOfLine.
func endOfLineRef(b *Buffer, pos int) int {
	n := b.Len()
	if pos > n {
		pos = n
	}
	for i := pos; i < n; i++ {
		if b.RuneAt(i) == '\n' {
			return i
		}
	}
	return n
}

// lineBoundaryCorpus returns buffers covering the line shapes and gap positions
// the boundary helpers have to cope with.
func lineBoundaryCorpus() []struct {
	name string
	b    *Buffer
} {
	huge := strings.Repeat("x", 5000)

	gapMid := NewWithContent("test", "one\ntwo\nthree\nfour")
	gapMid.InsertString(8, "XY\nZ") // leaves the gap mid-buffer

	gapStart := NewWithContent("test", "alpha\nbeta\ngamma")
	gapStart.Delete(0, 1) // gap sits at position 0

	gapEnd := NewWithContent("test", "alpha\nbeta\n")
	gapEnd.InsertString(gapEnd.Len(), "gamma") // gap at the very end

	narrowed := NewWithContent("test", "abc\ndefgh\nij\n\nk")
	narrowed.Narrow(5, 11)

	narrowedEmpty := NewWithContent("test", "abc\ndef\n")
	narrowedEmpty.Narrow(4, 4)

	narrowedEdited := NewWithContent("test", "aa\nbb\ncc\ndd\n")
	narrowedEdited.InsertString(6, "zz\n")
	narrowedEdited.Narrow(3, 9)

	return []struct {
		name string
		b    *Buffer
	}{
		{"empty", New("test")},
		{"single newline", NewWithContent("test", "\n")},
		{"no trailing newline", NewWithContent("test", "abc\ndef")},
		{"trailing newline", NewWithContent("test", "abc\ndef\n")},
		{"leading newline", NewWithContent("test", "\nabc\ndef")},
		{"consecutive newlines", NewWithContent("test", "a\n\n\nb\n\n")},
		{"only newlines", NewWithContent("test", "\n\n\n\n")},
		{"crlf", NewWithContent("test", "a\r\nb\r\n\r\nc")},
		{"multibyte", NewWithContent("test", "héllo\n日本語テキスト\n\næøå\nend")},
		{"no newline at all", NewWithContent("test", "just one line")},
		{"enormous single line", NewWithContent("test", huge)},
		{"enormous line among others", NewWithContent("test", "head\n"+huge+"\ntail")},
		{"gap mid-buffer", gapMid},
		{"gap at start", gapStart},
		{"gap at end", gapEnd},
		{"narrowed", narrowed},
		{"narrowed to empty region", narrowedEmpty},
		{"narrowed after edits", narrowedEdited},
	}
}

// boundaryProbes returns the positions to test: every position for small
// buffers, a sampled set (plus every line boundary) for large ones, and
// out-of-range values in both directions.
func boundaryProbes(b *Buffer) []int {
	n := b.Len()
	probes := []int{-100, -1, n, n + 1, n + 100}
	if n <= 2000 {
		for pos := 0; pos <= n; pos++ {
			probes = append(probes, pos)
		}
		return probes
	}
	for pos := 0; pos <= n; pos += 97 {
		probes = append(probes, pos)
	}
	for i := range b.Len() {
		if b.RuneAt(i) == '\n' {
			probes = append(probes, i-1, i, i+1)
		}
	}
	return probes
}

// TestBeginningOfLineMatchesBruteForce pins the index-based lookup to the naive
// backwards scan across the whole corpus, including a narrowed buffer (both are
// absolute: narrowing must not shift the answer).
func TestBeginningOfLineMatchesBruteForce(t *testing.T) {
	for _, tc := range lineBoundaryCorpus() {
		t.Run(tc.name, func(t *testing.T) {
			for _, pos := range boundaryProbes(tc.b) {
				want := beginningOfLineRef(tc.b, pos)
				if got := tc.b.BeginningOfLine(pos); got != want {
					t.Fatalf("BeginningOfLine(%d) = %d, want %d", pos, got, want)
				}
			}
		})
	}
}

// TestEndOfLineMatchesBruteForce is TestBeginningOfLineMatchesBruteForce for the
// forward direction.
func TestEndOfLineMatchesBruteForce(t *testing.T) {
	for _, tc := range lineBoundaryCorpus() {
		t.Run(tc.name, func(t *testing.T) {
			for _, pos := range boundaryProbes(tc.b) {
				want := endOfLineRef(tc.b, pos)
				if got := tc.b.EndOfLine(pos); got != want {
					t.Fatalf("EndOfLine(%d) = %d, want %d", pos, got, want)
				}
			}
		})
	}
}

// TestLineBoundariesStayCorrectAcrossEdits checks the boundary helpers still
// match the brute-force scan after the edits that patch the line-start index in
// place and move the gap around.
func TestLineBoundariesStayCorrectAcrossEdits(t *testing.T) {
	b := NewWithContent("test", "one\ntwo\nthree\nfour\n")
	edits := []func(){
		func() { b.InsertString(0, "zero\n") },
		func() { b.InsertString(b.Len(), "five") },
		func() { b.InsertString(6, "\n\n") },
		func() { b.Delete(3, 4) },
		func() { b.Delete(0, 2) },
		func() { b.InsertString(b.Len()/2, "mid\nline") },
		func() { b.Delete(b.Len()-1, 1) },
	}
	for i, edit := range edits {
		edit()
		for pos := -1; pos <= b.Len()+1; pos++ {
			if got, want := b.BeginningOfLine(pos), beginningOfLineRef(b, pos); got != want {
				t.Fatalf("edit %d: BeginningOfLine(%d) = %d, want %d (buffer %q)", i, pos, got, want, b.String())
			}
			if got, want := b.EndOfLine(pos), endOfLineRef(b, pos); got != want {
				t.Fatalf("edit %d: EndOfLine(%d) = %d, want %d (buffer %q)", i, pos, got, want, b.String())
			}
		}
	}
}

// TestLineBoundariesAgreeWithLineStart checks the helpers stay consistent with
// the index API they are now built on.
func TestLineBoundariesAgreeWithLineStart(t *testing.T) {
	b := NewWithContent("test", "abc\n\ndefgh\nij\n")
	for pos := 0; pos <= b.Len(); pos++ {
		line, _ := b.LineCol(pos)
		if got, want := b.BeginningOfLine(pos), b.LineStart(line); got != want {
			t.Errorf("BeginningOfLine(%d) = %d, LineStart(%d) = %d", pos, got, line, want)
		}
		if eol := b.EndOfLine(pos); eol < b.BeginningOfLine(pos) {
			t.Errorf("EndOfLine(%d) = %d is before BeginningOfLine = %d", pos, eol, b.BeginningOfLine(pos))
		}
	}
}

// TestLineBoundariesDoNotMoveGap guards the performance property: the boundary
// lookups are read-only, so they must not shuffle the gap-buffer segments.
func TestLineBoundariesDoNotMoveGap(t *testing.T) {
	b := NewWithContent("test", "one\ntwo\nthree\nfour")
	b.InsertString(8, "XY\n")
	gapStart, gapEnd := b.gapStart, b.gapEnd
	for pos := 0; pos <= b.Len(); pos++ {
		b.BeginningOfLine(pos)
		b.EndOfLine(pos)
	}
	if b.gapStart != gapStart || b.gapEnd != gapEnd {
		t.Errorf("gap moved from [%d,%d) to [%d,%d)", gapStart, gapEnd, b.gapStart, b.gapEnd)
	}
}

// ---- AppendRunesRange -------------------------------------------------------

// appendRunesRangeRef is the naive per-rune reference implementation.
func appendRunesRangeRef(b *Buffer, start, end int) []rune {
	start = max(start, 0)
	end = min(end, b.Len())
	var out []rune
	for i := start; i < end; i++ {
		out = append(out, b.RuneAt(i))
	}
	return out
}

func TestAppendRunesRange(t *testing.T) {
	t.Run("relative to the gap", func(t *testing.T) {
		b := NewWithContent("test", "0123456789")
		b.InsertString(5, "abc") // "01234abc56789", gap sits at 8
		if b.gapStart != 8 {
			t.Fatalf("precondition: gapStart = %d, want 8", b.gapStart)
		}
		cases := []struct {
			name             string
			start, end       int
			want             string
			wantAllocsAtMost int
		}{
			{"entirely before the gap", 1, 5, "1234", 0},
			{"ends exactly at the gap", 0, 8, "01234abc", 0},
			{"entirely after the gap", 9, 13, "6789", 0},
			{"starts exactly at the gap", 8, 10, "56", 0},
			{"spans the gap", 3, 11, "34abc567", 0},
			{"whole buffer", 0, 13, "01234abc56789", 0},
			{"single rune before the gap", 2, 3, "2", 0},
			{"single rune after the gap", 12, 13, "9", 0},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got := string(b.AppendRunesRange(nil, tc.start, tc.end))
				if got != tc.want {
					t.Errorf("AppendRunesRange(nil, %d, %d) = %q, want %q", tc.start, tc.end, got, tc.want)
				}
			})
		}
	})

	t.Run("matches a per-rune scan for every range", func(t *testing.T) {
		for _, tc := range lineBoundaryCorpus() {
			t.Run(tc.name, func(t *testing.T) {
				n := tc.b.Len()
				if n > 200 {
					n = 200 // every (start,end) pair would be quadratic
				}
				for start := 0; start <= n; start++ {
					for end := start; end <= n; end++ {
						want := appendRunesRangeRef(tc.b, start, end)
						got := tc.b.AppendRunesRange(nil, start, end)
						if !slices.Equal(got, want) {
							t.Fatalf("AppendRunesRange(nil, %d, %d) = %q, want %q", start, end, string(got), string(want))
						}
					}
				}
			})
		}
	})

	t.Run("full range equals AppendRunes", func(t *testing.T) {
		for _, tc := range lineBoundaryCorpus() {
			full := string(tc.b.AppendRunesRange(nil, 0, tc.b.Len()))
			if want := string(tc.b.AppendRunes(nil)); full != want {
				t.Errorf("%s: full range = %q, AppendRunes = %q", tc.name, full, want)
			}
			if want := tc.b.String(); full != want {
				t.Errorf("%s: full range = %q, String = %q", tc.name, full, want)
			}
		}
	})

	t.Run("survives inserts and deletes that move the gap", func(t *testing.T) {
		b := NewWithContent("test", "one\ntwo\nthree\nfour\nfive")
		steps := []func(){
			func() { b.InsertString(0, "zero\n") },
			func() { b.Delete(10, 3) },
			func() { b.InsertString(b.Len(), "\nsix") },
			func() { b.InsertString(b.Len()/2, "MID") },
			func() { b.Delete(0, 1) },
		}
		for i, step := range steps {
			step()
			for start := 0; start <= b.Len(); start += 3 {
				for _, end := range []int{start, start + 1, start + 7, b.Len()} {
					want := string(appendRunesRangeRef(b, start, end))
					got := string(b.AppendRunesRange(nil, start, end))
					if got != want {
						t.Fatalf("step %d: range [%d,%d) = %q, want %q", i, start, end, got, want)
					}
				}
			}
		}
	})

	t.Run("empty and degenerate ranges", func(t *testing.T) {
		b := NewWithContent("test", "hello world")
		dst := []rune("keep")
		cases := []struct {
			name       string
			start, end int
		}{
			{"start equals end", 4, 4},
			{"start equals end at zero", 0, 0},
			{"start equals end at Len", b.Len(), b.Len()},
			{"reversed", 8, 3},
			{"both negative", -10, -2},
			{"end below zero", 0, -1},
			{"start past Len", b.Len() + 5, b.Len() + 9},
			{"reversed after clamping", b.Len() + 1, 2},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got := b.AppendRunesRange(dst, tc.start, tc.end)
				if string(got) != "keep" {
					t.Errorf("AppendRunesRange(dst, %d, %d) = %q, want %q (dst untouched)", tc.start, tc.end, string(got), "keep")
				}
			})
		}
	})

	t.Run("clamps out-of-range arguments", func(t *testing.T) {
		b := NewWithContent("test", "abcde")
		if got := string(b.AppendRunesRange(nil, -7, 3)); got != "abc" {
			t.Errorf("negative start = %q, want %q", got, "abc")
		}
		if got := string(b.AppendRunesRange(nil, 2, 99)); got != "cde" {
			t.Errorf("over-range end = %q, want %q", got, "cde")
		}
		if got := string(b.AppendRunesRange(nil, -99, 99)); got != "abcde" {
			t.Errorf("both out of range = %q, want %q", got, "abcde")
		}
	})

	t.Run("appends to existing content", func(t *testing.T) {
		b := NewWithContent("test", "world!")
		got := string(b.AppendRunesRange([]rune("hello "), 0, 5))
		if got != "hello world" {
			t.Errorf("AppendRunesRange = %q, want %q", got, "hello world")
		}
	})

	t.Run("grows an undersized destination", func(t *testing.T) {
		b := NewWithContent("test", strings.Repeat("ab\n", 50))
		dst := make([]rune, 0, 1)
		got := b.AppendRunesRange(dst, 10, 100)
		if want := string(appendRunesRangeRef(b, 10, 100)); string(got) != want {
			t.Errorf("AppendRunesRange into small slice = %q, want %q", string(got), want)
		}
	})

	t.Run("reuses spare capacity", func(t *testing.T) {
		b := NewWithContent("test", "hello world")
		scratch := make([]rune, 0, 64)
		got := b.AppendRunesRange(scratch, 0, 5)
		if &got[:1][0] != &scratch[:1:1][0] {
			t.Error("AppendRunesRange reallocated despite sufficient capacity")
		}
	})

	t.Run("no allocations when capacity suffices", func(t *testing.T) {
		b := NewWithContent("test", strings.Repeat("some line of text\n", 500))
		b.InsertString(b.Len()/2, "gap\nhere\n") // gap mid-buffer: spanning copy
		scratch := make([]rune, 0, b.Len())
		start, end := b.Len()/2-100, b.Len()/2+100
		if allocs := testing.AllocsPerRun(50, func() {
			scratch = b.AppendRunesRange(scratch[:0], start, end)
		}); allocs != 0 {
			t.Errorf("AppendRunesRange allocated %v times per run, want 0", allocs)
		}
		if want := string(appendRunesRangeRef(b, start, end)); string(scratch) != want {
			t.Errorf("after reuse = %q, want %q", string(scratch), want)
		}
		// The full-buffer wrapper must be allocation-free the same way.
		if allocs := testing.AllocsPerRun(50, func() {
			scratch = b.AppendRunes(scratch[:0])
		}); allocs != 0 {
			t.Errorf("AppendRunes allocated %v times per run, want 0", allocs)
		}
	})

	t.Run("ignores narrowing like AppendRunes", func(t *testing.T) {
		b := NewWithContent("test", "0123456789")
		b.Narrow(3, 7)
		// Absolute positions, so a range outside the accessible region still
		// yields its runes.
		if got := string(b.AppendRunesRange(nil, 0, 3)); got != "012" {
			t.Errorf("range before narrowMin = %q, want %q", got, "012")
		}
		if got := string(b.AppendRunesRange(nil, 7, 10)); got != "789" {
			t.Errorf("range after narrowMax = %q, want %q", got, "789")
		}
		if got, want := string(b.AppendRunesRange(nil, 0, b.Len())), b.String(); got != want {
			t.Errorf("full range while narrowed = %q, want %q", got, want)
		}
		if got := string(b.AppendRunesRange(nil, b.NarrowMin(), b.NarrowMax())); got != "3456" {
			t.Errorf("accessible region = %q, want %q", got, "3456")
		}
	})

	t.Run("does not move the gap", func(t *testing.T) {
		b := NewWithContent("test", "one\ntwo\nthree\nfour")
		b.InsertString(8, "XY\n")
		gapStart, gapEnd := b.gapStart, b.gapEnd
		for start := 0; start <= b.Len(); start++ {
			b.AppendRunesRange(nil, start, start+5)
		}
		if b.gapStart != gapStart || b.gapEnd != gapEnd {
			t.Errorf("gap moved from [%d,%d) to [%d,%d)", gapStart, gapEnd, b.gapStart, b.gapEnd)
		}
	})

	t.Run("leaves the buffer unmodified", func(t *testing.T) {
		b := NewWithContent("test", "one\ntwo")
		gen, mods := b.ChangeGen(), b.ModCount()
		b.AppendRunesRange(nil, 0, 4)
		if b.Modified() || b.ChangeGen() != gen || b.ModCount() != mods {
			t.Error("AppendRunesRange marked the buffer as modified")
		}
	})
}

// BenchmarkAppendRunesRange measures the windowed copy a highlighter needs: a
// screenful-sized prefix out of a large buffer, compared against
// BenchmarkAppendRunes which copies the whole thing.
func BenchmarkAppendRunesRange(b *testing.B) {
	for _, lines := range benchSizes {
		buf := NewWithContent("bench", benchContent(lines))
		buf.InsertString(buf.Len()/2, "x") // gap mid-buffer
		window := min(buf.Len(), 50*48)    // ~50 rendered lines
		b.Run(sizeName(lines)+"/prefix", func(b *testing.B) {
			b.ReportAllocs()
			scratch := make([]rune, 0, window)
			for b.Loop() {
				scratch = buf.AppendRunesRange(scratch[:0], 0, window)
			}
			_ = scratch
		})
		b.Run(sizeName(lines)+"/midWindow", func(b *testing.B) {
			b.ReportAllocs()
			scratch := make([]rune, 0, window)
			start := max(buf.Len()/2-window/2, 0)
			for b.Loop() {
				scratch = buf.AppendRunesRange(scratch[:0], start, start+window)
			}
			_ = scratch
		})
	}
}

// ---- AppendBytes / AppendBytesRange -----------------------------------------

// appendBytesCorpus returns the buffers AppendBytes is checked against: the
// shared line-boundary corpus (empty, CRLF, multibyte, huge single line, gap at
// start/middle/end, narrowed, …) plus entries that only matter for UTF-8
// encoding — wide CJK runes, astral-plane emoji, and runes that are not valid
// scalar values at all.
func appendBytesCorpus() []struct {
	name string
	b    *Buffer
} {
	corpus := lineBoundaryCorpus()

	// A lone surrogate and an out-of-range rune can reach the buffer via
	// Insert(), which takes a rune with no validity check.  Both must encode as
	// U+FFFD, exactly as String() does.
	invalid := New("test")
	invalid.InsertString(0, "a")
	invalid.Insert(1, rune(0xD800))   // lone surrogate
	invalid.Insert(2, rune(0x110000)) // above utf8.MaxRune
	invalid.Insert(3, rune(-1))       // negative
	invalid.InsertString(4, "\nb")    //nolint:gocritic // keeps the gap mid-buffer

	edited := NewWithContent("test", "héllo\n日本語\nworld\n")
	edited.InsertString(6, "🎉🚀")
	edited.Delete(2, 3)
	edited.InsertString(edited.Len(), "æøå")

	return append(corpus, []struct {
		name string
		b    *Buffer
	}{
		{"wide cjk", NewWithContent("test", "日本語テキスト\n中文字\nひらがな")},
		{"emoji", NewWithContent("test", "a🎉b\n👨‍👩‍👧‍👦\n🚀🚀🚀")},
		{"combining marks", NewWithContent("test", "éå\nñ")},
		{"invalid runes", invalid},
		{"multibyte after edits", edited},
		{"mixed width huge line", NewWithContent("test", strings.Repeat("aé日🎉", 500))},
	}...)
}

func TestAppendBytes(t *testing.T) {
	t.Run("matches String across the corpus", func(t *testing.T) {
		for _, tc := range appendBytesCorpus() {
			got := string(tc.b.AppendBytes(nil))
			if want := tc.b.String(); got != want {
				t.Errorf("%s: AppendBytes = %q, String = %q", tc.name, got, want)
			}
		}
	})

	t.Run("result is valid utf-8", func(t *testing.T) {
		for _, tc := range appendBytesCorpus() {
			if got := tc.b.AppendBytes(nil); !utf8.Valid(got) {
				t.Errorf("%s: AppendBytes produced invalid UTF-8: %q", tc.name, got)
			}
		}
	})

	t.Run("empty buffer leaves dst untouched", func(t *testing.T) {
		b := New("test")
		if got := b.AppendBytes(nil); got != nil {
			t.Errorf("AppendBytes(nil) on empty buffer = %v, want nil", got)
		}
		if got := string(b.AppendBytes([]byte("keep"))); got != "keep" {
			t.Errorf("AppendBytes on empty buffer = %q, want %q", got, "keep")
		}
	})

	t.Run("appends to existing content", func(t *testing.T) {
		b := NewWithContent("test", "world")
		if got := string(b.AppendBytes([]byte("hello "))); got != "hello world" {
			t.Errorf("AppendBytes = %q, want %q", got, "hello world")
		}
	})

	t.Run("ignores narrowing like String", func(t *testing.T) {
		b := NewWithContent("test", "0123456789")
		b.Narrow(3, 7)
		if got, want := string(b.AppendBytes(nil)), b.String(); got != want {
			t.Errorf("AppendBytes while narrowed = %q, want %q (same as String)", got, want)
		}
		if got := string(b.AppendBytes(nil)); got != "0123456789" {
			t.Errorf("AppendBytes while narrowed = %q, want the whole buffer", got)
		}
	})

	t.Run("survives inserts and deletes that move the gap", func(t *testing.T) {
		b := NewWithContent("test", "one\ntwo\nthree\nfour\nfive")
		steps := []func(){
			func() { b.InsertString(0, "zéro\n") },
			func() { b.Delete(10, 3) },
			func() { b.InsertString(b.Len(), "\n六") },
			func() { b.InsertString(b.Len()/2, "🎉MID") },
			func() { b.Delete(0, 1) },
			func() { b.InsertString(3, "日本") },
		}
		for i, step := range steps {
			step()
			if got, want := string(b.AppendBytes(nil)), b.String(); got != want {
				t.Fatalf("step %d: AppendBytes = %q, String = %q", i, got, want)
			}
		}
	})

	t.Run("reuses spare capacity", func(t *testing.T) {
		b := NewWithContent("test", "hello world")
		scratch := make([]byte, 0, 64)
		got := b.AppendBytes(scratch)
		if &got[:1][0] != &scratch[:1:1][0] {
			t.Error("AppendBytes reallocated despite sufficient capacity")
		}
		// A second pass over the reset slice must produce the same content.
		if again := string(b.AppendBytes(got[:0])); again != b.String() {
			t.Errorf("reused AppendBytes = %q, want %q", again, b.String())
		}
	})

	t.Run("grows an undersized destination", func(t *testing.T) {
		b := NewWithContent("test", strings.Repeat("ab\n", 50))
		got := b.AppendBytes(make([]byte, 0, 1))
		if string(got) != b.String() {
			t.Errorf("AppendBytes into small slice = %q, want %q", string(got), b.String())
		}
	})

	t.Run("no allocations when capacity suffices", func(t *testing.T) {
		b := NewWithContent("test", strings.Repeat("some line of text\n", 500))
		b.InsertString(b.Len()/2, "gap\nhere\n") // gap mid-buffer: spanning encode
		scratch := make([]byte, 0, 4*b.Len())
		if allocs := testing.AllocsPerRun(50, func() {
			scratch = b.AppendBytes(scratch[:0])
		}); allocs != 0 {
			t.Errorf("AppendBytes allocated %v times per run, want 0", allocs)
		}
		if string(scratch) != b.String() {
			t.Errorf("AppendBytes = %q, want %q", string(scratch), b.String())
		}
	})

	t.Run("does not move the gap or mutate the buffer", func(t *testing.T) {
		b := NewWithContent("test", "one\ntwo\nthree")
		b.InsertString(4, "XY") // park the gap mid-buffer
		gs, ge := b.gapStart, b.gapEnd
		gen, mods := b.ChangeGen(), b.ModCount()
		pt, saved := b.Point(), b.Modified()
		b.AppendBytes(nil)
		b.AppendBytesRange(nil, 2, 9)
		if b.gapStart != gs || b.gapEnd != ge {
			t.Errorf("gap moved: gapStart %d→%d, gapEnd %d→%d", gs, b.gapStart, ge, b.gapEnd)
		}
		if b.ChangeGen() != gen || b.ModCount() != mods || b.Modified() != saved || b.Point() != pt {
			t.Error("AppendBytes mutated buffer state")
		}
	})
}

func TestAppendBytesRange(t *testing.T) {
	t.Run("relative to the gap", func(t *testing.T) {
		b := NewWithContent("test", "0123456789")
		b.InsertString(5, "abc") // "01234abc56789", gap sits at 8
		if b.gapStart != 8 {
			t.Fatalf("precondition: gapStart = %d, want 8", b.gapStart)
		}
		cases := []struct {
			name       string
			start, end int
			want       string
		}{
			{"entirely before the gap", 1, 5, "1234"},
			{"ends exactly at the gap", 0, 8, "01234abc"},
			{"entirely after the gap", 9, 13, "6789"},
			{"starts exactly at the gap", 8, 10, "56"},
			{"spans the gap", 3, 11, "34abc567"},
			{"whole buffer", 0, 13, "01234abc56789"},
			{"single rune before the gap", 2, 3, "2"},
			{"single rune after the gap", 12, 13, "9"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				if got := string(b.AppendBytesRange(nil, tc.start, tc.end)); got != tc.want {
					t.Errorf("AppendBytesRange(nil, %d, %d) = %q, want %q", tc.start, tc.end, got, tc.want)
				}
			})
		}
	})

	t.Run("matches Substring for every range", func(t *testing.T) {
		for _, tc := range appendBytesCorpus() {
			t.Run(tc.name, func(t *testing.T) {
				n := min(tc.b.Len(), 200) // every (start,end) pair would be quadratic
				for start := 0; start <= n; start++ {
					for end := start; end <= n; end++ {
						got := string(tc.b.AppendBytesRange(nil, start, end))
						if want := tc.b.Substring(start, end); got != want {
							t.Fatalf("AppendBytesRange(nil, %d, %d) = %q, want %q", start, end, got, want)
						}
					}
				}
			})
		}
	})

	t.Run("full range equals AppendBytes and String", func(t *testing.T) {
		for _, tc := range appendBytesCorpus() {
			full := string(tc.b.AppendBytesRange(nil, 0, tc.b.Len()))
			if want := string(tc.b.AppendBytes(nil)); full != want {
				t.Errorf("%s: full range = %q, AppendBytes = %q", tc.name, full, want)
			}
			if want := tc.b.String(); full != want {
				t.Errorf("%s: full range = %q, String = %q", tc.name, full, want)
			}
		}
	})

	t.Run("multibyte range boundaries land on rune edges", func(t *testing.T) {
		// Rune indices, not byte offsets: [1,3) of "a日本b" is "日本" (6 bytes).
		b := NewWithContent("test", "a日本b")
		got := b.AppendBytesRange(nil, 1, 3)
		if string(got) != "日本" {
			t.Errorf("AppendBytesRange(nil, 1, 3) = %q, want %q", string(got), "日本")
		}
		if len(got) != 6 {
			t.Errorf("AppendBytesRange produced %d bytes, want 6", len(got))
		}
	})

	t.Run("empty and degenerate ranges", func(t *testing.T) {
		b := NewWithContent("test", "hello world")
		dst := []byte("keep")
		cases := []struct {
			name       string
			start, end int
		}{
			{"start equals end", 4, 4},
			{"start equals end at zero", 0, 0},
			{"start equals end at Len", b.Len(), b.Len()},
			{"reversed", 8, 3},
			{"both negative", -10, -2},
			{"end below zero", 0, -1},
			{"start past Len", b.Len() + 5, b.Len() + 9},
			{"reversed after clamping", b.Len() + 1, 2},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got := b.AppendBytesRange(dst, tc.start, tc.end)
				if string(got) != "keep" {
					t.Errorf("AppendBytesRange(dst, %d, %d) = %q, want %q (dst untouched)", tc.start, tc.end, string(got), "keep")
				}
			})
		}
	})

	t.Run("clamps out-of-range arguments", func(t *testing.T) {
		b := NewWithContent("test", "abcde")
		if got := string(b.AppendBytesRange(nil, -7, 3)); got != "abc" {
			t.Errorf("negative start = %q, want %q", got, "abc")
		}
		if got := string(b.AppendBytesRange(nil, 2, 99)); got != "cde" {
			t.Errorf("over-range end = %q, want %q", got, "cde")
		}
		if got := string(b.AppendBytesRange(nil, -99, 99)); got != "abcde" {
			t.Errorf("both out of range = %q, want %q", got, "abcde")
		}
	})

	t.Run("appends to existing content", func(t *testing.T) {
		b := NewWithContent("test", "world!")
		if got := string(b.AppendBytesRange([]byte("hello "), 0, 5)); got != "hello world" {
			t.Errorf("AppendBytesRange = %q, want %q", got, "hello world")
		}
	})

	t.Run("grows an undersized destination", func(t *testing.T) {
		b := NewWithContent("test", strings.Repeat("ab\n", 50))
		got := b.AppendBytesRange(make([]byte, 0, 1), 10, 100)
		if want := b.Substring(10, 100); string(got) != want {
			t.Errorf("AppendBytesRange into small slice = %q, want %q", string(got), want)
		}
	})

	t.Run("no allocations when capacity suffices", func(t *testing.T) {
		b := NewWithContent("test", strings.Repeat("some line of text\n", 500))
		b.InsertString(b.Len()/2, "gap\nhere\n") // gap mid-buffer: spanning encode
		scratch := make([]byte, 0, 4*b.Len())
		start, end := b.Len()/2-100, b.Len()/2+100
		if allocs := testing.AllocsPerRun(50, func() {
			scratch = b.AppendBytesRange(scratch[:0], start, end)
		}); allocs != 0 {
			t.Errorf("AppendBytesRange allocated %v times per run, want 0", allocs)
		}
		if want := b.Substring(start, end); string(scratch) != want {
			t.Errorf("AppendBytesRange = %q, want %q", string(scratch), want)
		}
	})
}

// BenchmarkAppendBytes measures the single-pass UTF-8 encode straight out of the
// gap buffer against String(), which materialises a []rune and then encodes it.
// The reused scratch slice is what makes the AppendBytes case allocation-free.
func BenchmarkAppendBytes(b *testing.B) {
	for _, lines := range benchSizes {
		buf := NewWithContent("bench", benchContent(lines))
		buf.InsertString(buf.Len()/2, "x") // gap mid-buffer: worst case, two segments
		b.Run(sizeName(lines)+"/AppendBytes", func(b *testing.B) {
			b.ReportAllocs()
			scratch := make([]byte, 0, 4*buf.Len())
			for b.Loop() {
				scratch = buf.AppendBytes(scratch[:0])
			}
			_ = scratch
		})
		b.Run(sizeName(lines)+"/String", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = buf.String()
			}
		})
		// What the LSP text-sync path actually pays: AppendBytes into a reused
		// scratch slice, then one string copy at the hand-off to the client.
		b.Run(sizeName(lines)+"/AppendBytesToString", func(b *testing.B) {
			b.ReportAllocs()
			scratch := make([]byte, 0, 4*buf.Len())
			for b.Loop() {
				scratch = buf.AppendBytes(scratch[:0])
				_ = string(scratch)
			}
		})
	}
}
