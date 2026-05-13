package editor

// Tests that improve coverage for:
//   - imenu.go: cmdImenu (entries + navigation paths)
//   - clipboard.go: clipboardCmd (all platform/env branches)
//   - commands.go: cmdProjectFindFile, promptSaveNext

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/skybert/gomacs/internal/buffer"
)

// ============================================================================
// cmdImenu — entries present
// ============================================================================

// TestCmdImenu_WithGoEntries verifies that calling cmdImenu on a Go-mode
// buffer with at least one function declaration opens the minibuffer and
// offers completions.
func TestCmdImenu_WithGoEntries(t *testing.T) {
	src := "package main\n\nfunc HelloWorld() {}\nfunc Goodbye() {}\n"
	e := newTestEditor(src)
	buf(e).SetMode("go")

	e.cmdImenu()

	if !e.minibufActive {
		t.Fatal("cmdImenu with Go entries: expected minibufActive=true")
	}
	if e.minibufPrompt == "" {
		t.Error("cmdImenu: prompt should not be empty")
	}
	if !strings.Contains(e.minibufPrompt, "imenu") {
		t.Errorf("cmdImenu: prompt %q should contain 'imenu'", e.minibufPrompt)
	}
}

// TestCmdImenu_WithMarkdownEntries verifies behaviour in Markdown mode.
func TestCmdImenu_WithMarkdownEntries(t *testing.T) {
	src := "# Introduction\n\nSome text.\n\n## Usage\n\nMore text.\n"
	e := newTestEditor(src)
	buf(e).SetMode("markdown")

	e.cmdImenu()

	if !e.minibufActive {
		t.Fatal("cmdImenu with Markdown entries: expected minibufActive=true")
	}
}

// TestCmdImenu_WithElispEntries verifies behaviour in Elisp mode.
func TestCmdImenu_WithElispEntries(t *testing.T) {
	src := "(defun my-func ()\n  nil)\n(defvar my-var 42)\n"
	e := newTestEditor(src)
	buf(e).SetMode("elisp")

	e.cmdImenu()

	if !e.minibufActive {
		t.Fatal("cmdImenu with Elisp entries: expected minibufActive=true")
	}
}

// TestCmdImenu_CompletionCallbackFilters verifies that the completion function
// registered by cmdImenu returns only labels that fuzzy-match the query.
func TestCmdImenu_CompletionCallbackFilters(t *testing.T) {
	src := "package main\n\nfunc HelloWorld() {}\nfunc Goodbye() {}\n"
	e := newTestEditor(src)
	buf(e).SetMode("go")

	e.cmdImenu()

	if e.minibufCompletions == nil {
		t.Fatal("cmdImenu: minibufCompletions should be set")
	}

	// "Hello" should match "HelloWorld (line 3)" but not "Goodbye (line 4)".
	results := e.minibufCompletions("Hello")
	found := false
	for _, r := range results {
		if strings.Contains(r, "HelloWorld") {
			found = true
		}
		if strings.Contains(r, "Goodbye") {
			t.Errorf("cmdImenu completions: 'Hello' query should not return Goodbye entry, got %v", results)
		}
	}
	if !found {
		t.Errorf("cmdImenu completions: 'Hello' query should return HelloWorld entry, got %v", results)
	}
}

// TestCmdImenu_CallbackNavigatesToLine verifies that finishing the minibuffer
// with a valid label moves point to the start of the corresponding line.
func TestCmdImenu_CallbackNavigatesToLine(t *testing.T) {
	// "package main\n" is 13 chars; "func HelloWorld" starts at rune 14.
	src := "package main\n\nfunc HelloWorld() {}\n"
	e := newTestEditor(src)
	buf(e).SetMode("go")

	// Move point away from the target.
	buf(e).SetPoint(0)

	e.cmdImenu()

	if !e.minibufActive {
		t.Fatal("cmdImenu: minibuf should be active")
	}

	// Simulate selecting "HelloWorld (line 3)" — the callback should move point.
	entries := imenuSymbols(buf(e))
	if len(entries) == 0 {
		t.Fatal("no imenu entries found in Go buffer")
	}
	var target imenuEntry
	for _, en := range entries {
		if strings.Contains(en.label, "HelloWorld") {
			target = en
			break
		}
	}
	if target.label == "" {
		t.Fatal("HelloWorld imenu entry not found")
	}

	// Invoke the callback directly (simulates user pressing Enter).
	e.minibufDoneFunc(target.label)

	wantPt := lineStartOffset(buf(e), target.line)
	if got := buf(e).Point(); got != wantPt {
		t.Errorf("cmdImenu navigation: want point=%d, got %d", wantPt, got)
	}
}

