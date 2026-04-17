package editor

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/syntax"
	"github.com/skybert/gomacs/internal/window"
)

// ---------------------------------------------------------------------------
// Command registry
// ---------------------------------------------------------------------------

// CommandFn is the signature of every editor command.
type CommandFn func(e *Editor)

// commands is the global command registry populated by init().
var commands = map[string]CommandFn{}

// commandDocs holds the documentation string for each registered command.
// These mirror the Go doc comments on the cmd* functions and are shown by
// describe-function (C-h f) and describe-key (C-h k).
var commandDocs = map[string]string{}

// registerCommand adds fn to the command registry under name with doc.
func registerCommand(name string, fn CommandFn, doc string) {
	commands[name] = fn
	if doc != "" {
		commandDocs[name] = doc
	}
}

func init() {
	// ---- movement ----------------------------------------------------------
	registerCommand("forward-char", (*Editor).cmdForwardChar,
		"Move point right N characters (left if N is negative).")
	registerCommand("backward-char", (*Editor).cmdBackwardChar,
		"Move point left N characters (right if N is negative).")
	registerCommand("next-line", (*Editor).cmdNextLine,
		"Move point to the next line, keeping the goal column.")
	registerCommand("previous-line", (*Editor).cmdPreviousLine,
		"Move point to the previous line, keeping the goal column.")
	registerCommand("beginning-of-line", (*Editor).cmdBeginningOfLine,
		"Move point to the beginning of the current line.")
	registerCommand("end-of-line", (*Editor).cmdEndOfLine,
		"Move point to the end of the current line.")
	registerCommand("forward-word", (*Editor).cmdForwardWord,
		"Move point forward until past the end of a word.")
	registerCommand("backward-word", (*Editor).cmdBackwardWord,
		"Move point backward until the start of a word.")
	registerCommand("beginning-of-buffer", (*Editor).cmdBeginningOfBuffer,
		"Move point to the beginning of the buffer. Sets the mark at the old position.")
	registerCommand("end-of-buffer", (*Editor).cmdEndOfBuffer,
		"Move point to the end of the buffer. Sets the mark at the old position.")
	registerCommand("scroll-up", (*Editor).cmdScrollUp,
		"Scroll the selected window upward by N screenfuls.")
	registerCommand("scroll-down", (*Editor).cmdScrollDown,
		"Scroll the selected window downward by N screenfuls.")
	registerCommand("recenter", (*Editor).cmdRecenter,
		"Center point in the window. Successive calls cycle: center, top, bottom.")
	registerCommand("beginning-of-sentence", (*Editor).cmdBeginningOfSentence,
		"Move point backward to the beginning of the current sentence.")
	registerCommand("end-of-sentence", (*Editor).cmdEndOfSentence,
		"Move point forward to the end of the current sentence.")
	registerCommand("upcase-word", (*Editor).cmdUpcaseWord,
		"Convert the following word to upper case.")
	registerCommand("downcase-word", (*Editor).cmdDowncaseWord,
		"Convert the following word to lower case.")
	registerCommand("capitalize-word", (*Editor).cmdCapitalizeWord,
		"Capitalize the following word.")

	// ---- editing -----------------------------------------------------------
	registerCommand("newline", (*Editor).cmdNewline,
		"Insert a newline and move point past it.")
	registerCommand("indent-or-complete", (*Editor).cmdIndentOrComplete,
		"Insert a tab character at point.")
	registerCommand("self-insert-command", (*Editor).cmdSelfInsert,
		"Insert the character you type.")
	registerCommand("delete-char", (*Editor).cmdDeleteChar,
		"Delete N characters forward. If the mark is active, kill the region instead.")
	registerCommand("backward-delete-char", (*Editor).cmdBackwardDeleteChar,
		"Delete N characters backward.")
	registerCommand("kill-line", (*Editor).cmdKillLine,
		"Kill the rest of the current line; before eol, kill the newline.")
	registerCommand("kill-region", (*Editor).cmdKillRegion,
		"Kill between point and mark. The text is saved in the kill ring.")
	registerCommand("copy-region-as-kill", (*Editor).cmdCopyRegionAsKill,
		"Save the region as the last killed text without actually killing it.")
	registerCommand("yank", (*Editor).cmdYank,
		"Reinsert (yank) the last stretch of killed text.")
	registerCommand("yank-pop", (*Editor).cmdYankPop,
		"Replace the just-yanked text with an earlier batch of killed text.")
	registerCommand("kill-word", (*Editor).cmdKillWord,
		"Kill characters forward until encountering the end of a word.")
	registerCommand("backward-kill-word", (*Editor).cmdBackwardKillWord,
		"Kill characters backward until encountering the beginning of a word.")
	registerCommand("kill-sentence", (*Editor).cmdKillSentence,
		"Kill from point to end of sentence.")
	registerCommand("transpose-chars", (*Editor).cmdTransposeChars,
		"Interchange characters around point, moving forward one character.")
	registerCommand("open-line", (*Editor).cmdOpenLine,
		"Insert a newline and leave point before it.")
	registerCommand("undo", (*Editor).cmdUndo,
		"Undo some previous changes. Repeat to undo more.")
	registerCommand("redo", (*Editor).cmdRedo,
		"Redo the most recently undone change.")

	// ---- marks / search / misc ---------------------------------------------
	registerCommand("set-mark-command", (*Editor).cmdSetMarkCommand,
		"Set the mark where point is.")
	registerCommand("keyboard-quit", (*Editor).cmdKeyboardQuit,
		"Cancel the current command or operation.")
	registerCommand("universal-argument", (*Editor).cmdUniversalArgument,
		"Begin a numeric argument for the following command.")
	registerCommand("execute-extended-command", (*Editor).cmdExecuteExtendedCommand,
		"Read a command name from the minibuffer and execute it.")
	registerCommand("comment-dwim", (*Editor).cmdCommentDwim,
		"Call the comment command you want (Do What I Mean).")
	registerCommand("isearch-forward", (*Editor).cmdIsearchForward,
		"Do incremental search forward.")
	registerCommand("isearch-backward", (*Editor).cmdIsearchBackward,
		"Do incremental search backward.")

	// ---- files / buffers ---------------------------------------------------
	registerCommand("find-file", (*Editor).cmdFindFile,
		"Edit file FILENAME. Switch to a buffer visiting file FILENAME, creating one if none already exists.")
	registerCommand("save-buffer", (*Editor).cmdSaveBuffer,
		"Save current buffer to its file.")
	registerCommand("toggle-read-only", (*Editor).cmdToggleReadOnly,
		"Change whether this buffer is read-only.")
	registerCommand("save-buffers-kill-terminal", (*Editor).cmdSaveBuffersKillTerminal,
		"Offer to save each buffer, then quit gomacs.")
	registerCommand("save-some-buffers", (*Editor).cmdSaveSomeBuffers,
		"Save all modified file-visiting buffers without asking.")
	registerCommand("switch-to-buffer", (*Editor).cmdSwitchToBuffer,
		"Display buffer in the selected window.")
	registerCommand("kill-buffer", (*Editor).cmdKillBuffer,
		"Kill the buffer specified by BUFFER-OR-NAME.")
	registerCommand("list-buffers", (*Editor).cmdListBuffers,
		"Display a list of existing buffers in the *Buffer List* buffer.")
	registerCommand("exchange-point-and-mark", (*Editor).cmdExchangePointAndMark,
		"Put the mark where point is now, and point where the mark is now.")
	registerCommand("delete-other-windows", (*Editor).cmdDeleteOtherWindows,
		"Make the selected window fill the screen.")
	registerCommand("split-window-below", (*Editor).cmdSplitWindowBelow,
		"Split the selected window into two windows, one above the other.")
	registerCommand("split-window-right", (*Editor).cmdSplitWindowRight,
		"Split the selected window into two windows side by side.")
	registerCommand("other-window", (*Editor).cmdOtherWindow,
		"Select another window in cyclic ordering of windows.")
	registerCommand("eval-last-sexp", (*Editor).cmdEvalLastSexp,
		"Evaluate the sexp before point and show the result in the echo area.")

	// ---- help --------------------------------------------------------------
	registerCommand("describe-key", (*Editor).cmdDescribeKey,
		"Display documentation of the function invoked by KEY.")
	registerCommand("describe-function", (*Editor).cmdDescribeFunction,
		"Display the documentation of FUNCTION (a command name).")
	registerCommand("describe-variable", (*Editor).cmdDescribeVariable,
		"Display the full documentation of VARIABLE (a global Elisp variable).")

	// ---- themes ------------------------------------------------------------
	registerCommand("load-theme", (*Editor).cmdLoadTheme,
		"Load a named colour theme (e.g. sweet, default).")

	// ---- navigation extras -------------------------------------------------
	registerCommand("goto-line", (*Editor).cmdGotoLine,
		"Go to line N (type number in minibuffer).")
	registerCommand("what-cursor-position", (*Editor).cmdWhatCursorPosition,
		"Print info on cursor position in buffer.")
	registerCommand("what-line", (*Editor).cmdWhatLine,
		"Print the current buffer line number and narrowed line number of point.")
	registerCommand("count-words", (*Editor).cmdCountWords,
		"Count words in the buffer (or region if active).")
	registerCommand("next-error", (*Editor).cmdNextError,
		"Visit next compilation error message and corresponding source code.")
	registerCommand("previous-error", (*Editor).cmdPreviousError,
		"Visit previous compilation error message and corresponding source code.")
	registerCommand("count-buffer-lines", (*Editor).cmdCountBufferLines,
		"Display number of lines in buffer and how many are before and after point.")
	registerCommand("help", (*Editor).cmdHelp,
		"Show a help buffer listing all commands and configuration variables.")
	registerCommand("project-build", (*Editor).cmdBuild,
		"Run make in the project root and show output in *compilation* buffer.")
	registerCommand("project-find-file", (*Editor).cmdProjectFindFile,
		"Fuzzy-search all files in the current project (VC root) and open the selected one.")
	registerCommand("project-grep", (*Editor).cmdProjectGrep,
		"Grep for a pattern across the project using VC backend or grep -R -i -n (C-x p g).")

	// ---- mark extras -------------------------------------------------------
	registerCommand("mark-whole-buffer", (*Editor).cmdMarkWholeBuffer,
		"Put point at beginning and mark at end of buffer.")
	registerCommand("mark-word", (*Editor).cmdMarkWord,
		"Set mark at the end of the next word.")

	// ---- editing extras ----------------------------------------------------
	registerCommand("transpose-words", (*Editor).cmdTransposeWords,
		"Interchange words around point, moving forward.")
	registerCommand("delete-blank-lines", (*Editor).cmdDeleteBlankLines,
		"Delete blank lines around point.")
	registerCommand("delete-trailing-whitespace", (*Editor).cmdDeleteTrailingWhitespace,
		"Delete trailing whitespace in the buffer or the active region.")
	registerCommand("join-line", (*Editor).cmdJoinLine,
		"Join this line to the previous one.")
	registerCommand("back-to-indentation", (*Editor).cmdBackToIndentation,
		"Move point to the first non-whitespace character on this line.")
	registerCommand("upcase-region", (*Editor).cmdUpcaseRegion,
		"Convert the region to upper case.")
	registerCommand("downcase-region", (*Editor).cmdDowncaseRegion,
		"Convert the region to lower case.")
	registerCommand("fill-paragraph", (*Editor).cmdFillParagraph,
		"Fill paragraph at or after point.")
	registerCommand("set-fill-column", (*Editor).cmdSetFillColumn,
		"Set fill-column to the current column or a numeric argument.")
	registerCommand("indent-region", (*Editor).cmdIndentRegion,
		"Indent each nonblank line in the region to the current column.")
	registerCommand("indent-rigidly", (*Editor).cmdIndentRigidly,
		"Indent all lines in region sideways by ARG columns.")

	// ---- search / replace --------------------------------------------------
	registerCommand("replace-string", (*Editor).cmdReplaceString,
		"Replace occurrences of FROM-STRING with TO-STRING.")
	registerCommand("query-replace", (*Editor).cmdQueryReplace,
		"Replace some occurrences of STRING with NEWSTRING.")

	// ---- shell -------------------------------------------------------------
	registerCommand("shell-command", (*Editor).cmdShellCommand,
		"Execute string COMMAND in inferior shell; display output.")
	registerCommand("shell-command-on-region", (*Editor).cmdShellCommandOnRegion,
		"Execute string COMMAND in inferior shell with region as input.")
	registerCommand("shell", (*Editor).cmdShell,
		"Create or switch to a *shell* buffer backed by a PTY terminal.")
	registerCommand("man", (*Editor).cmdMan,
		"Display the manual page for TOPIC in a read-only *Man TOPIC* buffer.")
	registerCommand("vc-print-log", (*Editor).cmdVcPrintLog,
		"Show git log for the current repository (C-x v l).")
	registerCommand("vc-diff", (*Editor).cmdVcDiff,
		"Show uncommitted changes via git diff in diff-mode (C-x v =).")
	registerCommand("vc-status", (*Editor).cmdVcStatus,
		"Show VCS status for the current repository (C-x v s).")
	registerCommand("vc-grep", (*Editor).cmdVcGrep,
		"Run VCS grep and navigate results (C-x v G).")
	registerCommand("vc-next-action", (*Editor).cmdVcNextAction,
		"Stage the file or open a commit buffer if changes are already staged (C-x v v).")
	registerCommand("vc-revert", (*Editor).cmdVcRevert,
		"Revert the current file to its last committed version (C-x v u).")
	registerCommand("vc-annotate", (*Editor).cmdVcAnnotate,
		"Show git blame annotation for the current file (C-x v g).")
	registerCommand("messages", (*Editor).cmdMessages,
		"Switch to the *messages* buffer showing recent editor messages.")
	registerCommand("gomacs-version", (*Editor).cmdGomacsVersion,
		"Display gomacs version, Go runtime version, and uptime.")
	registerCommand("what-key", (*Editor).cmdWhatKey,
		"Intercept the next key press and display its raw key code, rune, and modifier in the message line.")

	// ---- narrowing ---------------------------------------------------------
	registerCommand("narrow-to-region", (*Editor).cmdNarrowToRegion,
		"Restrict editing in this buffer to the current region.")
	registerCommand("widen", (*Editor).cmdWiden,
		"Remove restrictions (narrowing) from current buffer.")

	// ---- registers ---------------------------------------------------------
	registerCommand("point-to-register", (*Editor).cmdPointToRegister,
		"Store current location of point in register R.")
	registerCommand("jump-to-register", (*Editor).cmdJumpToRegister,
		"Move point to location stored in register R.")
	registerCommand("copy-to-register", (*Editor).cmdCopyToRegister,
		"Copy region into register R.")
	registerCommand("insert-register", (*Editor).cmdInsertRegister,
		"Insert text contents of register R.")
	registerCommand("copy-rectangle-to-register", (*Editor).cmdCopyRectangleToRegister,
		"Copy the rectangle delimited by point and mark into register R.")

	// ---- keyboard macros ---------------------------------------------------
	registerCommand("start-kbd-macro", (*Editor).cmdStartKbdMacro,
		"Record subsequent keyboard input, defining a keyboard macro.")
	registerCommand("end-kbd-macro", (*Editor).cmdEndKbdMacro,
		"Finish defining a keyboard macro.")
	registerCommand("call-last-kbd-macro", (*Editor).cmdCallLastKbdMacro,
		"Call the last keyboard macro that you defined with start-kbd-macro.")

	// ---- dired -------------------------------------------------------------
	registerCommand("dired", (*Editor).cmdDired,
		"Edit a directory. You can visit files, mark for deletion, etc.")

	// ---- language modes ----------------------------------------------------
	registerCommand("go-mode", (*Editor).cmdGoMode,
		"Select Go mode for the current buffer.")
	registerCommand("python-mode", (*Editor).cmdPythonMode,
		"Select Python mode for the current buffer.")
	registerCommand("java-mode", (*Editor).cmdJavaMode,
		"Select Java mode for the current buffer.")
	registerCommand("bash-mode", (*Editor).cmdBashMode,
		"Select Bash mode for the current buffer.")
	registerCommand("markdown-mode", (*Editor).cmdMarkdownMode,
		"Select Markdown mode for the current buffer.")
	registerCommand("elisp-mode", (*Editor).cmdElispMode,
		"Select Emacs Lisp mode for the current buffer.")
	registerCommand("fundamental-mode", (*Editor).cmdFundamentalMode,
		"Select Fundamental mode (no syntax highlighting or indentation).")
	registerCommand("text-mode", (*Editor).cmdTextMode,
		"Select Text mode (plain text with spell checking).")
	registerCommand("makefile-mode", (*Editor).cmdMakefileMode,
		"Select Makefile mode for the current buffer.")

	// ---- LSP ---------------------------------------------------------------
	registerCommand("lsp-find-definition", (*Editor).cmdLSPFindDefinition,
		"Find the definition of the symbol at point using the LSP server.")
	registerCommand("lsp-pop-definition", (*Editor).cmdLSPPopDefinition,
		"Pop back to the position before the last lsp-find-definition jump.")
	registerCommand("lsp-find-references", (*Editor).cmdLSPFindReferences,
		"Find all references to the symbol at point and show them in *LSP References*.")
	registerCommand("lsp-show-doc", (*Editor).cmdLSPShowDoc,
		"Show documentation for the symbol at point in a floating popup (press any key to dismiss).")

	registerCommand("imenu", (*Editor).cmdImenu,
		"Navigate to a definition in the current buffer by name.")

	registerCommand("sort-lines", (*Editor).cmdSortLines,
		"Sort lines in the active region (or whole buffer) lexicographically.")
	registerCommand("delete-duplicate-lines", (*Editor).cmdDeleteDuplicateLines,
		"Remove duplicate lines from the active region (or whole buffer).")

	registerCommand("json-mode", (*Editor).cmdJsonMode,
		"Activate JSON mode on the current buffer.")
	registerCommand("yaml-mode", (*Editor).cmdYamlMode,
		"Activate YAML mode on the current buffer.")
	registerCommand("conf-mode", (*Editor).cmdConfMode,
		"Activate Conf mode on the current buffer (conf/ini/toml files).")
	registerCommand("perl-mode", (*Editor).cmdPerlMode,
		"Activate Perl mode on the current buffer.")
	registerCommand("gherkin-mode", (*Editor).cmdGherkinMode,
		"Activate Gherkin mode on the current buffer (.feature files).")

	// ---- spell checking ----------------------------------------------------
	registerCommand("ispell-word", (*Editor).cmdIspellWord,
		"Check spelling of word at point (uses spell-command, default aspell).")
	registerCommand("ispell-buffer", (*Editor).cmdSpell,
		"Interactively spell-check the current buffer (uses spell-command, default aspell).")
	registerCommand("spell", (*Editor).cmdSpell,
		"Interactively spell-check the current buffer using the configured spell checker.")
	registerCommand("dabbrev-expand", (*Editor).cmdDabbrevExpand,
		"Expand word before point to the nearest matching word in any open buffer (M-/).")

	// ---- list navigation ---------------------------------------------------
	registerCommand("forward-list", (*Editor).cmdForwardList,
		"Move forward past the next balanced list delimiter or closing bracket.")
	registerCommand("backward-list", (*Editor).cmdBackwardList,
		"Move backward past the previous balanced list delimiter or opening bracket.")
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const (
	cmdNameRecenter = "recenter"
	modeElisp       = "elisp"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// arg returns the universal argument count (always at least 1).
func (e *Editor) arg() int {
	if e.universalArgSet && e.universalArg > 0 {
		return e.universalArg
	}
	return 1
}

// clearArg resets the universal argument after a command consumes it.
func (e *Editor) clearArg() {
	e.universalArg = 1
	e.universalArgSet = false
	e.universalArgDigits = ""
	e.universalArgTyping = false
}

// isWordRune reports whether r is a word-constituent character.
func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// subwordForwardOne advances past one subword from pt and returns the new
// position.  Subword boundaries are:
//   - non-word → word transitions (standard word start)
//   - lowercase/digit → uppercase transitions (camelCase: fooBar, FooBar)
//   - end of an all-caps run before a TitleCase word (FOOBar → stops at B)
func subwordForwardOne(buf *buffer.Buffer, pt int) int {
	length := buf.Len()
	// Skip non-word chars.
	for pt < length && !isWordRune(buf.RuneAt(pt)) {
		pt++
	}
	if pt >= length {
		return pt
	}
	ch := buf.RuneAt(pt)
	if !unicode.IsUpper(ch) {
		// Lowercase / digit / underscore run: skip until uppercase or end.
		for pt < length && isWordRune(buf.RuneAt(pt)) && !unicode.IsUpper(buf.RuneAt(pt)) {
			pt++
		}
		return pt
	}
	// Uppercase start.  Peek at next char.
	if pt+1 < length && isWordRune(buf.RuneAt(pt+1)) && !unicode.IsUpper(buf.RuneAt(pt+1)) {
		// TitleCase (e.g. Foo): skip this uppercase then trailing lowercase run.
		pt++
		for pt < length && isWordRune(buf.RuneAt(pt)) && !unicode.IsUpper(buf.RuneAt(pt)) {
			pt++
		}
		return pt
	}
	// All-caps run (e.g. FOO, FOOBar).  Advance while uppercase; stop just
	// before the uppercase that starts the next TitleCase word.
	for pt < length && unicode.IsUpper(buf.RuneAt(pt)) {
		pt++
		// After advancing: if we're now at an uppercase whose next char is a
		// lowercase word-rune, the next subword starts here → stop.
		if pt < length && unicode.IsUpper(buf.RuneAt(pt)) {
			if pt+1 < length && isWordRune(buf.RuneAt(pt+1)) && !unicode.IsUpper(buf.RuneAt(pt+1)) {
				break
			}
		}
	}
	return pt
}

// subwordBackwardOne moves backward past one subword from pt and returns the
// new position.
func subwordBackwardOne(buf *buffer.Buffer, pt int) int {
	// Skip non-word chars backward.
	for pt > 0 && !isWordRune(buf.RuneAt(pt-1)) {
		pt--
	}
	if pt <= 0 {
		return 0
	}
	// Skip lowercase/digit/underscore run backward.
	for pt > 0 && isWordRune(buf.RuneAt(pt-1)) && !unicode.IsUpper(buf.RuneAt(pt-1)) {
		pt--
	}
	// Skip uppercase run backward.
	// For TitleCase (single upper before lowercase, e.g. B in Bar): skip only
	// that one uppercase.  For AllCaps (e.g. FOO): skip all uppercase.
	for pt > 0 && unicode.IsUpper(buf.RuneAt(pt-1)) {
		pt--
		// After backing up: if the char at pt (that we just passed over) is
		// uppercase and pt+1 is a lowercase word-rune, this is a TitleCase
		// start → don't back up further.
		if unicode.IsUpper(buf.RuneAt(pt)) {
			next := pt + 1
			if next < buf.Len() && isWordRune(buf.RuneAt(next)) && !unicode.IsUpper(buf.RuneAt(next)) {
				break
			}
		}
	}
	return pt
}

// ---------------------------------------------------------------------------
// Movement commands
// ---------------------------------------------------------------------------

func (e *Editor) cmdForwardChar() {
	n := e.arg()
	e.clearArg()
	buf := e.ActiveBuffer()
	buf.SetPoint(buf.Point() + n)
	buf.SetMarkActive(false)
}

func (e *Editor) cmdBackwardChar() {
	n := e.arg()
	e.clearArg()
	buf := e.ActiveBuffer()
	buf.SetPoint(buf.Point() - n)
	buf.SetMarkActive(false)
}

func (e *Editor) cmdNextLine() {
	n := e.arg()
	e.clearArg()
	buf := e.ActiveBuffer()
	w := e.activeWin

	// Establish goal column as character offset from line start.
	if w.GoalCol() < 0 {
		w.SetGoalCol(buf.Point() - buf.BeginningOfLine(buf.Point()))
	}
	goal := w.GoalCol()

	for range n {
		eol := buf.EndOfLine(buf.Point())
		if eol >= buf.Len() {
			break // already on last line
		}
		nextStart := eol + 1 // first char of next line
		nextEol := buf.EndOfLine(nextStart)
		pos := nextStart + goal
		if pos > nextEol {
			pos = nextEol
		}
		buf.SetPoint(pos)
	}
}

func (e *Editor) cmdPreviousLine() {
	n := e.arg()
	e.clearArg()
	buf := e.ActiveBuffer()
	w := e.activeWin

	if w.GoalCol() < 0 {
		w.SetGoalCol(buf.Point() - buf.BeginningOfLine(buf.Point()))
	}
	goal := w.GoalCol()

	for range n {
		start := buf.BeginningOfLine(buf.Point())
		if start == 0 {
			break // already on first line
		}
		prevEol := start - 1 // the '\n' ending the previous line
		prevStart := buf.BeginningOfLine(prevEol)
		pos := prevStart + goal
		if pos > prevEol {
			pos = prevEol
		}
		buf.SetPoint(pos)
	}
}

func (e *Editor) cmdBeginningOfLine() {
	e.clearArg()
	buf := e.ActiveBuffer()
	buf.SetPoint(buf.BeginningOfLine(buf.Point()))
	e.activeWin.ClearGoalCol()
}

func (e *Editor) cmdEndOfLine() {
	e.clearArg()
	buf := e.ActiveBuffer()
	buf.SetPoint(buf.EndOfLine(buf.Point()))
	e.activeWin.ClearGoalCol()
}

func (e *Editor) cmdForwardWord() {
	n := e.arg()
	e.clearArg()
	buf := e.ActiveBuffer()
	pt := buf.Point()
	length := buf.Len()
	for range n {
		if e.subwordMode {
			pt = subwordForwardOne(buf, pt)
		} else {
			for pt < length && !isWordRune(buf.RuneAt(pt)) {
				pt++
			}
			for pt < length && isWordRune(buf.RuneAt(pt)) {
				pt++
			}
		}
	}
	buf.SetPoint(pt)
}

func (e *Editor) cmdBackwardWord() {
	n := e.arg()
	e.clearArg()
	buf := e.ActiveBuffer()
	pt := buf.Point()
	for range n {
		if e.subwordMode {
			pt = subwordBackwardOne(buf, pt)
		} else {
			for pt > 0 && !isWordRune(buf.RuneAt(pt-1)) {
				pt--
			}
			for pt > 0 && isWordRune(buf.RuneAt(pt-1)) {
				pt--
			}
		}
	}
	buf.SetPoint(pt)
}

func (e *Editor) cmdBeginningOfBuffer() {
	e.clearArg()
	buf := e.ActiveBuffer()
	buf.SetMark(buf.Point())
	buf.SetMarkActive(false)
	buf.SetPoint(0)
}

func (e *Editor) cmdEndOfBuffer() {
	e.clearArg()
	buf := e.ActiveBuffer()
	buf.SetMark(buf.Point())
	buf.SetMarkActive(false)
	buf.SetPoint(buf.Len())
}

func (e *Editor) cmdScrollUp() {
	n := e.arg()
	e.clearArg()
	w := e.activeWin
	lines := max(w.Height()-2, 1)
	w.ScrollUp(n * lines)
	// Clamp cursor using the actual view lines so wrap mode is handled
	// correctly (prevents EnsurePointVisible from undoing the scroll).
	buf := e.ActiveBuffer()
	pt := buf.Point()
	viewLines := w.ViewLines()
	if len(viewLines) == 0 {
		return
	}
	first := viewLines[0].StartPos
	last := viewLines[0].StartPos
	for _, vl := range viewLines {
		if vl.Line > 0 {
			last = vl.StartPos
		}
	}
	if pt < first {
		buf.SetPoint(first)
	} else if pt > last {
		buf.SetPoint(last)
	}
}

func (e *Editor) cmdScrollDown() {
	n := e.arg()
	e.clearArg()
	w := e.activeWin
	lines := max(w.Height()-2, 1)
	w.ScrollDown(n * lines)
	buf := e.ActiveBuffer()
	pt := buf.Point()
	viewLines := w.ViewLines()
	if len(viewLines) == 0 {
		return
	}
	first := viewLines[0].StartPos
	last := viewLines[0].StartPos
	for _, vl := range viewLines {
		if vl.Line > 0 {
			last = vl.StartPos
		}
	}
	if pt < first {
		buf.SetPoint(first)
	} else if pt > last {
		buf.SetPoint(last)
	}
}

func (e *Editor) cmdRecenter() {
	if e.lastCommand != cmdNameRecenter {
		e.recenterCycle = 0
	}
	e.clearArg()
	e.syncWindowPoint(e.activeWin)
	switch e.recenterCycle % 3 {
	case 0:
		e.activeWin.Recenter()
	case 1:
		e.activeWin.RecenterTop()
	case 2:
		e.activeWin.RecenterBottom()
	}
	e.recenterCycle++
}

// ---------------------------------------------------------------------------
// Editing commands
// ---------------------------------------------------------------------------

func (e *Editor) cmdNewline() {
	if e.bufReadOnly() {
		e.clearArg()
		return
	}
	n := e.arg()
	e.clearArg()
	buf := e.ActiveBuffer()
	pt := buf.Point()
	for i := range n {
		buf.Insert(pt+i, '\n')
	}
	buf.SetPoint(pt + n)
	e.activeWin.ClearGoalCol()
	// Auto-indent for Emacs Lisp buffers.
	if buf.Mode() == modeElisp {
		indentElispLine(buf)
		return
	}
	// Auto-indent for programming modes that have an indent engine.
	switch buf.Mode() {
	case "go", "java", "python", "bash", "perl", "json", "yaml":
		indentCurrentLine(buf, e.modeIndentStr(buf.Mode()))
	}
}

func (e *Editor) cmdIndentOrComplete() {
	e.clearArg()
	buf := e.ActiveBuffer()
	if buf.Mode() == modeElisp {
		indentElispLine(buf)
		return
	}
	indentCurrentLine(buf, e.modeIndentStr(buf.Mode()))
}
func (e *Editor) cmdSelfInsert() {
	// This command is not normally called via the registry (self-insert happens
	// in dispatchKey for unbound printable runes), but it is registered for
	// completeness / M-x usage.
	e.clearArg()
}

func (e *Editor) cmdDeleteChar() {
	if e.bufReadOnly() {
		e.clearArg()
		return
	}
	n := e.arg()
	e.clearArg()
	buf := e.ActiveBuffer()

	// If mark is active, kill the region instead.
	if buf.MarkActive() {
		e.killRegionFrom(buf)
		return
	}

	pt := buf.Point()
	deleted := buf.Delete(pt, n)
	if deleted != "" {
		e.addToKillRing(deleted)
	}
}

func (e *Editor) cmdBackwardDeleteChar() {
	if e.bufReadOnly() {
		e.clearArg()
		return
	}
	n := e.arg()
	e.clearArg()
	buf := e.ActiveBuffer()
	pt := buf.Point()
	if n > pt {
		n = pt
	}
	if n > 0 {
		buf.Delete(pt-n, n)
		buf.SetPoint(pt - n)
	}
	// After deleting, re-check whether completion should still be shown
	// (e.g. the user removed one char but is still at "os." which triggers).
	e.lspCompDismiss()
	e.lspMaybeTriggerCompletion()
}

func (e *Editor) cmdKillLine() {
	if e.bufReadOnly() {
		e.clearArg()
		return
	}
	n := e.arg()
	e.clearArg()
	buf := e.ActiveBuffer()
	pt := buf.Point()

	var killed string
	for range n {
		end := buf.EndOfLine(buf.Point())
		cur := buf.Point()
		if end == cur {
			// Point is at end of line — kill the newline.
			if cur < buf.Len() {
				killed += buf.Delete(cur, 1)
			}
		} else {
			killed += buf.Delete(cur, end-cur)
		}
	}
	_ = pt
	if killed != "" {
		e.addToKillRing(killed)
	}
}

func (e *Editor) cmdKillRegion() {
	if e.bufReadOnly() {
		return
	}
	buf := e.ActiveBuffer()
	e.killRegionFrom(buf)
}

// killRegionFrom kills the active region in buf and pushes it to the kill ring.
func (e *Editor) killRegionFrom(buf *buffer.Buffer) {
	if !buf.MarkActive() {
		e.Message("Mark not active")
		return
	}
	pt := buf.Point()
	mark := buf.Mark()
	start, end := pt, mark
	if mark < pt {
		start, end = mark, pt
	}
	killed := buf.Delete(start, end-start)
	buf.SetPoint(start)
	buf.SetMarkActive(false)
	if killed != "" {
		e.addToKillRing(killed)
	}
}

func (e *Editor) cmdCopyRegionAsKill() {
	buf := e.ActiveBuffer()
	if !buf.MarkActive() {
		e.Message("Mark not active")
		return
	}
	pt := buf.Point()
	mark := buf.Mark()
	start, end := pt, mark
	if mark < pt {
		start, end = mark, pt
	}
	copied := buf.Substring(start, end)
	buf.SetMarkActive(false)
	if copied != "" {
		e.addToKillRing(copied)
		e.Message("Region saved")
	}
}

func (e *Editor) cmdYank() {
	if e.bufReadOnly() {
		e.clearArg()
		return
	}
	e.clearArg()
	text := e.yank()
	if text == "" {
		e.Message("Kill ring is empty")
		return
	}
	buf := e.ActiveBuffer()
	pt := buf.Point()
	buf.InsertString(pt, text)
	newPt := pt + len([]rune(text))
	buf.SetPoint(newPt)
	// Record for yank-pop.
	e.lastYankEnd = newPt
	e.lastYankLen = len([]rune(text))
	e.yankIdx = 0
}

func (e *Editor) cmdYankPop() {
	if e.bufReadOnly() {
		return
	}
	if len(e.killRing) == 0 {
		e.Message("Kill ring is empty")
		return
	}
	buf := e.ActiveBuffer()

	// Remove the previously yanked text.
	start := max(e.lastYankEnd-e.lastYankLen, 0)
	if e.lastYankLen > 0 {
		buf.Delete(start, e.lastYankLen)
		buf.SetPoint(start)
	}

	// Insert the next kill ring entry.
	text := e.yankPop()
	pt := buf.Point()
	buf.InsertString(pt, text)
	newPt := pt + len([]rune(text))
	buf.SetPoint(newPt)
	e.lastYankEnd = newPt
	e.lastYankLen = len([]rune(text))
}

func (e *Editor) cmdKillWord() {
	if e.bufReadOnly() {
		e.clearArg()
		return
	}
	n := e.arg()
	e.clearArg()
	buf := e.ActiveBuffer()
	pt := buf.Point()
	end := pt
	length := buf.Len()
	for range n {
		if e.subwordMode {
			end = subwordForwardOne(buf, end)
		} else {
			for end < length && !isWordRune(buf.RuneAt(end)) {
				end++
			}
			for end < length && isWordRune(buf.RuneAt(end)) {
				end++
			}
		}
	}
	if end > pt {
		killed := buf.Delete(pt, end-pt)
		e.addToKillRing(killed)
	}
}

func (e *Editor) cmdBackwardKillWord() {
	if e.bufReadOnly() {
		e.clearArg()
		return
	}
	n := e.arg()
	e.clearArg()
	buf := e.ActiveBuffer()
	pt := buf.Point()
	start := pt
	for range n {
		if e.subwordMode {
			start = subwordBackwardOne(buf, start)
		} else {
			for start > 0 && !isWordRune(buf.RuneAt(start-1)) {
				start--
			}
			for start > 0 && isWordRune(buf.RuneAt(start-1)) {
				start--
			}
		}
	}
	if start < pt {
		killed := buf.Delete(start, pt-start)
		buf.SetPoint(start)
		e.addToKillRing(killed)
	}
}

func (e *Editor) cmdTransposeChars() {
	if e.bufReadOnly() {
		return
	}
	e.clearArg()
	buf := e.ActiveBuffer()
	pt := buf.Point()
	length := buf.Len()

	// If at end of buffer, transpose the two chars before point.
	if pt == length && pt >= 2 {
		a := buf.RuneAt(pt - 2)
		b := buf.RuneAt(pt - 1)
		buf.Delete(pt-2, 2)
		buf.InsertString(pt-2, string([]rune{b, a}))
		return
	}
	if pt < 1 || pt >= length {
		return
	}
	a := buf.RuneAt(pt - 1)
	b := buf.RuneAt(pt)
	buf.Delete(pt-1, 2)
	buf.InsertString(pt-1, string([]rune{b, a}))
	buf.SetPoint(pt + 1)
}

func (e *Editor) cmdOpenLine() {
	if e.bufReadOnly() {
		return
	}
	e.clearArg()
	buf := e.ActiveBuffer()
	pt := buf.Point()
	buf.Insert(pt, '\n')
	// Leave point before the newline (standard Emacs open-line behaviour).
	buf.SetPoint(pt)
}

func (e *Editor) cmdUndo() {
	if e.bufReadOnly() {
		return
	}
	e.clearArg()
	buf := e.ActiveBuffer()
	if !buf.ApplyUndo() {
		e.Message("No further undo information")
	}
}

func (e *Editor) cmdRedo() {
	if e.bufReadOnly() {
		return
	}
	e.clearArg()
	buf := e.ActiveBuffer()
	if !buf.ApplyRedo() {
		e.Message("No further redo information")
	}
}

// ---------------------------------------------------------------------------
// Marks, search, misc
// ---------------------------------------------------------------------------

func (e *Editor) cmdSetMarkCommand() {
	buf := e.ActiveBuffer()
	if e.universalArgSet {
		// C-u C-SPC: pop mark ring and jump to previous mark.
		e.clearArg()
		prev := buf.PopMarkRing()
		if prev < 0 {
			e.Message("Mark ring is empty")
			return
		}
		buf.SetPoint(prev)
		e.Message("Mark popped")
		return
	}
	e.clearArg()
	// Push old mark onto ring before setting a new one.
	if buf.Mark() >= 0 {
		buf.PushMarkRing(buf.Mark())
	}
	buf.SetMark(buf.Point())
	buf.SetMarkActive(true)
	e.Message("Mark set")
}

func (e *Editor) cmdKeyboardQuit() {
	// Cancel any in-flight LSP operation.
	e.lspOpCancel()
	if e.minibufActive {
		e.cancelMinibuffer()
		return
	}
	if e.isearching {
		buf := e.ActiveBuffer()
		buf.SetPoint(e.isearchStart)
		e.isearching = false
		e.isearchStr = ""
	}
	if e.prefixKeymap != nil {
		e.prefixKeymap = nil
		e.prefixKeySeq = ""
	}
	buf := e.ActiveBuffer()
	buf.SetMarkActive(false)
	e.universalArgSet = false
	e.universalArg = 1
	e.Message("Quit")
}

func (e *Editor) cmdUniversalArgument() {
	if !e.universalArgSet {
		e.universalArg = 4
		e.universalArgSet = true
	} else {
		e.universalArg *= 4
	}
	e.Message("C-u %d-", e.universalArg)
}

func (e *Editor) cmdExecuteExtendedCommand() {
	e.ReadMinibuffer("M-x ", func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if fn, ok := commands[name]; ok {
			fn(e)
			e.pushCommandLRU(name)
		} else {
			e.Message("No command: %s", name)
		}
	})
	e.SetMinibufCompletions(func(prefix string) []string {
		lruPos := make(map[string]int, len(e.commandLRU))
		for i, name := range e.commandLRU {
			lruPos[name] = i
		}
		type scored struct {
			name     string
			score    int
			lruIndex int
		}
		var matches []scored
		for name := range commands {
			if !fuzzyMatch(name, prefix) {
				continue
			}
			idx, inLRU := lruPos[name]
			if !inLRU {
				idx = len(e.commandLRU)
			}
			matches = append(matches, scored{name, fuzzyScore(name, prefix), idx})
		}
		sort.Slice(matches, func(i, j int) bool {
			if matches[i].score != matches[j].score {
				return matches[i].score < matches[j].score
			}
			if matches[i].lruIndex != matches[j].lruIndex {
				return matches[i].lruIndex < matches[j].lruIndex
			}
			return matches[i].name < matches[j].name
		})
		out := make([]string, len(matches))
		for i, m := range matches {
			out[i] = m.name
		}
		return out
	})
}

func (e *Editor) cmdCommentDwim() {
	if e.bufReadOnly() {
		return
	}
	e.clearArg()
	buf := e.ActiveBuffer()
	pt := buf.Point()
	bol := buf.BeginningOfLine(pt)

	var prefix string
	switch buf.Mode() {
	case "go":
		prefix = "// "
	default:
		prefix = "# "
	}
	buf.InsertString(bol, prefix)
	buf.SetPoint(pt + len([]rune(prefix)))
}

func (e *Editor) cmdIsearchForward() {
	e.clearArg()
	e.startIsearch(true)
	e.Message("I-search: ")
}

func (e *Editor) cmdIsearchBackward() {
	e.clearArg()
	e.startIsearch(false)
	e.Message("I-search backward: ")
}

func (e *Editor) cmdExchangePointAndMark() {
	e.clearArg()
	buf := e.ActiveBuffer()
	if buf.Mark() < 0 {
		e.Message("No mark set in this buffer")
		return
	}
	pt := buf.Point()
	mark := buf.Mark()
	buf.SetPoint(mark)
	buf.SetMark(pt)
	buf.SetMarkActive(true)
}

// ---------------------------------------------------------------------------
// File / buffer commands
// ---------------------------------------------------------------------------

// cmdProjectFindFile presents a fuzzy-search menu of all files in the current
// project (VC root) and opens the selected file (C-x p f).
// Candidates are ordered LRU first, then by fuzzy match quality.
func (e *Editor) cmdProjectFindFile() {
	e.clearArg()

	_, root := vcFind(vcDir(e.ActiveBuffer()))
	if root == "" {
		// No VC root found — fall back to regular find-file.
		e.cmdFindFile()
		return
	}

	// Collect all project files synchronously (once per invocation).
	files := walkProjectFiles(root)

	e.ReadMinibuffer("Project find file: ", func(rel string) {
		rel = strings.TrimSpace(rel)
		if rel == "" {
			return
		}
		var absPath string
		if filepath.IsAbs(rel) {
			absPath = rel
		} else {
			absPath = filepath.Join(root, rel)
		}
		b, err := e.loadFile(absPath)
		if err != nil {
			e.Message("Error opening file: %v", err)
			return
		}
		e.activeWin.SetBuf(b)
		e.Message("Opened %s", rel)
	})
	e.SetMinibufCompletions(func(query string) []string {
		return e.projectFileCompletions(root, files, query)
	})
}

// projectFileCompletions returns relative file paths from root that fuzzy-match
// query, sorted LRU-first then by match quality then alphabetically.
func (e *Editor) projectFileCompletions(root string, files []string, query string) []string {
	// Build an LRU index from open buffers keyed by filename.
	lruIdx := make(map[string]int, len(e.bufferMRU))
	for i, b := range e.bufferMRU {
		if b.Filename() != "" {
			lruIdx[b.Filename()] = i
		}
	}
	// Also include the currently active buffer.
	if ab := e.ActiveBuffer(); ab != nil && ab.Filename() != "" {
		if _, seen := lruIdx[ab.Filename()]; !seen {
			lruIdx[ab.Filename()] = len(e.bufferMRU)
		}
	}

	type scored struct {
		rel    string
		abs    string
		score  int
		lruIdx int
	}

	noLRU := len(e.bufferMRU) + 1
	var matches []scored
	for _, absPath := range files {
		rel, _ := filepath.Rel(root, absPath)
		base := filepath.Base(absPath)
		if query != "" {
			// Match against both the base name and the relative path.
			if !fuzzyMatch(base, query) && !fuzzyMatch(rel, query) {
				continue
			}
		}
		idx, inLRU := lruIdx[absPath]
		if !inLRU {
			idx = noLRU
		}
		score := 0
		if query != "" {
			s1 := fuzzyScore(base, query)
			s2 := fuzzyScore(rel, query)
			score = min(s1, s2)
		}
		matches = append(matches, scored{rel, absPath, score, idx})
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score < matches[j].score
		}
		if matches[i].lruIdx != matches[j].lruIdx {
			return matches[i].lruIdx < matches[j].lruIdx
		}
		return matches[i].rel < matches[j].rel
	})

	result := make([]string, len(matches))
	for i, m := range matches {
		result[i] = m.rel
	}
	return result
}

