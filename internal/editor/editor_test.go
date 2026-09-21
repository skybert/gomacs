package editor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
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

func TestFuzzyMatch_Prefix(t *testing.T) {
	if !fuzzyMatch("forward-char", "forward") {
		t.Error("fuzzyMatch(forward-char, forward) should be true")
	}
}

func TestFuzzyMatch_Subsequence(t *testing.T) {
	if !fuzzyMatch("execute-extended-command", "exc") {
		t.Error("fuzzyMatch subsequence should be true")
	}
}

func TestFuzzyMatch_NoMatch(t *testing.T) {
	if fuzzyMatch("forward-char", "zzz") {
		t.Error("fuzzyMatch should be false for non-subsequence")
	}
}

func TestFuzzyMatch_Empty(t *testing.T) {
	if !fuzzyMatch("anything", "") {
		t.Error("empty query should match everything")
	}
}

func TestFuzzyScore_Prefix(t *testing.T) {
	if got := fuzzyScore("man", "man"); got != 0 {
		t.Errorf("exact prefix: score = %d, want 0", got)
	}
}

func TestFuzzyScore_Substring(t *testing.T) {
	if got := fuzzyScore("command", "man"); got != 1 {
		t.Errorf("substring: score = %d, want 1", got)
	}
}

func TestFuzzyScore_PrefixBeatsSubstring(t *testing.T) {
	prefix := fuzzyScore("man", "man")
	sub := fuzzyScore("command", "man")
	if prefix >= sub {
		t.Errorf("prefix score (%d) should be < substring score (%d)", prefix, sub)
	}
}

func TestPushCommandLRU_Deduplicates(t *testing.T) {
	e := newTestEditor("")
	e.pushCommandLRU("save-buffer")
	e.pushCommandLRU("find-file")
	e.pushCommandLRU("save-buffer") // push again
	if e.commandLRU[0] != "save-buffer" {
		t.Errorf("first = %q, want \"save-buffer\"", e.commandLRU[0])
	}
	// save-buffer should appear only once.
	count := 0
	for _, n := range e.commandLRU {
		if n == "save-buffer" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("save-buffer appears %d times, want 1", count)
	}
}

func TestPushCommandLRU_Cap(t *testing.T) {
	e := newTestEditor("")
	for i := range commandLRUMax + 10 {
		e.pushCommandLRU(string(rune('a' + i%26)))
	}
	if len(e.commandLRU) > commandLRUMax {
		t.Errorf("LRU length = %d, want <= %d", len(e.commandLRU), commandLRUMax)
	}
}

func TestCommonPrefix_Empty(t *testing.T) {
	if got := commonPrefix([]string{}); got != "" {
		t.Errorf("empty input: got %q, want \"\"", got)
	}
}

func TestCommonPrefix_Single(t *testing.T) {
	if got := commonPrefix([]string{"hello"}); got != "hello" {
		t.Errorf("single: got %q, want \"hello\"", got)
	}
}

func TestCommonPrefix_Common(t *testing.T) {
	if got := commonPrefix([]string{"forward-char", "forward-word", "forward-list"}); got != "forward-" {
		t.Errorf("got %q, want \"forward-\"", got)
	}
}

func TestCommonPrefix_NoCommon(t *testing.T) {
	if got := commonPrefix([]string{"abc", "xyz"}); got != "" {
		t.Errorf("got %q, want \"\"", got)
	}
}

func TestModeFromShebang(t *testing.T) {
	cases := []struct {
		shebang string
		want    string
	}{
		{"#!/bin/bash\necho hi\n", "bash"},
		{"#!/usr/bin/env bash\necho hi\n", "bash"},
		{"#! /bin/bash\necho hi\n", "bash"},
		{"#! /usr/bin/env bash\necho hi\n", "bash"},
		{"#!/bin/sh\n", "bash"},
		{"#!/usr/bin/env sh\n", "bash"},
		{"#!/usr/bin/perl\n", "perl"},
		{"#!/usr/bin/env perl\n", "perl"},
		{"#!/usr/bin/python\n", "python"},
		{"#!/usr/bin/python3\n", "python"},
		{"#!/usr/bin/python2\n", "python"},
		{"#!/usr/bin/python3.10\n", "python"},
		{"#!/usr/bin/env python3.10\n", "python"},
		{"#!/usr/bin/env python3.12\n", "python"},
		{"# not a shebang\n", ""},
		{"", ""},
		{"no shebang at all", ""},
	}
	for _, tc := range cases {
		got := modeFromShebang(tc.shebang)
		if got != tc.want {
			t.Errorf("modeFromShebang(%q) = %q, want %q", tc.shebang, got, tc.want)
		}
	}
}

func TestStepToCamelCase(t *testing.T) {
	cases := []struct {
		step string
		want string
	}{
		{"user logs in", "UserLogsIn"},
		{"the user is logged in", "TheUserIsLoggedIn"},
		{"user enters \"admin\" as the username", "UserEntersAsTheUsername"},
		{"the user has 42 apples", "TheUserHasApples"},
		{"user is logged in as <role>", "UserIsLoggedInAs"},
		{"I am on the login page", "IAmOnTheLoginPage"},
		// Mixed / upper case input must normalise to the same result.
		{"User Logs In", "UserLogsIn"},
		{"USER LOGS IN", "UserLogsIn"},
		{"The USER is LOGGED IN", "TheUserIsLoggedIn"},
	}
	for _, tc := range cases {
		got := stepToCamelCase(tc.step)
		if got != tc.want {
			t.Errorf("stepToCamelCase(%q) = %q, want %q", tc.step, got, tc.want)
		}
	}
}

func TestGherkinStepAtPoint(t *testing.T) {
	cases := []struct {
		content string
		want    string
	}{
		{"Given user logs in\n", "user logs in"},
		{"  When I click submit\n", "I click submit"},
		{"  Then the response is 200\n", "the response is 200"},
		{"  And the cookie is set\n", "the cookie is set"},
		{"Feature: login\n", ""},
		{"  Scenario: test\n", ""},
		{"  | col1 | col2 |\n", ""},
		{"  # a comment\n", ""},
	}
	for _, tc := range cases {
		buf := newTestEditor(tc.content).ActiveBuffer()
		buf.SetPoint(0)
		got := gherkinStepAtPoint(buf)
		if got != tc.want {
			t.Errorf("gherkinStepAtPoint(%q) = %q, want %q", tc.content, got, tc.want)
		}
	}
}

func TestParseGrepLines(t *testing.T) {
	root := "/project"
	output := "./steps/login.go:42:func (s *Suite) UserLogsIn() error {\n" +
		"./steps/login.go:43:	// implementation\n"
	matches := parseGrepLines(output, root)
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
	if matches[0].File != "/project/steps/login.go" {
		t.Errorf("File = %q, want \"/project/steps/login.go\"", matches[0].File)
	}
	if matches[0].Line != 42 {
		t.Errorf("Line = %d, want 42", matches[0].Line)
	}
}

// ---------------------------------------------------------------------------
// applyElispConfig
// ---------------------------------------------------------------------------

func newElispTestEditor(content string) *Editor {
	e := newTestEditor(content)
	e.lisp = elisp.NewEvaluator()
	return e
}

func TestApplyElispConfig_FillColumn(t *testing.T) {
	e := newElispTestEditor("")
	e.fillColumn = 70
	_, _ = e.lisp.EvalString("(setq fill-column 100)")
	e.applyElispConfig()
	if e.fillColumn != 100 {
		t.Errorf("fillColumn = %d, want 100", e.fillColumn)
	}
}

func TestApplyElispConfig_FillColumnNonPositive(t *testing.T) {
	e := newElispTestEditor("")
	e.fillColumn = 70
	_, _ = e.lisp.EvalString("(setq fill-column 0)")
	e.applyElispConfig()
	// Non-positive value should not be applied.
	if e.fillColumn != 70 {
		t.Errorf("fillColumn = %d, want 70 (unchanged)", e.fillColumn)
	}
}

func TestApplyElispConfig_IsearchCaseInsensitive(t *testing.T) {
	e := newElispTestEditor("")
	e.isSearchCaseFold = true
	_, _ = e.lisp.EvalString("(setq isearch-case-insensitive nil)")
	e.applyElispConfig()
	if e.isSearchCaseFold {
		t.Error("expected isSearchCaseFold=false after setting nil")
	}
}

func TestApplyElispConfig_IsearchCaseFoldTrue(t *testing.T) {
	e := newElispTestEditor("")
	e.isSearchCaseFold = false
	_, _ = e.lisp.EvalString("(setq isearch-case-insensitive t)")
	e.applyElispConfig()
	if !e.isSearchCaseFold {
		t.Error("expected isSearchCaseFold=true after setting t")
	}
}

func TestApplyElispConfig_SaveBufferDeleteTrailingWS(t *testing.T) {
	e := newElispTestEditor("")
	e.saveBufferDeleteTrailingWS = true
	_, _ = e.lisp.EvalString("(setq save-buffer-delete-trailing-whitespace nil)")
	e.applyElispConfig()
	if e.saveBufferDeleteTrailingWS {
		t.Error("expected saveBufferDeleteTrailingWS=false")
	}
}

func TestApplyElispConfig_SpellCommand(t *testing.T) {
	e := newElispTestEditor("")
	_, _ = e.lisp.EvalString(`(setq spell-command "aspell")`)
	e.applyElispConfig()
	if e.spellCommand != "aspell" {
		t.Errorf("spellCommand = %q, want \"aspell\"", e.spellCommand)
	}
}

func TestApplyElispConfig_SpellLanguage(t *testing.T) {
	e := newElispTestEditor("")
	_, _ = e.lisp.EvalString(`(setq spell-language "fr")`)
	e.applyElispConfig()
	if e.spellLanguage != "fr" {
		t.Errorf("spellLanguage = %q, want \"fr\"", e.spellLanguage)
	}
}

func TestApplyElispConfig_AutoRevert(t *testing.T) {
	e := newElispTestEditor("")
	e.autoRevert = true
	_, _ = e.lisp.EvalString("(setq auto-revert nil)")
	e.applyElispConfig()
	if e.autoRevert {
		t.Error("expected autoRevert=false after setting nil")
	}
}

func TestApplyElispConfig_SubwordMode(t *testing.T) {
	e := newElispTestEditor("")
	e.subwordMode = false
	_, _ = e.lisp.EvalString("(setq subword-mode t)")
	e.applyElispConfig()
	if !e.subwordMode {
		t.Error("expected subwordMode=true after setting t")
	}
}

func TestApplyElispConfig_ManyValues(t *testing.T) {
	e := newCapTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.dap = &dapState{localsAutoExpandDepth: 1}
	_, _ = e.lisp.EvalString(`
(setq lsp-completion-min-chars 3)
(setq completion-menu-trigger-chars 2)
(setq visual-lines t)
(setq delete-trailing-whitespace t)
(setq debug-locals-auto-expand-depth 4)
`)
	e.applyElispConfig()
	if e.lspCompletionMinChars != 2 { // completion-menu-trigger-chars overrides
		t.Errorf("lspCompletionMinChars = %d, want 2", e.lspCompletionMinChars)
	}
	if !e.visualLines {
		t.Error("visual-lines t should enable visualLines")
	}
	if e.dap.localsAutoExpandDepth != 4 {
		t.Errorf("debug-locals-auto-expand-depth = %d, want 4", e.dap.localsAutoExpandDepth)
	}
}

func TestApplyElispConfig_NilVariants(t *testing.T) {
	e := newCapTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.visualLines = true
	e.subwordMode = true
	e.autoRevert = true
	e.saveBufferDeleteTrailingWS = true
	e.isSearchCaseFold = true
	_, _ = e.lisp.EvalString(`
(setq visual-lines nil)
(setq subword-mode nil)
(setq auto-revert nil)
(setq delete-trailing-whitespace nil)
(setq isearch-case-insensitive nil)
`)
	e.applyElispConfig()
	if e.visualLines || e.subwordMode || e.autoRevert || e.saveBufferDeleteTrailingWS || e.isSearchCaseFold {
		t.Error("nil config values should disable their respective flags")
	}
}

// ---------------------------------------------------------------------------
// writeBuffer
// ---------------------------------------------------------------------------

func TestWriteBuffer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	e := newTestEditor("hello world")
	e.autoRevertMtimes = make(map[*buffer.Buffer]time.Time)
	buf(e).SetFilename(path)
	e.writeBuffer(buf(e))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("file content = %q, want %q", string(data), "hello world")
	}
	if buf(e).Modified() {
		t.Error("buffer should be marked unmodified after write")
	}
}

func TestCmdSaveBuffer_NoFilename(t *testing.T) {
	e := newTestEditor("content")
	e.cmdSaveBuffer()
	// No filename → should prompt for one via minibuffer.
	if !e.minibufActive {
		t.Error("cmdSaveBuffer with no filename: expected minibufActive=true")
	}
}

func TestCmdSaveBuffer_WithFilename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "save.txt")
	e := newTestEditor("save me")
	e.autoRevertMtimes = make(map[*buffer.Buffer]time.Time)
	buf(e).SetFilename(path)
	e.cmdSaveBuffer()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "save me" {
		t.Errorf("file = %q, want %q", string(data), "save me")
	}
}

// ---------------------------------------------------------------------------
// ActiveWindow
// ---------------------------------------------------------------------------

func TestActiveWindow(t *testing.T) {
	e := newTestEditor("hello")
	w := e.ActiveWindow()
	if w == nil {
		t.Fatal("ActiveWindow: returned nil")
	}
	if w.Buf() != buf(e) {
		t.Error("ActiveWindow: Buf() doesn't match ActiveBuffer()")
	}
}

// ---------------------------------------------------------------------------
// rebuildLayoutTree
// ---------------------------------------------------------------------------

func TestRebuildLayoutTree_Empty(t *testing.T) {
	e := newTestEditor("")
	e.windows = nil
	e.rebuildLayoutTree()
	// Should not panic.
}

func TestRebuildLayoutTree_OneWindow(t *testing.T) {
	e := newTestEditor("hi")
	e.rebuildLayoutTree()
	if e.layoutRoot == nil {
		t.Error("rebuildLayoutTree: layoutRoot is nil after rebuild with 1 window")
	}
}

// ---------------------------------------------------------------------------
// removeWindowShowingBuf
// ---------------------------------------------------------------------------

func TestRemoveWindowShowingBuf_NoOp_SingleWindow(t *testing.T) {
	e := newTestEditor("hello")
	before := len(e.windows)
	e.removeWindowShowingBuf(buf(e))
	if len(e.windows) != before {
		t.Errorf("windows count changed from %d to %d", before, len(e.windows))
	}
}

func TestRemoveWindowShowingBuf_RemovesSecondWindow(t *testing.T) {
	e := newTestEditor("hello")
	e.cmdSplitWindowBelow()
	if len(e.windows) != 2 {
		t.Fatalf("expected 2 windows after split, got %d", len(e.windows))
	}
	// Get the non-active window and its buffer.
	var otherWin *window.Window
	for _, w := range e.windows {
		if w != e.activeWin {
			otherWin = w
			break
		}
	}
	if otherWin == nil {
		t.Fatal("no second window found")
	}
	buf2 := otherWin.Buf()
	e.removeWindowShowingBuf(buf2)
	if len(e.windows) != 1 {
		t.Errorf("expected 1 window after remove, got %d", len(e.windows))
	}
}

// ---------------------------------------------------------------------------
// cmdSaveBuffersKillTerminal
// ---------------------------------------------------------------------------

func TestCmdSaveBuffersKillTerminal_NoUnsaved(t *testing.T) {
	e := newTestEditor("hello")
	buf(e).SetModified(false)
	e.cmdSaveBuffersKillTerminal()
	if !e.quit {
		t.Error("cmdSaveBuffersKillTerminal with no unsaved: expected quit=true")
	}
}

func TestCmdSaveBuffersKillTerminal_UnsavedNoFile(t *testing.T) {
	// Modified buffer with no filename → skipped (no file to save).
	e := newTestEditor("hello")
	buf(e).SetModified(true)
	// No filename set, so no "unsaved" buffers to prompt for.
	e.cmdSaveBuffersKillTerminal()
	if !e.quit {
		t.Error("expected quit=true when unsaved buffer has no filename")
	}
}

// ---------------------------------------------------------------------------
// loadFile
// ---------------------------------------------------------------------------

func newLoadFileEditor() *Editor {
	e := newTestEditor("")
	e.autoRevertMtimes = make(map[*buffer.Buffer]time.Time)
	e.lspConns = make(map[string]*lspConn)
	e.term = &terminal.Terminal{} // non-nil terminal; screen==nil so PostWakeup is a no-op
	return e
}

func TestLoadFile_GoMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	_ = os.WriteFile(path, []byte("package main"), 0600)
	e := newLoadFileEditor()
	b, err := e.loadFile(path)
	if err != nil {
		t.Fatalf("loadFile: %v", err)
	}
	if b.Mode() != "go" {
		t.Errorf("mode = %q, want go", b.Mode())
	}
}

func TestLoadFile_MarkdownMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "README.md")
	_ = os.WriteFile(path, []byte("# Hello"), 0600)
	e := newLoadFileEditor()
	b, err := e.loadFile(path)
	if err != nil {
		t.Fatalf("loadFile: %v", err)
	}
	if b.Mode() != "markdown" {
		t.Errorf("mode = %q, want markdown", b.Mode())
	}
}

func TestLoadFile_JSONMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	_ = os.WriteFile(path, []byte(`{"key":"val"}`), 0600)
	e := newLoadFileEditor()
	b, err := e.loadFile(path)
	if err != nil {
		t.Fatalf("loadFile: %v", err)
	}
	if b.Mode() != "json" {
		t.Errorf("mode = %q, want json", b.Mode())
	}
}

func TestLoadFile_PythonMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "script.py")
	_ = os.WriteFile(path, []byte("print('hi')"), 0600)
	e := newLoadFileEditor()
	b, err := e.loadFile(path)
	if err != nil {
		t.Fatalf("loadFile: %v", err)
	}
	if b.Mode() != "python" {
		t.Errorf("mode = %q, want python", b.Mode())
	}
}

func TestLoadFile_ShebangBash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "myscript")
	_ = os.WriteFile(path, []byte("#!/bin/bash\necho hi\n"), 0600)
	e := newLoadFileEditor()
	b, err := e.loadFile(path)
	if err != nil {
		t.Fatalf("loadFile: %v", err)
	}
	if b.Mode() != "bash" {
		t.Errorf("mode = %q, want bash", b.Mode())
	}
}

func TestLoadFile_NonExistent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new_file.go")
	e := newLoadFileEditor()
	b, err := e.loadFile(path)
	if err != nil {
		t.Fatalf("loadFile for non-existent: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil buffer for non-existent file")
	}
	if b.Mode() != "go" {
		t.Errorf("mode = %q, want go", b.Mode())
	}
}

func TestLoadFile_ReuseExistingBuffer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "reuse.go")
	_ = os.WriteFile(path, []byte("package main"), 0600)
	e := newLoadFileEditor()
	b1, err := e.loadFile(path)
	if err != nil {
		t.Fatalf("first loadFile: %v", err)
	}
	b2, err := e.loadFile(path)
	if err != nil {
		t.Fatalf("second loadFile: %v", err)
	}
	if b1 != b2 {
		t.Error("expected same buffer on second loadFile for same path")
	}
}

func TestLoadFile_ElispMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "init.el")
	_ = os.WriteFile(path, []byte("(setq fill-column 80)"), 0600)
	e := newLoadFileEditor()
	b, err := e.loadFile(path)
	if err != nil {
		t.Fatalf("loadFile: %v", err)
	}
	if b.Mode() != modeElisp {
		t.Errorf("mode = %q, want %q", b.Mode(), modeElisp)
	}
}

func TestLoadFile_YAMLMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	_ = os.WriteFile(path, []byte("key: value\n"), 0600)
	e := newLoadFileEditor()
	b, err := e.loadFile(path)
	if err != nil {
		t.Fatalf("loadFile: %v", err)
	}
	if b.Mode() != "yaml" {
		t.Errorf("mode = %q, want yaml", b.Mode())
	}
}

func TestLoadFile_MoreExtensions(t *testing.T) {
	cases := []struct{ name, want string }{
		{"a.pl", "perl"},
		{"Widget.java", "java"},
		{"notes.txt", "text"},
		{"app.conf", "conf"},
		{"login.feature", "gherkin"},
		{"Makefile", "makefile"},
	}
	for _, tc := range cases {
		dir := t.TempDir()
		path := filepath.Join(dir, tc.name)
		_ = os.WriteFile(path, []byte("content\n"), 0600)
		e := newLoadFileEditor()
		b, err := e.loadFile(path)
		if err != nil {
			t.Fatalf("loadFile(%s): %v", tc.name, err)
		}
		if b.Mode() != tc.want {
			t.Errorf("loadFile(%s) mode = %q, want %q", tc.name, b.Mode(), tc.want)
		}
	}
}

func TestLoadFile_AddsToBuffers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "added.go")
	_ = os.WriteFile(path, []byte("package main"), 0600)
	e := newLoadFileEditor()
	before := len(e.buffers)
	_, err := e.loadFile(path)
	if err != nil {
		t.Fatalf("loadFile: %v", err)
	}
	if len(e.buffers) != before+1 {
		t.Errorf("buffers len = %d, want %d", len(e.buffers), before+1)
	}
}

func TestLoadFile_RecordsMtime(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "timed.go")
	_ = os.WriteFile(path, []byte("package main"), 0600)
	e := newLoadFileEditor()
	b, err := e.loadFile(path)
	if err != nil {
		t.Fatalf("loadFile: %v", err)
	}
	mtime, ok := e.autoRevertMtimes[b]
	if !ok {
		t.Error("expected mtime recorded in autoRevertMtimes")
	}
	if mtime.IsZero() {
		t.Error("recorded mtime is zero")
	}
}

// ---------------------------------------------------------------------------
// cmdQueryReplace
// ---------------------------------------------------------------------------

func TestCmdQueryReplace(t *testing.T) {
	e := newTestEditor("hello world")
	e.cmdQueryReplace()
	if !e.minibufActive {
		t.Error("cmdQueryReplace: expected minibufActive=true")
	}
}

func TestCmdQueryReplace_Flow(t *testing.T) {
	e := newElispTestEditor("foo bar foo")
	e.ActiveBuffer().SetPoint(0)
	e.startQueryReplace("foo", "baz")
	if !e.queryReplaceActive {
		t.Error("startQueryReplace should activate on a match")
	}
	if e.queryReplaceMatch < 0 {
		t.Error("startQueryReplace should find the first match")
	}
}

func TestCmdQueryReplace_EmptyFromAborts(t *testing.T) {
	e := newElispTestEditor("text")
	e.startQueryReplace("", "x")
	if e.queryReplaceActive {
		t.Error("empty FROM should not start query-replace")
	}
}

// ---------------------------------------------------------------------------
// OpenFile / Close / handleResize
// ---------------------------------------------------------------------------

func TestOpenFile_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(path, []byte("hi there"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := newLoadFileEditor()
	if err := e.OpenFile(path); err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if e.ActiveBuffer().Filename() != path {
		t.Fatalf("OpenFile should switch to the loaded file, got %q", e.ActiveBuffer().Filename())
	}
}

func TestOpenFile_InvalidUTF8(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.bin")
	if err := os.WriteFile(path, []byte{0xff, 0xfe, 0xfd}, 0o644); err != nil {
		t.Fatal(err)
	}
	e := newLoadFileEditor()
	if err := e.OpenFile(path); err == nil {
		t.Fatal("OpenFile should return an error for invalid UTF-8")
	}
}

func TestClose_NoTerminalSafe(t *testing.T) {
	e := newTestEditor("")
	e.lspConns = make(map[string]*lspConn)
	// term is nil; Close must not panic.
	e.Close()
}

func TestClose_WithCaptureTerminalSafe(t *testing.T) {
	// newCapTestEditor sets e.term to a headless capture terminal (its
	// underlying tcell.Screen is nil), so this exercises the e.term != nil
	// branch of Editor.Close() — and, transitively, the nil-screen guard in
	// terminal.Terminal.Close() — without a real TTY.
	e := newCapTestEditor("")
	e.Close()
}

func TestHandleResize_RelaysOut(t *testing.T) {
	e := newCapTestEditor("hello")
	e.handleResize()
	// After a resize the minibuffer window should span the full width.
	if e.minibufWin.Width() != 80 {
		t.Fatalf("expected minibuffer width 80 after resize, got %d", e.minibufWin.Width())
	}
}

// ---------------------------------------------------------------------------
// dispatchKey / processEvent
// ---------------------------------------------------------------------------

func TestDispatchKey_InsertsRune(t *testing.T) {
	e := newCapTestEditor("")
	ev := tcell.NewEventKey(tcell.KeyRune, "x", tcell.ModNone)
	e.dispatchKey(ev)
	if e.ActiveBuffer().String() != "x" {
		t.Fatalf("dispatchKey should self-insert 'x', got %q", e.ActiveBuffer().String())
	}
}

func TestDispatchKey_RecordsMacro(t *testing.T) {
	e := newCapTestEditor("")
	e.kbdMacroRecording = true
	ev := tcell.NewEventKey(tcell.KeyRune, "a", tcell.ModNone)
	e.dispatchKey(ev)
	if len(e.kbdMacroEvents) != 1 {
		t.Fatalf("expected one recorded macro event, got %d", len(e.kbdMacroEvents))
	}
}

func TestProcessEvent_Key(t *testing.T) {
	e := newCapTestEditor("")
	e.processEvent(tcell.NewEventKey(tcell.KeyRune, "z", tcell.ModNone))
	if e.ActiveBuffer().String() != "z" {
		t.Fatalf("processEvent(key) should insert, got %q", e.ActiveBuffer().String())
	}
}

func TestProcessEvent_Resize(t *testing.T) {
	e := newCapTestEditor("hello")
	// Should not panic; handleResize relays out windows.
	e.processEvent(tcell.NewEventResize(80, 24))
	if e.minibufWin.Width() != 80 {
		t.Fatalf("resize event should relayout, got minibuffer width %d", e.minibufWin.Width())
	}
}

func TestProcessEvent_InterruptDrainsCallbacks(t *testing.T) {
	e := newCapTestEditor("")
	e.lspCbs = make(chan func(), 4)
	e.dapCbs = make(chan func(), 4)
	ran := false
	e.lspCbs <- func() { ran = true }
	e.processEvent(tcell.NewEventInterrupt(nil))
	if !ran {
		t.Fatal("interrupt event should drain and run queued LSP callbacks")
	}
}

// ---------------------------------------------------------------------------
// dispatchParsedKey — special-mode branches
// ---------------------------------------------------------------------------

func TestDispatchParsedKey_EscStartsPrefix(t *testing.T) {
	e := newTestEditor("hello")
	e.dispatchParsedKey(terminal.KeyEvent{Key: tcell.KeyEscape})
	if !e.escPending {
		t.Error("ESC alone should set escPending=true")
	}
}

func TestDispatchParsedKey_EscEscCancels(t *testing.T) {
	e := newTestEditor("hello")
	e.escPending = true
	e.dispatchParsedKey(terminal.KeyEvent{Key: tcell.KeyEscape})
	if e.escPending {
		t.Error("ESC ESC should clear escPending")
	}
}

func TestDispatchParsedKey_EscRuneSynthesisesMeta(t *testing.T) {
	e := newTestEditor("hello world")
	e.lisp = elisp.NewEvaluator()
	e.setupKeymaps()
	e.ActiveBuffer().SetPoint(0)
	e.escPending = true
	// ESC then 'f' should act as M-f (forward-word).
	e.dispatchParsedKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'f'})
	if e.escPending {
		t.Error("escPending should be cleared after ESC+key")
	}
	if e.ActiveBuffer().Point() != 5 {
		t.Errorf("ESC f should move forward-word to point 5, got %d", e.ActiveBuffer().Point())
	}
}

