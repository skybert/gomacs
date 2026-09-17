package editor

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/dap"
	"github.com/skybert/gomacs/internal/terminal"
)

// dapRefreshPanels updates the Locals and Stack panel buffers with the latest
// fetched data.  Called on the main goroutine after a stopped event.
func (e *Editor) dapRefreshPanels() {
	if e.dap == nil {
		return
	}
	dapRenderLocals(e)
	dapRenderStack(e)
}

// dapRenderLocals writes the local variables tree to the Locals buffer and
// rebuilds e.dap.localsLineMap so that dapLocalsToggleExpand can map a line
// number back to the exact variable in the tree.
func dapRenderLocals(e *Editor) {
	if e.dap == nil || e.dap.localsBuf == nil {
		return
	}
	buf := e.dap.localsBuf
	buf.SetReadOnly(false)
	buf.Delete(0, buf.Len())

	e.dap.localsMu.RLock()
	vars := e.dap.locals
	e.dap.localsMu.RUnlock()

	var sb strings.Builder
	var lineMap []*dapVariable
	for i := range vars {
		dapWriteVariable(&sb, &vars[i], 0, &lineMap)
	}
	if sb.Len() == 0 {
		sb.WriteString("(no locals)\n")
	}
	buf.InsertString(0, sb.String())
	buf.SetPoint(0)
	buf.SetReadOnly(true)

	e.dap.localsLineMap = lineMap
}

func dapWriteVariable(sb *strings.Builder, v *dapVariable, depth int, lineMap *[]*dapVariable) {
	if lineMap != nil {
		*lineMap = append(*lineMap, v)
	}
	indent := strings.Repeat("  ", v.depth+depth)
	arrow := " "
	if v.varRef != 0 {
		if v.expanded {
			arrow = "▼"
		} else {
			arrow = "▶"
		}
	}
	if v.typeStr != "" {
		fmt.Fprintf(sb, "%s%s %s %s = %s\n", indent, arrow, v.name, v.typeStr, v.value)
	} else {
		fmt.Fprintf(sb, "%s%s %s = %s\n", indent, arrow, v.name, v.value)
	}
	if v.expanded {
		for i := range v.children {
			dapWriteVariable(sb, &v.children[i], depth, lineMap)
		}
	}
}

// dapFrameFile returns the path to show for a stack frame, falling back to the
// source's bare name when the adapter reported no path.
func dapFrameFile(f dap.StackFrame) string {
	if f.Source.Path != "" {
		return f.Source.Path
	}
	return f.Source.Name
}

// dapRenderStack writes the call stack of every thread to the Stack buffer and
// rebuilds e.dap.stackLineMap so that dapStackJumpToFrame can map a line back to
// its frame.  Threads are grouped under a header line, the stopped thread first
// and marked with an arrow.  Frame lines stay unindented so that
// syntax.DapStackHighlighter still recognises them.
func dapRenderStack(e *Editor) {
	if e.dap == nil || e.dap.stackBuf == nil {
		return
	}
	buf := e.dap.stackBuf
	buf.SetReadOnly(false)
	buf.Delete(0, buf.Len())

	e.dap.framesMu.RLock()
	threads := e.dap.threads
	frames := e.dap.frames
	e.dap.framesMu.RUnlock()

	var sb strings.Builder
	var lineMap []*dap.StackFrame
	switch {
	case len(threads) > 0:
		for ti := range threads {
			th := &threads[ti]
			marker := " "
			if th.stopped {
				marker = "→"
			}
			name := th.name
			if name == "" {
				name = "(unnamed)"
			}
			fmt.Fprintf(&sb, "%s Thread %d: %s\n", marker, th.id, name)
			lineMap = append(lineMap, nil)
			for fi := range th.frames {
				f := &th.frames[fi]
				fmt.Fprintf(&sb, "#%d  %s (%s:%d)\n", fi, f.Name, dapFrameFile(*f), f.Line)
				lineMap = append(lineMap, f)
			}
			if len(th.frames) == 0 {
				sb.WriteString("(no frames)\n")
				lineMap = append(lineMap, nil)
			}
		}
	case len(frames) > 0:
		// No thread list (adapter without a threads request, or state set up
		// directly): show the stopped thread's frames on their own.
		for i := range frames {
			f := &frames[i]
			fmt.Fprintf(&sb, "#%d  %s (%s:%d)\n", i, f.Name, dapFrameFile(*f), f.Line)
			lineMap = append(lineMap, f)
		}
	default:
		sb.WriteString("(no stack)\n")
		lineMap = append(lineMap, nil)
	}
	buf.InsertString(0, sb.String())
	buf.SetPoint(0)
	buf.SetReadOnly(true)

	e.dap.stackLineMap = lineMap
}