// walkProjectFiles returns the absolute paths of all regular files under root,
// skipping common non-source directories (.git, node_modules, vendor, etc.).
func walkProjectFiles(root string) []string {
	var files []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".svn", ".hg", ".bzr",
				"node_modules", "vendor", "__pycache__",
				"target", ".gradle", ".idea", ".vscode",
				"dist", "build", ".cache":
				return filepath.SkipDir
			}
			if strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
				return filepath.SkipDir
			}
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files
}

func (e *Editor) cmdFindFile() {
	// Pre-populate with the directory of the current buffer (or cwd).
	defaultDir := e.bufferDir(e.ActiveBuffer())

	e.ReadMinibuffer("Find file: ", func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		// Expand ~.
		if strings.HasPrefix(path, "~/") {
			home, err := os.UserHomeDir()
			if err == nil {
				path = home + path[1:]
			}
		}
		// If the path is a directory, open it in dired instead.
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			e.openDired(path)
			return
		}
		b, err := e.loadFile(path)
		if err != nil {
			e.Message("Error opening file: %v", err)
			return
		}
		e.activeWin.SetBuf(b)
		e.Message("Opened %s", path)
	})
	// Insert the default directory into the minibuffer so the user can edit it.
	if defaultDir != "" {
		e.minibufBuf.InsertString(0, defaultDir)
		e.minibufBuf.SetPoint(e.minibufBuf.Len())
	}
	e.SetMinibufCompletions(filePathCompletions)
	e.SetMinibufPreferTyped(func(s string) bool {
		if strings.HasPrefix(s, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				s = home + s[1:]
			}
		}
		_, err := os.Stat(s)
		return err == nil
	})
}

