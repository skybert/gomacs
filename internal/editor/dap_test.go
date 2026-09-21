package editor

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/dap"
	"github.com/skybert/gomacs/internal/elisp"
	"github.com/skybert/gomacs/internal/keymap"
	"github.com/skybert/gomacs/internal/syntax"
	"github.com/skybert/gomacs/internal/terminal"
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

	abs := canonPath(fname)
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
	abs := canonPath(fname)
	line, _ := e.ActiveBuffer().LineCol(e.ActiveBuffer().Point())

	// Toggle off.
	e.cmdDebugToggleBreakpoint()
	if _, ok := e.dapBreakpoints[abs][line]; ok {
		t.Errorf("breakpoint should have been removed at line %d", line)
	}
}

// ---------------------------------------------------------------------------
// dapLaunchArgs
// ---------------------------------------------------------------------------

func TestDapLaunchArgs_TestFile(t *testing.T) {
	dir := t.TempDir()
	fname := filepath.Join(dir, "foo_test.go")
	src := "package main\nimport \"testing\"\n\nfunc TestFoo(t *testing.T) {\n\tx := 1\n\t_ = x\n}\n"
	if err := os.WriteFile(fname, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	buf := buffer.NewWithContent(fname, src)
	buf.SetFilename(fname)
	buf.SetMode("go")
	// Point inside TestFoo's body: that is the test debug-start must select.
	buf.SetPoint(strings.Index(src, "x := 1"))

	e := newDAPTestEditor("")
	args, _, err := e.dapLaunchArgs(buf)
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
	if prog != canonPath(dir) {
		t.Errorf("program = %q, want package dir %q", prog, canonPath(dir))
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
	args, _, err := e.dapLaunchArgs(buf)
	if err != nil {
		t.Fatalf("dapLaunchArgs: %v", err)
	}
	if args["mode"] != "debug" {
		t.Errorf("mode = %q, want \"debug\"", args["mode"])
	}
	prog, ok := args["program"].(string)
	if !ok || !strings.HasPrefix(prog, canonPath(dir)) {
		t.Errorf("program = %q, should start with temp dir %q", prog, canonPath(dir))
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
	args, _, err := e.dapLaunchArgs(buf)
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
	_, _, err := e.dapLaunchArgs(buf)
	if err == nil {
		t.Error("expected error when buffer has no filename")
	}
}

// ---------------------------------------------------------------------------
// window geometry helper shared with dap_layout_test.go
// ---------------------------------------------------------------------------

// sourceLeft returns one past the right edge of the source window (where panels start).
func sourceLeft(src *window.Window) int { return src.Left() + src.Width() + 1 }

// ---------------------------------------------------------------------------
// dapTestFuncAtPoint
// ---------------------------------------------------------------------------

func TestDapTestFuncAtPoint_Found(t *testing.T) {
	src := "package main\n\nimport \"testing\"\n\nfunc TestAlpha(t *testing.T) {\n}\n\nfunc TestBeta(t *testing.T) {\n\tinBeta := 1\n}\n"
	buf := buffer.NewWithContent("foo_test.go", src)
	// Point inside TestBeta's body.
	buf.SetPoint(strings.Index(src, "inBeta := 1"))

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

// rangeAwareTestSrc is one file exercising every way point can sit inside or
// outside a test function, including braces the range scan must not count
// because they are quoted or commented out.
var rangeAwareTestSrc = "package main\n" +
	"\n" +
	"import \"testing\"\n" +
	"\n" +
	"func TestAlpha(t *testing.T) {\n" +
	"\tif s := \"}\"; s != \"}\" { // an unbalanced brace in a string\n" +
	"\t\tt.Fatal(\"nope\")\n" +
	"\t}\n" +
	"\tbrace := '}' // and one in a rune literal\n" +
	"\traw := `}}}`\n" +
	"\t/* }}} in a block comment\n" +
	"\t   spanning lines } */\n" +
	"\tinAlpha := 1\n" +
	"\t_, _, _ = brace, raw, inAlpha\n" +
	"}\n" +
	"\n" +
	"var betweenTests = 1\n" +
	"\n" +
	"func TestBeta(t *testing.T) {\n" +
	"\tinBeta := 1\n" +
	"\t_ = inBeta\n" +
	"}\n" +
	"\n" +
	"func helperAfterTests(t *testing.T) {\n" +
	"\tinHelper := 1\n" +
	"\t_ = inHelper\n" +
	"}\n"

// TestDapTestFuncAtPoint_OnlyInsideFunctionBody covers the spec's rule that the
// test at point is only used when point really is inside it: "If the cursor is on
// a class, or outside any function, the entire file is debugged."  Scanning
// backwards for the nearest preceding declaration used to attribute a top-level
// var, or a helper declared after a test, to that test.
func TestDapTestFuncAtPoint_OnlyInsideFunctionBody(t *testing.T) {
	tests := []struct {
		name string
		at   string // point goes at the first occurrence of this text ("" = end of buffer)
		want string
	}{
		{"before any test", "import \"testing\"", "."},
		{"on the declaration line", "func TestAlpha", "TestAlpha"},
		{"brace inside a string", "s := \"}\"", "TestAlpha"},
		{"brace inside a rune literal", "brace := '}'", "TestAlpha"},
		{"brace inside a raw string", "raw := `", "TestAlpha"},
		{"brace inside a block comment", "/* }}} ", "TestAlpha"},
		{"after the quoted braces", "inAlpha := 1", "TestAlpha"},
		{"on the closing brace", "}\n\nvar betweenTests", "TestAlpha"},
		{"between two tests", "var betweenTests", "."},
		{"inside the second test", "inBeta := 1", "TestBeta"},
		{"inside a helper declared after a test", "inHelper := 1", "."},
		{"end of buffer", "", "."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := buffer.NewWithContent("foo_test.go", rangeAwareTestSrc)
			pt := buf.Len()
			if tt.at != "" {
				idx := strings.Index(rangeAwareTestSrc, tt.at)
				if idx < 0 {
					t.Fatalf("marker %q is not in the fixture", tt.at)
				}
				pt = utf8.RuneCountInString(rangeAwareTestSrc[:idx])
			}
			buf.SetPoint(pt)
			if got := dapTestFuncAtPoint(buf); got != tt.want {
				t.Errorf("point at %q: got %q, want %q", tt.at, got, tt.want)
			}
		})
	}
}

// TestDapTestFuncAtPoint_MultibyteBeforePoint guards the byte/rune conversion:
// regexp reports byte offsets while buffer positions are rune indices, so a
// multi-byte rune before the declaration used to shift the range.
func TestDapTestFuncAtPoint_MultibyteBeforePoint(t *testing.T) {
	src := "package main\n\n// ★ højere ★ unicode\n\nfunc TestUnicode(t *testing.T) {\n\ts := \"π\"\n\t_ = s\n}\n\nvar after = 1\n"
	buf := buffer.NewWithContent("foo_test.go", src)

	buf.SetPoint(utf8.RuneCountInString(src[:strings.Index(src, "_ = s")]))
	if got := dapTestFuncAtPoint(buf); got != "TestUnicode" {
		t.Errorf("inside the body: got %q, want \"TestUnicode\"", got)
	}
	buf.SetPoint(utf8.RuneCountInString(src[:strings.Index(src, "var after")]))
	if got := dapTestFuncAtPoint(buf); got != "." {
		t.Errorf("after the body: got %q, want \".\"", got)
	}
}

// TestDapTestFuncAtPoint_UnbalancedBraces covers the half-written test: the body
// has no closing brace yet, and debugging it must still select it rather than
// silently running the whole file.
func TestDapTestFuncAtPoint_UnbalancedBraces(t *testing.T) {
	src := "package main\n\nfunc TestHalfWritten(t *testing.T) {\n\tif true {\n\t\tx := 1\n"
	buf := buffer.NewWithContent("foo_test.go", src)
	buf.SetPoint(buf.Len())
	if got := dapTestFuncAtPoint(buf); got != "TestHalfWritten" {
		t.Errorf("got %q, want \"TestHalfWritten\"", got)
	}
}

// TestGoBodyEnd covers the brace scanner on its own, including the literals and
// comments whose braces it has to ignore.
func TestGoBodyEnd(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string // text the closing brace is expected to start
		ok   bool
	}{
		{"simple body", "func f() {\n}\nafter", "}\nafter", true},
		{"nested braces", "func f() {\n\tif x {\n\t}\n}\nafter", "}\nafter", true},
		{"brace in string", "func f() {\n\ts := \"}\"\n}\nafter", "}\nafter", true},
		{"brace in rune literal", "func f() {\n\tr := '}'\n}\nafter", "}\nafter", true},
		{"escaped quote", "func f() {\n\ts := \"\\\"}\"\n}\nafter", "}\nafter", true},
		{"brace in raw string", "func f() {\n\ts := `}` + `{`\n}\nafter", "}\nafter", true},
		{"brace in line comment", "func f() {\n\t// }\n}\nafter", "}\nafter", true},
		{"brace in block comment", "func f() {\n\t/* } */\n}\nafter", "}\nafter", true},
		{"division is not a comment", "func f() {\n\tx := a / b\n}\nafter", "}\nafter", true},
		{"unterminated string", "func f() {\n\ts := \"oops\n}\nafter", "}\nafter", true},
		{"never closed", "func f() {\n\tif x {\n", "", false},
		{"no body", "func f()", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runes := []rune(tt.src)
			end, ok := goBodyEnd(runes, 0)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v (end %d)", ok, tt.ok, end)
			}
			if !ok {
				return
			}
			if got := string(runes[end:]); got != tt.want {
				t.Errorf("closing brace at %q, want it at %q", got, tt.want)
			}
		})
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

// ---------------------------------------------------------------------------
// debug step/eval/exit commands with a mock backend
// ---------------------------------------------------------------------------

type mockDebugBackend struct {
	called     chan string
	err        error
	evalResult string
	closed     chan struct{}
	bpFile     string
	bpLines    []int
}

func newMockDebugBackend() *mockDebugBackend {
	return &mockDebugBackend{called: make(chan string, 16), closed: make(chan struct{}, 1)}
}

func (m *mockDebugBackend) Continue(int) error { m.called <- "continue"; return m.err }
func (m *mockDebugBackend) StepNext(int) error { m.called <- "next"; return m.err }
func (m *mockDebugBackend) StepIn(int) error   { m.called <- "in"; return m.err }
func (m *mockDebugBackend) StepOut(int) error  { m.called <- "out"; return m.err }
func (m *mockDebugBackend) Evaluate(expr string, frameID, stoppedThread int, ctx string) (string, error) {
	m.called <- "eval:" + expr
	return m.evalResult, m.err
}
func (m *mockDebugBackend) SetBreakpoints(file string, lines []int) error {
	m.bpFile = file
	m.bpLines = lines
	m.called <- "setbp"
	return m.err
}
func (m *mockDebugBackend) Close() { m.closed <- struct{}{} }

func newDAPMockEditor(content string) (*Editor, *mockDebugBackend) {
	e := newDAPTestEditor(content)
	e.term = &terminal.Terminal{} // non-nil; screen nil so PostWakeup is a no-op
	m := newMockDebugBackend()
	e.dap = &dapState{backend: m, client: nil, stoppedThread: 1}
	return e, m
}

func waitCall(t *testing.T, m *mockDebugBackend) string {
	t.Helper()
	select {
	case s := <-m.called:
		return s
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for backend call")
		return ""
	}
}

func TestCmdDebugContinue_NoSession(t *testing.T) {
	e := newDAPTestEditor("")
	e.dap = nil
	e.cmdDebugContinue() // must be a no-op, not panic
}

func TestCmdDebugContinue_CallsBackend(t *testing.T) {
	e, m := newDAPMockEditor("")
	e.cmdDebugContinue()
	if got := waitCall(t, m); got != "continue" {
		t.Fatalf("expected continue call, got %q", got)
	}
}

func TestCmdDebugStepNext_CallsBackend(t *testing.T) {
	e, m := newDAPMockEditor("")
	e.cmdDebugStepNext()
	if got := waitCall(t, m); got != "next" {
		t.Fatalf("expected next call, got %q", got)
	}
}

func TestCmdDebugStepIn_CallsBackend(t *testing.T) {
	e, m := newDAPMockEditor("")
	e.cmdDebugStepIn()
	if got := waitCall(t, m); got != "in" {
		t.Fatalf("expected in call, got %q", got)
	}
}

func TestCmdDebugStepOut_CallsBackend(t *testing.T) {
	e, m := newDAPMockEditor("")
	e.cmdDebugStepOut()
	if got := waitCall(t, m); got != "out" {
		t.Fatalf("expected out call, got %q", got)
	}
}

func TestCmdDebugContinue_ErrorPosted(t *testing.T) {
	e, m := newDAPMockEditor("")
	m.err = errors.New("boom")
	e.cmdDebugContinue()
	waitCall(t, m)
	// The error callback is posted to dapCbs; drain and run it.
	select {
	case fn := <-e.dapCbs:
		fn()
	case <-time.After(5 * time.Second):
		t.Fatal("expected an error callback on dapCbs")
	}
	if !strings.Contains(e.message, "boom") {
		t.Fatalf("expected error message, got %q", e.message)
	}
}

func TestCmdDebugEval_NoSession(t *testing.T) {
	e := newDAPTestEditor("foo")
	e.dap = nil
	e.cmdDebugEval()
	if !strings.Contains(e.message, "No active debug session") {
		t.Fatalf("expected no-session message, got %q", e.message)
	}
}

func TestCmdDebugEval_NoExpression(t *testing.T) {
	e, _ := newDAPMockEditor("   ")
	buf(e).SetPoint(0)
	e.cmdDebugEval()
	if !strings.Contains(e.message, "no expression") {
		t.Fatalf("expected 'no expression' message, got %q", e.message)
	}
}

func TestCmdDebugEval_EvaluatesWord(t *testing.T) {
	e, m := newDAPMockEditor("myVar")
	m.evalResult = "42"
	buf(e).SetPoint(2)
	e.cmdDebugEval()
	if got := waitCall(t, m); got != "eval:myVar" {
		t.Fatalf("expected eval of myVar, got %q", got)
	}
	select {
	case fn := <-e.dapCbs:
		fn()
	case <-time.After(5 * time.Second):
		t.Fatal("expected eval result callback")
	}
	if !strings.Contains(e.message, "42") {
		t.Fatalf("expected result message with 42, got %q", e.message)
	}
}

func TestCmdDebugEval_Region(t *testing.T) {
	e, m := newDAPMockEditor("alpha beta")
	m.evalResult = "ok"
	b := buf(e)
	b.SetMark(0)
	b.SetMarkActive(true)
	b.SetPoint(5) // selects "alpha"
	e.cmdDebugEval()
	if got := waitCall(t, m); got != "eval:alpha" {
		t.Fatalf("region eval should evaluate the selection, got %q", got)
	}
}

func TestCmdDebugEval_ErrorPosted(t *testing.T) {
	e, m := newDAPMockEditor("myVar")
	m.err = errors.New("boom")
	buf(e).SetPoint(2)
	e.cmdDebugEval()
	waitCall(t, m)
	select {
	case fn := <-e.dapCbs:
		fn()
	case <-time.After(5 * time.Second):
		t.Fatal("expected an error callback on dapCbs")
	}
	if !strings.Contains(e.message, "boom") {
		t.Fatalf("expected eval error message, got %q", e.message)
	}
}

func TestCmdDebugStepNext_ErrorPosted(t *testing.T) {
	assertDapStepError(t, "next", (*Editor).cmdDebugStepNext, "step-next")
}

func TestCmdDebugStepIn_ErrorPosted(t *testing.T) {
	assertDapStepError(t, "in", (*Editor).cmdDebugStepIn, "step-in")
}

func TestCmdDebugStepOut_ErrorPosted(t *testing.T) {
	assertDapStepError(t, "out", (*Editor).cmdDebugStepOut, "step-out")
}

// assertDapStepError drives a step command whose backend returns an error and
// verifies the error callback posts a message mentioning wantMsg.
func assertDapStepError(t *testing.T, wantCall string, cmd func(*Editor), wantMsg string) {
	t.Helper()
	e, m := newDAPMockEditor("")
	m.err = errors.New("kaboom")
	cmd(e)
	if got := waitCall(t, m); got != wantCall {
		t.Fatalf("expected %q call, got %q", wantCall, got)
	}
	select {
	case fn := <-e.dapCbs:
		fn()
	case <-time.After(5 * time.Second):
		t.Fatal("expected an error callback on dapCbs")
	}
	if !strings.Contains(e.message, wantMsg) {
		t.Fatalf("expected message mentioning %q, got %q", wantMsg, e.message)
	}
}

func TestCmdDebugExit(t *testing.T) {
	e, m := newDAPMockEditor("")
	e.cmdDebugExit()
	if e.dap != nil {
		t.Fatal("cmdDebugExit should clear the debug session")
	}
	select {
	case <-m.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("cmdDebugExit should close the backend")
	}
	if !strings.Contains(e.message, "Debug session ended") {
		t.Fatalf("expected 'Debug session ended', got %q", e.message)
	}
}

func TestCmdDebugExit_NoSession(t *testing.T) {
	e := newDAPTestEditor("")
	e.dap = nil
	e.cmdDebugExit() // no-op, no panic
}

// ---------------------------------------------------------------------------
// dapHandleEvent / scrollWindowToLine / debugSourceDispatch
// ---------------------------------------------------------------------------

func newDAPCapEditor(content string) (*Editor, *mockDebugBackend) {
	e := newCapTestEditor(content)
	e.dapBreakpoints = make(map[string]map[int]struct{})
	e.dapCbs = make(chan func(), 16)
	m := newMockDebugBackend()
	e.dap = &dapState{backend: m, stoppedThread: 1}
	return e, m
}

func TestDapHandleEvent_NoSession(t *testing.T) {
	e := newDAPTestEditor("")
	e.dap = nil
	e.dapHandleEvent("stopped", []byte(`{}`)) // no-op
}

func TestDapHandleEvent_Continued(t *testing.T) {
	e, _ := newDAPCapEditor("")
	e.dap.stoppedFile = "x.go"
	e.dap.stoppedLine = 5
	e.dapHandleEvent("continued", []byte(`{}`))
	if e.dap.stoppedFile != "" || e.dap.stoppedLine != 0 {
		t.Fatal("continued event should clear stopped position")
	}
}

func TestDapHandleEvent_Output(t *testing.T) {
	e, _ := newDAPCapEditor("")
	e.dap.replBuf = buffer.New("*Debug REPL*")
	dapReplReset(e.dap.replBuf)
	e.dapHandleEvent("output", []byte(`{"output":"hello from program\n"}`))
	if !strings.Contains(e.dap.replBuf.String(), "hello from program") {
		t.Fatalf("output event should append to REPL, got %q", e.dap.replBuf.String())
	}
}

func TestDapHandleEvent_Terminated(t *testing.T) {
	e, _ := newDAPCapEditor("")
	e.dapHandleEvent("terminated", []byte(`{}`))
	if e.dap != nil {
		t.Fatal("terminated event should end the debug session")
	}
}

func TestDapHandleEvent_StoppedNilClient(t *testing.T) {
	e, _ := newDAPCapEditor("")
	// client is nil so dapFetchStoppedInfo is a no-op, but stoppedThread is set.
	e.dapHandleEvent("stopped", []byte(`{"threadId":7,"reason":"breakpoint"}`))
	if e.dap.stoppedThread != 7 {
		t.Fatalf("stopped event should record thread id, got %d", e.dap.stoppedThread)
	}
}

func TestScrollWindowToLine(t *testing.T) {
	content := strings.Repeat("line\n", 100)
	e, _ := newDAPCapEditor(content)
	w := e.activeWin
	e.scrollWindowToLine(w, 50)
	want := 50 - w.Height()/2
	if w.ScrollLine() != want {
		t.Fatalf("scrollWindowToLine: got %d, want %d", w.ScrollLine(), want)
	}
}

func TestDebugSourceDispatch_Keys(t *testing.T) {
	e, m := newDAPCapEditor("foo")
	// 'c' continue
	if !e.debugSourceDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'c'}) {
		t.Fatal("'c' should be handled")
	}
	if got := waitCall(t, m); got != "continue" {
		t.Fatalf("'c' should continue, got %q", got)
	}
	// 'n' step next (stopped)
	if !e.debugSourceDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'n'}) {
		t.Fatal("'n' should be handled")
	}
	if got := waitCall(t, m); got != "next" {
		t.Fatalf("'n' should step next, got %q", got)
	}
	// unknown rune
	if e.debugSourceDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'Z'}) {
		t.Fatal("'Z' should not be handled")
	}
	// modified key not handled
	if e.debugSourceDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'n', Mod: tcell.ModAlt}) {
		t.Fatal("M-n should not be handled by debugSourceDispatch")
	}
}

