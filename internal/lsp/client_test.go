package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
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

	c := NewConnClient(stdinW, stdoutR)

	done := make(chan struct{})
	go func() {
		defer close(done)
		serverFn(bufio.NewReaderSize(stdinR, 1<<16), stdoutW)
	}()

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

// ---------------------------------------------------------------------------
// Outbound queue: a wedged server must not block the caller
//
// Notify/Call used to write straight to the server's stdin from the calling
// goroutine, which for gomacs is the main event loop (Redraw →
// lspMaybeDidChange → Notify).  A server that stopped reading its stdin froze
// the whole editor.  Writes now go through a FIFO queue drained by a single
// writer goroutine; the tests below pin down the queue's ordering, coalescing,
// backpressure, and shutdown behaviour.
// ---------------------------------------------------------------------------

// gatedWriter is an io.WriteCloser that emulates a language server which has
// stopped reading its stdin: every Write blocks until a token is handed out by
// allow (or allowAll), and Close unblocks any in-flight Write the way closing a
// real pipe does.  Frames that get through are recorded in order.
type gatedWriter struct {
	gate     chan struct{}
	gateOnce sync.Once
	closed   chan struct{}
	closeOne sync.Once

	mu     sync.Mutex
	frames [][]byte
}

func newGatedWriter() *gatedWriter {
	return &gatedWriter{gate: make(chan struct{}), closed: make(chan struct{})}
}

func (w *gatedWriter) Write(p []byte) (int, error) {
	select {
	case <-w.gate:
	case <-w.closed:
		return 0, io.ErrClosedPipe
	}
	w.mu.Lock()
	w.frames = append(w.frames, append([]byte(nil), p...))
	w.mu.Unlock()
	return len(p), nil
}

func (w *gatedWriter) Close() error {
	w.closeOne.Do(func() { close(w.closed) })
	return nil
}

// allow lets n queued writes through, blocking until the writer goroutine has
// taken every token.
func (w *gatedWriter) allow(t *testing.T, n int) {
	t.Helper()
	for i := range n {
		select {
		case w.gate <- struct{}{}:
		case <-time.After(2 * time.Second):
			t.Fatalf("writer goroutine did not take write token %d/%d", i+1, n)
		}
	}
}

// allowAll stops gating writes entirely.
func (w *gatedWriter) allowAll() { w.gateOnce.Do(func() { close(w.gate) }) }

func (w *gatedWriter) frameCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.frames)
}

// messages parses every recorded frame back into an rpcMsg, in write order.
func (w *gatedWriter) messages(t *testing.T) []rpcMsg {
	t.Helper()
	w.mu.Lock()
	frames := append([][]byte(nil), w.frames...)
	w.mu.Unlock()
	out := make([]rpcMsg, 0, len(frames))
	for i, f := range frames {
		msg, err := readLSPMessage(bufio.NewReader(strings.NewReader(string(f))))
		if err != nil {
			t.Fatalf("frame %d is not a complete LSP frame (%v): %q", i, err, f)
		}
		out = append(out, msg)
	}
	return out
}

// newWedgedClient returns a Client whose stdin is a gatedWriter (nothing is
// written until the test allows it) and whose stdout never produces data,
// together with that writer.
func newWedgedClient(t *testing.T) (*Client, *gatedWriter) {
	t.Helper()
	gw := newGatedWriter()
	pr, pw := io.Pipe()
	c := NewConnClient(gw, pr)
	t.Cleanup(func() {
		c.Close()
		_ = pw.Close()
		_ = pr.Close()
	})
	// Ungate first (LIFO cleanup order) so Close never waits on a token.
	t.Cleanup(gw.allowAll)
	return c, gw
}

// waitFor polls cond until it holds or the timeout expires.
func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func didChangeParams(uri string, version int, text string) map[string]any {
	return map[string]any{
		"textDocument":   map[string]any{"uri": uri, "version": version},
		"contentChanges": []map[string]any{{"text": text}},
	}
}

func TestClientNotifyDoesNotBlockOnWedgedServer(t *testing.T) {
	c, gw := newWedgedClient(t)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 20 {
			_ = c.Notify(fmt.Sprintf("method%d", i), map[string]any{"i": i})
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Notify blocked on a server that never reads its stdin")
	}
	if n := gw.frameCount(); n != 0 {
		t.Fatalf("no frame should have completed while the server is wedged, got %d", n)
	}
}