// TestCmdImenu_CallbackUnknownLabelIgnored verifies that an unknown label
// passed to the callback does not move point or panic.
func TestCmdImenu_CallbackUnknownLabelIgnored(t *testing.T) {
	src := "package main\n\nfunc Foo() {}\n"
	e := newTestEditor(src)
	buf(e).SetMode("go")

	buf(e).SetPoint(5)
	e.cmdImenu()

	// Passing a label that doesn't match any entry should be a no-op.
	ptBefore := buf(e).Point()
	e.minibufDoneFunc("this label does not exist")
	if got := buf(e).Point(); got != ptBefore {
		t.Errorf("cmdImenu unknown label: point should not change; before=%d after=%d", ptBefore, got)
	}
}

// TestCmdImenu_PythonWithEntries verifies Imenu in Python mode.
func TestCmdImenu_PythonWithEntries(t *testing.T) {
	src := "def greet(name):\n    pass\n\nclass Animal:\n    pass\n"
	e := newTestEditor(src)
	buf(e).SetMode("python")

	e.cmdImenu()

	if !e.minibufActive {
		t.Fatal("cmdImenu with Python entries: expected minibufActive=true")
	}
}

// TestCmdImenu_BashWithEntries verifies Imenu in Bash mode.
func TestCmdImenu_BashWithEntries(t *testing.T) {
	src := "#!/bin/bash\ndeploy() {\n    echo deploy\n}\nrollback() {\n    echo rollback\n}\n"
	e := newTestEditor(src)
	buf(e).SetMode("bash")

	e.cmdImenu()

	if !e.minibufActive {
		t.Fatal("cmdImenu with Bash entries: expected minibufActive=true")
	}
}

// ============================================================================
// clipboardCmd
// ============================================================================

// TestClipboardCmd_Darwin verifies that on macOS the function returns pbcopy.
func TestClipboardCmd_Darwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only test")
	}
	cmd := clipboardCmd()
	if cmd == nil {
		t.Fatal("clipboardCmd on darwin: expected non-nil command")
	}
	if !strings.HasSuffix(cmd.Path, "pbcopy") {
		t.Errorf("clipboardCmd darwin: want pbcopy, got %q", cmd.Path)
	}
}

// TestClipboardCmd_NoneOnNonDarwinWithoutDisplay verifies that when
// WAYLAND_DISPLAY and DISPLAY are both unset (and we are not on macOS),
// clipboardCmd returns nil.
func TestClipboardCmd_NoneOnNonDarwinWithoutDisplay(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("macOS always returns pbcopy; skip nil-return test")
	}
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")

	cmd := clipboardCmd()
	if cmd != nil {
		t.Errorf("clipboardCmd with no display env: expected nil, got %v", cmd.Path)
	}
}

// TestClipboardCmd_WaylandWhenWlCopyPresent checks the Wayland branch when
// wl-copy is available.  On CI/macOS, wl-copy is absent so the test is skipped.
func TestClipboardCmd_WaylandWhenWlCopyPresent(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("darwin: always returns pbcopy, not wl-copy")
	}
	// Locate wl-copy; skip if not installed.
	wlCopy, err := findInPATH("wl-copy")
	if err != nil || wlCopy == "" {
		t.Skip("wl-copy not found in PATH")
	}

	t.Setenv("WAYLAND_DISPLAY", ":0")

	cmd := clipboardCmd()
	if cmd == nil {
		t.Fatal("clipboardCmd with WAYLAND_DISPLAY and wl-copy: expected non-nil")
	}
	if !strings.HasSuffix(cmd.Path, "wl-copy") {
		t.Errorf("want wl-copy, got %q", cmd.Path)
	}
}

