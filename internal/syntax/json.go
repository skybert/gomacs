package syntax

// JSONHighlighter highlights JSON source using a hand-written scanner.
type JSONHighlighter struct{}

// Highlight implements Highlighter for JSON.
func (h JSONHighlighter) Highlight(text string, start, end int) []Span {
	runes := []rune(text)
	n := len(runes)
	var spans []Span

	i := 0
	for i < n {
		r := runes[i]
		switch {
		case r == '"':
			// String literal — scan to closing unescaped quote.
			j := i + 1
			for j < n {
				if runes[j] == '\\' {
					j += 2 // skip escaped character
					continue
				}
				if runes[j] == '"' {
					j++
					break
				}
				j++
			}
			spans = append(spans, Span{Start: i, End: j, Face: FaceString})
			i = j

		case r == '-' || (r >= '0' && r <= '9'):
			// Number — optional minus, digits, optional fraction, optional exponent.
			j := i
			if j < n && runes[j] == '-' {
				j++
			}
			for j < n && runes[j] >= '0' && runes[j] <= '9' {
				j++
			}
			if j < n && runes[j] == '.' {
				j++
				for j < n && runes[j] >= '0' && runes[j] <= '9' {
					j++
				}
			}
			if j < n && (runes[j] == 'e' || runes[j] == 'E') {
				j++
				if j < n && (runes[j] == '+' || runes[j] == '-') {
					j++
				}
				for j < n && runes[j] >= '0' && runes[j] <= '9' {
					j++
				}
			}
			if j > i {
				spans = append(spans, Span{Start: i, End: j, Face: FaceNumber})
			}
			i = j

		case r == 't' && i+4 <= n && string(runes[i:i+4]) == "true":
			spans = append(spans, Span{Start: i, End: i + 4, Face: FaceKeyword})
			i += 4

		case r == 'f' && i+5 <= n && string(runes[i:i+5]) == "false":
			spans = append(spans, Span{Start: i, End: i + 5, Face: FaceKeyword})
			i += 5

		case r == 'n' && i+4 <= n && string(runes[i:i+4]) == "null":
			spans = append(spans, Span{Start: i, End: i + 4, Face: FaceKeyword})
			i += 4

		default:
			i++
		}
	}
	return spans
}
