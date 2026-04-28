// Package editor wires together all sub-packages into a running Emacs-like
// editor.  Editor is the top-level application object; it owns the terminal,
// all buffers, all windows, the keymap hierarchy and the Elisp evaluator.
package editor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/elisp"
	"github.com/skybert/gomacs/internal/keymap"
	"github.com/skybert/gomacs/internal/lsp"
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
	ctrlXVKeymap *keymap.Keymap // C-x v (version control)
	ctrlXPKeymap *keymap.Keymap // C-x p (project)
	metaGKeymap  *keymap.Keymap // M-g (goto)
	ctrlCKeymap  *keymap.Keymap // C-c prefix map
	ctrlDKeymap  *keymap.Keymap // C-c d (DAP debug)

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

	// isearch rune caches: avoid O(n) buf.String()/[]rune conversions on every
	// keystroke during incremental search.
	isearchRunes     []rune
	isearchRunesGen  int
	isearchRunesBuf  *buffer.Buffer
	isearchNeedle    []rune
	isearchNeedleStr string

	// prefix key state (non-nil while processing a multi-key sequence)
	prefixKeymap *keymap.Keymap
	// prefixKeySeq accumulates the human-readable prefix typed so far
	// (e.g. "C-x v") and is shown in the minibuffer while waiting for the
	// next key.  It is cleared when the sequence completes or is cancelled.
	prefixKeySeq string

	// describe-key state: set by C-h k; intercepts the next key sequence
	// and shows the bound command and its documentation.
	describeKeyPending bool
	describeKeySeq     string
	describeKeyMap     *keymap.Keymap

	// what-key state: intercepts the next key and reports raw details.
	whatKeyPending bool

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
	// minibufHistory stores per-prompt input history (most recent first).
	// Keyed by the prompt string so different commands keep separate histories.
	minibufHistory      map[string][]string
	minibufHistoryIdx   int    // -1 = not in history mode; >=0 = index into hist
	minibufHistorySaved string // text saved before entering history navigation

	// minibufCandidateChosen is true only when the user has explicitly
	// navigated the popup (Tab/Down/Up), so Enter uses the highlighted
	// candidate instead of the raw typed text.
	minibufCandidateChosen bool
	// minibufPreferTyped, when set, is called with the raw typed text on
	// Enter.  If it returns true the typed text is used as-is even when
	// candidates are present.  Used by find-file to prefer a typed
	// directory path over the first completion inside that directory.
	minibufPreferTyped func(string) bool

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

	escPending bool // ESC pressed — next key becomes Meta chord

	// Query replace state.
	queryReplaceActive bool
	queryReplaceFrom   string
	queryReplaceTo     string
	queryReplaceCursor int // position to search from for next match
	queryReplaceMatch  int // start of current highlighted match (-1 if none)

	// Dired state keyed by buffer pointer.
	diredStates map[*buffer.Buffer]*diredState

	// vcLogRoots maps *VC Log* buffers to their git repository root dir.
	vcLogRoots map[*buffer.Buffer]string
	// vcLogFiles maps *VC Log* buffers to the source file path used for the log query.
	vcLogFiles map[*buffer.Buffer]string

	// vcParent maps a child VC output buffer (e.g. *VC Log Message*, *VC Show*)
	// to the parent VC buffer that opened it.  Used by q to navigate back.
	vcParent map[*buffer.Buffer]*buffer.Buffer

	// vcCommitRoots maps *VC Commit* buffers to their git repository root dir.
	vcCommitRoots map[*buffer.Buffer]string

	// bufferMRU tracks the most recently displayed buffers (most recent first).
	// Updated by showBuf; used by vcQuit to return to the right buffer.
	bufferMRU []*buffer.Buffer

	// commandLRU is the most-recently-used command list.  The first entry is
	// the most recently executed command.  Capped at commandLRUMax entries.
	commandLRU []string

	// LSP connections keyed by mode name (e.g. "go").
	lspConns    map[string]*lspConn
	lspDefStack []lspDefPos // jump history for M-,
	// lspOpCancel cancels the in-flight user-initiated LSP operation (M-., C-c h).
	// C-g resets this to cancel whatever is pending.
	lspOpCancel context.CancelFunc
	// lspCbs carries callbacks from async LSP goroutines back to the main loop.
	lspCbs chan func()

	// dap is the active debug session state; nil when no session is running.
	dap *dapState
	// dapBreakpoints stores breakpoints set by the user, keyed by absolute file
	// path → set of 1-based line numbers.  Persists across debug sessions.
	dapBreakpoints map[string]map[int]struct{}
	// dapCbs carries callbacks from async DAP goroutines back to the main loop.
	dapCbs chan func()

	// fillColumn is the target column for fill-paragraph (default 70).
	fillColumn int

	// isSearchCaseFold enables case-insensitive isearch (default true).
	isSearchCaseFold bool

	// subwordMode, when true, makes word-motion commands stop at camelCase
	// boundaries within identifiers (e.g. M-f on FooBar stops at Bar).
	// Disable with (setq subword-mode nil).
	subwordMode bool

	// saveBufferDeleteTrailingWS enables deleting trailing whitespace on save (default true).
	// Set to false via (setq save-buffer-delete-trailing-whitespace nil).
	saveBufferDeleteTrailingWS bool

	// visualLines enables visual line wrapping at 80 characters (default true).
	// Disabled via (setq visual-lines nil) in ~/.gomacs.
	visualLines bool

	// passive LSP hover state (eldoc-style)
	lastHoverFile  string
	lastHoverPoint int
	hoverInflight  bool

	// lsp-show-doc popup state.
	lspDocLines []string // non-nil while doc popup is visible

	// LSP inline completion popup state.
	lspCompActive         bool
	lspCompItems          []lsp.CompletionItem
	lspCompSelectedIdx    int
	lspCompOffset         int
	lspCompInflight       bool
	lspCompWordStart      int
	lspCompletionMinChars int // default lspDefaultCompletionMinChars
	// lspCompDelayCancel cancels a pending delayed buffer-word completion.
	lspCompDelayCancel context.CancelFunc

	// spanCaches caches syntax-highlight spans per buffer so that
	// scrolling (C-n/C-p) does not re-highlight the whole file every frame.
	spanCaches map[*buffer.Buffer]*spanCache

	// spellCommand is the spell-checker executable (default "aspell").
	// Set to "" to disable spell checking.
	spellCommand string
	// spellLanguage is the language code passed to aspell (default "en").
	spellLanguage string
	// spellCaches caches spell-error spans per buffer, keyed by changeGen.
	spellCaches map[*buffer.Buffer]*spellCache
	// spellPending records the changeGen of an in-flight async spell check per buffer.
	spellPending map[*buffer.Buffer]int
	// Interactive spell-check state (M-x spell).
	spellActive      bool
	spellErrors      []spellError
	spellErrorIdx    int
	spellCurrentSugs []string // cached suggestions for the current error

	// version is the build version string (embedded at compile time).
	version string
	// startTime is when the editor was started (for uptime display).
	startTime time.Time

	// dabbrev state: last expansion prefix and list of remaining candidates.
	dabbrevPrefix     string
	dabbrevCandidates []string
	dabbrevIdx        int
	dabbrevLastEnd    int // end of the last expansion in buffer, for cycling

	// compilation state
	compilationErrors   []compilationError
	compilationErrorIdx int
	// compilationExitOK is nil before any build, true if the last build
	// exited 0, false otherwise.  Used to colour the modeline buffer name.
	compilationExitOK *bool
	// customHighlighters overrides the default mode-based highlighter per buffer.
	// Used by the compilation buffer to supply pre-computed ANSI spans.
	customHighlighters map[*buffer.Buffer]syntax.Highlighter

	// autoRevert, when true, silently reloads unmodified buffers when their
	// backing file changes on disk.  Disable with (setq auto-revert nil).
	autoRevert bool
	// autoRevertMtimes tracks the file modification time at the last load or
	// save for each buffer that has a backing file.
	autoRevertMtimes map[*buffer.Buffer]time.Time
	// autoRevertLastCheck is the last time we polled file mtimes.
	autoRevertLastCheck time.Time

	// shellStates maps shell buffers (mode=="shell") to their live PTY state.
	shellStates map[*buffer.Buffer]*shellState

	// Layout cache: skip relayoutWindows when terminal size and footer haven't
	// changed since the last layout pass.
	lastLayoutW      int
	lastLayoutH      int
	lastLayoutFooter int

	// visualLinesSynced tracks whether applyVisualLines has been applied for
	// the current visualLines setting and window configuration.
	visualLinesSynced bool

	// window-jump (M-o ace-window style) state.
	windowJumpActive bool
	windowJumpMap    map[rune]*window.Window

	// layoutRoot is the root of the window split tree.  It is used by
	// relayoutWindows to correctly redistribute screen area after mixed
	// horizontal + vertical splits.
	layoutRoot *layoutNode
}

// spanCache holds pre-computed highlighting data for a buffer.  It is
// invalidated whenever the buffer content or mode changes.
type spanCache struct {
	gen   int
	mode  string
	text  string
	runes []rune
	spans []syntax.Span
}

// ---------------------------------------------------------------------------
// Construction
// ---------------------------------------------------------------------------

// Options controls optional editor startup behaviour.
type Options struct {
	// Quick suppresses loading of the user init file, equivalent to emacs -Q.
	Quick bool
	// StdinData, if non-nil, is opened as a read-only *stdin* buffer on startup.
	// The caller must read and close os.Stdin before calling New so that tcell
	// can claim /dev/tty for keyboard input.
	StdinData []byte
	// Version is the build version string embedded at compile time.
	Version string
}

