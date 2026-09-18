package editor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/syntax"
	"github.com/skybert/gomacs/internal/terminal"
)

func TestGetSpellSpans_AsyncNoOpCommand(t *testing.T) {
	e := newCapTestEditor("hello world")
	e.lspCbs = make(chan func(), 4)
	e.spellCommand = "true" // exits 0 with no output → no misspellings
	e.spellLanguage = "en"
	b := e.ActiveBuffer()
	b.SetMode("text")
	b.InsertString(b.Len(), "!") // bump ChangeGen above 0 so the pending check is meaningful

	// First call schedules the async check and returns nil (no stale spans).
	if spans := e.getSpellSpans(b); spans != nil {
		t.Fatalf("first call should return nil while the check is in flight, got %v", spans)
	}
	select {
	case fn := <-e.lspCbs:
		fn() // store the (empty) result in the cache
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for spell check callback")
	}
	// Second call hits the cache for the same generation.
	_ = e.getSpellSpans(b)
}

// newSpellTestEditor returns an editor configured to use aspell, skipping the
// test if aspell is not installed.
func newSpellTestEditor(t *testing.T, content string) *Editor {
	t.Helper()
	if _, err := exec.LookPath("aspell"); err != nil {
		t.Skip("aspell not available")
	}
	e := newCapTestEditor(content)
	e.spellCommand = "aspell"
	e.spellLanguage = "en"
	e.spellCaches = make(map[*buffer.Buffer]*spellCache)
	return e
}

