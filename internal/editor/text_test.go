package editor

import (
	"testing"
)

func TestRegionBounds_NoMark(t *testing.T) {
	e := newTestEditor("hello")
	b := buf(e)
	b.SetPoint(3)
	start, end := regionBounds(b)
	if start != 3 || end != 3 {
		t.Errorf("no mark: want (3,3), got (%d,%d)", start, end)
	}
}

func TestRegionBounds_MarkBeforePoint(t *testing.T) {
	e := newTestEditor("hello")
	b := buf(e)
	b.SetMark(1)
	b.SetMarkActive(true)
	b.SetPoint(4)
	start, end := regionBounds(b)
	if start != 1 || end != 4 {
		t.Errorf("mark<point: want (1,4), got (%d,%d)", start, end)
	}
}

func TestRegionBounds_MarkAfterPoint(t *testing.T) {
	e := newTestEditor("hello")
	b := buf(e)
	b.SetMark(4)
	b.SetMarkActive(true)
	b.SetPoint(1)
	start, end := regionBounds(b)
	if start != 1 || end != 4 {
		t.Errorf("mark>point: want (1,4), got (%d,%d)", start, end)
	}
}

func TestDeleteTrailingWhitespace(t *testing.T) {
	e := newTestEditor("hello   \nworld  \nfoo")
	e.cmdDeleteTrailingWhitespace()
	got := buf(e).String()
	if got != "hello\nworld\nfoo" {
		t.Errorf("got %q, want \"hello\\nworld\\nfoo\"", got)
	}
}

func TestJoinLine(t *testing.T) {
	e := newTestEditor("hello\nworld")
	buf(e).SetPoint(6) // start of "world"
	e.cmdJoinLine()
	got := buf(e).String()
	if got != "hello world" {
		t.Errorf("got %q, want \"hello world\"", got)
	}
}

func TestBackToIndentation(t *testing.T) {
	e := newTestEditor("   hello")
	buf(e).SetPoint(0)
	e.cmdBackToIndentation()
	if got := buf(e).Point(); got != 3 {
		t.Errorf("point = %d, want 3", got)
	}
}

func TestDeleteBlankLines_OnNonBlank(t *testing.T) {
	// On a non-blank line, C-x C-o deletes all blank lines that follow.
	e := newTestEditor("first\n\n\nsecond")
	buf(e).SetPoint(0) // on "first"
	e.cmdDeleteBlankLines()
	got := buf(e).String()
	if got != "first\nsecond" {
		t.Errorf("got %q, want \"first\\nsecond\"", got)
	}
}

func TestDeleteBlankLines_CollapseMultiple(t *testing.T) {
	// Three blank lines between first and second: one blank line is removed.
	e := newTestEditor("first\n\n\n\nsecond")
	buf(e).SetPoint(7) // on middle blank line
	e.cmdDeleteBlankLines()
	got := buf(e).String()
	if got != "first\n\n\nsecond" {
		t.Errorf("got %q, want \"first\\n\\n\\nsecond\"", got)
	}
}

func TestTransposeWords(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetPoint(0)
	e.cmdTransposeWords()
	got := buf(e).String()
	if got != "world hello" {
		t.Errorf("got %q, want \"world hello\"", got)
	}
}

func TestSortLines(t *testing.T) {
	e := newTestEditor("banana\napple\ncherry")
	b := buf(e)
	b.SetMark(0)
	b.SetMarkActive(true)
	b.SetPoint(b.Len())
	e.cmdSortLines()
	got := b.String()
	if got != "apple\nbanana\ncherry" {
		t.Errorf("got %q, want \"apple\\nbanana\\ncherry\"", got)
	}
}

func TestUpcaseRegion(t *testing.T) {
	e := newTestEditor("hello world")
	b := buf(e)
	b.SetMark(0)
	b.SetMarkActive(true)
	b.SetPoint(5)
	e.cmdUpcaseRegion()
	if got := b.String(); got != "HELLO world" {
		t.Errorf("got %q, want \"HELLO world\"", got)
	}
}

func TestDowncaseRegion(t *testing.T) {
	e := newTestEditor("HELLO WORLD")
	b := buf(e)
	b.SetMark(0)
	b.SetMarkActive(true)
	b.SetPoint(5)
	e.cmdDowncaseRegion()
	if got := b.String(); got != "hello WORLD" {
		t.Errorf("got %q, want \"hello WORLD\"", got)
	}
}
