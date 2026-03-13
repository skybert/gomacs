package elisp

import (
	"fmt"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Env is a linked-list environment (scope chain).
type Env struct {
	vars   map[string]Value
	parent *Env
}

// NewEnv creates a new root environment.
func NewEnv() *Env {
	return &Env{vars: make(map[string]Value)}
}

// NewChildEnv creates a child environment.
func NewChildEnv(parent *Env) *Env {
	return &Env{vars: make(map[string]Value), parent: parent}
}

// Set binds name to val in this (local) environment frame.
func (e *Env) Set(name string, val Value) {
	e.vars[name] = val
}

// Get looks up name, walking up the scope chain.
func (e *Env) Get(name string) (Value, bool) {
	if v, ok := e.vars[name]; ok {
		return v, true
	}
	if e.parent != nil {
		return e.parent.Get(name)
	}
	return nil, false
}

// SetGlobal walks up to the root environment and binds name there.
func (e *Env) SetGlobal(name string, val Value) {
	root := e
	for root.parent != nil {
		root = root.parent
	}
	root.vars[name] = val
}

// -------------------------------------------------------------------
// Evaluator
// -------------------------------------------------------------------

// Evaluator holds global interpreter state.
type Evaluator struct {
	global      *Env
	goFns       map[string]func(args []Value, env *Env) (Value, error)
	keyBindings map[string]string // key description → command name
}

// NewEvaluator creates an Evaluator with all built-ins registered.
func NewEvaluator() *Evaluator {
	ev := &Evaluator{
		global:      NewEnv(),
		goFns:       make(map[string]func(args []Value, env *Env) (Value, error)),
		keyBindings: make(map[string]string),
	}
	ev.registerBuiltins()
	return ev
}

// RegisterGoFn registers a Go function callable from Elisp.
func (ev *Evaluator) RegisterGoFn(name string, fn func(args []Value, env *Env) (Value, error)) {
	ev.goFns[name] = fn
	ev.global.Set(name, Builtin{Name: name, Fn: fn})
}

// GetKeyBindings returns the current key binding map.
func (ev *Evaluator) GetKeyBindings() map[string]string {
	return ev.keyBindings
}

// GlobalVarNames returns the names of all variables in the global environment.
func (ev *Evaluator) GlobalVarNames() []string {
	names := make([]string, 0, len(ev.global.vars))
	for name := range ev.global.vars {
		names = append(names, name)
	}
	return names
}

// GetGlobalVar returns the value of the named variable in the global
// environment, and whether it was found.
func (ev *Evaluator) GetGlobalVar(name string) (Value, bool) {
	return ev.global.Get(name)
}

// EvalString parses and evaluates a string, returning the last value.
func (ev *Evaluator) EvalString(src string) (Value, error) {
	forms, err := Parse(src)
	if err != nil {
		return nil, err
	}
	var result Value = Nil{}
	for _, f := range forms {
		result, err = ev.Eval(f, ev.global)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// EvalFile reads and evaluates a file.
func (ev *Evaluator) EvalFile(path string) error {
	data, err := os.ReadFile(path) //nolint:gosec // user-provided path is intentional
	if err != nil {
		return err
	}
	if !utf8.Valid(data) {
		return fmt.Errorf("%s: file is not valid UTF-8", path)
	}
	_, err = ev.EvalString(string(data))
	return err
}

// -------------------------------------------------------------------
// Eval
// -------------------------------------------------------------------

// Eval evaluates a Value in the given environment.
func (ev *Evaluator) Eval(v Value, env *Env) (Value, error) {
	switch val := v.(type) {
	case Nil:
		return val, nil
	case Bool:
		return val, nil
	case Int:
		return val, nil
	case Float:
		return val, nil
	case StringVal:
		return val, nil
	case Vector:
		elems := make([]Value, len(val.Elems))
		for i, e := range val.Elems {
			var err error
			elems[i], err = ev.Eval(e, env)
			if err != nil {
				return nil, err
			}
		}
		return Vector{Elems: elems}, nil
	case Symbol:
		return ev.evalSymbol(val, env)
	case Cons:
		return ev.evalCons(val, env)
	case Lambda:
		return val, nil
	case Builtin:
		return val, nil
	default:
		return nil, fmt.Errorf("cannot eval %T", v)
	}
}

func (ev *Evaluator) evalSymbol(s Symbol, env *Env) (Value, error) {
	v, ok := env.Get(s.Name)
	if !ok {
		// Check registered go functions
		if fn, ok2 := ev.goFns[s.Name]; ok2 {
			return Builtin{Name: s.Name, Fn: fn}, nil
		}
		return nil, fmt.Errorf("symbol's value as variable is void: %s", s.Name)
	}
	return v, nil
}

func (ev *Evaluator) evalCons(c Cons, env *Env) (Value, error) {
	// c.Car should be the function/special-form
	head, ok := c.Car.(Symbol)
	if !ok {
		// Could be a lambda expression directly or a form that evaluates to a function
		fn, err := ev.Eval(c.Car, env)
		if err != nil {
			return nil, err
		}
		args, ok2 := ToSlice(c.Cdr)
		if !ok2 {
			return nil, fmt.Errorf("improper argument list")
		}
		return ev.applyFunction(fn, args, env, true)
	}

	// Special forms (not evaluated like normal calls)
	switch head.Name {
	case "quote":
		return ev.evalQuote(c.Cdr)
	case "setq":
		return ev.evalSetq(c.Cdr, env)
	case "defvar":
		return ev.evalDefvar(c.Cdr, env)
	case "defun":
		return ev.evalDefun(c.Cdr, env)
	case "if":
		return ev.evalIf(c.Cdr, env)
	case "cond":
		return ev.evalCond(c.Cdr, env)
	case "progn":
		return ev.evalProgn(c.Cdr, env)
	case "let":
		return ev.evalLet(c.Cdr, env, false)
	case "let*":
		return ev.evalLet(c.Cdr, env, true)
	case "when":
		return ev.evalWhen(c.Cdr, env)
	case "unless":
		return ev.evalUnless(c.Cdr, env)
	case "lambda":
		return ev.evalLambda(c.Cdr, env)
	case "and":
		return ev.evalAnd(c.Cdr, env)
	case "or":
		return ev.evalOr(c.Cdr, env)
	case "quasiquote":
		args, ok2 := ToSlice(c.Cdr)
		if !ok2 || len(args) != 1 {
			return nil, fmt.Errorf("quasiquote: expected 1 argument")
		}
		return ev.evalQuasiquote(args[0], env)
	}

	// Normal function call: evaluate all arguments
	fn, err := ev.Eval(c.Car, env)
	if err != nil {
		return nil, fmt.Errorf("evaluating function %s: %w", head.Name, err)
	}
	args, ok2 := ToSlice(c.Cdr)
	if !ok2 {
		return nil, fmt.Errorf("improper argument list in call to %s", head.Name)
	}
	return ev.applyFunction(fn, args, env, true)
}

// applyFunction calls fn with args (optionally evaluating them first).
func (ev *Evaluator) applyFunction(fn Value, args []Value, env *Env, evalArgs bool) (Value, error) {
	var evaledArgs []Value
	if evalArgs {
		evaledArgs = make([]Value, len(args))
		for i, a := range args {
			var err error
			evaledArgs[i], err = ev.Eval(a, env)
			if err != nil {
				return nil, err
			}
		}
	} else {
		evaledArgs = args
	}

	switch f := fn.(type) {
	case Builtin:
		return f.Fn(evaledArgs, env)
	case Lambda:
		return ev.callLambda(f, evaledArgs)
	default:
		return nil, fmt.Errorf("not a function: %s", fn.String())
	}
}

func (ev *Evaluator) callLambda(l Lambda, args []Value) (Value, error) {
	callEnv := NewChildEnv(l.Env)

	// Bind positional parameters
	for i, p := range l.Params {
		if i < len(args) {
			callEnv.Set(p, args[i])
		} else {
			callEnv.Set(p, Nil{})
		}
	}

	// Bind &rest
	if l.Rest != "" {
		var rest []Value
		if len(l.Params) < len(args) {
			rest = args[len(l.Params):]
		}
		callEnv.Set(l.Rest, List(rest...))
	} else if len(args) > len(l.Params) {
		return nil, fmt.Errorf("too many arguments: expected %d, got %d", len(l.Params), len(args))
	}

	var result Value = Nil{}
	var err error
	for _, form := range l.Body {
		result, err = ev.Eval(form, callEnv)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// -------------------------------------------------------------------
// Special forms
// -------------------------------------------------------------------

func (ev *Evaluator) evalQuote(args Value) (Value, error) {
	s, ok := ToSlice(args)
	if !ok || len(s) != 1 {
		return nil, fmt.Errorf("quote: expected exactly 1 argument")
	}
	return s[0], nil
}

func (ev *Evaluator) evalSetq(args Value, env *Env) (Value, error) {
	s, ok := ToSlice(args)
	if !ok || len(s)%2 != 0 {
		return nil, fmt.Errorf("setq: expected even number of arguments")
	}
	var result Value = Nil{}
	for i := 0; i < len(s); i += 2 {
		sym, ok2 := s[i].(Symbol)
		if !ok2 {
			return nil, fmt.Errorf("setq: expected symbol, got %T", s[i])
		}
		val, err := ev.Eval(s[i+1], env)
		if err != nil {
			return nil, err
		}
		env.SetGlobal(sym.Name, val)
		result = val
	}
	return result, nil
}

func (ev *Evaluator) evalDefvar(args Value, env *Env) (Value, error) {
	s, ok := ToSlice(args)
	if !ok || len(s) < 1 {
		return nil, fmt.Errorf("defvar: expected at least 1 argument")
	}
	sym, ok2 := s[0].(Symbol)
	if !ok2 {
		return nil, fmt.Errorf("defvar: expected symbol, got %T", s[0])
	}
	// Only set if not already set
	if _, exists := env.Get(sym.Name); !exists {
		if len(s) >= 2 {
			val, err := ev.Eval(s[1], env)
			if err != nil {
				return nil, err
			}
			env.SetGlobal(sym.Name, val)
		}
	}
	return Symbol{Name: sym.Name}, nil
}

func (ev *Evaluator) evalDefun(args Value, env *Env) (Value, error) {
	s, ok := ToSlice(args)
	if !ok || len(s) < 2 {
		return nil, fmt.Errorf("defun: expected name, params, body")
	}
	sym, ok2 := s[0].(Symbol)
	if !ok2 {
		return nil, fmt.Errorf("defun: expected symbol as name, got %T", s[0])
	}
	params, rest, err := parseParamList(s[1])
	if err != nil {
		return nil, fmt.Errorf("defun %s: %w", sym.Name, err)
	}
	lam := Lambda{
		Params: params,
		Rest:   rest,
		Body:   s[2:],
		Env:    env,
	}
	env.SetGlobal(sym.Name, lam)
	return Symbol{Name: sym.Name}, nil
}

func parseParamList(v Value) (params []string, rest string, err error) {
	s, ok := ToSlice(v)
	if !ok {
		return nil, "", fmt.Errorf("expected parameter list")
	}
	restMode := false
	for _, p := range s {
		sym, ok2 := p.(Symbol)
		if !ok2 {
			return nil, "", fmt.Errorf("parameter must be symbol, got %T", p)
		}
		if sym.Name == "&rest" || sym.Name == "&body" {
			restMode = true
			continue
		}
		if sym.Name == "&optional" {
			continue
		}
		if restMode {
			rest = sym.Name
			break
		}
		params = append(params, sym.Name)
	}
	return params, rest, nil
}

func (ev *Evaluator) evalIf(args Value, env *Env) (Value, error) {
	s, ok := ToSlice(args)
	if !ok || len(s) < 2 {
		return nil, fmt.Errorf("if: expected cond then [else...]")
	}
	cond, err := ev.Eval(s[0], env)
	if err != nil {
		return nil, err
	}
	if isTruthy(cond) {
		return ev.Eval(s[1], env)
	}
	if len(s) > 2 {
		// else forms: evaluate as progn
		var result Value = Nil{}
		for _, f := range s[2:] {
			result, err = ev.Eval(f, env)
			if err != nil {
				return nil, err
			}
		}
		return result, nil
	}
	return Nil{}, nil
}

func (ev *Evaluator) evalCond(args Value, env *Env) (Value, error) {
	clauses, ok := ToSlice(args)
	if !ok {
		return nil, fmt.Errorf("cond: expected clauses")
	}
	for _, clause := range clauses {
		parts, ok2 := ToSlice(clause)
		if !ok2 || len(parts) == 0 {
			return nil, fmt.Errorf("cond: invalid clause %s", clause.String())
		}
		// Special case: t as the last condition (else)
		sym, isSym := parts[0].(Symbol)
		var test Value
		var err error
		if isSym && sym.Name == "t" {
			test = Bool{V: true}
		} else {
			test, err = ev.Eval(parts[0], env)
			if err != nil {
				return nil, err
			}
		}
		if isTruthy(test) {
			if len(parts) == 1 {
				return test, nil
			}
			var result Value = Nil{}
			for _, f := range parts[1:] {
				result, err = ev.Eval(f, env)
				if err != nil {
					return nil, err
				}
			}
			return result, nil
		}
	}
	return Nil{}, nil
}

func (ev *Evaluator) evalProgn(args Value, env *Env) (Value, error) {
	forms, ok := ToSlice(args)
	if !ok {
		return nil, fmt.Errorf("progn: invalid form list")
	}
	var result Value = Nil{}
	var err error
	for _, f := range forms {
		result, err = ev.Eval(f, env)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (ev *Evaluator) evalLet(args Value, env *Env, sequential bool) (Value, error) {
	s, ok := ToSlice(args)
	if !ok || len(s) < 1 {
		return nil, fmt.Errorf("let: expected bindings and body")
	}
	bindings, ok2 := ToSlice(s[0])
	if !ok2 {
		return nil, fmt.Errorf("let: expected binding list")
	}

	letEnv := NewChildEnv(env)
	evalEnv := env // for let, all vals evaluated in parent env
	if sequential {
		evalEnv = letEnv // for let*, each val sees previous bindings
	}

	for _, binding := range bindings {
		switch b := binding.(type) {
		case Symbol:
			letEnv.Set(b.Name, Nil{})
		case Cons:
			parts, ok3 := ToSlice(b)
			if !ok3 || len(parts) < 1 {
				return nil, fmt.Errorf("let: invalid binding")
			}
			sym, ok4 := parts[0].(Symbol)
			if !ok4 {
				return nil, fmt.Errorf("let: binding name must be symbol")
			}
			var val Value = Nil{}
			var err error
			if len(parts) >= 2 {
				val, err = ev.Eval(parts[1], evalEnv)
				if err != nil {
					return nil, err
				}
			}
			letEnv.Set(sym.Name, val)
		case Nil:
			// empty binding (nil) — skip
		default:
			return nil, fmt.Errorf("let: unexpected binding type %T", binding)
		}
	}

	var result Value = Nil{}
	var err error
	for _, f := range s[1:] {
		result, err = ev.Eval(f, letEnv)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (ev *Evaluator) evalWhen(args Value, env *Env) (Value, error) {
	s, ok := ToSlice(args)
	if !ok || len(s) < 1 {
		return nil, fmt.Errorf("when: expected condition and body")
	}
	cond, err := ev.Eval(s[0], env)
	if err != nil {
		return nil, err
	}
	if !isTruthy(cond) {
		return Nil{}, nil
	}
	var result Value = Nil{}
	for _, f := range s[1:] {
		result, err = ev.Eval(f, env)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (ev *Evaluator) evalUnless(args Value, env *Env) (Value, error) {
	s, ok := ToSlice(args)
	if !ok || len(s) < 1 {
		return nil, fmt.Errorf("unless: expected condition and body")
	}
	cond, err := ev.Eval(s[0], env)
	if err != nil {
		return nil, err
	}
	if isTruthy(cond) {
		return Nil{}, nil
	}
	var result Value = Nil{}
	for _, f := range s[1:] {
		result, err = ev.Eval(f, env)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (ev *Evaluator) evalLambda(args Value, env *Env) (Value, error) {
	s, ok := ToSlice(args)
	if !ok || len(s) < 1 {
		return nil, fmt.Errorf("lambda: expected params and body")
	}
	params, rest, err := parseParamList(s[0])
	if err != nil {
		return nil, fmt.Errorf("lambda: %w", err)
	}
	return Lambda{
		Params: params,
		Rest:   rest,
		Body:   s[1:],
		Env:    env,
	}, nil
}

func (ev *Evaluator) evalAnd(args Value, env *Env) (Value, error) {
	forms, ok := ToSlice(args)
	if !ok {
		return nil, fmt.Errorf("and: invalid form")
	}
	var result Value = Bool{V: true}
	var err error
	for _, f := range forms {
		result, err = ev.Eval(f, env)
		if err != nil {
			return nil, err
		}
		if !isTruthy(result) {
			return Nil{}, nil
		}
	}
	return result, nil
}

func (ev *Evaluator) evalOr(args Value, env *Env) (Value, error) {
	forms, ok := ToSlice(args)
	if !ok {
		return nil, fmt.Errorf("or: invalid form")
	}
	var err error
	for _, f := range forms {
		v, evalErr := ev.Eval(f, env)
		if evalErr != nil {
			err = evalErr
			return nil, err
		}
		if isTruthy(v) {
			return v, nil
		}
	}
	return Nil{}, nil
}

func (ev *Evaluator) evalQuasiquote(v Value, env *Env) (Value, error) {
	switch val := v.(type) {
	case Cons:
		// Check for unquote
		if sym, ok := val.Car.(Symbol); ok && sym.Name == "unquote" {
			args, ok2 := ToSlice(val.Cdr)
			if ok2 && len(args) == 1 {
				return ev.Eval(args[0], env)
			}
		}
		// Process list elements
		car, err := ev.evalQuasiquote(val.Car, env)
		if err != nil {
			return nil, err
		}
		cdr, err := ev.evalQuasiquote(val.Cdr, env)
		if err != nil {
			return nil, err
		}
		return Cons{Car: car, Cdr: cdr}, nil
	default:
		return v, nil
	}
}

// -------------------------------------------------------------------
// Built-in functions
// -------------------------------------------------------------------

func isTruthy(v Value) bool {
	return !IsNil(v)
}

func (ev *Evaluator) registerBuiltins() {
	builtins := map[string]func(args []Value, env *Env) (Value, error){
		"not": func(args []Value, env *Env) (Value, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("not: expected 1 argument")
			}
			if IsNil(args[0]) {
				return Bool{V: true}, nil
			}
			return Nil{}, nil
		},
		"null": func(args []Value, env *Env) (Value, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("null: expected 1 argument")
			}
			if IsNil(args[0]) {
				return Bool{V: true}, nil
			}
			return Nil{}, nil
		},
		"list": func(args []Value, env *Env) (Value, error) {
			return List(args...), nil
		},
		"car": func(args []Value, env *Env) (Value, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("car: expected 1 argument")
			}
			if IsNil(args[0]) {
				return Nil{}, nil
			}
			c, ok := args[0].(Cons)
			if !ok {
				return nil, fmt.Errorf("car: expected cons, got %T", args[0])
			}
			return c.Car, nil
		},
		"cdr": func(args []Value, env *Env) (Value, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("cdr: expected 1 argument")
			}
			if IsNil(args[0]) {
				return Nil{}, nil
			}
			c, ok := args[0].(Cons)
			if !ok {
				return nil, fmt.Errorf("cdr: expected cons, got %T", args[0])
			}
			return c.Cdr, nil
		},
		"cons": func(args []Value, env *Env) (Value, error) {
			if len(args) != 2 {
				return nil, fmt.Errorf("cons: expected 2 arguments")
			}
			return Cons{Car: args[0], Cdr: args[1]}, nil
		},
		"eq": func(args []Value, env *Env) (Value, error) {
			if len(args) != 2 {
				return nil, fmt.Errorf("eq: expected 2 arguments")
			}
			if eqValues(args[0], args[1]) {
				return Bool{V: true}, nil
			}
			return Nil{}, nil
		},
		"equal": func(args []Value, env *Env) (Value, error) {
			if len(args) != 2 {
				return nil, fmt.Errorf("equal: expected 2 arguments")
			}
			if equalValues(args[0], args[1]) {
				return Bool{V: true}, nil
			}
			return Nil{}, nil
		},
		"+": func(args []Value, env *Env) (Value, error) {
			return numericFold(args, 0, 0.0, func(a, b int64) int64 { return a + b },
				func(a, b float64) float64 { return a + b })
		},
		"-": func(args []Value, env *Env) (Value, error) {
			if len(args) == 0 {
				return Int{V: 0}, nil
			}
			if len(args) == 1 {
				switch v := args[0].(type) {
				case Int:
					return Int{V: -v.V}, nil
				case Float:
					return Float{V: -v.V}, nil
				}
				return nil, fmt.Errorf("-: expected number")
			}
			return numericFoldFrom(args, func(a, b int64) int64 { return a - b },
				func(a, b float64) float64 { return a - b })
		},
		"*": func(args []Value, env *Env) (Value, error) {
			return numericFold(args, 1, 1.0, func(a, b int64) int64 { return a * b },
				func(a, b float64) float64 { return a * b })
		},
		"/": func(args []Value, env *Env) (Value, error) {
			if len(args) < 2 {
				return nil, fmt.Errorf("/: expected at least 2 arguments")
			}
			return numericFoldFrom(args,
				func(a, b int64) int64 {
					if b == 0 {
						panic("division by zero")
					}
					return a / b
				},
				func(a, b float64) float64 { return a / b })
		},
		"<": func(args []Value, env *Env) (Value, error) {
			return numericCompare(args, func(a, b float64) bool { return a < b })
		},
		">": func(args []Value, env *Env) (Value, error) {
			return numericCompare(args, func(a, b float64) bool { return a > b })
		},
		"<=": func(args []Value, env *Env) (Value, error) {
			return numericCompare(args, func(a, b float64) bool { return a <= b })
		},
		">=": func(args []Value, env *Env) (Value, error) {
			return numericCompare(args, func(a, b float64) bool { return a >= b })
		},
		"=": func(args []Value, env *Env) (Value, error) {
			return numericCompare(args, func(a, b float64) bool { return a == b })
		},
		"concat": func(args []Value, env *Env) (Value, error) {
			var sb strings.Builder
			for _, a := range args {
				switch v := a.(type) {
				case StringVal:
					sb.WriteString(v.V)
				case Symbol:
					sb.WriteString(v.Name)
				default:
					sb.WriteString(v.String())
				}
			}
			return StringVal{V: sb.String()}, nil
		},
		"length": func(args []Value, env *Env) (Value, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("length: expected 1 argument")
			}
			switch v := args[0].(type) {
			case StringVal:
				return Int{V: int64(len([]rune(v.V)))}, nil
			case Vector:
				return Int{V: int64(len(v.Elems))}, nil
			default:
				// count list
				elems, ok := ToSlice(args[0])
				if !ok {
					return nil, fmt.Errorf("length: expected string or list")
				}
				return Int{V: int64(len(elems))}, nil
			}
		},
		"message": func(args []Value, env *Env) (Value, error) {
			if len(args) == 0 {
				return StringVal{V: ""}, nil
			}
			msg, err := formatString(args)
			if err != nil {
				return nil, err
			}
			fmt.Fprintln(os.Stderr, msg)
			return StringVal{V: msg}, nil
		},
		"format": func(args []Value, env *Env) (Value, error) {
			msg, err := formatString(args)
			if err != nil {
				return nil, err
			}
			return StringVal{V: msg}, nil
		},
		"string-to-int": func(args []Value, env *Env) (Value, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("string-to-int: expected 1 argument")
			}
			s, ok := args[0].(StringVal)
			if !ok {
				return nil, fmt.Errorf("string-to-int: expected string")
			}
			n, err := strconv.ParseInt(strings.TrimSpace(s.V), 10, 64)
			if err != nil {
				return Int{V: 0}, err
			}
			return Int{V: n}, nil
		},
		"int-to-string": func(args []Value, env *Env) (Value, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("int-to-string: expected 1 argument")
			}
			switch v := args[0].(type) {
			case Int:
				return StringVal{V: strconv.FormatInt(v.V, 10)}, nil
			case Float:
				return StringVal{V: strconv.FormatFloat(v.V, 'g', -1, 64)}, nil
			default:
				return nil, fmt.Errorf("int-to-string: expected number")
			}
		},
		"number-to-string": func(args []Value, env *Env) (Value, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("number-to-string: expected 1 argument")
			}
			switch v := args[0].(type) {
			case Int:
				return StringVal{V: strconv.FormatInt(v.V, 10)}, nil
			case Float:
				return StringVal{V: strconv.FormatFloat(v.V, 'g', -1, 64)}, nil
			default:
				return nil, fmt.Errorf("number-to-string: expected number")
			}
		},
		"stringp": func(args []Value, env *Env) (Value, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("stringp: expected 1 argument")
			}
			if _, ok := args[0].(StringVal); ok {
				return Bool{V: true}, nil
			}
			return Nil{}, nil
		},
		"numberp": func(args []Value, env *Env) (Value, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("numberp: expected 1 argument")
			}
			switch args[0].(type) {
			case Int, Float:
				return Bool{V: true}, nil
			}
			return Nil{}, nil
		},
		"symbolp": func(args []Value, env *Env) (Value, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("symbolp: expected 1 argument")
			}
			switch args[0].(type) {
			case Symbol, Nil, Bool:
				return Bool{V: true}, nil
			}
			return Nil{}, nil
		},
		"listp": func(args []Value, env *Env) (Value, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("listp: expected 1 argument")
			}
			switch args[0].(type) {
			case Cons, Nil:
				return Bool{V: true}, nil
			}
			return Nil{}, nil
		},
		"functionp": func(args []Value, env *Env) (Value, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("functionp: expected 1 argument")
			}
			switch args[0].(type) {
			case Lambda, Builtin:
				return Bool{V: true}, nil
			}
			return Nil{}, nil
		},
		"global-set-key": func(args []Value, env *Env) (Value, error) {
			if len(args) != 2 {
				return nil, fmt.Errorf("global-set-key: expected 2 arguments")
			}
			keyDesc, ok := args[0].(StringVal)
			if !ok {
				return nil, fmt.Errorf("global-set-key: expected string key description")
			}
			var cmdName string
			switch v := args[1].(type) {
			case Symbol:
				cmdName = v.Name
			case StringVal:
				cmdName = v.V
			default:
				cmdName = args[1].String()
			}
			ev.keyBindings[keyDesc.V] = cmdName
			return Nil{}, nil
		},
		"kbd": func(args []Value, env *Env) (Value, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("kbd: expected 1 argument")
			}
			s, ok := args[0].(StringVal)
			if !ok {
				return nil, fmt.Errorf("kbd: expected string")
			}
			return StringVal{V: s.V}, nil
		},
		"require": func(args []Value, env *Env) (Value, error) {
			// no-op: in a real implementation this would load a feature
			if len(args) >= 1 {
				return args[0], nil
			}
			return Nil{}, nil
		},
		"load": func(args []Value, env *Env) (Value, error) {
			if len(args) < 1 {
				return nil, fmt.Errorf("load: expected filename")
			}
			s, ok := args[0].(StringVal)
			if !ok {
				return nil, fmt.Errorf("load: expected string filename")
			}
			err := ev.EvalFile(s.V)
			if err != nil {
				// If the file doesn't exist and noerror is set, return nil
				if len(args) >= 3 && isTruthy(args[2]) {
					return Nil{}, nil
				}
				return nil, err
			}
			return Bool{V: true}, nil
		},
		"funcall": func(args []Value, env *Env) (Value, error) {
			if len(args) < 1 {
				return nil, fmt.Errorf("funcall: expected function")
			}
			return ev.applyFunction(args[0], args[1:], env, false)
		},
		"apply": func(args []Value, env *Env) (Value, error) {
			if len(args) < 2 {
				return nil, fmt.Errorf("apply: expected function and args")
			}
			fn := args[0]
			// Last arg must be a list; preceding args are prepended
			lastList, ok := ToSlice(args[len(args)-1])
			if !ok {
				return nil, fmt.Errorf("apply: last argument must be a list")
			}
			prefix := args[1 : len(args)-1]
			allArgs := make([]Value, len(prefix)+len(lastList))
			copy(allArgs, prefix)
			copy(allArgs[len(prefix):], lastList)
			return ev.applyFunction(fn, allArgs, env, false)
		},
		"mapcar": func(args []Value, env *Env) (Value, error) {
			if len(args) != 2 {
				return nil, fmt.Errorf("mapcar: expected function and list")
			}
			fn := args[0]
			elems, ok := ToSlice(args[1])
			if !ok {
				return nil, fmt.Errorf("mapcar: expected list")
			}
			results := make([]Value, len(elems))
			for i, e := range elems {
				var err error
				results[i], err = ev.applyFunction(fn, []Value{e}, env, false)
				if err != nil {
					return nil, err
				}
			}
			return List(results...), nil
		},
	}

	for name, fn := range builtins {
		ev.global.Set(name, Builtin{Name: name, Fn: fn})
	}
}

