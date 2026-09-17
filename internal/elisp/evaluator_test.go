package elisp

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// Tests for evaluator.go: Env, the Evaluator, special forms and builtins.

// -------------------------------------------------------------------
// Evaluator tests
// -------------------------------------------------------------------

func newEval() *Evaluator { return NewEvaluator() }

func mustEval(t *testing.T, ev *Evaluator, src string) Value {
	t.Helper()
	v, err := ev.EvalString(src)
	if err != nil {
		t.Fatalf("EvalString(%q): %v", src, err)
	}
	return v
}

// wantErr evaluates src and fails unless an error is returned.
func wantErr(t *testing.T, ev *Evaluator, src string) {
	t.Helper()
	if _, err := ev.EvalString(src); err == nil {
		t.Fatalf("EvalString(%q): expected error, got nil", src)
	}
}

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

// -------------------------------------------------------------------
// Arithmetic: -, /
// -------------------------------------------------------------------

func TestEval_ArithSub(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(- 10 3)")
	i, ok := v.(Int)
	if !ok || i.V != 7 {
		t.Fatalf("expected 7, got %v", v)
	}
}

func TestEval_ArithSubNegate(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(- 5)")
	i, ok := v.(Int)
	if !ok || i.V != -5 {
		t.Fatalf("expected -5, got %v", v)
	}
}

func TestEval_ArithSubChained(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(- 20 3 4 5)")
	i, ok := v.(Int)
	if !ok || i.V != 8 {
		t.Fatalf("expected 8, got %v", v)
	}
}

func TestEval_ArithDiv(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(/ 20 4)")
	i, ok := v.(Int)
	if !ok || i.V != 5 {
		t.Fatalf("expected 5, got %v", v)
	}
}

func TestEval_ArithDivByZeroPanics(t *testing.T) {
	ev := newEval()
	// Division by zero panics inside the builtin; the evaluator does not
	// catch it, so we use recover to verify the panic occurs.
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for division by zero, got none")
		}
	}()
	_, _ = ev.EvalString("(/ 1 0)") //nolint:errcheck
}

func TestEval_ArithFloat(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(+ 1.5 2.5)")
	f, ok := v.(Float)
	if !ok || f.V != 4.0 {
		t.Fatalf("expected 4.0, got %v", v)
	}
}

func TestEval_ArithAddNoArgs(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(+)")
	i, ok := v.(Int)
	if !ok || i.V != 0 {
		t.Fatalf("expected 0, got %v", v)
	}
}

func TestEval_ArithMulNoArgs(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(*)")
	i, ok := v.(Int)
	if !ok || i.V != 1 {
		t.Fatalf("expected 1, got %v", v)
	}
}

// -------------------------------------------------------------------
// eq and equal (deep equality)
// -------------------------------------------------------------------

func TestEval_EqIntegers(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(eq 42 42)")
	if !isTruthy(v) {
		t.Fatalf("expected t for (eq 42 42), got %v", v)
	}
}

func TestEval_EqIntegersDiffer(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(eq 1 2)")
	if isTruthy(v) {
		t.Fatalf("expected nil for (eq 1 2), got %v", v)
	}
}

func TestEval_EqSymbols(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(eq 'foo 'foo)")
	if !isTruthy(v) {
		t.Fatalf("expected t for (eq 'foo 'foo), got %v", v)
	}
}

func TestEval_EqualDeepLists(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(equal '(1 2 3) '(1 2 3))")
	if !isTruthy(v) {
		t.Fatalf("expected t for equal lists, got %v", v)
	}
}

func TestEval_EqualDeepListsDiffer(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(equal '(1 2 3) '(1 2 4))")
	if isTruthy(v) {
		t.Fatalf("expected nil for unequal lists, got %v", v)
	}
}

func TestEval_EqualNestedLists(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(equal '(1 (2 3)) '(1 (2 3)))")
	if !isTruthy(v) {
		t.Fatalf("expected t for nested equal lists, got %v", v)
	}
}

func TestEval_EqualStrings(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, `(equal "abc" "abc")`)
	if !isTruthy(v) {
		t.Fatalf("expected t for equal strings, got %v", v)
	}
}

// -------------------------------------------------------------------
// not / null edge cases
// -------------------------------------------------------------------

func TestEval_NotTrue(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(not t)")
	if isTruthy(v) {
		t.Fatalf("expected nil for (not t), got %v", v)
	}
}

func TestEval_NotNonNilValue(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(not 42)")
	if isTruthy(v) {
		t.Fatalf("expected nil for (not 42), got %v", v)
	}
}

func TestEval_NullEmptyList(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(null '())")
	if !isTruthy(v) {
		t.Fatalf("expected t for (null '()), got %v", v)
	}
}

func TestEval_NullNonEmptyList(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(null '(1))")
	if isTruthy(v) {
		t.Fatalf("expected nil for (null '(1)), got %v", v)
	}
}

// -------------------------------------------------------------------
// length on list and string
// -------------------------------------------------------------------

func TestEval_LengthList(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(length '(a b c d))")
	i, ok := v.(Int)
	if !ok || i.V != 4 {
		t.Fatalf("expected 4, got %v", v)
	}
}

func TestEval_LengthEmptyList(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(length '())")
	i, ok := v.(Int)
	if !ok || i.V != 0 {
		t.Fatalf("expected 0, got %v", v)
	}
}

func TestEval_LengthString(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, `(length "hello")`)
	i, ok := v.(Int)
	if !ok || i.V != 5 {
		t.Fatalf("expected 5, got %v", v)
	}
}

func TestEval_LengthStringUnicode(t *testing.T) {
	ev := newEval()
	// "abc" with a two-byte rune — length counts runes, not bytes
	v := mustEval(t, ev, `(length "héllo")`)
	i, ok := v.(Int)
	if !ok || i.V != 5 {
		t.Fatalf("expected 5 runes, got %v", v)
	}
}

// -------------------------------------------------------------------
// string-to-int / int-to-string / number-to-string
// -------------------------------------------------------------------

func TestEval_StringToInt(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, `(string-to-int "123")`)
	i, ok := v.(Int)
	if !ok || i.V != 123 {
		t.Fatalf("expected 123, got %v", v)
	}
}

func TestEval_IntToString(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(int-to-string 42)")
	s, ok := v.(StringVal)
	if !ok || s.V != "42" {
		t.Fatalf("expected \"42\", got %v", v)
	}
}

func TestEval_NumberToStringInt(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(number-to-string 99)")
	s, ok := v.(StringVal)
	if !ok || s.V != "99" {
		t.Fatalf("expected \"99\", got %v", v)
	}
}

func TestEval_NumberToStringFloat(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(number-to-string 3.14)")
	s, ok := v.(StringVal)
	if !ok || s.V != "3.14" {
		t.Fatalf("expected \"3.14\", got %v", v)
	}
}

// -------------------------------------------------------------------
// format with %f
// -------------------------------------------------------------------

func TestEval_FormatFloat(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, `(format "%f" 2.5)`)
	s, ok := v.(StringVal)
	if !ok {
		t.Fatalf("expected StringVal, got %T", v)
	}
	// strconv.FormatFloat with 'f', 6 decimal places
	if s.V != "2.500000" {
		t.Fatalf("expected \"2.500000\", got %q", s.V)
	}
}

func TestEval_FormatPercent(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, `(format "100%%")`)
	s, ok := v.(StringVal)
	if !ok || s.V != "100%" {
		t.Fatalf("expected \"100%%\", got %v", v)
	}
}

// -------------------------------------------------------------------
// functionp
// -------------------------------------------------------------------

func TestEval_FunctionpLambda(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(functionp (lambda (x) x))")
	if !isTruthy(v) {
		t.Fatalf("expected t for lambda, got %v", v)
	}
}

func TestEval_FunctionpBuiltin(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(functionp #'+)")
	if !isTruthy(v) {
		t.Fatalf("expected t for (functionp #'+), got %v", v)
	}
	mustEval(t, ev, "(setq fn (lambda (x) x))")
	v = mustEval(t, ev, "(functionp fn)")
	if !isTruthy(v) {
		t.Fatalf("expected t for stored lambda, got %v", v)
	}
}

func TestEval_FunctionpNonFunction(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(functionp 42)")
	if isTruthy(v) {
		t.Fatalf("expected nil for (functionp 42), got %v", v)
	}
}

// -------------------------------------------------------------------
// car / cdr on nil returns nil (not an error)
// -------------------------------------------------------------------

func TestEval_CarNilReturnsNil(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(car nil)")
	if !IsNil(v) {
		t.Fatalf("expected nil for (car nil), got %v", v)
	}
}

func TestEval_CdrNilReturnsNil(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(cdr nil)")
	if !IsNil(v) {
		t.Fatalf("expected nil for (cdr nil), got %v", v)
	}
}

// -------------------------------------------------------------------
// Error cases
// -------------------------------------------------------------------

func TestEval_CarOfNonListErrors(t *testing.T) {
	ev := newEval()
	_, err := ev.EvalString("(car 42)")
	if err == nil {
		t.Fatal("expected error for (car 42), got nil")
	}
}

func TestEval_UndefinedSymbolErrors(t *testing.T) {
	ev := newEval()
	_, err := ev.EvalString("undefined-symbol-xyz")
	if err == nil {
		t.Fatal("expected error for undefined symbol, got nil")
	}
}

func TestEval_DivTooFewArgsErrors(t *testing.T) {
	ev := newEval()
	_, err := ev.EvalString("(/ 5)")
	if err == nil {
		t.Fatal("expected error for (/ 5) with only one arg, got nil")
	}
}

// -------------------------------------------------------------------
// Globals: GetGlobalVar and GlobalVarNames
// -------------------------------------------------------------------

func TestEval_GetGlobalVar(t *testing.T) {
	ev := newEval()
	mustEval(t, ev, "(setq my-config 42)")
	v, ok := ev.GetGlobalVar("my-config")
	if !ok {
		t.Fatal("GetGlobalVar: variable not found")
	}
	i, isInt := v.(Int)
	if !isInt || i.V != 42 {
		t.Fatalf("expected 42, got %v", v)
	}
}

func TestEval_GlobalVarNamesContains(t *testing.T) {
	ev := newEval()
	mustEval(t, ev, "(setq foo 42)")
	mustEval(t, ev, "(setq bar 99)")
	names := ev.GlobalVarNames()
	if !slices.Contains(names, "foo") {
		t.Errorf("GlobalVarNames does not contain \"foo\"; got %v", names)
	}
	if !slices.Contains(names, "bar") {
		t.Errorf("GlobalVarNames does not contain \"bar\"; got %v", names)
	}
}

func TestEval_GetGlobalVarMissing(t *testing.T) {
	ev := newEval()
	_, ok := ev.GetGlobalVar("no-such-variable")
	if ok {
		t.Fatal("expected ok=false for missing variable")
	}
}

// -------------------------------------------------------------------
// EvalFile
// -------------------------------------------------------------------

func TestEval_EvalFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "init.el")
	content := "(setq loaded-from-file 777)\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ev := newEval()
	if err := ev.EvalFile(path); err != nil {
		t.Fatalf("EvalFile: %v", err)
	}
	v, ok := ev.GetGlobalVar("loaded-from-file")
	if !ok {
		t.Fatal("variable not set after EvalFile")
	}
	i, isInt := v.(Int)
	if !isInt || i.V != 777 {
		t.Fatalf("expected 777, got %v", v)
	}
}

