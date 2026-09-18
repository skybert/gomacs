package syntax

import (
	"testing"
	"unicode/utf8"
)

// wantExactSpan asserts that spans contains a span whose rune range exactly
// covers the first occurrence of substr in text and whose Face is want.
//
// It deliberately checks both the offsets and the face: spanCoversText only
// checks offsets (a keyword painted FaceComment passes) and
// firstSpanWithFace/findSpanWithFace only check that some span carries the
// face (a single span covering the whole line passes).  Tests in this file
// and in python_test.go / bash_test.go / java_test.go use this helper so that
// both mistakes are caught.
func wantExactSpan(t *testing.T, spans []Span, text, substr string, want Face) {
	t.Helper()
	byteIdx := indexOf(text, substr)
	if byteIdx < 0 {
		t.Fatalf("substring %q is not present in %q", substr, text)
	}
	start := utf8.RuneCountInString(text[:byteIdx])
	end := start + utf8.RuneCountInString(substr)
	for _, sp := range spans {
		if sp.Start == start && sp.End == end {
			if sp.Face != want {
				t.Errorf("span for %q at [%d,%d): face = %+v, want %+v",
					substr, start, end, sp.Face, want)
			}
			return
		}
	}
	t.Errorf("no span exactly covering %q at [%d,%d); got %v", substr, start, end, spans)
}

func perlSpans(text string) []Span {
	h := PerlHighlighter{}
	return h.Highlight(text, 0, len([]rune(text)))
}

func TestPerlHighlighter_Comment(t *testing.T) {
	text := "# this is a comment\n"
	spans := perlSpans(text)
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d: %v", len(spans), spans)
	}
	wantExactSpan(t, spans, text, "# this is a comment", FaceComment)
}

func TestPerlHighlighter_Shebang(t *testing.T) {
	text := "#!/usr/bin/perl\n"
	spans := perlSpans(text)
	if len(spans) != 1 {
		t.Fatalf("expected 1 span for shebang, got %d: %v", len(spans), spans)
	}
	wantExactSpan(t, spans, text, "#!/usr/bin/perl", FaceComment)
}

func TestPerlHighlighter_Keyword(t *testing.T) {
	text := "my $x = 1;\n"
	wantExactSpan(t, perlSpans(text), text, "my", FaceKeyword)
}

func TestPerlHighlighter_DoubleQuotedString(t *testing.T) {
	text := `print "hello world";` + "\n"
	wantExactSpan(t, perlSpans(text), text, `"hello world"`, FaceString)
}

func TestPerlHighlighter_SingleQuotedString(t *testing.T) {
	text := "my $s = 'hello';\n"
	wantExactSpan(t, perlSpans(text), text, "'hello'", FaceString)
}

func TestPerlHighlighter_Variable(t *testing.T) {
	text := "my $name = 'Alice';\n"
	wantExactSpan(t, perlSpans(text), text, "$name", FaceType)
}

func TestPerlHighlighter_ArrayVariable(t *testing.T) {
	text := "my @items = (1, 2, 3);\n"
	wantExactSpan(t, perlSpans(text), text, "@items", FaceType)
}

func TestPerlHighlighter_Number(t *testing.T) {
	text := "my $n = 42;\n"
	wantExactSpan(t, perlSpans(text), text, "42", FaceNumber)
}

func TestPerlHighlighter_HexNumber(t *testing.T) {
	text := "my $n = 0xFF;\n"
	wantExactSpan(t, perlSpans(text), text, "0xFF", FaceNumber)
}

func TestPerlHighlighter_Builtin(t *testing.T) {
	text := "print \"hello\\n\";\n"
	spans := perlSpans(text)
	wantExactSpan(t, spans, text, "print", FaceFunction)
	wantExactSpan(t, spans, text, "\"hello\\n\"", FaceString)
}

func TestPerlHighlighter_PODComment(t *testing.T) {
	text := "code;\n=pod\nThis is POD documentation.\n=cut\nmore code;\n"
	spans := perlSpans(text)
	// The whole POD block, =pod through =cut, is one comment span; the code
	// before and after it is not part of it.
	wantExactSpan(t, spans, text, "=pod\nThis is POD documentation.\n=cut", FaceComment)
	if len(spans) != 1 {
		t.Errorf("expected only the POD span, got %d spans: %v", len(spans), spans)
	}
}

func TestPerlHighlighter_BacktickString(t *testing.T) {
	text := "my $out = `ls -l`;\n"
	wantExactSpan(t, perlSpans(text), text, "`ls -l`", FaceString)
}

func TestPerlHighlighter_BracedVariable(t *testing.T) {
	text := "print ${name};\n"
	wantExactSpan(t, perlSpans(text), text, "${name}", FaceType)
}

func TestPerlHighlighter_PunctuationVariable(t *testing.T) {
	text := "print $_;\n"
	wantExactSpan(t, perlSpans(text), text, "$_", FaceType)
}

func TestPerlHighlighter_CaptureVariable(t *testing.T) {
	text := "my $first = $1;\n"
	spans := perlSpans(text)
	wantExactSpan(t, spans, text, "$first", FaceType)
	wantExactSpan(t, spans, text, "$1", FaceType)
	count := 0
	for i := range spans {
		if spans[i].Face == FaceType {
			count++
		}
	}
	if count != 2 {
		t.Errorf("expected exactly 2 FaceType spans ($first and $1), got %d: %v", count, spans)
	}
}

func TestPerlHighlighter_OctalNumber(t *testing.T) {
	text := "my $n = 0b1010;\n"
	wantExactSpan(t, perlSpans(text), text, "0b1010", FaceNumber)
}

func TestPerlHighlighter_FloatNumber(t *testing.T) {
	text := "my $pi = 3.14e0;\n"
	wantExactSpan(t, perlSpans(text), text, "3.14e0", FaceNumber)
}

func TestPerlHighlighter_Empty(t *testing.T) {
	h := PerlHighlighter{}
	if spans := h.Highlight("", 0, 0); len(spans) != 0 {
		t.Errorf("expected no spans for empty input, got %v", spans)
	}
}

func TestPerlHighlighter_PlainIdentifier(t *testing.T) {
	// An identifier that is neither keyword nor builtin produces no span.
	text := "frobnicate;\n"
	spans := perlSpans(text)
	if len(spans) != 0 {
		t.Errorf("plain identifier should not be highlighted, got %v", spans)
	}
}
