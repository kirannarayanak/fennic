package interp

import (
	"context"
	"fmt"
	"strings"

	"github.com/kirannarayanak/fennic/ast"
	"github.com/kirannarayanak/fennic/token"
	"github.com/kirannarayanak/fennic/types"
)

func (in *Interp) evalStmt(s ast.Stmt, env *Env, t *task) Value {
	switch s := s.(type) {
	case *ast.LetStmt:
		env.Define(s.Name.Name, in.eval(s.Value, env, t), s.Mutable)
		return Unit
	case *ast.AssignStmt:
		v := in.eval(s.Value, env, t)
		if !env.Assign(s.Name.Name, v) {
			in.fail(s.Name, fmt.Sprintf("declare it first with `var %s = ...`", s.Name.Name), "`%s` is not defined", s.Name.Name)
		}
		return Unit
	case *ast.AssertStmt:
		a := &assertion{stmt: s}
		var cond Value
		if b, ok := s.Cond.(*ast.BinaryExpr); ok && b.Op == token.Eq {
			a.left, a.right, a.compared = in.eval(b.Left, env, t), in.eval(b.Right, env, t), true
			cond = equal(a.left, a.right)
		} else {
			cond = in.eval(s.Cond, env, t)
		}
		ok, isBool := cond.(bool)
		if !isBool {
			in.fail(s.Cond, "assert a comparison, like `assert x == 3`", "assert needs a Bool, but this is a %s", kindName(cond))
		}
		if !ok {
			panic(a)
		}
		return Unit
	case *ast.ExprStmt:
		return in.eval(s.X, env, t)
	}
	in.fail(s, "", "this statement could not be parsed")
	return nil
}

// evalBlock runs a block in its own scope. Its value is its last statement's,
// if that is an expression, and nothing otherwise.
func (in *Interp) evalBlock(b *ast.Block, env *Env, t *task) Value {
	scope := NewEnv(env)
	var last Value = Unit
	for _, s := range b.Stmts {
		v := in.evalStmt(s, scope, t)
		if _, ok := s.(*ast.ExprStmt); ok {
			last = v
		} else {
			last = Unit
		}
	}
	return last
}

func (in *Interp) eval(e ast.Expr, env *Env, t *task) Value {
	switch e := e.(type) {
	case *ast.IntLit:
		return e.Value
	case *ast.FloatLit:
		return e.Value
	case *ast.StrLit:
		return e.Value
	case *ast.BoolLit:
		return e.Value
	case *ast.NoneLit:
		return None
	case *ast.InterpStr:
		var b strings.Builder
		for _, p := range e.Parts {
			b.WriteString(Display(in.eval(p, env, t)))
		}
		return b.String()
	case *ast.Ident:
		v, ok := env.Lookup(e.Name)
		if !ok {
			in.fail(e, fmt.Sprintf("declare it with `let %s = ...` before this line, or check the spelling", e.Name), "`%s` is not defined", e.Name)
		}
		return v
	case *ast.ListLit:
		elems := make([]Value, len(e.Elems))
		for i, x := range e.Elems {
			elems[i] = in.eval(x, env, t)
		}
		return &List{Elems: elems}
	case *ast.RecordLit:
		return in.evalRecord(e, env, t)
	case *ast.WithExpr:
		return in.evalWith(e, env, t)
	case *ast.UnaryExpr:
		switch v := in.eval(e.X, env, t).(type) {
		case int64:
			return -v
		case float64:
			return -v
		default:
			in.fail(e, "", "`-` needs an Int or a Float, but this is a %s", kindName(v))
		}
	case *ast.BinaryExpr:
		return in.evalBinary(e, env, t)
	case *ast.PipeExpr:
		x := in.eval(e.X, env, t)
		fn := in.eval(e.Call.Fn, env, t)
		args := append([]Value{x}, in.evalArgs(e.Call.Args, env, t)...)
		return in.call(fn, args, &callSite{node: e, t: t})
	case *ast.CallExpr:
		fn := in.eval(e.Fn, env, t)
		return in.call(fn, in.evalArgs(e.Args, env, t), &callSite{node: e, t: t})
	case *ast.FieldExpr:
		return in.evalField(e, env, t)
	case *ast.PropagateExpr:
		v := in.eval(e.X, env, t)
		if f, ok := v.(*Failure); ok {
			panic(&propagation{failure: f, at: e})
		}
		return v
	case *ast.OrExpr:
		v := in.eval(e.X, env, t)
		f, ok := v.(*Failure)
		if !ok {
			return v
		}
		if e.ErrName == nil {
			return in.eval(e.Fallback, env, t)
		}
		scope := NewEnv(env)
		scope.Define(e.ErrName.Name, f, false)
		return in.eval(e.Fallback, scope, t)
	case *ast.LambdaExpr:
		return &Closure{Lambda: e, Env: env}
	case *ast.Block:
		return in.evalBlock(e, env, t)
	case *ast.MatchExpr:
		return in.evalMatch(e, env, t)
	case *ast.TogetherExpr:
		return in.evalTogether(e, env, t)
	}
	in.fail(e, "", "this expression could not be parsed")
	return nil
}

