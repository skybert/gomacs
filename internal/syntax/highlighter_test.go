package syntax

import (
	"reflect"
	"testing"
	"unicode/utf8"
)

func TestNilHighlighter(t *testing.T) {
	var h NilHighlighter
	if spans := h.Highlight("anything at all", 0, 5); spans != nil {
		t.Errorf("NilHighlighter.Highlight returned %v, want nil", spans)
	}
	if spans := h.Highlight("", 0, 0); spans != nil {
		t.Errorf("NilHighlighter.Highlight on empty returned %v, want nil", spans)
	}
}

// ---------------------------------------------------------------------------
// Early-termination differential test
// ---------------------------------------------------------------------------
//
// Every highlighter scans from offset 0 but may stop once it has moved past
// `end`.  That optimisation is only correct if the spans it produces for a
// truncated `end` are indistinguishable — for the region the renderer actually
// draws — from the spans produced for the whole buffer.
//
// "Indistinguishable" means: the set of spans that *start* before `end` must be
// identical.  A span that starts before `end` and reaches past it is still
// needed (the visible part of it must be coloured), so it must be present and
// carry the same bounds and face.  Spans starting at or after `end` are never
// drawn and may be present or absent.

// spansStartingBefore returns the spans that begin before limit, i.e. the spans
// the renderer could actually consult when drawing [0, limit).
func spansStartingBefore(spans []Span, limit int) []Span {
	out := []Span{}
	for _, sp := range spans {
		if sp.Start < limit {
			out = append(out, sp)
		}
	}
	return out
}

// diffSample is one (highlighter, source text) pair to check.
type diffSample struct {
	name string
	hl   Highlighter
	text string
}

// goDiffSample exercises block comments, raw strings and a rune literal.  The
// raw string and the block comment are deliberately long so that many
// truncation points fall inside them.
const goDiffSample = "package main\n" +
	"\n" +
	"/* a block comment\n" +
	"   that spans\n" +
	"   several lines */\n" +
	"\n" +
	"import \"fmt\"\n" +
	"\n" +
	"const tmpl = `raw string\n" +
	"with a // fake comment\n" +
	"and a \"quote\" inside\n" +
	"spanning many lines`\n" +
	"\n" +
	"// line comment with 0x2a and keywords: for range func\n" +
	"func main() {\n" +
	"\tvar n int = 42\n" +
	"\tc := 'x'\n" +
	"\tfor i := range n {\n" +
	"\t\tfmt.Println(i, c, tmpl)\n" +
	"\t}\n" +
	"}\n"

// markdownDiffSample has a fenced code block that any truncation point may
// land inside, so the fence state must be carried from offset 0.
const markdownDiffSample = "# Title\n" +
	"\n" +
	"Some *italic* and **bold** and `code` and a [link](http://x).\n" +
	"\n" +
	"```go\n" +
	"func f() {}\n" +
	"# not a heading inside the fence\n" +
	"> not a blockquote either\n" +
	"still fenced\n" +
	"```\n" +
	"\n" +
	"## Second heading\n" +
	"\n" +
	"> a blockquote\n" +
	"\n" +
	"### Third heading\n" +
	"plain trailing text\n"

// pythonDiffSample has a triple-quoted string spanning the truncation range.
const pythonDiffSample = "import os\n" +
	"\n" +
	"DOC = \"\"\"a docstring\n" +
	"with # not a comment\n" +
	"and 'quotes' inside\n" +
	"over several lines\n" +
	"\"\"\"\n" +
	"\n" +
	"@decorator\n" +
	"def f(a, b=0x1f):\n" +
	"    # a comment\n" +
	"    return len(str(a)) + b\n"

// bashDiffSample includes a here-doc body.  BashHighlighter has no here-doc
// state, so the body is scanned as ordinary shell text — the differential
// property must hold there too.
const bashDiffSample = "#!/bin/bash\n" +
	"set -euo pipefail\n" +
	"\n" +
	"cat <<'EOF' > /tmp/x\n" +
	"if then fi inside the here-doc\n" +
	"$NOT_EXPANDED and 'a quote\n" +
	"still inside\n" +
	"EOF\n" +
	"\n" +
	"# a comment\n" +
	"for f in *.txt; do\n" +
	"  echo \"${f} has $(wc -l < \"$f\") lines\"\n" +
	"done\n"

