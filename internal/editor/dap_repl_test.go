package editor

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/terminal"
	"github.com/skybert/gomacs/internal/window"
)

func TestDapReplSubmit_Evaluates(t *testing.T) {
	e := newDAPReplTestEditor(t)
	e.term = &terminal.Terminal{}
	e.dapCbs = make(chan func(), 16)
	c, cleanup := dapFakeServer(t, map[string]any{
		"stackTrace": map[string]any{"stackFrames": []map[string]any{{"id": 1}}},
		"evaluate":   map[string]any{"result": "99"},
	})
	defer cleanup()
	e.dap.client = c
	e.dap.stoppedThread = 1
	e.dapReplSetInput("myvar")
	e.dapReplSubmit()
	select {
	case fn := <-e.dapCbs:
		fn()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for evaluate callback")
	}
	if !strings.Contains(e.dap.replBuf.String(), "99") {
		t.Errorf("expected eval result 99 in REPL, got %q", e.dap.replBuf.String())
	}
}

// newDAPReplTestEditor builds an Editor with a *Debug REPL* buffer wired up.
func newDAPReplTestEditor(t *testing.T) *Editor {
	t.Helper()
	e := newDAPTestEditor("")
	replBuf := buffer.New("*Debug REPL*")
	replBuf.SetMode("debug-repl")
	e.buffers = append(e.buffers, replBuf)
	replWin := window.New(replBuf, 18, 0, 80, 6)
	e.windows = append(e.windows, replWin)
	e.dap = &dapState{
		replBuf:               replBuf,
		localsAutoExpandDepth: 1,
	}
	dapReplReset(replBuf)
	return e
}

// ---------------------------------------------------------------------------
// dapReplReset / dapReplGetInput / dapReplPromptPos
// ---------------------------------------------------------------------------

func TestDapReplResetWritesPrompt(t *testing.T) {
	b := buffer.New("*Debug REPL*")
	dapReplReset(b)
	if b.String() != dapReplPrompt {
		t.Errorf("buf = %q, want %q", b.String(), dapReplPrompt)
	}
	if b.Point() != b.Len() {
		t.Errorf("point = %d, want %d", b.Point(), b.Len())
	}
}

func TestDapReplGetInput(t *testing.T) {
	b := buffer.New("*Debug REPL*")
	dapReplReset(b)
	b.SetReadOnly(false)
	b.InsertString(b.Len(), "hello")
	if got := dapReplGetInput(b); got != "hello" {
		t.Errorf("input = %q, want %q", got, "hello")
	}
}

func TestDapReplGetInputNoPrompt(t *testing.T) {
	b := buffer.NewWithContent("x", "raw text")
	if got := dapReplGetInput(b); got != "raw text" {
		t.Errorf("input = %q, want raw passthrough", got)
	}
}

func TestDapReplPromptPos(t *testing.T) {
	b := buffer.New("*Debug REPL*")
	dapReplReset(b)
	pos := dapReplPromptPos(b)
	want := len([]rune(dapReplPrompt))
	if pos != want {
		t.Errorf("promptPos = %d, want %d", pos, want)
	}
}

func TestDapReplPromptPosNoPrompt(t *testing.T) {
	b := buffer.NewWithContent("x", "no prompt here")
	if pos := dapReplPromptPos(b); pos != 0 {
		t.Errorf("promptPos = %d, want 0 when no prompt", pos)
	}
}

// ---------------------------------------------------------------------------
// dapReplAppend / dapReplSetInput
// ---------------------------------------------------------------------------

func TestDapReplAppendKeepsPromptAtEnd(t *testing.T) {
	e := newDAPReplTestEditor(t)
	e.dapReplAppend("hello\n")
	got := e.dap.replBuf.String()
	want := "hello\n" + dapReplPrompt
	if got != want {
		t.Errorf("buf = %q, want %q", got, want)
	}
}

func TestDapReplAppendAddsTrailingNewline(t *testing.T) {
	e := newDAPReplTestEditor(t)
	e.dapReplAppend("no newline")
	got := e.dap.replBuf.String()
	if got != "no newline\n"+dapReplPrompt {
		t.Errorf("buf = %q", got)
	}
}

func TestDapReplAppendNoOpWhenNoSession(t *testing.T) {
	e := newDAPTestEditor("")
	// e.dap is nil; should not panic.
	e.dapReplAppend("ignored")
}

