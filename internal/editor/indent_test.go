package editor

import (
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
