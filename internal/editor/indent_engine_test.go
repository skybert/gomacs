package editor

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/skybert/gomacs/internal/buffer"
)

// ---------------------------------------------------------------------------
// netBraceCountJSON
// ---------------------------------------------------------------------------

func TestNetBraceCountJSONEmpty(t *testing.T) {
	if got := netBraceCountJSON(""); got != 0 {
		t.Errorf("empty line: want 0, got %d", got)
	}
}

func TestNetBraceCountJSONOpenCurly(t *testing.T) {
	if got := netBraceCountJSON("{"); got != 1 {
		t.Errorf("{: want 1, got %d", got)
	}
}

func TestNetBraceCountJSONOpenSquare(t *testing.T) {
	if got := netBraceCountJSON("["); got != 1 {
		t.Errorf("[: want 1, got %d", got)
	}
}

func TestNetBraceCountJSONClose(t *testing.T) {
	if got := netBraceCountJSON("}"); got != -1 {
		t.Errorf("}: want -1, got %d", got)
	}
	if got := netBraceCountJSON("]"); got != -1 {
		t.Errorf("]: want -1, got %d", got)
	}
}

func TestNetBraceCountJSONIgnoresStrings(t *testing.T) {
	// Braces inside strings must not be counted.
	if got := netBraceCountJSON(`"{ not a brace }"`); got != 0 {
		t.Errorf("brace in string: want 0, got %d", got)
	}
}

func TestNetBraceCountJSONEscapedQuote(t *testing.T) {
	// An escaped quote must not end the string prematurely.
	// The { after the escaped quote is still inside the string.
	if got := netBraceCountJSON(`"he said \"hello\" {"`); got != 0 {
		t.Errorf("escaped quote in string: want 0, got %d", got)
	}
}

func TestNetBraceCountJSONMixed(t *testing.T) {
	// "key": { opens one level
	if got := netBraceCountJSON(`"key": {`); got != 1 {
		t.Errorf(`"key": {: want 1, got %d`, got)
	}
}

// ---------------------------------------------------------------------------
// calcIndentJSON
// ---------------------------------------------------------------------------

func TestCalcIndentJSONTopLevel(t *testing.T) {
	lines := []string{`{`}
	// Line 0 is { itself; no preceding lines → depth 0 before line 0.
	if got := calcIndentJSON(lines, 0, "  "); got != "" {
		t.Errorf("top-level opening brace: want \"\", got %q", got)
	}
}

func TestCalcIndentJSONFirstKey(t *testing.T) {
	lines := []string{`{`, `"key": "value"`}
	// After {, depth is 1 → one level of indent.
	if got := calcIndentJSON(lines, 1, "  "); got != "  " {
		t.Errorf("first key: want \"  \", got %q", got)
	}
}

func TestCalcIndentJSONClosingBrace(t *testing.T) {
	lines := []string{`{`, `"key": "value"`, `}`}
	// } dedents: depth accumulated from previous lines is 1, then -1 → 0.
	if got := calcIndentJSON(lines, 2, "  "); got != "" {
		t.Errorf("closing brace: want \"\", got %q", got)
	}
}

func TestCalcIndentJSONNestedObject(t *testing.T) {
	lines := []string{
		`{`,
		`  "outer": {`,
		``,
	}
	// After two lines that each open one brace, depth = 2.
	if got := calcIndentJSON(lines, 2, "  "); got != "    " {
		t.Errorf("nested object: want \"    \", got %q", got)
	}
}

func TestCalcIndentJSONArray(t *testing.T) {
	lines := []string{`[`, ``}
	// After [, depth = 1.
	if got := calcIndentJSON(lines, 1, "  "); got != "  " {
		t.Errorf("array entry: want \"  \", got %q", got)
	}
}

func TestCalcIndentJSONClosingSquare(t *testing.T) {
	lines := []string{`[`, `  1`, `]`}
	// ] at start of line dedents from depth 1 → 0.
	if got := calcIndentJSON(lines, 2, "  "); got != "" {
		t.Errorf("closing bracket: want \"\", got %q", got)
	}
}

// ---------------------------------------------------------------------------
// benchmarks
// ---------------------------------------------------------------------------

// benchGoSource builds a Go source file with `funcs` seven-line functions,
// i.e. roughly 7×funcs lines of correctly indented, brace-nested code.
func benchGoSource(funcs int) string {
	var sb strings.Builder
	sb.WriteString("package main\n\n")
	for i := range funcs {
		fmt.Fprintf(&sb, "func f%d(x int) int {\n\tif x > 0 {\n\t\treturn x // positive\n\t}\n\treturn -x\n}\n\n", i)
	}
	return sb.String()
}

// benchBashSource builds a Bash script with `blocks` five-line if/for blocks.
func benchBashSource(blocks int) string {
	var sb strings.Builder
	sb.WriteString("#!/usr/bin/env bash\n\n")
	for i := range blocks {
		fmt.Fprintf(&sb, "if [ -f file%d ]; then\n  for f in a b c; do\n    echo \"$f\"\n  done\nfi\n", i)
	}
	return sb.String()
}

// benchPythonSource builds a Python file with `blocks` four-line functions.
func benchPythonSource(blocks int) string {
	var sb strings.Builder
	for i := range blocks {
		fmt.Fprintf(&sb, "def f%d(x):\n    if x:\n        return x\n    return -x\n", i)
	}
	return sb.String()
}

// BenchmarkIndentCurrentLineDeepInLargeFile measures one Tab press on an
// already correctly indented line 80% of the way into a ~10k line Go file —
// the case that used to copy the whole buffer and rescan every preceding line.
func BenchmarkIndentCurrentLineDeepInLargeFile(b *testing.B) {
	src := benchGoSource(1400)
	e := newTestEditor(src)
	bf := buf(e)
	bf.SetMode("go")
	bf.SetPoint(strings.Index(src, "\t\treturn x // positive\n\t}\n\treturn -x\n}\n\nfunc f1200("))
	for b.Loop() {
		indentCurrentLine(bf, "\t")
	}
}

// BenchmarkIndentNewlineDeepInLargeFile measures the auto-indent that follows
// every Enter keypress deep inside a large Go file: the buffer changes each
// iteration, so no cached depth checkpoint survives.
func BenchmarkIndentNewlineDeepInLargeFile(b *testing.B) {
	src := benchGoSource(1400)
	e := newTestEditor(src)
	bf := buf(e)
	bf.SetMode("go")
	bf.SetPoint(strings.Index(src, "func f1200("))
	for b.Loop() {
		pt := bf.Point()
		bf.Insert(pt, '\n')
		bf.SetPoint(pt + 1)
		indentCurrentLine(bf, "\t")
	}
}

// BenchmarkIndentCurrentLineDeepInLargeFileBash measures the Bash engine deep
// inside a large script.
func BenchmarkIndentCurrentLineDeepInLargeFileBash(b *testing.B) {
	src := benchBashSource(2000)
	e := newTestEditor(src)
	bf := buf(e)
	bf.SetMode("bash")
	bf.SetPoint(strings.Index(src, "if [ -f file1600 ]"))
	for b.Loop() {
		indentCurrentLine(bf, "  ")
	}
}