// -------------------------------------------------------------------
// Numeric helpers
// -------------------------------------------------------------------

func toFloat(v Value) (float64, bool) {
	switch n := v.(type) {
	case Int:
		return float64(n.V), true
	case Float:
		return n.V, true
	}
	return 0, false
}

func isFloat(v Value) bool {
	_, ok := v.(Float)
	return ok
}

func numericFold(args []Value, initI int64, initF float64,
	opI func(int64, int64) int64, opF func(float64, float64) float64) (Value, error) {
	anyFloat := slices.ContainsFunc(args, isFloat)
	if anyFloat {
		acc := initF
		for _, a := range args {
			f, ok := toFloat(a)
			if !ok {
				return nil, fmt.Errorf("arithmetic: expected number, got %T", a)
			}
			acc = opF(acc, f)
		}
		return Float{V: acc}, nil
	}
	acc := initI
	for _, a := range args {
		i, ok := a.(Int)
		if !ok {
			return nil, fmt.Errorf("arithmetic: expected integer, got %T", a)
		}
		acc = opI(acc, i.V)
	}
	return Int{V: acc}, nil
}

func numericFoldFrom(args []Value, opI func(int64, int64) int64, opF func(float64, float64) float64) (Value, error) {
	anyFloat := slices.ContainsFunc(args, isFloat)
	if anyFloat {
		first, ok := toFloat(args[0])
		if !ok {
			return nil, fmt.Errorf("arithmetic: expected number, got %T", args[0])
		}
		acc := first
		for _, a := range args[1:] {
			f, ok2 := toFloat(a)
			if !ok2 {
				return nil, fmt.Errorf("arithmetic: expected number, got %T", a)
			}
			acc = opF(acc, f)
		}
		return Float{V: acc}, nil
	}
	first, ok := args[0].(Int)
	if !ok {
		return nil, fmt.Errorf("arithmetic: expected integer, got %T", args[0])
	}
	acc := first.V
	for _, a := range args[1:] {
		i, ok2 := a.(Int)
		if !ok2 {
			return nil, fmt.Errorf("arithmetic: expected integer, got %T", a)
		}
		acc = opI(acc, i.V)
	}
	return Int{V: acc}, nil
}

