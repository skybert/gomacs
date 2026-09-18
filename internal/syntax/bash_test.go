package syntax

import "testing"

func TestBashHighlightKeyword(t *testing.T) {
	h := BashHighlighter{}
	src := "if [ -f foo ]; then echo hi; fi"
	spans := h.Highlight(src, 0, len([]rune(src)))
	for _, kw := range []string{"if", "then", "fi"} {
		wantExactSpan(t, spans, src, kw, FaceKeyword)
	}
	wantExactSpan(t, spans, src, "echo", FaceFunction)
}

func TestBashHighlightBuiltin(t *testing.T) {
	h := BashHighlighter{}
	src := "echo hello"
	spans := h.Highlight(src, 0, len([]rune(src)))
	wantExactSpan(t, spans, src, "echo", FaceFunction)
}

func TestBashHighlightComment(t *testing.T) {
	h := BashHighlighter{}
	src := "# this is a comment\necho hi"
	spans := h.Highlight(src, 0, len([]rune(src)))
	wantExactSpan(t, spans, src, "# this is a comment", FaceComment)
}

func TestBashHighlightShebang(t *testing.T) {
	h := BashHighlighter{}
	src := "#!/bin/bash\necho hi"
	spans := h.Highlight(src, 0, len([]rune(src)))
	wantExactSpan(t, spans, src, "#!/bin/bash", FaceComment)
}

func TestBashHighlightDoubleQuotedString(t *testing.T) {
	h := BashHighlighter{}
	src := `echo "hello world"`
	spans := h.Highlight(src, 0, len([]rune(src)))
	wantExactSpan(t, spans, src, `"hello world"`, FaceString)
}

func TestBashHighlightSingleQuotedString(t *testing.T) {
	h := BashHighlighter{}
	src := "echo 'hello world'"
	spans := h.Highlight(src, 0, len([]rune(src)))
	wantExactSpan(t, spans, src, "'hello world'", FaceString)
}

func TestBashHighlightVariable(t *testing.T) {
	h := BashHighlighter{}
	src := "echo $HOME"
	spans := h.Highlight(src, 0, len([]rune(src)))
	wantExactSpan(t, spans, src, "$HOME", FaceType)
}

func TestBashHighlightBraceVariable(t *testing.T) {
	h := BashHighlighter{}
	src := "echo ${HOME}"
	spans := h.Highlight(src, 0, len([]rune(src)))
	wantExactSpan(t, spans, src, "${HOME}", FaceType)
}

func TestBashHighlightNumber(t *testing.T) {
	h := BashHighlighter{}
	src := "exit 42"
	spans := h.Highlight(src, 0, len([]rune(src)))
	wantExactSpan(t, spans, src, "42", FaceNumber)
	wantExactSpan(t, spans, src, "exit", FaceKeyword)
}

func TestBashHighlightEmpty(t *testing.T) {
	h := BashHighlighter{}
	spans := h.Highlight("", 0, 0)
	if len(spans) != 0 {
		t.Errorf("empty input: want no spans, got %d", len(spans))
	}
}
