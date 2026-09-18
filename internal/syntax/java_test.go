package syntax

import "testing"

func TestJavaHighlightKeyword(t *testing.T) {
	h := JavaHighlighter{}
	src := "public class Foo { void bar() { return; } }"
	spans := h.Highlight(src, 0, len([]rune(src)))
	for _, kw := range []string{"public", "class", "void", "return"} {
		wantExactSpan(t, spans, src, kw, FaceKeyword)
	}
}

func TestJavaHighlightPrimitive(t *testing.T) {
	h := JavaHighlighter{}
	src := "int x = 0;"
	spans := h.Highlight(src, 0, len([]rune(src)))
	wantExactSpan(t, spans, src, "int", FaceType)
}

func TestJavaHighlightType(t *testing.T) {
	h := JavaHighlighter{}
	src := "String s = \"hello\";"
	spans := h.Highlight(src, 0, len([]rune(src)))
	wantExactSpan(t, spans, src, "String", FaceType)
}

func TestJavaHighlightLineComment(t *testing.T) {
	h := JavaHighlighter{}
	src := "// this is a comment\nint x = 1;"
	spans := h.Highlight(src, 0, len([]rune(src)))
	wantExactSpan(t, spans, src, "// this is a comment", FaceComment)
}

func TestJavaHighlightBlockComment(t *testing.T) {
	h := JavaHighlighter{}
	src := "/* block comment */\nint x = 1;"
	spans := h.Highlight(src, 0, len([]rune(src)))
	wantExactSpan(t, spans, src, "/* block comment */", FaceComment)
}

func TestJavaHighlightAnnotation(t *testing.T) {
	h := JavaHighlighter{}
	src := "@Override\npublic void foo() {}"
	spans := h.Highlight(src, 0, len([]rune(src)))
	wantExactSpan(t, spans, src, "@Override", FaceFunction)
	wantExactSpan(t, spans, src, "public", FaceKeyword)
}

func TestJavaHighlightStringLiteral(t *testing.T) {
	h := JavaHighlighter{}
	src := `String s = "hello";`
	spans := h.Highlight(src, 0, len([]rune(src)))
	wantExactSpan(t, spans, src, `"hello"`, FaceString)
}

func TestJavaHighlightNumber(t *testing.T) {
	h := JavaHighlighter{}
	src := "int x = 42;"
	spans := h.Highlight(src, 0, len([]rune(src)))
	wantExactSpan(t, spans, src, "42", FaceNumber)
}

func TestJavaHighlightEmpty(t *testing.T) {
	h := JavaHighlighter{}
	spans := h.Highlight("", 0, 0)
	if len(spans) != 0 {
		t.Errorf("empty input: want no spans, got %d", len(spans))
	}
}

func TestJavaHighlightCharLiteral(t *testing.T) {
	h := JavaHighlighter{}
	src := "char c = 'a';"
	spans := h.Highlight(src, 0, len([]rune(src)))
	wantExactSpan(t, spans, src, "'a'", FaceString)
}

func TestJavaHighlightCharLiteralEscape(t *testing.T) {
	h := JavaHighlighter{}
	src := "char c = '\\n';"
	spans := h.Highlight(src, 0, len([]rune(src)))
	wantExactSpan(t, spans, src, "'\\n'", FaceString)
}

func TestJavaHighlightUnterminatedBlockComment(t *testing.T) {
	h := JavaHighlighter{}
	src := "/* never closed"
	spans := h.Highlight(src, 0, len([]rune(src)))
	if len(spans) != 1 {
		t.Fatalf("expected 1 span for unterminated block comment, got %v", spans)
	}
	if spans[0].Face != FaceComment {
		t.Errorf("unterminated block comment: face = %+v, want FaceComment", spans[0].Face)
	}
	// An unterminated block comment runs to the very end of the text.  This
	// used to stop one rune short, leaving the final rune unhighlighted.
	nRunes := len([]rune(src))
	if spans[0].Start != 0 || spans[0].End != nRunes {
		t.Errorf("unterminated block comment span = [%d,%d), want [0,%d)",
			spans[0].Start, spans[0].End, nRunes)
	}
}

func TestJavaHighlightPlainIdentifier(t *testing.T) {
	h := JavaHighlighter{}
	src := "myLocalVariable"
	spans := h.Highlight(src, 0, len([]rune(src)))
	if len(spans) != 0 {
		t.Errorf("plain identifier should not be highlighted, got %v", spans)
	}
}

func TestJavaHighlightHexNumber(t *testing.T) {
	h := JavaHighlighter{}
	src := "int x = 0xCAFE_BABEL;"
	spans := h.Highlight(src, 0, len([]rune(src)))
	wantExactSpan(t, spans, src, "0xCAFE_BABEL", FaceNumber)
}
