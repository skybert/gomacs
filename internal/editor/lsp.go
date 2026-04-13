package editor

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/lsp"
	"github.com/skybert/gomacs/internal/syntax"
	"github.com/skybert/gomacs/internal/terminal"
)

// lspConn holds a running LSP server connection for one language mode.
type lspConn struct {
	client  *lsp.Client
	rootURI string
	isReady bool // true after initialize handshake completes

	filesMu   sync.Mutex
	openFiles map[string]int // uri → last-sent modCount

	diagMu      sync.RWMutex
	diagnostics map[string][]lsp.Diagnostic // uri → diagnostics
}

// lspDefPos is one entry on the definition jump stack (for M-,).
type lspDefPos struct {
	filename string
	point    int
}

// ---- async helper ----------------------------------------------------------

// lspAsync runs work in a goroutine.  When work returns a callback fn (which
// may be nil), fn is sent to e.lspCbs and the event loop is woken via
// PostWakeup so the callback runs on the main goroutine.
func (e *Editor) lspAsync(work func() func()) {
	go func() {
		fn := work()
		if fn != nil {
			e.lspCbs <- fn
			e.term.PostWakeup()
		}
	}()
}

// lspNewOpCtx creates a fresh cancellable context for a user-initiated LSP
// operation, cancelling any previous one.
func (e *Editor) lspNewOpCtx() context.Context {
	e.lspOpCancel()
	ctx, cancel := context.WithCancel(context.Background())
	e.lspOpCancel = cancel
	return ctx
}

// ---- lifecycle -------------------------------------------------------------

// lspActivate ensures an LSP server is running for buf's mode, then sends
// textDocument/didOpen for buf.  The blocking initialize handshake runs in a
// goroutine so the UI is never frozen.
func (e *Editor) lspActivate(buf *buffer.Buffer) {
	info := langModeByName(buf.Mode())
	if info == nil || len(info.lspCmd) == 0 {
		return
	}

	conn := e.lspConns[buf.Mode()]
	if conn == nil {
		root := findProjectRoot(buf.Filename(), info.rootMarkers)
		c, err := lsp.Start(info.lspCmd[0], info.lspCmd[1:]...)
		if err != nil {
			e.Message("LSP: cannot start %s: %v", info.lspCmd[0], err)
			return
		}
		conn = &lspConn{
			client:      c,
			rootURI:     lsp.FileURI(root),
			openFiles:   make(map[string]int),
			diagnostics: make(map[string][]lsp.Diagnostic),
		}
		e.lspConns[buf.Mode()] = conn

		conn.client.SetNotifyHandler(func(method string, params json.RawMessage) {
			// Diagnostics arrive asynchronously; store them and wake the UI.
			e.lspHandleNotify(conn, method, params)
			e.term.PostWakeup()
		})

		// Run the blocking initialize handshake in a goroutine.
		mode := buf.Mode()
		filename := buf.Filename()
		connCopy := conn
		e.lspAsync(func() func() {
			if err := lspInitialize(connCopy); err != nil {
				return func() {
					e.Message("LSP: initialize failed: %v", err)
					delete(e.lspConns, mode)
					connCopy.client.Close()
				}
			}
			// Open the file that triggered activation.
			return func() {
				connCopy.isReady = true
				if b := e.findBufferByFilename(filename); b != nil {
					lspDidOpen(connCopy, b)
				}
			}
		})
		return
	}

	lspDidOpen(conn, buf)
}

// lspClose shuts down all LSP connections.  Called when the editor exits.
func (e *Editor) lspClose() {
	for mode, conn := range e.lspConns {
		conn.client.Close()
		delete(e.lspConns, mode)
	}
}

// lspMaybeDidChange sends textDocument/didChange if buf has been modified
// since the last send.  Called from Redraw so diagnostics stay fresh.
func (e *Editor) lspMaybeDidChange(buf *buffer.Buffer) {
	if buf.Filename() == "" {
		return
	}
	conn := e.lspConns[buf.Mode()]
	if conn == nil {
		return
	}
	uri := lsp.FileURI(buf.Filename())
	conn.filesMu.Lock()
	lastMod, open := conn.openFiles[uri]
	conn.filesMu.Unlock()
	if !open || lastMod == buf.ModCount() {
		return
	}
	conn.filesMu.Lock()
	conn.openFiles[uri] = buf.ModCount()
	conn.filesMu.Unlock()
	_ = conn.client.Notify("textDocument/didChange", map[string]any{
		"textDocument": map[string]any{
			"uri":     uri,
			"version": buf.ModCount(),
		},
		"contentChanges": []map[string]any{
			{"text": buf.String()},
		},
	})
}