func (in *Interp) evalArgs(xs []ast.Expr, env *Env, t *task) []Value {
	args := make([]Value, len(xs))
	for i, x := range xs {
		args[i] = in.eval(x, env, t)
	}
	return args
}

func (in *Interp) evalRecord(e *ast.RecordLit, env *Env, t *task) Value {
	rt, ok := in.records[e.Type.Name]
	if !ok {
		if vt, isVariant := in.variants[e.Type.Name]; isVariant {
			in.fail(e.Type, fmt.Sprintf("build a variant with parentheses: `%s(%s)`", vt.Name, joinNames(vt.Fields)),
				"`%s` is a variant, not a record", e.Type.Name)
		}
		in.fail(e.Type, "declare it with `type "+e.Type.Name+" { ... }`", "there is no record type `%s`", e.Type.Name)
	}
	fields := make([]Value, len(rt.Fields))
	set := make([]bool, len(rt.Fields))
	for _, fi := range e.Fields {
		i := rt.index(fi.Name.Name)
		if i < 0 {
			in.fail(fi.Name, "its fields are "+joinNames(rt.Fields), "`%s` has no field `%s`", rt.Name, fi.Name.Name)
		}
		if set[i] {
			in.fail(fi.Name, "remove one of them", "field `%s` is given twice", fi.Name.Name)
		}
		fields[i], set[i] = in.eval(fi.Value, env, t), true
	}
	for i, ok := range set {
		if !ok {
			in.fail(e, fmt.Sprintf("records have no defaults; add `%s: ...` (use `none` if it is optional)", rt.Fields[i]),
				"`%s` is missing field `%s`", rt.Name, rt.Fields[i])
		}
	}
	return &Record{Type: rt, Fields: fields}
}

func (in *Interp) evalWith(e *ast.WithExpr, env *Env, t *task) Value {
	r, ok := in.eval(e.X, env, t).(*Record)
	if !ok {
		in.fail(e.X, "", "`with` copies a record, but this is not one")
	}
	fields := append([]Value(nil), r.Fields...)
	for _, fi := range e.Fields {
		i := r.Type.index(fi.Name.Name)
		if i < 0 {
			in.fail(fi.Name, "its fields are "+joinNames(r.Type.Fields), "`%s` has no field `%s`", kindName(r), fi.Name.Name)
		}
		fields[i] = in.eval(fi.Value, env, t)
	}
	return &Record{Type: r.Type, Fields: fields}
}