func (e *Editor) cmdSaveBuffer() {
	e.clearArg()
	buf := e.ActiveBuffer()
	if buf.Filename() == "" {
		e.ReadMinibuffer("Write file: ", func(path string) {
			path = strings.TrimSpace(path)
			if path == "" {
				return
			}
			buf.SetFilename(path)
			e.writeBuffer(buf)
		})
		return
	}
	e.writeBuffer(buf)
}

func (e *Editor) writeBuffer(buf *buffer.Buffer) {
	if e.saveBufferDeleteTrailingWS {
		e.deleteTrailingWhitespace(buf, 0, buf.Len())
	}
	err := os.WriteFile(buf.Filename(), []byte(buf.String()), 0600)
	if err != nil {
		e.Message("Write error: %v", err)
		return
	}
	buf.SetModified(false)
	// Update mtime so auto-revert doesn't immediately reload what we just wrote.
	if info, err2 := os.Stat(buf.Filename()); err2 == nil {
		e.autoRevertMtimes[buf] = info.ModTime()
	}
	e.lspDidSave(buf)
	e.Message("Wrote %s", buf.Filename())
}

// cmdToggleReadOnly toggles the read-only state of the current buffer (C-x C-q).
func (e *Editor) cmdToggleReadOnly() {
	e.clearArg()
	buf := e.ActiveBuffer()
	buf.SetReadOnly(!buf.ReadOnly())
	if buf.ReadOnly() {
		e.Message("Buffer is read-only")
	} else {
		e.Message("Buffer is writable")
	}
}

