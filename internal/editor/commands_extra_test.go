package editor

// commands_extra_test.go covers branches of commands.go and text.go that were
// not yet exercised by the existing test files.  New tests are appended here
// rather than modifying the committed test files.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/syntax"
	"github.com/skybert/gomacs/internal/terminal"
)

// newTestEditorFull returns a test editor that also has the auxiliary maps
// (autoRevertMtimes, shellStates, customHighlighters, spanCaches) that are
// required by writeBuffer and KillBuffer.  Use this for tests that exercise
// those code paths.
func newTestEditorFull(content string) *Editor {
	e := newTestEditor(content)
	e.autoRevertMtimes = make(map[*buffer.Buffer]time.Time)
	e.shellStates = make(map[*buffer.Buffer]*shellState)
	e.customHighlighters = make(map[*buffer.Buffer]syntax.Highlighter)
	e.spanCaches = make(map[*buffer.Buffer]*spanCache)
	return e
}

// ---------------------------------------------------------------------------
// cmdUndo / cmdRedo
// ---------------------------------------------------------------------------

func TestCmdUndo_NoHistory(t *testing.T) {
	e := newTestEditor("hello")
	e.cmdUndo()
	if !strings.Contains(e.message, "No further undo") {
		t.Errorf("expected 'No further undo' message, got %q", e.message)
	}
}

func TestCmdUndo_AfterEdit(t *testing.T) {
	e := newElispTestEditor("")
	e.selfInsert('X')
	e.selfInsert('Y')
	e.cmdUndo()
	if strings.Contains(e.ActiveBuffer().String(), "Y") && strings.Contains(e.ActiveBuffer().String(), "X") {
		// undo should have removed at least the most recent insertion
		t.Errorf("undo did not revert insertion: %q", e.ActiveBuffer().String())
	}
}

func TestCmdUndo_ReadOnly(t *testing.T) {
	e := newTestEditor("data")
	e.ActiveBuffer().SetReadOnly(true)
	e.cmdUndo()
	if e.ActiveBuffer().String() != "data" {
		t.Error("cmdUndo on read-only buffer must not modify it")
	}
}

func TestCmdRedo_NoHistory(t *testing.T) {
	e := newTestEditor("hello")
	e.cmdRedo()
	if !strings.Contains(e.message, "No further redo") {
		t.Errorf("expected 'No further redo' message, got %q", e.message)
	}
}

func TestCmdRedo_AfterUndo(t *testing.T) {
	e := newElispTestEditor("")
	e.selfInsert('A')
	e.cmdUndo()
	e.cmdRedo()
	// Redo should not panic and the buffer is in a defined state.
	_ = e.ActiveBuffer().String()
}

// ---------------------------------------------------------------------------
// cmdDescribeFunction / cmdDescribeVariable
// ---------------------------------------------------------------------------

func TestCmdDescribeFunction_ShowsHelp(t *testing.T) {
	e := newElispTestEditor("")
	e.setupKeymaps()
	e.cmdDescribeFunction()
	if e.minibufDoneFunc == nil {
		t.Fatal("cmdDescribeFunction should activate the minibuffer")
	}
	e.minibufDoneFunc("forward-char")
	if e.FindBuffer("*Help*") == nil {
		t.Error("expected *Help* buffer after describing a function")
	}
}

func TestCmdDescribeFunction_EmptyNameNoHelp(t *testing.T) {
	e := newElispTestEditor("")
	e.setupKeymaps()
	e.cmdDescribeFunction()
	e.minibufDoneFunc("")
	if e.FindBuffer("*Help*") != nil {
		t.Error("empty function name should not create *Help*")
	}
}

func TestCmdDescribeVariable_ShowsHelp(t *testing.T) {
	e := newElispTestEditor("")
	_, _ = e.lisp.EvalString("(setq fill-column 80)")
	e.cmdDescribeVariable()
	if e.minibufDoneFunc == nil {
		t.Fatal("cmdDescribeVariable should activate the minibuffer")
	}
	e.minibufDoneFunc("fill-column")
	if e.FindBuffer("*Help*") == nil {
		t.Error("expected *Help* buffer after describing a variable")
	}
}

