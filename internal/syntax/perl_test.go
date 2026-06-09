package syntax

import "testing"

func TestPerlHighlighter_Comment(t *testing.T) {
	h := PerlHighlighter{}
	text := "# this is a comment\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	if len(spans) != 1 || spans[0].Face != FaceComment {
		t.Fatalf("expected 1 FaceComment span, got %v", spans)
	}
}

func TestPerlHighlighter_Shebang(t *testing.T) {
	h := PerlHighlighter{}
	text := "#!/usr/bin/perl\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	if len(spans) != 1 {
		t.Fatalf("expected 1 span for shebang, got %d: %v", len(spans), spans)
	}
	if spans[0].Face != FaceComment {
		t.Errorf("shebang: expected FaceComment, got %v", spans[0].Face)
	}
}

func TestPerlHighlighter_Keyword(t *testing.T) {
	h := PerlHighlighter{}
	text := "my $x = 1;\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	var kwSpan *Span
	for i := range spans {
		if spans[i].Face == FaceKeyword {
			kwSpan = &spans[i]
			break
		}
	}
	if kwSpan == nil {
		t.Error("expected FaceKeyword span for 'my'")
	}
}

func TestPerlHighlighter_DoubleQuotedString(t *testing.T) {
	h := PerlHighlighter{}
	text := `print "hello world";` + "\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	var strSpan *Span
	for i := range spans {
		if spans[i].Face == FaceString {
			strSpan = &spans[i]
		}
	}
	if strSpan == nil {
		t.Error("expected FaceString for double-quoted string")
	}
}

func TestPerlHighlighter_SingleQuotedString(t *testing.T) {
	h := PerlHighlighter{}
	text := "my $s = 'hello';\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	var strSpan *Span
	for i := range spans {
		if spans[i].Face == FaceString {
			strSpan = &spans[i]
		}
	}
	if strSpan == nil {
		t.Error("expected FaceString for single-quoted string")
	}
}

func TestPerlHighlighter_Variable(t *testing.T) {
	h := PerlHighlighter{}
	text := "my $name = 'Alice';\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	var varSpan *Span
	for i := range spans {
		if spans[i].Face == FaceType {
			varSpan = &spans[i]
		}
	}
	if varSpan == nil {
		t.Error("expected FaceType for scalar variable")
	}
}

func TestPerlHighlighter_ArrayVariable(t *testing.T) {
	h := PerlHighlighter{}
	text := "my @items = (1, 2, 3);\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	var varSpan *Span
	for i := range spans {
		if spans[i].Face == FaceType {
			varSpan = &spans[i]
		}
	}
	if varSpan == nil {
		t.Error("expected FaceType for array variable")
	}
}

func TestPerlHighlighter_Number(t *testing.T) {
	h := PerlHighlighter{}
	text := "my $n = 42;\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	var numSpan *Span
	for i := range spans {
		if spans[i].Face == FaceNumber {
			numSpan = &spans[i]
		}
	}
	if numSpan == nil {
		t.Error("expected FaceNumber for integer literal")
	}
}

func TestPerlHighlighter_HexNumber(t *testing.T) {
	h := PerlHighlighter{}
	text := "my $n = 0xFF;\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	var numSpan *Span
	for i := range spans {
		if spans[i].Face == FaceNumber {
			numSpan = &spans[i]
		}
	}
	if numSpan == nil {
		t.Error("expected FaceNumber for hex literal")
	}
}

func TestPerlHighlighter_Builtin(t *testing.T) {
	h := PerlHighlighter{}
	text := "print \"hello\\n\";\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	var fnSpan *Span
	for i := range spans {
		if spans[i].Face == FaceFunction {
			fnSpan = &spans[i]
		}
	}
	if fnSpan == nil {
		t.Error("expected FaceFunction for builtin 'print'")
	}
}

func TestPerlHighlighter_PODComment(t *testing.T) {
	h := PerlHighlighter{}
	text := "code;\n=pod\nThis is POD documentation.\n=cut\nmore code;\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	var commentSpan *Span
	for i := range spans {
		if spans[i].Face == FaceComment {
			commentSpan = &spans[i]
		}
	}
	if commentSpan == nil {
		t.Error("expected FaceComment for POD block")
	}
}

func TestPerlHighlighter_BacktickString(t *testing.T) {
	h := PerlHighlighter{}
	text := "my $out = `ls -l`;\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	var strSpan *Span
	for i := range spans {
		if spans[i].Face == FaceString {
			strSpan = &spans[i]
		}
	}
	if strSpan == nil {
		t.Error("expected FaceString for backtick command string")
	}
}

func TestPerlHighlighter_BracedVariable(t *testing.T) {
	h := PerlHighlighter{}
	text := "print ${name};\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	var varSpan *Span
	for i := range spans {
		if spans[i].Face == FaceType {
			varSpan = &spans[i]
		}
	}
	if varSpan == nil {
		t.Error("expected FaceType for ${...} braced variable")
	}
}

func TestPerlHighlighter_PunctuationVariable(t *testing.T) {
	h := PerlHighlighter{}
	text := "print $_;\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	var varSpan *Span
	for i := range spans {
		if spans[i].Face == FaceType {
			varSpan = &spans[i]
		}
	}
	if varSpan == nil {
		t.Error("expected FaceType for $_ punctuation variable")
	}
}

func TestPerlHighlighter_CaptureVariable(t *testing.T) {
	h := PerlHighlighter{}
	text := "my $first = $1;\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	count := 0
	for i := range spans {
		if spans[i].Face == FaceType {
			count++
		}
	}
	if count < 2 {
		t.Errorf("expected FaceType for both $first and capture var $1, got %d", count)
	}
}

func TestPerlHighlighter_OctalNumber(t *testing.T) {
	h := PerlHighlighter{}
	text := "my $n = 0b1010;\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	var numSpan *Span
	for i := range spans {
		if spans[i].Face == FaceNumber {
			numSpan = &spans[i]
		}
	}
	if numSpan == nil {
		t.Error("expected FaceNumber for binary literal")
	}
}

func TestPerlHighlighter_FloatNumber(t *testing.T) {
	h := PerlHighlighter{}
	text := "my $pi = 3.14e0;\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	var numSpan *Span
	for i := range spans {
		if spans[i].Face == FaceNumber {
			numSpan = &spans[i]
		}
	}
	if numSpan == nil {
		t.Error("expected FaceNumber for float literal")
	}
}

func TestPerlHighlighter_Empty(t *testing.T) {
	h := PerlHighlighter{}
	if spans := h.Highlight("", 0, 0); len(spans) != 0 {
		t.Errorf("expected no spans for empty input, got %v", spans)
	}
}

func TestPerlHighlighter_PlainIdentifier(t *testing.T) {
	h := PerlHighlighter{}
	// An identifier that is neither keyword nor builtin produces no span.
	text := "frobnicate;\n"
	spans := h.Highlight(text, 0, len([]rune(text)))
	for _, sp := range spans {
		if sp.Face == FaceKeyword || sp.Face == FaceFunction {
			t.Errorf("plain identifier should not be highlighted: %v", sp)
		}
	}
}
