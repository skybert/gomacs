package editor

import (
	"sort"
	"strings"
	"unicode"

	"github.com/skybert/gomacs/internal/buffer"
)

// indentCurrentLine re-indents the line containing buf.Point() according to
// the buffer's major mode.  Point is moved past the new indentation.
// The operation is idempotent: calling it again on an already-correct line
// is a no-op (point stays at the first non-whitespace character).
// unit is the per-level indentation string (e.g. "\t" for Go, "  " for Python).
//
// Only the lines the mode's engine actually needs are read from the buffer, and
// the accumulated block depth comes from a per-buffer checkpoint cache, so
// pressing Enter deep inside a large file neither copies the whole buffer nor
// rescans it from line 1.
func indentCurrentLine(buf *buffer.Buffer, unit string) {
	if indentNoOpMode(buf.Mode()) {
		// The mode has no indentation logic at all: leave both the line and
		// point exactly as they are.
		return
	}
	bol := buf.BeginningOfLine(buf.Point())
	applyIndentAt(buf, bol, calcIndentAt(buf, buf.Mode(), bol, unit))
}

// modeConf is the major-mode name of conf-mode.
const modeConf = "conf"

// indentNoOpMode reports whether the major mode deliberately has no
// indentation logic.  conf-mode is syntax highlighting only: configuration
// files carry no block structure to derive an indentation from, and the
// copy-the-previous-line fallback is actively harmful there — it both invents
// indentation the user never typed and strips indentation they did.
func indentNoOpMode(mode string) bool {
	switch mode {
	case modeConf:
		return true
	default:
		return false
	}
}

// calcIndentAt returns the desired indentation string for the line starting at
// buffer position bol.  It is the buffer-backed counterpart of calcIndent:
// identical results, but it reads only the lines it needs instead of splitting
// the whole buffer into a []string.
// unit is the per-level indent string used for Python, Bash, and braced
// languages (overrides the built-in defaults for those modes).
func calcIndentAt(buf *buffer.Buffer, mode string, bol int, unit string) string {
	switch mode {
	case "go", "java":
		depth := blockDepthAt(buf, bol, mode, netBraceDeltaSlash)
		return bracedIndentFor(depth, lineTextAt(buf, bol), unit)
	case "perl":
		depth := blockDepthAt(buf, bol, mode, netBraceDeltaHash)
		return bracedIndentFor(depth, lineTextAt(buf, bol), unit)
	case "bash":
		depth := blockDepthAt(buf, bol, mode, bashNetIndentRunes)
		return bashIndentFor(depth, lineTextAt(buf, bol), unit)
	case "json":
		depth := blockDepthAt(buf, bol, mode, netBraceCountJSONRunes)
		return jsonIndentFor(depth, lineTextAt(buf, bol), unit)
	case "python":
		return calcIndentPythonAt(buf, bol, unit)
	case modeConf:
		// No indentation logic (see indentNoOpMode): the line keeps whatever
		// indentation it already has, so applyIndentAt is a no-op.
		return leadingWSStr(lineTextAt(buf, bol))
	default:
		// markdown, fundamental, unknown: copy previous line's indentation
		return calcIndentCopyAt(buf, bol)
	}
}

// calcIndent returns the desired indentation string for the given line.
// unit is the per-level indent string used for Python, Bash, and braced
// languages (overrides the built-in defaults for those modes).
// This is the []string entry point; the editor itself goes through
// calcIndentAt, which does not need the buffer split into lines.
func calcIndent(mode string, lines []string, lineIdx int, unit string) string {
	switch mode {
	case "go":
		return calcIndentBraced(lines, lineIdx, unit, "//")
	case "java":
		return calcIndentBraced(lines, lineIdx, unit, "//")
	case "perl":
		return calcIndentBraced(lines, lineIdx, unit, "#")
	case "python":
		return calcIndentPython(lines, lineIdx, unit)
	case "bash":
		return calcIndentBash(lines, lineIdx, unit)
	case "json":
		return calcIndentJSON(lines, lineIdx, unit)
	case modeConf:
		// No indentation logic (see indentNoOpMode): keep the line as typed.
		return leadingWSStr(lines[lineIdx])
	default:
		// markdown, fundamental, unknown: copy previous line's indentation
		return calcIndentCopy(lines, lineIdx)
	}
}

