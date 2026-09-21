package dap

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"testing"
	"time"
)

// fakeAdapterEnv, when set to "1" in the environment, tells this test binary
// (when re-exec'd as a subprocess) to act as a minimal DAP adapter instead of
// running the test suite. See TestMain and TestStart_HappyPath: they let
// TestStart_HappyPath exercise Start's real "spawn a process, listen, wait
// for it to dial us back on --client-addr" path without depending on any
// real debug adapter binary being installed.
const fakeAdapterEnv = "GOMACS_DAP_TEST_FAKE_ADAPTER"

// TestMain intercepts the fake-adapter re-exec before testing.M ever parses
// flags, since the re-exec passes "--client-addr HOST:PORT" (Start's own
// argument convention), not any go test flag.
func TestMain(m *testing.M) {
	if os.Getenv(fakeAdapterEnv) == "1" {
		runFakeAdapter()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// runFakeAdapter dials back the --client-addr passed on the command line,
// answers exactly one request with a success response, and then blocks
// (simulating a running adapter) until the connection is closed by the
// client under test.
func runFakeAdapter() {
	addr := ""
	for i, a := range os.Args {
		if a == "--client-addr" && i+1 < len(os.Args) {
			addr = os.Args[i+1]
			break
		}
	}
	if addr == "" {
		os.Exit(1)
	}
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		os.Exit(1)
	}
	defer conn.Close() //nolint:errcheck

	if req, err := readDAP(conn); err == nil {
		resp := map[string]any{
			"seq":         1,
			"type":        "response",
			"request_seq": req.Seq,
			"command":     req.Command,
			"success":     true,
			"body":        map[string]any{"supportsConfigurationDoneRequest": true},
		}
		_ = writeDAP(conn, resp)
	}

	// Keep the connection open until the real Client closes it, so Close's
	// cmd.Wait() path (this process exiting) is only reached once the test
	// is actually done with us.
	buf := make([]byte, 1)
	for {
		if _, err := conn.Read(buf); err != nil {
			return
		}
	}
}

// newPipeClient starts a Client connected to an in-process mock adapter driven
// by serverFn. serverFn receives the adapter's read side (to read requests)
// and write side (to write responses/events) and runs until the connection
// is closed.
func newPipeClient(t *testing.T, serverFn func(r io.Reader, w io.Writer)) *Client {
	t.Helper()

	// Use a net.Pipe pair: clientConn ↔ serverConn.
	clientConn, serverConn, err := makePipeConn()
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		defer serverConn.Close() //nolint:errcheck
		serverFn(serverConn, serverConn)
	}()

	return NewConnClient(clientConn)
}

// makePipeConn returns a connected in-process net.Conn pair.
func makePipeConn() (net.Conn, net.Conn, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}
	defer ln.Close() //nolint:errcheck

	type result struct {
		conn net.Conn
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		conn, e := ln.Accept()
		ch <- result{conn, e}
	}()
	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		return nil, nil, err
	}
	r := <-ch
	if r.err != nil {
		_ = client.Close()
		return nil, nil, r.err
	}
	return client, r.conn, nil
}

func writeDAP(w io.Writer, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "Content-Length: %d\r\n\r\n%s", len(body), body)
	return err
}

func readDAP(r io.Reader) (Message, error) {
	// minimal framing reader for tests
	buf := make([]byte, 4096)
	// Read header
	headerBuf := make([]byte, 0, 64)
	single := make([]byte, 1)
	for {
		_, err := r.Read(single)
		if err != nil {
			return Message{}, err
		}
		headerBuf = append(headerBuf, single[0])
		if len(headerBuf) >= 4 &&
			headerBuf[len(headerBuf)-4] == '\r' &&
			headerBuf[len(headerBuf)-3] == '\n' &&
			headerBuf[len(headerBuf)-2] == '\r' &&
			headerBuf[len(headerBuf)-1] == '\n' {
			break
		}
	}
	header := string(headerBuf)
	var contentLength int
	_, _ = fmt.Sscanf(header, "Content-Length: %d", &contentLength)
	if contentLength > len(buf) {
		buf = make([]byte, contentLength)
	}
	body := buf[:contentLength]
	if _, err := io.ReadFull(r, body); err != nil {
		return Message{}, err
	}
	var msg Message
	err := json.Unmarshal(body, &msg)
	return msg, err
}

