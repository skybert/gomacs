package elisp

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// mustEval evaluates src in ev and calls t.Fatal if there is an error.
func mustEval(t *testing.T, ev *Evaluator, src string) Value {
	t.Helper()
	v, err := ev.EvalString(src)
	if err != nil {
		t.Fatalf("EvalString(%q): %v", src, err)
	}
	return v
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
// lexer — peek1/peek2 at EOF
// -------------------------------------------------------------------

func TestLexer_Peek1AtEOF(t *testing.T) {
	l := NewLexer("")
	if l.peek1() != 0 {
		t.Fatal("peek1 at EOF should return 0")
	}
}

func TestLexer_Peek2AtEOF(t *testing.T) {
	l := NewLexer("x")
	if l.peek2() != 0 {
		t.Fatal("peek2 at EOF should return 0")
	}
}

// -------------------------------------------------------------------
// lexer — readString escape sequences
// -------------------------------------------------------------------

func TestLexer_ReadStringTabEscape(t *testing.T) {
	l := NewLexer(`"\t"`)
	tok := l.Next()
	if tok.Type != TokenString || tok.Value != "\t" {
		t.Fatalf("expected tab char, got %q", tok.Value)
	}
}

func TestLexer_ReadStringUnknownEscape(t *testing.T) {
	l := NewLexer(`"\q"`)
	tok := l.Next()
	if tok.Type != TokenString || tok.Value != `\q` {
		t.Fatalf("expected literal \\q, got %q", tok.Value)
	}
}

func TestLexer_ReadStringEOFInEscape(t *testing.T) {
	// Unterminated string with backslash at end.
	l := NewLexer(`"\`)
	tok := l.Next()
	// Should return a string token (possibly empty/partial) without panicking.
	if tok.Type != TokenString {
		t.Fatalf("expected TokenString even for unterminated escape, got %v", tok.Type)
	}
}

// -------------------------------------------------------------------
// lexer — readNumber float with exponent
// -------------------------------------------------------------------

func TestLexer_ReadNumberExponent(t *testing.T) {
	l := NewLexer("1e5")
	tok := l.Next()
	if tok.Type != TokenFloat || tok.Value != "1e5" {
		t.Fatalf("expected TokenFloat \"1e5\", got type=%v val=%q", tok.Type, tok.Value)
	}
}

func TestLexer_ReadNumberNegativeExponent(t *testing.T) {
	l := NewLexer("2.5e-3")
	tok := l.Next()
	if tok.Type != TokenFloat || tok.Value != "2.5e-3" {
		t.Fatalf("expected TokenFloat \"2.5e-3\", got type=%v val=%q", tok.Type, tok.Value)
	}
}

func TestLexer_ReadNumberDotTrailingEOF(t *testing.T) {
	// "1." — dot followed by EOF; peek2 returns 0 which is not a digit,
	// but the condition `next == 0` makes it treated as float.
	l := NewLexer("1.")
	tok := l.Next()
	if tok.Type != TokenFloat {
		t.Fatalf("expected TokenFloat for \"1.\", got %v", tok.Type)
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
