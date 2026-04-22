package dap

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

// Client is a DAP (Debug Adapter Protocol) client.
//
// DAP messages are distinguished by their "type" field:
//   - "request"  – sent by the client; expects a "response"
//   - "response" – reply from the adapter (matched by request_seq)
//   - "event"    – unsolicited notification from the adapter
type Client struct {
	cmd    *exec.Cmd
	rw     io.ReadWriteCloser
	reader *bufio.Reader

	pendingMu sync.Mutex
	nextSeq   int
	pending   map[int]chan callResult

	writeMu sync.Mutex

	eventMu sync.RWMutex
	onEvent func(event string, body json.RawMessage)

	closed chan struct{}
	once   sync.Once
}

type callResult struct {
	result json.RawMessage
	err    error
}

// Start launches command as a DAP adapter using the --client-addr
// reverse-connect approach: we listen on a random TCP port, pass
// --client-addr HOST:PORT to the adapter, wait for it to connect,
// then hand the accepted connection to the Client.
//
// The adapter process's stderr is discarded; its stdout/stdin are
// NOT used for DAP traffic (only TCP is).
func Start(command string, args ...string) (*Client, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("dap: listen: %w", err)
	}

	addr := ln.Addr().String()
	allArgs := append(args, "--client-addr", addr) //nolint:gocritic

	cmd := exec.Command(command, allArgs...) //nolint:gosec
	if err := cmd.Start(); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("dap: start %q: %w", command, err)
	}

	// Accept with a reasonable timeout so we don't hang forever if
	// the adapter fails to connect.
	type accepted struct {
		conn net.Conn
		err  error
	}
	ch := make(chan accepted, 1)
	go func() {
		conn, e := ln.Accept()
		ch <- accepted{conn, e}
	}()

	// Wait for either the connection or the process dying.
	a := <-ch
	_ = ln.Close()
	if a.err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("dap: accept: %w", a.err)
	}
	conn := a.conn

	c := &Client{
		cmd:     cmd,
		rw:      conn,
		reader:  bufio.NewReaderSize(conn, 1<<16),
		pending: make(map[int]chan callResult),
		closed:  make(chan struct{}),
		nextSeq: 1,
	}
	go c.readLoop()
	return c, nil
}

// SetEventHandler installs a callback invoked for every adapter event.
// It is called from the read goroutine; keep it non-blocking.
func (c *Client) SetEventHandler(fn func(event string, body json.RawMessage)) {
	c.eventMu.Lock()
	c.onEvent = fn
	c.eventMu.Unlock()
}

// Request sends a DAP request and blocks until the adapter replies or the
// client closes. Use RequestCtx for cancellation support.
func (c *Client) Request(command string, args any) (json.RawMessage, error) {
	return c.RequestCtx(context.Background(), command, args)
}

// RequestCtx sends a DAP request and blocks until the adapter replies, ctx is
// cancelled, or the client is closed.
func (c *Client) RequestCtx(ctx context.Context, command string, args any) (json.RawMessage, error) {
	c.pendingMu.Lock()
	seq := c.nextSeq
	c.nextSeq++
	ch := make(chan callResult, 1)
	c.pending[seq] = ch
	c.pendingMu.Unlock()

	if err := c.send(seq, command, args); err != nil {
		c.pendingMu.Lock()
		delete(c.pending, seq)
		c.pendingMu.Unlock()
		return nil, err
	}

	select {
	case res := <-ch:
		return res.result, res.err
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, seq)
		c.pendingMu.Unlock()
		return nil, ctx.Err()
	case <-c.closed:
		return nil, fmt.Errorf("dap: client closed")
	}
}

// Close shuts down the DAP adapter.
func (c *Client) Close() {
	c.once.Do(func() {
		close(c.closed)
		_ = c.rw.Close()
		if c.cmd != nil && c.cmd.Process != nil {
			_ = c.cmd.Wait()
		}
	})
}

func (c *Client) send(seq int, command string, args any) error {
	msg := map[string]any{
		"seq":     seq,
		"type":    "request",
		"command": command,
	}
	if args != nil {
		msg["arguments"] = args
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if _, err := io.WriteString(c.rw, header); err != nil {
		return err
	}
	_, err = c.rw.Write(body)
	return err
}

func (c *Client) readLoop() {
	for {
		contentLength := 0
		for {
			line, err := c.reader.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				break
			}
			if v, ok := strings.CutPrefix(line, "Content-Length: "); ok {
				n, _ := strconv.Atoi(v)
				contentLength = n
			}
		}
		if contentLength == 0 {
			continue
		}
		buf := make([]byte, contentLength)
		if _, err := io.ReadFull(c.reader, buf); err != nil {
			return
		}
		c.dispatch(buf)
	}
}

func (c *Client) dispatch(body []byte) {
	var msg Message
	if err := json.Unmarshal(body, &msg); err != nil {
		return
	}
	switch msg.Type {
	case "response":
		c.pendingMu.Lock()
		ch, ok := c.pending[msg.RequestSeq]
		if ok {
			delete(c.pending, msg.RequestSeq)
		}
		c.pendingMu.Unlock()
		if ok {
			var res callResult
			if !msg.Success {
				res.err = fmt.Errorf("dap %s: %s", msg.Command, msg.Message)
			} else {
				res.result = msg.Body
			}
			select {
			case ch <- res:
			default:
			}
		}
	case "event":
		c.eventMu.RLock()
		handler := c.onEvent
		c.eventMu.RUnlock()
		if handler != nil {
			handler(msg.Event, msg.Body)
		}
	}
}