// lspDidSave sends textDocument/didSave for buf.  Called from cmdSaveBuffer.
func (e *Editor) lspDidSave(buf *buffer.Buffer) {
	if buf.Filename() == "" {
		return
	}
	conn := e.lspConns[buf.Mode()]
	if conn == nil {
		return
	}
	uri := lsp.FileURI(buf.Filename())
	_ = conn.client.Notify("textDocument/didSave", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"text":         buf.String(),
	})
}

// ---- notifications from server --------------------------------------------

func (e *Editor) lspHandleNotify(conn *lspConn, method string, params json.RawMessage) {
	if method != "textDocument/publishDiagnostics" {
		return
	}
	var p struct {
		URI         string           `json:"uri"`
		Diagnostics []lsp.Diagnostic `json:"diagnostics"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return
	}
	conn.diagMu.Lock()
	conn.diagnostics[p.URI] = p.Diagnostics
	conn.diagMu.Unlock()
}

// lspDiagnosticsForBuf returns the current diagnostics for buf, or nil.
func (e *Editor) lspDiagnosticsForBuf(buf *buffer.Buffer) []lsp.Diagnostic {
	if buf.Filename() == "" {
		return nil
	}
	conn := e.lspConns[buf.Mode()]
	if conn == nil {
		return nil
	}
	uri := lsp.FileURI(buf.Filename())
	conn.diagMu.RLock()
	diags := conn.diagnostics[uri]
	conn.diagMu.RUnlock()
	return diags
}

// lspDiagSummary returns a short modeline tag like "[2E 1W]", or "".
func (e *Editor) lspDiagSummary(buf *buffer.Buffer) string {
	diags := e.lspDiagnosticsForBuf(buf)
	if len(diags) == 0 {
		return ""
	}
	errs, warns := 0, 0
	for _, d := range diags {
		switch d.Severity {
		case lsp.SeverityError:
			errs++
		case lsp.SeverityWarning:
			warns++
		}
	}
	parts := []string{}
	if errs > 0 {
		parts = append(parts, fmt.Sprintf("%dE", errs))
	}
	if warns > 0 {
		parts = append(parts, fmt.Sprintf("%dW", warns))
	}
	if len(parts) == 0 {
		return ""
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// ---- editor commands -------------------------------------------------------

// cmdLSPFindDefinition jumps to the definition of the symbol under point (M-.).
// The LSP call runs in a goroutine; C-g cancels it.
func (e *Editor) cmdLSPFindDefinition() {
	e.clearArg()
	buf := e.ActiveBuffer()
	conn := e.lspConns[buf.Mode()]
	if conn == nil {
		e.Message("No LSP server for mode %q", buf.Mode())
		return
	}
	if !conn.isReady {
		e.Message("LSP server is initializing, please wait…")
		return
	}
	ctx := e.lspNewOpCtx()
	pos := e.bufPointToLSP(buf)
	uri := lsp.FileURI(buf.Filename())
	fromFile := buf.Filename()
	fromPoint := buf.Point()
	e.Message("Finding definition…")
	e.lspAsync(func() func() {
		result, err := conn.client.CallCtx(ctx, "textDocument/definition", map[string]any{
			"textDocument": map[string]any{"uri": uri},
			"position":     pos,
		})
		if err != nil {
			if ctx.Err() != nil {
				return func() { e.Message("Cancelled") }
			}
			return func() { e.Message("lsp-find-definition: %v", err) }
		}
		loc, _ := parseSingleLocation(result)
		if loc.URI == "" {
			return func() { e.Message("No definition found") }
		}
		return func() {
			e.lspDefStack = append(e.lspDefStack, lspDefPos{filename: fromFile, point: fromPoint})
			destPath := lsp.PathFromURI(loc.URI)
			destBuf, loadErr := e.loadFile(destPath)
			if loadErr != nil {
				e.Message("lsp-find-definition: %v", loadErr)
				return
			}
			e.activeWin.SetBuf(destBuf)
			pt := e.lspPosToPoint(destBuf, loc.Range.Start)
			destBuf.SetPoint(pt)
			e.activeWin.SetPoint(pt)
			e.syncWindowPoint(e.activeWin)
			e.activeWin.EnsurePointVisible()
		}
	})
}

// cmdLSPPopDefinition pops the definition stack and returns to the previous
// position (M-,).
func (e *Editor) cmdLSPPopDefinition() {
	e.clearArg()
	if len(e.lspDefStack) == 0 {
		e.Message("No previous definition position")
		return
	}
	top := e.lspDefStack[len(e.lspDefStack)-1]
	e.lspDefStack = e.lspDefStack[:len(e.lspDefStack)-1]

	var destBuf *buffer.Buffer
	for _, b := range e.buffers {
		if b.Filename() == top.filename {
			destBuf = b
			break
		}
	}
	if destBuf == nil {
		var err error
		destBuf, err = e.loadFile(top.filename)
		if err != nil {
			e.Message("lsp-pop-definition: %v", err)
			return
		}
	}
	e.activeWin.SetBuf(destBuf)
	destBuf.SetPoint(top.point)
	e.activeWin.SetPoint(top.point)
}

// cmdLSPShowDoc shows documentation for the symbol under point (C-c h).
// The LSP call runs in a goroutine; C-g cancels it.
// The result is shown in a floating popup that dismisses on any key.
func (e *Editor) cmdLSPShowDoc() {
	e.clearArg()
	buf := e.ActiveBuffer()
	conn := e.lspConns[buf.Mode()]
	if conn == nil {
		e.Message("No LSP server for mode %q", buf.Mode())
		return
	}
	if !conn.isReady {
		e.Message("LSP server is initializing, please wait…")
		return
	}
	ctx := e.lspNewOpCtx()
	pos := e.bufPointToLSP(buf)
	uri := lsp.FileURI(buf.Filename())
	e.Message("Fetching documentation…")
	e.lspAsync(func() func() {
		result, err := conn.client.CallCtx(ctx, "textDocument/hover", map[string]any{
			"textDocument": map[string]any{"uri": uri},
			"position":     pos,
		})
		if err != nil {
			if ctx.Err() != nil {
				return func() { e.Message("Cancelled") }
			}
			return func() { e.Message("lsp-show-doc: %v", err) }
		}
		if result == nil || string(result) == "null" {
			return func() { e.Message("No documentation found") }
		}
		text := extractHoverText(result)
		if text == "" {
			return func() { e.Message("No documentation found") }
		}
		return func() {
			e.message = ""
			e.lspDocLines = wrapDocText(text, 70)
			e.Redraw()
		}
	})
}

// cmdLSPFindReferences finds all references to the symbol under point (M-?).
// Results are displayed in a *LSP References* buffer; Enter jumps to a location.
func (e *Editor) cmdLSPFindReferences() {
	e.clearArg()
	buf := e.ActiveBuffer()
	conn := e.lspConns[buf.Mode()]
	if conn == nil {
		e.Message("No LSP server for mode %q", buf.Mode())
		return
	}
	if !conn.isReady {
		e.Message("LSP server is initializing, please wait…")
		return
	}
	ctx := e.lspNewOpCtx()
	pos := e.bufPointToLSP(buf)
	uri := lsp.FileURI(buf.Filename())
	e.Message("Finding references…")
	e.lspAsync(func() func() {
		result, err := conn.client.CallCtx(ctx, "textDocument/references", map[string]any{
			"textDocument": map[string]any{"uri": uri},
			"position":     pos,
			"context":      map[string]any{"includeDeclaration": true},
		})
		if err != nil {
			if ctx.Err() != nil {
				return func() { e.Message("Cancelled") }
			}
			return func() { e.Message("lsp-find-references: %v", err) }
		}
		locs := parseLocations(result)
		if len(locs) == 0 {
			return func() { e.Message("No references found") }
		}
		// Build display text: "absfile:lineNum:col: lineText"
		var sb strings.Builder
		for _, loc := range locs {
			path := lsp.PathFromURI(loc.URI)
			lineNum := loc.Range.Start.Line + 1  // 1-based
			col := loc.Range.Start.Character + 1 // 1-based for display
			lineText := lspReadFileLine(path, loc.Range.Start.Line)
			fmt.Fprintf(&sb, "%s:%d:%d: %s\n", path, lineNum, col, lineText)
		}
		text := sb.String()
		return func() {
			e.vcShowOutput("*LSP References*", text, "lsp-refs")
			e.Message("%d reference(s) found", len(locs))
		}
	})
}

// lspRefsDispatch handles key events in a *LSP References* buffer.
// Enter jumps to the reference on the current line; q quits to the previous buffer.
func (e *Editor) lspRefsDispatch(ke terminal.KeyEvent) bool {
	if ke.Key != tcell.KeyRune && ke.Key != tcell.KeyEnter {
		return false
	}

	buf := e.ActiveBuffer()

	if ke.Key == tcell.KeyRune && ke.Rune == 'q' {
		e.vcQuit("lsp-refs")
		return true
	}

	if ke.Key != tcell.KeyEnter {
		return false
	}

	pt := buf.Point()
	bol := buf.BeginningOfLine(pt)
	eol := buf.EndOfLine(pt)
	line := buf.Substring(bol, eol)
	if line == "" {
		return true
	}

	// Format: /abs/path.go:lineNum:col: content
	// Split into at most 4 parts so the content after col is preserved.
	parts := strings.SplitN(line, ":", 4)
	if len(parts) < 3 {
		return true
	}
	absPath := parts[0]
	lineNum, err := strconv.Atoi(parts[1])
	if err != nil || lineNum < 1 {
		return true
	}
	col, _ := strconv.Atoi(parts[2])
	if col < 1 {
		col = 1
	}

	b, loadErr := e.loadFile(absPath)
	if loadErr != nil {
		e.Message("Cannot open %s: %v", absPath, loadErr)
		return true
	}
	e.activeWin.SetBuf(b)
	// col is 1-based; PosForLineCol takes a 0-based column.
	pos := b.PosForLineCol(lineNum, col-1)
	b.SetPoint(pos)
	e.activeWin.SetPoint(pos)
	e.syncWindowPoint(e.activeWin)
	e.activeWin.EnsurePointVisible()
	return true
}

// lspReadFileLine returns the trimmed text of 0-based line lineIdx in the file
// at path.  Used while building the *LSP References* buffer.
func lspReadFileLine(path string, lineIdx int) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for i := 0; sc.Scan(); i++ {
		if i == lineIdx {
			return strings.TrimRight(sc.Text(), "\r")
		}
	}
	return ""
}

// lspMaybeHover passively shows the first line of hover documentation in the
// user command.  It is called from Run() after each Redraw().
func (e *Editor) lspMaybeHover() {
	buf := e.ActiveBuffer()
	if buf.Filename() == "" || e.minibufActive || e.isearching {
		return
	}
	conn := e.lspConns[buf.Mode()]
	if conn == nil || !conn.isReady || e.hoverInflight {
		return
	}
	// Only trigger if cursor moved since last hover.
	if buf.Filename() == e.lastHoverFile && buf.Point() == e.lastHoverPoint {
		return
	}
	// Don't clobber a fresh user message.
	if time.Now().UnixNano()-e.messageTime < 2e9 {
		return
	}
	e.lastHoverFile = buf.Filename()
	e.lastHoverPoint = buf.Point()
	e.hoverInflight = true
	pos := e.bufPointToLSP(buf)
	uri := lsp.FileURI(buf.Filename())
	ctx := context.Background()
	e.lspAsync(func() func() {
		result, err := conn.client.CallCtx(ctx, "textDocument/hover", map[string]any{
			"textDocument": map[string]any{"uri": uri},
			"position":     pos,
		})
		return func() {
			e.hoverInflight = false
			if err != nil || result == nil {
				return
			}
			text := extractHoverText(result)
			if text == "" {
				return
			}
			// Only show if no newer message was set.
			if time.Now().UnixNano()-e.messageTime < 2e9 {
				return
			}
			firstLine := strings.SplitN(text, "\n", 2)[0]
			if len(firstLine) > 120 {
				firstLine = firstLine[:117] + "..."
			}
			e.Message("%s", firstLine)
		}
	})
}

// findBufferByFilename returns the first buffer with the given filename, or nil.
func (e *Editor) findBufferByFilename(filename string) *buffer.Buffer {
	for _, b := range e.buffers {
		if b.Filename() == filename {
			return b
		}
	}
	return nil
}

// ---- position helpers -------------------------------------------------------

// bufPointToLSP converts the buffer's current point to an LSP Position.
func (e *Editor) bufPointToLSP(buf *buffer.Buffer) lsp.Position {
	pt := buf.Point()
	line, runeCol := buf.LineCol(pt)
	// LineCol returns 1-based line, 0-based col; LSP needs 0-based line.
	bol := buf.BeginningOfLine(pt)
	lineText := buf.Substring(bol, bol+runeCol)
	return lsp.Position{
		Line:      line - 1,
		Character: lsp.UTF16Offset(lineText, runeCol),
	}
}

// lspPosToPoint converts an LSP Position to a rune-index buffer point.
func (e *Editor) lspPosToPoint(buf *buffer.Buffer, pos lsp.Position) int {
	pt := buf.PosForLineCol(pos.Line+1, 0)
	// Walk forward pos.Character UTF-16 code units.
	utf16 := 0
	for utf16 < pos.Character && pt < buf.Len() {
		r := buf.RuneAt(pt)
		if r >= 0x10000 {
			utf16 += 2
		} else {
			utf16++
		}
		pt++
	}
	return pt
}

// ---- internal helpers -------------------------------------------------------

func lspInitialize(conn *lspConn) error {
	result, err := conn.client.Call("initialize", map[string]any{
		"processId": os.Getpid(),
		"rootUri":   conn.rootURI,
		"workspaceFolders": []map[string]any{
			{"uri": conn.rootURI, "name": filepath.Base(lsp.PathFromURI(conn.rootURI))},
		},
		"capabilities": map[string]any{
			"workspace": map[string]any{
				"workspaceFolders": true,
			},
			"textDocument": map[string]any{
				"synchronization": map[string]any{
					"dynamicRegistration": false,
					"willSave":            false,
					"didSave":             true,
					"willSaveWaitUntil":   false,
				},
				"publishDiagnostics": map[string]any{},
				"hover": map[string]any{
					"contentFormat": []string{"plaintext", "markdown"},
				},
				"definition": map[string]any{},
				"references": map[string]any{},
				"completion": map[string]any{
					"completionItem": map[string]any{
						"snippetSupport": false,
						"insertTextModeSupport": map[string]any{
							"valueSet": []int{1, 2},
						},
					},
				},
			},
		},
		"clientInfo": map[string]any{"name": "gomacs", "version": "0.1"},
	})
	if err != nil {
		return err
	}
	_ = result // server capabilities (ignored for now)
	return conn.client.Notify("initialized", map[string]any{})
}

func lspDidOpen(conn *lspConn, buf *buffer.Buffer) {
	uri := lsp.FileURI(buf.Filename())
	conn.filesMu.Lock()
	if _, already := conn.openFiles[uri]; already {
		conn.filesMu.Unlock()
		return
	}
	conn.openFiles[uri] = buf.ModCount()
	conn.filesMu.Unlock()

	langID := buf.Mode()
	_ = conn.client.Notify("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri":        uri,
			"languageId": langID,
			"version":    buf.ModCount(),
			"text":       buf.String(),
		},
	})
}

// findProjectRoot walks upward from dir looking for any of the marker files.
// Falls back to dir itself if none are found.
func findProjectRoot(filePath string, markers []string) string {
	dir := filepath.Dir(filePath)
	if !filepath.IsAbs(dir) {
		if abs, err := filepath.Abs(dir); err == nil {
			dir = abs
		}
	}
	for {
		for _, m := range markers {
			if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return filepath.Dir(filePath)
}

// parseSingleLocation extracts the first Location from a definition response,
// which may be null, a Location, or a []Location.
func parseSingleLocation(raw json.RawMessage) (lsp.Location, error) {
	if raw == nil || string(raw) == "null" {
		return lsp.Location{}, nil
	}
	// Try single Location.
	var loc lsp.Location
	if err := json.Unmarshal(raw, &loc); err == nil && loc.URI != "" {
		return loc, nil
	}
	// Try []Location.
	var locs []lsp.Location
	if err := json.Unmarshal(raw, &locs); err == nil && len(locs) > 0 {
		return locs[0], nil
	}
	return lsp.Location{}, nil
}

// parseLocations decodes a references response (null or []Location).
func parseLocations(raw json.RawMessage) []lsp.Location {
	if raw == nil || string(raw) == "null" {
		return nil
	}
	var locs []lsp.Location
	if err := json.Unmarshal(raw, &locs); err == nil {
		return locs
	}
	return nil
}

// extractHoverText pulls plain text out of a hover response, which may
// contain a MarkupContent or a plain string.
func extractHoverText(raw json.RawMessage) string {
	if raw == nil || string(raw) == "null" {
		return ""
	}
	var h struct {
		Contents json.RawMessage `json:"contents"`
	}
	if err := json.Unmarshal(raw, &h); err != nil || h.Contents == nil {
		return ""
	}
	// MarkupContent {"kind":"...", "value":"..."}
	var mc struct {
		Kind  string `json:"kind"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(h.Contents, &mc); err == nil && mc.Value != "" {
		return strings.TrimSpace(mc.Value)
	}
	// Plain string
	var s string
	if err := json.Unmarshal(h.Contents, &s); err == nil {
		return strings.TrimSpace(s)
	}
	return ""
}

// wrapDocText word-wraps text to maxW columns, splitting on existing newlines
// and then re-wrapping long lines.
func wrapDocText(text string, maxW int) []string {
	var out []string
	for _, para := range strings.Split(text, "\n") {
		if para == "" {
			out = append(out, "")
			continue
		}
		words := strings.Fields(para)
		line := ""
		for _, w := range words {
			if line == "" {
				line = w
			} else if len(line)+1+len(w) <= maxW {
				line += " " + w
			} else {
				out = append(out, line)
				line = w
			}
		}
		if line != "" {
			out = append(out, line)
		}
	}
	// Trim trailing blank lines.
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

// renderLSPDocPopup draws the lsp-show-doc popup as a bordered box positioned
// below the cursor (or above if there is no room below).
func (e *Editor) renderLSPDocPopup() {
	if len(e.lspDocLines) == 0 || e.term == nil {
		return
	}
	lines := e.lspDocLines

	// Determine popup width from widest line (capped at 72).
	popupW := 20
	for _, l := range lines {
		if n := len([]rune(l)); n+2 > popupW {
			popupW = n + 2
		}
	}
	if popupW > 72 {
		popupW = 72
	}

	totalW, totalH := e.term.Size()

	// Cursor position.
	w := e.activeWin
	buf := w.Buf()
	docLine, col := buf.LineCol(buf.Point())
	cursorRow := w.Top() + (docLine - w.ScrollLine())
	cursorCol := w.Left() + col

	nLines := len(lines)
	popupH := nLines + 2 // +2 for top and bottom borders

	// Try to place below the cursor; flip above if not enough room.
	borderTop := cursorRow + 1
	if borderTop+popupH > totalH-1 {
		borderTop = cursorRow - popupH
	}
	if borderTop < 0 {
		borderTop = 0
	}

	// Clamp left so the popup fits horizontally.
	left := max(0, min(cursorCol, totalW-popupW-2))

	border := syntax.FaceCompletionBorder
	text := syntax.FaceCandidate

	// Top border.
	e.term.SetCell(left, borderTop, '╭', border)
	for j := 1; j <= popupW; j++ {
		e.term.SetCell(left+j, borderTop, '─', border)
	}
	e.term.SetCell(left+popupW+1, borderTop, '╮', border)

	// Content rows.
	for i, docLine := range lines {
		row := borderTop + 1 + i
		if row >= totalH-1 {
			break
		}
		runes := []rune(docLine)
		e.term.SetCell(left, row, '│', border)
		for j := 0; j < popupW; j++ {
			ch := ' '
			if j < len(runes) {
				ch = runes[j]
			}
			e.term.SetCell(left+1+j, row, ch, text)
		}
		e.term.SetCell(left+popupW+1, row, '│', border)
	}

	// Bottom border.
	borderBot := borderTop + 1 + nLines
	if borderBot < totalH-1 {
		e.term.SetCell(left, borderBot, '╰', border)
		for j := 1; j <= popupW; j++ {
			e.term.SetCell(left+j, borderBot, '─', border)
		}
		e.term.SetCell(left+popupW+1, borderBot, '╯', border)
	}
}