// / New creates and initialises the editor: terminal, scratch buffer, keymaps,
// windows, and the Elisp evaluator.  It also attempts to load the user's init
// file (~/.gomacs or ~/.config/gomacs/init.el) unless opts.Quick is true.
func New(opts Options) (*Editor, error) {
	term, err := terminal.New()
	if err != nil {
		return nil, fmt.Errorf("editor.New: %w", err)
	}

	_, nopCancel := context.WithCancel(context.Background())
	e := &Editor{
		term:                       term,
		universalArg:               1,
		lisp:                       elisp.NewEvaluator(),
		registers:                  make(map[rune]register),
		diredStates:                make(map[*buffer.Buffer]*diredState),
		vcLogRoots:                 make(map[*buffer.Buffer]string),
		vcLogFiles:                 make(map[*buffer.Buffer]string),
		vcParent:                   make(map[*buffer.Buffer]*buffer.Buffer),
		vcCommitRoots:              make(map[*buffer.Buffer]string),
		fillColumn:                 70,
		isSearchCaseFold:           true,
		subwordMode:                true,
		saveBufferDeleteTrailingWS: true,
		visualLines:                true,
		spellCommand:               "aspell",
		spellLanguage:              "en",
		lspConns:                   make(map[string]*lspConn),
		lspOpCancel:                nopCancel,
		lspCompDelayCancel:         nopCancel,
		lspCbs:                     make(chan func(), 16),
		dapBreakpoints:             make(map[string]map[int]struct{}),
		dapCbs:                     make(chan func(), 16),
		version:                    opts.Version,
		startTime:                  time.Now(),
		customHighlighters:         make(map[*buffer.Buffer]syntax.Highlighter),
		autoRevert:                 true,
		autoRevertMtimes:           make(map[*buffer.Buffer]time.Time),
		shellStates:                make(map[*buffer.Buffer]*shellState),
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
		if e.term != nil {
			e.term.InvalidateStyleCache()
		}
		return elisp.Nil{}, nil
	})

	// (setq theme 'sweet) is an alternative to (load-theme 'sweet).
	// The hook fires immediately when the variable is assigned.
	e.lisp.SetSetqHook("theme", func(v elisp.Value) {
		name := strings.Trim(v.String(), `'"`)
		if syntax.LoadTheme(name) && e.term != nil {
			e.term.InvalidateStyleCache()
		}
	})

	// set-face-attribute lets users tweak individual face colours from ~/.gomacs.
	// Usage: (set-face-attribute 'keyword :foreground "#e17df3" :bold t)
	// Emacs-style frame argument (nil) is accepted and ignored.
	e.lisp.RegisterGoFn("set-face-attribute", func(args []elisp.Value, _ *elisp.Env) (elisp.Value, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("set-face-attribute: expected face name")
		}
		faceName := strings.Trim(args[0].String(), `'"`)
		facePtr, ok := syntax.GetFacePtr(faceName)
		if !ok {
			return nil, fmt.Errorf("set-face-attribute: unknown face %q", faceName)
		}
		i := 1
		// Skip optional nil frame argument (Emacs compatibility).
		if i < len(args) && elisp.IsNil(args[i]) {
			i++
		}
		for i+1 < len(args) {
			kw := args[i].String()
			val := args[i+1]
			i += 2
			switch kw {
			case ":foreground":
				facePtr.Fg = strings.Trim(val.String(), `'"`)
			case ":background":
				facePtr.Bg = strings.Trim(val.String(), `'"`)
			case ":bold":
				facePtr.Bold = !elisp.IsNil(val)
			case ":italic":
				facePtr.Italic = !elisp.IsNil(val)
			case ":underline":
				facePtr.Underline = !elisp.IsNil(val)
			case ":reverse":
				facePtr.Reverse = !elisp.IsNil(val)
			default:
				return nil, fmt.Errorf("set-face-attribute: unknown attribute %q", kw)
			}
		}
		if e.term != nil {
			e.term.InvalidateStyleCache()
		}
		return elisp.Nil{}, nil
	})

	// define-gomacs-theme registers a named theme built from face specs.
	// Usage:
	//   (define-gomacs-theme "my-theme"
	//     '((keyword :foreground "#e17df3" :bold t)
	//       (string  :foreground "#06c993")))
	//   (setq theme "my-theme")   ; or (load-theme "my-theme")
	e.lisp.RegisterGoFn("define-gomacs-theme", func(args []elisp.Value, _ *elisp.Env) (elisp.Value, error) {
		if len(args) < 2 {
			return nil, fmt.Errorf("define-gomacs-theme: expected name and face-spec list")
		}
		name := strings.Trim(args[0].String(), `'"`)
		faceSpecs, ok := elisp.ToSlice(args[1])
		if !ok {
			return nil, fmt.Errorf("define-gomacs-theme: second argument must be a list")
		}

		type faceOverride struct {
			name string
			face syntax.Face
		}
		overrides := make([]faceOverride, 0, len(faceSpecs))

		for _, spec := range faceSpecs {
			fields, ok2 := elisp.ToSlice(spec)
			if !ok2 || len(fields) < 1 {
				return nil, fmt.Errorf("define-gomacs-theme: invalid face spec %s", spec.String())
			}
			faceName := strings.Trim(fields[0].String(), `'"`)
			facePtr, ok2 := syntax.GetFacePtr(faceName)
			if !ok2 {
				return nil, fmt.Errorf("define-gomacs-theme: unknown face %q", faceName)
			}
			// Start from the current face value so unset attrs keep their value.
			f := *facePtr
			for j := 1; j+1 < len(fields); j += 2 {
				kw := fields[j].String()
				val := fields[j+1]
				switch kw {
				case ":foreground":
					f.Fg = strings.Trim(val.String(), `'"`)
				case ":background":
					f.Bg = strings.Trim(val.String(), `'"`)
				case ":bold":
					f.Bold = !elisp.IsNil(val)
				case ":italic":
					f.Italic = !elisp.IsNil(val)
				case ":underline":
					f.Underline = !elisp.IsNil(val)
				case ":reverse":
					f.Reverse = !elisp.IsNil(val)
				default:
					return nil, fmt.Errorf("define-gomacs-theme: unknown attribute %q in face %q", kw, faceName)
				}
			}
			overrides = append(overrides, faceOverride{name: faceName, face: f})
		}

		syntax.RegisterTheme(name, func() {
			for _, o := range overrides {
				syntax.SetFaceByName(o.name, o.face)
			}
		})
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
	e.layoutRoot = leafNode(mainWin)

	// Minibuffer window is the very last row.
	e.minibufWin = window.New(e.minibufBuf, h-1, 0, w, 1)

	// Load user init file (skipped with -Q).
	if !opts.Quick {
		e.loadInitFile()
	}

	// Apply visual-lines wrap column (may have been changed by init file,
	// or is the default true value set above).
	e.applyVisualLines()

	// If the caller read piped stdin data, open it as a *stdin* buffer.
	if len(opts.StdinData) > 0 {
		if utf8.Valid(opts.StdinData) {
			stdinBuf := buffer.NewWithContent("*stdin*", string(opts.StdinData))
			stdinBuf.SetPoint(0)
			e.buffers = append(e.buffers, stdinBuf)
			e.activeWin.SetBuf(stdinBuf)
		} else {
			e.Message("stdin: not valid UTF-8, ignored")
		}
	}

	return e, nil
}

func (e *Editor) setupKeymaps() {
	e.globalKeymap = keymap.New("global")
	e.ctrlXKeymap = keymap.New("C-x")
	e.ctrlHKeymap = keymap.New("C-h")
	e.ctrlXNKeymap = keymap.New("C-x n")
	e.ctrlXRKeymap = keymap.New("C-x r")
	e.ctrlXVKeymap = keymap.New("C-x v")
	e.ctrlXPKeymap = keymap.New("C-x p")
	e.metaGKeymap = keymap.New("M-g")
	e.ctrlCKeymap = keymap.New("C-c")
	e.ctrlDKeymap = keymap.New("C-c d")

	gk := e.globalKeymap
	cx := e.ctrlXKeymap
	ch := e.ctrlHKeymap
	cxn := e.ctrlXNKeymap
	cxr := e.ctrlXRKeymap
	cxv := e.ctrlXVKeymap
	mg := e.metaGKeymap
	cc := e.ctrlCKeymap

	// ---- C-x prefix --------------------------------------------------------
	gk.BindPrefix(keymap.CtrlKey('x'), cx)

	// ---- C-c prefix (mode-specific / LSP) ----------------------------------
	gk.BindPrefix(keymap.CtrlKey('c'), cc)
	cc.Bind(keymap.PlainKey('h'), "lsp-show-doc")
	cc.Bind(keymap.PlainKey(','), "imenu")

	// ---- C-c d prefix (DAP debug) ------------------------------------------
	cd := e.ctrlDKeymap
	cc.BindPrefix(keymap.PlainKey('d'), cd)
	cd.Bind(keymap.PlainKey('b'), "debug-toggle-breakpoint")
	cd.Bind(keymap.PlainKey('d'), "debug-start")
	cd.Bind(keymap.PlainKey('c'), "debug-continue")
	cd.Bind(keymap.PlainKey('n'), "debug-step-next")
	cd.Bind(keymap.PlainKey('i'), "debug-step-in")
	cd.Bind(keymap.PlainKey('o'), "debug-step-out")
	cd.Bind(keymap.PlainKey('e'), "debug-eval")
	cd.Bind(keymap.PlainKey('q'), "debug-exit")

	// ---- C-h prefix (help) -------------------------------------------------
	gk.BindPrefix(keymap.CtrlKey('h'), ch)
	ch.Bind(keymap.PlainKey('k'), "describe-key")
	ch.Bind(keymap.PlainKey('f'), "describe-function")
	ch.Bind(keymap.PlainKey('v'), "describe-variable")
	ch.Bind(keymap.PlainKey('h'), "help")

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
	cxr.Bind(keymap.MakeKey(tcell.KeyRune, ' ', tcell.ModCtrl), "point-to-register")
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
	gk.Bind(keymap.MetaKey('.'), "lsp-find-definition")
	gk.Bind(keymap.MetaKey(','), "lsp-pop-definition")
	gk.Bind(keymap.MetaKey('?'), "lsp-find-references")
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
	gk.Bind(keymap.CtrlKey('k'), "kill-line")
	gk.Bind(keymap.CtrlKey('w'), "kill-region")
	gk.Bind(keymap.MetaKey('w'), "copy-region-as-kill")
	gk.Bind(keymap.CtrlKey('y'), "yank")
	gk.Bind(keymap.MetaKey('y'), "yank-pop")
	gk.Bind(keymap.MetaKey('d'), "kill-word")
	gk.Bind(keymap.MakeKey(tcell.KeyBackspace, 0, tcell.ModAlt), "backward-kill-word")
	gk.Bind(keymap.MetaKey('/'), "dabbrev-expand")
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
	gk.Bind(keymap.MetaKey('o'), "window-jump")

	// Word-case commands (M-u / M-l / M-c)
	gk.Bind(keymap.MetaKey('u'), "upcase-word")
	gk.Bind(keymap.MetaKey('l'), "downcase-word")
	gk.Bind(keymap.MetaKey('c'), "capitalize-word")

	// C-M bindings (represented as Meta + Ctrl combos)
	gk.Bind(keymap.MakeKey(tcell.KeyRune, '\\', tcell.ModCtrl|tcell.ModAlt), "indent-region")
	gk.Bind(keymap.MakeKey(tcell.KeyRune, 'n', tcell.ModCtrl|tcell.ModAlt), "forward-list")
	gk.Bind(keymap.MakeKey(tcell.KeyRune, 'p', tcell.ModCtrl|tcell.ModAlt), "backward-list")

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
	cx.Bind(keymap.PlainKey('l'), "count-buffer-lines")
	cx.Bind(keymap.CtrlKey('o'), "delete-blank-lines")
	cx.Bind(keymap.CtrlKey('u'), "upcase-region")
	cx.Bind(keymap.CtrlKey('l'), "downcase-region")
	cx.Bind(keymap.PlainKey('f'), "set-fill-column")
	cx.Bind(keymap.MakeKey(tcell.KeyTab, 0, 0), "indent-rigidly")
	cx.Bind(keymap.PlainKey('('), "start-kbd-macro")
	cx.Bind(keymap.PlainKey(')'), "end-kbd-macro")
	cx.Bind(keymap.PlainKey('e'), "call-last-kbd-macro")
	cx.Bind(keymap.PlainKey('`'), "next-error")

	// ---- C-x v prefix (version control) ------------------------------------
	cx.BindPrefix(keymap.PlainKey('v'), cxv)
	cxv.Bind(keymap.PlainKey('l'), "vc-print-log")
	cxv.Bind(keymap.PlainKey('='), "vc-diff")
	cxv.Bind(keymap.PlainKey('s'), "vc-status")
	cxv.Bind(keymap.PlainKey('g'), "vc-annotate")
	cxv.Bind(keymap.PlainKey('G'), "vc-grep")
	cxv.Bind(keymap.PlainKey('v'), "vc-next-action")
	cxv.Bind(keymap.PlainKey('u'), "vc-revert")

	// ---- C-x p prefix (project) --------------------------------------------
	cxp := e.ctrlXPKeymap
	cx.BindPrefix(keymap.PlainKey('p'), cxp)
	cxp.Bind(keymap.PlainKey('f'), "project-find-file")
	cxp.Bind(keymap.PlainKey('g'), "project-grep")
	cxp.Bind(keymap.PlainKey('!'), "project-build")
}

// loadInitFile tries ~/.gomacs and ~/.config/gomacs/init.el in that order.
func (e *Editor) loadInitFile() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	candidates := []string{
		filepath.Join(home, ".gomacs"),
		filepath.Join(home, ".config", "gomacs", "init.el"),
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			_ = e.lisp.EvalFile(path)
			e.applyElispConfig()
			return
		}
	}
}

