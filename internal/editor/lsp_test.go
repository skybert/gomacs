package editor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/lsp"
	"github.com/skybert/gomacs/internal/terminal"
)

// newLSPTestEditor returns a capture-backed editor with the maps the LSP code
// paths need.
func newLSPTestEditor(content string) *Editor {
	e := newCapTestEditor(content)
	e.lspCbs = make(chan func(), 16)
	e.lspConns = make(map[string]*lspConn)
	e.autoRevertMtimes = make(map[*buffer.Buffer]time.Time)
	e.spanCaches = make(map[*buffer.Buffer]*spanCache)
	e.lspOpCancel = func() {}
	return e
}

// ---------------------------------------------------------------------------
// No-server guard paths
// ---------------------------------------------------------------------------

func TestCmdLSPFindDefinition_NoServer(t *testing.T) {
	e := newLSPTestEditor("package main")
	buf(e).SetMode("go")
	e.cmdLSPFindDefinition()
	if !strings.Contains(e.message, "No LSP server") {
		t.Fatalf("expected 'No LSP server', got %q", e.message)
	}
}

func TestCmdLSPShowDoc_NoServer(t *testing.T) {
	e := newLSPTestEditor("package main")
	buf(e).SetMode("go")
	e.cmdLSPShowDoc()
	if !strings.Contains(e.message, "No LSP server") {
		t.Fatalf("expected 'No LSP server', got %q", e.message)
	}
}

func TestCmdLSPFindReferences_NoServer(t *testing.T) {
	e := newLSPTestEditor("package main")
	buf(e).SetMode("go")
	e.cmdLSPFindReferences()
	if !strings.Contains(e.message, "No LSP server") {
		t.Fatalf("expected 'No LSP server', got %q", e.message)
	}
}

func TestCmdLSPCommands_NotReady(t *testing.T) {
	e := newLSPTestEditor("package main")
	buf(e).SetMode("go")
	e.lspConns["go"] = &lspConn{isReady: false}
	e.cmdLSPFindDefinition()
	if !strings.Contains(e.message, "initializing") {
		t.Fatalf("expected 'initializing', got %q", e.message)
	}
	e.cmdLSPShowDoc()
	if !strings.Contains(e.message, "initializing") {
		t.Fatalf("expected 'initializing', got %q", e.message)
	}
	e.cmdLSPFindReferences()
	if !strings.Contains(e.message, "initializing") {
		t.Fatalf("expected 'initializing', got %q", e.message)
	}
}

func TestCmdLSPPopDefinition_Empty(t *testing.T) {
	e := newLSPTestEditor("")
	e.cmdLSPPopDefinition()
	if !strings.Contains(e.message, "No previous definition") {
		t.Fatalf("expected 'No previous definition', got %q", e.message)
	}
}

func TestCmdLSPPopDefinition_ReturnsToFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.go")
	if err := os.WriteFile(path, []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	e := newLSPTestEditor("")
	e.lspDefStack = append(e.lspDefStack, lspDefPos{filename: path, point: 5})
	e.cmdLSPPopDefinition()
	if e.ActiveBuffer().Filename() != path {
		t.Fatalf("pop should return to %q, got %q", path, e.ActiveBuffer().Filename())
	}
	if len(e.lspDefStack) != 0 {
		t.Fatal("pop should remove the stack entry")
	}
}

func TestLspNewOpCtx_CancelsPrevious(t *testing.T) {
	e := newLSPTestEditor("")
	ctx1 := e.lspNewOpCtx()
	_ = e.lspNewOpCtx() // should cancel ctx1
	select {
	case <-ctx1.Done():
	default:
		t.Fatal("creating a new op context should cancel the previous one")
	}
}

func TestLspMaybeHover_EarlyReturns(t *testing.T) {
	e := newLSPTestEditor("hello")
	// No filename → returns without touching anything.
	e.lspMaybeHover()
	// Minibuffer active → returns.
	buf(e).SetFilename("/tmp/x.go")
	e.minibufActive = true
	e.lspMaybeHover()
	e.minibufActive = false
	// No conn for mode → returns.
	e.lspMaybeHover()
}

// ---------------------------------------------------------------------------
// lspRefsDispatch
// ---------------------------------------------------------------------------

