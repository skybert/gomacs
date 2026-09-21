package editor

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/terminal"
)

// cmdQueryReplace starts the interactive query-replace (M-%).
func (e *Editor) cmdQueryReplace() {
	e.clearArg()
	e.ReadMinibuffer("Query replace: ", func(from string) {
		if from == "" {
			return
		}
		prompt := fmt.Sprintf("Query replace %s with: ", from)
		e.ReadMinibuffer(prompt, func(to string) {
			e.startQueryReplace(from, to)
		})
	})
}

// startQueryReplace initialises query-replace state and finds the first match.
func (e *Editor) startQueryReplace(from, to string) {
	if from == "" {
		return
	}
	e.queryReplaceFrom = from
	e.queryReplaceFromRunes = []rune(from)
	e.queryReplaceTo = to
	e.queryReplaceCursor = e.ActiveBuffer().Point()
	e.queryReplaceMatch = -1
	e.queryReplaceActive = true
	if !e.queryReplaceFindNext() {
		e.queryReplaceActive = false
		e.Message("No matches for %q", from)
	}
}

// queryReplaceNeedle returns the search string as runes, reusing the cached
// slice as long as queryReplaceFrom has not changed.
func (e *Editor) queryReplaceNeedle() []rune {
	if string(e.queryReplaceFromRunes) != e.queryReplaceFrom {
		e.queryReplaceFromRunes = []rune(e.queryReplaceFrom)
	}
	return e.queryReplaceFromRunes
}

// queryReplaceFindNext locates the next occurrence of queryReplaceFrom
// starting at queryReplaceCursor.  Returns true if a match is found.
func (e *Editor) queryReplaceFindNext() bool {
	buf := e.ActiveBuffer()
	// Reuse the changeGen-keyed rune cache instead of re-materialising the
	// whole buffer on every call; without it each keystroke copied the entire
	// buffer (~8ms for a 50k-line file).
	runes := e.isearchGetRunes(buf)
	needle := e.queryReplaceNeedle()
	if len(needle) == 0 {
		e.queryReplaceMatch = -1
		return false
	}
	pos := max(e.queryReplaceCursor, 0)
	for pos <= len(runes)-len(needle) {
		if runesMatch(runes[pos:], needle) {
			e.queryReplaceMatch = pos
			buf.SetPoint(pos + len(needle))
			e.activeWin.EnsurePointVisible()
			e.queryReplaceFindPrompt()
			return true
		}
		pos++
	}
	e.queryReplaceMatch = -1
	return false
}

// queryReplaceFindPrompt shows the current query-replace prompt.
func (e *Editor) queryReplaceFindPrompt() {
	e.Message("Query replacing %q with %q: (y/n/q/!/.)  ?=help",
		e.queryReplaceFrom, e.queryReplaceTo)
}

// queryReplaceHandleKey processes a single key during query-replace.
//
//nolint:exhaustive // external enum; default case handles unknowns
func (e *Editor) queryReplaceHandleKey(ke terminal.KeyEvent) {
	buf := e.ActiveBuffer()
	switch {
	case ke.Key == tcell.KeyRune && ke.Rune == 'y',
		ke.Key == tcell.KeyRune && ke.Rune == ' ':
		// Replace this occurrence.
		e.queryReplaceDoReplace()

	case ke.Key == tcell.KeyRune && ke.Rune == 'n',
		ke.Key == tcell.KeyBackspace,
		ke.Key == tcell.KeyDelete:
		// Skip this occurrence.
		e.queryReplaceCursor = e.queryReplaceMatch + 1
		if !e.queryReplaceFindNext() {
			e.queryReplaceFinish("No more matches")
		}

	case ke.Key == tcell.KeyRune && ke.Rune == '!':
		// Replace all remaining occurrences without asking.
		count := e.queryReplaceAll()
		e.queryReplaceFinish(fmt.Sprintf("Replaced %d occurrence(s)", count))

	case ke.Key == tcell.KeyRune && ke.Rune == '.':
		// Replace this occurrence and quit.
		e.queryReplaceDoReplace()
		e.queryReplaceFinish("Done")

	case ke.Key == tcell.KeyRune && ke.Rune == 'q',
		ke.Key == tcell.KeyEnter,
		ke.Key == tcell.KeyEscape,
		ke.Key == tcell.KeyCtrlG:
		// Quit without replacing.
		e.queryReplaceFinish("Query replace done")
		_ = buf

	case ke.Key == tcell.KeyRune && (ke.Rune == '?' || ke.Rune == 'h'):
		// Show help.
		e.Message("y=replace  n=skip  !=replace all  .=replace+quit  q/ESC=quit")

	default:
		e.queryReplaceFindPrompt()
	}
}

