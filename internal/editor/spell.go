package editor

import (
	"fmt"
	"os/exec"
	"strings"
	"unicode"

	"github.com/gdamore/tcell/v3"
	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/syntax"
	"github.com/skybert/gomacs/internal/terminal"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// spellCache holds pre-computed spell-check spans for a buffer.
type spellCache struct {
	gen   int
	spans []syntax.Span
}

// commentMapping records where a comment span lives in the original buffer
// and in the virtual text built for aspell.
type commentMapping struct {
	origStart int
	virtStart int
	length    int
}

// spellError records a single misspelled word occurrence in the buffer.
type spellError struct {
	start int    // rune offset (inclusive)
	end   int    // rune offset (exclusive)
	word  string // the original word text
}

// FaceSpellError is the face applied to misspelled words (underline).
var FaceSpellError = syntax.Face{Underline: true}

// ---------------------------------------------------------------------------
// Mode predicates
// ---------------------------------------------------------------------------

// spellCheckAll returns true for modes where the whole buffer is spell-checked.
func spellCheckAll(mode string) bool {
	switch mode {
	case "markdown", "text", "fundamental", "":
		return true
	default:
		return false
	}
}

// spellCheckComments returns true for modes where only comments are spell-checked.
func spellCheckComments(mode string) bool {
	switch mode {
	case "go", "python", "java", "bash", "elisp":
		return true
	default:
		return false
	}
}

// isSpellWordRune reports whether r is a word character for spell-check
// purposes. Includes letters and apostrophes (for contractions like "don't").
func isSpellWordRune(r rune) bool {
	return unicode.IsLetter(r) || r == '\''
}

// ---------------------------------------------------------------------------
// Async span computation
// ---------------------------------------------------------------------------

// getSpellSpans returns the cached spell-error spans for buf.
// If the buffer content has changed since the last computation, a background
// aspell process is started and the previous (possibly stale) spans are
// returned until the new result is available.
// Returns nil if spell checking is not configured or not applicable to the mode.
func (e *Editor) getSpellSpans(buf *buffer.Buffer) []syntax.Span {
	if e.spellCommand == "" {
		return nil
	}
	mode := buf.Mode()
	if !spellCheckAll(mode) && !spellCheckComments(mode) {
		return nil
	}
	if e.spellCaches == nil {
		e.spellCaches = make(map[*buffer.Buffer]*spellCache)
	}
	if e.spellPending == nil {
		e.spellPending = make(map[*buffer.Buffer]int)
	}
	gen := buf.ChangeGen()
	c := e.spellCaches[buf]
	if c != nil && c.gen == gen {
		return c.spans
	}
	// Don't start a new check if one is already in-flight for this generation.
	if e.spellPending[buf] == gen {
		if c != nil {
			return c.spans
		}
		return nil
	}
	// Start an async spell check; capture only data (no buf reference in goroutine).
	e.spellPending[buf] = gen
	text := buf.String()
	spellCmd := e.spellCommand
	e.lspAsync(func() func() {
		spans := computeSpellSpansForMode(spellCmd, text, mode)
		return func() {
			if e.spellCaches == nil {
				e.spellCaches = make(map[*buffer.Buffer]*spellCache)
			}
			// Only store if the buffer content matches what we checked.
			if buf.ChangeGen() == gen {
				e.spellCaches[buf] = &spellCache{gen: gen, spans: spans}
			}
			delete(e.spellPending, buf)
		}
	})
	// Return stale spans (or nil) while the new check runs.
	if c != nil {
		return c.spans
	}
	return nil
}

// computeSpellSpansForMode runs aspell on text and returns spell-error spans.
// For modes in spellCheckAll, the whole text is checked.
// For modes in spellCheckComments, only comment-face spans are checked.
// This function is safe to call from a goroutine.
func computeSpellSpansForMode(spellCmd, text, mode string) []syntax.Span {
	runes := []rune(text)
	n := len(runes)
	if spellCheckAll(mode) {
		return findSpellSpans(spellCmd, runes, 0, n)
	}
	// Comment-only: extract comment spans via the language highlighter.
	hl := syntax.LangToHighlighter(mode)
	if hl == nil {
		return nil
	}
	syntaxSpans := hl.Highlight(text, 0, n)
	var mapping []commentMapping
	var virtual []rune
	for _, sp := range syntaxSpans {
		if sp.Face != syntax.FaceComment {
			continue
		}
		length := sp.End - sp.Start
		if length <= 0 {
			continue
		}
		mapping = append(mapping, commentMapping{
			origStart: sp.Start,
			virtStart: len(virtual),
			length:    length,
		})
		virtual = append(virtual, runes[sp.Start:sp.End]...)
		virtual = append(virtual, '\n')
	}
	if len(virtual) == 0 {
		return nil
	}
	virtSpans := findSpellSpans(spellCmd, virtual, 0, len(virtual))
	if len(virtSpans) == 0 {
		return nil
	}
	// Remap virtual positions to original buffer positions.
	var out []syntax.Span
	for _, vs := range virtSpans {
		os1 := virtToOrigPos(mapping, vs.Start)
		if os1 < 0 {
			continue
		}
		os2 := virtToOrigPos(mapping, vs.End-1)
		if os2 < 0 {
			continue
		}
		out = append(out, syntax.Span{Start: os1, End: os2 + 1, Face: vs.Face})
	}
	return out
}

// virtToOrigPos maps a virtual rune index to the original buffer position.
// Returns -1 if virtPos falls on a virtual separator (newline) or is out of range.
func virtToOrigPos(mapping []commentMapping, virtPos int) int {
	for i := len(mapping) - 1; i >= 0; i-- {
		m := mapping[i]
		if virtPos >= m.virtStart && virtPos < m.virtStart+m.length {
			return m.origStart + (virtPos - m.virtStart)
		}
	}
	return -1
}

// findSpellSpans runs aspell list on runes[start:end] and returns a span for
// every misspelled word occurrence found in that range.
// Positions in the returned spans are absolute (relative to runes[0]).
func findSpellSpans(aspellCmd string, runes []rune, start, end int) []syntax.Span {
	if start >= end || aspellCmd == "" {
		return nil
	}
	text := string(runes[start:end])
	misspelled, err := runAspellList(aspellCmd, text)
	if err != nil || len(misspelled) == 0 {
		return nil
	}
	wordSet := make(map[string]bool, len(misspelled))
	for _, w := range misspelled {
		wordSet[strings.ToLower(w)] = true
	}
	var spans []syntax.Span
	i := start
	for i < end {
		if !isSpellWordRune(runes[i]) {
			i++
			continue
		}
		j := i
		for j < end && isSpellWordRune(runes[j]) {
			j++
		}
		word := strings.ToLower(string(runes[i:j]))
		if wordSet[word] {
			spans = append(spans, syntax.Span{Start: i, End: j, Face: FaceSpellError})
		}
		i = j
	}
	return spans
}

// isSpellErrorAt reports whether pos is covered by any spell-error span.
// Assumes spans are sorted by Start.
func isSpellErrorAt(spans []syntax.Span, pos int) bool {
	for _, sp := range spans {
		if sp.Start > pos {
			break
		}
		if pos < sp.End {
			return true
		}
	}
	return false
}

// runAspellList runs cmd with the "list" subcommand, feeds text to stdin, and
// returns each output line as a misspelled word.
// Returns (nil, nil) if aspell is not found or the text has no misspellings.
func runAspellList(cmd, text string) ([]string, error) {
	c := exec.Command(cmd, "list") //nolint:gosec
	c.Stdin = strings.NewReader(text)
	out, err := c.Output()
	if err != nil {
		return nil, err
	}
	var words []string
	for _, line := range strings.Split(string(out), "\n") {
		if w := strings.TrimSpace(line); w != "" {
			words = append(words, w)
		}
	}
	return words, nil
}

// ---------------------------------------------------------------------------
// Interactive spell check (M-x spell)
// ---------------------------------------------------------------------------

// cmdSpell starts an interactive spell check of the entire current buffer.
func (e *Editor) cmdSpell() {
	e.clearArg()
	if e.spellCommand == "" {
		e.Message("spell: no spell checker configured; set (setq spell-command \"/usr/bin/aspell\") in ~/.gomacs")
		return
	}
	buf := e.ActiveBuffer()
	text := buf.String()
	misspelled, err := runAspellList(e.spellCommand, text)
	if err != nil {
		e.Message("spell: %v", err)
		return
	}
	if len(misspelled) == 0 {
		e.Message("No misspellings found")
		return
	}
	runes := []rune(text)
	n := len(runes)
	wordSet := make(map[string]bool, len(misspelled))
	for _, w := range misspelled {
		wordSet[strings.ToLower(w)] = true
	}
	// Collect all misspelled occurrences in buffer order.
	var errors []spellError
	i := 0
	for i < n {
		if !isSpellWordRune(runes[i]) {
			i++
			continue
		}
		j := i
		for j < n && isSpellWordRune(runes[j]) {
			j++
		}
		word := string(runes[i:j])
		if wordSet[strings.ToLower(word)] {
			errors = append(errors, spellError{start: i, end: j, word: word})
		}
		i = j
	}
	if len(errors) == 0 {
		e.Message("No misspellings found")
		return
	}
	e.spellErrors = errors
	e.spellErrorIdx = 0
	e.spellActive = true
	e.spellShowCurrent()
}

// spellShowCurrent moves point to the current spell error and shows the prompt.
func (e *Editor) spellShowCurrent() {
	if e.spellErrorIdx >= len(e.spellErrors) {
		e.spellActive = false
		e.Message("Spell check done")
		return
	}
	se := e.spellErrors[e.spellErrorIdx]
	buf := e.ActiveBuffer()
	buf.SetPoint(se.start)
	e.activeWin.EnsurePointVisible()
	total := len(e.spellErrors)
	e.Message("Misspelled: %q  [%d/%d]  SPC=skip  r=replace  q=quit",
		se.word, e.spellErrorIdx+1, total)
}

// spellHandleKey processes a key press while the interactive spell check is active.
//
//nolint:exhaustive
func (e *Editor) spellHandleKey(ke terminal.KeyEvent) {
	switch {
	case ke.Key == tcell.KeyCtrlG,
		ke.Key == tcell.KeyRune && ke.Rune == 'q':
		e.spellActive = false
		e.Message("Spell check cancelled")

	case ke.Key == tcell.KeyRune && (ke.Rune == ' ' || ke.Rune == 'n'):
		e.spellErrorIdx++
		e.spellShowCurrent()

	case ke.Key == tcell.KeyRune && ke.Rune == 'r':
		if e.spellErrorIdx >= len(e.spellErrors) {
			return
		}
		se := e.spellErrors[e.spellErrorIdx]
		e.spellActive = false
		e.ReadMinibuffer(fmt.Sprintf("Replace %q with: ", se.word), func(replacement string) {
			if replacement == "" {
				e.spellActive = true
				e.spellShowCurrent()
				return
			}
			buf := e.ActiveBuffer()
			delta := len([]rune(replacement)) - (se.end - se.start)
			buf.ReplaceString(se.start, se.end-se.start, replacement)
			// Shift positions of subsequent errors.
			for i := e.spellErrorIdx + 1; i < len(e.spellErrors); i++ {
				e.spellErrors[i].start += delta
				e.spellErrors[i].end += delta
			}
			// Remove the now-replaced error.
			e.spellErrors = append(
				e.spellErrors[:e.spellErrorIdx],
				e.spellErrors[e.spellErrorIdx+1:]...,
			)
			e.spellActive = true
			e.spellShowCurrent()
		})

	default:
		// Re-show the prompt for unrecognised keys.
		e.spellShowCurrent()
	}
}