func TestEval_EvalFileMissingReturnsError(t *testing.T) {
	ev := newEval()
	err := ev.EvalFile("/nonexistent/path/to/file.el")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// -------------------------------------------------------------------
// RegisterGoFn
// -------------------------------------------------------------------

func TestEval_RegisterGoFn(t *testing.T) {
	ev := newEval()
	ev.RegisterGoFn("my-add", func(args []Value, _ *Env) (Value, error) {
		a := args[0].(Int).V
		b := args[1].(Int).V
		return Int{V: a + b}, nil
	})
	v := mustEval(t, ev, "(my-add 10 32)")
	i, ok := v.(Int)
	if !ok || i.V != 42 {
		t.Fatalf("expected 42, got %v", v)
	}
}

func TestEval_RegisterGoFnReturnsString(t *testing.T) {
	ev := newEval()
	ev.RegisterGoFn("greet", func(args []Value, _ *Env) (Value, error) {
		name := args[0].(StringVal).V
		return StringVal{V: "Hello, " + name + "!"}, nil
	})
	v := mustEval(t, ev, `(greet "world")`)
	s, ok := v.(StringVal)
	if !ok || s.V != "Hello, world!" {
		t.Fatalf("expected \"Hello, world!\", got %v", v)
	}
}

// -------------------------------------------------------------------
// Recursive functions
// -------------------------------------------------------------------

func TestEval_RecursiveFactorial(t *testing.T) {
	ev := newEval()
	mustEval(t, ev, `
(defun factorial (n)
  (if (= n 0)
      1
      (* n (factorial (- n 1)))))
`)
	v := mustEval(t, ev, "(factorial 6)")
	i, ok := v.(Int)
	if !ok || i.V != 720 {
		t.Fatalf("expected 720, got %v", v)
	}
}

func TestEval_RecursiveFibonacci(t *testing.T) {
	ev := newEval()
	mustEval(t, ev, `
(defun fib (n)
  (if (< n 2)
      n
      (+ (fib (- n 1)) (fib (- n 2)))))
`)
	v := mustEval(t, ev, "(fib 10)")
	i, ok := v.(Int)
	if !ok || i.V != 55 {
		t.Fatalf("expected 55, got %v", v)
	}
}

// -------------------------------------------------------------------
// Closures
// -------------------------------------------------------------------

func TestEval_ClosureCapturesOuter(t *testing.T) {
	ev := newEval()
	// make-adder returns a closure that captures 'n' from the enclosing let
	mustEval(t, ev, `
(defun make-adder (n)
  (lambda (x) (+ n x)))
`)
	mustEval(t, ev, "(setq add5 (make-adder 5))")
	v := mustEval(t, ev, "(funcall add5 10)")
	i, ok := v.(Int)
	if !ok || i.V != 15 {
		t.Fatalf("expected 15, got %v", v)
	}
}

func TestEval_ClosureIndependentCaptures(t *testing.T) {
	ev := newEval()
	mustEval(t, ev, `
(defun make-adder (n)
  (lambda (x) (+ n x)))
`)
	mustEval(t, ev, "(setq add3 (make-adder 3))")
	mustEval(t, ev, "(setq add7 (make-adder 7))")
	v3 := mustEval(t, ev, "(funcall add3 10)")
	v7 := mustEval(t, ev, "(funcall add7 10)")
	i3, ok3 := v3.(Int)
	i7, ok7 := v7.(Int)
	if !ok3 || i3.V != 13 {
		t.Fatalf("add3: expected 13, got %v", v3)
	}
	if !ok7 || i7.V != 17 {
		t.Fatalf("add7: expected 17, got %v", v7)
	}
}

// -------------------------------------------------------------------
// &rest parameters
// -------------------------------------------------------------------

func TestEval_RestParams(t *testing.T) {
	ev := newEval()
	mustEval(t, ev, `(defun my-list (&rest args) args)`)
	v := mustEval(t, ev, "(my-list 1 2 3)")
	elems, ok := ToSlice(v)
	if !ok || len(elems) != 3 {
		t.Fatalf("expected list of 3, got %v", v)
	}
	for idx, want := range []int64{1, 2, 3} {
		iv, isInt := elems[idx].(Int)
		if !isInt || iv.V != want {
			t.Fatalf("elems[%d]: expected %d, got %v", idx, want, elems[idx])
		}
	}
}

func TestEval_RestParamsEmpty(t *testing.T) {
	ev := newEval()
	mustEval(t, ev, `(defun my-list (&rest args) args)`)
	v := mustEval(t, ev, "(my-list)")
	if !IsNil(v) {
		t.Fatalf("expected nil for empty &rest, got %v", v)
	}
}

func TestEval_RestParamsMixed(t *testing.T) {
	ev := newEval()
	mustEval(t, ev, `(defun head-and-rest (h &rest tail) (cons h tail))`)
	v := mustEval(t, ev, "(head-and-rest 1 2 3 4)")
	elems, ok := ToSlice(v)
	if !ok || len(elems) != 4 {
		t.Fatalf("expected list of 4, got %v", v)
	}
}

// -------------------------------------------------------------------
// quasiquote / unquote
// -------------------------------------------------------------------

func TestEval_Quasiquote(t *testing.T) {
	ev := newEval()
	mustEval(t, ev, "(setq x 10)")
	v := mustEval(t, ev, "`(a ,x c)")
	elems, ok := ToSlice(v)
	if !ok || len(elems) != 3 {
		t.Fatalf("expected list of 3, got %v", v)
	}
	sym, ok := elems[0].(Symbol)
	if !ok || sym.Name != "a" {
		t.Fatalf("elems[0]: expected 'a, got %v", elems[0])
	}
	i, ok := elems[1].(Int)
	if !ok || i.V != 10 {
		t.Fatalf("elems[1]: expected 10, got %v", elems[1])
	}
	sym2, ok := elems[2].(Symbol)
	if !ok || sym2.Name != "c" {
		t.Fatalf("elems[2]: expected 'c, got %v", elems[2])
	}
}

// -------------------------------------------------------------------
// setq with multiple pairs
// -------------------------------------------------------------------

func TestEval_SetqMultiplePairs(t *testing.T) {
	ev := newEval()
	mustEval(t, ev, "(setq a 1 b 2 c 3)")
	for name, want := range map[string]int64{"a": 1, "b": 2, "c": 3} {
		v, ok := ev.GetGlobalVar(name)
		if !ok {
			t.Fatalf("variable %q not found", name)
		}
		i, isInt := v.(Int)
		if !isInt || i.V != want {
			t.Fatalf("%s: expected %d, got %v", name, want, v)
		}
	}
}

// -------------------------------------------------------------------
// cond with no matching clause
// -------------------------------------------------------------------

func TestEval_CondNoMatch(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(cond ((= 1 2) \"no\"))")
	if !IsNil(v) {
		t.Fatalf("expected nil when no cond clause matches, got %v", v)
	}
}

// -------------------------------------------------------------------
// when returns nil on false condition
// -------------------------------------------------------------------

func TestEval_WhenFalse(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(when nil 42)")
	if !IsNil(v) {
		t.Fatalf("expected nil for (when nil 42), got %v", v)
	}
}

// -------------------------------------------------------------------
// unless returns nil on true condition
// -------------------------------------------------------------------

func TestEval_UnlessTrue(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(unless t 99)")
	if !IsNil(v) {
		t.Fatalf("expected nil for (unless t 99), got %v", v)
	}
}

// -------------------------------------------------------------------
// and / or with no arguments
// -------------------------------------------------------------------

func TestEval_AndNoArgs(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(and)")
	// (and) with no args returns t
	if !isTruthy(v) {
		t.Fatalf("expected truthy for (and), got %v", v)
	}
}

func TestEval_OrNoArgs(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(or)")
	if isTruthy(v) {
		t.Fatalf("expected nil for (or), got %v", v)
	}
}

// -------------------------------------------------------------------
// require (no-op built-in)
// -------------------------------------------------------------------

func TestEval_RequireNoOp(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(require 'cl-lib)")
	// require returns its argument
	sym, ok := v.(Symbol)
	if !ok || sym.Name != "cl-lib" {
		t.Fatalf("expected symbol cl-lib, got %v", v)
	}
}

// -------------------------------------------------------------------
// list built-in
// -------------------------------------------------------------------

func TestEval_ListBuiltin(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(list 1 2 3)")
	elems, ok := ToSlice(v)
	if !ok || len(elems) != 3 {
		t.Fatalf("expected list of 3, got %v", v)
	}
}

func TestEval_ListEmpty(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(list)")
	if !IsNil(v) {
		t.Fatalf("expected nil for (list), got %v", v)
	}
}

// -------------------------------------------------------------------
// cons builds a dotted pair when cdr is not a list
// -------------------------------------------------------------------

func TestEval_ConsDottedPair(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(cons 1 2)")
	c, ok := v.(Cons)
	if !ok {
		t.Fatalf("expected Cons, got %T", v)
	}
	car, ok := c.Car.(Int)
	if !ok || car.V != 1 {
		t.Fatalf("car: expected 1, got %v", c.Car)
	}
	cdr, ok := c.Cdr.(Int)
	if !ok || cdr.V != 2 {
		t.Fatalf("cdr: expected 2, got %v", c.Cdr)
	}
}

// -------------------------------------------------------------------
// apply with prefix args
// -------------------------------------------------------------------

func TestEval_ApplyWithPrefix(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(apply + 1 2 '(3 4))")
	i, ok := v.(Int)
	if !ok || i.V != 10 {
		t.Fatalf("expected 10, got %v", v)
	}
}

// -------------------------------------------------------------------
// mapcar with defun
// -------------------------------------------------------------------

func TestEval_MapcarWithDefun(t *testing.T) {
	ev := newEval()
	mustEval(t, ev, "(defun double (x) (* x 2))")
	v := mustEval(t, ev, "(mapcar #'double '(1 2 3 4))")
	elems, ok := ToSlice(v)
	if !ok || len(elems) != 4 {
		t.Fatalf("expected list of 4, got %v", v)
	}
	for idx, want := range []int64{2, 4, 6, 8} {
		iv, isInt := elems[idx].(Int)
		if !isInt || iv.V != want {
			t.Fatalf("elems[%d]: expected %d, got %v", idx, want, elems[idx])
		}
	}
}

// -------------------------------------------------------------------
// while loop
// -------------------------------------------------------------------

func TestEval_While(t *testing.T) {
	ev := newEval()
	// Accumulate sum 1+2+3+4+5 = 15 using while
	mustEval(t, ev, `
(setq i 1)
(setq sum 0)
(while (<= i 5)
  (setq sum (+ sum i))
  (setq i (+ i 1)))
`)
	v, ok := ev.GetGlobalVar("sum")
	if !ok {
		t.Fatal("sum not found")
	}
	i, isInt := v.(Int)
	if !isInt || i.V != 15 {
		t.Fatalf("expected sum=15, got %v", v)
	}
}

func TestEval_WhileFalseCondition(t *testing.T) {
	ev := newEval()
	mustEval(t, ev, "(setq ran nil)")
	mustEval(t, ev, "(while nil (setq ran t))")
	v, ok := ev.GetGlobalVar("ran")
	if !ok {
		t.Fatal("ran not found")
	}
	if isTruthy(v) {
		t.Fatal("while body should not run when condition is false")
	}
}

// -------------------------------------------------------------------
// let with bare symbol (no initial value)
// -------------------------------------------------------------------

func TestEval_LetBareSymbol(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(let (x) x)")
	if !IsNil(v) {
		t.Fatalf("expected nil for uninitialized let binding, got %v", v)
	}
}

// -------------------------------------------------------------------
// Vector evaluation
// -------------------------------------------------------------------

func TestEval_Vector(t *testing.T) {
	ev := newEval()
	mustEval(t, ev, "(setq n 7)")
	v := mustEval(t, ev, "[1 n 3]")
	vec, ok := v.(Vector)
	if !ok {
		t.Fatalf("expected Vector, got %T", v)
	}
	if len(vec.Elems) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(vec.Elems))
	}
	i, ok := vec.Elems[1].(Int)
	if !ok || i.V != 7 {
		t.Fatalf("vec[1]: expected 7 (from n), got %v", vec.Elems[1])
	}
}

// -------------------------------------------------------------------
// funcall with builtin
// -------------------------------------------------------------------

func TestEval_FuncallBuiltin(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(funcall #'+ 3 4)")
	i, ok := v.(Int)
	if !ok || i.V != 7 {
		t.Fatalf("expected 7, got %v", v)
	}
}

// -------------------------------------------------------------------
// defvar does not overwrite existing value
// -------------------------------------------------------------------

func TestEval_DefvarNoOverwrite(t *testing.T) {
	ev := newEval()
	mustEval(t, ev, "(setq counter 10)")
	mustEval(t, ev, "(defvar counter 999)")
	v, _ := ev.GetGlobalVar("counter")
	i, ok := v.(Int)
	if !ok || i.V != 10 {
		t.Fatalf("defvar should not overwrite: expected 10, got %v", v)
	}
}

// -------------------------------------------------------------------
// EvalFile with invalid UTF-8 returns error
// -------------------------------------------------------------------

func TestEval_EvalFileInvalidUTF8(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.el")
	// Write raw bytes that are not valid UTF-8
	if err := os.WriteFile(path, []byte{0xff, 0xfe}, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	ev := newEval()
	err := ev.EvalFile(path)
	if err == nil {
		t.Fatal("expected error for invalid UTF-8 file, got nil")
	}
}

// -------------------------------------------------------------------
// setq hook fires with correct value type
// -------------------------------------------------------------------

func TestEval_SetqHookReceivesString(t *testing.T) {
	ev := newEval()
	var received Value
	ev.SetSetqHook("fill-column", func(v Value) { received = v })
	mustEval(t, ev, "(setq fill-column 80)")
	i, ok := received.(Int)
	if !ok || i.V != 80 {
		t.Fatalf("expected Int{80}, got %v (%T)", received, received)
	}
}

// -------------------------------------------------------------------
// equalValues — deep structural equality
// -------------------------------------------------------------------

func TestEval_EqualLists(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(equal '(1 2 3) '(1 2 3))")
	if _, ok := v.(Bool); !ok {
		t.Fatalf("expected Bool t, got %v (%T)", v, v)
	}
}

func TestEval_EqualListsDiffer(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(equal '(1 2 3) '(1 2 4))")
	if !IsNil(v) {
		t.Fatalf("expected nil, got %v", v)
	}
}

func TestEval_EqualVectors(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(equal [1 2 3] [1 2 3])")
	if _, ok := v.(Bool); !ok {
		t.Fatalf("expected Bool t, got %v (%T)", v, v)
	}
}

func TestEval_EqualVectorsDifferLength(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(equal [1 2] [1 2 3])")
	if !IsNil(v) {
		t.Fatalf("expected nil, got %v", v)
	}
}

func TestEval_EqualStringsSameContent(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, `(equal "hello" "hello")`)
	if _, ok := v.(Bool); !ok {
		t.Fatalf("expected Bool t, got %v (%T)", v, v)
	}
}

func TestEval_EqualStringsDiffer(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, `(equal "hello" "world")`)
	if !IsNil(v) {
		t.Fatalf("expected nil, got %v", v)
	}
}

func TestEval_EqualNilNil(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(equal nil nil)")
	if _, ok := v.(Bool); !ok {
		t.Fatalf("expected Bool t, got %v (%T)", v, v)
	}
}

func TestEval_EqualTT(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(equal t t)")
	if _, ok := v.(Bool); !ok {
		t.Fatalf("expected Bool t, got %v (%T)", v, v)
	}
}

func TestEval_EqualInts(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(equal 1 1)")
	if _, ok := v.(Bool); !ok {
		t.Fatalf("expected Bool t, got %v (%T)", v, v)
	}
}

func TestEval_EqualIntsDiffer(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(equal 1 2)")
	if !IsNil(v) {
		t.Fatalf("expected nil, got %v", v)
	}
}

// -------------------------------------------------------------------
// eqValues — pointer/identity equality
// -------------------------------------------------------------------

func TestEval_EqNilNil(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(eq nil nil)")
	if _, ok := v.(Bool); !ok {
		t.Fatalf("expected Bool t, got %v (%T)", v, v)
	}
}

func TestEval_EqTT(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(eq t t)")
	if _, ok := v.(Bool); !ok {
		t.Fatalf("expected Bool t, got %v (%T)", v, v)
	}
}

func TestEval_EqSameInt(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(eq 1 1)")
	if _, ok := v.(Bool); !ok {
		t.Fatalf("expected Bool t, got %v (%T)", v, v)
	}
}

func TestEval_EqDifferentInts(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(eq 1 2)")
	if !IsNil(v) {
		t.Fatalf("expected nil, got %v", v)
	}
}

func TestEval_EqStrings(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, `(eq "a" "a")`)
	if _, ok := v.(Bool); !ok {
		t.Fatalf("expected Bool t, got %v (%T)", v, v)
	}
}

func TestEval_EqSameSymbol(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(eq 'foo 'foo)")
	if _, ok := v.(Bool); !ok {
		t.Fatalf("expected Bool t, got %v (%T)", v, v)
	}
}

func TestEval_EqDifferentSymbols(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(eq 'foo 'bar)")
	if !IsNil(v) {
		t.Fatalf("expected nil, got %v", v)
	}
}

// -------------------------------------------------------------------
// numericFoldFrom — used by -, / when mixing ints and floats
// -------------------------------------------------------------------

func TestEval_SubFloatInt(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(- 5.0 1.5)")
	f, ok := v.(Float)
	if !ok || f.V != 3.5 {
		t.Fatalf("expected 3.5, got %v", v)
	}
}

func TestEval_SubIntFloat(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(- 5 1.0)")
	f, ok := v.(Float)
	if !ok || f.V != 4.0 {
		t.Fatalf("expected 4.0, got %v", v)
	}
}

func TestEval_DivFloatInt(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(/ 9.0 3)")
	f, ok := v.(Float)
	if !ok || f.V != 3.0 {
		t.Fatalf("expected 3.0, got %v", v)
	}
}

func TestEval_NumericFoldFromStringError(t *testing.T) {
	ev := newEval()
	_, err := ev.EvalString(`(- "hello" 1)`)
	if err == nil {
		t.Fatal("expected error for (- string int), got nil")
	}
}

// -------------------------------------------------------------------
// evalFunction — (function ...) special form / #'
// -------------------------------------------------------------------

func TestEval_FunctionBuiltin(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(functionp #'car)")
	if _, ok := v.(Bool); !ok {
		t.Fatalf("expected Bool t from (functionp #'car), got %v (%T)", v, v)
	}
}

func TestEval_FunctionLambda(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(function (lambda (x) (* x x)))")
	if _, ok := v.(Lambda); !ok {
		t.Fatalf("expected Lambda, got %v (%T)", v, v)
	}
}

func TestEval_FunctionVoidError(t *testing.T) {
	ev := newEval()
	_, err := ev.EvalString("#'nonexistent-fn")
	if err == nil {
		t.Fatal("expected error for void function reference, got nil")
	}
}

// -------------------------------------------------------------------
// evalOr — short-circuit cases
// -------------------------------------------------------------------

func TestEval_OrAllNil(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(or nil nil nil)")
	if !IsNil(v) {
		t.Fatalf("expected nil, got %v", v)
	}
}

func TestEval_OrReturnsFirstTruthy(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, `(or nil 42 "never")`)
	i, ok := v.(Int)
	if !ok || i.V != 42 {
		t.Fatalf("expected 42, got %v (%T)", v, v)
	}
}

// -------------------------------------------------------------------
// evalLambda — lambda as first-class value
// -------------------------------------------------------------------

func TestEval_LambdaFuncall(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(let ((f (lambda (x) (* x 2)))) (funcall f 5))")
	i, ok := v.(Int)
	if !ok || i.V != 10 {
		t.Fatalf("expected 10, got %v", v)
	}
}

func TestEval_LambdaNoParams(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "((lambda () 42))")
	i, ok := v.(Int)
	if !ok || i.V != 42 {
		t.Fatalf("expected 42, got %v", v)
	}
}

