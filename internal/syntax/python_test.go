package syntax

import "testing"

func TestPythonHighlightKeyword(t *testing.T) {
	h := PythonHighlighter{}
	src := "def foo():\n    return True"
	spans := h.Highlight(src, 0, len([]rune(src)))
	for _, kw := range []string{"def", "return", "True"} {
		if !spanCoversText(spans, src, kw) {
			t.Errorf("keyword %q not highlighted", kw)
		}
	}
}

func TestPythonHighlightBuiltin(t *testing.T) {
	h := PythonHighlighter{}
	src := "print(len(x))"
	spans := h.Highlight(src, 0, len([]rune(src)))
	for _, fn := range []string{"print", "len"} {
		if !spanCoversText(spans, src, fn) {
			t.Errorf("builtin %q not highlighted", fn)
		}
	}
}

func TestPythonHighlightComment(t *testing.T) {
	h := PythonHighlighter{}
	src := "# this is a comment\nx = 1"
	spans := h.Highlight(src, 0, len([]rune(src)))
	if s := firstSpanWithFace(spans, FaceComment); s == nil {
		t.Error("comment not highlighted")
	}
}

func TestPythonHighlightString(t *testing.T) {
	h := PythonHighlighter{}
	src := `x = "hello"`
	spans := h.Highlight(src, 0, len([]rune(src)))
	if !spanCoversText(spans, src, `"hello"`) {
		t.Error("string literal not highlighted")
	}
}

func TestPythonHighlightTripleQuotedString(t *testing.T) {
	h := PythonHighlighter{}
	src := `"""docstring"""`
	spans := h.Highlight(src, 0, len([]rune(src)))
	if !spanCoversText(spans, src, `"""docstring"""`) {
		t.Error("triple-quoted string not highlighted")
	}
}

func TestPythonHighlightDecorator(t *testing.T) {
	h := PythonHighlighter{}
	src := "@staticmethod\ndef foo(): pass"
	spans := h.Highlight(src, 0, len([]rune(src)))
	if !spanCoversText(spans, src, "@staticmethod") {
		t.Error("decorator @staticmethod not highlighted")
	}
}

func TestPythonHighlightNumber(t *testing.T) {
	h := PythonHighlighter{}
	src := "x = 42"
	spans := h.Highlight(src, 0, len([]rune(src)))
	if !spanCoversText(spans, src, "42") {
		t.Error("number 42 not highlighted")
	}
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