// BenchmarkIndentCurrentLineDeepInLargeFilePython measures the Python engine,
// which only ever needs the previous non-blank line.
func BenchmarkIndentCurrentLineDeepInLargeFilePython(b *testing.B) {
	src := benchPythonSource(2500)
	e := newTestEditor(src)
	bf := buf(e)
	bf.SetMode("python")
	bf.SetPoint(strings.Index(src, "def f2000(x):"))
	for b.Loop() {
		indentCurrentLine(bf, "    ")
	}
}

// ---------------------------------------------------------------------------
// calcIndentAt — differential tests against the []string engine
// ---------------------------------------------------------------------------

// goTricky exercises the corner cases of the brace scanner: braces inside
// strings, runes, raw strings and comments, closing braces at the start of a
// line, and more closers than openers (depth must clamp at 0).
const goTricky = `func tricky() {
s := "{ not a brace }"
r := '{'
raw := ` + "`" + `{{{` + "`" + `
x := 1 // } comment brace
if x > 0 {
call(
1,
)
}
}
}
}
`

// indentDiffCases are sources covering every indent engine.  Each is checked
// line by line: the buffer-backed calcIndentAt must agree with the []string
// calcIndent everywhere.  The generated sources are long enough (>128 lines) to
// force several depth checkpoints.
var indentDiffCases = []struct {
	name, mode, unit, src string
}{
	{"go", "go", "\t", benchGoSource(40) + goTricky},
	{"java", "java", "    ", benchGoSource(40) + goTricky},
	{"perl", "perl", "  ", "sub f {\nmy $x = 1; # { in a comment\nif ($x) {\nprint $x;\n}\n}\n\n" + benchGoSource(20)},
	{"bash", "bash", "  ", benchBashSource(40) + "if true; then\necho hi\nelse\necho ho\nfi\n# then\nfi\nfi\n"},
	{"json", "json", "  ", benchJSONSource(40) + "{\n\"a\": [\n1,\n{\n\"b\": \"} ]\"\n}\n]\n}\n]\n"},
	{"python", "python", "    ", benchPythonSource(50) + "def g(x):\n\nif x:\npass\nelse:\npass\n# comment:\nreturn\n"},
	{"markdown", "markdown", "  ", "# Title\n\n  indented\n\n\nnext\n\ttabbed\n"},
	{"fundamental", "fundamental", "  ", "\n\n   \nfirst real line\n  second\n"},
	{"empty", "go", "\t", ""},
	{"blank-first-lines", "python", "  ", "   \n\nx = 1\n"},
}

// benchJSONSource builds a JSON document with `entries` nested objects.
func benchJSONSource(entries int) string {
	var sb strings.Builder
	sb.WriteString("{\n")
	for i := range entries {
		fmt.Fprintf(&sb, "  \"key%d\": {\n    \"list\": [\n      1,\n      2\n    ]\n  },\n", i)
	}
	sb.WriteString("}\n")
	return sb.String()
}

// lineStartsOf returns the buffer position of the first rune of every line of
// src, mirroring the line indices of strings.Split(src, "\n").
func lineStartsOf(src string) []int {
	lines := strings.Split(src, "\n")
	starts := make([]int, len(lines))
	pos := 0
	for i, l := range lines {
		starts[i] = pos
		pos += len([]rune(l)) + 1 // +1 for the '\n'
	}
	return starts
}

// TestCalcIndentAtMatchesSliceEngine is a differential test: it only asserts
// that the buffer-backed engine agrees with the []string engine.  Because both
// engines share the underlying counters and *IndentFor helpers for most modes,
// what it really guards is the line reading and the depth checkpoint cache —
// not the indent rules, which are pinned by literal expectations in
// TestCalcIndentAtLiteralPerMode.
func TestCalcIndentAtMatchesSliceEngine(t *testing.T) {
	for _, tc := range indentDiffCases {
		e := newTestEditor(tc.src)
		b := buf(e)
		b.SetMode(tc.mode)
		lines := strings.Split(tc.src, "\n")
		starts := lineStartsOf(tc.src)

		for idx := range lines {
			want := calcIndent(tc.mode, lines, idx, tc.unit)
			if got := calcIndentAt(b, tc.mode, starts[idx], tc.unit); got != want {
				t.Fatalf("%s line %d (%q): calcIndentAt = %q, want %q",
					tc.name, idx, lines[idx], got, want)
			}
		}
	}
}

// TestCalcIndentAtMatchesSliceEngineBackwards walks the lines from the bottom
// up, so most lookups start from a checkpoint in the middle of the list rather
// than from its end.
func TestCalcIndentAtMatchesSliceEngineBackwards(t *testing.T) {
	for _, tc := range indentDiffCases {
		e := newTestEditor(tc.src)
		b := buf(e)
		b.SetMode(tc.mode)
		lines := strings.Split(tc.src, "\n")
		starts := lineStartsOf(tc.src)

		for idx := len(lines) - 1; idx >= 0; idx-- {
			want := calcIndent(tc.mode, lines, idx, tc.unit)
			if got := calcIndentAt(b, tc.mode, starts[idx], tc.unit); got != want {
				t.Fatalf("%s line %d (%q): calcIndentAt = %q, want %q",
					tc.name, idx, lines[idx], got, want)
			}
		}
	}
}

// TestCalcIndentAtColdCache checks the same equivalence with the checkpoint
// cache dropped before every single call.
func TestCalcIndentAtColdCache(t *testing.T) {
	tc := indentDiffCases[0]
	e := newTestEditor(tc.src)
	b := buf(e)
	b.SetMode(tc.mode)
	lines := strings.Split(tc.src, "\n")
	starts := lineStartsOf(tc.src)

	for idx := range lines {
		indentCaches = map[*buffer.Buffer]*indentCache{}
		want := calcIndent(tc.mode, lines, idx, tc.unit)
		if got := calcIndentAt(b, tc.mode, starts[idx], tc.unit); got != want {
			t.Fatalf("line %d: cold-cache calcIndentAt = %q, want %q", idx, got, want)
		}
	}
}

// TestCalcIndentAtAfterEditAbove is the invalidation test: an extra opening
// brace inserted near the top of the buffer must deepen the indentation of a
// line far below, i.e. the checkpoints taken before the edit must not be
// reused.  Both the before and the after indentation are absolute
// expectations, so a uniformly off-by-one engine cannot pass by keeping the
// difference right (cf. TestElispIndentLevelAtAfterEditAbove in
// indent_test.go).
func TestCalcIndentAtAfterEditAbove(t *testing.T) {
	src := benchGoSource(40)
	e := newTestEditor(src)
	b := buf(e)
	b.SetMode("go")

	starts := lineStartsOf(src)
	deep := starts[len(starts)-3] // the final "}" of the last function
	before := calcIndentAt(b, "go", deep, "\t")
	if before != "" {
		t.Fatalf("a top-level closing brace should want no indent, got %q", before)
	}

	// Insert an unclosed brace on the first line, far above `deep`.
	b.InsertString(len("package main"), " {")
	after := calcIndentAt(b, "go", deep+2, "\t")

	if after != "\t" {
		t.Errorf("after inserting an opening brace above: got %q, want %q", after, "\t")
	}
}

