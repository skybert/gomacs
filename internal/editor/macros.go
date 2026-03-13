package editor

// cmdStartKbdMacro begins recording a keyboard macro (C-x ().
func (e *Editor) cmdStartKbdMacro() {
	e.clearArg()
	if e.kbdMacroRecording {
		e.Message("Already defining keyboard macro")
		return
	}
	e.kbdMacroRecording = true
	e.kbdMacroEvents = nil
	e.Message("Defining keyboard macro...")
}

// cmdEndKbdMacro stops recording the current keyboard macro (C-x )).
func (e *Editor) cmdEndKbdMacro() {
	e.clearArg()
	if !e.kbdMacroRecording {
		e.Message("Not defining keyboard macro")
		return
	}
	e.kbdMacroRecording = false
	// Drop the C-x ) events that ended the macro from the recording.
	// The last two events are C-x and ), strip them.
	if len(e.kbdMacroEvents) >= 2 {
		e.kbdMacroEvents = e.kbdMacroEvents[:len(e.kbdMacroEvents)-2]
	}
	e.kbdMacro = e.kbdMacroEvents
	e.kbdMacroEvents = nil
	e.Message("Keyboard macro defined (%d events)", len(e.kbdMacro))
}

// cmdCallLastKbdMacro replays the last defined keyboard macro (C-x e).
func (e *Editor) cmdCallLastKbdMacro() {
	n := e.arg()
	e.clearArg()
	if len(e.kbdMacro) == 0 {
		e.Message("No keyboard macro defined")
		return
	}
	if e.kbdMacroRecording {
		e.Message("Cannot call macro while defining one")
		return
	}
	e.kbdMacroPlaying = true
	defer func() { e.kbdMacroPlaying = false }()
	for range n {
		for _, ke := range e.kbdMacro {
			e.dispatchParsedKey(ke)
		}
	}
	e.Message("Keyboard macro executed")
}
