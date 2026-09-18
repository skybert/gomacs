package syntax

// JavaHighlighter highlights Java source using a hand-written scanner.
type JavaHighlighter struct{}

var javaKeywords = map[string]bool{
	"abstract": true, "assert": true, "break": true, "case": true,
	"catch": true, "class": true, "const": true, "continue": true,
	"default": true, "do": true, "else": true, "enum": true,
	"extends": true, "final": true, "finally": true, "for": true,
	"goto": true, "if": true, "implements": true, "import": true,
	"instanceof": true, "interface": true, "native": true, "new": true,
	"package": true, "private": true, "protected": true, "public": true,
	"return": true, "static": true, "strictfp": true, "super": true,
	"switch": true, "synchronized": true, "this": true, "throw": true,
	"throws": true, "transient": true, "try": true, "var": true,
	"void": true, "volatile": true, "while": true,
	"true": true, "false": true, "null": true,
}

var javaPrimitives = map[string]bool{
	"boolean": true, "byte": true, "char": true, "double": true,
	"float": true, "int": true, "long": true, "short": true,
}

var javaTypes = map[string]bool{
	"String": true, "Integer": true, "Long": true, "Boolean": true,
	"Double": true, "Float": true, "Byte": true, "Short": true,
	"Character": true, "Object": true, "Number": true, "Math": true,
	"System": true, "Runtime": true, "Thread": true, "Runnable": true,
	"Exception": true, "RuntimeException": true, "Error": true,
	"ArrayList": true, "LinkedList": true, "HashMap": true, "HashSet": true,
	"TreeMap": true, "TreeSet": true, "List": true, "Map": true,
	"Set": true, "Collection": true, "Iterator": true, "Optional": true,
	"StringBuilder": true, "StringBuffer": true, "Arrays": true,
	"Collections": true, "Objects": true, "Stream": true,
}

// Highlight implements Highlighter for Java.
//
// Scanning always begins at offset 0 so multi-line constructs are tracked from
// the top of the file, but it stops as soon as the next token starts at or
// past end.
func (h JavaHighlighter) Highlight(text string, start, end int) []Span {
	return h.HighlightRunes([]rune(text), start, end)
}

// HighlightRunes implements RuneHighlighter.
func (h JavaHighlighter) HighlightRunes(runes []rune, start, end int) []Span {
	return h.scan(runes, ScanState{}, start, end, nil)
}

// HighlightResume implements Resumable.  A /* … */ comment is consumed whole by
// the token that opens it, so nothing is carried between tokens and the top of
// the token loop is always a safe restart point.
func (h JavaHighlighter) HighlightResume(runes []rune, st ScanState, end int, cp *Checkpoints) []Span {
	cp.arm(st.Pos)
	return h.scan(runes, st, st.Pos, end, cp)
}

func (h JavaHighlighter) scan(runes []rune, st ScanState, start, end int, cp *Checkpoints) []Span {
	n := len(runes)
	var spans []Span

	emit := func(s, e int, face Face) {
		if e > start && s < end {
			spans = append(spans, Span{Start: s, End: e, Face: face})
		}
	}

	// scanLimit bounds where a *new* token may start.  Inner scans still use n
	// so a token beginning just before end is emitted in full.
	scanLimit := min(n, end)

	i := st.Pos
	for i < scanLimit {
		cp.mark(ScanState{Pos: i}, len(spans))
		r := runes[i]

		// Line comment // ...
		if r == '/' && i+1 < n && runes[i+1] == '/' {
			j := i
			for j < n && runes[j] != '\n' {
				j++
			}
			emit(i, j, FaceComment)
			i = j
			continue
		}

		// Block comment /* ... */
		if r == '/' && i+1 < n && runes[i+1] == '*' {
			j := i + 2
			closed := false
			for j+1 < n {
				if runes[j] == '*' && runes[j+1] == '/' {
					j += 2
					closed = true
					break
				}
				j++
			}
			if !closed {
				// Unterminated: the comment runs to the end of the text.  The
				// loop above stops at n-1, which would leave the final rune
				// unhighlighted.
				j = n
			}
			emit(i, j, FaceComment)
			i = j
			continue
		}

		// Annotation: @Name
		if r == '@' && i+1 < n && isAlpha(runes[i+1]) {
			j := i + 1
			for j < n && isIdentChar(runes[j]) {
				j++
			}
			emit(i, j, FaceFunction)
			i = j
			continue
		}

		// String literal "..."
		if r == '"' {
			j := i + 1
			for j < n {
				if runes[j] == '\\' {
					j += 2
					continue
				}
				j++
				if runes[j-1] == '"' {
					break
				}
			}
			emit(i, j, FaceString)
			i = j
			continue
		}

		// Char literal '.'
		if r == '\'' {
			j := i + 1
			if j < n && runes[j] == '\\' {
				j += 2
			} else if j < n {
				j++
			}
			if j < n && runes[j] == '\'' {
				j++
			}
			emit(i, j, FaceString)
			i = j
			continue
		}

		// Number.
		if r >= '0' && r <= '9' {
			j := i
			for j < n && (runes[j] >= '0' && runes[j] <= '9' || runes[j] == '.' ||
				runes[j] == 'x' || runes[j] == 'X' || runes[j] == 'L' || runes[j] == 'l' ||
				runes[j] == 'f' || runes[j] == 'F' || runes[j] == '_' ||
				(runes[j] >= 'a' && runes[j] <= 'f') || (runes[j] >= 'A' && runes[j] <= 'F')) {
				j++
			}
			emit(i, j, FaceNumber)
			i = j
			continue
		}

		// Identifier / keyword / type.
		if isAlpha(r) || r == '_' {
			j := i
			for j < n && isIdentChar(runes[j]) {
				j++
			}
			sym := string(runes[i:j])
			switch {
			case javaKeywords[sym]:
				emit(i, j, FaceKeyword)
			case javaPrimitives[sym]:
				emit(i, j, FaceType)
			case javaTypes[sym]:
				emit(i, j, FaceType)
			}
			i = j
			continue
		}

		i++
	}
	return spans
}