func (in *Interp) evalField(e *ast.FieldExpr, env *Env, t *task) Value {
	x := in.eval(e.X, env, t)
	name := e.Name.Name
	switch v := x.(type) {
	case noneValue:
		if e.Optional {
			return None
		}
		in.fail(e.X, fmt.Sprintf("use `?.%s` to skip a missing value, or `??` to give a default", name),
			"this value is none, so it has no `%s`", name)
	case *Module:
		if m, ok := v.Members[name]; ok {
			return m
		}
		in.fail(e.Name, "", "`%s` has no `%s`", v.Name, name)
	case *TypeRef:
		if f, ok := in.methods[v.Name][name]; ok {
			if f.Decl.HasSelf() {
				in.fail(e, fmt.Sprintf("call it on a value, like `x.%s(...)`", name), "`%s.%s` is a method and needs a value to work on", v.Name, name)
			}
			return f
		}
		in.fail(e.Name, fmt.Sprintf("declare it with `fn %s.%s(...)`", v.Name, name), "`%s` has no function `%s`", v.Name, name)
	case *Record:
		if i := v.Type.index(name); i >= 0 {
			return v.Fields[i]
		}
	}
	if f, ok := in.methods[typeName(x)][name]; ok && f.Decl.HasSelf() {
		return &Bound{Func: f, Self: x}
	}
	help := ""
	if r, ok := x.(*Record); ok {
		help = "its fields are " + joinNames(r.Type.Fields)
	}
	in.fail(e.Name, help, "%s has no field or method `%s`", article(kindName(x)), name)
	return nil
}

func typeName(v Value) string {
	switch v := v.(type) {
	case *Record:
		return v.Type.Name
	case *Variant:
		return v.Type.Sum
	}
	return kindName(v)
}

func (in *Interp) evalMatch(e *ast.MatchExpr, env *Env, t *task) Value {
	x := in.eval(e.X, env, t)
	v, ok := x.(*Variant)
	if !ok {
		in.fail(e.X, "", "`match` takes apart a sum type, but this is %s", article(kindName(x)))
	}
	for _, arm := range e.Arms {
		if arm.Pattern.Name.Name != v.Type.Name {
			continue
		}
		if len(arm.Pattern.Binds) != len(v.Fields) {
			in.fail(arm.Pattern, fmt.Sprintf("write `%s(%s)`", v.Type.Name, joinNames(v.Type.Fields)),
				"`%s` has %d %s, but the pattern names %d", v.Type.Name, len(v.Fields), plural(len(v.Fields), "field", "fields"), len(arm.Pattern.Binds))
		}
		scope := NewEnv(env)
		for i, b := range arm.Pattern.Binds {
			scope.Define(b.Name, v.Fields[i], false)
		}
		return in.eval(arm.Body, scope, t)
	}
	in.fail(e, fmt.Sprintf("add an arm: `%s(%s) -> ...`", v.Type.Name, joinNames(v.Type.Fields)), "no arm matches `%s`", v.Type.Name)
	return nil
}

// evalTogether runs a block as a structured scope. Its result is a failure
// if the block failed, which `!` or `or` then handle.
func (in *Interp) evalTogether(e *ast.TogetherExpr, env *Env, t *task) (result Value) {
	ctx, cancel := context.WithCancel(t.ctx)
	defer cancel()
	inner := &task{ctx: ctx, cancel: cancel, together: true}
	defer func() {
		if r := recover(); r != nil {
			p, ok := r.(*propagation)
			if !ok {
				panic(r)
			}
			result = p.failure
		}
	}()
	return in.evalBlock(e.Body, env, inner)
}

func (in *Interp) evalBinary(e *ast.BinaryExpr, env *Env, t *task) Value {
	if e.Op == token.QQ {
		l := in.eval(e.Left, env, t)
		if l == None {
			return in.eval(e.Right, env, t)
		}
		return l
	}
	l := in.eval(e.Left, env, t)
	r := in.eval(e.Right, env, t)
	switch e.Op {
	case token.Eq:
		return equal(l, r)
	case token.NotEq:
		return !equal(l, r)
	case token.Lt, token.LtEq, token.Gt, token.GtEq:
		c, ok := compare(l, r)
		if !ok {
			in.fail(e, "compare two Ints, two Floats or two Strs", "cannot compare %s with %s", article(kindName(l)), article(kindName(r)))
		}
		switch e.Op {
		case token.Lt:
			return c < 0
		case token.LtEq:
			return c <= 0
		case token.Gt:
			return c > 0
		}
		return c >= 0
	}
	return in.arith(e, l, r)
}