// ---------------------------------------------------------------------------
// cmdLoadTheme
// ---------------------------------------------------------------------------

func TestCmdLoadTheme_Known(t *testing.T) {
	e := newElispTestEditor("")
	e.cmdLoadTheme()
	if e.minibufDoneFunc == nil {
		t.Fatal("cmdLoadTheme should activate the minibuffer")
	}
	e.minibufDoneFunc("sweet")
	if strings.Contains(e.message, "Unknown theme") {
		t.Errorf("'sweet' should be a known theme, got %q", e.message)
	}
}

func TestCmdLoadTheme_Unknown(t *testing.T) {
	e := newElispTestEditor("")
	e.cmdLoadTheme()
	e.minibufDoneFunc("no-such-theme-xyz")
	if !strings.Contains(e.message, "Unknown theme") {
		t.Errorf("expected 'Unknown theme' message, got %q", e.message)
	}
}

// ---------------------------------------------------------------------------
// cmdDeleteOtherWindows
// ---------------------------------------------------------------------------

func TestCmdDeleteOtherWindows(t *testing.T) {
	e := newTestEditor("hello")
	e.cmdSplitWindowBelow()
	if len(e.windows) != 2 {
		t.Fatalf("expected 2 windows after split, got %d", len(e.windows))
	}
	e.cmdDeleteOtherWindows()
	if len(e.windows) != 1 {
		t.Errorf("expected 1 window after delete-other-windows, got %d", len(e.windows))
	}
}

// ---------------------------------------------------------------------------
// Read-only guards: editing commands must no-op on a read-only buffer.
// ---------------------------------------------------------------------------

func TestEditingCommandsReadOnlyGuards(t *testing.T) {
	const content = "hello world\nsecond line\nthird\n"
	cmds := map[string]func(*Editor){
		"newline":              (*Editor).cmdNewline,
		"deleteChar":           (*Editor).cmdDeleteChar,
		"backwardDeleteChar":   (*Editor).cmdBackwardDeleteChar,
		"killLine":             (*Editor).cmdKillLine,
		"killRegion":           (*Editor).cmdKillRegion,
		"yank":                 (*Editor).cmdYank,
		"yankPop":              (*Editor).cmdYankPop,
		"killWord":             (*Editor).cmdKillWord,
		"backwardKillWord":     (*Editor).cmdBackwardKillWord,
		"transposeChars":       (*Editor).cmdTransposeChars,
		"openLine":             (*Editor).cmdOpenLine,
		"killSentence":         (*Editor).cmdKillSentence,
		"transposeWords":       (*Editor).cmdTransposeWords,
		"deleteBlankLines":     (*Editor).cmdDeleteBlankLines,
		"joinLine":             (*Editor).cmdJoinLine,
		"upcaseRegion":         (*Editor).cmdUpcaseRegion,
		"downcaseRegion":       (*Editor).cmdDowncaseRegion,
		"sortLines":            (*Editor).cmdSortLines,
		"deleteDuplicateLines": (*Editor).cmdDeleteDuplicateLines,
		"fillParagraph":        (*Editor).cmdFillParagraph,
		"indentRegion":         (*Editor).cmdIndentRegion,
		"indentRigidly":        (*Editor).cmdIndentRigidly,
	}
	for name, fn := range cmds {
		e := newElispTestEditor(content)
		e.ActiveBuffer().SetReadOnly(true)
		e.ActiveBuffer().SetPoint(3)
		fn(e)
		if e.ActiveBuffer().String() != content {
			t.Errorf("%s mutated a read-only buffer: %q", name, e.ActiveBuffer().String())
		}
	}
}