// debugLocalsDispatch handles key events in the *Debug Locals* buffer.
func (e *Editor) debugLocalsDispatch(ke terminal.KeyEvent) bool {
	if ke.Key == tcell.KeyRune && ke.Mod == 0 {
		switch ke.Rune {
		case 'q':
			if e.dap != nil && e.dap.prevActiveWin != nil {
				e.activeWin = e.dap.prevActiveWin
			}
			return true

		case 'n':
			e.cmdNextLine()
			return true

		case 'p':
			e.cmdPreviousLine()
			return true
		}
	}

	// Enter, Right: expand; Left, Backspace: collapse.
	switch ke.Key {
	case tcell.KeyEnter, tcell.KeyRight:
		e.dapLocalsToggleExpand(true)
		return true
	case tcell.KeyLeft, tcell.KeyBackspace:
		e.dapLocalsToggleExpand(false)
		return true
	case tcell.KeyUp:
		e.cmdPreviousLine()
		return true
	case tcell.KeyDown:
		e.cmdNextLine()
		return true
	}
	return false
}

// dapLocalsToggleExpand expands (expand=true) or collapses the variable on the
// current line in the locals buffer.  Uses localsLineMap to find the correct
// variable even when tree nodes above it are expanded.
func (e *Editor) dapLocalsToggleExpand(expand bool) {
	if e.dap == nil || e.dap.localsBuf == nil || e.dap.client == nil {
		return
	}
	buf := e.dap.localsBuf
	line, _ := buf.LineCol(buf.Point())
	// line is 1-based; map to zero-based lineMap index.
	lineIdx := line - 1
	if lineIdx < 0 || lineIdx >= len(e.dap.localsLineMap) {
		return
	}
	v := e.dap.localsLineMap[lineIdx] // pointer into the live tree

	if v.varRef == 0 {
		return // leaf — nothing to expand
	}

	if expand {
		if !v.expanded || len(v.children) == 0 {
			ref := v.varRef
			maxDepth := e.dap.localsAutoExpandDepth
			client := e.dap.client
			// Capture a pointer-stable reference: v already points into the tree.
			varPtr := v
			e.dapAsync(func() func() {
				children := dapFetchVars(client, ref, 1, maxDepth)
				return func() {
					if e.dap == nil {
						return
					}
					e.dap.localsMu.Lock()
					varPtr.expanded = true
					varPtr.children = children
					e.dap.localsMu.Unlock()
					dapRenderLocals(e)
				}
			})
			return
		}
		e.dap.localsMu.Lock()
		v.expanded = true
		e.dap.localsMu.Unlock()
	} else {
		e.dap.localsMu.Lock()
		v.expanded = false
		e.dap.localsMu.Unlock()
	}
	dapRenderLocals(e)
}

// debugStackDispatch handles key events in the *Debug Stack* buffer.
func (e *Editor) debugStackDispatch(ke terminal.KeyEvent) bool {
	if ke.Key == tcell.KeyRune && ke.Mod == 0 {
		switch ke.Rune {
		case 'q':
			if e.dap != nil && e.dap.prevActiveWin != nil {
				e.activeWin = e.dap.prevActiveWin
			}
			return true

		case 'n':
			e.cmdNextLine()
			return true

		case 'p':
			e.cmdPreviousLine()
			return true
		}
	}

	switch ke.Key {
	case tcell.KeyEnter:
		e.dapStackJumpToFrame()
		return true
	case tcell.KeyUp:
		e.cmdPreviousLine()
		return true
	case tcell.KeyDown:
		e.cmdNextLine()
		return true
	}
	return false
}

// dapStackJumpToFrame opens the source file for the frame on the current line
// and scrolls to that frame's line.  Thread header lines have no frame and are
// ignored.
func (e *Editor) dapStackJumpToFrame() {
	if e.dap == nil {
		return
	}
	buf := e.dap.stackBuf
	if buf == nil {
		return
	}
	line, _ := buf.LineCol(buf.Point())
	lineIdx := line - 1
	if lineIdx < 0 || lineIdx >= len(e.dap.stackLineMap) {
		return
	}
	frame := e.dap.stackLineMap[lineIdx]
	if frame == nil {
		return // thread header or placeholder line
	}
	path := frame.Source.Path
	if path == "" {
		return
	}

	// Open file in the source window.
	srcWin := e.dap.prevActiveWin
	if srcWin == nil && len(e.windows) > 0 {
		srcWin = e.windows[0]
	}
	if srcWin == nil {
		return
	}

	fileBuf, err := e.openFileIntoBuffer(path)
	if err != nil {
		e.Message("debug: cannot open %s: %v", path, err)
		return
	}
	srcWin.SetBuf(fileBuf)
	e.debugMarkSourceReadOnly(fileBuf)
	e.activeWin = srcWin
	fileBuf.SetPoint(fileBuf.LineStart(frame.Line))
	e.scrollWindowToLine(srcWin, frame.Line)
}

// openFileIntoBuffer finds or creates a buffer for path, loads the file, and returns it.
func (e *Editor) openFileIntoBuffer(path string) (*buffer.Buffer, error) {
	// Check if already open.
	for _, b := range e.buffers {
		if b.Filename() == path {
			return b, nil
		}
	}
	return e.loadFile(path)
}
