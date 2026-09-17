package editor

import (
	"testing"

	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/window"
)

// ---------------------------------------------------------------------------
// debugSetupLayout / debugTeardownLayout
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

func TestDebugSetupLayoutGeometry(t *testing.T) {
	e, _ := newDAPCapEditor("package main\n")
	e.debugSetupLayout()

	tw, th := e.term.Size()
	totalH := th - 1
	rightW := max(tw/3, 10)
	wantSrcW := tw - rightW - 1

	src, locals, stack, repl := e.windows[0], e.windows[1], e.windows[2], e.windows[3]
	if src.Width() != wantSrcW {
		t.Errorf("source width = %d, want %d", src.Width(), wantSrcW)
	}
	if src.GutterWidth() != 2 {
		t.Errorf("source gutter = %d, want 2 (breakpoint column)", src.GutterWidth())
	}
	if locals.Left() != sourceLeft(src) || stack.Left() != sourceLeft(src) {
		t.Errorf("panels left = %d/%d, want %d", locals.Left(), stack.Left(), sourceLeft(src))
	}
	if repl.Width() != tw {
		t.Errorf("repl width = %d, want full width %d", repl.Width(), tw)
	}
	if locals.Height()+stack.Height()+repl.Height() != totalH {
		t.Errorf("panel heights %d+%d+%d should fill %d",
			locals.Height(), stack.Height(), repl.Height(), totalH)
	}
	if got := e.dap.replBuf.String(); got != dapReplPrompt {
		t.Errorf("REPL buffer = %q, want the seeded prompt %q", got, dapReplPrompt)
	}
	if e.dap.prevActiveWin != src {
		t.Error("debugSetupLayout should remember the source window")
	}
}

func TestDebugSetupLayoutCollapsesExistingSplits(t *testing.T) {
	e, _ := newDAPCapEditor("package main\n")
	extra := buffer.New("*other*")
	e.buffers = append(e.buffers, extra)
	e.windows = append(e.windows, window.New(extra, 0, 40, 40, 12))

	e.debugSetupLayout()
	if len(e.windows) != 4 {
		t.Fatalf("existing splits should be collapsed to the 4 debug windows, got %d", len(e.windows))
	}
	if e.windows[0].Buf() != e.buffers[0] {
		t.Error("the previously active window should stay at index 0")
	}
}

func TestDebugSetupLayoutReplModeCarriesLanguage(t *testing.T) {
	e, _ := newDAPCapEditor("class Foo {}\n")
	e.dap.mode = "java"
	e.debugSetupLayout()
	if got := e.dap.replBuf.Mode(); got != "debug-repl+java" {
		t.Errorf("REPL mode = %q, want \"debug-repl+java\"", got)
	}
}

func TestDebugSetupLayoutReplModePlainWithoutLanguage(t *testing.T) {
	e, _ := newDAPCapEditor("x\n")
	// e.dap.mode is empty (session state built directly by the test helper).
	e.debugSetupLayout()
	if got := e.dap.replBuf.Mode(); got != debugReplMode {
		t.Errorf("REPL mode = %q, want %q", got, debugReplMode)
	}
}

// ---------------------------------------------------------------------------
// dapReplModeFor
// ---------------------------------------------------------------------------