// TestCalcIndentAtLiteralPerMode gives the buffer-backed engine absolute
// expectations for each mode's indent rules.
//
// It complements TestCalcIndentAtMatchesSliceEngine and friends, which only
// assert that calcIndentAt agrees with calcIndent.  For most modes the two
// share the underlying decision helpers (the counters and *IndentFor), so the
// differential tests really exercise the line splitting and the depth
// checkpoint cache rather than the indent rules themselves; a wrong rule is
// wrong identically on both sides and the comparison still passes.  The cases
// below pin the rules.
func TestCalcIndentAtLiteralPerMode(t *testing.T) {
	tests := []struct {
		name string
		mode string
		unit string
		src  string
		line int // index into strings.Split(src, "\n")
		want string
	}{
		{"go after open brace", "go", "\t", "func foo() {\n\n}\n", 1, "\t"},
		{"go closing brace dedents", "go", "\t", "func foo() {\n\tx := 1\n}\n", 2, ""},
		{"go brace in comment ignored", "go", "\t", "// func foo() {\n\n", 1, ""},
		{"go brace in string ignored", "go", "\t", "s := \"{\"\n\n", 1, ""},
		{"go nested two levels", "go", "\t", "func foo() {\n\tif true {\n\n", 2, "\t\t"},
		{"java four-space unit", "java", "    ", "public class Foo {\n\n}\n", 1, "    "},
		{"perl hash comment ignored", "perl", "  ", "sub f {\n\tmy $x = 1; # {\n\n", 2, "  "},
		{"bash after then", "bash", "  ", "if true; then\n\n", 1, "  "},
		{"bash fi dedents", "bash", "  ", "if true; then\n  echo hi\nfi\n", 2, ""},
		{"bash comment line ignored", "bash", "  ", "# if true; then\n\n", 1, ""},
		{"json after open brace", "json", "  ", "{\n\n}\n", 1, "  "},
		{"json closing square dedents", "json", "  ", "{\n  \"a\": [\n    1\n  ]\n}\n", 3, "  "},
		{"python after colon", "python", "    ", "def foo():\n\n", 1, "    "},
		{"python else dedents", "python", "    ", "if x:\n    pass\nelse:\n", 2, ""},
		{"markdown copies previous indent", "markdown", "  ", "  - item\n\n", 1, "  "},
		{"fundamental copies previous indent", "fundamental", "  ", "    text\n\n", 1, "    "},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newTestEditor(tc.src)
			b := buf(e)
			b.SetMode(tc.mode)
			starts := lineStartsOf(tc.src)
			got := calcIndentAt(b, tc.mode, starts[tc.line], tc.unit)
			if got != tc.want {
				t.Errorf("calcIndentAt(%s, line %d of %q) = %q, want %q",
					tc.mode, tc.line, tc.src, got, tc.want)
			}
			// The []string engine must agree with the literal too.
			lines := strings.Split(tc.src, "\n")
			if got := calcIndent(tc.mode, lines, tc.line, tc.unit); got != tc.want {
				t.Errorf("calcIndent(%s, line %d of %q) = %q, want %q",
					tc.mode, tc.line, tc.src, got, tc.want)
			}
		})
	}
}

// TestBlockDepthAtSwitchesMode checks that depth checkpoints recorded for one
// mode are not reused for another (the counters differ).
func TestBlockDepthAtSwitchesMode(t *testing.T) {
	// Bash keywords in a file also containing braces: the two counters
	// disagree (bash counts `then` and `{`, Go only `{`), so a stale
	// checkpoint would be visible.
	src := "if true; then\n{\nx\n"
	e := newTestEditor(src)
	b := buf(e)
	bol := len([]rune(src))

	if got := blockDepthAt(b, bol, "bash", bashNetIndentRunes); got != 2 {
		t.Errorf("bash depth: want 2, got %d", got)
	}
	if got := blockDepthAt(b, bol, "go", netBraceDeltaSlash); got != 1 {
		t.Errorf("go depth: want 1, got %d", got)
	}
	if got := blockDepthAt(b, bol, "bash", bashNetIndentRunes); got != 2 {
		t.Errorf("bash depth after mode switch: want 2, got %d", got)
	}
}

// TestIndentCacheBounded checks that the cache does not grow without bound as
// buffers come and go.
func TestIndentCacheBounded(t *testing.T) {
	indentCaches = map[*buffer.Buffer]*indentCache{}
	for i := range indentCacheMaxBuffers * 3 {
		e := newTestEditor(fmt.Sprintf("func f%d() {\n}\n", i))
		b := buf(e)
		b.SetMode("go")
		indentCurrentLine(b, "\t")
		if len(indentCaches) > indentCacheMaxBuffers {
			t.Fatalf("after %d buffers the cache holds %d entries (max %d)",
				i+1, len(indentCaches), indentCacheMaxBuffers)
		}
	}
}

// TestIndentCacheForSameGeneration checks that a second call within the same
// change generation reuses the cache instead of rebuilding it.
func TestIndentCacheForSameGeneration(t *testing.T) {
	e := newTestEditor(benchGoSource(4))
	b := buf(e)
	c1 := indentCacheFor(b)
	c1.depths = append(c1.depths, 42) // marker that must survive
	c2 := indentCacheFor(b)
	if c2 != c1 || len(c2.depths) != 2 {
		t.Errorf("cache was rebuilt within one generation: %v", c2.depths)
	}
	b.InsertString(0, "x")
	if c3 := indentCacheFor(b); len(c3.depths) != 1 {
		t.Errorf("cache survived an edit: %v", c3.depths)
	}
}

// ---------------------------------------------------------------------------
// buffer reading helpers
// ---------------------------------------------------------------------------

func TestReadLineRunes(t *testing.T) {
	e := newTestEditor("one\ntwo\nthree")
	b := buf(e)

	got, eol := readLineRunes(b, 0, nil)
	if string(got) != "one" || eol != 3 {
		t.Errorf("first line: got %q eol=%d, want \"one\" eol=3", string(got), eol)
	}
	// The scratch slice is reused for the next line.
	got, eol = readLineRunes(b, 4, got)
	if string(got) != "two" || eol != 7 {
		t.Errorf("second line: got %q eol=%d, want \"two\" eol=7", string(got), eol)
	}
	// Last line: eol is the buffer end.
	got, eol = readLineRunes(b, 8, got)
	if string(got) != "three" || eol != b.Len() {
		t.Errorf("last line: got %q eol=%d, want \"three\" eol=%d", string(got), eol, b.Len())
	}
}

func TestReadLineRunesEmptyLine(t *testing.T) {
	e := newTestEditor("a\n\nb")
	got, eol := readLineRunes(buf(e), 2, nil)
	if len(got) != 0 || eol != 2 {
		t.Errorf("empty line: got %q eol=%d, want \"\" eol=2", string(got), eol)
	}
}

func TestLineTextAt(t *testing.T) {
	e := newTestEditor("  first\nsecond line\n")
	b := buf(e)
	if got := lineTextAt(b, 0); got != "  first" {
		t.Errorf("line 0: got %q", got)
	}
	if got := lineTextAt(b, 8); got != "second line" {
		t.Errorf("line 1: got %q", got)
	}
	if got := lineTextAt(b, b.Len()); got != "" {
		t.Errorf("empty last line: got %q", got)
	}
}

