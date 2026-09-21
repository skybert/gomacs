package editor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/terminal"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func keRune2(r rune) terminal.KeyEvent {
	return terminal.KeyEvent{Key: tcell.KeyRune, Rune: r}
}

// startQR is a helper: resets point to 0 then starts query-replace.
func startQR(e *Editor, from, to string) {
	e.ActiveBuffer().SetPoint(0)
	e.startQueryReplace(from, to)
}

func TestStartQueryReplace_NoMatch(t *testing.T) {
	e := newTestEditor("hello world")
	e.startQueryReplace("xyz", "ABC")
	if e.queryReplaceActive {
		t.Error("queryReplaceActive should be false when no match found")
	}
}

func TestStartQueryReplace_WithMatch(t *testing.T) {
	e := newTestEditor("hello world")
	startQR(e, "hello", "goodbye")
	if !e.queryReplaceActive {
		t.Error("queryReplaceActive should be true when a match is found")
	}
	if e.queryReplaceMatch != 0 {
		t.Errorf("queryReplaceMatch should be 0, got %d", e.queryReplaceMatch)
	}
}

func TestStartQueryReplace_EmptyFrom(t *testing.T) {
	e := newTestEditor("hello world")
	e.startQueryReplace("", "replacement")
	if e.queryReplaceActive {
		t.Error("startQueryReplace with empty from should not activate")
	}
}

func TestStartQueryReplace_SetsFromRunes(t *testing.T) {
	e := newTestEditor("hello world")
	startQR(e, "hello", "goodbye")
	if string(e.queryReplaceFromRunes) != "hello" {
		t.Errorf("queryReplaceFromRunes = %q, want %q", string(e.queryReplaceFromRunes), "hello")
	}
}

// ---------------------------------------------------------------------------
// queryReplaceHandleKey — y (replace and continue)
// ---------------------------------------------------------------------------

func TestQueryReplaceHandleKey_Y_ReplacesAndContinues(t *testing.T) {
	e := newTestEditor("foo foo foo")
	startQR(e, "foo", "bar")
	if !e.queryReplaceActive {
		t.Fatal("query replace did not activate")
	}
	// First 'y': replace first occurrence.
	e.queryReplaceHandleKey(keRune2('y'))
	got := e.ActiveBuffer().String()
	if got[:3] != "bar" {
		t.Errorf("after first y: buffer starts with %q, want %q", got[:3], "bar")
	}
	// Should have found next occurrence.
	if !e.queryReplaceActive {
		t.Error("queryReplaceActive should still be true after first replacement")
	}
}

func TestQueryReplaceHandleKey_Y_AllReplacements(t *testing.T) {
	e := newTestEditor("foo foo")
	startQR(e, "foo", "bar")
	e.queryReplaceHandleKey(keRune2('y'))
	e.queryReplaceHandleKey(keRune2('y'))
	got := e.ActiveBuffer().String()
	want := "bar bar"
	if got != want {
		t.Errorf("buffer = %q, want %q", got, want)
	}
	if e.queryReplaceActive {
		t.Error("queryReplaceActive should be false after all replacements")
	}
}

// ---------------------------------------------------------------------------
// queryReplaceHandleKey — n (skip)
// ---------------------------------------------------------------------------

func TestQueryReplaceHandleKey_N_Skips(t *testing.T) {
	e := newTestEditor("foo foo")
	startQR(e, "foo", "bar")
	e.queryReplaceHandleKey(keRune2('n'))
	// Buffer unchanged, but moved to second occurrence.
	if e.ActiveBuffer().String() != "foo foo" {
		t.Errorf("after n: buffer changed unexpectedly: %q", e.ActiveBuffer().String())
	}
	if e.queryReplaceMatch != 4 {
		t.Errorf("queryReplaceMatch should be 4 (second 'foo'), got %d", e.queryReplaceMatch)
	}
}

// ---------------------------------------------------------------------------
// queryReplaceHandleKey — ! (replace all)
// ---------------------------------------------------------------------------

func TestQueryReplaceHandleKey_Bang_ReplacesAll(t *testing.T) {
	e := newTestEditor("foo foo foo")
	startQR(e, "foo", "bar")
	e.queryReplaceHandleKey(keRune2('!'))
	got := e.ActiveBuffer().String()
	want := "bar bar bar"
	if got != want {
		t.Errorf("buffer = %q, want %q", got, want)
	}
	if e.queryReplaceActive {
		t.Error("queryReplaceActive should be false after replace-all")
	}
}

