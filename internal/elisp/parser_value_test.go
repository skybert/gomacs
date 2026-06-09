package elisp

import "testing"

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