const perlDiffSample = "#!/usr/bin/perl\n" +
	"use strict;\n" +
	"\n" +
	"=pod\n" +
	"POD block that spans\n" +
	"several lines and mentions sub and my\n" +
	"=cut\n" +
	"\n" +
	"my $x = 0xff;\n" +
	"my @list = ('a', \"b$x\", `echo c`);\n" +
	"# comment\n" +
	"print join(',', @list), \"\\n\";\n"

const javaDiffSample = "package com.example;\n" +
	"\n" +
	"/* block comment\n" +
	" * spanning\n" +
	" * lines\n" +
	" */\n" +
	"@Override\n" +
	"public class Foo {\n" +
	"  private static final String S = \"a \\\" quoted string\";\n" +
	"  // line comment\n" +
	"  int f(char c) { return 0x2aL + (int) c; }\n" +
	"}\n"

const elispDiffSample = ";;; init.el --- a comment\n" +
	"(setq big \"a string\n" +
	"that spans lines\n" +
	"and has ;; a fake comment\n" +
	"inside it\")\n" +
	"\n" +
	"(defun f (x)\n" +
	"  \"Docstring.\"\n" +
	"  (when (numberp x)\n" +
	"    (message \"%d\" (+ x 42 ?a))))\n"

const jsonDiffSample = "{\n" +
	"  \"a\": \"a long string value that runs on\",\n" +
	"  \"b\": [1, 2.5, -3e10, true, false, null],\n" +
	"  \"nested\": { \"deep\": { \"deeper\": \"value\" } },\n" +
	"  \"esc\": \"a \\\" quote and a \\\\ backslash\",\n" +
	"  \"last\": 0\n" +
	"}\n"

const yamlDiffSample = "---\n" +
	"# a comment\n" +
	"key: value\n" +
	"block: |\n" +
	"  literal line one\n" +
	"  literal line two\n" +
	"anchored: &anchor v\n" +
	"ref: *anchor\n" +
	"nums: [1, 2.5, 3]\n" +
	"flags: {a: true, b: null}\n" +
	"quoted: \"a string with # not a comment\"\n" +
	"...\n"

const confDiffSample = "# a comment\n" +
	"[section]\n" +
	"key = \"a value\"\n" +
	"other: 42\n" +
	"flag = true\n" +
	"; another comment\n" +
	"[[array.of.tables]]\n" +
	"when = 2026-09-16T10:00:00Z\n" +
	"hex = 0xdeadbeef\n"

const makefileDiffSample = "CC := gcc\n" +
	"CFLAGS ?= -O2\n" +
	"\n" +
	"# a comment\n" +
	"all: build test\n" +
	"\t$(CC) $(CFLAGS) -o $@ $<\n" +
	"\n" +
	"ifeq ($(DEBUG),1)\n" +
	"CFLAGS += -g\n" +
	"endif\n" +
	"\n" +
	".PHONY: all build test\n"

const gherkinDiffSample = "@tag1 @tag2\n" +
	"Feature: something\n" +
	"\n" +
	"  Background:\n" +
	"    Given a <param>\n" +
	"\n" +
	"  Scenario Outline: a case\n" +
	"    When I do \"\"\"\n" +
	"    a docstring body\n" +
	"    Given not a keyword in here\n" +
	"    \"\"\"\n" +
	"    Then it works\n" +
	"\n" +
	"    Examples:\n" +
	"      | a | b |\n" +
	"      | 1 | 2 |\n"

const diffDiffSample = "diff --git a/x.go b/x.go\n" +
	"--- a/x.go\n" +
	"+++ b/x.go\n" +
	"@@ -1,4 +1,4 @@\n" +
	" context line\n" +
	"-removed line\n" +
	"+added line\n" +
	" more context\n" +
	"@@ -20,2 +20,2 @@\n" +
	"-another removal\n" +
	"+another addition\n"

const vcShowDiffSample = "commit deadbeefcafebabe1234567890abcdef12345678\n" +
	"Author: A U Thor <a@example.com>\n" +
	"AuthorDate: Mon Sep 16 10:00:00 2026 +0200\n" +
	"Date: Mon Sep 16 10:00:00 2026 +0200\n" +
	"\n" +
	"    A commit message.\n" +
	"\n" +
	diffDiffSample

const vcLogDiffSample = "deadbee first commit subject\n" +
	"cafebab second commit subject\n" +
	"0123456 third commit subject\n" +
	"abcdef0 fourth commit subject\n"

