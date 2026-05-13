package editor

import (
	"context"
	"strings"
	"testing"

	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/elisp"
)

// ---------------------------------------------------------------------------
// subwordForwardOne
// ---------------------------------------------------------------------------

func TestSubwordForwardOneLowercase(t *testing.T) {
	b := buffer.NewWithContent("*sw*", "hello world")
	// From position 0, lowercase run → stop at space (position 5).
	got := subwordForwardOne(b, 0)
	if got != 5 {
		t.Fatalf("subwordForwardOne lowercase: want 5, got %d", got)
	}
}

func TestSubwordForwardOneCamelCaseLower(t *testing.T) {
	b := buffer.NewWithContent("*sw*", "camelCase")
	// "camel" is a lowercase run → stops at 'C' (position 5).
	got := subwordForwardOne(b, 0)
	if got != 5 {
		t.Fatalf("subwordForwardOne camelCase lower part: want 5, got %d", got)
	}
}

func TestSubwordForwardOneCamelCaseUpper(t *testing.T) {
	b := buffer.NewWithContent("*sw*", "camelCase")
	// Starting at position 5 ('C'), TitleCase "Case" → stop at 9.
	got := subwordForwardOne(b, 5)
	if got != 9 {
		t.Fatalf("subwordForwardOne camelCase upper part: want 9, got %d", got)
	}
}

func TestSubwordForwardOneAllCaps(t *testing.T) {
	b := buffer.NewWithContent("*sw*", "FOO")
	// All-caps run from position 0 → stop at end (3).
	got := subwordForwardOne(b, 0)
	if got != 3 {
		t.Fatalf("subwordForwardOne all-caps: want 3, got %d", got)
	}
}

func TestSubwordForwardOneAllCapsBeforeTitleCase(t *testing.T) {
	b := buffer.NewWithContent("*sw*", "FOOBar")
	// All-caps "FOO" before TitleCase "Bar" → stops at 3.
	got := subwordForwardOne(b, 0)
	if got != 3 {
		t.Fatalf("subwordForwardOne FOOBar: want 3, got %d", got)
	}
}

func TestSubwordForwardOneSkipsLeadingNonWord(t *testing.T) {
	b := buffer.NewWithContent("*sw*", "  hello")
	// Leading spaces skipped, then "hello" → stops at 7.
	got := subwordForwardOne(b, 0)
	if got != 7 {
		t.Fatalf("subwordForwardOne skip spaces: want 7, got %d", got)
	}
}