// fakeAspell writes a stub spell checker to a temporary directory and returns
// its path.  It ignores its arguments and its stdin and reports every word in
// words as misspelled, which is the whole contract runAspellList relies on.
// The stub keeps the tests hermetic: CI has no aspell installed.
func fakeAspell(t *testing.T, words ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-aspell")
	script := "#!/bin/sh\ncat > /dev/null\nprintf '%s\\n'"
	for _, w := range words {
		script += " " + w
	}
	if err := os.WriteFile(path, []byte(script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// ---------------------------------------------------------------------------
// computeSpellSpansForMode — comment-only modes
// ---------------------------------------------------------------------------

// TestComputeSpellSpansForMode_ConfComments checks that conf-mode '#' comments
// are spell-checked: doc/gomacs-spec.md requires conf-mode to spell-check
// comments like the other programming modes.  It regressed once because
// syntax.LangToHighlighter had no "conf" case, so no comment spans were found.
func TestComputeSpellSpansForMode_ConfComments(t *testing.T) {
	const bad = "mispelled"
	stub := fakeAspell(t, bad)
	tests := []struct {
		name string
		text string
		// want holds the [start, end) rune ranges expected to be flagged.
		want [][2]int
	}{
		{
			name: "full line comment",
			text: "# a mispelled word\nkey = 1\n",
			want: [][2]int{{4, 13}},
		},
		{
			name: "inline comment",
			text: "key = 1 # mispelled\n",
			want: [][2]int{{10, 19}},
		},
		{
			// Keys and values are code, not prose: only comments are checked.
			name: "no comments",
			text: "mispelled = mispelled\n",
			want: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spans := computeSpellSpansForMode(stub, "en", tc.text, "conf")
			if len(spans) != len(tc.want) {
				t.Fatalf("got %d spans %v, want %d", len(spans), spans, len(tc.want))
			}
			for i, w := range tc.want {
				if spans[i].Start != w[0] || spans[i].End != w[1] {
					t.Errorf("span %d = [%d,%d), want [%d,%d)",
						i, spans[i].Start, spans[i].End, w[0], w[1])
				}
				if got := []rune(tc.text)[spans[i].Start:spans[i].End]; string(got) != bad {
					t.Errorf("span %d covers %q, want %q", i, string(got), bad)
				}
			}
		})
	}
}

// TestComputeSpellSpansForMode_ConfMatchesBash pins conf-mode's comment
// spell-checking to bash-mode's, since both use '#' comments.
func TestComputeSpellSpansForMode_ConfMatchesBash(t *testing.T) {
	stub := fakeAspell(t, "mispelled")
	text := "# mispelled\n"
	conf := computeSpellSpansForMode(stub, "en", text, "conf")
	bash := computeSpellSpansForMode(stub, "en", text, "bash")
	if len(conf) == 0 {
		t.Fatal("conf-mode found no spell errors in a '#' comment")
	}
	if len(conf) != len(bash) || conf[0] != bash[0] {
		t.Errorf("conf spans %v differ from bash spans %v", conf, bash)
	}
}

// ---------------------------------------------------------------------------
// findSpellSpans
// ---------------------------------------------------------------------------

func TestFindSpellSpansEmpty(t *testing.T) {
	spans := findSpellSpans("", "", []rune("hello world"), 0, 11)
	if spans != nil {
		t.Fatalf("expected nil spans with empty command, got %v", spans)
	}
}

func TestFindSpellSpansNoMisspellings(t *testing.T) {
	// "aspell list" with valid words returns no output.
	// We simulate by passing a word set that won't match anything.
	// There's no easy way to unit-test without mocking, so test the
	// helper logic with a known no-match scenario.
	spans := findSpellSpans("", "", []rune(""), 0, 0)
	if spans != nil {
		t.Fatalf("expected nil spans for empty input, got %v", spans)
	}
}

// ---------------------------------------------------------------------------
// virtToOrigPos
// ---------------------------------------------------------------------------

func TestVirtToOrigPos(t *testing.T) {
	// Simulate two comment regions:
	//   orig[5:10] → virt[0:5]
	//   orig[20:25] → virt[6:11]  (virt[5] is the separator '\n')
	mapping := []commentMapping{
		{origStart: 5, virtStart: 0, length: 5},
		{origStart: 20, virtStart: 6, length: 5},
	}
	tests := []struct {
		virtPos  int
		wantOrig int
	}{
		{0, 5},
		{4, 9},
		{5, -1}, // separator '\n'
		{6, 20},
		{10, 24},
		{11, -1}, // out of range
	}
	for _, tc := range tests {
		got := virtToOrigPos(mapping, tc.virtPos)
		if got != tc.wantOrig {
			t.Errorf("virtToOrigPos(%d) = %d, want %d", tc.virtPos, got, tc.wantOrig)
		}
	}
}

// ---------------------------------------------------------------------------
// isSpellErrorAt
// ---------------------------------------------------------------------------

func TestIsSpellErrorAt(t *testing.T) {
	spans := []syntax.Span{
		{Start: 3, End: 7, Face: FaceSpellError},
		{Start: 10, End: 14, Face: FaceSpellError},
	}
	// Inside first span.
	for _, pos := range []int{3, 4, 5, 6} {
		if !isSpellErrorAt(spans, pos) {
			t.Errorf("isSpellErrorAt(%d): expected true", pos)
		}
	}
	// Outside spans.
	for _, pos := range []int{0, 2, 7, 8, 9, 14, 15} {
		if isSpellErrorAt(spans, pos) {
			t.Errorf("isSpellErrorAt(%d): expected false", pos)
		}
	}
	// Inside second span.
	for _, pos := range []int{10, 11, 12, 13} {
		if !isSpellErrorAt(spans, pos) {
			t.Errorf("isSpellErrorAt(%d): expected true", pos)
		}
	}
}

// ---------------------------------------------------------------------------
// spellCheckAll / spellCheckComments
// ---------------------------------------------------------------------------

func TestSpellCheckAllModes(t *testing.T) {
	allModes := []string{"markdown", "text", "fundamental", ""}
	for _, m := range allModes {
		if !spellCheckAll(m) {
			t.Errorf("spellCheckAll(%q): expected true", m)
		}
	}
	notAllModes := []string{"go", "python", "java", "bash", "elisp", "json"}
	for _, m := range notAllModes {
		if spellCheckAll(m) {
			t.Errorf("spellCheckAll(%q): expected false", m)
		}
	}
}

func TestSpellCheckCommentsModes(t *testing.T) {
	commentModes := []string{"go", "python", "java", "bash", "elisp"}
	for _, m := range commentModes {
		if !spellCheckComments(m) {
			t.Errorf("spellCheckComments(%q): expected true", m)
		}
	}
	// markdown/text/fundamental use full-text checking, not comment-only.
	nonCommentModes := []string{"markdown", "text", "fundamental", "json"}
	for _, m := range nonCommentModes {
		if spellCheckComments(m) {
			t.Errorf("spellCheckComments(%q): expected false", m)
		}
	}
}

// ---------------------------------------------------------------------------
// blankHTMLTags
// ---------------------------------------------------------------------------

func TestBlankHTMLTags(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello world", "hello world"},
		{"<b>bold</b>", "   bold    "},
		{"no tags here", "no tags here"},
		{"<br/>", "     "},
		{"text <em>em</em> text", "text     em      text"},
		{"", ""},
	}
	for _, tc := range tests {
		got := string(blankHTMLTags([]rune(tc.input)))
		if got != tc.want {
			t.Errorf("blankHTMLTags(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// aspell-backed tests (require aspell binary)
// ---------------------------------------------------------------------------

func TestRunAspellList_FindsMisspelling(t *testing.T) {
	if _, err := exec.LookPath("aspell"); err != nil {
		t.Skip("aspell not available")
	}
	words, err := runAspellList("aspell", "en", "thiss is a tpyo")
	if err != nil {
		t.Fatalf("runAspellList: %v", err)
	}
	if len(words) == 0 {
		t.Fatal("expected at least one misspelled word")
	}
}

func TestRunAspellList_CleanText(t *testing.T) {
	if _, err := exec.LookPath("aspell"); err != nil {
		t.Skip("aspell not available")
	}
	words, err := runAspellList("aspell", "en", "this is correct\n")
	if err != nil {
		t.Fatalf("runAspellList: %v", err)
	}
	if len(words) != 0 {
		t.Fatalf("expected no misspellings, got %v", words)
	}
}

func TestAspellSuggest_ReturnsSuggestions(t *testing.T) {
	if _, err := exec.LookPath("aspell"); err != nil {
		t.Skip("aspell not available")
	}
	sugs := aspellSuggest("aspell", "en", "teh")
	if len(sugs) == 0 {
		t.Fatal("expected suggestions for 'teh'")
	}
}

func TestComputeSpellSpansForMode_TextWholeBuffer(t *testing.T) {
	if _, err := exec.LookPath("aspell"); err != nil {
		t.Skip("aspell not available")
	}
	spans := computeSpellSpansForMode("aspell", "en", "thiss has a tpyo", "text")
	if len(spans) == 0 {
		t.Fatal("expected spell spans for misspelled text")
	}
}

func TestComputeSpellSpansForMode_GoCommentsOnly(t *testing.T) {
	if _, err := exec.LookPath("aspell"); err != nil {
		t.Skip("aspell not available")
	}
	src := "package main\n\n// thiss is a tpyo in a comment\nfunc main() {}\n"
	spans := computeSpellSpansForMode("aspell", "en", src, "go")
	if len(spans) == 0 {
		t.Fatal("expected spell spans inside the Go comment")
	}
}

func TestComputeSpellSpansForMode_UnknownMode(t *testing.T) {
	spans := computeSpellSpansForMode("aspell", "en", "thiss tpyo", "no-such-mode")
	if spans != nil {
		t.Fatalf("unknown mode should yield nil spans, got %v", spans)
	}
}

func TestCmdIspellWord_Misspelled(t *testing.T) {
	e := newSpellTestEditor(t, "thiss\n")
	buf(e).SetPoint(2)
	e.cmdIspellWord()
	if !e.spellActive {
		t.Fatal("misspelled word should activate interactive spell check")
	}
	if len(e.spellErrors) != 1 {
		t.Fatalf("expected one spell error, got %d", len(e.spellErrors))
	}
}

func TestCmdIspellWord_Correct(t *testing.T) {
	e := newSpellTestEditor(t, "hello\n")
	buf(e).SetPoint(2)
	e.cmdIspellWord()
	if e.spellActive {
		t.Fatal("correctly spelled word should not activate spell check")
	}
}

func TestCmdIspellWord_NoWordAtPoint(t *testing.T) {
	e := newSpellTestEditor(t, "   \n")
	buf(e).SetPoint(1)
	e.cmdIspellWord()
	if !strings.Contains(e.message, "No word at point") {
		t.Fatalf("expected 'No word at point', got %q", e.message)
	}
}

func TestCmdIspellWord_NoChecker(t *testing.T) {
	e := newCapTestEditor("word\n")
	e.spellCommand = ""
	e.cmdIspellWord()
	if !strings.Contains(e.message, "no spell checker") {
		t.Fatalf("expected 'no spell checker', got %q", e.message)
	}
}

func TestCmdSpell_FindsErrors(t *testing.T) {
	e := newSpellTestEditor(t, "thiss sentense has erors\n")
	e.cmdSpell()
	if !e.spellActive {
		t.Fatal("cmdSpell should activate interactive spell check")
	}
	if len(e.spellErrors) == 0 {
		t.Fatal("cmdSpell should collect misspelled words")
	}
}

func TestCmdSpell_Clean(t *testing.T) {
	e := newSpellTestEditor(t, "this sentence is correct\n")
	e.cmdSpell()
	if e.spellActive {
		t.Fatal("clean buffer should not activate spell check")
	}
	if !strings.Contains(e.message, "No misspellings") {
		t.Fatalf("expected 'No misspellings', got %q", e.message)
	}
}

func TestSpellHandleKey_QuitCancels(t *testing.T) {
	e := newSpellTestEditor(t, "thiss\n")
	e.spellErrors = []spellError{{start: 0, end: 5, word: "thiss"}}
	e.spellErrorIdx = 0
	e.spellActive = true
	e.spellHandleKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'q'})
	if e.spellActive {
		t.Fatal("'q' should cancel spell check")
	}
}

func TestSpellHandleKey_SkipAdvances(t *testing.T) {
	e := newSpellTestEditor(t, "thiss tpyo\n")
	e.spellErrors = []spellError{
		{start: 0, end: 5, word: "thiss"},
		{start: 6, end: 10, word: "tpyo"},
	}
	e.spellErrorIdx = 0
	e.spellActive = true
	e.spellHandleKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: ' '})
	if e.spellErrorIdx != 1 {
		t.Fatalf("space should advance to next error, idx=%d", e.spellErrorIdx)
	}
}

func TestSpellHandleKey_DigitReplaces(t *testing.T) {
	e := newSpellTestEditor(t, "teh\n")
	e.spellErrors = []spellError{{start: 0, end: 3, word: "teh"}}
	e.spellErrorIdx = 0
	e.spellActive = true
	e.spellCurrentSugs = []string{"the", "tech"}
	e.spellHandleKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: '1'})
	if !strings.HasPrefix(buf(e).String(), "the") {
		t.Fatalf("digit selection should replace word, got %q", buf(e).String())
	}
}

