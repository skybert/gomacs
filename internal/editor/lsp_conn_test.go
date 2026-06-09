package editor

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/lsp"
)

// fakeLSPServer wires an lsp.Client to an in-process server goroutine that
// replies to each request using the responder (method → result JSON).  Returns
// the client and a cleanup func.
func fakeLSPServer(t *testing.T, responder func(method string) any) (*lsp.Client, func()) {
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

func TestLspMaybeTriggerCompletion_PopsUp(t *testing.T) {
	e, _ := newLSPConnEditor(t, func(method string) any {
		if method == "textDocument/completion" {
			return map[string]any{"items": []map[string]any{
				{"label": "foobar"}, {"label": "foobaz"},
			}}
		}
		return nil
	})
	b := e.ActiveBuffer()
	b.SetReadOnly(false)
	b.InsertString(b.Len(), "foob")
	b.SetPoint(b.Len())
	e.lspMaybeTriggerCompletion()
	drainOneLSPCb(t, e)
	if !e.lspCompActive {
		t.Errorf("expected completion popup active, items=%d", len(e.lspCompItems))
	}
}

func TestLspMaybeTriggerCompletion_FallbackBufferWords(t *testing.T) {
	// No ready conn → falls back to buffer-word completion.
	e, conn := newLSPConnEditor(t, func(string) any { return nil })
	conn.isReady = false
	b := e.ActiveBuffer()
	b.SetReadOnly(false)
	b.InsertString(b.Len(), "mainx main") // "main" appears as a completable word
	b.SetPoint(b.Len())
	e.lspMaybeTriggerCompletion()
	// Either pops up buffer-word completion or no-ops; must not panic.
}
