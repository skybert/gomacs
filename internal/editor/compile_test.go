package editor

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/terminal"
)

func TestDefaultBuildCommand_Make(t *testing.T) {
	dir := t.TempDir()
	if got := defaultBuildCommand(dir); got != "make" {
		t.Errorf("want \"make\", got %q", got)
	}
}

func TestDefaultBuildCommand_Maven(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pom.xml"), []byte("<project/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := defaultBuildCommand(dir); got != "mvn clean install" {
		t.Errorf("want \"mvn clean install\", got %q", got)
	}
}

func TestErrRe_GoCompilerLine(t *testing.T) {
	line := "internal/editor/editor.go:42: undefined: foo"
	m := errRe.FindStringSubmatch(line)
	if m == nil {
		t.Fatalf("errRe did not match Go compiler line %q", line)
	}
	if m[1] != "internal/editor/editor.go" {
		t.Errorf("file = %q, want %q", m[1], "internal/editor/editor.go")
	}
	if m[2] != "42" {
		t.Errorf("line = %q, want %q", m[2], "42")
	}
}

func TestErrRe_WithColumn(t *testing.T) {
	line := "src/main.go:10:5: syntax error"
	m := errRe.FindStringSubmatch(line)
	if m == nil {
		t.Fatalf("errRe did not match %q", line)
	}
	if m[2] != "10" {
		t.Errorf("line = %q, want \"10\"", m[2])
	}
	if m[3] != "5" {
		t.Errorf("col = %q, want \"5\"", m[3])
	}
}

func TestErrRe_NoMatch(t *testing.T) {
	if errRe.MatchString("this is a plain info line") {
		t.Error("errRe should not match a plain info line")
	}
}

// newCompileTestEditor builds a capture-backed editor with the maps needed to
// drive runBuild and gotoCompilationError without a real event loop.
func newCompileTestEditor(content string) *Editor {
	e := newCapTestEditor(content)
	e.lspCbs = make(chan func(), 16)
	e.autoRevertMtimes = make(map[*buffer.Buffer]time.Time)
	e.lspConns = make(map[string]*lspConn)
	e.spanCaches = make(map[*buffer.Buffer]*spanCache)
	return e
}

func TestCmdBuild_PrefillsDefault(t *testing.T) {
	e := newCompileTestEditor("")
	e.cmdBuild()
	if e.minibufDoneFunc == nil {
		t.Fatal("cmdBuild should activate the minibuffer")
	}
	// The minibuffer should be pre-filled with a default build command.
	got := e.minibufBuf.String()
	if got != "make" && got != "mvn clean install" {
		t.Fatalf("expected default build command pre-filled, got %q", got)
	}
}

func TestCmdBuild_RunsCommand(t *testing.T) {
	e := newCompileTestEditor("")
	e.cmdBuild()
	if e.minibufDoneFunc == nil {
		t.Fatal("cmdBuild should activate the minibuffer")
	}
	e.minibufDoneFunc("echo built")
	// runBuild created the *compilation* buffer and scheduled an async callback.
	if e.FindBuffer("*compilation*") == nil {
		t.Fatal("cmdBuild done-func should create the *compilation* buffer")
	}
	select {
	case fn := <-e.lspCbs:
		fn()
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for build callback")
	}
}

func TestCmdBuild_EmptyUsesDefaultPrefill(t *testing.T) {
	// With no explicit input the minibuffer keeps the pre-filled default; we
	// assert the prefill rather than executing it (the default could be a slow
	// real build in the surrounding repo).
	e := newCompileTestEditor("")
	e.cmdBuild()
	if e.minibufBuf.String() == "" {
		t.Error("cmdBuild should pre-fill a default build command")
	}
}

