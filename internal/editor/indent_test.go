package editor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/skybert/gomacs/internal/buffer"
)

func TestElispIndentLevelTopLevel(t *testing.T) {
	// Empty text: top-level → 0.
	if got := elispIndentLevel("", 0); got != 0 {
		t.Errorf("empty text: want 0, got %d", got)
	}
}

func TestElispIndentLevelBody(t *testing.T) {
	// Body line of a defun: 1 unclosed '(' at col 0 → indent = 2.
	src := "(defun foo ()\n"
	if got := elispIndentLevel(src, len(src)); got != 2 {
		t.Errorf("body after defun: want 2, got %d", got)
	}
}

func TestElispIndentLevelNested(t *testing.T) {
	// (defun foo ()
	//   (let ((x 1))
	//     <cursor>
	// Innermost unclosed '(' is '(let …' at col 2 → indent = 4.
	src := "(defun foo ()\n  (let ((x 1))\n"
	lineStart := len([]rune(src))
	if got := elispIndentLevel(src, lineStart); got != 4 {
		t.Errorf("inside let body: want 4, got %d", got)
	}
}

func TestElispIndentLevelAfterCompleteForm(t *testing.T) {
	// After a complete top-level form the indent is 0.
	src := "(defun foo () nil)\n"
	if got := elispIndentLevel(src, len(src)); got != 0 {
		t.Errorf("after complete form: want 0, got %d", got)
	}
}

func TestElispIndentLevelStringIgnored(t *testing.T) {
	// Parens inside a string literal must not affect depth.
	// One real unclosed '(' at col 0 → indent = 2.
	src := "(message \"(((\"\n"
	if got := elispIndentLevel(src, len(src)); got != 2 {
		t.Errorf("paren inside string: want 2, got %d", got)
	}
}

func TestElispIndentLevelCommentIgnored(t *testing.T) {
	// Parens on comment lines must not affect depth.
	// '(((` on the comment line are ignored; '(defun …' at col 0 → indent = 2.
	src := "; (((\n(defun foo ()\n"
	if got := elispIndentLevel(src, len(src)); got != 2 {
		t.Errorf("paren inside comment: want 2, got %d", got)
	}
}

func TestIndentElispLine(t *testing.T) {
	// Wrong indentation (4 spaces) inside a defun body — should become 2.
	src := "(defun foo ()\n    wrong)\n"
	buf := buffer.NewWithContent("*test*", src)
	lineStart := len("(defun foo ()\n")
	buf.SetPoint(lineStart + 2) // somewhere on the wrongly-indented line

	indentElispLine(buf)

	got := buf.String()
	want := "(defun foo ()\n  wrong)\n"
	if got != want {
		t.Errorf("after indent:\nwant: %q\ngot:  %q", want, got)
	}
}

func TestIndentElispLineTopLevel(t *testing.T) {
	// A top-level form should be indented to column 0.
	src := "  (defun foo () nil)\n"
	buf := buffer.NewWithContent("*test*", src)
	buf.SetPoint(2)

	indentElispLine(buf)

	got := buf.String()
	want := "(defun foo () nil)\n"
	if got != want {
		t.Errorf("top-level indent:\nwant: %q\ngot:  %q", want, got)
	}
}

// ---------------------------------------------------------------------------
// elispIndentLevelAt — differential tests against elispIndentLevel
// ---------------------------------------------------------------------------

// elispTricky exercises the corner cases of the paren scanner: parens inside
// comments and strings, escaped quotes and escaped newlines inside strings,
// character literals, and more closers than openers.
const elispTricky = `;; (((
(defun tricky (x)
  ;; a comment with (unbalanced parens
  (let ((y "a string with ) and \" quote")
        (z "multi
line ( string"))
    (message "%s%s" y z))))
)))
(setq a '(1 2 3)
      b "escaped backslash \\"
      c "escaped newline \
continued")
`

// benchElispSource builds an Elisp file with `forms` five-line defuns.
func benchElispSource(forms int) string {
	var sb strings.Builder
	for i := range forms {
		fmt.Fprintf(&sb, "(defun f%d (x)\n  ;; comment with (paren\n  (let ((y (* x 2)))\n    (+ x y)))\n\n", i)
	}
	return sb.String()
}

// elispDiffSources are checked line by line: the buffer-backed
// elispIndentLevelAt must agree with the string-based elispIndentLevel.
// The generated source is long enough to force several checkpoints.
var elispDiffSources = []string{
	"",
	"\n\n\n",
	elispTricky,
	benchElispSource(40) + elispTricky,
}