// queryReplaceDoReplace replaces the current match and finds the next one.
func (e *Editor) queryReplaceDoReplace() {
	e.queryReplaceDoReplaceRaw()
	if !e.queryReplaceFindNext() {
		e.queryReplaceFinish(fmt.Sprintf("Replaced; no more matches for %q", e.queryReplaceFrom))
	}
}

// queryReplaceDoReplaceRaw performs the substitution without searching for next.
func (e *Editor) queryReplaceDoReplaceRaw() {
	buf := e.ActiveBuffer()
	ms := e.queryReplaceMatch
	needle := e.queryReplaceNeedle()
	to := e.queryReplaceTo
	// One gap move and one undo record instead of Delete + InsertString.
	buf.ReplaceString(ms, len(needle), to)
	e.queryReplaceCursor = ms + len([]rune(to))
	e.queryReplaceMatch = -1
}

// queryReplaceAll replaces the current match and every following one, then
// returns the number of replacements made.
//
// The buffer is scanned once (over the cached rune slice) while the replacement
// text for the whole affected span is assembled, and the span from the first to
// the last match is then rewritten with a single buf.ReplaceString call.  That
// makes replace-all O(buffer) with one undo record and one redraw, instead of
// the old O(matches x buffer) loop that re-materialised the buffer per match.
//
// Searching resumes *after* each match rather than inside the text that was
// just inserted, so a replacement containing the search string is never
// re-replaced (and the scan always terminates).
func (e *Editor) queryReplaceAll() int {
	buf := e.ActiveBuffer()
	needle := e.queryReplaceNeedle()
	if len(needle) == 0 || e.queryReplaceMatch < 0 {
		return 0
	}
	runes := e.isearchGetRunes(buf)
	toRunes := []rune(e.queryReplaceTo)

	first := e.queryReplaceMatch
	// out holds the rewritten text for [first, lastEnd): the untouched runes
	// between matches interleaved with the replacement.
	out := make([]rune, 0, len(needle)*8)
	count := 0
	prev, lastEnd := first, first
	for pos := first; pos <= len(runes)-len(needle); {
		if runesMatch(runes[pos:], needle) {
			out = append(out, runes[prev:pos]...)
			out = append(out, toRunes...)
			pos += len(needle)
			prev, lastEnd = pos, pos
			count++
			continue
		}
		pos++
	}
	if count == 0 {
		e.queryReplaceMatch = -1
		return 0
	}

	buf.ReplaceString(first, lastEnd-first, string(out))

	// Leave point where the incremental version did: just after the last
	// replacement.
	end := first + len(out)
	buf.SetPoint(end)
	e.queryReplaceCursor = end
	e.queryReplaceMatch = -1
	if e.activeWin != nil {
		e.activeWin.EnsurePointVisible()
	}
	return count
}

// queryReplaceFinish ends query-replace mode.
func (e *Editor) queryReplaceFinish(msg string) {
	e.queryReplaceActive = false
	e.queryReplaceMatch = -1
	e.isearchClearCaches() // release the cached rune copy of the buffer
	e.Message("%s", strings.TrimSpace(msg))
}
