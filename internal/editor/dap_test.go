package editor

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/buffer"
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
// debug layout
// ---------------------------------------------------------------------------

func TestDebugSetupAndTeardownLayout(t *testing.T) {
	e, _ := newDAPCapEditor("package main\n")
	e.debugSetupLayout()
	if len(e.windows) != 4 {
		t.Fatalf("debugSetupLayout should create 4 windows, got %d", len(e.windows))
	}
	if e.dap.localsBuf == nil || e.dap.stackBuf == nil || e.dap.replBuf == nil {
		t.Fatal("debugSetupLayout should create the three panel buffers")
	}
	e.debugTeardownLayout()
	if len(e.windows) != 1 {
		t.Fatalf("debugTeardownLayout should restore a single window, got %d", len(e.windows))
	}
}

func TestEnsureDebugBuf_CreatesAndReuses(t *testing.T) {
	e, _ := newDAPCapEditor("")
	b1 := e.ensureDebugBuf("*Debug Locals*", "debug-locals")
	if b1.Mode() != "debug-locals" {
		t.Fatalf("expected mode debug-locals, got %q", b1.Mode())
	}
	b2 := e.ensureDebugBuf("*Debug Locals*", "debug-locals")
	if b1 != b2 {
		t.Fatal("ensureDebugBuf should reuse an existing buffer")
	}
}

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
