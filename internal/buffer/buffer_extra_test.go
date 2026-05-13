package buffer

import (
	"strings"
	"testing"
)

// ---- Substring edge cases --------------------------------------------------

func TestSubstringNegativeStart(t *testing.T) {
	b := NewWithContent("t", "Hello")
	// Negative start should be clamped to 0.
	got := b.Substring(-5, 3)
	if got != "Hel" {
		t.Errorf("Substring(-5,3) = %q, want %q", got, "Hel")
	}
}

func TestSubstringEndBeyondLen(t *testing.T) {
	b := NewWithContent("t", "Hello")
	// end > Len() should be clamped to Len().
	got := b.Substring(3, 100)
	if got != "lo" {
		t.Errorf("Substring(3,100) = %q, want %q", got, "lo")
	}
}

func TestSubstringStartEqualsEnd(t *testing.T) {
	b := NewWithContent("t", "Hello")
	if got := b.Substring(2, 2); got != "" {
		t.Errorf("Substring(2,2) = %q, want empty", got)
	}
}

func TestSubstringStartAfterEnd(t *testing.T) {
	b := NewWithContent("t", "Hello")
	if got := b.Substring(4, 2); got != "" {
		t.Errorf("Substring(4,2) = %q, want empty", got)
	}
}

// Force the gap to be in the middle of the extracted range so the
// "spans the gap" branch of Substring is exercised.
func TestSubstringSpansGap(t *testing.T) {
	b := NewWithContent("t", "abc")
	// Insert at position 1 to move the gap mid-buffer.
	b.Insert(1, 'X')
	// b now contains "aXbc", gap is after position 2.
	got := b.String()
	if got != "aXbc" {
		t.Fatalf("unexpected buffer content %q after insert", got)
	}
	// Extract a range that spans the gap: positions 0..4.
	if sub := b.Substring(0, 4); sub != "aXbc" {
		t.Errorf("Substring(0,4) across gap = %q, want %q", sub, "aXbc")
	}
}

// Range entirely after the gap.
func TestSubstringAfterGap(t *testing.T) {
	b := NewWithContent("t", "Hello World")
	// Move the gap to position 5 by inserting then deleting.
	b.Insert(5, 'X')
	b.Delete(5, 1)
	// Gap is now around position 5; extract a range after it.
	got := b.Substring(6, 11)
	if got != "World" {
		t.Errorf("Substring(6,11) after gap = %q, want %q", got, "World")
	}
}

// Narrowed buffer: Substring still returns raw logical positions.
func TestSubstringNarrowedBuffer(t *testing.T) {
	b := NewWithContent("t", "0123456789")
	b.Narrow(3, 7)
	// Substring uses logical positions, not narrowed ones.
	got := b.Substring(3, 7)
	if got != "3456" {
		t.Errorf("Substring(3,7) in narrowed buffer = %q, want %q", got, "3456")
	}
}

// ---- LineStartsFromPos -----------------------------------------------------

func TestLineStartsFromPosSingleLine(t *testing.T) {
	b := NewWithContent("t", "hello")
	got := b.LineStartsFromPos(1, 0, 1)
	if len(got) != 1 || got[0] != 0 {
		t.Errorf("LineStartsFromPos single line: %v, want [0]", got)
	}
}

func TestLineStartsFromPosMultiLine(t *testing.T) {
	// "abc\nde\nfghi" — line 1 at 0, line 2 at 4, line 3 at 7.
	b := NewWithContent("t", "abc\nde\nfghi")
	got := b.LineStartsFromPos(1, 0, 3)
	want := []int{0, 4, 7}
	for i, w := range want {
		if i >= len(got) || got[i] != w {
			t.Errorf("LineStartsFromPos[%d] = %v, want %d", i, got, w)
		}
	}
}

func TestLineStartsFromPosMidBuffer(t *testing.T) {
	b := NewWithContent("t", "abc\nde\nfghi")
	// Start from line 2 whose known position is 4.
	got := b.LineStartsFromPos(2, 4, 2)
	want := []int{4, 7}
	for i, w := range want {
		if i >= len(got) || got[i] != w {
			t.Errorf("LineStartsFromPos from mid[%d] = %v, want %d", i, got, w)
		}
	}
}