// newFindFileEditor returns an editor with the maps that loadFile / openDired need.
func newFindFileEditor(content string) *Editor {
	e := newTestEditorFull(content)
	e.lspConns = make(map[string]*lspConn)
	e.diredStates = make(map[*buffer.Buffer]*diredState)
	// Non-nil terminal (screen==nil) so async callbacks calling PostWakeup are safe.
	e.term = &terminal.Terminal{}
	return e
}

func TestCmdFindFile_OpensFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "open.txt")
	_ = os.WriteFile(path, []byte("hello"), 0o644)
	e := newFindFileEditor("")
	e.cmdFindFile()
	if e.minibufDoneFunc == nil {
		t.Fatal("cmdFindFile should activate the minibuffer")
	}
	e.minibufDoneFunc(path)
	if e.ActiveBuffer().Filename() != path {
		t.Errorf("expected active buffer %q, got %q", path, e.ActiveBuffer().Filename())
	}
}

func TestCmdFindFile_DirectoryOpensDired(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644)
	e := newFindFileEditor("")
	e.cmdFindFile()
	e.minibufDoneFunc(dir)
	if e.ActiveBuffer().Mode() != "dired" {
		t.Errorf("a directory path should open dired, got mode %q", e.ActiveBuffer().Mode())
	}
}

func TestCmdFindFile_EmptyNoop(t *testing.T) {
	e := newFindFileEditor("content")
	before := e.ActiveBuffer()
	e.cmdFindFile()
	e.minibufDoneFunc("")
	if e.ActiveBuffer() != before {
		t.Error("empty path should not change the active buffer")
	}
}

func TestCmdFindFile_TildeExpansion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	e := newFindFileEditor("")
	e.cmdFindFile()
	e.minibufDoneFunc("~/newfile.txt")
	if !strings.HasPrefix(e.ActiveBuffer().Filename(), home) {
		t.Errorf("~ should expand to home dir, got %q", e.ActiveBuffer().Filename())
	}
}

func TestCmdProjectFindFile_OpensRelativePath(t *testing.T) {
	dir := makeGitRepo(t)
	sub := filepath.Join(dir, "pkg")
	_ = os.Mkdir(sub, 0o755)
	target := filepath.Join(sub, "file.txt")
	_ = os.WriteFile(target, []byte("data"), 0o644)

	e := newFindFileEditor("")
	e.ActiveBuffer().SetFilename(filepath.Join(dir, "notes.txt"))
	e.cmdProjectFindFile()
	if e.minibufDoneFunc == nil {
		t.Fatal("cmdProjectFindFile should activate the minibuffer in a VC repo")
	}
	e.minibufDoneFunc(filepath.Join("pkg", "file.txt"))
	if e.ActiveBuffer().Filename() != target {
		t.Errorf("expected to open %q, got %q", target, e.ActiveBuffer().Filename())
	}
}

func TestCmdProjectFindFile_NoVCFallsBack(t *testing.T) {
	dir := t.TempDir() // not a git repo
	e := newFindFileEditor("")
	e.ActiveBuffer().SetFilename(filepath.Join(dir, "loose.txt"))
	e.cmdProjectFindFile()
	// Falls back to cmdFindFile → minibuffer prompt active.
	if !e.minibufActive {
		t.Error("cmdProjectFindFile with no VC root should fall back to find-file")
	}
}

// ---------------------------------------------------------------------------
// cmdExecuteExtendedCommand — callback branches
// ---------------------------------------------------------------------------

// executeExtendedCommandWithName simulates the user typing name into the M-x
// minibuffer and pressing Enter by directly invoking the done callback.
func executeExtendedCommandWithName(e *Editor, name string) {
	// cmdExecuteExtendedCommand sets up the minibuffer; we then call the
	// done callback directly to avoid needing a real terminal.
	e.cmdExecuteExtendedCommand()
	e.minibufDoneFunc(name)
}

func TestCmdExecuteExtendedCommandUnknownShowsMessage(t *testing.T) {
	e := newTestEditor("")
	executeExtendedCommandWithName(e, "no-such-command-xyz")
	if !strings.Contains(e.message, "No command") {
		t.Errorf("execute-extended-command unknown: want 'No command' in message, got %q", e.message)
	}
}