func (in *Interp) arith(e *ast.BinaryExpr, l, r Value) Value {
	switch a := l.(type) {
	case int64:
		if b, ok := r.(int64); ok {
			switch e.Op {
			case token.Plus:
				return a + b
			case token.Minus:
				return a - b
			case token.Star:
				return a * b
			case token.Slash, token.Percent:
				if b == 0 {
					in.fail(e.Right, "check the divisor before dividing", "division by zero")
				}
				if e.Op == token.Slash {
					return a / b
				}
				return a % b
			}
		}
	case float64:
		if b, ok := r.(float64); ok {
			switch e.Op {
			case token.Plus:
				return a + b
			case token.Minus:
				return a - b
			case token.Star:
				return a * b
			case token.Slash:
				return a / b
			case token.Percent:
				in.fail(e, "`%` works on Ints", "`%%` needs two Ints")
			}
		}
	case string:
		if _, ok := r.(string); ok && e.Op == token.Plus {
			in.fail(e, `to join strings, interpolate them: "{a}{b}"`, "`+` adds numbers, not strings")
		}
	}
	_, li := l.(int64)
	_, lf := l.(float64)
	_, ri := r.(int64)
	_, rf := r.(float64)
	if (li && rf) || (lf && ri) {
		in.fail(e, "write the Int as a Float, like `2.0` instead of `2`; Fennic never converts numbers for you",
			"`%s` cannot mix an Int and a Float", e.Op)
	}
	in.fail(e, "", "`%s` needs two numbers, but got %s and %s", e.Op, article(kindName(l)), article(kindName(r)))
	return nil
}

// equal is structural equality.
func equal(a, b Value) bool {
	switch x := a.(type) {
	case *List:
		y, ok := b.(*List)
		if !ok || len(x.Elems) != len(y.Elems) {
			return false
		}
		for i := range x.Elems {
			if !equal(x.Elems[i], y.Elems[i]) {
				return false
			}
		}
		return true
	case *Record:
		y, ok := b.(*Record)
		if !ok || x.Type != y.Type {
			return false
		}
		for i := range x.Fields {
			if !equal(x.Fields[i], y.Fields[i]) {
				return false
			}
		}
		return true
	case *Variant:
		y, ok := b.(*Variant)
		if !ok || x.Type != y.Type {
			return false
		}
		for i := range x.Fields {
			if !equal(x.Fields[i], y.Fields[i]) {
				return false
			}
		}
		return true
	case *Failure:
		y, ok := b.(*Failure)
		return ok && x.Msg == y.Msg
	case int64, float64, string, bool, noneValue, unitValue:
		return a == b
	}
	return false
}

// compare orders two values of the same kind: Ints, Floats or Strs.
func compare(a, b Value) (int, bool) {
	switch x := a.(type) {
	case int64:
		if y, ok := b.(int64); ok {
			return cmp3(x < y, x > y), true
		}
	case float64:
		if y, ok := b.(float64); ok {
			return cmp3(x < y, x > y), true
		}
	case string:
		if y, ok := b.(string); ok {
			return strings.Compare(x, y), true
		}
	}
	return 0, false
}

func cmp3(lt, gt bool) int {
	switch {
	case lt:
		return -1
	case gt:
		return 1
	}
	return 0
}

func joinNames(names []string) string { return strings.Join(names, ", ") }

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func article(kind string) string {
	switch kind {
	case "none", "nothing":
		return kind
	}
	if strings.HasPrefix(kind, "type ") || strings.HasPrefix(kind, "module ") {
		return kind
	}
	return types.Article(kind)
}