// ---------------------------------------------------------------------------
// queryReplaceHandleKey — . (replace one and quit)
// ---------------------------------------------------------------------------

func TestQueryReplaceHandleKey_Dot_ReplacesAndQuits(t *testing.T) {
	e := newTestEditor("foo foo")
	startQR(e, "foo", "bar")
	e.queryReplaceHandleKey(keRune2('.'))
	got := e.ActiveBuffer().String()
	if got != "bar foo" {
		t.Errorf("buffer = %q, want %q", got, "bar foo")
	}
	if e.queryReplaceActive {
		t.Error("queryReplaceActive should be false after dot")
	}
}

// ---------------------------------------------------------------------------
// queryReplaceHandleKey — q / C-g (quit)
// ---------------------------------------------------------------------------

func TestQueryReplaceHandleKey_Q_Quits(t *testing.T) {
	e := newTestEditor("foo foo")
	startQR(e, "foo", "bar")
	e.queryReplaceHandleKey(keRune2('q'))
	if e.queryReplaceActive {
		t.Error("queryReplaceActive should be false after q")
	}
	// Buffer should be unchanged.
	if e.ActiveBuffer().String() != "foo foo" {
		t.Errorf("buffer changed after quit: %q", e.ActiveBuffer().String())
	}
}

func TestQueryReplaceHandleKey_CtrlG_Quits(t *testing.T) {
	e := newTestEditor("hello world")
	startQR(e, "hello", "goodbye")
	e.queryReplaceHandleKey(terminal.KeyEvent{Key: tcell.KeyCtrlG})
	if e.queryReplaceActive {
		t.Error("queryReplaceActive should be false after C-g")
	}
}

// ---------------------------------------------------------------------------
// queryReplaceDoReplaceRaw
// ---------------------------------------------------------------------------

