package editor

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/lsp"
	"github.com/skybert/gomacs/internal/syntax"
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

func TestLspFlushDidChange_EarlyReturns(t *testing.T) {
	e := newLSPTestEditor("hello")
	b := buf(e)
	// No filename → returns.
	e.lspFlushDidChange(b)
	// No conn → returns.
	b.SetFilename("/tmp/c.go")
	b.SetMode("go")
	e.lspFlushDidChange(b)
}

// ---------------------------------------------------------------------------
// lspMaybeDidChange / lspFlushDidChange debounce behaviour
// ---------------------------------------------------------------------------

func TestLspSendDidChange_NoOpWhenBufferUnmodified(t *testing.T) {
	rec := &notifyRecorder{}
	e, _, _ := newDidChangeTestConn(t, rec, time.Now().Add(-time.Hour), nil)
	b := buf(e)

	// ModCount hasn't changed since the conn was set up: neither the
	// debounced nor the forced path should send anything.
	e.lspMaybeDidChange(b)
	e.lspFlushDidChange(b)
	time.Sleep(20 * time.Millisecond)
	if got := rec.count("textDocument/didChange"); got != 0 {
		t.Fatalf("expected no didChange for an unmodified buffer, got %d", got)
	}
}

func TestLspMaybeDidChange_DebouncesRapidEdits(t *testing.T) {
	orig := lspDidChangeDebounce
	lspDidChangeDebounce = 50 * time.Millisecond
	t.Cleanup(func() { lspDidChangeDebounce = orig })

	rec := &notifyRecorder{}
	// lastSent is far in the past so the first edit sends immediately.
	e, _, _ := newDidChangeTestConn(t, rec, time.Now().Add(-time.Hour), nil)
	b := buf(e)

	b.InsertString(b.Len(), "!")
	e.lspMaybeDidChange(b)
	waitForNotifyCount(t, rec, "textDocument/didChange", 1, time.Second)

	// A second edit hot on the heels of the first falls inside the debounce
	// window and must not trigger another send yet.
	b.InsertString(b.Len(), "!")
	e.lspMaybeDidChange(b)
	time.Sleep(15 * time.Millisecond)
	if got := rec.count("textDocument/didChange"); got != 1 {
		t.Fatalf("expected the second send to be debounced, got %d didChange notifications", got)
	}

	// Once the debounce window has elapsed, the same (still-dirty) buffer
	// sends on the next call.
	time.Sleep(60 * time.Millisecond)
	e.lspMaybeDidChange(b)
	waitForNotifyCount(t, rec, "textDocument/didChange", 2, time.Second)
}

func TestLspFlushDidChange_BypassesDebounceWindow(t *testing.T) {
	orig := lspDidChangeDebounce
	lspDidChangeDebounce = time.Hour // effectively "never" via the debounced path
	t.Cleanup(func() { lspDidChangeDebounce = orig })

	rec := &notifyRecorder{}
	e, _, _ := newDidChangeTestConn(t, rec, time.Now().Add(-time.Hour), nil)
	b := buf(e)

	b.InsertString(b.Len(), "!")
	e.lspMaybeDidChange(b)
	waitForNotifyCount(t, rec, "textDocument/didChange", 1, time.Second)

	b.InsertString(b.Len(), "!")
	e.lspMaybeDidChange(b) // still well inside the huge debounce window
	time.Sleep(15 * time.Millisecond)
	if got := rec.count("textDocument/didChange"); got != 1 {
		t.Fatalf("expected debounced send to be skipped, got %d", got)
	}

	e.lspFlushDidChange(b) // force: must send despite the debounce window
	waitForNotifyCount(t, rec, "textDocument/didChange", 2, time.Second)
}

func TestLspFlushDidChange_InitializesNilLastSentMap(t *testing.T) {
	rec := &notifyRecorder{}
	e, conn, uri := newDidChangeTestConn(t, rec, time.Now(), nil)
	// Simulate a conn built the old way (e.g. by an older test literal) with
	// no lastSent map at all; lspSendDidChange must not panic on a nil write.
	conn.lastSent = nil
	b := buf(e)
	b.InsertString(b.Len(), "!")

	e.lspFlushDidChange(b)
	waitForNotifyCount(t, rec, "textDocument/didChange", 1, time.Second)

	conn.filesMu.Lock()
	_, ok := conn.lastSent[uri]
	conn.filesMu.Unlock()
	if !ok {
		t.Fatal("expected lspSendDidChange to lazily initialize a nil lastSent map")
	}
}