// -------------------------------------------------------------------
// evalWhen — multiple body forms
// -------------------------------------------------------------------

func TestEval_WhenMultipleBodyForms(t *testing.T) {
	ev := newEval()
	mustEval(t, ev, "(setq side-effect nil)")
	v := mustEval(t, ev, "(when t (setq side-effect 1) (setq side-effect 2) 99)")
	i, ok := v.(Int)
	if !ok || i.V != 99 {
		t.Fatalf("expected 99 from when body, got %v", v)
	}
}

// -------------------------------------------------------------------
// evalUnless — false condition executes body
// -------------------------------------------------------------------

func TestEval_UnlessFalseRunsBody(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(unless nil 77)")
	i, ok := v.(Int)
	if !ok || i.V != 77 {
		t.Fatalf("expected 77 when unless condition is nil, got %v", v)
	}
}

// -------------------------------------------------------------------
// formatString — additional format specifiers
// -------------------------------------------------------------------

func TestEval_FormatTwoStrings(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, `(format "%s and %s" "foo" "bar")`)
	s, ok := v.(StringVal)
	if !ok || s.V != "foo and bar" {
		t.Fatalf("expected \"foo and bar\", got %v", v)
	}
}

func TestEval_FormatDecimalInt(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, `(format "%d items" 5)`)
	s, ok := v.(StringVal)
	if !ok || s.V != "5 items" {
		t.Fatalf("expected \"5 items\", got %v", v)
	}
}