func TestSpellHandleKey_ReplaceViaMinibuffer(t *testing.T) {
	e := newSpellTestEditor(t, "teh\n")
	e.spellErrors = []spellError{{start: 0, end: 3, word: "teh"}}
	e.spellErrorIdx = 0
	e.spellActive = true
	e.spellHandleKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'r'})
	if e.minibufDoneFunc == nil {
		t.Fatal("'r' should prompt for a replacement")
	}
	e.minibufDoneFunc("the")
	if !strings.HasPrefix(buf(e).String(), "the") {
		t.Fatalf("replacement should be applied, got %q", buf(e).String())
	}
}

func TestSpellShowCurrent_Done(t *testing.T) {
	e := newSpellTestEditor(t, "x\n")
	e.spellErrors = []spellError{{start: 0, end: 1, word: "x"}}
	e.spellErrorIdx = 1 // past the end
	e.spellActive = true
	e.spellShowCurrent()
	if e.spellActive {
		t.Fatal("spellShowCurrent past the end should finish the session")
	}
}

// ---------------------------------------------------------------------------
// getSpellSpans — early-return and cache-hit paths (no external aspell)
// ---------------------------------------------------------------------------

func TestGetSpellSpans_NoCommand(t *testing.T) {
	e := newCapTestEditor("hello wrold")
	e.spellCommand = ""
	if spans := e.getSpellSpans(e.ActiveBuffer()); spans != nil {
		t.Fatalf("expected nil spans with no spell command, got %v", spans)
	}
}

