package editor

import (
	"testing"

	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/elisp"
)

// newEditorWithLisp returns a test editor with an initialised Elisp evaluator.
func newEditorWithLisp(content string) *Editor {
	e := newTestEditor(content)
	e.lisp = elisp.NewEvaluator()
	return e
}

func TestLangModeByName_Known(t *testing.T) {
	for _, name := range []string{"go", "python", "java", "bash", "markdown", "elisp", "json", "yaml", "makefile", "text", "fundamental"} {
		if m := langModeByName(name); m == nil {
			t.Errorf("langModeByName(%q) = nil, want a result", name)
		}
	}
}

func TestLangModeByName_Unknown(t *testing.T) {
	if m := langModeByName("cobol"); m != nil {
		t.Errorf("langModeByName(\"cobol\") = %v, want nil", m)
	}
}

func TestModeIndentStr_GoDefault(t *testing.T) {
	e := newEditorWithLisp("")
	if got := e.modeIndentStr("go"); got != "\t" {
		t.Errorf("go default = %q, want \"\\t\"", got)
	}
}

func TestModeIndentStr_OtherDefault(t *testing.T) {
	e := newEditorWithLisp("")
	if got := e.modeIndentStr("python"); got != "  " {
		t.Errorf("python default = %q, want \"  \"", got)
	}
}

func TestModeIndentStr_ElispInt(t *testing.T) {
	e := newEditorWithLisp("")
	// Set python-indent to 4 via the evaluator.
	_, err := e.lisp.EvalString("(setq python-indent 4)")
	if err != nil {
		t.Fatalf("setq failed: %v", err)
	}
	if got := e.modeIndentStr("python"); got != "    " {
		t.Errorf("python-indent=4: got %q, want \"    \"", got)
	}
}

func TestModeIndentStr_ElispString(t *testing.T) {
	e := newEditorWithLisp("")
	_, err := e.lisp.EvalString(`(setq go-indent "    ")`)
	if err != nil {
		t.Fatalf("setq failed: %v", err)
	}
	if got := e.modeIndentStr("go"); got != "    " {
		t.Errorf("go-indent string: got %q, want \"    \"", got)
	}
}

func TestModeIndentStr_BashUsesSh(t *testing.T) {
	e := newEditorWithLisp("")
	_, err := e.lisp.EvalString("(setq sh-indent 4)")
	if err != nil {
		t.Fatalf("setq failed: %v", err)
	}
	if got := e.modeIndentStr("bash"); got != "    " {
		t.Errorf("bash (sh-indent=4): got %q, want \"    \"", got)
	}
}

func TestCmdGoMode(t *testing.T) {
	e := newTestEditor("package main")
	e.cmdGoMode()
	if buf(e).Mode() != "go" {
		t.Errorf("mode = %q, want \"go\"", buf(e).Mode())
	}
}

func TestCmdPythonMode(t *testing.T) {
	e := newTestEditor("x = 1")
	e.cmdPythonMode()
	if buf(e).Mode() != "python" {
		t.Errorf("mode = %q, want \"python\"", buf(e).Mode())
	}
}

// ---------------------------------------------------------------------------
// debug adapter configuration
// ---------------------------------------------------------------------------

func TestLangModeHasDebugAdapter(t *testing.T) {
	tests := []struct {
		mode string
		want bool
	}{
		{"go", true},   // dlv dap, spawned as a process
		{"java", true}, // jdtls, reached over LSP
		{"python", false},
		{"text", false},
	}
	for _, tt := range tests {
		info := langModeByName(tt.mode)
		if info == nil {
			t.Fatalf("langModeByName(%q) = nil", tt.mode)
		}
		if got := info.hasDebugAdapter(); got != tt.want {
			t.Errorf("%s hasDebugAdapter() = %v, want %v", tt.mode, got, tt.want)
		}
	}
}

func TestLangModeGoUsesDelveProcess(t *testing.T) {
	info := langModeByName("go")
	if info.dapKind != dapAdapterProcess {
		t.Errorf("go dapKind = %v, want dapAdapterProcess", info.dapKind)
	}
	if len(info.dapCmd) != 2 || info.dapCmd[0] != "dlv" || info.dapCmd[1] != "dap" {
		t.Errorf("go dapCmd = %v, want [dlv dap]", info.dapCmd)
	}
}

func TestLangModeJavaUsesJdtls(t *testing.T) {
	info := langModeByName("java")
	if info.dapKind != dapAdapterJdtls {
		t.Errorf("java dapKind = %v, want dapAdapterJdtls", info.dapKind)
	}
	if len(info.dapCmd) != 0 {
		t.Errorf("java dapCmd = %v, want empty (the adapter is not a child process)", info.dapCmd)
	}
	for _, marker := range []string{"pom.xml", "build.gradle"} {
		found := false
		for _, m := range info.rootMarkers {
			if m == marker {
				found = true
			}
		}
		if !found {
			t.Errorf("java rootMarkers = %v, want it to include %q", info.rootMarkers, marker)
		}
	}
}

