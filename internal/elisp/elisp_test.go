package elisp

import (
	"testing"
)

// -------------------------------------------------------------------
// Lexer tests
// -------------------------------------------------------------------

func TestLexer_SetqExpr(t *testing.T) {
	l := NewLexer("(setq x 42)")
	tokens := []Token{}
	for {
		tok := l.Next()
		tokens = append(tokens, tok)
		if tok.Type == TokenEOF {
			break
		}
	}

	expected := []struct {
		typ TokenType
		val string
	}{
		{TokenLParen, "("},
		{TokenSymbol, "setq"},
		{TokenSymbol, "x"},
		{TokenInt, "42"},
		{TokenRParen, ")"},
		{TokenEOF, ""},
	}

	if len(tokens) != len(expected) {
		t.Fatalf("expected %d tokens, got %d", len(expected), len(tokens))
	}
	for i, e := range expected {
		if tokens[i].Type != e.typ {
			t.Errorf("token[%d]: expected type %d, got %d", i, e.typ, tokens[i].Type)
		}
		if e.val != "" && tokens[i].Value != e.val {
			t.Errorf("token[%d]: expected value %q, got %q", i, e.val, tokens[i].Value)
		}
	}
}

func TestLexer_String(t *testing.T) {
	l := NewLexer(`"hello\nworld"`)
	tok := l.Next()
	if tok.Type != TokenString {
		t.Fatalf("expected TokenString, got %d", tok.Type)
	}
	if tok.Value != "hello\nworld" {
		t.Fatalf("expected hello\\nworld, got %q", tok.Value)
	}
}

func TestLexer_Comment(t *testing.T) {
	l := NewLexer("; this is a comment\n42")
	tok := l.Next()
	if tok.Type != TokenInt || tok.Value != "42" {
		t.Fatalf("expected int 42, got type=%d val=%q", tok.Type, tok.Value)
	}
}

func TestLexer_Float(t *testing.T) {
	l := NewLexer("3.14")
	tok := l.Next()
	if tok.Type != TokenFloat {
		t.Fatalf("expected TokenFloat, got %d", tok.Type)
	}
	if tok.Value != "3.14" {
		t.Fatalf("expected 3.14, got %q", tok.Value)
	}
}

func TestLexer_Quote(t *testing.T) {
	l := NewLexer("'foo")
	tok := l.Next()
	if tok.Type != TokenQuote {
		t.Fatalf("expected TokenQuote")
	}
	tok = l.Next()
	if tok.Type != TokenSymbol || tok.Value != "foo" {
		t.Fatalf("expected symbol foo")
	}
}

func TestLexer_CommaAt(t *testing.T) {
	l := NewLexer(",@x")
	tok := l.Next()
	if tok.Type != TokenCommaAt {
		t.Fatalf("expected TokenCommaAt, got %d", tok.Type)
	}
}

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
// Evaluator tests
// -------------------------------------------------------------------

func newEval() *Evaluator { return NewEvaluator() }

func TestEval_SetqAndRead(t *testing.T) {
	ev := newEval()
	_, err := ev.EvalString("(setq x 42)")
	if err != nil {
		t.Fatalf("setq error: %v", err)
	}
	val, err := ev.EvalString("x")
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	i, ok := val.(Int)
	if !ok || i.V != 42 {
		t.Fatalf("expected 42, got %v", val)
	}
}

func TestEval_Defun(t *testing.T) {
	ev := newEval()
	_, err := ev.EvalString("(defun square (x) (* x x))")
	if err != nil {
		t.Fatalf("defun error: %v", err)
	}
	val, err := ev.EvalString("(square 7)")
	if err != nil {
		t.Fatalf("call error: %v", err)
	}
	i, ok := val.(Int)
	if !ok || i.V != 49 {
		t.Fatalf("expected 49, got %v", val)
	}
}

func TestEval_IfTrue(t *testing.T) {
	ev := newEval()
	val, err := ev.EvalString("(if t 1 2)")
	if err != nil {
		t.Fatalf("if error: %v", err)
	}
	i, ok := val.(Int)
	if !ok || i.V != 1 {
		t.Fatalf("expected 1, got %v", val)
	}
}

func TestEval_IfFalse(t *testing.T) {
	ev := newEval()
	val, err := ev.EvalString("(if nil 1 2)")
	if err != nil {
		t.Fatalf("if error: %v", err)
	}
	i, ok := val.(Int)
	if !ok || i.V != 2 {
		t.Fatalf("expected 2, got %v", val)
	}
}

func TestEval_IfNoElse(t *testing.T) {
	ev := newEval()
	val, err := ev.EvalString("(if nil 1)")
	if err != nil {
		t.Fatalf("if error: %v", err)
	}
	if !IsNil(val) {
		t.Fatalf("expected nil, got %v", val)
	}
}

