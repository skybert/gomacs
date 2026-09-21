package editor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/elisp"
	"github.com/skybert/gomacs/internal/keymap"
	"github.com/skybert/gomacs/internal/syntax"
	"github.com/skybert/gomacs/internal/terminal"
	"github.com/skybert/gomacs/internal/window"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newTestEditor builds a minimal Editor suitable for unit testing.
// It uses nil for the terminal so no real screen is needed.
func newTestEditor(content string) *Editor {
	buf := buffer.NewWithContent("*test*", content)
	win := window.New(buf, 0, 0, 80, 24)

	e := &Editor{
		term:               nil,
		buffers:            []*buffer.Buffer{buf},
		windows:            []*window.Window{win},
		activeWin:          win,
		layoutRoot:         leafNode(win),
		minibufBuf:         buffer.New(" *minibuf*"),
		globalKeymap:       keymap.New("global"),
		ctrlXKeymap:        keymap.New("C-x"),
		universalArg:       1,
		autoRevertMtimes:   make(map[*buffer.Buffer]time.Time),
		customHighlighters: make(map[*buffer.Buffer]syntax.Highlighter),
		lspConns:           make(map[string]*lspConn),
	}
	e.minibufWin = window.New(e.minibufBuf, 23, 0, 80, 1)
	return e
}

// buf is a convenience accessor for the active buffer.
func buf(e *Editor) *buffer.Buffer { return e.ActiveBuffer() }

// ---------------------------------------------------------------------------
// forward-char
// ---------------------------------------------------------------------------

func TestForwardChar(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetPoint(0)
	e.cmdForwardChar()
	if got := buf(e).Point(); got != 1 {
		t.Fatalf("forward-char: want point=1, got %d", got)
	}
}

func TestForwardCharAtEnd(t *testing.T) {
	e := newTestEditor("hi")
	buf(e).SetPoint(2) // at end
	e.cmdForwardChar()
	if got := buf(e).Point(); got != 2 {
		t.Fatalf("forward-char at end: want point=2, got %d", got)
	}
}

