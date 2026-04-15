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

func (HelpHighlighter) Highlight(text string, start, end int) []Span {
	runes := []rune(text)
	n := len(runes)
	if end > n {
		end = n
	}

	var spans []Span
	emit := func(s, e int, f Face) {
		if e > s {
			spans = append(spans, Span{Start: s, End: e, Face: f})
		}
	}

	// Collect all line boundaries so we can look ahead.
	type line struct{ start, end int }
	var lines []line
	i := 0
	for i <= n {
		ls := i
		for i < n && runes[i] != '\n' {
			i++
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
		// Skip lines outside the requested range.
		if le < start || ls >= end {
			continue
		}
		// Clamp to requested range.
		cls, cle := ls, le
		if cls < start {
			cls = start
		}
		if cle > end {
			cle = end
		}
		if cls >= cle {
			continue
		}

		// Title: very first line.
		if idx == 0 {
			emit(cls, cle, FaceHeader1)
			continue
		}

		// Separator lines: all '=' or all '-'.
		if isAllRune(ls, le, '=') || isAllRune(ls, le, '-') {
			emit(cls, cle, FaceComment)
			continue
		}

		// Non-indented non-empty line → heading of some kind.
		if ls < le && runes[ls] != ' ' {
			// If the next non-empty line is a '---' separator, this is a
			// top-level section heading; otherwise a group heading.
			ni := nextNonEmpty(idx)
			if ni >= 0 && isAllRune(lines[ni].start, lines[ni].end, '-') {
				emit(cls, cle, FaceHeader1)
			} else {
				emit(cls, cle, FaceFunction)
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