// applyIndentAt replaces the leading whitespace of the line starting at bol
// with `desired` and moves point past the indentation.  The buffer is only
// touched when the indentation actually differs, which keeps the operation
// idempotent and the buffer's change generation stable.
func applyIndentAt(buf *buffer.Buffer, bol int, desired string) {
	n := buf.Len()
	wsEnd := bol
	for wsEnd < n {
		if r := buf.RuneAt(wsEnd); r != ' ' && r != '\t' {
			break
		}
		wsEnd++
	}

	if existing := buf.Substring(bol, wsEnd); existing != desired {
		buf.Delete(bol, wsEnd-bol)
		if desired != "" {
			buf.InsertString(bol, desired)
		}
	}

	// Move point to first non-whitespace (or end of indent if line is blank).
	buf.SetPoint(bol + len([]rune(desired)))
}

// applyIndent replaces the leading whitespace of the given line with `desired`
// and moves point past the indentation.  This is the []string entry point; the
// editor itself goes through applyIndentAt.
func applyIndent(buf *buffer.Buffer, lines []string, lineIdx int, desired string) {
	// Compute BOL as a logical position.
	bol := 0
	for i := range lineIdx {
		bol += len([]rune(lines[i])) + 1 // +1 for '\n'
	}
	applyIndentAt(buf, bol, desired)
}

// ---- block-depth checkpoint cache ------------------------------------------

// indentCheckpointStride is the number of lines between two cached block-depth
// checkpoints.  64 keeps the cache down to one entry per 64 lines while
// bounding the forward rescan after a cache hit to at most 63 lines.
const indentCheckpointStride = 64

// indentCacheMaxBuffers bounds how many buffers hold indent checkpoints at
// once; the map is dropped wholesale when the limit is exceeded.  Only the
// buffer currently being edited benefits from the cache, so a small bound is
// plenty.
const indentCacheMaxBuffers = 8

// lineDelta returns the net block-nesting change contributed by one line.
// The rune-slice form lets the buffer-backed scan reuse a single line buffer.
type lineDelta func(runes []rune) int

// netBraceDeltaSlash and netBraceDeltaHash are the two lineDelta flavours of
// netBraceCountRunes, one per single-line comment syntax.
func netBraceDeltaSlash(runes []rune) int { return netBraceCountRunes(runes, "//") }
func netBraceDeltaHash(runes []rune) int  { return netBraceCountRunes(runes, "#") }

// indentCache holds what the indent engines would otherwise recompute from
// line 1 of the buffer on every keystroke.  Like the syntax span cache
// (getSpanCache) it is keyed on the buffer's ChangeGen(), so any edit starts a
// fresh set of checkpoints; within one generation repeated re-indentation only
// rescans from the nearest checkpoint.  ModCount() is part of the key as well:
// ChangeGen() alone repeats itself when an undo is followed by a different
// edit, whereas ModCount() only ever grows.
//
// Both checkpoint lists start with the zero-state entry at position 0 and are
// kept in ascending position order.  Checkpoint positions are always line
// starts.
type indentCache struct {
	gen int
	mod int

	// Block depth per checkpoint, for the mode the depths were counted with.
	depthMode string
	depthPos  []int
	depths    []int

	// Elisp paren-column stack per checkpoint.
	elispPos   []int
	elispStack [][]int
	elispInStr []bool

	line  []rune // reusable line buffer, keeps the line scans allocation-free
	stack []int  // reusable Elisp paren-column stack
}

// indentCaches holds the indent caches of the handful of buffers most recently
// re-indented.  All editing happens on the single event-loop goroutine, so no
// locking is needed.
var indentCaches = map[*buffer.Buffer]*indentCache{}

// indentCacheFor returns buf's indent cache, resetting it when the buffer has
// changed since its checkpoints were taken.
func indentCacheFor(buf *buffer.Buffer) *indentCache {
	c := indentCaches[buf]
	if c != nil && c.gen == buf.ChangeGen() && c.mod == buf.ModCount() {
		return c
	}
	if c == nil {
		if len(indentCaches) >= indentCacheMaxBuffers {
			indentCaches = make(map[*buffer.Buffer]*indentCache, indentCacheMaxBuffers)
		}
		c = &indentCache{}
		indentCaches[buf] = c
	}
	c.gen = buf.ChangeGen()
	c.mod = buf.ModCount()
	c.resetDepth("")
	c.elispPos = append(c.elispPos[:0], 0)
	c.elispStack = append(c.elispStack[:0], nil)
	c.elispInStr = append(c.elispInStr[:0], false)
	return c
}

