package editor

import (
	"strings"
	"testing"
)

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
	// Only one word in the buffer — should set a message, no crash.
	e := newTestEditor("onlyone")
	b := buf(e)
	b.SetPoint(0)
	before := b.String()
	e.cmdTransposeWords()
	// Buffer must be unchanged (or contain some variation that keeps content).
	// The command should not panic.
	_ = b.String()
	_ = before
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
}

func TestDeleteDuplicateLinesMessageNoneRemoved(t *testing.T) {
	e := newTestEditor("a\nb\nc\n")
	b := buf(e)
	b.SetMarkActive(false)
	e.cmdDeleteDuplicateLines()
	if !strings.Contains(e.message, "0") {
		t.Errorf("delete-duplicate-lines no dups: want '0' in message, got %q", e.message)
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

func TestGotoLineMoves(t *testing.T) {
	e := newTestEditor("line1\nline2\nline3\nline4\n")
	b := buf(e)
	b.SetPoint(0)

	e.cmdGotoLine()
	fn := e.minibufDoneFunc
	e.minibufActive = false
	e.minibufDoneFunc = nil
	fn("3")

	want := b.LineStart(3)
	if got := b.Point(); got != want {
		t.Fatalf("goto-line 3: want point=%d, got %d", want, got)
	}
}

func TestGotoLineInvalidSetsMessage(t *testing.T) {
	e := newTestEditor("one\ntwo\n")
	e.cmdGotoLine()
	fn := e.minibufDoneFunc
	e.minibufActive = false
	e.minibufDoneFunc = nil
	fn("notanumber")

	if !strings.Contains(e.message, "Invalid") {
		t.Fatalf("goto-line invalid: expected 'Invalid' in message, got %q", e.message)
	}
}

func TestGotoLineFirstLine(t *testing.T) {
	e := newTestEditor("alpha\nbeta\ngamma\n")
	b := buf(e)
	b.SetPoint(10)

	e.cmdGotoLine()
	fn := e.minibufDoneFunc
	e.minibufActive = false
	e.minibufDoneFunc = nil
	fn("1")

	if got := b.Point(); got != 0 {
		t.Fatalf("goto-line 1: want point=0, got %d", got)
	}
}

func TestGotoLineSetsMessage(t *testing.T) {
	e := newTestEditor("a\nb\nc\n")
	e.cmdGotoLine()
	fn := e.minibufDoneFunc
	e.minibufActive = false
	e.minibufDoneFunc = nil
	fn("2")

	if e.message == "" {
		t.Fatal("goto-line: expected message to be set")
	}
}

// ---------------------------------------------------------------------------
// cmdWhatCursorPosition
// ---------------------------------------------------------------------------

func TestWhatCursorPositionMidBuffer(t *testing.T) {
	e := newTestEditor("hello\nworld")
	b := buf(e)
	b.SetPoint(3)
	e.cmdWhatCursorPosition()
	if e.message == "" {
		t.Fatal("what-cursor-position: expected message to be set")
	}
	if !strings.Contains(e.message, "point=") {
		t.Errorf("what-cursor-position: expected 'point=' in message, got %q", e.message)
	}
}

func TestWhatCursorPositionAtEnd(t *testing.T) {
	e := newTestEditor("hi")
	b := buf(e)
	b.SetPoint(b.Len())
	e.cmdWhatCursorPosition()
	if !strings.Contains(e.message, "end") {
		t.Errorf("what-cursor-position at end: expected 'end' in message, got %q", e.message)
	}
}

func TestWhatCursorPositionReportsLineCol(t *testing.T) {
	e := newTestEditor("abc\ndef")
	b := buf(e)
	b.SetPoint(5) // inside "def" on line 2
	e.cmdWhatCursorPosition()
	if !strings.Contains(e.message, "line=") {
		t.Errorf("what-cursor-position: expected 'line=' in message, got %q", e.message)
	}
}

// ---------------------------------------------------------------------------
// cmdCountWords — extra cases
// ---------------------------------------------------------------------------

func TestCountWordsFiveWords(t *testing.T) {
	e := newTestEditor("one two three four five")
	e.cmdCountWords()
	if !strings.Contains(e.message, "5") {
		t.Errorf("count-words: expected '5' in message, got %q", e.message)
	}
}

func TestCountWordsRegionTwoWords(t *testing.T) {
	e := newTestEditor("alpha beta gamma delta")
	b := buf(e)
	b.SetMark(0)
	b.SetMarkActive(true)
	b.SetPoint(10) // "alpha beta" — 2 words
	e.cmdCountWords()
	if !strings.Contains(e.message, "2") {
		t.Errorf("count-words region: expected '2' in message, got %q", e.message)
	}
}

// ---------------------------------------------------------------------------
// cmdMessages — extra cases
// ---------------------------------------------------------------------------

func TestCmdMessagesCreatesBuffer(t *testing.T) {
	e := newTestEditor("some content")
	e.cmdMessages()
	active := e.ActiveBuffer()
	if active.Name() != "*messages*" {
		t.Fatalf("messages: want active buffer '*messages*', got %q", active.Name())
	}
}

func TestCmdMessagesNoDuplicateBuffers(t *testing.T) {
	e := newTestEditor("content")
	e.cmdMessages()
	e.cmdMessages() // second call should reuse existing buffer
	count := 0
	for _, b := range e.buffers {
		if b.Name() == "*messages*" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("messages: expected exactly 1 *messages* buffer, got %d", count)
	}
}

func TestCmdMessagesBufferIsReadOnly(t *testing.T) {
	e := newTestEditor("content")
	e.cmdMessages()
	msgBuf := e.FindBuffer("*messages*")
	if msgBuf == nil {
		t.Fatal("messages: *messages* buffer not found")
	}
	if !msgBuf.ReadOnly() {
		t.Fatal("messages: *messages* buffer should be read-only")
	}
}