func TestEval_Let(t *testing.T) {
	ev := newEval()
	val, err := ev.EvalString("(let ((x 10) (y 20)) (+ x y))")
	if err != nil {
		t.Fatalf("let error: %v", err)
	}
	i, ok := val.(Int)
	if !ok || i.V != 30 {
		t.Fatalf("expected 30, got %v", val)
	}
}

func TestEval_LetStar(t *testing.T) {
	ev := newEval()
	// In let*, y can reference x
	val, err := ev.EvalString("(let* ((x 5) (y (* x 2))) y)")
	if err != nil {
		t.Fatalf("let* error: %v", err)
	}
	i, ok := val.(Int)
	if !ok || i.V != 10 {
		t.Fatalf("expected 10, got %v", val)
	}
}

func TestEval_Cond(t *testing.T) {
	ev := newEval()
	val, err := ev.EvalString(`
(cond
  ((= 1 2) "no")
  ((= 1 1) "yes")
  (t "else"))
`)
	if err != nil {
		t.Fatalf("cond error: %v", err)
	}
	s, ok := val.(StringVal)
	if !ok || s.V != "yes" {
		t.Fatalf("expected \"yes\", got %v", val)
	}
}

func TestEval_CondElse(t *testing.T) {
	ev := newEval()
	val, err := ev.EvalString(`
(cond
  ((= 1 2) "no")
  (t "else"))
`)
	if err != nil {
		t.Fatalf("cond error: %v", err)
	}
	s, ok := val.(StringVal)
	if !ok || s.V != "else" {
		t.Fatalf("expected \"else\", got %v", val)
	}
}

func TestEval_ArithAdd(t *testing.T) {
	ev := newEval()
	val, err := ev.EvalString("(+ 1 2)")
	if err != nil {
		t.Fatalf("+ error: %v", err)
	}
	i, ok := val.(Int)
	if !ok || i.V != 3 {
		t.Fatalf("expected 3, got %v", val)
	}
}

func TestEval_ArithMul(t *testing.T) {
	ev := newEval()
	val, err := ev.EvalString("(* 3 4)")
	if err != nil {
		t.Fatalf("* error: %v", err)
	}
	i, ok := val.(Int)
	if !ok || i.V != 12 {
		t.Fatalf("expected 12, got %v", val)
	}
}

func TestEval_StringConcat(t *testing.T) {
	ev := newEval()
	val, err := ev.EvalString(`(concat "hello" " " "world")`)
	if err != nil {
		t.Fatalf("concat error: %v", err)
	}
	s, ok := val.(StringVal)
	if !ok || s.V != "hello world" {
		t.Fatalf("expected \"hello world\", got %v", val)
	}
}

func TestEval_Lambda(t *testing.T) {
	ev := newEval()
	val, err := ev.EvalString("((lambda (x) (* x x)) 6)")
	if err != nil {
		t.Fatalf("lambda error: %v", err)
	}
	i, ok := val.(Int)
	if !ok || i.V != 36 {
		t.Fatalf("expected 36, got %v", val)
	}
}

func TestEval_LambdaViaFuncall(t *testing.T) {
	ev := newEval()
	_, err := ev.EvalString("(setq double (lambda (x) (* x 2)))")
	if err != nil {
		t.Fatalf("setq lambda error: %v", err)
	}
	val, err := ev.EvalString("(funcall double 5)")
	if err != nil {
		t.Fatalf("funcall error: %v", err)
	}
	i, ok := val.(Int)
	if !ok || i.V != 10 {
		t.Fatalf("expected 10, got %v", val)
	}
}

func TestEval_GlobalSetKeyAndKbd(t *testing.T) {
	ev := newEval()
	_, err := ev.EvalString(`(global-set-key (kbd "C-f") 'forward-char)`)
	if err != nil {
		t.Fatalf("global-set-key error: %v", err)
	}
	bindings := ev.GetKeyBindings()
	cmd, ok := bindings["C-f"]
	if !ok {
		t.Fatalf("binding for C-f not found")
	}
	if cmd != "forward-char" {
		t.Fatalf("expected forward-char, got %q", cmd)
	}
}

func TestEval_KbdReturnString(t *testing.T) {
	ev := newEval()
	val, err := ev.EvalString(`(kbd "M-x")`)
	if err != nil {
		t.Fatalf("kbd error: %v", err)
	}
	s, ok := val.(StringVal)
	if !ok || s.V != "M-x" {
		t.Fatalf("expected M-x, got %v", val)
	}
}