func numericCompare(args []Value, cmp func(float64, float64) bool) (Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("comparison: expected at least 2 arguments")
	}
	for i := range len(args) - 1 {
		a, ok1 := toFloat(args[i])
		b, ok2 := toFloat(args[i+1])
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("comparison: expected numbers")
		}
		if !cmp(a, b) {
			return Nil{}, nil
		}
	}
	return Bool{V: true}, nil
}

// -------------------------------------------------------------------
// Equality
// -------------------------------------------------------------------

func eqValues(a, b Value) bool {
	switch av := a.(type) {
	case Nil:
		return IsNil(b)
	case Bool:
		bv, ok := b.(Bool)
		return ok && av.V == bv.V
	case Int:
		bv, ok := b.(Int)
		return ok && av.V == bv.V
	case Float:
		bv, ok := b.(Float)
		return ok && av.V == bv.V
	case Symbol:
		bv, ok := b.(Symbol)
		return ok && av.Name == bv.Name
	case StringVal:
		bv, ok := b.(StringVal)
		return ok && av.V == bv.V
	}
	return false
}

func equalValues(a, b Value) bool {
	switch av := a.(type) {
	case Cons:
		bv, ok := b.(Cons)
		if !ok {
			return false
		}
		return equalValues(av.Car, bv.Car) && equalValues(av.Cdr, bv.Cdr)
	case Vector:
		bv, ok := b.(Vector)
		if !ok || len(av.Elems) != len(bv.Elems) {
			return false
		}
		for i := range av.Elems {
			if !equalValues(av.Elems[i], bv.Elems[i]) {
				return false
			}
		}
		return true
	}
	return eqValues(a, b)
}

