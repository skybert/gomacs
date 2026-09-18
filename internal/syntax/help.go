package syntax

// HelpHighlighter colours the *Help* buffer produced by cmdHelp:
//   - title line ("gomacs help")          → FaceHeader1
//   - ===/--- separator lines             → FaceComment
//   - top-level section headings          → FaceHeader1
//     ("Commands", "Configuration Variables" — identified by the
//     immediately-following --- separator)
//   - group headings ("Navigation", …)    → FaceFunction
//   - command/config-var names            → FaceKeyword
//   - key-binding text (…)               → FaceString
type HelpHighlighter struct{}

func (h HelpHighlighter) Highlight(text string, start, end int) []Span {
	return h.HighlightRunes([]rune(text), start, end)
}

// HighlightRunes implements RuneHighlighter.  HelpHighlighter does not implement
// Resumable: whether a heading is top-level depends on the next non-empty line,
// so it looks ahead as well as behind and cannot restart from a single offset.
func (h HelpHighlighter) HighlightRunes(runes []rune, start, end int) []Span {
	n := len(runes)
	end = min(end, n)

	var spans []Span
	emit := func(s, e int, f Face) {
		if e > s {
			spans = append(spans, Span{Start: s, End: e, Face: f})
		}
	}

	// Collect line boundaries so we can look ahead.  Only lines that start
	// before end are highlighted, but collection continues past end until one
	// more non-empty line has been seen so that nextNonEmpty() gives the same
	// answer it would for the whole buffer.
	type line struct{ start, end int }
	var lines []line
	sawTail := false
	i := 0
	for i <= n {
		ls := i
		for i < n && runes[i] != '\n' {
			i++
		}
		if ls >= end {
			if sawTail {
				break
			}
			if ls < i {
				sawTail = true
			}
		}
		lines = append(lines, line{ls, i})
		i++ // skip '\n' (or move past end)
	}

	isAllRune := func(ls, le int, r rune) bool {
		if ls >= le {
			return false
		}
		for k := ls; k < le; k++ {
			if runes[k] != r {
				return false
			}
		}
		return true
	}

	// nextNonEmptyLine returns the index of the first non-empty line after idx,
	// or -1 if none exists.
	nextNonEmpty := func(idx int) int {
		for j := idx + 1; j < len(lines); j++ {
			if lines[j].start < lines[j].end {
				return j
			}
		}
		return -1
	}

	for idx, l := range lines {
		ls, le := l.start, l.end
		// Skip lines outside the requested range.  Lines that overlap it are
		// emitted at their true bounds — clamping would truncate the span of a
		// line that starts inside the range and reaches past it.
		if le < start || ls >= end || ls >= le {
			continue
		}

		// Title: very first line.
		if idx == 0 {
			emit(ls, le, FaceHeader1)
			continue
		}

		// Separator lines: all '=' or all '-'.
		if isAllRune(ls, le, '=') || isAllRune(ls, le, '-') {
			emit(ls, le, FaceComment)
			continue
		}

		// Non-indented non-empty line → heading of some kind.
		if ls < le && runes[ls] != ' ' {
			// If the next non-empty line is a '---' separator, this is a
			// top-level section heading; otherwise a group heading.
			ni := nextNonEmpty(idx)
			if ni >= 0 && isAllRune(lines[ni].start, lines[ni].end, '-') {
				emit(ls, le, FaceHeader1)
			} else {
				emit(ls, le, FaceFunction)
			}
			continue
		}

		// Command / config-var entry line: exactly 2-space indent.
		// Format: "  <name><spaces>(<keys>)" or "  <name><spaces><doc>"
		if le-ls >= 3 && runes[ls] == ' ' && runes[ls+1] == ' ' && runes[ls+2] != ' ' {
			nameStart := ls + 2
			j := nameStart
			for j < le && runes[j] != ' ' {
				j++
			}
			nameEnd := j
			emit(nameStart, nameEnd, FaceKeyword)

			// Key-binding: text inside trailing (…).
			if le > ls && runes[le-1] == ')' {
				k := le - 2
				for k > nameEnd && runes[k] != '(' {
					k--
				}
				if runes[k] == '(' && k > nameEnd {
					emit(k, le, FaceString)
				}
			}
		}
	}
	return spans
}