func TestClientCallDoesNotBlockIndefinitelyOnWedgedServer(t *testing.T) {
	c, _ := newWedgedClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := c.CallCtx(ctx, "textDocument/hover", nil)
	if err == nil {
		t.Fatal("expected an error from a wedged server")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("CallCtx took %v; the request write must not block the caller", elapsed)
	}
}

// TestClientOutboundOrderPreservedUnderBurst sends a burst of notifications
// followed by a request and checks the server sees them in exactly that order.
func TestClientOutboundOrderPreservedUnderBurst(t *testing.T) {
	got := make(chan string, 64)
	c, cleanup := newPipeClient(t, func(r *bufio.Reader, w io.Writer) {
		for {
			msg, err := readLSPMessage(r)
			if err != nil {
				return
			}
			got <- msg.Method
			if msg.ID != nil {
				_ = writeLSPMessage(w, map[string]any{
					"jsonrpc": "2.0",
					"id":      *msg.ID,
					"result":  map[string]any{},
				})
			}
		}
	})
	defer cleanup()

	const burst = 10
	want := make([]string, 0, burst+1)
	for i := range burst {
		m := fmt.Sprintf("notify%d", i)
		// Distinct URIs so nothing is coalesced away.
		if err := c.Notify(m, didChangeParams(fmt.Sprintf("file:///f%d.go", i), i, "x")); err != nil {
			t.Fatalf("Notify %d: %v", i, err)
		}
		want = append(want, m)
	}
	if _, err := c.Call("textDocument/definition", nil); err != nil {
		t.Fatalf("Call: %v", err)
	}
	want = append(want, "textDocument/definition")

	for i, wm := range want {
		select {
		case m := <-got:
			if m != wm {
				t.Fatalf("message %d = %q, want %q (ordering must be FIFO)", i, m, wm)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out waiting for message %d (%q)", i, wm)
		}
	}
}

// TestClientCoalescesDidChangeKeepingLatest checks that when the server is not
// draining, queued didChange notifications for the same document collapse to
// the newest one — each carries the full document text, so older ones are
// redundant — and that the text the server eventually sees is the latest.
func TestClientCoalescesDidChangeKeepingLatest(t *testing.T) {
	c, gw := newWedgedClient(t)
	const uri = "file:///tmp/a.go"

	if err := c.Notify(methodDidChange, didChangeParams(uri, 1, "v1")); err != nil {
		t.Fatalf("Notify v1: %v", err)
	}
	// Wait until the writer goroutine has taken v1 out of the queue and is
	// blocked writing it, so v2… really do pile up behind it.
	waitFor(t, 2*time.Second, "the writer to pick up the first didChange", func() bool {
		queued, _, _, _ := c.outboundStats()
		return queued == 0
	})

	for v := 2; v <= 6; v++ {
		if err := c.Notify(methodDidChange, didChangeParams(uri, v, fmt.Sprintf("v%d", v))); err != nil {
			t.Fatalf("Notify v%d: %v", v, err)
		}
	}
	queued, _, _, coalesced := c.outboundStats()
	if queued != 1 {
		t.Fatalf("queued = %d, want 1 (five didChange for one URI must coalesce)", queued)
	}
	if coalesced != 4 {
		t.Fatalf("coalesced = %d, want 4", coalesced)
	}

	gw.allow(t, 2)
	waitFor(t, 2*time.Second, "both frames to be written", func() bool {
		return gw.frameCount() == 2
	})

	msgs := gw.messages(t)
	if len(msgs) != 2 {
		t.Fatalf("server saw %d messages, want 2", len(msgs))
	}
	for i, m := range msgs {
		if m.Method != methodDidChange {
			t.Fatalf("message %d method = %q", i, m.Method)
		}
	}
	if txt := changeText(t, msgs[0]); txt != "v1" {
		t.Errorf("first didChange text = %q, want %q (the in-flight one)", txt, "v1")
	}
	if txt := changeText(t, msgs[1]); txt != "v6" {
		t.Errorf("second didChange text = %q, want the latest %q", txt, "v6")
	}
}

// changeText pulls contentChanges[0].text out of a didChange notification.
func changeText(t *testing.T, msg rpcMsg) string {
	t.Helper()
	var p struct {
		ContentChanges []struct {
			Text string `json:"text"`
		} `json:"contentChanges"`
	}
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		t.Fatalf("params: %v", err)
	}
	if len(p.ContentChanges) == 0 {
		t.Fatal("didChange without contentChanges")
	}
	return p.ContentChanges[0].Text
}