// -------------------------------------------------------------------
// Format
// -------------------------------------------------------------------

func formatString(args []Value) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	fmtStr, ok := args[0].(StringVal)
	if !ok {
		// No format string: just convert to string
		return args[0].String(), nil
	}
	if len(args) == 1 {
		return fmtStr.V, nil
	}

	var sb strings.Builder
	argIdx := 1
	runes := []rune(fmtStr.V)
	for i := 0; i < len(runes); i++ {
		if runes[i] != '%' || i+1 >= len(runes) {
			sb.WriteRune(runes[i])
			continue
		}
		i++
		if argIdx >= len(args) {
			sb.WriteRune('%')
			sb.WriteRune(runes[i])
			continue
		}
		arg := args[argIdx]
		argIdx++
		switch runes[i] {
		case 's':
			switch v := arg.(type) {
			case StringVal:
				sb.WriteString(v.V)
			default:
				sb.WriteString(v.String())
			}
		case 'd':
			switch v := arg.(type) {
			case Int:
				sb.WriteString(strconv.FormatInt(v.V, 10))
			case Float:
				sb.WriteString(strconv.FormatInt(int64(math.Round(v.V)), 10))
			default:
				sb.WriteString(v.String())
			}
		case 'f':
			switch v := arg.(type) {
			case Float:
				sb.WriteString(strconv.FormatFloat(v.V, 'f', 6, 64))
			case Int:
				sb.WriteString(strconv.FormatFloat(float64(v.V), 'f', 6, 64))
			default:
				sb.WriteString(v.String())
			}
		case '%':
			sb.WriteRune('%')
			argIdx-- // no arg consumed
		default:
			sb.WriteRune('%')
			sb.WriteRune(runes[i])
			argIdx--
		}
	}
	return sb.String(), nil
}