func TestLangModeDapAdapterName(t *testing.T) {
	if got := langModeByName("go").dapAdapterName(); got != "dlv" {
		t.Errorf("go adapter name = %q, want \"dlv\"", got)
	}
	if got := langModeByName("java").dapAdapterName(); got != "jdtls" {
		t.Errorf("java adapter name = %q, want \"jdtls\"", got)
	}
	if got := langModeByName("text").dapAdapterName(); got != "debug adapter" {
		t.Errorf("unconfigured adapter name = %q, want the generic fallback", got)
	}
}

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

func TestCmdJsonMode(t *testing.T) {
	e := newTestEditor("{ \"key\": 1 }")
	e.cmdJsonMode()
	if got := buf(e).Mode(); got != "json" {
		t.Errorf("json-mode: want mode=%q, got %q", "json", got)
	}
}

func TestCmdYamlModeIndent(t *testing.T) {
	e := newTestEditor("key: value")
	e.cmdYamlMode()
	if buf(e).Mode() != "yaml" {
		t.Errorf("mode = %q, want \"yaml\"", buf(e).Mode())
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

// ---------------------------------------------------------------------------
// configuration variable names
// ---------------------------------------------------------------------------

func TestIndentVarName(t *testing.T) {
	tests := []struct{ mode, want string }{
		{"go", "go-indent"},
		{"python", "python-indent"},
		{"bash", "sh-indent"}, // Emacs spelling, not "bash-indent"
		{"json", "json-indent"},
	}
	for _, tt := range tests {
		if got := indentVarName(tt.mode); got != tt.want {
			t.Errorf("indentVarName(%q) = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

func TestLspCommandVarName(t *testing.T) {
	tests := []struct{ mode, want string }{
		{"go", "go-lsp-command"},
		{"java", "java-lsp-command"},
		{"python", "python-lsp-command"},
	}
	for _, tt := range tests {
		if got := lspCommandVarName(tt.mode); got != tt.want {
			t.Errorf("lspCommandVarName(%q) = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

// indentUnitProbes holds, per major mode, a snippet whose last line should be
// indented one level deep.  TestModeIndentUnitAwarenessMatchesIndentEngine feeds
// each snippet to the indentation engine twice with different indent units and
// checks whether the result changes, which is the ground truth for the
// indentUnitAware flag in langModes.  Every mode needs an entry so that a new
// mode cannot be added without classifying it.
var indentUnitProbes = map[string]string{
	"go":          "package p\n\nfunc f() {\nx\n",
	"java":        "class C {\nx\n",
	"perl":        "sub f {\nx\n",
	"bash":        "if true; then\nx\n",
	"json":        "{\nx\n",
	"python":      "if x:\nx\n",
	"conf":        "[s]\n  a=1\nx\n",
	"markdown":    "- a\n  b\nx\n",
	"yaml":        "a:\n  b: 1\nx\n",
	"makefile":    "all:\n\techo hi\nx\n",
	"gherkin":     "Feature: f\n  Scenario: s\nx\n",
	"elisp":       "(defun f ()\nx\n",
	"text":        "para\n  cont\nx\n",
	"fundamental": "line\n  cont\nx\n",
}

// TestModeIndentUnitAwarenessMatchesIndentEngine keeps langModes'
// indentUnitAware flags honest by exercising the real indentation engine rather
// than trusting a hand-maintained list: a mode is unit-aware exactly when
// changing the indent unit changes the indentation it produces.  This is what
// stops M-x help (and the man page) from advertising a "<mode>-indent" variable
// for a mode that silently discards it — the failure mode this test was written
// for, where markdown-indent and yaml-indent were documented but had no effect.
func TestModeIndentUnitAwarenessMatchesIndentEngine(t *testing.T) {
	for i := range langModes {
		mode := langModes[i].modeName
		probe, ok := indentUnitProbes[mode]
		if !ok {
			t.Errorf("mode %q has no entry in indentUnitProbes: add one and set "+
				"indentUnitAware to whatever the indentation engine actually does", mode)
			continue
		}
		b := buffer.NewWithContent(mode+"-probe", probe)
		b.SetMode(mode)
		bol := b.BeginningOfLine(b.Len() - 1)
		narrow := calcIndentAt(b, mode, bol, "  ")
		wide := calcIndentAt(b, mode, bol, "        ")
		unitUsed := narrow != wide
		if unitUsed != langModes[i].indentUnitAware {
			t.Errorf("mode %q: indent engine honours the indent unit = %v "+
				"(indent with a 2-space unit %q, with an 8-space unit %q), but "+
				"indentUnitAware = %v",
				mode, unitUsed, narrow, wide, langModes[i].indentUnitAware)
		}
	}
}

func TestIndentUnitProbesHaveNoUnknownModes(t *testing.T) {
	for mode := range indentUnitProbes {
		if langModeByName(mode) == nil {
			t.Errorf("indentUnitProbes has an entry for %q, which is not a known major mode", mode)
		}
	}
}

// ---------------------------------------------------------------------------
// <mode>-lsp-command
// ---------------------------------------------------------------------------

func TestLangModeJavaDefaultsToJdtls(t *testing.T) {
	// java debugging goes through the language server (dapStartJdtls), so java
	// without an lspCmd means debug-start can never work.
	info := langModeByName("java")
	if len(info.lspCmd) == 0 || info.lspCmd[0] != "jdtls" {
		t.Errorf("java lspCmd = %v, want [jdtls]", info.lspCmd)
	}
}

// restoreLangModeLspCmds snapshots every mode's lspCmd and restores it when the
// test ends: applyElispLspCommands writes into the process-wide langModes table.
func restoreLangModeLspCmds(t *testing.T) {
	t.Helper()
	saved := make([][]string, len(langModes))
	for i := range langModes {
		saved[i] = langModes[i].lspCmd
	}
	t.Cleanup(func() {
		for i := range langModes {
			langModes[i].lspCmd = saved[i]
		}
	})
}

func TestApplyElispLspCommands_SetsCommandWithArgs(t *testing.T) {
	restoreLangModeLspCmds(t)
	e := newEditorWithLisp("")
	if _, err := e.lisp.EvalString(`(setq java-lsp-command "jdtls -data /tmp/ws")`); err != nil {
		t.Fatalf("setq failed: %v", err)
	}
	e.applyElispLspCommands()

	want := []string{"jdtls", "-data", "/tmp/ws"}
	got := langModeByName("java").lspCmd
	if len(got) != len(want) {
		t.Fatalf("java lspCmd = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("java lspCmd = %v, want %v", got, want)
		}
	}
}

func TestApplyElispLspCommands_GivesModeWithoutDefaultAServer(t *testing.T) {
	restoreLangModeLspCmds(t)
	if len(langModeByName("python").lspCmd) != 0 {
		t.Fatal("python is expected to ship without a default language server")
	}
	e := newEditorWithLisp("")
	if _, err := e.lisp.EvalString(`(setq python-lsp-command "pylsp")`); err != nil {
		t.Fatalf("setq failed: %v", err)
	}
	e.applyElispLspCommands()
	if got := langModeByName("python").lspCmd; len(got) != 1 || got[0] != "pylsp" {
		t.Errorf("python lspCmd = %v, want [pylsp]", got)
	}
}

func TestApplyElispLspCommands_EmptyStringDisablesServer(t *testing.T) {
	restoreLangModeLspCmds(t)
	e := newEditorWithLisp("")
	if _, err := e.lisp.EvalString(`(setq go-lsp-command "")`); err != nil {
		t.Fatalf("setq failed: %v", err)
	}
	e.applyElispLspCommands()
	if got := langModeByName("go").lspCmd; len(got) != 0 {
		t.Errorf("go lspCmd = %v, want empty (server disabled)", got)
	}
}

func TestApplyElispLspCommands_UnsetLeavesDefaults(t *testing.T) {
	restoreLangModeLspCmds(t)
	e := newEditorWithLisp("")
	e.applyElispLspCommands()
	if got := langModeByName("go").lspCmd; len(got) != 1 || got[0] != "gopls" {
		t.Errorf("go lspCmd = %v, want the [gopls] default", got)
	}
}

func TestApplyElispLspCommands_NonStringIgnored(t *testing.T) {
	restoreLangModeLspCmds(t)
	e := newEditorWithLisp("")
	if _, err := e.lisp.EvalString("(setq go-lsp-command 42)"); err != nil {
		t.Fatalf("setq failed: %v", err)
	}
	e.applyElispLspCommands()
	if got := langModeByName("go").lspCmd; len(got) != 1 || got[0] != "gopls" {
		t.Errorf("go lspCmd = %v, want the [gopls] default to survive a non-string value", got)
	}
}

func TestApplyElispLspCommands_NoEvaluator(t *testing.T) {
	restoreLangModeLspCmds(t)
	e := newTestEditor("")
	e.lisp = nil
	e.applyElispLspCommands() // must not panic
}

// TestSetLangModeAppliesLspCommand covers the mode-switch path: applyElispConfig
// applies the overrides at startup, and setLangMode re-reads them so a value the
// user sets after startup takes effect on the next M-x <lang>-mode.
func TestSetLangModeAppliesLspCommand(t *testing.T) {
	restoreLangModeLspCmds(t)
	e := newEditorWithLisp("#!/usr/bin/perl\n")
	// A command that cannot exist, so nothing is spawned when lspActivate runs.
	if _, err := e.lisp.EvalString(`(setq perl-lsp-command "gomacs-no-such-lsp-server")`); err != nil {
		t.Fatalf("setq failed: %v", err)
	}
	buf(e).SetFilename("/tmp/script.pl")

	e.cmdPerlMode()

	got := langModeByName("perl").lspCmd
	if len(got) != 1 || got[0] != "gomacs-no-such-lsp-server" {
		t.Errorf("perl lspCmd = %v, want the configured command", got)
	}
}
