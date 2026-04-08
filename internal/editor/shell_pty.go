//go:build linux || darwin

package editor

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/keymap"
	"github.com/skybert/gomacs/internal/syntax"
	"github.com/skybert/gomacs/internal/terminal"
	"github.com/skybert/gomacs/internal/window"
)

// shellState holds the live PTY state for a single *shell* buffer.
type shellState struct {
	master *os.File
	mu     sync.Mutex
	vt     *vtScreen
}

func (s *shellState) close() { _ = s.master.Close() }

// cmdShell creates (or switches to) a VC-repo-specific shell buffer backed by
// a real PTY.  The buffer is named *shell/<repo>* when a VC root is found, or
// *shell* otherwise.  Each repo gets its own persistent shell buffer.
func (e *Editor) cmdShell() {
	e.clearArg()

	// Resolve the VC root for the current buffer to derive the buffer name and
	// starting directory.
	_, vcRoot := vcFind(vcDir(e.ActiveBuffer()))
	var shellBufName, startDir string
	if vcRoot != "" {
		repoName := filepath.Base(vcRoot)
		shellBufName = "*shell/" + repoName + "*"
		startDir = vcRoot
	} else {
		shellBufName = "*shell*"
		startDir, _ = os.Getwd()
	}

	// Initialise the shellStates map lazily.
	if e.shellStates == nil {
		e.shellStates = make(map[*buffer.Buffer]*shellState)
	}

	// If a shell buffer for this repo already exists, just switch to it.
	for _, b := range e.buffers {
		if b.Name() == shellBufName {
			e.showBuf(b)
			return
		}
	}

	// Work out the terminal dimensions from the active window.
	w := e.activeWin
	rows := w.Height() - 1 // leave 1 row for the modeline
	cols := w.Width()
	if rows < 2 {
		rows = 2
	}
	if cols < 4 {
		cols = 4
	}

	master, slave, err := openPTY()
	if err != nil {
		e.Message("shell: %v", err)
		return
	}

	if err := setPTYSize(master, rows, cols); err != nil {
		_ = master.Close()
		_ = slave.Close()
		e.Message("shell: setPTYSize: %v", err)
		return
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.Command(shell) //nolint:gosec
	cmd.Dir = startDir
	cmd.Stdin = slave
	cmd.Stdout = slave
	cmd.Stderr = slave
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
		Ctty:    0, // stdin fd in child
	}

	if err := cmd.Start(); err != nil {
		_ = master.Close()
		_ = slave.Close()
		e.Message("shell: start: %v", err)
		return
	}
	// Slave is handed off to the child; close our copy so EOF propagates.
	_ = slave.Close()

	shellBuf := buffer.NewWithContent(shellBufName, "")
	shellBuf.SetMode("shell")
	shellBuf.SetReadOnly(true)
	e.buffers = append(e.buffers, shellBuf)
	e.showBuf(shellBuf)

	st := &shellState{
		master: master,
		vt:     newVTScreen(rows, cols),
	}
	e.shellStates[shellBuf] = st

	// Background goroutine: read PTY output → update vtScreen → redraw.
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := master.Read(buf)
			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				e.lspCbs <- func() {
					st.mu.Lock()
					st.vt.write(data)
					st.mu.Unlock()
					e.Redraw()
				}
				e.term.PostWakeup()
			}
			if err != nil {
				// PTY closed (shell exited).
				e.lspCbs <- func() {
					e.Message("Shell process exited")
					if st2, ok := e.shellStates[shellBuf]; ok {
						st2.close()
						delete(e.shellStates, shellBuf)
					}
					shellBuf.SetReadOnly(false)
					shellBuf.InsertString(shellBuf.Len(), "\n[Process exited]\n")
					shellBuf.SetReadOnly(true)
					e.Redraw()
				}
				e.term.PostWakeup()
				break
			}
		}
		// Reap child to avoid zombie.
		_ = cmd.Wait()
	}()
}

// shellDispatch handles key events when the active buffer is *shell*.
// Returns true if the event was consumed (forwarded to PTY or handled locally).
// Reserved gomacs keys are returned as false so normal dispatch continues.
func (e *Editor) shellDispatch(ke terminal.KeyEvent) bool {
	buf := e.ActiveBuffer()
	st, ok := e.shellStates[buf]
	if !ok {
		return false
	}

	// Keys reserved for gomacs proper (spec §shell).
	switch {
	case ke.Key == tcell.KeyRune && ke.Mod == tcell.ModAlt:
		// M-x, M-v, M-w: fall through to normal dispatch.
		if ke.Rune == 'x' || ke.Rune == 'v' || ke.Rune == 'w' {
			return false
		}
	case ke.Key == tcell.KeyCtrlV:
		return false // scroll down
	case ke.Key == tcell.KeyRune && ke.Mod == 0 && ke.Rune == 0:
		return false // C-space (set mark)
	}

	// C-x prefix → let normal dispatch handle it.
	mk := keymap.MakeKey(ke.Key, ke.Rune, ke.Mod)
	if e.globalKeymap != nil {
		if b, found := e.globalKeymap.Lookup(mk); found && b.Prefix != nil {
			return false
		}
	}

	data := keyToShellBytes(ke)
	if len(data) == 0 {
		return false
	}

	st.mu.Lock()
	_, werr := st.master.Write(data)
	st.mu.Unlock()
	if werr != nil {
		e.Message("shell write: %v", werr)
	}
	return true
}