// applyElispConfig reads configurable variables from the Elisp environment and
// applies them to the editor state.  It is called after loadInitFile so that
// user settings in ~/.gomacs (or init.el) take effect immediately.
func (e *Editor) applyElispConfig() {
	if v, ok := e.lisp.GetGlobalVar("fill-column"); ok {
		if i, ok := v.(elisp.Int); ok && i.V > 0 {
			e.fillColumn = int(i.V)
		}
	}
	if v, ok := e.lisp.GetGlobalVar("isearch-case-insensitive"); ok {
		switch val := v.(type) {
		case elisp.Bool:
			e.isSearchCaseFold = val.V
		case elisp.Nil:
			e.isSearchCaseFold = false
		}
	}
	if v, ok := e.lisp.GetGlobalVar("subword-mode"); ok {
		switch val := v.(type) {
		case elisp.Bool:
			e.subwordMode = val.V
		case elisp.Nil:
			e.subwordMode = false
		}
	}
	if v, ok := e.lisp.GetGlobalVar("lsp-completion-min-chars"); ok {
		if i, ok := v.(elisp.Int); ok && i.V > 0 {
			e.lspCompletionMinChars = int(i.V)
		}
	}
	if v, ok := e.lisp.GetGlobalVar("completion-menu-trigger-chars"); ok {
		if i, ok := v.(elisp.Int); ok && i.V > 0 {
			e.lspCompletionMinChars = int(i.V)
		}
	}
	if v, ok := e.lisp.GetGlobalVar("save-buffer-delete-trailing-whitespace"); ok {
		switch val := v.(type) {
		case elisp.Bool:
			e.saveBufferDeleteTrailingWS = val.V
		case elisp.Nil:
			e.saveBufferDeleteTrailingWS = false
		}
	}
	// Also accept the shorter alias.
	if v, ok := e.lisp.GetGlobalVar("delete-trailing-whitespace"); ok {
		switch val := v.(type) {
		case elisp.Bool:
			e.saveBufferDeleteTrailingWS = val.V
		case elisp.Nil:
			e.saveBufferDeleteTrailingWS = false
		}
	}
	if v, ok := e.lisp.GetGlobalVar("visual-lines"); ok {
		switch val := v.(type) {
		case elisp.Bool:
			e.visualLines = val.V
		case elisp.Nil:
			e.visualLines = false
		}
		e.markVisualLinesDirty()
	}
	if v, ok := e.lisp.GetGlobalVar("spell-command"); ok {
		if s, ok := v.(elisp.StringVal); ok && s.V != "" {
			e.spellCommand = s.V
		}
	}
	if v, ok := e.lisp.GetGlobalVar("spell-language"); ok {
		if s, ok := v.(elisp.StringVal); ok && s.V != "" {
			e.spellLanguage = s.V
		}
	}
	if v, ok := e.lisp.GetGlobalVar("auto-revert"); ok {
		switch val := v.(type) {
		case elisp.Bool:
			e.autoRevert = val.V
		case elisp.Nil:
			e.autoRevert = false
		}
	}
	if v, ok := e.lisp.GetGlobalVar("debug-locals-auto-expand-depth"); ok {
		if i, ok := v.(elisp.Int); ok && i.V > 0 && e.dap != nil {
			e.dap.localsAutoExpandDepth = int(i.V)
		}
	}
	e.applyVisualLines()
}

// ---------------------------------------------------------------------------
// Main event loop
// ---------------------------------------------------------------------------

// Run starts the editor's main event loop.  It blocks until the user quits.
func (e *Editor) Run() {
	defer e.term.Close()
	e.Redraw()
	for !e.quit {
		ev := e.term.PollEvent() // block until at least one event arrives
		e.processEvent(ev)
		// Drain any additional events that arrived while we were processing
		// the first one.  This coalesces rapid key-repeat, paste bursts, and
		// macro playback into a single Redraw() at the end.
		for !e.quit {
			next := e.term.TryPollEvent()
			if next == nil {
				break
			}
			e.processEvent(next)
		}
		e.Redraw()
		e.lspMaybeHover()
		e.maybeAutoRevert()
	}
}

// processEvent handles a single tcell event.
func (e *Editor) processEvent(ev tcell.Event) {
	switch ev := ev.(type) {
	case *tcell.EventResize:
		e.handleResize()
	case *tcell.EventKey:
		e.dispatchKey(ev)
		// Sync window-local point from the buffer before scrolling.
		e.syncWindowPoint(e.activeWin)
		e.activeWin.EnsurePointVisible()
	case *tcell.EventInterrupt:
		// Drain all pending LSP callbacks posted by async goroutines.
		for {
			select {
			case fn := <-e.lspCbs:
				fn()
			case fn := <-e.dapCbs:
				fn()
			default:
				return
			}
		}
	}
}

// syncWindowPoint copies buf.Point() into the window's local point field so
// that EnsurePointVisible and Recenter operate on the current cursor position.
func (e *Editor) syncWindowPoint(w *window.Window) {
	w.SetPoint(w.Buf().Point())
}

