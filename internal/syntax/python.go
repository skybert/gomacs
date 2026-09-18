package syntax

// PythonHighlighter highlights Python source using a hand-written scanner.
type PythonHighlighter struct{}

var pythonKeywords = map[string]bool{
	"False": true, "None": true, "True": true,
	"and": true, "as": true, "assert": true, "async": true, "await": true,
	"break": true, "class": true, "continue": true, "def": true, "del": true,
	"elif": true, "else": true, "except": true, "finally": true, "for": true,
	"from": true, "global": true, "if": true, "import": true, "in": true,
	"is": true, "lambda": true, "nonlocal": true, "not": true, "or": true,
	"pass": true, "raise": true, "return": true, "try": true, "while": true,
	"with": true, "yield": true,
}

var pythonBuiltins = map[string]bool{
	"abs": true, "all": true, "any": true, "bin": true, "bool": true,
	"bytes": true, "callable": true, "chr": true, "classmethod": true,
	"compile": true, "complex": true, "delattr": true, "dict": true,
	"dir": true, "divmod": true, "enumerate": true, "eval": true,
	"exec": true, "filter": true, "float": true, "format": true,
	"frozenset": true, "getattr": true, "globals": true, "hasattr": true,
	"hash": true, "help": true, "hex": true, "id": true, "input": true,
	"int": true, "isinstance": true, "issubclass": true, "iter": true,
	"len": true, "list": true, "locals": true, "map": true, "max": true,
	"memoryview": true, "min": true, "next": true, "object": true,
	"oct": true, "open": true, "ord": true, "pow": true, "print": true,
	"property": true, "range": true, "repr": true, "reversed": true,
	"round": true, "set": true, "setattr": true, "slice": true,
	"sorted": true, "staticmethod": true, "str": true, "sum": true,
	"super": true, "tuple": true, "type": true, "vars": true, "zip": true,
}

// Highlight implements Highlighter for Python.
//
// Scanning always begins at offset 0 so multi-line constructs are tracked from
// the top of the file, but it stops as soon as the next token starts at or
// past end.
func (h PythonHighlighter) Highlight(text string, start, end int) []Span {
	return h.HighlightRunes([]rune(text), start, end)
}

// HighlightRunes implements RuneHighlighter.
func (h PythonHighlighter) HighlightRunes(runes []rune, start, end int) []Span {
	return h.scan(runes, ScanState{}, start, end, nil)
}

// HighlightResume implements Resumable.  Python needs no carried state: every
// multi-line construct (a triple-quoted string) is consumed whole by the token
// that opens it, so the top of the token loop is always a safe restart point and
// ScanState.Pos alone describes where the scan is.
func (h PythonHighlighter) HighlightResume(runes []rune, st ScanState, end int, cp *Checkpoints) []Span {
	cp.arm(st.Pos)
	return h.scan(runes, st, st.Pos, end, cp)
}

func (h PythonHighlighter) scan(runes []rune, st ScanState, start, end int, cp *Checkpoints) []Span {
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

		// Comment: # to end of line.
		if r == '#' {
			j := i
			for j < n && runes[j] != '\n' {
				j++
			}
			emit(i, j, FaceComment)
			i = j
			continue
		}

		// Triple-quoted string: """...""" or '''...'''
		if i+2 < n && (r == '"' || r == '\'') && runes[i+1] == r && runes[i+2] == r {
			quote := r
			j := i + 3
			for j+2 < n {
				if runes[j] == quote && runes[j+1] == quote && runes[j+2] == quote {
					j += 3
					break
				}
				j++
			}
			emit(i, j, FaceString)
			i = j
			continue
		}

		// Regular string: "..." or '...'
		if r == '"' || r == '\'' {
			quote := r
			j := i + 1
			for j < n && runes[j] != '\n' {
				if runes[j] == '\\' {
					j += 2
					continue
				}
				j++
				if runes[j-1] == quote {
					break
				}
			}
			emit(i, j, FaceString)
			i = j
			continue
		}

		// Decorator: @identifier
		if r == '@' && i+1 < n && isAlpha(runes[i+1]) {
			j := i + 1
			for j < n && isIdentChar(runes[j]) {
				j++
			}
			emit(i, j, FaceFunction)
			i = j
			continue
		}

		// Number.
		if r >= '0' && r <= '9' {
			j := i
			for j < n && (runes[j] >= '0' && runes[j] <= '9' || runes[j] == '.' || runes[j] == '_' ||
				runes[j] == 'x' || runes[j] == 'X' || runes[j] == 'o' || runes[j] == 'O' ||
				(runes[j] >= 'a' && runes[j] <= 'f') || (runes[j] >= 'A' && runes[j] <= 'F')) {
				j++
			}
			emit(i, j, FaceNumber)
			i = j
			continue
		}

		// Identifier / keyword / builtin.
		if isAlpha(r) || r == '_' {
			j := i
			for j < n && isIdentChar(runes[j]) {
				j++
			}
			sym := string(runes[i:j])
			switch {
			case pythonKeywords[sym]:
				emit(i, j, FaceKeyword)
			case pythonBuiltins[sym]:
				emit(i, j, FaceFunction)
			}
			i = j
			continue
		}

		i++
	}
	return spans
}

func isAlpha(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func isIdentChar(r rune) bool {
	return isAlpha(r) || (r >= '0' && r <= '9') || r == '_'
}
