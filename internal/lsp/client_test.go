package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// newPipeClient builds a Client whose stdin/stdout are connected to in-process
// pipes. serverFn runs in a goroutine and acts as the LSP "server", reading
// the framed requests the Client sends and writing back framed responses.
//
// The returned (close func) shuts down the pipes; do NOT call c.Close() on a
// pipe-backed client because that calls cmd.Wait() on a nil exec.Cmd.
func newPipeClient(t *testing.T, serverFn func(r *bufio.Reader, w io.Writer)) (*Client, func()) {
	t.Helper()
	// stdinR/stdinW: client writes requests, server reads them.
	stdinR, stdinW := io.Pipe()
	// stdoutR/stdoutW: server writes responses, client reads them.
	stdoutR, stdoutW := io.Pipe()

	c := &Client{
		cmd:     nil,
		stdin:   stdinW,
		stdout:  bufio.NewReaderSize(stdoutR, 1<<16),
		pending: make(map[int]chan callResult),
		closed:  make(chan struct{}),
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		serverFn(bufio.NewReaderSize(stdinR, 1<<16), stdoutW)
	}()
	go c.readLoop()

	cleanup := func() {
		_ = stdinW.Close()
		_ = stdinR.Close()
		_ = stdoutW.Close()
		_ = stdoutR.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}
	return c, cleanup
}

// readLSPMessage reads an LSP-framed JSON message from r.
func readLSPMessage(r *bufio.Reader) (rpcMsg, error) {
	contentLength := 0
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return rpcMsg{}, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if v, ok := strings.CutPrefix(line, "Content-Length: "); ok {
			fmt.Sscanf(v, "%d", &contentLength) //nolint:errcheck
		}
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(r, body); err != nil {
		return rpcMsg{}, err
	}
	var msg rpcMsg
	if err := json.Unmarshal(body, &msg); err != nil {
		return rpcMsg{}, err
	}
	return msg, nil
}

// writeLSPMessage writes a JSON value with LSP framing to w.
func writeLSPMessage(w io.Writer, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "Content-Length: %d\r\n\r\n%s", len(body), body)
	return err
}

func TestClientCallReturnsResult(t *testing.T) {
	c, cleanup := newPipeClient(t, func(r *bufio.Reader, w io.Writer) {
		req, err := readLSPMessage(r)
		if err != nil {
			return
		}
		_ = writeLSPMessage(w, map[string]any{
			"jsonrpc": "2.0",
			"id":      *req.ID,
			"result":  map[string]any{"hello": "world"},
		})
	})
	defer cleanup()

	raw, err := c.Call("initialize", map[string]any{"foo": "bar"})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["hello"] != "world" {
		t.Errorf("Call result = %v, want hello=world", got)
	}
}

func TestClientCallReturnsError(t *testing.T) {
	c, cleanup := newPipeClient(t, func(r *bufio.Reader, w io.Writer) {
		req, err := readLSPMessage(r)
		if err != nil {
			return
		}
		_ = writeLSPMessage(w, map[string]any{
			"jsonrpc": "2.0",
			"id":      *req.ID,
			"error":   map[string]any{"code": -32601, "message": "method not found"},
		})
	})
	defer cleanup()

	_, err := c.Call("bogus", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "method not found") {
		t.Errorf("err = %v, want to contain 'method not found'", err)
	}
}

func TestClientNotifyAcceptedByServer(t *testing.T) {
	gotMethod := make(chan string, 1)
	c, cleanup := newPipeClient(t, func(r *bufio.Reader, _ io.Writer) {
		msg, err := readLSPMessage(r)
		if err != nil {
			return
		}
		gotMethod <- msg.Method
	})
	defer cleanup()

	if err := c.Notify("initialized", map[string]any{}); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	select {
	case m := <-gotMethod:
		if m != "initialized" {
			t.Errorf("notify method = %q, want %q", m, "initialized")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for server to receive notification")
	}
}

func TestClientNotifyHandlerReceivesNotifications(t *testing.T) {
	received := make(chan string, 1)
	c, cleanup := newPipeClient(t, func(_ *bufio.Reader, w io.Writer) {
		_ = writeLSPMessage(w, map[string]any{
			"jsonrpc": "2.0",
			"method":  "window/showMessage",
			"params":  map[string]any{"type": 3, "message": "hi"},
		})
	})
	defer cleanup()

	c.SetNotifyHandler(func(method string, _ json.RawMessage) {
		received <- method
	})

	select {
	case m := <-received:
		if m != "window/showMessage" {
			t.Errorf("notification method = %q", m)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for notification")
	}
}

func TestClientCallCtxCancellation(t *testing.T) {
	// Server never responds.
	c, cleanup := newPipeClient(t, func(r *bufio.Reader, _ io.Writer) {
		_, _ = readLSPMessage(r)
		// Block forever (until pipe closes).
		<-make(chan struct{})
	})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := c.CallCtx(ctx, "slow", nil)
	if err == nil {
		t.Fatal("expected ctx error, got nil")
	}
	if !strings.Contains(err.Error(), "deadline") && !strings.Contains(err.Error(), "canceled") {
		t.Errorf("err = %v, want context error", err)
	}
}

func TestClientCallReturnsErrorAfterClose(t *testing.T) {
	c, cleanup := newPipeClient(t, func(_ *bufio.Reader, _ io.Writer) {
		<-make(chan struct{})
	})

	// Manually trigger the closed channel without calling Close (which would
	// dereference nil cmd).
	close(c.closed)
	cleanup()

	_, err := c.Call("foo", nil)
	if err == nil {
		t.Fatal("expected error after close, got nil")
	}
}

// startEcho starts "cat" so we have a real exec.Cmd to test Close.
func startEcho(t *testing.T) *Client {
	t.Helper()
	c, err := Start("cat")
	if err != nil {
		// "cat" should be available on every Unix; skip if not.
		if _, ok := err.(*exec.Error); ok {
			t.Skipf("cat not available: %v", err)
		}
		t.Fatalf("Start: %v", err)
	}
	return c
}

func TestStartAndClose(t *testing.T) {
	c := startEcho(t)
	// Wait briefly so Close's "exit" notify has time to flush.
	time.Sleep(20 * time.Millisecond)
	c.Close()
	// Calling Close again should be a no-op (sync.Once).
	c.Close()
}

func TestStartFailsForMissingBinary(t *testing.T) {
	_, err := Start("definitely-not-a-real-binary-xyzzy-12345")
	if err == nil {
		t.Fatal("expected error for missing binary, got nil")
	}
}

func TestClientCallConcurrent(t *testing.T) {
	// Server echoes id back as result.
	var serverWG sync.WaitGroup
	c, cleanup := newPipeClient(t, func(r *bufio.Reader, w io.Writer) {
		for {
			req, err := readLSPMessage(r)
			if err != nil {
				return
			}
			if req.ID == nil {
				continue
			}
			serverWG.Add(1)
			id := *req.ID
			go func() {
				defer serverWG.Done()
				_ = writeLSPMessage(w, map[string]any{
					"jsonrpc": "2.0",
					"id":      id,
					"result":  map[string]any{"id": id},
				})
			}()
		}
	})
	defer cleanup()

	const N = 5
	errs := make(chan error, N)
	for range N {
		go func() {
			_, err := c.Call("ping", nil)
			errs <- err
		}()
	}
	for i := range N {
		select {
		case err := <-errs:
			if err != nil {
				t.Errorf("Call %d: %v", i, err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("timeout waiting for concurrent Call")
		}
	}
}
