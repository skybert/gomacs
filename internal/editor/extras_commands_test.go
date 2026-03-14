package editor

import (
	"strings"
	"testing"

	"github.com/skybert/gomacs/internal/buffer"
)

// ---------------------------------------------------------------------------
// kill-word / backward-kill-word
// ---------------------------------------------------------------------------

func TestKillWord(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetPoint(0)
	e.cmdKillWord()

	if got := buf(e).String(); got != " world" {
		t.Fatalf("kill-word: want %q, got %q", " world", got)
	}
	if len(e.killRing) == 0 || e.killRing[0] != "hello" {
		kr := ""
		if len(e.killRing) > 0 {
			kr = e.killRing[0]
		}
		t.Fatalf("kill-word: want kill-ring[0]=%q, got %q", "hello", kr)
	}
}

func TestKillWordFromMidWord(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetPoint(2) // inside "hello"
	e.cmdKillWord()

	// Should kill from point to end of word: "llo".
	if got := buf(e).String(); got != "he world" {
		t.Fatalf("kill-word mid: want %q, got %q", "he world", got)
	}
}

func TestBackwardKillWord(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetPoint(11) // end of buffer
	e.cmdBackwardKillWord()

	if got := buf(e).String(); got != "hello " {
		t.Fatalf("backward-kill-word: want %q, got %q", "hello ", got)
	}
	if len(e.killRing) == 0 || e.killRing[0] != "world" {
		kr := ""
		if len(e.killRing) > 0 {
			kr = e.killRing[0]
		}
		t.Fatalf("backward-kill-word: want kill-ring[0]=%q, got %q", "world", kr)
	}
}

// ---------------------------------------------------------------------------
// yank-pop
// ---------------------------------------------------------------------------

func TestYankPop(t *testing.T) {
	e := newTestEditor("")
	e.addToKillRing("first")
	e.addToKillRing("second") // most recent

	// Yank inserts most-recent entry.
	buf(e).SetPoint(0)
	e.cmdYank()
	if got := buf(e).String(); got != "second" {
		t.Fatalf("yank: want %q, got %q", "second", got)
	}

	// yank-pop should replace with next entry ("first").
	e.lastCommand = "yank"
	e.cmdYankPop()
	if got := buf(e).String(); got != "first" {
		t.Fatalf("yank-pop: want %q, got %q", "first", got)
	}
}

func TestYankPopWithoutPriorYank(t *testing.T) {
	e := newTestEditor("hello")
	e.addToKillRing("something")
	// yank-pop with no previous yank (lastYankLen == 0) inserts at point.
	e.lastCommand = "forward-char"
	before := buf(e).String()
	e.cmdYankPop()
	// No yank was done before, so lastYankLen is 0; it inserts at point.
	// Buffer should now contain the kill-ring entry appended.
	if buf(e).String() == before {
		t.Fatal("yank-pop: expected buffer to change (insert from kill ring)")
	}
}

// ---------------------------------------------------------------------------
// beginning-of-buffer / end-of-buffer
// ---------------------------------------------------------------------------

func TestBeginningOfBuffer(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetPoint(8)
	e.cmdBeginningOfBuffer()
	if got := buf(e).Point(); got != 0 {
		t.Fatalf("beginning-of-buffer: want 0, got %d", got)
	}
}

func TestEndOfBuffer(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetPoint(0)
	e.cmdEndOfBuffer()
	if got := buf(e).Point(); got != buf(e).Len() {
		t.Fatalf("end-of-buffer: want %d, got %d", buf(e).Len(), got)
	}
}

// ---------------------------------------------------------------------------
// addToKillRing
// ---------------------------------------------------------------------------

