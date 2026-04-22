package dap

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

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

	c := &Client{
		cmd:     nil,
		rw:      clientConn,
		reader:  bufio.NewReaderSize(clientConn, 1<<16),
		pending: make(map[int]chan callResult),
		closed:  make(chan struct{}),
		nextSeq: 1,
	}
	go c.readLoop()
	return c
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