// TestClipboardCmd_X11WhenXclipPresent checks the X11/xclip branch.
func TestClipboardCmd_X11WhenXclipPresent(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("darwin: always returns pbcopy")
	}
	xclip, err := findInPATH("xclip")
	if err != nil || xclip == "" {
		t.Skip("xclip not found in PATH")
	}

	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", ":0")

	cmd := clipboardCmd()
	if cmd == nil {
		t.Fatal("clipboardCmd with DISPLAY and xclip: expected non-nil")
	}
	if !strings.HasSuffix(cmd.Path, "xclip") {
		t.Errorf("want xclip, got %q", cmd.Path)
	}
}

// findInPATH looks up a binary in PATH; returns "" if not found (no error).
func findInPATH(name string) (string, error) {
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		full := filepath.Join(dir, name)
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			return full, nil
		}
	}
	return "", nil
}

// ============================================================================
// cmdProjectFindFile
// ============================================================================

// TestCmdProjectFindFile_WithVCRoot verifies that calling cmdProjectFindFile
// inside a git repository opens the minibuffer with a "Project find file:"
// prompt rather than falling back to regular find-file.
func TestCmdProjectFindFile_WithVCRoot(t *testing.T) {
	// Use the gomacs repo itself as a project root — we know it has a .git dir.
	root := "/Users/torstein/src/skybert/gomacs"
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		t.Skip("gomacs .git dir not accessible; skipping")
	}

	e := newTestEditor("")
	buf(e).SetFilename(filepath.Join(root, "dummy.go"))

	e.cmdProjectFindFile()

	if !e.minibufActive {
		t.Fatal("cmdProjectFindFile in VC root: expected minibufActive=true")
	}
	if !strings.Contains(e.minibufPrompt, "Project find file") {
		t.Errorf("cmdProjectFindFile: prompt = %q, want to contain 'Project find file'",
			e.minibufPrompt)
	}
}

// TestCmdProjectFindFile_NoVCRoot verifies that outside any VC root the
// function falls back to regular find-file (which also sets minibufActive).
func TestCmdProjectFindFile_NoVCRoot(t *testing.T) {
	// Use a temp dir that has no .git ancestor.
	dir := t.TempDir()
	e := newTestEditor("")
	buf(e).SetFilename(filepath.Join(dir, "dummy.go"))

	e.cmdProjectFindFile()

	// Either "Project find file" (if temp dir is somehow under a VC root) or
	// the regular "Find file" prompt — either way minibufActive should be true.
	if !e.minibufActive {
		t.Error("cmdProjectFindFile fallback: expected minibufActive=true")
	}
}

// TestCmdProjectFindFile_Completions verifies that after calling
// cmdProjectFindFile in a VC root, the completions callback returns at least
// some results when given an empty query.
func TestCmdProjectFindFile_Completions(t *testing.T) {
	root := "/Users/torstein/src/skybert/gomacs"
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		t.Skip("gomacs .git dir not accessible; skipping")
	}

	e := newTestEditor("")
	buf(e).SetFilename(filepath.Join(root, "dummy.go"))

	e.cmdProjectFindFile()

	if e.minibufCompletions == nil {
		t.Fatal("cmdProjectFindFile: minibufCompletions should be set")
	}
	results := e.minibufCompletions("")
	if len(results) == 0 {
		t.Error("cmdProjectFindFile completions: expected non-empty list for empty query")
	}
}

// ============================================================================
// promptSaveNext
// ============================================================================

// TestPromptSaveNext_AllHandled verifies that when the unsaved slice is
// exhausted, e.quit becomes true.
func TestPromptSaveNext_AllHandled(t *testing.T) {
	e := newTestEditor("hello")
	e.promptSaveNext([]*buffer.Buffer{}, 0)
	if !e.quit {
		t.Error("promptSaveNext: with empty slice, e.quit should be true")
	}
}

// TestPromptSaveNext_IdxBeyondEnd verifies the boundary condition where idx
// starts at len(unsaved).
func TestPromptSaveNext_IdxBeyondEnd(t *testing.T) {
	e := newTestEditor("hello")
	b := buffer.NewWithContent("*unsaved*", "data")
	unsaved := []*buffer.Buffer{b}
	e.promptSaveNext(unsaved, 1) // idx == len(unsaved) → quit
	if !e.quit {
		t.Error("promptSaveNext idx=len: e.quit should be true")
	}
}