func TestAddToKillRing(t *testing.T) {
	e := newTestEditor("")
	e.addToKillRing("alpha")
	e.addToKillRing("beta")
	if len(e.killRing) < 2 {
		t.Fatalf("kill ring should have 2 entries, got %d", len(e.killRing))
	}
	if e.killRing[0] != "beta" {
		t.Errorf("killRing[0] = %q, want %q", e.killRing[0], "beta")
	}
	if e.killRing[1] != "alpha" {
		t.Errorf("killRing[1] = %q, want %q", e.killRing[1], "alpha")
	}
}

// ---------------------------------------------------------------------------
// delete-char
// ---------------------------------------------------------------------------

func TestDeleteChar(t *testing.T) {
	e := newTestEditor("hello")
	buf(e).SetPoint(0)
	e.cmdDeleteChar()
	if got := buf(e).String(); got != "ello" {
		t.Fatalf("delete-char: want %q, got %q", "ello", got)
	}
}

func TestDeleteCharAtEnd(t *testing.T) {
	e := newTestEditor("hello")
	buf(e).SetPoint(5) // end of buffer
	before := buf(e).String()
	e.cmdDeleteChar()
	if buf(e).String() != before {
		t.Fatalf("delete-char at end: buffer changed unexpectedly")
	}
}

// ---------------------------------------------------------------------------
// next-line / previous-line
// ---------------------------------------------------------------------------

func TestNextLine(t *testing.T) {
	e := newTestEditor("hello\nworld")
	buf(e).SetPoint(0)
	e.activeWin.SetPoint(0)
	e.activeWin.ClearGoalCol()
	e.cmdNextLine()
	// Point should be on line 2.
	line, _ := buf(e).LineCol(buf(e).Point())
	if line != 2 {
		t.Fatalf("next-line: want line 2, got %d", line)
	}
}

func TestPreviousLine(t *testing.T) {
	e := newTestEditor("hello\nworld")
	buf(e).SetPoint(6) // start of "world"
	e.activeWin.SetPoint(6)
	e.activeWin.ClearGoalCol()
	e.cmdPreviousLine()
	line, _ := buf(e).LineCol(buf(e).Point())
	if line != 1 {
		t.Fatalf("previous-line: want line 1, got %d", line)
	}
}

// ---------------------------------------------------------------------------
// switch-to-buffer / kill-buffer
// ---------------------------------------------------------------------------

func TestSwitchToBuffer(t *testing.T) {
	e := newTestEditor("content")
	initial := buf(e).Name()

	e.SwitchToBuffer("*scratch*")
	if got := buf(e).Name(); got == initial {
		t.Fatalf("SwitchToBuffer: buffer did not change from %q", initial)
	}
	if got := buf(e).Name(); got != "*scratch*" {
		t.Fatalf("SwitchToBuffer: want %q, got %q", "*scratch*", got)
	}
}

func TestKillBuffer(t *testing.T) {
	e := newTestEditor("hello")
	// Add a second buffer without switching to it.
	e.buffers = append(e.buffers, buffer.NewWithContent("*extra*", ""))
	countBefore := len(e.buffers)
	e.KillBuffer("*extra*")
	if len(e.buffers) != countBefore-1 {
		t.Fatalf("KillBuffer: expected %d buffers, got %d", countBefore-1, len(e.buffers))
	}
	if e.FindBuffer("*extra*") != nil {
		t.Fatal("KillBuffer: *extra* buffer still exists")
	}
}

// ---------------------------------------------------------------------------
// indent-region (C-M-\)
// ---------------------------------------------------------------------------

func TestIndentCurrentLine(t *testing.T) {
	e := newTestEditor("func foo() {\nx := 1\n}")
	buf(e).SetMode("go")
	// Place point on the second line "x := 1" (position 13).
	buf(e).SetPoint(13)
	e.activeWin.SetPoint(13)
	indentCurrentLine(buf(e), "\t")
	got := buf(e).String()
	if !strings.HasPrefix(strings.SplitN(got, "\n", 3)[1], "\t") {
		t.Fatalf("indent: second line of Go code should start with a tab; got %q", got)
	}
}