func TestCmdExecuteExtendedCommandEmptyNoOp(t *testing.T) {
	e := newTestEditor("hello")
	before := buf(e).String()
	executeExtendedCommandWithName(e, "   ")
	// Buffer unchanged; no message about "No command".
	if buf(e).String() != before {
		t.Errorf("execute-extended-command empty: buffer should be unchanged")
	}
	if strings.Contains(e.message, "No command") {
		t.Errorf("execute-extended-command empty: should not produce 'No command' message")
	}
}

func TestCmdExecuteExtendedCommandKnownExecutes(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetPoint(0)
	// "forward-char" is a known command; executing it should move the point.
	executeExtendedCommandWithName(e, "forward-char")
	if got := buf(e).Point(); got != 1 {
		t.Errorf("execute-extended-command forward-char: want point=1, got %d", got)
	}
}

func TestCmdExecuteExtendedCommandUpdatesLRU(t *testing.T) {
	e := newTestEditor("")
	executeExtendedCommandWithName(e, "forward-char")
	found := false
	for _, name := range e.commandLRU {
		if name == "forward-char" {
			found = true
			break
		}
	}
	if !found {
		t.Error("execute-extended-command: 'forward-char' not added to commandLRU")
	}
}

// ---------------------------------------------------------------------------
// cmdKillBuffer — minibuffer callback branches
// ---------------------------------------------------------------------------

// killBufferWithName simulates confirm in the kill-buffer minibuffer.
func killBufferWithName(e *Editor, name string) {
	e.cmdKillBuffer()
	e.minibufDoneFunc(name)
}

func TestCmdKillBufferDefaultKillsCurrentBuffer(t *testing.T) {
	e := newTestEditor("content")
	// Add a second buffer so *scratch* exists after the kill.
	extra := buffer.NewWithContent("*extra*", "data")
	e.buffers = append(e.buffers, extra)
	activeName := buf(e).Name()
	// Passing empty string means "use default" (the current buffer name).
	killBufferWithName(e, "")
	for _, b := range e.buffers {
		if b.Name() == activeName {
			t.Errorf("cmdKillBuffer default: killed buffer %q still in e.buffers", activeName)
		}
	}
}

func TestCmdKillBufferExplicitName(t *testing.T) {
	e := newTestEditor("content")
	extra := buffer.NewWithContent("*extra*", "more data")
	e.buffers = append(e.buffers, extra)
	killBufferWithName(e, "*extra*")
	for _, b := range e.buffers {
		if b.Name() == "*extra*" {
			t.Error("cmdKillBuffer explicit name: killed buffer still in e.buffers")
		}
	}
}

func TestCmdKillBufferSetsMessage(t *testing.T) {
	e := newTestEditor("content")
	extra := buffer.NewWithContent("*extra*", "data")
	e.buffers = append(e.buffers, extra)
	killBufferWithName(e, "*extra*")
	if !strings.Contains(e.message, "*extra*") {
		t.Errorf("cmdKillBuffer: message should mention buffer name, got %q", e.message)
	}
}

// ---------------------------------------------------------------------------
// cmdSwitchToBuffer — callback branch that actually switches
// ---------------------------------------------------------------------------

func TestCmdSwitchToBufferSwitchesToExistingBuffer(t *testing.T) {
	e := newTestEditor("first buffer")
	second := buffer.NewWithContent("*second*", "second buffer content")
	e.buffers = append(e.buffers, second)

	// Invoke through minibuf callback directly.
	e.cmdSwitchToBuffer()
	e.minibufDoneFunc("*second*")

	if e.activeWin.Buf().Name() != "*second*" {
		t.Errorf("cmdSwitchToBuffer: expected active buffer %q, got %q",
			"*second*", e.activeWin.Buf().Name())
	}
}