func TestPrevLineText(t *testing.T) {
	e := newTestEditor("  alpha\n\n   \nbeta\n")
	b := buf(e)

	// Line 3 ("beta") is preceded by two blank lines, then "  alpha".
	text, found := prevLineText(b, 13)
	if !found || text != "  alpha" {
		t.Errorf("prev non-blank line: got %q found=%v, want \"  alpha\" true", text, found)
	}
}

func TestPrevLineTextAllBlankAbove(t *testing.T) {
	e := newTestEditor("   \n\nx\n")
	// Line 2 ("x") has only blank lines above it: found is false and the text
	// is that of the first line, matching the []string engines.
	text, found := prevLineText(buf(e), 5)
	if found || text != "   " {
		t.Errorf("all-blank above: got %q found=%v, want \"   \" false", text, found)
	}
}

// ---------------------------------------------------------------------------
// applyIndentAt
// ---------------------------------------------------------------------------

func TestApplyIndentAtReplacesWhitespace(t *testing.T) {
	e := newTestEditor("func f() {\n      x := 1\n}")
	b := buf(e)
	applyIndentAt(b, 11, "\t")
	if got := b.String(); got != "func f() {\n\tx := 1\n}" {
		t.Errorf("applyIndentAt: got %q", got)
	}
	if got := b.Point(); got != 12 {
		t.Errorf("point should sit after the indent: got %d, want 12", got)
	}
}

func TestApplyIndentAtNoOpKeepsChangeGen(t *testing.T) {
	e := newTestEditor("func f() {\n\tx := 1\n}")
	b := buf(e)
	gen := b.ChangeGen()
	applyIndentAt(b, 11, "\t")
	if b.ChangeGen() != gen {
		t.Errorf("no-op applyIndentAt changed the buffer (gen %d → %d)", gen, b.ChangeGen())
	}
}

func TestApplyIndentAtBlankLine(t *testing.T) {
	e := newTestEditor("x\n\n")
	b := buf(e)
	applyIndentAt(b, 2, "  ")
	if got := b.String(); got != "x\n  \n" {
		t.Errorf("blank line: got %q", got)
	}
	if got := b.Point(); got != 4 {
		t.Errorf("point: got %d, want 4", got)
	}
}

// ---------------------------------------------------------------------------
// rune-slice helpers
// ---------------------------------------------------------------------------

func TestHasPrefixAt(t *testing.T) {
	runes := []rune("x := 1 // note")
	if !hasPrefixAt(runes, 7, "//") {
		t.Error("comment prefix at index 7 should match")
	}
	if hasPrefixAt(runes, 6, "//") {
		t.Error("index 6 is a space, must not match")
	}
	if hasPrefixAt(runes, len(runes)-1, "//") {
		t.Error("a prefix running past the end must not match")
	}
	if hasPrefixAt(runes, 0, "") {
		t.Error("an empty prefix must never match")
	}
}

func TestRunesEqual(t *testing.T) {
	if !runesEqual([]rune("done"), "done") {
		t.Error("done should equal done")
	}
	if runesEqual([]rune("done"), "don") {
		t.Error("done must not equal don")
	}
	if runesEqual([]rune("don"), "done") {
		t.Error("don must not equal done")
	}
}

func TestEndsWithWord(t *testing.T) {
	if !endsWithWord([]rune("if [ -f x ]; then"), "then") {
		t.Error("space-separated then should match")
	}
	if !endsWithWord([]rune("while true;\tdo"), "do") {
		t.Error("tab-separated do should match")
	}
	if endsWithWord([]rune("shorten"), "ten") {
		t.Error("ten inside a word must not match")
	}
	if endsWithWord([]rune("then"), "then") {
		t.Error("a bare keyword is handled by runesEqual, not endsWithWord")
	}
}

func TestStartsWithWord(t *testing.T) {
	if !startsWithWord([]rune("done < input"), "done") {
		t.Error("done followed by a space should match")
	}
	if startsWithWord([]rune("donefoo"), "done") {
		t.Error("done glued to a word must not match")
	}
	if startsWithWord([]rune("done"), "done") {
		t.Error("a bare keyword is handled by runesEqual, not startsWithWord")
	}
}

func TestTrimSpaceRunes(t *testing.T) {
	if got := string(trimSpaceRunes([]rune("  \tfi \t"))); got != "fi" {
		t.Errorf("trim: got %q, want \"fi\"", got)
	}
	if got := string(trimSpaceRunes([]rune("   "))); got != "" {
		t.Errorf("all blank: got %q, want \"\"", got)
	}
	if got := string(trimSpaceRunes(nil)); got != "" {
		t.Errorf("nil: got %q, want \"\"", got)
	}
}

// ---------------------------------------------------------------------------
// the line counters, pinned to literal expectations
// ---------------------------------------------------------------------------

// TestLineCountersLiteral pins every counter used by the indent engines against
// hard-coded results for a set of awkward lines: braces inside strings, chars,
// raw strings and comments, an apostrophe that opens a "char literal", non-ASCII
// text, and closers with no opener.
//
// It replaces TestRuneCountersMatchStringCounters, which compared
// netBraceCountRunes(runes, p) with netBraceCount(l, p).  That could not fail
// for any implementation whatsoever, because netBraceCount is defined as
// `return netBraceCountRunes([]rune(line), prefix)` (likewise bashNetIndent and
// netBraceCountJSON) -- including an implementation returning a constant.  The
// rune/string agreement it meant to check is still covered, since both entry
// points are asserted against the same literal below.
//
// The TestNetBraceCount_* family covers netBraceCount alone, and only for the
// "//" and "#" prefixes; the awkward lines here are additionally run through
// the empty prefix, bashNetIndent and netBraceCountJSON.
func TestLineCountersLiteral(t *testing.T) {
	tests := []struct {
		line  string
		slash int // netBraceCount(line, "//")
		hash  int // netBraceCount(line, "#")
		none  int // netBraceCount(line, "")
		bash  int // bashNetIndent(line)
		json  int // netBraceCountJSON(line)
	}{
		{"", 0, 0, 0, 0, 0},
		{"{", 1, 1, 1, 1, 1},
		{"}", -1, -1, -1, -1, -1},
		{"func f() {", 1, 1, 1, 1, 1},
		{"s := \"{\"", 0, 0, 0, 0, 0},
		// The JSON counter has no notion of single-quoted text (JSON has no
		// char literals), so the '}' counts there but nowhere else.
		{"c := '}'", 0, 0, 0, 0, -1},
		{"raw := `{}`", 0, 0, 0, 0, 0},
		{"x := 1 // {", 0, 1, 1, 1, 1},
		// bashNetIndent only honours a whole-line '#' comment, so the brace
		// after a trailing '#' still counts (a known limitation, pinned here).
		{"x = 1  # {", 1, 0, 1, 1, 1},
		{"if [ -f x ]; then", 0, 0, 0, 1, 0},
		{"  done", 0, 0, 0, -1, 0},
		{"esac", 0, 0, 0, -1, 0},
		{"# then", 0, 0, 0, 0, 0},
		{"\tfi ", 0, 0, 0, -1, 0},
		{"} else {", 0, 0, 0, 0, 0},
		{`"key": [`, 0, 0, 0, 0, 1},
		{`"a": "} ]"`, 0, 0, 0, 0, 0},
		// bashNetIndent only dedents on a closer that *starts* the line.
		{"]}", -1, -1, -1, 0, -2},
		{"nøn-ascii { øø }", 0, 0, 0, 0, 0},
		{"  \t  ", 0, 0, 0, 0, 0},
		// The apostrophe of "don't" opens a char literal for the Go/Perl
		// scanner, hiding the brace; the JSON scanner still sees it.
		{"don't { count '", 0, 0, 0, 0, 1},
		{"then", 0, 0, 0, 1, 0},
		{`c := '\\' {`, 1, 1, 1, 1, 1},
		{`s := "\\" }`, -1, -1, -1, 0, -1},
	}
	for _, tc := range tests {
		t.Run(strconv.Quote(tc.line), func(t *testing.T) {
			runes := []rune(tc.line)
			checks := []struct {
				what string
				got  int
				want int
			}{
				{`netBraceCount(line, "//")`, netBraceCount(tc.line, "//"), tc.slash},
				{`netBraceCountRunes(runes, "//")`, netBraceCountRunes(runes, "//"), tc.slash},
				{`netBraceCount(line, "#")`, netBraceCount(tc.line, "#"), tc.hash},
				{`netBraceCountRunes(runes, "#")`, netBraceCountRunes(runes, "#"), tc.hash},
				{`netBraceCount(line, "")`, netBraceCount(tc.line, ""), tc.none},
				{`netBraceCountRunes(runes, "")`, netBraceCountRunes(runes, ""), tc.none},
				{"bashNetIndent(line)", bashNetIndent(tc.line), tc.bash},
				{"bashNetIndentRunes(runes)", bashNetIndentRunes(runes), tc.bash},
				{"netBraceCountJSON(line)", netBraceCountJSON(tc.line), tc.json},
				{"netBraceCountJSONRunes(runes)", netBraceCountJSONRunes(runes), tc.json},
			}
			for _, c := range checks {
				if c.got != c.want {
					t.Errorf("%s on %q = %d, want %d", c.what, tc.line, c.got, c.want)
				}
			}
		})
	}
}

