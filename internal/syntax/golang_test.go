package syntax

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

// firstSpan returns the first span in spans whose face equals want, or nil.
func firstSpanWithFace(spans []Span, want Face) *Span {
	for i := range spans {
		if spans[i].Face == want {
			return &spans[i]
		}
	}
	return nil
}

// spanCoversText returns true if some span in spans covers the rune range of
// the first occurrence of substr within text.
func spanCoversText(spans []Span, text, substr string) bool {
	byteIdx := indexOf(text, substr)
	if byteIdx < 0 {
		return false
	}
	runeStart := utf8.RuneCountInString(text[:byteIdx])
	runeEnd := runeStart + utf8.RuneCountInString(substr)
	for _, sp := range spans {
		if sp.Start <= runeStart && sp.End >= runeEnd {
			return true
		}
	}
	return false
}

// indexOf returns the byte index of the first occurrence of sub in s, or -1.
func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// spanCoversRuneRange returns true if sp covers the rune range of the first
// occurrence of substr in text.
func spanCoversRuneRange(sp Span, text, substr string) bool {
	byteIdx := indexOf(text, substr)
	if byteIdx < 0 {
		return false
	}
	runeStart := utf8.RuneCountInString(text[:byteIdx])
	runeEnd := runeStart + utf8.RuneCountInString(substr)
	return sp.Start <= runeStart && sp.End >= runeEnd
}

// fullLen returns 0 and the rune count of text.
func fullLen(text string) (int, int) {
	return 0, utf8.RuneCountInString(text)
}

// ---- Tests ------------------------------------------------------------------

func TestGoHighlighter_PackageKeyword(t *testing.T) {
	h := GoHighlighter{}
	text := "package main"
	start, end := fullLen(text)
	spans := h.Highlight(text, start, end)

	if !spanCoversText(spans, text, "package") {
		t.Fatal("expected FaceKeyword span covering 'package'")
	}
	found := false
	for _, sp := range spans {
		if sp.Face == FaceKeyword && spanCoversRuneRange(sp, text, "package") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no FaceKeyword span for 'package'; spans: %v", spans)
	}
}

func TestGoHighlighter_Comment(t *testing.T) {
	h := GoHighlighter{}
	text := "// comment\n"
	start, end := fullLen(text)
	spans := h.Highlight(text, start, end)

	sp := firstSpanWithFace(spans, FaceComment)
	if sp == nil {
		t.Fatalf("expected FaceComment span; got %v", spans)
	}
}

func TestGoHighlighter_String(t *testing.T) {
	h := GoHighlighter{}
	text := `"hello"`
	start, end := fullLen(text)
	spans := h.Highlight(text, start, end)

	sp := firstSpanWithFace(spans, FaceString)
	if sp == nil {
		t.Fatalf("expected FaceString span; got %v", spans)
	}
}

func TestGoHighlighter_Number(t *testing.T) {
	h := GoHighlighter{}
	text := "x := 42"
	start, end := fullLen(text)
	spans := h.Highlight(text, start, end)

	sp := firstSpanWithFace(spans, FaceNumber)
	if sp == nil {
		t.Fatalf("expected FaceNumber span for '42'; got %v", spans)
	}
}

func TestGoHighlighter_TypeIdent(t *testing.T) {
	h := GoHighlighter{}
	for _, typeName := range []string{"int", "string"} {
		text := typeName
		start, end := fullLen(text)
		spans := h.Highlight(text, start, end)
		sp := firstSpanWithFace(spans, FaceType)
		if sp == nil {
			t.Errorf("expected FaceType span for %q; got %v", typeName, spans)
		}
	}
}

func TestGoHighlighter_BuiltinFunc(t *testing.T) {
	h := GoHighlighter{}
	for _, fn := range []string{"len", "make"} {
		text := fn
		start, end := fullLen(text)
		spans := h.Highlight(text, start, end)
		sp := firstSpanWithFace(spans, FaceFunction)
		if sp == nil {
			t.Errorf("expected FaceFunction span for %q; got %v", fn, spans)
		}
	}
}

func TestGoHighlighter_FuncKeyword(t *testing.T) {
	h := GoHighlighter{}
	text := "func main() {}"
	start, end := fullLen(text)
	spans := h.Highlight(text, start, end)

	found := false
	for _, sp := range spans {
		if sp.Face == FaceKeyword && spanCoversRuneRange(sp, text, "func") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected FaceKeyword span for 'func'; spans: %v", spans)
	}
}

func TestGoHighlighter_PartialRange(t *testing.T) {
	h := GoHighlighter{}
	text := "package main\nfunc f() {}\n"
	start := utf8.RuneCountInString("package main\n")
	end := utf8.RuneCountInString(text)
	spans := h.Highlight(text, start, end)

	for _, sp := range spans {
		if sp.End <= start {
			t.Errorf("span %v is outside requested range [%d,%d)", sp, start, end)
		}
	}
	for _, sp := range spans {
		if sp.Face == FaceKeyword && sp.Start < start {
			t.Errorf("got keyword span before start offset: %v", sp)
		}
	}
}

// ---------------------------------------------------------------------------
// Benchmarks: early termination
// ---------------------------------------------------------------------------

// benchGoSource builds a Go source file with n functions (8 lines each).
func benchGoSource(n int) string {
	var sb strings.Builder
	sb.WriteString("package main\n\nimport \"fmt\"\n\n")
	for i := range n {
		fmt.Fprintf(&sb, "// doc comment for f%d\nfunc f%d(a int) string {\n"+
			"\tif a > %d {\n\t\treturn fmt.Sprintf(\"big %%d\", a)\n\t}\n"+
			"\treturn \"small\"\n}\n\n", i, i, i)
	}
	return sb.String()
}

// benchGoHighlight measures one Highlight call over a ~10 000-line Go file with
// the given end offset, i.e. what a single redraw costs.
func benchGoHighlight(bench *testing.B, visibleRunes int) {
	text := benchGoSource(1250)
	n := utf8.RuneCountInString(text)
	end := min(visibleRunes, n)
	h := GoHighlighter{}
	bench.ReportAllocs()
	bench.ResetTimer()
	for range bench.N {
		if spans := h.Highlight(text, 0, end); len(spans) == 0 {
			bench.Fatal("no spans")
		}
	}
}

// BenchmarkGoHighlightFullBuffer is the cost when the whole buffer is requested
// (the behaviour before highlighting was bounded to the visible region).
func BenchmarkGoHighlightFullBuffer(bench *testing.B) {
	benchGoHighlight(bench, 1<<30)
}

// BenchmarkGoHighlightOneScreen is the cost when only a screenful plus the
// editor's look-ahead margin is requested — the top-of-file case.
func BenchmarkGoHighlightOneScreen(bench *testing.B) {
	benchGoHighlight(bench, 9000)
}