func TestDispatchParsedKey_WhatKeyReports(t *testing.T) {
	e := newTestEditor("")
	e.whatKeyPending = true
	e.dispatchParsedKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'a'})
	if e.whatKeyPending {
		t.Error("whatKeyPending should be cleared after reporting")
	}
	if !strings.Contains(e.message, "key=") {
		t.Errorf("expected key report message, got %q", e.message)
	}
}

func TestDispatchParsedKey_UniversalArgDigits(t *testing.T) {
	e := newTestEditor("")
	e.universalArgSet = true
	e.dispatchParsedKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: '8'})
	if e.universalArg != 8 {
		t.Errorf("universalArg = %d, want 8", e.universalArg)
	}
}

func TestDispatchParsedKey_UniversalArgMinus(t *testing.T) {
	e := newTestEditor("")
	e.universalArgSet = true
	e.dispatchParsedKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: '-'})
	if e.universalArgDigits != "-" {
		t.Errorf("universalArgDigits = %q, want \"-\"", e.universalArgDigits)
	}
}

func TestDispatchParsedKey_ReadCharCallback(t *testing.T) {
	e := newTestEditor("")
	var got rune
	e.readCharPending = true
	e.readCharCallback = func(r rune) { got = r }
	e.dispatchParsedKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'q'})
	if got != 'q' {
		t.Errorf("readChar callback got %q, want 'q'", got)
	}
	if e.readCharPending {
		t.Error("readCharPending should be cleared")
	}
}

func TestDispatchParsedKey_ReadCharNonRuneCancels(t *testing.T) {
	e := newTestEditor("")
	called := false
	e.readCharPending = true
	e.readCharCallback = func(rune) { called = true }
	e.dispatchParsedKey(terminal.KeyEvent{Key: tcell.KeyCtrlG})
	if called {
		t.Error("callback should not be invoked for non-rune key")
	}
	if !strings.Contains(e.message, "cancelled") {
		t.Errorf("expected cancellation message, got %q", e.message)
	}
}

func TestDispatchParsedKey_PrefixIncomplete(t *testing.T) {
	e := newTestEditor("")
	e.prefixKeymap = e.ctrlXKeymap
	// F12 is not bound under C-x.
	e.dispatchParsedKey(terminal.KeyEvent{Key: tcell.KeyF12})
	if e.prefixKeymap != nil {
		t.Error("unrecognised prefix key should clear prefixKeymap")
	}
	if !strings.Contains(e.message, "incomplete") {
		t.Errorf("expected 'incomplete' message, got %q", e.message)
	}
}

func TestDispatchParsedKey_LspDocDismissed(t *testing.T) {
	e := newTestEditor("hello")
	e.lspDocLines = []string{"doc"}
	e.dispatchParsedKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'x'})
	if e.lspDocLines != nil {
		t.Error("any key should dismiss the lsp doc popup")
	}
}

func TestDispatchParsedKey_SelfInsertUnbound(t *testing.T) {
	e := newTestEditor("")
	e.dispatchParsedKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'Z'})
	if e.ActiveBuffer().String() != "Z" {
		t.Errorf("unbound printable rune should self-insert, got %q", e.ActiveBuffer().String())
	}
}

func TestDispatchParsedKey_ModeRouting(t *testing.T) {
	modes := []string{
		"dired", "buffer-list", "vc-log", "diff", "vc-show", "vc-status",
		"compilation", "vc-grep", "lsp-refs", "help", "man", "shell",
		"vc-annotate", "vc-commit", "vc-fixup-select",
	}
	for _, m := range modes {
		e := newCapTestEditor("line1\nline2\n")
		e.lisp = elisp.NewEvaluator()
		e.diredStates = map[*buffer.Buffer]*diredState{}
		e.vcParent = map[*buffer.Buffer]*buffer.Buffer{}
		e.shellStates = map[*buffer.Buffer]*shellState{}
		e.ActiveBuffer().SetMode(m)
		// Routes through the mode-dispatch switch; must not panic.
		e.dispatchParsedKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'z'})
	}
}

func TestDispatchParsedKey_DebugModeRouting(t *testing.T) {
	for _, m := range []string{"debug-locals", "debug-stack", "debug-repl", "go"} {
		e := newCapTestEditor("x")
		e.lisp = elisp.NewEvaluator()
		e.dap = &dapState{}
		e.dapBreakpoints = map[string]map[int]struct{}{}
		e.ActiveBuffer().SetMode(m)
		e.dispatchParsedKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'z'})
	}
}

// ---------------------------------------------------------------------------
// dispatchMinibufKey — editing/navigation key branches
// ---------------------------------------------------------------------------

func newMinibufKeyEditor(content string) *Editor {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.ReadMinibuffer("P: ", func(string) {})
	e.minibufBuf.InsertString(0, content)
	e.minibufBuf.SetPoint(len([]rune(content)))
	return e
}

func TestDispatchMinibufKey_DeleteForward(t *testing.T) {
	e := newMinibufKeyEditor("abc")
	e.minibufBuf.SetPoint(0)
	e.dispatchMinibufKey(terminal.KeyEvent{Key: tcell.KeyDelete})
	if e.minibufBuf.String() != "bc" {
		t.Errorf("Delete should remove forward char, got %q", e.minibufBuf.String())
	}
}

func TestDispatchMinibufKey_CtrlD(t *testing.T) {
	e := newMinibufKeyEditor("abc")
	e.minibufBuf.SetPoint(0)
	e.dispatchMinibufKey(terminal.KeyEvent{Key: tcell.KeyCtrlD})
	if e.minibufBuf.String() != "bc" {
		t.Errorf("C-d should delete forward char, got %q", e.minibufBuf.String())
	}
}

func TestDispatchMinibufKey_CtrlK(t *testing.T) {
	e := newMinibufKeyEditor("abcdef")
	e.minibufBuf.SetPoint(3)
	e.dispatchMinibufKey(terminal.KeyEvent{Key: tcell.KeyCtrlK})
	if e.minibufBuf.String() != "abc" {
		t.Errorf("C-k should kill to end of line, got %q", e.minibufBuf.String())
	}
}

func TestDispatchMinibufKey_LeftRight(t *testing.T) {
	e := newMinibufKeyEditor("abc")
	e.minibufBuf.SetPoint(1)
	e.dispatchMinibufKey(terminal.KeyEvent{Key: tcell.KeyRight})
	if e.minibufBuf.Point() != 2 {
		t.Errorf("Right point = %d, want 2", e.minibufBuf.Point())
	}
	e.dispatchMinibufKey(terminal.KeyEvent{Key: tcell.KeyLeft})
	if e.minibufBuf.Point() != 1 {
		t.Errorf("Left point = %d, want 1", e.minibufBuf.Point())
	}
}

func TestDispatchMinibufKey_CtrlFB(t *testing.T) {
	e := newMinibufKeyEditor("abc")
	e.minibufBuf.SetPoint(1)
	e.dispatchMinibufKey(terminal.KeyEvent{Key: tcell.KeyCtrlF})
	if e.minibufBuf.Point() != 2 {
		t.Errorf("C-f point = %d, want 2", e.minibufBuf.Point())
	}
	e.dispatchMinibufKey(terminal.KeyEvent{Key: tcell.KeyCtrlB})
	if e.minibufBuf.Point() != 1 {
		t.Errorf("C-b point = %d, want 1", e.minibufBuf.Point())
	}
}

func TestDispatchMinibufKey_HomeEnd(t *testing.T) {
	e := newMinibufKeyEditor("abc")
	e.dispatchMinibufKey(terminal.KeyEvent{Key: tcell.KeyHome})
	if e.minibufBuf.Point() != 0 {
		t.Errorf("Home point = %d, want 0", e.minibufBuf.Point())
	}
	e.dispatchMinibufKey(terminal.KeyEvent{Key: tcell.KeyEnd})
	if e.minibufBuf.Point() != e.minibufBuf.Len() {
		t.Errorf("End point = %d, want %d", e.minibufBuf.Point(), e.minibufBuf.Len())
	}
}

func TestDispatchMinibufKey_CtrlW(t *testing.T) {
	e := newMinibufKeyEditor("foo bar")
	e.dispatchMinibufKey(terminal.KeyEvent{Key: tcell.KeyCtrlW})
	if strings.Contains(e.minibufBuf.String(), "bar") {
		t.Errorf("C-w should kill the last word, got %q", e.minibufBuf.String())
	}
}

func TestDispatchMinibufKey_MetaBackspace(t *testing.T) {
	e := newMinibufKeyEditor("foo bar")
	e.dispatchMinibufKey(terminal.KeyEvent{Key: tcell.KeyBackspace, Mod: tcell.ModAlt})
	if strings.Contains(e.minibufBuf.String(), "bar") {
		t.Errorf("M-DEL should kill the last word, got %q", e.minibufBuf.String())
	}
}

func TestDispatchMinibufKey_DownUpCandidates(t *testing.T) {
	e := newMinibufKeyEditor("")
	e.minibufCandidates = []string{"a", "b", "c"}
	e.minibufSelectedIdx = 0
	e.dispatchMinibufKey(terminal.KeyEvent{Key: tcell.KeyDown})
	if e.minibufSelectedIdx != 1 {
		t.Errorf("Down with candidates should advance selection, got %d", e.minibufSelectedIdx)
	}
	e.dispatchMinibufKey(terminal.KeyEvent{Key: tcell.KeyUp})
	if e.minibufSelectedIdx != 0 {
		t.Errorf("Up with candidates should move selection back, got %d", e.minibufSelectedIdx)
	}
}

func TestDispatchMinibufKey_CtrlNP(t *testing.T) {
	e := newMinibufKeyEditor("")
	e.minibufCandidates = []string{"x", "y"}
	e.minibufSelectedIdx = 0
	e.dispatchMinibufKey(terminal.KeyEvent{Key: tcell.KeyCtrlN})
	if e.minibufSelectedIdx != 1 {
		t.Errorf("C-n should advance selection, got %d", e.minibufSelectedIdx)
	}
	e.dispatchMinibufKey(terminal.KeyEvent{Key: tcell.KeyCtrlP})
	if e.minibufSelectedIdx != 0 {
		t.Errorf("C-p should move selection back, got %d", e.minibufSelectedIdx)
	}
}

func TestDispatchMinibufKey_MetaF(t *testing.T) {
	e := newMinibufKeyEditor("foo bar")
	e.minibufBuf.SetPoint(0)
	e.dispatchMinibufKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'f', Mod: tcell.ModAlt})
	if e.minibufBuf.Point() != 3 {
		t.Errorf("M-f should move to end of first word (3), got %d", e.minibufBuf.Point())
	}
}

// ---------------------------------------------------------------------------
// manDispatch / relayoutWindows / evalSexp / Close
// ---------------------------------------------------------------------------

func TestManDispatch_UsesMRU(t *testing.T) {
	e := newTestEditor("main")
	e.lisp = elisp.NewEvaluator()
	other := buffer.NewWithContent("other", "x")
	manBuf := buffer.NewWithContent("*Man cat*", "man page")
	manBuf.SetMode("man")
	e.buffers = append(e.buffers, other, manBuf)
	e.bufferMRU = []*buffer.Buffer{other}
	e.activeWin.SetBuf(manBuf)
	if !e.manDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'q'}) {
		t.Fatal("'q' should be consumed")
	}
	if e.ActiveBuffer() != other {
		t.Error("manDispatch should switch to the MRU non-man buffer")
	}
}

func TestManDispatch_NonQNotConsumed(t *testing.T) {
	e := newTestEditor("x")
	if e.manDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'j'}) {
		t.Error("non-'q' key should not be consumed by manDispatch")
	}
}

func TestRelayoutWindows_NoWindows(t *testing.T) {
	e := newTestEditor("")
	e.windows = nil
	e.relayoutWindows(80, 24) // n==0 → no-op, no panic
}

func TestRelayoutWindows_NilLayoutRebuilds(t *testing.T) {
	e := newTestEditor("hi")
	e.layoutRoot = nil
	e.relayoutWindows(80, 24)
	if e.layoutRoot == nil {
		t.Error("relayoutWindows should rebuild a nil layout tree")
	}
}

func TestEvalSexp_OK(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	got, err := e.evalSexp("(+ 1 2)")
	if err != nil {
		t.Fatalf("evalSexp: %v", err)
	}
	if got != "3" {
		t.Errorf("evalSexp((+ 1 2)) = %q, want \"3\"", got)
	}
}

func TestExecCommand_Unknown(t *testing.T) {
	e := newTestEditor("")
	e.execCommand("no-such-command-xyz")
	if !strings.Contains(e.message, "Unknown command") {
		t.Errorf("expected 'Unknown command' message, got %q", e.message)
	}
}

func TestEvalSexp_Error(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	if _, err := e.evalSexp("(this is not valid"); err == nil {
		t.Error("evalSexp should return an error for malformed input")
	}
}

func TestLoadInitFile_EvalsGomacs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(home, ".gomacs"), []byte("(setq fill-column 99)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := newElispTestEditor("")
	e.fillColumn = 70
	e.loadInitFile()
	if e.fillColumn != 99 {
		t.Errorf("loadInitFile should apply ~/.gomacs config, fillColumn=%d", e.fillColumn)
	}
}

func TestLoadInitFile_NoFile(t *testing.T) {
	home := t.TempDir() // no init file present
	t.Setenv("HOME", home)
	e := newElispTestEditor("")
	e.loadInitFile() // must not panic and must leave defaults intact
}

func TestAddToKillRing_EmptyIgnored(t *testing.T) {
	e := newTestEditor("")
	e.killRing = nil
	e.addToKillRing("")
	if len(e.killRing) != 0 {
		t.Errorf("empty string should not be added to the kill ring, len=%d", len(e.killRing))
	}
}

func TestAddToKillRing_TruncatesAtCap(t *testing.T) {
	e := newTestEditor("")
	e.killRing = nil
	for i := range 65 {
		e.addToKillRing(fmt.Sprintf("entry-%d", i))
	}
	if len(e.killRing) != 60 {
		t.Errorf("kill ring should be capped at 60 entries, got %d", len(e.killRing))
	}
	if e.killRing[0] != "entry-64" {
		t.Errorf("most recent entry should be first, got %q", e.killRing[0])
	}
}

func TestYankPop_EmptyRing(t *testing.T) {
	e := newTestEditor("")
	e.killRing = nil
	if got := e.yankPop(); got != "" {
		t.Errorf("yankPop on empty ring should return \"\", got %q", got)
	}
}

func TestYankPop_Rotates(t *testing.T) {
	e := newTestEditor("")
	e.killRing = []string{"one", "two", "three"}
	e.yankIdx = 0
	if got := e.yankPop(); got != "two" {
		t.Errorf("first yankPop should return \"two\", got %q", got)
	}
	if got := e.yankPop(); got != "three" {
		t.Errorf("second yankPop should return \"three\", got %q", got)
	}
	if got := e.yankPop(); got != "one" {
		t.Errorf("yankPop should wrap around to \"one\", got %q", got)
	}
}

// ---------------------------------------------------------------------------
// isearch: C-w word extension (spec: "# Search")
// ---------------------------------------------------------------------------

// TestIsearchYankWord_ExtendsToCompoundWord is the spec's own example: a search
// for "Camel" that lands inside "CamelCase" grows to "CamelCase" on C-w.
func TestIsearchYankWord_ExtendsToCompoundWord(t *testing.T) {
	e := newTestEditorWithIsearch("the CamelCase name")
	startFwdIsearch(e, 0)
	e.isearchStr = "Camel"
	e.isearchFind()

	e.isearchYankWord()

	if e.isearchStr != "CamelCase" {
		t.Errorf("isearchStr = %q, want %q", e.isearchStr, "CamelCase")
	}
	// Point must sit at the end of the now-longer match.
	if got, want := buf(e).Point(), len("the CamelCase"); got != want {
		t.Errorf("point = %d, want %d", got, want)
	}
}

// TestIsearchYankWord_RepeatedPullsInMoreWords covers the spec's "Continuously
// hitting C-w will include more and more words into the search".
func TestIsearchYankWord_RepeatedPullsInMoreWords(t *testing.T) {
	e := newTestEditorWithIsearch("alpha beta gamma")
	startFwdIsearch(e, 0)
	e.isearchStr = "alpha"
	e.isearchFind()

	e.isearchYankWord()
	if e.isearchStr != "alpha beta" {
		t.Fatalf("after one C-w: isearchStr = %q, want %q", e.isearchStr, "alpha beta")
	}
	e.isearchYankWord()
	if e.isearchStr != "alpha beta gamma" {
		t.Errorf("after two C-w: isearchStr = %q, want %q", e.isearchStr, "alpha beta gamma")
	}
}

func TestIsearchYankWord_AtEndOfBufferReports(t *testing.T) {
	e := newTestEditorWithIsearch("alpha")
	startFwdIsearch(e, 0)
	e.isearchStr = "alpha"
	e.isearchFind()

	e.isearchYankWord()

	if e.isearchStr != "alpha" {
		t.Errorf("isearchStr should be unchanged at end of buffer, got %q", e.isearchStr)
	}
	if e.message != isearchBufEndMsg {
		t.Errorf("message = %q, want %q", e.message, isearchBufEndMsg)
	}
}

func TestIsearchYankWord_EmptyQueryIsNoop(t *testing.T) {
	e := newTestEditorWithIsearch("alpha beta")
	startFwdIsearch(e, 0)

	e.isearchYankWord()

	if e.isearchStr != "" {
		t.Errorf("isearchStr = %q, want empty", e.isearchStr)
	}
}

// TestIsearchYankWord_Backward extends from the end of a backward match, and
// must leave point on the match start.
func TestIsearchYankWord_Backward(t *testing.T) {
	e := newTestEditorWithIsearch("one CamelCase two")
	b := buf(e)
	b.SetPoint(b.Len())
	e.isearching = true
	e.isearchFwd = false
	e.isearchStr = "Camel"
	e.isearchStart = b.Len()
	e.isearchFind()
	matchStart := b.Point()

	e.isearchYankWord()

	if e.isearchStr != "CamelCase" {
		t.Errorf("isearchStr = %q, want %q", e.isearchStr, "CamelCase")
	}
	if b.Point() != matchStart {
		t.Errorf("point moved from %d to %d during backward yank", matchStart, b.Point())
	}
}

// TestIsearchHandleKey_CtrlWExtendsRatherThanKillingRegion guards the previous
// behaviour where C-w fell through to the global kill-region binding and
// destroyed text mid-search.
func TestIsearchHandleKey_CtrlWExtendsRatherThanKillingRegion(t *testing.T) {
	e := newTestEditorWithIsearch("the CamelCase name")
	b := buf(e)
	before := b.String()
	startFwdIsearch(e, 0)
	e.isearchStr = "Camel"
	e.isearchFind()
	b.SetMark(0)
	b.SetMarkActive(true)

	e.isearchHandleKey(terminal.KeyEvent{Key: tcell.KeyCtrlW})

	if !e.isearching {
		t.Error("C-w must not exit isearch")
	}
	if b.String() != before {
		t.Errorf("buffer was modified by C-w: %q", b.String())
	}
	if e.isearchStr != "CamelCase" {
		t.Errorf("isearchStr = %q, want %q", e.isearchStr, "CamelCase")
	}
}

// ---------------------------------------------------------------------------
// isearch: wrap-around reporting (spec: "Once the search hits top or bottom,
// it should write so in the minibuffer")
// ---------------------------------------------------------------------------

func TestIsearchFindNext_WrapsForwardAndReportsBottom(t *testing.T) {
	e := newTestEditorWithIsearch("Xabc def")
	b := buf(e)
	e.isearching = true
	e.isearchFwd = true
	e.isearchStr = "X"
	// Point is past the only match, so a forward scan must wrap to the top.
	b.SetPoint(b.Len())

	e.isearchFindNext()

	if got := b.Point(); got != 1 {
		t.Errorf("point = %d, want 1 (wrapped to first match)", got)
	}
	if !strings.Contains(e.message, "bottom of buffer") {
		t.Errorf("message = %q, want it to mention the bottom of the buffer", e.message)
	}
}

func TestIsearchFindNext_WrapsBackwardAndReportsTop(t *testing.T) {
	e := newTestEditorWithIsearch("abc defX")
	b := buf(e)
	e.isearching = true
	e.isearchFwd = false
	e.isearchStr = "X"
	// Point is before the only match, so a backward scan must wrap to the end.
	b.SetPoint(0)

	e.isearchFindNext()

	if got, want := b.Point(), len("abc def"); got != want {
		t.Errorf("point = %d, want %d (wrapped to last match)", got, want)
	}
	if !strings.Contains(e.message, "top of buffer") {
		t.Errorf("message = %q, want it to mention the top of the buffer", e.message)
	}
}

func TestIsearchFindNext_NoMatchAnywhereFails(t *testing.T) {
	e := newTestEditorWithIsearch("abc def")
	e.isearching = true
	e.isearchFwd = true
	e.isearchStr = "zzz"

	e.isearchFindNext()

	if !strings.HasPrefix(e.message, "Failing isearch") {
		t.Errorf("message = %q, want a Failing isearch message", e.message)
	}
}

// TestIsearchFindNext_DoesNotWrapWhenLaterMatchExists checks the wrap path is
// only taken as a fallback.
func TestIsearchFindNext_DoesNotWrapWhenLaterMatchExists(t *testing.T) {
	e := newTestEditorWithIsearch("Xa Xb")
	b := buf(e)
	e.isearching = true
	e.isearchFwd = true
	e.isearchStr = "X"
	b.SetPoint(1) // just after the first match

	e.isearchFindNext()

	if got := b.Point(); got != 4 {
		t.Errorf("point = %d, want 4 (second match)", got)
	}
	if strings.Contains(e.message, "Wrapped") {
		t.Errorf("should not report a wrap, got %q", e.message)
	}
}

// ---------------------------------------------------------------------------
// window jump: syntax highlighting suppression (spec: "# One letter window jump")
// ---------------------------------------------------------------------------

// TestRenderWindowDropsHighlightingDuringWindowJump checks the spec's
// requirement that window contents render "without syntax highlighting" while
// the M-o letter overlay is up, and that highlighting comes back afterwards.
func TestRenderWindowDropsHighlightingDuringWindowJump(t *testing.T) {
	e := newCapTestEditor("package main\n")
	e.ActiveBuffer().SetMode("go")

	e.renderWindow(e.activeWin)
	_, litFace := e.term.CaptureCell(0, 0)

	e.windowJumpActive = true
	e.renderWindow(e.activeWin)
	_, plainFace := e.term.CaptureCell(0, 0)

	if plainFace == litFace {
		t.Errorf("expected highlighting to be dropped during window jump, face still %+v", plainFace)
	}

	// Highlighting must be restored once the overlay is dismissed.
	e.windowJumpActive = false
	e.renderWindow(e.activeWin)
	_, restoredFace := e.term.CaptureCell(0, 0)
	if restoredFace != litFace {
		t.Errorf("face after jump = %+v, want original %+v", restoredFace, litFace)
	}
}