func TestGetSpellSpans_ReadOnly(t *testing.T) {
	e := newCapTestEditor("hello wrold")
	e.spellCommand = "aspell"
	b := e.ActiveBuffer()
	b.SetMode("text")
	b.SetReadOnly(true)
	if spans := e.getSpellSpans(b); spans != nil {
		t.Fatalf("expected nil spans for read-only buffer, got %v", spans)
	}
}

func TestGetSpellSpans_NonSpellMode(t *testing.T) {
	e := newCapTestEditor("hello wrold")
	e.spellCommand = "aspell"
	b := e.ActiveBuffer()
	b.SetMode("json") // neither spellCheckAll nor spellCheckComments
	if spans := e.getSpellSpans(b); spans != nil {
		t.Fatalf("expected nil spans for non-spell mode, got %v", spans)
	}
}

func TestGetSpellSpans_CacheHit(t *testing.T) {
	e := newCapTestEditor("hello wrold")
	e.spellCommand = "aspell"
	b := e.ActiveBuffer()
	b.SetMode("text")
	want := []syntax.Span{{Start: 6, End: 11, Face: FaceSpellError}}
	e.spellCaches = map[*buffer.Buffer]*spellCache{
		b: {gen: b.ChangeGen(), spans: want, spansSet: true},
	}
	got := e.getSpellSpans(b)
	if len(got) != 1 || got[0].Start != 6 {
		t.Fatalf("expected cached spans, got %v", got)
	}
}