// TestDebugSourceDispatch_AdoptsFileOpenedMidSession covers the case the spec
// calls out — "the source buffers should be switched to read only, so that the
// user can use single letter shortcuts to navigate" — for a file the user opens
// while the session is running rather than one stepping opened.  Such a buffer
// used to stay writable, so anything the single-letter shortcuts do not claim was
// typed straight into it.  The other half of the contract matters just as much:
// debug-exit must hand the buffer back writable.
func TestDebugSourceDispatch_AdoptsFileOpenedMidSession(t *testing.T) {
	e, m := newDAPCapEditor("package main\n")
	e.autoRevertMtimes = make(map[*buffer.Buffer]time.Time)
	e.lspConns = make(map[string]*lspConn)

	path := filepath.Join(t.TempDir(), "opened.go")
	if err := os.WriteFile(path, []byte("package p\n\nfunc f() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// The user runs find-file while stopped at a breakpoint.  loadFile adopts
	// the buffer straight away, so the modeline shows it read-only immediately
	// rather than only after the first keystroke.
	opened, err := e.loadFile(path)
	if err != nil {
		t.Fatalf("loadFile: %v", err)
	}
	if !opened.ReadOnly() {
		t.Fatal("a file opened during a debug session should be read-only at once")
	}
	e.activeWin.SetBuf(opened)

	// 'c' must drive the debugger, and the buffer must be read-only from here on.
	if !e.debugSourceDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'c'}) {
		t.Fatal("'c' should be handled as a debug shortcut")
	}
	if got := waitCall(t, m); got != "continue" {
		t.Fatalf("'c' should continue, got %q", got)
	}
	if !opened.ReadOnly() {
		t.Error("a file opened mid-session should be read-only while debugging")
	}

	// Even a rune the debugger does not claim must not reach self-insert.
	if e.debugSourceDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'Z'}) {
		t.Fatal("'Z' is not a debug shortcut and should not be consumed here")
	}
	if !opened.ReadOnly() {
		t.Error("the buffer should still be read-only after an unhandled key")
	}

	e.cmdDebugExit()
	if opened.ReadOnly() {
		t.Error("debug-exit must restore the buffer the user opened to writable")
	}
}

