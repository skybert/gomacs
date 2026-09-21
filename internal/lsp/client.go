package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Client is an LSP JSON-RPC 2.0 client backed by a subprocess communicating
// via stdio using the LSP base-protocol framing.
//
// All writes to the server go through an internal outbound queue that is
// drained by a single writer goroutine (writeLoop).  Callers therefore never
// block on the server's stdin: a language server that stalls reading (GC pause,
// heavy indexing, hung process) slows the queue down but cannot freeze the
// goroutine that called Notify/Call — which for gomacs is the editor's main
// event loop.
type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader

	pendingMu sync.Mutex
	nextID    int
	pending   map[int]chan callResult

	// Outbound queue.  outQ is a FIFO of not-yet-written messages; outSig
	// (capacity 1) wakes the writer goroutine when work arrives.
	outMu        sync.Mutex
	outQ         []outMsg
	outInFlight  int // enqueued messages not yet written, dropped, or coalesced
	outWritten   int // frames handed to stdin (successfully or not)
	outDropped   int // notifications dropped because the queue was full
	outCoalesced int // notifications superseded by a newer one for the same key
	outClosing   bool
	outSig       chan struct{}

	notifyMu sync.RWMutex
	onNotify func(method string, params json.RawMessage)

	closed chan struct{}
	once   sync.Once

	// broken is closed when the connection can no longer be used: the reader
	// goroutine hit EOF/an error, or a write to the server failed.  brokenErr
	// is written before the close, so any goroutine that observes the close
	// also observes the error.
	broken     chan struct{}
	brokenOnce sync.Once
	brokenErr  error
}

// Outbound queue limits.
//
// Notifications are capped at outboundSoftLimit: beyond that a notification is
// dropped and Notify returns ErrOutboundFull rather than blocking the caller.
// This is safe for the notifications gomacs sends most often —
// textDocument/didChange carries the *full* document text, so a dropped one is
// superseded by the next one (and the editor rolls back its "the server has
// this version" bookkeeping when Notify fails, so the next edit resends).
//
// Requests get the much larger outboundHardLimit and are never dropped
// silently: if even that is exceeded, Call/CallCtx returns ErrOutboundFull so
// the caller always learns the request was not sent.
//
// The limits also bound memory: a queued didChange/didOpen holds a full
// document copy, and coalescing keeps at most one pending didChange per URI, so
// the queue cannot grow past roughly "one document per open file" while a
// server is wedged.
const (
	outboundSoftLimit = 64
	outboundHardLimit = 256
)

// outboundFlushTimeout bounds how long Close waits for queued frames (notably
// the final "exit" notification) to drain before the pipe is torn down.  A
// wedged server therefore costs at most this much on editor exit.
const outboundFlushTimeout = 100 * time.Millisecond

// procExitTimeout bounds how long Close waits for the server process to exit
// after its stdin has been closed, before killing it.  Without this a hung
// server would block the editor's exit path forever inside cmd.Wait.
const procExitTimeout = 500 * time.Millisecond

// ErrOutboundFull is returned when a message cannot be queued because the
// server is not draining the queue fast enough.
var ErrOutboundFull = errors.New("lsp: outbound queue full")

// ErrClientClosed is returned once the client has been closed.
var ErrClientClosed = errors.New("lsp: client closed")

// methodDidChange is the one notification whose payload fully supersedes any
// earlier queued payload for the same document.
const methodDidChange = "textDocument/didChange"

// outMsg is one queued outbound JSON-RPC message.  It is kept unmarshalled:
// json.Marshal of a full-document didChange costs milliseconds on a large file
// and runs on the writer goroutine, not on the caller's.
type outMsg struct {
	id     int // negative for notifications
	method string
	params any
	// coalesceKey, when non-empty, marks this message as superseding any
	// queued message with the same key (see coalesceKeyFor).
	coalesceKey string
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

// newClient wires up a Client around an already-connected pair of streams and
// starts its reader and writer goroutines.
func newClient(cmd *exec.Cmd, stdin io.WriteCloser, stdout io.Reader) *Client {
	c := &Client{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  bufio.NewReaderSize(stdout, 1<<16),
		pending: make(map[int]chan callResult),
		outSig:  make(chan struct{}, 1),
		closed:  make(chan struct{}),
		broken:  make(chan struct{}),
	}
	go c.readLoop()
	go c.writeLoop()
	return c
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
	return newClient(cmd, stdin, stdout), nil
}

// NewConnClient builds a Client that speaks LSP over an already-connected pair
// of streams (stdin to write requests, stdout to read replies).  Unlike Start
// it does not spawn a server process; it is used to attach to a server reached
// over existing pipes (and by tests).
func NewConnClient(stdin io.WriteCloser, stdout io.Reader) *Client {
	return newClient(nil, stdin, stdout)
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
	case <-c.broken:
		return nil, c.brokenErr
	case <-c.closed:
		return nil, ErrClientClosed
	}
}