// maybeAutoRevert checks whether any open buffer's backing file has been
// modified on disk since it was last loaded or saved.  If auto-revert is
// enabled and the buffer has no unsaved changes, it silently reloads the
// buffer.  Checks are throttled to once every 2 seconds.
func (e *Editor) maybeAutoRevert() {
	if !e.autoRevert {
		return
	}
	if time.Since(e.autoRevertLastCheck) < 2*time.Second {
		return
	}
	e.autoRevertLastCheck = time.Now()
	for _, b := range e.buffers {
		path := b.Filename()
		if path == "" || b.Modified() {
			continue
		}
		info, err := os.Stat(path) //nolint:gosec
		if err != nil {
			continue
		}
		recorded, seen := e.autoRevertMtimes[b]
		if !seen || !info.ModTime().After(recorded) {
			continue
		}
		// File changed on disk — reload silently.
		data, err := os.ReadFile(path) //nolint:gosec
		if err != nil {
			continue
		}
		pt := b.Point()
		b.Delete(0, b.Len())
		b.InsertString(0, string(data))
		b.SetModified(false)
		b.SetPoint(min(pt, b.Len()))
		e.autoRevertMtimes[b] = info.ModTime()
		delete(e.spanCaches, b)
		e.Message("Reverted buffer %s", b.Name())
	}
}

// handleResize adjusts all windows to the new terminal dimensions.
func (e *Editor) handleResize() {
	w, h := e.term.Size()
	e.minibufWin.SetRegion(h-1, 0, w, 1)
	e.relayoutWindows(w, h-1)
	e.invalidateLayout()
}

// invalidateLayout forces the next Redraw to re-run relayoutWindows even if
// the terminal size hasn't changed.  Call this after split/delete-window ops.
func (e *Editor) invalidateLayout() {
	e.lastLayoutW = -1
	e.markVisualLinesDirty()
}

// relayoutWindows redistributes the available screen area (totalW×totalH)
// among all non-minibuffer windows using the split tree (e.layoutRoot).
func (e *Editor) relayoutWindows(totalW, totalH int) {
	n := len(e.windows)
	if n == 0 {
		return
	}
	// DAP mode uses a fixed 4-window layout managed separately.
	if e.dap != nil && n == 4 {
		e.dapRelayoutWindows(totalW, totalH)
		return
	}
	if e.layoutRoot == nil {
		// Safety fallback: rebuild a flat tree from current windows.
		e.rebuildLayoutTree()
	}
	e.layoutRoot.applyLayout(0, 0, totalW, totalH)
}