func TestDebugSourceDispatch_NotStopped(t *testing.T) {
	e, _ := newDAPCapEditor("foo")
	e.dap.stoppedThread = 0
	if !e.debugSourceDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'n'}) {
		t.Fatal("'n' should still be handled (with a 'not stopped' message)")
	}
	if !strings.Contains(e.message, "not stopped") {
		t.Fatalf("expected 'not stopped' message, got %q", e.message)
	}
}

func TestDebugSourceDispatch_StepInAndOut(t *testing.T) {
	e, m := newDAPCapEditor("foo")
	if !e.debugSourceDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'i'}) {
		t.Fatal("'i' should be handled")
	}
	if got := waitCall(t, m); got != "in" {
		t.Fatalf("'i' should step in, got %q", got)
	}
	if !e.debugSourceDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 's'}) {
		t.Fatal("'s' should be handled")
	}
	if got := waitCall(t, m); got != "in" {
		t.Fatalf("'s' should also step in, got %q", got)
	}
	if !e.debugSourceDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'o'}) {
		t.Fatal("'o' should be handled")
	}
	if got := waitCall(t, m); got != "out" {
		t.Fatalf("'o' should step out, got %q", got)
	}
}

func TestDebugSourceDispatch_StepInOutNotStopped(t *testing.T) {
	for _, r := range []rune{'i', 's', 'o'} {
		e, _ := newDAPCapEditor("foo")
		e.dap.stoppedThread = 0
		if !e.debugSourceDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: r}) {
			t.Fatalf("%q should still be handled when not stopped", r)
		}
		if !strings.Contains(e.message, "not stopped") {
			t.Fatalf("%q: expected 'not stopped' message, got %q", r, e.message)
		}
	}
}

