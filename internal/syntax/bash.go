package syntax

// BashHighlighter highlights Bash/shell source using a hand-written scanner.
type BashHighlighter struct{}

var bashKeywords = map[string]bool{
	"if": true, "then": true, "else": true, "elif": true, "fi": true,
	"for": true, "while": true, "until": true, "do": true, "done": true,
	"case": true, "esac": true, "in": true, "function": true,
	"return": true, "exit": true, "break": true, "continue": true,
	"select": true, "time": true, "coproc": true,
}

var bashBuiltins = map[string]bool{
	"echo": true, "printf": true, "read": true, "cd": true, "pwd": true,
	"pushd": true, "popd": true, "dirs": true, "export": true, "unset": true,
	"declare": true, "local": true, "readonly": true, "typeset": true,
	"source": true, "eval": true, "exec": true, "trap": true, "wait": true,
	"jobs": true, "fg": true, "bg": true, "kill": true, "disown": true,
	"alias": true, "unalias": true, "set": true, "shopt": true,
	"getopts": true, "shift": true, "test": true, "true": true, "false": true,
	"type": true, "which": true, "command": true, "builtin": true,
	"enable": true, "help": true, "let": true, "expr": true,
	"cat": true, "ls": true, "grep": true, "sed": true, "awk": true,
	"find": true, "sort": true, "uniq": true, "wc": true, "head": true,
	"tail": true, "cut": true, "tr": true, "xargs": true, "tee": true,
	"mkdir": true, "rmdir": true, "rm": true, "cp": true, "mv": true,
	"touch": true, "chmod": true, "chown": true, "ln": true, "stat": true,
}

// Highlight implements Highlighter for Bash.
func (h BashHighlighter) Highlight(text string, start, end int) []Span {
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

		// Shebang: #! at position 0
		if r == '#' && i == 0 && i+1 < n && runes[i+1] == '!' {
			j := i
			for j < n && runes[j] != '\n' {
				j++
			}
			emit(i, j, FaceComment)
			i = j
			continue
		}

		// Comment: # to end of line (not inside strings)
		if r == '#' {
			j := i
			for j < n && runes[j] != '\n' {
				j++
			}
			emit(i, j, FaceComment)
			i = j
			continue
		}

		// Double-quoted string: "..." with $var expansion awareness
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

		// Single-quoted string: '...' (no escapes)
		if r == '\'' {
			j := i + 1
			for j < n && runes[j] != '\'' {
				j++
			}
			if j < n {
				j++ // consume closing quote
			}
			emit(i, j, FaceString)
			i = j
			continue
		}

		// Variable: $VAR or ${VAR} or $( subshell)
		if r == '$' && i+1 < n {
			next := runes[i+1]
			switch {
			case next == '{':
				// ${...}
				j := i + 2
				for j < n && runes[j] != '}' {
					j++
				}
				if j < n {
					j++
				}
				emit(i, j, FaceType)
				i = j
				continue
			case next == '(':
				// $(...) subshell — just highlight the $
				emit(i, i+1, FaceType)
				i++
				continue
			case isAlpha(next) || next == '_':
				j := i + 1
				for j < n && isIdentChar(runes[j]) {
					j++
				}
				emit(i, j, FaceType)
				i = j
				continue
			}
		}

		// Number.
		if r >= '0' && r <= '9' {
			j := i
			for j < n && (runes[j] >= '0' && runes[j] <= '9' || runes[j] == '.' ||
				runes[j] == 'x' || runes[j] == 'X' ||
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
			case bashKeywords[sym]:
				emit(i, j, FaceKeyword)
			case bashBuiltins[sym]:
				emit(i, j, FaceFunction)
			}
			i = j
			continue
		}

		i++
	}
	return spans
}
