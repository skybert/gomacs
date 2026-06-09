package editor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/dap"
	"github.com/skybert/gomacs/internal/terminal"
	"github.com/skybert/gomacs/internal/window"
)

// newDAPPanelsTestEditor wires up a Locals + Stack buffer.
func newDAPPanelsTestEditor(t *testing.T) *Editor {
	t.Helper()
	e := newDAPTestEditor("")
	localsBuf := buffer.New("*Debug Locals*")
	localsBuf.SetMode("debug-locals")
	stackBuf := buffer.New("*Debug Stack*")
	stackBuf.SetMode("debug-stack")
	e.buffers = append(e.buffers, localsBuf, stackBuf)
	e.windows = append(e.windows,
		window.New(localsBuf, 0, 60, 20, 12),
		window.New(stackBuf, 12, 60, 20, 12),
	)
	e.dap = &dapState{
		localsBuf:             localsBuf,
		stackBuf:              stackBuf,
		localsAutoExpandDepth: 1,
	}
	return e
}

// ---------------------------------------------------------------------------
// dapWriteVariable
// ---------------------------------------------------------------------------

func TestDapWriteVariableLeaf(t *testing.T) {
	var sb strings.Builder
	v := &dapVariable{name: "x", value: "42", typeStr: "int", varRef: 0}
	dapWriteVariable(&sb, v, 0, nil)
	got := sb.String()
	if !strings.Contains(got, "x int = 42") {
		t.Errorf("output = %q, want to contain \"x int = 42\"", got)
	}
	// No expand arrow for leaves.
	if strings.Contains(got, "▶") || strings.Contains(got, "▼") {
		t.Errorf("leaf should have no arrow: %q", got)
	}
}

func TestDapWriteVariableCollapsed(t *testing.T) {
	var sb strings.Builder
	v := &dapVariable{name: "obj", value: "0xdeadbeef", varRef: 7}
	dapWriteVariable(&sb, v, 0, nil)
	if !strings.Contains(sb.String(), "▶") {
		t.Errorf("collapsed expected ▶: %q", sb.String())
	}
}

func TestDapWriteVariableExpanded(t *testing.T) {
	var sb strings.Builder
	v := &dapVariable{
		name:     "obj",
		value:    "{...}",
		varRef:   7,
		expanded: true,
		children: []dapVariable{
			{name: "field", value: "1", depth: 1},
		},
	}
	dapWriteVariable(&sb, v, 0, nil)
	got := sb.String()
	if !strings.Contains(got, "▼") {
		t.Errorf("expanded expected ▼: %q", got)
	}
	if !strings.Contains(got, "field = 1") {
		t.Errorf("expanded child missing: %q", got)
	}
}

func TestDapWriteVariableLineMap(t *testing.T) {
	var sb strings.Builder
	var lineMap []*dapVariable
	v := &dapVariable{
		name:     "obj",
		value:    "v",
		varRef:   1,
		expanded: true,
		children: []dapVariable{
			{name: "child1", value: "c1"},
			{name: "child2", value: "c2"},
		},
	}
	dapWriteVariable(&sb, v, 0, &lineMap)
	if len(lineMap) != 3 {
		t.Errorf("lineMap len = %d, want 3", len(lineMap))
	}
	if lineMap[0] != v || lineMap[1] != &v.children[0] || lineMap[2] != &v.children[1] {
		t.Errorf("lineMap content unexpected")
	}
}

// ---------------------------------------------------------------------------
// dapRenderLocals / dapRenderStack
// ---------------------------------------------------------------------------

func TestDapRenderLocalsEmpty(t *testing.T) {
	e := newDAPPanelsTestEditor(t)
	dapRenderLocals(e)
	if got := e.dap.localsBuf.String(); !strings.Contains(got, "(no locals)") {
		t.Errorf("empty locals = %q", got)
	}
}

func TestDapRenderLocalsTree(t *testing.T) {
	e := newDAPPanelsTestEditor(t)
	e.dap.locals = []dapVariable{
		{name: "a", value: "1"},
		{name: "b", value: "2"},
	}
	dapRenderLocals(e)
	got := e.dap.localsBuf.String()
	if !strings.Contains(got, "a = 1") || !strings.Contains(got, "b = 2") {
		t.Errorf("locals = %q", got)
	}
	if len(e.dap.localsLineMap) != 2 {
		t.Errorf("lineMap len = %d, want 2", len(e.dap.localsLineMap))
	}
}

func TestDapRenderLocalsNoSession(t *testing.T) {
	e := newDAPTestEditor("")
	// e.dap nil → no panic.
	dapRenderLocals(e)
}

func TestDapRenderLocalsNoBuf(t *testing.T) {
	e := newDAPTestEditor("")
	e.dap = &dapState{}
	dapRenderLocals(e)
}

