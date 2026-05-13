package editor

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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
