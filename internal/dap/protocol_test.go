package dap

import (
	"encoding/json"
	"reflect"
	"testing"
)

// roundTrip marshals want, unmarshals the result into a fresh value of the
// same type, and asserts the result deep-equals want. This is the core check
// for every wire type below: it catches typo'd json tags (a misspelled tag
// on the encode side would still decode back onto the wrong field or be
// dropped entirely, breaking the round trip) as well as any accidental
// asymmetry between encoding and decoding.
func roundTrip[T any](t *testing.T, want T) {
	t.Helper()
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal(%#v): %v", want, err)
	}
	var got T
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal(%s): %v", data, err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Errorf("round trip mismatch for %T\n got  %#v\nwant  %#v\n(json: %s)", want, got, want, data)
	}
}

// wantJSON marshals v and asserts the exact wire form, byte for byte. Real
// DAP adapters (e.g. dlv dap) expect these exact field names, so this pins
// the wire format rather than just checking that Go can read back what it
// wrote.
func wantJSON(t *testing.T, v any, want string) {
	t.Helper()
	got, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal(%#v): %v", v, err)
	}
	if string(got) != want {
		t.Errorf("Marshal(%#v) = %s, want %s", v, got, want)
	}
}

// ---- Message ----

func TestMessage_RoundTrip(t *testing.T) {
	roundTrip(t, Message{
		Seq:        1,
		Type:       "request",
		Command:    "initialize",
		RequestSeq: 0,
		Success:    false,
		Message:    "",
		Event:      "",
		Body:       nil,
		Arguments:  json.RawMessage(`{"adapterID":"gomacs"}`),
	})
	roundTrip(t, Message{
		Seq:        2,
		Type:       "response",
		Command:    "initialize",
		RequestSeq: 1,
		Success:    true,
		Body:       json.RawMessage(`{"supportsConfigurationDoneRequest":true}`),
	})
	roundTrip(t, Message{
		Seq:   3,
		Type:  "event",
		Event: "stopped",
		Body:  json.RawMessage(`{"reason":"breakpoint"}`),
	})
}

func TestMessage_ZeroValue(t *testing.T) {
	roundTrip(t, Message{})
}

// TestMessage_WireFormat pins the exact field names DAP requires, matching
// what a real adapter (dlv dap) sends and expects. In particular Command,
// RequestSeq, Success, Message, Event, Body and Arguments must all be
// omitted (via omitempty) when zero, since request and response/event
// messages populate disjoint subsets of these fields.
func TestMessage_WireFormat(t *testing.T) {
	wantJSON(t, Message{Seq: 1, Type: "request", Command: "next"},
		`{"seq":1,"type":"request","command":"next"}`)

	wantJSON(t, Message{Seq: 2, Type: "response", RequestSeq: 1, Command: "next", Success: true},
		`{"seq":2,"type":"response","command":"next","request_seq":1,"success":true}`)

	wantJSON(t, Message{Seq: 3, Type: "response", RequestSeq: 1, Command: "next", Success: false, Message: "boom"},
		`{"seq":3,"type":"response","command":"next","request_seq":1,"message":"boom"}`)

	wantJSON(t, Message{Seq: 4, Type: "event", Event: "terminated"},
		`{"seq":4,"type":"event","event":"terminated"}`)
}

func TestMessage_UnmarshalFromRealAdapterShape(t *testing.T) {
	// Shape as actually emitted by dlv dap for a failed request.
	raw := `{"seq":7,"type":"response","request_seq":3,"command":"evaluate","success":false,"message":"could not find symbol value for x"}`
	var msg Message
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	want := Message{Seq: 7, Type: "response", RequestSeq: 3, Command: "evaluate", Success: false, Message: "could not find symbol value for x"}
	if !reflect.DeepEqual(msg, want) {
		t.Errorf("got %#v, want %#v", msg, want)
	}
}

// ---- Initialize ----

func TestInitializeArgs_RoundTrip(t *testing.T) {
	roundTrip(t, InitializeArgs{
		ClientID:                     "gomacs",
		ClientName:                   "gomacs",
		AdapterID:                    "go",
		PathFormat:                   "path",
		LinesStartAt1:                true,
		ColumnsStartAt1:              true,
		SupportsVariableType:         true,
		SupportsRunInTerminalRequest: false,
	})
	roundTrip(t, InitializeArgs{})
}

func TestInitializeArgs_WireFormat(t *testing.T) {
	// Only AdapterID, LinesStartAt1 and ColumnsStartAt1 lack omitempty, so a
	// minimal args value should serialize with exactly those three keys
	// (plus whatever bools happen to be true).
	wantJSON(t, InitializeArgs{AdapterID: "go"},
		`{"adapterID":"go","linesStartAt1":false,"columnsStartAt1":false}`)

	wantJSON(t, InitializeArgs{
		ClientID:        "gomacs",
		AdapterID:       "go",
		LinesStartAt1:   true,
		ColumnsStartAt1: true,
	}, `{"clientID":"gomacs","adapterID":"go","linesStartAt1":true,"columnsStartAt1":true}`)
}

