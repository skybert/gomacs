package syntax

import "testing"

func TestPythonHighlightKeyword(t *testing.T) {
	h := PythonHighlighter{}
	src := "def foo():\n    return True"
	spans := h.Highlight(src, 0, len([]rune(src)))
	for _, kw := range []string{"def", "return", "True"} {
		wantExactSpan(t, spans, src, kw, FaceKeyword)
	}
}

func TestPythonHighlightBuiltin(t *testing.T) {
	h := PythonHighlighter{}
	src := "print(len(x))"
	spans := h.Highlight(src, 0, len([]rune(src)))
	for _, fn := range []string{"print", "len"} {
		wantExactSpan(t, spans, src, fn, FaceFunction)
	}
}

func TestPythonHighlightComment(t *testing.T) {
	h := PythonHighlighter{}
	src := "# this is a comment\nx = 1"
	spans := h.Highlight(src, 0, len([]rune(src)))
	wantExactSpan(t, spans, src, "# this is a comment", FaceComment)
}

func TestPythonHighlightString(t *testing.T) {
	h := PythonHighlighter{}
	src := `x = "hello"`
	spans := h.Highlight(src, 0, len([]rune(src)))
	wantExactSpan(t, spans, src, `"hello"`, FaceString)
}

func TestPythonHighlightTripleQuotedString(t *testing.T) {
	h := PythonHighlighter{}
	src := `"""docstring"""`
	spans := h.Highlight(src, 0, len([]rune(src)))
	wantExactSpan(t, spans, src, `"""docstring"""`, FaceString)
}

func TestPythonHighlightDecorator(t *testing.T) {
	h := PythonHighlighter{}
	src := "@staticmethod\ndef foo(): pass"
	spans := h.Highlight(src, 0, len([]rune(src)))
	wantExactSpan(t, spans, src, "@staticmethod", FaceFunction)
	wantExactSpan(t, spans, src, "def", FaceKeyword)
}

func TestPythonHighlightNumber(t *testing.T) {
	h := PythonHighlighter{}
	src := "x = 42"
	spans := h.Highlight(src, 0, len([]rune(src)))
	wantExactSpan(t, spans, src, "42", FaceNumber)
}

func TestPythonHighlightEmpty(t *testing.T) {
	h := PythonHighlighter{}
	spans := h.Highlight("", 0, 0)
	if len(spans) != 0 {
		t.Errorf("empty input: want no spans, got %d", len(spans))
	}
}

func TestIsAlpha(t *testing.T) {
	for _, r := range "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ" {
		if !isAlpha(r) {
			t.Errorf("isAlpha(%q) = false, want true", r)
		}
	}
	for _, r := range "0123456789_!@#" {
		if isAlpha(r) {
			t.Errorf("isAlpha(%q) = true, want false", r)
		}
	}
}

func TestIsIdentChar(t *testing.T) {
	for _, r := range "abcXYZ_09" {
		if !isIdentChar(r) {
			t.Errorf("isIdentChar(%q) = false, want true", r)
		}
	}
	for _, r := range "!@# ." {
		if isIdentChar(r) {
			t.Errorf("isIdentChar(%q) = true, want false", r)
		}
	}
}
