package syntax

// ElispHighlighter highlights Emacs Lisp source using a hand-written scanner.
type ElispHighlighter struct{}

// elispKeywords contains Emacs Lisp special forms and common macros.
var elispKeywords = map[string]bool{
	"and":                 true,
	"catch":               true,
	"cond":                true,
	"condition-case":      true,
	"defconst":            true,
	"defmacro":            true,
	"defun":               true,
	"defvar":              true,
	"defcustom":           true,
	"defface":             true,
	"defgroup":            true,
	"dolist":              true,
	"dotimes":             true,
	"function":            true,
	"if":                  true,
	"interactive":         true,
	"lambda":              true,
	"let":                 true,
	"let*":                true,
	"nil":                 true,
	"or":                  true,
	"progn":               true,
	"prog1":               true,
	"prog2":               true,
	"quote":               true,
	"require":             true,
	"save-excursion":      true,
	"save-restriction":    true,
	"save-current-buffer": true,
	"setq":                true,
	"setq-local":          true,
	"setq-default":        true,
	"t":                   true,
	"unless":              true,
	"unwind-protect":      true,
	"when":                true,
	"while":               true,
	"with-current-buffer": true,
	"with-temp-buffer":    true,
}

// elispBuiltins contains commonly used Emacs Lisp built-in functions.
var elispBuiltins = map[string]bool{
	"apply":            true,
	"assoc":            true,
	"buffer-string":    true,
	"buffer-substring": true,
	"car":              true,
	"cdr":              true,
	"char-after":       true,
	"concat":           true,
	"cons":             true,
	"current-buffer":   true,
	"delete":           true,
	"eq":               true,
	"equal":            true,
	"error":            true,
	"eval":             true,
	"format":           true,
	"funcall":          true,
	"get-buffer":       true,
	"global-set-key":   true,
	"kbd":              true,
	"length":           true,
	"list":             true,
	"listp":            true,
	"load":             true,
	"mapcar":           true,
	"member":           true,
	"message":          true,
	"not":              true,
	"nth":              true,
	"null":             true,
	"number-to-string": true,
	"numberp":          true,
	"point":            true,
	"provide":          true,
	"push":             true,
	"reverse":          true,
	"set-buffer":       true,
	"string-to-number": true,
	"stringp":          true,
	"substring":        true,
	"symbol-name":      true,
	"symbolp":          true,
	"prin1":            true,
	"princ":            true,
	"print":            true,
}

// Highlight implements Highlighter for Emacs Lisp source.
func (h ElispHighlighter) Highlight(text string, start, end int) []Span {
	runes := []rune(text)
	n := len(runes)
	var spans []Span

	emit := func(s, e int, face Face) {
		if e > start && s < end {
			spans = append(spans, Span{Start: s, End: e, Face: face})
		}
	}

	i := 0
	for i < n {
		r := runes[i]

		// Line comment: ; to end of line.
		if r == ';' {
			j := i
			for j < n && runes[j] != '\n' {
				j++
			}
			emit(i, j, FaceComment)
			i = j
			continue
		}

		// String literal: "..." with backslash escapes.
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

		// Character literal: ?x or ?\n etc.
		if r == '?' && i+1 < n {
			j := i + 1
			if runes[j] == '\\' && j+1 < n {
				j += 2
			} else {
				j++
			}
			emit(i, j, FaceString)
			i = j
			continue
		}

		// Number: optional leading minus, digits, optional decimal point.
		if (r >= '0' && r <= '9') ||
			(r == '-' && i+1 < n && runes[i+1] >= '0' && runes[i+1] <= '9') {
			j := i
			if runes[j] == '-' {
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
			emit(i, j, FaceNumber)
			i = j
			continue
		}

		// Symbol or keyword.
		if isElispSymbolStart(r) {
			j := i
			for j < n && isElispSymbolChar(runes[j]) {
				j++
			}
			sym := string(runes[i:j])
			switch {
			case elispKeywords[sym]:
				emit(i, j, FaceKeyword)
			case elispBuiltins[sym]:
				emit(i, j, FaceFunction)
			}
			i = j
			continue
		}

		i++
	}
	return spans
}

// isElispSymbolStart reports whether r may start an Elisp symbol.
func isElispSymbolStart(r rune) bool {
	if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
		return true
	}
	switch r {
	case '-', '_', '+', '/', '=', '<', '>', '!', '&', ':':
		return true
	}
	return false
}

// isElispSymbolChar reports whether r may continue an Elisp symbol.
func isElispSymbolChar(r rune) bool {
	if isElispSymbolStart(r) {
		return true
	}
	if r >= '0' && r <= '9' {
		return true
	}
	// Allow * for names like let*, setq-default*, etc.
	return r == '*'
}