// TestBlockDepthAtAfterUndoAndEdit guards the cache key: an undo followed by a
// different edit brings ChangeGen() back to a value the cache has already seen,
// so ModCount() has to be part of the key.
func TestBlockDepthAtAfterUndoAndEdit(t *testing.T) {
	e := newTestEditor("x\ny\n")
	b := buf(e)

	// An opening brace at the top puts the last line one level deep.
	b.InsertString(0, "{\n")
	if got := blockDepthAt(b, b.Len(), "go", netBraceDeltaSlash); got != 1 {
		t.Fatalf("with an opening brace above: want depth 1, got %d", got)
	}

	// Undo it, then make a different edit that lands on the same change
	// generation.  The brace is gone, so the depth must be 0 again.
	b.ApplyUndo()
	b.InsertString(0, "y\n")
	if got := blockDepthAt(b, b.Len(), "go", netBraceDeltaSlash); got != 0 {
		t.Errorf("stale checkpoint reused after undo + edit: want depth 0, got %d", got)
	}
}

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

// ============================================================================
// leadingWSStr
// ============================================================================

func TestLeadingWSStr_NoIndent(t *testing.T) {
	if got := leadingWSStr("hello"); got != "" {
		t.Errorf("no indent: want \"\", got %q", got)
	}
}

func TestLeadingWSStr_Spaces(t *testing.T) {
	if got := leadingWSStr("    hello"); got != "    " {
		t.Errorf("4 spaces: want \"    \", got %q", got)
	}
}

func TestLeadingWSStr_Tabs(t *testing.T) {
	if got := leadingWSStr("\t\thello"); got != "\t\t" {
		t.Errorf("2 tabs: want \"\\t\\t\", got %q", got)
	}
}

func TestLeadingWSStr_MixedSpacesAndTabs(t *testing.T) {
	if got := leadingWSStr("\t  x"); got != "\t  " {
		t.Errorf("tab+2spaces: want \"\\t  \", got %q", got)
	}
}

func TestLeadingWSStr_EmptyString(t *testing.T) {
	if got := leadingWSStr(""); got != "" {
		t.Errorf("empty string: want \"\", got %q", got)
	}
}

func TestLeadingWSStr_AllWhitespace(t *testing.T) {
	if got := leadingWSStr("   "); got != "   " {
		t.Errorf("all spaces: want \"   \", got %q", got)
	}
}

// ============================================================================
// netBraceCount
// ============================================================================

func TestNetBraceCount_Empty(t *testing.T) {
	if got := netBraceCount("", "//"); got != 0 {
		t.Errorf("empty: want 0, got %d", got)
	}
}

func TestNetBraceCount_OpenBrace(t *testing.T) {
	if got := netBraceCount("{", "//"); got != 1 {
		t.Errorf("{: want 1, got %d", got)
	}
}

func TestNetBraceCount_CloseBrace(t *testing.T) {
	if got := netBraceCount("}", "//"); got != -1 {
		t.Errorf("}: want -1, got %d", got)
	}
}

func TestNetBraceCount_OpenParen(t *testing.T) {
	if got := netBraceCount("func foo(", "//"); got != 1 {
		t.Errorf("open paren: want 1, got %d", got)
	}
}

func TestNetBraceCount_BalancedBraces(t *testing.T) {
	if got := netBraceCount("{}", "//"); got != 0 {
		t.Errorf("{}: want 0, got %d", got)
	}
}

func TestNetBraceCount_MultipleBraces(t *testing.T) {
	// "func foo() {" opens one net brace (the { at end; parens cancel).
	if got := netBraceCount("func foo() {", "//"); got != 1 {
		t.Errorf("func foo() {: want 1, got %d", got)
	}
}

func TestNetBraceCount_IgnoresBraceInString(t *testing.T) {
	if got := netBraceCount(`s := "{"`, "//"); got != 0 {
		t.Errorf("brace in string: want 0, got %d", got)
	}
}

func TestNetBraceCount_IgnoresBraceInBacktick(t *testing.T) {
	if got := netBraceCount("s := `{}`", "//"); got != 0 {
		t.Errorf("brace in backtick: want 0, got %d", got)
	}
}

func TestNetBraceCount_IgnoresBraceInChar(t *testing.T) {
	if got := netBraceCount("c := '{'", "//"); got != 0 {
		t.Errorf("brace in char literal: want 0, got %d", got)
	}
}

func TestNetBraceCount_StopsAtLineComment(t *testing.T) {
	// The { after // is in a comment and must be ignored.
	if got := netBraceCount("x := 1 // {", "//"); got != 0 {
		t.Errorf("brace after //: want 0, got %d", got)
	}
}

func TestNetBraceCount_HashCommentPrefix(t *testing.T) {
	// Python/Bash: the { after # must be ignored.
	if got := netBraceCount("x = 1  # {", "#"); got != 0 {
		t.Errorf("brace after #: want 0, got %d", got)
	}
}

func TestNetBraceCount_EscapedQuoteInString(t *testing.T) {
	// The backslash-escaped quote must not end the string; the { is inside it.
	if got := netBraceCount(`s := "he said \"{\"`, "//"); got != 0 {
		t.Errorf("escaped quote then brace: want 0, got %d", got)
	}
}