const vcGrepDiffSample = "internal/editor/editor.go:42:func (e *Editor) Redraw() {\n" +
	"internal/editor/editor.go:99:\tfor _, w := range e.windows {\n" +
	"internal/syntax/bash.go:7:var bashKeywords = map[string]bool{\n" +
	"internal/syntax/bash.go:31:// Highlight implements Highlighter for Bash.\n"

const vcStatusDiffSample = "On branch main\n" +
	"Your branch is up to date with 'origin/main'.\n" +
	"\n" +
	"Changes to be committed:\n" +
	"  (use \"git restore --staged <file>...\" to unstage)\n" +
	"\tmodified:   internal/syntax/bash.go\n" +
	"\tnew file:   internal/syntax/x.go\n" +
	"\n" +
	"Changes not staged for commit:\n" +
	"\tdeleted:    old.go\n" +
	"\trenamed:    a.go -> b.go\n"

const vcCommitDiffSample = "A commit subject\n" +
	"\n" +
	"A longer body.\n" +
	"\n" +
	"# Please enter the commit message.\n" +
	"# Lines starting with '#' are ignored.\n" +
	"#\n" +
	"# On branch main\n"

const vcAnnotateDiffSample = "deadbee1 (A U Thor 2026-09-16 1) package main\n" +
	"deadbee1 (A U Thor 2026-09-16 2) \n" +
	"cafebab2 (B Author  2026-09-16 3) // a comment\n" +
	"cafebab2 (B Author  2026-09-16 4) func main() {\n" +
	"^0123456 (C Author  2026-01-01 5) \tvar n int = 42\n" +
	"^0123456 (C Author  2026-01-01 6) \tfmt.Println(n)\n" +
	"cafebab2 (B Author  2026-09-16 7) }\n"

const compilationDiffSample = "go build ./...\n" +
	"internal/editor/editor.go:42:6: undefined: foo\n" +
	"internal/syntax/bash.go:7: syntax error\n" +
	"make: *** [build] Error 1\n" +
	"some trailing chatter\n"

const helpDiffSample = "gomacs help\n" +
	"===========\n" +
	"\n" +
	"Commands\n" +
	"--------\n" +
	"\n" +
	"Navigation\n" +
	"  forward-char             (C-f)\n" +
	"  backward-char            (C-b)\n" +
	"  next-line                (C-n)\n" +
	"\n" +
	"Editing\n" +
	"  kill-line                (C-k)\n" +
	"  yank                     (C-y)\n" +
	"\n" +
	"Configuration Variables\n" +
	"-----------------------\n" +
	"\n" +
	"  fill-column              70\n" +
	"  go-indent                tab\n"

const dapLocalsDiffSample = "  ▶ conf *editor.Config = &{...}\n" +
	"  ▼ count int = 42\n" +
	"      name string = \"hello\"\n" +
	"    ok bool = true\n" +
	"    err error = nil\n" +
	"plain text line\n"

const dapStackDiffSample = "#0  main.f (main.go:12)\n" +
	"#1  main.g (main.go:34)\n" +
	"#2  main.main (main.go:56)\n" +
	"not a stack line\n"

const manDiffSample = "NAME\n" +
	"     gomacs -- a TTY Emacs clone\n" +
	"\n" +
	"SYNOPSIS\n" +
	"     gomacs [-Q] [--version] [file ...]\n" +
	"\n" +
	"DESCRIPTION\n" +
	"     The -Q flag skips the init file.\n"

