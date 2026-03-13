// Package editor wires together all sub-packages into a running Emacs-like
// editor.  Editor is the top-level application object; it owns the terminal,
// all buffers, all windows, the keymap hierarchy and the Elisp evaluator.
package editor

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/elisp"
	"github.com/skybert/gomacs/internal/keymap"
	"github.com/skybert/gomacs/internal/syntax"
	"github.com/skybert/gomacs/internal/terminal"
	"github.com/skybert/gomacs/internal/window"
)

// Editor is the top-level application state.
type Editor struct {
	term       *terminal.Terminal
	buffers    []*buffer.Buffer
	windows    []*window.Window
	activeWin  *window.Window
	minibufWin *window.Window
	minibufBuf *buffer.Buffer

	globalKeymap *keymap.Keymap
	ctrlXKeymap  *keymap.Keymap // C-x prefix map
	ctrlHKeymap  *keymap.Keymap // C-h prefix map
	ctrlXNKeymap *keymap.Keymap // C-x n (narrowing)
	ctrlXRKeymap *keymap.Keymap // C-x r (registers)
	metaGKeymap  *keymap.Keymap // M-g (goto)

	killRing []string // yank ring; [0] is most recent
	yankIdx  int      // current yank-pop index

	universalArg    int  // current C-u argument (default 1)
	universalArgSet bool // true if C-u has been pressed
	// Digit accumulation for C-u 8 C-f style prefix arguments.
	universalArgDigits string
	universalArgTyping bool

	// isearch state
	isearching   bool
	isearchFwd   bool
	isearchStr   string
	isearchStart int // buffer position where isearch was initiated

	// prefix key state (non-nil while processing a multi-key sequence)
	prefixKeymap *keymap.Keymap

	// describe-key state: set by C-h k; intercepts the next key sequence
	// and shows the bound command and its documentation.
	describeKeyPending bool
	describeKeySeq     string
	describeKeyMap     *keymap.Keymap

	// readChar: when set, the next key press delivers its rune to the callback
	// instead of normal dispatch (used by register commands).
	readCharPending  bool
	readCharCallback func(rune)

	// minibuffer state
	minibufActive   bool
	minibufPrompt   string
	minibufDoneFunc func(input string)

	// transient status message shown in the minibuffer area
	message     string
	messageTime int64 // Unix nano; message expires after 5 s

	// Elisp evaluator
	lisp *elisp.Evaluator

	// quit flag; set by save-buffers-kill-terminal
	quit bool

	// lastCommand is the name of the most recently executed command.
	// Used for C-l cycle detection.
	lastCommand string

	// recenterCycle tracks the C-l cycling state (0=center, 1=top, 2=bottom).
	recenterCycle int

	// minibufCompletions is an optional tab-completion function for the
	// active minibuffer.  Set via SetMinibufCompletions after ReadMinibuffer.
	minibufCompletions func(string) []string
	// minibufHint is the last completions hint shown after TAB.
	// Displayed just above the minibuffer line when the minibuffer is active.
	minibufHint string

	// minibuf completion popup
	minibufCandidates      []string
	minibufSelectedIdx     int
	minibufCandidateOffset int
	minibufLastQuery       string

	// lastYankEnd tracks the end position of the last yank so that
	// yank-pop can replace it.
	lastYankEnd int
	lastYankLen int

	// Registers: each register holds a position or a string.
	registers map[rune]register

	// Keyboard macros.
	kbdMacroRecording bool
	kbdMacroEvents    []terminal.KeyEvent // events captured during recording
	kbdMacro          []terminal.KeyEvent // last completed macro
	kbdMacroPlaying   bool

	// Query replace state.
	queryReplaceActive bool
	queryReplaceFrom   string
	queryReplaceTo     string
	queryReplaceCursor int // position to search from for next match
	queryReplaceMatch  int // start of current highlighted match (-1 if none)

	// Dired state keyed by buffer pointer.
	diredStates map[*buffer.Buffer]*diredState

	// fillColumn is the target column for fill-paragraph (default 70).
	fillColumn int
}

// ---------------------------------------------------------------------------
// Construction
// ---------------------------------------------------------------------------

// Options controls optional editor startup behaviour.
type Options struct {
	// Quick suppresses loading of the user init file, equivalent to emacs -Q.
	Quick bool
}

// New creates and initialises the editor: terminal, scratch buffer, keymaps,
// windows, and the Elisp evaluator.  It also attempts to load the user's init
// file (~/.emacs or ~/.emacs.d/init.el) unless opts.Quick is true.
func New(opts Options) (*Editor, error) {
	term, err := terminal.New()
	if err != nil {
		return nil, fmt.Errorf("editor.New: %w", err)
	}

	e := &Editor{
		term:         term,
		universalArg: 1,
		lisp:         elisp.NewEvaluator(),
		registers:    make(map[rune]register),
		diredStates:  make(map[*buffer.Buffer]*diredState),
		fillColumn:   70,
	}

	// Apply the default colour theme.
	syntax.LoadTheme("sweet")

	// Register load-theme as an Elisp callable so init.el can switch themes.
	e.lisp.RegisterGoFn("load-theme", func(args []elisp.Value, _ *elisp.Env) (elisp.Value, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("load-theme: expected 1 argument")
		}
		name := args[0].String()
		name = strings.Trim(name, `'"`)
		if !syntax.LoadTheme(name) {
			return nil, fmt.Errorf("load-theme: unknown theme %q", name)
		}
		return elisp.Nil{}, nil
	})

	// Create the *scratch* buffer with Emacs Lisp mode.
	scratch := buffer.NewWithContent("*scratch*",
		";; This buffer is for text that is not saved, and for Lisp evaluation.\n")
	scratch.SetMode(modeElisp)
	e.buffers = append(e.buffers, scratch)

	// Create the minibuffer buffer (never shown in the buffer list).
	e.minibufBuf = buffer.New(" *minibuf*")

	// Set up keymaps.
	e.setupKeymaps()

	// Create windows based on terminal size.
	w, h := term.Size()
	// Main window occupies all rows except the last (minibuffer row).
	mainWin := window.New(scratch, 0, 0, w, h-1)
	e.windows = append(e.windows, mainWin)
	e.activeWin = mainWin

	// Minibuffer window is the very last row.
	e.minibufWin = window.New(e.minibufBuf, h-1, 0, w, 1)

	// Load user init file (skipped with -Q).
	if !opts.Quick {
		e.loadInitFile()
	}

	return e, nil
}

