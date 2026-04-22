package editor

import (
	"encoding/json"
	"strings"
	"unicode"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/dap"
	"github.com/skybert/gomacs/internal/terminal"
)

const dapReplPrompt = "> "

// dapReplReset clears replBuf and writes the initial prompt line.
func dapReplReset(replBuf *buffer.Buffer) {
	replBuf.SetReadOnly(false)
	replBuf.Delete(0, replBuf.Len())
	replBuf.InsertString(0, dapReplPrompt)
	replBuf.SetPoint(replBuf.Len())
}

// dapReplAppend inserts text before the input (prompt) line in the REPL buffer.
func (e *Editor) dapReplAppend(text string) {
	if e.dap == nil || e.dap.replBuf == nil {
		return
	}
	buf := e.dap.replBuf
	buf.SetReadOnly(false)

	// Find the start of the current (last) prompt line.
	content := buf.Substring(0, buf.Len())
	promptStart := strings.LastIndex(content, dapReplPrompt)
	if promptStart < 0 {
		promptStart = buf.Len()
	}
	promptStartRunes := len([]rune(content[:promptStart]))

	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	buf.InsertString(promptStartRunes, text)
	buf.SetPoint(buf.Len())
	e.dapReplEnsureVisible()
}

// dapReplEnsureVisible scrolls the REPL window so that the buffer point
// (always at the prompt) is visible.  The REPL window is typically not the
// active window, so we must sync it manually.
func (e *Editor) dapReplEnsureVisible() {
	if e.dap == nil || e.dap.replBuf == nil {
		return
	}
	buf := e.dap.replBuf
	for _, w := range e.windows {
		if w.Buf() == buf {
			e.syncWindowPoint(w)
			w.EnsurePointVisible()
			return
		}
	}
}

// dapReplGetInput returns the current input text (everything after the last prompt).
func dapReplGetInput(buf *buffer.Buffer) string {
	content := buf.Substring(0, buf.Len())
	idx := strings.LastIndex(content, dapReplPrompt)
	if idx < 0 {
		return content
	}
	return content[idx+len(dapReplPrompt):]
}

// dapReplPromptPos returns the rune position of the start of the current
// input (i.e. right after the last "> " prompt).
func dapReplPromptPos(buf *buffer.Buffer) int {
	content := buf.Substring(0, buf.Len())
	idx := strings.LastIndex(content, dapReplPrompt)
	if idx < 0 {
		return 0
	}
	return len([]rune(content[:idx])) + len([]rune(dapReplPrompt))
}

// debugReplDispatch handles key events when the active buffer is in "debug-repl" mode.
// Returns true if the key was consumed.
func (e *Editor) debugReplDispatch(ke terminal.KeyEvent) bool {
	if e.dap == nil || e.dap.replBuf == nil {
		return false
	}
	buf := e.dap.replBuf
	inputStart := dapReplPromptPos(buf)

	switch ke.Key {
	case tcell.KeyEnter:
		e.dapReplSubmit()
		return true

	case tcell.KeyTab:
		e.dapReplComplete()
		return true

	case tcell.KeyBackspace:
		// Only allow backspace at or after the prompt.
		if buf.Point() > inputStart {
			buf.SetReadOnly(false)
			e.cmdBackwardDeleteChar()
		}
		return true

	case tcell.KeyUp, tcell.KeyCtrlP:
		e.dapReplHistoryPrev()
		return true

	case tcell.KeyDown, tcell.KeyCtrlN:
		e.dapReplHistoryNext()
		return true

	case tcell.KeyLeft:
		if buf.Point() > inputStart {
			e.cmdBackwardChar()
		}
		return true

	case tcell.KeyHome, tcell.KeyCtrlA:
		buf.SetPoint(inputStart)
		return true

	case tcell.KeyEnd, tcell.KeyCtrlE:
		buf.SetPoint(buf.Len())
		return true

	case tcell.KeyRune:
		if ke.Mod != 0 {
			return false
		}
		// Allow printable runes only; ensure point is on the input line.
		if unicode.IsPrint(ke.Rune) {
			if buf.Point() < inputStart {
				buf.SetPoint(buf.Len())
			}
			buf.SetReadOnly(false)
			buf.Insert(buf.Point(), ke.Rune)
			buf.SetPoint(buf.Point() + 1)
			buf.SetReadOnly(true)
			return true
		}
		// 'q' on empty input → exit session.
		if ke.Rune == 'q' && strings.TrimSpace(dapReplGetInput(buf)) == "" {
			e.cmdDebugExit()
			return true
		}
	}
	return false
}

// dapReplComplete performs Tab completion in the REPL using known local
// variable names.  It finds the identifier token just before the cursor,
// matches it against all variables in scope, and either completes (single
// match) or shows candidates (multiple matches).
func (e *Editor) dapReplComplete() {
	if e.dap == nil || e.dap.replBuf == nil {
		return
	}
	buf := e.dap.replBuf
	inputStart := dapReplPromptPos(buf)
	pt := buf.Point()
	if pt < inputStart {
		pt = buf.Len()
	}

	// Extract the identifier token ending at pt.
	isIdent := func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
	}
	start := pt
	for start > inputStart && isIdent(buf.RuneAt(start-1)) {
		start--
	}
	prefix := buf.Substring(start, pt)

	// Collect all variable names from locals.
	e.dap.localsMu.RLock()
	names := dapCollectVarNames(e.dap.locals)
	e.dap.localsMu.RUnlock()

	// Filter by prefix.
	var matches []string
	for _, n := range names {
		if strings.HasPrefix(n, prefix) {
			matches = append(matches, n)
		}
	}
	if len(matches) == 0 {
		e.Message("No completions for %q", prefix)
		return
	}

	// Find longest common prefix of all matches.
	common := matches[0]
	for _, m := range matches[1:] {
		common = commonPrefixTwo(common, m)
	}

	if common != prefix {
		// Extend current token to the common prefix.
		buf.SetReadOnly(false)
		if pt > start {
			buf.Delete(start, pt)
		}
		buf.InsertString(start, common)
		buf.SetPoint(start + len([]rune(common)))
		buf.SetReadOnly(true)
	}

	if len(matches) == 1 {
		return // single match, already inserted
	}
	e.Message("Completions: %s", strings.Join(matches, "  "))
}

