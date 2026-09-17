package elisp

import "testing"

// Tests for parser.go: Value types, their String() methods, IsNil/ToSlice
// helpers, and the S-expression parser.

// -------------------------------------------------------------------
// Parser tests
// -------------------------------------------------------------------

func TestParser_SimpleList(t *testing.T) {
	v, err := ParseOne("(+ 1 2)")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	// Expect Cons{+, Cons{1, Cons{2, Nil}}}
	c, ok := v.(Cons)
	if !ok {
		t.Fatalf("expected Cons, got %T", v)
	}
	sym, ok := c.Car.(Symbol)
	if !ok || sym.Name != "+" {
		t.Fatalf("expected symbol +")
	}
	c2, ok := c.Cdr.(Cons)
	if !ok {
		t.Fatalf("expected Cons for cdr, got %T", c.Cdr)
	}
	i1, ok := c2.Car.(Int)
	if !ok || i1.V != 1 {
		t.Fatalf("expected Int 1")
	}
	c3, ok := c2.Cdr.(Cons)
	if !ok {
		t.Fatalf("expected Cons for cddr, got %T", c2.Cdr)
	}
	i2, ok := c3.Car.(Int)
	if !ok || i2.V != 2 {
		t.Fatalf("expected Int 2")
	}
	if !IsNil(c3.Cdr) {
		t.Fatalf("expected nil at end of list")
	}
}

func TestParser_Quote(t *testing.T) {
	v, err := ParseOne("'foo")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	// Should produce (quote foo)
	c, ok := v.(Cons)
	if !ok {
		t.Fatalf("expected Cons")
	}
	sym, ok := c.Car.(Symbol)
	if !ok || sym.Name != "quote" {
		t.Fatalf("expected symbol quote, got %v", c.Car)
	}
}

func TestParser_Nested(t *testing.T) {
	_, err := ParseOne("(defun add (a b) (+ a b))")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
}

// -------------------------------------------------------------------
// parser — parseForm error paths
// -------------------------------------------------------------------

func TestParse_UnterminatedList(t *testing.T) {
	_, err := Parse("(1 2 3")
	if err == nil {
		t.Fatal("expected error for unterminated list")
	}
}

func TestParse_UnterminatedVector(t *testing.T) {
	_, err := Parse("[1 2 3")
	if err == nil {
		t.Fatal("expected error for unterminated vector")
	}
}

func TestParse_UnexpectedEOFQuote(t *testing.T) {
	_, err := Parse("'")
	if err == nil {
		t.Fatal("expected error for bare quote with no form")
	}
}

func TestParse_UnexpectedEOFSharpQuote(t *testing.T) {
	_, err := Parse("#'")
	if err == nil {
		t.Fatal("expected error for bare #' with no form")
	}
}

func TestParse_UnexpectedEOFBackquote(t *testing.T) {
	_, err := Parse("`")
	if err == nil {
		t.Fatal("expected error for bare backquote with no form")
	}
}

func TestParse_UnexpectedEOFComma(t *testing.T) {
	_, err := Parse(",")
	if err == nil {
		t.Fatal("expected error for bare comma with no form")
	}
}

func TestParse_UnexpectedEOFCommaAt(t *testing.T) {
	_, err := Parse(",@")
	if err == nil {
		t.Fatal("expected error for bare ,@ with no form")
	}
}

func TestParse_DottedPairMissingCloseParen(t *testing.T) {
	_, err := Parse("(1 . 2 3)")
	if err == nil {
		t.Fatal("expected error for dotted pair missing closing paren")
	}
}

func TestParse_UnexpectedToken(t *testing.T) {
	// A lone `)` at the top level is an unexpected token for parseAtom.
	_, err := Parse(")")
	if err == nil {
		t.Fatal("expected error for unexpected )")
	}
}

// -------------------------------------------------------------------
// IsNil — Bool{false} is nil
// -------------------------------------------------------------------

func TestIsNil_BoolFalse(t *testing.T) {
	if !IsNil(Bool{V: false}) {
		t.Fatal("Bool{false} should be nil")
	}
}

func TestIsNil_NilGoValue(t *testing.T) {
	if !IsNil(nil) {
		t.Fatal("Go nil should be nil")
	}
}

// -------------------------------------------------------------------
// ToSlice — improper list returns false
// -------------------------------------------------------------------

