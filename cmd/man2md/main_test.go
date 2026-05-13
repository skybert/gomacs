package main

import (
	"os"
	"strings"
	"testing"
)

// ---- splitArgs -------------------------------------------------------------

func TestSplitArgsSimple(t *testing.T) {
	got := splitArgs(".SH NAME")
	want := []string{".SH", "NAME"}
	if len(got) != len(want) {
		t.Fatalf("splitArgs simple: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("splitArgs[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSplitArgsQuoted(t *testing.T) {
	got := splitArgs(`.SH "SEE ALSO"`)
	want := []string{".SH", "SEE ALSO"}
	if len(got) != len(want) {
		t.Fatalf("splitArgs quoted: got %v, want %v", got, want)
	}
	if got[1] != "SEE ALSO" {
		t.Errorf("splitArgs quoted arg = %q, want %q", got[1], "SEE ALSO")
	}
}

func TestSplitArgsTabs(t *testing.T) {
	got := splitArgs(".B\targ1\targ2")
	if len(got) != 3 {
		t.Fatalf("splitArgs tabs: got %v", got)
	}
	if got[0] != ".B" || got[1] != "arg1" || got[2] != "arg2" {
		t.Errorf("splitArgs tabs: got %v", got)
	}
}

func TestSplitArgsEmptyString(t *testing.T) {
	got := splitArgs("")
	if len(got) != 0 {
		t.Errorf("splitArgs empty: got %v, want []", got)
	}
}

func TestSplitArgsOnlyWhitespace(t *testing.T) {
	got := splitArgs("   \t  ")
	if len(got) != 0 {
		t.Errorf("splitArgs whitespace only: got %v, want []", got)
	}
}

func TestSplitArgsUnclosedQuote(t *testing.T) {
	// Unclosed quote collects to end of string.
	got := splitArgs(`.SH "unclosed`)
	if len(got) != 2 {
		t.Fatalf("splitArgs unclosed quote: got %v", got)
	}
	if got[1] != "unclosed" {
		t.Errorf("splitArgs unclosed quote arg = %q, want %q", got[1], "unclosed")
	}
}

func TestSplitArgsMacroOnly(t *testing.T) {
	got := splitArgs(".PP")
	if len(got) != 1 || got[0] != ".PP" {
		t.Errorf("splitArgs macro only: got %v", got)
	}
}

// ---- inline ----------------------------------------------------------------

func TestInlinePlainText(t *testing.T) {
	if got := inline("hello world"); got != "hello world" {
		t.Errorf("inline plain = %q, want %q", got, "hello world")
	}
}

func TestInlineBold(t *testing.T) {
	got := inline(`\fBbold\fP`)
	if !strings.Contains(got, "**bold**") {
		t.Errorf("inline bold = %q, should contain **bold**", got)
	}
}

func TestInlineItalic(t *testing.T) {
	got := inline(`\fIitalic\fP`)
	if !strings.Contains(got, "_italic_") {
		t.Errorf("inline italic = %q, should contain _italic_", got)
	}
}

func TestInlineBoldThenItalic(t *testing.T) {
	got := inline(`\fBbold\fR and \fIitalic\fR`)
	if !strings.Contains(got, "**bold**") {
		t.Errorf("inline bold+italic = %q, missing **bold**", got)
	}
	if !strings.Contains(got, "_italic_") {
		t.Errorf("inline bold+italic = %q, missing _italic_", got)
	}
}

func TestInlineDashEscape(t *testing.T) {
	got := inline(`dash\-here`)
	if got != "dash-here" {
		t.Errorf("inline dash escape = %q, want %q", got, "dash-here")
	}
}

func TestInlineZeroWidthJoiner(t *testing.T) {
	// \& is a zero-width joiner — should be discarded.
	got := inline(`ab\&cd`)
	if got != "abcd" {
		t.Errorf("inline \\& = %q, want %q", got, "abcd")
	}
}

func TestInlineInlineComment(t *testing.T) {
	// \" starts an inline comment — everything after is dropped.
	got := inline(`visible \" this is a comment`)
	if strings.Contains(got, "comment") {
		t.Errorf("inline comment: comment text leaked into output: %q", got)
	}
	if !strings.Contains(got, "visible") {
		t.Errorf("inline comment: visible text missing from %q", got)
	}
}

func TestInlineTrailingSpace(t *testing.T) {
	// Trailing whitespace should be trimmed.
	got := inline("hello   ")
	if got != "hello" {
		t.Errorf("inline trailing space = %q, want %q", got, "hello")
	}
}

func TestInlineFontReset(t *testing.T) {
	// \fR and \fP both close open font tags.
	gotR := inline(`\fBtest\fR`)
	gotP := inline(`\fBtest\fP`)
	if gotR != gotP {
		t.Errorf("\\fR and \\fP should produce same output: %q vs %q", gotR, gotP)
	}
}

func TestInlineUnknownEscape(t *testing.T) {
	// Unknown escape: emit the escaped character.
	got := inline(`\z`)
	if got != "z" {
		t.Errorf("unknown escape = %q, want %q", got, "z")
	}
}

// ---- plainText -------------------------------------------------------------

func TestPlainTextNoMarkup(t *testing.T) {
	if got := plainText("hello"); got != "hello" {
		t.Errorf("plainText = %q, want %q", got, "hello")
	}
}

func TestPlainTextStripsBold(t *testing.T) {
	got := plainText(`\fBbold\fP`)
	if strings.Contains(got, "**") || strings.Contains(got, "\\f") {
		t.Errorf("plainText should strip bold markers, got %q", got)
	}
	if got != "bold" {
		t.Errorf("plainText bold = %q, want %q", got, "bold")
	}
}

func TestPlainTextDashEscape(t *testing.T) {
	got := plainText(`em\-dash`)
	if got != "em-dash" {
		t.Errorf("plainText dash = %q, want %q", got, "em-dash")
	}
}

func TestPlainTextComment(t *testing.T) {
	got := plainText(`visible\" ignored`)
	if strings.Contains(got, "ignored") {
		t.Errorf("plainText: comment text leaked: %q", got)
	}
}

func TestPlainTextTrailingWhitespace(t *testing.T) {
	got := plainText("code line   ")
	if strings.HasSuffix(got, " ") {
		t.Errorf("plainText: trailing whitespace not trimmed: %q", got)
	}
}

// ---- isTableFormat ---------------------------------------------------------

func TestIsTableFormatTrue(t *testing.T) {
	cases := []string{"l l.", "l l l.", "c c.", "  l l.  "}
	for _, c := range cases {
		if !isTableFormat(c) {
			t.Errorf("isTableFormat(%q) = false, want true", c)
		}
	}
}

func TestIsTableFormatFalseWithTab(t *testing.T) {
	// A line with a tab is a data row, not a format line.
	if isTableFormat("col1\tcol2") {
		t.Error("isTableFormat with tab should be false")
	}
}

func TestIsTableFormatFalseWithBackslash(t *testing.T) {
	if isTableFormat(`l \fBl.`) {
		t.Error("isTableFormat with backslash should be false")
	}
}

func TestIsTableFormatFalseNoDot(t *testing.T) {
	if isTableFormat("l l l") {
		t.Error("isTableFormat without trailing dot should be false")
	}
}

func TestIsTableFormatEmptyString(t *testing.T) {
	if isTableFormat("") {
		t.Error("isTableFormat empty string should be false (no trailing dot)")
	}
}

// ---- altFonts --------------------------------------------------------------

func TestAltFontsBR(t *testing.T) {
	// .BR: first arg bold, second plain.
	got := altFonts([]string{"bold", "plain"}, true)
	if !strings.Contains(got, "**bold**") {
		t.Errorf("altFonts BR: missing **bold** in %q", got)
	}
	if !strings.Contains(got, "plain") {
		t.Errorf("altFonts BR: missing plain in %q", got)
	}
}

func TestAltFontsRB(t *testing.T) {
	// .RB: first arg plain, second bold.
	got := altFonts([]string{"plain", "bold"}, false)
	if strings.HasPrefix(got, "**") {
		t.Errorf("altFonts RB: first word should not be bold: %q", got)
	}
	if !strings.Contains(got, "**bold**") {
		t.Errorf("altFonts RB: missing **bold** in %q", got)
	}
}

func TestAltFontsEmpty(t *testing.T) {
	got := altFonts([]string{}, true)
	if got != "" {
		t.Errorf("altFonts empty = %q, want empty", got)
	}
}

func TestAltFontsSingleArg(t *testing.T) {
	got := altFonts([]string{"word"}, true)
	if !strings.Contains(got, "**word**") {
		t.Errorf("altFonts single BR = %q, want **word**", got)
	}
}

// ---- proc / run (the full state machine) -----------------------------------

func runProc(input string) string {
	lines := strings.Split(strings.TrimRight(input, "\n"), "\n")
	p := &proc{lines: lines}
	p.run()
	return p.sb.String()
}

func TestProcTH(t *testing.T) {
	out := runProc(".TH GOMACS 1 \"2024-01-01\"")
	if !strings.Contains(out, "# gomacs(1)") {
		t.Errorf(".TH: got %q, want '# gomacs(1)'", out)
	}
}

func TestProcSH(t *testing.T) {
	out := runProc(".SH NAME")
	if !strings.Contains(out, "## NAME") {
		t.Errorf(".SH NAME: got %q, want '## NAME'", out)
	}
}

func TestProcSS(t *testing.T) {
	out := runProc(".SS Subsection")
	if !strings.Contains(out, "### Subsection") {
		t.Errorf(".SS: got %q, want '### Subsection'", out)
	}
}

func TestProcPP(t *testing.T) {
	out := runProc(".PP\nsome text")
	if !strings.Contains(out, "some text") {
		t.Errorf(".PP: missing 'some text' in %q", out)
	}
}

func TestProcB(t *testing.T) {
	out := runProc(".B bold-word")
	if !strings.Contains(out, "**bold-word**") {
		t.Errorf(".B: got %q, want **bold-word**", out)
	}
}

func TestProcI(t *testing.T) {
	out := runProc(".I italic-word")
	if !strings.Contains(out, "_italic-word_") {
		t.Errorf(".I: got %q, want _italic-word_", out)
	}
}

func TestProcBR(t *testing.T) {
	out := runProc(".BR bold plain")
	if !strings.Contains(out, "**bold**") {
		t.Errorf(".BR: got %q, missing **bold**", out)
	}
	if !strings.Contains(out, "plain") {
		t.Errorf(".BR: got %q, missing plain", out)
	}
}

func TestProcRB(t *testing.T) {
	out := runProc(".RB plain bold")
	if !strings.Contains(out, "**bold**") {
		t.Errorf(".RB: got %q, missing **bold**", out)
	}
}

func TestProcNFCodeBlock(t *testing.T) {
	out := runProc(".nf\ncode line\n.fi")
	if !strings.Contains(out, "```") {
		t.Errorf(".nf/.fi: missing ``` in %q", out)
	}
	if !strings.Contains(out, "code line") {
		t.Errorf(".nf/.fi: missing code line in %q", out)
	}
}

func TestProcNFNoMarkupInsideBlock(t *testing.T) {
	// Font escapes inside .nf blocks should be stripped (not turned into Markdown).
	out := runProc(`.nf
\fBbold\fP
.fi`)
	if strings.Contains(out, "**") {
		t.Errorf(".nf block: Markdown bold marker ** should not appear: %q", out)
	}
}

func TestProcTPTaggedParagraph(t *testing.T) {
	out := runProc(".TP\nterm\ndefinition text")
	if !strings.Contains(out, "term") {
		t.Errorf(".TP: missing term in %q", out)
	}
	if !strings.Contains(out, "definition text") {
		t.Errorf(".TP: missing definition in %q", out)
	}
}

func TestProcTable(t *testing.T) {
	input := ".TS\nl l.\nKey\tValue\n.TE"
	out := runProc(input)
	if !strings.Contains(out, "|") {
		t.Errorf(".TS/.TE: expected markdown table with | in %q", out)
	}
	if !strings.Contains(out, "Key") {
		t.Errorf(".TS/.TE: missing 'Key' in %q", out)
	}
	if !strings.Contains(out, "Value") {
		t.Errorf(".TS/.TE: missing 'Value' in %q", out)
	}
}

func TestProcTableFormatLineSkipped(t *testing.T) {
	// The format line "l l." should not appear as a table row.
	input := ".TS\nl l.\nA\tB\n.TE"
	out := runProc(input)
	// The format line "l l." should not appear literally.
	if strings.Contains(out, "l l.") {
		t.Errorf("table format line leaked into output: %q", out)
	}
}

func TestProcPlainTextLine(t *testing.T) {
	out := runProc("This is plain text.")
	if !strings.Contains(out, "This is plain text.") {
		t.Errorf("plain text line: got %q", out)
	}
}

func TestProcEmptyLine(t *testing.T) {
	// Empty text line → blank line in output (p.nl()).
	out := runProc("first\n\nsecond")
	if !strings.Contains(out, "first") || !strings.Contains(out, "second") {
		t.Errorf("empty line: got %q", out)
	}
}

func TestProcUnknownMacroIgnored(t *testing.T) {
	// Unknown macros are silently ignored.
	out := runProc(".br\ntext")
	if !strings.Contains(out, "text") {
		t.Errorf("after .br unknown macro: got %q", out)
	}
}

func TestProcVersionSubstitution(t *testing.T) {
	// The convert() function replaces @VERSION@ before feeding to proc.
	// Test the proc via the convert() function using a temp file.
	content := ".TH GOMACS 1\n.SH VERSION\n@VERSION@"
	f, err := os.CreateTemp("", "man2md-*.1")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	out, err := convert(f.Name(), "v1.2.3", "Alice", "2024-01-01")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if !strings.Contains(out, "v1.2.3") {
		t.Errorf("convert: @VERSION@ not substituted in %q", out)
	}
}

func TestProcAuthorsSubstitution(t *testing.T) {
	content := "@AUTHORS@"
	f, err := os.CreateTemp("", "man2md-*.1")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	out, err := convert(f.Name(), "v1.0", "Bob, Carol", "2024-01-01")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if !strings.Contains(out, "Bob, Carol") {
		t.Errorf("convert: @AUTHORS@ not substituted in %q", out)
	}
}

func TestConvertMissingFileReturnsError(t *testing.T) {
	_, err := convert("/nonexistent/path/file.1", "v1", "a", "2024-01-01")
	if err == nil {
		t.Error("convert: expected error for missing file, got nil")
	}
}

func TestProcTablePipeEscape(t *testing.T) {
	// A literal | in a cell should be escaped to \| so the Markdown table parses.
	input := ".TS\nl l.\nkey|name\tvalue\n.TE"
	out := runProc(input)
	if strings.Contains(out, "key|name") && !strings.Contains(out, `key\|name`) {
		t.Errorf("table: pipe not escaped in %q", out)
	}
}
