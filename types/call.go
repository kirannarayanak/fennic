package types

import (
	"fmt"
	"strings"

	"github.com/kirannarayanak/fennic/ast"
)

// call checks a call. For `a |> f(b)`, pipeX is `a` and becomes the first
// argument.
func (c *checker) call(node ast.Node, callee ast.Expr, args []ast.Expr, pipeX ast.Expr, sc *scope, x *ctx, want Type) Type {
	if pipeX != nil {
		args = append([]ast.Expr{pipeX}, args...)
	}
	switch fn := callee.(type) {
	case *ast.Ident:
		name := fn.Name
		if b, ok := sc.lookup(name); ok {
			return c.callValue(node, "`"+name+"`", b.t, args, sc, x)
		}
		if s, ok := c.funcs[name]; ok {
			return c.callSig(node, s, args, sc, x)
		}
		if builtins[name] {
			return c.builtin(name, node, args, sc, x, want)
		}
		if v, ok := c.variant[name]; ok {
			return c.construct(node, v, args, sc, x)
		}
		if n, ok := c.named[name]; ok && !n.IsSum() {
			c.checkArgsLoosely(args, sc, x)
			c.errorf(fn, fmt.Sprintf("build a record with braces: `%s { ... }`", name), "`%s` is a record type, not a function", name)
			return bad
		}
		c.checkArgsLoosely(args, sc, x)
		c.ident(fn, sc)
		return bad
	case *ast.FieldExpr:
		if id, ok := fn.X.(*ast.Ident); ok {
			if _, isValue := sc.lookup(id.Name); !isValue {
				if modules[id.Name] {
					return c.builtin(id.Name+"."+fn.Name.Name, node, args, sc, x, want)
				}
				if _, isType := c.named[id.Name]; isType {
					return c.typeFunc(node, fn, id.Name, args, sc, x)
				}
			}
		}
		recv := c.expr(fn.X, sc, x, nil)
		r := prune(recv)
		if o, ok := r.(*Opt); ok && fn.Optional {
			return optional2(c.method(node, fn, o.Elem, args, sc, x))
		}
		if _, ok := r.(*Opt); ok || fn.Optional {
			t := c.member(fn, recv, fn.X, fn.Name, fn.Optional)
			return c.callValue(node, "`"+fn.Name.Name+"`", t, args, sc, x)
		}
		return c.method(node, fn, recv, args, sc, x)
	}
	t := c.expr(callee, sc, x, nil)
	return c.callValue(node, "this function", t, args, sc, x)
}

// checkArgsLoosely checks arguments whose call is already wrong, so that
// mistakes inside them are still reported.
func (c *checker) checkArgsLoosely(args []ast.Expr, sc *scope, x *ctx) {
	for _, a := range args {
		c.expr(a, sc, x, nil)
	}
}

// method checks `recv.name(args)`.
func (c *checker) method(node ast.Node, fn *ast.FieldExpr, recv Type, args []ast.Expr, sc *scope, x *ctx) Type {
	if t, handled := c.builtinMethod(node, fn, recv, args, sc, x); handled {
		return t
	}
	if n, ok := prune(recv).(*Named); ok {
		if s, ok := c.methods[n.Name][fn.Name.Name]; ok && s.self {
			return c.callSig(node, s, args, sc, x)
		}
	}
	t := c.member(fn, recv, fn.X, fn.Name, false)
	return c.callValue(node, "`"+fn.Name.Name+"`", t, args, sc, x)
}

// typeFunc checks `Type.name(args)`, a function declared without self.
func (c *checker) typeFunc(node ast.Node, fn *ast.FieldExpr, typeName string, args []ast.Expr, sc *scope, x *ctx) Type {
	s, ok := c.methods[typeName][fn.Name.Name]
	switch {
	case !ok:
		c.checkArgsLoosely(args, sc, x)
		c.errorf(fn.Name, fmt.Sprintf("declare it with `fn %s.%s(...)`", typeName, fn.Name.Name), "`%s` has no function `%s`", typeName, fn.Name.Name)
		return bad
	case s.self:
		c.checkArgsLoosely(args, sc, x)
		c.errorf(fn, fmt.Sprintf("call it on a value, like `x.%s(...)`", fn.Name.Name), "`%s.%s` is a method and needs a value to work on", typeName, fn.Name.Name)
		return bad
	}
	return c.callSig(node, s, args, sc, x)
}