func TestEval_FormatPercentLiteral(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, `(format "100%%")`)
	s, ok := v.(StringVal)
	if !ok || s.V != "100%" {
		t.Fatalf("expected \"100%%\", got %v", v)
	}
}

func TestEval_FormatNonStringWithS(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, `(format "%s" 42)`)
	s, ok := v.(StringVal)
	if !ok || s.V != "42" {
		t.Fatalf("expected \"42\", got %v", v)
	}
}

// -------------------------------------------------------------------
// Eval — Lambda and Builtin pass-through
// -------------------------------------------------------------------

func TestEval_LambdaValuePassthrough(t *testing.T) {
	ev := newEval()
	// A let binding captures the lambda; referencing the variable returns it.
	v := mustEval(t, ev, "(let ((f (lambda (x) x))) f)")
	if _, ok := v.(Lambda); !ok {
		t.Fatalf("expected Lambda, got %T %v", v, v)
	}
}

func TestEval_BuiltinValuePassthrough(t *testing.T) {
	ev := newEval()
	// Referencing a builtin symbol returns a Builtin value.
	v := mustEval(t, ev, "(let ((f #'+)) f)")
	if _, ok := v.(Builtin); !ok {
		t.Fatalf("expected Builtin, got %T %v", v, v)
	}
}

// -------------------------------------------------------------------
// evalSetq — setq updates global env even when called from within let
// -------------------------------------------------------------------

func TestEval_SetqUpdatesGlobalFromLet(t *testing.T) {
	ev := newEval()
	// setq always writes to the global root, so after the let the global is 10.
	mustEval(t, ev, "(let ((x 5)) (setq x 10))")
	v, ok := ev.GetGlobalVar("x")
	if !ok {
		t.Fatal("x not in global env after setq inside let")
	}
	i, isInt := v.(Int)
	if !isInt || i.V != 10 {
		t.Fatalf("expected global x=10, got %v", v)
	}
}

// -------------------------------------------------------------------
// evalDefvar — initial value and no-overwrite with docstring
// -------------------------------------------------------------------

func TestEval_DefvarSetsInitialValue(t *testing.T) {
	ev := newEval()
	mustEval(t, ev, "(defvar my-count 42)")
	v, ok := ev.GetGlobalVar("my-count")
	if !ok {
		t.Fatal("my-count not defined after defvar")
	}
	i, isInt := v.(Int)
	if !isInt || i.V != 42 {
		t.Fatalf("expected 42, got %v", v)
	}
}

func TestEval_DefvarWithDocstring(t *testing.T) {
	ev := newEval()
	// defvar with docstring as third arg — value should still be set.
	mustEval(t, ev, `(defvar my-doc-var 99 "a docstring")`)
	v, ok := ev.GetGlobalVar("my-doc-var")
	if !ok {
		t.Fatal("my-doc-var not defined after defvar with docstring")
	}
	i, isInt := v.(Int)
	if !isInt || i.V != 99 {
		t.Fatalf("expected 99, got %v", v)
	}
}

func TestEval_DefvarDoesNotOverwriteExisting(t *testing.T) {
	ev := newEval()
	mustEval(t, ev, "(defvar my-count 42)")
	mustEval(t, ev, "(defvar my-count 999)") // should NOT change it
	v, _ := ev.GetGlobalVar("my-count")
	i, ok := v.(Int)
	if !ok || i.V != 42 {
		t.Fatalf("defvar should not overwrite: expected 42, got %v", v)
	}
}

// -------------------------------------------------------------------
// evalDefun — with docstring and &rest
// -------------------------------------------------------------------

func TestEval_DefunWithDocstring(t *testing.T) {
	ev := newEval()
	mustEval(t, ev, `(defun greet (name) "Greet someone." (concat "Hello " name))`)
	v := mustEval(t, ev, `(greet "World")`)
	s, ok := v.(StringVal)
	if !ok || s.V != "Hello World" {
		t.Fatalf("expected \"Hello World\", got %v", v)
	}
}

func TestEval_DefunWithRestParam(t *testing.T) {
	ev := newEval()
	mustEval(t, ev, "(defun sum-all (&rest args) (apply #'+ args))")
	v := mustEval(t, ev, "(sum-all 1 2 3)")
	i, ok := v.(Int)
	if !ok || i.V != 6 {
		t.Fatalf("expected 6, got %v", v)
	}
}

// -------------------------------------------------------------------
// parseParamList — &optional handling
// -------------------------------------------------------------------

func TestEval_OptionalParam(t *testing.T) {
	ev := newEval()
	mustEval(t, ev, "(defun foo (a &optional b) (if b (+ a b) a))")
	// no optional arg supplied
	v1 := mustEval(t, ev, "(foo 5)")
	i1, ok1 := v1.(Int)
	if !ok1 || i1.V != 5 {
		t.Fatalf("expected 5 (optional missing), got %v", v1)
	}
	// optional arg supplied
	v2 := mustEval(t, ev, "(foo 3 4)")
	i2, ok2 := v2.(Int)
	if !ok2 || i2.V != 7 {
		t.Fatalf("expected 7 (optional present), got %v", v2)
	}
}

// -------------------------------------------------------------------
// evalCond — all conditions false, and condition-only clause
// -------------------------------------------------------------------

func TestEval_CondAllFalseReturnsNil(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(cond (nil 1) (nil 2))")
	if !IsNil(v) {
		t.Fatalf("expected nil when all cond clauses are false, got %v", v)
	}
}

func TestEval_CondConditionOnlyClause(t *testing.T) {
	ev := newEval()
	// A clause with no body returns the condition value itself.
	v := mustEval(t, ev, "(cond (42))")
	i, ok := v.(Int)
	if !ok || i.V != 42 {
		t.Fatalf("expected 42 from condition-only cond clause, got %v", v)
	}
}

// -------------------------------------------------------------------
// evalProgn — empty progn returns nil
// -------------------------------------------------------------------

func TestEval_PrognEmpty(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(progn)")
	if !IsNil(v) {
		t.Fatalf("expected nil for (progn), got %v", v)
	}
}

// -------------------------------------------------------------------
// evalWhile — body executes and counter advances
// -------------------------------------------------------------------

func TestEval_WhileBodyExecutes(t *testing.T) {
	ev := newEval()
	// setq writes to global, so use global variables (not let bindings) here
	// to ensure the while condition sees the updated value.
	mustEval(t, ev, "(setq wn 0)")
	mustEval(t, ev, "(while (< wn 3) (setq wn (+ wn 1)))")
	v, ok := ev.GetGlobalVar("wn")
	if !ok {
		t.Fatal("wn not in global env")
	}
	i, isInt := v.(Int)
	if !isInt || i.V != 3 {
		t.Fatalf("expected wn=3 after while loop, got %v", v)
	}
}

// -------------------------------------------------------------------
// evalQuote — wrong arg count returns error
// -------------------------------------------------------------------

func TestEval_QuoteWrongArgCount(t *testing.T) {
	ev := newEval()
	// quoting two forms at once is invalid
	_, err := ev.EvalString("(quote a b)")
	if err == nil {
		t.Fatal("expected error for (quote a b), got nil")
	}
}

// -------------------------------------------------------------------
// evalAnd — early termination on false, last truthy value returned
// -------------------------------------------------------------------

func TestEval_AndLastTruthyReturned(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(and t 42)")
	i, ok := v.(Int)
	if !ok || i.V != 42 {
		t.Fatalf("expected 42 as last truthy value of (and t 42), got %v", v)
	}
}

func TestEval_AndShortCircuitsOnNil(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(and t nil 99)")
	if !IsNil(v) {
		t.Fatalf("expected nil for (and t nil 99), got %v", v)
	}
}

// -------------------------------------------------------------------
// callLambda — &rest param via funcall
// -------------------------------------------------------------------

func TestEval_LambdaRestViaFuncall(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(let ((f (lambda (&rest args) args))) (funcall f 1 2 3))")
	elems, ok := ToSlice(v)
	if !ok || len(elems) != 3 {
		t.Fatalf("expected list of 3, got %v", v)
	}
	for idx, want := range []int64{1, 2, 3} {
		iv, isInt := elems[idx].(Int)
		if !isInt || iv.V != want {
			t.Fatalf("elems[%d]: expected %d, got %v", idx, want, elems[idx])
		}
	}
}

// -------------------------------------------------------------------
// registerBuiltins — error cases for wrong arg counts
// -------------------------------------------------------------------

func TestEval_NotWrongArgCount(t *testing.T) {
	ev := newEval()
	_, err := ev.EvalString("(not 1 2)")
	if err == nil {
		t.Error("expected error for (not 1 2)")
	}
}

func TestEval_NullWrongArgCount(t *testing.T) {
	ev := newEval()
	_, err := ev.EvalString("(null)")
	if err == nil {
		t.Error("expected error for (null) with no args")
	}
}