func (e *Editor) cmdSaveBuffersKillTerminal() {
	// Collect buffers with unsaved changes that have a backing file.
	var unsaved []*buffer.Buffer
	for _, b := range e.buffers {
		if b.Modified() && b.Filename() != "" {
			unsaved = append(unsaved, b)
		}
	}
	if len(unsaved) == 0 {
		e.quit = true
		return
	}
	// Ask about each unsaved buffer in turn.
	e.promptSaveNext(unsaved, 0)
}

// promptSaveNext iterates through unsaved buffers one at a time, prompting
// the user whether to save each.  When all are handled it sets e.quit.
func (e *Editor) promptSaveNext(unsaved []*buffer.Buffer, idx int) {
	if idx >= len(unsaved) {
		e.quit = true
		return
	}
	b := unsaved[idx]
	e.Message("Save buffer %s? (y/n)", b.Name())
	e.readCharPending = true
	e.readCharCallback = func(r rune) {
		switch r {
		case 'y', 'Y':
			if err := os.WriteFile(b.Filename(), []byte(b.String()), 0600); err != nil {
				e.Message("Error saving %s: %v", b.Name(), err)
				return
			}
			b.SetModified(false)
		}
		// 'n' or anything else: skip this buffer, continue with the next.
		e.promptSaveNext(unsaved, idx+1)
	}
}