// callValue calls a value of function type, such as a lambda bound by let.
func (c *checker) callValue(node ast.Node, what string, t Type, args []ast.Expr, sc *scope, x *ctx) Type {
	switch f := prune(t).(type) {
	case *Bad:
		c.checkArgsLoosely(args, sc, x)
		return bad
	case *Var:
		params := make([]Type, len(args))
		for i, a := range args {
			params[i] = c.expr(a, sc, x, nil)
		}
		result := c.fresh()
		unify(f, &Func{params, result})
		return result
	case *Func:
		if len(args) != len(f.Params) {
			c.checkArgsLoosely(args, sc, x)
			c.errorf(node, "", "%s takes %d %s, but was given %d", what, len(f.Params), plural2(len(f.Params), "argument"), len(args))
			return bad
		}
		for i, a := range args {
			got := c.expr(a, sc, x, f.Params[i])
			if as := c.assign(f.Params[i], got); !as.ok {
				c.mismatch(a, f.Params[i], got, as, what+" needs "+article(f.Params[i]), "pass "+article(f.Params[i]))
			}
		}
		return f.Result
	}
	c.checkArgsLoosely(args, sc, x)
	c.errorf(node, "", "this is %s, not a function, so it cannot be called", article(t))
	return bad
}

// callSig calls a declared function or method, inferring its type
// parameters from the arguments.
func (c *checker) callSig(node ast.Node, s *sig, args []ast.Expr, sc *scope, x *ctx) Type {
	inst := map[*Param]Type{}
	for _, tp := range s.tparams {
		inst[tp] = c.fresh()
	}
	if targs := typeArgsOf(node); len(targs) > 0 {
		if len(targs) != len(s.tparams) {
			var names []string
			for _, tp := range s.tparams {
				names = append(names, tp.Name)
			}
			help := "remove the `[...]`"
			if len(names) > 0 {
				help = fmt.Sprintf("give one type for each of %s", strings.Join(names, ", "))
			}
			c.errorf(node, help, "`%s` takes %d type %s, but was given %d", s.name, len(s.tparams), plural2(len(s.tparams), "argument"), len(targs))
		} else {
			for i, tp := range s.tparams {
				inst[tp] = c.typeOf(targs[i], nil)
			}
		}
	}
	if len(args) != len(s.params) {
		c.checkArgsLoosely(args, sc, x)
		var names []string
		for _, p := range s.params {
			names = append(names, p.Name)
		}
		c.errorf(node, fmt.Sprintf("write `%s(%s)`", s.name, strings.Join(names, ", ")),
			"`%s` takes %d %s, but was given %d", s.name, len(s.params), plural2(len(s.params), "argument"), len(args))
		return subst(s.result, inst)
	}
	for i, a := range args {
		p := s.params[i]
		pt := subst(p.Type, inst)
		got := c.expr(a, sc, x, pt)
		if as := c.assign(pt, got); !as.ok {
			c.mismatch(a, pt, got, as, fmt.Sprintf("`%s` needs %s for `%s`", s.name, article(pt), p.Name), "pass "+article(pt))
		}
	}
	for _, tp := range s.tparams {
		if tp.Constraint == nil {
			continue
		}
		var at ast.Node = node
		for i, p := range s.params {
			if mentions(p.Type, tp) {
				at = args[i]
				break
			}
		}
		k := constraintCheck{t: inst[tp], constraint: tp.Constraint, fn: s.name, tparam: tp.Name, at: at}
		if _, unknown := prune(k.t).(*Var); unknown {
			c.pending = append(c.pending, k)
		} else {
			c.checkConstraint(k)
		}
	}
	return subst(s.result, inst)
}

func mentions(t Type, p *Param) bool {
	switch t := prune(t).(type) {
	case *Param:
		return t == p
	case *List:
		return mentions(t.Elem, p)
	case *Opt:
		return mentions(t.Elem, p)
	case *Fail:
		return mentions(t.Elem, p)
	}
	return false
}