func TestEval_CarWrongArgCount(t *testing.T) {
	ev := newEval()
	_, err := ev.EvalString("(car)")
	if err == nil {
		t.Error("expected error for (car) with no args")
	}
}

func TestEval_CdrWrongArgCount(t *testing.T) {
	ev := newEval()
	_, err := ev.EvalString("(cdr nil nil)")
	if err == nil {
		t.Error("expected error for (cdr nil nil) with two args")
	}
}

func TestEval_ConsWrongArgCount(t *testing.T) {
	ev := newEval()
	_, err := ev.EvalString("(cons 1)")
	if err == nil {
		t.Error("expected error for (cons 1) with one arg")
	}
}

func TestEval_EqWrongArgCount(t *testing.T) {
	ev := newEval()
	_, err := ev.EvalString("(eq 1 2 3)")
	if err == nil {
		t.Error("expected error for (eq 1 2 3) with three args")
	}
}

func TestEval_EqualWrongArgCount(t *testing.T) {
	ev := newEval()
	_, err := ev.EvalString("(equal 1)")
	if err == nil {
		t.Error("expected error for (equal 1) with one arg")
	}
}

func TestEval_CdrOfNonListErrors(t *testing.T) {
	ev := newEval()
	_, err := ev.EvalString("(cdr 5)")
	if err == nil {
		t.Fatal("expected error for (cdr 5), got nil")
	}
}

// -------------------------------------------------------------------
// eqValues — nil vs non-nil comparisons
// -------------------------------------------------------------------

func TestEval_EqNilVsT(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(eq nil t)")
	if !IsNil(v) {
		t.Fatalf("expected nil for (eq nil t), got %v", v)
	}
}

func TestEval_EqNilVsInt(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(eq nil 0)")
	if !IsNil(v) {
		t.Fatalf("expected nil for (eq nil 0), got %v", v)
	}
}

func TestEval_EqFloatSame(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(eq 3.14 3.14)")
	if _, ok := v.(Bool); !ok {
		t.Fatalf("expected Bool t for (eq 3.14 3.14), got %v (%T)", v, v)
	}
}

func TestEval_EqFloatDiffer(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(eq 3.14 2.71)")
	if !IsNil(v) {
		t.Fatalf("expected nil for (eq 3.14 2.71), got %v", v)
	}
}

// -------------------------------------------------------------------
// equalValues — dotted pairs
// -------------------------------------------------------------------

func TestEval_EqualDottedPairsSame(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(equal '(1 . 2) '(1 . 2))")
	if _, ok := v.(Bool); !ok {
		t.Fatalf("expected Bool t for equal dotted pairs, got %v (%T)", v, v)
	}
}

func TestEval_EqualDottedPairsDiffer(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(equal '(1 . 2) '(1 . 3))")
	if !IsNil(v) {
		t.Fatalf("expected nil for unequal dotted pairs, got %v", v)
	}
}

func TestEval_EqualConsDifferentTypes(t *testing.T) {
	ev := newEval()
	// cons vs non-cons
	v := mustEval(t, ev, "(equal '(1 . 2) 42)")
	if !IsNil(v) {
		t.Fatalf("expected nil for (equal cons int), got %v", v)
	}
}

// -------------------------------------------------------------------
// formatString — %d with float, %f with int, unknown specifier, no fmt string
// -------------------------------------------------------------------

func TestEval_FormatDWithFloat(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, `(format "%d" 3.7)`)
	s, ok := v.(StringVal)
	if !ok || s.V != "4" {
		t.Fatalf("expected \"4\" for (format \"%%d\" 3.7), got %v", v)
	}
}

func TestEval_FormatFWithInt(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, `(format "%f" 2)`)
	s, ok := v.(StringVal)
	if !ok || s.V != "2.000000" {
		t.Fatalf("expected \"2.000000\" for (format \"%%f\" 2), got %v", v)
	}
}

func TestEval_FormatUnknownSpecifier(t *testing.T) {
	ev := newEval()
	// %z is an unknown specifier; it should be passed through literally.
	v := mustEval(t, ev, `(format "%z" 42)`)
	s, ok := v.(StringVal)
	if !ok {
		t.Fatalf("expected StringVal, got %T", v)
	}
	// unknown specifiers keep their % and specifier char; arg is not consumed
	if s.V != "%z" {
		t.Fatalf("expected \"%%z\" for unknown specifier, got %q", s.V)
	}
}

func TestEval_FormatNoFormatString(t *testing.T) {
	ev := newEval()
	// When the first arg is not a string, formatString uses .String() on it.
	v := mustEval(t, ev, "(format 42)")
	s, ok := v.(StringVal)
	if !ok || s.V != "42" {
		t.Fatalf("expected \"42\" for (format 42), got %v", v)
	}
}

func TestEval_FormatEmpty(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(format)")
	s, ok := v.(StringVal)
	if !ok || s.V != "" {
		t.Fatalf("expected \"\" for (format), got %v", v)
	}
}

func TestEval_FormatSpecifierBeyondArgs(t *testing.T) {
	ev := newEval()
	// More format specs than args: the extra spec is passed through literally.
	v := mustEval(t, ev, `(format "%s %s" "only-one")`)
	s, ok := v.(StringVal)
	if !ok {
		t.Fatalf("expected StringVal, got %T", v)
	}
	if s.V != "only-one %s" {
		t.Fatalf("expected \"only-one %%s\" for missing arg, got %q", s.V)
	}
}

// -------------------------------------------------------------------
// SetGlobal — path through nested environments
// -------------------------------------------------------------------

func TestEnv_SetGlobalFromChild(t *testing.T) {
	root := NewEnv()
	child := NewChildEnv(root)
	grandchild := NewChildEnv(child)
	grandchild.SetGlobal("x", Int{V: 99})
	v, ok := root.Get("x")
	if !ok {
		t.Fatal("SetGlobal did not set value in root")
	}
	i, isInt := v.(Int)
	if !isInt || i.V != 99 {
		t.Fatalf("expected 99, got %v", v)
	}
}

// -------------------------------------------------------------------
// numericCompare — non-number error
// -------------------------------------------------------------------

func TestEval_NumericCompareNonNumber(t *testing.T) {
	ev := newEval()
	_, err := ev.EvalString(`(< "a" 1)`)
	if err == nil {
		t.Fatal("expected error for (< string int)")
	}
}

// -------------------------------------------------------------------
// applyFunction — not a function error
// -------------------------------------------------------------------

func TestEval_ApplyNonFunction(t *testing.T) {
	ev := newEval()
	_, err := ev.EvalString("(42 1 2)")
	if err == nil {
		t.Fatal("expected error when calling non-function 42")
	}
}

// -------------------------------------------------------------------
// evalCons — improper argument list
// -------------------------------------------------------------------

func TestEval_ImproperArgList(t *testing.T) {
	ev := newEval()
	// Build a cons with a non-list cdr via Go, then eval it.
	// The easiest way is to construct and call applyFunction with an improper list.
	// We test via a defun that causes too many args.
	mustEval(t, ev, "(defun one-arg (x) x)")
	_, err := ev.EvalString("(one-arg 1 2 3)")
	if err == nil {
		t.Fatal("expected error for too many arguments")
	}
}

// -------------------------------------------------------------------
// evalLambda — error on no params
// -------------------------------------------------------------------

func TestEval_LambdaInvalidParams(t *testing.T) {
	ev := newEval()
	_, err := ev.EvalString("(lambda)")
	if err == nil {
		t.Fatal("expected error for (lambda) with no params")
	}
}

// -------------------------------------------------------------------
// evalFunction — non-symbol arg (lambda literal)
// -------------------------------------------------------------------

func TestEval_FunctionWithLambdaArg(t *testing.T) {
	ev := newEval()
	// (function (lambda (x) x)) should return the lambda.
	v := mustEval(t, ev, "(function (lambda (x) (* x 3)))")
	if _, ok := v.(Lambda); !ok {
		t.Fatalf("expected Lambda from (function (lambda ...)), got %T", v)
	}
}

// -------------------------------------------------------------------
// global-set-key — string command name and fallback String()
// -------------------------------------------------------------------

func TestEval_GlobalSetKeyStringCommand(t *testing.T) {
	ev := newEval()
	mustEval(t, ev, `(global-set-key (kbd "C-x C-s") "save-buffer")`)
	kb := ev.GetKeyBindings()
	if kb["C-x C-s"] != "save-buffer" {
		t.Fatalf("expected save-buffer binding, got %q", kb["C-x C-s"])
	}
}

// -------------------------------------------------------------------
// load built-in — noerror suppresses missing-file error
// -------------------------------------------------------------------

func TestEval_LoadNoerror(t *testing.T) {
	ev := newEval()
	// (load "nonexistent" nil t) — third arg truthy = noerror
	v := mustEval(t, ev, `(load "/nonexistent/path.el" nil t)`)
	if !IsNil(v) {
		t.Fatalf("expected nil from load with noerror, got %v", v)
	}
}

func TestEval_LoadReturnsErrorWhenNoNoerror(t *testing.T) {
	ev := newEval()
	_, err := ev.EvalString(`(load "/nonexistent/path.el")`)
	if err == nil {
		t.Fatal("expected error from load without noerror")
	}
}

// -------------------------------------------------------------------
// Special-form error and edge-case branches
// -------------------------------------------------------------------

func TestEval_SetqOddArgsErrors(t *testing.T) {
	wantErr(t, newEval(), "(setq a)")
}

func TestEval_SetqNonSymbolErrors(t *testing.T) {
	wantErr(t, newEval(), "(setq 1 2)")
}

func TestEval_SetqEvalErrorPropagates(t *testing.T) {
	wantErr(t, newEval(), "(setq x undefined-xyz)")
}

func TestEval_DefvarNoArgsErrors(t *testing.T) {
	wantErr(t, newEval(), "(defvar)")
}

func TestEval_DefvarNonSymbolErrors(t *testing.T) {
	wantErr(t, newEval(), "(defvar 5 1)")
}

func TestEval_DefvarDoesNotOverride(t *testing.T) {
	ev := newEval()
	mustEval(t, ev, "(setq dv 10)")
	mustEval(t, ev, "(defvar dv 99)")
	if got := mustEval(t, ev, "dv"); got.(Int).V != 10 {
		t.Fatalf("defvar overrode existing var: got %v", got)
	}
}

func TestEval_DefvarNoValueOnNewSymbol(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(defvar only-decl)")
	if s, ok := v.(Symbol); !ok || s.Name != "only-decl" {
		t.Fatalf("defvar return: want symbol only-decl, got %v", v)
	}
}

