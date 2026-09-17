package editor

import (
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/syntax"
	"github.com/skybert/gomacs/internal/terminal"
	"github.com/skybert/gomacs/internal/window"
)

const windowJumpKeys = "asdfghkl"

// cmdWindowJump activates window-jump mode (M-o).
//
// Every visible window — including the currently active one — gets a home-row
// letter badge, and syntax highlighting is suppressed while the overlay is up
// so the badges stand out.  The user presses a letter to jump, or C-g to
// cancel.
func (e *Editor) cmdWindowJump() {
	if len(e.windows) < 2 {
		e.Message("Only one window")
		return
	}

	e.windowJumpActive = true
	e.windowJumpMap = make(map[rune]*window.Window)
	keys := []rune(windowJumpKeys)
	k := 0
	for _, w := range e.windows {
		if k >= len(keys) {
			break
		}
		e.windowJumpMap[keys[k]] = w
		k++
	}
	labels := make([]string, 0, k)
	for _, r := range keys[:k] {
		labels = append(labels, string(r))
	}
	e.Message("Jump to window: [%s]  C-g cancels", strings.Join(labels, "/"))
}

// windowJumpHandleKey processes the key that selects a target window.
// By the time this is called the ESC-prefix Meta assembler has already run,
// so Meta+key sequences arrive with ke.Mod=ModAlt — we only accept plain
// unmodified runes to avoid eating useful key bindings.
func (e *Editor) windowJumpHandleKey(ke terminal.KeyEvent) {
	jumpMap := e.windowJumpMap
	e.windowJumpActive = false
	e.windowJumpMap = nil

	if ke.Key == tcell.KeyCtrlG {
		e.Message("")
		return
	}
	// Only act on plain (unmodified) rune keys.
	if ke.Key == tcell.KeyRune && ke.Mod == 0 {
		if w, ok := jumpMap[ke.Rune]; ok {
			e.activeWin = w
			e.Message("")
			return
		}
	}
	// Any other key cancels silently.
}

// renderWindowJumpOverlays draws the window-jump UI on top of the already-
// rendered windows.  Syntax highlighting is already suppressed for the frame
// (see highlighterFor), so all that is left is a single green letter badge at
// the top-left of every visible window.
func (e *Editor) renderWindowJumpOverlays() {
	badgeFace := syntax.FaceWindowJump

	keys := []rune(windowJumpKeys)
	k := 0
	for _, w := range e.windows {
		if k >= len(keys) {
			break
		}

		gutterW := w.GutterWidth()
		if gutterW == 0 && w.Buf().Filename() != "" {
			absName, _ := filepath.Abs(w.Buf().Filename())
			if len(e.dapBreakpoints[absName]) > 0 || e.dap != nil {
				gutterW = 2
			}
		}

		// Place the badge at the very first character position of the first line.
		col := w.Left() + gutterW
		row := w.Top()
		e.term.SetCell(col, row, keys[k], badgeFace)
		k++
	}
}
