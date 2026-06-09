package editor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/elisp"
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
// cmdImenu (no entries)
// ---------------------------------------------------------------------------

func TestCmdImenu_NoEntries(t *testing.T) {
	e := newTestEditor("hello world")
	// fundamental mode has no imenu entries.
	e.cmdImenu()
	if e.minibufActive {
		t.Error("cmdImenu with no entries: minibuf should not be active")
	}
	if e.message == "" {
		t.Error("cmdImenu with no entries: expected a message")
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