// resetDepth drops the block-depth checkpoints and rebases them on mode.
func (c *indentCache) resetDepth(mode string) {
	c.depthMode = mode
	c.depthPos = append(c.depthPos[:0], 0)
	c.depths = append(c.depths[:0], 0)
}

// blockDepthAt returns the accumulated block depth of every line before the
// line starting at bol, as counted by delta.  The scan starts at the nearest
// cached checkpoint at or before bol instead of at line 1, and extends the
// checkpoint list while it walks past the end of it.  mode names the counter
// delta implements: checkpoints recorded for another mode are dropped, since
// the modes count different things.
func blockDepthAt(buf *buffer.Buffer, bol int, mode string, delta lineDelta) int {
	c := indentCacheFor(buf)
	if c.depthMode != mode {
		// Depths counted for another mode do not carry over.
		c.resetDepth(mode)
	}

	// Nearest checkpoint at or before bol; index 0 (position 0) always
	// qualifies.
	i := sort.SearchInts(c.depthPos, bol+1) - 1
	extend := i == len(c.depthPos)-1

	pos := c.depthPos[i]
	depth := c.depths[i]
	for lines := 0; pos < bol; lines++ {
		var eol int
		c.line, eol = readLineRunes(buf, pos, c.line)
		depth += delta(c.line)
		pos = eol + 1
		if extend && (lines+1)%indentCheckpointStride == 0 && pos <= bol {
			c.depthPos = append(c.depthPos, pos)
			c.depths = append(c.depths, depth)
		}
	}
	return depth
}

// readLineRunes copies the runes of the line starting at pos into dst, reusing
// its backing array, and returns the filled slice together with the position of
// the line's terminating newline (or Len() on the last line).  It is a single
// pass over the line: EndOfLine plus a copy would read every rune twice.
func readLineRunes(buf *buffer.Buffer, pos int, dst []rune) ([]rune, int) {
	dst = dst[:0]
	n := buf.Len()
	for ; pos < n; pos++ {
		r := buf.RuneAt(pos)
		if r == '\n' {
			break
		}
		dst = append(dst, r)
	}
	return dst, pos
}

// lineTextAt returns the text of the line starting at bol, newline excluded.
func lineTextAt(buf *buffer.Buffer, bol int) string {
	return buf.Substring(bol, buf.EndOfLine(bol))
}

// prevLineText returns the text of the nearest non-blank line before the line
// starting at bol, and whether such a line was found.  Like the []string
// engines it stops at the first line of the buffer, returning that line's text
// with found == false when every preceding line is blank.  bol must be > 0.
func prevLineText(buf *buffer.Buffer, bol int) (text string, found bool) {
	for pos := bol; pos > 0; {
		start := buf.BeginningOfLine(pos - 1)
		text = buf.Substring(start, pos-1) // pos-1 is the '\n' before pos
		if strings.TrimSpace(text) != "" {
			return text, true
		}
		pos = start
	}
	return text, false
}

// ---- brace-counting indentation (Go, Java) ---------------------------------

// calcIndentBraced computes indentation for C-family brace languages.
// unit is the per-level string ("\t" for Go, "    " for Java).
// commentPrefix is the single-line comment prefix ("//").
func calcIndentBraced(lines []string, lineIdx int, unit, commentPrefix string) string {
	depth := 0
	for i := range lineIdx {
		depth += netBraceCount(lines[i], commentPrefix)
	}
	return bracedIndentFor(depth, lines[lineIdx], unit)
}

// bracedIndentFor turns an accumulated brace depth into the indentation of the
// line being indented, whose text is cur.
func bracedIndentFor(depth int, cur, unit string) string {
	depth = max(depth, 0)

	// A line that opens with } or ) dedents by one.
	trimmed := strings.TrimLeft(cur, " \t")
	if strings.HasPrefix(trimmed, "}") || strings.HasPrefix(trimmed, ")") {
		depth = max(depth-1, 0)
	}

	// Go: case / default: stay at one level but don't add extra.
	// (they're already inside a { block, so depth accounts for it.)

	return strings.Repeat(unit, depth)
}

// netBraceCount returns net { - } count for a line, ignoring string and
// comment contents.
func netBraceCount(line, commentPrefix string) int {
	return netBraceCountRunes([]rune(line), commentPrefix)
}

