package syntax

import "strings"

// ConfHighlighter highlights configuration files (.conf, *rc, .toml) using a
// hand-written scanner.
//
// Supported constructs:
//   - Comments: lines (or line suffixes) beginning with '#'
//   - Section headers: [section] and [[array-of-tables]] (TOML)
//   - Keys: everything left of '=' or ':' assignment
//   - Strings: "..." and '...'
//   - Numbers: bare numeric tokens
//   - Booleans: true/false
type ConfHighlighter struct{}

// Highlight implements Highlighter for conf/ini/toml files.
func (h ConfHighlighter) Highlight(text string, start, end int) []Span {
	runes := []rune(text)
	n := len(runes)
	var spans []Span

	emit := func(s, e int, face Face) {
		if e > start && s < end && s < e {
			spans = append(spans, Span{Start: s, End: e, Face: face})
		}
	}

	i := 0
	for i < n {
		lineStart := i
		// Find end of line.
		for i < n && runes[i] != '\n' {
			i++
		}
		lineEnd := i
		if i < n {
			i++ // consume '\n'
		}
		if lineStart >= lineEnd {
			continue
		}

		// Skip leading whitespace to find the first non-blank character.
		j := lineStart
		for j < lineEnd && (runes[j] == ' ' || runes[j] == '\t') {
			j++
		}
		if j >= lineEnd {
			continue
		}

		// Full-line comment: # or ; at the start of the (trimmed) line.
		if runes[j] == '#' || runes[j] == ';' {
			emit(j, lineEnd, FaceComment)
			continue
		}

		// Section header: [section] or [[array-of-tables]].
		if runes[j] == '[' {
			emit(j, lineEnd, FaceType)
			continue
		}

		// Scan the line for key = value or key: value.
		// Find the '=' or ':' separator.
		sep := -1
		inStr := false
		strChar := rune(0)
		for k := j; k < lineEnd; k++ {
			r := runes[k]
			if inStr {
				if r == '\\' {
					k++ // skip escaped char
					continue
				}
				if r == strChar {
					inStr = false
				}
				continue
			}
			if r == '"' || r == '\'' {
				inStr = true
				strChar = r
				continue
			}
			if r == '#' || r == ';' {
				break // rest is comment
			}
			if r == '=' || r == ':' {
				sep = k
				break
			}
		}

		if sep > j {
			// Highlight the key.
			keyEnd := sep
			// Trim trailing whitespace from key.
			for keyEnd > j && (runes[keyEnd-1] == ' ' || runes[keyEnd-1] == '\t') {
				keyEnd--
			}
			emit(j, keyEnd, FaceFunction)

			// Highlight the value portion (after separator).
			valStart := sep + 1
			for valStart < lineEnd && (runes[valStart] == ' ' || runes[valStart] == '\t') {
				valStart++
			}
			if valStart < lineEnd {
				highlightConfValue(runes, valStart, lineEnd, emit)
			}
		}
	}
	return spans
}

// highlightConfValue highlights the value portion of a conf key=value line.
func highlightConfValue(runes []rune, start, end int, emit func(int, int, Face)) {
	i := start
	for i < end {
		r := runes[i]

		// Inline comment.
		if r == '#' || r == ';' {
			emit(i, end, FaceComment)
			return
		}

		// Quoted string.
		if r == '"' || r == '\'' {
			quote := r
			j := i + 1
			for j < end {
				if runes[j] == '\\' && quote == '"' {
					j += 2
					continue
				}
				if runes[j] == quote {
					j++
					break
				}
				j++
			}
			emit(i, j, FaceString)
			i = j
			continue
		}

		// Number (integer or float, including hex 0x... and TOML datetime prefix).
		if r >= '0' && r <= '9' || r == '-' && i+1 < end && runes[i+1] >= '0' && runes[i+1] <= '9' {
			j := i
			if runes[j] == '-' {
				j++
			}
			for j < end && isConfNumRune(runes[j]) {
				j++
			}
			emit(i, j, FaceNumber)
			i = j
			continue
		}

		// Boolean / bare keyword: true, false, yes, no, on, off, null.
		if isConfAlpha(r) {
			j := i
			for j < end && isConfAlpha(runes[j]) {
				j++
			}
			word := strings.ToLower(string(runes[i:j]))
			switch word {
			case "true", "false", "yes", "no", "on", "off", "null":
				emit(i, j, FaceKeyword)
			}
			i = j
			continue
		}

		i++
	}
}

func isConfNumRune(r rune) bool {
	return (r >= '0' && r <= '9') || r == '.' || r == '_' ||
		r == 'x' || r == 'X' || r == 'e' || r == 'E' ||
		(r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') ||
		r == '+' || r == '-' || r == ':' || r == 'T' || r == 'Z'
}

func isConfAlpha(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || r == '-'
}
