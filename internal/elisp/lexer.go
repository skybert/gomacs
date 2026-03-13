package elisp

import (
	"strings"
	"unicode"
)

// elispTrue and elispNil are the canonical Elisp boolean symbol names.
const (
	elispTrue = "t"
	elispNil  = "nil"
)

// TokenType identifies the kind of token
type TokenType int

const (
	TokenEOF       TokenType = iota
	TokenLParen              // (
	TokenRParen              // )
	TokenQuote               // '
	TokenBackquote           // `
	TokenComma               // ,
	TokenCommaAt             // ,@
	TokenDot                 // .
	TokenSymbol              // foo, bar, t, nil, +, -, etc.
	TokenString              // "hello"
	TokenInt                 // 42, -1
	TokenFloat               // 3.14
	TokenBool                // t or nil (also symbol, but special)
)

// Token is a single lexical token.
type Token struct {
	Type   TokenType
	Value  string // raw source text
	Line   int
	Column int
}

// Lexer tokenises an Elisp source string.
type Lexer struct {
	src  []rune
	pos  int
	line int
	col  int

	peeked    bool
	peekToken Token
}

// NewLexer creates a new Lexer for the given source.
func NewLexer(src string) *Lexer {
	return &Lexer{
		src:  []rune(src),
		pos:  0,
		line: 1,
		col:  1,
	}
}

func (l *Lexer) eof() bool { return l.pos >= len(l.src) }

func (l *Lexer) peek1() rune {
	if l.eof() {
		return 0
	}
	return l.src[l.pos]
}

func (l *Lexer) peek2() rune {
	if l.pos+1 >= len(l.src) {
		return 0
	}
	return l.src[l.pos+1]
}

func (l *Lexer) advance() rune {
	r := l.src[l.pos]
	l.pos++
	if r == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return r
}

func (l *Lexer) skipWhitespaceAndComments() {
	for !l.eof() {
		r := l.peek1()
		switch {
		case unicode.IsSpace(r):
			l.advance()
		case r == ';':
			// comment: skip to end of line
			for !l.eof() && l.peek1() != '\n' {
				l.advance()
			}
		default:
			return
		}
	}
}

// isSymbolChar returns true for runes that can appear in a symbol name.
func isSymbolChar(r rune) bool {
	if unicode.IsLetter(r) || unicode.IsDigit(r) {
		return true
	}
	switch r {
	case '-', '+', '*', '/', '=', '<', '>', '!', '?', '_', '&',
		'%', '#', ':', '@', '^', '~', '|', '\\':
		return true
	}
	return false
}

// Next returns the next token, consuming it.
func (l *Lexer) Next() Token {
	if l.peeked {
		l.peeked = false
		return l.peekToken
	}
	return l.readToken()
}

// Peek returns the next token without consuming it.
func (l *Lexer) Peek() Token {
	if !l.peeked {
		l.peekToken = l.readToken()
		l.peeked = true
	}
	return l.peekToken
}

func (l *Lexer) readToken() Token {
	l.skipWhitespaceAndComments()

	if l.eof() {
		return Token{Type: TokenEOF, Line: l.line, Column: l.col}
	}

	line, col := l.line, l.col
	r := l.peek1()

	switch r {
	case '(':
		l.advance()
		return Token{Type: TokenLParen, Value: "(", Line: line, Column: col}
	case ')':
		l.advance()
		return Token{Type: TokenRParen, Value: ")", Line: line, Column: col}
	case '\'':
		l.advance()
		return Token{Type: TokenQuote, Value: "'", Line: line, Column: col}
	case '`':
		l.advance()
		return Token{Type: TokenBackquote, Value: "`", Line: line, Column: col}
	case ',':
		l.advance()
		if !l.eof() && l.peek1() == '@' {
			l.advance()
			return Token{Type: TokenCommaAt, Value: ",@", Line: line, Column: col}
		}
		return Token{Type: TokenComma, Value: ",", Line: line, Column: col}
	case '.':
		// A dot is a dot token only if not followed by a digit (which would be a float)
		if l.pos+1 < len(l.src) && unicode.IsDigit(l.src[l.pos+1]) {
			return l.readNumber(line, col)
		}
		l.advance()
		return Token{Type: TokenDot, Value: ".", Line: line, Column: col}
	case '"':
		return l.readString(line, col)
	}

	// Numbers: start with digit, or '-'/'+'  followed by digit
	if unicode.IsDigit(r) {
		return l.readNumber(line, col)
	}
	if (r == '-' || r == '+') && l.pos+1 < len(l.src) && unicode.IsDigit(l.src[l.pos+1]) {
		return l.readNumber(line, col)
	}

	// Symbol
	return l.readSymbol(line, col)
}

func (l *Lexer) readString(line, col int) Token {
	l.advance() // consume opening "
	var sb strings.Builder
	for !l.eof() {
		r := l.advance()
		if r == '"' {
			break
		}
		if r == '\\' {
			if l.eof() {
				break
			}
			esc := l.advance()
			switch esc {
			case 'n':
				sb.WriteRune('\n')
			case 't':
				sb.WriteRune('\t')
			case '"':
				sb.WriteRune('"')
			case '\\':
				sb.WriteRune('\\')
			default:
				sb.WriteRune('\\')
				sb.WriteRune(esc)
			}
		} else {
			sb.WriteRune(r)
		}
	}
	return Token{Type: TokenString, Value: sb.String(), Line: line, Column: col}
}

func (l *Lexer) readNumber(line, col int) Token {
	var sb strings.Builder
	isFloat := false

	if !l.eof() && (l.peek1() == '-' || l.peek1() == '+') {
		sb.WriteRune(l.advance())
	}
	// integer part
	for !l.eof() && unicode.IsDigit(l.peek1()) {
		sb.WriteRune(l.advance())
	}
	// decimal part
	if !l.eof() && l.peek1() == '.' {
		next := l.peek2()
		if unicode.IsDigit(next) || next == 0 {
			isFloat = true
			sb.WriteRune(l.advance()) // '.'
			for !l.eof() && unicode.IsDigit(l.peek1()) {
				sb.WriteRune(l.advance())
			}
		}
	}
	// exponent
	if !l.eof() && (l.peek1() == 'e' || l.peek1() == 'E') {
		isFloat = true
		sb.WriteRune(l.advance())
		if !l.eof() && (l.peek1() == '+' || l.peek1() == '-') {
			sb.WriteRune(l.advance())
		}
		for !l.eof() && unicode.IsDigit(l.peek1()) {
			sb.WriteRune(l.advance())
		}
	}
	if isFloat {
		return Token{Type: TokenFloat, Value: sb.String(), Line: line, Column: col}
	}
	return Token{Type: TokenInt, Value: sb.String(), Line: line, Column: col}
}

func (l *Lexer) readSymbol(line, col int) Token {
	var sb strings.Builder
	for !l.eof() && isSymbolChar(l.peek1()) {
		sb.WriteRune(l.advance())
	}
	s := sb.String()
	if s == elispTrue || s == elispNil {
		return Token{Type: TokenBool, Value: s, Line: line, Column: col}
	}
	return Token{Type: TokenSymbol, Value: s, Line: line, Column: col}
}
