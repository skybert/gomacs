package syntax

// JSONHighlighter highlights JSON source using a hand-written scanner.
type JSONHighlighter struct{}

// Highlight implements Highlighter for JSON.
//
// Scanning always begins at offset 0 so that multi-line string literals are
// tracked from the top of the file, but it stops as soon as the next token
// starts at or past end.
func (h JSONHighlighter) Highlight(text string, start, end int) []Span {
	return h.HighlightRunes([]rune(text), start, end)
}

// HighlightRunes implements RuneHighlighter.
func (h JSONHighlighter) HighlightRunes(runes []rune, start, end int) []Span {
	return h.scan(runes, ScanState{}, end, nil)
}

// HighlightResume implements Resumable.  A string literal is consumed whole by
// the token that opens it, so nothing is carried between tokens and the top of
// the token loop is always a safe restart point.
func (h JSONHighlighter) HighlightResume(runes []rune, st ScanState, end int, cp *Checkpoints) []Span {
	cp.arm(st.Pos)
	return h.scan(runes, st, end, cp)
}

// scan needs no start parameter: JSON emits a span for every token it
// recognises, so the caller's start only ever filtered spans the resumed scan
// does not reach in the first place.
func (h JSONHighlighter) scan(runes []rune, st ScanState, end int, cp *Checkpoints) []Span {
	n := len(runes)
	var spans []Span

	// scanLimit bounds where a *new* token may start.  Inner scans still use n
	// so a token beginning just before end is emitted in full.
	scanLimit := min(n, end)

	i := st.Pos
	for i < scanLimit {
		cp.mark(ScanState{Pos: i}, len(spans))
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