func TestRunBuild_ParsesErrorsAndPopulatesBuffer(t *testing.T) {
	e := newCompileTestEditor("")
	dir := t.TempDir()
	// echo emits a compiler-style error line that errRe can parse.
	e.runBuild(dir, "echo x.txt:10:5: oops")

	comp := e.FindBuffer("*compilation*")
	if comp == nil {
		t.Fatal("runBuild should create *compilation* buffer")
	}
	// Drain and run the async completion callback.
	select {
	case fn := <-e.lspCbs:
		fn()
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for build completion callback")
	}

	if len(e.compilationErrors) != 1 {
		t.Fatalf("expected 1 parsed error, got %d", len(e.compilationErrors))
	}
	ce := e.compilationErrors[0]
	if ce.Line != 10 || ce.Col != 5 {
		t.Fatalf("expected line 10 col 5, got line %d col %d", ce.Line, ce.Col)
	}
	if ce.File != filepath.Join(dir, "x.txt") {
		t.Fatalf("expected joined path, got %q", ce.File)
	}
	if e.compilationExitOK == nil || !*e.compilationExitOK {
		t.Fatal("echo should exit OK")
	}
}

func TestRunBuild_NoErrors(t *testing.T) {
	e := newCompileTestEditor("")
	dir := t.TempDir()
	e.runBuild(dir, "echo all good")
	select {
	case fn := <-e.lspCbs:
		fn()
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}
	if len(e.compilationErrors) != 0 {
		t.Fatalf("expected no errors, got %d", len(e.compilationErrors))
	}
}

func TestGotoCompilationError_OpensFile(t *testing.T) {
	e := newCompileTestEditor("")
	dir := t.TempDir()
	p := filepath.Join(dir, "x.txt")
	if err := os.WriteFile(p, []byte("line1\nline2\nline3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	e.compilationErrors = []compilationError{{File: p, Line: 2, Col: 1}}
	e.compilationErrorIdx = -1

	e.cmdNextError()
	if e.ActiveBuffer().Filename() != p {
		t.Fatalf("cmdNextError should open %q, got %q", p, e.ActiveBuffer().Filename())
	}
	line, _ := e.ActiveBuffer().LineCol(e.ActiveBuffer().Point())
	if line != 2 {
		t.Fatalf("point should be on line 2, got %d", line)
	}

	// Previous wraps around to the same single error.
	e.cmdPreviousError()
	if e.compilationErrorIdx != 0 {
		t.Fatalf("expected idx 0 after wrap, got %d", e.compilationErrorIdx)
	}
}

func TestNextError_NoErrors(t *testing.T) {
	e := newCompileTestEditor("")
	e.compilationErrors = nil
	e.cmdNextError()     // should just message, not crash
	e.cmdPreviousError() // same
}

func TestCompilationDispatch(t *testing.T) {
	e := newCompileTestEditor("hello")
	comp := buffer.NewWithContent("*compilation*", "out\n")
	comp.SetMode("compilation")
	e.buffers = append(e.buffers, comp)
	e.activeWin.SetBuf(comp)

	// 'n' / 'p' with no errors are handled.
	if !e.compilationDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'n'}) {
		t.Fatal("'n' should be handled")
	}
	if !e.compilationDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'p'}) {
		t.Fatal("'p' should be handled")
	}
	// Unknown rune not handled.
	if e.compilationDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'z'}) {
		t.Fatal("'z' should not be handled")
	}
	// Non-rune key not handled.
	if e.compilationDispatch(terminal.KeyEvent{Key: tcell.KeyEnter}) {
		t.Fatal("Enter should not be handled")
	}
	// 'q' quits the compilation view.
	if !e.compilationDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'q'}) {
		t.Fatal("'q' should be handled")
	}
}

func TestShowCompilationWindow_SplitsSingleWindow(t *testing.T) {
	e := newCompileTestEditor("hello")
	comp := buffer.NewWithContent("*compilation*", "out")
	if len(e.windows) != 1 {
		t.Fatalf("expected 1 window to start, got %d", len(e.windows))
	}
	e.showCompilationWindow(comp)
	if len(e.windows) != 2 {
		t.Fatalf("expected a split (2 windows), got %d", len(e.windows))
	}
	// Calling again is a no-op since the buffer is already shown.
	e.showCompilationWindow(comp)
	if len(e.windows) != 2 {
		t.Fatalf("second call should not add a window, got %d", len(e.windows))
	}
}