func TestQueryReplaceDoReplaceRaw_ReplacesInPlace(t *testing.T) {
	e := newTestEditor("hello world")
	startQR(e, "hello", "goodbye")
	e.queryReplaceDoReplaceRaw()
	got := e.ActiveBuffer().String()
	want := "goodbye world"
	if got != want {
		t.Errorf("buffer = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// cmdQueryReplace — drives the two nested minibuffer prompts
// ---------------------------------------------------------------------------

func TestCmdQueryReplace_FullFlow(t *testing.T) {
	e := newTestEditor("hello world")
	e.ActiveBuffer().SetPoint(0)
	e.cmdQueryReplace()
	if e.minibufDoneFunc == nil {
		t.Fatal("cmdQueryReplace should open a minibuffer for the search string")
	}
	// Answer the "from" prompt, which opens the "to" prompt.
	e.minibufDoneFunc("hello")
	if e.minibufDoneFunc == nil {
		t.Fatal("answering 'from' should open a second minibuffer for the replacement")
	}
	// Answer the "to" prompt, which starts query-replace.
	e.minibufDoneFunc("goodbye")
	if !e.queryReplaceActive {
		t.Fatal("query-replace should be active after both prompts are answered")
	}
	if e.queryReplaceFrom != "hello" || e.queryReplaceTo != "goodbye" {
		t.Errorf("from/to = %q/%q, want hello/goodbye", e.queryReplaceFrom, e.queryReplaceTo)
	}
}

func TestCmdQueryReplace_EmptyFromCallbackAborts(t *testing.T) {
	e := newTestEditor("hello world")
	e.cmdQueryReplace()
	// Submitting an empty "from" returns early without opening a second prompt
	// or starting query-replace.
	e.minibufDoneFunc("")
	if e.queryReplaceActive {
		t.Error("empty search string should abort without starting query-replace")
	}
}

// ---------------------------------------------------------------------------
// replace-all: batching, counts, and pathological replacement strings
// ---------------------------------------------------------------------------

// qrBang runs startQueryReplace from point 0 and then presses '!'.
func qrBang(e *Editor, from, to string) {
	startQR(e, from, to)
	e.queryReplaceHandleKey(keRune2('!'))
}

func TestQueryReplaceAll_ManyMatches(t *testing.T) {
	const n = 500
	var sb strings.Builder
	for range n {
		sb.WriteString("alpha beta\n")
	}
	e := newTestEditor(sb.String())
	startQR(e, "beta", "gamma")
	count := e.queryReplaceAll()
	if count != n {
		t.Errorf("count = %d, want %d", count, n)
	}
	want := strings.ReplaceAll(sb.String(), "beta", "gamma")
	if got := e.ActiveBuffer().String(); got != want {
		t.Errorf("buffer text wrong after replace-all (len %d, want %d)", len(got), len(want))
	}
}

func TestQueryReplaceAll_MessageReportsCount(t *testing.T) {
	e := newTestEditor("foo foo foo foo")
	qrBang(e, "foo", "bar")
	if !strings.Contains(e.message, "Replaced 4 occurrence(s)") {
		t.Errorf("message = %q, want it to report 4 replacements", e.message)
	}
	if e.queryReplaceActive {
		t.Error("query-replace should be finished after !")
	}
}

// The replacement text contains the search string: the inserted text must not
// be rescanned (no infinite loop, no double replacement).
func TestQueryReplaceAll_ReplacementContainsSearchString(t *testing.T) {
	e := newTestEditor("foo foo foo")
	startQR(e, "foo", "foofoo")
	count := e.queryReplaceAll()
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}
	want := "foofoo foofoo foofoo"
	if got := e.ActiveBuffer().String(); got != want {
		t.Errorf("buffer = %q, want %q", got, want)
	}
}

func TestQueryReplaceAll_ReplacementIsSuperstringOfSearch(t *testing.T) {
	e := newTestEditor("x ab ab y")
	startQR(e, "ab", "abab")
	if count := e.queryReplaceAll(); count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	if got := e.ActiveBuffer().String(); got != "x abab abab y" {
		t.Errorf("buffer = %q, want %q", got, "x abab abab y")
	}
}

func TestQueryReplaceAll_LengthVariants(t *testing.T) {
	cases := []struct {
		name           string
		input          string
		from, to       string
		want           string
		wantCount      int
		wantFinalPoint int
	}{
		{
			name: "shorter", input: "aaa-longword-bbb-longword-ccc",
			from: "longword", to: "x",
			want: "aaa-x-bbb-x-ccc", wantCount: 2, wantFinalPoint: 11,
		},
		{
			name: "longer", input: "a-x-b-x-c",
			from: "x", to: "longword",
			want: "a-longword-b-longword-c", wantCount: 2, wantFinalPoint: 21,
		},
		{
			name: "equal", input: "cat dog cat dog",
			from: "cat", to: "COW",
			want: "COW dog COW dog", wantCount: 2, wantFinalPoint: 11,
		},
		{
			name: "empty replacement deletes", input: "a-zz-b-zz-c",
			from: "zz", to: "",
			want: "a--b--c", wantCount: 2, wantFinalPoint: 5,
		},
		{
			name: "single match", input: "only one here",
			from: "one", to: "two",
			want: "only two here", wantCount: 1, wantFinalPoint: 8,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newTestEditor(tc.input)
			startQR(e, tc.from, tc.to)
			count := e.queryReplaceAll()
			if count != tc.wantCount {
				t.Errorf("count = %d, want %d", count, tc.wantCount)
			}
			if got := e.ActiveBuffer().String(); got != tc.want {
				t.Errorf("buffer = %q, want %q", got, tc.want)
			}
			if got := e.ActiveBuffer().Point(); got != tc.wantFinalPoint {
				t.Errorf("point = %d, want %d (just after last replacement)", got, tc.wantFinalPoint)
			}
		})
	}
}

// Overlapping candidates: after a match the scan resumes past it, so "aaaa"
// with from="aa" yields two replacements, not three.
func TestQueryReplaceAll_OverlappingMatches(t *testing.T) {
	e := newTestEditor("aaaa")
	startQR(e, "aa", "b")
	if count := e.queryReplaceAll(); count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	if got := e.ActiveBuffer().String(); got != "bb" {
		t.Errorf("buffer = %q, want %q", got, "bb")
	}
}

// Replace-all only touches matches at or after the current match; text before
// it — and anything the user already skipped with 'n' — is left alone.
func TestQueryReplaceAll_StartsAtCurrentMatch(t *testing.T) {
	e := newTestEditor("foo foo foo")
	startQR(e, "foo", "bar")
	e.queryReplaceHandleKey(keRune2('n')) // skip the first occurrence
	count := e.queryReplaceAll()
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	if got := e.ActiveBuffer().String(); got != "foo bar bar" {
		t.Errorf("buffer = %q, want %q", got, "foo bar bar")
	}
}

func TestQueryReplaceAll_IsCaseSensitive(t *testing.T) {
	e := newTestEditor("Foo foo FOO foo")
	startQR(e, "foo", "bar")
	if count := e.queryReplaceAll(); count != 2 {
		t.Errorf("count = %d, want 2 (case-sensitive)", count)
	}
	if got := e.ActiveBuffer().String(); got != "Foo bar FOO bar" {
		t.Errorf("buffer = %q, want %q", got, "Foo bar FOO bar")
	}
}

func TestQueryReplaceAll_NoCurrentMatchReturnsZero(t *testing.T) {
	e := newTestEditor("foo foo")
	e.queryReplaceFrom = "foo"
	e.queryReplaceFromRunes = []rune("foo")
	e.queryReplaceTo = "bar"
	e.queryReplaceMatch = -1
	if count := e.queryReplaceAll(); count != 0 {
		t.Errorf("count = %d, want 0 when there is no current match", count)
	}
	if got := e.ActiveBuffer().String(); got != "foo foo" {
		t.Errorf("buffer changed: %q", got)
	}
}

func TestQueryReplaceAll_MultibyteRunePositions(t *testing.T) {
	e := newTestEditor("æøå naïve æøå naïve")
	startQR(e, "naïve", "simple")
	if count := e.queryReplaceAll(); count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	if got := e.ActiveBuffer().String(); got != "æøå simple æøå simple" {
		t.Errorf("buffer = %q", got)
	}
}

// ---------------------------------------------------------------------------
// undo granularity
// ---------------------------------------------------------------------------

// Replace-all is a single buffer edit, so one undo restores the original text.
func TestQueryReplaceAll_SingleUndoRestoresBuffer(t *testing.T) {
	const orig = "foo bar foo bar foo"
	e := newTestEditor(orig)
	qrBang(e, "foo", "QUUX")
	if got := e.ActiveBuffer().String(); got != "QUUX bar QUUX bar QUUX" {
		t.Fatalf("buffer = %q", got)
	}
	e.ActiveBuffer().ApplyUndo()
	if got := e.ActiveBuffer().String(); got != orig {
		t.Errorf("after one undo: buffer = %q, want %q", got, orig)
	}
}

// A single 'y' replacement is likewise one undo step.
func TestQueryReplaceY_SingleUndoPerReplacement(t *testing.T) {
	e := newTestEditor("foo foo")
	startQR(e, "foo", "bar")
	e.queryReplaceHandleKey(keRune2('y'))
	if got := e.ActiveBuffer().String(); got != "bar foo" {
		t.Fatalf("buffer = %q, want %q", got, "bar foo")
	}
	e.ActiveBuffer().ApplyUndo()
	if got := e.ActiveBuffer().String(); got != "foo foo" {
		t.Errorf("after one undo: buffer = %q, want %q", got, "foo foo")
	}
}

// ---------------------------------------------------------------------------
// y / n / q / . state machine regression checks
// ---------------------------------------------------------------------------

func TestQueryReplaceStateMachine_MixedYAndN(t *testing.T) {
	e := newTestEditor("foo foo foo foo")
	startQR(e, "foo", "bar")
	e.queryReplaceHandleKey(keRune2('y')) // replace #1
	e.queryReplaceHandleKey(keRune2('n')) // skip #2
	e.queryReplaceHandleKey(keRune2('y')) // replace #3
	e.queryReplaceHandleKey(keRune2('n')) // skip #4 -> no more matches
	if got := e.ActiveBuffer().String(); got != "bar foo bar foo" {
		t.Errorf("buffer = %q, want %q", got, "bar foo bar foo")
	}
	if e.queryReplaceActive {
		t.Error("query-replace should be finished after the last skip")
	}
}

func TestQueryReplaceStateMachine_SpaceIsSameAsY(t *testing.T) {
	e := newTestEditor("foo foo")
	startQR(e, "foo", "bar")
	e.queryReplaceHandleKey(keRune2(' '))
	if got := e.ActiveBuffer().String(); got != "bar foo" {
		t.Errorf("buffer = %q, want %q", got, "bar foo")
	}
	if !e.queryReplaceActive {
		t.Error("SPC should replace and continue")
	}
	if e.queryReplaceMatch != 4 {
		t.Errorf("queryReplaceMatch = %d, want 4", e.queryReplaceMatch)
	}
}

func TestQueryReplaceStateMachine_QuitAfterSomeReplacements(t *testing.T) {
	e := newTestEditor("foo foo foo")
	startQR(e, "foo", "bar")
	e.queryReplaceHandleKey(keRune2('y'))
	e.queryReplaceHandleKey(keRune2('q'))
	if got := e.ActiveBuffer().String(); got != "bar foo foo" {
		t.Errorf("buffer = %q, want %q", got, "bar foo foo")
	}
	if e.queryReplaceActive {
		t.Error("q should end query-replace")
	}
}

func TestQueryReplaceStateMachine_DotAfterSkip(t *testing.T) {
	e := newTestEditor("foo foo foo")
	startQR(e, "foo", "bar")
	e.queryReplaceHandleKey(keRune2('n'))
	e.queryReplaceHandleKey(keRune2('.'))
	if got := e.ActiveBuffer().String(); got != "foo bar foo" {
		t.Errorf("buffer = %q, want %q", got, "foo bar foo")
	}
	if e.queryReplaceActive {
		t.Error(". should end query-replace")
	}
}

func TestQueryReplaceStateMachine_HelpKeepsSessionAlive(t *testing.T) {
	e := newTestEditor("foo foo")
	startQR(e, "foo", "bar")
	e.queryReplaceHandleKey(keRune2('?'))
	if !e.queryReplaceActive {
		t.Error("? should not end query-replace")
	}
	if e.queryReplaceMatch != 0 {
		t.Errorf("queryReplaceMatch = %d, want 0 (unchanged by ?)", e.queryReplaceMatch)
	}
	if got := e.ActiveBuffer().String(); got != "foo foo" {
		t.Errorf("? changed the buffer: %q", got)
	}
}

// ---------------------------------------------------------------------------
// rune cache reuse
// ---------------------------------------------------------------------------

// Skipping with 'n' must not re-materialise the buffer: the cached rune slice
// is keyed on the buffer's changeGen, which an unmodified buffer keeps.
func TestQueryReplaceFindNext_ReusesRuneCache(t *testing.T) {
	e := newTestEditor("foo foo foo")
	startQR(e, "foo", "bar")
	first := e.isearchRunes
	if first == nil {
		t.Fatal("expected the rune cache to be populated by the first search")
	}
	e.queryReplaceHandleKey(keRune2('n'))
	if &e.isearchRunes[0] != &first[0] {
		t.Error("skipping re-materialised the buffer instead of reusing the cache")
	}
}

func TestQueryReplaceFinish_ReleasesRuneCache(t *testing.T) {
	e := newTestEditor("foo foo")
	startQR(e, "foo", "bar")
	e.queryReplaceHandleKey(keRune2('q'))
	if e.isearchRunes != nil {
		t.Error("finishing query-replace should release the cached rune slice")
	}
}

// ---------------------------------------------------------------------------
// equivalence with the old incremental loop
// ---------------------------------------------------------------------------

// qrReplaceAllOld is the pre-fix '!' implementation: replace, then rescan the
// whole buffer from scratch.  Used as the reference oracle.
func qrReplaceAllOld(e *Editor) int {
	count := 0
	for e.queryReplaceMatch >= 0 {
		e.queryReplaceDoReplaceRaw()
		count++
		if !qrFindNextUncached(e) {
			break
		}
	}
	return count
}

// The batched replace-all must produce exactly the same text, count and point
// as the old one-match-at-a-time loop.
func TestQueryReplaceAll_MatchesOldAlgorithm(t *testing.T) {
	cases := []struct{ input, from, to string }{
		{"foo foo foo", "foo", "bar"},
		{"foo foo foo", "foo", "foofoo"},
		{"aaaa", "aa", "b"},
		{"aaaa", "aa", "aaa"},
		{"a-x-b-x-c", "x", "longword"},
		{"aaa-longword-bbb-longword-ccc", "longword", "x"},
		{"a-zz-b-zz-c", "zz", ""},
		{"no matches at all", "zzz", "q"},
		{"end with match foo", "foo", "bar"},
		{"foo at start", "foo", "barbaz"},
		{"æøå naïve æøå naïve", "naïve", "simple"},
		{"line1\nline1\nline1\n", "line1", "L\n1"},
		{"abababab", "abab", "ab"},
	}
	for _, tc := range cases {
		t.Run(tc.input+"/"+tc.from+"->"+tc.to, func(t *testing.T) {
			oldE := newTestEditor(tc.input)
			startQR(oldE, tc.from, tc.to)
			wantCount := qrReplaceAllOld(oldE)
			wantText := oldE.ActiveBuffer().String()
			wantPoint := oldE.ActiveBuffer().Point()

			newE := newTestEditor(tc.input)
			startQR(newE, tc.from, tc.to)
			gotCount := newE.queryReplaceAll()

			if gotCount != wantCount {
				t.Errorf("count = %d, want %d", gotCount, wantCount)
			}
			if got := newE.ActiveBuffer().String(); got != wantText {
				t.Errorf("text = %q, want %q", got, wantText)
			}
			if got := newE.ActiveBuffer().Point(); got != wantPoint {
				t.Errorf("point = %d, want %d", got, wantPoint)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// benchmark
// ---------------------------------------------------------------------------

// qrBenchContent builds a large buffer with one match per line.
func qrBenchContent(lines int) string {
	var sb strings.Builder
	sb.Grow(lines * 48)
	for i := range lines {
		sb.WriteString("func helper")
		sb.WriteString("(arg int) int { return arg + ")
		sb.WriteString(string(rune('0' + i%10)))
		sb.WriteString(" } // needle here\n")
	}
	return sb.String()
}

// qrBenchSizes are the buffer sizes (in lines, one match per line) used by the
// replace-all benchmarks.  The old algorithm is quadratic, so it is only run on
// the small size — extrapolate x100 for the large one.
var qrBenchSizes = []int{2000, 20000}

func BenchmarkQueryReplaceAll(b *testing.B) {
	for _, lines := range qrBenchSizes {
		content := qrBenchContent(lines)
		b.Run(fmt.Sprintf("lines=%d", lines), func(b *testing.B) {
			for b.Loop() {
				b.StopTimer()
				e := newTestEditor(content)
				e.ActiveBuffer().SetPoint(0)
				e.startQueryReplace("needle", "haystack")
				b.StartTimer()
				if n := e.queryReplaceAll(); n != lines {
					b.Fatalf("replaced %d, want %d", n, lines)
				}
			}
		})
	}
}

// BenchmarkQueryReplaceAllOld reproduces the previous algorithm (full
// buf.String() rescan per match) for comparison.
func BenchmarkQueryReplaceAllOld(b *testing.B) {
	lines := qrBenchSizes[0]
	content := qrBenchContent(lines)
	b.Run(fmt.Sprintf("lines=%d", lines), func(b *testing.B) {
		for b.Loop() {
			b.StopTimer()
			e := newTestEditor(content)
			e.ActiveBuffer().SetPoint(0)
			e.startQueryReplace("needle", "haystack")
			b.StartTimer()
			if n := qrReplaceAllOld(e); n != lines {
				b.Fatalf("replaced %d, want %d", n, lines)
			}
		}
	})
}

// qrFindNextUncached is the pre-fix queryReplaceFindNext: it re-materialises the
// whole buffer on every call.  Kept only to benchmark the regression.
func qrFindNextUncached(e *Editor) bool {
	buf := e.ActiveBuffer()
	runes := []rune(buf.String())
	needle := []rune(e.queryReplaceFrom)
	pos := e.queryReplaceCursor
	for pos <= len(runes)-len(needle) {
		if runesMatch(runes[pos:], needle) {
			e.queryReplaceMatch = pos
			buf.SetPoint(pos + len(needle))
			return true
		}
		pos++
	}
	e.queryReplaceMatch = -1
	return false
}

// BenchmarkQueryReplaceSkip measures the cost of a single 'n' keystroke, which
// used to copy the whole buffer.
func BenchmarkQueryReplaceSkip(b *testing.B) {
	content := qrBenchContent(20000)
	e := newTestEditor(content)
	e.ActiveBuffer().SetPoint(0)
	e.startQueryReplace("needle", "haystack")
	for b.Loop() {
		e.queryReplaceCursor = e.queryReplaceMatch + 1
		if !e.queryReplaceFindNext() {
			e.queryReplaceCursor = 0
			if !e.queryReplaceFindNext() {
				b.Fatal("no match")
			}
		}
	}
}