func TestDebugSourceDispatch_EvalAndQuit(t *testing.T) {
	e, m := newDAPCapEditor("myVar")
	e.ActiveBuffer().SetPoint(2)
	if !e.debugSourceDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'e'}) {
		t.Fatal("'e' should be handled")
	}
	if got := waitCall(t, m); got != "eval:myVar" {
		t.Fatalf("'e' should evaluate the word at point, got %q", got)
	}
	// 'q' exits the session.
	if !e.debugSourceDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'q'}) {
		t.Fatal("'q' should be handled")
	}
	if e.dap != nil {
		t.Fatal("'q' should end the debug session")
	}
}

// ---------------------------------------------------------------------------
// dapReplSubmit
// ---------------------------------------------------------------------------

func TestDapReplSubmit_NoSession(t *testing.T) {
	e, _ := newDAPCapEditor("")
	e.dap.replBuf = buffer.New("*Debug REPL*")
	dapReplReset(e.dap.replBuf)
	e.dap.replBuf.InsertString(e.dap.replBuf.Len(), "myexpr")
	e.dap.client = nil // no active session
	e.dapReplSubmit()
	if !strings.Contains(e.dap.replBuf.String(), "no active session") {
		t.Fatalf("dapReplSubmit without a client should note no active session, got %q", e.dap.replBuf.String())
	}
}

// ---------------------------------------------------------------------------
// delve (dlv) integration — drives a real debug session
// ---------------------------------------------------------------------------