// Notify queues a JSON-RPC notification (no reply expected).  It never blocks
// on the server: the message is handed to the writer goroutine and Notify
// returns immediately.  It returns ErrOutboundFull when the server is so far
// behind that the notification had to be dropped, ErrClientClosed after Close,
// or the connection error once the stream is broken.
func (c *Client) Notify(method string, params any) error {
	return c.send(-1, method, params)
}

// Close shuts down the LSP server.
//
// Ordering: the "exit" notification is queued behind everything already queued
// and Close waits (bounded by outboundFlushTimeout) for the queue to drain, so
// a healthy server sees a clean shutdown.  A wedged server costs at most
// outboundFlushTimeout + procExitTimeout; closing stdin unblocks the writer
// goroutine even when it is stuck mid-write, so nothing is leaked.
func (c *Client) Close() {
	c.once.Do(func() {
		_ = c.Notify("exit", nil)
		c.waitDrained(outboundFlushTimeout)

		c.outMu.Lock()
		c.outClosing = true
		c.outInFlight -= len(c.outQ)
		c.outQ = nil
		c.outMu.Unlock()

		close(c.closed)
		c.signalWriter()
		_ = c.stdin.Close()
		c.reapProcess()
	})
}

// reapProcess waits for the server process to exit, killing it if it outlives
// procExitTimeout.  A no-op for connection-backed clients (no process).
func (c *Client) reapProcess() {
	if c.cmd == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		_ = c.cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(procExitTimeout):
		if c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
		<-done
	}
}

// ---- outbound queue --------------------------------------------------------

// send queues a JSON-RPC message (request when id >= 0, notification when id
// is negative) for the writer goroutine.  It performs no I/O and never blocks.
func (c *Client) send(id int, method string, params any) error {
	return c.enqueue(outMsg{
		id:          id,
		method:      method,
		params:      params,
		coalesceKey: coalesceKeyFor(id, method, params),
	})
}

// enqueue appends m to the outbound FIFO, applying the coalescing and
// backpressure policy described on outboundSoftLimit.
func (c *Client) enqueue(m outMsg) error {
	if err := c.connErr(); err != nil {
		return err
	}

	c.outMu.Lock()
	if c.outClosing {
		c.outMu.Unlock()
		return ErrClientClosed
	}
	// Coalescing removes the superseded message instead of overwriting it in
	// place, so what the server receives is always a *subsequence* of what was
	// sent: messages are never reordered relative to each other.
	if m.coalesceKey != "" {
		for i, q := range c.outQ {
			if q.coalesceKey == m.coalesceKey {
				c.outQ = append(c.outQ[:i], c.outQ[i+1:]...)
				c.outCoalesced++
				c.outInFlight--
				break
			}
		}
	}
	limit := outboundHardLimit
	if m.id < 0 {
		limit = outboundSoftLimit
	}
	if len(c.outQ) >= limit {
		if m.id < 0 {
			c.outDropped++
		}
		c.outMu.Unlock()
		return ErrOutboundFull
	}
	c.outQ = append(c.outQ, m)
	c.outInFlight++
	c.outMu.Unlock()

	c.signalWriter()
	return nil
}

// signalWriter nudges the writer goroutine without ever blocking: outSig has
// capacity 1 and acts as a "work available" flag.
func (c *Client) signalWriter() {
	select {
	case c.outSig <- struct{}{}:
	default:
	}
}

// dequeue pops the head of the outbound FIFO.
func (c *Client) dequeue() (outMsg, bool) {
	c.outMu.Lock()
	defer c.outMu.Unlock()
	if len(c.outQ) == 0 {
		return outMsg{}, false
	}
	m := c.outQ[0]
	c.outQ = c.outQ[1:]
	return m, true
}

