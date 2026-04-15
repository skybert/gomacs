package editor

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
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

// FaceSpellError is the face applied to misspelled words (red underline).
var FaceSpellError = syntax.Face{Underline: true, UnderlineColor: "red"}

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
	case "go", "python", "java", "bash", "elisp", "conf":
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

// blankHTMLTags returns a copy of runes with all HTML tag content (<...>)
// replaced by spaces, preserving all other positions so span offsets remain valid.
func blankHTMLTags(runes []rune) []rune {
	out := make([]rune, len(runes))
	copy(out, runes)
	inTag := false
	for i, r := range out {
		if r == '<' {
			inTag = true
		}
		if inTag {
			out[i] = ' '
		}
		if r == '>' {
			inTag = false
		}
	}
	return out
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
	if buf.ReadOnly() {
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
	spellLang := e.spellLanguage
	e.lspAsync(func() func() {
		spans := computeSpellSpansForMode(spellCmd, spellLang, text, mode)
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
	// Don't return stale spans: their positions are relative to old content
	// and would mark wrong characters as misspelled during undo/edit.
	return nil
}

// computeSpellSpansForMode runs aspell on text and returns spell-error spans.
// For modes in spellCheckAll, the whole text is checked.
// For modes in spellCheckComments, only comment-face spans are checked.
// This function is safe to call from a goroutine.
func computeSpellSpansForMode(spellCmd, spellLang, text, mode string) []syntax.Span {
	runes := []rune(text)
	n := len(runes)
	if spellCheckAll(mode) {
		check := runes
		if mode == "markdown" {
			check = blankHTMLTags(runes)
		}
		return findSpellSpans(spellCmd, spellLang, check, 0, n)
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
	virtSpans := findSpellSpans(spellCmd, spellLang, virtual, 0, len(virtual))
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
func findSpellSpans(aspellCmd, lang string, runes []rune, start, end int) []syntax.Span {
	if start >= end || aspellCmd == "" {
		return nil
	}
	text := string(runes[start:end])
	misspelled, err := runAspellList(aspellCmd, lang, text)
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
	i := sort.Search(len(spans), func(j int) bool { return spans[j].Start > pos })
	if i > 0 {
		sp := spans[i-1]
		if pos < sp.End {
			return true
		}
	}
	return false
}

// runAspellList runs cmd with the "list" subcommand and optional language flag,
// feeds text to stdin, and returns each output line as a misspelled word.
// Returns (nil, nil) if aspell is not found or the text has no misspellings.
func runAspellList(cmd, lang, text string) ([]string, error) {
	args := []string{"list"}
	if lang != "" {
		args = append(args, "--lang="+lang)
	}
	c := exec.Command(cmd, args...) //nolint:gosec
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

// cmdIspellWord checks the spelling of the word at point.
// If the word is misspelled the interactive repair loop is started for that
// single word; if it is correct a message is shown and nothing else happens.
func (e *Editor) cmdIspellWord() {
	e.clearArg()
	if e.spellCommand == "" {
		e.Message("spell: no spell checker configured; set (setq spell-command \"/usr/bin/aspell\") in ~/.gomacs")
		return
	}
	buf := e.ActiveBuffer()
	pt := buf.Point()
	runes := []rune(buf.String())
	n := len(runes)
	// Find word bounds around point.
	start := pt
	for start > 0 && isSpellWordRune(runes[start-1]) {
		start--
	}
	end := pt
	for end < n && isSpellWordRune(runes[end]) {
		end++
	}
	if start >= end {
		e.Message("No word at point")
		return
	}
	word := string(runes[start:end])
	misspelled, err := runAspellList(e.spellCommand, e.spellLanguage, word)
	if err != nil {
		e.Message("ispell-word: %v", err)
		return
	}
	if len(misspelled) == 0 {
		e.Message("%q is correctly spelled", word)
		return
	}
	e.spellErrors = []spellError{{start: start, end: end, word: word}}
	e.spellErrorIdx = 0
	e.spellActive = true
	e.spellShowCurrent()
}

// cmdSpell starts an interactive spell check of the entire current buffer.
func (e *Editor) cmdSpell() {
	e.clearArg()
	if e.spellCommand == "" {
		e.Message("spell: no spell checker configured; set (setq spell-command \"/usr/bin/aspell\") in ~/.gomacs")
		return
	}
	buf := e.ActiveBuffer()
	text := buf.String()
	misspelled, err := runAspellList(e.spellCommand, e.spellLanguage, text)
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

// aspellSuggest returns replacement suggestions for word using aspell pipe mode.
// Returns nil if aspell is unavailable or has no suggestions.
func aspellSuggest(cmd, lang, word string) []string {
	args := []string{"-a"}
	if lang != "" {
		args = append(args, "--lang="+lang)
	}
	c := exec.Command(cmd, args...) //nolint:gosec
	c.Stdin = strings.NewReader(word + "\n")
	out, err := c.Output()
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		// Format: "& word count offset: sug1, sug2, ..."
		if !strings.HasPrefix(line, "& ") {
			continue
		}
		colon := strings.Index(line, ": ")
		if colon < 0 {
			continue
		}
		parts := strings.Split(line[colon+2:], ", ")
		var result []string
		for _, p := range parts {
			if s := strings.TrimSpace(p); s != "" {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

// spellShowCurrent moves point to the current spell error and shows the prompt.
func (e *Editor) spellShowCurrent() {
	if e.spellErrorIdx >= len(e.spellErrors) {
		e.spellActive = false
		e.spellCurrentSugs = nil
		e.Message("Spell check done")
		return
	}
	se := e.spellErrors[e.spellErrorIdx]
	buf := e.ActiveBuffer()
	buf.SetPoint(se.start)
	e.activeWin.EnsurePointVisible()
	total := len(e.spellErrors)
	sugs := aspellSuggest(e.spellCommand, e.spellLanguage, se.word)
	if len(sugs) > 4 {
		sugs = sugs[:4]
	}
	e.spellCurrentSugs = sugs
	if len(sugs) > 0 {
		var sugParts []string
		for i, s := range sugs {
			sugParts = append(sugParts, fmt.Sprintf("%d=%s", i+1, s))
		}
		e.Message("Misspelled: %q  [%d/%d]  %s  SPC=skip  r=replace  i=add  q=quit",
			se.word, e.spellErrorIdx+1, total, strings.Join(sugParts, "  "))
	} else {
		e.Message("Misspelled: %q  [%d/%d]  SPC=skip  r=replace  i=add to dict  q=quit",
			se.word, e.spellErrorIdx+1, total)
	}
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

	case ke.Key == tcell.KeyRune && ke.Rune >= '1' && ke.Rune <= '9':
		// Select a suggestion by number.
		idx := int(ke.Rune - '1')
		if idx >= len(e.spellCurrentSugs) {
			e.spellShowCurrent()
			return
		}
		if e.spellErrorIdx >= len(e.spellErrors) {
			return
		}
		se := e.spellErrors[e.spellErrorIdx]
		replacement := e.spellCurrentSugs[idx]
		delta := len([]rune(replacement)) - (se.end - se.start)
		e.ActiveBuffer().ReplaceString(se.start, se.end-se.start, replacement)
		for i := e.spellErrorIdx + 1; i < len(e.spellErrors); i++ {
			e.spellErrors[i].start += delta
			e.spellErrors[i].end += delta
		}
		e.spellErrors = append(
			e.spellErrors[:e.spellErrorIdx],
			e.spellErrors[e.spellErrorIdx+1:]...,
		)
		e.spellActive = true
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

	case ke.Key == tcell.KeyRune && ke.Rune == 'i':
		// Add word to personal aspell dictionary and skip all occurrences.
		if e.spellErrorIdx >= len(e.spellErrors) {
			return
		}
		se := e.spellErrors[e.spellErrorIdx]
		word := se.word
		// Use aspell pipe mode: '*word' adds to personal dict, '#' saves it.
		// Pass --lang so the correct personal dictionary file is used.
		args := []string{"-a"}
		if e.spellLanguage != "" {
			args = append(args, "--lang="+e.spellLanguage)
		}
		c := exec.Command(e.spellCommand, args...) //nolint:gosec
		c.Stdin = strings.NewReader("*" + word + "\n#\n")
		_ = c.Run()
		// Remove all occurrences of this word from the error list.
		remaining := e.spellErrors[:0]
		for _, err := range e.spellErrors {
			if err.word != word {
				remaining = append(remaining, err)
			}
		}
		e.spellErrors = remaining
		if e.spellErrorIdx > len(e.spellErrors) {
			e.spellErrorIdx = len(e.spellErrors)
		}
		// Invalidate spell cache so the word is no longer underlined.
		delete(e.spellCaches, e.ActiveBuffer())
		// Report which dictionary file was updated.
		lang := e.spellLanguage
		if lang == "" {
			lang = "en"
		}
		home, _ := os.UserHomeDir()
		dictFile := home + "/.aspell." + lang + ".pws"
		e.Message("Added %q to %s", word, dictFile)
		e.spellShowCurrent()

	default:
		// Re-show the prompt for unrecognised keys.
		e.spellShowCurrent()
	}
}