func TestEval_DefvarValueEvalErrorPropagates(t *testing.T) {
	wantErr(t, newEval(), "(defvar nv undefined-xyz)")
}

func TestEval_DefunTooFewArgsErrors(t *testing.T) {
	wantErr(t, newEval(), "(defun foo)")
}

func TestEval_DefunNonSymbolNameErrors(t *testing.T) {
	wantErr(t, newEval(), "(defun 5 () 1)")
}

func TestEval_DefunBadParamListErrors(t *testing.T) {
	wantErr(t, newEval(), "(defun foo (1) 1)")
}

func TestEval_IfTooFewArgsErrors(t *testing.T) {
	wantErr(t, newEval(), "(if t)")
}

func TestEval_IfCondErrorPropagates(t *testing.T) {
	wantErr(t, newEval(), "(if undefined-xyz 1 2)")
}

func TestEval_IfElseProgn(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(if nil 0 1 2 3)")
	if i, ok := v.(Int); !ok || i.V != 3 {
		t.Fatalf("if multi-else: want 3, got %v", v)
	}
}

func TestEval_IfElseErrorPropagates(t *testing.T) {
	wantErr(t, newEval(), "(if nil 1 undefined-xyz)")
}

func TestEval_CondInvalidClauseErrors(t *testing.T) {
	wantErr(t, newEval(), "(cond 5)")
}

func TestEval_CondTestErrorPropagates(t *testing.T) {
	wantErr(t, newEval(), "(cond (undefined-xyz 1))")
}

func TestEval_CondSingleElementClause(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(cond (42))")
	if i, ok := v.(Int); !ok || i.V != 42 {
		t.Fatalf("cond single-element: want 42, got %v", v)
	}
}

func TestEval_CondBodyErrorPropagates(t *testing.T) {
	wantErr(t, newEval(), "(cond (t undefined-xyz))")
}

func TestEval_PrognErrorPropagates(t *testing.T) {
	wantErr(t, newEval(), "(progn 1 undefined-xyz)")
}

func TestEval_LetNoArgsErrors(t *testing.T) {
	wantErr(t, newEval(), "(let)")
}

func TestEval_LetBadBindingListErrors(t *testing.T) {
	wantErr(t, newEval(), "(let 5 1)")
}

func TestEval_LetBindingNameNotSymbolErrors(t *testing.T) {
	wantErr(t, newEval(), "(let ((1 2)) 1)")
}

func TestEval_LetUnexpectedBindingTypeErrors(t *testing.T) {
	wantErr(t, newEval(), "(let (5) 1)")
}

func TestEval_LetSymbolBindingDefaultsNil(t *testing.T) {
	ev := newEval()
	if v := mustEval(t, ev, "(let (x) x)"); !IsNil(v) {
		t.Fatalf("let bare symbol: want nil, got %v", v)
	}
}

func TestEval_LetEmptyConsBinding(t *testing.T) {
	ev := newEval()
	if v := mustEval(t, ev, "(let ((x)) x)"); !IsNil(v) {
		t.Fatalf("let empty cons binding: want nil, got %v", v)
	}
}

func TestEval_LetNilBindingSkipped(t *testing.T) {
	ev := newEval()
	if v := mustEval(t, ev, "(let (()) 7)"); v.(Int).V != 7 {
		t.Fatalf("let nil binding: want 7, got %v", v)
	}
}

func TestEval_LetBindingValueErrorPropagates(t *testing.T) {
	wantErr(t, newEval(), "(let ((x undefined-xyz)) x)")
}

func TestEval_LetBodyErrorPropagates(t *testing.T) {
	wantErr(t, newEval(), "(let ((x 1)) undefined-xyz)")
}

func TestEval_WhenNoArgsErrors(t *testing.T) {
	wantErr(t, newEval(), "(when)")
}

func TestEval_WhenCondErrorPropagates(t *testing.T) {
	wantErr(t, newEval(), "(when undefined-xyz 1)")
}

func TestEval_WhenFalseReturnsNil(t *testing.T) {
	ev := newEval()
	if v := mustEval(t, ev, "(when nil 1)"); !IsNil(v) {
		t.Fatalf("when nil: want nil, got %v", v)
	}
}

func TestEval_WhenBodyErrorPropagates(t *testing.T) {
	wantErr(t, newEval(), "(when t undefined-xyz)")
}

func TestEval_UnlessNoArgsErrors(t *testing.T) {
	wantErr(t, newEval(), "(unless)")
}

func TestEval_UnlessCondErrorPropagates(t *testing.T) {
	wantErr(t, newEval(), "(unless undefined-xyz 1)")
}

func TestEval_UnlessTrueReturnsNil(t *testing.T) {
	ev := newEval()
	if v := mustEval(t, ev, "(unless t 1)"); !IsNil(v) {
		t.Fatalf("unless t: want nil, got %v", v)
	}
}

func TestEval_UnlessBodyErrorPropagates(t *testing.T) {
	wantErr(t, newEval(), "(unless nil undefined-xyz)")
}

func TestEval_WhileNoArgsErrors(t *testing.T) {
	wantErr(t, newEval(), "(while)")
}

func TestEval_WhileCondErrorPropagates(t *testing.T) {
	wantErr(t, newEval(), "(while undefined-xyz 1)")
}

func TestEval_WhileBodyErrorPropagates(t *testing.T) {
	ev := newEval()
	mustEval(t, ev, "(setq i 0)")
	wantErr(t, ev, "(while (= i 0) (setq i 1) undefined-xyz)")
}

func TestEval_WhileLoops(t *testing.T) {
	ev := newEval()
	mustEval(t, ev, "(setq i 0)")
	mustEval(t, ev, "(while (< i 3) (setq i (+ i 1)))")
	if got := mustEval(t, ev, "i"); got.(Int).V != 3 {
		t.Fatalf("while loop: want i=3, got %v", got)
	}
}

func TestEval_FunctionWrongArgsErrors(t *testing.T) {
	wantErr(t, newEval(), "(function)")
}

func TestEval_FunctionVoidErrors(t *testing.T) {
	wantErr(t, newEval(), "(function never-defined-xyz)")
}

func TestEval_FunctionNonSymbolEvaluates(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(function (lambda (x) x))")
	if _, ok := v.(Lambda); !ok {
		t.Fatalf("function on lambda form: want Lambda, got %T", v)
	}
}

func TestEval_FunctionUserDefined(t *testing.T) {
	ev := newEval()
	mustEval(t, ev, "(defun myf (x) x)")
	v := mustEval(t, ev, "(function myf)")
	if _, ok := v.(Lambda); !ok {
		t.Fatalf("function myf: want Lambda, got %T", v)
	}
}

func TestEval_LambdaNoArgsErrors(t *testing.T) {
	wantErr(t, newEval(), "(lambda)")
}

func TestEval_LambdaBadParamListErrors(t *testing.T) {
	wantErr(t, newEval(), "(lambda 5 1)")
}

func TestEval_AndEmptyReturnsT(t *testing.T) {
	ev := newEval()
	if v := mustEval(t, ev, "(and)"); !isTruthy(v) {
		t.Fatalf("(and): want t, got %v", v)
	}
}

func TestEval_AndErrorPropagates(t *testing.T) {
	wantErr(t, newEval(), "(and t undefined-xyz)")
}

func TestEval_AndShortCircuits(t *testing.T) {
	ev := newEval()
	if v := mustEval(t, ev, "(and 1 nil 2)"); !IsNil(v) {
		t.Fatalf("(and 1 nil 2): want nil, got %v", v)
	}
}

func TestEval_OrEmptyReturnsNil(t *testing.T) {
	ev := newEval()
	if v := mustEval(t, ev, "(or)"); !IsNil(v) {
		t.Fatalf("(or): want nil, got %v", v)
	}
}

func TestEval_OrErrorPropagates(t *testing.T) {
	wantErr(t, newEval(), "(or nil undefined-xyz)")
}

func TestEval_QuasiquoteUnquote(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(setq x 3) `(a ,x b)")
	if got := v.String(); got != "(a 3 b)" {
		t.Fatalf("quasiquote unquote: want (a 3 b), got %q", got)
	}
}

func TestEval_QuasiquoteAtom(t *testing.T) {
	ev := newEval()
	if v := mustEval(t, ev, "`5"); v.(Int).V != 5 {
		t.Fatalf("quasiquote atom: want 5, got %v", v)
	}
}

func TestEval_QuasiquoteUnquoteErrorPropagates(t *testing.T) {
	wantErr(t, newEval(), "`(a ,undefined-xyz)")
}

func TestEval_LambdaTooManyArgsErrors(t *testing.T) {
	ev := newEval()
	mustEval(t, ev, "(defun one (x) x)")
	wantErr(t, ev, "(one 1 2)")
}

func TestEval_LambdaRestCollectsExtra(t *testing.T) {
	ev := newEval()
	mustEval(t, ev, "(defun collect (a &rest r) r)")
	v := mustEval(t, ev, "(collect 1 2 3)")
	if got := v.String(); got != "(2 3)" {
		t.Fatalf("&rest collect: want (2 3), got %q", got)
	}
}

func TestEval_LambdaMissingArgsDefaultNil(t *testing.T) {
	ev := newEval()
	mustEval(t, ev, "(defun pair (a b) b)")
	if v := mustEval(t, ev, "(pair 1)"); !IsNil(v) {
		t.Fatalf("missing arg default: want nil, got %v", v)
	}
}

func TestEval_ConsHeadNotSymbolCalls(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "((lambda (x) (+ x 1)) 4)")
	if v.(Int).V != 5 {
		t.Fatalf("direct lambda call: want 5, got %v", v)
	}
}

func TestEval_CallNonFunctionErrors(t *testing.T) {
	wantErr(t, newEval(), "(5 1 2)")
}

func TestEval_VectorEvaluatesElements(t *testing.T) {
	ev := newEval()
	mustEval(t, ev, "(setq y 9)")
	v := mustEval(t, ev, "[1 y 3]")
	vec, ok := v.(Vector)
	if !ok || len(vec.Elems) != 3 || vec.Elems[1].(Int).V != 9 {
		t.Fatalf("vector eval: got %v", v)
	}
}

func TestEval_VectorElementErrorPropagates(t *testing.T) {
	wantErr(t, newEval(), "[1 undefined-xyz]")
}

func TestEval_SelfEvaluatingAtoms(t *testing.T) {
	ev := newEval()
	for _, src := range []string{"42", "3.5", `"hi"`, "t", "nil"} {
		if _, err := ev.EvalString(src); err != nil {
			t.Fatalf("self-eval %q: %v", src, err)
		}
	}
}