// setupKeymaps builds the global keymap and all prefix keymaps, and binds
// every default key chord.
func (e *Editor) setupKeymaps() {
	e.globalKeymap = keymap.New("global")
	e.ctrlXKeymap = keymap.New("C-x")
	e.ctrlHKeymap = keymap.New("C-h")
	e.ctrlXNKeymap = keymap.New("C-x n")
	e.ctrlXRKeymap = keymap.New("C-x r")
	e.metaGKeymap = keymap.New("M-g")

	gk := e.globalKeymap
	cx := e.ctrlXKeymap
	ch := e.ctrlHKeymap
	cxn := e.ctrlXNKeymap
	cxr := e.ctrlXRKeymap
	mg := e.metaGKeymap

	// ---- C-x prefix --------------------------------------------------------
	gk.BindPrefix(keymap.CtrlKey('x'), cx)

	// ---- C-h prefix (help) -------------------------------------------------
	gk.BindPrefix(keymap.CtrlKey('h'), ch)
	ch.Bind(keymap.PlainKey('k'), "describe-key")
	ch.Bind(keymap.PlainKey('f'), "describe-function")
	ch.Bind(keymap.PlainKey('v'), "describe-variable")

	// ---- M-g prefix (goto) -------------------------------------------------
	gk.BindPrefix(keymap.MetaKey('g'), mg)
	mg.Bind(keymap.PlainKey('g'), "goto-line")
	mg.Bind(keymap.MetaKey('g'), "goto-line")
	mg.Bind(keymap.PlainKey('n'), "next-error")
	mg.Bind(keymap.PlainKey('p'), "previous-error")

	// ---- C-x n prefix (narrowing) ------------------------------------------
	cx.BindPrefix(keymap.PlainKey('n'), cxn)
	cxn.Bind(keymap.PlainKey('n'), "narrow-to-region")
	cxn.Bind(keymap.PlainKey('w'), "widen")

	// ---- C-x r prefix (registers) ------------------------------------------
	cx.BindPrefix(keymap.PlainKey('r'), cxr)
	cxr.Bind(keymap.MakeKey(tcell.KeyCtrlSpace, 0, 0), "point-to-register")
	cxr.Bind(keymap.PlainKey(' '), "point-to-register")
	cxr.Bind(keymap.PlainKey('j'), "jump-to-register")
	cxr.Bind(keymap.PlainKey('s'), "copy-to-register")
	cxr.Bind(keymap.PlainKey('i'), "insert-register")
	cxr.Bind(keymap.PlainKey('r'), "copy-rectangle-to-register")

	// ---- movement ----------------------------------------------------------
	gk.Bind(keymap.CtrlKey('f'), "forward-char")
	gk.Bind(keymap.CtrlKey('b'), "backward-char")
	gk.Bind(keymap.CtrlKey('n'), "next-line")
	gk.Bind(keymap.CtrlKey('p'), "previous-line")
	gk.Bind(keymap.CtrlKey('a'), "beginning-of-line")
	gk.Bind(keymap.CtrlKey('e'), "end-of-line")
	gk.Bind(keymap.MetaKey('f'), "forward-word")
	gk.Bind(keymap.MetaKey('b'), "backward-word")
	gk.Bind(keymap.MetaKey('<'), "beginning-of-buffer")
	gk.Bind(keymap.MetaKey('>'), "end-of-buffer")
	gk.Bind(keymap.CtrlKey('v'), "scroll-up")
	gk.Bind(keymap.MetaKey('v'), "scroll-down")
	gk.Bind(keymap.CtrlKey('l'), "recenter")
	gk.Bind(keymap.MetaKey('m'), "back-to-indentation")

	// Arrow keys.
	gk.Bind(keymap.MakeKey(tcell.KeyRight, 0, 0), "forward-char")
	gk.Bind(keymap.MakeKey(tcell.KeyLeft, 0, 0), "backward-char")
	gk.Bind(keymap.MakeKey(tcell.KeyDown, 0, 0), "next-line")
	gk.Bind(keymap.MakeKey(tcell.KeyUp, 0, 0), "previous-line")
	gk.Bind(keymap.MakeKey(tcell.KeyHome, 0, 0), "beginning-of-line")
	gk.Bind(keymap.MakeKey(tcell.KeyEnd, 0, 0), "end-of-line")
	gk.Bind(keymap.MakeKey(tcell.KeyPgDn, 0, 0), "scroll-up")
	gk.Bind(keymap.MakeKey(tcell.KeyPgUp, 0, 0), "scroll-down")

	// ---- editing -----------------------------------------------------------
	gk.Bind(keymap.MakeKey(tcell.KeyEnter, 0, 0), "newline")
	gk.Bind(keymap.MakeKey(tcell.KeyTab, 0, 0), "indent-or-complete")
	gk.Bind(keymap.CtrlKey('d'), "delete-char")
	gk.Bind(keymap.MakeKey(tcell.KeyDelete, 0, 0), "delete-char")
	gk.Bind(keymap.MakeKey(tcell.KeyBackspace, 0, 0), "backward-delete-char")
	gk.Bind(keymap.MakeKey(tcell.KeyBackspace2, 0, 0), "backward-delete-char")
	gk.Bind(keymap.CtrlKey('k'), "kill-line")
	gk.Bind(keymap.CtrlKey('w'), "kill-region")
	gk.Bind(keymap.MetaKey('w'), "copy-region-as-kill")
	gk.Bind(keymap.CtrlKey('y'), "yank")
	gk.Bind(keymap.MetaKey('y'), "yank-pop")
	gk.Bind(keymap.MetaKey('d'), "kill-word")
	gk.Bind(keymap.MakeKey(tcell.KeyBackspace, 0, tcell.ModAlt), "backward-kill-word")
	gk.Bind(keymap.CtrlKey('t'), "transpose-chars")
	gk.Bind(keymap.CtrlKey('o'), "open-line")
	gk.Bind(keymap.MetaKey('t'), "transpose-words")
	gk.Bind(keymap.MetaKey('^'), "join-line")
	gk.Bind(keymap.MetaKey('q'), "fill-paragraph")
	gk.Bind(keymap.MetaKey('$'), "ispell-word") // stub

	// ---- marks / search / misc --------------------------------------------
	gk.Bind(keymap.CtrlKey(' '), "set-mark-command")
	gk.Bind(keymap.CtrlKey('s'), "isearch-forward")
	gk.Bind(keymap.CtrlKey('r'), "isearch-backward")
	gk.Bind(keymap.CtrlKey('/'), "undo")
	gk.Bind(keymap.CtrlKey('g'), "keyboard-quit")
	gk.Bind(keymap.CtrlKey('u'), "universal-argument")
	gk.Bind(keymap.MetaKey('x'), "execute-extended-command")
	gk.Bind(keymap.MetaKey(';'), "comment-dwim")
	gk.Bind(keymap.MetaKey('%'), "query-replace")
	gk.Bind(keymap.MetaKey('!'), "shell-command")
	gk.Bind(keymap.MetaKey('|'), "shell-command-on-region")
	gk.Bind(keymap.MetaKey('@'), "mark-word")

	// Sentence movement (M-a / M-e / M-k)
	gk.Bind(keymap.MetaKey('a'), "beginning-of-sentence")
	gk.Bind(keymap.MetaKey('e'), "end-of-sentence")
	gk.Bind(keymap.MetaKey('k'), "kill-sentence")

	// Word-case commands (M-u / M-l / M-c)
	gk.Bind(keymap.MetaKey('u'), "upcase-word")
	gk.Bind(keymap.MetaKey('l'), "downcase-word")
	gk.Bind(keymap.MetaKey('c'), "capitalize-word")

	// C-M bindings (represented as Meta + Ctrl combos)
	gk.Bind(keymap.MakeKey(tcell.KeyCtrlBackslash, 0, tcell.ModAlt), "indent-region")

	// ---- C-x bindings -------------------------------------------------------
	cx.Bind(keymap.CtrlKey('f'), "find-file")
	cx.Bind(keymap.CtrlKey('s'), "save-buffer")
	cx.Bind(keymap.CtrlKey('c'), "save-buffers-kill-terminal")
	cx.Bind(keymap.PlainKey('b'), "switch-to-buffer")
	cx.Bind(keymap.PlainKey('k'), "kill-buffer")
	cx.Bind(keymap.CtrlKey('b'), "list-buffers")
	cx.Bind(keymap.CtrlKey('x'), "exchange-point-and-mark")
	cx.Bind(keymap.PlainKey('u'), "undo")
	cx.Bind(keymap.PlainKey('s'), "save-some-buffers")
	cx.Bind(keymap.PlainKey('1'), "delete-other-windows")
	cx.Bind(keymap.PlainKey('2'), "split-window-below")
	cx.Bind(keymap.PlainKey('3'), "split-window-right")
	cx.Bind(keymap.PlainKey('o'), "other-window")
	cx.Bind(keymap.CtrlKey('q'), "toggle-read-only")
	cx.Bind(keymap.CtrlKey('e'), "eval-last-sexp")
	cx.Bind(keymap.PlainKey('h'), "mark-whole-buffer")
	cx.Bind(keymap.PlainKey('d'), "dired")
	cx.Bind(keymap.PlainKey('='), "what-cursor-position")
	cx.Bind(keymap.CtrlKey('o'), "delete-blank-lines")
	cx.Bind(keymap.CtrlKey('u'), "upcase-region")
	cx.Bind(keymap.CtrlKey('l'), "downcase-region")
	cx.Bind(keymap.PlainKey('f'), "set-fill-column")
	cx.Bind(keymap.MakeKey(tcell.KeyTab, 0, 0), "indent-rigidly")
	cx.Bind(keymap.PlainKey('('), "start-kbd-macro")
	cx.Bind(keymap.PlainKey(')'), "end-kbd-macro")
	cx.Bind(keymap.PlainKey('e'), "call-last-kbd-macro")
}

