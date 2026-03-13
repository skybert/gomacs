package syntax

import "testing"

func TestBashHighlightKeyword(t *testing.T) {
	h := BashHighlighter{}
	src := "if [ -f foo ]; then echo hi; fi"
	spans := h.Highlight(src, 0, len([]rune(src)))
	for _, kw := range []string{"if", "then", "fi"} {
		if !spanCoversText(spans, src, kw) {
			t.Errorf("keyword %q not highlighted in %q", kw, src)
		}
	}
}

func TestBashHighlightBuiltin(t *testing.T) {
	h := BashHighlighter{}
	src := "echo hello"
	spans := h.Highlight(src, 0, len([]rune(src)))
	if !spanCoversText(spans, src, "echo") {
		t.Errorf("builtin 'echo' not highlighted")
	}
}

func TestBashHighlightComment(t *testing.T) {
	h := BashHighlighter{}
	src := "# this is a comment\necho hi"
	spans := h.Highlight(src, 0, len([]rune(src)))
	if s := firstSpanWithFace(spans, FaceComment); s == nil {
		t.Error("comment not highlighted")
	}
}

func TestBashHighlightShebang(t *testing.T) {
	h := BashHighlighter{}
	src := "#!/bin/bash\necho hi"
	spans := h.Highlight(src, 0, len([]rune(src)))
	if s := firstSpanWithFace(spans, FaceComment); s == nil {
		t.Error("shebang not highlighted as comment")
	}
}

func TestBashHighlightDoubleQuotedString(t *testing.T) {
	h := BashHighlighter{}
	src := `echo "hello world"`
	spans := h.Highlight(src, 0, len([]rune(src)))
	if !spanCoversText(spans, src, `"hello world"`) {
		t.Error("double-quoted string not highlighted")
	}
}

func TestBashHighlightSingleQuotedString(t *testing.T) {
	h := BashHighlighter{}
	src := "echo 'hello world'"
	spans := h.Highlight(src, 0, len([]rune(src)))
	if !spanCoversText(spans, src, "'hello world'") {
		t.Error("single-quoted string not highlighted")
	}
}

func TestBashHighlightVariable(t *testing.T) {
	h := BashHighlighter{}
	src := "echo $HOME"
	spans := h.Highlight(src, 0, len([]rune(src)))
	if !spanCoversText(spans, src, "$HOME") {
		t.Error("variable $HOME not highlighted")
	}
}

func TestBashHighlightBraceVariable(t *testing.T) {
	h := BashHighlighter{}
	src := "echo ${HOME}"
	spans := h.Highlight(src, 0, len([]rune(src)))
	if !spanCoversText(spans, src, "${HOME}") {
		t.Error("brace variable ${HOME} not highlighted")
	}
}

func TestBashHighlightNumber(t *testing.T) {
	h := BashHighlighter{}
	src := "exit 42"
	spans := h.Highlight(src, 0, len([]rune(src)))
	if !spanCoversText(spans, src, "42") {
		t.Error("number 42 not highlighted")
	}
}

func TestBashHighlightEmpty(t *testing.T) {
	h := BashHighlighter{}
	spans := h.Highlight("", 0, 0)
	if len(spans) != 0 {
		t.Errorf("empty input: want no spans, got %d", len(spans))
	}
}
