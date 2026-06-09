package syntax

import "testing"

func TestJavaHighlightKeyword(t *testing.T) {
	h := JavaHighlighter{}
	src := "public class Foo { void bar() { return; } }"
	spans := h.Highlight(src, 0, len([]rune(src)))
	for _, kw := range []string{"public", "class", "void", "return"} {
		if !spanCoversText(spans, src, kw) {
			t.Errorf("keyword %q not highlighted", kw)
		}
	}
}

func TestJavaHighlightPrimitive(t *testing.T) {
	h := JavaHighlighter{}
	src := "int x = 0;"
	spans := h.Highlight(src, 0, len([]rune(src)))
	if !spanCoversText(spans, src, "int") {
		t.Error("primitive 'int' not highlighted")
	}
}

func TestJavaHighlightType(t *testing.T) {
	h := JavaHighlighter{}
	src := "String s = \"hello\";"
	spans := h.Highlight(src, 0, len([]rune(src)))
	if !spanCoversText(spans, src, "String") {
		t.Error("type 'String' not highlighted")
	}
}

func TestJavaHighlightLineComment(t *testing.T) {
	h := JavaHighlighter{}
	src := "// this is a comment\nint x = 1;"
	spans := h.Highlight(src, 0, len([]rune(src)))
	if s := firstSpanWithFace(spans, FaceComment); s == nil {
		t.Error("line comment not highlighted")
	}
}

func TestJavaHighlightBlockComment(t *testing.T) {
	h := JavaHighlighter{}
	src := "/* block comment */\nint x = 1;"
	spans := h.Highlight(src, 0, len([]rune(src)))
	if !spanCoversText(spans, src, "/* block comment */") {
		t.Error("block comment not highlighted")
	}
}

func TestJavaHighlightAnnotation(t *testing.T) {
	h := JavaHighlighter{}
	src := "@Override\npublic void foo() {}"
	spans := h.Highlight(src, 0, len([]rune(src)))
	if !spanCoversText(spans, src, "@Override") {
		t.Error("annotation @Override not highlighted")
	}
}

func TestJavaHighlightStringLiteral(t *testing.T) {
	h := JavaHighlighter{}
	src := `String s = "hello";`
	spans := h.Highlight(src, 0, len([]rune(src)))
	if !spanCoversText(spans, src, `"hello"`) {
		t.Error("string literal not highlighted")
	}
}

func TestJavaHighlightNumber(t *testing.T) {
	h := JavaHighlighter{}
	src := "int x = 42;"
	spans := h.Highlight(src, 0, len([]rune(src)))
	if !spanCoversText(spans, src, "42") {
		t.Error("number 42 not highlighted")
	}
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
	if !spanCoversText(spans, src, "'a'") {
		t.Error("char literal 'a' not highlighted as string")
	}
}

func TestJavaHighlightCharLiteralEscape(t *testing.T) {
	h := JavaHighlighter{}
	src := "char c = '\\n';"
	spans := h.Highlight(src, 0, len([]rune(src)))
	var found bool
	for _, sp := range spans {
		if sp.Face == FaceString {
			found = true
		}
	}
	if !found {
		t.Error("escaped char literal not highlighted as string")
	}
}

func TestJavaHighlightUnterminatedBlockComment(t *testing.T) {
	h := JavaHighlighter{}
	src := "/* never closed"
	spans := h.Highlight(src, 0, len([]rune(src)))
	if len(spans) == 0 || spans[0].Face != FaceComment {
		t.Errorf("expected FaceComment for unterminated block comment, got %v", spans)
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
	var found bool
	for _, sp := range spans {
		if sp.Face == FaceNumber {
			found = true
		}
	}
	if !found {
		t.Error("expected FaceNumber for hex literal")
	}
}