func TestNetBraceCount_NestedBraces(t *testing.T) {
	// "{{" opens two levels.
	if got := netBraceCount("{{", "//"); got != 2 {
		t.Errorf("{{: want 2, got %d", got)
	}
}

// ============================================================================
// calcIndentBraced
// ============================================================================

func TestCalcIndentBraced_TopLevel(t *testing.T) {
	lines := []string{"package main"}
	if got := calcIndentBraced(lines, 0, "\t", "//"); got != "" {
		t.Errorf("top level line 0: want \"\", got %q", got)
	}
}

func TestCalcIndentBraced_AfterOpenBrace(t *testing.T) {
	lines := []string{"func foo() {", ""}
	if got := calcIndentBraced(lines, 1, "\t", "//"); got != "\t" {
		t.Errorf("after open brace: want \"\\t\", got %q", got)
	}
}

func TestCalcIndentBraced_ClosingBraceDedents(t *testing.T) {
	lines := []string{"func foo() {", "\tx := 1", "}"}
	if got := calcIndentBraced(lines, 2, "\t", "//"); got != "" {
		t.Errorf("closing brace: want \"\", got %q", got)
	}
}

func TestCalcIndentBraced_ClosingParenDedents(t *testing.T) {
	lines := []string{"foo(", "\t1,", ")"}
	if got := calcIndentBraced(lines, 2, "\t", "//"); got != "" {
		t.Errorf("closing paren: want \"\", got %q", got)
	}
}

func TestCalcIndentBraced_NestedTwoLevels(t *testing.T) {
	lines := []string{
		"func foo() {",
		"\tif true {",
		"",
	}
	if got := calcIndentBraced(lines, 2, "\t", "//"); got != "\t\t" {
		t.Errorf("two-level nest: want \"\\t\\t\", got %q", got)
	}
}

func TestCalcIndentBraced_NegativeDepthClamped(t *testing.T) {
	// A stray } at top level must not produce negative depth: the line below
	// it is indented at column 0, not at some negative-repeat nonsense.
	lines := []string{"}", ""}
	if got := calcIndentBraced(lines, 1, "\t", "//"); got != "" {
		t.Errorf("depth must clamp at 0: want \"\", got %q", got)
	}
}

func TestCalcIndentBraced_CommentLinesIgnored(t *testing.T) {
	// A { inside a // comment must not increase depth.
	lines := []string{"// func foo() {", ""}
	if got := calcIndentBraced(lines, 1, "\t", "//"); got != "" {
		t.Errorf("comment { must not indent: want \"\", got %q", got)
	}
}

func TestCalcIndentBraced_FourSpaceUnit(t *testing.T) {
	lines := []string{"public class Foo {", ""}
	if got := calcIndentBraced(lines, 1, "    ", "//"); got != "    " {
		t.Errorf("four-space unit: want \"    \", got %q", got)
	}
}

// ============================================================================
// calcIndentPython
// ============================================================================

func TestCalcIndentPython_FirstLine(t *testing.T) {
	// Line 0 has no previous line → always "".
	lines := []string{"def foo():"}
	if got := calcIndentPython(lines, 0, "    "); got != "" {
		t.Errorf("line 0: want \"\", got %q", got)
	}
}

func TestCalcIndentPython_AfterColon(t *testing.T) {
	lines := []string{"def foo():", ""}
	if got := calcIndentPython(lines, 1, "    "); got != "    " {
		t.Errorf("after colon: want \"    \", got %q", got)
	}
}

func TestCalcIndentPython_AfterColonTwoSpaces(t *testing.T) {
	lines := []string{"if x:", ""}
	if got := calcIndentPython(lines, 1, "  "); got != "  " {
		t.Errorf("after if colon 2-space: want \"  \", got %q", got)
	}
}

func TestCalcIndentPython_ContinuationLine(t *testing.T) {
	// A line that doesn't end with ':' → copy previous indent.
	lines := []string{"    x = 1", ""}
	if got := calcIndentPython(lines, 1, "    "); got != "    " {
		t.Errorf("continuation: want \"    \", got %q", got)
	}
}

func TestCalcIndentPython_ElseDedent(t *testing.T) {
	lines := []string{"if x:", "    pass", "else:"}
	// "else:" is a dedent keyword at line 2; prev line "    pass" has 4-space
	// indent which does not end with ':' → base = "    "; then strip one unit.
	if got := calcIndentPython(lines, 2, "    "); got != "" {
		t.Errorf("else dedent: want \"\", got %q", got)
	}
}

func TestCalcIndentPython_ElifDedent(t *testing.T) {
	lines := []string{"if x:", "    pass", "elif y:"}
	if got := calcIndentPython(lines, 2, "    "); got != "" {
		t.Errorf("elif dedent: want \"\", got %q", got)
	}
}

func TestCalcIndentPython_ExceptDedent(t *testing.T) {
	lines := []string{"try:", "    pass", "except"}
	if got := calcIndentPython(lines, 2, "    "); got != "" {
		t.Errorf("except dedent: want \"\", got %q", got)
	}
}

func TestCalcIndentPython_SkipsBlankLines(t *testing.T) {
	// Blank line between def and body must be skipped.
	lines := []string{"def foo():", "", ""}
	// Line 2 should still see line 0 ("def foo():") as previous non-blank.
	if got := calcIndentPython(lines, 2, "    "); got != "    " {
		t.Errorf("skip blank line: want \"    \", got %q", got)
	}
}

func TestCalcIndentPython_CommentNotColon(t *testing.T) {
	// A comment line starting with '#' that ends with ':' must NOT trigger indent.
	lines := []string{"# not a colon:", ""}
	if got := calcIndentPython(lines, 1, "    "); got != "" {
		t.Errorf("comment colon must not indent: want \"\", got %q", got)
	}
}

// ============================================================================
// bashNetIndent
// ============================================================================

func TestBashNetIndent_Then(t *testing.T) {
	if got := bashNetIndent("then"); got != 1 {
		t.Errorf("then: want 1, got %d", got)
	}
}

func TestBashNetIndent_Do(t *testing.T) {
	if got := bashNetIndent("do"); got != 1 {
		t.Errorf("do: want 1, got %d", got)
	}
}

func TestBashNetIndent_OpenBrace(t *testing.T) {
	if got := bashNetIndent("{"); got != 1 {
		t.Errorf("{: want 1, got %d", got)
	}
}

func TestBashNetIndent_Fi(t *testing.T) {
	if got := bashNetIndent("fi"); got != -1 {
		t.Errorf("fi: want -1, got %d", got)
	}
}

func TestBashNetIndent_Done(t *testing.T) {
	if got := bashNetIndent("done"); got != -1 {
		t.Errorf("done: want -1, got %d", got)
	}
}

func TestBashNetIndent_Esac(t *testing.T) {
	if got := bashNetIndent("esac"); got != -1 {
		t.Errorf("esac: want -1, got %d", got)
	}
}

func TestBashNetIndent_CloseBrace(t *testing.T) {
	if got := bashNetIndent("}"); got != -1 {
		t.Errorf("}: want -1, got %d", got)
	}
}