// rebuildLayoutTree constructs a flat horizontal split tree from the current
// window slice.  This is only used as a safety fallback; normally layoutRoot
// is maintained incrementally by the split/delete commands.
func (e *Editor) rebuildLayoutTree() {
	if len(e.windows) == 0 {
		return
	}
	root := leafNode(e.windows[0])
	for _, w := range e.windows[1:] {
		root = &layoutNode{
			dir:      layoutHoriz,
			children: [2]*layoutNode{root, leafNode(w)},
		}
	}
	e.layoutRoot = root
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

// removeWindowShowingBuf removes the window that shows buf (if any) and
// gives its screen area back to the adjacent window.  The active window is
// never removed.  No-op when only one window exists or buf is not visible.
func (e *Editor) removeWindowShowingBuf(buf *buffer.Buffer) {
	if len(e.windows) <= 1 {
		return
	}
	idx := -1
	for i, w := range e.windows {
		if w != e.activeWin && w.Buf() == buf {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	toRemove := e.windows[idx]
	// Remove from the layout tree and trigger a full relayout.
	if e.layoutRoot != nil {
		e.layoutRoot = e.layoutRoot.removeLeaf(toRemove)
	}
	// Remove toRemove from the slice.
	e.windows = append(e.windows[:idx], e.windows[idx+1:]...)
	e.invalidateLayout()
}

// showBuf displays b in the active window and records the previous buffer
// in bufferMRU so that vcQuit (and future "switch to previous buffer" commands)
// can return to the right place.
func (e *Editor) showBuf(b *buffer.Buffer) {
	prev := e.activeWin.Buf()
	if prev != nil && prev != b {
		// Prepend prev to MRU, deduplicating.
		filtered := e.bufferMRU[:0]
		for _, entry := range e.bufferMRU {
			if entry != prev {
				filtered = append(filtered, entry)
			}
		}
		e.bufferMRU = append([]*buffer.Buffer{prev}, filtered...)
		if len(e.bufferMRU) > 20 {
			e.bufferMRU = e.bufferMRU[:20]
		}
	}
	e.activeWin.SetBuf(b)
	e.markVisualLinesDirty()
}

// SwitchToBuffer displays the buffer with name in the active window,
// creating it if it does not exist.
func (e *Editor) SwitchToBuffer(name string) *buffer.Buffer {
	b := e.FindBuffer(name)
	if b == nil {
		b = buffer.New(name)
		e.buffers = append(e.buffers, b)
	}
	e.showBuf(b)
	return b
}

// KillBuffer removes the named buffer.  If the active window was displaying
// it, the window switches to *scratch* (creating it if necessary).
func (e *Editor) KillBuffer(name string) {
	var remaining []*buffer.Buffer
	for _, b := range e.buffers {
		if b.Name() != name {
			remaining = append(remaining, b)
		} else {
			delete(e.spanCaches, b)
			// Clean up PTY state if this is a shell buffer.
			if st, ok := e.shellStates[b]; ok {
				st.close()
				delete(e.shellStates, b)
			}
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
	msg := fmt.Sprintf(format, args...)
	e.message = msg
	e.messageTime = time.Now().UnixNano()
	e.appendToMessagesBuffer(msg)
}

// appendToMessagesBuffer appends msg to the *messages* buffer, creating it if
// needed.  The buffer is kept read-only between appends.
func (e *Editor) appendToMessagesBuffer(msg string) {
	b := e.FindBuffer("*messages*")
	if b == nil {
		b = buffer.NewWithContent("*messages*", "")
		e.buffers = append(e.buffers, b)
	}
	b.SetReadOnly(false)
	pos := b.Len()
	b.InsertString(pos, msg+"\n")
	b.SetReadOnly(true)
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
	e.minibufPreferTyped = nil // reset; caller may set via SetMinibufPreferTyped
	e.minibufHint = ""
	e.minibufHistoryIdx = -1
	e.minibufHistorySaved = ""
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

// SetMinibufPreferTyped registers a predicate that, when it returns true for
// the raw typed text, causes Enter to submit the typed text rather than the
// first popup candidate.  Call immediately after ReadMinibuffer.
func (e *Editor) SetMinibufPreferTyped(fn func(string) bool) {
	e.minibufPreferTyped = fn
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
	// Use the typed text when:
	//   (a) the caller says to prefer it (e.g. find-file with a valid path), or
	//   (b) the user has not explicitly navigated the popup.
	// Otherwise use the highlighted candidate.
	input := e.minibufBuf.String()
	if len(e.minibufCandidates) > 0 {
		idx := e.minibufSelectedIdx
		if idx < 0 || idx >= len(e.minibufCandidates) {
			idx = 0
		}
		if e.minibufCandidateChosen {
			// User explicitly navigated the popup — always use their selection.
			input = e.minibufCandidates[idx]
		} else if e.minibufPreferTyped == nil || !e.minibufPreferTyped(input) {
			// No explicit navigation; typed text is not a valid existing path — use top candidate.
			input = e.minibufCandidates[idx]
		}
	}
	// Save non-empty input to history for this prompt.
	if input != "" {
		prompt := e.minibufPrompt
		if e.minibufHistory == nil {
			e.minibufHistory = make(map[string][]string)
		}
		hist := e.minibufHistory[prompt]
		filtered := hist[:0]
		for _, h := range hist {
			if h != input {
				filtered = append(filtered, h)
			}
		}
		e.minibufHistory[prompt] = append([]string{input}, filtered...)
		if len(e.minibufHistory[prompt]) > 50 {
			e.minibufHistory[prompt] = e.minibufHistory[prompt][:50]
		}
	}
	e.minibufActive = false
	e.minibufHint = ""
	e.minibufCandidates = nil
	e.minibufCandidateChosen = false
	e.minibufPreferTyped = nil
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
	e.minibufCandidateChosen = false
	e.minibufPreferTyped = nil
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
	clipboardWrite(s)
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
	b.SetPoint(0)
	b.SetFilename(path)

	// Set mode from extension.
	ext := strings.ToLower(filepath.Ext(path))
	base := strings.ToLower(filepath.Base(path))
	switch {
	case ext == ".go":
		b.SetMode("go")
	case ext == ".md" || ext == ".markdown":
		b.SetMode("markdown")
	case ext == ".el" || ext == ".emacs" || ext == ".gomacs":
		b.SetMode(modeElisp)
	case ext == ".py":
		b.SetMode("python")
	case ext == ".java":
		b.SetMode("java")
	case ext == ".txt":
		b.SetMode("text")
	case ext == ".sh" || ext == ".bash" || base == ".bashrc" || base == ".zshrc":
		b.SetMode("bash")
	case ext == ".pl" || ext == ".pm" || ext == ".t":
		b.SetMode("perl")
	case ext == ".feature":
		b.SetMode("gherkin")
	case ext == ".conf" || ext == ".toml" || strings.HasSuffix(base, "rc"):
		b.SetMode("conf")
	case ext == ".json":
		b.SetMode("json")
	case ext == ".yaml" || ext == ".yml":
		b.SetMode("yaml")
	case ext == ".mk" || base == "makefile" || base == "gnumakefile" || base == "bsdmakefile":
		b.SetMode("makefile")
	default:
		if mode := modeFromShebang(b.String()); mode != "" {
			b.SetMode(mode)
		} else {
			b.SetMode("fundamental")
		}
	}

	e.buffers = append(e.buffers, b)
	// Record mtime for auto-revert tracking.
	if info, err2 := os.Stat(path); err2 == nil { //nolint:gosec
		e.autoRevertMtimes[b] = info.ModTime()
	}
	// Start LSP server for this file's mode if one is configured.
	e.lspActivate(b)
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

	// Dismiss lsp-show-doc popup on any key.
	if e.lspDocLines != nil {
		e.lspDocLines = nil
	}

	// LSP completion popup: navigation keys go to the popup; other keys
	// dismiss the popup and fall through to normal dispatch.
	if e.lspCompActive {
		if e.lspCompletionHandleKey(ke) {
			return
		}
		// Key was not consumed — popup is now dismissed; continue dispatch.
	}

	// Query-replace intercepts keys.
	if e.queryReplaceActive {
		e.queryReplaceHandleKey(ke)
		return
	}

	// Interactive spell check intercepts keys.
	// Exception: C-x prefix keys fall through so that C-x C-c always works.
	if e.spellActive {
		if ke.Key == tcell.KeyCtrlX {
			e.spellActive = false
		} else {
			e.spellHandleKey(ke)
			return
		}
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
		// If ESC prefix is pending, fold it into the key as ModAlt before describing.
		if e.escPending {
			e.escPending = false
			ke.Mod |= tcell.ModAlt
		}
		e.handleDescribeKey(ke)
		return
	}

	// what-key mode: report raw key details then cancel.
	if e.whatKeyPending {
		e.whatKeyPending = false
		mk := keymap.MakeKey(ke.Key, ke.Rune, ke.Mod)
		e.Message("key=%d rune=%d(%s) mod=%d → %s",
			int(ke.Key), int(ke.Rune), string(ke.Rune),
			int(ke.Mod), keymap.FormatKey(mk))
		return
	}

	// ESC-prefix Meta: ESC followed by a key synthesises the Meta chord.
	// This handles terminals that deliver M-<key> as ESC + <key> rather
	// than a single event with ModAlt set (e.g. kitty on macOS).
	if e.escPending {
		e.escPending = false
		if ke.Key == tcell.KeyEscape {
			// ESC ESC → cancel prefix (no-op)
			e.Message("")
			return
		}
		// Apply ModAlt to all key types (rune and special keys alike) so
		// that ESC + <special> (e.g. ESC + Delete = M-DEL) works uniformly.
		ke.Mod |= tcell.ModAlt
		// Re-enter dispatch with the augmented key (no re-recording needed).
		e.dispatchParsedKey(ke)
		return
	}

	// ESC alone (outside special modes) starts the ESC-prefix Meta sequence.
	if ke.Key == tcell.KeyEscape && ke.Mod == 0 {
		e.escPending = true
		e.Message("ESC-")
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
	// Window-jump mode: at this point ESC-prefix Meta has already been assembled,
	// so the key arriving here is the real intended key (plain letter or C-g).
	if e.windowJumpActive {
		e.windowJumpHandleKey(ke)
		return
	}

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

	// When in a *Buffer List* buffer, handle navigation and selection.
	if e.prefixKeymap == nil && e.ActiveBuffer().Mode() == "buffer-list" {
		if e.bufferListDispatch(ke) {
			return
		}
	}

	// When in a *VC Log* buffer, handle q and Enter.
	if e.prefixKeymap == nil && e.ActiveBuffer().Mode() == "vc-log" {
		if e.vcLogDispatch(ke) {
			return
		}
	}

	// When in a *VC Fixup Select* buffer, handle q / C-c C-c.
	if (e.prefixKeymap == nil || e.prefixKeymap == e.ctrlCKeymap) && e.ActiveBuffer().Mode() == "vc-fixup-select" {
		if e.vcFixupSelectDispatch(ke) {
			return
		}
	}

	// When in a diff-mode buffer (*vc diff*, *vc show*), handle q/n/p/Enter.
	if e.prefixKeymap == nil && (e.ActiveBuffer().Mode() == "diff" || e.ActiveBuffer().Mode() == "vc-show") {
		if e.vcDiffDispatch(ke) {
			return
		}
	}

	// When in a *vc status* buffer, handle q and Enter.
	if e.prefixKeymap == nil && e.ActiveBuffer().Mode() == "vc-status" {
		if e.vcStatusDispatch(ke) {
			return
		}
	}

	// When in a *compilation* buffer, handle q/g/n/p.
	if e.prefixKeymap == nil && e.ActiveBuffer().Mode() == "compilation" {
		if e.compilationDispatch(ke) {
			return
		}
	}

	// When in a *vc grep* buffer, handle q and Enter.
	if e.prefixKeymap == nil && e.ActiveBuffer().Mode() == "vc-grep" {
		if e.vcGrepDispatch(ke) {
			return
		}
	}

	// When in a *LSP References* buffer, handle q and Enter.
	if e.prefixKeymap == nil && e.ActiveBuffer().Mode() == "lsp-refs" {
		if e.lspRefsDispatch(ke) {
			return
		}
	}

	// When in a *vc-annotate* buffer, handle l/d/q.
	if e.prefixKeymap == nil && strings.HasPrefix(e.ActiveBuffer().Mode(), "vc-annotate") {
		if e.vcAnnotateDispatch(ke) {
			return
		}
	}

	// When in a *Help* buffer, q closes it.
	if e.prefixKeymap == nil && e.ActiveBuffer().Mode() == "help" {
		if e.helpDispatch(ke) {
			return
		}
	}

	// When in a *Man ...* buffer, q closes it.
	if e.prefixKeymap == nil && e.ActiveBuffer().Mode() == "man" {
		if e.manDispatch(ke) {
			return
		}
	}

	// When in a *shell* buffer, forward most keys to the PTY.
	if e.prefixKeymap == nil && e.ActiveBuffer().Mode() == "shell" {
		if e.shellDispatch(ke) {
			return
		}
	}

	// Debug: single-letter shortcuts in source buffers when session active.
	if e.prefixKeymap == nil && e.dap != nil {
		mode := e.ActiveBuffer().Mode()
		switch mode {
		case "debug-locals":
			if e.debugLocalsDispatch(ke) {
				return
			}
		case "debug-stack":
			if e.debugStackDispatch(ke) {
				return
			}
		case "debug-repl":
			if e.debugReplDispatch(ke) {
				return
			}
		default:
			if e.debugSourceDispatch(ke) {
				return
			}
		}
	}

	// When in a *VC Commit* buffer, C-c C-c submits and C-c C-k aborts.
	if e.ActiveBuffer().Mode() == "vc-commit" {
		if e.vcCommitDispatch(ke) {
			return
		}
	}

	binding, found := activeMap.Lookup(mk)
	if !found {
		// Unrecognised key in a prefix sequence: cancel prefix, beep.
		if e.prefixKeymap != nil {
			e.prefixKeymap = nil
			e.prefixKeySeq = ""
			e.Message("Key sequence incomplete")
			return
		}
		// Plain printable rune → self-insert.
		if ke.Key == tcell.KeyRune && ke.Mod&(tcell.ModCtrl|tcell.ModAlt) == 0 && unicode.IsPrint(ke.Rune) {
			e.selfInsert(ke.Rune)
		}
		return
	}

	// Clear prefix map now that we have a binding.
	e.prefixKeymap = nil

	if binding.Prefix != nil {
		// Begin or extend a prefix sequence (e.g. C-x, then C-x v).
		keyStr := keymap.FormatKey(mk)
		if e.prefixKeySeq == "" {
			// First chord (e.g. C-x): record it but don't show yet.
			e.prefixKeySeq = keyStr
		} else {
			// Second+ chord (e.g. C-x v): append and show in minibuffer.
			e.prefixKeySeq = e.prefixKeySeq + " " + keyStr
			e.Message("%s", e.prefixKeySeq)
		}
		e.prefixKeymap = binding.Prefix
		return
	}

	// Execute the bound command.
	// If a prefix-sequence hint was showing in the minibuffer, clear it now
	// so it doesn't linger after the command runs.
	if e.prefixKeySeq != "" {
		e.message = ""
	}
	e.prefixKeySeq = ""
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

// keysForCommand returns all key sequences (as human-readable strings) that
// are bound to cmdName in the global keymap tree.
func (e *Editor) keysForCommand(cmdName string) []string {
	var results []string
	var walk func(km *keymap.Keymap, prefix string)
	walk = func(km *keymap.Keymap, prefix string) {
		for key, binding := range km.Bindings() {
			keyStr := keymap.FormatKey(key)
			seq := prefix + keyStr
			switch {
			case binding.Command == cmdName:
				results = append(results, seq)
			case binding.Prefix != nil:
				walk(binding.Prefix, seq+" ")
			}
		}
	}
	walk(e.globalKeymap, "")
	sort.Strings(results)
	return results
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
		// Look up all key bindings for this command.
		if keys := e.keysForCommand(cmdName); len(keys) > 0 {
			sb.WriteString("Bound to: " + strings.Join(keys, ", ") + "\n\n")
		}
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
		helpBuf.SetReadOnly(false)
		helpBuf.Delete(0, helpBuf.Len())
		helpBuf.InsertString(0, sb.String())
	}
	helpBuf.SetMode("help")
	helpBuf.SetReadOnly(true)
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
		helpBuf.SetReadOnly(false)
		helpBuf.Delete(0, helpBuf.Len())
		helpBuf.InsertString(0, body)
	}
	helpBuf.SetMode("help")
	helpBuf.SetReadOnly(true)
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
	if ke.Key == tcell.KeyBackspace && ke.Mod == tcell.ModAlt {
		e.minibufBackwardKillWord()
		return
	}

	//nolint:exhaustive // external enum; default case handles unknowns
	switch ke.Key {
	case tcell.KeyEnter, tcell.KeyCtrlJ:
		e.finishMinibuffer()
	case tcell.KeyCtrlG, tcell.KeyEscape:
		e.cancelMinibuffer()
	case tcell.KeyBackspace:
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
		prev := buf.String()
		e.minibufComplete() // extends common prefix and shows hint; inserts only on unique match
		if buf.String() != prev {
			e.refreshMinibufCandidates() // update popup after prefix extension
		}
	case tcell.KeyDown:
		if len(e.minibufCandidates) > 0 {
			e.minibufSelectNext()
		} else {
			e.minibufHistoryNext()
		}
	case tcell.KeyUp:
		if len(e.minibufCandidates) > 0 {
			e.minibufSelectPrev()
		} else {
			e.minibufHistoryPrev()
		}
	case tcell.KeyCtrlN:
		e.minibufSelectNext()
	case tcell.KeyCtrlP:
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
		case ke.Mod&(tcell.ModCtrl|tcell.ModAlt) == 0 && unicode.IsPrint(ke.Rune):
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
	e.minibufCandidateChosen = false
}

// minibufSelectNext moves the popup selection one step down.
func (e *Editor) minibufSelectNext() {
	if len(e.minibufCandidates) == 0 {
		return
	}
	if e.minibufSelectedIdx < len(e.minibufCandidates)-1 {
		e.minibufSelectedIdx++
		if e.minibufSelectedIdx >= e.minibufCandidateOffset+minibufPopupMaxVisible {
			e.minibufCandidateOffset++
		}
	}
	e.minibufCandidateChosen = true
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
	e.minibufCandidateChosen = true
}

// minibufHistoryPrev navigates to the previous (older) history entry.
func (e *Editor) minibufHistoryPrev() {
	if e.minibufHistory == nil {
		return
	}
	hist := e.minibufHistory[e.minibufPrompt]
	if len(hist) == 0 {
		return
	}
	if e.minibufHistoryIdx == -1 {
		e.minibufHistorySaved = e.minibufBuf.String()
	}
	if e.minibufHistoryIdx < len(hist)-1 {
		e.minibufHistoryIdx++
		text := hist[e.minibufHistoryIdx]
		e.minibufBuf.Delete(0, e.minibufBuf.Len())
		e.minibufBuf.InsertString(0, text)
		e.minibufBuf.SetPoint(len([]rune(text)))
		e.refreshMinibufCandidates()
	}
}

// minibufHistoryNext navigates to the next (newer) history entry, restoring
// the saved text when the beginning of history is reached.
func (e *Editor) minibufHistoryNext() {
	if e.minibufHistoryIdx == -1 {
		return
	}
	e.minibufHistoryIdx--
	var text string
	if e.minibufHistoryIdx < 0 {
		text = e.minibufHistorySaved
		e.minibufHistoryIdx = -1
	} else {
		hist := e.minibufHistory[e.minibufPrompt]
		if e.minibufHistoryIdx < len(hist) {
			text = hist[e.minibufHistoryIdx]
		}
	}
	e.minibufBuf.Delete(0, e.minibufBuf.Len())
	e.minibufBuf.InsertString(0, text)
	e.minibufBuf.SetPoint(len([]rune(text)))
	e.refreshMinibufCandidates()
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

// fuzzyScore returns a rank for how well query matches candidate (lower = better).
// Priority: prefix match (0) > substring match (1) > subsequence match (2).
func fuzzyScore(candidate, query string) int {
	if query == "" {
		return 2
	}
	lower := strings.ToLower(candidate)
	q := strings.ToLower(query)
	if strings.HasPrefix(lower, q) {
		return 0
	}
	if strings.Contains(lower, q) {
		return 1
	}
	return 2
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
	// Auto-fill in text/markdown modes.
	e.maybeAutoFill()
	// Dismiss any stale completion popup, then maybe trigger a new one.
	e.lspCompDismiss()
	e.lspMaybeTriggerCompletion()
}

// maybeAutoFill breaks the current line at the last word boundary when it
// exceeds fill-column, but only in text and markdown modes.
func (e *Editor) maybeAutoFill() {
	buf := e.ActiveBuffer()
	mode := buf.Mode()
	if mode != "text" && mode != "markdown" {
		return
	}
	pt := buf.Point()
	bol := buf.BeginningOfLine(pt)
	// Column of point (0-based rune count from beginning of line).
	col := pt - bol
	if col <= e.fillColumn {
		return
	}
	// Find the last space at or before fill-column.
	breakPos := -1
	for pos := bol; pos < pt; pos++ {
		if buf.RuneAt(pos) == ' ' && (pos-bol) <= e.fillColumn {
			breakPos = pos
		}
	}
	if breakPos < 0 {
		return // no suitable break point
	}
	// Replace the space at breakPos with a newline.
	// After delete(breakPos,1) + insert(breakPos,'\n'), net shift is 0 so pt stays.
	buf.Delete(breakPos, 1)
	buf.Insert(breakPos, '\n')
	buf.SetPoint(pt)
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
const commandLRUMax = 50

func (e *Editor) execCommand(name string) {
	fn, ok := commands[name]
	if !ok {
		e.Message("Unknown command: %s", name)
		return
	}
	// fn reads e.lastCommand (e.g. for C-l cycling) before we overwrite it.
	fn(e)
	e.lastCommand = name
	// Don't record execute-extended-command itself in the LRU — the command
	// it dispatches to is already pushed by cmdExecuteExtendedCommand.
	if name != "execute-extended-command" {
		e.pushCommandLRU(name)
	}
}

// pushCommandLRU records name as the most recently used command.
// Duplicates are removed before prepending so each command appears once.
func (e *Editor) pushCommandLRU(name string) {
	// Remove existing occurrence (if any).
	filtered := e.commandLRU[:0]
	for _, n := range e.commandLRU {
		if n != name {
			filtered = append(filtered, n)
		}
	}
	e.commandLRU = append([]string{name}, filtered...)
	if len(e.commandLRU) > commandLRUMax {
		e.commandLRU = e.commandLRU[:commandLRUMax]
	}
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
		e.isearchClearCaches()
		e.Message("Quit")

	case tcell.KeyEnter:
		// Accept current match position.
		e.isearching = false
		e.isearchClearCaches()

	case tcell.KeyBackspace:
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
		if ke.Key == tcell.KeyRune && ke.Mod&(tcell.ModCtrl|tcell.ModAlt) == 0 && unicode.IsPrint(ke.Rune) {
			e.isearchStr += string(ke.Rune)
			e.isearchFind()
		} else {
			// Any other key (motion commands, etc.) exits isearch and executes the key.
			e.isearching = false
			e.isearchClearCaches()
			e.Message("")
			e.dispatchParsedKey(ke)
		}
	}
}

// isearchGetRunes returns the buffer content as []rune, reusing the cached
// slice when the buffer has not changed since the last call.
func (e *Editor) isearchGetRunes(buf *buffer.Buffer) []rune {
	gen := buf.ChangeGen()
	if e.isearchRunes != nil && e.isearchRunesGen == gen && e.isearchRunesBuf == buf {
		return e.isearchRunes
	}
	e.isearchRunes = []rune(buf.String())
	e.isearchRunesGen = gen
	e.isearchRunesBuf = buf
	return e.isearchRunes
}

// isearchGetNeedle returns the current search string as []rune, reusing the
// cached slice when isearchStr hasn't changed.
func (e *Editor) isearchGetNeedle() []rune {
	if e.isearchNeedleStr != e.isearchStr {
		e.isearchNeedle = []rune(e.isearchStr)
		e.isearchNeedleStr = e.isearchStr
	}
	return e.isearchNeedle
}

// isearchClearCaches releases the isearch rune caches.  Called when isearch exits.
func (e *Editor) isearchClearCaches() {
	e.isearchRunes = nil
	e.isearchRunesBuf = nil
	e.isearchNeedle = nil
	e.isearchNeedleStr = ""
}

// isearchFind searches for e.isearchStr from e.isearchStart.
func (e *Editor) isearchFind() {
	buf := e.ActiveBuffer()
	runes := e.isearchGetRunes(buf)
	needle := e.isearchGetNeedle()

	if len(needle) == 0 {
		buf.SetPoint(e.isearchStart)
		return
	}

	match := runesMatch
	if e.isSearchCaseFold {
		match = runesMatchFold
	}

	start := e.isearchStart
	if e.isearchFwd {
		for i := start; i <= len(runes)-len(needle); i++ {
			if match(runes[i:], needle) {
				buf.SetPoint(i + len(needle))
				return
			}
		}
	} else {
		for i := start; i >= 0; i-- {
			if i+len(needle) <= len(runes) && match(runes[i:], needle) {
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
	runes := e.isearchGetRunes(buf)
	needle := e.isearchGetNeedle()

	if len(needle) == 0 {
		return
	}

	match := runesMatch
	if e.isSearchCaseFold {
		match = runesMatchFold
	}

	cur := buf.Point()
	if e.isearchFwd {
		start := cur
		for i := start; i <= len(runes)-len(needle); i++ {
			if match(runes[i:], needle) {
				buf.SetPoint(i + len(needle))
				return
			}
		}
		e.Message("Failing isearch: %s", e.isearchStr)
	} else {
		// Search backward from one before current.
		start := max(cur-len(needle)-1, 0)
		for i := start; i >= 0; i-- {
			if i+len(needle) <= len(runes) && match(runes[i:], needle) {
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

// runesMatchFold is like runesMatch but case-insensitive.
func runesMatchFold(haystack, needle []rune) bool {
	if len(haystack) < len(needle) {
		return false
	}
	for i, r := range needle {
		if unicode.ToLower(haystack[i]) != unicode.ToLower(r) {
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
	// Sync visual-lines wrap column on all windows before rendering.
	e.applyVisualLines()
	// Notify LSP of any buffered changes before rendering.
	e.lspMaybeDidChange(e.ActiveBuffer())
	// Reflow window heights to reserve space for the minibuffer footer:
	// one row for the prompt plus one row per visible candidate, so the
	// popup sits between the modeline and the prompt (never above it).
	tw, th := e.term.Size()
	footer := 1
	if e.minibufActive {
		footer = 1 + min(len(e.minibufCandidates), minibufPopupMaxVisible)
	}
	if tw != e.lastLayoutW || th-footer != e.lastLayoutH || footer != e.lastLayoutFooter {
		e.relayoutWindows(tw, th-footer)
		e.lastLayoutW = tw
		e.lastLayoutH = th - footer
		e.lastLayoutFooter = footer
	}
	e.term.Clear()
	for _, w := range e.windows {
		if w.Buf().Mode() == "shell" {
			e.renderShellWindow(w)
		} else {
			e.renderWindow(w)
		}
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
	// Modelines are drawn after the minibuffer so they overwrite any candidate
	// popup rows that may have expanded upward into window text rows.
	for _, w := range e.windows {
		e.renderModeline(w)
	}
	// LSP completion popup (rendered over the text, below the cursor).
	e.renderLSPCompletionPopup()
	// LSP doc popup (lsp-show-doc), rendered over the text near the cursor.
	e.renderLSPDocPopup()
	// Window-jump overlays: drawn last so they appear on top of everything.
	if e.windowJumpActive {
		e.renderWindowJumpOverlays()
	}
	// Position the hardware cursor.
	e.placeCursor()
	e.term.Show()
}

// getSpanCache returns the cached syntax spans for buf, recomputing them if
// the buffer has changed since the last render.
func (e *Editor) getSpanCache(buf *buffer.Buffer) *spanCache {
	if e.spanCaches == nil {
		e.spanCaches = make(map[*buffer.Buffer]*spanCache)
	}
	c := e.spanCaches[buf]
	gen := buf.ChangeGen()
	mode := buf.Mode()
	if c != nil && c.gen == gen && c.mode == mode {
		return c
	}
	hl := e.customHighlighters[buf]
	if hl == nil {
		hl = highlighterFor(buf)
	}
	text := buf.String()
	runes := []rune(text)
	spans := hl.Highlight(text, 0, len(runes))
	c = &spanCache{gen: gen, mode: mode, text: text, runes: runes, spans: spans}
	e.spanCaches[buf] = c
	return c
}

// modeFromShebang inspects the first line of content for a shebang (#!) and
// returns the corresponding major mode name, or "" if none is recognised.
// It handles both "#!" and "#! " variants and both direct paths and env-style
// invocations (e.g. "#!/usr/bin/env python3.10").
func modeFromShebang(content string) string {
	if len(content) < 2 || content[0] != '#' || content[1] != '!' {
		return ""
	}
	// Extract the first line.
	line := content
	if nl := strings.IndexByte(line, '\n'); nl >= 0 {
		line = line[:nl]
	}
	// Grab the interpreter name: the last path component after optional "env".
	fields := strings.Fields(line[2:]) // skip "#!"
	if len(fields) == 0 {
		return ""
	}
	interp := fields[len(fields)-1]
	// Strip the directory component.
	if i := strings.LastIndexByte(interp, '/'); i >= 0 {
		interp = interp[i+1:]
	}
	switch {
	case interp == "bash":
		return "bash"
	case interp == "sh":
		return "bash"
	case interp == "perl":
		return "perl"
	case interp == "python" || interp == "python3" || interp == "python2":
		return "python"
	case strings.HasPrefix(interp, "python"):
		// e.g. python3.10, python3.12
		return "python"
	}
	return ""
}

// highlighterFor returns the appropriate syntax highlighter for buf.
func highlighterFor(buf *buffer.Buffer) syntax.Highlighter {
	mode := buf.Mode()
	switch {
	case mode == "go":
		return syntax.GoHighlighter{}
	case mode == "markdown":
		return syntax.MarkdownHighlighter{}
	case mode == modeElisp:
		return syntax.ElispHighlighter{}
	case mode == "python":
		return syntax.PythonHighlighter{}
	case mode == "java":
		return syntax.JavaHighlighter{}
	case mode == "bash":
		return syntax.BashHighlighter{}
	case mode == "json":
		return syntax.JSONHighlighter{}
	case mode == "yaml":
		return syntax.YAMLHighlighter{}
	case mode == "diff":
		return syntax.DiffHighlighter{}
	case mode == "vc-show":
		return syntax.VcShowHighlighter{}
	case mode == "vc-log" || mode == "vc-fixup-select":
		return syntax.VcLogHighlighter{}
	case mode == "vc-grep" || mode == "lsp-refs":
		return syntax.VcGrepHighlighter{}
	case mode == "vc-status":
		return syntax.VcStatusHighlighter{}
	case mode == "vc-annotate" || strings.HasPrefix(mode, "vc-annotate+"):
		hl := syntax.VcAnnotateHighlighter{}
		if _, lang, ok := strings.Cut(mode, "+"); ok {
			hl.Source = syntax.LangToHighlighter(lang)
		}
		return hl
	case mode == "vc-commit":
		return syntax.VcCommitHighlighter{}
	case mode == "conf":
		return syntax.ConfHighlighter{}
	case mode == "perl":
		return syntax.PerlHighlighter{}
	case mode == "gherkin":
		return syntax.GherkinHighlighter{}
	case mode == "makefile":
		return syntax.MakefileHighlighter{}
	case mode == "help":
		return syntax.HelpHighlighter{}
	case mode == "debug-locals":
		return syntax.DapLocalsHighlighter{}
	case mode == "debug-stack":
		return syntax.DapStackHighlighter{}
	case mode == "debug-repl":
		return syntax.GoHighlighter{}
	default:
		return syntax.NilHighlighter{}
	}
}

// helpDispatch handles key events in a *Help* buffer.
// q closes the buffer and switches to the most recently used non-help buffer.
func (e *Editor) helpDispatch(ke terminal.KeyEvent) bool {
	if ke.Key != tcell.KeyRune || ke.Rune != 'q' {
		return false
	}
	cur := e.ActiveBuffer()
	for _, b := range e.bufferMRU {
		if b != cur && b.Mode() != "help" {
			e.activeWin.SetBuf(b)
			return true
		}
	}
	for _, b := range e.buffers {
		if b != cur && b.Mode() != "help" {
			e.activeWin.SetBuf(b)
			return true
		}
	}
	e.SwitchToBuffer("*scratch*")
	return true
}

// manDispatch handles key events in a *Man ...* buffer.
// q closes the buffer and switches to the most recently used non-man buffer.
func (e *Editor) manDispatch(ke terminal.KeyEvent) bool {
	if ke.Key != tcell.KeyRune || ke.Rune != 'q' {
		return false
	}
	cur := e.ActiveBuffer()
	for _, b := range e.bufferMRU {
		if b != cur && b.Mode() != "man" {
			e.activeWin.SetBuf(b)
			return true
		}
	}
	for _, b := range e.buffers {
		if b != cur && b.Mode() != "man" {
			e.activeWin.SetBuf(b)
			return true
		}
	}
	e.SwitchToBuffer("*scratch*")
	return true
}

// slice of spans.  Falls back to FaceDefault.
func faceAtPos(spans []syntax.Span, pos int) syntax.Face {
	// Binary search: find the last span with Start <= pos.
	i := sort.Search(len(spans), func(j int) bool { return spans[j].Start > pos })
	if i > 0 {
		sp := spans[i-1]
		if pos < sp.End {
			return sp.Face
		}
	}
	return syntax.FaceDefault
}

// tabWidth is the number of spaces a tab character expands to when rendered.
const tabWidth = 2

// renderWindow draws the text content of w.
func (e *Editor) renderWindow(w *window.Window) {
	buf := w.Buf()
	cache := e.getSpanCache(buf)
	runes := cache.runes
	spans := cache.spans

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
		needle := e.isearchGetNeedle()
		cur := buf.Point()
		isMatch := runesMatch
		if e.isSearchCaseFold {
			isMatch = runesMatchFold
		}
		// The most recent match ends at point (forward) or starts at point (backward).
		if e.isearchFwd {
			mstart := cur - len(needle)
			if mstart >= 0 && mstart+len(needle) <= len(runes) &&
				isMatch(runes[mstart:], needle) {
				isearchMatchStart = mstart
				isearchMatchEnd = cur
			}
		} else if cur+len(needle) <= len(runes) && isMatch(runes[cur:], needle) {
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
	textH := max(winH-1, 1)

	// Gutter: columns reserved at the left for breakpoint/exec-pos indicators.
	// Always show a 2-column gutter when the file has any breakpoints set OR a
	// debug session is active (even before dapSetupLayout has run).
	gutterW := w.GutterWidth()
	if gutterW == 0 && buf.Filename() != "" {
		absName, _ := filepath.Abs(buf.Filename())
		if len(e.dapBreakpoints[absName]) > 0 || e.dap != nil {
			gutterW = 2
		}
	}
	textW := winW - gutterW // effective columns available for buffer text

	// Spell-check spans (nil when disabled or mode not applicable).
	spellSpans := e.getSpellSpans(buf)
	// Compute the word bounds around the cursor so we can suppress spell
	// highlighting on the word currently being typed.
	pt := buf.Point()
	curWordStart := pt
	for curWordStart > 0 && isSpellWordRune(buf.RuneAt(curWordStart-1)) {
		curWordStart--
	}
	curWordEnd := pt
	for curWordEnd < buf.Len() && isSpellWordRune(buf.RuneAt(curWordEnd)) {
		curWordEnd++
	}

	viewLines := w.ViewLines()
	// spanIdx is a monotonic cursor into spans used to look up the syntax face
	// for each character.  Since we iterate pos in strictly increasing order,
	// we advance spanIdx forward rather than binary-searching from the start.
	spanIdx := 0
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

		lineRunes := cache.runes[vl.StartPos:vl.EndPos]
		screenCol := 0

		// Gutter phase: draw breakpoint/exec-pos indicators in the reserved columns.
		if gutterW >= 2 {
			bpCh, bpFace := ' ', syntax.FaceDefault
			if e.dapHasBreakpoint(buf.Filename(), vl.Line) {
				bpCh, bpFace = '●', syntax.FaceBreakpoint
			}
			e.term.SetCell(w.Left(), screenRow, rune(bpCh), bpFace)

			epCh, epFace := ' ', syntax.FaceDefault
			if e.dap != nil && e.dap.stoppedFile == buf.Filename() && e.dap.stoppedLine == vl.Line {
				epCh, epFace = '→', syntax.FaceExecPos
			}
			e.term.SetCell(w.Left()+1, screenRow, rune(epCh), epFace)
		}

		// Phase 1: render actual line content, expanding tabs.
		for bufOffset, ch := range lineRunes {
			if screenCol >= textW {
				break
			}
			pos := vl.StartPos + bufOffset
			// Advance spanIdx forward past spans that start before pos.
			for spanIdx+1 < len(spans) && spans[spanIdx+1].Start <= pos {
				spanIdx++
			}
			var face syntax.Face
			if spanIdx < len(spans) {
				sp := spans[spanIdx]
				if pos >= sp.Start && pos < sp.End {
					face = sp.Face
				}
			}
			// Overlay spell-error underline (skip word currently being typed).
			if isSpellErrorAt(spellSpans, pos) && (pos < curWordStart || pos >= curWordEnd) {
				face.Underline = true
				face.UnderlineColor = "red"
			}
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
			if ch == '\t' {
				for j := 0; j < tabWidth && screenCol < textW; j++ {
					e.term.SetCell(w.Left()+gutterW+screenCol, screenRow, ' ', face)
					screenCol++
				}
			} else {
				e.term.SetCell(w.Left()+gutterW+screenCol, screenRow, ch, face)
				screenCol++
			}
		}

		// Phase 2: fill the rest of the row with spaces (possibly region-highlighted).
		eolPos := vl.StartPos + len(lineRunes)
		for screenCol < textW {
			face := syntax.FaceDefault
			if regionActive && eolPos >= regionStart && eolPos < regionEnd {
				face = syntax.FaceRegion
			}
			e.term.SetCell(w.Left()+gutterW+screenCol, screenRow, ' ', face)
			screenCol++
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
	// Strip any language suffix from vc-annotate mode (e.g. "vc-annotate+go" → "vc-annotate").
	if modeBase, _, ok := strings.Cut(mode, "+"); ok && strings.HasPrefix(mode, "vc-annotate") {
		mode = modeBase
	}
	narrow := ""
	if buf.Narrowed() {
		narrow = " Narrow"
	}
	macro := ""
	if e.kbdMacroRecording {
		macro = " Def"
	}
	diag := e.lspDiagSummary(buf)
	if diag != "" {
		diag = " " + diag
	}

	label := fmt.Sprintf(" %s  %-20s  (%s%s%s%s)  L%d C%d ", modifiedMark, name, mode, narrow, macro, diag, line, col)
	// Pad to window width.
	for len(label) < winW {
		label += " "
	}
	if len(label) > winW {
		label = label[:winW]
	}

	// For the *compilation* buffer, colour the buffer-name segment on the
	// modeline using the theme's compilation-ok / compilation-fail face.
	if buf.Name() == "*compilation*" && e.compilationExitOK != nil {
		nameFace := syntax.FaceCompilationFail
		if *e.compilationExitOK {
			nameFace = syntax.FaceCompilationOK
		}
		// Blend: keep the modeline background, override only Fg/Bold.
		nameFace.Bg = syntax.FaceModeline.Bg
		// Split label around the buffer name.
		prefix := fmt.Sprintf(" %s  ", modifiedMark)
		suffix := label[len(prefix)+len(fmt.Sprintf("%-20s", name)):]
		nameField := fmt.Sprintf("%-20s", name)
		col := w.Left()
		e.term.DrawString(col, modeRow, prefix, syntax.FaceModeline)
		col += len([]rune(prefix))
		e.term.DrawString(col, modeRow, nameField, nameFace)
		col += len([]rune(nameField))
		e.term.DrawString(col, modeRow, suffix, syntax.FaceModeline)
		return
	}

	e.term.DrawString(w.Left(), modeRow, label, syntax.FaceModeline)
}

// renderMinibuffer draws the minibuffer / message area.
// When a completion popup is active it is drawn above the prompt row,
// which stays at the last terminal row.
func (e *Editor) renderMinibuffer() {
	baseRow := e.minibufWin.Top() // always the last terminal row
	width := e.minibufWin.Width()

	var line string
	promptRow := baseRow

	if e.minibufActive {
		line = e.minibufPrompt + e.minibufBuf.String()
		// Refresh candidates now so nVisible is correct before we draw.
		if e.minibufCompletions != nil {
			if e.minibufBuf.String() != e.minibufLastQuery {
				e.refreshMinibufCandidates()
			}
		}
		nVisible := min(len(e.minibufCandidates), minibufPopupMaxVisible)
		if nVisible > 0 {
			e.renderCandidatePopup(baseRow-nVisible, width)
		}
	} else if e.message != "" {
		// Expire messages after 5 seconds.
		age := time.Now().UnixNano() - e.messageTime
		if age < 5*int64(time.Second) {
			line = e.message
		} else {
			e.message = ""
		}
	}

	// Draw prompt / message at promptRow.
	runes := []rune(line)

	// When showing a plain message (not in minibuf), try to syntax-highlight it
	// using the active buffer's mode — this makes LSP eldoc signatures readable.
	if !e.minibufActive && line != "" {
		hl := highlighterFor(e.ActiveBuffer())
		spans := hl.Highlight(line, 0, len(runes))
		for col := range width {
			ch := rune(' ')
			face := syntax.FaceMinibuffer
			if col < len(runes) {
				ch = runes[col]
				sf := faceAtPos(spans, col)
				if sf.Fg != "" && sf.Fg != "default" {
					face = syntax.Face{
						Fg:     sf.Fg,
						Bg:     syntax.FaceMinibuffer.Bg,
						Bold:   sf.Bold,
						Italic: sf.Italic,
					}
				}
			}
			e.term.SetCell(col, promptRow, ch, face)
		}
		return
	}

	for col := range width {
		ch := ' '
		if col < len(runes) {
			ch = runes[col]
		}
		e.term.SetCell(col, promptRow, ch, syntax.FaceMinibuffer)
	}
}

// minibufPopupMaxVisible is the maximum number of completion candidates shown
// at once in the minibuffer popup.
const minibufPopupMaxVisible = 5

// renderCandidatePopup draws the fuzzy-completion popup starting at startRow.
// Scroll indicators (▲/▼) appear in the first/last visible row when needed.
func (e *Editor) renderCandidatePopup(startRow, width int) {
	cands := e.minibufCandidates
	if len(cands) == 0 {
		return
	}

	offset := e.minibufCandidateOffset
	end := min(offset+minibufPopupMaxVisible, len(cands))
	visible := cands[offset:end]
	nVisible := len(visible)

	for i, cand := range visible {
		row := startRow + i
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

	// Shell buffers use the PTY's own cursor position.
	if buf.Mode() == "shell" {
		if col, row := e.shellCursorPos(buf); col >= 0 {
			e.term.ShowCursor(col, row)
			return
		}
	}

	pt := w.Point()
	// Use VisualRowForPoint so the cursor lands on the correct visual row
	// when visual line wrapping is active.
	visualRow := w.VisualRowForPoint()
	// Clamp to the last text row — never let the cursor land on the modeline.
	maxTextRow := w.Height() - 2
	if visualRow > maxTextRow {
		visualRow = maxTextRow
	}
	screenRow := w.Top() + visualRow
	// When wrapping, the screen column is within the current visual segment.
	screenCol := screenColForPoint(buf, pt)
	if w.WrapCol() > 0 {
		screenCol = screenCol % w.WrapCol()
	}
	e.term.ShowCursor(w.Left()+screenCol, screenRow)
}

// screenColForPoint returns the visual screen column for position pt within
// its line, expanding tab characters to tabWidth spaces.
func screenColForPoint(buf *buffer.Buffer, pt int) int {
	bol := buf.BeginningOfLine(pt)
	col := 0
	for i := bol; i < pt; i++ {
		if buf.RuneAt(i) == '\t' {
			col += tabWidth
		} else {
			col++
		}
	}
	return col
}

// ---------------------------------------------------------------------------
// Public helpers used by main
// ---------------------------------------------------------------------------

// applyVisualLines sets the wrapCol on all non-minibuffer windows according to
// the current visualLines setting (80 when enabled, 0 when disabled).
func (e *Editor) applyVisualLines() {
	if e.visualLinesSynced {
		return
	}
	col := 0
	if e.visualLines {
		col = 80
	}
	for _, w := range e.windows {
		mode := w.Buf().Mode()
		if mode == "vc-grep" || mode == "lsp-refs" || mode == "vc-status" ||
			mode == "vc-log" || mode == "vc-show" || mode == "diff" ||
			mode == "compilation" || mode == "vc-fixup-select" || mode == "shell" ||
			mode == "debug-locals" || mode == "debug-stack" || mode == "debug-repl" {
			w.SetWrapCol(0)
			continue
		}
		w.SetWrapCol(col)
	}
	e.visualLinesSynced = true
}

// markVisualLinesDirty forces the next applyVisualLines call to re-apply wrap
// columns to all windows.  Call this whenever visualLines or any window's
// buffer mode changes.
func (e *Editor) markVisualLinesDirty() {
	e.visualLinesSynced = false
}

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
	e.lspClose()
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