func TestLineStartsFromPosCountZero(t *testing.T) {
	b := NewWithContent("t", "abc\nde")
	got := b.LineStartsFromPos(1, 0, 0)
	if got != nil {
		t.Errorf("LineStartsFromPos count=0 should return nil, got %v", got)
	}
}

func TestLineStartsFromPosCountBeyondLines(t *testing.T) {
	b := NewWithContent("t", "abc\nde")
	// Only 2 lines; request 5 — extras should be Len().
	got := b.LineStartsFromPos(1, 0, 5)
	if len(got) != 5 {
		t.Fatalf("expected length 5, got %d", len(got))
	}
	n := b.Len()
	for _, v := range got[2:] {
		if v != n {
			t.Errorf("past-EOF slot = %d, want %d (Len)", v, n)
		}
	}
}

// Start position is after the gap.
func TestLineStartsFromPosAfterGap(t *testing.T) {
	b := NewWithContent("t", "abc\nde\nfghi")
	// Force gap near position 4 by inserting then deleting.
	b.Insert(4, 'X')
	b.Delete(4, 1)
	// Now request from line 2 (buf pos 4).
	got := b.LineStartsFromPos(2, 4, 2)
	want := []int{4, 7}
	for i, w := range want {
		if i >= len(got) || got[i] != w {
			t.Errorf("after gap: LineStartsFromPos[%d] = %v, want %d", i, got, w)
		}
	}
}

// ---- ModCount --------------------------------------------------------------

func TestModCountInitiallyZero(t *testing.T) {
	b := New("t")
	if b.ModCount() != 0 {
		t.Errorf("initial ModCount = %d, want 0", b.ModCount())
	}
}

func TestModCountIncreasesOnInsert(t *testing.T) {
	b := New("t")
	b.Insert(0, 'A')
	if b.ModCount() != 1 {
		t.Errorf("ModCount after Insert = %d, want 1", b.ModCount())
	}
	b.Insert(1, 'B')
	if b.ModCount() != 2 {
		t.Errorf("ModCount after 2nd Insert = %d, want 2", b.ModCount())
	}
}

func TestModCountIncreasesOnDelete(t *testing.T) {
	b := NewWithContent("t", "Hello")
	initial := b.ModCount()
	b.Delete(0, 1)
	if b.ModCount() != initial+1 {
		t.Errorf("ModCount after Delete = %d, want %d", b.ModCount(), initial+1)
	}
}

func TestModCountIncreasesOnInsertString(t *testing.T) {
	b := New("t")
	b.InsertString(0, "hi")
	if b.ModCount() != 1 {
		t.Errorf("ModCount after InsertString = %d, want 1", b.ModCount())
	}
}

func TestModCountNotAffectedByNewWithContent(t *testing.T) {
	// NewWithContent resets the undo ring but does NOT reset modCount — internal
	// detail. What matters is that subsequent edits increment it.
	b := NewWithContent("t", "abc")
	mc := b.ModCount()
	b.Insert(0, 'X')
	if b.ModCount() != mc+1 {
		t.Errorf("ModCount after Insert on NewWithContent = %d, want %d", b.ModCount(), mc+1)
	}
}

// ---- ChangeGen -------------------------------------------------------------

func TestChangeGenInitiallyZero(t *testing.T) {
	b := NewWithContent("t", "hello")
	if b.ChangeGen() != 0 {
		t.Errorf("initial ChangeGen = %d, want 0", b.ChangeGen())
	}
}

func TestChangeGenIncreasesOnInsert(t *testing.T) {
	b := NewWithContent("t", "hello")
	g0 := b.ChangeGen()
	b.Insert(0, 'X')
	if b.ChangeGen() != g0+1 {
		t.Errorf("ChangeGen after Insert = %d, want %d", b.ChangeGen(), g0+1)
	}
}

func TestChangeGenIncreasesOnDelete(t *testing.T) {
	b := NewWithContent("t", "hello")
	g0 := b.ChangeGen()
	b.Delete(0, 1)
	if b.ChangeGen() != g0+1 {
		t.Errorf("ChangeGen after Delete = %d, want %d", b.ChangeGen(), g0+1)
	}
}

