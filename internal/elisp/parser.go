package elisp

import (
	"fmt"
	"strconv"
	"strings"
)

// Value is an Elisp value (S-expression).
type Value interface {
	isValue()
	String() string
}

// --- concrete types ---

// Nil represents nil / the empty list ().
type Nil struct{}

func (Nil) isValue()       {}
func (Nil) String() string { return elispNil }

// Bool represents t (true). nil is represented by Nil.
type Bool struct{ V bool }

func (Bool) isValue() {}
func (b Bool) String() string {
	if b.V {
		return "t"
	}
	return elispNil
}

// Int is an integer value.
type Int struct{ V int64 }

func (Int) isValue()         {}
func (i Int) String() string { return strconv.FormatInt(i.V, 10) }

// Float is a floating-point value.
type Float struct{ V float64 }

func (Float) isValue()         {}
func (f Float) String() string { return strconv.FormatFloat(f.V, 'g', -1, 64) }

// Symbol is an Elisp symbol.
type Symbol struct{ Name string }

func (Symbol) isValue()         {}
func (s Symbol) String() string { return s.Name }

// StringVal is a string value.
type StringVal struct{ V string }

func (StringVal) isValue()         {}
func (s StringVal) String() string { return fmt.Sprintf("%q", s.V) }

// Cons is a linked-list node.
type Cons struct{ Car, Cdr Value }

func (Cons) isValue() {}
func (c Cons) String() string {
	var sb strings.Builder
	sb.WriteRune('(')
	var cur Value = c
	first := true
	for {
		cons, ok := cur.(Cons)
		if !ok {
			break
		}
		if !first {
			sb.WriteRune(' ')
		}
		first = false
		sb.WriteString(cons.Car.String())
		cur = cons.Cdr
	}
	if !IsNil(cur) {
		sb.WriteString(" . ")
		sb.WriteString(cur.String())
	}
	sb.WriteRune(')')
	return sb.String()
}

// Vector is a fixed-size array.
type Vector struct{ Elems []Value }

