package syntax

import (
	"testing"
)

// findSpan returns the first span with the given face, or the zero value.
func findElispSpan(spans []Span, face Face) (Span, bool) {
	for _, s := range spans {
		if s.Face == face {
			return s, true
		}
	}
	return Span{}, false
}

func elispSpans(text string) []Span {
	h := ElispHighlighter{}
	return h.Highlight(text, 0, len([]rune(text)))
}

// ---------------------------------------------------------------------------
// Comments
// ---------------------------------------------------------------------------

func TestElispComment(t *testing.T) {
	spans := elispSpans("; this is a comment\n(+ 1 2)")
	s, ok := findElispSpan(spans, FaceComment)
	if !ok {
		t.Fatal("expected a comment span")
	}
	if s.Start != 0 {
		t.Errorf("comment start: want 0, got %d", s.Start)
	}
	// Comment ends before the newline (exclusive).
	if s.End != 19 {
		t.Errorf("comment end: want 19, got %d", s.End)
	}
}

// ---------------------------------------------------------------------------
// Strings
// ---------------------------------------------------------------------------

func TestElispString(t *testing.T) {
	spans := elispSpans(`"hello world"`)
	s, ok := findElispSpan(spans, FaceString)
	if !ok {
		t.Fatal("expected a string span")
	}
	if s.Start != 0 || s.End != 13 {
		t.Errorf("string span: want [0,13), got [%d,%d)", s.Start, s.End)
	}
}

func TestElispStringWithEscape(t *testing.T) {
	spans := elispSpans(`"say \"hi\""`)
	_, ok := findElispSpan(spans, FaceString)
	if !ok {
		t.Fatal("expected a string span with escapes")
	}
}

func TestElispCharLiteral(t *testing.T) {
	spans := elispSpans("?a")
	_, ok := findElispSpan(spans, FaceString)
	if !ok {
		t.Fatal("expected char literal as string span")
	}
}

func TestElispCharLiteralEscape(t *testing.T) {
	spans := elispSpans(`?\n`)
	_, ok := findElispSpan(spans, FaceString)
	if !ok {
		t.Fatal("expected escaped char literal as string span")
	}
}

// ---------------------------------------------------------------------------
// Numbers
// ---------------------------------------------------------------------------

func TestElispInteger(t *testing.T) {
	spans := elispSpans("42")
	_, ok := findElispSpan(spans, FaceNumber)
	if !ok {
		t.Fatal("expected number span for 42")
	}
}

func TestElispFloat(t *testing.T) {
	spans := elispSpans("3.14")
	_, ok := findElispSpan(spans, FaceNumber)
	if !ok {
		t.Fatal("expected number span for 3.14")
	}
}

func TestElispNegativeNumber(t *testing.T) {
	spans := elispSpans("-7")
	_, ok := findElispSpan(spans, FaceNumber)
	if !ok {
		t.Fatal("expected number span for -7")
	}
}

// ---------------------------------------------------------------------------
// Keywords
// ---------------------------------------------------------------------------

func TestElispKeyword(t *testing.T) {
	for _, kw := range []string{"defun", "let", "let*", "setq", "if", "when", "unless", "progn", "lambda"} {
		spans := elispSpans(kw)
		_, ok := findElispSpan(spans, FaceKeyword)
		if !ok {
			t.Errorf("expected keyword face for %q", kw)
		}
	}
}

func TestElispKeywordInExpr(t *testing.T) {
	spans := elispSpans("(defun foo () nil)")
	_, ok := findElispSpan(spans, FaceKeyword)
	if !ok {
		t.Fatal("expected keyword span for defun inside expression")
	}
}

// ---------------------------------------------------------------------------
// Builtins
// ---------------------------------------------------------------------------

func TestElispBuiltin(t *testing.T) {
	for _, fn := range []string{"car", "cdr", "cons", "message", "format", "mapcar"} {
		spans := elispSpans(fn)
		_, ok := findElispSpan(spans, FaceFunction)
		if !ok {
			t.Errorf("expected builtin face for %q", fn)
		}
	}
}

// ---------------------------------------------------------------------------
// No false positives for plain symbols
// ---------------------------------------------------------------------------

func TestElispPlainSymbolNoHighlight(t *testing.T) {
	spans := elispSpans("my-custom-function")
	for _, s := range spans {
		if s.Face == FaceKeyword || s.Face == FaceFunction {
			t.Errorf("plain symbol got unexpected face %v", s.Face)
		}
	}
}

// ---------------------------------------------------------------------------
// Mixed content
// ---------------------------------------------------------------------------

func TestElispMixed(t *testing.T) {
	src := `; comment
(defun greet (name)
  "Say hello."
  (message "Hello, %s!" name))`
	spans := elispSpans(src)

	hasComment := false
	hasKeyword := false
	hasString := false
	hasBuiltin := false
	for _, s := range spans {
		switch s.Face {
		case FaceComment:
			hasComment = true
		case FaceKeyword:
			hasKeyword = true
		case FaceString:
			hasString = true
		case FaceFunction:
			hasBuiltin = true
		}
	}
	if !hasComment {
		t.Error("mixed: missing comment span")
	}
	if !hasKeyword {
		t.Error("mixed: missing keyword span")
	}
	if !hasString {
		t.Error("mixed: missing string span")
	}
	if !hasBuiltin {
		t.Error("mixed: missing builtin span")
	}
}