func TestBashNetIndent_PlainLine(t *testing.T) {
	if got := bashNetIndent("echo hello"); got != 0 {
		t.Errorf("plain line: want 0, got %d", got)
	}
}

func TestBashNetIndent_Comment(t *testing.T) {
	// A comment line starting with # must contribute 0 (even if it contains "then").
	if got := bashNetIndent("# then"); got != 0 {
		t.Errorf("comment with then: want 0, got %d", got)
	}
}

func TestBashNetIndent_IfThenOnOneLine(t *testing.T) {
	// "if [ ... ]; then" — the "then" keyword at end opens one level.
	if got := bashNetIndent("if [ -f /etc/passwd ]; then"); got != 1 {
		t.Errorf("if ... then: want 1, got %d", got)
	}
}

func TestBashNetIndent_CloseBraceWithFollowingText(t *testing.T) {
	// "} else {" closes one, opens one → net 0? Actually bashNetIndent only
	// checks closers as prefix and openers as suffix.  "} else {" has {
	// appended — suffix " {" → +1 from opener, and "}" prefix → -1.  Net = 0.
	// This is an edge case; just verify it doesn't panic.
	_ = bashNetIndent("} else {")
}

// ============================================================================
// calcIndentBash
// ============================================================================

func TestCalcIndentBash_AfterThen(t *testing.T) {
	lines := []string{"if [ -f x ]; then", ""}
	if got := calcIndentBash(lines, 1, "  "); got != "  " {
		t.Errorf("after then: want \"  \", got %q", got)
	}
}

func TestCalcIndentBash_FiDedents(t *testing.T) {
	lines := []string{"if [ -f x ]; then", "  echo hi", "fi"}
	if got := calcIndentBash(lines, 2, "  "); got != "" {
		t.Errorf("fi: want \"\", got %q", got)
	}
}

func TestCalcIndentBash_AfterDo(t *testing.T) {
	lines := []string{"for i in 1 2 3; do", ""}
	if got := calcIndentBash(lines, 1, "  "); got != "  " {
		t.Errorf("after do: want \"  \", got %q", got)
	}
}

func TestCalcIndentBash_DoneDedents(t *testing.T) {
	lines := []string{"for i in 1 2 3; do", "  echo $i", "done"}
	if got := calcIndentBash(lines, 2, "  "); got != "" {
		t.Errorf("done: want \"\", got %q", got)
	}
}

func TestCalcIndentBash_ElseDedents(t *testing.T) {
	lines := []string{"if [ -f x ]; then", "  echo hi", "else"}
	// "else" is on line 2; depth from lines 0+1 = then(+1)+plain(0) = 1; then
	// else on current line subtracts 1 → 0.
	if got := calcIndentBash(lines, 2, "  "); got != "" {
		t.Errorf("else: want \"\", got %q", got)
	}
}

func TestCalcIndentBash_NegativeDepthClamped(t *testing.T) {
	// A stray fi at top level must not produce negative depth: the next line
	// sits at column 0.
	lines := []string{"fi", ""}
	if got := calcIndentBash(lines, 1, "  "); got != "" {
		t.Errorf("depth must clamp at 0: want \"\", got %q", got)
	}
}

func TestCalcIndentBash_CaseEsac(t *testing.T) {
	// esac is a closer, so an esac at depth 0 keeps the next line at column 0
	// rather than dropping below it.
	lines := []string{"esac", ""}
	if got := calcIndentBash(lines, 1, "  "); got != "" {
		t.Errorf("esac at depth 0: want \"\", got %q", got)
	}
}

// ============================================================================
// calcIndentCopy
// ============================================================================

func TestCalcIndentCopy_PreviousLineIndent(t *testing.T) {
	lines := []string{"    hello", ""}
	if got := calcIndentCopy(lines, 1); got != "    " {
		t.Errorf("copy previous: want \"    \", got %q", got)
	}
}

func TestCalcIndentCopy_SkipsBlanks(t *testing.T) {
	lines := []string{"  text", "", ""}
	// Line 2 skips blank line 1 and copies line 0's indent.
	if got := calcIndentCopy(lines, 2); got != "  " {
		t.Errorf("skip blank: want \"  \", got %q", got)
	}
}

func TestCalcIndentCopy_FirstLine(t *testing.T) {
	lines := []string{"text"}
	if got := calcIndentCopy(lines, 0); got != "" {
		t.Errorf("line 0: want \"\", got %q", got)
	}
}

func TestCalcIndentCopy_AllPreviousBlanks(t *testing.T) {
	lines := []string{"", "", "text"}
	// No non-blank line before line 2 except the first (empty), returns "".
	if got := calcIndentCopy(lines, 2); got != "" {
		t.Errorf("all blanks before: want \"\", got %q", got)
	}
}

func TestCalcIndentCopy_TabIndent(t *testing.T) {
	lines := []string{"\t\thello", ""}
	if got := calcIndentCopy(lines, 1); got != "\t\t" {
		t.Errorf("tab copy: want \"\\t\\t\", got %q", got)
	}
}

// ============================================================================
// calcIndent — dispatch to the right sub-function
// ============================================================================

func TestCalcIndent_GoMode(t *testing.T) {
	lines := []string{"func foo() {", ""}
	if got := calcIndent("go", lines, 1, "\t"); got != "\t" {
		t.Errorf("go mode: want \"\\t\", got %q", got)
	}
}

func TestCalcIndent_JavaMode(t *testing.T) {
	lines := []string{"public class Foo {", ""}
	if got := calcIndent("java", lines, 1, "    "); got != "    " {
		t.Errorf("java mode: want \"    \", got %q", got)
	}
}

func TestCalcIndent_PythonMode(t *testing.T) {
	lines := []string{"def foo():", ""}
	if got := calcIndent("python", lines, 1, "    "); got != "    " {
		t.Errorf("python mode: want \"    \", got %q", got)
	}
}

func TestCalcIndent_BashMode(t *testing.T) {
	lines := []string{"if true; then", ""}
	if got := calcIndent("bash", lines, 1, "  "); got != "  " {
		t.Errorf("bash mode: want \"  \", got %q", got)
	}
}

func TestCalcIndent_JSONMode(t *testing.T) {
	lines := []string{"{", ""}
	if got := calcIndent("json", lines, 1, "  "); got != "  " {
		t.Errorf("json mode: want \"  \", got %q", got)
	}
}

func TestCalcIndent_MarkdownMode(t *testing.T) {
	// Markdown copies previous line's indent.
	lines := []string{"  - item", ""}
	if got := calcIndent("markdown", lines, 1, "  "); got != "  " {
		t.Errorf("markdown mode: want \"  \", got %q", got)
	}
}

func TestCalcIndent_FundamentalMode(t *testing.T) {
	lines := []string{"    text", ""}
	if got := calcIndent("fundamental", lines, 1, "  "); got != "    " {
		t.Errorf("fundamental mode: want \"    \", got %q", got)
	}
}

func TestCalcIndent_PerlMode(t *testing.T) {
	lines := []string{"sub foo {", ""}
	if got := calcIndent("perl", lines, 1, "    "); got != "    " {
		t.Errorf("perl mode: want \"    \", got %q", got)
	}
}