// loadInitFile tries ~/.emacs and ~/.emacs.d/init.el in that order.
func (e *Editor) loadInitFile() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	candidates := []string{
		filepath.Join(home, ".emacs"),
		filepath.Join(home, ".emacs.d", "init.el"),
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			_ = e.lisp.EvalFile(path)
			return
		}
	}
}

// ---------------------------------------------------------------------------
// Main event loop
// ---------------------------------------------------------------------------

// Run starts the editor's main event loop.  It blocks until the user quits.
func (e *Editor) Run() {
	defer e.term.Close()
	e.Redraw()
	for !e.quit {
		ev := e.term.PollEvent()
		switch ev := ev.(type) {
		case *tcell.EventResize:
			e.handleResize()
		case *tcell.EventKey:
			e.dispatchKey(ev)
			// Sync window-local point from the buffer before scrolling.
			e.syncWindowPoint(e.activeWin)
			e.activeWin.EnsurePointVisible()
		}
		e.Redraw()
	}
}

// syncWindowPoint copies buf.Point() into the window's local point field so
// that EnsurePointVisible and Recenter operate on the current cursor position.
func (e *Editor) syncWindowPoint(w *window.Window) {
	w.SetPoint(w.Buf().Point())
}

// handleResize adjusts all windows to the new terminal dimensions.
func (e *Editor) handleResize() {
	w, h := e.term.Size()
	e.minibufWin.SetRegion(h-1, 0, w, 1)
	e.relayoutWindows(w, h-1)
}

// relayoutWindows redistributes the available screen area (totalW×totalH)
// equally among all non-minibuffer windows.  Two windows are assumed to share
// the area: if their tops differ they are stacked vertically; otherwise they
// are placed side by side horizontally.
func (e *Editor) relayoutWindows(totalW, totalH int) {
	n := len(e.windows)
	if n == 0 {
		return
	}
	if n == 1 {
		e.windows[0].SetRegion(0, 0, totalW, totalH)
		return
	}
	// Determine split orientation from current positions.
	// If windows share the same top row → vertical (side-by-side) split.
	// Otherwise → horizontal (stacked) split.
	sameLine := true
	for _, w := range e.windows[1:] {
		if w.Top() != e.windows[0].Top() {
			sameLine = false
			break
		}
	}
	if sameLine {
		// Vertical split: divide width equally, leaving 1 col per separator.
		// n windows → n-1 separator columns.
		available := max(totalW-(n-1), n) // leave 1 col per separator, min 1 col/window
		each := available / n
		for i, w := range e.windows {
			left := i*each + i // i separator columns to the left
			width := each
			if i == n-1 {
				width = available - i*each
			}
			w.SetRegion(0, left, width, totalH)
		}
	} else {
		// Horizontal split: divide height equally (leave 1 row for modeline per window).
		each := totalH / n
		for i, w := range e.windows {
			top := i * each
			height := each
			if i == n-1 {
				height = totalH - top
			}
			w.SetRegion(top, 0, totalW, height)
		}
	}
}

// ---------------------------------------------------------------------------
// Buffer / window accessors
// ---------------------------------------------------------------------------

// ActiveBuffer returns the buffer shown in the active window.
func (e *Editor) ActiveBuffer() *buffer.Buffer {
	return e.activeWin.Buf()
}

// ActiveWindow returns the active window.
func (e *Editor) ActiveWindow() *window.Window {
	return e.activeWin
}

// FindBuffer returns the buffer with the given name, or nil.
func (e *Editor) FindBuffer(name string) *buffer.Buffer {
	for _, b := range e.buffers {
		if b.Name() == name {
			return b
		}
	}
	return nil
}

// SwitchToBuffer displays the buffer with name in the active window,
// creating it if it does not exist.
func (e *Editor) SwitchToBuffer(name string) *buffer.Buffer {
	b := e.FindBuffer(name)
	if b == nil {
		b = buffer.New(name)
		e.buffers = append(e.buffers, b)
	}
	e.activeWin.SetBuf(b)
	return b
}

// KillBuffer removes the named buffer.  If the active window was displaying
// it, the window switches to *scratch* (creating it if necessary).
func (e *Editor) KillBuffer(name string) {
	var remaining []*buffer.Buffer
	for _, b := range e.buffers {
		if b.Name() != name {
			remaining = append(remaining, b)
		}
	}
	e.buffers = remaining

	// If the active window's buffer was killed, show *scratch*.
	if e.activeWin.Buf().Name() == name {
		e.SwitchToBuffer("*scratch*")
	}
}

// ---------------------------------------------------------------------------
// Messaging
// ---------------------------------------------------------------------------

