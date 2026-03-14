package editor

import (
	"strings"

	"github.com/skybert/gomacs/internal/buffer"
)

// indentCurrentLine re-indents the line containing buf.Point() according to
// the buffer's major mode.  Point is moved past the new indentation.
// The operation is idempotent: calling it again on an already-correct line
// is a no-op (point stays at the first non-whitespace character).
// unit is the per-level indentation string (e.g. "\t" for Go, "  " for Python).
func indentCurrentLine(buf *buffer.Buffer, unit string) {
	text := buf.String()
	lines := strings.Split(text, "\n")
	pt := buf.Point()

	// Find which line index contains pt.
	lineIdx := 0
	pos := 0
	for i, l := range lines {
		end := pos + len([]rune(l))
		if pt <= end {
			lineIdx = i
			break
		}
		pos = end + 1 // +1 for the '\n'
		lineIdx = i + 1
	}
	if lineIdx >= len(lines) {
		lineIdx = len(lines) - 1
	}

	desired := calcIndent(buf.Mode(), lines, lineIdx, unit)
	applyIndent(buf, lines, lineIdx, desired)
}

// calcIndent returns the desired indentation string for the given line.
// unit is the per-level indent string used for Python, Bash, and braced
// languages (overrides the built-in defaults for those modes).
func calcIndent(mode string, lines []string, lineIdx int, unit string) string {
	switch mode {
	case "go":
		return calcIndentBraced(lines, lineIdx, unit, "//")
	case "java":
		return calcIndentBraced(lines, lineIdx, unit, "//")
	case "python":
		return calcIndentPython(lines, lineIdx, unit)
	case "bash":
		return calcIndentBash(lines, lineIdx, unit)
	case "json":
		return calcIndentJSON(lines, lineIdx, unit)
	default:
		// markdown, fundamental, unknown: copy previous line's indentation
		return calcIndentCopy(lines, lineIdx)
	}
}

// applyIndent replaces the leading whitespace of the given line with `desired`
// and moves point past the indentation.
func applyIndent(buf *buffer.Buffer, lines []string, lineIdx int, desired string) {
	// Compute BOL as a logical position.
	bol := 0
	for i := range lineIdx {
		bol += len([]rune(lines[i])) + 1 // +1 for '\n'
	}

	line := lines[lineIdx]
	runes := []rune(line)

	// Count existing leading whitespace.
	wsEnd := 0
	for wsEnd < len(runes) && (runes[wsEnd] == ' ' || runes[wsEnd] == '\t') {
		wsEnd++
	}
	existing := string(runes[:wsEnd])

	if existing != desired {
		buf.Delete(bol, len([]rune(existing)))
		if desired != "" {
			buf.InsertString(bol, desired)
		}
	}

	// Move point to first non-whitespace (or end of indent if line is blank).
	newIndentLen := len([]rune(desired))
	lineLen := len([]rune(lines[lineIdx]))
	// After the edit the line length may have changed; use desired + rest.
	firstContent := bol + newIndentLen
	if firstContent > bol+lineLen {
		firstContent = bol + newIndentLen
	}
	buf.SetPoint(firstContent)
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
	if depth < 0 {
		depth = 0
	}

	cur := strings.TrimLeft(lines[lineIdx], " \t")

	// A line that opens with } or ) dedents by one.
	if strings.HasPrefix(cur, "}") || strings.HasPrefix(cur, ")") {
		depth--
		if depth < 0 {
			depth = 0
		}
	}

	// Go: case / default: stay at one level but don't add extra.
	// (they're already inside a { block, so depth accounts for it.)

	return strings.Repeat(unit, depth)
}

// netBraceCount returns net { - } count for a line, ignoring string and
// comment contents.
func netBraceCount(line, commentPrefix string) int {
	runes := []rune(line)
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
		if commentPrefix != "" && strings.HasPrefix(string(runes[i:]), commentPrefix) {
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
	prev := lines[prevIdx]
	prevIndent := leadingWSStr(prev)
	prevTrimmed := strings.TrimSpace(prev)

	indent := prevIndent

	// If the previous non-blank line ends with ':', add one level.
	if strings.HasSuffix(prevTrimmed, ":") && !strings.HasPrefix(prevTrimmed, "#") {
		indent += unit
	}

	// If the current line starts a dedent keyword, strip one level.
	curTrimmed := strings.TrimSpace(lines[lineIdx])
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

func calcIndentBash(lines []string, lineIdx int, unit string) string {
	depth := 0
	for i := range lineIdx {
		depth += bashNetIndent(lines[i])
	}
	if depth < 0 {
		depth = 0
	}

	// else/elif/fi/done/esac/} on the current line dedents by 1.
	cur := strings.TrimSpace(lines[lineIdx])
	if cur == "else" || cur == "fi" || cur == "done" || cur == "esac" ||
		strings.HasPrefix(cur, "elif ") || strings.HasPrefix(cur, "else ") ||
		cur == "}" || strings.HasPrefix(cur, "} ") {
		depth--
		if depth < 0 {
			depth = 0
		}
	}

	return strings.Repeat(unit, depth)
}

func bashNetIndent(line string) int {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "#") {
		return 0
	}
	net := 0
	// Openers.
	for _, kw := range []string{"then", "do", "{"} {
		if trimmed == kw || strings.HasSuffix(trimmed, " "+kw) ||
			strings.HasSuffix(trimmed, "\t"+kw) {
			net++
		}
	}
	// Closers.
	for _, kw := range []string{"fi", "done", "esac", "}"} {
		if trimmed == kw || strings.HasPrefix(trimmed, kw+" ") {
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

// leadingWSStr returns the leading whitespace of a string.
func leadingWSStr(s string) string {
	runes := []rune(s)
	i := 0
	for i < len(runes) && (runes[i] == ' ' || runes[i] == '\t') {
		i++
	}
	return string(runes[:i])
}

// ---- JSON indentation -------------------------------------------------------

// netBraceCountJSON returns net opener - closer count for a JSON line,
// counting both { } and [ ] openers/closers, ignoring string contents.
func netBraceCountJSON(line string) int {
	runes := []rune(line)
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
	if depth < 0 {
		depth = 0
	}

	cur := strings.TrimLeft(lines[lineIdx], " \t")
	// A line opening with } or ] dedents by one.
	if strings.HasPrefix(cur, "}") || strings.HasPrefix(cur, "]") {
		depth--
		if depth < 0 {
			depth = 0
		}
	}

	return strings.Repeat(unit, depth)
}
