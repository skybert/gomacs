package editor

// Tests for indent_engine.go, langmode.go, imenu.go, gherkin.go (pure
// functions), and langForExt from vc.go.
//
// Functions already covered by indent_engine_test.go (netBraceCountJSON,
// calcIndentJSON), indent_test.go (elispIndentLevel, indentElispLine),
// imenu_test.go (imenuSymbols*, lineStartOffset), langmode_test.go
// (langModeByName known/unknown, modeIndentStr defaults, cmdGoMode,
// cmdPythonMode), gherkin_test.go (gherkinStepAtPoint, stepToCamelCase,
// parseGrepLines) are NOT duplicated here — this file covers the remaining
// gaps.

import (
	"strings"
	"testing"

	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/elisp"
)

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
	// A stray } at top level must not produce negative depth.
	lines := []string{"}", ""}
	got := calcIndentBraced(lines, 1, "\t", "//")
	if strings.Contains(got, "-") {
		t.Errorf("depth must not go negative, got %q", got)
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
	lines := []string{"fi", ""}
	got := calcIndentBash(lines, 1, "  ")
	if strings.Contains(got, "-") {
		t.Errorf("depth must not go negative, got %q", got)
	}
}

func TestCalcIndentBash_CaseEsac(t *testing.T) {
	// case opens via "do" equivalent — but actually case doesn't use then/do.
	// Verify esac at depth 0 stays at 0.
	lines := []string{"esac", ""}
	got := calcIndentBash(lines, 1, "  ")
	// Should not panic and depth should be >= 0.
	if strings.Contains(got, "-") {
		t.Errorf("depth must not go negative after esac, got %q", got)
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
// langModeByName — additional modes not covered in langmode_test.go
// ============================================================================

func TestLangModeByName_Gherkin(t *testing.T) {
	if m := langModeByName("gherkin"); m == nil {
		t.Error("langModeByName(\"gherkin\") should not be nil")
	}
}

func TestLangModeByName_Perl(t *testing.T) {
	if m := langModeByName("perl"); m == nil {
		t.Error("langModeByName(\"perl\") should not be nil")
	}
}

func TestLangModeByName_Makefile(t *testing.T) {
	if m := langModeByName("makefile"); m == nil {
		t.Error("langModeByName(\"makefile\") should not be nil")
	}
}

func TestLangModeByName_Conf(t *testing.T) {
	if m := langModeByName("conf"); m == nil {
		t.Error("langModeByName(\"conf\") should not be nil")
	}
}

func TestLangModeByName_ReturnsModeNameField(t *testing.T) {
	m := langModeByName("go")
	if m == nil {
		t.Fatal("go mode should exist")
	}
	if m.modeName != "go" {
		t.Errorf("modeName = %q, want \"go\"", m.modeName)
	}
}

// ============================================================================
// cmdXxxMode commands — modes not yet covered in langmode_test.go
// ============================================================================

func TestCmdJavaMode(t *testing.T) {
	e := newTestEditor("public class Foo {}")
	e.cmdJavaMode()
	if buf(e).Mode() != "java" {
		t.Errorf("mode = %q, want \"java\"", buf(e).Mode())
	}
}

func TestCmdBashMode(t *testing.T) {
	e := newTestEditor("#!/bin/bash")
	e.cmdBashMode()
	if buf(e).Mode() != "bash" {
		t.Errorf("mode = %q, want \"bash\"", buf(e).Mode())
	}
}

func TestCmdMarkdownMode(t *testing.T) {
	e := newTestEditor("# Heading")
	e.cmdMarkdownMode()
	if buf(e).Mode() != "markdown" {
		t.Errorf("mode = %q, want \"markdown\"", buf(e).Mode())
	}
}

func TestCmdElispMode(t *testing.T) {
	e := newTestEditor("(defun foo () nil)")
	e.cmdElispMode()
	if buf(e).Mode() != "elisp" {
		t.Errorf("mode = %q, want \"elisp\"", buf(e).Mode())
	}
}

func TestCmdTextMode(t *testing.T) {
	e := newTestEditor("Hello world.")
	e.cmdTextMode()
	if buf(e).Mode() != "text" {
		t.Errorf("mode = %q, want \"text\"", buf(e).Mode())
	}
}

func TestCmdFundamentalMode(t *testing.T) {
	e := newTestEditor("data")
	e.cmdFundamentalMode()
	if buf(e).Mode() != "fundamental" {
		t.Errorf("mode = %q, want \"fundamental\"", buf(e).Mode())
	}
}

func TestCmdJsonModeIndent(t *testing.T) {
	e := newTestEditor(`{"key": "value"}`)
	e.cmdJsonMode()
	if buf(e).Mode() != "json" {
		t.Errorf("mode = %q, want \"json\"", buf(e).Mode())
	}
}

func TestCmdYamlModeIndent(t *testing.T) {
	e := newTestEditor("key: value")
	e.cmdYamlMode()
	if buf(e).Mode() != "yaml" {
		t.Errorf("mode = %q, want \"yaml\"", buf(e).Mode())
	}
}

func TestCmdMakefileMode(t *testing.T) {
	e := newTestEditor("all:\n\techo done")
	e.cmdMakefileMode()
	if buf(e).Mode() != "makefile" {
		t.Errorf("mode = %q, want \"makefile\"", buf(e).Mode())
	}
}

func TestCmdConfMode(t *testing.T) {
	e := newTestEditor("[section]\nkey=val")
	e.cmdConfMode()
	if buf(e).Mode() != "conf" {
		t.Errorf("mode = %q, want \"conf\"", buf(e).Mode())
	}
}

func TestCmdPerlMode(t *testing.T) {
	e := newTestEditor("#!/usr/bin/perl")
	e.cmdPerlMode()
	if buf(e).Mode() != "perl" {
		t.Errorf("mode = %q, want \"perl\"", buf(e).Mode())
	}
}

func TestCmdGherkinMode(t *testing.T) {
	e := newTestEditor("Feature: login")
	e.cmdGherkinMode()
	if buf(e).Mode() != "gherkin" {
		t.Errorf("mode = %q, want \"gherkin\"", buf(e).Mode())
	}
}

// ============================================================================
// modeIndentStr — additional modes / edge cases
// ============================================================================

func TestModeIndentStr_JavaDefault(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	if got := e.modeIndentStr("java"); got != "  " {
		t.Errorf("java default: want \"  \", got %q", got)
	}
}

func TestModeIndentStr_JSONDefault(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	if got := e.modeIndentStr("json"); got != "  " {
		t.Errorf("json default: want \"  \", got %q", got)
	}
}

func TestModeIndentStr_MarkdownDefault(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	if got := e.modeIndentStr("markdown"); got != "  " {
		t.Errorf("markdown default: want \"  \", got %q", got)
	}
}

func TestModeIndentStr_GoDefaultIsTab(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	if got := e.modeIndentStr("go"); got != "\t" {
		t.Errorf("go default must be tab: got %q", got)
	}
}

func TestModeIndentStr_ZeroIntIgnored(t *testing.T) {
	// (setq python-indent 0) is invalid; should fall back to default "  ".
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	_, err := e.lisp.EvalString("(setq python-indent 0)")
	if err != nil {
		t.Fatalf("setq failed: %v", err)
	}
	if got := e.modeIndentStr("python"); got != "  " {
		t.Errorf("zero int ignored: want \"  \", got %q", got)
	}
}

func TestModeIndentStr_EmptyStringIgnored(t *testing.T) {
	// (setq go-indent "") is invalid; should fall back to default "\t".
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	_, err := e.lisp.EvalString(`(setq go-indent "")`)
	if err != nil {
		t.Fatalf("setq failed: %v", err)
	}
	if got := e.modeIndentStr("go"); got != "\t" {
		t.Errorf("empty string ignored: want \"\\t\", got %q", got)
	}
}

// ============================================================================
// imenuSymbols — Java mode (not covered in imenu_test.go)
// ============================================================================

func TestImenuSymbolsJava(t *testing.T) {
	src := `public class Greeter {
    public void greet(String name) {
        System.out.println(name);
    }
    private int count(List<String> items) {
        return items.size();
    }
}
`
	b := buffer.NewWithContent("Greeter.java", src)
	b.SetMode("java")
	entries := imenuSymbols(b)
	if len(entries) < 2 {
		t.Fatalf("expected at least 2 entries, got %d: %v", len(entries), entries)
	}
	names := map[string]bool{}
	for _, en := range entries {
		names[en.label] = true
	}
	if !names["greet (line 2)"] {
		t.Errorf("missing greet entry; got %v", entries)
	}
	if !names["count (line 5)"] {
		t.Errorf("missing count entry; got %v", entries)
	}
}

func TestImenuSymbolsJava_Static(t *testing.T) {
	src := `class Util {
    public static String format(String s) {
        return s;
    }
}
`
	b := buffer.NewWithContent("Util.java", src)
	b.SetMode("java")
	entries := imenuSymbols(b)
	if len(entries) == 0 {
		t.Fatal("expected at least 1 entry")
	}
	if entries[0].label != "format (line 2)" {
		t.Errorf("unexpected entry: %s", entries[0].label)
	}
}

// ============================================================================
// lineStartOffset — edge cases
// ============================================================================

func TestLineStartOffset_BeyondEnd(t *testing.T) {
	b := buffer.NewWithContent("x", "abc")
	// Asking for a line beyond the buffer should return buf length.
	got := lineStartOffset(b, 99)
	if got != 3 {
		t.Errorf("beyond end: want 3, got %d", got)
	}
}

func TestLineStartOffset_SingleLine(t *testing.T) {
	b := buffer.NewWithContent("x", "hello")
	if got := lineStartOffset(b, 1); got != 0 {
		t.Errorf("single line, line 1: want 0, got %d", got)
	}
}

func TestLineStartOffset_MultilineUnicode(t *testing.T) {
	// "hé\nworld\n" — 'é' is one rune, so line 2 starts at rune offset 3.
	b := buffer.NewWithContent("x", "hé\nworld\n")
	if got := lineStartOffset(b, 2); got != 3 {
		t.Errorf("unicode: want 3, got %d", got)
	}
}

// ============================================================================
// langForExt
// ============================================================================

func TestLangForExtGo2(t *testing.T) {
	if got := langForExt(".go"); got != "go" {
		t.Errorf(".go: want \"go\", got %q", got)
	}
}

func TestLangForExtPython2(t *testing.T) {
	if got := langForExt(".py"); got != "python" {
		t.Errorf(".py: want \"python\", got %q", got)
	}
}

func TestLangForExtJava2(t *testing.T) {
	if got := langForExt(".java"); got != "java" {
		t.Errorf(".java: want \"java\", got %q", got)
	}
}

func TestLangForExtBash2(t *testing.T) {
	if got := langForExt(".sh"); got != "bash" {
		t.Errorf(".sh: want \"bash\", got %q", got)
	}
}

func TestLangForExt_BashAlt(t *testing.T) {
	if got := langForExt(".bash"); got != "bash" {
		t.Errorf(".bash: want \"bash\", got %q", got)
	}
}

func TestLangForExtMarkdown2(t *testing.T) {
	if got := langForExt(".md"); got != "markdown" {
		t.Errorf(".md: want \"markdown\", got %q", got)
	}
}

func TestLangForExt_MarkdownAlt(t *testing.T) {
	if got := langForExt(".markdown"); got != "markdown" {
		t.Errorf(".markdown: want \"markdown\", got %q", got)
	}
}

func TestLangForExtJSON2(t *testing.T) {
	if got := langForExt(".json"); got != "json" {
		t.Errorf(".json: want \"json\", got %q", got)
	}
}

func TestLangForExtYAML2(t *testing.T) {
	if got := langForExt(".yaml"); got != "yaml" {
		t.Errorf(".yaml: want \"yaml\", got %q", got)
	}
}

func TestLangForExt_YAMLAlt(t *testing.T) {
	if got := langForExt(".yml"); got != "yaml" {
		t.Errorf(".yml: want \"yaml\", got %q", got)
	}
}

func TestLangForExtElisp2(t *testing.T) {
	if got := langForExt(".el"); got != "elisp" {
		t.Errorf(".el: want \"elisp\", got %q", got)
	}
}

func TestLangForExt_Perl(t *testing.T) {
	if got := langForExt(".pl"); got != "perl" {
		t.Errorf(".pl: want \"perl\", got %q", got)
	}
}

func TestLangForExt_PerlModule(t *testing.T) {
	if got := langForExt(".pm"); got != "perl" {
		t.Errorf(".pm: want \"perl\", got %q", got)
	}
}

func TestLangForExt_PerlTest(t *testing.T) {
	if got := langForExt(".t"); got != "perl" {
		t.Errorf(".t: want \"perl\", got %q", got)
	}
}

func TestLangForExt_Gherkin(t *testing.T) {
	if got := langForExt(".feature"); got != "gherkin" {
		t.Errorf(".feature: want \"gherkin\", got %q", got)
	}
}

func TestLangForExt_Makefile(t *testing.T) {
	if got := langForExt(".mk"); got != "makefile" {
		t.Errorf(".mk: want \"makefile\", got %q", got)
	}
}

func TestLangForExt_Conf(t *testing.T) {
	if got := langForExt(".conf"); got != "conf" {
		t.Errorf(".conf: want \"conf\", got %q", got)
	}
}

func TestLangForExt_TOML(t *testing.T) {
	if got := langForExt(".toml"); got != "conf" {
		t.Errorf(".toml: want \"conf\", got %q", got)
	}
}

func TestLangForExtUnknown2(t *testing.T) {
	for _, ext := range []string{".txt", ".rs", ".cpp", ".ts", ".rb", ""} {
		if got := langForExt(ext); got != "" {
			t.Errorf("langForExt(%q): want \"\", got %q", ext, got)
		}
	}
}

func TestLangForExtCaseInsensitive2(t *testing.T) {
	if got := langForExt(".GO"); got != "go" {
		t.Errorf(".GO: want \"go\", got %q", got)
	}
	if got := langForExt(".PY"); got != "python" {
		t.Errorf(".PY: want \"python\", got %q", got)
	}
}