// Message formats a status message displayed in the minibuffer area.
func (e *Editor) Message(format string, args ...any) {
	e.message = fmt.Sprintf(format, args...)
	e.messageTime = time.Now().UnixNano()
}

// ---------------------------------------------------------------------------
// Minibuffer
// ---------------------------------------------------------------------------

// ReadMinibuffer activates the minibuffer with prompt, calling done when the
// user presses Enter.
func (e *Editor) ReadMinibuffer(prompt string, done func(string)) {
	e.minibufActive = true
	e.minibufPrompt = prompt
	e.minibufDoneFunc = done
	e.minibufCompletions = nil // reset; caller may set via SetMinibufCompletions
	e.minibufHint = ""
	// Clear the minibuffer buffer.
	e.minibufBuf.Delete(0, e.minibufBuf.Len())
	e.minibufBuf.SetPoint(0)
	e.message = ""
}

// SetMinibufCompletions registers a tab-completion function for the currently
// active minibuffer.  It must be called immediately after ReadMinibuffer.
func (e *Editor) SetMinibufCompletions(fn func(string) []string) {
	e.minibufCompletions = fn
	e.refreshMinibufCandidates()
}

// commonPrefix returns the longest string that is a prefix of every element
// of ss.
func commonPrefix(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	prefix := ss[0]
	for _, s := range ss[1:] {
		for !strings.HasPrefix(s, prefix) {
			if len(prefix) == 0 {
				return ""
			}
			// Trim one rune at a time to handle multibyte correctly.
			runes := []rune(prefix)
			prefix = string(runes[:len(runes)-1])
		}
	}
	return prefix
}

// finishMinibuffer completes a minibuffer interaction.
func (e *Editor) finishMinibuffer() {
	if !e.minibufActive {
		return
	}
	input := e.minibufBuf.String()
	e.minibufActive = false
	e.minibufHint = ""
	e.minibufCandidates = nil
	e.minibufLastQuery = ""
	fn := e.minibufDoneFunc
	e.minibufDoneFunc = nil
	e.minibufPrompt = ""
	if fn != nil {
		fn(input)
	}
}

// cancelMinibuffer aborts an active minibuffer read.
func (e *Editor) cancelMinibuffer() {
	e.minibufActive = false
	e.minibufHint = ""
	e.minibufCandidates = nil
	e.minibufLastQuery = ""
	e.minibufDoneFunc = nil
	e.minibufPrompt = ""
	e.Message("Quit")
}

// ---------------------------------------------------------------------------
// Kill ring
// ---------------------------------------------------------------------------

// addToKillRing prepends s to the kill ring (max 60 entries).
func (e *Editor) addToKillRing(s string) {
	if s == "" {
		return
	}
	e.killRing = append([]string{s}, e.killRing...)
	if len(e.killRing) > 60 {
		e.killRing = e.killRing[:60]
	}
	e.yankIdx = 0
}

// yank returns the most recent kill ring entry, or "".
func (e *Editor) yank() string {
	if len(e.killRing) == 0 {
		return ""
	}
	return e.killRing[0]
}

// yankPop rotates to the next kill ring entry and returns it.
func (e *Editor) yankPop() string {
	if len(e.killRing) == 0 {
		return ""
	}
	e.yankIdx = (e.yankIdx + 1) % len(e.killRing)
	return e.killRing[e.yankIdx]
}

// ---------------------------------------------------------------------------
// File loading
// ---------------------------------------------------------------------------

// loadFile reads path from disk and returns a buffer containing its content.
// The buffer is added to e.buffers.  If path does not exist an empty buffer
// is returned (so the user can start editing a new file).
func (e *Editor) loadFile(path string) (*buffer.Buffer, error) {
	data, err := os.ReadFile(path) //nolint:gosec // user-provided path is intentional
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if len(data) > 0 && !utf8.Valid(data) {
		return nil, fmt.Errorf("%s: file is not valid UTF-8", path)
	}
	name := filepath.Base(path)

	// Reuse an existing buffer for this file if one already exists.
	for _, b := range e.buffers {
		if b.Filename() == path {
			return b, nil
		}
	}

	var b *buffer.Buffer
	if len(data) > 0 {
		b = buffer.NewWithContent(name, string(data))
	} else {
		b = buffer.New(name)
	}
	b.SetFilename(path)

	// Set mode from extension.
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		b.SetMode("go")
	case ".md", ".markdown":
		b.SetMode("markdown")
	case ".el":
		b.SetMode(modeElisp)
	case ".py":
		b.SetMode("python")
	case ".java":
		b.SetMode("java")
	case ".sh", ".bash":
		b.SetMode("bash")
	default:
		b.SetMode("fundamental")
	}

	e.buffers = append(e.buffers, b)
	return b, nil
}

// ---------------------------------------------------------------------------
// Key dispatch
// ---------------------------------------------------------------------------

// dispatchKey handles a single key event from the terminal.
func (e *Editor) dispatchKey(ev *tcell.EventKey) {
	ke := terminal.ParseKey(ev)
	// Record keystroke for keyboard macro if recording.
	if e.kbdMacroRecording && !e.kbdMacroPlaying {
		e.kbdMacroEvents = append(e.kbdMacroEvents, ke)
	}
	e.dispatchParsedKey(ke)
}

// dispatchParsedKey processes a terminal.KeyEvent through the keymap /
// special-mode dispatch.  Separated from dispatchKey so that macro playback
// can inject events without them being re-recorded.
func (e *Editor) dispatchParsedKey(ke terminal.KeyEvent) {
	mk := keymap.MakeKey(ke.Key, ke.Rune, ke.Mod)

	// While reading from the minibuffer, most keys go directly to it.
	if e.minibufActive {
		e.dispatchMinibufKey(ke)
		return
	}

	// Query-replace intercepts keys.
	if e.queryReplaceActive {
		e.queryReplaceHandleKey(ke)
		return
	}

	// readChar: next key delivers its rune to a callback (used by registers).
	if e.readCharPending {
		e.readCharPending = false
		cb := e.readCharCallback
		e.readCharCallback = nil
		if ke.Key == tcell.KeyRune {
			cb(ke.Rune)
		} else {
			e.Message("Register command cancelled")
		}
		return
	}

	// While in isearch, hand the key to the isearch handler.
	if e.isearching {
		e.isearchHandleKey(ke)
		return
	}

	// describe-key mode: intercept the next complete key sequence.
	if e.describeKeyPending {
		e.handleDescribeKey(ke)
		return
	}

	// After C-u, digits accumulate to form the repeat count (e.g. C-u 8 C-f).
	if e.universalArgSet && ke.Key == tcell.KeyRune && ke.Mod == 0 {
		r := ke.Rune
		if r >= '0' && r <= '9' {
			e.universalArgDigits += string(r)
			e.universalArgTyping = true
			n, _ := strconv.Atoi(e.universalArgDigits)
			e.universalArg = n
			e.Message("C-u %d", n)
			return
		}
		if r == '-' && !e.universalArgTyping {
			e.universalArgDigits = "-"
			e.universalArgTyping = true
			e.Message("C-u -")
			return
		}
	}

	// Determine which keymap to look up in (global or current prefix).
	activeMap := e.globalKeymap
	if e.prefixKeymap != nil {
		activeMap = e.prefixKeymap
	}

	// When in a dired buffer, check the dired keymap first (before prefix maps).
	if e.prefixKeymap == nil && e.ActiveBuffer().Mode() == "dired" {
		if e.diredDispatch(ke) {
			return
		}
	}

	binding, found := activeMap.Lookup(mk)
	if !found {
		// Unrecognised key in a prefix sequence: cancel prefix, beep.
		if e.prefixKeymap != nil {
			e.prefixKeymap = nil
			e.Message("Key sequence incomplete")
			return
		}
		// Plain printable rune → self-insert.
		if ke.Key == tcell.KeyRune && ke.Mod == 0 && unicode.IsPrint(ke.Rune) {
			e.selfInsert(ke.Rune)
		}
		return
	}

	// Clear prefix map now that we have a binding.
	e.prefixKeymap = nil

	if binding.Prefix != nil {
		// Begin a prefix sequence (e.g. C-x).
		e.prefixKeymap = binding.Prefix
		return
	}

	// Execute the bound command.
	if binding.Command != "" {
		e.execCommand(binding.Command)
	}
}