func TestEval_Mapcar(t *testing.T) {
	ev := newEval()
	val, err := ev.EvalString("(mapcar (lambda (x) (* x x)) '(1 2 3))")
	if err != nil {
		t.Fatalf("mapcar error: %v", err)
	}
	elems, ok := ToSlice(val)
	if !ok || len(elems) != 3 {
		t.Fatalf("expected list of 3, got %v", val)
	}
	expected := []int64{1, 4, 9}
	for i, e := range elems {
		iv, ok := e.(Int)
		if !ok || iv.V != expected[i] {
			t.Fatalf("elems[%d]: expected %d, got %v", i, expected[i], e)
		}
	}
}

func TestEval_Progn(t *testing.T) {
	ev := newEval()
	val, err := ev.EvalString("(progn 1 2 3)")
	if err != nil {
		t.Fatalf("progn error: %v", err)
	}
	i, ok := val.(Int)
	if !ok || i.V != 3 {
		t.Fatalf("expected 3, got %v", val)
	}
}

func TestEval_When(t *testing.T) {
	ev := newEval()
	val, err := ev.EvalString("(when t 42)")
	if err != nil {
		t.Fatalf("when error: %v", err)
	}
	i, ok := val.(Int)
	if !ok || i.V != 42 {
		t.Fatalf("expected 42, got %v", val)
	}
}

func TestEval_Unless(t *testing.T) {
	ev := newEval()
	val, err := ev.EvalString("(unless nil 99)")
	if err != nil {
		t.Fatalf("unless error: %v", err)
	}
	i, ok := val.(Int)
	if !ok || i.V != 99 {
		t.Fatalf("expected 99, got %v", val)
	}
}

func TestEval_AndOr(t *testing.T) {
	ev := newEval()

	// and returns last truthy value
	val, err := ev.EvalString("(and 1 2 3)")
	if err != nil {
		t.Fatalf("and error: %v", err)
	}
	i, ok := val.(Int)
	if !ok || i.V != 3 {
		t.Fatalf("and: expected 3, got %v", val)
	}

	// and short-circuits on nil
	val, err = ev.EvalString("(and 1 nil 3)")
	if err != nil {
		t.Fatalf("and error: %v", err)
	}
	if !IsNil(val) {
		t.Fatalf("and: expected nil, got %v", val)
	}

	// or returns first truthy
	val, err = ev.EvalString("(or nil 5 6)")
	if err != nil {
		t.Fatalf("or error: %v", err)
	}
	i, ok = val.(Int)
	if !ok || i.V != 5 {
		t.Fatalf("or: expected 5, got %v", val)
	}
}

func TestEval_Apply(t *testing.T) {
	ev := newEval()
	val, err := ev.EvalString("(apply + '(1 2 3))")
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	i, ok := val.(Int)
	if !ok || i.V != 6 {
		t.Fatalf("expected 6, got %v", val)
	}
}

func TestEval_Format(t *testing.T) {
	ev := newEval()
	val, err := ev.EvalString(`(format "hello %s, you are %d years old" "alice" 30)`)
	if err != nil {
		t.Fatalf("format error: %v", err)
	}
	s, ok := val.(StringVal)
	if !ok || s.V != "hello alice, you are 30 years old" {
		t.Fatalf("expected formatted string, got %q", val)
	}
}

func TestEval_NotAndNull(t *testing.T) {
	ev := newEval()
	val, err := ev.EvalString("(not nil)")
	if err != nil {
		t.Fatalf("not error: %v", err)
	}
	b, ok := val.(Bool)
	if !ok || !b.V {
		t.Fatalf("expected t, got %v", val)
	}

	val, err = ev.EvalString("(null '())")
	if err != nil {
		t.Fatalf("null error: %v", err)
	}
	b, ok = val.(Bool)
	if !ok || !b.V {
		t.Fatalf("expected t for null of empty list, got %v", val)
	}
}

func TestEval_Predicates(t *testing.T) {
	ev := newEval()

	tests := []struct {
		expr string
		want bool
	}{
		{`(stringp "hi")`, true},
		{`(stringp 42)`, false},
		{`(numberp 42)`, true},
		{`(numberp "hi")`, false},
		{`(symbolp 'foo)`, true},
		{`(listp '(1 2))`, true},
		{`(listp 42)`, false},
	}

	for _, tt := range tests {
		val, err := ev.EvalString(tt.expr)
		if err != nil {
			t.Fatalf("%s: error %v", tt.expr, err)
		}
		got := isTruthy(val)
		if got != tt.want {
			t.Errorf("%s: expected truthy=%v, got %v", tt.expr, tt.want, val)
		}
	}
}