// ============================================================================
// applyIndent
// ============================================================================

func TestApplyIndent_AddIndent(t *testing.T) {
	src := "func foo() {\nhello\n}"
	b := buffer.NewWithContent("*test*", src)
	lines := strings.Split(src, "\n")
	applyIndent(b, lines, 1, "\t")
	// After applying, line 1 should start with \t.
	got := b.String()
	if !strings.Contains(got, "\thello") {
		t.Errorf("applyIndent should add tab: got %q", got)
	}
}

func TestApplyIndent_RemoveIndent(t *testing.T) {
	src := "    misindented"
	b := buffer.NewWithContent("*test*", src)
	lines := strings.Split(src, "\n")
	applyIndent(b, lines, 0, "")
	got := b.String()
	if strings.HasPrefix(got, " ") {
		t.Errorf("applyIndent should remove leading spaces: got %q", got)
	}
}

func TestApplyIndent_NoChangeWhenAlreadyCorrect(t *testing.T) {
	src := "\tcorrect"
	b := buffer.NewWithContent("*test*", src)
	lines := strings.Split(src, "\n")
	originalGen := b.ChangeGen()
	applyIndent(b, lines, 0, "\t")
	// If the indent is already correct, the buffer should not have been
	// modified (changeGen stays the same).
	if b.ChangeGen() != originalGen {
		t.Errorf("no-op applyIndent changed buffer (changeGen %d → %d)",
			originalGen, b.ChangeGen())
	}
}

func TestApplyIndent_PointMovesToFirstNonWS(t *testing.T) {
	src := "func foo() {\nhello\n}"
	b := buffer.NewWithContent("*test*", src)
	lines := strings.Split(src, "\n")
	applyIndent(b, lines, 1, "\t")
	// Point should be at the '\t' + 'h' position (start of content).
	pt := b.Point()
	// Line 1 starts at offset 13 ("func foo() {\n" = 13 runes).
	// After inserting \t, content starts at 13+1=14.
	if pt < 13 {
		t.Errorf("point should be on line 1 content, got %d", pt)
	}
}

// ============================================================================
// indentCurrentLine
// ============================================================================

func TestIndentCurrentLine_GoIndent(t *testing.T) {
	src := "func foo() {\nhello\n}"
	b := buffer.NewWithContent("*test*", src)
	b.SetMode("go")
	// Place point on "hello" line.
	b.SetPoint(14)
	indentCurrentLine(b, "\t")
	got := b.String()
	if !strings.Contains(got, "\thello") {
		t.Errorf("go indent: expected \\thello in %q", got)
	}
}

func TestIndentCurrentLine_ClosingBrace(t *testing.T) {
	src := "func foo() {\n\tx := 1\n\t}"
	b := buffer.NewWithContent("*test*", src)
	b.SetMode("go")
	// Place point on the closing "}" line.
	b.SetPoint(b.Len() - 1)
	indentCurrentLine(b, "\t")
	got := b.String()
	// The closing } should be at column 0 (no tab before it).
	lines := strings.Split(got, "\n")
	last := lines[len(lines)-1]
	if strings.HasPrefix(last, "\t\t") {
		t.Errorf("closing brace should dedent: got line %q", last)
	}
}

func TestIndentCurrentLine_PythonAfterColon(t *testing.T) {
	src := "def foo():\nhello"
	b := buffer.NewWithContent("*test*", src)
	b.SetMode("python")
	b.SetPoint(11) // on "hello"
	indentCurrentLine(b, "    ")
	got := b.String()
	if !strings.Contains(got, "    hello") {
		t.Errorf("python: expected 4-space indent before hello in %q", got)
	}
}

func TestIndentCurrentLine_Idempotent(t *testing.T) {
	src := "func foo() {\n\talready\n}"
	b := buffer.NewWithContent("*test*", src)
	b.SetMode("go")
	b.SetPoint(14)
	indentCurrentLine(b, "\t")
	gen1 := b.ChangeGen()
	// Run again — should be a no-op.
	b.SetPoint(14)
	indentCurrentLine(b, "\t")
	gen2 := b.ChangeGen()
	if gen1 != gen2 {
		t.Errorf("indentCurrentLine should be idempotent: changeGen %d → %d", gen1, gen2)
	}
}

// ============================================================================
// conf-mode — no indentation logic (doc/gomacs-spec.md "conf-mode")
// ============================================================================

// TestIndentCurrentLine_ConfPreservesIndent checks that Tab in a conf buffer
// neither invents indentation from the previous line nor strips indentation the
// user typed, and that it leaves point where it was.
func TestIndentCurrentLine_ConfPreservesIndent(t *testing.T) {
	tests := []struct {
		name string
		src  string
		// pt is the point placed before the Tab press.
		pt int
	}{
		// The previous line is indented: Tab must not copy that indentation.
		{name: "no indent added", src: "    indented = 1\nkey = 2\n", pt: 17},
		// The previous line is flush left: Tab must not strip the indentation.
		{name: "no indent removed", src: "key = 2\n    indented = 1\n", pt: 12},
		// Nothing above at all.
		{name: "first line", src: "    key = 1\n", pt: 4},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := buffer.NewWithContent("*test*", tc.src)
			b.SetMode(modeConf)
			b.SetPoint(tc.pt)
			gen := b.ChangeGen()
			indentCurrentLine(b, "  ")
			if got := b.String(); got != tc.src {
				t.Errorf("conf-mode Tab changed the buffer: %q → %q", tc.src, got)
			}
			if b.ChangeGen() != gen {
				t.Errorf("conf-mode Tab bumped ChangeGen %d → %d", gen, b.ChangeGen())
			}
			if b.Point() != tc.pt {
				t.Errorf("conf-mode Tab moved point %d → %d", tc.pt, b.Point())
			}
		})
	}
}

// TestCalcIndent_ConfMode checks both calc entry points return the line's own
// indentation for conf-mode, so applying the result is always a no-op.
func TestCalcIndent_ConfMode(t *testing.T) {
	lines := []string{"    indented = 1", "key = 2"}
	if got := calcIndent(modeConf, lines, 1, "  "); got != "" {
		t.Errorf("calcIndent conf: want %q, got %q", "", got)
	}
	if got := calcIndent(modeConf, lines, 0, "  "); got != "    " {
		t.Errorf("calcIndent conf: want %q, got %q", "    ", got)
	}

	src := strings.Join(lines, "\n") + "\n"
	b := buffer.NewWithContent("*test*", src)
	if got := calcIndentAt(b, modeConf, 0, "  "); got != "    " {
		t.Errorf("calcIndentAt conf line 0: want %q, got %q", "    ", got)
	}
	if got := calcIndentAt(b, modeConf, 17, "  "); got != "" {
		t.Errorf("calcIndentAt conf line 1: want %q, got %q", "", got)
	}
}

func TestIndentNoOpMode(t *testing.T) {
	if !indentNoOpMode(modeConf) {
		t.Error("conf-mode should have no indentation logic")
	}
	for _, mode := range []string{"go", "java", "python", "bash", "perl", "json", "markdown", "yaml", "fundamental"} {
		if indentNoOpMode(mode) {
			t.Errorf("%s-mode should keep its indentation behaviour", mode)
		}
	}
}