func TestInitializeResponse_RoundTrip(t *testing.T) {
	roundTrip(t, InitializeResponse{
		SupportsConfigurationDoneRequest: true,
		SupportsFunctionBreakpoints:      true,
		SupportsConditionalBreakpoints:   true,
		SupportsEvaluateForHovers:        true,
		SupportsStepBack:                 true,
	})
	roundTrip(t, InitializeResponse{})
}

func TestInitializeResponse_ZeroValueOmitsAllFields(t *testing.T) {
	wantJSON(t, InitializeResponse{}, `{}`)
}

func TestInitializeResponse_UnmarshalFromRealAdapterShape(t *testing.T) {
	// Subset of the capabilities dlv dap actually reports.
	raw := `{"supportsConfigurationDoneRequest":true,"supportsConditionalBreakpoints":true}`
	var caps InitializeResponse
	if err := json.Unmarshal([]byte(raw), &caps); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	want := InitializeResponse{SupportsConfigurationDoneRequest: true, SupportsConditionalBreakpoints: true}
	if caps != want {
		t.Errorf("got %#v, want %#v", caps, want)
	}
}

// ---- Launch / Attach ----

func TestLaunchArgs_RoundTrip(t *testing.T) {
	// Use only JSON-stable types (string/bool/nested map) so unmarshalling
	// back into map[string]any reproduces the same values (numeric literals
	// would decode as float64, which is a different, but expected, quirk
	// covered separately below).
	roundTrip(t, LaunchArgs{
		"program": "/tmp/main",
		"stopOnEntry": map[string]any{
			"enabled": true,
		},
		"args": []any{"--flag"},
	})
	roundTrip(t, LaunchArgs{})
	roundTrip(t, LaunchArgs(nil))
}

// TestLaunchArgs_NumbersDecodeAsFloat64 pins a real gotcha: LaunchArgs is an
// open map (each adapter defines its own launch keys), so encoding/json
// decodes any JSON number into float64, never back into int, even though it
// was marshalled from an int.
func TestLaunchArgs_NumbersDecodeAsFloat64(t *testing.T) {
	args := LaunchArgs{"port": 1234}
	data, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got LaunchArgs
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	port, ok := got["port"].(float64)
	if !ok {
		t.Fatalf("got[\"port\"] = %#v (%T), want float64", got["port"], got["port"])
	}
	if port != 1234 {
		t.Errorf("port = %v, want 1234", port)
	}
}

// ---- Breakpoints ----

func TestSource_RoundTrip(t *testing.T) {
	roundTrip(t, Source{Name: "main.go", Path: "/tmp/main.go"})
	roundTrip(t, Source{})
}

func TestSource_ZeroValueOmitsAllFields(t *testing.T) {
	wantJSON(t, Source{}, `{}`)
}

func TestSourceBreakpoint_RoundTrip(t *testing.T) {
	roundTrip(t, SourceBreakpoint{Line: 42})
	roundTrip(t, SourceBreakpoint{})
}

func TestSourceBreakpoint_LineHasNoOmitempty(t *testing.T) {
	// Line lacks omitempty, so a breakpoint at line 0 (which would never
	// happen in practice, but is the zero value) must still be sent
	// explicitly rather than silently dropped.
	wantJSON(t, SourceBreakpoint{}, `{"line":0}`)
}

func TestSetBreakpointsArgs_RoundTrip(t *testing.T) {
	roundTrip(t, SetBreakpointsArgs{
		Source:      Source{Path: "/tmp/main.go"},
		Breakpoints: []SourceBreakpoint{{Line: 10}, {Line: 20}},
	})
	roundTrip(t, SetBreakpointsArgs{})
}

func TestBreakpoint_RoundTrip(t *testing.T) {
	roundTrip(t, Breakpoint{ID: 1, Verified: true, Line: 10, Message: ""})
	roundTrip(t, Breakpoint{Verified: false, Message: "invalid line"})
	roundTrip(t, Breakpoint{})
}

func TestBreakpoint_WireFormat(t *testing.T) {
	// Verified lacks omitempty (an unverified breakpoint at the zero value
	// must still report verified:false explicitly); ID, Line and Message do
	// have omitempty.
	wantJSON(t, Breakpoint{}, `{"verified":false}`)
	wantJSON(t, Breakpoint{ID: 3, Verified: true, Line: 5},
		`{"id":3,"verified":true,"line":5}`)
}