// subst replaces type parameters with the types chosen for them.
func subst(t Type, m map[*Param]Type) Type {
	if len(m) == 0 {
		return t
	}
	switch t := prune(t).(type) {
	case *Param:
		if r, ok := m[t]; ok {
			return r
		}
		return t
	case *List:
		return &List{subst(t.Elem, m)}
	case *Opt:
		return &Opt{subst(t.Elem, m)}
	case *Fail:
		return &Fail{subst(t.Elem, m)}
	case *Func:
		ps := make([]Type, len(t.Params))
		for i, p := range t.Params {
			ps[i] = subst(p, m)
		}
		return &Func{ps, subst(t.Result, m)}
	case *Tuple:
		es := make([]Type, len(t.Elems))
		for i, e := range t.Elems {
			es[i] = subst(e, m)
		}
		return &Tuple{es}
	}
	return t
}

// construct checks `Circle(1.0)`.
func (c *checker) construct(node ast.Node, v *Variant, args []ast.Expr, sc *scope, x *ctx) Type {
	if len(args) != len(v.Fields) {
		c.checkArgsLoosely(args, sc, x)
		c.errorf(node, fmt.Sprintf("write `%s(%s)`", v.Name, fieldNames(v.Fields)),
			"`%s` takes %d %s, but was given %d", v.Name, len(v.Fields), plural2(len(v.Fields), "value"), len(args))
		return v.Sum
	}
	for i, a := range args {
		f := v.Fields[i]
		got := c.expr(a, sc, x, f.Type)
		if as := c.assign(f.Type, got); !as.ok {
			c.mismatch(a, f.Type, got, as, fmt.Sprintf("`%s` needs %s for `%s`", v.Name, article(f.Type), f.Name), "pass "+article(f.Type))
		}
	}
	return v.Sum
}

// ---- built-ins ----

