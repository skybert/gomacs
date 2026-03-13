package editor

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
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
	e.queryReplaceTo = to
	e.queryReplaceCursor = e.ActiveBuffer().Point()
	e.queryReplaceMatch = -1
	e.queryReplaceActive = true
	if !e.queryReplaceFindNext() {
		e.queryReplaceActive = false
		e.Message("No matches for %q", from)
	}
}

// queryReplaceFindNext locates the next occurrence of queryReplaceFrom
// starting at queryReplaceCursor.  Returns true if a match is found.
func (e *Editor) queryReplaceFindNext() bool {
	buf := e.ActiveBuffer()
	runes := []rune(buf.String())
	needle := []rune(e.queryReplaceFrom)
	pos := e.queryReplaceCursor
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
		ke.Key == tcell.KeyBackspace, ke.Key == tcell.KeyBackspace2,
		ke.Key == tcell.KeyDelete:
		// Skip this occurrence.
		e.queryReplaceCursor = e.queryReplaceMatch + 1
		if !e.queryReplaceFindNext() {
			e.queryReplaceFinish("No more matches")
		}

	case ke.Key == tcell.KeyRune && ke.Rune == '!':
		// Replace all remaining occurrences without asking.
		count := 0
		for e.queryReplaceMatch >= 0 {
			e.queryReplaceDoReplaceRaw()
			count++
			if !e.queryReplaceFindNext() {
				break
			}
		}
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
	needle := []rune(e.queryReplaceFrom)
	to := e.queryReplaceTo
	buf.Delete(ms, len(needle))
	buf.InsertString(ms, to)
	e.queryReplaceCursor = ms + len([]rune(to))
	e.queryReplaceMatch = -1
}

// queryReplaceFinish ends query-replace mode.
func (e *Editor) queryReplaceFinish(msg string) {
	e.queryReplaceActive = false
	e.queryReplaceMatch = -1
	e.Message("%s", strings.TrimSpace(msg))
}