func TestSetBreakpointsResponse_RoundTrip(t *testing.T) {
	roundTrip(t, SetBreakpointsResponse{Breakpoints: []Breakpoint{{ID: 1, Verified: true, Line: 10}}})
	roundTrip(t, SetBreakpointsResponse{})
}

func TestSetBreakpointsResponse_EmptyBreakpointsIsNullNotOmitted(t *testing.T) {
	// Breakpoints lacks omitempty, so a nil slice marshals as "breakpoints":null
	// rather than being dropped from the response.
	wantJSON(t, SetBreakpointsResponse{}, `{"breakpoints":null}`)
}

// ---- Execution control ----

func TestExecutionControlArgs_RoundTrip(t *testing.T) {
	roundTrip(t, ContinueArgs{ThreadID: 1})
	roundTrip(t, NextArgs{ThreadID: 2})
	roundTrip(t, StepInArgs{ThreadID: 3})
	roundTrip(t, StepOutArgs{ThreadID: 4})
}

func TestExecutionControlArgs_WireFormat(t *testing.T) {
	wantJSON(t, ContinueArgs{ThreadID: 1}, `{"threadId":1}`)
	wantJSON(t, NextArgs{ThreadID: 1}, `{"threadId":1}`)
	wantJSON(t, StepInArgs{ThreadID: 1}, `{"threadId":1}`)
	wantJSON(t, StepOutArgs{ThreadID: 1}, `{"threadId":1}`)
}

func TestDisconnectArgs_RoundTrip(t *testing.T) {
	roundTrip(t, DisconnectArgs{TerminateDebuggee: true})
	roundTrip(t, DisconnectArgs{})
}

func TestDisconnectArgs_ZeroValueOmitsField(t *testing.T) {
	wantJSON(t, DisconnectArgs{}, `{}`)
}

// ---- Stack / Scopes / Variables ----

func TestStackTraceArgs_RoundTrip(t *testing.T) {
	roundTrip(t, StackTraceArgs{ThreadID: 1, StartFrame: 0, Levels: 20})
	roundTrip(t, StackTraceArgs{ThreadID: 1})
}

func TestStackFrame_RoundTrip(t *testing.T) {
	roundTrip(t, StackFrame{
		ID:     1,
		Name:   "main.main",
		Source: Source{Path: "/tmp/main.go"},
		Line:   10,
		Column: 2,
	})
	roundTrip(t, StackFrame{ID: 1, Name: "main.main", Line: 10})
}

// TestStackFrame_SourceOmitemptyIsIneffective documents a real
// encoding/json limitation: omitempty only suppresses zero values for
// basic types, slices, maps and pointers — never for a struct-typed field
// like Source. So an empty Source still serializes as "source":{} even
// though the tag says omitempty. This is intentional/acceptable for DAP
// (adapters ignore an empty source object), but worth pinning so nobody
// "fixes" the tag expecting it to start working.
func TestStackFrame_SourceOmitemptyIsIneffective(t *testing.T) {
	wantJSON(t, StackFrame{ID: 1, Name: "main.main", Line: 10},
		`{"id":1,"name":"main.main","source":{},"line":10}`)
}

func TestStackTraceResponse_RoundTrip(t *testing.T) {
	roundTrip(t, StackTraceResponse{
		StackFrames: []StackFrame{{ID: 1, Name: "main.main", Line: 10}},
		TotalFrames: 1,
	})
	roundTrip(t, StackTraceResponse{})
}

func TestScopesArgs_RoundTrip(t *testing.T) {
	roundTrip(t, ScopesArgs{FrameID: 1})
}

func TestScope_RoundTrip(t *testing.T) {
	roundTrip(t, Scope{Name: "Locals", VariablesReference: 1, Expensive: false})
	roundTrip(t, Scope{Name: "Globals", VariablesReference: 2, Expensive: true})
}

func TestScopesResponse_RoundTrip(t *testing.T) {
	roundTrip(t, ScopesResponse{Scopes: []Scope{{Name: "Locals", VariablesReference: 1}}})
	roundTrip(t, ScopesResponse{})
}

func TestVariablesArgs_RoundTrip(t *testing.T) {
	roundTrip(t, VariablesArgs{VariablesReference: 5})
}

func TestVariable_RoundTrip(t *testing.T) {
	roundTrip(t, Variable{Name: "x", Value: "42", Type: "int", VariablesReference: 0})
	roundTrip(t, Variable{Name: "s", Value: "&{...}", VariablesReference: 7})
}

func TestVariable_VariablesReferenceHasNoOmitempty(t *testing.T) {
	// A leaf variable (no children) has VariablesReference 0, which must
	// still be sent explicitly since it lacks omitempty.
	wantJSON(t, Variable{Name: "x", Value: "42"}, `{"name":"x","value":"42","variablesReference":0}`)
}