// ---------------------------------------------------------------------------
// Force-flush before dependent LSP requests
// ---------------------------------------------------------------------------

func TestCmdLSPFindDefinition_FlushesPendingDidChange(t *testing.T) {
	orig := lspDidChangeDebounce
	lspDidChangeDebounce = time.Hour
	t.Cleanup(func() { lspDidChangeDebounce = orig })

	rec := &notifyRecorder{}
	e, _, _ := newDidChangeTestConn(t, rec, time.Now().Add(-time.Hour), nil)
	b := buf(e)
	b.InsertString(b.Len(), "!") // dirty; a plain debounced send would skip this

	e.cmdLSPFindDefinition()
	drainOne(t, e, "definition")

	waitForNotifyCount(t, rec, "textDocument/didChange", 1, time.Second)
}

func TestCmdLSPShowDoc_FlushesPendingDidChange(t *testing.T) {
	orig := lspDidChangeDebounce
	lspDidChangeDebounce = time.Hour
	t.Cleanup(func() { lspDidChangeDebounce = orig })

	rec := &notifyRecorder{}
	e, _, _ := newDidChangeTestConn(t, rec, time.Now().Add(-time.Hour), nil)
	b := buf(e)
	b.InsertString(b.Len(), "!")

	e.cmdLSPShowDoc()
	drainOne(t, e, "hover")

	waitForNotifyCount(t, rec, "textDocument/didChange", 1, time.Second)
}

func TestCmdLSPFindReferences_FlushesPendingDidChange(t *testing.T) {
	orig := lspDidChangeDebounce
	lspDidChangeDebounce = time.Hour
	t.Cleanup(func() { lspDidChangeDebounce = orig })

	rec := &notifyRecorder{}
	e, _, _ := newDidChangeTestConn(t, rec, time.Now().Add(-time.Hour), nil)
	b := buf(e)
	b.InsertString(b.Len(), "!")

	e.cmdLSPFindReferences()
	drainOne(t, e, "references")

	waitForNotifyCount(t, rec, "textDocument/didChange", 1, time.Second)
}

func TestLspMaybeHover_FlushesPendingDidChange(t *testing.T) {
	orig := lspDidChangeDebounce
	lspDidChangeDebounce = time.Hour
	t.Cleanup(func() { lspDidChangeDebounce = orig })

	rec := &notifyRecorder{}
	e, _, _ := newDidChangeTestConn(t, rec, time.Now().Add(-time.Hour), nil)
	b := buf(e)
	b.InsertString(b.Len(), "!")
	e.lastHoverFile = ""
	e.lastHoverPoint = -1
	e.messageTime = 0

	e.lspMaybeHover()
	waitForNotifyCount(t, rec, "textDocument/didChange", 1, time.Second)
	drainOne(t, e, "hover")
}

func TestLspDidSave_EarlyReturns(t *testing.T) {
	e := newLSPTestEditor("hello")
	b := buf(e)
	e.lspDidSave(b) // no filename
	b.SetFilename("/tmp/s.go")
	b.SetMode("go")
	e.lspDidSave(b) // no conn
}

// ---------------------------------------------------------------------------
// lspDidSave debounce bookkeeping
// ---------------------------------------------------------------------------

func TestLspDidSave_RecordsSendState(t *testing.T) {
	rec := &notifyRecorder{}
	e, conn, uri := newDidChangeTestConn(t, rec, time.Now().Add(-time.Hour), nil)
	b := buf(e)
	b.InsertString(b.Len(), "!")

	e.lspDidSave(b)
	waitForNotifyCount(t, rec, "textDocument/didSave", 1, time.Second)

	conn.filesMu.Lock()
	gotMod, tracked := conn.openFiles[uri]
	_, gotSent := conn.lastSent[uri]
	conn.filesMu.Unlock()
	if !tracked || gotMod != b.ModCount() {
		t.Fatalf("expected openFiles[uri] updated to %d after save, got %d (tracked=%v)", b.ModCount(), gotMod, tracked)
	}
	if !gotSent {
		t.Fatal("expected lastSent[uri] to be recorded after save")
	}

	// The buffer is unchanged since the save, so a following didChange call
	// must be a no-op — the server already has this exact content.
	e.lspMaybeDidChange(b)
	time.Sleep(15 * time.Millisecond)
	if got := rec.count("textDocument/didChange"); got != 0 {
		t.Fatalf("expected no redundant didChange right after save, got %d", got)
	}
}