func TestEval_QuasiquoteWrongArgsErrors(t *testing.T) {
	ev := newEval()
	form, err := ParseOne("(quasiquote 1 2)")
	if err != nil {
		t.Fatalf("ParseOne: %v", err)
	}
	if _, err := ev.Eval(form, ev.global); err == nil {
		t.Fatal("quasiquote with 2 args: expected error")
	}
}

// -------------------------------------------------------------------
// numericFold / numericFoldFrom / numericCompare error paths
// -------------------------------------------------------------------

func TestEval_AddNonNumberErrors(t *testing.T) {
	wantErr(t, newEval(), `(+ 1 "x")`)
}

func TestEval_AddFloatNonNumberErrors(t *testing.T) {
	wantErr(t, newEval(), `(+ 1.0 "x")`)
}

func TestEval_SubNonNumberErrors(t *testing.T) {
	wantErr(t, newEval(), `(- 1 "x")`)
}

func TestEval_SubSingleNonNumberErrors(t *testing.T) {
	wantErr(t, newEval(), `(- "x")`)
}

func TestEval_SubFloatFirstNonNumber(t *testing.T) {
	wantErr(t, newEval(), `(- "x" 1.0)`)
}

func TestEval_DivFloat(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(/ 6.0 2)")
	if f, ok := v.(Float); !ok || f.V != 3.0 {
		t.Fatalf("(/ 6.0 2): want 3.0, got %v", v)
	}
}

func TestEval_CompareNonNumberErrors(t *testing.T) {
	wantErr(t, newEval(), `(< 1 "x")`)
}

func TestEval_CompareTooFewArgsErrors(t *testing.T) {
	wantErr(t, newEval(), "(< 1)")
}

// -------------------------------------------------------------------
// Builtin coverage: success and error branches
// -------------------------------------------------------------------

func TestEval_LengthVector(t *testing.T) {
	ev := newEval()
	if v := mustEval(t, ev, "(length [1 2 3])"); v.(Int).V != 3 {
		t.Fatalf("length vector: want 3, got %v", v)
	}
}

func TestEval_LengthImproperErrors(t *testing.T) {
	wantErr(t, newEval(), "(length (cons 1 2))")
}

func TestEval_LengthWrongArgCountErrors(t *testing.T) {
	wantErr(t, newEval(), "(length)")
}

func TestEval_MessageNoArgs(t *testing.T) {
	ev := newEval()
	if v := mustEval(t, ev, "(message)"); v.(StringVal).V != "" {
		t.Fatalf("message no args: want empty, got %v", v)
	}
}

func TestEval_MessageFormat(t *testing.T) {
	ev := newEval()
	if v := mustEval(t, ev, `(message "hi %d" 5)`); v.(StringVal).V != "hi 5" {
		t.Fatalf("message format: want \"hi 5\", got %v", v)
	}
}

func TestEval_StringToIntWrongArgCountErrors(t *testing.T) {
	wantErr(t, newEval(), "(string-to-int)")
}

func TestEval_StringToIntNonStringErrors(t *testing.T) {
	wantErr(t, newEval(), "(string-to-int 42)")
}

func TestEval_StringToIntBadFormatErrors(t *testing.T) {
	wantErr(t, newEval(), `(string-to-int "abc")`)
}

func TestEval_IntToStringFloat(t *testing.T) {
	ev := newEval()
	if v := mustEval(t, ev, "(int-to-string 2.5)"); v.(StringVal).V != "2.5" {
		t.Fatalf("int-to-string float: want 2.5, got %v", v)
	}
}

func TestEval_IntToStringWrongArgCountErrors(t *testing.T) {
	wantErr(t, newEval(), "(int-to-string)")
}

func TestEval_IntToStringNonNumberErrors(t *testing.T) {
	wantErr(t, newEval(), `(int-to-string "x")`)
}

func TestEval_NumberToStringWrongArgCountErrors(t *testing.T) {
	wantErr(t, newEval(), "(number-to-string)")
}

func TestEval_NumberToStringNonNumberErrors(t *testing.T) {
	wantErr(t, newEval(), `(number-to-string "x")`)
}

func TestEval_StringpWrongArgCountErrors(t *testing.T) {
	wantErr(t, newEval(), "(stringp)")
}

func TestEval_StringpFalse(t *testing.T) {
	ev := newEval()
	if v := mustEval(t, ev, "(stringp 5)"); !IsNil(v) {
		t.Fatalf("stringp 5: want nil, got %v", v)
	}
}

func TestEval_NumberpFloatAndFalse(t *testing.T) {
	ev := newEval()
	if v := mustEval(t, ev, "(numberp 1.5)"); !isTruthy(v) {
		t.Fatalf("numberp float: want t, got %v", v)
	}
	if v := mustEval(t, ev, `(numberp "x")`); !IsNil(v) {
		t.Fatalf("numberp string: want nil, got %v", v)
	}
}

func TestEval_NumberpWrongArgCountErrors(t *testing.T) {
	wantErr(t, newEval(), "(numberp)")
}

func TestEval_SymbolpVariants(t *testing.T) {
	ev := newEval()
	if v := mustEval(t, ev, "(symbolp 'foo)"); !isTruthy(v) {
		t.Fatalf("symbolp 'foo: want t, got %v", v)
	}
	if v := mustEval(t, ev, "(symbolp nil)"); !isTruthy(v) {
		t.Fatalf("symbolp nil: want t, got %v", v)
	}
	if v := mustEval(t, ev, "(symbolp 5)"); !IsNil(v) {
		t.Fatalf("symbolp 5: want nil, got %v", v)
	}
}

func TestEval_SymbolpWrongArgCountErrors(t *testing.T) {
	wantErr(t, newEval(), "(symbolp)")
}

func TestEval_ListpVariants(t *testing.T) {
	ev := newEval()
	if v := mustEval(t, ev, "(listp '(1))"); !isTruthy(v) {
		t.Fatalf("listp list: want t, got %v", v)
	}
	if v := mustEval(t, ev, "(listp nil)"); !isTruthy(v) {
		t.Fatalf("listp nil: want t, got %v", v)
	}
	if v := mustEval(t, ev, "(listp 5)"); !IsNil(v) {
		t.Fatalf("listp 5: want nil, got %v", v)
	}
}

func TestEval_ListpWrongArgCountErrors(t *testing.T) {
	wantErr(t, newEval(), "(listp)")
}

func TestEval_FunctionpWrongArgCountErrors(t *testing.T) {
	wantErr(t, newEval(), "(functionp)")
}

func TestEval_GlobalSetKeyWrongArgCountErrors(t *testing.T) {
	wantErr(t, newEval(), `(global-set-key "C-x")`)
}

func TestEval_GlobalSetKeyNonStringKeyErrors(t *testing.T) {
	wantErr(t, newEval(), "(global-set-key 5 'foo)")
}

func TestEval_GlobalSetKeyOtherCommand(t *testing.T) {
	ev := newEval()
	mustEval(t, ev, `(global-set-key "C-y" 5)`)
	if ev.keyBindings["C-y"] != "5" {
		t.Fatalf("global-set-key int cmd: got %q", ev.keyBindings["C-y"])
	}
}

func TestEval_KbdWrongArgCountErrors(t *testing.T) {
	wantErr(t, newEval(), "(kbd)")
}

func TestEval_KbdNonStringErrors(t *testing.T) {
	wantErr(t, newEval(), "(kbd 5)")
}

func TestEval_RequireNoArgs(t *testing.T) {
	ev := newEval()
	if v := mustEval(t, ev, "(require)"); !IsNil(v) {
		t.Fatalf("require no args: want nil, got %v", v)
	}
}

func TestEval_LoadWrongArgCountErrors(t *testing.T) {
	wantErr(t, newEval(), "(load)")
}

func TestEval_LoadNonStringErrors(t *testing.T) {
	wantErr(t, newEval(), "(load 5)")
}

func TestEval_FuncallNoArgsErrors(t *testing.T) {
	wantErr(t, newEval(), "(funcall)")
}

func TestEval_ApplyTooFewArgsErrors(t *testing.T) {
	wantErr(t, newEval(), "(apply '+)")
}

func TestEval_ApplyLastNotListErrors(t *testing.T) {
	wantErr(t, newEval(), "(apply '+ 5)")
}

func TestEval_MapcarWrongArgCountErrors(t *testing.T) {
	wantErr(t, newEval(), "(mapcar (lambda (x) x))")
}

func TestEval_MapcarNonListErrors(t *testing.T) {
	wantErr(t, newEval(), "(mapcar (lambda (x) x) 5)")
}

func TestEval_MapcarFnErrorPropagates(t *testing.T) {
	wantErr(t, newEval(), "(mapcar 'car '(1 2))")
}

func TestEval_ConcatSymbolAndOther(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, `(concat "a" 'b 3)`)
	if v.(StringVal).V != "ab3" {
		t.Fatalf("concat mixed: want ab3, got %v", v)
	}
}

// -------------------------------------------------------------------
// Eval dispatch: self-evaluating function values and unknown types
// -------------------------------------------------------------------

// opaqueValue is a test-only Value implementation that Eval's type switch does
// not know about, so it exercises the switch's default branch.
type opaqueValue struct{}

func (opaqueValue) isValue()       {}
func (opaqueValue) String() string { return "#<opaque>" }

func TestEval_LambdaValueEvaluatesToItself(t *testing.T) {
	ev := newEval()
	lam := Lambda{Params: []string{"x"}, Body: []Value{Symbol{Name: "x"}}}
	got, err := ev.Eval(lam, NewEnv())
	if err != nil {
		t.Fatalf("Eval(Lambda): %v", err)
	}
	l, ok := got.(Lambda)
	if !ok {
		t.Fatalf("expected Lambda, got %T", got)
	}
	if len(l.Params) != 1 || l.Params[0] != "x" {
		t.Fatalf("lambda params changed: %v", l.Params)
	}
}

func TestEval_BuiltinValueEvaluatesToItself(t *testing.T) {
	ev := newEval()
	b := Builtin{Name: "noop", Fn: func([]Value, *Env) (Value, error) { return Nil{}, nil }}
	got, err := ev.Eval(b, NewEnv())
	if err != nil {
		t.Fatalf("Eval(Builtin): %v", err)
	}
	gb, ok := got.(Builtin)
	if !ok {
		t.Fatalf("expected Builtin, got %T", got)
	}
	if gb.Name != "noop" {
		t.Fatalf("builtin name = %q, want noop", gb.Name)
	}
}