func TestClientRequestResponse(t *testing.T) {
	c := newPipeClient(t, func(r io.Reader, w io.Writer) {
		// Read one initialize request, send back a response.
		req, err := readDAP(r)
		if err != nil {
			return
		}
		resp := map[string]any{
			"seq":         1,
			"type":        "response",
			"request_seq": req.Seq,
			"command":     req.Command,
			"success":     true,
			"body":        map[string]any{"supportsConfigurationDoneRequest": true},
		}
		_ = writeDAP(w, resp)
	})

	raw, err := c.Request("initialize", InitializeArgs{
		AdapterID:       "gomacs",
		LinesStartAt1:   true,
		ColumnsStartAt1: true,
	})
	if err != nil {
		t.Fatalf("Request: %v", err)
	}

	var caps InitializeResponse
	if err := json.Unmarshal(raw, &caps); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !caps.SupportsConfigurationDoneRequest {
		t.Error("expected supportsConfigurationDoneRequest=true")
	}
}

func TestClientEventDelivery(t *testing.T) {
	received := make(chan string, 1)

	c := newPipeClient(t, func(_ io.Reader, w io.Writer) {
		// Immediately send an initialized event.
		evt := map[string]any{
			"seq":   1,
			"type":  "event",
			"event": "initialized",
			"body":  map[string]any{},
		}
		_ = writeDAP(w, evt)
	})
	c.SetEventHandler(func(event string, _ json.RawMessage) {
		received <- event
	})

	select {
	case ev := <-received:
		if ev != "initialized" {
			t.Errorf("got event %q, want %q", ev, "initialized")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestClientErrorResponse(t *testing.T) {
	c := newPipeClient(t, func(r io.Reader, w io.Writer) {
		req, err := readDAP(r)
		if err != nil {
			return
		}
		resp := map[string]any{
			"seq":         1,
			"type":        "response",
			"request_seq": req.Seq,
			"command":     req.Command,
			"success":     false,
			"message":     "unsupported command",
		}
		_ = writeDAP(w, resp)
	})

	_, err := c.Request("bogusCommand", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestClientClose(t *testing.T) {
	c := newPipeClient(t, func(r io.Reader, _ io.Writer) {
		// Block until connection closed.
		buf := make([]byte, 1)
		for {
			if _, err := r.Read(buf); err != nil {
				return
			}
		}
	})

	c.Close()

	// Subsequent request should return an error quickly.
	done := make(chan error, 1)
	go func() {
		_, err := c.Request("initialize", nil)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Error("expected error after close, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: Request did not return after Close")
	}
}

func TestStart_BadCommandReturnsError(t *testing.T) {
	if _, err := Start("", "/nonexistent/dap-adapter-xyz"); err == nil {
		t.Fatal("Start with a non-existent command should return an error")
	}
}

// TestStart_HappyPath exercises Start's reverse-connect accept path end to
// end: re-exec this test binary with fakeAdapterEnv set so it dials back
// the --client-addr Start listens on, then does one real request/response
// round trip over the resulting connection.
func TestStart_HappyPath(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("os.Executable: %v", err)
	}

	t.Setenv(fakeAdapterEnv, "1")

	c, err := Start("", exe)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Close()

	raw, err := c.Request("initialize", InitializeArgs{
		AdapterID:       "gomacs",
		LinesStartAt1:   true,
		ColumnsStartAt1: true,
	})
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	var caps InitializeResponse
	if err := json.Unmarshal(raw, &caps); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !caps.SupportsConfigurationDoneRequest {
		t.Error("expected supportsConfigurationDoneRequest=true")
	}
}

func TestRequestCtx_Cancelled(t *testing.T) {
	c := newPipeClient(t, func(r io.Reader, _ io.Writer) {
		// Never reply, so the request blocks until ctx is cancelled.
		buf := make([]byte, 1)
		for {
			if _, err := r.Read(buf); err != nil {
				return
			}
		}
	})
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	if _, err := c.RequestCtx(ctx, "initialize", nil); err == nil {
		t.Fatal("RequestCtx with a cancelled context should return an error")
	}
}