// writeLoop is the only goroutine that ever writes to the server's stdin.
//
// Ordering guarantee: messages are written strictly in the order they were
// enqueued (FIFO), one complete frame per io.Write, so didOpen/didChange/
// didClose and requests always reach the server in the order the editor issued
// them and no frame is ever interleaved with another.  The only messages that
// may be missing are notifications the queue deliberately coalesced or dropped
// (see outboundSoftLimit); nothing is ever reordered.
func (c *Client) writeLoop() {
	for {
		m, ok := c.dequeue()
		if !ok {
			select {
			case <-c.outSig:
				continue
			case <-c.closed:
				return
			case <-c.broken:
				return
			}
		}
		frame, err := frameMsg(m)
		if err != nil {
			// Unmarshallable params: drop this one message, keep the stream.
			c.finishWrite(false)
			continue
		}
		_, werr := c.stdin.Write(frame)
		c.finishWrite(true)
		if werr != nil {
			// The stream is unusable (server gone, or Close shut the pipe):
			// stop writing and fail every pending request.
			c.markBroken(fmt.Errorf("lsp: write: %w", werr))
			return
		}
	}
}

// finishWrite records that one dequeued message left the queue.  written
// distinguishes "handed to the pipe" from "discarded".
func (c *Client) finishWrite(written bool) {
	c.outMu.Lock()
	c.outInFlight--
	if written {
		c.outWritten++
	}
	c.outMu.Unlock()
}

// waitDrained blocks until every enqueued message has been written (or
// discarded), the connection breaks, or timeout elapses.
func (c *Client) waitDrained(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for {
		c.outMu.Lock()
		inFlight := c.outInFlight
		c.outMu.Unlock()
		if inFlight <= 0 {
			return
		}
		select {
		case <-c.broken:
			return
		default:
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(time.Millisecond)
	}
}

// outboundStats reports queue counters (queued frames, frames written,
// notifications dropped, notifications coalesced away).
func (c *Client) outboundStats() (queued, written, dropped, coalesced int) {
	c.outMu.Lock()
	defer c.outMu.Unlock()
	return len(c.outQ), c.outWritten, c.outDropped, c.outCoalesced
}

// frameMsg marshals m and prepends the LSP base-protocol header, returning one
// complete frame so it can be written with a single io.Write call.
func frameMsg(m outMsg) ([]byte, error) {
	msg := map[string]any{"jsonrpc": "2.0", "method": m.method}
	if m.id >= 0 {
		msg["id"] = m.id
	}
	if m.params != nil {
		msg["params"] = m.params
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	frame := make([]byte, 0, len(header)+len(body))
	frame = append(frame, header...)
	frame = append(frame, body...)
	return frame, nil
}

// coalesceKeyFor returns the supersede key for a message, or "" when the
// message must be delivered as sent.  Only textDocument/didChange
// notifications are coalescible: each carries the full document text, so a
// newer one for the same URI makes every older queued one redundant.  Requests
// are never coalesced.
func coalesceKeyFor(id int, method string, params any) string {
	if id >= 0 || method != methodDidChange {
		return ""
	}
	uri := paramsURI(params)
	if uri == "" {
		return ""
	}
	return method + "\x00" + uri
}

// paramsURI digs textDocument.uri out of a params map, or returns "".
func paramsURI(params any) string {
	m, ok := params.(map[string]any)
	if !ok {
		return ""
	}
	td, ok := m["textDocument"].(map[string]any)
	if !ok {
		return ""
	}
	uri, _ := td["uri"].(string)
	return uri
}

// connErr reports the reason the connection is unusable, or nil.
func (c *Client) connErr() error {
	select {
	case <-c.broken:
		if c.brokenErr != nil {
			return c.brokenErr
		}
		return ErrClientClosed
	default:
	}
	select {
	case <-c.closed:
		return ErrClientClosed
	default:
	}
	return nil
}

// markBroken records the first fatal connection error, wakes everyone waiting
// on the connection, and fails all in-flight requests.  Safe to call from both
// the reader and the writer goroutine.
func (c *Client) markBroken(err error) {
	c.brokenOnce.Do(func() {
		c.brokenErr = err
		close(c.broken)
	})
	c.failPending(err)
}

// failPending delivers err to every request still waiting for a reply.
func (c *Client) failPending(err error) {
	c.pendingMu.Lock()
	pending := c.pending
	c.pending = make(map[int]chan callResult)
	c.pendingMu.Unlock()
	for _, ch := range pending {
		select {
		case ch <- callResult{err: err}:
		default:
		}
	}
}

// ---- inbound ---------------------------------------------------------------

func (c *Client) readLoop() {
	defer func() {
		// The server will send no further replies; unblock pending requests
		// instead of leaving them to wait for their context or Close.
		c.markBroken(fmt.Errorf("lsp: connection closed"))
	}()
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
