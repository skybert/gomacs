package syntax

import "strings"

// YAMLHighlighter highlights YAML source using a hand-written scanner.
type YAMLHighlighter struct{}

var yamlBoolNullValues = map[string]bool{
	"true": true, "false": true, "null": true, "~": true,
	"yes": true, "no": true, "on": true, "off": true,
	"True": true, "False": true, "Null": true,
	"Yes": true, "No": true, "On": true, "Off": true,
	"TRUE": true, "FALSE": true, "NULL": true,
	"YES": true, "NO": true, "ON": true, "OFF": true,
}

// Highlight implements Highlighter for YAML.
func (h YAMLHighlighter) Highlight(text string, start, end int) []Span {
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

		line := string(runes[lineStart:lineEnd])
		trimmed := strings.TrimSpace(line)

		// Document markers.
		if trimmed == "---" || trimmed == "..." {
			emit(lineStart, lineEnd, FaceKeyword)
			continue
		}

		// YAML directives (%YAML, %TAG).
		if strings.HasPrefix(trimmed, "%") {
			emit(lineStart, lineEnd, FaceKeyword)
			continue
		}

		// Comments: # ...
		if strings.HasPrefix(trimmed, "#") {
			emit(lineStart, lineEnd, FaceComment)
			continue
		}

		// Scan character by character within the line.
		j := lineStart
		for j < lineEnd {
			r := runes[j]

			// Inline comment.
			if r == '#' {
				emit(j, lineEnd, FaceComment)
				break
			}

			// Quoted strings.
			if r == '"' || r == '\'' {
				quote := r
				end2 := j + 1
				for end2 < lineEnd {
					if runes[end2] == '\\' && quote == '"' {
						end2 += 2
						continue
					}
					if runes[end2] == quote {
						end2++
						break
					}
					end2++
				}
				emit(j, end2, FaceString)
				j = end2
				continue
			}

			// Block scalar indicators: | and > at certain positions.
			if (r == '|' || r == '>') && j == lineStart+len(line)-len(strings.TrimLeft(line, " \t")) {
				emit(j, j+1, FaceKeyword)
				j++
				continue
			}

			// Anchors (&name) and aliases (*name).
			if r == '&' || r == '*' {
				k := j + 1
				for k < lineEnd && !isYAMLWhitespace(runes[k]) && runes[k] != ':' && runes[k] != ',' && runes[k] != ']' && runes[k] != '}' {
					k++
				}
				if k > j+1 {
					emit(j, k, FaceType)
					j = k
					continue
				}
			}

			// Flow indicators.
			if r == '{' || r == '}' || r == '[' || r == ']' || r == ',' {
				emit(j, j+1, FaceOperator)
				j++
				continue
			}

			// Numbers: leading digit or sign+digit.
			if r >= '0' && r <= '9' || ((r == '-' || r == '+') && j+1 < lineEnd && runes[j+1] >= '0' && runes[j+1] <= '9') {
				k := j
				if runes[k] == '-' || runes[k] == '+' {
					k++
				}
				for k < lineEnd && ((runes[k] >= '0' && runes[k] <= '9') || runes[k] == '.' || runes[k] == 'e' || runes[k] == 'E' || runes[k] == '_') {
					k++
				}
				// Only emit as number if followed by whitespace, comma, or line end.
				if k < lineEnd && !isYAMLWhitespace(runes[k]) && runes[k] != ',' && runes[k] != ']' && runes[k] != '}' && runes[k] != ':' && runes[k] != '\n' {
					j = k
					continue
				}
				emit(j, k, FaceNumber)
				j = k
				continue
			}

			// Words: check for bool/null keywords and mapping keys.
			if isYAMLWordStart(r) {
				k := j
				for k < lineEnd && isYAMLWordRune(runes[k]) {
					k++
				}
				word := string(runes[j:k])

				// Skip whitespace after word to check for ':'.
				m := k
				for m < lineEnd && runes[m] == ' ' {
					m++
				}
				if m < lineEnd && runes[m] == ':' && (m+1 >= lineEnd || isYAMLWhitespace(runes[m+1]) || runes[m+1] == '\n') {
					// This is a mapping key.
					emit(j, k, FaceFunction)
					j = k
					continue
				}
				// Check for colon immediately after word (key: value).
				if k < lineEnd && runes[k] == ':' && (k+1 >= lineEnd || isYAMLWhitespace(runes[k+1]) || runes[k+1] == '\n') {
					emit(j, k, FaceFunction)
					j = k
					continue
				}

				// Bool/null values.
				if yamlBoolNullValues[word] {
					emit(j, k, FaceKeyword)
					j = k
					continue
				}

				j = k
				continue
			}

			j++
		}
	}
	return spans
}

func isYAMLWhitespace(r rune) bool {
	return r == ' ' || r == '\t'
}

func isYAMLWordStart(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_'
}

func isYAMLWordRune(r rune) bool {
	return isYAMLWordStart(r) || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.'
}
