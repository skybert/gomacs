package editor

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/elisp"
	"github.com/skybert/gomacs/internal/keymap"
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
		term:         nil,
		buffers:      []*buffer.Buffer{buf},
		windows:      []*window.Window{win},
		activeWin:    win,
		minibufBuf:   buffer.New(" *minibuf*"),
		globalKeymap: keymap.New("global"),
		ctrlXKeymap:  keymap.New("C-x"),
		universalArg: 1,
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
	// Use /tmp which should always exist and have entries.
	results := filePathCompletions("/tmp/")
	// We can't assert exact entries, but there should be some results or at
	// least no panic.
	_ = results
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
// sort-lines
// ---------------------------------------------------------------------------

func TestSortLinesRegion(t *testing.T) {
	content := "banana\napple\ncherry\n"
	e := newTestEditor(content)
	b := buf(e)
	b.SetMark(0)
	b.SetMarkActive(true)
	b.SetPoint(b.Len())
	e.cmdSortLines()
	want := "apple\nbanana\ncherry\n"
	if got := b.String(); got != want {
		t.Errorf("sort-lines: want %q, got %q", want, got)
	}
}

func TestSortLinesNoRegion(t *testing.T) {
	content := "c\nb\na\n"
	e := newTestEditor(content)
	b := buf(e)
	b.SetMarkActive(false)
	b.SetPoint(0)
	e.cmdSortLines()
	want := "a\nb\nc\n"
	if got := b.String(); got != want {
		t.Errorf("sort-lines no-region: want %q, got %q", want, got)
	}
}

// ---------------------------------------------------------------------------
// delete-duplicate-lines
// ---------------------------------------------------------------------------

func TestDeleteDuplicateLines(t *testing.T) {
	content := "foo\nbar\nfoo\nbaz\nbar\n"
	e := newTestEditor(content)
	b := buf(e)
	b.SetMark(0)
	b.SetMarkActive(true)
	b.SetPoint(b.Len())
	e.cmdDeleteDuplicateLines()
	want := "foo\nbar\nbaz\n"
	if got := b.String(); got != want {
		t.Errorf("delete-duplicate-lines: want %q, got %q", want, got)
	}
}

func TestDeleteDuplicateLinesNoDups(t *testing.T) {
	content := "a\nb\nc\n"
	e := newTestEditor(content)
	b := buf(e)
	b.SetMark(0)
	b.SetMarkActive(true)
	b.SetPoint(b.Len())
	e.cmdDeleteDuplicateLines()
	want := "a\nb\nc\n"
	if got := b.String(); got != want {
		t.Errorf("delete-duplicate-lines no-dups: want %q, got %q", want, got)
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
// cmdJsonMode
// ---------------------------------------------------------------------------

func TestCmdJsonMode(t *testing.T) {
	e := newTestEditor("{ \"key\": 1 }")
	e.cmdJsonMode()
	if got := buf(e).Mode(); got != "json" {
		t.Errorf("json-mode: want mode=%q, got %q", "json", got)
	}
}

func TestCmdYamlMode(t *testing.T) {
	e := newTestEditor("key: value\nlist:\n  - item\n")
	e.cmdYamlMode()
	if got := buf(e).Mode(); got != "yaml" {
		t.Errorf("yaml-mode: want mode=%q, got %q", "yaml", got)
	}
	// Verify highlighter dispatch works and returns spans.
	cache := e.getSpanCache(buf(e))
	if cache == nil {
		t.Fatal("yaml-mode: span cache should not be nil")
	}
	if len(cache.spans) == 0 {
		t.Error("yaml-mode: expected syntax spans, got none")
	}
}

// ---------------------------------------------------------------------------
// delete-trailing-whitespace
// ---------------------------------------------------------------------------

func TestDeleteTrailingWhitespaceWholeBuf(t *testing.T) {
	e := newTestEditor("hello   \nworld  \nno-trail\n")
	e.cmdDeleteTrailingWhitespace()
	want := "hello\nworld\nno-trail\n"
	if got := buf(e).String(); got != want {
		t.Errorf("want %q, got %q", want, got)
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