// handleDescribeKey processes one key in describe-key mode.  Multi-key
// sequences (e.g. C-x C-f) are accumulated until a terminal binding is found.
func (e *Editor) handleDescribeKey(ke terminal.KeyEvent) {
	mk := keymap.MakeKey(ke.Key, ke.Rune, ke.Mod)
	km := e.globalKeymap
	if e.describeKeyMap != nil {
		km = e.describeKeyMap
	}
	name := keymap.FormatKey(mk)
	if e.describeKeySeq != "" {
		name = e.describeKeySeq + " " + name
	}

	binding, found := km.Lookup(mk)
	if !found {
		e.describeKeyPending = false
		e.describeKeySeq = ""
		e.describeKeyMap = nil
		e.Message("%s is undefined", name)
		return
	}
	if binding.Prefix != nil {
		// First half of a multi-key sequence (e.g. C-x) — wait for more.
		e.describeKeySeq = name
		e.describeKeyMap = binding.Prefix
		e.Message("%s-", name)
		return
	}

	// Terminal binding: show doc.
	e.describeKeyPending = false
	e.describeKeySeq = ""
	e.describeKeyMap = nil
	e.showCommandHelp(name, binding.Command)
}

// showCommandHelp displays help for command in the *Help* buffer and
// switches the active window to it.  keySeq is the key sequence that invokes
// it (empty when called from describe-function).
func (e *Editor) showCommandHelp(keySeq, cmdName string) {
	var sb strings.Builder
	if keySeq != "" {
		sb.WriteString(keySeq + " runs the command " + cmdName + "\n\n")
	} else {
		sb.WriteString(cmdName + "\n\n")
	}
	doc, ok := commandDocs[cmdName]
	if ok {
		sb.WriteString(doc + "\n")
	} else if _, exists := commands[cmdName]; exists {
		sb.WriteString("Not documented.\n")
	} else {
		sb.WriteString("No such command: " + cmdName + "\n")
	}

	helpBuf := e.FindBuffer("*Help*")
	if helpBuf == nil {
		helpBuf = buffer.NewWithContent("*Help*", sb.String())
		e.buffers = append(e.buffers, helpBuf)
	} else {
		helpBuf.Delete(0, helpBuf.Len())
		helpBuf.InsertString(0, sb.String())
	}
	helpBuf.SetPoint(0)
	e.activeWin.SetBuf(helpBuf)
}

// showVariableHelp displays the name and current value of an Elisp variable
// in the *Help* buffer and switches the active window to it.
func (e *Editor) showVariableHelp(varName string) {
	var body string
	val, ok := e.lisp.GetGlobalVar(varName)
	if !ok {
		body = varName + "\n\nVariable is void (not defined).\n"
	} else {
		body = varName + "\n\nValue: " + val.String() + "\n"
	}
	helpBuf := e.FindBuffer("*Help*")
	if helpBuf == nil {
		helpBuf = buffer.NewWithContent("*Help*", body)
		e.buffers = append(e.buffers, helpBuf)
	} else {
		helpBuf.Delete(0, helpBuf.Len())
		helpBuf.InsertString(0, body)
	}
	helpBuf.SetPoint(0)
	e.activeWin.SetBuf(helpBuf)
}

// dispatchMinibufKey handles a key while the minibuffer is active.
func (e *Editor) dispatchMinibufKey(ke terminal.KeyEvent) {
	buf := e.minibufBuf
	pt := buf.Point()

	// Clear any completion hint on every keypress except TAB itself.
	if ke.Key != tcell.KeyTab {
		e.minibufHint = ""
	}

	// M-<backspace> / M-DEL → backward-kill-word.
	if (ke.Key == tcell.KeyBackspace || ke.Key == tcell.KeyBackspace2) && ke.Mod == tcell.ModAlt {
		e.minibufBackwardKillWord()
		return
	}

	//nolint:exhaustive // external enum; default case handles unknowns
	switch ke.Key {
	case tcell.KeyEnter, tcell.KeyCtrlJ:
		e.finishMinibuffer()
	case tcell.KeyCtrlG, tcell.KeyEscape:
		e.cancelMinibuffer()
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if pt > 0 {
			buf.Delete(pt-1, 1)
			buf.SetPoint(pt - 1)
			e.refreshMinibufCandidates()
		}
	case tcell.KeyDelete:
		// C-d / Delete → delete forward char.
		if pt < buf.Len() {
			buf.Delete(pt, 1)
			e.refreshMinibufCandidates()
		}
	case tcell.KeyTab:
		if len(e.minibufCandidates) > 0 {
			// Insert the currently selected candidate.
			cand := e.minibufCandidates[e.minibufSelectedIdx]
			buf.Delete(0, buf.Len())
			buf.InsertString(0, cand)
			buf.SetPoint(len([]rune(cand)))
			e.refreshMinibufCandidates()
		} else {
			e.minibufComplete()
		}
	case tcell.KeyDown:
		e.minibufSelectNext()
	case tcell.KeyUp:
		e.minibufSelectPrev()
	case tcell.KeyLeft:
		if pt > 0 {
			buf.SetPoint(pt - 1)
		}
	case tcell.KeyRight:
		if pt < buf.Len() {
			buf.SetPoint(pt + 1)
		}
	case tcell.KeyHome:
		buf.SetPoint(0)
	case tcell.KeyEnd:
		buf.SetPoint(buf.Len())
	case tcell.KeyCtrlA:
		buf.SetPoint(0)
	case tcell.KeyCtrlE:
		buf.SetPoint(buf.Len())
	case tcell.KeyCtrlF:
		if pt < buf.Len() {
			buf.SetPoint(pt + 1)
		}
	case tcell.KeyCtrlB:
		if pt > 0 {
			buf.SetPoint(pt - 1)
		}
	case tcell.KeyCtrlD:
		if pt < buf.Len() {
			buf.Delete(pt, 1)
			e.refreshMinibufCandidates()
		}
	case tcell.KeyCtrlK:
		end := buf.EndOfLine(pt)
		if end > pt {
			buf.Delete(pt, end-pt)
			e.refreshMinibufCandidates()
		}
	case tcell.KeyCtrlW:
		// C-w: kill word backward (same as M-<backspace> in minibuffer).
		e.minibufBackwardKillWord()
	default:
		if ke.Key != tcell.KeyRune {
			return
		}
		switch {
		case ke.Mod == tcell.ModAlt && ke.Rune == 'n':
			e.minibufSelectNext()
		case ke.Mod == tcell.ModAlt && ke.Rune == 'p':
			e.minibufSelectPrev()
		case ke.Mod == tcell.ModAlt && ke.Rune == 'f':
			// M-f: forward-word.
			pos := pt
			length := buf.Len()
			for pos < length && !isWordRune(buf.RuneAt(pos)) {
				pos++
			}
			for pos < length && isWordRune(buf.RuneAt(pos)) {
				pos++
			}
			buf.SetPoint(pos)
		case ke.Mod == tcell.ModAlt && ke.Rune == 'b':
			// M-b: backward-word.
			pos := pt
			for pos > 0 && !isWordRune(buf.RuneAt(pos-1)) {
				pos--
			}
			for pos > 0 && isWordRune(buf.RuneAt(pos-1)) {
				pos--
			}
			buf.SetPoint(pos)
		case ke.Mod == tcell.ModAlt && ke.Rune == 'd':
			// M-d: kill-word forward.
			pos := pt
			length := buf.Len()
			for pos < length && !isWordRune(buf.RuneAt(pos)) {
				pos++
			}
			for pos < length && isWordRune(buf.RuneAt(pos)) {
				pos++
			}
			if pos > pt {
				buf.Delete(pt, pos-pt)
				e.refreshMinibufCandidates()
			}
		case ke.Mod == 0 && unicode.IsPrint(ke.Rune):
			buf.Insert(pt, ke.Rune)
			buf.SetPoint(pt + 1)
			e.refreshMinibufCandidates()
		}
	}
}