func TestGetSpellSpans_PendingReturnsCache(t *testing.T) {
	e := newCapTestEditor("hello wrold")
	e.spellCommand = "aspell"
	b := e.ActiveBuffer()
	b.SetMode("text")
	gen := b.ChangeGen()
	stale := []syntax.Span{{Start: 0, End: 5, Face: FaceSpellError}}
	e.spellCaches = map[*buffer.Buffer]*spellCache{b: {gen: gen - 1, spans: stale}}
	e.spellPending = map[*buffer.Buffer]int{b: gen} // in-flight for this gen
	got := e.getSpellSpans(b)
	if len(got) != 1 {
		t.Fatalf("expected stale cached spans while pending, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// spellHandleKey — interactive spell check (hermetic: spellCommand="true")
// ---------------------------------------------------------------------------

// newInteractiveSpellEditor returns a capture editor with two pending spell
// errors and a no-op spell command so no real aspell process is spawned.
func newInteractiveSpellEditor(content string) *Editor {
	e := newCapTestEditor(content)
	e.spellCommand = "true" // exits 0 with no output → no suggestions
	e.spellLanguage = "en"
	e.spellActive = true
	e.spellErrors = []spellError{
		{start: 0, end: 5, word: "helo"},
		{start: 6, end: 11, word: "wrold"},
	}
	e.spellErrorIdx = 0
	return e
}

func TestSpellHandleKey_Quit(t *testing.T) {
	e := newInteractiveSpellEditor("helo wrold")
	e.spellHandleKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'q'})
	if e.spellActive {
		t.Fatal("'q' should cancel spell check")
	}
}

func TestSpellHandleKey_CtrlGQuits(t *testing.T) {
	e := newInteractiveSpellEditor("helo wrold")
	e.spellHandleKey(terminal.KeyEvent{Key: tcell.KeyCtrlG})
	if e.spellActive {
		t.Fatal("C-g should cancel spell check")
	}
}

func TestSpellHandleKey_Skip(t *testing.T) {
	e := newInteractiveSpellEditor("helo wrold")
	e.spellHandleKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: ' '})
	if e.spellErrorIdx != 1 {
		t.Fatalf("space should advance to next error, idx=%d", e.spellErrorIdx)
	}
}

func TestSpellHandleKey_SelectSuggestion(t *testing.T) {
	e := newInteractiveSpellEditor("helo wrold")
	e.spellCurrentSugs = []string{"hello"}
	e.spellHandleKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: '1'})
	if !strings.HasPrefix(e.ActiveBuffer().String(), "hello") {
		t.Fatalf("digit should replace word with suggestion, got %q", e.ActiveBuffer().String())
	}
	if len(e.spellErrors) != 1 {
		t.Fatalf("replaced error should be removed, %d remain", len(e.spellErrors))
	}
}

func TestSpellHandleKey_SelectOutOfRangeSuggestion(t *testing.T) {
	e := newInteractiveSpellEditor("helo wrold")
	e.spellCurrentSugs = []string{"hello"}
	e.spellHandleKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: '9'}) // no 9th suggestion
	if e.ActiveBuffer().String() != "helo wrold" {
		t.Fatalf("out-of-range digit should not change buffer, got %q", e.ActiveBuffer().String())
	}
}

func TestSpellHandleKey_ReplacePromptsMinibuffer(t *testing.T) {
	e := newInteractiveSpellEditor("helo wrold")
	e.spellHandleKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'r'})
	if e.spellActive {
		t.Fatal("'r' should deactivate interactive mode while prompting")
	}
	if !e.minibufActive {
		t.Fatal("'r' should open the minibuffer for replacement input")
	}
}