func (e *Editor) cmdSwitchToBuffer() {
	// Build a default suggestion: the most recently visited other buffer.
	defaultName := ""
	for _, b := range e.bufferMRU {
		if b != e.ActiveBuffer() {
			defaultName = b.Name()
			break
		}
	}
	if defaultName == "" {
		for _, b := range e.buffers {
			if b != e.ActiveBuffer() {
				defaultName = b.Name()
				break
			}
		}
	}
	prompt := fmt.Sprintf("Switch to buffer (default %s): ", defaultName)
	e.ReadMinibuffer(prompt, func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			name = defaultName
		}
		if name == "" {
			return
		}
		e.SwitchToBuffer(name)
	})
	e.SetMinibufCompletions(func(prefix string) []string {
		// Show MRU buffers first, then remaining buffers alphabetically.
		inMRU := make(map[string]bool, len(e.bufferMRU))
		var mruMatches []string
		for _, b := range e.bufferMRU {
			if fuzzyMatch(b.Name(), prefix) {
				mruMatches = append(mruMatches, b.Name())
				inMRU[b.Name()] = true
			}
		}
		var rest []string
		for _, b := range e.buffers {
			if !inMRU[b.Name()] && fuzzyMatch(b.Name(), prefix) {
				rest = append(rest, b.Name())
			}
		}
		sort.Strings(rest)
		return append(mruMatches, rest...)
	})
}