func TestChangeGenDecreasesOnUndo(t *testing.T) {
	b := NewWithContent("t", "hello")
	b.Insert(0, 'X')
	g1 := b.ChangeGen()
	b.ApplyUndo()
	if b.ChangeGen() == g1 {
		t.Errorf("ChangeGen should change after ApplyUndo")
	}
}

func TestChangeGenIncreasesOnReplaceString(t *testing.T) {
	b := NewWithContent("t", "hello")
	g0 := b.ChangeGen()
	b.ReplaceString(0, 2, "HE")
	if b.ChangeGen() != g0+1 {
		t.Errorf("ChangeGen after ReplaceString = %d, want %d", b.ChangeGen(), g0+1)
	}
}

// ---- BeginningOfLine -------------------------------------------------------

func TestBeginningOfLineAtStart(t *testing.T) {
	b := NewWithContent("t", "hello\nworld")
	if got := b.BeginningOfLine(0); got != 0 {
		t.Errorf("BeginningOfLine(0) = %d, want 0", got)
	}
}

func TestBeginningOfLineMidLine(t *testing.T) {
	b := NewWithContent("t", "hello\nworld")
	// Position 3 is 'l' on line 1; beginning of line is 0.
	if got := b.BeginningOfLine(3); got != 0 {
		t.Errorf("BeginningOfLine(3) = %d, want 0", got)
	}
}

func TestBeginningOfLineSecondLine(t *testing.T) {
	b := NewWithContent("t", "hello\nworld")
	// Position 7 is 'o' on line 2; beginning is 6 (after the \n at 5).
	if got := b.BeginningOfLine(7); got != 6 {
		t.Errorf("BeginningOfLine(7) = %d, want 6", got)
	}
}

func TestBeginningOfLineAtNewline(t *testing.T) {
	b := NewWithContent("t", "abc\ndef")
	// Position 3 is the newline; BeginningOfLine should return 0 (start of line 1).
	if got := b.BeginningOfLine(3); got != 0 {
		t.Errorf("BeginningOfLine(3) at newline = %d, want 0", got)
	}
}

func TestBeginningOfLinePastEnd(t *testing.T) {
	b := NewWithContent("t", "abc\ndef")
	// Position beyond Len() should be clamped.
	got := b.BeginningOfLine(b.Len() + 5)
	if got != 4 {
		t.Errorf("BeginningOfLine past end = %d, want 4", got)
	}
}

// ---- EndOfLine -------------------------------------------------------------

func TestEndOfLineAtStart(t *testing.T) {
	b := NewWithContent("t", "hello\nworld")
	// Position 0: EndOfLine is at the newline position 5.
	if got := b.EndOfLine(0); got != 5 {
		t.Errorf("EndOfLine(0) = %d, want 5", got)
	}
}

func TestEndOfLineMidLine(t *testing.T) {
	b := NewWithContent("t", "hello\nworld")
	if got := b.EndOfLine(2); got != 5 {
		t.Errorf("EndOfLine(2) = %d, want 5", got)
	}
}

func TestEndOfLineLastLine(t *testing.T) {
	b := NewWithContent("t", "hello\nworld")
	// Line 2 has no trailing newline; EndOfLine returns Len().
	if got := b.EndOfLine(6); got != b.Len() {
		t.Errorf("EndOfLine(6) = %d, want %d (Len)", got, b.Len())
	}
}

func TestEndOfLineSingleLine(t *testing.T) {
	b := NewWithContent("t", "noNewline")
	if got := b.EndOfLine(0); got != b.Len() {
		t.Errorf("EndOfLine(0) single-line = %d, want %d", got, b.Len())
	}
}

func TestEndOfLinePastEnd(t *testing.T) {
	b := NewWithContent("t", "abc")
	got := b.EndOfLine(b.Len() + 10)
	if got != b.Len() {
		t.Errorf("EndOfLine past end = %d, want %d", got, b.Len())
	}
}

// ---- ReplaceString ---------------------------------------------------------