func TestToSlice_ImproperList(t *testing.T) {
	improper := Cons{Car: Int{V: 1}, Cdr: Int{V: 2}}
	_, ok := ToSlice(improper)
	if ok {
		t.Fatal("ToSlice should return false for improper list")
	}
}

// -------------------------------------------------------------------
// String() representations for all concrete Value types
// -------------------------------------------------------------------

func TestString_Nil(t *testing.T) {
	v := Nil{}
	if v.String() != "nil" {
		t.Fatalf("Nil.String() = %q, want \"nil\"", v.String())
	}
}

func TestString_BoolTrue(t *testing.T) {
	v := Bool{V: true}
	if v.String() != "t" {
		t.Fatalf("Bool{true}.String() = %q, want \"t\"", v.String())
	}
}

func TestString_BoolFalse(t *testing.T) {
	v := Bool{V: false}
	if v.String() != "nil" {
		t.Fatalf("Bool{false}.String() = %q, want \"nil\"", v.String())
	}
}

func TestString_Int(t *testing.T) {
	v := Int{V: 42}
	if v.String() != "42" {
		t.Fatalf("Int{42}.String() = %q, want \"42\"", v.String())
	}
}

func TestString_IntNegative(t *testing.T) {
	v := Int{V: -7}
	if v.String() != "-7" {
		t.Fatalf("Int{-7}.String() = %q, want \"-7\"", v.String())
	}
}

func TestString_Float(t *testing.T) {
	v := Float{V: 3.14}
	s := v.String()
	if s != "3.14" {
		t.Fatalf("Float{3.14}.String() = %q, want \"3.14\"", s)
	}
}

func TestString_Symbol(t *testing.T) {
	v := Symbol{Name: "foo"}
	if v.String() != "foo" {
		t.Fatalf("Symbol{foo}.String() = %q, want \"foo\"", v.String())
	}
}

func TestString_StringVal(t *testing.T) {
	v := StringVal{V: "hello"}
	s := v.String()
	if s != `"hello"` {
		t.Fatalf("StringVal{hello}.String() = %q, want `\"hello\"`", s)
	}
}

func TestString_ConsProperList(t *testing.T) {
	// (1 2 3)
	v := List(Int{V: 1}, Int{V: 2}, Int{V: 3})
	if v.String() != "(1 2 3)" {
		t.Fatalf("List(1,2,3).String() = %q, want \"(1 2 3)\"", v.String())
	}
}

func TestString_ConsDottedPair(t *testing.T) {
	v := Cons{Car: Int{V: 1}, Cdr: Int{V: 2}}
	if v.String() != "(1 . 2)" {
		t.Fatalf("Cons{1,2}.String() = %q, want \"(1 . 2)\"", v.String())
	}
}

func TestString_Vector(t *testing.T) {
	v := Vector{Elems: []Value{Int{V: 1}, Int{V: 2}}}
	if v.String() != "[1 2]" {
		t.Fatalf("Vector{1,2}.String() = %q, want \"[1 2]\"", v.String())
	}
}

func TestString_VectorEmpty(t *testing.T) {
	v := Vector{Elems: nil}
	if v.String() != "[]" {
		t.Fatalf("Vector{}.String() = %q, want \"[]\"", v.String())
	}
}

func TestString_LambdaNoParams(t *testing.T) {
	l := Lambda{Params: nil, Rest: "", Body: nil, Env: NewEnv()}
	if l.String() != "#<lambda ()>" {
		t.Fatalf("Lambda{}.String() = %q, want \"#<lambda ()>\"", l.String())
	}
}

func TestString_LambdaWithParams(t *testing.T) {
	l := Lambda{Params: []string{"x", "y"}, Rest: "", Body: nil, Env: NewEnv()}
	if l.String() != "#<lambda (x y)>" {
		t.Fatalf("Lambda{x,y}.String() = %q, want \"#<lambda (x y)>\"", l.String())
	}
}

func TestString_LambdaWithRest(t *testing.T) {
	l := Lambda{Params: nil, Rest: "args", Body: nil, Env: NewEnv()}
	if l.String() != "#<lambda (&rest args)>" {
		t.Fatalf("Lambda{&rest args}.String() = %q", l.String())
	}
}

func TestString_LambdaWithParamsAndRest(t *testing.T) {
	l := Lambda{Params: []string{"a"}, Rest: "rest", Body: nil, Env: NewEnv()}
	if l.String() != "#<lambda (a &rest rest)>" {
		t.Fatalf("Lambda{a,&rest rest}.String() = %q", l.String())
	}
}