func TestSpellHandleKey_UnknownReshows(t *testing.T) {
	e := newInteractiveSpellEditor("helo wrold")
	e.spellHandleKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'Z'})
	// Unknown key just re-shows the prompt; still active.
	if !e.spellActive {
		t.Fatal("unknown key should keep spell check active")
	}
}

func TestSpellHandleKey_AddToDict(t *testing.T) {
	e := newInteractiveSpellEditor("helo wrold")
	// Add a third error duplicating the first word to verify all occurrences
	// of the added word are dropped from the error list.
	e.spellErrors = append(e.spellErrors, spellError{start: 12, end: 16, word: "helo"})
	e.spellHandleKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'i'})
	for _, se := range e.spellErrors {
		if se.word == "helo" {
			t.Fatalf("all %q occurrences should be removed after add-to-dict", "helo")
		}
	}
	// The spell cache for the buffer should be invalidated.
	if _, ok := e.spellCaches[e.ActiveBuffer()]; ok {
		t.Error("add-to-dict should invalidate the buffer spell cache")
	}
}

func TestSpellHandleKey_AddToDict_NoErrorsNoop(t *testing.T) {
	e := newInteractiveSpellEditor("helo wrold")
	e.spellErrorIdx = len(e.spellErrors) // out of range
	before := e.ActiveBuffer().String()
	e.spellHandleKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'i'})
	if e.ActiveBuffer().String() != before {
		t.Error("add-to-dict with no current error should not modify the buffer")
	}
}

func TestSpellHandleKey_ReplaceCallbackReplaces(t *testing.T) {
	e := newInteractiveSpellEditor("helo wrold")
	e.spellHandleKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'r'})
	if e.minibufDoneFunc == nil {
		t.Fatal("'r' should open a replacement minibuffer")
	}
	e.minibufDoneFunc("hello")
	if !strings.HasPrefix(e.ActiveBuffer().String(), "hello") {
		t.Fatalf("replacement should rewrite the word, got %q", e.ActiveBuffer().String())
	}
	if len(e.spellErrors) != 1 {
		t.Fatalf("replaced error should be removed, %d remain", len(e.spellErrors))
	}
	if !e.spellActive {
		t.Error("spell check should resume after a replacement")
	}
}

func TestSpellHandleKey_ReplaceCallbackEmptyResumes(t *testing.T) {
	e := newInteractiveSpellEditor("helo wrold")
	e.spellHandleKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'r'})
	e.minibufDoneFunc("") // empty input cancels the replacement
	if e.ActiveBuffer().String() != "helo wrold" {
		t.Errorf("empty replacement should not change the buffer, got %q", e.ActiveBuffer().String())
	}
	if !e.spellActive {
		t.Error("empty replacement should resume the spell check")
	}
	if len(e.spellErrors) != 2 {
		t.Errorf("empty replacement should keep all errors, %d remain", len(e.spellErrors))
	}
}

func TestSpellHandleKey_ReplaceNoErrorNoop(t *testing.T) {
	e := newInteractiveSpellEditor("helo wrold")
	e.spellErrorIdx = len(e.spellErrors) // out of range
	e.spellHandleKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'r'})
	if e.minibufActive {
		t.Error("'r' with no current error should not open a minibuffer")
	}
}

func TestSpellHandleKey_DigitNoErrorNoop(t *testing.T) {
	e := newInteractiveSpellEditor("helo wrold")
	e.spellCurrentSugs = []string{"hello"}
	e.spellErrorIdx = len(e.spellErrors) // out of range
	before := e.ActiveBuffer().String()
	e.spellHandleKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: '1'})
	if e.ActiveBuffer().String() != before {
		t.Error("digit with no current error should not modify the buffer")
	}
}

// ---------------------------------------------------------------------------
// getSpellSpans debounce
// ---------------------------------------------------------------------------