// netBraceCountRunes is netBraceCount over a rune slice, so the buffer-backed
// scan can reuse one line buffer instead of allocating per line.
func netBraceCountRunes(runes []rune, commentPrefix string) int {
	n := len(runes)
	net := 0
	inString := false
	inChar := false
	inBacktick := false

	for i := 0; i < n; i++ {
		r := runes[i]

		if inString {
			switch r {
			case '\\':
				i++ // skip escaped
			case '"':
				inString = false
			}
			continue
		}
		if inChar {
			switch r {
			case '\\':
				i++
			case '\'':
				inChar = false
			}
			continue
		}
		if inBacktick {
			if r == '`' {
				inBacktick = false
			}
			continue
		}

		// Check for single-line comment.
		if hasPrefixAt(runes, i, commentPrefix) {
			break
		}

		switch r {
		case '"':
			inString = true
		case '\'':
			inChar = true
		case '`':
			inBacktick = true
		case '{', '(':
			net++
		case '}', ')':
			net--
		}
	}
	return net
}

// ---- Python indentation ----------------------------------------------------

var pythonDedentKeywords = []string{"else:", "elif ", "except", "finally:", "except:"}

func calcIndentPython(lines []string, lineIdx int, unit string) string {
	if lineIdx == 0 {
		return ""
	}

	// Find the nearest previous non-blank line.
	prevIdx := lineIdx - 1
	for prevIdx > 0 && strings.TrimSpace(lines[prevIdx]) == "" {
		prevIdx--
	}

	return pythonIndentFor(lines[prevIdx], lines[lineIdx], unit)
}

// calcIndentPythonAt is the buffer-backed calcIndentPython: it only needs the
// previous non-blank line and the line being indented.
func calcIndentPythonAt(buf *buffer.Buffer, bol int, unit string) string {
	if bol == 0 {
		return ""
	}
	prev, _ := prevLineText(buf, bol)
	return pythonIndentFor(prev, lineTextAt(buf, bol), unit)
}

// pythonIndentFor derives the indentation of line cur from prev, the nearest
// non-blank line above it.
func pythonIndentFor(prev, cur, unit string) string {
	indent := leadingWSStr(prev)
	prevTrimmed := strings.TrimSpace(prev)

	// If the previous non-blank line ends with ':', add one level.
	if strings.HasSuffix(prevTrimmed, ":") && !strings.HasPrefix(prevTrimmed, "#") {
		indent += unit
	}

	// If the current line starts a dedent keyword, strip one level.
	curTrimmed := strings.TrimSpace(cur)
	for _, kw := range pythonDedentKeywords {
		if strings.HasPrefix(curTrimmed, kw) {
			if len(indent) >= len(unit) {
				indent = indent[len(unit):]
			} else {
				indent = ""
			}
			break
		}
	}

	return indent
}

// ---- Bash indentation -------------------------------------------------------

// bashOpeners open one indentation level when they end a line; bashClosers
// close one when they start a line.
var (
	bashOpeners = []string{"then", "do", "{"}
	bashClosers = []string{"fi", "done", "esac", "}"}
)

func calcIndentBash(lines []string, lineIdx int, unit string) string {
	depth := 0
	for i := range lineIdx {
		depth += bashNetIndent(lines[i])
	}
	return bashIndentFor(depth, lines[lineIdx], unit)
}

// bashIndentFor turns an accumulated block depth into the indentation of the
// Bash line being indented, whose text is cur.
func bashIndentFor(depth int, cur, unit string) string {
	depth = max(depth, 0)

	// else/elif/fi/done/esac/} on the current line dedents by 1.
	trimmed := strings.TrimSpace(cur)
	if trimmed == "else" || trimmed == "fi" || trimmed == "done" || trimmed == "esac" ||
		strings.HasPrefix(trimmed, "elif ") || strings.HasPrefix(trimmed, "else ") ||
		trimmed == "}" || strings.HasPrefix(trimmed, "} ") {
		depth = max(depth-1, 0)
	}

	return strings.Repeat(unit, depth)
}

func bashNetIndent(line string) int {
	return bashNetIndentRunes([]rune(line))
}

// bashNetIndentRunes is bashNetIndent over a rune slice; it neither allocates
// nor builds the trimmed line as a string.
func bashNetIndentRunes(runes []rune) int {
	trimmed := trimSpaceRunes(runes)
	if hasPrefixAt(trimmed, 0, "#") {
		return 0
	}
	net := 0
	// Openers.
	for _, kw := range bashOpeners {
		if runesEqual(trimmed, kw) || endsWithWord(trimmed, kw) {
			net++
		}
	}
	// Closers.
	for _, kw := range bashClosers {
		if runesEqual(trimmed, kw) || startsWithWord(trimmed, kw) {
			net--
		}
	}
	return net
}