func (e *Editor) cmdKillBuffer() {
	cur := e.ActiveBuffer().Name()
	prompt := fmt.Sprintf("Kill buffer (default %s): ", cur)
	e.ReadMinibuffer(prompt, func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			name = cur
		}
		e.KillBuffer(name)
		e.Message("Buffer %s killed", name)
	})
}

func (e *Editor) cmdListBuffers() {
	e.clearArg()
	var sb strings.Builder
	sb.WriteString(" CRM  Buffer                    Size  Mode         File\n")
	sb.WriteString(" ---  ------                    ----  ----         ----\n")
	for _, b := range e.buffers {
		mod := " "
		if b.Modified() {
			mod = "*"
		}
		cur := " "
		if b == e.ActiveBuffer() {
			cur = "."
		}
		line := fmt.Sprintf("  %s%s   %-24s  %6d  %-12s %s\n",
			cur, mod, b.Name(), b.Len(), b.Mode(), b.Filename())
		sb.WriteString(line)
	}
	listBuf := e.FindBuffer("*Buffer List*")
	if listBuf == nil {
		listBuf = buffer.NewWithContent("*Buffer List*", sb.String())
		listBuf.SetMode("buffer-list")
		listBuf.SetReadOnly(true)
		e.buffers = append(e.buffers, listBuf)
	} else {
		listBuf.SetReadOnly(false)
		listBuf.Delete(0, listBuf.Len())
		listBuf.InsertString(0, sb.String())
		listBuf.SetReadOnly(true)
	}
	// Start on the first actual buffer line (skip 2 header lines).
	firstLine := listBuf.EndOfLine(0) + 1        // skip header line 1
	firstLine = listBuf.EndOfLine(firstLine) + 1 // skip header line 2
	if firstLine > listBuf.Len() {
		firstLine = 0
	}
	listBuf.SetPoint(firstLine)
	e.activeWin.SetBuf(listBuf)
}