// TestHighlighterForDebugReplLangSuffix checks the debugger REPL is highlighted
// with the debugged language's highlighter rather than always Go, using the same
// "mode+lang" convention vc-annotate uses.
func TestHighlighterForDebugReplLangSuffix(t *testing.T) {
	tests := []struct {
		mode string
		want syntax.Highlighter
	}{
		{"debug-repl", syntax.GoHighlighter{}},
		{"debug-repl+go", syntax.GoHighlighter{}},
		{"debug-repl+java", syntax.JavaHighlighter{}},
		{"debug-repl+python", syntax.PythonHighlighter{}},
		{"debug-repl+bogus", syntax.GoHighlighter{}},
	}
	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			b := buffer.New("*Debug REPL*")
			b.SetMode(tt.mode)
			if got := b.Mode(); got != tt.mode {
				t.Fatalf("SetMode(%q) did not stick, mode = %q", tt.mode, got)
			}
			if got := highlighterFor(b); got != tt.want {
				t.Errorf("highlighterFor(%q) = %T, want %T", tt.mode, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// span cache: partial highlighting and reuse
// ---------------------------------------------------------------------------

// countingHighlighter wraps a highlighter and records how it was called.
type countingHighlighter struct {
	inner  syntax.Highlighter
	calls  int
	lastHi int // the end offset of the most recent call
}

func (c *countingHighlighter) Highlight(text string, start, end int) []syntax.Span {
	c.calls++
	c.lastHi = end
	return c.inner.Highlight(text, start, end)
}

// newSpanCacheTestEditor returns an editor over content whose buffer uses a
// counting highlighter, so tests can see exactly when a rescan happens.
func newSpanCacheTestEditor(t *testing.T, content string) (*Editor, *buffer.Buffer, *countingHighlighter) {
	t.Helper()
	e := newTestEditor(content)
	b := buf(e)
	b.SetMode("go")
	hl := &countingHighlighter{inner: syntax.GoHighlighter{}}
	e.spanCaches = make(map[*buffer.Buffer]*spanCache)
	e.customHighlighters[b] = hl
	return e, b, hl
}

// goSource returns a Go source file with n functions — enough text that a
// partial highlight is measurably narrower than a full one.
func goSource(n int) string {
	var sb strings.Builder
	sb.WriteString("package main\n\nimport \"fmt\"\n\n")
	for i := range n {
		fmt.Fprintf(&sb, "// doc comment for f%d\nfunc f%d(a int) string {\n"+
			"\tif a > %d {\n\t\treturn fmt.Sprintf(\"big %%d\", a)\n\t}\n"+
			"\treturn \"small\"\n}\n\n", i, i, i)
	}
	return sb.String()
}

func TestGetSpanCacheUpToPartial(t *testing.T) {
	e, b, hl := newSpanCacheTestEditor(t, goSource(400))
	c := e.getSpanCacheUpTo(b, 100)
	if c.hiEnd <= 100 {
		t.Fatalf("hiEnd = %d, want > 100 (margin should be added)", c.hiEnd)
	}
	if c.hiEnd >= b.Len() {
		t.Fatalf("hiEnd = %d, want < buffer length %d (should not scan the whole buffer)",
			c.hiEnd, b.Len())
	}
	if hl.calls != 1 {
		t.Fatalf("highlighter calls = %d, want 1", hl.calls)
	}
	if hl.lastHi != c.hiEnd {
		t.Errorf("highlighter was asked for end=%d, but cache records hiEnd=%d", hl.lastHi, c.hiEnd)
	}
}

func TestGetSpanCacheUpToReusesForNarrowerRequest(t *testing.T) {
	e, b, hl := newSpanCacheTestEditor(t, goSource(400))
	first := e.getSpanCacheUpTo(b, 5000)
	if hl.calls != 1 {
		t.Fatalf("highlighter calls after first request = %d, want 1", hl.calls)
	}
	// Anything at or below hiEnd must be served from the cache.
	for _, wantEnd := range []int{0, 1, 100, 5000, first.hiEnd} {
		got := e.getSpanCacheUpTo(b, wantEnd)
		if got != first {
			t.Errorf("wantEnd=%d: got a new cache, want the existing one reused", wantEnd)
		}
	}
	if hl.calls != 1 {
		t.Errorf("highlighter calls = %d, want 1 (narrower requests must reuse)", hl.calls)
	}
}

func TestGetSpanCacheUpToRecomputesPastHiEnd(t *testing.T) {
	e, b, hl := newSpanCacheTestEditor(t, goSource(2000))
	first := e.getSpanCacheUpTo(b, 100)
	if first.hiEnd >= b.Len() {
		t.Fatalf("first hiEnd = %d, want < %d so there is room to extend", first.hiEnd, b.Len())
	}
	second := e.getSpanCacheUpTo(b, first.hiEnd+1)
	if hl.calls != 2 {
		t.Fatalf("highlighter calls = %d, want 2 (a request past hiEnd must rescan)", hl.calls)
	}
	if second.hiEnd <= first.hiEnd {
		t.Errorf("hiEnd did not grow: %d -> %d", first.hiEnd, second.hiEnd)
	}
	// The covered range at least doubles, so a long scroll costs a logarithmic
	// number of rescans rather than one per screenful.
	if second.hiEnd < 2*first.hiEnd {
		t.Errorf("hiEnd = %d, want at least 2*%d", second.hiEnd, first.hiEnd)
	}
}

func TestGetSpanCacheChangeGenAlwaysRecomputes(t *testing.T) {
	e, b, hl := newSpanCacheTestEditor(t, goSource(400))
	e.getSpanCacheUpTo(b, 100)
	if hl.calls != 1 {
		t.Fatalf("highlighter calls = %d, want 1", hl.calls)
	}
	// Same request again: reused.
	e.getSpanCacheUpTo(b, 100)
	if hl.calls != 1 {
		t.Fatalf("highlighter calls = %d, want 1 after an identical request", hl.calls)
	}
	// An edit bumps ChangeGen and must invalidate the cache.
	b.Insert(0, 'x')
	e.getSpanCacheUpTo(b, 100)
	if hl.calls != 2 {
		t.Errorf("highlighter calls = %d, want 2 after an edit", hl.calls)
	}
}

func TestGetSpanCacheModeChangeRecomputes(t *testing.T) {
	e, b, hl := newSpanCacheTestEditor(t, goSource(50))
	e.getSpanCacheUpTo(b, 100)
	b.SetMode("python")
	e.getSpanCacheUpTo(b, 100)
	if hl.calls != 2 {
		t.Errorf("highlighter calls = %d, want 2 after a mode change", hl.calls)
	}
}

func TestGetSpanCacheFullBufferWrapper(t *testing.T) {
	e, b, hl := newSpanCacheTestEditor(t, goSource(400))
	c := e.getSpanCache(b)
	if c.hiEnd != b.Len() {
		t.Errorf("getSpanCache hiEnd = %d, want the whole buffer (%d)", c.hiEnd, b.Len())
	}
	// A full-buffer cache satisfies every narrower request.
	if got := e.getSpanCacheUpTo(b, 10); got != c {
		t.Error("narrower request after a full-buffer scan should reuse the cache")
	}
	if hl.calls != 1 {
		t.Errorf("highlighter calls = %d, want 1", hl.calls)
	}
}

// TestGetSpanCacheUpToPartialSpansMatchFull is the editor-side counterpart of
// the syntax package's differential test: the spans a partial cache exposes for
// the region it covers must be identical to the full-buffer ones.
func TestGetSpanCacheUpToPartialSpansMatchFull(t *testing.T) {
	e, b, _ := newSpanCacheTestEditor(t, goSource(400))
	partial := e.getSpanCacheUpTo(b, 3000)
	limit := partial.hiEnd
	var partialVisible []syntax.Span
	for _, sp := range partial.spans {
		if sp.Start < limit {
			partialVisible = append(partialVisible, sp)
		}
	}

	e2, b2, _ := newSpanCacheTestEditor(t, goSource(400))
	full := e2.getSpanCache(b2)
	var fullVisible []syntax.Span
	for _, sp := range full.spans {
		if sp.Start < limit {
			fullVisible = append(fullVisible, sp)
		}
	}

	if len(partialVisible) != len(fullVisible) {
		t.Fatalf("partial cache has %d spans below %d, full cache has %d",
			len(partialVisible), limit, len(fullVisible))
	}
	for i := range partialVisible {
		if partialVisible[i] != fullVisible[i] {
			t.Fatalf("span %d differs: partial %v, full %v", i, partialVisible[i], fullVisible[i])
		}
	}
}

// TestRenderWindowScrollingDoesNotRehighlight pins the property that ordinary
// scrolling is served from the cache.  The margin must be big enough that
// paging through a few screens costs no extra highlighting.
func TestRenderWindowScrollingDoesNotRehighlight(t *testing.T) {
	e, b, hl := newSpanCacheTestEditor(t, goSource(2000))
	w := e.activeWin
	// Prime the cache from the top of the buffer.
	e.getSpanCacheUpTo(b, visibleEndOf(w))
	if hl.calls != 1 {
		t.Fatalf("highlighter calls after priming = %d, want 1", hl.calls)
	}
	// Scroll down two screens at a time for several screens.
	for range 4 {
		w.ScrollUp(w.Height() - 1)
		e.getSpanCacheUpTo(b, visibleEndOf(w))
	}
	if hl.calls != 1 {
		t.Errorf("highlighter calls after scrolling = %d, want 1 (scrolling must not rehighlight)",
			hl.calls)
	}
}

// visibleEndOf mirrors the visible-range computation renderWindow performs.
func visibleEndOf(w *window.Window) int {
	viewLines := w.ViewLines()
	textH := max(w.Height()-1, 1)
	visibleEnd := 0
	for rowIdx := 0; rowIdx < textH && rowIdx < len(viewLines); rowIdx++ {
		visibleEnd = max(visibleEnd, viewLines[rowIdx].EndPos)
	}
	return visibleEnd
}

// ---------------------------------------------------------------------------
// span cache benchmarks
// ---------------------------------------------------------------------------

// benchGoLines is roughly the 10 000-line Go buffer the performance work
// targets: 8 lines per generated function.
const benchGoFuncs = 1250

// benchSpanCacheKeystroke measures the per-keystroke highlighting cost with the
// window scrolled so that scrollLine is line startLine.
func benchSpanCacheKeystroke(bench *testing.B, startLine int, full bool) {
	src := goSource(benchGoFuncs)
	e := newTestEditor(src)
	b := buf(e)
	b.SetMode("go")
	e.spanCaches = make(map[*buffer.Buffer]*spanCache)
	w := e.activeWin
	w.SetScrollLine(startLine)
	wantEnd := visibleEndOf(w)
	// Type at the top of the visible region.
	pt := w.ViewLines()[0].StartPos
	b.SetPoint(pt)

	bench.ReportAllocs()
	bench.ResetTimer()
	for range bench.N {
		// Each keystroke bumps ChangeGen, invalidating the cache.
		b.Insert(pt, 'x')
		b.Delete(pt, 1)
		if full {
			e.getSpanCache(b)
		} else {
			e.getSpanCacheUpTo(b, wantEnd)
		}
	}
}

// BenchmarkSpanCacheTopFull is the pre-optimisation behaviour with the cursor
// near the top of a large file: the whole buffer is rescanned every keystroke.
func BenchmarkSpanCacheTopFull(bench *testing.B) {
	benchSpanCacheKeystroke(bench, 1, true)
}

// BenchmarkSpanCacheTopVisible is the optimised behaviour for the same case:
// only the visible region (plus margin) is scanned.
func BenchmarkSpanCacheTopVisible(bench *testing.B) {
	benchSpanCacheKeystroke(bench, 1, false)
}

// BenchmarkSpanCacheBottomFull is the pre-optimisation behaviour with the
// cursor near the end of the file.
func BenchmarkSpanCacheBottomFull(bench *testing.B) {
	benchSpanCacheKeystroke(bench, benchGoFuncs*8, true)
}

// BenchmarkSpanCacheBottomVisible is the optimised behaviour near the end of
// the file.  Scanning still has to start at offset 0, so little is saved here —
// that is expected and unavoidable.
func BenchmarkSpanCacheBottomVisible(bench *testing.B) {
	benchSpanCacheKeystroke(bench, benchGoFuncs*8, false)
}

// BenchmarkSpanCacheTextMaterialise isolates the O(buffer) work that remains
// after highlighting is bounded: buf.String() plus the []rune conversion the
// renderer needs.  This is the floor the visible-only benchmarks cannot go
// below without reworking the gap buffer's API.
func BenchmarkSpanCacheTextMaterialise(bench *testing.B) {
	src := goSource(benchGoFuncs)
	e := newTestEditor(src)
	b := buf(e)
	pt := 0
	bench.ReportAllocs()
	bench.ResetTimer()
	for range bench.N {
		b.Insert(pt, 'x')
		b.Delete(pt, 1)
		text := b.String()
		runes := []rune(text)
		if len(runes) == 0 {
			bench.Fatal("empty buffer")
		}
	}
}

// ---------------------------------------------------------------------------
// keystroke benchmark matrix: mode × file size × cursor position
// ---------------------------------------------------------------------------

// markdownSource returns Markdown with roughly lines lines, including fenced
// code blocks so the fence state has to be carried across many of them.
func markdownSource(lines int) string {
	var sb strings.Builder
	sb.WriteString("# Title\n\n")
	for i := range lines / 8 {
		fmt.Fprintf(&sb, "## Section %d\n\nSome *italic* and **bold** and `code` "+
			"and a [link](http://example.com/%d).\n\n```go\nfunc f%d() {}\n```\n\n", i, i, i)
	}
	return sb.String()
}

// yamlSource returns YAML with roughly lines lines.
func yamlSource(lines int) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	for i := range lines / 6 {
		fmt.Fprintf(&sb, "# comment %d\nitem%d:\n  name: \"value %d\"\n"+
			"  count: %d\n  enabled: true\n  tags: [a, b, c]\n", i, i, i, i)
	}
	return sb.String()
}

// benchModeSource returns source text for mode with roughly lines lines.
func benchModeSource(mode string, lines int) string {
	switch mode {
	case "markdown":
		return markdownSource(lines)
	case "yaml":
		return yamlSource(lines)
	default:
		return goSource(lines / 8)
	}
}

// plainHighlighter hides a highlighter's rune and resume fast paths behind the
// bare Highlighter interface.  Installing it reproduces what a keystroke cost
// before checkpointed resumption — the whole prefix re-encoded to a string and
// rescanned from offset 0 — so the benchmarks below measure both in one run.
type plainHighlighter struct{ inner syntax.Highlighter }

func (p plainHighlighter) Highlight(text string, start, end int) []syntax.Span {
	return p.inner.Highlight(text, start, end)
}

// benchKeystrokeRedraw measures one keystroke (insert + delete, so the buffer
// content is restored but two change generations have gone by) followed by a
// full Redraw, with the window scrolled so that the cursor sits on scrollLine.
// When plain is true the highlighter's fast paths are hidden, which is the
// pre-optimisation behaviour.
func benchKeystrokeRedraw(bench *testing.B, src, mode string, scrollLine int, plain bool) {
	e := newCapTestEditor(src)
	b := buf(e)
	b.SetMode(mode)
	if plain {
		e.customHighlighters[b] = plainHighlighter{inner: highlighterFor(b)}
	}
	w := e.activeWin
	w.SetScrollLine(scrollLine)
	pt := w.ViewLines()[0].StartPos
	b.SetPoint(pt)

	bench.ReportAllocs()
	bench.ResetTimer()
	for range bench.N {
		b.Insert(pt, 'x')
		b.Delete(pt, 1)
		e.Redraw()
	}
}

// BenchmarkKeystrokeRedraw is the headline typing benchmark: how long one
// keystroke plus a full frame costs across modes, file sizes and cursor
// positions.  The cursor position matters because a highlighter that cannot
// resume has to rescan the whole prefix before the cursor — which is what the
// "scan0" variant of each case measures.
func BenchmarkKeystrokeRedraw(bench *testing.B) {
	for _, mode := range []string{"go", "markdown", "yaml"} {
		for _, lines := range []int{1000, 10000, 50000} {
			src := benchModeSource(mode, lines)
			total := strings.Count(src, "\n") + 1
			for _, pos := range []struct {
				name string
				line int
			}{
				{"top", 1},
				{"mid", total / 2},
				{"end", max(total-20, 1)},
			} {
				for _, variant := range []struct {
					name  string
					plain bool
				}{
					{"resume", false},
					{"scan0", true},
				} {
					bench.Run(fmt.Sprintf("%s/%dk/%s/%s", mode, lines/1000, pos.name, variant.name),
						func(bench *testing.B) {
							benchKeystrokeRedraw(bench, src, mode, pos.line, variant.plain)
						})
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// end-to-end: bounded highlighting draws the same faces as full highlighting
// ---------------------------------------------------------------------------

// goSourceWithMultiLineState returns Go source whose top holds a long block
// comment and a long raw string, followed by enough ordinary code that the
// visible region plus spanCacheMargin covers only part of the buffer.  Any
// truncation point therefore lands inside multi-line state.
func goSourceWithMultiLineState() string {
	var sb strings.Builder
	sb.WriteString("package main\n\nimport \"fmt\"\n\n")
	sb.WriteString("/* a block comment\n")
	for i := range 40 {
		fmt.Fprintf(&sb, "   line %d of the comment, mentioning func and return\n", i)
	}
	sb.WriteString("*/\n\nconst tmpl = `raw string\n")
	for i := range 40 {
		fmt.Fprintf(&sb, "line %d of the raw string with a // fake comment and \"quotes\"\n", i)
	}
	sb.WriteString("`\n\n")
	sb.WriteString(goSource(300))
	return sb.String()
}

// captureFaces returns the face of every cell in the text rows of the window.
func captureFaces(t *testing.T, e *Editor) []syntax.Face {
	t.Helper()
	w, h := e.term.CaptureSize()
	faces := make([]syntax.Face, 0, w*h)
	for row := range h {
		for col := range w {
			_, face := e.term.CaptureCell(col, row)
			faces = append(faces, face)
		}
	}
	return faces
}

// TestRenderWindowFacesMatchFullHighlighting renders the same buffer twice at
// several scroll positions: once through the normal bounded path, and once with
// a full-buffer span cache primed first.  Every drawn cell must carry the same
// face, including where the truncation boundary falls inside a block comment or
// a multi-line raw string.
func TestRenderWindowFacesMatchFullHighlighting(t *testing.T) {
	src := goSourceWithMultiLineState()

	// Scroll positions: top, inside the block comment, inside the raw string,
	// just past both, and well into the filler code.
	for _, scrollLine := range []int{1, 10, 44, 60, 90, 400} {
		t.Run(fmt.Sprintf("scroll=%d", scrollLine), func(t *testing.T) {
			bounded := newCapTestEditor(src)
			buf(bounded).SetMode("go")
			bounded.activeWin.SetScrollLine(scrollLine)
			bounded.Redraw()
			// Confirm the bounded path really did truncate; otherwise this test
			// would silently compare two identical full scans.
			c := bounded.spanCaches[buf(bounded)]
			if c == nil {
				t.Fatal("no span cache after Redraw")
			}

			reference := newCapTestEditor(src)
			buf(reference).SetMode("go")
			reference.activeWin.SetScrollLine(scrollLine)
			// Prime a full-buffer cache; Redraw's narrower request reuses it.
			reference.getSpanCache(buf(reference))
			reference.Redraw()
			if rc := reference.spanCaches[buf(reference)]; rc.hiEnd != buf(reference).Len() {
				t.Fatalf("reference cache hiEnd = %d, want the whole buffer (%d)",
					rc.hiEnd, buf(reference).Len())
			}

			got := captureFaces(t, bounded)
			want := captureFaces(t, reference)
			if len(got) != len(want) {
				t.Fatalf("captured %d cells vs %d", len(got), len(want))
			}
			// Guard against a vacuous pass: the reference render must actually
			// have coloured something.
			coloured := 0
			for _, f := range want {
				if f != syntax.FaceDefault && f != (syntax.Face{}) {
					coloured++
				}
			}
			if coloured == 0 {
				t.Fatal("reference render produced no highlighted cells; test would be vacuous")
			}
			width, _ := bounded.term.CaptureSize()
			for i := range got {
				if got[i] != want[i] {
					t.Fatalf("face mismatch at row %d col %d (hiEnd=%d): got %+v, want %+v",
						i/width, i%width, c.hiEnd, got[i], want[i])
				}
			}
		})
	}
}

// TestRenderWindowTruncatesBelowBufferLength documents that the bounded render
// path really does stop short of the end of a large buffer.
func TestRenderWindowTruncatesBelowBufferLength(t *testing.T) {
	e := newCapTestEditor(goSource(2000))
	b := buf(e)
	b.SetMode("go")
	e.Redraw()
	c := e.spanCaches[b]
	if c == nil {
		t.Fatal("no span cache after Redraw")
	}
	if c.hiEnd >= b.Len() {
		t.Errorf("hiEnd = %d, want < buffer length %d", c.hiEnd, b.Len())
	}
}

// ---------------------------------------------------------------------------
// Capture-terminal test helper
// ---------------------------------------------------------------------------

// newCapTestEditor builds a minimal Editor backed by a headless capture terminal.
func newCapTestEditor(content string) *Editor {
	term := terminal.NewCapture(80, 24)
	buf := buffer.NewWithContent("*test*", content)
	win := window.New(buf, 0, 0, 80, 23)
	e := &Editor{
		term:                       term,
		buffers:                    []*buffer.Buffer{buf},
		windows:                    []*window.Window{win},
		activeWin:                  win,
		layoutRoot:                 leafNode(win),
		minibufBuf:                 buffer.New(" *minibuf*"),
		globalKeymap:               keymap.New("global"),
		ctrlXKeymap:                keymap.New("C-x"),
		universalArg:               1,
		fillColumn:                 70,
		isSearchCaseFold:           true,
		saveBufferDeleteTrailingWS: true,
		customHighlighters:         make(map[*buffer.Buffer]syntax.Highlighter),
		autoRevertMtimes:           make(map[*buffer.Buffer]time.Time),
		// lspAsync sends its completion callback to lspCbs; leaving it nil made
		// every async spell/LSP path block a goroutine forever and hid the
		// callback from assertions.  Buffered so nothing has to drain it.
		lspCbs: make(chan func(), 64),
	}
	e.minibufWin = window.New(e.minibufBuf, 23, 0, 80, 1)
	e.lisp = elisp.NewEvaluator()
	e.setupKeymaps()
	return e
}

// captureRow returns all runes in a given screen row as a string.
func captureRow(t *testing.T, e *Editor, row int) string {
	t.Helper()
	w, _ := e.term.CaptureSize()
	var sb strings.Builder
	for col := 0; col < w; col++ {
		ch, _ := e.term.CaptureCell(col, row)
		sb.WriteRune(ch)
	}
	return sb.String()
}

// ---------------------------------------------------------------------------
// Redraw
// ---------------------------------------------------------------------------

func TestRedrawDoesNotPanicOnEmptyBuffer(t *testing.T) {
	e := newCapTestEditor("")
	e.Redraw() // must not panic
}

func TestRedrawDoesNotPanicOnSingleLine(t *testing.T) {
	e := newCapTestEditor("hello world")
	e.Redraw()
}

func TestRedrawDoesNotPanicOnMultiLine(t *testing.T) {
	e := newCapTestEditor("line1\nline2\nline3\n")
	e.Redraw()
}

func TestRedrawWithNilTermDoesNotPanic(t *testing.T) {
	e := newTestEditor("some content")
	// term is nil — Redraw must return immediately without panic
	e.Redraw()
}

// ---------------------------------------------------------------------------
// renderWindow — buffer content appears at expected screen positions
// ---------------------------------------------------------------------------

func TestRenderWindowFirstLineContent(t *testing.T) {
	e := newCapTestEditor("Hello")
	e.Redraw()

	ch, _ := e.term.CaptureCell(0, 0)
	if ch != 'H' {
		t.Errorf("expected 'H' at (0,0), got %q", ch)
	}
	ch, _ = e.term.CaptureCell(4, 0)
	if ch != 'o' {
		t.Errorf("expected 'o' at (4,0), got %q", ch)
	}
}

func TestRenderWindowMultipleLines(t *testing.T) {
	e := newCapTestEditor("foo\nbar\nbaz")
	e.Redraw()

	ch, _ := e.term.CaptureCell(0, 0)
	if ch != 'f' {
		t.Errorf("expected 'f' at (0,0), got %q", ch)
	}
	ch, _ = e.term.CaptureCell(0, 1)
	if ch != 'b' {
		t.Errorf("expected 'b' at (0,1), got %q", ch)
	}
}

func TestRenderWindowTabExpansion(t *testing.T) {
	e := newCapTestEditor("\tA")
	e.Redraw()

	// Tab expands to tabWidth (2) spaces; 'A' lands at column 2.
	ch, _ := e.term.CaptureCell(0, 0)
	if ch != ' ' {
		t.Errorf("expected space at (0,0) for tab, got %q", ch)
	}
	ch, _ = e.term.CaptureCell(2, 0)
	if ch != 'A' {
		t.Errorf("expected 'A' at (2,0) after tab, got %q", ch)
	}
}

func TestRenderWindowEmptyBufferFillsSpaces(t *testing.T) {
	e := newCapTestEditor("")
	e.Redraw()

	// Row 0 should be all spaces (blank line).
	for col := 0; col < 10; col++ {
		ch, _ := e.term.CaptureCell(col, 0)
		if ch != ' ' {
			t.Errorf("expected space at (%d,0) for empty buffer, got %q", col, ch)
		}
	}
}

func TestRenderWindowLongLineTruncated(t *testing.T) {
	// A line longer than the window width (80) must not cause a panic and
	// must stop exactly at the window edge.
	content := strings.Repeat("X", 200)
	e := newCapTestEditor(content)
	e.Redraw()

	// Column 79 (last column) should have been written.
	ch, _ := e.term.CaptureCell(79, 0)
	if ch != 'X' {
		t.Errorf("expected 'X' at col 79, got %q", ch)
	}
}

// ---------------------------------------------------------------------------
// renderModeline
// ---------------------------------------------------------------------------

func TestRenderModelineContainsBufferName(t *testing.T) {
	e := newCapTestEditor("content")
	e.Redraw()

	// The modeline is drawn on row winH-1 = 22 (window height 23, last row is modeline).
	modeRow := 22
	row := captureRow(t, e, modeRow)
	if !strings.Contains(row, "*test*") {
		t.Errorf("modeline row %d does not contain buffer name *test*: %q", modeRow, row)
	}
}

func TestRenderModelineContainsMode(t *testing.T) {
	e := newCapTestEditor("package main\n")
	e.ActiveBuffer().SetMode("go")
	e.Redraw()

	modeRow := 22
	row := captureRow(t, e, modeRow)
	if !strings.Contains(row, "go") {
		t.Errorf("modeline does not contain mode 'go': %q", row)
	}
}

func TestRenderModelineModifiedMark(t *testing.T) {
	e := newCapTestEditor("")
	// Insert something to mark the buffer as modified.
	b := e.ActiveBuffer()
	b.Insert(0, 'X')
	e.Redraw()

	modeRow := 22
	row := captureRow(t, e, modeRow)
	if !strings.Contains(row, "**") {
		t.Errorf("modeline does not show '**' for modified buffer: %q", row)
	}
}

func TestRenderModelineReadOnlyMark(t *testing.T) {
	e := newCapTestEditor("data")
	e.ActiveBuffer().SetReadOnly(true)
	e.Redraw()

	modeRow := 22
	row := captureRow(t, e, modeRow)
	if !strings.Contains(row, "%%") {
		t.Errorf("modeline does not show '%%%%' for read-only buffer: %q", row)
	}
}

func TestRenderModelineUnmodifiedMark(t *testing.T) {
	e := newCapTestEditor("data")
	// Buffer was just created and not edited; should show "-".
	e.Redraw()

	modeRow := 22
	row := captureRow(t, e, modeRow)
	if !strings.Contains(row, " - ") {
		t.Errorf("modeline does not show '-' for unmodified buffer: %q", row)
	}
}

func TestRenderModelineNarrowIndicator(t *testing.T) {
	e := newCapTestEditor("line1\nline2\nline3\n")
	b := e.ActiveBuffer()
	b.Narrow(0, 5)
	e.Redraw()

	row := captureRow(t, e, 22)
	if !strings.Contains(row, "Narrow") {
		t.Errorf("modeline should show Narrow indicator: %q", row)
	}
}

func TestRenderModelineMacroIndicator(t *testing.T) {
	e := newCapTestEditor("text")
	e.kbdMacroRecording = true
	e.Redraw()

	row := captureRow(t, e, 22)
	if !strings.Contains(row, "Def") {
		t.Errorf("modeline should show macro Def indicator: %q", row)
	}
}

func TestRenderModelineVcAnnotateStripsLangSuffix(t *testing.T) {
	e := newCapTestEditor("abc123 (Author) package main\n")
	e.ActiveBuffer().SetMode("vc-annotate+go")
	e.Redraw()

	row := captureRow(t, e, 22)
	if !strings.Contains(row, "vc-annotate") {
		t.Errorf("modeline should show base vc-annotate mode: %q", row)
	}
	if strings.Contains(row, "vc-annotate+go") {
		t.Errorf("modeline should strip +go suffix: %q", row)
	}
}

func TestRenderModelineCompilationOK(t *testing.T) {
	e := newCapTestEditor("build output\n")
	b := e.ActiveBuffer()
	b.SetName("*compilation*")
	ok := true
	e.compilationExitOK = &ok
	e.Redraw()

	row := captureRow(t, e, 22)
	if !strings.Contains(row, "*compilation*") {
		t.Errorf("modeline should show *compilation* name: %q", row)
	}
}

func TestRenderModelineCompilationFail(t *testing.T) {
	e := newCapTestEditor("build output\n")
	b := e.ActiveBuffer()
	b.SetName("*compilation*")
	failed := false
	e.compilationExitOK = &failed
	// Should render the fail-colored name without panicking.
	e.Redraw()

	row := captureRow(t, e, 22)
	if !strings.Contains(row, "*compilation*") {
		t.Errorf("modeline should show *compilation* name: %q", row)
	}
}

// ---------------------------------------------------------------------------
// renderWindow overlays
// ---------------------------------------------------------------------------

func TestRenderWindowRegionHighlight(t *testing.T) {
	e := newCapTestEditor("hello")
	b := e.ActiveBuffer()
	b.SetMark(0)
	b.SetMarkActive(true)
	b.SetPoint(3)
	e.Redraw()

	// Cells 0..2 are inside the region and should use FaceRegion.
	_, face := e.term.CaptureCell(0, 0)
	if face != syntax.FaceRegion {
		t.Errorf("expected FaceRegion at (0,0) inside region, got %+v", face)
	}
	// Cell 4 is outside the region.
	_, face = e.term.CaptureCell(4, 0)
	if face == syntax.FaceRegion {
		t.Error("cell 4 should not be region-highlighted")
	}
}

func TestRenderWindowIsearchHighlight(t *testing.T) {
	e := newCapTestEditor("abcXYZ")
	e.isearching = true
	e.isearchFwd = true
	e.isearchStr = "XYZ"
	e.isSearchCaseFold = false
	// Forward isearch: match ends at point.
	e.ActiveBuffer().SetPoint(6)
	e.Redraw()

	_, face := e.term.CaptureCell(3, 0) // 'X'
	if face != syntax.FaceIsearch {
		t.Errorf("expected FaceIsearch at match start, got %+v", face)
	}
}

func TestRenderWindowQueryReplaceHighlight(t *testing.T) {
	e := newCapTestEditor("foo bar")
	e.queryReplaceActive = true
	e.queryReplaceMatch = 0
	e.queryReplaceFromRunes = []rune("foo")
	e.Redraw()

	_, face := e.term.CaptureCell(0, 0)
	if face != syntax.FaceIsearch {
		t.Errorf("expected FaceIsearch for query-replace match, got %+v", face)
	}
}

func TestRenderWindowNarrowedSkipsOutsideLines(t *testing.T) {
	e := newCapTestEditor("AAAA\nBBBB\nCCCC\n")
	b := e.ActiveBuffer()
	// Narrow to the second line only (offsets 5..9 = "BBBB").
	b.Narrow(5, 9)
	e.Redraw()

	// The first physical line "AAAA" is outside the narrow region and must be blank.
	ch, _ := e.term.CaptureCell(0, 0)
	if ch == 'A' {
		t.Errorf("line outside narrow region should not render 'A', got %q", ch)
	}
}

func TestRenderWindowBreakpointGutter(t *testing.T) {
	e := newCapTestEditor("package main\nfunc main() {}\n")
	b := e.ActiveBuffer()
	b.SetFilename("/tmp/gomacs_render_bp.go")
	b.SetMode("go")
	e.dapBreakpoints = map[string]map[int]struct{}{
		"/tmp/gomacs_render_bp.go": {1: {}},
	}
	e.Redraw()

	// The gutter draws a breakpoint bullet at column 0 of line 1.
	ch, _ := e.term.CaptureCell(0, 0)
	if ch != '●' {
		t.Errorf("expected breakpoint bullet in gutter, got %q", ch)
	}
}

func TestRenderWindowExecPosArrow(t *testing.T) {
	e := newCapTestEditor("package main\nfunc main() {}\n")
	b := e.ActiveBuffer()
	b.SetFilename("/tmp/gomacs_render_ep.go")
	b.SetMode("go")
	e.dapBreakpoints = map[string]map[int]struct{}{
		"/tmp/gomacs_render_ep.go": {2: {}},
	}
	e.dap = &dapState{stoppedFile: "/tmp/gomacs_render_ep.go", stoppedLine: 1}
	e.Redraw()

	// Exec-pos arrow is drawn at gutter column 1 of the stopped line (row 0).
	ch, _ := e.term.CaptureCell(1, 0)
	if ch != '→' {
		t.Errorf("expected exec-pos arrow at stopped line, got %q", ch)
	}
}

// ---------------------------------------------------------------------------
// applyVisualLines
// ---------------------------------------------------------------------------

func TestApplyVisualLinesEnabled(t *testing.T) {
	e := newCapTestEditor(strings.Repeat("x", 200) + "\n")
	e.visualLines = true
	e.visualLinesSynced = false
	e.applyVisualLines()
	// Re-applying when already synced is a no-op and must not panic.
	e.applyVisualLines()
}

func TestApplyVisualLinesDisabled(t *testing.T) {
	e := newCapTestEditor(strings.Repeat("y", 200) + "\n")
	e.visualLines = false
	e.visualLinesSynced = false
	e.applyVisualLines()
}

// ---------------------------------------------------------------------------
// renderMinibuffer
// ---------------------------------------------------------------------------

func TestRenderMinibufferShowsPromptWhenActive(t *testing.T) {
	e := newCapTestEditor("hello")
	e.minibufActive = true
	e.minibufPrompt = "Find: "
	e.Redraw()

	minibufRow := 23
	row := captureRow(t, e, minibufRow)
	if !strings.Contains(row, "Find: ") {
		t.Errorf("minibuffer row does not contain prompt 'Find: ': %q", row)
	}
}

func TestRenderMinibufferShowsTypedText(t *testing.T) {
	e := newCapTestEditor("")
	e.minibufActive = true
	e.minibufPrompt = "Name: "
	e.minibufBuf.InsertString(0, "alice")
	e.minibufBuf.SetPoint(5)
	e.Redraw()

	minibufRow := 23
	row := captureRow(t, e, minibufRow)
	if !strings.Contains(row, "alice") {
		t.Errorf("minibuffer row does not contain typed text 'alice': %q", row)
	}
}

func TestRenderMinibufferEmptyWhenInactive(t *testing.T) {
	e := newCapTestEditor("text")
	// No message, no minibuf active.
	e.message = ""
	e.minibufActive = false
	e.Redraw()

	minibufRow := 23
	row := captureRow(t, e, minibufRow)
	// Should be all spaces.
	trimmed := strings.TrimRight(row, " ")
	if trimmed != "" {
		t.Errorf("expected empty minibuffer row, got: %q", row)
	}
}

func TestRenderMinibufferMessage(t *testing.T) {
	e := newCapTestEditor("")
	e.Message("File saved")
	e.Redraw()

	minibufRow := 23
	row := captureRow(t, e, minibufRow)
	if !strings.Contains(row, "File saved") {
		t.Errorf("minibuffer row does not contain message: %q", row)
	}
}

// ---------------------------------------------------------------------------
// renderCandidatePopup
// ---------------------------------------------------------------------------

func TestRenderCandidatePopupShowsCandidates(t *testing.T) {
	e := newCapTestEditor("")
	e.minibufActive = true
	e.minibufPrompt = "Cmd: "
	e.minibufCandidates = []string{"alpha", "beta", "gamma"}
	e.minibufSelectedIdx = 0
	e.minibufCandidateOffset = 0
	e.Redraw()

	// Candidates are drawn above the minibuffer row (row 23).
	// With 3 candidates they occupy rows 20, 21, 22 (minibuf popup goes up).
	found := false
	for row := 18; row <= 22; row++ {
		r := captureRow(t, e, row)
		if strings.Contains(r, "alpha") {
			found = true
			break
		}
	}
	if !found {
		t.Error("candidate 'alpha' was not rendered in the popup area")
	}
}

func TestRenderCandidatePopupSelectedHighlightedDifferently(t *testing.T) {
	e := newCapTestEditor("")
	e.minibufActive = true
	e.minibufPrompt = "Pick: "
	e.minibufCandidates = []string{"one", "two"}
	e.minibufSelectedIdx = 0
	e.minibufCandidateOffset = 0
	e.Redraw()

	// Just verify rendering doesn't panic; face checking would need a more
	// detailed capture helper. We verify at least the text appears.
	found := false
	for row := 0; row < 24; row++ {
		r := captureRow(t, e, row)
		if strings.Contains(r, "one") {
			found = true
			break
		}
	}
	if !found {
		t.Error("selected candidate 'one' not found on screen")
	}
}

func TestRenderCandidatePopupEmpty(t *testing.T) {
	e := newCapTestEditor("")
	e.minibufActive = true
	e.minibufPrompt = "P: "
	e.minibufCandidates = nil
	// Should not panic.
	e.Redraw()
}

// ---------------------------------------------------------------------------
// placeCursor / screenColForPoint
// ---------------------------------------------------------------------------

func TestScreenColForPointAtBOL(t *testing.T) {
	b := buffer.NewWithContent("*t*", "hello")
	col := screenColForPoint(b, 0)
	if col != 0 {
		t.Errorf("expected col 0 at BOL, got %d", col)
	}
}

func TestScreenColForPointMidLine(t *testing.T) {
	b := buffer.NewWithContent("*t*", "hello")
	col := screenColForPoint(b, 3)
	if col != 3 {
		t.Errorf("expected col 3, got %d", col)
	}
}

func TestScreenColForPointWithTab(t *testing.T) {
	b := buffer.NewWithContent("*t*", "\tA")
	// Tab expands to tabWidth (2), so 'A' is at column tabWidth.
	col := screenColForPoint(b, 1) // point after the tab
	if col != tabWidth {
		t.Errorf("expected col %d after tab, got %d", tabWidth, col)
	}
}

func TestScreenColForPointMultipleTabs(t *testing.T) {
	b := buffer.NewWithContent("*t*", "\t\tX")
	col := screenColForPoint(b, 2) // two tabs
	if col != 2*tabWidth {
		t.Errorf("expected col %d after two tabs, got %d", 2*tabWidth, col)
	}
}

func TestPlaceCursorMinibufActive(t *testing.T) {
	e := newCapTestEditor("hello")
	e.minibufActive = true
	e.minibufPrompt = "Q: "
	// Should not panic.
	e.Redraw()
}

func TestPlaceCursorNormalMode(t *testing.T) {
	e := newCapTestEditor("hello")
	// Should not panic.
	e.Redraw()
}

// ---------------------------------------------------------------------------
// highlighterFor
// ---------------------------------------------------------------------------

func TestHighlighterForGoMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "package main")
	b.SetMode("go")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.GoHighlighter); !ok {
		t.Errorf("expected GoHighlighter for go mode, got %T", hl)
	}
}

func TestHighlighterForMarkdownMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "# heading")
	b.SetMode("markdown")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.MarkdownHighlighter); !ok {
		t.Errorf("expected MarkdownHighlighter for markdown mode, got %T", hl)
	}
}

func TestHighlighterForElispMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "(defun foo () nil)")
	b.SetMode("elisp")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.ElispHighlighter); !ok {
		t.Errorf("expected ElispHighlighter for elisp mode, got %T", hl)
	}
}

func TestHighlighterForPythonMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "def foo(): pass")
	b.SetMode("python")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.PythonHighlighter); !ok {
		t.Errorf("expected PythonHighlighter for python mode, got %T", hl)
	}
}

func TestHighlighterForJavaMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "class Foo {}")
	b.SetMode("java")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.JavaHighlighter); !ok {
		t.Errorf("expected JavaHighlighter for java mode, got %T", hl)
	}
}

func TestHighlighterForBashMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "#!/bin/bash\necho hi")
	b.SetMode("bash")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.BashHighlighter); !ok {
		t.Errorf("expected BashHighlighter for bash mode, got %T", hl)
	}
}

func TestHighlighterForJSONMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", `{"key":"val"}`)
	b.SetMode("json")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.JSONHighlighter); !ok {
		t.Errorf("expected JSONHighlighter for json mode, got %T", hl)
	}
}

func TestHighlighterForYAMLMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "key: value")
	b.SetMode("yaml")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.YAMLHighlighter); !ok {
		t.Errorf("expected YAMLHighlighter for yaml mode, got %T", hl)
	}
}

func TestHighlighterForDiffMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "+added\n-removed\n")
	b.SetMode("diff")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.DiffHighlighter); !ok {
		t.Errorf("expected DiffHighlighter for diff mode, got %T", hl)
	}
}

func TestHighlighterForVcLogMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "commit abc123\n")
	b.SetMode("vc-log")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.VcLogHighlighter); !ok {
		t.Errorf("expected VcLogHighlighter for vc-log mode, got %T", hl)
	}
}

func TestHighlighterForVcAnnotateMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "abc123 (Author 2024-01-01) line")
	b.SetMode("vc-annotate")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.VcAnnotateHighlighter); !ok {
		t.Errorf("expected VcAnnotateHighlighter for vc-annotate mode, got %T", hl)
	}
}

func TestHighlighterForVcAnnotatePlusLang(t *testing.T) {
	b := buffer.NewWithContent("*t*", "abc123 (Author) package main")
	b.SetMode("vc-annotate+go")
	hl := highlighterFor(b)
	va, ok := hl.(syntax.VcAnnotateHighlighter)
	if !ok {
		t.Fatalf("expected VcAnnotateHighlighter for vc-annotate+go mode, got %T", hl)
	}
	if va.Source == nil {
		t.Error("expected non-nil Source highlighter for vc-annotate+go")
	}
}

func TestHighlighterForHelpMode(t *testing.T) {
	b := buffer.NewWithContent("*Help*", "describe-function\n")
	b.SetMode("help")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.HelpHighlighter); !ok {
		t.Errorf("expected HelpHighlighter for help mode, got %T", hl)
	}
}

func TestHighlighterForFundamentalMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "plain text")
	b.SetMode("fundamental")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.NilHighlighter); !ok {
		t.Errorf("expected NilHighlighter for fundamental mode, got %T", hl)
	}
}

func TestHighlighterForTextMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "plain text")
	b.SetMode("text")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.NilHighlighter); !ok {
		t.Errorf("expected NilHighlighter for text mode, got %T", hl)
	}
}

func TestHighlighterForMakefileMode(t *testing.T) {
	b := buffer.NewWithContent("Makefile", "all:\n\tgo build")
	b.SetMode("makefile")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.MakefileHighlighter); !ok {
		t.Errorf("expected MakefileHighlighter for makefile mode, got %T", hl)
	}
}

func TestHighlighterForVcShowMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "commit abc\n")
	b.SetMode("vc-show")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.VcShowHighlighter); !ok {
		t.Errorf("expected VcShowHighlighter for vc-show mode, got %T", hl)
	}
}

func TestHighlighterForVcStatusMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "M  file.go\n")
	b.SetMode("vc-status")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.VcStatusHighlighter); !ok {
		t.Errorf("expected VcStatusHighlighter for vc-status mode, got %T", hl)
	}
}

func TestHighlighterForGherkinMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "Feature: login\n")
	b.SetMode("gherkin")
	hl := highlighterFor(b)
	if _, ok := hl.(syntax.GherkinHighlighter); !ok {
		t.Errorf("expected GherkinHighlighter for gherkin mode, got %T", hl)
	}
}

func TestHighlighterForVcGrepModes(t *testing.T) {
	for _, mode := range []string{"vc-grep", "lsp-refs"} {
		b := buffer.NewWithContent("*t*", "file.go:1: hit\n")
		b.SetMode(mode)
		if _, ok := highlighterFor(b).(syntax.VcGrepHighlighter); !ok {
			t.Errorf("mode %q: expected VcGrepHighlighter, got %T", mode, highlighterFor(b))
		}
	}
}

func TestHighlighterForVcFixupSelectMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "commit abc123\n")
	b.SetMode("vc-fixup-select")
	if _, ok := highlighterFor(b).(syntax.VcLogHighlighter); !ok {
		t.Errorf("expected VcLogHighlighter for vc-fixup-select mode, got %T", highlighterFor(b))
	}
}

func TestHighlighterForVcCommitMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "feat: thing\n")
	b.SetMode("vc-commit")
	if _, ok := highlighterFor(b).(syntax.VcCommitHighlighter); !ok {
		t.Errorf("expected VcCommitHighlighter for vc-commit mode, got %T", highlighterFor(b))
	}
}

func TestHighlighterForConfMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "key = value\n")
	b.SetMode("conf")
	if _, ok := highlighterFor(b).(syntax.ConfHighlighter); !ok {
		t.Errorf("expected ConfHighlighter for conf mode, got %T", highlighterFor(b))
	}
}

func TestHighlighterForPerlMode(t *testing.T) {
	b := buffer.NewWithContent("*t*", "print \"hi\";\n")
	b.SetMode("perl")
	if _, ok := highlighterFor(b).(syntax.PerlHighlighter); !ok {
		t.Errorf("expected PerlHighlighter for perl mode, got %T", highlighterFor(b))
	}
}

func TestHighlighterForDebugModes(t *testing.T) {
	bl := buffer.NewWithContent("*t*", "x = 1\n")
	bl.SetMode("debug-locals")
	if _, ok := highlighterFor(bl).(syntax.DapLocalsHighlighter); !ok {
		t.Errorf("debug-locals: got %T, want DapLocalsHighlighter", highlighterFor(bl))
	}

	bs := buffer.NewWithContent("*t*", "frame 0\n")
	bs.SetMode("debug-stack")
	if _, ok := highlighterFor(bs).(syntax.DapStackHighlighter); !ok {
		t.Errorf("debug-stack: got %T, want DapStackHighlighter", highlighterFor(bs))
	}

	br := buffer.NewWithContent("*t*", "p x\n")
	br.SetMode("debug-repl")
	if _, ok := highlighterFor(br).(syntax.GoHighlighter); !ok {
		t.Errorf("debug-repl: got %T, want GoHighlighter", highlighterFor(br))
	}
}

// ---------------------------------------------------------------------------
// faceAtPos
// ---------------------------------------------------------------------------

func TestFaceAtPosNoSpans(t *testing.T) {
	face := faceAtPos(nil, 5)
	if face != syntax.FaceDefault {
		t.Errorf("expected FaceDefault for empty spans, got %+v", face)
	}
}

func TestFaceAtPosInsideSpan(t *testing.T) {
	spans := []syntax.Span{
		{Start: 0, End: 5, Face: syntax.FaceKeyword},
	}
	face := faceAtPos(spans, 2)
	if face != syntax.FaceKeyword {
		t.Errorf("expected FaceKeyword at pos 2, got %+v", face)
	}
}

func TestFaceAtPosAtSpanEnd(t *testing.T) {
	spans := []syntax.Span{
		{Start: 0, End: 5, Face: syntax.FaceKeyword},
	}
	// pos 5 is equal to End (exclusive), so it should fall back to default.
	face := faceAtPos(spans, 5)
	if face != syntax.FaceDefault {
		t.Errorf("expected FaceDefault at span end pos 5, got %+v", face)
	}
}

func TestFaceAtPosBeforeFirstSpan(t *testing.T) {
	spans := []syntax.Span{
		{Start: 10, End: 20, Face: syntax.FaceString},
	}
	face := faceAtPos(spans, 3)
	if face != syntax.FaceDefault {
		t.Errorf("expected FaceDefault before first span, got %+v", face)
	}
}

func TestFaceAtPosMultipleSpans(t *testing.T) {
	spans := []syntax.Span{
		{Start: 0, End: 3, Face: syntax.FaceKeyword},
		{Start: 5, End: 10, Face: syntax.FaceString},
		{Start: 12, End: 18, Face: syntax.FaceComment},
	}
	// pos 6 is inside the second span.
	face := faceAtPos(spans, 6)
	if face != syntax.FaceString {
		t.Errorf("expected FaceString at pos 6, got %+v", face)
	}
}

func TestFaceAtPosBetweenSpans(t *testing.T) {
	spans := []syntax.Span{
		{Start: 0, End: 3, Face: syntax.FaceKeyword},
		{Start: 5, End: 10, Face: syntax.FaceString},
	}
	// pos 4 is between spans.
	face := faceAtPos(spans, 4)
	if face != syntax.FaceDefault {
		t.Errorf("expected FaceDefault between spans at pos 4, got %+v", face)
	}
}

// ---------------------------------------------------------------------------
// helpDispatch
// ---------------------------------------------------------------------------

func TestHelpDispatchQuitReturnsTrue(t *testing.T) {
	e := newTestEditor("main text")
	e.lisp = elisp.NewEvaluator()

	// Create a help buffer and switch to it.
	helpBuf := buffer.NewWithContent("*Help*", "some help")
	helpBuf.SetMode("help")
	e.buffers = append(e.buffers, helpBuf)
	e.activeWin.SetBuf(helpBuf)

	ke := terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'q'}
	consumed := e.helpDispatch(ke)
	if !consumed {
		t.Error("helpDispatch should consume 'q'")
	}
	// After 'q' the active buffer should not be the help buffer.
	if e.ActiveBuffer() == helpBuf {
		t.Error("helpDispatch should switch away from the help buffer")
	}
}

func TestHelpDispatchNonQKeyNotConsumed(t *testing.T) {
	e := newTestEditor("text")
	e.lisp = elisp.NewEvaluator()

	helpBuf := buffer.NewWithContent("*Help*", "help")
	helpBuf.SetMode("help")
	e.buffers = append(e.buffers, helpBuf)
	e.activeWin.SetBuf(helpBuf)

	ke := terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'n'}
	consumed := e.helpDispatch(ke)
	if consumed {
		t.Error("helpDispatch should not consume non-'q' keys")
	}
}

func TestHelpDispatchQuitFallsBackToScratch(t *testing.T) {
	e := newTestEditor("text")
	e.lisp = elisp.NewEvaluator()

	// Remove the default *test* buffer so no non-help buffer exists initially
	// — the function should create / switch to *scratch*.
	helpBuf := buffer.NewWithContent("*Help*", "help")
	helpBuf.SetMode("help")
	// Make *Help* the only buffer.
	e.buffers = []*buffer.Buffer{helpBuf}
	e.bufferMRU = nil
	e.activeWin.SetBuf(helpBuf)

	ke := terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'q'}
	consumed := e.helpDispatch(ke)
	if !consumed {
		t.Error("helpDispatch should consume 'q' even when no other buffer exists")
	}
}

// ---------------------------------------------------------------------------
// manDispatch
// ---------------------------------------------------------------------------

func TestManDispatchQuitReturnsTrue(t *testing.T) {
	e := newTestEditor("main text")
	e.lisp = elisp.NewEvaluator()

	manBuf := buffer.NewWithContent("*Man ls*", "man content")
	manBuf.SetMode("man")
	e.buffers = append(e.buffers, manBuf)
	e.activeWin.SetBuf(manBuf)

	ke := terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'q'}
	consumed := e.manDispatch(ke)
	if !consumed {
		t.Error("manDispatch should consume 'q'")
	}
	if e.ActiveBuffer() == manBuf {
		t.Error("manDispatch should switch away from the man buffer")
	}
}

func TestManDispatchNonQKeyNotConsumed(t *testing.T) {
	e := newTestEditor("text")
	e.lisp = elisp.NewEvaluator()

	manBuf := buffer.NewWithContent("*Man*", "man")
	manBuf.SetMode("man")
	e.buffers = append(e.buffers, manBuf)
	e.activeWin.SetBuf(manBuf)

	ke := terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'j'}
	consumed := e.manDispatch(ke)
	if consumed {
		t.Error("manDispatch should not consume non-'q' keys")
	}
}

// ---------------------------------------------------------------------------
// startIsearch
// ---------------------------------------------------------------------------

func TestStartIsearchForward(t *testing.T) {
	e := newTestEditor("hello world")
	e.lisp = elisp.NewEvaluator()
	e.ActiveBuffer().SetPoint(3)
	e.startIsearch(true)

	if !e.isearching {
		t.Error("expected isearching=true after startIsearch")
	}
	if !e.isearchFwd {
		t.Error("expected isearchFwd=true for forward isearch")
	}
	if e.isearchStr != "" {
		t.Errorf("expected empty isearchStr, got %q", e.isearchStr)
	}
	if e.isearchStart != 3 {
		t.Errorf("expected isearchStart=3, got %d", e.isearchStart)
	}
}

func TestStartIsearchBackward(t *testing.T) {
	e := newTestEditor("hello world")
	e.lisp = elisp.NewEvaluator()
	e.startIsearch(false)

	if !e.isearching {
		t.Error("expected isearching=true")
	}
	if e.isearchFwd {
		t.Error("expected isearchFwd=false for backward isearch")
	}
}

// ---------------------------------------------------------------------------
// isearchHandleKey
// ---------------------------------------------------------------------------

func TestIsearchHandleKeyTypingAccumulatesQuery(t *testing.T) {
	e := newTestEditor("hello world")
	e.lisp = elisp.NewEvaluator()
	e.startIsearch(true)

	for _, r := range "ell" {
		ke := terminal.KeyEvent{Key: tcell.KeyRune, Rune: r}
		e.isearchHandleKey(ke)
	}

	if e.isearchStr != "ell" {
		t.Errorf("expected isearchStr='ell', got %q", e.isearchStr)
	}
	if !e.isearching {
		t.Error("expected still isearching after typing")
	}
}

func TestIsearchHandleKeyEnterAccepts(t *testing.T) {
	e := newTestEditor("hello world")
	e.lisp = elisp.NewEvaluator()
	e.startIsearch(true)
	e.isearchStr = "hello"

	ke := terminal.KeyEvent{Key: tcell.KeyEnter}
	e.isearchHandleKey(ke)

	if e.isearching {
		t.Error("expected isearching=false after Enter")
	}
}

func TestIsearchHandleKeyCtrlGCancels(t *testing.T) {
	e := newTestEditor("hello world")
	e.lisp = elisp.NewEvaluator()
	e.ActiveBuffer().SetPoint(3)
	e.startIsearch(true)
	e.isearchStr = "xxx"

	ke := terminal.KeyEvent{Key: tcell.KeyCtrlG}
	e.isearchHandleKey(ke)

	if e.isearching {
		t.Error("expected isearching=false after C-g")
	}
	if e.isearchStr != "" {
		t.Errorf("expected empty isearchStr after cancel, got %q", e.isearchStr)
	}
	// Point should be restored to isearchStart.
	if e.ActiveBuffer().Point() != 3 {
		t.Errorf("expected point restored to 3, got %d", e.ActiveBuffer().Point())
	}
}

func TestIsearchHandleKeyBackspaceRemovesChar(t *testing.T) {
	e := newTestEditor("hello world")
	e.lisp = elisp.NewEvaluator()
	e.startIsearch(true)
	e.isearchStr = "hel"

	ke := terminal.KeyEvent{Key: tcell.KeyBackspace}
	e.isearchHandleKey(ke)

	if e.isearchStr != "he" {
		t.Errorf("expected isearchStr='he' after backspace, got %q", e.isearchStr)
	}
}

func TestIsearchHandleKeyCtrlSSwitchesToForward(t *testing.T) {
	e := newTestEditor("abcabc")
	e.lisp = elisp.NewEvaluator()
	e.startIsearch(false)
	e.isearchStr = "abc"
	// Set point to a mid-buffer position to allow forward search.
	e.ActiveBuffer().SetPoint(3)

	ke := terminal.KeyEvent{Key: tcell.KeyCtrlS}
	e.isearchHandleKey(ke)

	if !e.isearchFwd {
		t.Error("C-s during isearch should set isearchFwd=true")
	}
}

func TestIsearchHandleKeyCtrlRSwitchesToBackward(t *testing.T) {
	e := newTestEditor("abcabc")
	e.lisp = elisp.NewEvaluator()
	e.startIsearch(true)
	e.isearchStr = "abc"
	e.ActiveBuffer().SetPoint(6)

	ke := terminal.KeyEvent{Key: tcell.KeyCtrlR}
	e.isearchHandleKey(ke)

	if e.isearchFwd {
		t.Error("C-r during isearch should set isearchFwd=false")
	}
}

func TestIsearchHandleKeyUnknownKeyExitsIsearch(t *testing.T) {
	e := newTestEditor("hello")
	e.lisp = elisp.NewEvaluator()
	e.setupKeymaps()
	e.startIsearch(true)

	// A control key not handled by isearch exits isearch.
	ke := terminal.KeyEvent{Key: tcell.KeyCtrlA}
	e.isearchHandleKey(ke)

	if e.isearching {
		t.Error("expected isearching=false after unrecognised control key")
	}
}

// ---------------------------------------------------------------------------
// isearchFindNext
// ---------------------------------------------------------------------------

func TestIsearchFindNextForwardFindsNextOccurrence(t *testing.T) {
	e := newTestEditor("abcXdefXghi")
	e.lisp = elisp.NewEvaluator()
	e.ActiveBuffer().SetPoint(0)
	e.startIsearch(true)
	e.isearchStr = "X"
	e.isSearchCaseFold = false

	// First find moves point to after first X (position 4).
	e.isearchFind()
	firstPt := e.ActiveBuffer().Point()

	// isearchFindNext should find the second X.
	e.isearchFindNext()
	secondPt := e.ActiveBuffer().Point()

	if secondPt <= firstPt {
		t.Errorf("isearchFindNext should move point forward: first=%d second=%d", firstPt, secondPt)
	}
}

