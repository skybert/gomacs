package syntax

import (
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