// renderShellWindow renders the vtScreen for a shell buffer into window w.
func (e *Editor) renderShellWindow(w *window.Window) {
	buf := w.Buf()
	st, ok := e.shellStates[buf]
	if !ok {
		e.renderWindow(w)
		return
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	// Resize vtScreen if the window dimensions changed.
	rows := w.Height() - 1
	cols := w.Width()
	if rows < 1 {
		rows = 1
	}
	if cols < 1 {
		cols = 1
	}
	if st.vt.rows != rows || st.vt.cols != cols {
		st.vt.resize(rows, cols)
		_ = setPTYSize(st.master, rows, cols) // best-effort resize; errors ignored
	}

	top := w.Top()
	left := w.Left()
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			cell := st.vt.cellAt(r, c)
			face := cell.face
			// Fall back to default face for blank cells.
			if face == (syntax.Face{}) {
				face = syntax.FaceDefault
			}
			e.term.SetCell(left+c, top+r, cell.ch, face)
		}
	}
}

// shellCursorPos returns the screen (col, row) for the terminal cursor in
// the shell window that contains buf, or (-1, -1) if not found.
func (e *Editor) shellCursorPos(buf *buffer.Buffer) (col, row int) {
	st, ok := e.shellStates[buf]
	if !ok {
		return -1, -1
	}
	for _, w := range e.windows {
		if w.Buf() == buf {
			st.mu.Lock()
			vtRow := st.vt.curRow
			vtCol := st.vt.curCol
			st.mu.Unlock()
			return w.Left() + vtCol, w.Top() + vtRow
		}
	}
	return -1, -1
}

// keyToShellBytes converts a terminal key event to the byte sequence that
// should be sent to the PTY slave.
func keyToShellBytes(ke terminal.KeyEvent) []byte {
	switch ke.Key {
	case tcell.KeyRune:
		r := ke.Rune
		if ke.Mod&tcell.ModCtrl != 0 && r >= 'a' && r <= 'z' {
			return []byte{byte(r - 'a' + 1)}
		}
		if ke.Mod&tcell.ModCtrl != 0 && r >= 'A' && r <= 'Z' {
			return []byte{byte(r - 'A' + 1)}
		}
		if ke.Mod&tcell.ModAlt != 0 {
			// Send ESC + rune for Meta keys.
			return append([]byte{0x1b}, []byte(string(r))...)
		}
		return []byte(string(r))

	case tcell.KeyEnter:
		return []byte{'\r'}
	case tcell.KeyTab:
		return []byte{'\t'}
	case tcell.KeyBackspace:
		return []byte{0x7f}
	case tcell.KeyDelete:
		return []byte{0x1b, '[', '3', '~'}
	case tcell.KeyEscape:
		return []byte{0x1b}

	case tcell.KeyUp:
		return []byte{0x1b, '[', 'A'}
	case tcell.KeyDown:
		return []byte{0x1b, '[', 'B'}
	case tcell.KeyRight:
		return []byte{0x1b, '[', 'C'}
	case tcell.KeyLeft:
		return []byte{0x1b, '[', 'D'}

	case tcell.KeyHome:
		return []byte{0x1b, '[', 'H'}
	case tcell.KeyEnd:
		return []byte{0x1b, '[', 'F'}
	case tcell.KeyPgUp:
		return []byte{0x1b, '[', '5', '~'}
	case tcell.KeyPgDn:
		return []byte{0x1b, '[', '6', '~'}

	case tcell.KeyF1:
		return []byte{0x1b, 'O', 'P'}
	case tcell.KeyF2:
		return []byte{0x1b, 'O', 'Q'}
	case tcell.KeyF3:
		return []byte{0x1b, 'O', 'R'}
	case tcell.KeyF4:
		return []byte{0x1b, 'O', 'S'}
	case tcell.KeyF5:
		return []byte{0x1b, '[', '1', '5', '~'}
	case tcell.KeyF6:
		return []byte{0x1b, '[', '1', '7', '~'}
	case tcell.KeyF7:
		return []byte{0x1b, '[', '1', '8', '~'}
	case tcell.KeyF8:
		return []byte{0x1b, '[', '1', '9', '~'}
	case tcell.KeyF9:
		return []byte{0x1b, '[', '2', '0', '~'}
	case tcell.KeyF10:
		return []byte{0x1b, '[', '2', '1', '~'}
	case tcell.KeyF11:
		return []byte{0x1b, '[', '2', '3', '~'}
	case tcell.KeyF12:
		return []byte{0x1b, '[', '2', '4', '~'}

	// Control keys.
	case tcell.KeyCtrlA:
		return []byte{1}
	case tcell.KeyCtrlB:
		return []byte{2}
	case tcell.KeyCtrlC:
		return []byte{3}
	case tcell.KeyCtrlD:
		return []byte{4}
	case tcell.KeyCtrlE:
		return []byte{5}
	case tcell.KeyCtrlF:
		return []byte{6}
	case tcell.KeyCtrlG:
		return []byte{7}
	case tcell.KeyCtrlH:
		return []byte{8}
	case tcell.KeyCtrlI: // same as Tab
		return []byte{9}
	case tcell.KeyCtrlJ:
		return []byte{10}
	case tcell.KeyCtrlK:
		return []byte{11}
	case tcell.KeyCtrlL:
		return []byte{12}
	case tcell.KeyCtrlM: // same as Enter
		return []byte{13}
	case tcell.KeyCtrlN:
		return []byte{14}
	case tcell.KeyCtrlO:
		return []byte{15}
	case tcell.KeyCtrlP:
		return []byte{16}
	case tcell.KeyCtrlQ:
		return []byte{17}
	case tcell.KeyCtrlR:
		return []byte{18}
	case tcell.KeyCtrlS:
		return []byte{19}
	case tcell.KeyCtrlT:
		return []byte{20}
	case tcell.KeyCtrlU:
		return []byte{21}
	case tcell.KeyCtrlW:
		return []byte{23}
	case tcell.KeyCtrlY:
		return []byte{25}
	case tcell.KeyCtrlZ:
		return []byte{26}
	}
	return nil
}