// minibufBackwardKillWord deletes the word immediately before point in the
// minibuffer (M-<backspace> / M-DEL).
func (e *Editor) minibufBackwardKillWord() {
	buf := e.minibufBuf
	pt := buf.Point()
	pos := pt
	for pos > 0 && !isWordRune(buf.RuneAt(pos-1)) {
		pos--
	}
	for pos > 0 && isWordRune(buf.RuneAt(pos-1)) {
		pos--
	}
	if pos < pt {
		buf.Delete(pos, pt-pos)
		buf.SetPoint(pos)
		e.refreshMinibufCandidates()
	}
}

// minibufComplete performs tab completion in the minibuffer using
// e.minibufCompletions (if set).
func (e *Editor) minibufComplete() {
	if e.minibufCompletions == nil {
		return
	}
	buf := e.minibufBuf
	current := buf.String()
	completions := e.minibufCompletions(current)
	switch len(completions) {
	case 0:
		e.minibufHint = fmt.Sprintf("No completions for %q", current)
	case 1:
		// Unique match: fill it in.
		e.minibufHint = ""
		buf.Delete(0, buf.Len())
		buf.InsertString(0, completions[0])
		buf.SetPoint(len([]rune(completions[0])))
	default:
		prefix := commonPrefix(completions)
		if len([]rune(prefix)) > len([]rune(current)) {
			buf.Delete(0, buf.Len())
			buf.InsertString(0, prefix)
			buf.SetPoint(len([]rune(prefix)))
		}
		// Show up to 15 candidates above the minibuffer line.
		shown := completions
		suffix := ""
		if len(shown) > 15 {
			shown = shown[:15]
			suffix = fmt.Sprintf(" [%d more]", len(completions)-15)
		}
		e.minibufHint = strings.Join(shown, "  ") + suffix
	}
}

// refreshMinibufCandidates recomputes the fuzzy completion popup list from
// the current minibuffer content.  Selection is reset to the first entry.
func (e *Editor) refreshMinibufCandidates() {
	if e.minibufCompletions == nil {
		e.minibufCandidates = nil
		e.minibufLastQuery = ""
		return
	}
	query := e.minibufBuf.String()
	e.minibufLastQuery = query
	e.minibufCandidates = e.minibufCompletions(query)
	e.minibufSelectedIdx = 0
	e.minibufCandidateOffset = 0
}

// minibufSelectNext moves the popup selection one step down.
func (e *Editor) minibufSelectNext() {
	if len(e.minibufCandidates) == 0 {
		return
	}
	if e.minibufSelectedIdx < len(e.minibufCandidates)-1 {
		e.minibufSelectedIdx++
		if e.minibufSelectedIdx >= e.minibufCandidateOffset+5 {
			e.minibufCandidateOffset++
		}
	}
}

// minibufSelectPrev moves the popup selection one step up.
func (e *Editor) minibufSelectPrev() {
	if len(e.minibufCandidates) == 0 {
		return
	}
	if e.minibufSelectedIdx > 0 {
		e.minibufSelectedIdx--
		if e.minibufSelectedIdx < e.minibufCandidateOffset {
			e.minibufCandidateOffset--
		}
	}
}

// fuzzyMatch reports whether query is a case-insensitive subsequence of candidate.
// An empty query always matches.
func fuzzyMatch(candidate, query string) bool {
	if query == "" {
		return true
	}
	qr := []rune(strings.ToLower(query))
	qi := 0
	for _, c := range strings.ToLower(candidate) {
		if c == qr[qi] {
			qi++
			if qi == len(qr) {
				return true
			}
		}
	}
	return false
}

// selfInsert inserts a printable rune at point in the active buffer.
func (e *Editor) selfInsert(r rune) {
	buf := e.ActiveBuffer()
	if buf.ReadOnly() {
		e.Message("Buffer is read-only")
		return
	}
	e.lastCommand = "self-insert-command"
	pt := buf.Point()
	buf.Insert(pt, r)
	buf.SetPoint(pt + 1)
	buf.SetMarkActive(false)
}

// bufReadOnly reports true and shows an error message if the active buffer is
// read-only.  Editing commands call this and return early when it returns true.
func (e *Editor) bufReadOnly() bool {
	if e.ActiveBuffer().ReadOnly() {
		e.Message("Buffer is read-only")
		return true
	}
	return false
}

// execCommand looks up and calls a named command.
func (e *Editor) execCommand(name string) {
	fn, ok := commands[name]
	if !ok {
		e.Message("Unknown command: %s", name)
		return
	}
	// fn reads e.lastCommand (e.g. for C-l cycling) before we overwrite it.
	fn(e)
	e.lastCommand = name
}

// ---------------------------------------------------------------------------
// isearch
// ---------------------------------------------------------------------------