func TestDapReplSetInput(t *testing.T) {
	e := newDAPReplTestEditor(t)
	e.dapReplSetInput("hello world")
	if got := dapReplGetInput(e.dap.replBuf); got != "hello world" {
		t.Errorf("input after Set = %q", got)
	}
	e.dapReplSetInput("")
	if got := dapReplGetInput(e.dap.replBuf); got != "" {
		t.Errorf("input after empty Set = %q", got)
	}
}

// ---------------------------------------------------------------------------
// History
// ---------------------------------------------------------------------------

func TestDapReplHistoryPrevNext(t *testing.T) {
	e := newDAPReplTestEditor(t)
	e.dap.replHistory = []string{"first", "second", "third"}
	e.dap.replHistoryIdx = len(e.dap.replHistory) // past-end

	e.dapReplHistoryPrev()
	if got := dapReplGetInput(e.dap.replBuf); got != "third" {
		t.Errorf("prev1 input = %q, want third", got)
	}
	e.dapReplHistoryPrev()
	if got := dapReplGetInput(e.dap.replBuf); got != "second" {
		t.Errorf("prev2 input = %q, want second", got)
	}
	e.dapReplHistoryPrev()
	if got := dapReplGetInput(e.dap.replBuf); got != "first" {
		t.Errorf("prev3 input = %q, want first", got)
	}
	// At start; further prev should stay.
	e.dapReplHistoryPrev()
	if got := dapReplGetInput(e.dap.replBuf); got != "first" {
		t.Errorf("prev4 input = %q, want first (clamped)", got)
	}

	e.dapReplHistoryNext()
	if got := dapReplGetInput(e.dap.replBuf); got != "second" {
		t.Errorf("next1 input = %q, want second", got)
	}
	e.dapReplHistoryNext()
	if got := dapReplGetInput(e.dap.replBuf); got != "third" {
		t.Errorf("next2 input = %q", got)
	}
	// Past end should clear.
	e.dapReplHistoryNext()
	if got := dapReplGetInput(e.dap.replBuf); got != "" {
		t.Errorf("next-past-end input = %q, want empty", got)
	}
}

func TestDapReplHistoryEmpty(t *testing.T) {
	e := newDAPReplTestEditor(t)
	// No history; should be no-op without panic.
	e.dapReplHistoryPrev()
	e.dapReplHistoryNext()
}

func TestDapReplHistoryNoSession(t *testing.T) {
	e := newDAPTestEditor("")
	// e.dap is nil; should not panic.
	e.dapReplHistoryPrev()
	e.dapReplHistoryNext()
	e.dapReplSetInput("ignored")
}

// ---------------------------------------------------------------------------
// commonPrefixTwo / dapCollectVarNames
// ---------------------------------------------------------------------------