func (Vector) isValue() {}
func (v Vector) String() string {
	parts := make([]string, len(v.Elems))
	for i, e := range v.Elems {
		parts[i] = e.String()
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// Lambda is a user-defined function.
type Lambda struct {
	Params []string
	Rest   string // name of &rest param, empty if none
	Body   []Value
	Env    *Env
}

func (Lambda) isValue() {}
func (l Lambda) String() string {
	params := strings.Join(l.Params, " ")
	if l.Rest != "" {
		if params != "" {
			params += " &rest " + l.Rest
		} else {
			params = "&rest " + l.Rest
		}
	}
	return fmt.Sprintf("#<lambda (%s)>", params)
}

// Builtin is a Go-backed function.
type Builtin struct {
	Name string
	Fn   func(args []Value, env *Env) (Value, error)
}

func (Builtin) isValue()         {}
func (b Builtin) String() string { return fmt.Sprintf("#<builtin %s>", b.Name) }

// --- helpers ---

// IsNil reports whether v is nil/empty list.
func IsNil(v Value) bool {
	if v == nil {
		return true
	}
	switch v := v.(type) {
	case Nil:
		return true
	case Bool:
		return !v.V
	}
	return false
}

// List builds a proper Lisp list from a slice of Values.
func List(vals ...Value) Value {
	var result Value = Nil{}
	for i := len(vals) - 1; i >= 0; i-- {
		result = Cons{Car: vals[i], Cdr: result}
	}
	return result
}

// ToSlice converts a proper Lisp list to a []Value slice.
// Returns (nil, false) if v is not a proper list.
func ToSlice(v Value) ([]Value, bool) {
	var out []Value
	cur := v
	for {
		if IsNil(cur) {
			return out, true
		}
		c, ok := cur.(Cons)
		if !ok {
			return nil, false
		}
		out = append(out, c.Car)
		cur = c.Cdr
	}
}

// --- parser ---

type parser struct {
	lexer *Lexer
}

// Parse parses all top-level forms from src.
func Parse(src string) ([]Value, error) {
	p := &parser{lexer: NewLexer(src)}
	var forms []Value
	for {
		tok := p.lexer.Peek()
		if tok.Type == TokenEOF {
			break
		}
		v, err := p.parseForm()
		if err != nil {
			return nil, err
		}
		forms = append(forms, v)
	}
	return forms, nil
}

// ParseOne parses a single form from src.
func ParseOne(src string) (Value, error) {
	p := &parser{lexer: NewLexer(src)}
	return p.parseForm()
}

func (p *parser) parseForm() (Value, error) {
	tok := p.lexer.Peek()
	//nolint:exhaustive // TokenType has a default case above covering unknowns
	switch tok.Type {
	case TokenEOF:
		return nil, fmt.Errorf("unexpected EOF")
	case TokenLParen:
		return p.parseList()
	case TokenQuote:
		p.lexer.Next()
		inner, err := p.parseForm()
		if err != nil {
			return nil, err
		}
		return List(Symbol{Name: "quote"}, inner), nil
	case TokenBackquote:
		p.lexer.Next()
		inner, err := p.parseForm()
		if err != nil {
			return nil, err
		}
		return List(Symbol{Name: "quasiquote"}, inner), nil
	case TokenComma:
		p.lexer.Next()
		inner, err := p.parseForm()
		if err != nil {
			return nil, err
		}
		return List(Symbol{Name: "unquote"}, inner), nil
	case TokenCommaAt:
		p.lexer.Next()
		inner, err := p.parseForm()
		if err != nil {
			return nil, err
		}
		return List(Symbol{Name: "unquote-splicing"}, inner), nil
	default:
		return p.parseAtom()
	}
}

func (p *parser) parseList() (Value, error) {
	p.lexer.Next() // consume '('

	var elems []Value
	var dotCdr Value // set if we encounter a dot

	for {
		tok := p.lexer.Peek()
		if tok.Type == TokenEOF {
			return nil, fmt.Errorf("unterminated list")
		}
		if tok.Type == TokenRParen {
			p.lexer.Next()
			break
		}
		if tok.Type == TokenDot {
			p.lexer.Next() // consume '.'
			var err error
			dotCdr, err = p.parseForm()
			if err != nil {
				return nil, err
			}
			// expect ')'
			closing := p.lexer.Next()
			if closing.Type != TokenRParen {
				return nil, fmt.Errorf("expected ) after dotted pair cdr")
			}
			break
		}
		v, err := p.parseForm()
		if err != nil {
			return nil, err
		}
		elems = append(elems, v)
	}

	// Build cons list
	var result Value
	if dotCdr != nil {
		result = dotCdr
	} else {
		result = Nil{}
	}
	for i := len(elems) - 1; i >= 0; i-- {
		result = Cons{Car: elems[i], Cdr: result}
	}
	return result, nil
}

func (p *parser) parseAtom() (Value, error) {
	tok := p.lexer.Next()
	//nolint:exhaustive // TokenType has a default case above covering unknowns
	switch tok.Type {
	case TokenBool:
		if tok.Value == "t" {
			return Bool{V: true}, nil
		}
		return Nil{}, nil
	case TokenInt:
		n, err := strconv.ParseInt(tok.Value, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("bad integer %q: %w", tok.Value, err)
		}
		return Int{V: n}, nil
	case TokenFloat:
		f, err := strconv.ParseFloat(tok.Value, 64)
		if err != nil {
			return nil, fmt.Errorf("bad float %q: %w", tok.Value, err)
		}
		return Float{V: f}, nil
	case TokenString:
		return StringVal{V: tok.Value}, nil
	case TokenSymbol:
		return Symbol{Name: tok.Value}, nil
	default:
		return nil, fmt.Errorf("unexpected token %q at %d:%d", tok.Value, tok.Line, tok.Column)
	}
}