// diffSamples pairs every highlighter in the package with source text that
// exercises its multi-line state.
func diffSamples() []diffSample {
	_, manSpans := ManParse("NAME\n     b\bbo\bol\bld\bd and _\bu_\bn_\bd\n")
	_, ansiSpans := ANSIParse("plain \x1b[31mred\x1b[0m then \x1b[1;32mbold green\x1b[0m tail\n")

	return []diffSample{
		{"go", GoHighlighter{}, goDiffSample},
		{"markdown", MarkdownHighlighter{}, markdownDiffSample},
		{"elisp", ElispHighlighter{}, elispDiffSample},
		{"python", PythonHighlighter{}, pythonDiffSample},
		{"java", JavaHighlighter{}, javaDiffSample},
		{"bash", BashHighlighter{}, bashDiffSample},
		{"perl", PerlHighlighter{}, perlDiffSample},
		{"json", JSONHighlighter{}, jsonDiffSample},
		{"yaml", YAMLHighlighter{}, yamlDiffSample},
		{"conf", ConfHighlighter{}, confDiffSample},
		{"makefile", MakefileHighlighter{}, makefileDiffSample},
		{"gherkin", GherkinHighlighter{}, gherkinDiffSample},
		{"diff", DiffHighlighter{}, diffDiffSample},
		{"vc-show", VcShowHighlighter{}, vcShowDiffSample},
		{"vc-log", VcLogHighlighter{}, vcLogDiffSample},
		{"vc-grep", VcGrepHighlighter{}, vcGrepDiffSample},
		{"vc-status", VcStatusHighlighter{}, vcStatusDiffSample},
		{"vc-commit", VcCommitHighlighter{}, vcCommitDiffSample},
		{"vc-annotate", VcAnnotateHighlighter{}, vcAnnotateDiffSample},
		{"vc-annotate+go", VcAnnotateHighlighter{Source: GoHighlighter{}}, vcAnnotateDiffSample},
		{"compilation", CompilationHighlighter{}, compilationDiffSample},
		{"help", HelpHighlighter{}, helpDiffSample},
		{"dap-locals", DapLocalsHighlighter{}, dapLocalsDiffSample},
		{"dap-stack", DapStackHighlighter{}, dapStackDiffSample},
		{"man-spans", ANSIHighlighter{Spans: manSpans}, manDiffSample},
		{"ansi", ANSIHighlighter{Spans: ansiSpans}, "plain red then bold green tail\n"},
		{"nil", NilHighlighter{}, goDiffSample},
	}
}

// TestHighlighterTruncationMatchesFullBuffer is the correctness gate for the
// early-termination optimisation: for every highlighter and every possible
// truncation point, the spans that start before the truncation point must be
// exactly the spans the full-buffer scan produces there.
func TestHighlighterTruncationMatchesFullBuffer(t *testing.T) {
	for _, s := range diffSamples() {
		t.Run(s.name, func(t *testing.T) {
			n := utf8.RuneCountInString(s.text)
			full := s.hl.Highlight(s.text, 0, n)

			// Check every truncation point, not just a few: multi-line
			// constructs must survive a cut anywhere inside them.
			for wantEnd := 0; wantEnd <= n; wantEnd++ {
				got := s.hl.Highlight(s.text, 0, wantEnd)
				wantVisible := spansStartingBefore(full, wantEnd)
				gotVisible := spansStartingBefore(got, wantEnd)
				if !reflect.DeepEqual(gotVisible, wantVisible) {
					t.Fatalf("end=%d: spans starting before end differ\n got: %v\nwant: %v",
						wantEnd, gotVisible, wantVisible)
				}
			}
		})
	}
}

// TestHighlighterTruncationStopsEarly guards the point of the optimisation:
// asking for a small prefix must not produce the whole buffer's worth of spans.
//
// Note that a span may legitimately start after end: the line (or token) it
// belongs to began before end, so it is scanned to completion.  The check is
// therefore that no span starts beyond the *line* containing end, and that the
// truncated scan yields strictly fewer spans than the full one.
func TestHighlighterTruncationStopsEarly(t *testing.T) {
	// Highlighters that legitimately return every span regardless of end:
	// they hold no scan state to advance.
	noTruncation := map[string]bool{
		"man-spans": true, // ANSIHighlighter: pre-computed spans, nothing to scan
		"ansi":      true,
		"nil":       true, // emits nothing at all
	}

	for _, s := range diffSamples() {
		t.Run(s.name, func(t *testing.T) {
			if noTruncation[s.name] {
				t.Skip("highlighter returns pre-computed spans; nothing to truncate")
			}
			n := utf8.RuneCountInString(s.text)
			full := s.hl.Highlight(s.text, 0, n)
			if len(full) < 4 {
				t.Skip("too few spans to measure truncation")
			}
			// Truncate to the first quarter of the text.
			cut := n / 4
			got := s.hl.Highlight(s.text, 0, cut)

			// No span may begin past the end of the line that contains cut.
			limit := cut
			for limit < n && []rune(s.text)[limit] != '\n' {
				limit++
			}
			for _, sp := range got {
				if sp.Start > limit {
					t.Errorf("span %v starts past the line containing end=%d: scan did not stop early", sp, cut)
					break
				}
			}
			if len(got) >= len(full) {
				t.Errorf("truncated scan returned %d spans, full scan %d: no work was saved",
					len(got), len(full))
			}
		})
	}
}