// dapCollectVarNames returns all variable names from the given variable tree.
func dapCollectVarNames(vars []dapVariable) []string {
	var names []string
	for _, v := range vars {
		names = append(names, v.name)
		if v.expanded {
			names = append(names, dapCollectVarNames(v.children)...)
		}
	}
	return names
}

// commonPrefixTwo returns the longest common prefix of a and b.
func commonPrefixTwo(a, b string) string {
	ra, rb := []rune(a), []rune(b)
	n := min(len(ra), len(rb))
	for i := range n {
		if ra[i] != rb[i] {
			return string(ra[:i])
		}
	}
	return string(ra[:n])
}

// dapReplSubmit evaluates the current input line.
func (e *Editor) dapReplSubmit() {
	if e.dap == nil || e.dap.replBuf == nil {
		return
	}
	input := strings.TrimRight(dapReplGetInput(e.dap.replBuf), "\n")
	if input == "" {
		return
	}

	// Save to history (dedup consecutive).
	if len(e.dap.replHistory) == 0 || e.dap.replHistory[len(e.dap.replHistory)-1] != input {
		e.dap.replHistory = append(e.dap.replHistory, input)
	}
	e.dap.replHistoryIdx = len(e.dap.replHistory)

	// Replace input text with "input\n> " so the echoed command stays on the
	// prompt line and a fresh prompt follows.
	buf := e.dap.replBuf
	buf.SetReadOnly(false)
	inputStart := dapReplPromptPos(buf)
	if buf.Len() > inputStart {
		buf.Delete(inputStart, buf.Len())
	}
	buf.InsertString(buf.Len(), input+"\n"+dapReplPrompt)
	buf.SetPoint(buf.Len())
	buf.SetReadOnly(true)
	e.dapReplEnsureVisible()

	// Send evaluate request.
	if e.dap.client == nil {
		e.dapReplAppend("(no active session)")
		return
	}
	frameID := 0
	if len(e.dap.frames) > 0 {
		frameID = e.dap.frames[0].ID
	}
	stoppedThread := e.dap.stoppedThread
	client := e.dap.client
	expr := input
	e.dapAsync(func() func() {
		// If frameID is still 0 (frames not yet fetched), get it now.
		if frameID == 0 && stoppedThread != 0 {
			raw, err := client.Request("stackTrace", dap.StackTraceArgs{
				ThreadID: stoppedThread,
				Levels:   1,
			})
			if err == nil {
				var resp dap.StackTraceResponse
				if jerr := json.Unmarshal(raw, &resp); jerr == nil && len(resp.StackFrames) > 0 {
					frameID = resp.StackFrames[0].ID
				}
			}
		}
		raw, err := client.Request("evaluate", dap.EvaluateArgs{
			Expression: expr,
			FrameID:    frameID,
			Context:    "repl",
		})
		if err != nil {
			return func() { e.dapReplAppend("Error: " + err.Error()) }
		}
		var resp dap.EvaluateResponse
		if unmarshalErr := json.Unmarshal(raw, &resp); unmarshalErr != nil {
			return func() { e.dapReplAppend("Error: " + unmarshalErr.Error()) }
		}
		result := resp.Result
		return func() { e.dapReplAppend(result) }
	})
}

// dapReplHistoryPrev cycles to the previous history entry.
func (e *Editor) dapReplHistoryPrev() {
	if e.dap == nil || len(e.dap.replHistory) == 0 {
		return
	}
	if e.dap.replHistoryIdx > 0 {
		e.dap.replHistoryIdx--
	}
	e.dapReplSetInput(e.dap.replHistory[e.dap.replHistoryIdx])
}

// dapReplHistoryNext cycles to the next history entry (or clears on past-end).
func (e *Editor) dapReplHistoryNext() {
	if e.dap == nil {
		return
	}
	if e.dap.replHistoryIdx < len(e.dap.replHistory)-1 {
		e.dap.replHistoryIdx++
		e.dapReplSetInput(e.dap.replHistory[e.dap.replHistoryIdx])
	} else {
		e.dap.replHistoryIdx = len(e.dap.replHistory)
		e.dapReplSetInput("")
	}
}

// dapReplSetInput replaces the current input line with text.
func (e *Editor) dapReplSetInput(text string) {
	if e.dap == nil || e.dap.replBuf == nil {
		return
	}
	buf := e.dap.replBuf
	buf.SetReadOnly(false)
	inputStart := dapReplPromptPos(buf)
	end := buf.Len()
	if end > inputStart {
		buf.Delete(inputStart, end)
	}
	if text != "" {
		buf.InsertString(inputStart, text)
	}
	buf.SetPoint(buf.Len())
	buf.SetReadOnly(true)
}