func TestDapRenderStackEmpty(t *testing.T) {
	e := newDAPPanelsTestEditor(t)
	dapRenderStack(e)
	if got := e.dap.stackBuf.String(); !strings.Contains(got, "(no stack)") {
		t.Errorf("empty stack = %q", got)
	}
}

func TestDapRenderStackFrames(t *testing.T) {
	e := newDAPPanelsTestEditor(t)
	e.dap.frames = []dap.StackFrame{
		{ID: 1, Name: "main.foo", Source: dap.Source{Path: "/a.go", Name: "a.go"}, Line: 10},
		{ID: 2, Name: "main.bar", Source: dap.Source{Path: "", Name: "b.go"}, Line: 20},
	}
	dapRenderStack(e)
	got := e.dap.stackBuf.String()
	if !strings.Contains(got, "#0  main.foo (/a.go:10)") {
		t.Errorf("frame0 = %q", got)
	}
	if !strings.Contains(got, "#1  main.bar (b.go:20)") {
		t.Errorf("frame1 = %q (should fall back to Name)", got)
	}
}

func TestDapRefreshPanels(t *testing.T) {
	e := newDAPPanelsTestEditor(t)
	e.dap.locals = []dapVariable{{name: "x", value: "1"}}
	e.dap.frames = []dap.StackFrame{{Name: "f", Line: 1}}
	e.dapRefreshPanels()
	if !strings.Contains(e.dap.localsBuf.String(), "x = 1") {
		t.Error("locals not rendered")
	}
	if !strings.Contains(e.dap.stackBuf.String(), "f") {
		t.Error("stack not rendered")
	}
}

// ---------------------------------------------------------------------------
// debugLocalsDispatch / debugStackDispatch
// ---------------------------------------------------------------------------

func TestDebugLocalsDispatchQuit(t *testing.T) {
	e := newDAPPanelsTestEditor(t)
	prevWin := e.windows[0]
	e.dap.prevActiveWin = prevWin
	if !e.debugLocalsDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'q'}) {
		t.Error("q should be consumed")
	}
	if e.activeWin != prevWin {
		t.Error("q should restore prevActiveWin")
	}
}

func TestDebugLocalsDispatchNavigation(t *testing.T) {
	e := newDAPPanelsTestEditor(t)
	e.dap.locals = []dapVariable{{name: "a", value: "1"}, {name: "b", value: "2"}}
	dapRenderLocals(e)
	// Set active window to locals window
	e.activeWin = e.windows[1] // locals window

	if !e.debugLocalsDispatch(terminal.KeyEvent{Key: tcell.KeyDown}) {
		t.Error("Down should be consumed")
	}
	if !e.debugLocalsDispatch(terminal.KeyEvent{Key: tcell.KeyUp}) {
		t.Error("Up should be consumed")
	}
	if !e.debugLocalsDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'n'}) {
		t.Error("n should be consumed")
	}
	if !e.debugLocalsDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'p'}) {
		t.Error("p should be consumed")
	}
}

func TestDebugLocalsDispatchExpandCollapseNoClient(t *testing.T) {
	e := newDAPPanelsTestEditor(t)
	// No client → toggle is a no-op but key consumed.
	if !e.debugLocalsDispatch(terminal.KeyEvent{Key: tcell.KeyEnter}) {
		t.Error("Enter consumed")
	}
	if !e.debugLocalsDispatch(terminal.KeyEvent{Key: tcell.KeyLeft}) {
		t.Error("Left consumed")
	}
}

func TestDebugLocalsDispatchUnknown(t *testing.T) {
	e := newDAPPanelsTestEditor(t)
	if e.debugLocalsDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'x'}) {
		t.Error("'x' should not be consumed")
	}
}

func TestDebugStackDispatchQuit(t *testing.T) {
	e := newDAPPanelsTestEditor(t)
	prevWin := e.windows[0]
	e.dap.prevActiveWin = prevWin
	e.activeWin = e.windows[2] // stack window
	if !e.debugStackDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'q'}) {
		t.Error("q consumed")
	}
	if e.activeWin != prevWin {
		t.Error("q restores prev active win")
	}
}

func TestDebugStackDispatchNavigation(t *testing.T) {
	e := newDAPPanelsTestEditor(t)
	e.dap.frames = []dap.StackFrame{{Name: "f1", Line: 1}, {Name: "f2", Line: 2}}
	dapRenderStack(e)
	e.activeWin = e.windows[2]
	if !e.debugStackDispatch(terminal.KeyEvent{Key: tcell.KeyDown}) {
		t.Error("Down consumed")
	}
	if !e.debugStackDispatch(terminal.KeyEvent{Key: tcell.KeyUp}) {
		t.Error("Up consumed")
	}
	if !e.debugStackDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'n'}) {
		t.Error("n consumed")
	}
	if !e.debugStackDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'p'}) {
		t.Error("p consumed")
	}
}