func TestElispIndentLevelAtMatchesStringEngine(t *testing.T) {
	for n, src := range elispDiffSources {
		e := newTestEditor(src)
		b := buf(e)
		b.SetMode(modeElisp)

		// Every line start, plus a position inside each line and one past the
		// end of the buffer.
		pos := 0
		for _, line := range strings.Split(src, "\n") {
			for _, at := range []int{pos, pos + len([]rune(line))/2} {
				want := elispIndentLevel(src, at)
				if got := elispIndentLevelAt(b, at); got != want {
					t.Fatalf("source %d, pos %d (%q): elispIndentLevelAt = %d, want %d",
						n, at, line, got, want)
				}
			}
			pos += len([]rune(line)) + 1
		}
		if got, want := elispIndentLevelAt(b, b.Len()+10), elispIndentLevel(src, len([]rune(src))+10); got != want {
			t.Errorf("source %d, past end: got %d, want %d", n, got, want)
		}
	}
}

// TestElispIndentLevelAtBackwards walks the lines bottom-up so that most
// lookups start from a checkpoint in the middle of the list.
func TestElispIndentLevelAtBackwards(t *testing.T) {
	src := benchElispSource(40) + elispTricky
	e := newTestEditor(src)
	b := buf(e)
	b.SetMode(modeElisp)

	starts := lineStartsOf(src)
	for i := len(starts) - 1; i >= 0; i-- {
		want := elispIndentLevel(src, starts[i])
		if got := elispIndentLevelAt(b, starts[i]); got != want {
			t.Fatalf("line %d: elispIndentLevelAt = %d, want %d", i, got, want)
		}
	}
}

// TestElispIndentLevelAtColdCache checks the same equivalence with the
// checkpoint cache dropped before every call.
func TestElispIndentLevelAtColdCache(t *testing.T) {
	src := benchElispSource(20) + elispTricky
	e := newTestEditor(src)
	b := buf(e)
	b.SetMode(modeElisp)

	for _, start := range lineStartsOf(src) {
		indentCaches = map[*buffer.Buffer]*indentCache{}
		want := elispIndentLevel(src, start)
		if got := elispIndentLevelAt(b, start); got != want {
			t.Fatalf("pos %d: cold-cache elispIndentLevelAt = %d, want %d", start, got, want)
		}
	}
}

// TestElispIndentLevelAtAfterEditAbove is the invalidation test: an extra
// opening paren inserted near the top must deepen the indentation far below.
func TestElispIndentLevelAtAfterEditAbove(t *testing.T) {
	src := benchElispSource(40)
	e := newTestEditor(src)
	b := buf(e)
	b.SetMode(modeElisp)

	deep := b.Len() - 2
	before := elispIndentLevelAt(b, deep)

	// Open an unclosed paren at the very top of the buffer.
	b.InsertString(0, "(progn\n")
	after := elispIndentLevelAt(b, deep+len("(progn\n"))

	if before != 0 || after != 2 {
		t.Errorf("indent above/below an inserted (progn: got %d then %d, want 0 then 2",
			before, after)
	}
}

// ---------------------------------------------------------------------------
// benchmarks
// ---------------------------------------------------------------------------

// BenchmarkIndentElispLineDeepInLargeFile measures one Tab press on an already
// correctly indented line deep inside a large Elisp file.
func BenchmarkIndentElispLineDeepInLargeFile(b *testing.B) {
	src := benchElispSource(2000)
	e := newTestEditor(src)
	bf := buf(e)
	bf.SetMode(modeElisp)
	bf.SetPoint(strings.Index(src, "(defun f1600 (x)"))
	for b.Loop() {
		indentElispLine(bf)
	}
}

// BenchmarkIndentElispNewlineDeepInLargeFile measures the auto-indent that
// follows every Enter keypress deep inside a large Elisp file.
func BenchmarkIndentElispNewlineDeepInLargeFile(b *testing.B) {
	src := benchElispSource(2000)
	e := newTestEditor(src)
	bf := buf(e)
	bf.SetMode(modeElisp)
	bf.SetPoint(strings.Index(src, "(defun f1600 (x)"))
	for b.Loop() {
		pt := bf.Point()
		bf.Insert(pt, '\n')
		bf.SetPoint(pt + 1)
		indentElispLine(bf)
	}
}