// drainDapUntil pumps dapCbs callbacks (running each on the calling goroutine,
// as the event loop would) until cond() is true or the deadline passes.
func drainDapUntil(t *testing.T, e *Editor, cond func() bool, what string) {
	t.Helper()
	deadline := time.After(30 * time.Second)
	for !cond() {
		select {
		case fn := <-e.dapCbs:
			fn()
		case <-deadline:
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

func TestDelve_DebugSession(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping delve integration test in -short mode")
	}
	if _, err := exec.LookPath("dlv"); err != nil {
		t.Skip("dlv not available")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/d\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	src := "package main\n\nfunc main() {\n\tx := 41\n\tx++\n\tprintln(x)\n}\n"
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	e := newCapTestEditor(src)
	e.dapBreakpoints = make(map[string]map[int]struct{})
	e.dapCbs = make(chan func(), 64)
	b := buf(e)
	b.SetMode("go")
	b.SetFilename(path)
	// Set a breakpoint on line 5 (x++).
	e.dapBreakpoints[path] = map[int]struct{}{5: {}}

	e.cmdDebugStart()
	// Pump callbacks until the session is set up and stopped at the breakpoint.
	drainDapUntil(t, e, func() bool {
		return e.dap != nil && e.dap.client != nil && e.dap.stoppedThread != 0 && e.dap.stoppedLine != 0
	}, "debug session to stop at breakpoint")

	if e.dap.stoppedLine != 5 {
		t.Logf("stopped at line %d (expected 5)", e.dap.stoppedLine)
	}

	// Locals should have been fetched.
	e.dap.localsMu.RLock()
	nLocals := len(e.dap.locals)
	e.dap.localsMu.RUnlock()
	if nLocals == 0 {
		t.Log("no locals fetched (delve returned none)")
	}

	// The stack panel groups frames per thread (goroutine, for delve).
	e.dap.framesMu.RLock()
	nThreads := len(e.dap.threads)
	stopped := nThreads > 0 && e.dap.threads[0].stopped
	e.dap.framesMu.RUnlock()
	if nThreads == 0 || !stopped {
		t.Errorf("expected at least the stopped thread in the stack panel, got %d threads", nThreads)
	}
	if e.dap.stackBuf != nil && !strings.Contains(e.dap.stackBuf.String(), "Thread ") {
		t.Errorf("stack panel should be grouped per thread, got %q", e.dap.stackBuf.String())
	}

	// Exercise dapSyncBreakpoints against the live session.
	e.dapSyncBreakpoints(path)
	// Drain any resulting callback (error path posts one; success posts nil).
	select {
	case fn := <-e.dapCbs:
		fn()
	case <-time.After(3 * time.Second):
	}

	// Step and continue via the real backend, then exit.
	e.cmdDebugStepNext()
	select {
	case fn := <-e.dapCbs:
		fn()
	case <-time.After(3 * time.Second):
	}
	e.cmdDebugExit()
	if e.dap != nil {
		t.Fatal("cmdDebugExit should end the session")
	}
}

// ---------------------------------------------------------------------------
// canonPath
// ---------------------------------------------------------------------------

func TestCanonPath_NonexistentFallsBackToAbs(t *testing.T) {
	got := canonPath("/nonexistent-gomacs-xyz/sub/file.go")
	if got != "/nonexistent-gomacs-xyz/sub/file.go" {
		t.Errorf("canonPath of a non-existent absolute path should pass through, got %q", got)
	}
}

func TestCanonPath_RelativeMadeAbsolute(t *testing.T) {
	got := canonPath("relative/does/not/exist.go")
	if !filepath.IsAbs(got) {
		t.Errorf("canonPath should return an absolute path, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// cmdDebugStart error branches
// ---------------------------------------------------------------------------

func TestCmdDebugStart_NoAdapterForMode(t *testing.T) {
	e := newDAPTestEditor("plain text")
	e.ActiveBuffer().SetMode("text") // text mode has no dapCmd
	e.cmdDebugStart()
	if e.dap != nil {
		t.Error("debug-start should not start a session for a mode with no adapter")
	}
	if !strings.Contains(e.message, "No debug adapter") {
		t.Errorf("expected 'No debug adapter' message, got %q", e.message)
	}
}

func TestCmdDebugStart_AlreadyActive(t *testing.T) {
	e := newDAPTestEditor("")
	e.dap = &dapState{}
	e.cmdDebugStart()
	if !strings.Contains(e.message, "already active") {
		t.Errorf("expected 'already active' message, got %q", e.message)
	}
}

func TestCmdDebugStart_NoFile(t *testing.T) {
	e := newDAPTestEditor("package main\n\nfunc main() {}\n")
	e.ActiveBuffer().SetMode("go")
	// No filename set → dapLaunchArgs returns an error.
	e.cmdDebugStart()
	if e.dap != nil {
		t.Error("debug-start should fail when the buffer has no file")
	}
}

func TestCmdDebugStart_JavaWithoutJdtls(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Main.java")
	src := "public class Main {\n  public static void main(String[] args) {}\n}\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	e := newDAPTestEditor(src)
	e.term = &terminal.Terminal{}
	e.ActiveBuffer().SetMode("java")
	e.ActiveBuffer().SetFilename(path)

	// java-mode has a debug adapter configured, but it is reached through the
	// language server, so with no connection debug-start refuses up front rather
	// than opening a session it cannot use.
	e.cmdDebugStart()
	if e.dap != nil {
		t.Error("debug-start must not open a session without a language server")
	}
	if !strings.Contains(e.message, "jdtls") {
		t.Errorf("expected a message naming jdtls, got %q", e.message)
	}
	if !strings.Contains(e.message, "java-debug") {
		t.Errorf("expected the message to say what to install, got %q", e.message)
	}
	if !strings.Contains(e.message, "java-lsp-command") {
		t.Errorf("expected the message to name the variable that fixes it, got %q", e.message)
	}
}

// ---------------------------------------------------------------------------
// dapStartAdapter
// ---------------------------------------------------------------------------

func TestDapStartAdapter_ProcessFailure(t *testing.T) {
	info := &langModeInfo{modeName: "go", dapCmd: []string{"gomacs-no-such-adapter"}}
	_, _, _, err := dapStartAdapter(info, dapLaunchRequest{runDir: t.TempDir()})
	if err == nil {
		t.Fatal("expected an error for a missing adapter binary")
	}
	if !strings.Contains(err.Error(), "cannot start") {
		t.Errorf("error = %v, want it to mention it cannot start the adapter", err)
	}
}

func TestDapStartAdapter_JdtlsWithoutConnection(t *testing.T) {
	info := &langModeInfo{modeName: "java", dapKind: dapAdapterJdtls}
	_, _, _, err := dapStartAdapter(info, dapLaunchRequest{runDir: t.TempDir()})
	if err == nil {
		t.Fatal("expected an error when there is no jdtls connection")
	}
	if !strings.Contains(err.Error(), "jdtls") {
		t.Errorf("error = %v, want it to mention jdtls", err)
	}
}

// ---------------------------------------------------------------------------
// dapLocalsAutoExpandDepth
// ---------------------------------------------------------------------------

func TestDapLocalsAutoExpandDepth_DefaultsToOne(t *testing.T) {
	e := newDAPTestEditor("") // e.lisp is nil
	if got := e.dapLocalsAutoExpandDepth(); got != 1 {
		t.Errorf("depth = %d, want 1 without an Elisp evaluator", got)
	}
	e.lisp = elisp.NewEvaluator()
	if got := e.dapLocalsAutoExpandDepth(); got != 1 {
		t.Errorf("depth = %d, want 1 when the variable is unset", got)
	}
}

func TestDapLocalsAutoExpandDepth_FromElisp(t *testing.T) {
	e := newDAPTestEditor("")
	e.lisp = elisp.NewEvaluator()
	if _, err := e.lisp.EvalString("(setq debug-locals-auto-expand-depth 3)"); err != nil {
		t.Fatal(err)
	}
	if got := e.dapLocalsAutoExpandDepth(); got != 3 {
		t.Errorf("depth = %d, want 3", got)
	}
}

func TestDapLocalsAutoExpandDepth_IgnoresBadValues(t *testing.T) {
	e := newDAPTestEditor("")
	e.lisp = elisp.NewEvaluator()
	if _, err := e.lisp.EvalString(`(setq debug-locals-auto-expand-depth "deep")`); err != nil {
		t.Fatal(err)
	}
	if got := e.dapLocalsAutoExpandDepth(); got != 1 {
		t.Errorf("depth = %d, want the default 1 for a non-integer value", got)
	}
	if _, err := e.lisp.EvalString("(setq debug-locals-auto-expand-depth 0)"); err != nil {
		t.Fatal(err)
	}
	if got := e.dapLocalsAutoExpandDepth(); got != 1 {
		t.Errorf("depth = %d, want the default 1 for a non-positive value", got)
	}
}

func TestCmdDebugStart_SeedsConfiguredExpandDepth(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Main.java")
	if err := os.WriteFile(path, []byte("class Main {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := newDAPTestEditor("class Main {}\n")
	e.term = &terminal.Terminal{}
	e.lisp = elisp.NewEvaluator()
	if _, err := e.lisp.EvalString("(setq debug-locals-auto-expand-depth 4)"); err != nil {
		t.Fatal(err)
	}
	e.ActiveBuffer().SetMode("java")
	e.ActiveBuffer().SetFilename(path)
	// A ready (if useless) language server so the java precheck lets debug-start
	// get as far as creating the session state; resolving the main class then
	// fails on the worker goroutine, which is fine here.
	conn, cleanup := fakeJdtlsServer(t, nil)
	defer cleanup()
	e.lspConns = map[string]*lspConn{"java": conn}

	e.cmdDebugStart()
	if e.dap == nil {
		t.Fatal("session state should exist right after debug-start")
	}
	if e.dap.localsAutoExpandDepth != 4 {
		t.Errorf("localsAutoExpandDepth = %d, want the configured 4", e.dap.localsAutoExpandDepth)
	}
	// Drain the (failing) start callback so the goroutine does not outlive the test.
	select {
	case fn := <-e.dapCbs:
		fn()
	case <-time.After(5 * time.Second):
	}
}

// ---------------------------------------------------------------------------
// bufContainsMainFunc — Go and Java entry points
// ---------------------------------------------------------------------------

func TestBufContainsMainFunc_Go(t *testing.T) {
	yes := buffer.NewWithContent("main.go", "package main\n\nfunc main() {\n}\n")
	if !bufContainsMainFunc(yes) {
		t.Error("func main() should be detected")
	}
	no := buffer.NewWithContent("lib.go", "package lib\n\nfunc Helper() {}\n")
	if bufContainsMainFunc(no) {
		t.Error("a package without main should not be detected")
	}
}

func TestBufContainsMainFunc_JavaVariants(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"canonical", "public class A {\n  public static void main(String[] args) {}\n}\n"},
		{"c-style array", "public class A {\n  public static void main(String args[]) {}\n}\n"},
		{"varargs", "public class A {\n  public static void main(String... args) {}\n}\n"},
		{"modifier order", "class A {\n  static public void main(String[] args) {}\n}\n"},
		{"final parameter", "class A {\n  public static void main(final String[] args) {}\n}\n"},
		{"final method", "class A {\n  public static final void main(String[] args) {}\n}\n"},
		{"synchronized", "class A {\n  public static synchronized void main(String[] args) {}\n}\n"},
		{"tight whitespace", "class A {\n\tpublic static void main(String[]args){}\n}\n"},
		{"loose whitespace", "class A {\n  public  static  void  main ( String [ ] args ) {}\n}\n"},
		{"bracket before name", "class A {\n  public static void main(String []args) {}\n}\n"},
		{"qualified type", "class A {\n  public static void main(java.lang.String[] args) {}\n}\n"},
		{"no parameter name", "interface A {\n  static void main(String[]);\n}\n"},
		{"throws clause", "class A {\n  public static void main(String[] args) throws Exception {}\n}\n"},
		{"instance main", "class A {\n  void main() {}\n}\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf := buffer.NewWithContent("A.java", tc.src)
			buf.SetMode("java")
			if !bufContainsMainFunc(buf) {
				t.Errorf("main method not detected in:\n%s", tc.src)
			}
		})
	}
}

func TestBufContainsMainFunc_JavaNegatives(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"other method", "class A {\n  public static void mainLoop(String[] args) {}\n}\n"},
		{"non-void", "class A {\n  public static int main(String[] args) { return 0; }\n}\n"},
		{"wrong parameter type", "class A {\n  public static void main(int[] args) {}\n}\n"},
		{"field named main", "class A {\n  private String main;\n}\n"},
		{"call site only", "class A {\n  void run() {\n    B.main(args);\n  }\n}\n"},
		{"static far above", "class A {\n  static int n = 1;\n  int helper(String[] args) { return 0; }\n}\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf := buffer.NewWithContent("A.java", tc.src)
			buf.SetMode("java")
			if bufContainsMainFunc(buf) {
				t.Errorf("should not be detected as a main class:\n%s", tc.src)
			}
		})
	}
}

func TestDapLaunchArgs_JavaMainUsesJdtlsResolution(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pom.xml"), []byte("<project/>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "Main.java")
	src := "public class Main {\n  public static void main(String[] args) {}\n}\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	buf := buffer.NewWithContent("Main.java", src)
	buf.SetFilename(path)
	buf.SetMode("java")

	e := newDAPTestEditor("")
	args, root, err := e.dapLaunchArgs(buf)
	if err != nil {
		t.Fatalf("dapLaunchArgs: %v", err)
	}
	if args != nil {
		t.Errorf("java launch args = %v, want nil (resolved over LSP by jdtls)", args)
	}
	if root != canonPath(dir) {
		t.Errorf("root = %q, want the project root %q", root, canonPath(dir))
	}
}

// ---------------------------------------------------------------------------
// dapFetchFrames / dapFetchThreads
// ---------------------------------------------------------------------------

func TestDapFetchFrames(t *testing.T) {
	c, cleanup := dapFakeServer(t, map[string]any{
		"stackTrace": map[string]any{"stackFrames": []map[string]any{
			{"id": 3, "name": "main.main", "line": 7},
		}},
	})
	defer cleanup()
	frames, err := dapFetchFrames(c, 1)
	if err != nil {
		t.Fatalf("dapFetchFrames: %v", err)
	}
	if len(frames) != 1 || frames[0].ID != 3 || frames[0].Line != 7 {
		t.Errorf("frames = %+v, want one frame id 3 line 7", frames)
	}
}

func TestDapFetchFrames_Error(t *testing.T) {
	c, cleanup := dapFakeServer(t, nil)
	cleanup() // close the client so the request fails
	if _, err := dapFetchFrames(c, 1); err == nil {
		t.Error("expected an error from a closed client")
	}
}

func TestDapFetchThreads_AllThreads(t *testing.T) {
	c, cleanup := dapFakeServer(t, map[string]any{
		"threads": map[string]any{"threads": []map[string]any{
			{"id": 1, "name": "main"},
			{"id": 2, "name": "worker"},
		}},
		"stackTrace": map[string]any{"stackFrames": []map[string]any{
			{"id": 9, "name": "runtime.gopark", "line": 1},
		}},
	})
	defer cleanup()

	stopped := []dap.StackFrame{{ID: 1, Name: "main.main", Line: 5}}
	threads := dapFetchThreads(c, 1, stopped)

	if len(threads) != 2 {
		t.Fatalf("threads = %d, want 2", len(threads))
	}
	if !threads[0].stopped || threads[0].id != 1 {
		t.Errorf("threads[0] = %+v, want the stopped thread first", threads[0])
	}
	if threads[0].name != "main" {
		t.Errorf("stopped thread name = %q, want \"main\"", threads[0].name)
	}
	if len(threads[0].frames) != 1 || threads[0].frames[0].Name != "main.main" {
		t.Errorf("the stopped thread should reuse the frames already fetched, got %+v", threads[0].frames)
	}
	if threads[1].id != 2 || threads[1].name != "worker" || threads[1].stopped {
		t.Errorf("threads[1] = %+v, want the unstopped worker thread", threads[1])
	}
	if len(threads[1].frames) != 1 || threads[1].frames[0].Name != "runtime.gopark" {
		t.Errorf("worker frames = %+v, want the fetched frame", threads[1].frames)
	}
}

func TestDapFetchThreads_NoThreadsSupport(t *testing.T) {
	c, cleanup := dapFakeServer(t, nil)
	cleanup() // closed client → the threads request fails
	stopped := []dap.StackFrame{{ID: 1, Name: "main.main", Line: 5}}
	threads := dapFetchThreads(c, 7, stopped)
	if len(threads) != 1 {
		t.Fatalf("threads = %d, want just the stopped thread", len(threads))
	}
	if threads[0].id != 7 || !threads[0].stopped || len(threads[0].frames) != 1 {
		t.Errorf("threads[0] = %+v, want the stopped thread with its frames", threads[0])
	}
}

func TestDapFetchThreads_CapsThreadCount(t *testing.T) {
	all := make([]map[string]any, 0, 40)
	for i := range 40 {
		all = append(all, map[string]any{"id": i + 1, "name": fmt.Sprintf("t%d", i+1)})
	}
	c, cleanup := dapFakeServer(t, map[string]any{
		"threads":    map[string]any{"threads": all},
		"stackTrace": map[string]any{"stackFrames": []map[string]any{{"id": 1, "name": "f", "line": 1}}},
	})
	defer cleanup()

	threads := dapFetchThreads(c, 1, nil)
	if len(threads) != dapMaxThreads {
		t.Errorf("threads = %d, want them capped at %d", len(threads), dapMaxThreads)
	}
	if !threads[0].stopped || threads[0].id != 1 {
		t.Error("the stopped thread must stay first even when capping")
	}
}

// ---------------------------------------------------------------------------
// dapFetchStoppedInfo
// ---------------------------------------------------------------------------

func TestDapFetchStoppedInfo_FetchesThreadsAndLocals(t *testing.T) {
	e, _ := newDAPCapEditor("line1\nline2\nline3\n")
	c, cleanup := dapFakeServer(t, map[string]any{
		"threads": map[string]any{"threads": []map[string]any{
			{"id": 1, "name": "main"},
			{"id": 2, "name": "worker"},
		}},
		"stackTrace": map[string]any{"stackFrames": []map[string]any{
			{"id": 11, "name": "main.main", "line": 2},
		}},
		"scopes": map[string]any{"scopes": []map[string]any{
			{"name": "Locals", "variablesReference": 100},
		}},
		"variables": map[string]any{"variables": []map[string]any{
			{"name": "x", "value": "42", "type": "int"},
		}},
	})
	defer cleanup()
	e.dap.client = c
	e.dap.localsBuf = e.ensureDebugBuf("*Debug Locals*", "debug-locals")
	e.dap.stackBuf = e.ensureDebugBuf("*Debug Stack*", "debug-stack")

	e.dapFetchStoppedInfo(dap.StoppedEvent{ThreadID: 1, Reason: "breakpoint"})
	select {
	case fn := <-e.dapCbs:
		fn()
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the stopped-info callback")
	}

	e.dap.framesMu.RLock()
	nThreads, nFrames := len(e.dap.threads), len(e.dap.frames)
	e.dap.framesMu.RUnlock()
	if nFrames != 1 {
		t.Errorf("frames = %d, want 1", nFrames)
	}
	if nThreads != 2 {
		t.Errorf("threads = %d, want both threads in the stack panel", nThreads)
	}
	if e.dap.stoppedLine != 2 {
		t.Errorf("stoppedLine = %d, want 2", e.dap.stoppedLine)
	}
	if !strings.Contains(e.dap.stackBuf.String(), "Thread 2: worker") {
		t.Errorf("stack panel should list every thread, got %q", e.dap.stackBuf.String())
	}
	e.dap.localsMu.RLock()
	locals := e.dap.locals
	e.dap.localsMu.RUnlock()
	if len(locals) != 1 || locals[0].name != "x" {
		t.Errorf("locals = %+v, want the single local x", locals)
	}
}

func TestDapFetchStoppedInfo_UsesConfiguredExpandDepth(t *testing.T) {
	e, _ := newDAPCapEditor("a\n")
	// Every "variables" reply reports a nested struct, so the recursion depth is
	// bounded only by localsAutoExpandDepth.
	c, cleanup := dapFakeServer(t, map[string]any{
		"stackTrace": map[string]any{"stackFrames": []map[string]any{{"id": 1, "name": "f", "line": 1}}},
		"scopes":     map[string]any{"scopes": []map[string]any{{"variablesReference": 5}}},
		"variables": map[string]any{"variables": []map[string]any{
			{"name": "nested", "value": "{...}", "variablesReference": 5},
		}},
	})
	defer cleanup()
	e.dap.client = c
	e.dap.localsBuf = e.ensureDebugBuf("*Debug Locals*", "debug-locals")
	e.dap.stackBuf = e.ensureDebugBuf("*Debug Stack*", "debug-stack")
	e.dap.localsAutoExpandDepth = 3

	e.dapFetchStoppedInfo(dap.StoppedEvent{ThreadID: 1, Reason: "step"})
	select {
	case fn := <-e.dapCbs:
		fn()
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the stopped-info callback")
	}

	e.dap.localsMu.RLock()
	defer e.dap.localsMu.RUnlock()
	depth := 0
	for v := e.dap.locals; len(v) > 0; v = v[0].children {
		depth++
	}
	// Depth 3 auto-expands three levels of children below the root level.
	if depth != 4 {
		t.Errorf("auto-expanded tree depth = %d, want 4 for debug-locals-auto-expand-depth 3", depth)
	}
}

func TestDapFetchStoppedInfo_MakesSteppedIntoFileReadOnly(t *testing.T) {
	dir := t.TempDir()
	other := filepath.Join(dir, "other.go")
	if err := os.WriteFile(other, []byte("package other\n\nfunc F() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	e, _ := newDAPCapEditor("package main\n")
	e.autoRevertMtimes = make(map[*buffer.Buffer]time.Time)
	e.lspConns = make(map[string]*lspConn)
	c, cleanup := dapFakeServer(t, map[string]any{
		"stackTrace": map[string]any{"stackFrames": []map[string]any{
			{"id": 1, "name": "other.F", "line": 3,
				"source": map[string]any{"path": other, "name": "other.go"}},
		}},
	})
	defer cleanup()
	e.dap.client = c
	e.dap.localsBuf = e.ensureDebugBuf("*Debug Locals*", "debug-locals")
	e.dap.stackBuf = e.ensureDebugBuf("*Debug Stack*", "debug-stack")
	e.dap.prevActiveWin = e.windows[0]

	e.dapFetchStoppedInfo(dap.StoppedEvent{ThreadID: 1, Reason: "step"})
	select {
	case fn := <-e.dapCbs:
		fn()
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the stopped-info callback")
	}

	steppedBuf := e.windows[0].Buf()
	if steppedBuf.Filename() != other {
		t.Fatalf("source window shows %q, want the stepped-into file %q", steppedBuf.Filename(), other)
	}
	if !steppedBuf.ReadOnly() {
		t.Error("a source buffer opened while stepping must be read-only")
	}
	if _, tracked := e.dap.prevReadOnly[steppedBuf]; !tracked {
		t.Error("the stepped-into buffer must be tracked so teardown can restore it")
	}

	e.debugTeardownLayout()
	if steppedBuf.ReadOnly() {
		t.Error("teardown should restore the stepped-into buffer to writable")
	}
}

// ---------------------------------------------------------------------------
// debugSourceDispatch routes a language-suffixed REPL mode to the REPL
// ---------------------------------------------------------------------------

func TestDebugSourceDispatch_RoutesSuffixedReplMode(t *testing.T) {
	e, _ := newDAPCapEditor("")
	replBuf := e.ensureDebugBuf("*Debug REPL*", "debug-repl+java")
	e.dap.replBuf = replBuf
	dapReplReset(replBuf)
	e.windows[0].SetBuf(replBuf)

	// 'n' must type into the REPL, not step to the next line.
	if !e.debugSourceDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'n'}) {
		t.Fatal("'n' should be consumed by the REPL")
	}
	if got := dapReplGetInput(replBuf); got != "n" {
		t.Errorf("REPL input = %q, want \"n\" (the key must not step)", got)
	}
}

func TestDapStartAdapter_JdtlsSuccess(t *testing.T) {
	// Stand in for the java-debug adapter jdtls would have created.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close() //nolint:errcheck
	_, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		if c, aerr := ln.Accept(); aerr == nil {
			defer c.Close() //nolint:errcheck
			<-time.After(time.Second)
		}
	}()

	conn, cleanup := fakeJdtlsServer(t, map[string]any{
		jdtlsStartDebugSession: port,
		jdtlsResolveMainClass: []any{
			map[string]any{"mainClass": "com.example.Main", "projectName": "demo", "filePath": "/src/Main.java"},
		},
		jdtlsResolveClasspath: []any{[]string{}, []string{"/target/classes"}},
	})
	defer cleanup()

	info := langModeByName("java")
	client, launch, note, err := dapStartAdapter(info, dapLaunchRequest{
		file:    "/src/Main.java",
		runDir:  "/src",
		lspConn: conn,
	})
	if err != nil {
		t.Fatalf("dapStartAdapter: %v", err)
	}
	defer client.Close()
	if launch["mainClass"] != "com.example.Main" {
		t.Errorf("launch args = %v, want the resolved main class", launch)
	}
	if note != "" {
		t.Errorf("note = %q, want none when the project's main class was used", note)
	}
}

