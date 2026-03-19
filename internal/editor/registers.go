package editor

// register holds a single register value (position or text).
type register struct {
	kind string // "point" or "text"
	pos  int
	text string
	buf  string // buffer name for point registers
}

// cmdPointToRegister stores the current point in a register (C-x r SPC).
func (e *Editor) cmdPointToRegister() {
	e.clearArg()
	e.Message("Point to register: ")
	e.readCharPending = true
	e.readCharCallback = func(r rune) {
		buf := e.ActiveBuffer()
		e.registers[r] = register{
			kind: "point",
			pos:  buf.Point(),
			buf:  buf.Name(),
		}
		e.Message("Saved point to register %c", r)
	}
}

// cmdJumpToRegister jumps to a position stored in a register (C-x r j).
func (e *Editor) cmdJumpToRegister() {
	e.clearArg()
	e.Message("Jump to register: ")
	e.readCharPending = true
	e.readCharCallback = func(r rune) {
		reg, ok := e.registers[r]
		if !ok {
			e.Message("Register %c is empty", r)
			return
		}
		switch reg.kind {
		case "point":
			if reg.buf != e.ActiveBuffer().Name() {
				e.SwitchToBuffer(reg.buf)
			}
			e.ActiveBuffer().SetPoint(reg.pos)
		case "text":
			e.Message("Register %c contains text, use insert-register", r)
		}
	}
}

// cmdCopyToRegister saves the region text to a register (C-x r s).
func (e *Editor) cmdCopyToRegister() {
	e.clearArg()
	e.Message("Copy to register: ")
	e.readCharPending = true
	e.readCharCallback = func(r rune) {
		buf := e.ActiveBuffer()
		start, end := regionBounds(buf)
		if start == end {
			e.Message("No region")
			return
		}
		text := buf.Substring(start, end)
		e.registers[r] = register{kind: "text", text: text}
		e.Message("Copied region to register %c", r)
	}
}

// cmdInsertRegister inserts the text stored in a register (C-x r i).
func (e *Editor) cmdInsertRegister() {
	if e.bufReadOnly() {
		return
	}
	e.clearArg()
	e.Message("Insert register: ")
	e.readCharPending = true
	e.readCharCallback = func(r rune) {
		reg, ok := e.registers[r]
		if !ok {
			e.Message("Register %c is empty", r)
			return
		}
		if reg.kind != "text" {
			e.Message("Register %c does not contain text", r)
			return
		}
		buf := e.ActiveBuffer()
		pt := buf.Point()
		buf.InsertString(pt, reg.text)
		buf.SetPoint(pt + len([]rune(reg.text)))
	}
}

// cmdCopyRectangleToRegister is a stub (C-x r r).
func (e *Editor) cmdCopyRectangleToRegister() {
	e.clearArg()
	e.Message("Rectangle registers not yet implemented")
}