func TestLspDidSave_NoBookkeepingWhenFileNotTracked(t *testing.T) {
	rec := &notifyRecorder{}
	e := newLSPTestEditor("hello")
	b := buf(e)
	path := filepath.Join(t.TempDir(), "b.go")
	b.SetFilename(path)
	b.SetMode("go")
	c, cleanup := fakeLSPServerWithNotify(t, func(string) any { return nil }, rec.record)
	t.Cleanup(cleanup)
	conn := &lspConn{client: c, isReady: true, openFiles: map[string]int{}, diagnostics: map[string][]lsp.Diagnostic{}}
	e.lspConns["go"] = conn

	e.lspDidSave(b)
	waitForNotifyCount(t, rec, "textDocument/didSave", 1, time.Second)

	uri := string(lsp.FileURI(path))
	conn.filesMu.Lock()
	_, tracked := conn.openFiles[uri]
	conn.filesMu.Unlock()
	if tracked {
		t.Fatal("lspDidSave should not start tracking a file that was never opened")
	}
}

func TestLspDidSave_InitializesNilLastSentMap(t *testing.T) {
	rec := &notifyRecorder{}
	e := newLSPTestEditor("hello")
	b := buf(e)
	path := filepath.Join(t.TempDir(), "c.go")
	b.SetFilename(path)
	b.SetMode("go")
	c, cleanup := fakeLSPServerWithNotify(t, func(string) any { return nil }, rec.record)
	t.Cleanup(cleanup)
	uri := string(lsp.FileURI(path))
	conn := &lspConn{
		client:      c,
		isReady:     true,
		openFiles:   map[string]int{uri: b.ModCount()}, // tracked, but lastSent is nil
		diagnostics: map[string][]lsp.Diagnostic{},
	}
	e.lspConns["go"] = conn

	e.lspDidSave(b) // must not panic writing to a nil lastSent map
	waitForNotifyCount(t, rec, "textDocument/didSave", 1, time.Second)

	conn.filesMu.Lock()
	_, ok := conn.lastSent[uri]
	conn.filesMu.Unlock()
	if !ok {
		t.Fatal("expected lspDidSave to lazily initialize a nil lastSent map")
	}
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

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

// benchDidChangeConn builds a ready lspConn (backed by a discarding fake LSP
// server) around a buffer prefilled with n characters, for benchmarking the
// cost of lspMaybeDidChange / lspFlushDidChange against a realistically sized
// file.
func benchDidChangeConn(b *testing.B, n int) (*Editor, *buffer.Buffer) {
	b.Helper()
	content := strings.Repeat("x", n)
	e := newLSPTestEditor(content)
	buf := e.ActiveBuffer()
	path := filepath.Join(b.TempDir(), "bench.go")
	buf.SetFilename(path)
	buf.SetMode("go")
	rec := &notifyRecorder{}
	c, cleanup := fakeLSPServerWithNotify(b, func(string) any { return nil }, rec.record)
	b.Cleanup(cleanup)
	uri := string(lsp.FileURI(path))
	conn := &lspConn{
		client:      c,
		isReady:     true,
		openFiles:   map[string]int{uri: buf.ModCount()},
		lastSent:    map[string]time.Time{uri: time.Now()},
		diagnostics: map[string][]lsp.Diagnostic{},
	}
	e.lspConns["go"] = conn
	return e, buf
}

// BenchmarkLspFlushDidChange_EveryKeystroke simulates the pre-fix behaviour:
// every simulated keystroke forces a full-document didChange send (an O(n)
// buf.String() copy, JSON marshal, and pipe write).  Each iteration inserts
// then removes one rune so the buffer size — and thus the send cost — stays
// constant across the run.
func BenchmarkLspFlushDidChange_EveryKeystroke(b *testing.B) {
	e, buf := benchDidChangeConn(b, 50_000)
	for range b.N {
		buf.InsertString(buf.Len(), "x")
		e.lspFlushDidChange(buf)
		buf.Delete(buf.Len()-1, 1)
	}
}

// BenchmarkLspMaybeDidChange_Debounced simulates the fixed, steady-state
// behaviour for a typing burst: the same edit pattern, but with the debounce
// window wide enough that no send actually happens during the run, so each
// call is just a dirty-check and a couple of map lookups instead of an O(n)
// copy.
func BenchmarkLspMaybeDidChange_Debounced(b *testing.B) {
	orig := lspDidChangeDebounce
	lspDidChangeDebounce = time.Hour // outlives the whole benchmark run
	defer func() { lspDidChangeDebounce = orig }()

	e, buf := benchDidChangeConn(b, 50_000)
	for range b.N {
		buf.InsertString(buf.Len(), "x")
		e.lspMaybeDidChange(buf)
		buf.Delete(buf.Len()-1, 1)
	}
}

// ---------------------------------------------------------------------------
// parseSingleLocation
// ---------------------------------------------------------------------------

func TestParseSingleLocationNull(t *testing.T) {
	loc, err := parseSingleLocation(json.RawMessage("null"))
	if err != nil || loc.URI != "" {
		t.Errorf("parseSingleLocation(null) = (%v, %v), want zero,nil", loc, err)
	}
	loc, err = parseSingleLocation(nil)
	if err != nil || loc.URI != "" {
		t.Errorf("parseSingleLocation(nil) = (%v, %v), want zero,nil", loc, err)
	}
}

func TestParseSingleLocationSingle(t *testing.T) {
	raw := json.RawMessage(`{"uri":"file:///tmp/foo.go","range":{"start":{"line":3,"character":2},"end":{"line":3,"character":7}}}`)
	loc, err := parseSingleLocation(raw)
	if err != nil {
		t.Fatalf("parseSingleLocation: %v", err)
	}
	if loc.URI != "file:///tmp/foo.go" {
		t.Errorf("URI = %q", loc.URI)
	}
	if loc.Range.Start.Line != 3 {
		t.Errorf("Start.Line = %d, want 3", loc.Range.Start.Line)
	}
}

func TestParseSingleLocationArray(t *testing.T) {
	raw := json.RawMessage(`[{"uri":"file:///a.go","range":{"start":{"line":1,"character":0},"end":{"line":1,"character":1}}},{"uri":"file:///b.go","range":{"start":{"line":2,"character":0},"end":{"line":2,"character":1}}}]`)
	loc, err := parseSingleLocation(raw)
	if err != nil {
		t.Fatalf("parseSingleLocation: %v", err)
	}
	if loc.URI != "file:///a.go" {
		t.Errorf("expected first location, got URI=%q", loc.URI)
	}
}

func TestParseSingleLocationEmptyArray(t *testing.T) {
	raw := json.RawMessage(`[]`)
	loc, err := parseSingleLocation(raw)
	if err != nil || loc.URI != "" {
		t.Errorf("parseSingleLocation([]) = (%v, %v), want zero,nil", loc, err)
	}
}

// ---------------------------------------------------------------------------
// parseLocations
// ---------------------------------------------------------------------------

func TestParseLocationsNull(t *testing.T) {
	if locs := parseLocations(json.RawMessage("null")); locs != nil {
		t.Errorf("parseLocations(null) = %v, want nil", locs)
	}
	if locs := parseLocations(nil); locs != nil {
		t.Errorf("parseLocations(nil) = %v, want nil", locs)
	}
}

func TestParseLocationsArray(t *testing.T) {
	raw := json.RawMessage(`[{"uri":"file:///a.go","range":{"start":{"line":1,"character":0},"end":{"line":1,"character":1}}},{"uri":"file:///b.go","range":{"start":{"line":2,"character":0},"end":{"line":2,"character":1}}}]`)
	locs := parseLocations(raw)
	if len(locs) != 2 {
		t.Fatalf("len = %d, want 2", len(locs))
	}
	if locs[0].URI != "file:///a.go" || locs[1].URI != "file:///b.go" {
		t.Errorf("URIs = %q, %q", locs[0].URI, locs[1].URI)
	}
}

func TestParseLocationsBadJSON(t *testing.T) {
	if locs := parseLocations(json.RawMessage(`not json`)); locs != nil {
		t.Errorf("expected nil for invalid JSON, got %v", locs)
	}
}

// ---------------------------------------------------------------------------
// extractHoverText
// ---------------------------------------------------------------------------

func TestExtractHoverTextNull(t *testing.T) {
	if got := extractHoverText(nil); got != "" {
		t.Errorf("extractHoverText(nil) = %q", got)
	}
	if got := extractHoverText(json.RawMessage("null")); got != "" {
		t.Errorf("extractHoverText(null) = %q", got)
	}
}

func TestExtractHoverTextMarkupContent(t *testing.T) {
	raw := json.RawMessage(`{"contents":{"kind":"markdown","value":"  hello world  "}}`)
	if got := extractHoverText(raw); got != "hello world" {
		t.Errorf("extractHoverText = %q, want %q", got, "hello world")
	}
}

func TestExtractHoverTextPlainString(t *testing.T) {
	raw := json.RawMessage(`{"contents":"  plain text  "}`)
	if got := extractHoverText(raw); got != "plain text" {
		t.Errorf("extractHoverText = %q, want %q", got, "plain text")
	}
}

func TestExtractHoverTextNoContents(t *testing.T) {
	raw := json.RawMessage(`{}`)
	if got := extractHoverText(raw); got != "" {
		t.Errorf("extractHoverText empty = %q", got)
	}
}

func TestExtractHoverTextBadJSON(t *testing.T) {
	if got := extractHoverText(json.RawMessage(`not json`)); got != "" {
		t.Errorf("extractHoverText invalid = %q", got)
	}
}

// ---------------------------------------------------------------------------
// wrapDocText
// ---------------------------------------------------------------------------

func TestWrapDocTextSimple(t *testing.T) {
	got := wrapDocText("hello world", 80)
	if len(got) != 1 || got[0] != "hello world" {
		t.Errorf("wrapDocText = %v, want [\"hello world\"]", got)
	}
}

func TestWrapDocTextWrapsLongLines(t *testing.T) {
	got := wrapDocText("aaaa bbbb cccc dddd", 10)
	// expect to wrap at word boundaries: "aaaa bbbb" (9 cols), "cccc dddd"
	if len(got) != 2 {
		t.Errorf("expected 2 lines, got %d: %v", len(got), got)
	}
}

func TestWrapDocTextSplitsOnNewlines(t *testing.T) {
	got := wrapDocText("first\nsecond\nthird", 80)
	want := []string{"first", "second", "third"}
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d: %v", len(got), len(want), got)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("line %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestWrapDocTextTrimsTrailingBlank(t *testing.T) {
	got := wrapDocText("hello\n\n\n", 80)
	if len(got) != 1 {
		t.Errorf("expected trailing blanks trimmed, got %v", got)
	}
}

func TestWrapDocTextEmpty(t *testing.T) {
	got := wrapDocText("", 80)
	if len(got) != 0 {
		t.Errorf("wrapDocText empty = %v, want []", got)
	}
}

// ---------------------------------------------------------------------------
// lspReadFileLine
// ---------------------------------------------------------------------------

func TestLspReadFileLine(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.txt")
	content := "line0\nline1\r\nline2\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := lspReadFileLine(p, 0); got != "line0" {
		t.Errorf("line 0 = %q, want %q", got, "line0")
	}
	if got := lspReadFileLine(p, 1); got != "line1" {
		t.Errorf("line 1 = %q, want %q (CR trimmed)", got, "line1")
	}
	if got := lspReadFileLine(p, 2); got != "line2" {
		t.Errorf("line 2 = %q, want %q", got, "line2")
	}
	if got := lspReadFileLine(p, 99); got != "" {
		t.Errorf("out of range = %q, want empty", got)
	}
}

func TestLspReadFileLineMissingFile(t *testing.T) {
	if got := lspReadFileLine("/nonexistent/file.xyzzy", 0); got != "" {
		t.Errorf("missing file = %q, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// findBufferByFilename
// ---------------------------------------------------------------------------

func TestFindBufferByFilename(t *testing.T) {
	e := newTestEditor("hello")
	buf(e).SetFilename("/tmp/a.go")
	if got := e.findBufferByFilename("/tmp/a.go"); got != buf(e) {
		t.Errorf("findBufferByFilename returned wrong buffer")
	}
	if got := e.findBufferByFilename("/nope"); got != nil {
		t.Errorf("expected nil for unknown filename, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// bufPointToLSP / lspPosToPoint
// ---------------------------------------------------------------------------

func TestBufPointToLSPSimple(t *testing.T) {
	e := newTestEditor("hello\nworld")
	b := buf(e)
	b.SetPoint(8) // 8 = "hello\nwo|rld"; line 2, col 2
	pos := e.bufPointToLSP(b)
	if pos.Line != 1 {
		t.Errorf("Line = %d, want 1 (0-based)", pos.Line)
	}
	if pos.Character != 2 {
		t.Errorf("Character = %d, want 2", pos.Character)
	}
}

func TestLspPosToPointRoundTrip(t *testing.T) {
	e := newTestEditor("abc\ndef\nghi")
	b := buf(e)
	for _, pt := range []int{0, 1, 4, 5, 7, 11} {
		b.SetPoint(pt)
		pos := e.bufPointToLSP(b)
		got := e.lspPosToPoint(b, pos)
		if got != pt {
			t.Errorf("round-trip pt=%d → pos=%+v → pt=%d", pt, pos, got)
		}
	}
}

func TestLspPosToPointPastEnd(t *testing.T) {
	e := newTestEditor("ab")
	b := buf(e)
	pos := lsp.Position{Line: 0, Character: 100}
	pt := e.lspPosToPoint(b, pos)
	if pt != b.Len() {
		t.Errorf("past-end pt = %d, want %d", pt, b.Len())
	}
}

// fakeLSPServer wires an lsp.Client to an in-process server goroutine that
// replies to each request using the responder (method → result JSON).  Returns
// the client and a cleanup func.  Accepts testing.TB so it can be reused from
// benchmarks as well as tests.
func fakeLSPServer(t testing.TB, responder func(method string) any) (*lsp.Client, func()) {
	t.Helper()
	return fakeLSPServerWithNotify(t, responder, nil)
}

// fakeLSPServerWithNotify behaves like fakeLSPServer, but additionally invokes
// onNotify (if non-nil) for every JSON-RPC notification (a message with no
// "id") the client sends, with the method name and raw body.  Used to assert
// on debounced textDocument/didChange traffic.
func fakeLSPServerWithNotify(t testing.TB, responder func(method string) any, onNotify func(method string, body []byte)) (*lsp.Client, func()) {
	t.Helper()
	// client.stdin (w1) → server reads (r1); server writes (w2) → client.stdout (r2).
	r1, w1, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	r2, w2, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		br := bufio.NewReader(r1)
		for {
			clen := 0
			for {
				line, e := br.ReadString('\n')
				if e != nil {
					return
				}
				line = strings.TrimRight(line, "\r\n")
				if line == "" {
					break
				}
				if v, ok := strings.CutPrefix(line, "Content-Length: "); ok {
					_, _ = fmt.Sscanf(v, "%d", &clen)
				}
			}
			if clen == 0 {
				continue
			}
			body := make([]byte, clen)
			if _, e := io.ReadFull(br, body); e != nil {
				return
			}
			var req struct {
				ID     *int   `json:"id"`
				Method string `json:"method"`
			}
			_ = json.Unmarshal(body, &req)
			if req.ID == nil {
				if onNotify != nil {
					onNotify(req.Method, body)
				}
				continue // notification — no reply
			}
			result := responder(req.Method)
			resp := map[string]any{"jsonrpc": "2.0", "id": *req.ID, "result": result}
			rb, _ := json.Marshal(resp)
			_, _ = fmt.Fprintf(w2, "Content-Length: %d\r\n\r\n%s", len(rb), rb)
		}
	}()

	c := lsp.NewConnClient(w1, r2)
	cleanup := func() { c.Close(); _ = r1.Close(); _ = w2.Close() }
	return c, cleanup
}

// notifyRecorder collects the JSON-RPC notification methods a fake LSP server
// received, for assertions about debounced textDocument/didChange traffic.
type notifyRecorder struct {
	mu      sync.Mutex
	methods []string
}

// record is passed as the onNotify callback to fakeLSPServerWithNotify.
func (r *notifyRecorder) record(method string, _ []byte) {
	r.mu.Lock()
	r.methods = append(r.methods, method)
	r.mu.Unlock()
}

// count returns how many notifications matching method have been recorded.
func (r *notifyRecorder) count(method string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, m := range r.methods {
		if m == method {
			n++
		}
	}
	return n
}

// waitForNotifyCount blocks until rec has recorded at least want notifications
// of method, or fails the test after timeout.  Notifications arrive on a
// background goroutine (the fake server's read loop), so tests must poll
// rather than check the count immediately.
func waitForNotifyCount(t testing.TB, rec *notifyRecorder, method string, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if rec.count(method) >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d %q notification(s), got %d", want, method, rec.count(method))
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// newDidChangeTestConn returns an editor with a single Go buffer backed by a
// ready lspConn whose openFiles/lastSent already track the buffer's initial
// ModCount, as if didOpen had already run.  lastSent is seeded to sentAt so
// tests can control how "stale" the debounce state starts out.  responder may
// be nil to have every request return a null result.
func newDidChangeTestConn(t testing.TB, rec *notifyRecorder, sentAt time.Time, responder func(string) any) (e *Editor, conn *lspConn, uri string) {
	t.Helper()
	e = newLSPTestEditor("hello")
	b := buf(e)
	path := filepath.Join(t.TempDir(), "a.go")
	b.SetFilename(path)
	b.SetMode("go")
	if responder == nil {
		responder = func(string) any { return nil }
	}
	c, cleanup := fakeLSPServerWithNotify(t, responder, rec.record)
	t.Cleanup(cleanup)
	uri = string(lsp.FileURI(path))
	conn = &lspConn{
		client:      c,
		isReady:     true,
		openFiles:   map[string]int{uri: b.ModCount()},
		lastSent:    map[string]time.Time{uri: sentAt},
		diagnostics: map[string][]lsp.Diagnostic{},
	}
	e.lspConns["go"] = conn
	return e, conn, uri
}

func drainOneLSPCb(t *testing.T, e *Editor) {
	t.Helper()
	select {
	case fn := <-e.lspCbs:
		fn()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for LSP callback")
	}
}

func newLSPConnEditor(t *testing.T, responder func(string) any) (*Editor, *lspConn) {
	t.Helper()
	e := newCapTestEditor("package main\n\nfunc main() {}\n")
	e.lspCbs = make(chan func(), 16)
	e.lspConns = make(map[string]*lspConn)
	e.autoRevertMtimes = make(map[*buffer.Buffer]time.Time)
	e.lspOpCancel = func() {}
	e.lspCompDelayCancel = func() {}
	b := e.ActiveBuffer()
	b.SetMode("go")
	b.SetFilename(filepath.Join(t.TempDir(), "main.go"))
	c, cleanup := fakeLSPServer(t, responder)
	t.Cleanup(cleanup)
	conn := &lspConn{client: c, isReady: true, openFiles: map[string]int{}}
	e.lspConns["go"] = conn
	return e, conn
}

func TestLspMaybeHover_ShowsMessage(t *testing.T) {
	e, _ := newLSPConnEditor(t, func(method string) any {
		if method == "textDocument/hover" {
			return map[string]any{"contents": map[string]any{"kind": "markdown", "value": "func main()"}}
		}
		return nil
	})
	e.ActiveBuffer().SetPoint(5)
	e.lspMaybeHover()
	drainOneLSPCb(t, e)
	if !strings.Contains(e.message, "func main()") {
		t.Errorf("expected hover text in message, got %q", e.message)
	}
}

func TestLspMaybeHover_NotReadyNoOp(t *testing.T) {
	e, conn := newLSPConnEditor(t, func(string) any { return nil })
	conn.isReady = false
	e.lspMaybeHover()
	select {
	case <-e.lspCbs:
		t.Fatal("hover should not run when conn is not ready")
	default:
	}
}

func TestCmdLSPFindDefinition_JumpsToLocation(t *testing.T) {
	destDir := t.TempDir()
	destPath := filepath.Join(destDir, "dest.txt")
	_ = os.WriteFile(destPath, []byte("line1\nline2\nTARGET\n"), 0o644)

	e, _ := newLSPConnEditor(t, func(method string) any {
		if method == "textDocument/definition" {
			return map[string]any{
				"uri": string(lsp.FileURI(destPath)),
				"range": map[string]any{
					"start": map[string]any{"line": 2, "character": 0},
					"end":   map[string]any{"line": 2, "character": 6},
				},
			}
		}
		return nil
	})
	e.cmdLSPFindDefinition()
	drainOneLSPCb(t, e)
	if e.ActiveBuffer().Filename() != destPath {
		t.Errorf("expected jump to %q, got %q", destPath, e.ActiveBuffer().Filename())
	}
}

func TestCmdLSPFindDefinition_NoLocation(t *testing.T) {
	e, _ := newLSPConnEditor(t, func(string) any { return nil }) // null result
	e.cmdLSPFindDefinition()
	drainOneLSPCb(t, e)
	if !strings.Contains(e.message, "No definition found") {
		t.Errorf("expected 'No definition found', got %q", e.message)
	}
}

func TestCmdLSPShowDoc_ShowsPopup(t *testing.T) {
	e, _ := newLSPConnEditor(t, func(method string) any {
		if method == "textDocument/hover" {
			return map[string]any{"contents": map[string]any{"kind": "markdown", "value": "documentation here"}}
		}
		return nil
	})
	e.cmdLSPShowDoc()
	drainOneLSPCb(t, e)
	if len(e.lspDocLines) == 0 {
		t.Error("expected doc popup lines after show-doc")
	}
}

func TestCmdLSPShowDoc_NoDoc(t *testing.T) {
	e, _ := newLSPConnEditor(t, func(string) any { return nil })
	e.cmdLSPShowDoc()
	drainOneLSPCb(t, e)
	if !strings.Contains(e.message, "No documentation found") {
		t.Errorf("expected 'No documentation found', got %q", e.message)
	}
}

func TestCmdLSPFindReferences_ShowsBuffer(t *testing.T) {
	dir := t.TempDir()
	refPath := filepath.Join(dir, "ref.txt")
	_ = os.WriteFile(refPath, []byte("alpha\nbeta\n"), 0o644)
	e, _ := newLSPConnEditor(t, func(method string) any {
		if method == "textDocument/references" {
			return []map[string]any{{
				"uri": string(lsp.FileURI(refPath)),
				"range": map[string]any{
					"start": map[string]any{"line": 0, "character": 0},
					"end":   map[string]any{"line": 0, "character": 1},
				},
			}}
		}
		return nil
	})
	e.cmdLSPFindReferences()
	drainOneLSPCb(t, e)
	if e.FindBuffer("*LSP References*") == nil {
		t.Error("expected *LSP References* buffer after find-references")
	}
}

func TestCmdLSPFindReferences_NoRefs(t *testing.T) {
	e, _ := newLSPConnEditor(t, func(string) any { return nil })
	e.cmdLSPFindReferences()
	drainOneLSPCb(t, e)
	if !strings.Contains(e.message, "No references found") {
		t.Errorf("expected 'No references found', got %q", e.message)
	}
}

// TestRenderLSPDocPopup_HighlightsCode checks the code API doc popup is syntax
// highlighted rather than drawn in one flat face, while keeping the popup's own
// background so it still reads as a popup.
func TestRenderLSPDocPopup_HighlightsCode(t *testing.T) {
	e := newCapTestEditor("package main\n")
	e.ActiveBuffer().SetMode("go")
	e.lspDocLines = []string{"func Open(name string) (*File, error)"}

	e.renderLSPDocPopup()

	// Find the row holding the doc text and collect the faces used on it.
	popupBg := syntax.FaceCandidate.Bg
	_, totalH := e.term.Size()
	foundRow := -1
	for row := range totalH {
		if ch, _ := e.term.CaptureCell(1, row); ch == 'f' {
			if ch2, _ := e.term.CaptureCell(2, row); ch2 == 'u' {
				foundRow = row
				break
			}
		}
	}
	if foundRow < 0 {
		t.Fatal("could not locate the doc popup text row")
	}

	fgs := make(map[string]bool)
	for col := 1; col <= len("func Open(name string) (*File, error)"); col++ {
		_, face := e.term.CaptureCell(col, foundRow)
		fgs[face.Fg] = true
		if face.Bg != popupBg {
			t.Errorf("col %d: popup background = %q, want %q", col, face.Bg, popupBg)
		}
	}
	if len(fgs) < 2 {
		t.Errorf("expected more than one foreground colour across the doc line, got %v", fgs)
	}
}