// ---------------------------------------------------------------------------
// New file/buffer commands
// ---------------------------------------------------------------------------

// cmdSaveSomeBuffers saves all modified file-visiting buffers (C-x s).
func (e *Editor) cmdSaveSomeBuffers() {
	e.clearArg()
	saved := 0
	for _, b := range e.buffers {
		if b.Modified() && b.Filename() != "" {
			if err := os.WriteFile(b.Filename(), []byte(b.String()), 0600); err != nil {
				e.Message("Error saving %s: %v", b.Name(), err)
				return
			}
			b.SetModified(false)
			saved++
		}
	}
	if saved == 0 {
		e.Message("(No files need saving)")
	} else {
		e.Message("Saved %d file(s)", saved)
	}
}

// cmdDeleteOtherWindows deletes all windows except the selected one (C-x 1).
func (e *Editor) cmdDeleteOtherWindows() {
	e.clearArg()
	if len(e.windows) <= 1 {
		return
	}
	// Keep only the active window; resize it to fill the full available area.
	e.windows = []*window.Window{e.activeWin}
	if e.term != nil {
		w, h := e.term.Size()
		e.activeWin.SetRegion(0, 0, w, h-1)
		e.minibufWin.SetRegion(h-1, 0, w, 1)
	}
	e.invalidateLayout()
}

// cmdSplitWindowBelow splits the active window into two windows stacked
// vertically (C-x 2).  Both show the same buffer.
func (e *Editor) cmdSplitWindowBelow() {
	e.clearArg()
	w := e.activeWin
	totalH := w.Height()
	if totalH < 4 {
		e.Message("Window too small to split")
		return
	}
	topH := totalH / 2
	botH := totalH - topH
	top := w.Top()
	left := w.Left()
	width := w.Width()

	w.SetRegion(top, left, width, topH)
	newWin := window.New(w.Buf(), top+topH, left, width, botH)
	newWin.SetPoint(w.Point())
	e.windows = append(e.windows, newWin)
	e.invalidateLayout()
}

// cmdSplitWindowRight splits the active window into two side-by-side windows
// (C-x 3).  Both show the same buffer.
func (e *Editor) cmdSplitWindowRight() {
	e.clearArg()
	w := e.activeWin
	totalW := w.Width()
	if totalW < 5 {
		e.Message("Window too small to split")
		return
	}
	// Leave 1 column for the │ separator.
	leftW := (totalW - 1) / 2
	rightW := totalW - leftW - 1
	top := w.Top()
	left := w.Left()
	height := w.Height()

	w.SetRegion(top, left, leftW, height)
	newWin := window.New(w.Buf(), top, left+leftW+1, rightW, height)
	newWin.SetPoint(w.Point())
	e.windows = append(e.windows, newWin)
	e.invalidateLayout()
}

// cmdOtherWindow switches focus to the next window (C-x o).
func (e *Editor) cmdOtherWindow() {
	e.clearArg()
	if len(e.windows) <= 1 {
		return
	}
	for i, w := range e.windows {
		if w == e.activeWin {
			next := e.windows[(i+1)%len(e.windows)]
			// Restore the incoming window's saved point into the shared buffer
			// so that syncWindowPoint (called after dispatch) keeps it intact.
			next.Buf().SetPoint(next.Point())
			e.activeWin = next
			return
		}
	}
}

// ---------------------------------------------------------------------------
// Sentence commands
// ---------------------------------------------------------------------------

// isSentenceEnd reports whether runes[i] ends a sentence: it must be `.`,
// `?`, or `!` followed by whitespace or end-of-buffer.
func isSentenceEnd(runes []rune, i int) bool {
	if i >= len(runes) {
		return false
	}
	r := runes[i]
	if r != '.' && r != '?' && r != '!' {
		return false
	}
	return i+1 >= len(runes) || runes[i+1] == ' ' || runes[i+1] == '\n'
}

func (e *Editor) cmdBeginningOfSentence() {
	n := e.arg()
	e.clearArg()
	buf := e.ActiveBuffer()
	pt := buf.Point()
	runes := []rune(buf.String())
	for range n {
		// Step back past any leading whitespace.
		for pt > 0 && (runes[pt-1] == ' ' || runes[pt-1] == '\n') {
			pt--
		}
		// Step back until we find a sentence terminator or beginning of buffer.
		for pt > 0 {
			if isSentenceEnd(runes, pt-1) {
				break
			}
			pt--
		}
		// Skip whitespace after the sentence terminator.
		for pt < len(runes) && (runes[pt] == ' ' || runes[pt] == '\n') {
			pt++
		}
	}
	buf.SetPoint(pt)
}

func (e *Editor) cmdEndOfSentence() {
	n := e.arg()
	e.clearArg()
	buf := e.ActiveBuffer()
	pt := buf.Point()
	runes := []rune(buf.String())
	for range n {
		// Skip whitespace.
		for pt < len(runes) && (runes[pt] == ' ' || runes[pt] == '\n') {
			pt++
		}
		// Advance to next sentence end.
		for pt < len(runes) {
			if isSentenceEnd(runes, pt) {
				pt++
				break
			}
			pt++
		}
	}
	buf.SetPoint(pt)
}

func (e *Editor) cmdKillSentence() {
	if e.bufReadOnly() {
		e.clearArg()
		return
	}
	n := e.arg()
	e.clearArg()
	buf := e.ActiveBuffer()
	start := buf.Point()
	runes := []rune(buf.String())
	pt := start
	for range n {
		for pt < len(runes) && (runes[pt] == ' ' || runes[pt] == '\n') {
			pt++
		}
		for pt < len(runes) {
			if isSentenceEnd(runes, pt) {
				pt++
				break
			}
			pt++
		}
	}
	if pt > start {
		killed := buf.Delete(start, pt-start)
		e.addToKillRing(killed)
	}
}

// ---------------------------------------------------------------------------
// Case commands
// ---------------------------------------------------------------------------

func (e *Editor) cmdUpcaseWord() {
	n := e.arg()
	e.clearArg()
	buf := e.ActiveBuffer()
	pt := buf.Point()
	length := buf.Len()
	for range n {
		start := pt
		if e.subwordMode {
			pt = subwordForwardOne(buf, pt)
			// subwordForwardOne skips leading non-word chars; start is after them.
			for start < pt && !isWordRune(buf.RuneAt(start)) {
				start++
			}
		} else {
			for pt < length && !isWordRune(buf.RuneAt(pt)) {
				pt++
			}
			start = pt
			for pt < length && isWordRune(buf.RuneAt(pt)) {
				pt++
			}
		}
		if pt > start {
			buf.ReplaceString(start, pt-start, strings.ToUpper(buf.Substring(start, pt)))
		}
	}
	buf.SetPoint(pt)
}

