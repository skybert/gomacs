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

// ---------------------------------------------------------------------------
// maybeAutoFill
// ---------------------------------------------------------------------------

func TestAutoFill_WrapsInTextMode(t *testing.T) {
	e := newTestEditor("Hello World Foo")
	b := buf(e)
	b.SetMode("text")
	e.fillColumn = 10
	// Simulate the cursor being after "Foo" (position 15).
	b.SetPoint(15)
	e.maybeAutoFill()
	got := b.String()
	// "Hello " (6) exceeds col 10, last space at col 5 → break there.
	want := "Hello\nWorld Foo"
	if got != want {
		t.Errorf("auto-fill text: got %q, want %q", got, want)
	}
}

func TestAutoFill_NoWrapUnderFillColumn(t *testing.T) {
	e := newTestEditor("Short line")
	b := buf(e)
	b.SetMode("text")
	e.fillColumn = 70
	b.SetPoint(10)
	e.maybeAutoFill()
	if got := b.String(); got != "Short line" {
		t.Errorf("auto-fill no-op: got %q, want unchanged", got)
	}
}

func TestAutoFill_NoWrapInGoMode(t *testing.T) {
	e := newTestEditor("func foo() { return something + more + stuff }")
	b := buf(e)
	b.SetMode("go")
	e.fillColumn = 10
	b.SetPoint(b.Len())
	e.maybeAutoFill()
	// Go mode should never auto-fill.
	if got := b.String(); got != "func foo() { return something + more + stuff }" {
		t.Errorf("auto-fill in go mode should be no-op, got %q", got)
	}
}

func TestAutoFill_MarkdownMode(t *testing.T) {
	e := newTestEditor("Hello World Foo")
	b := buf(e)
	b.SetMode("markdown")
	e.fillColumn = 10
	b.SetPoint(15)
	e.maybeAutoFill()
	want := "Hello\nWorld Foo"
	if got := b.String(); got != want {
		t.Errorf("auto-fill markdown: got %q, want %q", got, want)
	}
}