// TestClientDropsNotificationsWhenFullButNeverDropsRequests pins the
// backpressure policy: past the soft limit a *notification* is dropped and
// Notify reports it, while a request is still accepted (it gets the larger hard
// limit) so requests are never silently lost.
func TestClientDropsNotificationsWhenFullButNeverDropsRequests(t *testing.T) {
	c, _ := newWedgedClient(t)

	full := 0
	for i := range outboundSoftLimit + 20 {
		// Distinct URIs so coalescing cannot hide the overflow.
		err := c.Notify(methodDidChange, didChangeParams(fmt.Sprintf("file:///f%d.go", i), i, "x"))
		if errors.Is(err, ErrOutboundFull) {
			full++
		} else if err != nil {
			t.Fatalf("Notify %d: unexpected error %v", i, err)
		}
	}
	if full == 0 {
		t.Fatal("expected some notifications to be dropped once the queue filled up")
	}
	if _, _, dropped, _ := c.outboundStats(); dropped != full {
		t.Errorf("dropped counter = %d, want %d", dropped, full)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := c.CallCtx(ctx, "textDocument/definition", nil)
	if errors.Is(err, ErrOutboundFull) {
		t.Fatal("a request must not be dropped just because the notification queue is full")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("CallCtx err = %v, want a context deadline (request queued, server wedged)", err)
	}
}

func TestClientRequestFailsWhenOutboundQueueIsSaturated(t *testing.T) {
	c, _ := newWedgedClient(t)

	// Saturate the request (hard) limit; requests are never dropped silently,
	// so every overflowing one must report ErrOutboundFull to its caller.
	var lastErr error
	for range outboundHardLimit + 5 {
		if err := c.send(0, "textDocument/definition", nil); err != nil {
			lastErr = err
			break
		}
	}
	if !errors.Is(lastErr, ErrOutboundFull) {
		t.Fatalf("saturated queue error = %v, want ErrOutboundFull", lastErr)
	}
}

// TestClientCloseWithWedgedWriteDoesNotLeak checks the shutdown path: Close
// returns promptly even while the writer goroutine is stuck mid-write, no
// goroutine is left behind, and nothing panics.
func TestClientCloseWithWedgedWriteDoesNotLeak(t *testing.T) {
	before := runtime.NumGoroutine()

	gw := newGatedWriter()
	pr, pw := io.Pipe()
	c := NewConnClient(gw, pr)

	for i := range 5 {
		_ = c.Notify(fmt.Sprintf("m%d", i), nil)
	}

	done := make(chan struct{})
	go func() {
		c.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Close deadlocked while a write was pending")
	}

	_ = pw.Close() // let the reader goroutine finish too
	waitFor(t, 3*time.Second, "reader and writer goroutines to exit", func() bool {
		return runtime.NumGoroutine() <= before
	})

	// Post-close sends must fail, not panic on a closed channel.
	if err := c.Notify("late", nil); err == nil {
		t.Error("Notify after Close should return an error")
	}
	c.Close() // idempotent
}

// TestClientCloseRacesWithNotifies hammers Notify from several goroutines while
// Close runs; run with -race this catches send-on-closed-channel and data races
// in the queue.
func TestClientCloseRacesWithNotifies(t *testing.T) {
	gw := newGatedWriter()
	gw.allowAll()
	pr, pw := io.Pipe()
	defer func() { _ = pw.Close() }()
	c := NewConnClient(gw, pr)

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 50 {
				_ = c.Notify(methodDidChange, didChangeParams("file:///a.go", i, "x"))
			}
		}()
	}
	c.Close()
	wg.Wait()
}

// TestClientFlushesExitNotificationOnClose makes sure moving writes onto a
// background goroutine did not turn the shutdown handshake into a no-op.
func TestClientFlushesExitNotificationOnClose(t *testing.T) {
	gotExit := make(chan struct{})
	var once sync.Once
	c, cleanup := newPipeClient(t, func(r *bufio.Reader, _ io.Writer) {
		for {
			msg, err := readLSPMessage(r)
			if err != nil {
				return
			}
			if msg.Method == "exit" {
				once.Do(func() { close(gotExit) })
			}
		}
	})
	defer cleanup()

	if err := c.Notify("initialized", map[string]any{}); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	go c.Close()

	select {
	case <-gotExit:
	case <-time.After(3 * time.Second):
		t.Fatal("server never received the exit notification queued by Close")
	}
}

