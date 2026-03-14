package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

// Client is an LSP JSON-RPC 2.0 client backed by a subprocess communicating
// via stdio using the LSP base-protocol framing.
type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader

	pendingMu sync.Mutex
	nextID    int
	pending   map[int]chan callResult

	writeMu sync.Mutex

	notifyMu sync.RWMutex
	onNotify func(method string, params json.RawMessage)

	closed chan struct{}
	once   sync.Once
}

type callResult struct {
	result json.RawMessage
	err    error
}

type rpcMsg struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Start launches command as an LSP server and returns a connected Client.
func Start(command string, args ...string) (*Client, error) {
	cmd := exec.Command(command, args...) //nolint:gosec // user-controlled command is intentional
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("lsp: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("lsp: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("lsp: start %q: %w", command, err)
	}
	c := &Client{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  bufio.NewReaderSize(stdout, 1<<16),
		pending: make(map[int]chan callResult),
		closed:  make(chan struct{}),
	}
	go c.readLoop()
	return c, nil
}

// SetNotifyHandler installs a callback invoked for every server notification.
// It is called from the read goroutine; keep it non-blocking.
func (c *Client) SetNotifyHandler(fn func(method string, params json.RawMessage)) {
	c.notifyMu.Lock()
	c.onNotify = fn
	c.notifyMu.Unlock()
}

// Call sends a request and blocks until the server replies or the client closes.
// Use CallCtx for cancellation support.
func (c *Client) Call(method string, params any) (json.RawMessage, error) {
	return c.CallCtx(context.Background(), method, params)
}

// CallCtx sends a request and blocks until the server replies, ctx is
// cancelled, or the client is closed.  When ctx is cancelled the pending
// request is cleaned up and ctx.Err() is returned.
func (c *Client) CallCtx(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.pendingMu.Lock()
	id := c.nextID
	c.nextID++
	ch := make(chan callResult, 1)
	c.pending[id] = ch
	c.pendingMu.Unlock()

	if err := c.send(id, method, params); err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, err
	}

	select {
	case res := <-ch:
		return res.result, res.err
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, ctx.Err()
	case <-c.closed:
		return nil, fmt.Errorf("lsp: client closed")
	}
}

// Notify sends a JSON-RPC notification (no reply expected).
func (c *Client) Notify(method string, params any) error {
	return c.send(-1, method, params)
}

// Close shuts down the LSP server.
func (c *Client) Close() {
	c.once.Do(func() {
		_ = c.Notify("exit", nil)
		close(c.closed)
		_ = c.stdin.Close()
		_ = c.cmd.Wait()
	})
}

// send marshals and writes a JSON-RPC message (request or notification).
func (c *Client) send(id int, method string, params any) error {
	msg := map[string]any{"jsonrpc": "2.0", "method": method}
	if id >= 0 {
		msg["id"] = id
	}
	if params != nil {
		msg["params"] = params
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if _, err := io.WriteString(c.stdin, header); err != nil {
		return err
	}
	_, err = c.stdin.Write(body)
	return err
}

func (c *Client) readLoop() {
	for {
		contentLength := 0
		for {
			line, err := c.stdout.ReadString('\n')
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
		body := make([]byte, contentLength)
		if _, err := io.ReadFull(c.stdout, body); err != nil {
			return
		}
		c.dispatch(body)
	}
}

func (c *Client) dispatch(body []byte) {
	var msg rpcMsg
	if err := json.Unmarshal(body, &msg); err != nil {
		return
	}
	switch {
	case msg.ID != nil && msg.Method == "":
		// Response to one of our requests.
		c.pendingMu.Lock()
		ch, ok := c.pending[*msg.ID]
		if ok {
			delete(c.pending, *msg.ID)
		}
		c.pendingMu.Unlock()
		if ok {
			var res callResult
			if msg.Error != nil {
				res.err = fmt.Errorf("lsp %s (code %d)", msg.Error.Message, msg.Error.Code)
			} else {
				res.result = msg.Result
			}
			select {
			case ch <- res:
			default:
			}
		}
	case msg.Method != "" && msg.ID == nil:
		// Server-sent notification.
		c.notifyMu.RLock()
		handler := c.onNotify
		c.notifyMu.RUnlock()
		if handler != nil {
			handler(msg.Method, msg.Params)
		}
	}
}