func TestSubwordForwardOneAtEnd(t *testing.T) {
	b := buffer.NewWithContent("*sw*", "abc")
	got := subwordForwardOne(b, 3)
	if got != 3 {
		t.Fatalf("subwordForwardOne at end: want 3, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// subwordBackwardOne
// ---------------------------------------------------------------------------

func TestSubwordBackwardOneLowercase(t *testing.T) {
	b := buffer.NewWithContent("*sw*", "hello")
	// From end (5) backward through lowercase → stops at 0.
	got := subwordBackwardOne(b, 5)
	if got != 0 {
		t.Fatalf("subwordBackwardOne lowercase: want 0, got %d", got)
	}
}

func TestSubwordBackwardOneCamelCaseUpperPart(t *testing.T) {
	b := buffer.NewWithContent("*sw*", "camelCase")
	// From end (9) backward through "Case" TitleCase → stops at 5.
	got := subwordBackwardOne(b, 9)
	if got != 5 {
		t.Fatalf("subwordBackwardOne camelCase from end: want 5, got %d", got)
	}
}

func TestSubwordBackwardOneCamelCaseLowerPart(t *testing.T) {
	b := buffer.NewWithContent("*sw*", "camelCase")
	// From position 5 backward through "camel" → stops at 0.
	got := subwordBackwardOne(b, 5)
	if got != 0 {
		t.Fatalf("subwordBackwardOne camelCase lower from mid: want 0, got %d", got)
	}
}

func TestSubwordBackwardOneAtStart(t *testing.T) {
	b := buffer.NewWithContent("*sw*", "hello")
	got := subwordBackwardOne(b, 0)
	if got != 0 {
		t.Fatalf("subwordBackwardOne at start: want 0, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// cmdForwardWord — edge cases not covered in commands_test.go
// ---------------------------------------------------------------------------

func TestForwardWordAtEnd(t *testing.T) {
	e := newTestEditor("hello")
	buf(e).SetPoint(5)
	e.cmdForwardWord()
	if got := buf(e).Point(); got != 5 {
		t.Fatalf("forward-word at end: want 5, got %d", got)
	}
}

func TestForwardWordSkipsLeadingPunct(t *testing.T) {
	e := newTestEditor("  hello")
	buf(e).SetPoint(0)
	e.cmdForwardWord()
	// Skips spaces, then consumes "hello" → 7.
	if got := buf(e).Point(); got != 7 {
		t.Fatalf("forward-word skip spaces: want 7, got %d", got)
	}
}

func TestForwardWordWithArg(t *testing.T) {
	e := newTestEditor("one two three")
	buf(e).SetPoint(0)
	e.universalArg = 2
	e.universalArgSet = true
	e.cmdForwardWord()
	// After 2 forward-word from 0: "one"→3, skip space, "two"→7.
	if got := buf(e).Point(); got != 7 {
		t.Fatalf("forward-word C-u 2: want 7, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// cmdBackwardWord — edge cases
// ---------------------------------------------------------------------------

func TestBackwardWordAtStart(t *testing.T) {
	e := newTestEditor("hello")
	buf(e).SetPoint(0)
	e.cmdBackwardWord()
	if got := buf(e).Point(); got != 0 {
		t.Fatalf("backward-word at start: want 0, got %d", got)
	}
}

func TestBackwardWordFromMidWord(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetPoint(8) // inside "world"
	e.cmdBackwardWord()
	if got := buf(e).Point(); got != 6 {
		t.Fatalf("backward-word from mid: want 6, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// cmdScrollUp / cmdScrollDown
// ---------------------------------------------------------------------------

func newMultiLineEditor(lines int) *Editor {
	var sb strings.Builder
	for i := range lines {
		sb.WriteString(strings.Repeat("x", 40))
		if i < lines-1 {
			sb.WriteRune('\n')
		}
	}
	return newTestEditor(sb.String())
}

func TestScrollUpIncreasesScrollLine(t *testing.T) {
	e := newMultiLineEditor(50)
	before := e.activeWin.ScrollLine()
	e.cmdScrollUp()
	after := e.activeWin.ScrollLine()
	if after <= before {
		t.Fatalf("cmdScrollUp: scrollLine should increase; before=%d after=%d", before, after)
	}
}

func TestScrollDownDecreasesScrollLine(t *testing.T) {
	e := newMultiLineEditor(50)
	// Scroll up first so we have room to scroll back down.
	e.activeWin.SetScrollLine(20)
	before := e.activeWin.ScrollLine()
	e.cmdScrollDown()
	after := e.activeWin.ScrollLine()
	if after >= before {
		t.Fatalf("cmdScrollDown: scrollLine should decrease; before=%d after=%d", before, after)
	}
}

func TestScrollDownClampsAtOne(t *testing.T) {
	e := newMultiLineEditor(5)
	// Already at top — scrollLine stays at 1.
	e.activeWin.SetScrollLine(1)
	e.cmdScrollDown()
	if got := e.activeWin.ScrollLine(); got != 1 {
		t.Fatalf("cmdScrollDown at top: want scrollLine=1, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// cmdRecenter
// ---------------------------------------------------------------------------

func TestRecenterDoesNotCrash(t *testing.T) {
	e := newMultiLineEditor(50)
	buf(e).SetPoint(0)
	e.activeWin.SetScrollLine(25)
	e.lastCommand = ""
	// Verify it doesn't panic; scroll line value can vary.
	e.cmdRecenter()
	_ = e.activeWin.ScrollLine()
}

// ---------------------------------------------------------------------------
// cmdNewline — edge cases
// ---------------------------------------------------------------------------

func TestNewlineMultiple(t *testing.T) {
	e := newTestEditor("ab")
	buf(e).SetPoint(1)
	e.universalArg = 2
	e.universalArgSet = true
	e.cmdNewline()
	got := buf(e).String()
	if got != "a\n\nb" {
		t.Fatalf("cmdNewline C-u 2: want %q, got %q", "a\n\nb", got)
	}
}

func TestNewlineReadOnly(t *testing.T) {
	e := newTestEditor("ab")
	buf(e).SetReadOnly(true)
	buf(e).SetPoint(1)
	e.cmdNewline()
	got := buf(e).String()
	if got != "ab" {
		t.Fatalf("cmdNewline read-only: buffer should be unchanged, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// cmdOpenLine — edge case at end
// ---------------------------------------------------------------------------

func TestOpenLineAtEnd(t *testing.T) {
	e := newTestEditor("hello")
	buf(e).SetPoint(5) // end of buffer
	e.cmdOpenLine()
	got := buf(e).String()
	if got != "hello\n" {
		t.Fatalf("cmdOpenLine at end: want %q, got %q", "hello\n", got)
	}
	// Point should still be at 5 (before the inserted newline).
	if pt := buf(e).Point(); pt != 5 {
		t.Fatalf("cmdOpenLine at end: want point=5, got %d", pt)
	}
}

// ---------------------------------------------------------------------------
// cmdTransposeChars — edge cases
// ---------------------------------------------------------------------------

func TestTransposeCharsAtEnd(t *testing.T) {
	e := newTestEditor("abc")
	buf(e).SetPoint(3)
	e.cmdTransposeChars()
	got := buf(e).String()
	if got != "acb" {
		t.Fatalf("cmdTransposeChars at end: want %q, got %q", "acb", got)
	}
}

func TestTransposeCharsAtStart(t *testing.T) {
	e := newTestEditor("ab")
	buf(e).SetPoint(0)
	// Nothing to transpose at position 0 (pt < 1 and not at end with >=2 before).
	e.cmdTransposeChars()
	got := buf(e).String()
	if got != "ab" {
		t.Fatalf("cmdTransposeChars at start: want unchanged %q, got %q", "ab", got)
	}
}

// ---------------------------------------------------------------------------
// cmdKillWord
// ---------------------------------------------------------------------------

func TestKillWordForward(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetPoint(0)
	e.cmdKillWord()
	got := buf(e).String()
	if got != " world" {
		t.Fatalf("cmdKillWord: want %q, got %q", " world", got)
	}
}

func TestKillWordAddsToKillRing(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetPoint(0)
	e.cmdKillWord()
	if len(e.killRing) == 0 {
		t.Fatal("cmdKillWord: kill ring should not be empty")
	}
	if e.killRing[0] != "hello" {
		t.Fatalf("cmdKillWord: kill ring[0] want %q, got %q", "hello", e.killRing[0])
	}
}

func TestKillWordAtEnd(t *testing.T) {
	e := newTestEditor("hello")
	buf(e).SetPoint(5)
	before := len(e.killRing)
	e.cmdKillWord()
	// Nothing killed — kill ring unchanged.
	if len(e.killRing) != before {
		t.Fatal("cmdKillWord at end: kill ring should not grow")
	}
}

// ---------------------------------------------------------------------------
// cmdBackwardKillWord
// ---------------------------------------------------------------------------

func TestBackwardKillWordBasic(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetPoint(11)
	e.cmdBackwardKillWord()
	got := buf(e).String()
	if got != "hello " {
		t.Fatalf("cmdBackwardKillWord: want %q, got %q", "hello ", got)
	}
}

func TestBackwardKillWordAddsToKillRing(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetPoint(11)
	e.cmdBackwardKillWord()
	if len(e.killRing) == 0 {
		t.Fatal("cmdBackwardKillWord: kill ring should not be empty")
	}
	if e.killRing[0] != "world" {
		t.Fatalf("cmdBackwardKillWord: kill ring[0] want %q, got %q", "world", e.killRing[0])
	}
}

func TestBackwardKillWordAtStart(t *testing.T) {
	e := newTestEditor("hello")
	buf(e).SetPoint(0)
	before := len(e.killRing)
	e.cmdBackwardKillWord()
	if len(e.killRing) != before {
		t.Fatal("cmdBackwardKillWord at start: kill ring should not grow")
	}
}

// ---------------------------------------------------------------------------
// cmdKillRegion — extra cases
// ---------------------------------------------------------------------------

func TestKillRegionMarkAfterPoint(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetMark(5)
	buf(e).SetMarkActive(true)
	buf(e).SetPoint(0)
	e.cmdKillRegion()
	got := buf(e).String()
	if got != " world" {
		t.Fatalf("cmdKillRegion mark after point: want %q, got %q", " world", got)
	}
}

func TestKillRegionMarkNotActiveMessage(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetMarkActive(false)
	buf(e).SetPoint(5)
	e.cmdKillRegion()
	// Should produce a message about mark.
	if !strings.Contains(e.message, "Mark") {
		t.Fatalf("cmdKillRegion without active mark: expected message about mark, got %q", e.message)
	}
}

func TestKillRegionDeactivatesMark(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetMark(0)
	buf(e).SetMarkActive(true)
	buf(e).SetPoint(5)
	e.cmdKillRegion()
	if buf(e).MarkActive() {
		t.Fatal("cmdKillRegion: mark should not be active after kill")
	}
}

// ---------------------------------------------------------------------------
// cmdCopyRegionAsKill — extra cases
// ---------------------------------------------------------------------------

func TestCopyRegionAsKillNoDelete(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetMark(0)
	buf(e).SetMarkActive(true)
	buf(e).SetPoint(5)
	e.cmdCopyRegionAsKill()
	if got := buf(e).String(); got != "hello world" {
		t.Fatalf("cmdCopyRegionAsKill: buffer should be unchanged, got %q", got)
	}
}

func TestCopyRegionAsKillMarkNotActiveMessage(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetMarkActive(false)
	e.cmdCopyRegionAsKill()
	if !strings.Contains(e.message, "Mark") {
		t.Fatalf("cmdCopyRegionAsKill without mark: expected message, got %q", e.message)
	}
}

// ---------------------------------------------------------------------------
// cmdYankPop
// ---------------------------------------------------------------------------

func TestYankPopRotatesKillRing(t *testing.T) {
	e := newTestEditor("")
	e.killRing = []string{"third", "second", "first"}
	// Simulate a yank of "third" (index 0) at position 0.
	buf(e).InsertString(0, "third")
	buf(e).SetPoint(5)
	e.lastYankEnd = 5
	e.lastYankLen = 5
	e.yankIdx = 0

	e.cmdYankPop()

	got := buf(e).String()
	if got != "second" {
		t.Fatalf("cmdYankPop: want %q, got %q", "second", got)
	}
}

func TestYankPopEmptyKillRing(t *testing.T) {
	e := newTestEditor("")
	e.cmdYankPop()
	if !strings.Contains(e.message, "Kill ring") {
		t.Fatalf("cmdYankPop empty: expected Kill ring message, got %q", e.message)
	}
}

// ---------------------------------------------------------------------------
// cmdSetMarkCommand — extra cases
// ---------------------------------------------------------------------------

func TestSetMarkCommandSetsMarkAtCurrentPoint(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetPoint(5)
	e.cmdSetMarkCommand()
	if got := buf(e).Mark(); got != 5 {
		t.Fatalf("cmdSetMarkCommand: want mark=5, got %d", got)
	}
	if !buf(e).MarkActive() {
		t.Fatal("cmdSetMarkCommand: mark should be active")
	}
}

func TestSetMarkCommandPopsMarkRingWithUniversalArg(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetPoint(3)
	// Push a previous mark position onto the ring.
	buf(e).PushMarkRing(7)
	e.universalArgSet = true
	e.cmdSetMarkCommand()
	// Point should jump to the popped mark.
	if got := buf(e).Point(); got != 7 {
		t.Fatalf("cmdSetMarkCommand C-u: want point=7, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// cmdExchangePointAndMark — extra cases
// ---------------------------------------------------------------------------

func TestExchangePointAndMarkNoMarkMessage(t *testing.T) {
	e := newTestEditor("hello")
	buf(e).SetPoint(2)
	// Mark is -1 (unset) by default on a fresh buffer.
	e.cmdExchangePointAndMark()
	if !strings.Contains(e.message, "No mark") {
		t.Fatalf("cmdExchangePointAndMark no mark: expected message, got %q", e.message)
	}
}

func TestExchangePointAndMarkActivatesMark(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetPoint(3)
	buf(e).SetMark(8)
	buf(e).SetMarkActive(false)
	e.cmdExchangePointAndMark()
	if !buf(e).MarkActive() {
		t.Fatal("cmdExchangePointAndMark: mark should be active after exchange")
	}
}

// ---------------------------------------------------------------------------
// cmdCommentDwim — extra cases
// ---------------------------------------------------------------------------

func TestCommentDwimDefaultUsesHash(t *testing.T) {
	e := newTestEditor("hello")
	// Default (fundamental) mode uses "#".
	buf(e).SetPoint(0)
	e.cmdCommentDwim()
	got := buf(e).String()
	if !strings.HasPrefix(got, "# ") {
		t.Fatalf("cmdCommentDwim default: want line to start with '# ', got %q", got)
	}
}

func TestCommentDwimAdvancesPoint(t *testing.T) {
	e := newTestEditor("hello")
	buf(e).SetMode("go")
	buf(e).SetPoint(0)
	e.cmdCommentDwim()
	// "// " is 3 runes, so point should advance by 3.
	if got := buf(e).Point(); got != 3 {
		t.Fatalf("cmdCommentDwim: want point=3 after comment, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// isSentenceEnd — extra cases
// ---------------------------------------------------------------------------

func TestIsSentenceEndPeriodFollowedByNewline(t *testing.T) {
	runes := []rune("Hello.\nWorld")
	if !isSentenceEnd(runes, 5) {
		t.Fatal("isSentenceEnd: '.' followed by newline should be sentence end")
	}
}

func TestIsSentenceEndPeriodNotFollowedBySpace(t *testing.T) {
	// "e.g." in the middle — followed by a letter, not whitespace.
	runes := []rune("e.g.test")
	if isSentenceEnd(runes, 1) {
		t.Fatal("isSentenceEnd: '.' followed by letter 'g' should not be sentence end")
	}
}

func TestIsSentenceEndNotForLetter(t *testing.T) {
	runes := []rune("hello")
	if isSentenceEnd(runes, 2) {
		t.Fatal("isSentenceEnd: 'l' should not be a sentence end")
	}
}

func TestIsSentenceEndOutOfRange(t *testing.T) {
	runes := []rune("abc")
	if isSentenceEnd(runes, 10) {
		t.Fatal("isSentenceEnd: out-of-range index should return false")
	}
}

func TestIsSentenceEndAtBufferEnd(t *testing.T) {
	runes := []rune("The end.")
	if !isSentenceEnd(runes, 7) {
		t.Fatal("isSentenceEnd: '.' at end of buffer should be sentence end")
	}
}

// ---------------------------------------------------------------------------
// cmdBeginningOfSentence — extra cases
// ---------------------------------------------------------------------------

func TestBeginningOfSentenceAtBufferStart(t *testing.T) {
	e := newTestEditor("Hello world.")
	buf(e).SetPoint(0)
	e.cmdBeginningOfSentence()
	if got := buf(e).Point(); got != 0 {
		t.Fatalf("cmdBeginningOfSentence at buffer start: want 0, got %d", got)
	}
}

func TestBeginningOfSentenceMovesToSecondSentence(t *testing.T) {
	e := newTestEditor("Hello world. Goodbye world.")
	// Point is inside the second sentence ("Goodbye world.").
	// "Hello world. Goodbye world."
	//  0123456789012345678901234567
	// Second sentence starts at 13; placing point at 20 (inside "world").
	buf(e).SetPoint(20)
	e.cmdBeginningOfSentence()
	got := buf(e).Point()
	// "Hello world. " is 13 chars, so second sentence starts at 13.
	if got != 13 {
		t.Fatalf("cmdBeginningOfSentence: want 13, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// cmdKillSentence — extra cases
// ---------------------------------------------------------------------------

func TestKillSentenceAddsToKillRing(t *testing.T) {
	e := newTestEditor("Hello world. Goodbye.")
	buf(e).SetPoint(0)
	e.cmdKillSentence()
	if len(e.killRing) == 0 {
		t.Fatal("cmdKillSentence: kill ring should not be empty")
	}
	if e.killRing[0] != "Hello world." {
		t.Fatalf("cmdKillSentence: kill ring[0] want %q, got %q", "Hello world.", e.killRing[0])
	}
}

func TestKillSentenceLeavesTail(t *testing.T) {
	e := newTestEditor("Hello world. Goodbye.")
	buf(e).SetPoint(0)
	e.cmdKillSentence()
	got := buf(e).String()
	if got != " Goodbye." {
		t.Fatalf("cmdKillSentence: want %q, got %q", " Goodbye.", got)
	}
}

// ---------------------------------------------------------------------------
// cmdUpcaseWord — extra cases
// ---------------------------------------------------------------------------

func TestUpcaseWordMidBuffer(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetPoint(6)
	e.cmdUpcaseWord()
	got := buf(e).String()
	if got != "hello WORLD" {
		t.Fatalf("cmdUpcaseWord mid: want %q, got %q", "hello WORLD", got)
	}
}

func TestUpcaseWordAdvancesPointPastWord(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetPoint(0)
	e.cmdUpcaseWord()
	if pt := buf(e).Point(); pt != 5 {
		t.Fatalf("cmdUpcaseWord: want point=5, got %d", pt)
	}
}

// ---------------------------------------------------------------------------
// cmdDowncaseWord — extra cases
// ---------------------------------------------------------------------------

func TestDowncaseWordMidWord(t *testing.T) {
	e := newTestEditor("hello WORLD")
	buf(e).SetPoint(6)
	e.cmdDowncaseWord()
	got := buf(e).String()
	if got != "hello world" {
		t.Fatalf("cmdDowncaseWord mid-word: want %q, got %q", "hello world", got)
	}
}

func TestDowncaseWordAdvancesPointPastWord(t *testing.T) {
	e := newTestEditor("HELLO world")
	buf(e).SetPoint(0)
	e.cmdDowncaseWord()
	if pt := buf(e).Point(); pt != 5 {
		t.Fatalf("cmdDowncaseWord: want point=5, got %d", pt)
	}
}

// ---------------------------------------------------------------------------
// cmdCapitalizeWord — extra cases
// ---------------------------------------------------------------------------

func TestCapitalizeWordUpperToTitle(t *testing.T) {
	e := newTestEditor("HELLO world")
	buf(e).SetPoint(0)
	e.cmdCapitalizeWord()
	got := buf(e).String()
	if got != "Hello world" {
		t.Fatalf("cmdCapitalizeWord from upper: want %q, got %q", "Hello world", got)
	}
}

func TestCapitalizeWordLowerToTitle(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetPoint(0)
	e.cmdCapitalizeWord()
	got := buf(e).String()
	if got != "Hello world" {
		t.Fatalf("cmdCapitalizeWord lower: want %q, got %q", "Hello world", got)
	}
}

func TestCapitalizeWordAdvancesPoint(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetPoint(0)
	e.cmdCapitalizeWord()
	if pt := buf(e).Point(); pt != 5 {
		t.Fatalf("cmdCapitalizeWord: want point=5, got %d", pt)
	}
}

// ---------------------------------------------------------------------------
// KillBuffer (underlying mechanic, distinct from cmdKillBuffer's minibuf flow)
// ---------------------------------------------------------------------------

func TestKillBufferSwitchesToScratch(t *testing.T) {
	e := newTestEditor("content")
	activeName := buf(e).Name() // "*test*"

	// KillBuffer is called directly to test the underlying behaviour
	// without minibuffer interaction.
	e.KillBuffer(activeName)

	// After killing the active buffer the window must display something else.
	if e.activeWin.Buf().Name() == activeName {
		t.Fatalf("KillBuffer: active window still shows killed buffer %q", activeName)
	}
}

func TestKillBufferRemovesFromList(t *testing.T) {
	e := newTestEditor("content")
	extra := buffer.NewWithContent("*extra*", "data")
	e.buffers = append(e.buffers, extra)

	e.KillBuffer("*extra*")

	for _, b := range e.buffers {
		if b.Name() == "*extra*" {
			t.Fatal("KillBuffer: killed buffer still in e.buffers")
		}
	}
}

func TestKillBufferInactivePreservesActiveWindow(t *testing.T) {
	e := newTestEditor("content")
	extra := buffer.NewWithContent("*extra*", "data")
	e.buffers = append(e.buffers, extra)

	activeName := buf(e).Name()
	e.KillBuffer("*extra*")

	// Active window should still show the original buffer.
	if e.activeWin.Buf().Name() != activeName {
		t.Fatalf("KillBuffer non-active: active window changed unexpectedly")
	}
}

// ---------------------------------------------------------------------------
// bufferDir
// ---------------------------------------------------------------------------

func TestBufferDirWithFilename(t *testing.T) {
	e := newTestEditor("content")
	buf(e).SetFilename("/home/user/projects/main.go")
	got := e.bufferDir(buf(e))
	if got != "/home/user/projects/" {
		t.Fatalf("bufferDir with filename: want %q, got %q", "/home/user/projects/", got)
	}
}

func TestBufferDirNoFilename(t *testing.T) {
	e := newTestEditor("content")
	// No filename set → falls back to process working directory.
	got := e.bufferDir(buf(e))
	if got == "" {
		t.Fatal("bufferDir no filename: should return non-empty cwd")
	}
	if !strings.HasSuffix(got, "/") {
		t.Fatalf("bufferDir: result should end with '/', got %q", got)
	}
}

func TestBufferDirResultEndsWithSlash(t *testing.T) {
	e := newTestEditor("content")
	buf(e).SetFilename("/tmp/foo/bar.go")
	got := e.bufferDir(buf(e))
	if !strings.HasSuffix(got, "/") {
		t.Fatalf("bufferDir: want trailing '/', got %q", got)
	}
}

// ---------------------------------------------------------------------------
// lastSexp — extra cases
// ---------------------------------------------------------------------------

func TestLastSexpNestedParens(t *testing.T) {
	got := lastSexp("(+ (* 2 3) 4)")
	if got != "(+ (* 2 3) 4)" {
		t.Fatalf("lastSexp nested: want %q, got %q", "(+ (* 2 3) 4)", got)
	}
}

func TestLastSexpTrailingWhitespace(t *testing.T) {
	got := lastSexp("(+ 1 2)   ")
	if got != "(+ 1 2)" {
		t.Fatalf("lastSexp with trailing spaces: want %q, got %q", "(+ 1 2)", got)
	}
}

func TestLastSexpLastOfMultiple(t *testing.T) {
	got := lastSexp("(+ 1 2) (- 3 4)")
	if got != "(- 3 4)" {
		t.Fatalf("lastSexp last of multiple: want %q, got %q", "(- 3 4)", got)
	}
}

func TestLastSexpOnlyWhitespace(t *testing.T) {
	got := lastSexp("   ")
	if got != "" {
		t.Fatalf("lastSexp whitespace only: want empty, got %q", got)
	}
}

func TestLastSexpStringLiteral(t *testing.T) {
	got := lastSexp(`"hello"`)
	if got != `"hello"` {
		t.Fatalf("lastSexp string literal: want %q, got %q", `"hello"`, got)
	}
}

// ---------------------------------------------------------------------------
// cmdEvalLastSexp
// ---------------------------------------------------------------------------

func TestEvalLastSexpSimpleArithmetic(t *testing.T) {
	e := newTestEditor("(+ 1 2)")
	e.lisp = elisp.NewEvaluator()
	buf(e).SetPoint(buf(e).Len())
	e.cmdEvalLastSexp()
	if !strings.Contains(e.message, "3") {
		t.Fatalf("cmdEvalLastSexp (+ 1 2): want message containing '3', got %q", e.message)
	}
}

func TestEvalLastSexpNoSexp(t *testing.T) {
	e := newTestEditor("   ")
	e.lisp = elisp.NewEvaluator()
	buf(e).SetPoint(buf(e).Len())
	e.cmdEvalLastSexp()
	if !strings.Contains(e.message, "No sexp") {
		t.Fatalf("cmdEvalLastSexp no sexp: want 'No sexp' message, got %q", e.message)
	}
}

// ---------------------------------------------------------------------------
// cmdKeyboardQuit
// ---------------------------------------------------------------------------

// newNopCancelEditor returns a test editor with lspOpCancel set to a no-op so
// that cmdKeyboardQuit does not panic when calling e.lspOpCancel().
func newNopCancelEditor(content string) *Editor {
	e := newTestEditor(content)
	_, nopCancel := context.WithCancel(context.Background())
	e.lspOpCancel = nopCancel
	return e
}

func TestCmdKeyboardQuitDeactivatesMark(t *testing.T) {
	e := newNopCancelEditor("hello")
	buf(e).SetMarkActive(true)
	e.cmdKeyboardQuit()
	if buf(e).MarkActive() {
		t.Error("cmdKeyboardQuit: expected mark to be deactivated")
	}
}

func TestCmdKeyboardQuitSetsQuitMessage(t *testing.T) {
	e := newNopCancelEditor("hello")
	e.cmdKeyboardQuit()
	if e.message != "Quit" {
		t.Errorf("cmdKeyboardQuit: message = %q, want %q", e.message, "Quit")
	}
}

func TestCmdKeyboardQuitClearsUniversalArg(t *testing.T) {
	e := newNopCancelEditor("")
	e.universalArg = 16
	e.universalArgSet = true
	e.cmdKeyboardQuit()
	if e.universalArgSet {
		t.Error("cmdKeyboardQuit: expected universalArgSet to be cleared")
	}
	if e.universalArg != 1 {
		t.Errorf("cmdKeyboardQuit: universalArg = %d, want 1", e.universalArg)
	}
}

func TestCmdKeyboardQuitClearsPrefixKeymap(t *testing.T) {
	e := newNopCancelEditor("")
	e.prefixKeymap = e.ctrlXKeymap
	e.prefixKeySeq = "C-x"
	e.cmdKeyboardQuit()
	if e.prefixKeymap != nil {
		t.Error("cmdKeyboardQuit: expected prefixKeymap to be nil")
	}
	if e.prefixKeySeq != "" {
		t.Errorf("cmdKeyboardQuit: prefixKeySeq = %q, want empty", e.prefixKeySeq)
	}
}

func TestCmdKeyboardQuitCancelsIsearch(t *testing.T) {
	e := newNopCancelEditor("hello world")
	// Manually start isearch state.
	e.isearching = true
	e.isearchStr = "hel"
	buf(e).SetPoint(3)
	e.isearchStart = 0
	e.cmdKeyboardQuit()
	if e.isearching {
		t.Error("cmdKeyboardQuit: expected isearching to be false")
	}
	if e.isearchStr != "" {
		t.Errorf("cmdKeyboardQuit: isearchStr = %q, want empty", e.isearchStr)
	}
	// Point should be restored to isearchStart (0).
	if pt := buf(e).Point(); pt != 0 {
		t.Errorf("cmdKeyboardQuit: point = %d, want 0 (isearchStart)", pt)
	}
}

// ---------------------------------------------------------------------------
// cmdUniversalArgument
// ---------------------------------------------------------------------------

func TestCmdUniversalArgumentFirstCall(t *testing.T) {
	e := newTestEditor("")
	// universalArgSet starts false; first call sets to 4.
	e.universalArgSet = false
	e.universalArg = 1
	e.cmdUniversalArgument()
	if e.universalArg != 4 {
		t.Errorf("first C-u: universalArg = %d, want 4", e.universalArg)
	}
	if !e.universalArgSet {
		t.Error("first C-u: universalArgSet should be true")
	}
}

func TestCmdUniversalArgumentSecondCallMultiplies(t *testing.T) {
	e := newTestEditor("")
	e.universalArgSet = true
	e.universalArg = 4
	e.cmdUniversalArgument()
	if e.universalArg != 16 {
		t.Errorf("second C-u: universalArg = %d, want 16", e.universalArg)
	}
}

func TestCmdUniversalArgumentThirdCall(t *testing.T) {
	e := newTestEditor("")
	e.universalArgSet = true
	e.universalArg = 16
	e.cmdUniversalArgument()
	if e.universalArg != 64 {
		t.Errorf("third C-u: universalArg = %d, want 64", e.universalArg)
	}
}

// ---------------------------------------------------------------------------
// cmdSelfInsert
// ---------------------------------------------------------------------------

func TestCmdSelfInsertClearsArg(t *testing.T) {
	e := newTestEditor("")
	e.universalArg = 5
	e.universalArgSet = true
	e.cmdSelfInsert()
	if e.universalArgSet {
		t.Error("cmdSelfInsert: expected universalArgSet to be cleared")
	}
}

// ---------------------------------------------------------------------------
// cmdIndentOrComplete
// ---------------------------------------------------------------------------

func TestCmdIndentOrCompleteGoModeDoesNotPanic(t *testing.T) {
	e := newTestEditor("\tfoo()\n")
	e.lisp = elisp.NewEvaluator()
	buf(e).SetMode("go")
	buf(e).SetPoint(0)
	// Must not panic; just verify the call completes cleanly.
	e.cmdIndentOrComplete()
	_ = buf(e).String()
}

func TestCmdIndentOrCompleteElispMode(t *testing.T) {
	e := newTestEditor("(+ 1 2)\n")
	e.lisp = elisp.NewEvaluator()
	buf(e).SetMode("elisp")
	buf(e).SetPoint(0)
	// Must not panic.
	e.cmdIndentOrComplete()
	_ = buf(e).String()
}

func TestCmdIndentOrCompleteFundamentalMode(t *testing.T) {
	e := newTestEditor("hello\n")
	e.lisp = elisp.NewEvaluator()
	buf(e).SetPoint(0)
	e.cmdIndentOrComplete()
	_ = buf(e).String()
}

// ---------------------------------------------------------------------------
// cmdIsearchForward
// ---------------------------------------------------------------------------

func TestCmdIsearchForwardSetsIsearching(t *testing.T) {
	e := newTestEditor("hello world")
	e.cmdIsearchForward()
	if !e.isearching {
		t.Error("cmdIsearchForward: expected isearching=true")
	}
}

func TestCmdIsearchForwardSetsDirection(t *testing.T) {
	e := newTestEditor("hello world")
	e.cmdIsearchForward()
	if !e.isearchFwd {
		t.Error("cmdIsearchForward: expected isearchFwd=true")
	}
}

func TestCmdIsearchForwardSetsMessage(t *testing.T) {
	e := newTestEditor("hello world")
	e.cmdIsearchForward()
	if !strings.Contains(e.message, "I-search") {
		t.Errorf("cmdIsearchForward: message = %q, want to contain 'I-search'", e.message)
	}
}

// ---------------------------------------------------------------------------
// cmdIsearchBackward
// ---------------------------------------------------------------------------

func TestCmdIsearchBackwardSetsIsearching(t *testing.T) {
	e := newTestEditor("hello world")
	e.cmdIsearchBackward()
	if !e.isearching {
		t.Error("cmdIsearchBackward: expected isearching=true")
	}
}

func TestCmdIsearchBackwardSetsDirection(t *testing.T) {
	e := newTestEditor("hello world")
	e.cmdIsearchBackward()
	if e.isearchFwd {
		t.Error("cmdIsearchBackward: expected isearchFwd=false")
	}
}

func TestCmdIsearchBackwardSetsMessage(t *testing.T) {
	e := newTestEditor("hello world")
	e.cmdIsearchBackward()
	if !strings.Contains(e.message, "backward") {
		t.Errorf("cmdIsearchBackward: message = %q, want to contain 'backward'", e.message)
	}
}

// ---------------------------------------------------------------------------
// cmdSwitchToBuffer
// ---------------------------------------------------------------------------

func TestCmdSwitchToBufferOpensMinibuffer(t *testing.T) {
	e := newTestEditor("")
	e.cmdSwitchToBuffer()
	if !e.minibufActive {
		t.Error("cmdSwitchToBuffer: expected minibufActive=true")
	}
}

func TestCmdSwitchToBufferPromptContainsDefault(t *testing.T) {
	e := newTestEditor("")
	// Add a second buffer so the default name is non-empty.
	extra := buffer.NewWithContent("*other*", "data")
	e.buffers = append(e.buffers, extra)
	e.cmdSwitchToBuffer()
	if !strings.Contains(e.minibufPrompt, "Switch to buffer") {
		t.Errorf("cmdSwitchToBuffer: prompt = %q, want to contain 'Switch to buffer'", e.minibufPrompt)
	}
}

// ---------------------------------------------------------------------------
// cmdKillBuffer
// ---------------------------------------------------------------------------

func TestCmdKillBufferOpensMinibuffer(t *testing.T) {
	e := newTestEditor("hello")
	e.cmdKillBuffer()
	if !e.minibufActive {
		t.Error("cmdKillBuffer: expected minibufActive=true")
	}
}

func TestCmdKillBufferPromptContainsBufferName(t *testing.T) {
	e := newTestEditor("hello")
	e.cmdKillBuffer()
	if !strings.Contains(e.minibufPrompt, buf(e).Name()) {
		t.Errorf("cmdKillBuffer: prompt = %q, want to contain buffer name %q",
			e.minibufPrompt, buf(e).Name())
	}
}

// ---------------------------------------------------------------------------
// cmdExecuteExtendedCommand
// ---------------------------------------------------------------------------

func TestCmdExecuteExtendedCommandOpensMinibuffer(t *testing.T) {
	e := newTestEditor("")
	e.cmdExecuteExtendedCommand()
	if !e.minibufActive {
		t.Error("cmdExecuteExtendedCommand: expected minibufActive=true")
	}
}

func TestCmdExecuteExtendedCommandPromptIsMx(t *testing.T) {
	e := newTestEditor("")
	e.cmdExecuteExtendedCommand()
	if !strings.Contains(e.minibufPrompt, "M-x") {
		t.Errorf("cmdExecuteExtendedCommand: prompt = %q, want to contain 'M-x'", e.minibufPrompt)
	}
}

// ---------------------------------------------------------------------------
// cmdFindFile
// ---------------------------------------------------------------------------

func TestCmdFindFileOpensMinibuffer(t *testing.T) {
	e := newTestEditor("")
	e.cmdFindFile()
	if !e.minibufActive {
		t.Error("cmdFindFile: expected minibufActive=true")
	}
}

func TestCmdFindFilePromptContainsFindFile(t *testing.T) {
	e := newTestEditor("")
	e.cmdFindFile()
	if !strings.Contains(e.minibufPrompt, "Find file") {
		t.Errorf("cmdFindFile: prompt = %q, want to contain 'Find file'", e.minibufPrompt)
	}
}