func TestDebugStackDispatchEnterEmpty(t *testing.T) {
	e := newDAPPanelsTestEditor(t)
	// No frames → Enter consumed but no-op.
	if !e.debugStackDispatch(terminal.KeyEvent{Key: tcell.KeyEnter}) {
		t.Error("Enter consumed")
	}
}

func TestDebugStackDispatchUnknown(t *testing.T) {
	e := newDAPPanelsTestEditor(t)
	if e.debugStackDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'z'}) {
		t.Error("'z' should not be consumed")
	}
}

// ---------------------------------------------------------------------------
// dapStackJumpToFrame edge cases (no panic when nothing is set)
// ---------------------------------------------------------------------------

func TestDapStackJumpToFrameNoSession(t *testing.T) {
	e := newDAPTestEditor("")
	e.dapStackJumpToFrame() // nil dap → no panic
}

func TestDapStackJumpToFrameOutOfRange(t *testing.T) {
	e := newDAPPanelsTestEditor(t)
	// No frames; line 1 → frameIdx 0 → out of range.
	e.dapStackJumpToFrame()
}

func TestDapStackJumpToFrameNoPath(t *testing.T) {
	e := newDAPPanelsTestEditor(t)
	e.dap.frames = []dap.StackFrame{{Name: "f", Source: dap.Source{}, Line: 1}}
	dapRenderStack(e)
	e.dapStackJumpToFrame() // frame.Source.Path empty → return early
}

func TestDapStackJumpToFrameSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "src.txt")
	_ = os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0o644)

	e := newDAPPanelsTestEditor(t)
	e.autoRevertMtimes = make(map[*buffer.Buffer]time.Time)
	e.lspConns = make(map[string]*lspConn)
	e.dap.prevActiveWin = e.windows[0]
	e.dap.frames = []dap.StackFrame{{Name: "f", Source: dap.Source{Path: path}, Line: 2}}
	dapRenderStack(e)
	e.dap.stackBuf.SetPoint(0) // line 1 → frame index 0

	e.dapStackJumpToFrame()

	if e.activeWin.Buf().Filename() != path {
		t.Errorf("expected source window to show %q, got %q", path, e.activeWin.Buf().Filename())
	}
	line, _ := e.activeWin.Buf().LineCol(e.activeWin.Buf().Point())
	if line != 2 {
		t.Errorf("expected point at line 2, got %d", line)
	}
}

// ---------------------------------------------------------------------------
// dapLocalsToggleExpand (with a fake DAP client)
// ---------------------------------------------------------------------------

func TestDapLocalsToggleExpand_Collapse(t *testing.T) {
	e := newDAPPanelsTestEditor(t)
	c, cleanup := dapFakeServer(t, nil)
	defer cleanup()
	e.dap.client = c
	e.dap.locals = []dapVariable{{name: "obj", varRef: 5, expanded: true}}
	dapRenderLocals(e)
	e.dap.localsBuf.SetPoint(0) // first variable line
	e.dapLocalsToggleExpand(false)
	if e.dap.localsLineMap[0].expanded {
		t.Error("toggle(false) should collapse the variable")
	}
}

func TestDapLocalsToggleExpand_LeafNoOp(t *testing.T) {
	e := newDAPPanelsTestEditor(t)
	c, cleanup := dapFakeServer(t, nil)
	defer cleanup()
	e.dap.client = c
	e.dap.locals = []dapVariable{{name: "x", value: "1", varRef: 0}} // leaf
	dapRenderLocals(e)
	e.dap.localsBuf.SetPoint(0)
	e.dapLocalsToggleExpand(true) // leaf → no-op, no panic
}

func TestDapLocalsToggleExpand_FetchesChildren(t *testing.T) {
	e := newDAPPanelsTestEditor(t)
	e.term = &terminal.Terminal{}
	e.dapCbs = make(chan func(), 16)
	c, cleanup := dapFakeServer(t, map[string]any{
		"variables": map[string]any{"variables": []map[string]any{
			{"name": "field", "value": "7"},
		}},
	})
	defer cleanup()
	e.dap.client = c
	e.dap.locals = []dapVariable{{name: "obj", varRef: 5}}
	dapRenderLocals(e)
	e.dap.localsBuf.SetPoint(0)

	e.dapLocalsToggleExpand(true)
	select {
	case fn := <-e.dapCbs:
		fn()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for variables fetch")
	}
	if !e.dap.localsLineMap[0].expanded {
		t.Error("toggle(true) should expand the variable after fetching children")
	}
}

// ---------------------------------------------------------------------------
// openFileIntoBuffer
// ---------------------------------------------------------------------------

func TestOpenFileIntoBufferAlreadyOpen(t *testing.T) {
	e := newDAPTestEditor("")
	b := buffer.NewWithContent("/x", "data")
	b.SetFilename("/already/open.go")
	e.buffers = append(e.buffers, b)
	got, err := e.openFileIntoBuffer("/already/open.go")
	if err != nil {
		t.Fatal(err)
	}
	if got != b {
		t.Error("expected existing buffer")
	}
}