// startIsearch enters interactive search mode.
func (e *Editor) startIsearch(forward bool) {
	e.isearching = true
	e.isearchFwd = forward
	e.isearchStr = ""
	e.isearchStart = e.ActiveBuffer().Point()
}

// isearchHandleKey processes a key during an isearch session.
func (e *Editor) isearchHandleKey(ke terminal.KeyEvent) {
	buf := e.ActiveBuffer()

	//nolint:exhaustive // external enum; default case handles unknowns
	switch ke.Key {
	case tcell.KeyCtrlG, tcell.KeyEscape:
		// Cancel: restore original position.
		buf.SetPoint(e.isearchStart)
		e.isearching = false
		e.isearchStr = ""
		e.Message("Quit")

	case tcell.KeyEnter:
		// Accept current match position.
		e.isearching = false

	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if len(e.isearchStr) > 0 {
			runes := []rune(e.isearchStr)
			e.isearchStr = string(runes[:len(runes)-1])
			e.isearchFind()
		}

	case tcell.KeyCtrlS:
		// C-s during search: find next match forward.
		e.isearchFwd = true
		if e.isearchStr != "" {
			e.isearchFindNext()
		}

	case tcell.KeyCtrlR:
		// C-r during search: find next match backward.
		e.isearchFwd = false
		if e.isearchStr != "" {
			e.isearchFindNext()
		}

	default:
		if ke.Key == tcell.KeyRune && ke.Mod == 0 && unicode.IsPrint(ke.Rune) {
			e.isearchStr += string(ke.Rune)
			e.isearchFind()
		}
	}
}

// isearchFind searches for e.isearchStr from e.isearchStart.
func (e *Editor) isearchFind() {
	buf := e.ActiveBuffer()
	text := buf.String()
	runes := []rune(text)
	needle := []rune(e.isearchStr)

	if len(needle) == 0 {
		buf.SetPoint(e.isearchStart)
		return
	}

	start := e.isearchStart
	if e.isearchFwd {
		for i := start; i <= len(runes)-len(needle); i++ {
			if runesMatch(runes[i:], needle) {
				buf.SetPoint(i + len(needle))
				return
			}
		}
	} else {
		for i := start; i >= 0; i-- {
			if i+len(needle) <= len(runes) && runesMatch(runes[i:], needle) {
				buf.SetPoint(i)
				return
			}
		}
	}
	e.Message("Failing isearch: %s", e.isearchStr)
}

// isearchFindNext finds the next occurrence after the current point.
func (e *Editor) isearchFindNext() {
	buf := e.ActiveBuffer()
	text := buf.String()
	runes := []rune(text)
	needle := []rune(e.isearchStr)

	if len(needle) == 0 {
		return
	}

	cur := buf.Point()
	if e.isearchFwd {
		start := cur
		for i := start; i <= len(runes)-len(needle); i++ {
			if runesMatch(runes[i:], needle) {
				buf.SetPoint(i + len(needle))
				return
			}
		}
		e.Message("Failing isearch: %s", e.isearchStr)
	} else {
		// Search backward from one before current.
		start := max(cur-len(needle)-1, 0)
		for i := start; i >= 0; i-- {
			if i+len(needle) <= len(runes) && runesMatch(runes[i:], needle) {
				buf.SetPoint(i)
				return
			}
		}
		e.Message("Failing isearch: %s", e.isearchStr)
	}
}