func TestString_Builtin(t *testing.T) {
	b := Builtin{Name: "car", Fn: nil}
	if b.String() != "#<builtin car>" {
		t.Fatalf("Builtin{car}.String() = %q, want \"#<builtin car>\"", b.String())
	}
}

func TestValueStringMethods(t *testing.T) {
	cases := []struct {
		name string
		val  Value
		want string
	}{
		{"nil", Nil{}, elispNil},
		{"bool-true", Bool{V: true}, "t"},
		{"bool-false", Bool{V: false}, elispNil},
		{"int", Int{V: -42}, "-42"},
		{"float", Float{V: 3.5}, "3.5"},
		{"symbol", Symbol{Name: "foo"}, "foo"},
		{"string", StringVal{V: "hi"}, `"hi"`},
		{"vector", Vector{Elems: []Value{Int{V: 1}, Int{V: 2}}}, "[1 2]"},
		{"vector-empty", Vector{Elems: nil}, "[]"},
		{"builtin", Builtin{Name: "car"}, "#<builtin car>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.val.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestConsStringProperAndDotted(t *testing.T) {
	proper := List(Int{V: 1}, Int{V: 2}, Int{V: 3})
	if got := proper.String(); got != "(1 2 3)" {
		t.Errorf("proper list String() = %q, want (1 2 3)", got)
	}
	dotted := Cons{Car: Int{V: 1}, Cdr: Int{V: 2}}
	if got := dotted.String(); got != "(1 . 2)" {
		t.Errorf("dotted pair String() = %q, want (1 . 2)", got)
	}
}

func TestLambdaString(t *testing.T) {
	cases := []struct {
		name string
		lam  Lambda
		want string
	}{
		{"params-only", Lambda{Params: []string{"a", "b"}}, "#<lambda (a b)>"},
		{"rest-only", Lambda{Rest: "args"}, "#<lambda (&rest args)>"},
		{"params-and-rest", Lambda{Params: []string{"a"}, Rest: "rest"}, "#<lambda (a &rest rest)>"},
		{"no-params", Lambda{}, "#<lambda ()>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.lam.String(); got != tc.want {
				t.Errorf("Lambda.String() = %q, want %q", got, tc.want)
			}
		})
	}
}

// isValue is a no-op marker method; calling it confirms each type implements
// the Value interface and exercises the otherwise-uncovered method bodies.
func TestIsValueMarkers(t *testing.T) {
	vals := []Value{
		Nil{}, Bool{}, Int{}, Float{}, Symbol{}, StringVal{},
		Cons{}, Vector{}, Lambda{}, Builtin{},
	}
	for _, v := range vals {
		v.isValue()
	}
}

// -------------------------------------------------------------------
// parser — reader macros and nested error propagation
// -------------------------------------------------------------------

func TestParse_CommaAtProducesUnquoteSplicing(t *testing.T) {
	v, err := ParseOne(",@xs")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if got := v.String(); got != "(unquote-splicing xs)" {
		t.Fatalf("got %q, want (unquote-splicing xs)", got)
	}
}

func TestParse_NestedListErrorPropagates(t *testing.T) {
	// The inner list is unterminated; parseList must propagate the error
	// from the recursive parseForm rather than reporting its own.
	_, err := Parse("(1 (2")
	if err == nil {
		t.Fatal("expected error for unterminated inner list")
	}
}

func TestParse_DottedCdrErrorPropagates(t *testing.T) {
	// The cdr of the dotted pair is itself unparseable.
	_, err := Parse("(1 . (2")
	if err == nil {
		t.Fatal("expected error for unparseable dotted cdr")
	}
}

func TestParse_VectorElementErrorPropagates(t *testing.T) {
	_, err := Parse("[1 (2")
	if err == nil {
		t.Fatal("expected error for unparseable vector element")
	}
}

func TestParse_IntegerOutOfRangeErrors(t *testing.T) {
	// The lexer happily produces a TokenInt; strconv.ParseInt then overflows,
	// which parseAtom must report.
	_, err := Parse("99999999999999999999999999")
	if err == nil {
		t.Fatal("expected error for out-of-range integer")
	}
}

func TestParse_FloatOutOfRangeErrors(t *testing.T) {
	_, err := Parse("1e999999")
	if err == nil {
		t.Fatal("expected error for out-of-range float")
	}
}
