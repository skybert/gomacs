package editor

import (
	"encoding/json"

	"github.com/skybert/gomacs/internal/dap"
)

// debugBackend abstracts the debug adapter protocol so that editor-level
// command code is not coupled to DAP specifics.  A concrete implementation
// (dapBackend) talks the DAP wire protocol; future backends could use GDB/MI,
// LLDB, or others.
//
// All methods that send network requests execute synchronously in the caller's
// goroutine (usually an e.dapAsync worker goroutine); the Editor's dapAsync /
// dapCbs machinery is responsible for bouncing results back to the main loop.
type debugBackend interface {
	// Stepping / execution.
	Continue(threadID int) error
	StepNext(threadID int) error
	StepIn(threadID int) error
	StepOut(threadID int) error

	// Evaluate an expression.  If frameID==0 and stoppedThread!=0 the
	// implementation should fetch the top frame first.
	Evaluate(expr string, frameID, stoppedThread int, context string) (string, error)

	// Breakpoints.
	SetBreakpoints(file string, lines []int) error

	// Lifecycle — called from the main goroutine.
	Close()
}

// dapBackend implements debugBackend using the DAP wire protocol.
type dapBackend struct {
	client *dap.Client
}

func (b *dapBackend) Continue(threadID int) error {
	_, err := b.client.Request("continue", dap.ContinueArgs{ThreadID: threadID})
	return err
}

func (b *dapBackend) StepNext(threadID int) error {
	_, err := b.client.Request("next", dap.NextArgs{ThreadID: threadID})
	return err
}

func (b *dapBackend) StepIn(threadID int) error {
	_, err := b.client.Request("stepIn", dap.StepInArgs{ThreadID: threadID})
	return err
}

func (b *dapBackend) StepOut(threadID int) error {
	_, err := b.client.Request("stepOut", dap.StepOutArgs{ThreadID: threadID})
	return err
}

func (b *dapBackend) Evaluate(expr string, frameID, stoppedThread int, context string) (string, error) {
	// If we don't have a frame yet, fetch the top one first.
	if frameID == 0 && stoppedThread != 0 {
		raw, err := b.client.Request("stackTrace", dap.StackTraceArgs{
			ThreadID: stoppedThread,
			Levels:   1,
		})
		if err == nil {
			var resp dap.StackTraceResponse
			if jerr := json.Unmarshal(raw, &resp); jerr == nil && len(resp.StackFrames) > 0 {
				frameID = resp.StackFrames[0].ID
			}
		}
	}
	raw, err := b.client.Request("evaluate", dap.EvaluateArgs{
		Expression: expr,
		FrameID:    frameID,
		Context:    context,
	})
	if err != nil {
		return "", err
	}
	var resp dap.EvaluateResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	return resp.Result, nil
}

func (b *dapBackend) SetBreakpoints(file string, lines []int) error {
	bps := make([]dap.SourceBreakpoint, len(lines))
	for i, l := range lines {
		bps[i] = dap.SourceBreakpoint{Line: l}
	}
	_, err := b.client.Request("setBreakpoints", dap.SetBreakpointsArgs{
		Source:      dap.Source{Path: file},
		Breakpoints: bps,
	})
	return err
}

func (b *dapBackend) Close() {
	b.client.Close()
}
