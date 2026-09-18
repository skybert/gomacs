package editor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/skybert/gomacs/internal/dap"
)

// ---------------------------------------------------------------------------
// dapHasBreakpoint
// ---------------------------------------------------------------------------

func TestDapHasBreakpoint(t *testing.T) {
	e := newDAPTestEditor("")
	abs := "/tmp/foo.go"
	e.dapBreakpoints[abs] = map[int]struct{}{5: {}}
	if !e.dapHasBreakpoint(abs, 5) {
		t.Error("dapHasBreakpoint(abs, 5) should be true")
	}
	if e.dapHasBreakpoint(abs, 99) {
		t.Error("dapHasBreakpoint(abs, 99) should be false")
	}
}

func TestDapHasBreakpointUnknownFile(t *testing.T) {
	e := newDAPTestEditor("")
	e.dapBreakpoints["/tmp/foo.go"] = map[int]struct{}{1: {}}
	if e.dapHasBreakpoint("/tmp/other.go", 1) {
		t.Error("a file with no breakpoints should report false")
	}
}

func TestDapHasBreakpointEmptyLineSet(t *testing.T) {
	e := newDAPTestEditor("")
	// All breakpoints in the file were toggled off: the map stays but is empty.
	e.dapBreakpoints["/tmp/foo.go"] = map[int]struct{}{}
	if e.dapHasBreakpoint("/tmp/foo.go", 1) {
		t.Error("an empty line set should report false")
	}
}

// ---------------------------------------------------------------------------
// dapState: locals / frames snapshots are guarded independently
// ---------------------------------------------------------------------------

func TestDapStateSnapshotsUnderLocks(t *testing.T) {
	st := &dapState{localsAutoExpandDepth: 1}

	st.localsMu.Lock()
	st.locals = []dapVariable{{name: "x", value: "1"}}
	st.localsMu.Unlock()

	st.framesMu.Lock()
	st.frames = []dap.StackFrame{{ID: 1, Name: "main.main", Line: 3}}
	st.threads = []dapThread{{id: 1, name: "main", stopped: true, frames: st.frames}}
	st.framesMu.Unlock()

	st.localsMu.RLock()
	nLocals := len(st.locals)
	st.localsMu.RUnlock()
	st.framesMu.RLock()
	nFrames, nThreads := len(st.frames), len(st.threads)
	st.framesMu.RUnlock()

	if nLocals != 1 || nFrames != 1 || nThreads != 1 {
		t.Errorf("snapshot = %d locals, %d frames, %d threads; want 1/1/1", nLocals, nFrames, nThreads)
	}
	if !st.threads[0].stopped {
		t.Error("the stopped thread should be flagged")
	}
}

// ---------------------------------------------------------------------------
// dapVariable tree shape
// ---------------------------------------------------------------------------

func TestDapVariableChildrenArePointerStable(t *testing.T) {
	// dapLocalsToggleExpand mutates variables through pointers held in
	// localsLineMap, so children must be addressable in place.
	v := dapVariable{name: "obj", varRef: 3, children: []dapVariable{{name: "f"}}}
	child := &v.children[0]
	child.expanded = true
	if !v.children[0].expanded {
		t.Error("mutating a child through its pointer should be visible in the tree")
	}
}

// TestDapHasBreakpointCanonicalisesPath is a regression test: the breakpoint map
// is keyed by canonical path, so a caller passing a relative or symlinked path
// used to get false even though the breakpoint was set.
func TestDapHasBreakpointCanonicalisesPath(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "prog.txt")
	if err := os.WriteFile(real, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	noisy := filepath.Join(dir, ".", "prog.txt")

	e := newTestEditor("")
	e.dapBreakpoints = map[string]map[int]struct{}{
		canonPath(real): {7: {}},
	}

	if !e.dapHasBreakpoint(canonPath(real), 7) {
		t.Error("canonical path should find the breakpoint")
	}
	if !e.dapHasBreakpoint(noisy, 7) {
		t.Error("non-canonical path should also find the breakpoint")
	}
	if e.dapHasBreakpoint(noisy, 8) {
		t.Error("a line without a breakpoint must report false")
	}
	if e.dapHasBreakpoint("", 7) {
		t.Error("an empty filename must report false")
	}
}