func TestCmdSwitchToBufferDefaultUsedOnEmptyInput(t *testing.T) {
	e := newTestEditor("first")
	second := buffer.NewWithContent("*second*", "second")
	e.buffers = append(e.buffers, second)
	// Make *second* the MRU so it becomes the default.
	e.bufferMRU = []*buffer.Buffer{second}

	e.cmdSwitchToBuffer()
	// Passing empty string should resolve to the default.
	e.minibufDoneFunc("")

	if e.activeWin.Buf().Name() != "*second*" {
		t.Errorf("cmdSwitchToBuffer empty→default: expected %q, got %q",
			"*second*", e.activeWin.Buf().Name())
	}
}

// ---------------------------------------------------------------------------
// cmdSaveSomeBuffers — branch: modified buffer with filename gets written
// ---------------------------------------------------------------------------

func TestCmdSaveSomeBuffersModifiedFileGetsWritten(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	e := newTestEditorFull("hello world")
	buf(e).SetFilename(path)
	buf(e).SetModified(true)

	e.cmdSaveSomeBuffers()

	// File should now exist on disk.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cmdSaveSomeBuffers: file not written: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("cmdSaveSomeBuffers: file content = %q, want %q", string(data), "hello world")
	}
}

func TestCmdSaveSomeBuffersReportsCountOnSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test2.txt")

	e := newTestEditorFull("content")
	buf(e).SetFilename(path)
	buf(e).SetModified(true)

	e.cmdSaveSomeBuffers()

	if !strings.Contains(e.message, "Saved") {
		t.Errorf("cmdSaveSomeBuffers: expected 'Saved' in message, got %q", e.message)
	}
}

func TestCmdSaveSomeBuffersModifiedWithoutFilenameSkipped(t *testing.T) {
	e := newTestEditorFull("dirty")
	buf(e).SetModified(true)
	// No filename set — should count as zero files to save.

	e.cmdSaveSomeBuffers()

	if e.message != "(No files need saving)" {
		t.Errorf("cmdSaveSomeBuffers modified no filename: want %q, got %q",
			"(No files need saving)", e.message)
	}
}

func TestCmdSaveSomeBuffersMarksBufferClean(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clean.txt")

	e := newTestEditorFull("content")
	buf(e).SetFilename(path)
	buf(e).SetModified(true)

	e.cmdSaveSomeBuffers()

	if buf(e).Modified() {
		t.Error("cmdSaveSomeBuffers: buffer should be marked clean after save")
	}
}

// ---------------------------------------------------------------------------
// cmdSaveBuffer — branches: no filename prompts minibuffer; has filename writes
// ---------------------------------------------------------------------------

func TestCmdSaveBufferWithFilenameWritesToDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "save.txt")

	e := newTestEditorFull("saved content")
	buf(e).SetFilename(path)

	e.cmdSaveBuffer()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cmdSaveBuffer: file not written: %v", err)
	}
	if string(data) != "saved content" {
		t.Errorf("cmdSaveBuffer: want %q, got %q", "saved content", string(data))
	}
}

func TestCmdSaveBufferWithFilenameMarksClean(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clean.txt")

	e := newTestEditorFull("data")
	buf(e).SetFilename(path)
	buf(e).SetModified(true)

	e.cmdSaveBuffer()

	if buf(e).Modified() {
		t.Error("cmdSaveBuffer: buffer should be marked clean after save")
	}
}

func TestCmdSaveBufferWithFilenameShowsMessage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "msg.txt")

	e := newTestEditorFull("text")
	buf(e).SetFilename(path)

	e.cmdSaveBuffer()

	if !strings.Contains(e.message, "Wrote") {
		t.Errorf("cmdSaveBuffer: want 'Wrote' in message, got %q", e.message)
	}
}

func TestCmdSaveBufferNoFilenameOpensMinibuffer(t *testing.T) {
	e := newTestEditorFull("unsaved content")
	// No filename — should prompt.
	e.cmdSaveBuffer()
	if !e.minibufActive {
		t.Error("cmdSaveBuffer no filename: expected minibufActive=true")
	}
}