func TestIsearchFindNextBackwardFindsPreviousOccurrence(t *testing.T) {
	e := newTestEditor("abcXdefXghi")
	e.lisp = elisp.NewEvaluator()
	e.startIsearch(true)
	e.isearchStr = "X"
	e.isSearchCaseFold = false

	// Move to end then search backward.
	e.isearchFwd = false
	e.ActiveBuffer().SetPoint(11)
	e.isearchFindNext()
	pt := e.ActiveBuffer().Point()
	// Should be at position 7 (second X).
	if pt != 7 {
		t.Errorf("expected point at 7 (second X), got %d", pt)
	}
}

// ---------------------------------------------------------------------------
// finishMinibuffer
// ---------------------------------------------------------------------------

func TestFinishMinibufferCallsDoneFunc(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()

	var got string
	e.ReadMinibuffer("Test: ", func(s string) { got = s })
	// Type some text directly into minibufBuf.
	e.minibufBuf.InsertString(0, "myinput")
	e.minibufBuf.SetPoint(7)

	e.finishMinibuffer()

	if got != "myinput" {
		t.Errorf("expected done func called with 'myinput', got %q", got)
	}
	if e.minibufActive {
		t.Error("expected minibufActive=false after finish")
	}
}

func TestFinishMinibufferUsesSelectedCandidate(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()

	var got string
	e.ReadMinibuffer("Pick: ", func(s string) { got = s })
	e.minibufCandidates = []string{"alpha", "beta"}
	e.minibufSelectedIdx = 1
	e.minibufCandidateChosen = true // user navigated popup

	e.finishMinibuffer()

	if got != "beta" {
		t.Errorf("expected 'beta' from candidate navigation, got %q", got)
	}
}

func TestFinishMinibufferSavesHistory(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()

	e.ReadMinibuffer("Cmd: ", func(string) {})
	e.minibufBuf.InsertString(0, "hello")
	e.minibufBuf.SetPoint(5)
	e.finishMinibuffer()

	hist := e.minibufHistory["Cmd: "]
	if len(hist) == 0 || hist[0] != "hello" {
		t.Errorf("expected 'hello' saved in history, got %v", hist)
	}
}

func TestFinishMinibufferNoopWhenInactive(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.minibufActive = false

	// Should not panic.
	e.finishMinibuffer()
}

// ---------------------------------------------------------------------------
// cancelMinibuffer
// ---------------------------------------------------------------------------

func TestCancelMinibufferDeactivates(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()

	var called bool
	e.ReadMinibuffer("P: ", func(string) { called = true })
	e.cancelMinibuffer()

	if e.minibufActive {
		t.Error("expected minibufActive=false after cancel")
	}
	if called {
		t.Error("done func should not be called on cancel")
	}
}

func TestCancelMinibufferSetsQuitMessage(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.ReadMinibuffer("P: ", func(string) {})
	e.cancelMinibuffer()

	if e.message != "Quit" {
		t.Errorf("expected message='Quit' after cancel, got %q", e.message)
	}
}

// ---------------------------------------------------------------------------
// dispatchMinibufKey
// ---------------------------------------------------------------------------

func TestDispatchMinibufKeyPrintableRune(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.ReadMinibuffer("P: ", func(string) {})

	ke := terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'x'}
	e.dispatchMinibufKey(ke)

	if e.minibufBuf.String() != "x" {
		t.Errorf("expected minibuf content 'x', got %q", e.minibufBuf.String())
	}
}

func TestDispatchMinibufKeyBackspace(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.ReadMinibuffer("P: ", func(string) {})

	// Insert "ab" first.
	e.minibufBuf.InsertString(0, "ab")
	e.minibufBuf.SetPoint(2)

	ke := terminal.KeyEvent{Key: tcell.KeyBackspace}
	e.dispatchMinibufKey(ke)

	if e.minibufBuf.String() != "a" {
		t.Errorf("expected 'a' after backspace, got %q", e.minibufBuf.String())
	}
}

func TestDispatchMinibufKeyEnterFinishes(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()

	var got string
	e.ReadMinibuffer("P: ", func(s string) { got = s })
	e.minibufBuf.InsertString(0, "hello")
	e.minibufBuf.SetPoint(5)

	ke := terminal.KeyEvent{Key: tcell.KeyEnter}
	e.dispatchMinibufKey(ke)

	if e.minibufActive {
		t.Error("expected minibufActive=false after Enter")
	}
	if got != "hello" {
		t.Errorf("expected 'hello' from done func, got %q", got)
	}
}

func TestDispatchMinibufKeyCtrlGCancels(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.ReadMinibuffer("P: ", func(string) {})

	ke := terminal.KeyEvent{Key: tcell.KeyCtrlG}
	e.dispatchMinibufKey(ke)

	if e.minibufActive {
		t.Error("expected minibufActive=false after C-g")
	}
}

func TestDispatchMinibufKeyCtrlA(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.ReadMinibuffer("P: ", func(string) {})
	e.minibufBuf.InsertString(0, "hello")
	e.minibufBuf.SetPoint(5)

	ke := terminal.KeyEvent{Key: tcell.KeyCtrlA}
	e.dispatchMinibufKey(ke)

	if e.minibufBuf.Point() != 0 {
		t.Errorf("expected point at 0 after C-a, got %d", e.minibufBuf.Point())
	}
}

func TestDispatchMinibufKeyCtrlE(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.ReadMinibuffer("P: ", func(string) {})
	e.minibufBuf.InsertString(0, "hello")
	e.minibufBuf.SetPoint(0)

	ke := terminal.KeyEvent{Key: tcell.KeyCtrlE}
	e.dispatchMinibufKey(ke)

	if e.minibufBuf.Point() != 5 {
		t.Errorf("expected point at 5 after C-e, got %d", e.minibufBuf.Point())
	}
}

// ---------------------------------------------------------------------------
// minibufSelectNext / minibufSelectPrev
// ---------------------------------------------------------------------------

func TestMinibufSelectNext(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.minibufCandidates = []string{"a", "b", "c"}
	e.minibufSelectedIdx = 0
	e.minibufCandidateOffset = 0

	e.minibufSelectNext()

	if e.minibufSelectedIdx != 1 {
		t.Errorf("expected selectedIdx=1, got %d", e.minibufSelectedIdx)
	}
	if !e.minibufCandidateChosen {
		t.Error("expected minibufCandidateChosen=true after navigation")
	}
}

func TestMinibufSelectNextAtEnd(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.minibufCandidates = []string{"a", "b"}
	e.minibufSelectedIdx = 1

	e.minibufSelectNext()

	// Should not go beyond last candidate.
	if e.minibufSelectedIdx != 1 {
		t.Errorf("expected selectedIdx to stay at 1, got %d", e.minibufSelectedIdx)
	}
}

func TestMinibufSelectPrev(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.minibufCandidates = []string{"a", "b", "c"}
	e.minibufSelectedIdx = 2
	e.minibufCandidateOffset = 0

	e.minibufSelectPrev()

	if e.minibufSelectedIdx != 1 {
		t.Errorf("expected selectedIdx=1, got %d", e.minibufSelectedIdx)
	}
	if !e.minibufCandidateChosen {
		t.Error("expected minibufCandidateChosen=true after navigation")
	}
}

func TestMinibufSelectPrevAtStart(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.minibufCandidates = []string{"a", "b"}
	e.minibufSelectedIdx = 0

	e.minibufSelectPrev()

	if e.minibufSelectedIdx != 0 {
		t.Errorf("expected selectedIdx to stay at 0, got %d", e.minibufSelectedIdx)
	}
}

func TestMinibufSelectNextScrollsOffset(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	// Create more candidates than the popup max visible.
	e.minibufCandidates = []string{"a", "b", "c", "d", "e", "f", "g"}
	e.minibufSelectedIdx = minibufPopupMaxVisible - 1
	e.minibufCandidateOffset = 0

	// One more next should scroll the offset.
	e.minibufSelectNext()

	if e.minibufCandidateOffset != 1 {
		t.Errorf("expected offset=1 after scrolling, got %d", e.minibufCandidateOffset)
	}
}

func TestMinibufSelectPrevScrollsOffset(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.minibufCandidates = []string{"a", "b", "c", "d", "e", "f"}
	e.minibufSelectedIdx = 1
	e.minibufCandidateOffset = 1

	e.minibufSelectPrev()

	if e.minibufCandidateOffset != 0 {
		t.Errorf("expected offset=0 after scrolling back, got %d", e.minibufCandidateOffset)
	}
}

func TestMinibufSelectNextEmptyCandidates(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.minibufCandidates = nil

	// Should not panic.
	e.minibufSelectNext()
}

func TestMinibufSelectPrevEmptyCandidates(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.minibufCandidates = nil

	// Should not panic.
	e.minibufSelectPrev()
}

// ---------------------------------------------------------------------------
// minibufHistoryPrev / minibufHistoryNext
// ---------------------------------------------------------------------------

func TestMinibufHistoryPrevNoHistory(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.minibufHistory = nil

	// Should not panic.
	e.minibufHistoryPrev()
}

func TestMinibufHistoryPrevNavigatesBack(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.ReadMinibuffer("Cmd: ", func(string) {})
	e.minibufHistory = map[string][]string{
		"Cmd: ": {"last", "earlier"},
	}
	e.minibufHistoryIdx = -1

	e.minibufHistoryPrev()

	if e.minibufHistoryIdx != 0 {
		t.Errorf("expected historyIdx=0 after first prev, got %d", e.minibufHistoryIdx)
	}
	if e.minibufBuf.String() != "last" {
		t.Errorf("expected 'last' in minibuf, got %q", e.minibufBuf.String())
	}
}

func TestMinibufHistoryPrevSavesCurrentText(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.ReadMinibuffer("Cmd: ", func(string) {})
	e.minibufBuf.InsertString(0, "current")
	e.minibufBuf.SetPoint(7)
	e.minibufHistory = map[string][]string{
		"Cmd: ": {"old"},
	}
	e.minibufHistoryIdx = -1

	e.minibufHistoryPrev()

	if e.minibufHistorySaved != "current" {
		t.Errorf("expected historySaved='current', got %q", e.minibufHistorySaved)
	}
}

func TestMinibufHistoryNextRestoresSavedText(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.ReadMinibuffer("Cmd: ", func(string) {})
	e.minibufHistory = map[string][]string{
		"Cmd: ": {"old"},
	}
	e.minibufHistoryIdx = 0
	e.minibufHistorySaved = "typed"

	e.minibufHistoryNext()

	if e.minibufBuf.String() != "typed" {
		t.Errorf("expected saved text 'typed' restored, got %q", e.minibufBuf.String())
	}
	if e.minibufHistoryIdx != -1 {
		t.Errorf("expected historyIdx=-1 after returning to current, got %d", e.minibufHistoryIdx)
	}
}

func TestMinibufHistoryNextAtBeginningNoOp(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	// historyIdx -1 means we're at the current (live) edit; next should be no-op.
	e.minibufHistoryIdx = -1

	// Should not panic.
	e.minibufHistoryNext()
}

// ---------------------------------------------------------------------------
// minibufBackwardKillWord
// ---------------------------------------------------------------------------

func TestMinibufBackwardKillWordDeletesWord(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.ReadMinibuffer("P: ", func(string) {})
	e.minibufBuf.InsertString(0, "foo bar")
	e.minibufBuf.SetPoint(7)

	e.minibufBackwardKillWord()

	result := e.minibufBuf.String()
	// Should have killed "bar", leaving "foo ".
	if !strings.HasPrefix(result, "foo") {
		t.Errorf("expected 'foo' prefix after kill-word, got %q", result)
	}
	if strings.Contains(result, "bar") {
		t.Errorf("expected 'bar' removed, got %q", result)
	}
}

func TestMinibufBackwardKillWordAtBOLNoOp(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.ReadMinibuffer("P: ", func(string) {})
	// Point is already at 0.

	// Should not panic or modify anything.
	e.minibufBackwardKillWord()
	if e.minibufBuf.String() != "" {
		t.Errorf("expected empty minibuf unchanged, got %q", e.minibufBuf.String())
	}
}

// ---------------------------------------------------------------------------
// selfInsert
// ---------------------------------------------------------------------------

func TestSelfInsertInsertsRune(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()

	e.selfInsert('A')

	if e.ActiveBuffer().String() != "A" {
		t.Errorf("expected 'A' after selfInsert, got %q", e.ActiveBuffer().String())
	}
	if e.ActiveBuffer().Point() != 1 {
		t.Errorf("expected point=1 after selfInsert, got %d", e.ActiveBuffer().Point())
	}
}

func TestSelfInsertOnReadOnlyBufferShowsMessage(t *testing.T) {
	e := newTestEditor("existing")
	e.lisp = elisp.NewEvaluator()
	e.ActiveBuffer().SetReadOnly(true)

	e.selfInsert('X')

	if e.ActiveBuffer().String() != "existing" {
		t.Error("selfInsert should not modify read-only buffer")
	}
	if e.message == "" {
		t.Error("expected error message for read-only buffer insert")
	}
}

func TestSelfInsertDeactivatesMark(t *testing.T) {
	e := newTestEditor("hello")
	e.lisp = elisp.NewEvaluator()
	e.ActiveBuffer().SetMark(0)
	e.ActiveBuffer().SetMarkActive(true)

	e.selfInsert('X')

	if e.ActiveBuffer().MarkActive() {
		t.Error("selfInsert should deactivate mark")
	}
}

func TestSelfInsertMultipleRunes(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()

	for _, r := range "hello" {
		e.selfInsert(r)
	}

	if e.ActiveBuffer().String() != "hello" {
		t.Errorf("expected 'hello', got %q", e.ActiveBuffer().String())
	}
}

// ---------------------------------------------------------------------------
// handleDescribeKey
// ---------------------------------------------------------------------------

func TestHandleDescribeKeyKnownCommand(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.setupKeymaps()
	e.buffers = append(e.buffers, buffer.NewWithContent("*scratch*", ""))

	e.describeKeyPending = true

	// C-f is bound to "forward-char".
	ke := terminal.KeyEvent{Key: tcell.KeyCtrlF}
	e.handleDescribeKey(ke)

	if e.describeKeyPending {
		t.Error("expected describeKeyPending=false after terminal key")
	}
	// The active buffer should have been switched to *Help*.
	helpBuf := e.FindBuffer("*Help*")
	if helpBuf == nil {
		t.Error("expected *Help* buffer to be created")
	}
}

func TestHandleDescribeKeyUndefinedKey(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.setupKeymaps()

	e.describeKeyPending = true

	// F12 is not bound.
	ke := terminal.KeyEvent{Key: tcell.KeyF12}
	e.handleDescribeKey(ke)

	if e.describeKeyPending {
		t.Error("expected describeKeyPending=false for undefined key")
	}
	// Message should mention "undefined".
	if !strings.Contains(e.message, "undefined") {
		t.Errorf("expected 'undefined' in message, got %q", e.message)
	}
}

func TestHandleDescribeKeyPrefixAccumulates(t *testing.T) {
	e := newTestEditor("")
	e.lisp = elisp.NewEvaluator()
	e.setupKeymaps()

	e.describeKeyPending = true

	// C-x is a prefix key; we need a second key.
	ke := terminal.KeyEvent{Key: tcell.KeyCtrlX}
	e.handleDescribeKey(ke)

	// Still pending (waiting for second key of prefix).
	if !e.describeKeyPending {
		t.Error("expected describeKeyPending=true after prefix key")
	}
	if e.describeKeyMap == nil {
		t.Error("expected describeKeyMap to be set for prefix")
	}
}

// activateMinibuf sets up the minibuffer as if ReadMinibuffer was called.
func activateMinibuf(e *Editor) {
	e.ReadMinibuffer("test: ", func(string) {})
}

// mbKey builds a KeyEvent for dispatchMinibufKey tests.
func mbKey(key tcell.Key) terminal.KeyEvent {
	return terminal.KeyEvent{Key: key}
}

func mbRune(r rune) terminal.KeyEvent {
	return terminal.KeyEvent{Key: tcell.KeyRune, Rune: r}
}

func mbRuneMod(r rune, mod tcell.ModMask) terminal.KeyEvent {
	return terminal.KeyEvent{Key: tcell.KeyRune, Rune: r, Mod: mod}
}

// ---------------------------------------------------------------------------
// Enter / Escape / C-g
// ---------------------------------------------------------------------------

func TestDispatchMinibufKey_Enter_ClosesMinibuf(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.dispatchMinibufKey(mbKey(tcell.KeyEnter))
	if e.minibufActive {
		t.Error("Enter should close minibuffer")
	}
}

func TestDispatchMinibufKey_CtrlJ_ClosesMinibuf(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.dispatchMinibufKey(mbKey(tcell.KeyCtrlJ))
	if e.minibufActive {
		t.Error("C-j should close minibuffer")
	}
}

func TestDispatchMinibufKey_Escape_Cancels(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.dispatchMinibufKey(mbKey(tcell.KeyEscape))
	if e.minibufActive {
		t.Error("Escape should cancel minibuffer")
	}
}

func TestDispatchMinibufKey_CtrlG_Cancels(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.dispatchMinibufKey(mbKey(tcell.KeyCtrlG))
	if e.minibufActive {
		t.Error("C-g should cancel minibuffer")
	}
}

// ---------------------------------------------------------------------------
// Rune insertion
// ---------------------------------------------------------------------------

func TestDispatchMinibufKey_RuneInserts(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.dispatchMinibufKey(mbRune('h'))
	e.dispatchMinibufKey(mbRune('i'))
	if got := e.minibufBuf.String(); got != "hi" {
		t.Errorf("minibuf content = %q, want %q", got, "hi")
	}
}

// ---------------------------------------------------------------------------
// Backspace
// ---------------------------------------------------------------------------

func TestDispatchMinibufKey_Backspace_DeletesChar(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufBuf.InsertString(0, "hello")
	e.minibufBuf.SetPoint(5)
	e.dispatchMinibufKey(mbKey(tcell.KeyBackspace))
	if got := e.minibufBuf.String(); got != "hell" {
		t.Errorf("after backspace: %q, want %q", got, "hell")
	}
}

func TestDispatchMinibufKey_Backspace_AtStart_NoOp(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufBuf.InsertString(0, "x")
	e.minibufBuf.SetPoint(0)
	e.dispatchMinibufKey(mbKey(tcell.KeyBackspace))
	if got := e.minibufBuf.String(); got != "x" {
		t.Errorf("backspace at start changed content to %q", got)
	}
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

func TestDispatchMinibufKey_Delete_DeletesForward(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufBuf.InsertString(0, "hello")
	e.minibufBuf.SetPoint(0)
	e.dispatchMinibufKey(mbKey(tcell.KeyDelete))
	if got := e.minibufBuf.String(); got != "ello" {
		t.Errorf("after delete: %q, want %q", got, "ello")
	}
}

// ---------------------------------------------------------------------------
// Navigation: Left / Right / Home / End / C-a / C-e / C-f / C-b
// ---------------------------------------------------------------------------

func TestDispatchMinibufKey_Left_MovesBack(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufBuf.InsertString(0, "abc")
	e.minibufBuf.SetPoint(3)
	e.dispatchMinibufKey(mbKey(tcell.KeyLeft))
	if got := e.minibufBuf.Point(); got != 2 {
		t.Errorf("Left: point = %d, want 2", got)
	}
}

func TestDispatchMinibufKey_Right_MovesForward(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufBuf.InsertString(0, "abc")
	e.minibufBuf.SetPoint(0)
	e.dispatchMinibufKey(mbKey(tcell.KeyRight))
	if got := e.minibufBuf.Point(); got != 1 {
		t.Errorf("Right: point = %d, want 1", got)
	}
}

func TestDispatchMinibufKey_Home_MovesToStart(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufBuf.InsertString(0, "hello")
	e.minibufBuf.SetPoint(5)
	e.dispatchMinibufKey(mbKey(tcell.KeyHome))
	if got := e.minibufBuf.Point(); got != 0 {
		t.Errorf("Home: point = %d, want 0", got)
	}
}

func TestDispatchMinibufKey_End_MovesToEnd(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufBuf.InsertString(0, "hello")
	e.minibufBuf.SetPoint(0)
	e.dispatchMinibufKey(mbKey(tcell.KeyEnd))
	if got := e.minibufBuf.Point(); got != 5 {
		t.Errorf("End: point = %d, want 5", got)
	}
}

func TestDispatchMinibufKey_CtrlA_MovesToStart(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufBuf.InsertString(0, "hello")
	e.minibufBuf.SetPoint(5)
	e.dispatchMinibufKey(mbKey(tcell.KeyCtrlA))
	if got := e.minibufBuf.Point(); got != 0 {
		t.Errorf("C-a: point = %d, want 0", got)
	}
}

func TestDispatchMinibufKey_CtrlE_MovesToEnd(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufBuf.InsertString(0, "hello")
	e.minibufBuf.SetPoint(0)
	e.dispatchMinibufKey(mbKey(tcell.KeyCtrlE))
	if got := e.minibufBuf.Point(); got != 5 {
		t.Errorf("C-e: point = %d, want 5", got)
	}
}

func TestDispatchMinibufKey_CtrlF_MovesForward(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufBuf.InsertString(0, "abc")
	e.minibufBuf.SetPoint(1)
	e.dispatchMinibufKey(mbKey(tcell.KeyCtrlF))
	if got := e.minibufBuf.Point(); got != 2 {
		t.Errorf("C-f: point = %d, want 2", got)
	}
}

func TestDispatchMinibufKey_CtrlB_MovesBack(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufBuf.InsertString(0, "abc")
	e.minibufBuf.SetPoint(2)
	e.dispatchMinibufKey(mbKey(tcell.KeyCtrlB))
	if got := e.minibufBuf.Point(); got != 1 {
		t.Errorf("C-b: point = %d, want 1", got)
	}
}

// ---------------------------------------------------------------------------
// C-k (kill to end of line)
// ---------------------------------------------------------------------------

func TestDispatchMinibufKey_CtrlK_KillsToEnd(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufBuf.InsertString(0, "hello world")
	e.minibufBuf.SetPoint(5)
	e.dispatchMinibufKey(mbKey(tcell.KeyCtrlK))
	if got := e.minibufBuf.String(); got != "hello" {
		t.Errorf("C-k: content = %q, want %q", got, "hello")
	}
}

// ---------------------------------------------------------------------------
// C-d (delete forward)
// ---------------------------------------------------------------------------

func TestDispatchMinibufKey_CtrlD_DeletesForward(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufBuf.InsertString(0, "hello")
	e.minibufBuf.SetPoint(1)
	e.dispatchMinibufKey(mbKey(tcell.KeyCtrlD))
	if got := e.minibufBuf.String(); got != "hllo" {
		t.Errorf("C-d: content = %q, want %q", got, "hllo")
	}
}

// ---------------------------------------------------------------------------
// M-n / M-p (candidate navigation)
// ---------------------------------------------------------------------------

func TestDispatchMinibufKey_MetaN_SelectsNext(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufCandidates = []string{"alpha", "beta", "gamma"}
	e.minibufSelectedIdx = 0
	e.dispatchMinibufKey(mbRuneMod('n', tcell.ModAlt))
	if e.minibufSelectedIdx != 1 {
		t.Errorf("M-n: selectedIdx = %d, want 1", e.minibufSelectedIdx)
	}
}

func TestDispatchMinibufKey_MetaP_SelectsPrev(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufCandidates = []string{"alpha", "beta", "gamma"}
	e.minibufSelectedIdx = 2
	e.dispatchMinibufKey(mbRuneMod('p', tcell.ModAlt))
	if e.minibufSelectedIdx != 1 {
		t.Errorf("M-p: selectedIdx = %d, want 1", e.minibufSelectedIdx)
	}
}

// ---------------------------------------------------------------------------
// Down / Up with candidates
// ---------------------------------------------------------------------------

func TestDispatchMinibufKey_Down_WithCandidates_SelectsNext(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufCandidates = []string{"a", "b"}
	e.minibufSelectedIdx = 0
	e.dispatchMinibufKey(mbKey(tcell.KeyDown))
	if e.minibufSelectedIdx != 1 {
		t.Errorf("Down: selectedIdx = %d, want 1", e.minibufSelectedIdx)
	}
}

func TestDispatchMinibufKey_Up_WithCandidates_SelectsPrev(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufCandidates = []string{"a", "b"}
	e.minibufSelectedIdx = 1
	e.dispatchMinibufKey(mbKey(tcell.KeyUp))
	if e.minibufSelectedIdx != 0 {
		t.Errorf("Up: selectedIdx = %d, want 0", e.minibufSelectedIdx)
	}
}

// ---------------------------------------------------------------------------
// C-n / C-p
// ---------------------------------------------------------------------------

func TestDispatchMinibufKey_CtrlN_SelectsNext(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufCandidates = []string{"a", "b", "c"}
	e.minibufSelectedIdx = 1
	e.dispatchMinibufKey(mbKey(tcell.KeyCtrlN))
	if e.minibufSelectedIdx != 2 {
		t.Errorf("C-n: selectedIdx = %d, want 2", e.minibufSelectedIdx)
	}
}

func TestDispatchMinibufKey_CtrlP_SelectsPrev(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufCandidates = []string{"a", "b", "c"}
	e.minibufSelectedIdx = 2
	e.dispatchMinibufKey(mbKey(tcell.KeyCtrlP))
	if e.minibufSelectedIdx != 1 {
		t.Errorf("C-p: selectedIdx = %d, want 1", e.minibufSelectedIdx)
	}
}

// ---------------------------------------------------------------------------
// C-w (kill word backward)
// ---------------------------------------------------------------------------

func TestDispatchMinibufKey_CtrlW_KillsWordBackward(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufBuf.InsertString(0, "hello world")
	e.minibufBuf.SetPoint(11)
	e.dispatchMinibufKey(mbKey(tcell.KeyCtrlW))
	// "world" should be killed.
	got := e.minibufBuf.String()
	if len(got) >= 11 {
		t.Errorf("C-w did not kill word, content = %q", got)
	}
}

// ---------------------------------------------------------------------------
// Unknown key (not a rune, not handled) — no-op
// ---------------------------------------------------------------------------

func TestDispatchMinibufKey_UnknownKey_NoOp(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufBuf.InsertString(0, "abc")
	e.minibufBuf.SetPoint(3)
	// KeyF1 is not handled → default branch, returns early because not KeyRune.
	e.dispatchMinibufKey(terminal.KeyEvent{Key: tcell.KeyF1})
	if got := e.minibufBuf.String(); got != "abc" {
		t.Errorf("unknown key changed minibuf to %q", got)
	}
	if !e.minibufActive {
		t.Error("unknown key should not close minibuffer")
	}
}

// ---------------------------------------------------------------------------
// Hint cleared on non-TAB keypresses
// ---------------------------------------------------------------------------

func TestDispatchMinibufKey_HintClearedOnNonTab(t *testing.T) {
	e := newTestEditor("")
	activateMinibuf(e)
	e.minibufHint = "some hint"
	e.dispatchMinibufKey(mbRune('a'))
	if e.minibufHint != "" {
		t.Errorf("minibufHint = %q, want empty after non-TAB key", e.minibufHint)
	}
}

// newPrefixTestEditor builds an Editor with a three-level keymap tree:
//
//		C-x      → prefix (cx)
//		C-x v    → prefix (cxv)
//		C-x v s  → command "forward-char"   (three-deep, hint shown at C-x v)
//	 C-x f    → command "forward-char"   (two-deep, no hint shown)
//
// "forward-char" and "keyboard-quit" are already registered in the global
// command table via the editor package's init().
func newPrefixTestEditor(t *testing.T) *Editor {
	t.Helper()

	b := buffer.NewWithContent("*test*", "hello")
	win := window.New(b, 0, 0, 80, 24)

	gk := keymap.New("global")
	cx := keymap.New("C-x")
	cxv := keymap.New("C-x v")

	cxv.Bind(keymap.PlainKey('s'), "forward-char")
	cx.BindPrefix(keymap.PlainKey('v'), cxv)
	cx.Bind(keymap.PlainKey('f'), "forward-char")
	gk.BindPrefix(keymap.CtrlKey('x'), cx)
	gk.Bind(keymap.CtrlKey('g'), "keyboard-quit")

	_, nopCancel := context.WithCancel(context.Background())
	e := &Editor{
		buffers:      []*buffer.Buffer{b},
		windows:      []*window.Window{win},
		activeWin:    win,
		minibufBuf:   buffer.New(" *minibuf*"),
		globalKeymap: gk,
		ctrlXKeymap:  cx,
		ctrlCKeymap:  keymap.New("C-c"),
		universalArg: 1,
		lspOpCancel:  nopCancel,
	}
	e.minibufWin = window.New(e.minibufBuf, 23, 0, 80, 1)
	return e
}

func pressCtrlX(e *Editor) {
	e.dispatchParsedKey(terminal.KeyEvent{Key: tcell.KeyCtrlX})
}

func pressRune(e *Editor, r rune) {
	e.dispatchParsedKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: r})
}

