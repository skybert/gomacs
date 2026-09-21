package editor

import (
	"strings"
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

func TestDeleteTrailingWhitespaceRegion(t *testing.T) {
	// Only the selected region (first line) should have trailing WS removed.
	e := newTestEditor("hello   \nworld  \n")
	b := buf(e)
	b.SetMark(0)
	b.SetMarkActive(true)
	b.SetPoint(8) // end of "hello   "
	e.cmdDeleteTrailingWhitespace()
	want := "hello\nworld  \n"
	if got := b.String(); got != want {
		t.Errorf("want %q, got %q", want, got)
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

func TestCmdFillParagraphBasic(t *testing.T) {
	e := newTestEditor("one two three four five six seven eight nine ten")
	e.fillColumn = 20
	buf(e).SetPoint(0)
	e.cmdFillParagraph()
	got := buf(e).String()
	// Each line should be no wider than 20 columns.
	for _, line := range strings.Split(got, "\n") {
		if len([]rune(line)) > 20 {
			t.Errorf("cmdFillParagraph: line too long (%d > 20): %q", len([]rune(line)), line)
		}
	}
	// All words should still be present.
	for _, w := range []string{"one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten"} {
		if !strings.Contains(got, w) {
			t.Errorf("cmdFillParagraph: word %q missing from result", w)
		}
	}
}

func TestCmdFillParagraphReadOnlyNoOp(t *testing.T) {
	e := newTestEditor("one two three four five")
	buf(e).SetReadOnly(true)
	buf(e).SetPoint(0)
	e.fillColumn = 10
	before := buf(e).String()
	e.cmdFillParagraph()
	if buf(e).String() != before {
		t.Errorf("cmdFillParagraph read-only: buffer should be unchanged, got %q", buf(e).String())
	}
}

func TestCmdFillParagraphEmptyParagraph(t *testing.T) {
	// An empty buffer — should be a no-op.
	e := newTestEditor("")
	e.fillColumn = 70
	buf(e).SetPoint(0)
	e.cmdFillParagraph()
	if buf(e).String() != "" {
		t.Errorf("cmdFillParagraph empty: expected empty buffer, got %q", buf(e).String())
	}
}

func TestCmdFillParagraphMultilineParagraph(t *testing.T) {
	// A paragraph already split into multiple lines — should be joined and reflowed.
	content := "The quick brown fox\njumps over the lazy dog.\n"
	e := newTestEditor(content)
	e.fillColumn = 40
	buf(e).SetPoint(0)
	e.cmdFillParagraph()
	got := buf(e).String()
	// All words must be present.
	for _, w := range strings.Fields(content) {
		if !strings.Contains(got, w) {
			t.Errorf("cmdFillParagraph multiline: word %q missing", w)
		}
	}
}

func TestCmdFillParagraphStopsAtBlankLine(t *testing.T) {
	// Two paragraphs separated by a blank line — only the first should be filled.
	content := "first paragraph here\n\nsecond paragraph here\n"
	e := newTestEditor(content)
	e.fillColumn = 70
	buf(e).SetPoint(0)
	e.cmdFillParagraph()
	got := buf(e).String()
	// Second paragraph must be untouched.
	if !strings.Contains(got, "second paragraph here") {
		t.Errorf("cmdFillParagraph: second paragraph should be unchanged, got %q", got)
	}
}

func TestCmdDowncaseRegionNoRegionNoOp(t *testing.T) {
	e := newTestEditor("Hello World")
	buf(e).SetMarkActive(false)
	buf(e).SetPoint(5)
	before := buf(e).String()
	e.cmdDowncaseRegion()
	if buf(e).String() != before {
		t.Errorf("cmdDowncaseRegion no region: buffer changed, got %q", buf(e).String())
	}
}

func TestCmdDowncaseRegionWithRegion(t *testing.T) {
	e := newTestEditor("Hello World")
	b := buf(e)
	b.SetMark(0)
	b.SetMarkActive(true)
	b.SetPoint(5)
	e.cmdDowncaseRegion()
	got := b.String()
	if got != "hello World" {
		t.Errorf("cmdDowncaseRegion: want %q, got %q", "hello World", got)
	}
}

func TestCmdDowncaseRegionReadOnlyNoOp(t *testing.T) {
	e := newTestEditor("Hello World")
	b := buf(e)
	b.SetReadOnly(true)
	b.SetMark(0)
	b.SetMarkActive(true)
	b.SetPoint(5)
	before := b.String()
	e.cmdDowncaseRegion()
	if b.String() != before {
		t.Errorf("cmdDowncaseRegion read-only: buffer changed, got %q", b.String())
	}
}

func TestCmdUpcaseRegionNoRegionNoOp(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetMarkActive(false)
	buf(e).SetPoint(5)
	before := buf(e).String()
	e.cmdUpcaseRegion()
	if buf(e).String() != before {
		t.Errorf("cmdUpcaseRegion no region: buffer changed, got %q", buf(e).String())
	}
}

func TestCmdUpcaseRegionWithRegion(t *testing.T) {
	e := newTestEditor("hello world")
	b := buf(e)
	b.SetMark(0)
	b.SetMarkActive(true)
	b.SetPoint(5)
	e.cmdUpcaseRegion()
	got := b.String()
	if got != "HELLO world" {
		t.Errorf("cmdUpcaseRegion: want %q, got %q", "HELLO world", got)
	}
}

func TestCmdUpcaseRegionReadOnlyNoOp(t *testing.T) {
	e := newTestEditor("hello world")
	b := buf(e)
	b.SetReadOnly(true)
	b.SetMark(0)
	b.SetMarkActive(true)
	b.SetPoint(5)
	before := b.String()
	e.cmdUpcaseRegion()
	if b.String() != before {
		t.Errorf("cmdUpcaseRegion read-only: buffer changed, got %q", b.String())
	}
}

// ---------------------------------------------------------------------------
// abs
// ---------------------------------------------------------------------------

func TestAbsPositive(t *testing.T) {
	if got := abs(7); got != 7 {
		t.Fatalf("abs(7): want 7, got %d", got)
	}
}

func TestAbsNegative(t *testing.T) {
	if got := abs(-5); got != 5 {
		t.Fatalf("abs(-5): want 5, got %d", got)
	}
}

func TestAbsZero(t *testing.T) {
	if got := abs(0); got != 0 {
		t.Fatalf("abs(0): want 0, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// regionBounds — extra edge cases beyond text_test.go
// ---------------------------------------------------------------------------

func TestRegionBoundsPointEqualsMarkActive(t *testing.T) {
	// mark == point while active → zero-length region
	e := newTestEditor("hello")
	b := buf(e)
	b.SetMark(3)
	b.SetMarkActive(true)
	b.SetPoint(3)
	start, end := regionBounds(b)
	if start != 3 || end != 3 {
		t.Fatalf("regionBounds equal mark/point: want (3,3), got (%d,%d)", start, end)
	}
}

func TestRegionBoundsMarkInactiveLarge(t *testing.T) {
	// Mark present but inactive → should return point,point.
	e := newTestEditor("abcdefg")
	b := buf(e)
	b.SetMark(0)
	b.SetMarkActive(false)
	b.SetPoint(5)
	start, end := regionBounds(b)
	if start != 5 || end != 5 {
		t.Fatalf("regionBounds inactive mark: want (5,5), got (%d,%d)", start, end)
	}
}

// ---------------------------------------------------------------------------
// cmdTransposeWords — extra cases
// ---------------------------------------------------------------------------

func TestTransposeWordsMidPhrase(t *testing.T) {
	// Point between bar and baz — those two words should swap.
	e := newTestEditor("foo bar baz")
	b := buf(e)
	b.SetPoint(4) // just before "bar"
	e.cmdTransposeWords()
	got := b.String()
	if got != "foo baz bar" {
		t.Fatalf("transpose-words mid: want %q, got %q", "foo baz bar", got)
	}
}

func TestTransposeWordsSingleWord(t *testing.T) {
	// Only one word in the buffer: there is nothing to swap, so the buffer
	// must come out byte-for-byte unchanged and the change generation must not
	// move (no undo record for a no-op).
	e := newTestEditor("onlyone")
	b := buf(e)
	b.SetPoint(0)
	before := b.String()
	genBefore := b.ChangeGen()
	e.cmdTransposeWords()
	if got := b.String(); got != before {
		t.Errorf("transpose-words with a single word changed the buffer: %q -> %q", before, got)
	}
	if got := b.ChangeGen(); got != genBefore {
		t.Errorf("transpose-words with a single word bumped ChangeGen: %d -> %d", genBefore, got)
	}
}

// ---------------------------------------------------------------------------
// cmdDeleteBlankLines — non-blank line followed by blank lines
// ---------------------------------------------------------------------------

func TestDeleteBlankLinesFollowingNonBlank(t *testing.T) {
	e := newTestEditor("text\n\n\nnext")
	b := buf(e)
	b.SetPoint(0) // on "text"
	e.cmdDeleteBlankLines()
	got := b.String()
	if strings.Contains(got, "\n\n") {
		t.Fatalf("delete-blank-lines from non-blank: blank lines after should be removed; buffer=%q", got)
	}
	if !strings.Contains(got, "text") || !strings.Contains(got, "next") {
		t.Fatalf("delete-blank-lines: non-blank lines lost; got %q", got)
	}
}

// ---------------------------------------------------------------------------
// deleteTrailingWhitespace helper — direct call
// ---------------------------------------------------------------------------

func TestDeleteTrailingWhitespaceHelper(t *testing.T) {
	e := newTestEditor("  hello   \n  world  \nclean\n")
	b := buf(e)
	e.deleteTrailingWhitespace(b, 0, b.Len())
	want := "  hello\n  world\nclean\n"
	if got := b.String(); got != want {
		t.Fatalf("deleteTrailingWhitespace: want %q, got %q", want, got)
	}
}

func TestDeleteTrailingWhitespaceTabsOnly(t *testing.T) {
	e := newTestEditor("line\t\t\n")
	b := buf(e)
	e.deleteTrailingWhitespace(b, 0, b.Len())
	if got := b.String(); got != "line\n" {
		t.Fatalf("deleteTrailingWhitespace tabs: want %q, got %q", "line\n", got)
	}
}

// ---------------------------------------------------------------------------
// cmdJoinLine — extra cases
// ---------------------------------------------------------------------------

func TestJoinLineAtFirstLineBOF(t *testing.T) {
	// At the very first line there is no previous line — buffer must not change.
	e := newTestEditor("hello\nworld")
	b := buf(e)
	b.SetPoint(0)
	before := b.String()
	e.cmdJoinLine()
	if got := b.String(); got != before {
		t.Fatalf("join-line at first line: buffer changed; got %q", got)
	}
}

func TestJoinLineStripLeadingWhitespace(t *testing.T) {
	// The current line starts with spaces that are stripped on join.
	e := newTestEditor("hello\n   world")
	b := buf(e)
	b.SetPoint(6) // on "   world"
	e.cmdJoinLine()
	got := b.String()
	if !strings.Contains(got, "hello world") {
		t.Fatalf("join-line strip indent: want %q, got %q", "hello world", got)
	}
}

// ---------------------------------------------------------------------------
// cmdBackToIndentation — extra cases
// ---------------------------------------------------------------------------

func TestBackToIndentationTabIndent(t *testing.T) {
	e := newTestEditor("\t\thello")
	b := buf(e)
	b.SetPoint(0)
	e.cmdBackToIndentation()
	if got := b.Point(); got != 2 {
		t.Fatalf("back-to-indentation tabs: want point=2, got %d", got)
	}
}

func TestBackToIndentationOnBlankLine(t *testing.T) {
	// On a blank line the function should leave point at beginning of line.
	e := newTestEditor("first\n\nthird")
	b := buf(e)
	b.SetPoint(6) // on the blank line
	e.cmdBackToIndentation()
	// Blank line has no non-whitespace chars; point should stay at line start.
	if got := b.Point(); got != 6 {
		t.Fatalf("back-to-indentation blank line: want 6, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// cmdUpcaseRegion / cmdDowncaseRegion — extra cases
// ---------------------------------------------------------------------------

func TestUpcaseRegionMarkInverted(t *testing.T) {
	// When mark is after point (mark > point) the region is still handled.
	e := newTestEditor("hello world")
	b := buf(e)
	b.SetMark(5) // mark after point
	b.SetMarkActive(true)
	b.SetPoint(0)
	e.cmdUpcaseRegion()
	if got := b.String(); got != "HELLO world" {
		t.Fatalf("upcase-region inverted: want %q, got %q", "HELLO world", got)
	}
}

func TestDowncaseRegionEntireBuffer(t *testing.T) {
	e := newTestEditor("HELLO WORLD")
	b := buf(e)
	b.SetMark(0)
	b.SetMarkActive(true)
	b.SetPoint(b.Len())
	e.cmdDowncaseRegion()
	if got := b.String(); got != "hello world" {
		t.Fatalf("downcase-region full: want %q, got %q", "hello world", got)
	}
}

func TestUpcaseRegionDeactivatesMark(t *testing.T) {
	e := newTestEditor("hello")
	b := buf(e)
	b.SetMark(0)
	b.SetMarkActive(true)
	b.SetPoint(5)
	e.cmdUpcaseRegion()
	if b.MarkActive() {
		t.Fatal("upcase-region: mark should be deactivated after command")
	}
}

// ---------------------------------------------------------------------------
// cmdSortLines — message and no-region fallback
// ---------------------------------------------------------------------------

func TestSortLinesMessageReportsCount(t *testing.T) {
	e := newTestEditor("b\na\nc\n")
	b := buf(e)
	b.SetMarkActive(false)
	e.cmdSortLines()
	if e.message == "" {
		t.Fatal("sort-lines: expected message to be set")
	}
	if !strings.Contains(e.message, "3") {
		t.Errorf("sort-lines: expected '3' lines in message, got %q", e.message)
	}
	want := "a\nb\nc\n"
	if got := b.String(); got != want {
		t.Errorf("sort-lines no-region: want %q, got %q", want, got)
	}
}

// ---------------------------------------------------------------------------
// cmdDeleteDuplicateLines — extra coverage
// ---------------------------------------------------------------------------

func TestDeleteDuplicateLinesMessageSet(t *testing.T) {
	e := newTestEditor("x\nx\ny\n")
	b := buf(e)
	b.SetMarkActive(false)
	e.cmdDeleteDuplicateLines()
	if e.message == "" {
		t.Fatal("delete-duplicate-lines: expected message to be set")
	}
	want := "x\ny\n"
	if got := b.String(); got != want {
		t.Errorf("delete-duplicate-lines: want %q, got %q", want, got)
	}
}

func TestDeleteDuplicateLinesMessageNoneRemoved(t *testing.T) {
	e := newTestEditor("a\nb\nc\n")
	b := buf(e)
	b.SetMarkActive(false)
	e.cmdDeleteDuplicateLines()
	if !strings.Contains(e.message, "0") {
		t.Errorf("delete-duplicate-lines no dups: want '0' in message, got %q", e.message)
	}
	want := "a\nb\nc\n"
	if got := b.String(); got != want {
		t.Errorf("delete-duplicate-lines no-dups: want %q, got %q", want, got)
	}
}

// ---------------------------------------------------------------------------
// cmdFillParagraph
// ---------------------------------------------------------------------------

func TestFillParagraphWrapsAtFillColumn(t *testing.T) {
	content := "one two three four five six seven eight nine ten eleven twelve"
	e := newTestEditor(content)
	b := buf(e)
	b.SetPoint(0)
	e.fillColumn = 20
	e.cmdFillParagraph()
	got := b.String()
	for _, line := range strings.Split(got, "\n") {
		if len([]rune(line)) > 20 {
			t.Errorf("fill-paragraph: line %q exceeds fill-column 20", line)
		}
	}
	// All words must still be present.
	for _, w := range strings.Fields(content) {
		if !strings.Contains(got, w) {
			t.Errorf("fill-paragraph: word %q lost after fill", w)
		}
	}
}

func TestFillParagraphJoinsShortLines(t *testing.T) {
	e := newTestEditor("short\nlines\nhere")
	b := buf(e)
	b.SetPoint(0)
	e.fillColumn = 40
	e.cmdFillParagraph()
	got := b.String()
	if strings.Contains(got, "\n") {
		t.Fatalf("fill-paragraph short lines: expected single line, got %q", got)
	}
	if !strings.Contains(got, "short") || !strings.Contains(got, "here") {
		t.Fatalf("fill-paragraph: words lost; got %q", got)
	}
}

// ---------------------------------------------------------------------------
// cmdSetFillColumn
// ---------------------------------------------------------------------------

func TestSetFillColumnViaUniversalArg(t *testing.T) {
	e := newTestEditor("hello world")
	e.universalArg = 80
	e.universalArgSet = true
	e.cmdSetFillColumn()
	if e.fillColumn != 80 {
		t.Fatalf("set-fill-column: want 80, got %d", e.fillColumn)
	}
	if e.message == "" {
		t.Fatal("set-fill-column: expected message to be set")
	}
}

func TestSetFillColumnFromCursorColumn(t *testing.T) {
	e := newTestEditor("hello world")
	b := buf(e)
	b.SetPoint(5)
	e.universalArgSet = false
	e.cmdSetFillColumn()
	_, col := b.LineCol(b.Point())
	if e.fillColumn != col {
		t.Fatalf("set-fill-column from cursor: want %d, got %d", col, e.fillColumn)
	}
	if e.message == "" {
		t.Fatal("set-fill-column: expected message to be set")
	}
}

// ---------------------------------------------------------------------------
// cmdIndentRegion
// ---------------------------------------------------------------------------

func TestIndentRegionAddsTab(t *testing.T) {
	e := newTestEditor("line1\nline2\nline3\n")
	b := buf(e)
	b.SetMark(0)
	b.SetMarkActive(true)
	b.SetPoint(b.Len())
	e.cmdIndentRegion()
	got := b.String()
	for _, line := range strings.Split(strings.TrimRight(got, "\n"), "\n") {
		if !strings.HasPrefix(line, "\t") {
			t.Errorf("indent-region: line %q not indented with tab", line)
		}
	}
}

func TestIndentRegionDeactivatesMark(t *testing.T) {
	e := newTestEditor("line1\nline2\n")
	b := buf(e)
	b.SetMark(0)
	b.SetMarkActive(true)
	b.SetPoint(b.Len())
	e.cmdIndentRegion()
	if b.MarkActive() {
		t.Fatal("indent-region: mark should be deactivated after command")
	}
}

func TestIndentRegionNoMarkIsNoop(t *testing.T) {
	e := newTestEditor("line1\nline2\n")
	b := buf(e)
	b.SetPoint(0)
	before := b.String()
	e.cmdIndentRegion()
	if got := b.String(); got != before {
		t.Fatalf("indent-region no mark: buffer changed; got %q", got)
	}
}

// ---------------------------------------------------------------------------
// cmdIndentRigidly
// ---------------------------------------------------------------------------

func TestIndentRigidlyPositive(t *testing.T) {
	e := newTestEditor("alpha\nbeta\n")
	b := buf(e)
	b.SetMark(0)
	b.SetMarkActive(true)
	b.SetPoint(b.Len())
	e.universalArg = 4
	e.universalArgSet = true
	e.cmdIndentRigidly()
	got := b.String()
	for _, line := range strings.Split(strings.TrimRight(got, "\n"), "\n") {
		if !strings.HasPrefix(line, "    ") {
			t.Errorf("indent-rigidly +4: line %q not indented 4 spaces", line)
		}
	}
}

func TestIndentRigidlyNegative(t *testing.T) {
	e := newTestEditor("    alpha\n    beta\n")
	b := buf(e)
	b.SetMark(0)
	b.SetMarkActive(true)
	b.SetPoint(b.Len())
	e.universalArg = -2
	e.universalArgSet = true
	e.cmdIndentRigidly()
	got := b.String()
	for _, line := range strings.Split(strings.TrimRight(got, "\n"), "\n") {
		if strings.HasPrefix(line, "    ") {
			t.Errorf("indent-rigidly -2: line %q still has 4-space indent", line)
		}
		if !strings.HasPrefix(line, "  ") {
			t.Errorf("indent-rigidly -2: line %q should still have 2-space indent", line)
		}
	}
}

func TestIndentRigidlyNoMarkIsNoop(t *testing.T) {
	e := newTestEditor("hello\n")
	b := buf(e)
	b.SetPoint(0)
	before := b.String()
	e.universalArg = 2
	e.universalArgSet = true
	e.cmdIndentRigidly()
	if got := b.String(); got != before {
		t.Fatalf("indent-rigidly no mark: buffer changed; got %q", got)
	}
}

func TestIndentRigidlyDeactivatesMark(t *testing.T) {
	e := newTestEditor("  line\n")
	b := buf(e)
	b.SetMark(0)
	b.SetMarkActive(true)
	b.SetPoint(b.Len())
	e.universalArg = 1
	e.universalArgSet = true
	e.cmdIndentRigidly()
	if b.MarkActive() {
		t.Fatal("indent-rigidly: mark should be deactivated after command")
	}
}

// ---------------------------------------------------------------------------
// cmdReplaceString (via minibuf done-func simulation)
// ---------------------------------------------------------------------------

func TestReplaceStringReplacesAll(t *testing.T) {
	e := newTestEditor("foo bar foo baz foo")
	b := buf(e)
	b.SetPoint(0)

	// cmdReplaceString registers two nested ReadMinibuffer calls.
	// We invoke them directly without running an event loop.
	e.cmdReplaceString()
	fromFn := e.minibufDoneFunc
	e.minibufActive = false
	e.minibufDoneFunc = nil
	fromFn("foo") // triggers the second ReadMinibuffer

	toFn := e.minibufDoneFunc
	e.minibufActive = false
	e.minibufDoneFunc = nil
	toFn("qux") // perform replacement

	got := b.String()
	if strings.Contains(got, "foo") {
		t.Fatalf("replace-string: 'foo' should be gone; got %q", got)
	}
	if strings.Count(got, "qux") != 3 {
		t.Fatalf("replace-string: expected 3 'qux'; got %q", got)
	}
	if e.message == "" {
		t.Fatal("replace-string: expected message to be set")
	}
}

func TestReplaceStringNoOccurrences(t *testing.T) {
	e := newTestEditor("hello world")
	b := buf(e)
	b.SetPoint(0)

	e.cmdReplaceString()
	fromFn := e.minibufDoneFunc
	e.minibufActive = false
	e.minibufDoneFunc = nil
	fromFn("xyz")

	toFn := e.minibufDoneFunc
	e.minibufActive = false
	e.minibufDoneFunc = nil
	toFn("abc")

	if got := b.String(); got != "hello world" {
		t.Fatalf("replace-string no match: buffer changed; got %q", got)
	}
}

func TestReplaceStringEmptyFrom(t *testing.T) {
	// Passing an empty "from" string should be a no-op (early return).
	e := newTestEditor("hello world")
	b := buf(e)
	b.SetPoint(0)

	e.cmdReplaceString()
	fromFn := e.minibufDoneFunc
	e.minibufActive = false
	e.minibufDoneFunc = nil
	fromFn("") // empty → should return early, not register second callback

	// Second minibufDoneFunc should be nil since the first callback returned early.
	if e.minibufDoneFunc != nil {
		// If a second callback was registered, drain it harmlessly.
		toFn := e.minibufDoneFunc
		e.minibufActive = false
		e.minibufDoneFunc = nil
		toFn("")
	}
	if got := b.String(); got != "hello world" {
		t.Fatalf("replace-string empty from: buffer changed; got %q", got)
	}
}

// ---------------------------------------------------------------------------
// cmdNarrowToRegion / cmdWiden
// ---------------------------------------------------------------------------

func TestNarrowToRegionRestrictsView(t *testing.T) {
	e := newTestEditor("abcdefghij")
	b := buf(e)
	b.SetMark(2)
	b.SetMarkActive(true)
	b.SetPoint(7)
	e.cmdNarrowToRegion()
	if !b.Narrowed() {
		t.Fatal("narrow-to-region: buffer should be narrowed")
	}
	if b.NarrowMin() != 2 {
		t.Fatalf("narrow-to-region: want NarrowMin=2, got %d", b.NarrowMin())
	}
	if b.NarrowMax() != 7 {
		t.Fatalf("narrow-to-region: want NarrowMax=7, got %d", b.NarrowMax())
	}
}

func TestNarrowToRegionNoMarkSetsMessage(t *testing.T) {
	e := newTestEditor("hello")
	b := buf(e)
	b.SetPoint(3)
	e.cmdNarrowToRegion()
	if e.message == "" {
		t.Fatal("narrow-to-region with no mark: expected message to be set")
	}
	// Buffer content must be unchanged.
	if b.String() != "hello" {
		t.Fatalf("narrow-to-region no mark: buffer changed; got %q", b.String())
	}
}

func TestWidenRestoresFullBuffer(t *testing.T) {
	e := newTestEditor("abcdefghij")
	b := buf(e)
	b.SetMark(2)
	b.SetMarkActive(true)
	b.SetPoint(7)
	e.cmdNarrowToRegion()
	if !b.Narrowed() {
		t.Fatal("pre-condition: buffer should be narrowed before widen")
	}
	e.cmdWiden()
	if b.Narrowed() {
		t.Fatal("widen: buffer should no longer be narrowed")
	}
	// After widening, full buffer length is accessible.
	if got := b.NarrowMax(); got != b.Len() {
		t.Fatalf("widen: NarrowMax should equal Len()=%d, got %d", b.Len(), got)
	}
}

func TestWidenSetsMessage(t *testing.T) {
	e := newTestEditor("content")
	e.cmdWiden()
	if e.message == "" {
		t.Fatal("widen: expected message to be set")
	}
}

// ---------------------------------------------------------------------------
// cmdGotoLine (via minibuf done-func simulation)
// ---------------------------------------------------------------------------
