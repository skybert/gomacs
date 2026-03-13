package editor

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/skybert/gomacs/internal/terminal"
)

// bufferListDispatch handles keys in a *Buffer List* buffer.
// Returns true if the key was consumed.
func (e *Editor) bufferListDispatch(ke terminal.KeyEvent) bool {
	if ke.Key != tcell.KeyRune && ke.Key != tcell.KeyEnter {
		return false
	}
	openCurrent := ke.Key == tcell.KeyEnter || ke.Rune == 'f' || ke.Rune == 'e'
	if !openCurrent {
		switch ke.Rune {
		case 'n', ' ':
			e.execCommand("next-line")
			return true
		case 'p':
			e.execCommand("previous-line")
			return true
		case 'q':
			for _, b := range e.buffers {
				if b != e.ActiveBuffer() && b.Mode() != "buffer-list" {
					e.activeWin.SetBuf(b)
					return true
				}
			}
			e.SwitchToBuffer("*scratch*")
			return true
		}
		return false
	}

	// Open the buffer named on the current line.
	// Line format: "  <cur><mod>   <name (24 cols)>  ..."
	// Buffer name occupies columns 7–30 (0-based, rune indices).
	buf := e.ActiveBuffer()
	pt := buf.Point()
	bol := buf.BeginningOfLine(pt)
	eol := buf.EndOfLine(pt)
	line := buf.Substring(bol, eol)
	runes := []rune(line)
	const nameStart = 7
	const nameEnd = 31 // exclusive
	if len(runes) < nameEnd {
		return true
	}
	name := strings.TrimSpace(string(runes[nameStart:nameEnd]))
	if name == "" || name == "Buffer" || name == "------" {
		return true // header line
	}
	target := e.FindBuffer(name)
	if target == nil {
		e.Message("No buffer named %q", name)
		return true
	}
	e.activeWin.SetBuf(target)
	return true
}