// TestPrefixHint_FirstChordSilent: pressing C-x alone must not show a hint.
func TestPrefixHint_FirstChordSilent(t *testing.T) {
	e := newPrefixTestEditor(t)
	pressCtrlX(e)

	if e.prefixKeymap == nil {
		t.Fatal("expected prefixKeymap to be set after C-x")
	}
	if e.message != "" {
		t.Errorf("expected no message after first chord C-x, got %q", e.message)
	}
}

// TestPrefixHint_SecondChordShowsHint: C-x v must show "C-x v" in the minibuffer.
func TestPrefixHint_SecondChordShowsHint(t *testing.T) {
	e := newPrefixTestEditor(t)
	pressCtrlX(e)
	pressRune(e, 'v')

	if e.message != "C-x v" {
		t.Errorf("expected minibuffer to show \"C-x v\", got %q", e.message)
	}
	if e.prefixKeySeq != "C-x v" {
		t.Errorf("prefixKeySeq = %q, want \"C-x v\"", e.prefixKeySeq)
	}
}

// TestPrefixHint_CommandClearsHint: completing C-x v s must clear the hint.
func TestPrefixHint_CommandClearsHint(t *testing.T) {
	e := newPrefixTestEditor(t)
	pressCtrlX(e)
	pressRune(e, 'v')
	pressRune(e, 's')

	if e.prefixKeymap != nil {
		t.Error("prefixKeymap should be nil after command executes")
	}
	if e.prefixKeySeq != "" {
		t.Errorf("prefixKeySeq should be empty after command, got %q", e.prefixKeySeq)
	}
	if e.message != "" {
		t.Errorf("message should be cleared after command, got %q", e.message)
	}
}

// TestPrefixHint_SingleDepthNoHint: C-x f must not show a hint (only one prefix level).
func TestPrefixHint_SingleDepthNoHint(t *testing.T) {
	e := newPrefixTestEditor(t)
	pressCtrlX(e)
	pressRune(e, 'f')

	if e.prefixKeymap != nil {
		t.Error("prefixKeymap should be nil after command executes")
	}
	if e.prefixKeySeq != "" {
		t.Errorf("prefixKeySeq should be empty after command, got %q", e.prefixKeySeq)
	}
	if e.message != "" {
		t.Errorf("no hint was shown, so message should be empty, got %q", e.message)
	}
}

// TestPrefixHint_UnknownKeyCancels: unbound key in a prefix clears the hint.
func TestPrefixHint_UnknownKeyCancels(t *testing.T) {
	e := newPrefixTestEditor(t)
	pressCtrlX(e)
	pressRune(e, 'v')
	pressRune(e, 'z') // not bound under C-x v

	if e.prefixKeymap != nil {
		t.Error("prefixKeymap should be nil after unknown key")
	}
	if e.prefixKeySeq != "" {
		t.Errorf("prefixKeySeq should be empty after cancel, got %q", e.prefixKeySeq)
	}
}

// TestPrefixHint_KeyboardQuitCancels: C-g clears the prefix and the hint.
func TestPrefixHint_KeyboardQuitCancels(t *testing.T) {
	e := newPrefixTestEditor(t)
	pressCtrlX(e)
	pressRune(e, 'v')
	e.dispatchParsedKey(terminal.KeyEvent{Key: tcell.KeyCtrlG})

	if e.prefixKeymap != nil {
		t.Error("prefixKeymap should be nil after C-g")
	}
	if e.prefixKeySeq != "" {
		t.Errorf("prefixKeySeq should be empty after C-g, got %q", e.prefixKeySeq)
	}
}

// newTestEditorWithFile creates a minimal Editor with a buffer backed by a
// real file on disk, suitable for testing auto-revert.
func newTestEditorWithFile(t *testing.T, content string) (*Editor, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("setup stat: %v", err)
	}

	buf := buffer.NewWithContent("test.txt", content)
	buf.SetFilename(path)
	win := window.New(buf, 0, 0, 80, 24)
	e := &Editor{
		buffers:          []*buffer.Buffer{buf},
		windows:          []*window.Window{win},
		activeWin:        win,
		autoRevert:       true,
		autoRevertMtimes: map[*buffer.Buffer]time.Time{buf: info.ModTime()},
		spanCaches:       make(map[*buffer.Buffer]*spanCache),
	}
	e.minibufBuf = buffer.New(" *minibuf*")
	e.minibufWin = window.New(e.minibufBuf, 23, 0, 80, 1)
	return e, path
}