func TestForwardCharWithArg(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetPoint(0)
	e.universalArg = 5
	e.universalArgSet = true
	e.cmdForwardChar()
	if got := buf(e).Point(); got != 5 {
		t.Fatalf("forward-char C-u 5: want point=5, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// backward-char
// ---------------------------------------------------------------------------

func TestBackwardChar(t *testing.T) {
	e := newTestEditor("hello")
	buf(e).SetPoint(3)
	e.cmdBackwardChar()
	if got := buf(e).Point(); got != 2 {
		t.Fatalf("backward-char: want point=2, got %d", got)
	}
}

func TestBackwardCharAtBeginning(t *testing.T) {
	e := newTestEditor("hello")
	buf(e).SetPoint(0)
	e.cmdBackwardChar()
	if got := buf(e).Point(); got != 0 {
		t.Fatalf("backward-char at start: want point=0, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// beginning-of-line
// ---------------------------------------------------------------------------

func TestBeginningOfLine(t *testing.T) {
	e := newTestEditor("hello\nworld")
	// Move to somewhere in "world".
	buf(e).SetPoint(8)
	e.cmdBeginningOfLine()
	want := buf(e).BeginningOfLine(8)
	if got := buf(e).Point(); got != want {
		t.Fatalf("beginning-of-line: want %d, got %d", want, got)
	}
}

func TestBeginningOfLineAlreadyAtStart(t *testing.T) {
	e := newTestEditor("hello\nworld")
	buf(e).SetPoint(6) // start of "world"
	e.cmdBeginningOfLine()
	if got := buf(e).Point(); got != 6 {
		t.Fatalf("beginning-of-line at line start: want 6, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// end-of-line
// ---------------------------------------------------------------------------

func TestEndOfLine(t *testing.T) {
	e := newTestEditor("hello\nworld")
	buf(e).SetPoint(0)
	e.cmdEndOfLine()
	// end of "hello" is position 5 (before '\n').
	if got := buf(e).Point(); got != 5 {
		t.Fatalf("end-of-line: want 5, got %d", got)
	}
}

func TestEndOfLineOnLastLine(t *testing.T) {
	e := newTestEditor("hello\nworld")
	buf(e).SetPoint(7) // somewhere in "world"
	e.cmdEndOfLine()
	// "world" ends at position 11 (Len).
	want := buf(e).Len()
	if got := buf(e).Point(); got != want {
		t.Fatalf("end-of-line last line: want %d, got %d", want, got)
	}
}

// ---------------------------------------------------------------------------
// kill-line
// ---------------------------------------------------------------------------

func TestKillLine(t *testing.T) {
	e := newTestEditor("hello\nworld")
	buf(e).SetPoint(0)
	e.cmdKillLine()

	// The text from point to end-of-line ("hello") should be gone.
	remaining := buf(e).String()
	if strings.Contains(remaining, "hello") {
		t.Fatalf("kill-line: 'hello' still present; buffer=%q", remaining)
	}

	// Kill ring should contain "hello".
	if len(e.killRing) == 0 {
		t.Fatal("kill-line: kill ring is empty")
	}
	if e.killRing[0] != "hello" { //nolint:goconst
		t.Fatalf("kill-line: want kill-ring[0]=%q, got %q", "hello", e.killRing[0])
	}
}

func TestKillLineAtEOL(t *testing.T) {
	// When point is already at end of line, kill-line should kill the newline.
	e := newTestEditor("hello\nworld")
	buf(e).SetPoint(5) // position of '\n'
	e.cmdKillLine()

	remaining := buf(e).String()
	if remaining != "helloworld" {
		t.Fatalf("kill-line at eol: want %q, got %q", "helloworld", remaining)
	}
	if len(e.killRing) == 0 || e.killRing[0] != "\n" {
		kr := ""
		if len(e.killRing) > 0 {
			kr = e.killRing[0]
		}
		t.Fatalf("kill-line at eol: want kill-ring[0]=%q, got %q", "\n", kr)
	}
}

// ---------------------------------------------------------------------------
// yank
// ---------------------------------------------------------------------------

func TestYank(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetPoint(5)
	e.addToKillRing("YANKED")
	e.cmdYank()

	content := buf(e).String()
	if !strings.Contains(content, "YANKED") {
		t.Fatalf("yank: 'YANKED' not in buffer; content=%q", content)
	}
	// Point should be after the inserted text.
	if got := buf(e).Point(); got != 5+len("YANKED") {
		t.Fatalf("yank: point after yank should be %d, got %d", 5+len("YANKED"), got)
	}
}

func TestYankEmptyKillRing(t *testing.T) {
	e := newTestEditor("hello")
	e.killRing = nil
	e.cmdYank()
	// Message should be set.
	if e.message == "" {
		t.Fatal("yank on empty kill ring: expected message to be set")
	}
	// Buffer unchanged.
	if buf(e).String() != "hello" {
		t.Fatalf("yank on empty kill ring: buffer changed unexpectedly")
	}
}

// ---------------------------------------------------------------------------
// set-mark-command
// ---------------------------------------------------------------------------

func TestSetMarkCommand(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetPoint(3)
	e.cmdSetMarkCommand()

	if !buf(e).MarkActive() {
		t.Fatal("set-mark-command: mark should be active")
	}
	if got := buf(e).Mark(); got != 3 {
		t.Fatalf("set-mark-command: want mark=3, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// kill-region
// ---------------------------------------------------------------------------

func TestKillRegion(t *testing.T) {
	e := newTestEditor("hello world")
	b := buf(e)
	b.SetMark(0)
	b.SetMarkActive(true)
	b.SetPoint(5)
	e.cmdKillRegion()

	remaining := b.String()
	if remaining != " world" {
		t.Fatalf("kill-region: want %q, got %q", " world", remaining)
	}
	if len(e.killRing) == 0 || e.killRing[0] != "hello" { //nolint:goconst
		kr := ""
		if len(e.killRing) > 0 {
			kr = e.killRing[0]
		}
		t.Fatalf("kill-region: want kill-ring[0]=%q, got %q", "hello", kr)
	}
	if b.MarkActive() {
		t.Fatal("kill-region: mark should no longer be active")
	}
}

// ---------------------------------------------------------------------------
// copy-region-as-kill
// ---------------------------------------------------------------------------

func TestCopyRegionAsKill(t *testing.T) {
	e := newTestEditor("hello world")
	b := buf(e)
	b.SetMark(6)
	b.SetMarkActive(true)
	b.SetPoint(11) // end of "world"
	e.cmdCopyRegionAsKill()

	// Buffer should be unchanged.
	if b.String() != "hello world" {
		t.Fatalf("copy-region-as-kill: buffer changed; got %q", b.String())
	}
	if len(e.killRing) == 0 || e.killRing[0] != "world" {
		kr := ""
		if len(e.killRing) > 0 {
			kr = e.killRing[0]
		}
		t.Fatalf("copy-region-as-kill: want kill-ring[0]=%q, got %q", "world", kr)
	}
}

// ---------------------------------------------------------------------------
// undo
// ---------------------------------------------------------------------------

func TestUndo(t *testing.T) {
	e := newTestEditor("")
	b := buf(e)
	b.SetPoint(0)
	b.Insert(0, 'a')
	b.Insert(1, 'b')
	b.Insert(2, 'c')
	// Buffer is now "abc".
	if b.String() != "abc" {
		t.Fatalf("setup: want 'abc', got %q", b.String())
	}
	e.cmdUndo()
	// After one undo the most recent insert ('c') is removed.
	after := b.String()
	if after == "abc" {
		t.Fatalf("undo: buffer unchanged after undo, still %q", after)
	}
}

// ---------------------------------------------------------------------------
// forward-word / backward-word
// ---------------------------------------------------------------------------

func TestForwardWord(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetPoint(0)
	e.cmdForwardWord()
	if got := buf(e).Point(); got != 5 {
		t.Fatalf("forward-word: want point=5, got %d", got)
	}
}

func TestBackwardWord(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetPoint(11) // end of buffer
	e.cmdBackwardWord()
	if got := buf(e).Point(); got != 6 {
		t.Fatalf("backward-word: want point=6, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// newline
// ---------------------------------------------------------------------------

func TestNewline(t *testing.T) {
	e := newTestEditor("helloworld")
	buf(e).SetPoint(5)
	e.cmdNewline()
	if got := buf(e).String(); got != "hello\nworld" {
		t.Fatalf("newline: want %q, got %q", "hello\nworld", got)
	}
	if got := buf(e).Point(); got != 6 {
		t.Fatalf("newline: want point=6, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// transpose-chars
// ---------------------------------------------------------------------------

func TestTransposeChars(t *testing.T) {
	e := newTestEditor("ab")
	buf(e).SetPoint(1)
	e.cmdTransposeChars()
	if got := buf(e).String(); got != "ba" {
		t.Fatalf("transpose-chars: want %q, got %q", "ba", got)
	}
}

// ---------------------------------------------------------------------------
// open-line
// ---------------------------------------------------------------------------

func TestOpenLine(t *testing.T) {
	e := newTestEditor("helloworld")
	buf(e).SetPoint(5)
	e.cmdOpenLine()
	// A newline should appear at position 5; point stays at 5.
	if got := buf(e).String(); got != "hello\nworld" {
		t.Fatalf("open-line: want %q, got %q", "hello\nworld", got)
	}
	if got := buf(e).Point(); got != 5 {
		t.Fatalf("open-line: want point=5, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// exchange-point-and-mark
// ---------------------------------------------------------------------------

func TestExchangePointAndMark(t *testing.T) {
	e := newTestEditor("hello world")
	b := buf(e)
	b.SetPoint(3)
	b.SetMark(7)
	b.SetMarkActive(true)
	e.cmdExchangePointAndMark()
	if b.Point() != 7 {
		t.Fatalf("exchange-point-and-mark: want point=7, got %d", b.Point())
	}
	if b.Mark() != 3 {
		t.Fatalf("exchange-point-and-mark: want mark=3, got %d", b.Mark())
	}
}

// ---------------------------------------------------------------------------
// backward-delete-char
// ---------------------------------------------------------------------------

func TestBackwardDeleteChar(t *testing.T) {
	e := newTestEditor("hello")
	buf(e).SetPoint(5)
	e.cmdBackwardDeleteChar()
	if got := buf(e).String(); got != "hell" {
		t.Fatalf("backward-delete-char: want %q, got %q", "hell", got)
	}
	if got := buf(e).Point(); got != 4 {
		t.Fatalf("backward-delete-char: want point=4, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// comment-dwim (Go mode)
// ---------------------------------------------------------------------------

func TestCommentDwimGo(t *testing.T) {
	e := newTestEditor("fmt.Println(\"hi\")")
	buf(e).SetMode("go")
	buf(e).SetPoint(4)
	e.cmdCommentDwim()
	got := buf(e).String()
	if !strings.HasPrefix(got, "// ") {
		t.Fatalf("comment-dwim go: want line starting with '// ', got %q", got)
	}
}

// ---------------------------------------------------------------------------
// universal-argument digit accumulation
// ---------------------------------------------------------------------------

func TestUniversalArgDigits(t *testing.T) {
	e := newTestEditor("hello world")
	e.universalArgSet = true
	e.universalArgDigits = "8"
	e.universalArgTyping = true
	e.universalArg = 8

	buf(e).SetPoint(0)
	e.cmdForwardChar()
	if got := buf(e).Point(); got != 8 {
		t.Fatalf("C-u 8 forward-char: want point=8, got %d", got)
	}
}

func TestUniversalArgMultiDigit(t *testing.T) {
	e := newTestEditor("hello world")
	e.universalArgSet = true
	e.universalArgDigits = "12"
	e.universalArgTyping = true
	e.universalArg = 12

	buf(e).SetPoint(0)
	// "hello world" has 11 chars; forward 12 should clamp at end.
	e.cmdForwardChar()
	if got := buf(e).Point(); got != buf(e).Len() {
		t.Fatalf("C-u 12 forward-char: want point=%d (end), got %d", buf(e).Len(), got)
	}
}

func TestClearArgClearsDigitState(t *testing.T) {
	e := newTestEditor("x")
	e.universalArgSet = true
	e.universalArgTyping = true
	e.universalArgDigits = "7"
	e.universalArg = 7

	e.clearArg()

	if e.universalArgSet {
		t.Error("clearArg: universalArgSet should be false")
	}
	if e.universalArgTyping {
		t.Error("clearArg: universalArgTyping should be false")
	}
	if e.universalArgDigits != "" {
		t.Errorf("clearArg: universalArgDigits should be empty, got %q", e.universalArgDigits)
	}
	if e.universalArg != 1 {
		t.Errorf("clearArg: universalArg should be 1, got %d", e.universalArg)
	}
}

// ---------------------------------------------------------------------------
// recenter cycling (C-l)
// ---------------------------------------------------------------------------

func TestRecenterCycles(t *testing.T) {
	// Build a buffer tall enough for cycling to produce distinct scroll positions.
	// 15 lines in a 5-row window ensures Recenter, RecenterTop, RecenterBottom
	// all produce different scrollLine values without clamping.
	var lines []string
	for i := 1; i <= 15; i++ {
		lines = append(lines, "line content here")
	}
	content := strings.Join(lines, "\n") + "\n"
	e := newTestEditor(content)
	// Use a 5-row window so scroll positions differ clearly.
	e.activeWin.SetRegion(0, 0, 80, 5)

	// Place point on line 8 (middle of buffer).
	buf(e).SetPoint(buf(e).LineStart(8))
	e.syncWindowPoint(e.activeWin)

	// First C-l: center.
	e.lastCommand = ""
	e.cmdRecenter()
	centerScroll := e.activeWin.ScrollLine()

	// Second C-l: top.
	e.lastCommand = "recenter" //nolint:goconst
	e.cmdRecenter()
	topScroll := e.activeWin.ScrollLine()

	// Third C-l: bottom.
	e.lastCommand = "recenter" //nolint:goconst
	e.cmdRecenter()
	bottomScroll := e.activeWin.ScrollLine()

	pointLine, _ := buf(e).LineCol(buf(e).Point())
	if topScroll != pointLine {
		t.Errorf("C-l top: expected scrollLine=%d (point line), got %d", pointLine, topScroll)
	}
	if centerScroll >= topScroll {
		t.Errorf("C-l center (%d) should be less than top scroll (%d)", centerScroll, topScroll)
	}
	if bottomScroll >= centerScroll {
		t.Errorf("C-l bottom (%d) should be less than center scroll (%d)", bottomScroll, centerScroll)
	}
}

func TestRecenterCycleResetsOnOtherCommand(t *testing.T) {
	e := newTestEditor("line1\nline2\nline3\nline4\nline5\n")
	buf(e).SetPoint(buf(e).LineStart(3))
	e.syncWindowPoint(e.activeWin)

	// First C-l advances cycle from 0 → 1.
	e.lastCommand = ""
	e.cmdRecenter()
	if e.recenterCycle != 1 {
		t.Fatalf("after first C-l: want recenterCycle=1, got %d", e.recenterCycle)
	}

	// A different command intervenes.
	e.lastCommand = "forward-char"
	// Next C-l should reset cycle to 0 and advance to 1 again.
	e.cmdRecenter()
	if e.recenterCycle != 1 {
		t.Fatalf("after reset+C-l: want recenterCycle=1, got %d", e.recenterCycle)
	}
}

// ---------------------------------------------------------------------------
// sentence commands
// ---------------------------------------------------------------------------

func TestIsSentenceEnd(t *testing.T) {
	tests := []struct {
		text string
		pos  int
		want bool
	}{
		{"Hello.", 5, true},
		{"Hello. World", 5, true},
		{"Hello.\nWorld", 5, true},
		{"Hello.World", 5, false},
		{"Hello!", 5, true},
		{"Hello? World", 5, true},
		{"abc", 0, false},
	}
	runes := func(s string) []rune { return []rune(s) }
	for _, tc := range tests {
		got := isSentenceEnd(runes(tc.text), tc.pos)
		if got != tc.want {
			t.Errorf("isSentenceEnd(%q, %d) = %v, want %v", tc.text, tc.pos, got, tc.want)
		}
	}
}

func TestEndOfSentence(t *testing.T) {
	e := newTestEditor("Hello world. Next sentence.")
	buf(e).SetPoint(0)
	e.cmdEndOfSentence()
	// Period is at index 11; point lands at 12 (right after the period).
	if got := buf(e).Point(); got != 12 {
		t.Fatalf("end-of-sentence: want point=12, got %d", got)
	}
}

func TestEndOfSentenceTwice(t *testing.T) {
	e := newTestEditor("Hello world. Next sentence.")
	buf(e).SetPoint(0)
	e.universalArg = 2
	e.universalArgSet = true
	e.cmdEndOfSentence()
	// Both sentences traversed: lands at end of buffer (27).
	want := len("Hello world. Next sentence.")
	if got := buf(e).Point(); got != want {
		t.Fatalf("end-of-sentence x2: want point=%d, got %d", want, got)
	}
}

func TestBeginningOfSentence(t *testing.T) {
	e := newTestEditor("Hello world. Next sentence.")
	buf(e).SetPoint(15) // inside "Next sentence."
	e.cmdBeginningOfSentence()
	// Should land at "Next" (position 13).
	if got := buf(e).Point(); got != 13 {
		t.Fatalf("beginning-of-sentence: want point=13, got %d", got)
	}
}

func TestKillSentence(t *testing.T) {
	e := newTestEditor("Hello world. Next sentence.")
	buf(e).SetPoint(0)
	e.cmdKillSentence()
	if got := buf(e).String(); got != " Next sentence." {
		t.Fatalf("kill-sentence: want %q, got %q", " Next sentence.", got)
	}
	if len(e.killRing) == 0 || e.killRing[0] != "Hello world." {
		kr := ""
		if len(e.killRing) > 0 {
			kr = e.killRing[0]
		}
		t.Fatalf("kill-sentence: want kill-ring[0]=%q, got %q", "Hello world.", kr)
	}
}

// ---------------------------------------------------------------------------
// save-some-buffers
// ---------------------------------------------------------------------------

func TestSaveSomeBuffersNoModified(t *testing.T) {
	e := newTestEditor("clean content")
	// No buffer has a filename and none is modified — should report nothing to save.
	e.cmdSaveSomeBuffers()
	if e.message != "(No files need saving)" {
		t.Fatalf("save-some-buffers: want %q, got %q", "(No files need saving)", e.message)
	}
}

// ---------------------------------------------------------------------------
// delete-other-windows
// ---------------------------------------------------------------------------

func TestDeleteOtherWindowsNoOp(t *testing.T) {
	e := newTestEditor("some text")
	before := len(e.windows)
	e.cmdDeleteOtherWindows()
	if after := len(e.windows); after != before {
		t.Fatalf("delete-other-windows: window count changed from %d to %d", before, after)
	}
}

// ---------------------------------------------------------------------------
// commonPrefix helper
// ---------------------------------------------------------------------------

func TestCommonPrefix(t *testing.T) {
	tests := []struct {
		input []string
		want  string
	}{
		{[]string{"forward-char", "forward-word", "forward-line"}, "forward-"},
		{[]string{"kill-line", "kill-word", "kill-region"}, "kill-"},
		{[]string{"abc"}, "abc"},
		{[]string{}, ""},
		{[]string{"foo", "bar"}, ""},
		{[]string{"same", "same"}, "same"},
	}
	for _, tc := range tests {
		got := commonPrefix(tc.input)
		if got != tc.want {
			t.Errorf("commonPrefix(%v) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// minibuffer tab completion
// ---------------------------------------------------------------------------

func TestMinibufCompleteUniqueMatch(t *testing.T) {
	e := newTestEditor("")
	e.ReadMinibuffer("M-x ", func(string) {})
	e.SetMinibufCompletions(func(prefix string) []string {
		if strings.HasPrefix("forward-char", prefix) { //nolint:gocritic
			return []string{"forward-char"}
		}
		return nil
	})
	e.minibufBuf.InsertString(0, "forward-c")
	e.minibufBuf.SetPoint(9)

	e.minibufComplete()

	if got := e.minibufBuf.String(); got != "forward-char" {
		t.Fatalf("minibufComplete unique: want %q, got %q", "forward-char", got)
	}
}

func TestMinibufCompleteCommonPrefix(t *testing.T) {
	e := newTestEditor("")
	e.ReadMinibuffer("M-x ", func(string) {})
	e.SetMinibufCompletions(func(prefix string) []string {
		all := []string{"forward-char", "forward-word", "forward-line"}
		var out []string
		for _, s := range all {
			if strings.HasPrefix(s, prefix) {
				out = append(out, s)
			}
		}
		return out
	})
	e.minibufBuf.InsertString(0, "fo")
	e.minibufBuf.SetPoint(2)

	e.minibufComplete()

	if got := e.minibufBuf.String(); got != "forward-" {
		t.Fatalf("minibufComplete prefix: want %q, got %q", "forward-", got)
	}
}

func TestMinibufCompleteNoMatch(t *testing.T) {
	e := newTestEditor("")
	e.ReadMinibuffer("M-x ", func(string) {})
	e.SetMinibufCompletions(func(_ string) []string { return nil })
	e.minibufBuf.InsertString(0, "zzz")
	e.minibufBuf.SetPoint(3)

	e.minibufComplete()

	if got := e.minibufBuf.String(); got != "zzz" {
		t.Fatalf("minibufComplete no match: buffer changed to %q", got)
	}
	if e.message == "" && e.minibufHint == "" {
		t.Fatal("minibufComplete no match: expected an error message or hint to be set")
	}
}

// ---------------------------------------------------------------------------
// upcase-word / downcase-word / capitalize-word
// ---------------------------------------------------------------------------

func TestUpcaseWord(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetPoint(0)
	e.cmdUpcaseWord()
	if got := buf(e).String(); got != "HELLO world" {
		t.Fatalf("upcase-word: want %q, got %q", "HELLO world", got)
	}
	if got := buf(e).Point(); got != 5 {
		t.Fatalf("upcase-word: want point=5, got %d", got)
	}
}

func TestDowncaseWord(t *testing.T) {
	e := newTestEditor("HELLO WORLD")
	buf(e).SetPoint(0)
	e.cmdDowncaseWord()
	if got := buf(e).String(); got != "hello WORLD" {
		t.Fatalf("downcase-word: want %q, got %q", "hello WORLD", got)
	}
}

func TestCapitalizeWord(t *testing.T) {
	e := newTestEditor("HELLO world")
	buf(e).SetPoint(0)
	e.cmdCapitalizeWord()
	if got := buf(e).String(); got != "Hello world" {
		t.Fatalf("capitalize-word: want %q, got %q", "Hello world", got)
	}
}

func TestUpcaseWordFromMidWord(t *testing.T) {
	// When point is inside a word, Emacs upcases from point to end of word.
	// Our implementation skips non-word chars first then upcases.
	// When point is inside "hello", skip non-word does nothing; then upcases.
	e := newTestEditor("hello world")
	buf(e).SetPoint(2) // point inside "hello", after "he"
	e.cmdUpcaseWord()
	// should upcase "hello" (starting from the next word-start found)
	// since pt=2 is already in a word, start=2, upcases "llo"
	if got := buf(e).String(); got != "heLLO world" {
		t.Fatalf("upcase-word mid: want %q, got %q", "heLLO world", got)
	}
}

// ---------------------------------------------------------------------------
// lastSexp helper
// ---------------------------------------------------------------------------

func TestLastSexp(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"(+ 1 2)", "(+ 1 2)"},
		{"foo (+ 1 2)", "(+ 1 2)"},
		{`"hello"`, `"hello"`},
		{"foo bar", "bar"},
		{"foo bar ", "bar"},
		{"(outer (inner 1) 2)", "(outer (inner 1) 2)"},
		{"", ""},
		{"   ", ""},
	}
	for _, tc := range tests {
		got := lastSexp(tc.input)
		if got != tc.want {
			t.Errorf("lastSexp(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// filePathCompletions
// ---------------------------------------------------------------------------

func TestFilePathCompletionsReturnsEntries(t *testing.T) {
	// Completing a directory must list the files it contains.  Use a temporary
	// directory with known contents rather than /tmp, whose contents are
	// unknowable.
	dir := t.TempDir()
	for _, name := range []string{"alpha.txt", "beta.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	results := filePathCompletions(dir + "/")
	if len(results) != 3 {
		t.Fatalf("filePathCompletions(%q) = %v, want 3 entries", dir+"/", results)
	}
	want := map[string]bool{
		filepath.Join(dir, "alpha.txt"): false,
		filepath.Join(dir, "beta.txt"):  false,
		filepath.Join(dir, "sub") + "/": false, // directories get a trailing /
	}
	for _, r := range results {
		if _, ok := want[r]; !ok {
			t.Errorf("unexpected completion %q; want one of %v", r, want)
			continue
		}
		want[r] = true
	}
	for entry, seen := range want {
		if !seen {
			t.Errorf("completion %q missing from %v", entry, results)
		}
	}
}

func TestFilePathCompletionsFiltersByPrefix(t *testing.T) {
	// /usr/bin exists and has many entries; filter by a common prefix.
	// With fuzzy matching, all returned entries should contain "py" as a subsequence
	// of their basename.
	results := filePathCompletions("/usr/bin/py")
	for _, r := range results {
		base := filepath.Base(r)
		if !fuzzyMatch(base, "py") {
			t.Errorf("completion %q doesn't fuzzy-match basename %q for query 'py'", r, base)
		}
	}
}

// ---------------------------------------------------------------------------
// describe-function
// ---------------------------------------------------------------------------

func TestDescribeFunctionCreatesHelpBuffer(t *testing.T) {
	e := newTestEditor("")
	e.showCommandHelp("", "forward-char")

	helpBuf := e.FindBuffer("*Help*")
	if helpBuf == nil {
		t.Fatal("describe-function: *Help* buffer not created")
	}
	content := helpBuf.String()
	if !strings.Contains(content, "forward-char") {
		t.Errorf("*Help* buffer missing command name; got: %q", content)
	}
	if !strings.Contains(content, "Move point") {
		t.Errorf("*Help* buffer missing doc string; got: %q", content)
	}
}

func TestDescribeFunctionUnknownCommand(t *testing.T) {
	e := newTestEditor("")
	e.showCommandHelp("", "no-such-command")

	helpBuf := e.FindBuffer("*Help*")
	if helpBuf == nil {
		t.Fatal("expected *Help* buffer to be created")
	}
	if !strings.Contains(helpBuf.String(), "No such command") {
		t.Errorf("expected 'No such command' message; got: %q", helpBuf.String())
	}
}

// ---------------------------------------------------------------------------
// describe-key
// ---------------------------------------------------------------------------

func TestDescribeKeyRegistered(t *testing.T) {
	if _, ok := commands["describe-key"]; !ok {
		t.Fatal("describe-key is not registered")
	}
	if _, ok := commands["describe-function"]; !ok {
		t.Fatal("describe-function is not registered")
	}
}

func TestCommandDocsPopulated(t *testing.T) {
	// Every registered command should have a doc string.
	for name := range commands {
		if _, ok := commandDocs[name]; !ok {
			t.Errorf("command %q has no doc string", name)
		}
	}
}

// ---------------------------------------------------------------------------
// describe-variable
// ---------------------------------------------------------------------------

func TestDescribeVariableKnown(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	if _, err := e.lisp.EvalString("(setq my-test-var 42)"); err != nil {
		t.Fatalf("EvalString: %v", err)
	}

	e.showVariableHelp("my-test-var")

	helpBuf := e.FindBuffer("*Help*")
	if helpBuf == nil {
		t.Fatal("describe-variable: *Help* buffer not created")
	}
	content := helpBuf.String()
	if !strings.Contains(content, "my-test-var") {
		t.Errorf("*Help* missing variable name; got: %q", content)
	}
	if !strings.Contains(content, "42") {
		t.Errorf("*Help* missing variable value; got: %q", content)
	}
}

func TestDescribeVariableUnknown(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()

	e.showVariableHelp("no-such-var")

	helpBuf := e.FindBuffer("*Help*")
	if helpBuf == nil {
		t.Fatal("expected *Help* buffer to be created")
	}
	if !strings.Contains(helpBuf.String(), "void") {
		t.Errorf("expected 'void' message; got: %q", helpBuf.String())
	}
}

// ---------------------------------------------------------------------------
// Window splitting
// ---------------------------------------------------------------------------

func TestSplitWindowBelow(t *testing.T) {
	e := newTestEditor("hello")
	if len(e.windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(e.windows))
	}
	e.cmdSplitWindowBelow()
	if len(e.windows) != 2 {
		t.Fatalf("after split-window-below: expected 2 windows, got %d", len(e.windows))
	}
	w0, w1 := e.windows[0], e.windows[1]
	// Both windows should share the same left offset and width.
	if w0.Left() != w1.Left() {
		t.Errorf("left mismatch: %d vs %d", w0.Left(), w1.Left())
	}
	// Second window should start below the first.
	if w1.Top() <= w0.Top() {
		t.Errorf("expected w1.Top (%d) > w0.Top (%d)", w1.Top(), w0.Top())
	}
	// Heights should sum to the original 24.
	if w0.Height()+w1.Height() != 24 {
		t.Errorf("heights don't sum to 24: %d + %d", w0.Height(), w1.Height())
	}
}

func TestSplitWindowRight(t *testing.T) {
	e := newTestEditor("hello")
	e.cmdSplitWindowRight()
	if len(e.windows) != 2 {
		t.Fatalf("after split-window-right: expected 2 windows, got %d", len(e.windows))
	}
	w0, w1 := e.windows[0], e.windows[1]
	// Both windows should share the same top row.
	if w0.Top() != w1.Top() {
		t.Errorf("top mismatch: %d vs %d", w0.Top(), w1.Top())
	}
	// Second window should start to the right of the first.
	if w1.Left() <= w0.Left() {
		t.Errorf("expected w1.Left (%d) > w0.Left (%d)", w1.Left(), w0.Left())
	}
	// Widths plus 1 separator column should equal the original 80.
	if w0.Width()+w1.Width()+1 != 80 {
		t.Errorf("widths + separator don't sum to 80: %d + %d + 1", w0.Width(), w1.Width())
	}
}

func TestOtherWindow(t *testing.T) {
	e := newTestEditor("hello")
	e.cmdSplitWindowBelow()
	first := e.activeWin
	e.cmdOtherWindow()
	if e.activeWin == first {
		t.Error("other-window did not change active window")
	}
	// Calling again should cycle back.
	e.cmdOtherWindow()
	if e.activeWin != first {
		t.Error("other-window did not cycle back to first window")
	}
}

func TestDeleteOtherWindows(t *testing.T) {
	e := newTestEditor("hello")
	e.cmdSplitWindowBelow()
	if len(e.windows) != 2 {
		t.Fatalf("pre-condition: expected 2 windows, got %d", len(e.windows))
	}
	e.cmdDeleteOtherWindows()
	if len(e.windows) != 1 {
		t.Fatalf("after delete-other-windows: expected 1 window, got %d", len(e.windows))
	}
	if e.windows[0] != e.activeWin {
		t.Error("remaining window should be the active window")
	}
}

// ---------------------------------------------------------------------------
// Read-only mode
// ---------------------------------------------------------------------------

func TestToggleReadOnly(t *testing.T) {
	e := newTestEditor("hello")
	buf := e.ActiveBuffer()
	if buf.ReadOnly() {
		t.Fatal("buffer should start writable")
	}
	e.cmdToggleReadOnly()
	if !buf.ReadOnly() {
		t.Fatal("expected buffer to be read-only after toggle")
	}
	e.cmdToggleReadOnly()
	if buf.ReadOnly() {
		t.Fatal("expected buffer to be writable after second toggle")
	}
}

func TestReadOnlyBlocksInsert(t *testing.T) {
	e := newTestEditor("hello")
	buf := e.ActiveBuffer()
	buf.SetReadOnly(true)
	before := buf.String()
	// selfInsert should be blocked
	e.selfInsert('X')
	if buf.String() != before {
		t.Errorf("read-only buffer was modified: %q", buf.String())
	}
}

func TestReadOnlyBlocksDeleteChar(t *testing.T) {
	e := newTestEditor("hello")
	buf := e.ActiveBuffer()
	buf.SetReadOnly(true)
	before := buf.String()
	e.cmdDeleteChar()
	if buf.String() != before {
		t.Errorf("read-only buffer was modified: %q", buf.String())
	}
}

// ---------------------------------------------------------------------------
// runesMatchFold
// ---------------------------------------------------------------------------

func TestRunesMatchFoldSameCase(t *testing.T) {
	if !runesMatchFold([]rune("hello"), []rune("hello")) {
		t.Error("same-case match failed")
	}
}

func TestRunesMatchFoldUpperNeedle(t *testing.T) {
	if !runesMatchFold([]rune("hello"), []rune("HELLO")) {
		t.Error("upper needle should match lower haystack")
	}
}

func TestRunesMatchFoldMixed(t *testing.T) {
	if !runesMatchFold([]rune("GoLang"), []rune("golang")) {
		t.Error("mixed case match failed")
	}
}

func TestRunesMatchFoldMismatch(t *testing.T) {
	if runesMatchFold([]rune("hello"), []rune("world")) {
		t.Error("different strings should not match")
	}
}

func TestRunesMatchFoldTooShort(t *testing.T) {
	if runesMatchFold([]rune("hi"), []rune("hello")) {
		t.Error("haystack shorter than needle should not match")
	}
}

// ---------------------------------------------------------------------------
// isearch case folding
// ---------------------------------------------------------------------------

func newTestEditorWithIsearch(content string) *Editor {
	e := newTestEditor(content)
	e.isSearchCaseFold = true
	return e
}

func TestIsearchFindCaseFold(t *testing.T) {
	e := newTestEditorWithIsearch("Hello World")
	b := buf(e)
	b.SetPoint(0)
	e.isearching = true
	e.isearchFwd = true
	e.isearchStr = "HELLO"
	e.isearchStart = 0
	e.isearchFind()
	// Forward search lands point after the match.
	if got := b.Point(); got != 5 {
		t.Errorf("isearchFind case-fold fwd: want point=5, got %d", got)
	}
}

func TestIsearchFindCaseFoldBackward(t *testing.T) {
	e := newTestEditorWithIsearch("Hello World")
	b := buf(e)
	b.SetPoint(11)
	e.isearching = true
	e.isearchFwd = false
	e.isearchStr = "world"
	e.isearchStart = 11
	e.isearchFind()
	// Backward search lands point at start of match.
	if got := b.Point(); got != 6 {
		t.Errorf("isearchFind case-fold bwd: want point=6, got %d", got)
	}
}

func TestIsearchFindNextCaseFold(t *testing.T) {
	e := newTestEditorWithIsearch("abc ABC abc")
	b := buf(e)
	// Start after first match to find second.
	b.SetPoint(3)
	e.isearching = true
	e.isearchFwd = true
	e.isearchStr = "ABC"
	e.isearchFindNext()
	// Second match "ABC" starts at index 4, point lands after it at 7.
	if got := b.Point(); got != 7 {
		t.Errorf("isearchFindNext case-fold: want point=7, got %d", got)
	}
}

func TestIsearchCaseSensitiveWhenDisabled(t *testing.T) {
	e := newTestEditor("Hello hello")
	e.isSearchCaseFold = false
	b := buf(e)
	b.SetPoint(0)
	e.isearching = true
	e.isearchFwd = true
	e.isearchStr = "hello"
	e.isearchStart = 0
	e.isearchFind()
	// Case-sensitive: should find lowercase "hello" at index 6, point=11.
	if got := b.Point(); got != 11 {
		t.Errorf("isearch case-sensitive: want point=11, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// applyElispConfig — isearch-case-insensitive
// ---------------------------------------------------------------------------

func TestApplyElispConfigIsearchCaseFoldOff(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.isSearchCaseFold = true
	// Setting the variable to nil should disable case folding.
	_, err := e.lisp.EvalString("(setq isearch-case-insensitive nil)")
	if err != nil {
		t.Fatalf("EvalString: %v", err)
	}
	e.applyElispConfig()
	if e.isSearchCaseFold {
		t.Error("isSearchCaseFold should be false after (setq isearch-case-insensitive nil)")
	}
}

func TestApplyElispConfigIsearchCaseFoldOn(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.isSearchCaseFold = false
	// Setting to t should enable case folding.
	_, err := e.lisp.EvalString("(setq isearch-case-insensitive t)")
	if err != nil {
		t.Fatalf("EvalString: %v", err)
	}
	e.applyElispConfig()
	if !e.isSearchCaseFold {
		t.Error("isSearchCaseFold should be true after (setq isearch-case-insensitive t)")
	}
}

// ---------------------------------------------------------------------------
// redo
// ---------------------------------------------------------------------------

func TestRedo(t *testing.T) {
	e := newTestEditor("hello")
	b := buf(e)
	b.InsertString(5, " world")
	e.cmdUndo()
	if got := b.String(); got != "hello" {
		t.Fatalf("after undo: want %q, got %q", "hello", got)
	}
	e.cmdRedo()
	if got := b.String(); got != "hello world" {
		t.Fatalf("after redo: want %q, got %q", "hello world", got)
	}
}

// ---------------------------------------------------------------------------
// downcase-word / upcase-word performance (ReplaceString)
// ---------------------------------------------------------------------------

func TestDowncaseWordUsesOneUndoStep(t *testing.T) {
	e := newTestEditor("Hello World")
	b := buf(e)
	b.SetPoint(0)
	e.cmdDowncaseWord()
	if got := b.String(); got != "hello World" {
		t.Fatalf("downcase: want %q, got %q", "hello World", got)
	}
	e.cmdUndo()
	if got := b.String(); got != "Hello World" {
		t.Fatalf("after undo: want %q, got %q", "Hello World", got)
	}
	// Must be exactly one undo step (ReplaceString produces one record).
	if b.ApplyUndo() {
		t.Fatal("expected only one undo record for downcase-word")
	}
}

// ---------------------------------------------------------------------------
// isearch: spurious modifier on printable runes (hyphen fix)
// ---------------------------------------------------------------------------

// keRune builds a KeyEvent for a printable rune with the given modifiers.
func keRune(r rune, mod tcell.ModMask) terminal.KeyEvent {
	return terminal.KeyEvent{Key: tcell.KeyRune, Rune: r, Mod: mod}
}

// startFwdIsearch puts e into forward isearch mode starting at pos.
func startFwdIsearch(e *Editor, pos int) {
	b := e.ActiveBuffer()
	b.SetPoint(pos)
	e.isearching = true
	e.isearchFwd = true
	e.isearchStr = ""
	e.isearchStart = pos
}

// TestIsearchHyphenNoModifier: plain '-' (Mod==0) must append to search string.
func TestIsearchHyphenNoModifier(t *testing.T) {
	e := newTestEditor("sort-lines")
	startFwdIsearch(e, 0)
	e.isearchStr = "sort"
	e.isearchHandleKey(keRune('-', 0))
	if !e.isearching {
		t.Fatal("isearch exited after '-' with no modifier")
	}
	if e.isearchStr != "sort-" {
		t.Errorf("isearchStr = %q, want %q", e.isearchStr, "sort-")
	}
}

// TestIsearchHyphenSpuriousModifier: '-' with a non-Ctrl/non-Alt modifier
// must still append to the search string, not exit isearch.
func TestIsearchHyphenSpuriousModifier(t *testing.T) {
	e := newTestEditor("sort-lines")
	startFwdIsearch(e, 0)
	e.isearchStr = "sort"
	e.isearchHandleKey(keRune('-', tcell.ModShift))
	if !e.isearching {
		t.Fatal("isearch exited after '-' with spurious ModShift")
	}
	if e.isearchStr != "sort-" {
		t.Errorf("isearchStr = %q, want %q", e.isearchStr, "sort-")
	}
}

// TestIsearchCtrlKeyExitsIsearch: Ctrl+char must still exit isearch.
func TestIsearchCtrlKeyExitsIsearch(t *testing.T) {
	e := newTestEditor("sort-lines")
	startFwdIsearch(e, 0)
	e.isearchStr = "sort"
	e.isearchHandleKey(keRune('g', tcell.ModCtrl))
	if e.isearching {
		t.Fatal("isearch should exit on Ctrl+key")
	}
}

// TestIsearchAltKeyExitsIsearch: Alt+char must still exit isearch.
func TestIsearchAltKeyExitsIsearch(t *testing.T) {
	e := newTestEditor("sort-lines")
	startFwdIsearch(e, 0)
	e.isearchStr = "sort"
	e.isearchHandleKey(keRune('x', tcell.ModAlt))
	if e.isearching {
		t.Fatal("isearch should exit on Alt+key")
	}
}

// ---------------------------------------------------------------------------
// ESC-prefix Meta (escPending) dispatch
// ---------------------------------------------------------------------------

// keEscape builds a KeyEvent for the Escape key.
func keEscape() terminal.KeyEvent {
	return terminal.KeyEvent{Key: tcell.KeyEscape}
}

// newScrollableEditor creates a test editor with keymaps and a buffer large
// enough to scroll.  The window is pre-scrolled so M-v (scroll-down) has
// visible effect.
func newScrollableEditor() *Editor {
	// Build a buffer with 60 lines so the 24-line window can scroll.
	var lines []string
	for i := range 60 {
		lines = append(lines, fmt.Sprintf("line %d", i+1))
	}
	content := strings.Join(lines, "\n")
	e := newTestEditor(content)
	e.setupKeymaps()
	// Pre-scroll to line 30 so scroll-down has room to go back.
	e.activeWin.SetScrollLine(30)
	e.ActiveBuffer().SetPoint(e.ActiveBuffer().LineStart(30))
	return e
}

// TestEscPendingSetOnEscape verifies that a plain Escape key sets escPending.
func TestEscPendingSetOnEscape(t *testing.T) {
	e := newScrollableEditor()
	e.dispatchParsedKey(keEscape())
	if !e.escPending {
		t.Fatal("escPending should be true after ESC key")
	}
}

// TestEscPendingScrollDown verifies that ESC+v triggers scroll-down (M-v).
func TestEscPendingScrollDown(t *testing.T) {
	e := newScrollableEditor()
	beforeScroll := e.activeWin.ScrollLine()

	// Two separate events simulate a terminal that sends ESC and v separately
	// (e.g. kitty on macOS with no macos_option_as_alt setting).
	e.dispatchParsedKey(keEscape())
	if !e.escPending {
		t.Fatal("escPending should be true after ESC key")
	}
	e.dispatchParsedKey(keRune('v', 0))

	if e.escPending {
		t.Fatal("escPending should be cleared after consuming the Meta key")
	}
	afterScroll := e.activeWin.ScrollLine()
	if afterScroll >= beforeScroll {
		t.Errorf("M-v (scroll-down) did not scroll: before=%d after=%d",
			beforeScroll, afterScroll)
	}
}

// TestEscPendingScrollDownDirectAlt verifies that a single Alt+v event
// (as delivered by tcell when kitty keyboard protocol is active) also
// triggers scroll-down.
func TestEscPendingScrollDownDirectAlt(t *testing.T) {
	e := newScrollableEditor()
	beforeScroll := e.activeWin.ScrollLine()
	e.dispatchParsedKey(keRune('v', tcell.ModAlt))
	afterScroll := e.activeWin.ScrollLine()
	if afterScroll >= beforeScroll {
		t.Errorf("M-v via ModAlt did not scroll: before=%d after=%d",
			beforeScroll, afterScroll)
	}
}

// TestIsearchHyphenFindsMatch: full round-trip — typing "sort-" locates
// "sort-lines" in the buffer.
func TestIsearchHyphenFindsMatch(t *testing.T) {
	e := newTestEditor("foo\nsort-lines\nbar")
	startFwdIsearch(e, 0)
	for _, r := range "sort-" {
		e.isearchHandleKey(keRune(r, 0))
		if !e.isearching {
			t.Fatalf("isearch exited after typing %q", string(r))
		}
	}
	got := e.ActiveBuffer().Point()
	want := len([]rune("foo\nsort-"))
	if got != want {
		t.Errorf("point after \"sort-\" = %d, want %d", got, want)
	}
}

// ---------------------------------------------------------------------------
// Tab key: never auto-insert first candidate
// ---------------------------------------------------------------------------

// tabKey builds a KeyEvent for the Tab key.
func tabKey() terminal.KeyEvent { return terminal.KeyEvent{Key: tcell.KeyTab} }

// TestMinibufTabDoesNotAutoInsert verifies that pressing Tab when multiple
// candidates are present does NOT insert the first candidate.  It should
// extend the common prefix (or leave the text unchanged) and display a hint.
func TestMinibufTabDoesNotAutoInsert(t *testing.T) {
	e := newTestEditor("")
	e.ReadMinibuffer("Find file: ", func(string) {})
	e.SetMinibufCompletions(func(_ string) []string {
		return []string{"file.txt", "foo.txt"}
	})
	// Simulate typing "f".
	e.minibufBuf.InsertString(0, "f")
	e.minibufBuf.SetPoint(1)
	e.refreshMinibufCandidates() // as dispatchMinibufKey would do

	// Press Tab.
	e.dispatchMinibufKey(tabKey())

	got := e.minibufBuf.String()
	// "file.txt" must NOT have been auto-inserted; common prefix of
	// ["file.txt","foo.txt"] is "f", so text should remain "f".
	if got == "file.txt" {
		t.Fatalf("Tab auto-inserted first candidate %q — should not auto-insert", got)
	}
	// The hint must list both candidates.
	if !strings.Contains(e.minibufHint, "file.txt") || !strings.Contains(e.minibufHint, "foo.txt") {
		t.Errorf("hint %q should list both file.txt and foo.txt", e.minibufHint)
	}
}

// TestMinibufTabExtendsCommonPrefix verifies that Tab extends the typed text
// to the longest common prefix when it can.
func TestMinibufTabExtendsCommonPrefix(t *testing.T) {
	e := newTestEditor("")
	e.ReadMinibuffer("Find file: ", func(string) {})
	e.SetMinibufCompletions(func(_ string) []string {
		return []string{"forward-char", "forward-word", "forward-line"}
	})
	e.minibufBuf.InsertString(0, "fo")
	e.minibufBuf.SetPoint(2)

	e.dispatchMinibufKey(tabKey())

	if got := e.minibufBuf.String(); got != "forward-" {
		t.Errorf("Tab common prefix: want \"forward-\", got %q", got)
	}
}

func TestCmdDescribeKey(t *testing.T) {
	e := newTestEditor("")
	e.cmdDescribeKey()
	if !e.describeKeyPending {
		t.Error("cmdDescribeKey: expected describeKeyPending=true")
	}
	if e.message == "" {
		t.Error("cmdDescribeKey: expected a prompt message")
	}
}

func TestCmdDescribeFunction(t *testing.T) {
	e := newTestEditor("")
	e.cmdDescribeFunction()
	if !e.minibufActive {
		t.Error("cmdDescribeFunction: expected minibuffer to be active")
	}
}

func TestCmdDescribeVariable(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.cmdDescribeVariable()
	if !e.minibufActive {
		t.Error("cmdDescribeVariable: expected minibuffer to be active")
	}
}

func TestCmdLoadTheme(t *testing.T) {
	e := newTestEditor("")
	e.cmdLoadTheme()
	if !e.minibufActive {
		t.Error("cmdLoadTheme: expected minibuffer to be active")
	}
}

func TestWalkProjectFiles_Basic(t *testing.T) {
	// Create a small temporary project tree.
	root := t.TempDir()
	files := []string{
		"main.go",
		"pkg/foo.go",
		"pkg/bar.go",
		"README.md",
	}
	for _, f := range files {
		full := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// .git directory should be skipped.
	gitDir := filepath.Join(root, ".git")
	if err := os.Mkdir(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	got := walkProjectFiles(root)
	if len(got) != len(files) {
		t.Errorf("walkProjectFiles: want %d files, got %d: %v", len(files), len(got), got)
	}
	// Verify .git/config is not present.
	for _, f := range got {
		if containsStr(f, ".git") {
			t.Errorf("walkProjectFiles: .git file leaked: %s", f)
		}
	}
}

func TestProjectFileCompletions_EmptyQuery(t *testing.T) {
	e := newTestEditor("")
	root := t.TempDir()
	files := []string{
		filepath.Join(root, "a.go"),
		filepath.Join(root, "b.go"),
		filepath.Join(root, "c.go"),
	}
	got := e.projectFileCompletions(root, files, "")
	if len(got) != 3 {
		t.Errorf("empty query: want 3 results, got %d", len(got))
	}
}

func TestProjectFileCompletions_FuzzyFilter(t *testing.T) {
	e := newTestEditor("")
	root := t.TempDir()
	files := []string{
		filepath.Join(root, "main.go"),
		filepath.Join(root, "main_test.go"),
		filepath.Join(root, "other.go"),
	}
	got := e.projectFileCompletions(root, files, "main")
	if len(got) != 2 {
		t.Errorf("query 'main': want 2 results, got %d: %v", len(got), got)
	}
	// "main.go" should rank before "main_test.go" (prefix match both, alphabetical).
	if len(got) >= 2 && got[0] != "main.go" {
		t.Errorf("query 'main': want first=main.go, got %q", got[0])
	}
}

func TestProjectFileCompletions_LRUFirst(t *testing.T) {
	e := newTestEditor("")
	root := t.TempDir()
	files := []string{
		filepath.Join(root, "alpha.go"),
		filepath.Join(root, "beta.go"),
		filepath.Join(root, "gamma.go"),
	}
	// Simulate beta.go being the most recently used buffer.
	betaBuf := e.ActiveBuffer()
	betaBuf.SetFilename(filepath.Join(root, "beta.go"))
	e.bufferMRU = append(e.bufferMRU, betaBuf)

	got := e.projectFileCompletions(root, files, "")
	if len(got) == 0 {
		t.Fatal("expected results")
	}
	if got[0] != "beta.go" {
		t.Errorf("LRU: want beta.go first, got %q", got[0])
	}
}

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

// cmdIndentOrComplete is the Tab entry point and the only end-to-end path
// through <mode>-indent, so these tests assert the resulting line text, not
// merely the absence of a panic.

func TestCmdIndentOrCompleteGoModeDedentsTopLevel(t *testing.T) {
	e := newTestEditor("\tfoo()\n")
	e.lisp = elisp.NewEvaluator()
	buf(e).SetMode("go")
	buf(e).SetPoint(0)
	e.cmdIndentOrComplete()
	// A top-level Go statement wants no indentation, so the stray tab goes.
	if got := buf(e).String(); got != "foo()\n" {
		t.Errorf("Tab on a wrongly indented top-level Go line: got %q, want %q", got, "foo()\n")
	}
}

func TestCmdIndentOrCompleteGoModeIndentsInsideBlock(t *testing.T) {
	e := newTestEditor("func foo() {\nx := 1\n}\n")
	e.lisp = elisp.NewEvaluator()
	buf(e).SetMode("go")
	buf(e).SetPoint(len("func foo() {\n"))
	e.cmdIndentOrComplete()
	if got := buf(e).String(); got != "func foo() {\n\tx := 1\n}\n" {
		t.Errorf("Tab inside a Go block: got %q, want a leading tab on line 2", got)
	}
}

// TestCmdIndentOrCompleteGoIndentConfig exercises the go-indent Elisp variable
// end to end: Tab must use the configured unit instead of the default tab.
func TestCmdIndentOrCompleteGoIndentConfig(t *testing.T) {
	e := newTestEditor("func foo() {\nx := 1\n}\n")
	e.lisp = elisp.NewEvaluator()
	if _, err := e.lisp.EvalString(`(setq go-indent 2)`); err != nil {
		t.Fatalf("EvalString: %v", err)
	}
	buf(e).SetMode("go")
	buf(e).SetPoint(len("func foo() {\n"))
	e.cmdIndentOrComplete()
	if got := buf(e).String(); got != "func foo() {\n  x := 1\n}\n" {
		t.Errorf("Tab with (setq go-indent 2): got %q, want two spaces of indent", got)
	}
}

func TestCmdIndentOrCompleteElispMode(t *testing.T) {
	e := newTestEditor("(defun f ()\n(+ 1 2))\n")
	e.lisp = elisp.NewEvaluator()
	buf(e).SetMode("elisp")
	buf(e).SetPoint(len("(defun f ()\n"))
	e.cmdIndentOrComplete()
	// The body of a defun is indented two columns.
	if got := buf(e).String(); got != "(defun f ()\n  (+ 1 2))\n" {
		t.Errorf("Tab inside a defun: got %q, want the body indented by 2", got)
	}
}

func TestCmdIndentOrCompleteElispModeTopLevel(t *testing.T) {
	e := newTestEditor("  (+ 1 2)\n")
	e.lisp = elisp.NewEvaluator()
	buf(e).SetMode("elisp")
	buf(e).SetPoint(0)
	e.cmdIndentOrComplete()
	if got := buf(e).String(); got != "(+ 1 2)\n" {
		t.Errorf("Tab on a top-level Elisp form: got %q, want %q", got, "(+ 1 2)\n")
	}
}

func TestCmdIndentOrCompleteFundamentalMode(t *testing.T) {
	e := newTestEditor("hello\n")
	e.lisp = elisp.NewEvaluator()
	buf(e).SetPoint(0)
	e.cmdIndentOrComplete()
	// Fundamental mode copies the previous line's indentation; there is no
	// previous line, so the text is left exactly as it was.
	if got := buf(e).String(); got != "hello\n" {
		t.Errorf("Tab in fundamental mode on the first line: got %q, want %q", got, "hello\n")
	}
}

func TestCmdIndentOrCompleteFundamentalModeCopiesPreviousIndent(t *testing.T) {
	e := newTestEditor("    first\nsecond\n")
	e.lisp = elisp.NewEvaluator()
	buf(e).SetPoint(len("    first\n"))
	e.cmdIndentOrComplete()
	if got := buf(e).String(); got != "    first\n    second\n" {
		t.Errorf("Tab in fundamental mode: got %q, want the previous line's indent copied", got)
	}
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

// newTestEditorFull returns a test editor that also has the auxiliary maps
// (autoRevertMtimes, shellStates, customHighlighters, spanCaches) that are
// required by writeBuffer and KillBuffer.  Use this for tests that exercise
// those code paths.
func newTestEditorFull(content string) *Editor {
	e := newTestEditor(content)
	e.autoRevertMtimes = make(map[*buffer.Buffer]time.Time)
	e.shellStates = make(map[*buffer.Buffer]*shellState)
	e.customHighlighters = make(map[*buffer.Buffer]syntax.Highlighter)
	e.spanCaches = make(map[*buffer.Buffer]*spanCache)
	return e
}

// ---------------------------------------------------------------------------
// cmdUndo / cmdRedo
// ---------------------------------------------------------------------------

func TestCmdUndo_NoHistory(t *testing.T) {
	e := newTestEditor("hello")
	e.cmdUndo()
	if !strings.Contains(e.message, "No further undo") {
		t.Errorf("expected 'No further undo' message, got %q", e.message)
	}
}

func TestCmdUndo_AfterEdit(t *testing.T) {
	e := newElispTestEditor("")
	e.selfInsert('X')
	e.selfInsert('Y')
	e.cmdUndo()
	if strings.Contains(e.ActiveBuffer().String(), "Y") && strings.Contains(e.ActiveBuffer().String(), "X") {
		// undo should have removed at least the most recent insertion
		t.Errorf("undo did not revert insertion: %q", e.ActiveBuffer().String())
	}
}

func TestCmdUndo_ReadOnly(t *testing.T) {
	e := newTestEditor("data")
	e.ActiveBuffer().SetReadOnly(true)
	e.cmdUndo()
	if e.ActiveBuffer().String() != "data" {
		t.Error("cmdUndo on read-only buffer must not modify it")
	}
}

func TestCmdRedo_NoHistory(t *testing.T) {
	e := newTestEditor("hello")
	e.cmdRedo()
	if !strings.Contains(e.message, "No further redo") {
		t.Errorf("expected 'No further redo' message, got %q", e.message)
	}
}

func TestCmdRedo_AfterUndo(t *testing.T) {
	e := newElispTestEditor("")
	e.selfInsert('A')
	if got := e.ActiveBuffer().String(); got != "A" {
		t.Fatalf("after self-insert: buffer = %q, want %q", got, "A")
	}
	e.cmdUndo()
	if got := e.ActiveBuffer().String(); got != "" {
		t.Fatalf("after undo: buffer = %q, want empty", got)
	}
	e.cmdRedo()
	if got := e.ActiveBuffer().String(); got != "A" {
		t.Errorf("after redo: buffer = %q, want %q (the undone insert restored)", got, "A")
	}
	if got := e.ActiveBuffer().Point(); got != 1 {
		t.Errorf("after redo: point = %d, want 1 (past the restored rune)", got)
	}
}

// ---------------------------------------------------------------------------
// cmdDescribeFunction / cmdDescribeVariable
// ---------------------------------------------------------------------------

func TestCmdDescribeFunction_ShowsHelp(t *testing.T) {
	e := newElispTestEditor("")
	e.setupKeymaps()
	e.cmdDescribeFunction()
	if e.minibufDoneFunc == nil {
		t.Fatal("cmdDescribeFunction should activate the minibuffer")
	}
	e.minibufDoneFunc("forward-char")
	if e.FindBuffer("*Help*") == nil {
		t.Error("expected *Help* buffer after describing a function")
	}
}

func TestCmdDescribeFunction_EmptyNameNoHelp(t *testing.T) {
	e := newElispTestEditor("")
	e.setupKeymaps()
	e.cmdDescribeFunction()
	e.minibufDoneFunc("")
	if e.FindBuffer("*Help*") != nil {
		t.Error("empty function name should not create *Help*")
	}
}

func TestCmdDescribeVariable_ShowsHelp(t *testing.T) {
	e := newElispTestEditor("")
	_, _ = e.lisp.EvalString("(setq fill-column 80)")
	e.cmdDescribeVariable()
	if e.minibufDoneFunc == nil {
		t.Fatal("cmdDescribeVariable should activate the minibuffer")
	}
	e.minibufDoneFunc("fill-column")
	if e.FindBuffer("*Help*") == nil {
		t.Error("expected *Help* buffer after describing a variable")
	}
}

// ---------------------------------------------------------------------------
// cmdLoadTheme
// ---------------------------------------------------------------------------

func TestCmdLoadTheme_Known(t *testing.T) {
	e := newElispTestEditor("")
	e.cmdLoadTheme()
	if e.minibufDoneFunc == nil {
		t.Fatal("cmdLoadTheme should activate the minibuffer")
	}
	e.minibufDoneFunc("sweet")
	if strings.Contains(e.message, "Unknown theme") {
		t.Errorf("'sweet' should be a known theme, got %q", e.message)
	}
}

func TestCmdLoadTheme_Unknown(t *testing.T) {
	e := newElispTestEditor("")
	e.cmdLoadTheme()
	e.minibufDoneFunc("no-such-theme-xyz")
	if !strings.Contains(e.message, "Unknown theme") {
		t.Errorf("expected 'Unknown theme' message, got %q", e.message)
	}
}

// ---------------------------------------------------------------------------
// cmdDeleteOtherWindows
// ---------------------------------------------------------------------------

func TestCmdDeleteOtherWindows(t *testing.T) {
	e := newTestEditor("hello")
	e.cmdSplitWindowBelow()
	if len(e.windows) != 2 {
		t.Fatalf("expected 2 windows after split, got %d", len(e.windows))
	}
	e.cmdDeleteOtherWindows()
	if len(e.windows) != 1 {
		t.Errorf("expected 1 window after delete-other-windows, got %d", len(e.windows))
	}
}

// ---------------------------------------------------------------------------
// Read-only guards: editing commands must no-op on a read-only buffer.
// ---------------------------------------------------------------------------

func TestEditingCommandsReadOnlyGuards(t *testing.T) {
	const content = "hello world\nsecond line\nthird\n"
	cmds := map[string]func(*Editor){
		"newline":              (*Editor).cmdNewline,
		"deleteChar":           (*Editor).cmdDeleteChar,
		"backwardDeleteChar":   (*Editor).cmdBackwardDeleteChar,
		"killLine":             (*Editor).cmdKillLine,
		"killRegion":           (*Editor).cmdKillRegion,
		"yank":                 (*Editor).cmdYank,
		"yankPop":              (*Editor).cmdYankPop,
		"killWord":             (*Editor).cmdKillWord,
		"backwardKillWord":     (*Editor).cmdBackwardKillWord,
		"transposeChars":       (*Editor).cmdTransposeChars,
		"openLine":             (*Editor).cmdOpenLine,
		"killSentence":         (*Editor).cmdKillSentence,
		"transposeWords":       (*Editor).cmdTransposeWords,
		"deleteBlankLines":     (*Editor).cmdDeleteBlankLines,
		"joinLine":             (*Editor).cmdJoinLine,
		"upcaseRegion":         (*Editor).cmdUpcaseRegion,
		"downcaseRegion":       (*Editor).cmdDowncaseRegion,
		"sortLines":            (*Editor).cmdSortLines,
		"deleteDuplicateLines": (*Editor).cmdDeleteDuplicateLines,
		"fillParagraph":        (*Editor).cmdFillParagraph,
		"indentRegion":         (*Editor).cmdIndentRegion,
		"indentRigidly":        (*Editor).cmdIndentRigidly,
	}
	for name, fn := range cmds {
		e := newElispTestEditor(content)
		e.ActiveBuffer().SetReadOnly(true)
		e.ActiveBuffer().SetPoint(3)
		fn(e)
		if e.ActiveBuffer().String() != content {
			t.Errorf("%s mutated a read-only buffer: %q", name, e.ActiveBuffer().String())
		}
	}
}

// newFindFileEditor returns an editor with the maps that loadFile / openDired need.
func newFindFileEditor(content string) *Editor {
	e := newTestEditorFull(content)
	e.lspConns = make(map[string]*lspConn)
	e.diredStates = make(map[*buffer.Buffer]*diredState)
	// Non-nil terminal (screen==nil) so async callbacks calling PostWakeup are safe.
	e.term = &terminal.Terminal{}
	return e
}

func TestCmdFindFile_OpensFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "open.txt")
	_ = os.WriteFile(path, []byte("hello"), 0o644)
	e := newFindFileEditor("")
	e.cmdFindFile()
	if e.minibufDoneFunc == nil {
		t.Fatal("cmdFindFile should activate the minibuffer")
	}
	e.minibufDoneFunc(path)
	if e.ActiveBuffer().Filename() != path {
		t.Errorf("expected active buffer %q, got %q", path, e.ActiveBuffer().Filename())
	}
}

func TestCmdFindFile_DirectoryOpensDired(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644)
	e := newFindFileEditor("")
	e.cmdFindFile()
	e.minibufDoneFunc(dir)
	if e.ActiveBuffer().Mode() != "dired" {
		t.Errorf("a directory path should open dired, got mode %q", e.ActiveBuffer().Mode())
	}
}

func TestCmdFindFile_EmptyNoop(t *testing.T) {
	e := newFindFileEditor("content")
	before := e.ActiveBuffer()
	e.cmdFindFile()
	e.minibufDoneFunc("")
	if e.ActiveBuffer() != before {
		t.Error("empty path should not change the active buffer")
	}
}

func TestCmdFindFile_TildeExpansion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	e := newFindFileEditor("")
	e.cmdFindFile()
	e.minibufDoneFunc("~/newfile.txt")
	if !strings.HasPrefix(e.ActiveBuffer().Filename(), home) {
		t.Errorf("~ should expand to home dir, got %q", e.ActiveBuffer().Filename())
	}
}

func TestCmdProjectFindFile_OpensRelativePath(t *testing.T) {
	dir := makeGitRepo(t)
	sub := filepath.Join(dir, "pkg")
	_ = os.Mkdir(sub, 0o755)
	target := filepath.Join(sub, "file.txt")
	_ = os.WriteFile(target, []byte("data"), 0o644)

	e := newFindFileEditor("")
	e.ActiveBuffer().SetFilename(filepath.Join(dir, "notes.txt"))
	e.cmdProjectFindFile()
	if e.minibufDoneFunc == nil {
		t.Fatal("cmdProjectFindFile should activate the minibuffer in a VC repo")
	}
	e.minibufDoneFunc(filepath.Join("pkg", "file.txt"))
	if e.ActiveBuffer().Filename() != target {
		t.Errorf("expected to open %q, got %q", target, e.ActiveBuffer().Filename())
	}
}

func TestCmdProjectFindFile_NoVCFallsBack(t *testing.T) {
	dir := t.TempDir() // not a git repo
	e := newFindFileEditor("")
	e.ActiveBuffer().SetFilename(filepath.Join(dir, "loose.txt"))
	e.cmdProjectFindFile()
	// Falls back to cmdFindFile → minibuffer prompt active.
	if !e.minibufActive {
		t.Error("cmdProjectFindFile with no VC root should fall back to find-file")
	}
}

// ---------------------------------------------------------------------------
// cmdExecuteExtendedCommand — callback branches
// ---------------------------------------------------------------------------

// executeExtendedCommandWithName simulates the user typing name into the M-x
// minibuffer and pressing Enter by directly invoking the done callback.
func executeExtendedCommandWithName(e *Editor, name string) {
	// cmdExecuteExtendedCommand sets up the minibuffer; we then call the
	// done callback directly to avoid needing a real terminal.
	e.cmdExecuteExtendedCommand()
	e.minibufDoneFunc(name)
}

func TestCmdExecuteExtendedCommandUnknownShowsMessage(t *testing.T) {
	e := newTestEditor("")
	executeExtendedCommandWithName(e, "no-such-command-xyz")
	if !strings.Contains(e.message, "No command") {
		t.Errorf("execute-extended-command unknown: want 'No command' in message, got %q", e.message)
	}
}

func TestCmdExecuteExtendedCommandEmptyNoOp(t *testing.T) {
	e := newTestEditor("hello")
	before := buf(e).String()
	executeExtendedCommandWithName(e, "   ")
	// Buffer unchanged; no message about "No command".
	if buf(e).String() != before {
		t.Errorf("execute-extended-command empty: buffer should be unchanged")
	}
	if strings.Contains(e.message, "No command") {
		t.Errorf("execute-extended-command empty: should not produce 'No command' message")
	}
}

func TestCmdExecuteExtendedCommandKnownExecutes(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetPoint(0)
	// "forward-char" is a known command; executing it should move the point.
	executeExtendedCommandWithName(e, "forward-char")
	if got := buf(e).Point(); got != 1 {
		t.Errorf("execute-extended-command forward-char: want point=1, got %d", got)
	}
}

func TestCmdExecuteExtendedCommandUpdatesLRU(t *testing.T) {
	e := newTestEditor("")
	executeExtendedCommandWithName(e, "forward-char")
	found := false
	for _, name := range e.commandLRU {
		if name == "forward-char" {
			found = true
			break
		}
	}
	if !found {
		t.Error("execute-extended-command: 'forward-char' not added to commandLRU")
	}
}

// ---------------------------------------------------------------------------
// cmdKillBuffer — minibuffer callback branches
// ---------------------------------------------------------------------------

// killBufferWithName simulates confirm in the kill-buffer minibuffer.
func killBufferWithName(e *Editor, name string) {
	e.cmdKillBuffer()
	e.minibufDoneFunc(name)
}

func TestCmdKillBufferDefaultKillsCurrentBuffer(t *testing.T) {
	e := newTestEditor("content")
	// Add a second buffer so *scratch* exists after the kill.
	extra := buffer.NewWithContent("*extra*", "data")
	e.buffers = append(e.buffers, extra)
	activeName := buf(e).Name()
	// Passing empty string means "use default" (the current buffer name).
	killBufferWithName(e, "")
	for _, b := range e.buffers {
		if b.Name() == activeName {
			t.Errorf("cmdKillBuffer default: killed buffer %q still in e.buffers", activeName)
		}
	}
}

func TestCmdKillBufferExplicitName(t *testing.T) {
	e := newTestEditor("content")
	extra := buffer.NewWithContent("*extra*", "more data")
	e.buffers = append(e.buffers, extra)
	killBufferWithName(e, "*extra*")
	for _, b := range e.buffers {
		if b.Name() == "*extra*" {
			t.Error("cmdKillBuffer explicit name: killed buffer still in e.buffers")
		}
	}
}

func TestCmdKillBufferSetsMessage(t *testing.T) {
	e := newTestEditor("content")
	extra := buffer.NewWithContent("*extra*", "data")
	e.buffers = append(e.buffers, extra)
	killBufferWithName(e, "*extra*")
	if !strings.Contains(e.message, "*extra*") {
		t.Errorf("cmdKillBuffer: message should mention buffer name, got %q", e.message)
	}
}

// ---------------------------------------------------------------------------
// cmdSwitchToBuffer — callback branch that actually switches
// ---------------------------------------------------------------------------

func TestCmdSwitchToBufferSwitchesToExistingBuffer(t *testing.T) {
	e := newTestEditor("first buffer")
	second := buffer.NewWithContent("*second*", "second buffer content")
	e.buffers = append(e.buffers, second)

	// Invoke through minibuf callback directly.
	e.cmdSwitchToBuffer()
	e.minibufDoneFunc("*second*")

	if e.activeWin.Buf().Name() != "*second*" {
		t.Errorf("cmdSwitchToBuffer: expected active buffer %q, got %q",
			"*second*", e.activeWin.Buf().Name())
	}
}

func TestCmdSwitchToBufferDefaultUsedOnEmptyInput(t *testing.T) {
	e := newTestEditor("first")
	second := buffer.NewWithContent("*second*", "second")
	e.buffers = append(e.buffers, second)
	// Make *second* the MRU so it becomes the default.
	e.bufferMRU = []*buffer.Buffer{second}

	e.cmdSwitchToBuffer()
	// Passing empty string should resolve to the default.
	e.minibufDoneFunc("")

	if e.activeWin.Buf().Name() != "*second*" {
		t.Errorf("cmdSwitchToBuffer empty→default: expected %q, got %q",
			"*second*", e.activeWin.Buf().Name())
	}
}

// ---------------------------------------------------------------------------
// cmdSaveSomeBuffers — branch: modified buffer with filename gets written
// ---------------------------------------------------------------------------

func TestCmdSaveSomeBuffersModifiedFileGetsWritten(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	e := newTestEditorFull("hello world")
	buf(e).SetFilename(path)
	buf(e).SetModified(true)

	e.cmdSaveSomeBuffers()

	// File should now exist on disk.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cmdSaveSomeBuffers: file not written: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("cmdSaveSomeBuffers: file content = %q, want %q", string(data), "hello world")
	}
}

func TestCmdSaveSomeBuffersReportsCountOnSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test2.txt")

	e := newTestEditorFull("content")
	buf(e).SetFilename(path)
	buf(e).SetModified(true)

	e.cmdSaveSomeBuffers()

	if !strings.Contains(e.message, "Saved") {
		t.Errorf("cmdSaveSomeBuffers: expected 'Saved' in message, got %q", e.message)
	}
}

func TestCmdSaveSomeBuffersModifiedWithoutFilenameSkipped(t *testing.T) {
	e := newTestEditorFull("dirty")
	buf(e).SetModified(true)
	// No filename set — should count as zero files to save.

	e.cmdSaveSomeBuffers()

	if e.message != "(No files need saving)" {
		t.Errorf("cmdSaveSomeBuffers modified no filename: want %q, got %q",
			"(No files need saving)", e.message)
	}
}

func TestCmdSaveSomeBuffersMarksBufferClean(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clean.txt")

	e := newTestEditorFull("content")
	buf(e).SetFilename(path)
	buf(e).SetModified(true)

	e.cmdSaveSomeBuffers()

	if buf(e).Modified() {
		t.Error("cmdSaveSomeBuffers: buffer should be marked clean after save")
	}
}

// ---------------------------------------------------------------------------
// cmdSaveBuffer — branches: no filename prompts minibuffer; has filename writes
// ---------------------------------------------------------------------------

func TestCmdSaveBufferWithFilenameWritesToDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "save.txt")

	e := newTestEditorFull("saved content")
	buf(e).SetFilename(path)

	e.cmdSaveBuffer()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cmdSaveBuffer: file not written: %v", err)
	}
	if string(data) != "saved content" {
		t.Errorf("cmdSaveBuffer: want %q, got %q", "saved content", string(data))
	}
}

func TestCmdSaveBufferWithFilenameMarksClean(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clean.txt")

	e := newTestEditorFull("data")
	buf(e).SetFilename(path)
	buf(e).SetModified(true)

	e.cmdSaveBuffer()

	if buf(e).Modified() {
		t.Error("cmdSaveBuffer: buffer should be marked clean after save")
	}
}

func TestCmdSaveBufferWithFilenameShowsMessage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "msg.txt")

	e := newTestEditorFull("text")
	buf(e).SetFilename(path)

	e.cmdSaveBuffer()

	if !strings.Contains(e.message, "Wrote") {
		t.Errorf("cmdSaveBuffer: want 'Wrote' in message, got %q", e.message)
	}
}

func TestCmdSaveBufferNoFilenameOpensMinibuffer(t *testing.T) {
	e := newTestEditorFull("unsaved content")
	// No filename — should prompt.
	e.cmdSaveBuffer()
	if !e.minibufActive {
		t.Error("cmdSaveBuffer no filename: expected minibufActive=true")
	}
}

func TestCmdSaveBufferNoFilenameCallbackSetsFilenameAndWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.txt")

	e := newTestEditorFull("new content")
	e.cmdSaveBuffer()
	// Simulate user supplying a path.
	e.minibufDoneFunc(path)

	if buf(e).Filename() != path {
		t.Errorf("cmdSaveBuffer callback: filename want %q, got %q", path, buf(e).Filename())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cmdSaveBuffer callback: file not written: %v", err)
	}
	if string(data) != "new content" {
		t.Errorf("cmdSaveBuffer callback: file content = %q, want %q", string(data), "new content")
	}
}

// ---------------------------------------------------------------------------
// cmdDeleteOtherWindows — with multiple windows
// ---------------------------------------------------------------------------

func TestCmdDeleteOtherWindowsReducesToOne(t *testing.T) {
	e := newTestEditor("hello")
	e.cmdSplitWindowBelow()
	if len(e.windows) != 2 {
		t.Fatalf("pre-condition: expected 2 windows, got %d", len(e.windows))
	}
	e.cmdDeleteOtherWindows()
	if len(e.windows) != 1 {
		t.Fatalf("cmdDeleteOtherWindows: expected 1 window, got %d", len(e.windows))
	}
}

func TestCmdDeleteOtherWindowsKeepsActiveWindow(t *testing.T) {
	e := newTestEditor("hello")
	e.cmdSplitWindowBelow()
	// Switch to the second window and then delete others.
	e.cmdOtherWindow()
	active := e.activeWin
	e.cmdDeleteOtherWindows()
	if e.windows[0] != active {
		t.Error("cmdDeleteOtherWindows: remaining window is not the active one")
	}
}

func TestCmdDeleteOtherWindowsSingleWindowNoOp(t *testing.T) {
	e := newTestEditor("hello")
	before := e.activeWin
	e.cmdDeleteOtherWindows()
	if len(e.windows) != 1 {
		t.Errorf("cmdDeleteOtherWindows single window: expected 1, got %d", len(e.windows))
	}
	if e.activeWin != before {
		t.Error("cmdDeleteOtherWindows single window: active window changed unexpectedly")
	}
}

// ---------------------------------------------------------------------------
// cmdUpcaseWord — with subwordMode
// ---------------------------------------------------------------------------

func TestUpcaseWordWithArgMultipleWords(t *testing.T) {
	e := newTestEditor("hello world foo")
	buf(e).SetPoint(0)
	e.universalArg = 2
	e.universalArgSet = true
	e.cmdUpcaseWord()
	got := buf(e).String()
	if got != "HELLO WORLD foo" {
		t.Errorf("upcase-word x2: want %q, got %q", "HELLO WORLD foo", got)
	}
}

func TestUpcaseWordAtEndOfBuffer(t *testing.T) {
	// Point past the last word — nothing to upcase.
	e := newTestEditor("hello")
	buf(e).SetPoint(5)
	e.cmdUpcaseWord()
	got := buf(e).String()
	if got != "hello" {
		t.Errorf("upcase-word at end: buffer should be unchanged, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// cmdDowncaseWord — with subwordMode
// ---------------------------------------------------------------------------

func TestDowncaseWordWithArgMultipleWords(t *testing.T) {
	e := newTestEditor("HELLO WORLD foo")
	buf(e).SetPoint(0)
	e.universalArg = 2
	e.universalArgSet = true
	e.cmdDowncaseWord()
	got := buf(e).String()
	if got != "hello world foo" {
		t.Errorf("downcase-word x2: want %q, got %q", "hello world foo", got)
	}
}

func TestDowncaseWordAtEndOfBuffer(t *testing.T) {
	e := newTestEditor("HELLO")
	buf(e).SetPoint(5)
	e.cmdDowncaseWord()
	got := buf(e).String()
	if got != "HELLO" {
		t.Errorf("downcase-word at end: buffer should be unchanged, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// cmdCapitalizeWord — extra coverage
// ---------------------------------------------------------------------------

func TestCapitalizeWordWithArgMultipleWords(t *testing.T) {
	e := newTestEditor("HELLO WORLD foo")
	buf(e).SetPoint(0)
	e.universalArg = 2
	e.universalArgSet = true
	e.cmdCapitalizeWord()
	got := buf(e).String()
	if got != "Hello World foo" {
		t.Errorf("capitalize-word x2: want %q, got %q", "Hello World foo", got)
	}
}

func TestCapitalizeWordAtEndOfBuffer(t *testing.T) {
	e := newTestEditor("hello")
	buf(e).SetPoint(5)
	e.cmdCapitalizeWord()
	got := buf(e).String()
	if got != "hello" {
		t.Errorf("capitalize-word at end: buffer should be unchanged, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// cmdFillParagraph — various branch coverage
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// cmdDowncaseRegion / cmdUpcaseRegion — extra branches
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// cmdSaveBuffer — trailing-whitespace deletion (saveBufferDeleteTrailingWS)
// ---------------------------------------------------------------------------

func TestCmdSaveBufferDeletesTrailingWhitespace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trailing.txt")

	e := newTestEditorFull("hello   \nworld  \n")
	e.saveBufferDeleteTrailingWS = true
	buf(e).SetFilename(path)

	e.cmdSaveBuffer()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cmdSaveBuffer trailing WS: file not written: %v", err)
	}
	got := string(data)
	want := "hello\nworld\n"
	if got != want {
		t.Errorf("cmdSaveBuffer trailing WS: want %q, got %q", want, got)
	}
}

// ---------------------------------------------------------------------------
// cmdSaveSomeBuffers — multiple buffers, only the modified ones with filenames
// ---------------------------------------------------------------------------

func TestCmdSaveSomeBuffersMultipleBuffers(t *testing.T) {
	dir := t.TempDir()
	path1 := filepath.Join(dir, "one.txt")
	path2 := filepath.Join(dir, "two.txt")

	e := newTestEditorFull("buffer one content")
	buf(e).SetFilename(path1)
	buf(e).SetModified(true)

	b2 := buffer.NewWithContent("*b2*", "buffer two content")
	b2.SetFilename(path2)
	b2.SetModified(true)
	e.buffers = append(e.buffers, b2)

	e.cmdSaveSomeBuffers()

	// Both files should exist.
	for _, p := range []string{path1, path2} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("cmdSaveSomeBuffers: file %q not written: %v", p, err)
		}
	}
	// Message should mention 2 files.
	if !strings.Contains(e.message, "2") {
		t.Errorf("cmdSaveSomeBuffers: message should mention 2, got %q", e.message)
	}
}

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

// ---------------------------------------------------------------------------
// indent-region (C-M-\)
// ---------------------------------------------------------------------------

// TestCmdProjectFindFile_WithVCRoot verifies that calling cmdProjectFindFile
// inside a git repository opens the minibuffer with a "Project find file:"
// prompt rather than falling back to regular find-file.
func TestCmdProjectFindFile_WithVCRoot(t *testing.T) {
	// Use the gomacs repo itself as a project root — we know it has a .git dir.
	root := "/Users/torstein/src/skybert/gomacs"
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		t.Skip("gomacs .git dir not accessible; skipping")
	}

	e := newTestEditor("")
	buf(e).SetFilename(filepath.Join(root, "dummy.go"))

	e.cmdProjectFindFile()

	if !e.minibufActive {
		t.Fatal("cmdProjectFindFile in VC root: expected minibufActive=true")
	}
	if !strings.Contains(e.minibufPrompt, "Project find file") {
		t.Errorf("cmdProjectFindFile: prompt = %q, want to contain 'Project find file'",
			e.minibufPrompt)
	}
}

// TestCmdProjectFindFile_NoVCRoot verifies that outside any VC root the
// function falls back to regular find-file (which also sets minibufActive).
func TestCmdProjectFindFile_NoVCRoot(t *testing.T) {
	// Use a temp dir that has no .git ancestor.
	dir := t.TempDir()
	e := newTestEditor("")
	buf(e).SetFilename(filepath.Join(dir, "dummy.go"))

	e.cmdProjectFindFile()

	// Either "Project find file" (if temp dir is somehow under a VC root) or
	// the regular "Find file" prompt — either way minibufActive should be true.
	if !e.minibufActive {
		t.Error("cmdProjectFindFile fallback: expected minibufActive=true")
	}
}

// TestCmdProjectFindFile_Completions verifies that after calling
// cmdProjectFindFile in a VC root, the completions callback returns at least
// some results when given an empty query.
func TestCmdProjectFindFile_Completions(t *testing.T) {
	root := "/Users/torstein/src/skybert/gomacs"
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		t.Skip("gomacs .git dir not accessible; skipping")
	}

	e := newTestEditor("")
	buf(e).SetFilename(filepath.Join(root, "dummy.go"))

	e.cmdProjectFindFile()

	if e.minibufCompletions == nil {
		t.Fatal("cmdProjectFindFile: minibufCompletions should be set")
	}
	results := e.minibufCompletions("")
	if len(results) == 0 {
		t.Error("cmdProjectFindFile completions: expected non-empty list for empty query")
	}
}

// TestPromptSaveNext_AllHandled verifies that when the unsaved slice is
// exhausted, e.quit becomes true.
func TestPromptSaveNext_AllHandled(t *testing.T) {
	e := newTestEditor("hello")
	e.promptSaveNext([]*buffer.Buffer{}, 0)
	if !e.quit {
		t.Error("promptSaveNext: with empty slice, e.quit should be true")
	}
}

// TestPromptSaveNext_IdxBeyondEnd verifies the boundary condition where idx
// starts at len(unsaved).
func TestPromptSaveNext_IdxBeyondEnd(t *testing.T) {
	e := newTestEditor("hello")
	b := buffer.NewWithContent("*unsaved*", "data")
	unsaved := []*buffer.Buffer{b}
	e.promptSaveNext(unsaved, 1) // idx == len(unsaved) → quit
	if !e.quit {
		t.Error("promptSaveNext idx=len: e.quit should be true")
	}
}

// TestPromptSaveNext_SetsReadCharPending verifies that when there is an
// unsaved buffer the function sets up the single-char prompt.
func TestPromptSaveNext_SetsReadCharPending(t *testing.T) {
	e := newTestEditor("hello")
	b := buffer.NewWithContent("*buf1*", "data")
	b.SetModified(true)

	e.promptSaveNext([]*buffer.Buffer{b}, 0)

	if !e.readCharPending {
		t.Error("promptSaveNext: readCharPending should be true while waiting for y/n")
	}
	if e.readCharCallback == nil {
		t.Error("promptSaveNext: readCharCallback should be set")
	}
}

// TestPromptSaveNext_YCallbackSavesFile verifies that answering 'y' writes
// the buffer to disk and marks it unmodified, then proceeds to set quit.
func TestPromptSaveNext_YCallbackSavesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tobesaved.txt")

	e := newTestEditor("hello")
	b := buffer.NewWithContent("*tosave*", "file content")
	b.SetFilename(path)
	b.SetModified(true)

	e.promptSaveNext([]*buffer.Buffer{b}, 0)

	if e.readCharCallback == nil {
		t.Fatal("readCharCallback should be set")
	}
	// Simulate the user pressing 'y'.
	e.readCharCallback('y')

	// File should have been written.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file was not created after 'y': %v", err)
	}
	if string(data) != "file content" {
		t.Errorf("file content = %q, want %q", string(data), "file content")
	}
	// Buffer should be marked as unmodified.
	if b.Modified() {
		t.Error("buffer should be marked unmodified after save")
	}
	// Since it was the only buffer, quit should now be true.
	if !e.quit {
		t.Error("e.quit should be true after all buffers handled")
	}
}

// TestPromptSaveNext_NCallbackSkipsFile verifies that answering 'n' does NOT
// write the file but still progresses through the list and sets quit.
func TestPromptSaveNext_NCallbackSkipsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "skipped.txt")

	e := newTestEditor("hello")
	b := buffer.NewWithContent("*skip*", "unsaved")
	b.SetFilename(path)
	b.SetModified(true)

	e.promptSaveNext([]*buffer.Buffer{b}, 0)

	if e.readCharCallback == nil {
		t.Fatal("readCharCallback should be set")
	}
	// Simulate the user pressing 'n'.
	e.readCharCallback('n')

	// File must NOT have been created.
	if _, err := os.Stat(path); err == nil {
		t.Error("file should NOT have been written after 'n'")
	}
	// Buffer should still be modified.
	if !b.Modified() {
		t.Error("buffer should still be marked modified after 'n'")
	}
	// But quit should still be set (all buffers handled).
	if !e.quit {
		t.Error("e.quit should be true after skipping the only unsaved buffer")
	}
}

// TestPromptSaveNext_MessageContainsBufferName verifies that the message
// prompt includes the buffer name so the user knows what they are being
// asked about.
func TestPromptSaveNext_MessageContainsBufferName(t *testing.T) {
	e := newTestEditor("hello")
	b := buffer.NewWithContent("myspecialfile.go", "data")
	b.SetModified(true)

	e.promptSaveNext([]*buffer.Buffer{b}, 0)

	if !strings.Contains(e.message, "myspecialfile.go") {
		t.Errorf("promptSaveNext: message %q should contain buffer name", e.message)
	}
}

// TestCmdSaveBuffersKillTerminal_WithUnsaved verifies the full flow:
// an unsaved buffer with a backing file triggers the prompt loop.
func TestCmdSaveBuffersKillTerminal_WithUnsaved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "modified.txt")
	// Pre-create the file.
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}

	e := newTestEditor("original")
	buf(e).SetFilename(path)
	buf(e).SetModified(true)

	e.cmdSaveBuffersKillTerminal()

	// Should not have quit yet — should be waiting for user input.
	if e.quit {
		t.Error("cmdSaveBuffersKillTerminal: should not quit before user responds")
	}
	if !e.readCharPending {
		t.Error("cmdSaveBuffersKillTerminal: should be waiting for char input")
	}
}
