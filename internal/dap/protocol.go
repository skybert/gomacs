package dap

import "encoding/json"

// Message is the DAP base protocol envelope.
// Type is one of "request", "response", or "event".
type Message struct {
	Seq        int             `json:"seq"`
	Type       string          `json:"type"`
	Command    string          `json:"command,omitempty"`
	RequestSeq int             `json:"request_seq,omitempty"`
	Success    bool            `json:"success,omitempty"`
	Message    string          `json:"message,omitempty"` // error message on failure
	Event      string          `json:"event,omitempty"`
	Body       json.RawMessage `json:"body,omitempty"`
	Arguments  json.RawMessage `json:"arguments,omitempty"`
}

// ---- Initialize ----

type InitializeArgs struct {
	ClientID                     string `json:"clientID,omitempty"`
	ClientName                   string `json:"clientName,omitempty"`
	AdapterID                    string `json:"adapterID"`
	PathFormat                   string `json:"pathFormat,omitempty"`
	LinesStartAt1                bool   `json:"linesStartAt1"`
	ColumnsStartAt1              bool   `json:"columnsStartAt1"`
	SupportsVariableType         bool   `json:"supportsVariableType,omitempty"`
	SupportsRunInTerminalRequest bool   `json:"supportsRunInTerminalRequest,omitempty"`
}

type InitializeResponse struct {
	SupportsConfigurationDoneRequest bool `json:"supportsConfigurationDoneRequest,omitempty"`
	SupportsFunctionBreakpoints      bool `json:"supportsFunctionBreakpoints,omitempty"`
	SupportsConditionalBreakpoints   bool `json:"supportsConditionalBreakpoints,omitempty"`
	SupportsEvaluateForHovers        bool `json:"supportsEvaluateForHovers,omitempty"`
	SupportsStepBack                 bool `json:"supportsStepBack,omitempty"`
}

// ---- Launch / Attach ----

// LaunchArgs is an open map so each adapter can define its own keys.
type LaunchArgs map[string]any

// ---- Breakpoints ----

type Source struct {
	Name string `json:"name,omitempty"`
	Path string `json:"path,omitempty"`
}

type SourceBreakpoint struct {
	Line int `json:"line"`
}

type SetBreakpointsArgs struct {
	Source      Source             `json:"source"`
	Breakpoints []SourceBreakpoint `json:"breakpoints"`
}

type Breakpoint struct {
	ID       int    `json:"id,omitempty"`
	Verified bool   `json:"verified"`
	Line     int    `json:"line,omitempty"`
	Message  string `json:"message,omitempty"`
}

type SetBreakpointsResponse struct {
	Breakpoints []Breakpoint `json:"breakpoints"`
}

// ---- Execution control ----

type ContinueArgs struct {
	ThreadID int `json:"threadId"`
}

type NextArgs struct {
	ThreadID int `json:"threadId"`
}

type StepInArgs struct {
	ThreadID int `json:"threadId"`
}

type StepOutArgs struct {
	ThreadID int `json:"threadId"`
}

type DisconnectArgs struct {
	TerminateDebuggee bool `json:"terminateDebuggee,omitempty"`
}

// ---- Stack / Scopes / Variables ----

type StackTraceArgs struct {
	ThreadID   int `json:"threadId"`
	StartFrame int `json:"startFrame,omitempty"`
	Levels     int `json:"levels,omitempty"`
}

type StackFrame struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Source Source `json:"source,omitempty"`
	Line   int    `json:"line"`
	Column int    `json:"column,omitempty"`
}

type StackTraceResponse struct {
	StackFrames []StackFrame `json:"stackFrames"`
	TotalFrames int          `json:"totalFrames,omitempty"`
}

type ScopesArgs struct {
	FrameID int `json:"frameId"`
}

type Scope struct {
	Name               string `json:"name"`
	VariablesReference int    `json:"variablesReference"`
	Expensive          bool   `json:"expensive,omitempty"`
}

type ScopesResponse struct {
	Scopes []Scope `json:"scopes"`
}

type VariablesArgs struct {
	VariablesReference int `json:"variablesReference"`
}

type Variable struct {
	Name               string `json:"name"`
	Value              string `json:"value"`
	Type               string `json:"type,omitempty"`
	VariablesReference int    `json:"variablesReference"`
}

type VariablesResponse struct {
	Variables []Variable `json:"variables"`
}

// ---- Evaluate ----

type EvaluateArgs struct {
	Expression string `json:"expression"`
	FrameID    int    `json:"frameId,omitempty"`
	Context    string `json:"context,omitempty"` // "watch", "repl", "hover"
}

type EvaluateResponse struct {
	Result             string `json:"result"`
	Type               string `json:"type,omitempty"`
	VariablesReference int    `json:"variablesReference,omitempty"`
}

// ---- Threads ----

type Thread struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type ThreadsResponse struct {
	Threads []Thread `json:"threads"`
}

// ---- Events ----

type StoppedEvent struct {
	Reason            string `json:"reason"`
	Description       string `json:"description,omitempty"`
	ThreadID          int    `json:"threadId,omitempty"`
	AllThreadsStopped bool   `json:"allThreadsStopped,omitempty"`
	Text              string `json:"text,omitempty"`
}

type ContinuedEvent struct {
	ThreadID            int  `json:"threadId"`
	AllThreadsContinued bool `json:"allThreadsContinued,omitempty"`
}

type ExitedEvent struct {
	ExitCode int `json:"exitCode"`
}

type OutputEvent struct {
	Category string `json:"category,omitempty"` // "console", "stdout", "stderr", "telemetry"
	Output   string `json:"output"`
}

type ThreadEvent struct {
	ThreadID int    `json:"threadId"`
	Reason   string `json:"reason"` // "started", "exited"
}