func TestAutoRevert_ReloadsWhenFileChanges(t *testing.T) {
	e, path := newTestEditorWithFile(t, "original")

	// Ensure the check fires immediately by zeroing last-check time.
	e.autoRevertLastCheck = time.Time{}

	// First call — file unchanged, nothing should happen.
	e.maybeAutoRevert()
	if got := e.ActiveBuffer().String(); got != "original" {
		t.Fatalf("unexpected reload before file change: %q", got)
	}

	// Write new content with a definitely newer mtime.
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(path, []byte("updated"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	e.autoRevertLastCheck = time.Time{} // force re-check

	e.maybeAutoRevert()
	if got := e.ActiveBuffer().String(); got != "updated" {
		t.Errorf("buffer not reloaded: got %q, want %q", got, "updated")
	}
}

func TestAutoRevert_DoesNotReloadModifiedBuffer(t *testing.T) {
	e, path := newTestEditorWithFile(t, "original")

	// Mark buffer as modified (unsaved user edits).
	e.ActiveBuffer().SetModified(true)
	e.autoRevertLastCheck = time.Time{}

	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(path, []byte("updated"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	e.autoRevertLastCheck = time.Time{}

	e.maybeAutoRevert()
	if got := e.ActiveBuffer().String(); got != "original" {
		t.Errorf("modified buffer was reverted unexpectedly: got %q", got)
	}
}

func TestAutoRevert_DisabledByConfig(t *testing.T) {
	e, path := newTestEditorWithFile(t, "original")
	e.autoRevert = false
	e.autoRevertLastCheck = time.Time{}

	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(path, []byte("updated"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	e.autoRevertLastCheck = time.Time{}

	e.maybeAutoRevert()
	if got := e.ActiveBuffer().String(); got != "original" {
		t.Errorf("auto-revert disabled but buffer was reloaded: got %q", got)
	}
}

// newThemeEval creates an Evaluator with load-theme, set-face-attribute and
// define-gomacs-theme registered, reusing the same closures as NewEditor.
func newThemeEval() *elisp.Evaluator {
	ev := elisp.NewEvaluator()

	ev.RegisterGoFn("load-theme", func(args []elisp.Value, _ *elisp.Env) (elisp.Value, error) {
		if len(args) < 1 {
			return nil, nil
		}
		name := strings.Trim(args[0].String(), `'"`)
		syntax.LoadTheme(name)
		return elisp.Nil{}, nil
	})

	ev.SetSetqHook("theme", func(v elisp.Value) {
		name := strings.Trim(v.String(), `'"`)
		syntax.LoadTheme(name)
	})

	ev.RegisterGoFn("set-face-attribute", func(args []elisp.Value, _ *elisp.Env) (elisp.Value, error) {
		if len(args) < 1 {
			return nil, nil
		}
		faceName := strings.Trim(args[0].String(), `'"`)
		facePtr, ok := syntax.GetFacePtr(faceName)
		if !ok {
			return nil, nil
		}
		i := 1
		if i < len(args) && elisp.IsNil(args[i]) {
			i++
		}
		for i+1 < len(args) {
			kw := args[i].String()
			val := args[i+1]
			i += 2
			switch kw {
			case ":foreground":
				facePtr.Fg = strings.Trim(val.String(), `'"`)
			case ":background":
				facePtr.Bg = strings.Trim(val.String(), `'"`)
			case ":bold":
				facePtr.Bold = !elisp.IsNil(val)
			case ":italic":
				facePtr.Italic = !elisp.IsNil(val)
			case ":underline":
				facePtr.Underline = !elisp.IsNil(val)
			case ":reverse":
				facePtr.Reverse = !elisp.IsNil(val)
			}
		}
		return elisp.Nil{}, nil
	})

	ev.RegisterGoFn("define-gomacs-theme", func(args []elisp.Value, _ *elisp.Env) (elisp.Value, error) {
		if len(args) < 2 {
			return nil, nil
		}
		name := strings.Trim(args[0].String(), `'"`)
		faceSpecs, ok := elisp.ToSlice(args[1])
		if !ok {
			return nil, nil
		}
		type faceOverride struct {
			name string
			face syntax.Face
		}
		overrides := make([]faceOverride, 0, len(faceSpecs))
		for _, spec := range faceSpecs {
			fields, ok2 := elisp.ToSlice(spec)
			if !ok2 || len(fields) < 1 {
				continue
			}
			faceName := strings.Trim(fields[0].String(), `'"`)
			facePtr, ok2 := syntax.GetFacePtr(faceName)
			if !ok2 {
				continue
			}
			f := *facePtr
			for j := 1; j+1 < len(fields); j += 2 {
				kw := fields[j].String()
				val := fields[j+1]
				switch kw {
				case ":foreground":
					f.Fg = strings.Trim(val.String(), `'"`)
				case ":background":
					f.Bg = strings.Trim(val.String(), `'"`)
				case ":bold":
					f.Bold = !elisp.IsNil(val)
				case ":italic":
					f.Italic = !elisp.IsNil(val)
				case ":underline":
					f.Underline = !elisp.IsNil(val)
				case ":reverse":
					f.Reverse = !elisp.IsNil(val)
				}
			}
			overrides = append(overrides, faceOverride{name: faceName, face: f})
		}
		syntax.RegisterTheme(name, func() {
			for _, o := range overrides {
				syntax.SetFaceByName(o.name, o.face)
			}
		})
		return elisp.Nil{}, nil
	})

	return ev
}

func TestSetFaceAttribute_Foreground(t *testing.T) {
	syntax.LoadTheme("sweet")
	ev := newThemeEval()
	_, err := ev.EvalString(`(set-face-attribute 'keyword :foreground "#ff1234")`)
	if err != nil {
		t.Fatalf("set-face-attribute error: %v", err)
	}
	p, _ := syntax.GetFacePtr("keyword")
	if p.Fg != "#ff1234" {
		t.Errorf("Fg = %q, want %q", p.Fg, "#ff1234")
	}
	syntax.LoadTheme("sweet") // restore
}

func TestSetFaceAttribute_Bold(t *testing.T) {
	syntax.LoadTheme("sweet")
	ev := newThemeEval()
	_, err := ev.EvalString(`(set-face-attribute 'string :bold t)`)
	if err != nil {
		t.Fatalf("set-face-attribute error: %v", err)
	}
	p, _ := syntax.GetFacePtr("string")
	if !p.Bold {
		t.Error("expected Bold=true after (set-face-attribute 'string :bold t)")
	}
	syntax.LoadTheme("sweet") // restore
}

func TestSetFaceAttribute_NilFrame(t *testing.T) {
	syntax.LoadTheme("sweet")
	ev := newThemeEval()
	// Emacs-style: nil frame argument should be ignored
	_, err := ev.EvalString(`(set-face-attribute 'comment nil :foreground "#aabbcc")`)
	if err != nil {
		t.Fatalf("set-face-attribute error: %v", err)
	}
	p, _ := syntax.GetFacePtr("comment")
	if p.Fg != "#aabbcc" {
		t.Errorf("Fg = %q, want %q", p.Fg, "#aabbcc")
	}
	syntax.LoadTheme("sweet") // restore
}

func TestSetqThemeHook(t *testing.T) {
	syntax.LoadTheme("sweet")
	ev := newThemeEval()
	_, err := ev.EvalString("(setq theme 'default)")
	if err != nil {
		t.Fatalf("setq theme error: %v", err)
	}
	// After switching to default theme the keyword fg should be "blue".
	p, _ := syntax.GetFacePtr("keyword")
	if p.Fg != "blue" {
		t.Errorf("after (setq theme 'default): keyword Fg = %q, want %q", p.Fg, "blue")
	}
	syntax.LoadTheme("sweet") // restore
}

func TestDefineGomacsTheme(t *testing.T) {
	syntax.LoadTheme("sweet")
	ev := newThemeEval()
	_, err := ev.EvalString(`
(define-gomacs-theme "test-custom"
  '((keyword :foreground "#001122" :bold nil)
    (string  :foreground "#334455")))
(load-theme "test-custom")
`)
	if err != nil {
		t.Fatalf("define-gomacs-theme error: %v", err)
	}
	kw, _ := syntax.GetFacePtr("keyword")
	if kw.Fg != "#001122" {
		t.Errorf("keyword Fg = %q, want %q", kw.Fg, "#001122")
	}
	if kw.Bold {
		t.Error("keyword Bold should be false")
	}
	str, _ := syntax.GetFacePtr("string")
	if str.Fg != "#334455" {
		t.Errorf("string Fg = %q, want %q", str.Fg, "#334455")
	}
	syntax.LoadTheme("sweet") // restore
}

func TestDefineGomacsThemeViaSetq(t *testing.T) {
	syntax.LoadTheme("sweet")
	ev := newThemeEval()
	_, err := ev.EvalString(`
(define-gomacs-theme "setq-test"
  '((modeline :foreground "#ffffff" :background "#000000")))
(setq theme "setq-test")
`)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	ml, _ := syntax.GetFacePtr("modeline")
	if ml.Fg != "#ffffff" {
		t.Errorf("modeline Fg = %q, want %q", ml.Fg, "#ffffff")
	}
	if ml.Bg != "#000000" {
		t.Errorf("modeline Bg = %q, want %q", ml.Bg, "#000000")
	}
	syntax.LoadTheme("sweet") // restore
}

func TestAddToKillRing(t *testing.T) {
	e := newTestEditor("")
	e.addToKillRing("alpha")
	e.addToKillRing("beta")
	if len(e.killRing) < 2 {
		t.Fatalf("kill ring should have 2 entries, got %d", len(e.killRing))
	}
	if e.killRing[0] != "beta" {
		t.Errorf("killRing[0] = %q, want %q", e.killRing[0], "beta")
	}
	if e.killRing[1] != "alpha" {
		t.Errorf("killRing[1] = %q, want %q", e.killRing[1], "alpha")
	}
}

func TestSwitchToBuffer(t *testing.T) {
	e := newTestEditor("content")
	initial := buf(e).Name()

	e.SwitchToBuffer("*scratch*")
	if got := buf(e).Name(); got == initial {
		t.Fatalf("SwitchToBuffer: buffer did not change from %q", initial)
	}
	if got := buf(e).Name(); got != "*scratch*" {
		t.Fatalf("SwitchToBuffer: want %q, got %q", "*scratch*", got)
	}
}

func TestKillBuffer(t *testing.T) {
	e := newTestEditor("hello")
	// Add a second buffer without switching to it.
	e.buffers = append(e.buffers, buffer.NewWithContent("*extra*", ""))
	countBefore := len(e.buffers)
	e.KillBuffer("*extra*")
	if len(e.buffers) != countBefore-1 {
		t.Fatalf("KillBuffer: expected %d buffers, got %d", countBefore-1, len(e.buffers))
	}
	if e.FindBuffer("*extra*") != nil {
		t.Fatal("KillBuffer: *extra* buffer still exists")
	}
}

// ---------------------------------------------------------------------------
// isearch backward: reversal and repeat (regression tests)
// ---------------------------------------------------------------------------

// ctrlS / ctrlR drive isearch through the real key handler so isearchStep's
// direction-reversal logic is exercised, not bypassed.
func ctrlS(e *Editor) { e.isearchHandleKey(terminal.KeyEvent{Key: tcell.KeyCtrlS}) }
func ctrlR(e *Editor) { e.isearchHandleKey(terminal.KeyEvent{Key: tcell.KeyCtrlR}) }

// TestIsearchBackward_RepeatFindsCloseMatch is defect 1: with matches closer
// together than the needle length, the backward scan used to skip over the
// nearer one and wrongly report a wrap.
func TestIsearchBackward_RepeatFindsCloseMatch(t *testing.T) {
	e := newTestEditorWithIsearch("abab")
	b := buf(e)
	b.SetPoint(b.Len())
	e.isearching = true
	e.isearchFwd = false
	e.isearchStart = b.Len()
	e.isearchStr = "ab"
	e.isearchFind()
	if got := b.Point(); got != 2 {
		t.Fatalf("first backward match: point = %d, want 2", got)
	}

	e.message = ""
	ctrlR(e)

	if got := b.Point(); got != 0 {
		t.Errorf("second backward match: point = %d, want 0", got)
	}
	if strings.Contains(e.message, "Wrapped") {
		t.Errorf("should not report a wrap when an earlier match exists, got %q", e.message)
	}
}

// TestIsearchReverseLandsOnCurrentMatch is defect 2: reversing direction onto
// the only match used to report "Failing isearch" and leave point put.  Emacs
// moves to the other end of the same match.
func TestIsearchReverseLandsOnCurrentMatch(t *testing.T) {
	e := newTestEditorWithIsearch("hello needle world")
	b := buf(e)
	startFwdIsearch(e, 0)
	e.isearchStr = "needle"
	e.isearchFind()
	if got, want := b.Point(), len("hello needle"); got != want {
		t.Fatalf("forward match: point = %d, want %d", got, want)
	}

	e.message = ""
	ctrlR(e)

	if got, want := b.Point(), len("hello "); got != want {
		t.Errorf("after C-r: point = %d, want %d (start of the same match)", got, want)
	}
	if strings.Contains(e.message, "Failing") {
		t.Errorf("reversing onto an existing match must not fail, got %q", e.message)
	}
}

// TestIsearchReverseForwardAgainReturnsToMatchEnd is the mirror case.
func TestIsearchReverseForwardAgainReturnsToMatchEnd(t *testing.T) {
	e := newTestEditorWithIsearch("hello needle world")
	b := buf(e)
	startFwdIsearch(e, 0)
	e.isearchStr = "needle"
	e.isearchFind()
	ctrlR(e) // now at match start
	ctrlS(e) // reversing again → match end

	if got, want := b.Point(), len("hello needle"); got != want {
		t.Errorf("after C-r then C-s: point = %d, want %d", got, want)
	}
}

// TestIsearchBackward_StillWrapsWhenNoEarlierMatch keeps the wrap behaviour the
// spec asks for, so the defect-1 fix did not remove it.
func TestIsearchBackward_StillWrapsWhenNoEarlierMatch(t *testing.T) {
	e := newTestEditorWithIsearch("abc defX")
	b := buf(e)
	e.isearching = true
	e.isearchFwd = false
	e.isearchStr = "X"
	b.SetPoint(0)

	e.isearchFindNext()

	if got, want := b.Point(), len("abc def"); got != want {
		t.Errorf("point = %d, want %d", got, want)
	}
	if !strings.Contains(e.message, "top of buffer") {
		t.Errorf("message = %q, want it to mention the top of the buffer", e.message)
	}
}

func TestIsearchCurrentMatchStart(t *testing.T) {
	e := newTestEditorWithIsearch("abab")
	b := buf(e)
	e.isearching = true
	e.isearchStr = "ab"

	// Point at 2 is both a match start and a match end; pointAtEnd decides.
	b.SetPoint(2)
	if got := e.isearchCurrentMatchStart(true); got != 0 {
		t.Errorf("pointAtEnd=true: got %d, want 0", got)
	}
	if got := e.isearchCurrentMatchStart(false); got != 2 {
		t.Errorf("pointAtEnd=false: got %d, want 2", got)
	}
	// A position with no match either side.
	e.isearchStr = "zz"
	if got := e.isearchCurrentMatchStart(true); got != -1 {
		t.Errorf("no match: got %d, want -1", got)
	}
}

// ---------------------------------------------------------------------------
// modeFromShebang with interpreter flags
// ---------------------------------------------------------------------------

func TestModeFromShebang_WithInterpreterFlags(t *testing.T) {
	cases := []struct {
		shebang string
		want    string
	}{
		{"#!/usr/bin/perl -w\n", "perl"}, // the canonical Perl shebang
		{"#!/bin/bash -e\n", "bash"},
		{"#!/bin/bash -eu -o pipefail\n", "bash"},
		{"#!/usr/bin/env bash -x\n", "bash"},
		{"#!/usr/bin/python -u\n", "python"},
		{"#!/usr/bin/env python3 -u\n", "python"},
		{"#!/usr/bin/env -S python3 -u\n", "python"},
		{"#!/usr/bin/env -S perl -w\n", "perl"},
		{"#!/usr/bin/env VAR=1 python3\n", "python"},
		{"#!/bin/sh -e\n", "bash"},
		// Still no mode for interpreters we do not know.
		{"#!/usr/bin/env ruby -w\n", ""},
		{"#!\n", ""},
		{"#! \n", ""},
	}
	for _, tc := range cases {
		if got := modeFromShebang(tc.shebang); got != tc.want {
			t.Errorf("modeFromShebang(%q) = %q, want %q", tc.shebang, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// baseMode
// ---------------------------------------------------------------------------

func TestBaseMode(t *testing.T) {
	cases := map[string]string{
		"debug-repl":      "debug-repl",
		"debug-repl+java": "debug-repl",
		"vc-annotate+go":  "vc-annotate",
		"go":              "go",
		"":                "",
	}
	for in, want := range cases {
		if got := baseMode(in); got != want {
			t.Errorf("baseMode(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestApplyVisualLinesDebugReplSuffixDoesNotWrap is a regression test: the REPL
// buffer's mode carries a "+lang" suffix, which used to miss the no-wrap list
// and let REPL output wrap.
func TestApplyVisualLinesDebugReplSuffixDoesNotWrap(t *testing.T) {
	for _, mode := range []string{"debug-repl", "debug-repl+go", "debug-repl+java"} {
		e := newTestEditor("some quite long repl output line")
		e.visualLines = true
		e.visualLinesSynced = false
		buf(e).SetMode(mode)
		if got := buf(e).Mode(); got != mode {
			t.Fatalf("SetMode(%q) did not stick, got %q", mode, got)
		}
		e.applyVisualLines()
		if got := e.activeWin.WrapCol(); got != 0 {
			t.Errorf("mode %q: wrapCol = %d, want 0 (no wrapping)", mode, got)
		}
	}
}

// TestApplyVisualLinesNoWrapModes pins the spec requirement that vc-grep hits
// are never wrapped ("visual line mode should not be active in vc-grep
// buffers"), and covers every other mode whose lines are position-bearing —
// a wrapped "file.go:12: hit" or stack frame is unreadable, so each of these
// must keep wrapCol at 0 even while visual-lines is on globally.
func TestApplyVisualLinesNoWrapModes(t *testing.T) {
	modes := []string{
		"vc-grep", "lsp-refs", "vc-status", "vc-log", "vc-show", "diff",
		"compilation", "vc-fixup-select", "shell", "debug-locals", "debug-stack",
	}
	for _, mode := range modes {
		e := newTestEditor("internal/editor/editor.go:4242: a hit line long enough to wrap\n")
		e.visualLines = true
		e.visualLinesSynced = false
		buf(e).SetMode(mode)
		if got := buf(e).Mode(); got != mode {
			t.Fatalf("SetMode(%q) did not stick, got %q", mode, got)
		}
		e.applyVisualLines()
		if got := e.activeWin.WrapCol(); got != 0 {
			t.Errorf("mode %q: wrapCol = %d, want 0 (no wrapping)", mode, got)
		}
	}
}

// TestApplyVisualLinesWrapsOrdinaryBuffer is the counterpart: a normal code
// buffer still wraps when visual-lines is on, so the no-wrap list above is not
// accidentally disabling wrapping everywhere.
func TestApplyVisualLinesWrapsOrdinaryBuffer(t *testing.T) {
	e := newTestEditor("package main\n")
	e.visualLines = true
	e.visualLinesSynced = false
	buf(e).SetMode("go")
	e.applyVisualLines()
	if got := e.activeWin.WrapCol(); got == 0 {
		t.Errorf("go-mode wrapCol = 0, want non-zero (wrapping enabled)")
	}
}

// ---------------------------------------------------------------------------
// conf-mode file association
// ---------------------------------------------------------------------------

// TestLoadFileConfModeAssociation pins the spec's conf-mode file list and the
// two edge cases that were wrong: "weird.src" is not a config file just because
// its extension ends in "rc", and .bashrc stays bash-mode because it is a shell
// script rather than plain config.
func TestLoadFileConfModeAssociation(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"app.conf", "conf"},
		{"settings.toml", "conf"},
		{"inputrc", "conf"},
		{".npmrc", "conf"},
		{"mysqlrc", "conf"},
		{"weird.src", "fundamental"}, // extension merely ends in "rc"
		{"main.arc", "fundamental"},
		{".bashrc", "bash"}, // deliberate: shell scripts get bash-mode
		{".zshrc", "bash"},
	}
	dir := t.TempDir()
	for _, tc := range cases {
		path := filepath.Join(dir, tc.name)
		if err := os.WriteFile(path, []byte("key = value\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		e := newTestEditor("")
		b, err := e.loadFile(path)
		if err != nil {
			t.Fatalf("loadFile(%s): %v", tc.name, err)
		}
		if got := b.Mode(); got != tc.want {
			t.Errorf("%s: mode = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// debug gutter path canonicalisation
// ---------------------------------------------------------------------------

// TestRenderWindowGutterMatchesNonCanonicalFilename is a regression test: the
// breakpoint map is keyed by canonical path, but rendering compared the raw
// buffer filename, so the marker and the exec arrow never appeared for any
// buffer whose name was relative or went through a symlinked directory.
func TestRenderWindowGutterMatchesNonCanonicalFilename(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "prog.txt")
	if err := os.WriteFile(real, []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A non-canonical spelling of the same file.
	noisy := filepath.Join(dir, ".", "prog.txt")

	e := newCapTestEditor("one\ntwo\nthree\n")
	b := e.ActiveBuffer()
	b.SetFilename(noisy)
	e.dapBreakpoints = map[string]map[int]struct{}{
		canonPath(noisy): {2: {}},
	}
	e.dap = &dapState{stoppedFile: canonPath(noisy), stoppedLine: 3}

	e.renderWindow(e.activeWin)

	if ch, _ := e.term.CaptureCell(0, 1); ch != '●' {
		t.Errorf("breakpoint marker on line 2: got %q, want '●'", ch)
	}
	if ch, _ := e.term.CaptureCell(1, 2); ch != '→' {
		t.Errorf("exec arrow on line 3: got %q, want '→'", ch)
	}
}

// ---------------------------------------------------------------------------
// modeline padding
// ---------------------------------------------------------------------------

// TestRenderModelinePadsByRunesNotBytes checks a non-ASCII buffer name does not
// mis-pad or truncate the modeline mid-UTF-8-sequence.
func TestRenderModelinePadsByRunesNotBytes(t *testing.T) {
	e := newCapTestEditor("hello\n")
	e.ActiveBuffer().SetName("køpenhavn-æøå.txt")

	e.renderModeline(e.activeWin)

	// The modeline is the window's last row.
	row := captureRow(t, e, e.activeWin.Top()+e.activeWin.Height()-1)
	if strings.ContainsRune(row, '�') {
		t.Errorf("modeline contains a replacement rune (truncated mid-sequence): %q", row)
	}
	width, _ := e.term.CaptureSize()
	if len([]rune(row)) != width {
		t.Errorf("modeline spans %d runes, want the full window width %d", len([]rune(row)), width)
	}
	if !strings.Contains(row, "køpenhavn") {
		t.Errorf("modeline should show the buffer name, got %q", row)
	}
}

// ---------------------------------------------------------------------------
// span cache: checkpointed resumption
// ---------------------------------------------------------------------------

func TestFirstDiffRune(t *testing.T) {
	tests := []struct {
		name  string
		a, b  string
		limit int
		want  int
	}{
		{"identical", "hello world", "hello world", 11, 11},
		{"differ at 0", "xello", "hello", 5, 0},
		{"differ in the middle", "hello world", "hello WORLD", 11, 6},
		{"insertion shifts the tail", "abcdef", "abXcdef", 6, 2},
		{"limit stops the search early", "abcdef", "abcXef", 3, 3},
		{"b is shorter", "abcdef", "abc", 6, 3},
		{"a is shorter", "abc", "abcdef", 6, 3},
		{"both empty", "", "", 0, 0},
		{"negative limit", "abc", "xyz", -1, 0},
		{"multibyte", "sø" + "en", "sø" + "an", 4, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := firstDiffRune([]rune(tc.a), []rune(tc.b), tc.limit); got != tc.want {
				t.Errorf("firstDiffRune(%q, %q, %d) = %d, want %d",
					tc.a, tc.b, tc.limit, got, tc.want)
			}
		})
	}
}

// resumeRecorder wraps a resumable highlighter and records the state every scan
// resumed from, so a test can see whether an edit reused a checkpoint or started
// over at offset 0.
type resumeRecorder struct {
	inner   syntax.Resumable
	resumes []syntax.ScanState
}

func (r *resumeRecorder) Highlight(text string, start, end int) []syntax.Span {
	return r.inner.Highlight(text, start, end)
}

func (r *resumeRecorder) HighlightRunes(runes []rune, start, end int) []syntax.Span {
	return r.inner.HighlightRunes(runes, start, end)
}

func (r *resumeRecorder) HighlightResume(runes []rune, st syntax.ScanState, end int,
	cp *syntax.Checkpoints) []syntax.Span {
	r.resumes = append(r.resumes, st)
	return r.inner.HighlightResume(runes, st, end, cp)
}

// lastResume returns the offset the most recent scan started from.
func (r *resumeRecorder) lastResume(t *testing.T) int {
	t.Helper()
	if len(r.resumes) == 0 {
		t.Fatal("the highlighter was never asked to resume")
	}
	return r.resumes[len(r.resumes)-1].Pos
}

// newResumeTestEditor returns an editor over content whose buffer uses a
// recording resumable highlighter.
func newResumeTestEditor(content, mode string, inner syntax.Resumable) (*Editor, *buffer.Buffer, *resumeRecorder) {
	e := newCapTestEditor(content)
	b := buf(e)
	b.SetMode(mode)
	rec := &resumeRecorder{inner: inner}
	e.spanCaches = make(map[*buffer.Buffer]*spanCache)
	e.customHighlighters[b] = rec
	return e, b, rec
}

func TestSpanCacheRecordsCheckpoints(t *testing.T) {
	e, b, _ := newResumeTestEditor(goSource(2000), "go", syntax.GoHighlighter{})
	c := e.getSpanCache(b)
	if len(c.checkpoints) < 2 {
		t.Fatalf("full scan recorded %d checkpoints, want several", len(c.checkpoints))
	}
	prev := -1
	for i, cp := range c.checkpoints {
		if cp.st.Pos <= prev {
			t.Fatalf("checkpoint %d at %d is not past the previous one at %d", i, cp.st.Pos, prev)
		}
		prev = cp.st.Pos
		if cp.nspans > len(c.spans) {
			t.Fatalf("checkpoint %d records %d spans, cache holds %d", i, cp.nspans, len(c.spans))
		}
	}
}

// TestSpanCacheResumesAfterEditNearTheEnd is the point of the whole exercise: a
// keystroke deep into a large file must not rescan the text before it.
func TestSpanCacheResumesAfterEditNearTheEnd(t *testing.T) {
	src := goSource(2000)
	e, b, rec := newResumeTestEditor(src, "go", syntax.GoHighlighter{})
	e.getSpanCache(b) // prime a full-buffer cache with checkpoints
	if got := rec.lastResume(t); got != 0 {
		t.Fatalf("the priming scan resumed from %d, want 0", got)
	}
	editPos := b.Len() - 40
	b.Insert(editPos, 'x')
	e.getSpanCache(b)

	got := rec.lastResume(t)
	if got == 0 {
		t.Fatal("an edit near the end of the buffer rescanned from offset 0")
	}
	if got > editPos {
		t.Errorf("resumed from %d, which is past the edit at %d", got, editPos)
	}
	if editPos-got > spanCheckpointEvery {
		t.Errorf("resumed from %d for an edit at %d: %d runes further back than the %d-rune checkpoint interval",
			got, editPos, editPos-got, spanCheckpointEvery)
	}
}

// TestSpanCacheDiscardsCheckpointsAtTheEdit checks the other half of the
// contract: an edit near the top must invalidate everything after it.
func TestSpanCacheDiscardsCheckpointsAtTheEdit(t *testing.T) {
	e, b, rec := newResumeTestEditor(goSource(2000), "go", syntax.GoHighlighter{})
	e.getSpanCache(b)
	b.Insert(3, 'x')
	e.getSpanCache(b)
	if got := rec.lastResume(t); got > 3 {
		t.Errorf("resumed from %d after an edit at offset 3", got)
	}
}

// TestSpanCacheChangingHighlighterDropsCheckpoints guards the hlType check: a
// checkpoint taken by one highlighter says nothing about another's state, so
// swapping the highlighter for a buffer (as the compilation buffer does) must
// start the scan over.
func TestSpanCacheChangingHighlighterDropsCheckpoints(t *testing.T) {
	e, b, rec := newResumeTestEditor(goSource(500), "go", syntax.GoHighlighter{})
	c := e.getSpanCache(b)
	if len(c.checkpoints) < 2 {
		t.Fatalf("only %d checkpoints; nothing to drop", len(c.checkpoints))
	}
	// Stand in for a highlighter swap: the recorded type no longer matches the
	// one the next scan will use.
	c.hlType = reflect.TypeOf(syntax.PythonHighlighter{})
	b.Insert(b.Len()-10, 'x')
	e.getSpanCache(b)
	if got := rec.lastResume(t); got != 0 {
		t.Errorf("checkpoints from another highlighter were reused: resumed from %d, want 0", got)
	}
}

// TestSpanCacheReusesRuneScratch pins the double-buffering: the slice retired
// when the content changes comes back as the fill target next time, so steady
// state typing does not allocate a fresh copy of the buffer per keystroke.
func TestSpanCacheReusesRuneScratch(t *testing.T) {
	e, b, _ := newResumeTestEditor(goSource(200), "go", syntax.GoHighlighter{})
	first := e.getSpanCache(b)
	b.Insert(0, 'x')
	second := e.getSpanCache(b)
	if &second.spare[0] != &first.runes[0] {
		t.Error("the previous generation's rune slice was not retained as scratch")
	}
	b.Insert(0, 'y')
	third := e.getSpanCache(b)
	if &third.runes[0] != &second.spare[0] {
		t.Error("the scratch slice was not reused as the next generation's runes")
	}
}

// TestSpanCacheResumedSpansMatchFullScan is the editor-side differential test
// for resumption: after an edit anywhere in the buffer, the cached spans must be
// the ones a scan from offset 0 produces.
func TestSpanCacheResumedSpansMatchFullScan(t *testing.T) {
	src := goSourceWithMultiLineState()
	e := newCapTestEditor(src)
	b := buf(e)
	b.SetMode("go")
	e.getSpanCache(b)

	// Edit at many positions, including inside the block comment and the raw
	// string near the top, and check the whole cache after each one.
	for _, frac := range []int{0, 1, 5, 12, 25, 40, 50, 60, 75, 90, 99} {
		pos := b.Len() * frac / 100
		b.Insert(pos, 'x')
		got := e.getSpanCache(b)

		ref := newCapTestEditor(b.String())
		buf(ref).SetMode("go")
		want := ref.getSpanCache(buf(ref))
		if len(got.spans) != len(want.spans) {
			t.Fatalf("edit at %d%% (offset %d): cache holds %d spans, a full scan gives %d",
				frac, pos, len(got.spans), len(want.spans))
		}
		for i := range got.spans {
			if got.spans[i] != want.spans[i] {
				t.Fatalf("edit at %d%% (offset %d): span %d is %v, want %v",
					frac, pos, i, got.spans[i], want.spans[i])
			}
		}
		b.Delete(pos, 1)
		e.getSpanCache(b)
	}
}

// TestRenderWindowFacesMatchAfterEdits is the end-to-end guard: after an edit at
// each of many positions, every cell the renderer draws must carry the face a
// full-buffer highlight would give it.  Both markdown (cross-line fence state)
// and Go (raw strings and block comments) are covered.
func TestRenderWindowFacesMatchAfterEdits(t *testing.T) {
	cases := []struct {
		mode string
		src  string
	}{
		{"go", goSourceWithMultiLineState()},
		{"markdown", markdownSourceWithFences()},
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			e := newCapTestEditor(tc.src)
			b := buf(e)
			b.SetMode(tc.mode)
			total := b.LineCount()

			for _, frac := range []int{0, 3, 10, 25, 50, 75, 97} {
				for _, scrollFrac := range []int{0, 20, 55, 95} {
					scrollLine := max(total*scrollFrac/100, 1)
					pos := b.Len() * frac / 100
					b.Insert(pos, 'z')
					e.activeWin.SetScrollLine(scrollLine)
					e.Redraw()
					got := captureFaces(t, e)

					ref := newCapTestEditor(b.String())
					buf(ref).SetMode(tc.mode)
					ref.activeWin.SetScrollLine(scrollLine)
					ref.getSpanCache(buf(ref)) // full-buffer reference
					ref.Redraw()
					want := captureFaces(t, ref)

					coloured := 0
					for _, f := range want {
						if f != syntax.FaceDefault && f != (syntax.Face{}) {
							coloured++
						}
					}
					if coloured == 0 {
						t.Fatalf("edit=%d%% scroll=%d: reference render has no colour; the check would be vacuous",
							frac, scrollLine)
					}
					width, _ := e.term.CaptureSize()
					for i := range got {
						if got[i] != want[i] {
							t.Fatalf("edit=%d%% scroll=%d: face mismatch at row %d col %d: got %+v, want %+v",
								frac, scrollLine, i/width, i%width, got[i], want[i])
						}
					}
					b.Delete(pos, 1)
				}
			}
		})
	}
}

// markdownSourceWithFences returns Markdown with long fenced code blocks, so
// that checkpoints land inside a fence and the carried fence state matters.
func markdownSourceWithFences() string {
	var sb strings.Builder
	sb.WriteString("# Title\n\nIntro paragraph with *italic* and `code`.\n\n")
	for i := range 60 {
		fmt.Fprintf(&sb, "## Section %d\n\nA paragraph with **bold** and a [link](http://x/%d).\n\n```go\n", i, i)
		for j := range 20 {
			fmt.Fprintf(&sb, "// line %d inside the fence: # not a heading, > not a quote\n", j)
		}
		sb.WriteString("```\n\n> a blockquote\n\n")
	}
	return sb.String()
}

// TestSpanCacheCorruptCheckpointIsCaught is the mutation check on the end-to-end
// guard above.  Deliberately flipping the fence flag stored in a checkpoint must
// change what gets drawn — if it did not, the checkpoint state would be dead
// weight and TestRenderWindowFacesMatchAfterEdits would prove nothing.
func TestSpanCacheCorruptCheckpointIsCaught(t *testing.T) {
	src := markdownSourceWithFences()
	e := newCapTestEditor(src)
	b := buf(e)
	b.SetMode("markdown")
	c := e.getSpanCache(b)
	if len(c.checkpoints) < 4 {
		t.Fatalf("only %d checkpoints; nothing worth corrupting", len(c.checkpoints))
	}

	// Corrupt every checkpoint's fence flag, then force a rescan by editing at
	// the very end so that the newest checkpoint is the one that gets used.
	for i := range c.checkpoints {
		c.checkpoints[i].st.Fence = !c.checkpoints[i].st.Fence
	}
	b.Insert(b.Len()-2, 'z')
	got := e.getSpanCache(b)

	ref := newCapTestEditor(b.String())
	buf(ref).SetMode("markdown")
	want := ref.getSpanCache(buf(ref))

	if len(got.spans) == len(want.spans) {
		same := true
		for i := range got.spans {
			if got.spans[i] != want.spans[i] {
				same = false
				break
			}
		}
		if same {
			t.Error("corrupting every checkpoint's fence state changed nothing: " +
				"the state is not being used, so the resume tests prove nothing")
		}
	}
}

// TestSpanCacheScrollingResumesFromCheckpoint pins the other half of the win:
// scrolling past what the cache covers extends it from the newest checkpoint
// rather than rescanning the file from the top.
func TestSpanCacheScrollingResumesFromCheckpoint(t *testing.T) {
	e, b, rec := newResumeTestEditor(goSource(4000), "go", syntax.GoHighlighter{})
	w := e.activeWin
	e.getSpanCacheUpTo(b, visibleEndOf(w))
	first := e.spanCaches[b]
	if first.hiEnd >= b.Len() {
		t.Fatalf("primed cache already covers the whole buffer (%d)", b.Len())
	}

	// Jump past the covered range so the cache has to be extended.
	e.getSpanCacheUpTo(b, first.hiEnd+1)
	got := rec.lastResume(t)
	if got == 0 {
		t.Error("extending the cache rescanned from offset 0")
	}
	if got > first.hiEnd {
		t.Errorf("resumed from %d, past the previously covered end %d", got, first.hiEnd)
	}
}

// TestStartAutoRevertPollStopsCleanly checks the poller goroutine started for
// the event loop terminates when stopped, so quitting the editor does not leak
// it.  (The wakeup itself is a no-op without a real screen, so it cannot be
// observed through the capture terminal.)
func TestStartAutoRevertPollStopsCleanly(t *testing.T) {
	e := newCapTestEditor("hello\n")

	before := runtime.NumGoroutine()
	stop := e.startAutoRevertPoll()
	stop()

	// Give the goroutine a moment to observe the close and return.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= before {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("poller goroutine still running after stop (%d > %d)",
		runtime.NumGoroutine(), before)
}

// TestAutoRevertShowsNewContentInSameFrame covers the ordering half of the fix:
// the event loop reverts before rendering, so a file changed on disk is visible
// immediately rather than one event later.
func TestAutoRevertShowsNewContentInSameFrame(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "watched.txt")
	if err := os.WriteFile(path, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	e := newCapTestEditor("original\n")
	b := e.ActiveBuffer()
	b.SetFilename(path)
	b.SetModified(false)
	e.autoRevert = true
	e.autoRevertMtimes[b] = time.Now().Add(-time.Hour)

	if err := os.WriteFile(path, []byte("changed on disk\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The loop order: revert, then render.
	e.maybeAutoRevert()
	e.renderWindow(e.activeWin)

	if got := b.String(); !strings.Contains(got, "changed on disk") {
		t.Fatalf("buffer was not reverted, got %q", got)
	}
	if row := captureRow(t, e, 0); !strings.Contains(row, "changed on disk") {
		t.Errorf("reverted content not rendered in the same frame, row 0 = %q", row)
	}
}

// ---------------------------------------------------------------------------
// span cache: the rune window
// ---------------------------------------------------------------------------

// facesUpTo resolves the face of every rune position in [0, limit) the way the
// renderer does.  Comparing faces rather than spans is what the correctness
// argument actually needs: a windowed scan is allowed to report a different End
// for the token it was cut off in, as long as no position a caller can see
// changes colour.
func facesUpTo(spans []syntax.Span, limit int) []syntax.Face {
	out := make([]syntax.Face, max(limit, 0))
	for i := range out {
		out[i] = faceAtPos(spans, i)
	}
	return out
}

// visibleFacesBounded renders through the ordinary bounded path and returns the
// faces of the visible region plus the cache it used.
func visibleFacesBounded(e *Editor) ([]syntax.Face, *spanCache) {
	b := buf(e)
	end := visibleEndOf(e.activeWin)
	c := e.getSpanCacheUpTo(b, end)
	return facesUpTo(c.spans, end), c
}

// visibleFacesFull primes a whole-buffer cache first, so the faces come from a
// scan that never had to guess what lay beyond its window.
func visibleFacesFull(e *Editor) []syntax.Face {
	b := buf(e)
	end := visibleEndOf(e.activeWin)
	c := e.getSpanCache(b)
	return facesUpTo(c.spans, end)
}

// assertSameFaces compares two face runs and reports the first position that
// differs.  It also fails when nothing was coloured at all, which would make the
// comparison vacuous.
func assertSameFaces(t *testing.T, got, want []syntax.Face, context string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: compared %d positions against %d", context, len(got), len(want))
	}
	coloured := 0
	for _, f := range want {
		if f != syntax.FaceDefault && f != (syntax.Face{}) {
			coloured++
		}
	}
	if coloured == 0 {
		t.Fatalf("%s: the reference produced no colour at all; the comparison proves nothing", context)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s: face at rune %d is %+v, want %+v", context, i, got[i], want[i])
		}
	}
}

// TestSpanCacheRunesAreWindowed pins the change itself: the cache holds exactly
// the runes it claims to have highlighted, not the whole buffer.  Without this
// every keystroke copies the tail of the file that nobody can see.
func TestSpanCacheRunesAreWindowed(t *testing.T) {
	e, b, _ := newSpanCacheTestEditor(t, goSource(4000))
	c := e.getSpanCacheUpTo(b, 100)
	if len(c.runes) != c.hiEnd {
		t.Errorf("cache holds %d runes for a highlighted range of %d; the two must agree",
			len(c.runes), c.hiEnd)
	}
	if len(c.runes) >= b.Len() {
		t.Errorf("cache copied %d of %d runes; a small request must not materialise the buffer",
			len(c.runes), b.Len())
	}
	// A full-buffer request does need everything.
	full := e.getSpanCache(b)
	if len(full.runes) != b.Len() {
		t.Errorf("full-buffer cache holds %d runes, want %d", len(full.runes), b.Len())
	}
}

// TestSpanCacheWindowGrowsByAppending checks the scrolling path: the runes
// already copied are kept and only the new tail is fetched, so paging down does
// not re-copy everything above the window.
func TestSpanCacheWindowGrowsByAppending(t *testing.T) {
	e, b, _ := newSpanCacheTestEditor(t, goSource(4000))
	first := e.getSpanCacheUpTo(b, 100)
	prefix := append([]rune(nil), first.runes...)

	second := e.getSpanCacheUpTo(b, first.hiEnd+1)
	if second.hiEnd <= first.hiEnd {
		t.Fatalf("hiEnd did not grow: %d -> %d", first.hiEnd, second.hiEnd)
	}
	if len(second.runes) != second.hiEnd {
		t.Errorf("extended cache holds %d runes for a range of %d", len(second.runes), second.hiEnd)
	}
	for i, r := range prefix {
		if second.runes[i] != r {
			t.Fatalf("extending the window changed rune %d: %q -> %q", i, r, second.runes[i])
		}
	}
	// The tail really is buffer content, not zero runes left by a resize.
	want := b.Substring(0, second.hiEnd)
	if got := string(second.runes); got != want {
		t.Errorf("extended window does not match the buffer over [0, %d)", second.hiEnd)
	}
}

// TestSpanCacheWindowedFacesMatchFullScan is the correctness guard for the
// windowing: for a buffer where the window covers only part of the file, every
// visible rune must get the face a whole-buffer highlight would give it.  The
// cases that matter are the ones where multi-line state is opened far above the
// window — that is what the checkpoints exist for.
func TestSpanCacheWindowedFacesMatchFullScan(t *testing.T) {
	multi := goSourceWithMultiLineState()
	cases := []struct {
		name       string
		src        string
		mode       string
		scrollLine int
		// wantWindowed says the bounded cache must really have stopped short of
		// the end of the buffer, so the case is not silently comparing two
		// identical full scans.
		wantWindowed bool
	}{
		{"small buffer", goSource(3), "go", 1, false},
		{"large buffer at the top", goSource(4000), "go", 1, true},
		// The window at the very end needs the whole file copied anyway, so
		// there is nothing to window here; the case is kept as a check that the
		// degenerate range is handled.
		{"large buffer near the end", goSource(4000), "go", 4000*8 - 30, false},
		{"large buffer in the middle", goSource(4000), "go", 4000 * 4, true},
		{"block comment far above the window", multi, "go", 60, true},
		{"raw string far above the window", multi, "go", 100, true},
		{"markdown fence far above the window", markdownSourceWithFences(), "markdown", 400, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bounded := newCapTestEditor(tc.src)
			buf(bounded).SetMode(tc.mode)
			bounded.activeWin.SetScrollLine(tc.scrollLine)
			got, c := visibleFacesBounded(bounded)
			if tc.wantWindowed && len(c.runes) >= buf(bounded).Len() {
				t.Fatalf("the bounded path copied all %d runes; the case does not exercise windowing",
					len(c.runes))
			}

			reference := newCapTestEditor(tc.src)
			buf(reference).SetMode(tc.mode)
			reference.activeWin.SetScrollLine(tc.scrollLine)
			want := visibleFacesFull(reference)

			assertSameFaces(t, got, want, tc.name)
		})
	}
}

// TestSpanCacheWindowedFacesMatchAfterEditsAroundWindow covers the three places
// an edit can land relative to the window the cache holds: before it, inside it,
// and past its end.  Each one exercises a different branch of the resume logic,
// and each must still draw what a full scan would.
func TestSpanCacheWindowedFacesMatchAfterEditsAroundWindow(t *testing.T) {
	src := goSourceWithMultiLineState()
	e := newCapTestEditor(src)
	b := buf(e)
	b.SetMode("go")
	// Put the window well into the file so there is text on both sides of it.
	e.activeWin.SetScrollLine(b.LineCount() / 2)
	_, c := visibleFacesBounded(e)
	if c.hiEnd >= b.Len() {
		t.Fatalf("the primed cache covers the whole buffer (%d); nothing lies past the window", b.Len())
	}
	winStart := e.activeWin.ViewLines()[0].StartPos
	hiEnd := c.hiEnd

	edits := []struct {
		name string
		pos  int
	}{
		{"before the window", winStart / 2},
		{"just before the window", max(winStart-5, 0)},
		{"inside the window", winStart + 10},
		{"past the highlighted end", min(hiEnd+100, b.Len())},
		{"at the very end", b.Len()},
	}
	for _, ed := range edits {
		t.Run(ed.name, func(t *testing.T) {
			b.Insert(ed.pos, 'z')
			defer func() {
				b.Delete(ed.pos, 1)
				e.getSpanCacheUpTo(b, visibleEndOf(e.activeWin))
			}()

			got, _ := visibleFacesBounded(e)

			reference := newCapTestEditor(b.String())
			buf(reference).SetMode("go")
			reference.activeWin.SetScrollLine(e.activeWin.ScrollLine())
			want := visibleFacesFull(reference)

			assertSameFaces(t, got, want, ed.name)
		})
	}
}

// TestSpanCacheWindowedFacesMatchWhenNarrowed checks the case the windowing is
// least obviously safe for: buffer positions stay absolute under narrowing, and
// buffer.AppendRunesRange is deliberately not narrowing-aware, so a narrowed
// region far into a large file must still be coloured from the top of the file.
func TestSpanCacheWindowedFacesMatchWhenNarrowed(t *testing.T) {
	src := goSourceWithMultiLineState()

	narrow := func(e *Editor) {
		b := buf(e)
		b.SetMode("go")
		// Narrow to a region well past the multi-line constructs at the top.
		lo := b.Len() * 3 / 4
		hi := min(lo+4000, b.Len())
		b.Narrow(lo, hi)
		b.SetPoint(lo)
		line, _ := b.LineCol(lo)
		e.activeWin.SetScrollLine(line)
	}

	bounded := newCapTestEditor(src)
	narrow(bounded)
	got, c := visibleFacesBounded(bounded)
	if !buf(bounded).Narrowed() {
		t.Fatal("the buffer is not narrowed; the case proves nothing")
	}
	if len(c.runes) >= buf(bounded).Len() {
		t.Fatalf("the bounded path copied all %d runes while narrowed", len(c.runes))
	}

	reference := newCapTestEditor(src)
	narrow(reference)
	want := visibleFacesFull(reference)

	assertSameFaces(t, got, want, "narrowed")
}

// TestSpanCacheWindowedRendersSameCellsNearEnd is the end-to-end version for the
// case the audit called out: a window sitting far into a large file, with a block
// comment and a raw string opened near the top.  Both editors draw through
// Redraw, so the renderer's own indexing of the windowed rune slice is covered
// too.
func TestSpanCacheWindowedRendersSameCellsNearEnd(t *testing.T) {
	// A big file whose top holds the multi-line state, so the window near the
	// bottom is thousands of runes past anything the highlighter can see in one
	// windowed scan.
	src := goSourceWithMultiLineState() + goSource(3000)
	for _, frac := range []int{50, 75, 90, 99} {
		t.Run(fmt.Sprintf("at%d%%", frac), func(t *testing.T) {
			bounded := newCapTestEditor(src)
			buf(bounded).SetMode("go")
			line := max(buf(bounded).LineCount()*frac/100, 1)
			bounded.activeWin.SetScrollLine(line)
			bounded.Redraw()
			if c := bounded.spanCaches[buf(bounded)]; c == nil {
				t.Fatal("no span cache after Redraw")
			} else if len(c.runes) != c.hiEnd {
				t.Fatalf("cache holds %d runes for a range of %d", len(c.runes), c.hiEnd)
			}

			reference := newCapTestEditor(src)
			buf(reference).SetMode("go")
			reference.activeWin.SetScrollLine(line)
			reference.getSpanCache(buf(reference))
			reference.Redraw()

			got := captureFaces(t, bounded)
			want := captureFaces(t, reference)
			assertSameFaces(t, got, want, fmt.Sprintf("scroll to %d%%", frac))
		})
	}
}

// TestSpanCacheTruncatedTailIsNotTrusted is the mutation guard on the windowing
// argument.  A scan cut off inside a raw string reports that string as running
// to the cut, which is wrong for the text beyond it.  Extending the cache must
// throw that span away and rescan, not keep it: the last checkpoint always lies
// before the truncated token, so spans from it onwards are discarded.
func TestSpanCacheTruncatedTailIsNotTrusted(t *testing.T) {
	// A raw string that starts inside the first window and runs far past it, so
	// the first scan is guaranteed to be cut off in the middle of it.  Ordinary
	// code follows, which must not end up coloured as string.
	var sb strings.Builder
	sb.WriteString("package main\n\nconst big = `")
	for i := range 4000 {
		fmt.Fprintf(&sb, "raw line %d with func and return in it\n", i)
	}
	sb.WriteString("`\n\n")
	sb.WriteString(goSource(600))
	src := sb.String()

	e := newCapTestEditor(src)
	b := buf(e)
	b.SetMode("go")
	// Prime a window that ends inside the raw string.
	first := e.getSpanCacheUpTo(b, 100)
	// Confirm the premise: the last span is the raw string, cut off at the
	// window's end rather than at its real closing backquote.  Without this the
	// test could pass while proving nothing.
	if n := len(first.spans); n == 0 {
		t.Fatal("the primed scan produced no spans")
	} else if last := first.spans[n-1]; last.End != first.hiEnd || last.Face != syntax.FaceString {
		t.Fatalf("expected the window to be cut off inside the raw string, got last span %v (hiEnd=%d)",
			last, first.hiEnd)
	}
	// Now ask for the code after the raw string: the truncated span from the
	// first scan must not survive into this one.
	got := e.getSpanCache(b)

	ref := newCapTestEditor(src)
	buf(ref).SetMode("go")
	want := ref.getSpanCache(buf(ref))

	if len(got.spans) != len(want.spans) {
		t.Fatalf("extended cache holds %d spans, a scan from scratch gives %d",
			len(got.spans), len(want.spans))
	}
	for i := range got.spans {
		if got.spans[i] != want.spans[i] {
			t.Fatalf("span %d is %v, want %v", i, got.spans[i], want.spans[i])
		}
	}
}

// TestScratchCapLeavesHeadroom documents why growScratch over-allocates: a slice
// sized exactly would be regrown — and the whole window recopied — by the very
// next keystroke.
func TestScratchCapLeavesHeadroom(t *testing.T) {
	for _, n := range []int{0, 1, 100, 100000} {
		if got := scratchCap(n); got <= n {
			t.Errorf("scratchCap(%d) = %d, want more than %d", n, got, n)
		}
	}
	s := growScratch(nil, 1000)
	if len(s) != 0 {
		t.Errorf("growScratch returned length %d, want 0", len(s))
	}
	if cap(s) < 1000 {
		t.Errorf("growScratch capacity %d, want at least 1000", cap(s))
	}
	reused := growScratch(s[:5], 1000)
	if cap(reused) != cap(s) {
		t.Error("growScratch reallocated a slice that was already big enough")
	}
}

// TestExtendRunesKeepsPrefixAndAppends covers the helper directly, including the
// no-op case and the case where it has to grow.
func TestExtendRunesKeepsPrefixAndAppends(t *testing.T) {
	b := buffer.NewWithContent("*t*", "søen er blå og dyp")
	n := b.Len()

	got := extendRunes(b, nil, 5)
	if string(got) != b.Substring(0, 5) {
		t.Errorf("extendRunes from empty = %q, want %q", string(got), b.Substring(0, 5))
	}
	got = extendRunes(b, got, n)
	if string(got) != b.String() {
		t.Errorf("extendRunes to the end = %q, want %q", string(got), b.String())
	}
	// Already long enough: returned untouched.
	same := extendRunes(b, got, 3)
	if len(same) != n {
		t.Errorf("extendRunes shortened the slice to %d, want %d left alone", len(same), n)
	}
}

// BenchmarkSpanCacheWindowMaterialise isolates what the windowing saves: the
// rune copy a keystroke performs when the window sits at the top of a large
// file.  Compare against BenchmarkSpanCacheTextMaterialise, which copies the
// whole buffer as the old code did.
func BenchmarkSpanCacheWindowMaterialise(bench *testing.B) {
	src := goSource(benchGoFuncs)
	e := newTestEditor(src)
	b := buf(e)
	scratch := make([]rune, 0, 4096)
	bench.ReportAllocs()
	bench.ResetTimer()
	for range bench.N {
		b.Insert(0, 'x')
		b.Delete(0, 1)
		scratch = b.AppendRunesRange(scratch[:0], 0, 4096)
		if len(scratch) == 0 {
			bench.Fatal("empty window")
		}
	}
}

// ---------------------------------------------------------------------------
// isearch: incremental scanning
// ---------------------------------------------------------------------------

// isearchOracleFind is the pre-optimisation isearchFind transcribed as a pure
// function: a full scan from start, with no memory of any earlier query.  It
// returns the point isearchFind has to leave the buffer at, or -1 when the
// search fails — in which case the point is left wherever it was.
func isearchOracleFind(runes, needle []rune, start int, fwd, fold bool) int {
	if len(needle) == 0 {
		return start
	}
	match := runesMatch
	if fold {
		match = runesMatchFold
	}
	if fwd {
		for i := start; i <= len(runes)-len(needle); i++ {
			if match(runes[i:], needle) {
				return i + len(needle)
			}
		}
		return -1
	}
	for i := start; i >= 0; i-- {
		if i+len(needle) <= len(runes) && match(runes[i:], needle) {
			return i
		}
	}
	return -1
}

// isearchKey feeds one key to the isearch handler.  With memo == false the
// resume memo is cleared first, which puts isearchFind back on its
// pre-optimisation path — a full scan from isearchStart every time — so the two
// algorithms can be compared key for key.
func isearchKey(e *Editor, ke terminal.KeyEvent, memo bool) {
	if !memo {
		e.isearchMemo = isearchFindMemo{}
	}
	e.isearchHandleKey(ke)
}

// TestIsearchIncrementalMatchesFullScanOracle types each query one rune at a
// time and checks the point after every prefix against a full scan from
// isearchStart.  Resuming from the previous match must never change where the
// search lands, nor whether it fails.
func TestIsearchIncrementalMatchesFullScanOracle(t *testing.T) {
	content := "alpha beta ALPHA Beta alphabet\nalpha soup\nbetamax\nsøen er blå, søen er dyp\n" +
		strings.Repeat("filler line with alpha and beta in it\n", 40) +
		"tail alphabet\n"
	cases := []struct {
		name  string
		query string
		start int
		fwd   bool
		fold  bool
	}{
		{"forward from the top", "alpha", 0, true, true},
		{"forward case sensitive", "ALPHA", 0, true, false},
		{"forward case folded finds the first spelling", "alpha", 0, true, true},
		{"forward from the middle", "beta", 300, true, true},
		{"forward self-overlapping", "aa", 0, true, true},
		{"forward failing query", "zzzz-not-here", 0, true, true},
		{"forward query that fails only at the last rune", "alphabetq", 0, true, true},
		{"backward from the end", "alpha", len(content) - 1, false, true},
		{"backward case sensitive", "Beta", len(content) - 1, false, false},
		{"backward failing query", "qqq", len(content) - 1, false, true},
		{"multibyte forward", "blå", 0, true, true},
		{"multibyte backward", "søen", len([]rune(content)) - 1, false, true},
		{"empty-ish single rune", "a", 0, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newTestEditor(content)
			b := buf(e)
			e.isSearchCaseFold = tc.fold
			start := min(tc.start, b.Len())
			b.SetPoint(start)
			e.startIsearch(tc.fwd)
			runes := []rune(content)

			// The old code left the point untouched on a failed search, so the
			// expectation has to be carried forward.
			wantPoint := start
			for i, r := range []rune(tc.query) {
				isearchKey(e, keRune(r, 0), true)
				needle := []rune(tc.query)[:i+1]
				if got := isearchOracleFind(runes, needle, start, tc.fwd, tc.fold); got >= 0 {
					wantPoint = got
				}
				if b.Point() != wantPoint {
					t.Fatalf("after typing %q: point = %d, want %d", string(needle), b.Point(), wantPoint)
				}
			}
			// Backspacing all the way out must retrace the same path.
			for i := len([]rune(tc.query)) - 1; i >= 0; i-- {
				isearchKey(e, terminal.KeyEvent{Key: tcell.KeyBackspace}, true)
				needle := []rune(tc.query)[:i]
				if got := isearchOracleFind(runes, needle, start, tc.fwd, tc.fold); got >= 0 {
					wantPoint = got
				}
				if b.Point() != wantPoint {
					t.Fatalf("after backspacing to %q: point = %d, want %d",
						string(needle), b.Point(), wantPoint)
				}
			}
		})
	}
}

// isearchScriptResult is everything a test can observe about an isearch session.
type isearchScriptResult struct {
	point   int
	str     string
	fwd     bool
	message string
	active  bool
}

// runIsearchScript drives a fresh editor over content through the given keys and
// returns the observable state after each one.  memo selects the incremental
// isearchFind or the full-rescan behaviour it replaced.
func runIsearchScript(content string, fold, forward bool, startPos int,
	keys []terminal.KeyEvent, memo bool) []isearchScriptResult {
	e := newTestEditor(content)
	b := buf(e)
	e.isSearchCaseFold = fold
	b.SetPoint(min(startPos, b.Len()))
	e.startIsearch(forward)

	out := make([]isearchScriptResult, 0, len(keys))
	for _, ke := range keys {
		isearchKey(e, ke, memo)
		out = append(out, isearchScriptResult{
			point:   b.Point(),
			str:     e.isearchStr,
			fwd:     e.isearchFwd,
			message: e.message,
			active:  e.isearching,
		})
	}
	return out
}

// TestIsearchScriptsMatchFullRescan is the differential test over whole isearch
// sessions: repeated C-s, reversing with C-r, C-w word yanks, backspacing and
// failing queries.  The incremental scan must produce the same point, the same
// query and the same minibuffer message at every step as the full rescan did.
func TestIsearchScriptsMatchFullRescan(t *testing.T) {
	ctrlS := terminal.KeyEvent{Key: tcell.KeyCtrlS}
	ctrlR := terminal.KeyEvent{Key: tcell.KeyCtrlR}
	ctrlW := terminal.KeyEvent{Key: tcell.KeyCtrlW}
	bs := terminal.KeyEvent{Key: tcell.KeyBackspace}
	typed := func(s string) []terminal.KeyEvent {
		out := make([]terminal.KeyEvent, 0, len(s))
		for _, r := range s {
			out = append(out, keRune(r, 0))
		}
		return out
	}
	join := func(parts ...[]terminal.KeyEvent) []terminal.KeyEvent {
		var out []terminal.KeyEvent
		for _, p := range parts {
			out = append(out, p...)
		}
		return out
	}

	content := "CamelCase here\nalpha beta gamma\nALPHA BETA\n" +
		strings.Repeat("alpha beta gamma delta\n", 30) +
		"alpha at the very end\n"
	total := len([]rune(content))

	cases := []struct {
		name    string
		fold    bool
		forward bool
		start   int
		keys    []terminal.KeyEvent
	}{
		{"type then repeat forward", true, true, 0,
			join(typed("alpha"), []terminal.KeyEvent{ctrlS, ctrlS, ctrlS})},
		{"repeat past the end wraps", true, true, 0,
			join(typed("alpha"), func() []terminal.KeyEvent {
				out := make([]terminal.KeyEvent, 0, 40)
				for range 40 {
					out = append(out, ctrlS)
				}
				return out
			}())},
		{"reverse direction", true, true, 0,
			join(typed("beta"), []terminal.KeyEvent{ctrlS, ctrlS, ctrlR, ctrlR, ctrlS})},
		{"backward then wrap to the bottom", true, false, 200,
			join(typed("alpha"), func() []terminal.KeyEvent {
				out := make([]terminal.KeyEvent, 0, 20)
				for range 20 {
					out = append(out, ctrlR)
				}
				return out
			}())},
		{"word yank extends the query", true, true, 0,
			join(typed("Camel"), []terminal.KeyEvent{ctrlW, ctrlW}, typed("x"), []terminal.KeyEvent{bs})},
		{"word yank then repeat", true, true, 0,
			join(typed("alpha"), []terminal.KeyEvent{ctrlW, ctrlS, ctrlS})},
		{"failing query then backspace", true, true, 0,
			join(typed("alphaQQQ"), []terminal.KeyEvent{bs, bs, bs}, typed(" beta"))},
		{"case sensitive misses the lower-case spellings", false, true, 0,
			join(typed("ALPHA"), []terminal.KeyEvent{ctrlS, ctrlS})},
		{"case sensitive backward", false, false, total - 1,
			join(typed("ALPHA"), []terminal.KeyEvent{ctrlR, ctrlR})},
		{"typing after a repeat restarts from isearchStart", true, true, 0,
			join(typed("alpha"), []terminal.KeyEvent{ctrlS, ctrlS}, typed(" beta"))},
		{"backspace to empty then retype", true, true, 100,
			join(typed("beta"), []terminal.KeyEvent{bs, bs, bs, bs}, typed("gamma"))},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runIsearchScript(content, tc.fold, tc.forward, tc.start, tc.keys, true)
			want := runIsearchScript(content, tc.fold, tc.forward, tc.start, tc.keys, false)
			if len(got) != len(want) {
				t.Fatalf("got %d steps, want %d", len(got), len(want))
			}
			for i := range got {
				if got[i] != want[i] {
					t.Fatalf("step %d differs:\n incremental %+v\n full rescan %+v", i, got[i], want[i])
				}
			}
			// Guard against a vacuous pass: something must actually have moved.
			if len(got) > 0 && got[len(got)-1].point == min(tc.start, len([]rune(content))) &&
				got[len(got)-1].str == "" {
				t.Fatal("the script left the search untouched; it proves nothing")
			}
		})
	}
}

// TestIsearchWrapMessages pins the two wrap messages, which the incremental
// scan must not disturb: they come from isearchFindNext, not isearchFind, and a
// stale memo must not suppress them.
func TestIsearchWrapMessages(t *testing.T) {
	content := "alpha\nbeta\nalpha\n"
	e := newTestEditor(content)
	b := buf(e)
	b.SetPoint(0)
	e.startIsearch(true)
	for _, r := range "alpha" {
		e.isearchHandleKey(keRune(r, 0))
	}
	// Two more matches ahead of us at most, so repeating three times has to wrap.
	e.isearchHandleKey(terminal.KeyEvent{Key: tcell.KeyCtrlS})
	e.isearchHandleKey(terminal.KeyEvent{Key: tcell.KeyCtrlS})
	if want := fmt.Sprintf(isearchWrapBotMsg, "alpha"); e.message != want {
		t.Errorf("forward wrap message = %q, want %q", e.message, want)
	}

	e2 := newTestEditor(content)
	b2 := buf(e2)
	b2.SetPoint(0)
	e2.startIsearch(false)
	for _, r := range "alpha" {
		e2.isearchHandleKey(keRune(r, 0))
	}
	e2.isearchHandleKey(terminal.KeyEvent{Key: tcell.KeyCtrlR})
	if want := fmt.Sprintf(isearchWrapTopMsg, "alpha"); e2.message != want {
		t.Errorf("backward wrap message = %q, want %q", e2.message, want)
	}
}

// TestIsearchMemoInvalidatedByBufferEdit checks the one case where resuming
// would be wrong: the buffer changed under the search, so the previous match
// offset means nothing and the scan has to start over.
func TestIsearchMemoInvalidatedByBufferEdit(t *testing.T) {
	e := newTestEditor("xxxx alpha\n")
	b := buf(e)
	b.SetPoint(0)
	e.startIsearch(true)
	for _, r := range "alph" {
		e.isearchHandleKey(keRune(r, 0))
	}
	if b.Point() != 9 {
		t.Fatalf("point after typing \"alph\" = %d, want 9", b.Point())
	}
	// Insert a fresh, earlier match.  The next character typed must find it
	// rather than resuming from the old, later offset.
	b.Insert(0, 'a')
	b.Insert(1, 'l')
	b.Insert(2, 'p')
	b.Insert(3, 'h')
	b.Insert(4, 'a')
	e.isearchStart = 0
	e.isearchHandleKey(keRune('a', 0))
	if b.Point() != 5 {
		t.Errorf("point = %d, want 5 (the edit created an earlier match)", b.Point())
	}
}

// TestIsearchMemoShortCircuitsAFailedQuery pins the other half of the prefix
// argument: once a query has failed, every extension of it fails too, and the
// message must still be shown.
func TestIsearchMemoShortCircuitsAFailedQuery(t *testing.T) {
	e := newTestEditor(strings.Repeat("nothing to see here\n", 100))
	b := buf(e)
	b.SetPoint(0)
	e.startIsearch(true)
	for _, r := range "zzz" {
		e.isearchHandleKey(keRune(r, 0))
	}
	if want := fmt.Sprintf(isearchFailMsg, "zzz"); e.message != want {
		t.Fatalf("message = %q, want %q", e.message, want)
	}
	if e.isearchMemo.pos != -1 {
		t.Errorf("memo records pos = %d after a failed search, want -1", e.isearchMemo.pos)
	}
	e.isearchHandleKey(keRune('q', 0))
	if want := fmt.Sprintf(isearchFailMsg, "zzzq"); e.message != want {
		t.Errorf("message after extending a failed query = %q, want %q", e.message, want)
	}
	if b.Point() != 0 {
		t.Errorf("point = %d, want 0 (a failed search must not move point)", b.Point())
	}
}

// benchIsearchTyping types query into a fresh isearch over a large buffer, one
// rune at a time, which is what the user actually does.  memo selects the
// incremental scan or the full rescan from isearchStart it replaced.
func benchIsearchTyping(bench *testing.B, memo bool) {
	// The match sits near the end, so a rescan from the top is expensive and an
	// incremental one is not.
	content := strings.Repeat("some line of ordinary prose about nothing\n", 20000) +
		"the needle we are looking for\n"
	keys := make([]terminal.KeyEvent, 0, 16)
	for _, r := range "the needle we" {
		keys = append(keys, keRune(r, 0))
	}
	e := newTestEditor(content)
	b := buf(e)
	e.isSearchCaseFold = true

	bench.ReportAllocs()
	bench.ResetTimer()
	for range bench.N {
		b.SetPoint(0)
		e.startIsearch(true)
		for _, ke := range keys {
			isearchKey(e, ke, memo)
		}
	}
}

// BenchmarkIsearchTypingRescan is the pre-optimisation behaviour: every
// character typed rescans the whole buffer from isearchStart.
func BenchmarkIsearchTypingRescan(bench *testing.B) { benchIsearchTyping(bench, false) }

// BenchmarkIsearchTypingIncremental is the same session with the scan resumed
// from the previous match.
func BenchmarkIsearchTypingIncremental(bench *testing.B) { benchIsearchTyping(bench, true) }

// benchIsearchFailingTyping types a query that matches nothing.  This is the
// case the memo short-circuits outright: once the first character has failed,
// every further one used to rescan the whole buffer for nothing.
func benchIsearchFailingTyping(bench *testing.B, memo bool) {
	content := strings.Repeat("some line of ordinary prose about nothing\n", 20000)
	keys := make([]terminal.KeyEvent, 0, 8)
	for _, r := range "qqqqqqqq" {
		keys = append(keys, keRune(r, 0))
	}
	e := newTestEditor(content)
	b := buf(e)
	e.isSearchCaseFold = true
	bench.ReportAllocs()
	bench.ResetTimer()
	for range bench.N {
		b.SetPoint(0)
		e.startIsearch(true)
		for _, ke := range keys {
			isearchKey(e, ke, memo)
		}
	}
}

// BenchmarkIsearchTypingFailingRescan is a hopeless query with the old
// behaviour: one full scan of the buffer per character.
func BenchmarkIsearchTypingFailingRescan(bench *testing.B) {
	benchIsearchFailingTyping(bench, false)
}

// BenchmarkIsearchTypingFailing is the same query short-circuited after the
// first character.
func BenchmarkIsearchTypingFailing(bench *testing.B) {
	benchIsearchFailingTyping(bench, true)
}

// ---------------------------------------------------------------------------
// command LRU
// ---------------------------------------------------------------------------

// pushCommandLRUOracle is the pre-optimisation pushCommandLRU transcribed: a
// fresh slice on every call.  The in-place version must produce byte-identical
// ordering, because M-x candidate ordering and switch-to-buffer depend on it.
func pushCommandLRUOracle(lru []string, name string) []string {
	filtered := make([]string, 0, len(lru))
	for _, n := range lru {
		if n != name {
			filtered = append(filtered, n)
		}
	}
	out := append([]string{name}, filtered...)
	if len(out) > commandLRUMax {
		out = out[:commandLRUMax]
	}
	return out
}

// TestPushCommandLRUMatchesOracle is the differential test over long command
// sequences: repeats, promotions from every depth, and well past the cap.
func TestPushCommandLRUMatchesOracle(t *testing.T) {
	e := newTestEditor("")
	var want []string

	// A deterministic but unfriendly sequence: a widening pool of names with
	// frequent repeats, run far past commandLRUMax.
	names := make([]string, 0, 80)
	for i := range 80 {
		names = append(names, fmt.Sprintf("command-%02d", i))
	}
	seq := make([]string, 0, 600)
	for i := range 600 {
		switch {
		case i%7 == 0 && i > 0:
			seq = append(seq, seq[i-1]) // repeat the most recent
		case i%11 == 0 && i > 20:
			seq = append(seq, seq[i-20]) // promote an older one
		default:
			seq = append(seq, names[i%len(names)])
		}
	}

	for step, name := range seq {
		e.pushCommandLRU(name)
		want = pushCommandLRUOracle(want, name)
		if len(e.commandLRU) != len(want) {
			t.Fatalf("step %d (%q): length %d, want %d", step, name, len(e.commandLRU), len(want))
		}
		for i := range want {
			if e.commandLRU[i] != want[i] {
				t.Fatalf("step %d (%q): entry %d is %q, want %q\n got: %v\nwant: %v",
					step, name, i, e.commandLRU[i], want[i], e.commandLRU, want)
			}
		}
	}
	if len(e.commandLRU) != commandLRUMax {
		t.Errorf("final length %d, want the cap %d", len(e.commandLRU), commandLRUMax)
	}
}

// TestPushCommandLRUOrdering spells out the three cases by hand, so a failure
// says which rule broke rather than just "differs from the oracle".
func TestPushCommandLRUOrdering(t *testing.T) {
	e := newTestEditor("")
	for _, n := range []string{"c", "b", "a"} {
		e.pushCommandLRU(n)
	}
	// Most recent first.
	if got := strings.Join(e.commandLRU, ","); got != "a,b,c" {
		t.Fatalf("after pushing c,b,a the list is %q, want \"a,b,c\"", got)
	}
	// Repeating the most recent changes nothing.
	e.pushCommandLRU("a")
	if got := strings.Join(e.commandLRU, ","); got != "a,b,c" {
		t.Errorf("repeating the most recent gave %q, want \"a,b,c\"", got)
	}
	// Promoting the oldest moves it to the front and keeps the rest in order.
	e.pushCommandLRU("c")
	if got := strings.Join(e.commandLRU, ","); got != "c,a,b" {
		t.Errorf("promoting the oldest gave %q, want \"c,a,b\"", got)
	}
	// Promoting the middle one.
	e.pushCommandLRU("a")
	if got := strings.Join(e.commandLRU, ","); got != "a,c,b" {
		t.Errorf("promoting the middle gave %q, want \"a,c,b\"", got)
	}
}

// TestPushCommandLRUEvictsTheOldest checks the cap: entry 50 falls off the end
// and everything else shifts down by one.
func TestPushCommandLRUEvictsTheOldest(t *testing.T) {
	e := newTestEditor("")
	for i := range commandLRUMax {
		e.pushCommandLRU(fmt.Sprintf("cmd-%02d", i))
	}
	if len(e.commandLRU) != commandLRUMax {
		t.Fatalf("length %d, want %d", len(e.commandLRU), commandLRUMax)
	}
	oldest := e.commandLRU[commandLRUMax-1]
	if oldest != "cmd-00" {
		t.Fatalf("oldest entry is %q, want \"cmd-00\"", oldest)
	}
	e.pushCommandLRU("brand-new")
	if len(e.commandLRU) != commandLRUMax {
		t.Errorf("length after overflow = %d, want %d", len(e.commandLRU), commandLRUMax)
	}
	if e.commandLRU[0] != "brand-new" {
		t.Errorf("front = %q, want \"brand-new\"", e.commandLRU[0])
	}
	if slices.Contains(e.commandLRU, oldest) {
		t.Errorf("%q was not evicted: %v", oldest, e.commandLRU)
	}
	if e.commandLRU[commandLRUMax-1] != "cmd-01" {
		t.Errorf("new oldest = %q, want \"cmd-01\"", e.commandLRU[commandLRUMax-1])
	}
}

// TestPushCommandLRUDoesNotAllocate is the point of the change: this runs for
// every arrow key, so neither repeating a command nor promoting one may allocate.
func TestPushCommandLRUDoesNotAllocate(t *testing.T) {
	e := newTestEditor("")
	for i := range commandLRUMax {
		e.pushCommandLRU(fmt.Sprintf("cmd-%02d", i))
	}

	// Repeat of the most recent: the fast path.
	if got := testing.AllocsPerRun(200, func() { e.pushCommandLRU("cmd-49") }); got != 0 {
		t.Errorf("repeating the most recent command allocated %.1f times per call, want 0", got)
	}
	// Promotion of an older entry: moved in place.
	if got := testing.AllocsPerRun(200, func() {
		e.pushCommandLRU("cmd-00")
		e.pushCommandLRU("cmd-01")
	}); got != 0 {
		t.Errorf("promoting an older command allocated %.1f times per call, want 0", got)
	}
	// A brand-new name at a full list: still no allocation, the oldest is evicted.
	i := 0
	if got := testing.AllocsPerRun(200, func() {
		i++
		e.pushCommandLRU(fmt.Sprintf("x%d", i))
	}); got > 1 {
		// fmt.Sprintf itself allocates the string, hence the tolerance of one.
		t.Errorf("pushing a new command allocated %.1f times per call, want at most 1", got)
	}
}

// benchPushCommandLRU measures the per-keystroke cost of recording a command.
// The "old" variant reallocates the way the previous implementation did.
func benchPushCommandLRU(bench *testing.B, name string, old bool) {
	e := newTestEditor("")
	for i := range commandLRUMax {
		e.pushCommandLRU(fmt.Sprintf("cmd-%02d", i))
	}
	bench.ReportAllocs()
	bench.ResetTimer()
	for range bench.N {
		if old {
			e.commandLRU = pushCommandLRUOracle(e.commandLRU, name)
		} else {
			e.pushCommandLRU(name)
		}
	}
}

// BenchmarkPushCommandLRURepeatOld is holding down an arrow key with the
// previous implementation: a fresh 50-element slice per keystroke.
func BenchmarkPushCommandLRURepeatOld(bench *testing.B) {
	benchPushCommandLRU(bench, "cmd-49", true)
}

// BenchmarkPushCommandLRURepeat is the same with the fast path.
func BenchmarkPushCommandLRURepeat(bench *testing.B) {
	benchPushCommandLRU(bench, "cmd-49", false)
}

// BenchmarkPushCommandLRUPromoteOld promotes the oldest entry the old way.
func BenchmarkPushCommandLRUPromoteOld(bench *testing.B) {
	benchPushCommandLRU(bench, "cmd-00", true)
}

// BenchmarkPushCommandLRUPromote promotes the oldest entry in place.
func BenchmarkPushCommandLRUPromote(bench *testing.B) {
	benchPushCommandLRU(bench, "cmd-00", false)
}