// ---- Copy-indent (Markdown, Fundamental) ------------------------------------

func calcIndentCopy(lines []string, lineIdx int) string {
	for i := lineIdx - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return leadingWSStr(lines[i])
		}
	}
	return ""
}

// calcIndentCopyAt is the buffer-backed calcIndentCopy: it walks back only as
// far as the nearest non-blank line.
func calcIndentCopyAt(buf *buffer.Buffer, bol int) string {
	if bol == 0 {
		return ""
	}
	prev, found := prevLineText(buf, bol)
	if !found {
		return ""
	}
	return leadingWSStr(prev)
}

// leadingWSStr returns the leading whitespace of a string.
func leadingWSStr(s string) string {
	runes := []rune(s)
	i := 0
	for i < len(runes) && (runes[i] == ' ' || runes[i] == '\t') {
		i++
	}
	return string(runes[:i])
}

// ---- rune-slice helpers -----------------------------------------------------

// hasPrefixAt reports whether runes[i:] starts with the ASCII string s.
// An empty s never matches, mirroring the `commentPrefix != ""` guard of the
// string-based scanners.
func hasPrefixAt(runes []rune, i int, s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if i >= len(runes) || runes[i] != c {
			return false
		}
		i++
	}
	return true
}

// runesEqual reports whether runes holds exactly the ASCII string s.
func runesEqual(runes []rune, s string) bool {
	return len(runes) == len(s) && hasPrefixAt(runes, 0, s)
}

// endsWithWord reports whether runes ends with s preceded by a space or tab,
// i.e. the " "+s / "\t"+s suffix tests without the concatenation.
func endsWithWord(runes []rune, s string) bool {
	start := len(runes) - len(s)
	if start <= 0 {
		return false
	}
	if c := runes[start-1]; c != ' ' && c != '\t' {
		return false
	}
	return hasPrefixAt(runes, start, s)
}

// startsWithWord reports whether runes starts with s followed by a space,
// i.e. the s+" " prefix test without the concatenation.
func startsWithWord(runes []rune, s string) bool {
	return len(runes) > len(s) && runes[len(s)] == ' ' && hasPrefixAt(runes, 0, s)
}

// trimSpaceRunes returns the sub-slice of runes without leading and trailing
// whitespace, matching strings.TrimSpace without allocating.
func trimSpaceRunes(runes []rune) []rune {
	start := 0
	for start < len(runes) && unicode.IsSpace(runes[start]) {
		start++
	}
	end := len(runes)
	for end > start && unicode.IsSpace(runes[end-1]) {
		end--
	}
	return runes[start:end]
}

// ---- JSON indentation -------------------------------------------------------

// netBraceCountJSON returns net opener - closer count for a JSON line,
// counting both { } and [ ] openers/closers, ignoring string contents.
func netBraceCountJSON(line string) int {
	return netBraceCountJSONRunes([]rune(line))
}

// netBraceCountJSONRunes is netBraceCountJSON over a rune slice, so the
// buffer-backed scan can reuse one line buffer.
func netBraceCountJSONRunes(runes []rune) int {
	n := len(runes)
	net := 0
	inString := false

	for i := 0; i < n; i++ {
		r := runes[i]
		if inString {
			switch r {
			case '\\':
				i++ // skip escaped character
			case '"':
				inString = false
			}
			continue
		}
		switch r {
		case '"':
			inString = true
		case '{', '[':
			net++
		case '}', ']':
			net--
		}
	}
	return net
}

// calcIndentJSON computes indentation for JSON using { } and [ ] as
// openers/closers.
func calcIndentJSON(lines []string, lineIdx int, unit string) string {
	depth := 0
	for i := range lineIdx {
		depth += netBraceCountJSON(lines[i])
	}
	return jsonIndentFor(depth, lines[lineIdx], unit)
}

// jsonIndentFor turns an accumulated depth into the indentation of the JSON
// line being indented, whose text is cur.
func jsonIndentFor(depth int, cur, unit string) string {
	depth = max(depth, 0)

	// A line opening with } or ] dedents by one.
	trimmed := strings.TrimLeft(cur, " \t")
	if strings.HasPrefix(trimmed, "}") || strings.HasPrefix(trimmed, "]") {
		depth = max(depth-1, 0)
	}

	return strings.Repeat(unit, depth)
}