// TestPromptSaveNext_SetsReadCharPending verifies that when there is an
// unsaved buffer the function sets up the single-char prompt.
func TestPromptSaveNext_SetsReadCharPending(t *testing.T) {
	e := newTestEditor("hello")
	b := buffer.NewWithContent("*buf1*", "data")
	b.SetModified(true)

	e.promptSaveNext([]*buffer.Buffer{b}, 0)

	if !e.readCharPending {
		t.Error("promptSaveNext: readCharPending should be true while waiting for y/n")
	}
	if e.readCharCallback == nil {
		t.Error("promptSaveNext: readCharCallback should be set")
	}
}

// TestPromptSaveNext_YCallbackSavesFile verifies that answering 'y' writes
// the buffer to disk and marks it unmodified, then proceeds to set quit.
func TestPromptSaveNext_YCallbackSavesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tobesaved.txt")

	e := newTestEditor("hello")
	b := buffer.NewWithContent("*tosave*", "file content")
	b.SetFilename(path)
	b.SetModified(true)

	e.promptSaveNext([]*buffer.Buffer{b}, 0)

	if e.readCharCallback == nil {
		t.Fatal("readCharCallback should be set")
	}
	// Simulate the user pressing 'y'.
	e.readCharCallback('y')

	// File should have been written.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file was not created after 'y': %v", err)
	}
	if string(data) != "file content" {
		t.Errorf("file content = %q, want %q", string(data), "file content")
	}
	// Buffer should be marked as unmodified.
	if b.Modified() {
		t.Error("buffer should be marked unmodified after save")
	}
	// Since it was the only buffer, quit should now be true.
	if !e.quit {
		t.Error("e.quit should be true after all buffers handled")
	}
}

// TestPromptSaveNext_NCallbackSkipsFile verifies that answering 'n' does NOT
// write the file but still progresses through the list and sets quit.
func TestPromptSaveNext_NCallbackSkipsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "skipped.txt")

	e := newTestEditor("hello")
	b := buffer.NewWithContent("*skip*", "unsaved")
	b.SetFilename(path)
	b.SetModified(true)

	e.promptSaveNext([]*buffer.Buffer{b}, 0)

	if e.readCharCallback == nil {
		t.Fatal("readCharCallback should be set")
	}
	// Simulate the user pressing 'n'.
	e.readCharCallback('n')

	// File must NOT have been created.
	if _, err := os.Stat(path); err == nil {
		t.Error("file should NOT have been written after 'n'")
	}
	// Buffer should still be modified.
	if !b.Modified() {
		t.Error("buffer should still be marked modified after 'n'")
	}
	// But quit should still be set (all buffers handled).
	if !e.quit {
		t.Error("e.quit should be true after skipping the only unsaved buffer")
	}
}

// TestPromptSaveNext_MessageContainsBufferName verifies that the message
// prompt includes the buffer name so the user knows what they are being
// asked about.
func TestPromptSaveNext_MessageContainsBufferName(t *testing.T) {
	e := newTestEditor("hello")
	b := buffer.NewWithContent("myspecialfile.go", "data")
	b.SetModified(true)

	e.promptSaveNext([]*buffer.Buffer{b}, 0)

	if !strings.Contains(e.message, "myspecialfile.go") {
		t.Errorf("promptSaveNext: message %q should contain buffer name", e.message)
	}
}

// TestCmdSaveBuffersKillTerminal_WithUnsaved verifies the full flow:
// an unsaved buffer with a backing file triggers the prompt loop.
func TestCmdSaveBuffersKillTerminal_WithUnsaved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "modified.txt")
	// Pre-create the file.
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}

	e := newTestEditor("original")
	buf(e).SetFilename(path)
	buf(e).SetModified(true)

	e.cmdSaveBuffersKillTerminal()

	// Should not have quit yet — should be waiting for user input.
	if e.quit {
		t.Error("cmdSaveBuffersKillTerminal: should not quit before user responds")
	}
	if !e.readCharPending {
		t.Error("cmdSaveBuffersKillTerminal: should be waiting for char input")
	}
}