// TestClientPendingRequestFailsWhenReaderExits covers the "reader goroutine
// exits first" case: the request can never be answered, so it must fail
// immediately instead of hanging until Close.
func TestClientPendingRequestFailsWhenReaderExits(t *testing.T) {
	gw := newGatedWriter()
	gw.allowAll()
	pr, pw := io.Pipe()
	c := NewConnClient(gw, pr)
	t.Cleanup(func() { c.Close(); _ = pr.Close() })

	errs := make(chan error, 1)
	go func() {
		_, err := c.Call("textDocument/definition", nil)
		errs <- err
	}()
	waitFor(t, 2*time.Second, "the request to be written", func() bool {
		return gw.frameCount() > 0
	})
	_ = pw.Close() // server hangs up

	select {
	case err := <-errs:
		if err == nil {
			t.Fatal("expected an error once the connection broke")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Call hung after the reader goroutine exited")
	}
}

// ---------------------------------------------------------------------------
// Queue helpers
// ---------------------------------------------------------------------------

func TestCoalesceKeyFor(t *testing.T) {
	params := didChangeParams("file:///a.go", 1, "x")
	if got := coalesceKeyFor(-1, methodDidChange, params); got == "" {
		t.Error("didChange notification should be coalescible")
	}
	if got := coalesceKeyFor(7, methodDidChange, params); got != "" {
		t.Errorf("requests must never coalesce, got key %q", got)
	}
	if got := coalesceKeyFor(-1, "textDocument/didOpen", params); got != "" {
		t.Errorf("only didChange coalesces, got key %q for didOpen", got)
	}
	if got := coalesceKeyFor(-1, methodDidChange, map[string]any{}); got != "" {
		t.Errorf("params without a URI must not coalesce, got %q", got)
	}
	if got := coalesceKeyFor(-1, methodDidChange, nil); got != "" {
		t.Errorf("nil params must not coalesce, got %q", got)
	}
	a := coalesceKeyFor(-1, methodDidChange, didChangeParams("file:///a.go", 1, "x"))
	b := coalesceKeyFor(-1, methodDidChange, didChangeParams("file:///b.go", 1, "x"))
	if a == b {
		t.Error("different URIs must have different coalesce keys")
	}
}

func TestParamsURI(t *testing.T) {
	if got := paramsURI(didChangeParams("file:///a.go", 1, "x")); got != "file:///a.go" {
		t.Errorf("paramsURI = %q", got)
	}
	if got := paramsURI("not a map"); got != "" {
		t.Errorf("paramsURI(non-map) = %q", got)
	}
	if got := paramsURI(map[string]any{"textDocument": 42}); got != "" {
		t.Errorf("paramsURI(bad textDocument) = %q", got)
	}
	if got := paramsURI(map[string]any{"textDocument": map[string]any{}}); got != "" {
		t.Errorf("paramsURI(no uri) = %q", got)
	}
}

func TestFrameMsgIsOneCompleteFrame(t *testing.T) {
	frame, err := frameMsg(outMsg{id: 3, method: "initialize", params: map[string]any{"a": 1}})
	if err != nil {
		t.Fatalf("frameMsg: %v", err)
	}
	msg, err := readLSPMessage(bufio.NewReader(strings.NewReader(string(frame))))
	if err != nil {
		t.Fatalf("frame not parseable: %v", err)
	}
	if msg.ID == nil || *msg.ID != 3 || msg.Method != "initialize" {
		t.Fatalf("round-trip = %+v", msg)
	}

	// Notifications carry no id.
	frame, err = frameMsg(outMsg{id: -1, method: "exit"})
	if err != nil {
		t.Fatalf("frameMsg notification: %v", err)
	}
	msg, err = readLSPMessage(bufio.NewReader(strings.NewReader(string(frame))))
	if err != nil {
		t.Fatalf("frame not parseable: %v", err)
	}
	if msg.ID != nil {
		t.Errorf("notification must not carry an id, got %d", *msg.ID)
	}
}

// TestWriteLoopSurvivesUnmarshallableParams checks a single bad message is
// dropped without killing the stream (or the writer goroutine).
func TestWriteLoopSurvivesUnmarshallableParams(t *testing.T) {
	got := make(chan string, 4)
	c, cleanup := newPipeClient(t, func(r *bufio.Reader, _ io.Writer) {
		for {
			msg, err := readLSPMessage(r)
			if err != nil {
				return
			}
			got <- msg.Method
		}
	})
	defer cleanup()

	// A channel cannot be marshalled to JSON.
	if err := c.Notify("bogus", map[string]any{"ch": make(chan int)}); err != nil {
		t.Fatalf("Notify should queue even unmarshallable params: %v", err)
	}
	if err := c.Notify("initialized", map[string]any{}); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	select {
	case m := <-got:
		if m != "initialized" {
			t.Errorf("first delivered message = %q, want the good one", m)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("writer goroutine died on an unmarshallable message")
	}
}