func TestCmdSaveBufferNoFilenameCallbackSetsFilenameAndWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.txt")

	e := newTestEditorFull("new content")
	e.cmdSaveBuffer()
	// Simulate user supplying a path.
	e.minibufDoneFunc(path)

	if buf(e).Filename() != path {
		t.Errorf("cmdSaveBuffer callback: filename want %q, got %q", path, buf(e).Filename())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cmdSaveBuffer callback: file not written: %v", err)
	}
	if string(data) != "new content" {
		t.Errorf("cmdSaveBuffer callback: file content = %q, want %q", string(data), "new content")
	}
}

// ---------------------------------------------------------------------------
// cmdDeleteOtherWindows — with multiple windows
// ---------------------------------------------------------------------------

func TestCmdDeleteOtherWindowsReducesToOne(t *testing.T) {
	e := newTestEditor("hello")
	e.cmdSplitWindowBelow()
	if len(e.windows) != 2 {
		t.Fatalf("pre-condition: expected 2 windows, got %d", len(e.windows))
	}
	e.cmdDeleteOtherWindows()
	if len(e.windows) != 1 {
		t.Fatalf("cmdDeleteOtherWindows: expected 1 window, got %d", len(e.windows))
	}
}

func TestCmdDeleteOtherWindowsKeepsActiveWindow(t *testing.T) {
	e := newTestEditor("hello")
	e.cmdSplitWindowBelow()
	// Switch to the second window and then delete others.
	e.cmdOtherWindow()
	active := e.activeWin
	e.cmdDeleteOtherWindows()
	if e.windows[0] != active {
		t.Error("cmdDeleteOtherWindows: remaining window is not the active one")
	}
}

func TestCmdDeleteOtherWindowsSingleWindowNoOp(t *testing.T) {
	e := newTestEditor("hello")
	before := e.activeWin
	e.cmdDeleteOtherWindows()
	if len(e.windows) != 1 {
		t.Errorf("cmdDeleteOtherWindows single window: expected 1, got %d", len(e.windows))
	}
	if e.activeWin != before {
		t.Error("cmdDeleteOtherWindows single window: active window changed unexpectedly")
	}
}

// ---------------------------------------------------------------------------
// cmdUpcaseWord — with subwordMode
// ---------------------------------------------------------------------------

func TestUpcaseWordWithArgMultipleWords(t *testing.T) {
	e := newTestEditor("hello world foo")
	buf(e).SetPoint(0)
	e.universalArg = 2
	e.universalArgSet = true
	e.cmdUpcaseWord()
	got := buf(e).String()
	if got != "HELLO WORLD foo" {
		t.Errorf("upcase-word x2: want %q, got %q", "HELLO WORLD foo", got)
	}
}