func TestVariablesResponse_RoundTrip(t *testing.T) {
	roundTrip(t, VariablesResponse{Variables: []Variable{{Name: "x", Value: "42"}}})
	roundTrip(t, VariablesResponse{})
}

// ---- Evaluate ----

func TestEvaluateArgs_RoundTrip(t *testing.T) {
	roundTrip(t, EvaluateArgs{Expression: "x+1", FrameID: 2, Context: "watch"})
	roundTrip(t, EvaluateArgs{Expression: "x"})
}

func TestEvaluateResponse_RoundTrip(t *testing.T) {
	roundTrip(t, EvaluateResponse{Result: "42", Type: "int", VariablesReference: 0})
	roundTrip(t, EvaluateResponse{Result: "&{...}", VariablesReference: 3})
}

// ---- Threads ----

func TestThread_RoundTrip(t *testing.T) {
	roundTrip(t, Thread{ID: 1, Name: "main"})
	roundTrip(t, Thread{})
}

func TestThreadsResponse_RoundTrip(t *testing.T) {
	roundTrip(t, ThreadsResponse{Threads: []Thread{{ID: 1, Name: "main"}}})
	roundTrip(t, ThreadsResponse{})
}

// ---- Events ----

func TestStoppedEvent_RoundTrip(t *testing.T) {
	roundTrip(t, StoppedEvent{
		Reason:            "breakpoint",
		Description:       "Paused on breakpoint",
		ThreadID:          1,
		AllThreadsStopped: true,
		Text:              "",
	})
	roundTrip(t, StoppedEvent{Reason: "step"})
}

func TestStoppedEvent_WireFormat(t *testing.T) {
	wantJSON(t, StoppedEvent{Reason: "pause"}, `{"reason":"pause"}`)
	wantJSON(t, StoppedEvent{Reason: "breakpoint", ThreadID: 1, AllThreadsStopped: true},
		`{"reason":"breakpoint","threadId":1,"allThreadsStopped":true}`)
}

func TestContinuedEvent_RoundTrip(t *testing.T) {
	roundTrip(t, ContinuedEvent{ThreadID: 1, AllThreadsContinued: true})
	roundTrip(t, ContinuedEvent{ThreadID: 1})
}

func TestExitedEvent_RoundTrip(t *testing.T) {
	roundTrip(t, ExitedEvent{ExitCode: 0})
	roundTrip(t, ExitedEvent{ExitCode: 1})
}

func TestExitedEvent_ExitCodeHasNoOmitempty(t *testing.T) {
	// A successful exit (code 0) must still be reported explicitly.
	wantJSON(t, ExitedEvent{}, `{"exitCode":0}`)
}

func TestOutputEvent_RoundTrip(t *testing.T) {
	roundTrip(t, OutputEvent{Category: "stdout", Output: "hello\n"})
	roundTrip(t, OutputEvent{Output: "no category"})
}

func TestOutputEvent_OutputHasNoOmitempty(t *testing.T) {
	wantJSON(t, OutputEvent{}, `{"output":""}`)
}

func TestThreadEvent_RoundTrip(t *testing.T) {
	roundTrip(t, ThreadEvent{ThreadID: 1, Reason: "started"})
	roundTrip(t, ThreadEvent{ThreadID: 1, Reason: "exited"})
}

func TestThreadEvent_WireFormat(t *testing.T) {
	// Neither field has omitempty, so both are always present, matching
	// what dlv dap actually emits for thread lifecycle events.
	wantJSON(t, ThreadEvent{}, `{"threadId":0,"reason":""}`)
}

// ---- Body/Arguments embedding, as actually used via Client.dispatch/send ----

func TestMessage_BodyRoundTripsArbitraryPayload(t *testing.T) {
	// Body and Arguments are raw JSON blobs decoded a second time by
	// callers (see client.go's dispatch/Request); confirm that indirection
	// survives a full Message round trip.
	inner := StoppedEvent{Reason: "breakpoint", ThreadID: 2}
	innerJSON, err := json.Marshal(inner)
	if err != nil {
		t.Fatalf("Marshal(inner): %v", err)
	}

	msg := Message{Seq: 5, Type: "event", Event: "stopped", Body: innerJSON}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal(msg): %v", err)
	}

	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal(msg): %v", err)
	}

	var gotInner StoppedEvent
	if err := json.Unmarshal(decoded.Body, &gotInner); err != nil {
		t.Fatalf("Unmarshal(decoded.Body): %v", err)
	}
	if gotInner != inner {
		t.Errorf("got %#v, want %#v", gotInner, inner)
	}
}