// TestGetSpellSpans_DebounceSkipsRapidEdits checks that a burst of edits does
// not dispatch a fresh aspell run (and full-buffer copy) per redraw.
func TestGetSpellSpans_DebounceSkipsRapidEdits(t *testing.T) {
	e := newCapTestEditor("hello wrold")
	e.spellCommand = "true" // no real aspell process
	b := e.ActiveBuffer()
	b.SetMode("text")

	// First call dispatches and records the spawn time.
	e.getSpellSpans(b)
	c := e.spellCaches[b]
	if c == nil {
		t.Fatal("expected a spell cache record after the first call")
	}
	if c.lastSpawn.IsZero() {
		t.Fatal("first call should have recorded lastSpawn")
	}
	firstSpawn := c.lastSpawn

	// A new generation within the debounce window must not re-dispatch.
	delete(e.spellPending, b)
	b.InsertString(0, "x")
	e.getSpellSpans(b)

	if !e.spellCaches[b].lastSpawn.Equal(firstSpawn) {
		t.Error("edit inside the debounce window should not dispatch a new check")
	}
	if !e.spellCaches[b].wakeArmed {
		t.Error("a debounced call should arm a recheck wakeup")
	}
}

// TestGetSpellSpans_DebounceElapsedDispatches checks the window actually opens
// again once it has passed, so edits are never silently dropped.
func TestGetSpellSpans_DebounceElapsedDispatches(t *testing.T) {
	e := newCapTestEditor("hello wrold")
	e.spellCommand = "true"
	b := e.ActiveBuffer()
	b.SetMode("text")

	e.getSpellSpans(b)
	c := e.spellCaches[b]
	firstSpawn := c.lastSpawn

	// Pretend the debounce window has fully elapsed.
	c.lastSpawn = time.Now().Add(-2 * spellCheckDebounce)
	delete(e.spellPending, b)
	b.InsertString(0, "x")
	e.getSpellSpans(b)

	if e.spellCaches[b].lastSpawn.Equal(firstSpawn) {
		t.Error("expected a new dispatch once the debounce window elapsed")
	}
}

// TestArmSpellRecheck_ArmsOnlyOnce guards against one timer per keystroke.
func TestArmSpellRecheck_ArmsOnlyOnce(t *testing.T) {
	e := newCapTestEditor("hello wrold")
	b := e.ActiveBuffer()
	c := &spellCache{lastSpawn: time.Now()}
	e.spellCaches = map[*buffer.Buffer]*spellCache{b: c}

	e.armSpellRecheck(b, c)
	if !c.wakeArmed {
		t.Fatal("armSpellRecheck should set wakeArmed")
	}
	// Second call must be a no-op while one is still pending.
	e.armSpellRecheck(b, c)
	if !c.wakeArmed {
		t.Error("wakeArmed should stay set")
	}
}

// TestGetSpellSpans_SpansSetDistinguishesUncomputed pins why spansSet exists:
// ApplyUndo decrements ChangeGen, so no gen value can mean "not yet computed".
func TestGetSpellSpans_SpansSetDistinguishesUncomputed(t *testing.T) {
	e := newCapTestEditor("hello wrold")
	e.spellCommand = "true"
	b := e.ActiveBuffer()
	b.SetMode("text")

	// A record for the current generation but with nothing computed yet must
	// not be treated as a cache hit.
	gen := b.ChangeGen()
	e.spellCaches = map[*buffer.Buffer]*spellCache{b: {gen: gen}}
	e.getSpellSpans(b)

	if e.spellPending[b] != gen {
		t.Error("expected a check to be dispatched when spansSet is false")
	}
}

// TestGetSpellSpans_FreshBufferIsChecked is a regression test: a freshly loaded
// buffer sits at ChangeGen 0, which is also the zero value a missing
// spellPending entry reads back as.  A bare map lookup therefore made the very
// first check of every file look "already in flight", so an unedited file was
// never spell-checked at all.
func TestGetSpellSpans_FreshBufferIsChecked(t *testing.T) {
	e := newCapTestEditor("hello wrold")
	e.spellCommand = "true"
	b := e.ActiveBuffer()
	b.SetMode("text")

	if got := b.ChangeGen(); got != 0 {
		t.Fatalf("precondition: fresh buffer ChangeGen = %d, want 0", got)
	}
	if _, ok := e.spellPending[b]; ok {
		t.Fatal("precondition: nothing should be pending yet")
	}

	e.getSpellSpans(b)

	if pg, ok := e.spellPending[b]; !ok || pg != 0 {
		t.Errorf("expected a check dispatched for generation 0, pending=%v ok=%v", pg, ok)
	}
}