func TestDapReplModeFor(t *testing.T) {
	tests := []struct{ lang, want string }{
		{"", "debug-repl"},
		{"go", "debug-repl+go"},
		{"java", "debug-repl+java"},
	}
	for _, tt := range tests {
		if got := dapReplModeFor(tt.lang); got != tt.want {
			t.Errorf("dapReplModeFor(%q) = %q, want %q", tt.lang, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// read-only bookkeeping (debugMarkSourceReadOnly)
// ---------------------------------------------------------------------------

func TestDebugMarkSourceReadOnlyRecordsPreviousFlag(t *testing.T) {
	e, _ := newDAPCapEditor("code\n")
	src := e.buffers[0]
	if src.ReadOnly() {
		t.Fatal("test buffer should start writable")
	}
	e.debugMarkSourceReadOnly(src)
	if !src.ReadOnly() {
		t.Error("buffer should be read-only during the session")
	}
	if ro, ok := e.dap.prevReadOnly[src]; !ok || ro {
		t.Errorf("prevReadOnly[src] = (%v, %v), want (false, true)", ro, ok)
	}

	// Marking twice must not overwrite the remembered flag with the forced one.
	e.debugMarkSourceReadOnly(src)
	if e.dap.prevReadOnly[src] {
		t.Error("second mark overwrote the remembered read-only flag")
	}
}

func TestDebugMarkSourceReadOnlyNoSessionOrNilBuf(t *testing.T) {
	e, _ := newDAPCapEditor("code\n")
	e.debugMarkSourceReadOnly(nil) // no panic, nothing recorded
	if len(e.dap.prevReadOnly) != 0 {
		t.Error("nil buffer should not be recorded")
	}
	e.dap = nil
	e.debugMarkSourceReadOnly(e.buffers[0]) // no session → no panic
}

func TestDebugTeardownRestoresAllSourceBuffers(t *testing.T) {
	e, _ := newDAPCapEditor("code\n")
	e.debugSetupLayout()

	src := e.windows[0].Buf()
	// A second source file, as opened by stepping into another file.
	stepped := buffer.New("other.go")
	e.buffers = append(e.buffers, stepped)
	e.debugMarkSourceReadOnly(stepped)

	// A third one that was already read-only before the session started.
	alreadyRO := buffer.New("generated.go")
	alreadyRO.SetReadOnly(true)
	e.buffers = append(e.buffers, alreadyRO)
	e.debugMarkSourceReadOnly(alreadyRO)

	if !src.ReadOnly() || !stepped.ReadOnly() || !alreadyRO.ReadOnly() {
		t.Fatal("all source buffers should be read-only while debugging")
	}

	e.debugTeardownLayout()

	if src.ReadOnly() {
		t.Error("the original source buffer should be writable again")
	}
	if stepped.ReadOnly() {
		t.Error("a buffer opened while stepping should be writable again")
	}
	if !alreadyRO.ReadOnly() {
		t.Error("a buffer that was read-only before the session must stay read-only")
	}
	if e.dap.prevReadOnly != nil {
		t.Error("teardown should clear the read-only bookkeeping")
	}
}

func TestDebugTeardownLayoutNoSession(t *testing.T) {
	e, _ := newDAPCapEditor("")
	e.dap = nil
	e.debugTeardownLayout() // no-op, no panic
}

func TestDebugTeardownLayoutUntrackedSourceBuffer(t *testing.T) {
	// Session state built without debugSetupLayout: teardown still clears the
	// read-only flag of windows[0] so the user is not left with a locked buffer.
	e, _ := newDAPCapEditor("code\n")
	e.windows[0].Buf().SetReadOnly(true)
	e.debugTeardownLayout()
	if e.windows[0].Buf().ReadOnly() {
		t.Error("untracked source buffer should be made writable on teardown")
	}
}

func TestDebugTeardownLayoutResetsWindowState(t *testing.T) {
	e, _ := newDAPCapEditor("code\n")
	e.debugSetupLayout()
	e.activeWin = e.windows[3] // focus the REPL
	e.debugTeardownLayout()
	if e.activeWin != e.windows[0] {
		t.Error("teardown should make the source window active")
	}
	if e.windows[0].GutterWidth() != 0 {
		t.Errorf("gutter = %d, want 0 after teardown", e.windows[0].GutterWidth())
	}
	if e.layoutRoot == nil || e.layoutRoot.win != e.windows[0] {
		t.Error("teardown should reset the split tree to the source window")
	}
}

// ---------------------------------------------------------------------------
// ensureDebugBuf
// ---------------------------------------------------------------------------

func TestEnsureDebugBuf_CreatesAndReuses(t *testing.T) {
	e, _ := newDAPCapEditor("")
	before := len(e.buffers)
	b1 := e.ensureDebugBuf("*Debug Locals*", "debug-locals")
	if b1.Mode() != "debug-locals" {
		t.Fatalf("expected mode debug-locals, got %q", b1.Mode())
	}
	if len(e.buffers) != before+1 {
		t.Fatalf("ensureDebugBuf should add one buffer, %d → %d", before, len(e.buffers))
	}
	b2 := e.ensureDebugBuf("*Debug Locals*", "debug-locals")
	if b1 != b2 {
		t.Fatal("ensureDebugBuf should reuse an existing buffer")
	}
	if len(e.buffers) != before+1 {
		t.Error("reusing a buffer should not append another one")
	}
}

func TestEnsureDebugBufUpdatesMode(t *testing.T) {
	e, _ := newDAPCapEditor("")
	b := e.ensureDebugBuf("*Debug REPL*", debugReplMode)
	b2 := e.ensureDebugBuf("*Debug REPL*", "debug-repl+java")
	if b != b2 {
		t.Fatal("same name should give the same buffer")
	}
	if b.Mode() != "debug-repl+java" {
		t.Errorf("mode = %q, want it updated to debug-repl+java", b.Mode())
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

	// Stack sits directly below locals, REPL below both.
	stack := e.windows[2]
	if stack.Top() != locals.Top()+locals.Height() {
		t.Errorf("stack top = %d, want %d", stack.Top(), locals.Top()+locals.Height())
	}
	if repl.Top() != locals.Height()+stack.Height() {
		t.Errorf("repl top = %d, want %d", repl.Top(), locals.Height()+stack.Height())
	}
}

func TestDapRelayoutWindowsWrongWindowCount(t *testing.T) {
	e := newDAPTestEditor("hello")
	e.dap = &dapState{}
	before := e.windows[0].Width()
	e.dapRelayoutWindows(200, 60) // only 1 window → no-op
	if e.windows[0].Width() != before {
		t.Error("dapRelayoutWindows should do nothing unless there are 4 windows")
	}
}