func TestEval_UnknownValueTypeErrors(t *testing.T) {
	ev := newEval()
	if _, err := ev.Eval(opaqueValue{}, NewEnv()); err == nil {
		t.Fatal("expected error for unknown Value type")
	}
}

func TestEval_StringParseErrorPropagates(t *testing.T) {
	ev := newEval()
	// Unbalanced input fails in Parse, before any evaluation happens.
	if _, err := ev.EvalString("(setq x 1"); err == nil {
		t.Fatal("expected parse error from EvalString")
	}
}

// -------------------------------------------------------------------
// Symbol / function lookup falling through to registered Go functions
// -------------------------------------------------------------------

func TestEval_GoFnAsBareSymbolValue(t *testing.T) {
	ev := newEval()
	ev.RegisterGoFn("my-go-fn", func([]Value, *Env) (Value, error) { return Int{V: 7}, nil })
	// The symbol is unbound as a variable, so evalSymbol falls back to goFns
	// and yields a Builtin wrapper.
	v := mustEval(t, ev, "my-go-fn")
	b, ok := v.(Builtin)
	if !ok {
		t.Fatalf("expected Builtin, got %T", v)
	}
	if b.Name != "my-go-fn" {
		t.Fatalf("builtin name = %q, want my-go-fn", b.Name)
	}
}

func TestEval_SharpQuoteResolvesGoFn(t *testing.T) {
	ev := newEval()
	ev.RegisterGoFn("go-double", func(args []Value, _ *Env) (Value, error) {
		return Int{V: args[0].(Int).V * 2}, nil
	})
	v := mustEval(t, ev, "(funcall #'go-double 21)")
	if i, ok := v.(Int); !ok || i.V != 42 {
		t.Fatalf("got %v, want 42", v)
	}
}

// -------------------------------------------------------------------
// evalCons: non-symbol heads, undefined functions, improper arg lists
// -------------------------------------------------------------------

func TestEval_NonSymbolHeadEvalErrorPropagates(t *testing.T) {
	ev := newEval()
	// The head is a list, so it is evaluated as an expression — and fails.
	wantErr(t, ev, "((no-such-variable) 1)")
}

func TestEval_NonSymbolHeadImproperArgListErrors(t *testing.T) {
	ev := newEval()
	head, err := ParseOne("(lambda (x) x)")
	if err != nil {
		t.Fatalf("parse lambda: %v", err)
	}
	// Cdr is not a list, so ToSlice fails after the head evaluates fine.
	if _, err := ev.Eval(Cons{Car: head, Cdr: Int{V: 1}}, ev.global); err == nil {
		t.Fatal("expected improper argument list error")
	}
}

func TestEval_UndefinedFunctionCallErrors(t *testing.T) {
	ev := newEval()
	wantErr(t, ev, "(no-such-function 1 2)")
}

func TestEval_NamedCallImproperArgListErrors(t *testing.T) {
	ev := newEval()
	if _, err := ev.Eval(Cons{Car: Symbol{Name: "+"}, Cdr: Int{V: 1}}, ev.global); err == nil {
		t.Fatal("expected improper argument list error")
	}
}

func TestEval_ArgumentEvalErrorPropagates(t *testing.T) {
	ev := newEval()
	wantErr(t, ev, "(+ 1 no-such-variable)")
}

func TestEval_LambdaBodyErrorPropagates(t *testing.T) {
	ev := newEval()
	wantErr(t, ev, "((lambda () no-such-variable))")
}

// -------------------------------------------------------------------
// Special forms: improper argument lists and the cond `t` else clause
// -------------------------------------------------------------------

func TestEval_CondImproperClauseListErrors(t *testing.T) {
	ev := newEval()
	if _, err := ev.Eval(Cons{Car: Symbol{Name: "cond"}, Cdr: Int{V: 1}}, ev.global); err == nil {
		t.Fatal("expected cond clause list error")
	}
}

func TestEval_CondTClauseActsAsElse(t *testing.T) {
	ev := newEval()
	// `t` is recognised structurally, without being evaluated as a variable.
	v := mustEval(t, ev, "(cond (nil 1) (t 2))")
	if i, ok := v.(Int); !ok || i.V != 2 {
		t.Fatalf("got %v, want 2", v)
	}
}

func TestEval_PrognImproperFormListErrors(t *testing.T) {
	ev := newEval()
	if _, err := ev.Eval(Cons{Car: Symbol{Name: "progn"}, Cdr: Int{V: 1}}, ev.global); err == nil {
		t.Fatal("expected progn form list error")
	}
}

func TestEval_LetDottedBindingErrors(t *testing.T) {
	ev := newEval()
	// (a . 1) is a Cons but not a proper list, so ToSlice fails.
	wantErr(t, ev, "(let ((a . 1)) a)")
}

func TestEval_AndImproperFormListErrors(t *testing.T) {
	ev := newEval()
	if _, err := ev.Eval(Cons{Car: Symbol{Name: "and"}, Cdr: Int{V: 1}}, ev.global); err == nil {
		t.Fatal("expected and form list error")
	}
}

func TestEval_OrImproperFormListErrors(t *testing.T) {
	ev := newEval()
	if _, err := ev.Eval(Cons{Car: Symbol{Name: "or"}, Cdr: Int{V: 1}}, ev.global); err == nil {
		t.Fatal("expected or form list error")
	}
}

// -------------------------------------------------------------------
// Arithmetic edge cases
// -------------------------------------------------------------------

func TestEval_SubNoArgsIsZero(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(-)")
	if i, ok := v.(Int); !ok || i.V != 0 {
		t.Fatalf("got %v, want 0", v)
	}
}

func TestEval_SubSingleFloatNegates(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(- 1.5)")
	if f, ok := v.(Float); !ok || f.V != -1.5 {
		t.Fatalf("got %v, want -1.5", v)
	}
}

func TestEval_MulFloatAndInt(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(* 1.5 2)")
	if f, ok := v.(Float); !ok || f.V != 3.0 {
		t.Fatalf("got %v, want 3", v)
	}
}

func TestEval_SubFloatLaterArgNonNumberErrors(t *testing.T) {
	ev := newEval()
	// The float path is chosen because of 1.5, then "x" fails to convert.
	wantErr(t, ev, `(- 1.5 "x")`)
}

// -------------------------------------------------------------------
// Equality on aggregates
// -------------------------------------------------------------------

func TestEval_EqOnListsIsNil(t *testing.T) {
	ev := newEval()
	// eq compares identity for aggregates, so two equal lists are not eq.
	v := mustEval(t, ev, "(eq (list 1) (list 1))")
	if !IsNil(v) {
		t.Fatalf("got %v, want nil", v)
	}
}

func TestEval_EqOnVectorsIsNil(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(eq [1 2] [1 2])")
	if !IsNil(v) {
		t.Fatalf("got %v, want nil", v)
	}
}

func TestEval_EqualVectorElementDiffers(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, "(equal [1 2] [1 3])")
	if !IsNil(v) {
		t.Fatalf("got %v, want nil", v)
	}
}

// -------------------------------------------------------------------
// format: type mismatches fall back to the value's printed form
// -------------------------------------------------------------------

func TestEval_FormatDWithNonNumber(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, `(format "%d" "x")`)
	if s, ok := v.(StringVal); !ok || s.V != `"x"` {
		t.Fatalf("got %v, want %q", v, `"x"`)
	}
}

func TestEval_FormatFWithNonNumber(t *testing.T) {
	ev := newEval()
	v := mustEval(t, ev, `(format "%f" "x")`)
	if s, ok := v.(StringVal); !ok || s.V != `"x"` {
		t.Fatalf("got %v, want %q", v, `"x"`)
	}
}

// -------------------------------------------------------------------
// load: success path
// -------------------------------------------------------------------

func TestEval_LoadExistingFileReturnsT(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "init.el")
	if err := os.WriteFile(path, []byte("(setq loaded-flag 99)\n"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	ev := newEval()
	v := mustEval(t, ev, `(load "`+path+`")`)
	if b, ok := v.(Bool); !ok || !b.V {
		t.Fatalf("load returned %v, want t", v)
	}
	got, ok := ev.GetGlobalVar("loaded-flag")
	if !ok {
		t.Fatal("loaded-flag not set by loaded file")
	}
	if i, ok := got.(Int); !ok || i.V != 99 {
		t.Fatalf("loaded-flag = %v, want 99", got)
	}
}

// -------------------------------------------------------------------
// goFns fallback when the name is absent from the variable environment
// -------------------------------------------------------------------
//
// RegisterGoFn mirrors each function into the global variable environment, so
// the goFns fallback in evalSymbol / evalFunction is only reached for names
// present in the function namespace alone. These tests populate goFns directly
// to cover that path.

func TestEval_EvalSymbolFallsBackToGoFns(t *testing.T) {
	ev := newEval()
	ev.goFns["fn-only"] = func([]Value, *Env) (Value, error) { return Int{V: 5}, nil }
	if _, bound := ev.GetGlobalVar("fn-only"); bound {
		t.Fatal("precondition: fn-only must not be a bound variable")
	}
	v := mustEval(t, ev, "fn-only")
	b, ok := v.(Builtin)
	if !ok {
		t.Fatalf("expected Builtin, got %T", v)
	}
	if b.Name != "fn-only" {
		t.Fatalf("builtin name = %q, want fn-only", b.Name)
	}
}

func TestEval_FunctionFormFallsBackToGoFns(t *testing.T) {
	ev := newEval()
	ev.goFns["fn-only"] = func(args []Value, _ *Env) (Value, error) { return Int{V: 5}, nil }
	v := mustEval(t, ev, "(funcall #'fn-only)")
	if i, ok := v.(Int); !ok || i.V != 5 {
		t.Fatalf("got %v, want 5", v)
	}
}

func TestEval_CondSymbolTIsElseClause(t *testing.T) {
	// The reader turns `t` into Bool, so a literal Symbol{"t"} test only occurs
	// in forms built programmatically. evalCond must still treat it as `else`.
	ev := newEval()
	form := List(
		Symbol{Name: "cond"},
		List(Nil{}, Int{V: 1}),
		List(Symbol{Name: "t"}, Int{V: 2}),
	)
	got, err := ev.Eval(form, ev.global)
	if err != nil {
		t.Fatalf("Eval(cond): %v", err)
	}
	if i, ok := got.(Int); !ok || i.V != 2 {
		t.Fatalf("got %v, want 2", got)
	}
}