func (c *checker) builtin(name string, node ast.Node, args []ast.Expr, sc *scope, x *ctx, want Type) Type {
	arity := map[string]int{
		"print": 1, "log.warn": 1, "len": 1, "last": 1, "sum": 1, "fs.read": 1, "json.parse": 1,
		"sort": 1, "env": 1, "int.parse": 1,
		"map": 2, "each": 2, "filter": 2, "sort_by": 2, "take": 2, "map_par": 2, "reduce": 2,
	}
	if name == "first" {
		arity["first"] = 1
		if len(args) == 2 {
			arity["first"] = 2 // first(xs, pred): the first element that passes
		}
	}
	n, known := arity[name]
	if !known {
		c.checkArgsLoosely(args, sc, x)
		mod, member, _ := strings.Cut(name, ".")
		c.errorf(node, fmt.Sprintf("`%s` has `%s`", mod, map[string]string{"fs": "read", "json": "parse", "log": "warn"}[mod]), "`%s` has no function `%s`", mod, member)
		return bad
	}
	if len(args) != n {
		c.checkArgsLoosely(args, sc, x)
		c.errorf(node, "", "`%s` takes %d %s, but was given %d", name, n, plural2(n, "argument"), len(args))
		return bad
	}
	strArg := func(i int) {
		got := c.expr(args[i], sc, x, Str)
		if a := c.assign(Str, got); !a.ok {
			c.mismatch(args[i], Str, got, a, "`"+name+"` needs a Str", "pass a Str")
		}
	}
	switch name {
	case "log.warn":
		strArg(0)
		return Unit
	case "print":
		t := c.expr(args[0], sc, x, nil)
		if f, ok := prune(t).(*Fail); ok {
			c.mismatch(args[0], f.Elem, t, assignment{}, "`print` needs a value to show", "")
			return Unit
		}
		c.interpolated(args[0], t) // print shows anything a string can hold
		return Unit
	case "env":
		strArg(0)
		return &Opt{Str}
	case "int.parse":
		strArg(0)
		return &Fail{Int}
	case "sort":
		elem := c.listArg(name, args[0], sc, x)
		if _, unknown := prune(elem).(*Var); !unknown && !ordered(elem) {
			c.errorf(args[0], "sort by a key instead: `sort_by(x -> ...)`", "`sort` needs Ints, Floats or Strs, but this list holds %s", plural(elem))
		}
		return &List{elem}
	case "reduce":
		elem := c.listArg(name, args[0], sc, x)
		ft := c.expr(args[1], sc, x, &Func{[]Type{elem, elem}, elem})
		if f, ok := prune(ft).(*Func); ok && len(f.Params) == 2 {
			if a := c.assign(elem, f.Result); !a.ok {
				c.errorf(args[1], "", "`reduce` combines two %s into one, but this function gives %s", plural(elem), article(f.Result))
			}
		} else if !isBad(ft) {
			c.errorf(args[1], "give it a function of two values, like `(a, b) -> a + b`", "`reduce` needs a function of two values")
		}
		return optional2(elem)
	case "fs.read":
		strArg(0)
		return &Fail{Str}
	case "json.parse":
		strArg(0)
		if targs := typeArgsOf(node); len(targs) == 1 {
			want = c.typeOf(targs[0], nil)
		} else if len(targs) > 1 {
			c.errorf(node, "give one type: `json.parse[Config](text)`", "`json.parse` takes one type argument")
		}
		return c.jsonParse(node, want)
	case "len":
		t := c.expr(args[0], sc, x, nil)
		switch p := prune(t).(type) {
		case *List, *Bad:
			return Int
		case *Basic:
			if p == Str {
				return Int
			}
		case *Var:
			unify(p, &List{c.fresh()})
			return Int
		case *Fail:
			c.mismatch(args[0], p.Elem, t, assignment{}, "`len` needs a List or a Str", "")
			return Int
		case *Opt:
			c.mismatch(args[0], p.Elem, t, assignment{}, "`len` needs a List or a Str", "")
			return Int
		}
		c.mismatch(args[0], Str, t, assignment{}, "`len` needs a List or a Str", "")
		return Int
	case "first", "last":
		elem := c.listArg(name, args[0], sc, x)
		if len(args) == 2 {
			ft := c.expr(args[1], sc, x, &Func{[]Type{elem}, Bool})
			if f, ok := prune(ft).(*Func); ok && len(f.Params) == 1 {
				if a := c.assign(Bool, f.Result); !a.ok {
					c.errorf(args[1], "give it a function that returns a Bool, like `x -> x > 0`", "`first`'s function must return a Bool, but this one returns %s", article(f.Result))
				}
			}
		}
		return optional2(elem)
	case "sum":
		elem := c.listArg(name, args[0], sc, x)
		switch p := prune(elem).(type) {
		case *Bad:
			return bad
		case *Var:
			unify(p, Int)
			return Int
		}
		if prune(elem) == Int || prune(elem) == Float {
			return elem
		}
		c.errorf(args[0], "", "`sum` adds up Ints or Floats, but this list holds %s", plural(elem))
		return bad
	case "take":
		elem := c.listArg(name, args[0], sc, x)
		got := c.expr(args[1], sc, x, Int)
		if a := c.assign(Int, got); !a.ok {
			c.mismatch(args[1], Int, got, a, "`take` needs an Int", "write a count, like `take(10)`")
		}
		return &List{elem}
	}

	// map, each, filter, sort_by, map_par: a list and a function of its elements.
	if name == "map_par" && !x.insideTogether() {
		c.errorf(node, "wrap it: `together { xs |> map_par(f) }!`", "map_par spawns tasks, so it must run inside a `together` block")
	}
	elem := c.listArg(name, args[0], sc, x)
	ft := c.expr(args[1], sc, x, &Func{[]Type{elem}, c.fresh()})
	var result Type
	switch f := prune(ft).(type) {
	case *Bad:
		return bad
	case *Func:
		if len(f.Params) != 1 {
			c.errorf(args[1], "give it a function of one value, like `x -> ...`", "`%s` calls its function with one value, but this one takes %d", name, len(f.Params))
			return bad
		}
		if a := c.assign(f.Params[0], elem); !a.ok {
			c.errorf(args[1], "", "`%s` passes %s to its function, but this one takes %s", name, article(elem), article(f.Params[0]))
			return bad
		}
		result = f.Result
	default:
		c.errorf(args[1], "give it a function, like `x -> ...`", "`%s` needs a function, but this is %s", name, article(ft))
		return bad
	}
	failing := false
	if fr, ok := prune(result).(*Fail); ok {
		failing, result = true, fr.Elem
	}
	var out Type
	switch name {
	case "map", "map_par":
		out = &List{result}
	case "each":
		out = Unit
	case "filter":
		if a := c.assign(Bool, result); !a.ok {
			c.errorf(args[1], "give filter a function that returns a Bool, like `x -> x > 0`", "filter's function must return a Bool, but this one returns %s", article(result))
		}
		out = &List{elem}
	case "sort_by":
		if _, unknown := prune(result).(*Var); !unknown && !ordered(result) {
			c.errorf(args[1], "return an Int, a Float or a Str from the key function", "sort_by needs a key that can be ordered, but this one is %s", article(result))
		}
		out = &List{elem}
	}
	if failing {
		return failable(out)
	}
	return out
}