func TestEval_ArithComparisons(t *testing.T) {
	ev := newEval()

	tests := []struct {
		expr string
		want bool
	}{
		{"(< 1 2)", true},
		{"(< 2 1)", false},
		{"(> 2 1)", true},
		{"(<= 1 1)", true},
		{"(>= 2 1)", true},
		{"(= 3 3)", true},
		{"(= 3 4)", false},
	}

	for _, tt := range tests {
		val, err := ev.EvalString(tt.expr)
		if err != nil {
			t.Fatalf("%s: error %v", tt.expr, err)
		}
		got := isTruthy(val)
		if got != tt.want {
			t.Errorf("%s: expected %v, got %v", tt.expr, tt.want, val)
		}
	}
}

func TestEval_CarCdrCons(t *testing.T) {
	ev := newEval()

	val, err := ev.EvalString("(car '(1 2 3))")
	if err != nil {
		t.Fatalf("car error: %v", err)
	}
	i, ok := val.(Int)
	if !ok || i.V != 1 {
		t.Fatalf("car: expected 1, got %v", val)
	}

	val, err = ev.EvalString("(cdr '(1 2 3))")
	if err != nil {
		t.Fatalf("cdr error: %v", err)
	}
	elems, ok := ToSlice(val)
	if !ok || len(elems) != 2 {
		t.Fatalf("cdr: expected list of 2, got %v", val)
	}

	val, err = ev.EvalString("(cons 1 '(2 3))")
	if err != nil {
		t.Fatalf("cons error: %v", err)
	}
	elems, ok = ToSlice(val)
	if !ok || len(elems) != 3 {
		t.Fatalf("cons: expected list of 3, got %v", val)
	}
}

func TestEval_Defvar(t *testing.T) {
	ev := newEval()
	_, err := ev.EvalString("(defvar my-var 100)")
	if err != nil {
		t.Fatalf("defvar error: %v", err)
	}
	val, err := ev.EvalString("my-var")
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	i, ok := val.(Int)
	if !ok || i.V != 100 {
		t.Fatalf("expected 100, got %v", val)
	}

	// defvar should not overwrite existing binding
	_, err = ev.EvalString("(defvar my-var 999)")
	if err != nil {
		t.Fatalf("defvar overwrite error: %v", err)
	}
	val, err = ev.EvalString("my-var")
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	i, ok = val.(Int)
	if !ok || i.V != 100 {
		t.Fatalf("defvar should not overwrite: expected 100, got %v", val)
	}
}

func TestEval_KeywordSymbolSelfQuoting(t *testing.T) {
	ev := newEval()
	val, err := ev.EvalString(":foreground")
	if err != nil {
		t.Fatalf("keyword symbol error: %v", err)
	}
	sym, ok := val.(Symbol)
	if !ok {
		t.Fatalf("expected Symbol, got %T", val)
	}
	if sym.Name != ":foreground" {
		t.Errorf("Name = %q, want %q", sym.Name, ":foreground")
	}
}

func TestEval_KeywordSymbolInList(t *testing.T) {
	ev := newEval()
	// Simulate what set-face-attribute receives: (:foreground "#abc")
	var captured []Value
	ev.RegisterGoFn("my-fn", func(args []Value, _ *Env) (Value, error) {
		captured = args
		return Nil{}, nil
	})
	_, err := ev.EvalString(`(my-fn :foreground "#abc" :bold t)`)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(captured) != 4 {
		t.Fatalf("expected 4 args, got %d", len(captured))
	}
	kw, ok := captured[0].(Symbol)
	if !ok || kw.Name != ":foreground" {
		t.Errorf("arg[0] = %v, want :foreground", captured[0])
	}
	s, ok := captured[1].(StringVal)
	if !ok || s.V != "#abc" {
		t.Errorf("arg[1] = %v, want \"#abc\"", captured[1])
	}
}

func TestEval_SetqHook(t *testing.T) {
	ev := newEval()
	var hooked Value
	ev.SetSetqHook("theme", func(v Value) { hooked = v })

	_, err := ev.EvalString("(setq theme 'sweet)")
	if err != nil {
		t.Fatalf("setq error: %v", err)
	}
	if hooked == nil {
		t.Fatal("hook was not called")
	}
	sym, ok := hooked.(Symbol)
	if !ok || sym.Name != "sweet" {
		t.Errorf("hooked value = %v (%T), want Symbol{sweet}", hooked, hooked)
	}
}

func TestEval_SetqHookNotCalledForOtherVars(t *testing.T) {
	ev := newEval()
	called := false
	ev.SetSetqHook("theme", func(_ Value) { called = true })

	_, err := ev.EvalString("(setq other-var 42)")
	if err != nil {
		t.Fatalf("setq error: %v", err)
	}
	if called {
		t.Error("hook called for unrelated variable")
	}
}
