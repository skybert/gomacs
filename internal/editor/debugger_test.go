package editor

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/skybert/gomacs/internal/dap"
)

// dapFakeServer accepts one connection, then replies success to every request
// with the given per-command body (or an empty object).  It runs until the
// connection closes.
func dapFakeServer(t *testing.T, bodies map[string]any) (*dap.Client, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	clientConn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		ln.Close() //nolint:errcheck
		t.Fatal(err)
	}
	srv, err := ln.Accept()
	ln.Close() //nolint:errcheck
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		reader := srv
		buf := make([]byte, 65536)
		var acc []byte
		for {
			n, rerr := reader.Read(buf)
			if n > 0 {
				acc = append(acc, buf[:n]...)
				for {
					hdrEnd := indexHeaderEnd(acc)
					if hdrEnd < 0 {
						break
					}
					clen := parseContentLength(string(acc[:hdrEnd]))
					total := hdrEnd + 4 + clen
					if len(acc) < total {
						break
					}
					body := acc[hdrEnd+4 : total]
					acc = acc[total:]
					var msg dap.Message
					_ = json.Unmarshal(body, &msg)
					respBody, ok := bodies[msg.Command]
					if !ok {
						respBody = map[string]any{}
					}
					resp := map[string]any{
						"seq": 1, "type": "response",
						"request_seq": msg.Seq, "command": msg.Command,
						"success": true, "body": respBody,
					}
					rb, _ := json.Marshal(resp)
					_, _ = fmt.Fprintf(srv, "Content-Length: %d\r\n\r\n%s", len(rb), rb)
				}
			}
			if rerr != nil {
				return
			}
		}
	}()

	c := dap.NewConnClient(clientConn)
	return c, func() { c.Close(); srv.Close() } //nolint:errcheck
}

func indexHeaderEnd(b []byte) int {
	return strings.Index(string(b), "\r\n\r\n")
}

func parseContentLength(header string) int {
	var n int
	if i := strings.Index(header, "Content-Length: "); i >= 0 {
		_, _ = fmt.Sscanf(header[i:], "Content-Length: %d", &n)
	}
	return n
}

func TestDapBackend_StepCommands(t *testing.T) {
	c, cleanup := dapFakeServer(t, nil)
	defer cleanup()
	b := &dapBackend{client: c}

	if err := b.Continue(1); err != nil {
		t.Errorf("Continue: %v", err)
	}
	if err := b.StepNext(1); err != nil {
		t.Errorf("StepNext: %v", err)
	}
	if err := b.StepIn(1); err != nil {
		t.Errorf("StepIn: %v", err)
	}
	if err := b.StepOut(1); err != nil {
		t.Errorf("StepOut: %v", err)
	}
	if err := b.SetBreakpoints("/tmp/x.go", []int{3, 7}); err != nil {
		t.Errorf("SetBreakpoints: %v", err)
	}
}

func TestDapBackend_Evaluate(t *testing.T) {
	c, cleanup := dapFakeServer(t, map[string]any{
		"stackTrace": map[string]any{
			"stackFrames": []map[string]any{{"id": 42, "name": "main", "line": 5}},
		},
		"evaluate": map[string]any{"result": "41"},
	})
	defer cleanup()
	b := &dapBackend{client: c}

	// frameID 0 + stoppedThread != 0 triggers the stackTrace lookup first.
	got, err := b.Evaluate("x", 0, 1, "hover")
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if got != "41" {
		t.Errorf("Evaluate result = %q, want \"41\"", got)
	}
}

func TestDapBackend_Close(t *testing.T) {
	c, _ := dapFakeServer(t, nil)
	b := &dapBackend{client: c}
	done := make(chan struct{})
	go func() { b.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return")
	}
}

func TestDapFetchLocals_And_Vars(t *testing.T) {
	c, cleanup := dapFakeServer(t, map[string]any{
		"scopes": map[string]any{"scopes": []map[string]any{
			{"name": "Locals", "variablesReference": 10},
		}},
		"variables": map[string]any{"variables": []map[string]any{
			{"name": "x", "value": "1", "variablesReference": 0},
			{"name": "obj", "value": "{...}", "variablesReference": 11},
		}},
	})
	defer cleanup()

	vars := dapFetchLocals(c, 1, 1)
	if len(vars) != 2 {
		t.Fatalf("expected 2 locals, got %d", len(vars))
	}
	if vars[0].name != "x" {
		t.Errorf("first local = %q, want x", vars[0].name)
	}
	// "obj" has a non-zero ref and depth(0) < maxDepth(1) → children fetched.
	if !vars[1].expanded || len(vars[1].children) == 0 {
		t.Errorf("expandable var should have children fetched: %+v", vars[1])
	}
}

func TestDapFetchVars_ZeroRef(t *testing.T) {
	c, cleanup := dapFakeServer(t, nil)
	defer cleanup()
	if got := dapFetchVars(c, 0, 0, 1); got != nil {
		t.Errorf("ref 0 should return nil, got %v", got)
	}
}