// listArg checks a built-in's list argument and returns its element type.
func (c *checker) listArg(name string, a ast.Expr, sc *scope, x *ctx) Type {
	t := c.expr(a, sc, x, nil)
	switch l := prune(t).(type) {
	case *List:
		return l.Elem
	case *Bad:
		return bad
	case *Var:
		elem := c.fresh()
		unify(l, &List{elem})
		return elem
	}
	if f, ok := prune(t).(*Fail); ok {
		if _, isList := prune(f.Elem).(*List); isList {
			c.mismatch(a, f.Elem, t, assignment{}, "`"+name+"` needs a List", "")
			return bad
		}
	}
	c.errorf(a, "", "`%s` needs a List, but this is %s", name, article(t))
	return bad
}

// jsonParse decides what `json.parse` builds from the type expected of it.
func (c *checker) jsonParse(node ast.Node, want Type) Type {
	target := Type(nil)
	if want != nil {
		target = success(want)
	}
	if target == nil || unknownType(target) {
		c.errorf(node, "parse inside a function whose result type says it, like `fn load(text: Str) -> Config! { json.parse(text)! }`",
			"cannot tell what type `json.parse` should produce here")
		return bad
	}
	if why := jsonable(target); why != "" {
		c.errorf(node, "parse into a record, a List, a Str, an Int, a Float or a Bool", "`json.parse` cannot produce %s: %s", article(target), why)
		return bad
	}
	c.info.JSONTargets[node] = target
	return &Fail{target}
}

func unknownType(t Type) bool {
	switch t := prune(t).(type) {
	case *Var, *Bad:
		return true
	case *List:
		return unknownType(t.Elem)
	case *Opt:
		return unknownType(t.Elem)
	}
	return false
}

// jsonable says why JSON cannot be parsed into t, or "" if it can.
func jsonable(t Type) string {
	switch t := prune(t).(type) {
	case *Basic:
		if t == Unit || t == Error {
			return "it has no JSON form"
		}
		return ""
	case *List:
		return jsonable(t.Elem)
	case *Opt:
		return jsonable(t.Elem)
	case *Named:
		if t.IsSum() {
			return "sum types have no JSON form"
		}
		for _, f := range t.Fields {
			if why := jsonable(f.Type); why != "" {
				return fmt.Sprintf("field `%s`: %s", f.Name, why)
			}
		}
		return ""
	}
	return "it has no JSON form"
}

// typeArgsOf returns the explicit type arguments of a call, `f[T](...)`.
func typeArgsOf(node ast.Node) []ast.TypeExpr {
	if call, ok := node.(*ast.CallExpr); ok {
		return call.TypeArgs
	}
	return nil
}

// builtinMethod checks the methods built-in types have: `.len()` on a List
// or a Str, and `.push(x)` on a List.
func (c *checker) builtinMethod(node ast.Node, fn *ast.FieldExpr, recv Type, args []ast.Expr, sc *scope, x *ctx) (Type, bool) {
	r := prune(recv)
	l, isList := r.(*List)
	isStr := r == Str
	if !isList && !isStr {
		return nil, false
	}
	want := func(n int) bool {
		if len(args) != n {
			c.checkArgsLoosely(args, sc, x)
			c.errorf(node, "", "`.%s` takes %d %s, but was given %d", fn.Name.Name, n, plural2(n, "argument"), len(args))
			return false
		}
		return true
	}
	switch {
	case fn.Name.Name == "len":
		want(0)
		return Int, true
	case fn.Name.Name == "push" && isList:
		if !want(1) {
			return Unit, true
		}
		got := c.expr(args[0], sc, x, l.Elem)
		if a := c.assign(l.Elem, got); !a.ok {
			c.mismatch(args[0], l.Elem, got, a, "this list holds "+plural(l.Elem)+", so `.push` needs "+article(l.Elem), "push "+article(l.Elem))
		}
		if _, isIdent := fn.X.(*ast.Ident); !isIdent {
			c.errorf(fn, "push onto a `var` that holds the list", "`.push` changes a list in place, so it needs a `var`, not %s", "this expression")
		}
		return Unit, true
	}
	return nil, false
}
