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
// With exactly two windows the switch is instant — no letter overlay needed.
// With three or more windows each non-active window gets a home-row letter;
// the user presses that letter to jump, or C-g to cancel.
func (e *Editor) cmdWindowJump() {
	switch len(e.windows) {
	case 0, 1:
		e.Message("Only one window")
	case 2:
		// Instant jump: switch to whichever window is not currently active.
		for _, w := range e.windows {
			if w != e.activeWin {
				e.activeWin = w
				return
			}
		}
	default:
		e.windowJumpActive = true
		e.windowJumpMap = make(map[rune]*window.Window)
		keys := []rune(windowJumpKeys)
		k := 0
		for _, w := range e.windows {
			if w == e.activeWin {
				continue // active window needs no label
			}
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
// rendered windows when three or more windows are open.  The existing text
// and syntax highlighting are left untouched; a single green letter badge
// is drawn at the top-left of each non-active window.
func (e *Editor) renderWindowJumpOverlays() {
	badgeFace := syntax.Face{
		Fg:   "#13131e", // dark foreground
		Bg:   "#06c993", // sweet green background
		Bold: true,
	}

	keys := []rune(windowJumpKeys)
	k := 0
	for _, w := range e.windows {
		if w == e.activeWin {
			continue // active window needs no badge
		}
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