func TestDapStartAdapter_JdtlsClasspathFailureDoesNotStartAdapter(t *testing.T) {
	// resolveMainClass works but resolveClasspath is missing: the adapter must
	// never be asked for, so no session is leaked.
	conn, cleanup := fakeJdtlsServer(t, map[string]any{
		jdtlsResolveMainClass: []any{
			map[string]any{"mainClass": "com.example.Main", "projectName": "demo"},
		},
	})
	defer cleanup()

	info := langModeByName("java")
	_, _, _, err := dapStartAdapter(info, dapLaunchRequest{file: "/src/Main.java", runDir: "/src", lspConn: conn})
	if err == nil {
		t.Fatal("expected the classpath failure to abort the start")
	}
	if !strings.Contains(err.Error(), jdtlsResolveClasspath) {
		t.Errorf("error = %v, want it to name the failing command", err)
	}
}

func TestDispatchParsedKey_SuffixedReplModeTypesInsteadOfStepping(t *testing.T) {
	e, m := newDAPCapEditor("")
	replBuf := e.ensureDebugBuf("*Debug REPL*", "debug-repl+java")
	e.dap.replBuf = replBuf
	dapReplReset(replBuf)
	e.windows[0].SetBuf(replBuf)

	e.dispatchParsedKey(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'n'})

	if got := dapReplGetInput(replBuf); got != "n" {
		t.Errorf("REPL input = %q, want \"n\"", got)
	}
	select {
	case call := <-m.called:
		t.Errorf("'n' must not reach the debugger, got a %q call", call)
	case <-time.After(200 * time.Millisecond):
	}
}
