package syntax

import (
	"reflect"
	"strings"
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

// ---------------------------------------------------------------------------
// Rune fast path
// ---------------------------------------------------------------------------

// TestHighlightRunesMatchesHighlight pins the RuneHighlighter contract: taking
// the caller's []rune must not change a single span, at any truncation point.
func TestHighlightRunesMatchesHighlight(t *testing.T) {
	for _, s := range diffSamples() {
		rh, ok := s.hl.(RuneHighlighter)
		if !ok {
			continue
		}
		t.Run(s.name, func(t *testing.T) {
			runes := []rune(s.text)
			for wantEnd := 0; wantEnd <= len(runes); wantEnd++ {
				want := s.hl.Highlight(s.text, 0, wantEnd)
				got := rh.HighlightRunes(runes, 0, wantEnd)
				if !sameSpans(got, want) {
					t.Fatalf("end=%d: HighlightRunes differs from Highlight\n got: %v\nwant: %v",
						wantEnd, got, want)
				}
			}
		})
	}
}

// TestEveryHighlighterTakesRunes guards against a highlighter being added
// without the rune fast path: the renderer would then have to re-encode the
// whole buffer to a string on every keystroke just for that one mode.
func TestEveryHighlighterTakesRunes(t *testing.T) {
	for _, s := range diffSamples() {
		if _, ok := s.hl.(RuneHighlighter); !ok {
			t.Errorf("%s (%T) does not implement RuneHighlighter", s.name, s.hl)
		}
	}
}

// ---------------------------------------------------------------------------
// Checkpointed resumption
// ---------------------------------------------------------------------------
//
// A resumable highlighter promises that restarting at a reported checkpoint
// produces the same spans as never having stopped.  The tests below check that
// promise exhaustively: every checkpoint the highlighter is willing to report
// (Every: 1, so every safe point it knows of) is used as a restart point, and
// the spans kept from before it plus the spans produced after it must equal the
// spans of a single scan from offset 0.
//
// The samples are the same ones the truncation test uses, so a block comment, a
// raw string, a here-doc, a POD block and a fenced code block all straddle
// checkpoint boundaries.

// sameSpans reports whether two span slices hold the same spans, treating a nil
// slice and an empty one as equal.
func sameSpans(a, b []Span) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// checkpoint is one restart point reported during a scan.
type checkpoint struct {
	st     ScanState
	nspans int
}

// collectCheckpoints runs a full resumable scan of runes and returns every
// restart point it reports at the given interval.
func collectCheckpoints(hl Resumable, runes []rune, every int) []checkpoint {
	var out []checkpoint
	cp := Checkpoints{
		Every: every,
		Report: func(st ScanState, nspans int) {
			out = append(out, checkpoint{st, nspans})
		},
	}
	hl.HighlightResume(runes, ScanState{}, len(runes), &cp)
	return out
}

// resumableSamples returns the diff samples whose highlighter can resume.
func resumableSamples(t *testing.T) []diffSample {
	t.Helper()
	var out []diffSample
	for _, s := range diffSamples() {
		if _, ok := s.hl.(Resumable); ok {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		t.Fatal("no resumable highlighters found")
	}
	return out
}

// TestHighlightResumeZeroStateMatchesFullScan checks the base case of the
// Resumable contract: the zero state means "start at the beginning".
func TestHighlightResumeZeroStateMatchesFullScan(t *testing.T) {
	for _, s := range resumableSamples(t) {
		t.Run(s.name, func(t *testing.T) {
			runes := []rune(s.text)
			res := s.hl.(Resumable)
			for wantEnd := 0; wantEnd <= len(runes); wantEnd++ {
				want := res.HighlightRunes(runes, 0, wantEnd)
				got := res.HighlightResume(runes, ScanState{}, wantEnd, nil)
				if !sameSpans(got, want) {
					t.Fatalf("end=%d: resume from the zero state differs from a plain scan\n got: %v\nwant: %v",
						wantEnd, got, want)
				}
			}
		})
	}
}

// TestHighlightResumeFromEveryCheckpoint is the correctness gate for
// checkpointed highlighting: for every checkpoint, the spans before it plus the
// spans a resumed scan produces must be exactly the spans of one scan from 0.
func TestHighlightResumeFromEveryCheckpoint(t *testing.T) {
	for _, s := range resumableSamples(t) {
		t.Run(s.name, func(t *testing.T) {
			runes := []rune(s.text)
			n := len(runes)
			res := s.hl.(Resumable)
			full := res.HighlightRunes(runes, 0, n)
			cps := collectCheckpoints(res, runes, 1)
			if len(cps) == 0 {
				if len(full) != 0 {
					t.Fatal("no checkpoints reported; resumption is untested for this highlighter")
				}
				t.Skip("highlighter emits no spans, so it has nothing to resume")
			}
			for _, c := range cps {
				if c.nspans > len(full) {
					t.Fatalf("checkpoint at %d reports %d spans, full scan has %d",
						c.st.Pos, c.nspans, len(full))
				}
				got := append(full[:c.nspans:c.nspans], res.HighlightResume(runes, c.st, n, nil)...)
				if !sameSpans(got, full) {
					t.Fatalf("resume from %+v (after %d spans) differs from a full scan\n got: %v\nwant: %v",
						c.st, c.nspans, got, full)
				}
			}
		})
	}
}

// TestHighlightResumeTruncatedMatchesFullBuffer combines resumption with early
// termination, which is how the renderer actually uses both: resume from a
// checkpoint *and* stop at the end of the visible region.  The spans that start
// before end must still match the full-buffer scan.
func TestHighlightResumeTruncatedMatchesFullBuffer(t *testing.T) {
	for _, s := range resumableSamples(t) {
		t.Run(s.name, func(t *testing.T) {
			runes := []rune(s.text)
			n := len(runes)
			res := s.hl.(Resumable)
			full := res.HighlightRunes(runes, 0, n)
			for _, c := range collectCheckpoints(res, runes, 1) {
				for wantEnd := c.st.Pos; wantEnd <= n; wantEnd++ {
					got := append(full[:c.nspans:c.nspans],
						res.HighlightResume(runes, c.st, wantEnd, nil)...)
					wantVisible := spansStartingBefore(full, wantEnd)
					gotVisible := spansStartingBefore(got, wantEnd)
					if !sameSpans(gotVisible, wantVisible) {
						t.Fatalf("resume from %+v with end=%d: visible spans differ\n got: %v\nwant: %v",
							c.st, wantEnd, gotVisible, wantVisible)
					}
				}
			}
		})
	}
}

// TestCheckpointsAreAligned pins the property the span cache relies on when it
// keeps checkpoints across an edit: a resumed scan reports the same offsets a
// scan from 0 would have, so checkpoint positions do not drift as the resume
// point walks forward.
func TestCheckpointsAreAligned(t *testing.T) {
	const every = 8
	for _, s := range resumableSamples(t) {
		t.Run(s.name, func(t *testing.T) {
			runes := []rune(s.text)
			res := s.hl.(Resumable)
			full := collectCheckpoints(res, runes, every)
			if len(full) < 2 {
				t.Skipf("only %d checkpoints at Every=%d; nothing to compare", len(full), every)
			}
			for i, c := range full[:len(full)-1] {
				var got []checkpoint
				cp := Checkpoints{
					Every: every,
					Report: func(st ScanState, nspans int) {
						got = append(got, checkpoint{st, nspans})
					},
				}
				res.HighlightResume(runes, c.st, len(runes), &cp)
				want := full[i+1:]
				if len(got) != len(want) {
					t.Fatalf("resume from %+v reported %d checkpoints, want %d",
						c.st, len(got), len(want))
				}
				for j := range got {
					if got[j].st != want[j].st {
						t.Fatalf("resume from %+v: checkpoint %d is %+v, want %+v",
							c.st, j, got[j].st, want[j].st)
					}
				}
			}
		})
	}
}

// TestCheckpointsDisabled documents that a nil or unarmed Checkpoints reports
// nothing, which is what makes HighlightResume usable as a plain bounded scan.
func TestCheckpointsDisabled(t *testing.T) {
	runes := []rune(goDiffSample)
	res := GoHighlighter{}
	calls := 0
	cp := Checkpoints{Report: func(ScanState, int) { calls++ }} // Every == 0
	res.HighlightResume(runes, ScanState{}, len(runes), &cp)
	if calls != 0 {
		t.Errorf("Every=0 reported %d checkpoints, want 0", calls)
	}
	// A nil Checkpoints must not panic.
	res.HighlightResume(runes, ScanState{}, len(runes), nil)
}

// TestCorruptCheckpointStateIsDetected is the mutation check on the guard above:
// the resume tests would be worthless if ScanState carried nothing that mattered.
// For every highlighter that keeps cross-line state, flipping that state at a
// checkpoint must change the spans — otherwise the state is not really being
// used and the differential tests are vacuous.
func TestCorruptCheckpointStateIsDetected(t *testing.T) {
	tests := []struct {
		name string
		hl   Resumable
		text string
	}{
		{"markdown", MarkdownHighlighter{}, markdownDiffSample},
		{"gherkin", GherkinHighlighter{}, gherkinDiffSample},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runes := []rune(tc.text)
			n := len(runes)
			full := tc.hl.HighlightRunes(runes, 0, n)
			differed := 0
			for _, c := range collectCheckpoints(tc.hl, runes, 1) {
				bad := c.st
				bad.Fence = !bad.Fence
				got := append(full[:c.nspans:c.nspans],
					tc.hl.HighlightResume(runes, bad, n, nil)...)
				if !sameSpans(got, full) {
					differed++
				}
			}
			if differed == 0 {
				t.Errorf("flipping ScanState.Fence never changed the spans: " +
					"the state is unused, so the resume tests prove nothing")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Resumption benchmarks
// ---------------------------------------------------------------------------

// benchSource returns a large sample of the named language: the diff sample
// repeated until it is at least runes long, so multi-line constructs recur
// throughout.
func benchSource(sample string, runes int) []rune {
	var sb strings.Builder
	for utf8.RuneCountInString(sb.String()) < runes {
		sb.WriteString(sample)
	}
	return []rune(sb.String())
}

// benchResume compares the two ways of highlighting the screenful that ends at
// the far end of a large buffer: scanning everything from offset 0, and resuming
// from the last checkpoint before it.  This is the per-keystroke cost the span
// cache pays, isolated from the rest of a frame.
func benchResume(bench *testing.B, hl Resumable, sample string, fromZero bool) {
	const size = 400000
	const every = 4096
	runes := benchSource(sample, size)
	end := len(runes)

	// Find the last checkpoint, which is where a keystroke at the end of the
	// buffer would resume from.
	last := ScanState{}
	cp := Checkpoints{Every: every, Report: func(st ScanState, _ int) { last = st }}
	hl.HighlightResume(runes, ScanState{}, end, &cp)

	st := last
	if fromZero {
		st = ScanState{}
	}
	bench.ReportAllocs()
	bench.ResetTimer()
	for range bench.N {
		hl.HighlightResume(runes, st, end, nil)
	}
}

func BenchmarkResumeGoFromZero(bench *testing.B) {
	benchResume(bench, GoHighlighter{}, goDiffSample, true)
}

func BenchmarkResumeGoFromCheckpoint(bench *testing.B) {
	benchResume(bench, GoHighlighter{}, goDiffSample, false)
}

func BenchmarkResumeMarkdownFromZero(bench *testing.B) {
	benchResume(bench, MarkdownHighlighter{}, markdownDiffSample, true)
}

func BenchmarkResumeMarkdownFromCheckpoint(bench *testing.B) {
	benchResume(bench, MarkdownHighlighter{}, markdownDiffSample, false)
}

func BenchmarkResumeYAMLFromZero(bench *testing.B) {
	benchResume(bench, YAMLHighlighter{}, yamlDiffSample, true)
}

func BenchmarkResumeYAMLFromCheckpoint(bench *testing.B) {
	benchResume(bench, YAMLHighlighter{}, yamlDiffSample, false)
}
