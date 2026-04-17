package syntax

// PerlHighlighter highlights Perl 5 source using a hand-written scanner.
type PerlHighlighter struct{}

var perlKeywords = map[string]bool{
	"if": true, "unless": true, "else": true, "elsif": true,
	"while": true, "until": true, "for": true, "foreach": true,
	"do": true, "last": true, "next": true, "redo": true,
	"return": true, "sub": true, "my": true, "our": true, "local": true,
	"use": true, "no": true, "require": true, "package": true,
	"BEGIN": true, "END": true, "INIT": true, "CHECK": true, "UNITCHECK": true,
	"eval": true, "die": true, "warn": true, "exit": true,
	"and": true, "or": true, "not": true,
	"eq": true, "ne": true, "lt": true, "gt": true, "le": true, "ge": true,
	"defined": true, "undef": true, "wantarray": true,
	"bless": true, "ref": true,
}

var perlBuiltins = map[string]bool{
	"print": true, "say": true, "printf": true, "sprintf": true,
	"chomp": true, "chop": true,
	"push": true, "pop": true, "shift": true, "unshift": true,
	"splice": true, "join": true, "split": true, "sort": true,
	"reverse": true, "map": true, "grep": true,
	"scalar": true, "length": true, "substr": true,
	"index": true, "rindex": true,
	"uc": true, "lc": true, "ucfirst": true, "lcfirst": true,
	"chr": true, "ord": true, "hex": true, "oct": true,
	"int": true, "abs": true, "sqrt": true, "rand": true, "srand": true,
	"open": true, "close": true, "read": true, "write": true,
	"seek": true, "tell": true, "eof": true, "binmode": true, "fileno": true,
	"stat": true, "lstat": true, "chmod": true, "chown": true,
	"rename": true, "unlink": true, "mkdir": true, "rmdir": true,
	"chdir": true, "getcwd": true,
	"opendir": true, "readdir": true, "closedir": true,
	"system": true, "exec": true, "fork": true,
	"wait": true, "waitpid": true, "sleep": true,
	"time": true, "localtime": true, "gmtime": true,
	"keys": true, "values": true, "each": true,
	"delete": true, "exists": true,
	"caller": true, "pos": true,
	"pack": true, "unpack": true,
}

// Highlight implements Highlighter for Perl.
func (h PerlHighlighter) Highlight(text string, start, end int) []Span {
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

		// Comment: # to end of line
		if r == '#' {
			j := i
			for j < n && runes[j] != '\n' {
				j++
			}
			emit(i, j, FaceComment)
			i = j
			continue
		}

		// POD documentation: =word ... =cut
		if r == '=' && i > 0 && runes[i-1] == '\n' && i+1 < n && isAlpha(runes[i+1]) {
			j := i
			for j < n {
				// Search for =cut at the start of a line.
				if runes[j] == '\n' && j+4 < n &&
					runes[j+1] == '=' && runes[j+2] == 'c' &&
					runes[j+3] == 'u' && runes[j+4] == 't' {
					// Advance past the =cut line.
					j += 5
					for j < n && runes[j] != '\n' {
						j++
					}
					break
				}
				j++
			}
			emit(i, j, FaceComment)
			i = j
			continue
		}

		// Double-quoted string: "..." with escape awareness
		if r == '"' {
			j := i + 1
			for j < n {
				if runes[j] == '\\' {
					j += 2
					continue
				}
				if runes[j] == '"' {
					j++
					break
				}
				j++
			}
			emit(i, j, FaceString)
			i = j
			continue
		}

		// Single-quoted string: '...' (only \\ and \' are escapes)
		if r == '\'' {
			j := i + 1
			for j < n {
				if runes[j] == '\\' && j+1 < n &&
					(runes[j+1] == '\'' || runes[j+1] == '\\') {
					j += 2
					continue
				}
				if runes[j] == '\'' {
					j++
					break
				}
				j++
			}
			emit(i, j, FaceString)
			i = j
			continue
		}

		// Backtick command string: `...`
		if r == '`' {
			j := i + 1
			for j < n {
				if runes[j] == '\\' {
					j += 2
					continue
				}
				if runes[j] == '`' {
					j++
					break
				}
				j++
			}
			emit(i, j, FaceString)
			i = j
			continue
		}

		// Variables: $scalar, @array, %hash, and common special variables.
		if r == '$' || r == '@' || r == '%' {
			if i+1 < n {
				next := runes[i+1]
				switch {
				case next == '{':
					// ${...} or @{...} or %{...}
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
				case isAlpha(next) || next == '_':
					j := i + 1
					for j < n && isIdentChar(runes[j]) {
						j++
					}
					emit(i, j, FaceType)
					i = j
					continue
				case r == '$' && (next == '_' || next == '!' || next == '/' ||
					next == '\\' || next == '"' ||
					next == '.' || next == '&' || next == '`' || next == '\'' ||
					next == '+' || next == ';'):
					// Common punctuation variables: $_, $!, $/, etc.
					emit(i, i+2, FaceType)
					i += 2
					continue
				case r == '$' && next >= '0' && next <= '9':
					// Capture variables $0–$9
					emit(i, i+2, FaceType)
					i += 2
					continue
				}
			}
		}

		// Number: decimal, hex (0x), binary (0b), octal (0).
		if r >= '0' && r <= '9' {
			j := i
			if r == '0' && j+1 < n {
				next := runes[j+1]
				if next == 'x' || next == 'X' {
					j += 2
					for j < n && (isHexRune(runes[j]) || runes[j] == '_') {
						j++
					}
				} else if next == 'b' || next == 'B' {
					j += 2
					for j < n && (runes[j] == '0' || runes[j] == '1' || runes[j] == '_') {
						j++
					}
				} else {
					for j < n && (runes[j] >= '0' && runes[j] <= '9' ||
						runes[j] == '.' || runes[j] == '_' ||
						runes[j] == 'e' || runes[j] == 'E') {
						j++
					}
				}
			} else {
				for j < n && (runes[j] >= '0' && runes[j] <= '9' ||
					runes[j] == '.' || runes[j] == '_' ||
					runes[j] == 'e' || runes[j] == 'E') {
					j++
				}
			}
			emit(i, j, FaceNumber)
			i = j
			continue
		}

		// Identifier: keyword or builtin.
		if isAlpha(r) || r == '_' {
			j := i
			for j < n && isIdentChar(runes[j]) {
				j++
			}
			sym := string(runes[i:j])
			switch {
			case perlKeywords[sym]:
				emit(i, j, FaceKeyword)
			case perlBuiltins[sym]:
				emit(i, j, FaceFunction)
			}
			i = j
			continue
		}

		i++
	}
	return spans
}