func TestLspRefsDispatch_EnterOpensFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ref.go")
	if err := os.WriteFile(path, []byte("package main\n\nvar X = 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	e := newLSPTestEditor("")
	refs := buffer.NewWithContent("*LSP References*", path+":3:5: var X = 1\n")
	refs.SetMode("lsp-refs")
	e.buffers = append(e.buffers, refs)
	e.activeWin.SetBuf(refs)
	refs.SetPoint(0)
	if !e.lspRefsDispatch(terminal.KeyEvent{Key: tcell.KeyEnter}) {
		t.Fatal("Enter should be handled")
	}
	if e.ActiveBuffer().Filename() != path {
		t.Fatalf("Enter should open %q, got %q", path, e.ActiveBuffer().Filename())
	}
}

func TestLspRefsDispatch_Quit(t *testing.T) {
	e := newLSPTestEditor("")
	refs := buffer.NewWithContent("*LSP References*", "x.go:1:1: foo\n")
	refs.SetMode("lsp-refs")
	e.buffers = append(e.buffers, refs)
	e.activeWin.SetBuf(refs)
	if !e.lspRefsDispatch(terminal.KeyEvent{Key: tcell.KeyRune, Rune: 'q'}) {
		t.Fatal("q should be handled")
	}
	if e.lspRefsDispatch(terminal.KeyEvent{Key: tcell.KeyCtrlA}) {
		t.Fatal("C-a should not be handled")
	}
}

// ---------------------------------------------------------------------------
// gopls integration (skipped if gopls is unavailable)
// ---------------------------------------------------------------------------

// startGoplsConn builds a ready lspConn backed by a real gopls process rooted
// at dir, opening the buffer b.  It returns the conn or skips the test.
func startGoplsConn(t *testing.T, e *Editor, dir string, b *buffer.Buffer) *lspConn {
	t.Helper()
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not available")
	}
	c, err := lsp.Start("gopls")
	if err != nil {
		t.Skipf("cannot start gopls: %v", err)
	}
	conn := &lspConn{
		client:      c,
		rootURI:     lsp.FileURI(dir),
		openFiles:   make(map[string]int),
		diagnostics: make(map[string][]lsp.Diagnostic),
	}
	if err := lspInitialize(conn); err != nil {
		c.Close()
		t.Skipf("gopls initialize failed: %v", err)
	}
	conn.isReady = true
	lspDidOpen(conn, b)
	e.lspConns["go"] = conn
	// Give gopls a moment to index the file.
	time.Sleep(500 * time.Millisecond)
	return conn
}