// runesMatch reports whether haystack starts with needle.
func runesMatch(haystack, needle []rune) bool {
	if len(haystack) < len(needle) {
		return false
	}
	for i, r := range needle {
		if haystack[i] != r {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------

// Redraw renders the entire editor to the terminal.
func (e *Editor) Redraw() {
	if e.term == nil {
		return
	}
	e.term.Clear()
	for _, w := range e.windows {
		e.renderWindow(w)
		e.renderModeline(w)
	}
	// Draw vertical separators between side-by-side windows.
	for i := 1; i < len(e.windows); i++ {
		w := e.windows[i]
		prev := e.windows[i-1]
		if w.Top() == prev.Top() { // same row → side-by-side
			sepX := w.Left() - 1
			if sepX >= 0 {
				for row := w.Top(); row < w.Top()+w.Height()-1; row++ {
					e.term.SetCell(sepX, row, '│', syntax.FaceDefault)
				}
			}
		}
	}
	e.renderMinibuffer()
	// Position the hardware cursor.
	e.placeCursor()
	e.term.Show()
}

// highlighterFor returns the appropriate syntax highlighter for buf.
func highlighterFor(buf *buffer.Buffer) syntax.Highlighter {
	switch buf.Mode() {
	case "go":
		return syntax.GoHighlighter{}
	case "markdown":
		return syntax.MarkdownHighlighter{}
	case modeElisp:
		return syntax.ElispHighlighter{}
	case "python":
		return syntax.PythonHighlighter{}
	case "java":
		return syntax.JavaHighlighter{}
	case "bash":
		return syntax.BashHighlighter{}
	default:
		return syntax.NilHighlighter{}
	}
}

// faceAtPos returns the syntax face that covers position pos, given a sorted
// slice of spans.  Falls back to FaceDefault.
func faceAtPos(spans []syntax.Span, pos int) syntax.Face {
	for _, sp := range spans {
		if pos >= sp.Start && pos < sp.End {
			return sp.Face
		}
	}
	return syntax.FaceDefault
}

// renderWindow draws the text content of w.
func (e *Editor) renderWindow(w *window.Window) {
	buf := w.Buf()
	hl := highlighterFor(buf)
	text := buf.String()
	runes := []rune(text)

	spans := hl.Highlight(text, 0, len(runes))

	// Narrow region: restrict displayed area.
	narrowMin := buf.NarrowMin()
	narrowMax := buf.NarrowMax()

	// Region boundaries.
	regionActive := buf.MarkActive()
	regionStart, regionEnd := 0, 0
	if regionActive {
		mark := buf.Mark()
		pt := buf.Point()
		if mark < pt {
			regionStart, regionEnd = mark, pt
		} else {
			regionStart, regionEnd = pt, mark
		}
	}

	// isearch match boundaries (highlight the match).
	isearchMatchStart, isearchMatchEnd := -1, -1
	if e.isearching && e.isearchStr != "" {
		needle := []rune(e.isearchStr)
		cur := buf.Point()
		// The most recent match ends at point (forward) or starts at point (backward).
		if e.isearchFwd {
			mstart := cur - len(needle)
			if mstart >= 0 && mstart+len(needle) <= len(runes) &&
				runesMatch(runes[mstart:], needle) {
				isearchMatchStart = mstart
				isearchMatchEnd = cur
			}
		} else if cur+len(needle) <= len(runes) && runesMatch(runes[cur:], needle) {
			isearchMatchStart = cur
			isearchMatchEnd = cur + len(needle)
		}
	}

	// Query-replace match highlight.
	qrMatchStart, qrMatchEnd := -1, -1
	if e.queryReplaceActive && e.queryReplaceMatch >= 0 {
		needle := []rune(e.queryReplaceFrom)
		ms := e.queryReplaceMatch
		me := ms + len(needle)
		if me <= len(runes) && runesMatch(runes[ms:], needle) {
			qrMatchStart = ms
			qrMatchEnd = me
		}
	}

	_, winY, winW, winH := w.Left(), w.Top(), w.Width(), w.Height()
	textH := max(
		// reserve last row for modeline
		winH-1, 1)

	viewLines := w.ViewLines()
	// ViewLines returns height rows; only use the text rows (not the modeline row).
	for rowIdx := 0; rowIdx < textH && rowIdx < len(viewLines); rowIdx++ {
		vl := viewLines[rowIdx]
		screenRow := winY + rowIdx

		if vl.Line == 0 {
			// Past end of buffer — leave the row blank.
			continue
		}

		// Skip lines outside narrow region.
		if vl.StartPos < narrowMin || vl.StartPos >= narrowMax {
			continue
		}

		lineRunes := []rune(vl.Text)
		for col := range winW {
			pos := vl.StartPos + col
			var ch rune
			if col < len(lineRunes) {
				ch = lineRunes[col]
			} else {
				ch = ' '
			}

			face := faceAtPos(spans, pos)

			// Overlay region.
			if regionActive && pos >= regionStart && pos < regionEnd {
				face = syntax.FaceRegion
			}
			// Overlay isearch match.
			if isearchMatchStart >= 0 && pos >= isearchMatchStart && pos < isearchMatchEnd {
				face = syntax.FaceIsearch
			}
			// Overlay query-replace match.
			if qrMatchStart >= 0 && pos >= qrMatchStart && pos < qrMatchEnd {
				face = syntax.FaceIsearch
			}

			e.term.SetCell(w.Left()+col, screenRow, ch, face)
		}
	}
}

// renderModeline draws the mode line for window w.
func (e *Editor) renderModeline(w *window.Window) {
	_, winY, winW, winH := w.Left(), w.Top(), w.Width(), w.Height()
	modeRow := winY + winH - 1

	buf := w.Buf()
	modifiedMark := "-"
	if buf.ReadOnly() {
		modifiedMark = "%%"
	} else if buf.Modified() {
		modifiedMark = "**"
	}
	name := buf.Name()
	line, col := buf.LineCol(buf.Point())
	mode := buf.Mode()
	narrow := ""
	if buf.Narrowed() {
		narrow = " Narrow"
	}
	macro := ""
	if e.kbdMacroRecording {
		macro = " Def"
	}

	label := fmt.Sprintf(" %s  %-20s  (%s%s%s)  L%d C%d ", modifiedMark, name, mode, narrow, macro, line, col)
	// Pad to window width.
	for len(label) < winW {
		label += "-"
	}
	if len(label) > winW {
		label = label[:winW]
	}
	e.term.DrawString(w.Left(), modeRow, label, syntax.FaceModeline)
}

// renderMinibuffer draws the minibuffer / message area at the last row.
func (e *Editor) renderMinibuffer() {
	_, row, width, _ := e.minibufWin.Left(), e.minibufWin.Top(), e.minibufWin.Width(), e.minibufWin.Height()

	var line string
	if e.minibufActive {
		line = e.minibufPrompt + e.minibufBuf.String()
		e.renderCandidatePopup(row, width)
	} else if e.message != "" {
		// Expire messages after 5 seconds.
		age := time.Now().UnixNano() - e.messageTime
		if age < 5*int64(time.Second) {
			line = e.message
		} else {
			e.message = ""
		}
	}

	// Pad/trim to width.
	runes := []rune(line)
	for col := range width {
		ch := ' '
		if col < len(runes) {
			ch = runes[col]
		}
		e.term.SetCell(col, row, ch, syntax.FaceMinibuffer)
	}
}

// renderCandidatePopup draws the fuzzy-completion popup above the minibuffer.
// Up to 5 candidates are shown; the selected one is highlighted.
// Scroll indicators (▲/▼) appear in the first/last visible row when needed.
func (e *Editor) renderCandidatePopup(minibufRow, width int) {
	// Lazy refresh: recompute if the query changed since last draw.
	if e.minibufCompletions != nil {
		query := e.minibufBuf.String()
		if query != e.minibufLastQuery {
			e.refreshMinibufCandidates()
		}
	}

	cands := e.minibufCandidates
	if len(cands) == 0 {
		return
	}

	const maxVisible = 5
	offset := e.minibufCandidateOffset
	end := min(offset+maxVisible, len(cands))
	visible := cands[offset:end]
	nVisible := len(visible)

	startRow := minibufRow - nVisible
	for i, cand := range visible {
		row := startRow + i
		if row < 0 {
			continue
		}
		face := syntax.FaceCandidate
		if offset+i == e.minibufSelectedIdx {
			face = syntax.FaceSelected
		}

		runes := []rune(cand)

		// Add scroll indicator at the right edge of the first/last row.
		var indicator string
		if i == 0 && offset > 0 {
			indicator = fmt.Sprintf(" ▲%d", offset)
		} else if i == nVisible-1 && end < len(cands) {
			indicator = fmt.Sprintf(" ▼%d", len(cands)-end)
		}
		if indicator != "" {
			ir := []rune(indicator)
			// Truncate candidate so indicator fits.
			maxCand := max(width-len(ir), 0)
			if len(runes) > maxCand {
				runes = runes[:maxCand]
			}
			runes = append(runes, ir...)
		}

		for col := range width {
			ch := ' '
			if col < len(runes) {
				ch = runes[col]
			}
			e.term.SetCell(col, row, ch, face)
		}
	}
}

// placeCursor positions the terminal cursor on screen.
func (e *Editor) placeCursor() {
	if e.minibufActive {
		col := len([]rune(e.minibufPrompt)) + e.minibufBuf.Point()
		row := e.minibufWin.Top()
		e.term.ShowCursor(col, row)
		return
	}
	// Cursor in the active window: use the window-local point so each window
	// tracks its own cursor independently from the shared buffer point.
	w := e.activeWin
	buf := w.Buf()
	pt := w.Point()
	line, col := buf.LineCol(pt)
	screenRow := w.Top() + (line - w.ScrollLine())
	e.term.ShowCursor(w.Left()+col, screenRow)
}

// ---------------------------------------------------------------------------
// Public helpers used by main
// ---------------------------------------------------------------------------

// OpenFile loads path into a buffer and makes it current.
// Errors are stored as messages; the function never returns a fatal error.
func (e *Editor) OpenFile(path string) error {
	b, err := e.loadFile(path)
	if err != nil {
		return err
	}
	e.activeWin.SetBuf(b)
	return nil
}

// Close tears down the terminal.  Safe to call after New() fails.
func (e *Editor) Close() {
	if e.term != nil {
		e.term.Close()
	}
}

// evalSexp parses and evaluates an Elisp expression string, returning the
// result as a string.  Used by eval-last-sexp.
func (e *Editor) evalSexp(src string) (string, error) {
	val, err := e.lisp.EvalString(src)
	if err != nil {
		return "", err
	}
	if val == nil {
		return "nil", nil
	}
	return val.String(), nil
}