func TestCommonPrefixTwo(t *testing.T) {
	tests := []struct {
		a, b, want string
	}{
		{"foo", "foobar", "foo"},
		{"foobar", "foo", "foo"},
		{"abc", "abd", "ab"},
		{"x", "y", ""},
		{"", "abc", ""},
		{"abc", "", ""},
		{"abc", "abc", "abc"},
	}
	for _, tt := range tests {
		got := commonPrefixTwo(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("commonPrefixTwo(%q, %q) = %q, want %q", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestDapCollectVarNamesFlat(t *testing.T) {
	vars := []dapVariable{
		{name: "a"},
		{name: "b"},
	}
	got := dapCollectVarNames(vars)
	want := []string{"a", "b"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDapCollectVarNamesExpanded(t *testing.T) {
	vars := []dapVariable{
		{
			name:     "obj",
			expanded: true,
			children: []dapVariable{
				{name: "field1"},
				{name: "field2"},
			},
		},
		{
			name:     "obj2",
			expanded: false, // children skipped
			children: []dapVariable{
				{name: "skipped"},
			},
		},
	}
	got := dapCollectVarNames(vars)
	want := []string{"obj", "field1", "field2", "obj2"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// ---------------------------------------------------------------------------
// debugReplDispatch
// ---------------------------------------------------------------------------

func TestDebugReplDispatchTypeRune(t *testing.T) {
	e := newDAPReplTestEditor(t)
	if !e.debugReplDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'a'}) {
		t.Fatal("KeyRune 'a' not consumed")
	}
	if got := dapReplGetInput(e.dap.replBuf); got != "a" {
		t.Errorf("input after typing = %q", got)
	}
}

func TestDebugReplDispatchHomeEnd(t *testing.T) {
	e := newDAPReplTestEditor(t)
	e.debugReplDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'a'})
	e.debugReplDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'b'})
	// HOME jumps to inputStart.
	want := dapReplPromptPos(e.dap.replBuf)
	e.debugReplDispatch(terminal.KeyEvent{Key: tcell.KeyHome})
	if got := e.dap.replBuf.Point(); got != want {
		t.Errorf("HOME point = %d, want %d", got, want)
	}
	// END jumps to buffer end.
	e.debugReplDispatch(terminal.KeyEvent{Key: tcell.KeyEnd})
	if got := e.dap.replBuf.Point(); got != e.dap.replBuf.Len() {
		t.Errorf("END point = %d, want %d", got, e.dap.replBuf.Len())
	}
}

func TestDebugReplDispatchBackspaceAtPrompt(t *testing.T) {
	e := newDAPReplTestEditor(t)
	// At prompt, point == inputStart; backspace should be a no-op.
	before := e.dap.replBuf.String()
	e.debugReplDispatch(terminal.KeyEvent{Key: tcell.KeyBackspace})
	if got := e.dap.replBuf.String(); got != before {
		t.Errorf("backspace at prompt mutated buffer: %q → %q", before, got)
	}
}

func TestDebugReplDispatchTabCompletes(t *testing.T) {
	e := newDAPReplTestEditor(t)
	e.dap.locals = []dapVariable{
		{name: "foobar"},
		{name: "fizz"},
	}
	e.debugReplDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'f'})
	// Completion should extend "f" → "f" (common prefix unchanged) but not panic.
	e.debugReplDispatch(terminal.KeyEvent{Key: tcell.KeyTab})
	got := dapReplGetInput(e.dap.replBuf)
	if got != "f" {
		t.Errorf("after tab common-prefix = %q, want \"f\"", got)
	}

	// Now a unique prefix: complete to full name.
	e.dap.replBuf.SetReadOnly(false)
	e.dap.replBuf.Delete(dapReplPromptPos(e.dap.replBuf), e.dap.replBuf.Len())
	e.dap.replBuf.SetReadOnly(true)
	e.debugReplDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'f'})
	e.debugReplDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'o'})
	e.debugReplDispatch(terminal.KeyEvent{Key: tcell.KeyTab})
	if got := dapReplGetInput(e.dap.replBuf); got != "foobar" {
		t.Errorf("after tab unique = %q, want foobar", got)
	}
}

func TestDebugReplDispatchNoSession(t *testing.T) {
	e := newDAPTestEditor("")
	if e.debugReplDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'a'}) {
		t.Error("dispatch returned true with no session")
	}
}

func TestDebugReplDispatchHistoryKeys(t *testing.T) {
	e := newDAPReplTestEditor(t)
	e.dap.replHistory = []string{"first", "second"}
	e.dap.replHistoryIdx = len(e.dap.replHistory)
	if !e.debugReplDispatch(terminal.KeyEvent{Key: tcell.KeyUp}) {
		t.Fatal("Up not consumed")
	}
	if got := dapReplGetInput(e.dap.replBuf); got != "second" {
		t.Errorf("Up should recall last history entry, got %q", got)
	}
	if !e.debugReplDispatch(terminal.KeyEvent{Key: tcell.KeyDown}) {
		t.Fatal("Down not consumed")
	}
}

func TestDebugReplDispatchLeft(t *testing.T) {
	e := newDAPReplTestEditor(t)
	e.activeWin = e.windows[len(e.windows)-1] // the *Debug REPL* window
	e.debugReplDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'a'})
	before := e.dap.replBuf.Point()
	if !e.debugReplDispatch(terminal.KeyEvent{Key: tcell.KeyLeft}) {
		t.Fatal("Left not consumed")
	}
	if e.dap.replBuf.Point() != before-1 {
		t.Errorf("Left should move point back one, %d → %d", before, e.dap.replBuf.Point())
	}
}

func TestDebugReplDispatchEnterSubmits(t *testing.T) {
	e := newDAPReplTestEditor(t)
	e.debugReplDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'x'})
	if !e.debugReplDispatch(terminal.KeyEvent{Key: tcell.KeyEnter}) {
		t.Fatal("Enter not consumed")
	}
	// No client → submit notes there is no active session.
	if !strings.Contains(e.dap.replBuf.String(), "no active session") {
		t.Errorf("Enter with no client should note no active session, got %q", e.dap.replBuf.String())
	}
}