func (e *Editor) cmdDowncaseWord() {
	n := e.arg()
	e.clearArg()
	buf := e.ActiveBuffer()
	pt := buf.Point()
	length := buf.Len()
	for range n {
		start := pt
		if e.subwordMode {
			pt = subwordForwardOne(buf, pt)
			for start < pt && !isWordRune(buf.RuneAt(start)) {
				start++
			}
		} else {
			for pt < length && !isWordRune(buf.RuneAt(pt)) {
				pt++
			}
			start = pt
			for pt < length && isWordRune(buf.RuneAt(pt)) {
				pt++
			}
		}
		if pt > start {
			buf.ReplaceString(start, pt-start, strings.ToLower(buf.Substring(start, pt)))
		}
	}
	buf.SetPoint(pt)
}

func (e *Editor) cmdCapitalizeWord() {
	n := e.arg()
	e.clearArg()
	buf := e.ActiveBuffer()
	pt := buf.Point()
	length := buf.Len()
	for range n {
		start := pt
		if e.subwordMode {
			pt = subwordForwardOne(buf, pt)
			for start < pt && !isWordRune(buf.RuneAt(start)) {
				start++
			}
		} else {
			for pt < length && !isWordRune(buf.RuneAt(pt)) {
				pt++
			}
			start = pt
			for pt < length && isWordRune(buf.RuneAt(pt)) {
				pt++
			}
		}
		if pt > start {
			word := buf.Substring(start, pt)
			runes := []rune(word)
			runes[0] = unicode.ToUpper(runes[0])
			for j := 1; j < len(runes); j++ {
				runes[j] = unicode.ToLower(runes[j])
			}
			buf.ReplaceString(start, pt-start, string(runes))
		}
	}
	buf.SetPoint(pt)
}

// ---------------------------------------------------------------------------
// Eval commands
// ---------------------------------------------------------------------------

// cmdEvalLastSexp evaluates the Elisp expression immediately before point (C-x C-e).
func (e *Editor) cmdEvalLastSexp() {
	e.clearArg()
	buf := e.ActiveBuffer()
	text := buf.Substring(0, buf.Point())
	sexp := lastSexp(text)
	if sexp == "" {
		e.Message("No sexp before point")
		return
	}
	result, err := e.evalSexp(sexp)
	if err != nil {
		e.Message("Error: %v", err)
		return
	}
	e.Message("%s", result)
}

// lastSexp finds the last complete s-expression in text.
func lastSexp(text string) string {
	runes := []rune(text)
	n := len(runes)
	if n == 0 {
		return ""
	}

	// Walk backwards skipping trailing whitespace.
	end := n
	for end > 0 && (runes[end-1] == ' ' || runes[end-1] == '\n' || runes[end-1] == '\t') {
		end--
	}
	if end == 0 {
		return ""
	}

	// If the last non-space char ends a string or paren expression, find its start.
	last := runes[end-1]
	if last == ')' {
		// Find matching '('.
		depth := 0
		for i := end - 1; i >= 0; i-- {
			switch runes[i] {
			case ')':
				depth++
			case '(':
				depth--
				if depth == 0 {
					return string(runes[i:end])
				}
			}
		}
		return ""
	}
	if last == '"' {
		// Find matching opening '"'.
		for i := end - 2; i >= 0; i-- {
			if runes[i] == '"' && (i == 0 || runes[i-1] != '\\') {
				return string(runes[i:end])
			}
		}
		return ""
	}

	// Otherwise treat as an atom: scan back to whitespace or delimiter.
	start := end - 1
	for start > 0 {
		r := runes[start-1]
		if r == ' ' || r == '\n' || r == '\t' || r == '(' || r == ')' {
			break
		}
		start--
	}
	return string(runes[start:end])
}

// ---------------------------------------------------------------------------
// File path completion
// ---------------------------------------------------------------------------

// bufferDir returns the directory of buf's associated file, or the process
// working directory if the buffer has no file.  Always ends with "/".
// For dired buffers, returns the dired directory.
func (e *Editor) bufferDir(buf *buffer.Buffer) string {
	if buf.Mode() == "dired" {
		if ds := e.diredStates[buf]; ds != nil && ds.dir != "" {
			return ds.dir + "/"
		}
	}
	if f := buf.Filename(); f != "" {
		dir := filepath.Dir(f)
		if dir != "" && dir != "." {
			return dir + "/"
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd + "/"
	}
	return ""
}

// filePathCompletions returns file and directory completions for prefix.
func filePathCompletions(prefix string) []string {
	// Expand leading ~.
	expanded := prefix
	if strings.HasPrefix(expanded, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			expanded = home + expanded[1:]
		}
	} else if expanded == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			expanded = home
		}
	}

	// Determine the directory to list and the file prefix within it.
	dir := filepath.Dir(expanded)
	base := filepath.Base(expanded)
	if strings.HasSuffix(expanded, "/") || expanded == "" {
		dir = expanded
		if dir == "" {
			dir = "."
		}
		base = ""
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var results []string
	for _, entry := range entries {
		name := entry.Name()
		if base != "" && !fuzzyMatch(name, base) {
			continue
		}
		// Reconstruct the full completion using the original prefix's directory.
		var full string
		if strings.HasSuffix(prefix, "/") || prefix == "" {
			full = prefix + name
		} else {
			full = filepath.Join(filepath.Dir(prefix), name)
		}
		if entry.IsDir() {
			full += "/"
		}
		results = append(results, full)
	}
	// Sort by match quality (prefix > substring > subsequence), then alphabetically.
	sort.Slice(results, func(i, j int) bool {
		si := fuzzyScore(filepath.Base(results[i]), base)
		sj := fuzzyScore(filepath.Base(results[j]), base)
		if si != sj {
			return si < sj
		}
		return results[i] < results[j]
	})
	return results
}

// ---------------------------------------------------------------------------
// Help commands
// ---------------------------------------------------------------------------

// cmdDescribeKey starts key-capture mode: the next complete key sequence
// is looked up and its bound command + documentation are shown.
func (e *Editor) cmdDescribeKey() {
	e.clearArg()
	e.describeKeyPending = true
	e.describeKeySeq = ""
	e.describeKeyMap = nil
	e.Message("Describe key: ")
}

// cmdDescribeFunction reads a command name (with TAB completion) and shows
// its documentation in the *Help* buffer.
func (e *Editor) cmdDescribeFunction() {
	e.ReadMinibuffer("Describe function: ", func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		e.showCommandHelp("", name)
	})
	e.SetMinibufCompletions(func(prefix string) []string {
		var matches []string
		for name := range commands {
			if fuzzyMatch(name, prefix) {
				matches = append(matches, name)
			}
		}
		sort.Strings(matches)
		return matches
	})
}

// cmdDescribeVariable reads a variable name (with TAB completion over all
// global Elisp variables) and shows its current value in the *Help* buffer.
func (e *Editor) cmdDescribeVariable() {
	e.ReadMinibuffer("Describe variable: ", func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		e.showVariableHelp(name)
	})
	e.SetMinibufCompletions(func(prefix string) []string {
		var matches []string
		for _, name := range e.lisp.GlobalVarNames() {
			if fuzzyMatch(name, prefix) {
				matches = append(matches, name)
			}
		}
		sort.Strings(matches)
		return matches
	})
}

// ---------------------------------------------------------------------------
// Theme commands
// ---------------------------------------------------------------------------

// cmdLoadTheme reads a theme name from the minibuffer and applies it.
func (e *Editor) cmdLoadTheme() {
	e.ReadMinibuffer("Load theme: ", func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if !syntax.LoadTheme(name) {
			e.Message("Unknown theme: %s", name)
			return
		}
		if e.term != nil {
			e.term.InvalidateStyleCache()
		}
		e.Message("Theme %s loaded", name)
	})
	e.SetMinibufCompletions(func(prefix string) []string {
		var matches []string
		for _, name := range []string{"sweet", "default"} {
			if fuzzyMatch(name, prefix) {
				matches = append(matches, name)
			}
		}
		sort.Strings(matches)
		return matches
	})
	e.refreshMinibufCandidates()
}