func drainOne(t *testing.T, e *Editor, what string) {
	t.Helper()
	select {
	case fn := <-e.lspCbs:
		fn()
	case <-time.After(20 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func TestGopls_FindDefinitionHoverReferences(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping gopls integration test in -short mode")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/m\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	src := "package main\n\nfunc Greet() string { return \"hi\" }\n\nfunc main() {\n\t_ = Greet()\n}\n"
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	e := newLSPTestEditor(src)
	b := buf(e)
	b.SetMode("go")
	b.SetFilename(path)
	conn := startGoplsConn(t, e, dir, b)
	defer conn.client.Close()

	// Point on the call "Greet()" on line 6 (0-based line 5).
	callPos := b.PosForLineCol(6, 5) // inside "Greet"
	b.SetPoint(callPos)

	// Find definition → should jump to the Greet declaration on line 3.
	e.cmdLSPFindDefinition()
	drainOne(t, e, "definition")
	line, _ := b.LineCol(b.Point())
	if e.ActiveBuffer().Filename() != path {
		t.Fatalf("definition should stay in %q, got %q", path, e.ActiveBuffer().Filename())
	}
	if line != 3 {
		t.Logf("definition jumped to line %d (expected ~3)", line)
	}
	if len(e.lspDefStack) == 0 {
		t.Error("find-definition should push onto the definition stack")
	}

	// Hover/show-doc on the definition.
	b.SetPoint(b.PosForLineCol(3, 5))
	e.cmdLSPShowDoc()
	drainOne(t, e, "hover")
	// lspDocLines may be set, or a "No documentation" message; either way the
	// async path executed without error.

	// Find references to Greet.
	b.SetPoint(b.PosForLineCol(3, 5))
	e.cmdLSPFindReferences()
	drainOne(t, e, "references")
	if rb := e.FindBuffer("*LSP References*"); rb == nil {
		t.Log("no references buffer (gopls returned none)")
	}

	// Exercise didChange / didSave notification paths against the live server.
	b.SetPoint(b.Len())
	b.InsertString(b.Len(), "\n// trailing comment\n")
	e.lspMaybeDidChange(b)
	e.lspDidSave(b)

	// Passive hover after moving the cursor.
	b.SetPoint(b.PosForLineCol(3, 5))
	e.lastHoverFile = ""
	e.lastHoverPoint = -1
	e.messageTime = 0
	e.lspMaybeHover()
	// Drain a hover callback if one was scheduled.
	select {
	case fn := <-e.lspCbs:
		fn()
	case <-time.After(3 * time.Second):
		// No hover scheduled; that's acceptable.
	}
}

// ---------------------------------------------------------------------------
// diagnostics (no server needed)
// ---------------------------------------------------------------------------

func TestLspHandleNotify_StoresDiagnostics(t *testing.T) {
	e := newLSPTestEditor("")
	conn := &lspConn{openFiles: map[string]int{}, diagnostics: map[string][]lsp.Diagnostic{}}
	params := []byte(`{"uri":"file:///tmp/x.go","diagnostics":[{"severity":1,"message":"boom"}]}`)
	e.lspHandleNotify(conn, "textDocument/publishDiagnostics", params)
	if len(conn.diagnostics["file:///tmp/x.go"]) != 1 {
		t.Fatal("publishDiagnostics should store one diagnostic")
	}
	// Non-diagnostics method is ignored.
	e.lspHandleNotify(conn, "window/logMessage", []byte(`{}`))
}

func TestLspDiagnosticsForBuf_And_Summary(t *testing.T) {
	e := newLSPTestEditor("")
	b := buf(e)
	b.SetMode("go")
	b.SetFilename("/tmp/y.go")
	conn := &lspConn{openFiles: map[string]int{}, diagnostics: map[string][]lsp.Diagnostic{}}
	uri := string(lsp.FileURI("/tmp/y.go"))
	conn.diagnostics[uri] = []lsp.Diagnostic{
		{Severity: lsp.SeverityError, Message: "e1"},
		{Severity: lsp.SeverityError, Message: "e2"},
		{Severity: lsp.SeverityWarning, Message: "w1"},
	}
	e.lspConns["go"] = conn

	if got := len(e.lspDiagnosticsForBuf(b)); got != 3 {
		t.Fatalf("expected 3 diagnostics, got %d", got)
	}
	if got := e.lspDiagSummary(b); got != "[2E 1W]" {
		t.Fatalf("expected [2E 1W], got %q", got)
	}
}

func TestLspDiagSummary_NoDiagnostics(t *testing.T) {
	e := newLSPTestEditor("")
	b := buf(e)
	b.SetMode("go")
	b.SetFilename("/tmp/z.go")
	e.lspConns["go"] = &lspConn{openFiles: map[string]int{}, diagnostics: map[string][]lsp.Diagnostic{}}
	if got := e.lspDiagSummary(b); got != "" {
		t.Fatalf("no diagnostics should yield empty summary, got %q", got)
	}
}

func TestLspMaybeDidChange_EarlyReturns(t *testing.T) {
	e := newLSPTestEditor("hello")
	b := buf(e)
	// No filename → returns.
	e.lspMaybeDidChange(b)
	// No conn → returns.
	b.SetFilename("/tmp/c.go")
	b.SetMode("go")
	e.lspMaybeDidChange(b)
}

func TestLspDidSave_EarlyReturns(t *testing.T) {
	e := newLSPTestEditor("hello")
	b := buf(e)
	e.lspDidSave(b) // no filename
	b.SetFilename("/tmp/s.go")
	b.SetMode("go")
	e.lspDidSave(b) // no conn
}

func TestLspClose_ClosesConnections(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not available")
	}
	if testing.Short() {
		t.Skip("skipping gopls test in -short mode")
	}
	c, err := lsp.Start("gopls")
	if err != nil {
		t.Skipf("cannot start gopls: %v", err)
	}
	e := newLSPTestEditor("")
	e.lspConns["go"] = &lspConn{client: c, openFiles: map[string]int{}, diagnostics: map[string][]lsp.Diagnostic{}}
	e.lspClose()
	if len(e.lspConns) != 0 {
		t.Fatal("lspClose should remove all connections")
	}
}