func TestReplaceStringSameLength(t *testing.T) {
	b := NewWithContent("t", "Hello World")
	b.ReplaceString(6, 5, "Earth")
	got := b.String()
	if got != "Hello Earth" {
		t.Errorf("ReplaceString same length = %q, want %q", got, "Hello Earth")
	}
}

func TestReplaceStringShorter(t *testing.T) {
	b := NewWithContent("t", "Hello World")
	b.ReplaceString(6, 5, "Go")
	got := b.String()
	if got != "Hello Go" {
		t.Errorf("ReplaceString shorter = %q, want %q", got, "Hello Go")
	}
}

func TestReplaceStringLonger(t *testing.T) {
	b := NewWithContent("t", "Hello X")
	b.ReplaceString(6, 1, "World")
	got := b.String()
	if got != "Hello World" {
		t.Errorf("ReplaceString longer = %q, want %q", got, "Hello World")
	}
}

func TestReplaceStringAtStart(t *testing.T) {
	b := NewWithContent("t", "Hello World")
	b.ReplaceString(0, 5, "Hi")
	got := b.String()
	if got != "Hi World" {
		t.Errorf("ReplaceString at start = %q, want %q", got, "Hi World")
	}
}

func TestReplaceStringCountClamped(t *testing.T) {
	b := NewWithContent("t", "abcde")
	// count goes past end; should be clamped.
	b.ReplaceString(3, 100, "XY")
	got := b.String()
	if got != "abcXY" {
		t.Errorf("ReplaceString count clamped = %q, want %q", got, "abcXY")
	}
}

func TestReplaceStringNegativePosNoOp(t *testing.T) {
	b := NewWithContent("t", "hello")
	b.ReplaceString(-1, 2, "ZZ")
	// pos < 0 gets clamped to 0, not a no-op; verify content changed correctly.
	got := b.String()
	if got != "ZZllo" {
		t.Errorf("ReplaceString negative pos = %q, want %q", got, "ZZllo")
	}
}

func TestReplaceStringZeroCountNoOp(t *testing.T) {
	b := NewWithContent("t", "hello")
	b.ReplaceString(2, 0, "ZZ")
	// count <= 0 → no-op.
	if got := b.String(); got != "hello" {
		t.Errorf("ReplaceString count=0 should be no-op, got %q", got)
	}
}

func TestReplaceStringModCount(t *testing.T) {
	b := NewWithContent("t", "hello")
	mc := b.ModCount()
	b.ReplaceString(0, 2, "HE")
	if b.ModCount() != mc+1 {
		t.Errorf("ModCount after ReplaceString = %d, want %d", b.ModCount(), mc+1)
	}
}

func TestReplaceStringUndoable(t *testing.T) {
	b := NewWithContent("t", "hello")
	b.ReplaceString(0, 5, "world")
	if got := b.String(); got != "world" {
		t.Fatalf("after ReplaceString = %q, want %q", got, "world")
	}
	b.ApplyUndo()
	if got := b.String(); got != "hello" {
		t.Errorf("after undo of ReplaceString = %q, want %q", got, "hello")
	}
}

// ---- growGap (indirect via large insertion) --------------------------------

func TestGrowGapForcedByLargeInsert(t *testing.T) {
	b := New("t")
	// The initial gap is initialGapSize (64) runes. Insert more than that in
	// one shot so growGap is called.
	large := strings.Repeat("x", initialGapSize*3)
	b.InsertString(0, large)
	if b.Len() != initialGapSize*3 {
		t.Errorf("Len after large insert = %d, want %d", b.Len(), initialGapSize*3)
	}
	if b.String() != large {
		t.Error("buffer content corrupted after large insert forcing growGap")
	}
}

func TestGrowGapPreservesContent(t *testing.T) {
	b := NewWithContent("t", "start")
	// Now append enough to exceed the gap twice over.
	extra := strings.Repeat("y", initialGapSize*4)
	b.InsertString(b.Len(), extra)
	want := "start" + extra
	if b.String() != want {
		t.Errorf("content mismatch after forced growGap (len=%d, want %d)", b.Len(), len([]rune(want)))
	}
}