func TestUpcaseWordAtEndOfBuffer(t *testing.T) {
	// Point past the last word — nothing to upcase.
	e := newTestEditor("hello")
	buf(e).SetPoint(5)
	e.cmdUpcaseWord()
	got := buf(e).String()
	if got != "hello" {
		t.Errorf("upcase-word at end: buffer should be unchanged, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// cmdDowncaseWord — with subwordMode
// ---------------------------------------------------------------------------

func TestDowncaseWordWithArgMultipleWords(t *testing.T) {
	e := newTestEditor("HELLO WORLD foo")
	buf(e).SetPoint(0)
	e.universalArg = 2
	e.universalArgSet = true
	e.cmdDowncaseWord()
	got := buf(e).String()
	if got != "hello world foo" {
		t.Errorf("downcase-word x2: want %q, got %q", "hello world foo", got)
	}
}

func TestDowncaseWordAtEndOfBuffer(t *testing.T) {
	e := newTestEditor("HELLO")
	buf(e).SetPoint(5)
	e.cmdDowncaseWord()
	got := buf(e).String()
	if got != "HELLO" {
		t.Errorf("downcase-word at end: buffer should be unchanged, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// cmdCapitalizeWord — extra coverage
// ---------------------------------------------------------------------------

func TestCapitalizeWordWithArgMultipleWords(t *testing.T) {
	e := newTestEditor("HELLO WORLD foo")
	buf(e).SetPoint(0)
	e.universalArg = 2
	e.universalArgSet = true
	e.cmdCapitalizeWord()
	got := buf(e).String()
	if got != "Hello World foo" {
		t.Errorf("capitalize-word x2: want %q, got %q", "Hello World foo", got)
	}
}

func TestCapitalizeWordAtEndOfBuffer(t *testing.T) {
	e := newTestEditor("hello")
	buf(e).SetPoint(5)
	e.cmdCapitalizeWord()
	got := buf(e).String()
	if got != "hello" {
		t.Errorf("capitalize-word at end: buffer should be unchanged, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// cmdFillParagraph — various branch coverage
// ---------------------------------------------------------------------------

func TestCmdFillParagraphBasic(t *testing.T) {
	e := newTestEditor("one two three four five six seven eight nine ten")
	e.fillColumn = 20
	buf(e).SetPoint(0)
	e.cmdFillParagraph()
	got := buf(e).String()
	// Each line should be no wider than 20 columns.
	for _, line := range strings.Split(got, "\n") {
		if len([]rune(line)) > 20 {
			t.Errorf("cmdFillParagraph: line too long (%d > 20): %q", len([]rune(line)), line)
		}
	}
	// All words should still be present.
	for _, w := range []string{"one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten"} {
		if !strings.Contains(got, w) {
			t.Errorf("cmdFillParagraph: word %q missing from result", w)
		}
	}
}

func TestCmdFillParagraphReadOnlyNoOp(t *testing.T) {
	e := newTestEditor("one two three four five")
	buf(e).SetReadOnly(true)
	buf(e).SetPoint(0)
	e.fillColumn = 10
	before := buf(e).String()
	e.cmdFillParagraph()
	if buf(e).String() != before {
		t.Errorf("cmdFillParagraph read-only: buffer should be unchanged, got %q", buf(e).String())
	}
}

func TestCmdFillParagraphEmptyParagraph(t *testing.T) {
	// An empty buffer — should be a no-op.
	e := newTestEditor("")
	e.fillColumn = 70
	buf(e).SetPoint(0)
	e.cmdFillParagraph()
	if buf(e).String() != "" {
		t.Errorf("cmdFillParagraph empty: expected empty buffer, got %q", buf(e).String())
	}
}

func TestCmdFillParagraphMultilineParagraph(t *testing.T) {
	// A paragraph already split into multiple lines — should be joined and reflowed.
	content := "The quick brown fox\njumps over the lazy dog.\n"
	e := newTestEditor(content)
	e.fillColumn = 40
	buf(e).SetPoint(0)
	e.cmdFillParagraph()
	got := buf(e).String()
	// All words must be present.
	for _, w := range strings.Fields(content) {
		if !strings.Contains(got, w) {
			t.Errorf("cmdFillParagraph multiline: word %q missing", w)
		}
	}
}

func TestCmdFillParagraphStopsAtBlankLine(t *testing.T) {
	// Two paragraphs separated by a blank line — only the first should be filled.
	content := "first paragraph here\n\nsecond paragraph here\n"
	e := newTestEditor(content)
	e.fillColumn = 70
	buf(e).SetPoint(0)
	e.cmdFillParagraph()
	got := buf(e).String()
	// Second paragraph must be untouched.
	if !strings.Contains(got, "second paragraph here") {
		t.Errorf("cmdFillParagraph: second paragraph should be unchanged, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// cmdDowncaseRegion / cmdUpcaseRegion — extra branches
// ---------------------------------------------------------------------------

func TestCmdDowncaseRegionNoRegionNoOp(t *testing.T) {
	e := newTestEditor("Hello World")
	buf(e).SetMarkActive(false)
	buf(e).SetPoint(5)
	before := buf(e).String()
	e.cmdDowncaseRegion()
	if buf(e).String() != before {
		t.Errorf("cmdDowncaseRegion no region: buffer changed, got %q", buf(e).String())
	}
}

func TestCmdDowncaseRegionWithRegion(t *testing.T) {
	e := newTestEditor("Hello World")
	b := buf(e)
	b.SetMark(0)
	b.SetMarkActive(true)
	b.SetPoint(5)
	e.cmdDowncaseRegion()
	got := b.String()
	if got != "hello World" {
		t.Errorf("cmdDowncaseRegion: want %q, got %q", "hello World", got)
	}
}

func TestCmdDowncaseRegionReadOnlyNoOp(t *testing.T) {
	e := newTestEditor("Hello World")
	b := buf(e)
	b.SetReadOnly(true)
	b.SetMark(0)
	b.SetMarkActive(true)
	b.SetPoint(5)
	before := b.String()
	e.cmdDowncaseRegion()
	if b.String() != before {
		t.Errorf("cmdDowncaseRegion read-only: buffer changed, got %q", b.String())
	}
}

func TestCmdUpcaseRegionNoRegionNoOp(t *testing.T) {
	e := newTestEditor("hello world")
	buf(e).SetMarkActive(false)
	buf(e).SetPoint(5)
	before := buf(e).String()
	e.cmdUpcaseRegion()
	if buf(e).String() != before {
		t.Errorf("cmdUpcaseRegion no region: buffer changed, got %q", buf(e).String())
	}
}

func TestCmdUpcaseRegionWithRegion(t *testing.T) {
	e := newTestEditor("hello world")
	b := buf(e)
	b.SetMark(0)
	b.SetMarkActive(true)
	b.SetPoint(5)
	e.cmdUpcaseRegion()
	got := b.String()
	if got != "HELLO world" {
		t.Errorf("cmdUpcaseRegion: want %q, got %q", "HELLO world", got)
	}
}

func TestCmdUpcaseRegionReadOnlyNoOp(t *testing.T) {
	e := newTestEditor("hello world")
	b := buf(e)
	b.SetReadOnly(true)
	b.SetMark(0)
	b.SetMarkActive(true)
	b.SetPoint(5)
	before := b.String()
	e.cmdUpcaseRegion()
	if b.String() != before {
		t.Errorf("cmdUpcaseRegion read-only: buffer changed, got %q", b.String())
	}
}

// ---------------------------------------------------------------------------
// cmdSaveBuffer — trailing-whitespace deletion (saveBufferDeleteTrailingWS)
// ---------------------------------------------------------------------------

func TestCmdSaveBufferDeletesTrailingWhitespace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trailing.txt")

	e := newTestEditorFull("hello   \nworld  \n")
	e.saveBufferDeleteTrailingWS = true
	buf(e).SetFilename(path)

	e.cmdSaveBuffer()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cmdSaveBuffer trailing WS: file not written: %v", err)
	}
	got := string(data)
	want := "hello\nworld\n"
	if got != want {
		t.Errorf("cmdSaveBuffer trailing WS: want %q, got %q", want, got)
	}
}

// ---------------------------------------------------------------------------
// cmdSaveSomeBuffers — multiple buffers, only the modified ones with filenames
// ---------------------------------------------------------------------------

func TestCmdSaveSomeBuffersMultipleBuffers(t *testing.T) {
	dir := t.TempDir()
	path1 := filepath.Join(dir, "one.txt")
	path2 := filepath.Join(dir, "two.txt")

	e := newTestEditorFull("buffer one content")
	buf(e).SetFilename(path1)
	buf(e).SetModified(true)

	b2 := buffer.NewWithContent("*b2*", "buffer two content")
	b2.SetFilename(path2)
	b2.SetModified(true)
	e.buffers = append(e.buffers, b2)

	e.cmdSaveSomeBuffers()

	// Both files should exist.
	for _, p := range []string{path1, path2} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("cmdSaveSomeBuffers: file %q not written: %v", p, err)
		}
	}
	// Message should mention 2 files.
	if !strings.Contains(e.message, "2") {
		t.Errorf("cmdSaveSomeBuffers: message should mention 2, got %q", e.message)
	}
}
