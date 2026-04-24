package editor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/keymap"
	"github.com/skybert/gomacs/internal/syntax"
	"github.com/skybert/gomacs/internal/window"
)

// newDAPTestEditor is like newTestEditor but also initialises the DAP fields.
func newDAPTestEditor(content string) *Editor {
	buf := buffer.NewWithContent("*test*", content)
	win := window.New(buf, 0, 0, 80, 24)

	e := &Editor{
		term:               nil,
		buffers:            []*buffer.Buffer{buf},
		windows:            []*window.Window{win},
		activeWin:          win,
		minibufBuf:         buffer.New(" *minibuf*"),
		globalKeymap:       keymap.New("global"),
		ctrlXKeymap:        keymap.New("C-x"),
		universalArg:       1,
		dapBreakpoints:     make(map[string]map[int]struct{}),
		dapCbs:             make(chan func(), 16),
		customHighlighters: make(map[*buffer.Buffer]syntax.Highlighter),
	}
	e.minibufWin = window.New(e.minibufBuf, 23, 0, 80, 1)
	return e
}

// ---------------------------------------------------------------------------
// dap-toggle-breakpoint
// ---------------------------------------------------------------------------

func TestDapToggleBreakpoint_Set(t *testing.T) {
	e := newDAPTestEditor("hello\nworld\n")
	// Give the buffer a real file so the breakpoint has an absolute path.
	dir := t.TempDir()
	fname := filepath.Join(dir, "main.go")
	if err := os.WriteFile(fname, []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.ActiveBuffer().SetFilename(fname)
	e.ActiveBuffer().SetMode("go")
	// Move to line 2.
	e.ActiveBuffer().SetPoint(6) // "world\n" starts at offset 6

	e.cmdDebugToggleBreakpoint()

	abs, _ := filepath.Abs(fname)
	if _, ok := e.dapBreakpoints[abs]; !ok {
		t.Fatalf("expected breakpoint map for %q to exist", abs)
	}
	line, _ := e.ActiveBuffer().LineCol(e.ActiveBuffer().Point())
	if _, ok := e.dapBreakpoints[abs][line]; !ok {
		t.Errorf("breakpoint not set at line %d", line)
	}
}

func TestDapToggleBreakpoint_Remove(t *testing.T) {
	e := newDAPTestEditor("hello\nworld\n")
	dir := t.TempDir()
	fname := filepath.Join(dir, "main.go")
	if err := os.WriteFile(fname, []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.ActiveBuffer().SetFilename(fname)
	e.ActiveBuffer().SetMode("go")
	e.ActiveBuffer().SetPoint(6) // line 2

	// Toggle on.
	e.cmdDebugToggleBreakpoint()
	abs, _ := filepath.Abs(fname)
	line, _ := e.ActiveBuffer().LineCol(e.ActiveBuffer().Point())

	// Toggle off.
	e.cmdDebugToggleBreakpoint()
	if _, ok := e.dapBreakpoints[abs][line]; ok {
		t.Errorf("breakpoint should have been removed at line %d", line)
	}
}

func TestDapHasBreakpoint(t *testing.T) {
	e := newDAPTestEditor("")
	abs := "/tmp/foo.go"
	e.dapBreakpoints[abs] = map[int]struct{}{5: {}}
	if !e.dapHasBreakpoint(abs, 5) {
		t.Error("dapHasBreakpoint(abs, 5) should be true")
	}
	if e.dapHasBreakpoint(abs, 99) {
		t.Error("dapHasBreakpoint(abs, 99) should be false")
	}
}

// ---------------------------------------------------------------------------
// dapLaunchArgs
// ---------------------------------------------------------------------------

func TestDapLaunchArgs_TestFile(t *testing.T) {
	dir := t.TempDir()
	fname := filepath.Join(dir, "foo_test.go")
	src := "package main\nimport \"testing\"\n\nfunc TestFoo(t *testing.T) {}\n"
	if err := os.WriteFile(fname, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	buf := buffer.NewWithContent(fname, src)
	buf.SetFilename(fname)
	buf.SetMode("go")

	e := newDAPTestEditor("")
	args, err := e.dapLaunchArgs(buf)
	if err != nil {
		t.Fatalf("dapLaunchArgs: %v", err)
	}
	if args["mode"] != "test" {
		t.Errorf("mode = %q, want \"test\"", args["mode"])
	}
	testArgs, ok := args["args"].([]string)
	if !ok || len(testArgs) < 2 {
		t.Fatalf("args field malformed: %v", args["args"])
	}
	if testArgs[1] != "TestFoo" {
		t.Errorf("test name = %q, want \"TestFoo\"", testArgs[1])
	}
	// program must be the package directory, not the module root.
	prog, ok2 := args["program"].(string)
	if !ok2 {
		t.Fatalf("program field missing or wrong type: %v", args["program"])
	}
	if prog != dir {
		t.Errorf("program = %q, want package dir %q", prog, dir)
	}
}

func TestDapLaunchArgs_Main(t *testing.T) {
	dir := t.TempDir()
	fname := filepath.Join(dir, "main.go")
	src := "package main\n\nfunc main() {}\n"
	if err := os.WriteFile(fname, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	buf := buffer.NewWithContent(fname, src)
	buf.SetFilename(fname)
	buf.SetMode("go")

	e := newDAPTestEditor("")
	args, err := e.dapLaunchArgs(buf)
	if err != nil {
		t.Fatalf("dapLaunchArgs: %v", err)
	}
	if args["mode"] != "debug" {
		t.Errorf("mode = %q, want \"debug\"", args["mode"])
	}
	prog, ok := args["program"].(string)
	if !ok || !strings.HasPrefix(prog, dir) {
		t.Errorf("program = %q, should start with temp dir %q", prog, dir)
	}
}

func TestDapLaunchArgs_Server(t *testing.T) {
	dir := t.TempDir()
	fname := filepath.Join(dir, "server.go")
	src := "package main\n\nfunc serve() {}\n"
	if err := os.WriteFile(fname, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	buf := buffer.NewWithContent(fname, src)
	buf.SetFilename(fname)
	buf.SetMode("go")

	e := newDAPTestEditor("")
	args, err := e.dapLaunchArgs(buf)
	if err != nil {
		t.Fatalf("dapLaunchArgs: %v", err)
	}
	if args["mode"] != "debug" {
		t.Errorf("mode = %q, want \"debug\"", args["mode"])
	}
}

func TestDapLaunchArgs_NoFile(t *testing.T) {
	e := newDAPTestEditor("x")
	buf := buffer.New("*scratch*")
	buf.SetMode("go")
	// No filename set.
	_, err := e.dapLaunchArgs(buf)
	if err == nil {
		t.Error("expected error when buffer has no filename")
	}
}

// ---------------------------------------------------------------------------
// dapRelayoutWindows (avoids e.term.Size())
// ---------------------------------------------------------------------------

func TestDapRelayoutWindows_4Windows(t *testing.T) {
	e := newDAPTestEditor("hello")
	e.dap = &dapState{localsAutoExpandDepth: 1}

	// Manually create 4 placeholder windows.
	buf0 := e.buffers[0]
	buf1 := buffer.New("*Debug Locals*")
	buf2 := buffer.New("*Debug Stack*")
	buf3 := buffer.New("*Debug REPL*")
	e.buffers = append(e.buffers, buf1, buf2, buf3)
	e.windows = []*window.Window{
		window.New(buf0, 0, 0, 1, 1),
		window.New(buf1, 0, 0, 1, 1),
		window.New(buf2, 0, 0, 1, 1),
		window.New(buf3, 0, 0, 1, 1),
	}

	const totalW, totalH = 120, 40
	e.dapRelayoutWindows(totalW, totalH)

	if got := len(e.windows); got != 4 {
		t.Fatalf("want 4 windows, got %d", got)
	}

	// Source window should span left portion.
	src := e.windows[0]
	rightW := max(totalW/3, 10)
	wantSrcW := totalW - rightW - 1
	if src.Width() != wantSrcW {
		t.Errorf("source width = %d, want %d", src.Width(), wantSrcW)
	}

	// Locals + stack windows should be to the right.
	locals := e.windows[1]
	if locals.Left() != sourceLeft(src) {
		t.Errorf("locals left = %d, want %d (source right+1)", locals.Left(), sourceLeft(src))
	}

	// REPL window should span full width.
	repl := e.windows[3]
	if repl.Width() != totalW {
		t.Errorf("repl width = %d, want %d", repl.Width(), totalW)
	}
}

// sourceLeft returns one past the right edge of the source window (where panels start).
func sourceLeft(src *window.Window) int { return src.Left() + src.Width() + 1 }

// ---------------------------------------------------------------------------
// dapTestFuncAtPoint
// ---------------------------------------------------------------------------

func TestDapTestFuncAtPoint_Found(t *testing.T) {
	src := "package main\n\nimport \"testing\"\n\nfunc TestAlpha(t *testing.T) {\n}\n\nfunc TestBeta(t *testing.T) {\n}\n"
	buf := buffer.NewWithContent("foo_test.go", src)
	// Point at end of TestBeta body.
	buf.SetPoint(buf.Len())

	got := dapTestFuncAtPoint(buf)
	if got != "TestBeta" {
		t.Errorf("got %q, want \"TestBeta\"", got)
	}
}

func TestDapTestFuncAtPoint_Fallback(t *testing.T) {
	buf := buffer.NewWithContent("foo_test.go", "package main\n")
	got := dapTestFuncAtPoint(buf)
	if got != "." {
		t.Errorf("got %q, want \".\"", got)
	}
}

// ---------------------------------------------------------------------------
// dapWordAtPoint
// ---------------------------------------------------------------------------

func TestDapWordAtPoint(t *testing.T) {
	buf := buffer.NewWithContent("test.go", "x := someVar + y\n")
	// Point somewhere inside "someVar".
	buf.SetPoint(7) // 's' of someVar
	got := dapWordAtPoint(buf)
	if got != "someVar" {
		t.Errorf("got %q, want \"someVar\"", got)
	}
}

func TestDapWordAtPoint_Empty(t *testing.T) {
	buf := buffer.NewWithContent("test.go", "  ")
	buf.SetPoint(0)
	got := dapWordAtPoint(buf)
	if got != "" {
		t.Errorf("got %q, want \"\"", got)
	}
}
