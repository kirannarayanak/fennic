package types

import (
	"fmt"
	"strings"
	"unicode/utf8"

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
		if name == "Chan" {
			return c.newChan(node, args, sc, x, want)
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
			c.errorf(node, fmt.Sprintf("%s is `%s`; give it %s", what, String(f), plural2(len(f.Params), "argument")),
				"%s takes %d %s, but was given %d", what, len(f.Params), plural2(len(f.Params), "argument"), len(args))
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
	help := "only functions and lambdas can be called; remove the `(...)`"
	switch prune(t).(type) {
	case *List, *Map:
		help = "to read from it, use brackets: `xs[i]` or `m[key]`"
	}
	c.errorf(node, help, "this is %s, not a function, so it cannot be called", article(t))
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
	case *Map:
		return &Map{subst(t.Key, m), subst(t.Val, m)}
	case *Chan:
		return &Chan{subst(t.Elem, m)}
	case *Entry:
		return &Entry{subst(t.Key, m), subst(t.Val, m), t.ValName}
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
		"sort": 1, "env": 1, "int.parse": 1, "lower": 1, "words": 1, "tally": 1, "csv.read": 1,
		"top": 2, "group_by": 2, "map_values": 2, "sum_by": 2, "sort_by_value": 2,
		"flatten": 1, "unique": 1, "json.encode": 1, "fs.list": 2, "fs.write": 2,
		"to_float": 1, "to_int": 1,
		"map": 2, "each": 2, "filter": 2, "sort_by": 2, "take": 2, "map_par": 2, "reduce": 2,
		"split": 2, "join": 2, "contains": 2, "starts_with": 2, "trim": 1,
		"pad_start": 2, "pad_end": 2,
	}
	if (name == "pad_start" || name == "pad_end") && len(args) == 3 {
		arity[name] = 3 // with a fill character
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
		c.errorf(node, builtinUsage(name, n), "`%s` takes %d %s, but was given %d", name, n, plural2(n, "argument"), len(args))
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
			c.mismatch(args[0], f.Elem, t, assignment{}, "`print` needs a value to show", "handle the failure first, with `!` or `else`")
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
	case "split", "contains", "starts_with":
		strArg(0)
		got := c.expr(args[1], sc, x, Str)
		if a := c.assign(Str, got); !a.ok {
			c.mismatch(args[1], Str, got, a, "`"+name+"` needs a Str", "pass a Str")
		}
		if name == "split" {
			return &List{Str}
		}
		return Bool
	case "join":
		elem := c.listArg(name, args[0], sc, x)
		if a := c.assign(Str, elem); !a.ok && !isBad(elem) {
			c.errorf(args[0], "turn each element into a Str first, like `xs |> map(x -> \"{x}\") |> join(\", \")`",
				"`join` joins Strs, but this list holds %s", plural(elem))
		}
		got := c.expr(args[1], sc, x, Str)
		if a := c.assign(Str, got); !a.ok {
			c.mismatch(args[1], Str, got, a, "`join` needs a Str separator", "pass a Str")
		}
		return Str
	case "trim":
		strArg(0)
		return Str
	case "pad_start", "pad_end":
		strArg(0)
		got := c.expr(args[1], sc, x, Int)
		if a := c.assign(Int, got); !a.ok {
			c.mismatch(args[1], Int, got, a, "`"+name+"` needs an Int width", "pass the width in characters, like `"+name+"(s, 10)`")
		}
		if len(args) == 3 {
			fill := c.expr(args[2], sc, x, Str)
			if a := c.assign(Str, fill); !a.ok {
				c.mismatch(args[2], Str, fill, a, "`"+name+"` needs a one-character Str to fill with", "pass a Str, like `\"0\"`")
			} else if lit, ok := args[2].(*ast.StrLit); ok && utf8.RuneCountInString(lit.Value) != 1 {
				c.errorf(args[2], "fill with exactly one character, like `\" \"` or `\"0\"`", "`%s` fills with one character, but this is %d", name, utf8.RuneCountInString(lit.Value))
			}
		}
		return Str
	case "to_float", "to_int":
		from, to := Int, Float
		if name == "to_int" {
			from, to = Float, Int
		}
		got := c.expr(args[0], sc, x, from)
		if a := c.assign(from, got); !a.ok {
			c.mismatch(args[0], from, got, a, "`"+name+"` needs "+article(from), "pass "+article(from))
		}
		if name == "to_int" {
			return &Fail{Int} // NaN, infinity and Floats beyond the Int range have no Int
		}
		return to
	case "flatten":
		elem := c.listArg(name, args[0], sc, x)
		switch inner := prune(elem).(type) {
		case *List:
			return inner
		case *Var:
			e := c.fresh()
			unify(inner, &List{e})
			return &List{e}
		case *Bad:
			return bad
		}
		c.errorf(args[0], "this list is already flat; remove the `flatten`", "`flatten` needs a list of lists, but this list holds %s", plural(elem))
		return bad
	case "unique":
		return &List{c.listArg(name, args[0], sc, x)}
	case "json.encode":
		t := c.expr(args[0], sc, x, nil)
		if why := jsonable(t); why != "" && !isBad(t) && !unknownType(t) {
			c.errorf(args[0], "encode records, lists, maps with Str keys, and plain values", "`json.encode` cannot encode %s: %s", article(t), why)
		}
		return Str
	case "fs.list":
		strArg(0)
		got := c.expr(args[1], sc, x, Str)
		if a := c.assign(Str, got); !a.ok {
			c.mismatch(args[1], Str, got, a, "`fs.list` needs a Str pattern", "pass a pattern, like `\"*.csv\"`")
		}
		return &Fail{&List{Str}}
	case "fs.write":
		strArg(0)
		got := c.expr(args[1], sc, x, Str)
		if a := c.assign(Str, got); !a.ok {
			c.mismatch(args[1], Str, got, a, "`fs.write` needs a Str to write", "pass a Str")
		}
		return &Fail{Unit}
	case "lower":
		strArg(0)
		return Str
	case "words":
		strArg(0)
		return &List{Str}
	case "tally":
		elem := c.listArg(name, args[0], sc, x)
		if !keyType(elem) {
			c.errorf(args[0], "tally a list of Strs, Ints or Bools, such as a key taken from each element with `map`", "`tally` counts Strs, Ints or Bools, but this list holds %s", plural(elem))
		}
		return &Map{elem, Int}
	case "top":
		m := c.mapArg(name, args[0], sc, x)
		if m != nil {
			if a := c.assign(Int, m.Val); !a.ok {
				c.errorf(args[0], "count with `tally` first, then take the top", "`top` needs a Map of counts, but this map's values are %s", plural(m.Val))
			}
		}
		got := c.expr(args[1], sc, x, Int)
		if a := c.assign(Int, got); !a.ok {
			c.mismatch(args[1], Int, got, a, "`top` needs an Int", "write how many, like `top(5)`")
		}
		if m == nil {
			return bad
		}
		return &List{&Entry{m.Key, Int, "count"}}
	case "sort_by_value":
		m := c.mapArg(name, args[0], sc, x)
		got := c.expr(args[1], sc, x, Order)
		if a := c.assign(Order, got); !a.ok {
			c.mismatch(args[1], Order, got, a, "`sort_by_value` needs an order", "write `asc` or `desc`")
		}
		if m == nil {
			return bad
		}
		if _, unknown := prune(m.Val).(*Var); !unknown && !ordered(m.Val) {
			key := "..."
			switch v := prune(m.Val).(type) {
			case *List, *Map:
				key = "v.len()"
			case *Named:
				for _, f := range v.Fields {
					if ordered(f.Type) {
						key = "v." + f.Name
						break
					}
				}
			}
			c.errorf(args[0], fmt.Sprintf("turn each value into an Int, Float or Str with `map_values` first, for example `%s |> map_values(v -> %s) |> sort_by_value(%s)`", c.text(args[0]), key, c.text(args[1])),
				"`sort_by_value` needs values that can be ordered, but these are %s", plural(m.Val))
		}
		return &List{&Entry{m.Key, m.Val, "value"}}
	case "map_values":
		m := c.mapArg(name, args[0], sc, x)
		if m == nil {
			c.expr(args[1], sc, x, nil)
			return bad
		}
		ft := c.expr(args[1], sc, x, &Func{[]Type{m.Val}, c.fresh()})
		f, ok := prune(ft).(*Func)
		if !ok || len(f.Params) != 1 {
			if !isBad(ft) {
				c.errorf(args[1], "give it a function of one value, like `v -> ...`", "`map_values` needs a function of each value")
			}
			return bad
		}
		if fr, failing := prune(f.Result).(*Fail); failing {
			return &Fail{&Map{m.Key, fr.Elem}}
		}
		return &Map{m.Key, f.Result}
	case "csv.read":
		strArg(0)
		return c.csvRead(node, want)
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
				c.errorf(args[1], fmt.Sprintf("make the function give %s; to combine into another type, `map` the list to it first", article(elem)),
					"`reduce` combines two %s into one, but this function gives %s", plural(elem), article(f.Result))
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
			c.mismatch(args[0], p.Elem, t, assignment{}, "`len` needs a List or a Str", "handle the failure first, with `!` or `else`")
			return Int
		case *Opt:
			c.mismatch(args[0], p.Elem, t, assignment{}, "`len` needs a List or a Str", "give a default first, with `??`")
			return Int
		}
		help := "`len` counts a List's items or a Str's characters"
		if prune(t) == Int {
			help += fmt.Sprintf(`; to count an Int's digits, write `+"`len(\"{%s}\")`", c.text(args[0]))
		}
		c.mismatch(args[0], Str, t, assignment{}, "`len` needs a List or a Str", help)
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
		if prune(elem) == Str {
			c.errorf(args[0], fmt.Sprintf("to put Strs together, use `join(%s, \"\")`", c.text(args[0])), "`sum` adds up Ints or Floats, but this list holds %s", plural(elem))
			return bad
		}
		pick := "..."
		if n, ok := prune(elem).(*Named); ok {
			for _, f := range n.Fields {
				if t := prune(f.Type); t == Int || t == Float {
					pick = "x." + f.Name
					break
				}
			}
		}
		c.errorf(args[0], fmt.Sprintf("pick the number from each element with `sum_by`: `%s |> sum_by(x -> %s)`", c.text(args[0]), pick),
			"`sum` adds up Ints or Floats, but this list holds %s", plural(elem))
		return bad
	case "take":
		elem := c.listArg(name, args[0], sc, x)
		got := c.expr(args[1], sc, x, Int)
		if a := c.assign(Int, got); !a.ok {
			c.mismatch(args[1], Int, got, a, "`take` needs an Int", "write a count, like `take(10)`")
		}
		return &List{elem}
	}

	// map, each, filter, sort_by, map_par, group_by, sum_by: a list and a
	// function of its elements.
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
			c.errorf(args[1], fmt.Sprintf("make the function take %s, or leave out its parameter's type so it is inferred", article(elem)),
				"`%s` passes %s to its function, but this one takes %s", name, article(elem), article(f.Params[0]))
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
	case "group_by":
		if !keyType(result) {
			c.errorf(args[1], "group by a Str, an Int or a Bool, such as one field", "`group_by` needs a key that is a Str, an Int or a Bool, but this one is %s", article(result))
		}
		out = &Map{result, &List{elem}}
	case "sum_by":
		switch {
		case prune(result) == Int || prune(result) == Float || isBad(result):
		case func() bool { _, v := prune(result).(*Var); return v }():
			unify(result, Int)
		default:
			c.errorf(args[1], "return an Int or a Float from the function", "`sum_by` adds up Ints or Floats, but this function gives %s", article(result))
		}
		out = result
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
			c.mismatch(a, f.Elem, t, assignment{}, "`"+name+"` needs a List", "handle the failure first, with `!` or `else`")
			return bad
		}
	}
	help := "pass a List, like `[1, 2]`"
	switch o := prune(t).(type) {
	case *Map:
		help = "a Map's entries come as a List from `sort_by_value(asc)`"
	case *Opt:
		if _, isList := prune(o.Elem).(*List); isList {
			help = fmt.Sprintf("give a default first: `(%s ?? [])`", c.text(a))
		}
	case *Basic:
		if o == Str {
			help = fmt.Sprintf("`split` a Str into a List first: `split(%s, \" \")`", c.text(a))
		}
	}
	c.errorf(a, help, "`%s` needs a List, but this is %s", name, article(t))
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
		c.errorf(node, "parse into a record, a List, a Map with Str keys, a Str, an Int, a Float, a Bool or a Date", "`json.parse` cannot produce %s: %s", article(target), why)
		return bad
	}
	c.info.Targets[node] = target
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
		if t == Unit || t == Error || t == Order {
			return "it has no JSON form"
		}
		return ""
	case *List:
		return jsonable(t.Elem)
	case *Opt:
		return jsonable(t.Elem)
	case *Map:
		if prune(t.Key) != Str && !unknownType(t.Key) {
			return "only maps with Str keys have a JSON form"
		}
		return jsonable(t.Val)
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
	if ch, isChan := r.(*Chan); isChan {
		return c.chanMethod(node, fn, ch, args, sc, x), true
	}
	l, isList := r.(*List)
	_, isMap := r.(*Map)
	isStr := r == Str
	if !isList && !isStr && !isMap {
		return nil, false
	}
	want := func(n int) bool {
		if len(args) != n {
			c.checkArgsLoosely(args, sc, x)
			usage := fmt.Sprintf("write `.%s()`", fn.Name.Name)
			if n == 1 {
				usage = fmt.Sprintf("write `.%s(x)`", fn.Name.Name)
			}
			c.errorf(node, usage, "`.%s` takes %d %s, but was given %d", fn.Name.Name, n, plural2(n, "argument"), len(args))
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

// mapArg checks a built-in's map argument. It returns nil after reporting
// a value that is not a map.
func (c *checker) mapArg(name string, a ast.Expr, sc *scope, x *ctx) *Map {
	t := c.expr(a, sc, x, nil)
	switch m := prune(t).(type) {
	case *Map:
		return m
	case *Var:
		mt := &Map{c.fresh(), c.fresh()}
		unify(m, mt)
		return mt
	case *Bad:
		return nil
	}
	if f, ok := prune(t).(*Fail); ok {
		if _, isMap := prune(f.Elem).(*Map); isMap {
			c.mismatch(a, f.Elem, t, assignment{}, "`"+name+"` needs a Map", "handle the failure first, with `!` or `else`")
			return nil
		}
	}
	help := "pass a Map, like `Map{\"a\": 1}`"
	if _, isList := prune(t).(*List); isList {
		help = fmt.Sprintf("make a Map from a List with `group_by`: `%s |> group_by(x -> ...)`", c.text(a))
	}
	c.errorf(a, help, "`%s` needs a Map, but this is %s", name, article(t))
	return nil
}

// csvRead decides what record type `csv.read` builds: from its type
// argument, or from the List type expected of it.
func (c *checker) csvRead(node ast.Node, want Type) Type {
	var rec Type
	if targs := typeArgsOf(node); len(targs) == 1 {
		rec = c.typeOf(targs[0], nil)
	} else if l, ok := prune(success(want)).(*List); ok {
		rec = l.Elem
	}
	if rec == nil || unknownType(rec) {
		c.errorf(node, "name the record type each row becomes: `csv.read[Sale](path)`", "cannot tell what type `csv.read` should produce here")
		return bad
	}
	n, ok := prune(rec).(*Named)
	if !ok || n.IsSum() {
		c.errorf(node, "read into a record type, one field per column", "`csv.read` reads rows into a record, but this is %s", article(rec))
		return bad
	}
	for _, f := range n.Fields {
		ft := prune(f.Type)
		if o, opt := ft.(*Opt); opt {
			ft = prune(o.Elem)
		}
		if ft != Int && ft != Float && ft != Str && ft != Bool && ft != Date {
			c.errorf(node, "give each column an Int, a Float, a Str, a Bool or a Date field, optional if it may be empty",
				"`csv.read` cannot fill field `%s` of `%s`: a CSV cell cannot become %s", f.Name, n.Name, article(f.Type))
			return bad
		}
	}
	c.info.Targets[node] = rec
	return &Fail{&List{rec}}
}

// newChan checks `Chan[T](n)`: a channel of T with room for n values.
func (c *checker) newChan(node ast.Node, args []ast.Expr, sc *scope, x *ctx, want Type) Type {
	var elem Type
	if targs := typeArgsOf(node); len(targs) == 1 {
		elem = c.typeOf(targs[0], nil)
	} else if ch, ok := prune(want).(*Chan); ok {
		elem = ch.Elem
	}
	if len(args) != 1 {
		c.checkArgsLoosely(args, sc, x)
		c.errorf(node, "give how many values it can hold: `Chan[Str](100)`", "`Chan` takes 1 argument, but was given %d", len(args))
		return bad
	}
	got := c.expr(args[0], sc, x, Int)
	if a := c.assign(Int, got); !a.ok {
		c.mismatch(args[0], Int, got, a, "`Chan` needs an Int, how many values it can hold", "pass an Int, like `Chan[Str](100)`")
	}
	if elem == nil {
		c.errorf(node, "name what it carries: `Chan[Str](100)`", "cannot tell what type of value this channel carries")
		return bad
	}
	return &Chan{elem}
}

// chanMethod checks `.send(x)` and `.close()` on a channel.
func (c *checker) chanMethod(node ast.Node, fn *ast.FieldExpr, ch *Chan, args []ast.Expr, sc *scope, x *ctx) Type {
	switch fn.Name.Name {
	case "send":
		if len(args) != 1 {
			c.checkArgsLoosely(args, sc, x)
			c.errorf(node, "send one value: `ch.send(x)`", "`.send` takes 1 argument, but was given %d", len(args))
			return Unit
		}
		got := c.expr(args[0], sc, x, ch.Elem)
		if a := c.assign(ch.Elem, got); !a.ok {
			c.mismatch(args[0], ch.Elem, got, a, "this channel carries "+plural(ch.Elem)+", so `.send` needs "+article(ch.Elem), "send "+article(ch.Elem))
		}
		return Unit
	case "close":
		if len(args) != 0 {
			c.checkArgsLoosely(args, sc, x)
			c.errorf(node, "write `.close()`", "`.close` takes no arguments, but was given %d", len(args))
		}
		return Unit
	}
	c.checkArgsLoosely(args, sc, x)
	c.errorf(fn.Name, "a channel has `.send(x)` and `.close()`; read it with `for x in ch`", "a channel has no method `%s`", fn.Name.Name)
	return bad
}

// builtinSigs are the built-ins' signatures as docs/types.md writes them,
// for the help when a call has the wrong number of arguments.
var builtinSigs = map[string][]string{
	"print":         {"print(T)"},
	"log.warn":      {"log.warn(Str)"},
	"len":           {"len(List[T]) -> Int", "len(Str) -> Int"},
	"first":         {"first(List[T]) -> T?", "first(List[T], (T) -> Bool) -> T?"},
	"last":          {"last(List[T]) -> T?"},
	"sum":           {"sum(List[Int]) -> Int", "sum(List[Float]) -> Float"},
	"map":           {"map(List[T], (T) -> U) -> List[U]"},
	"each":          {"each(List[T], (T) -> U)"},
	"filter":        {"filter(List[T], (T) -> Bool) -> List[T]"},
	"sort_by":       {"sort_by(List[T], (T) -> K) -> List[T]"},
	"take":          {"take(List[T], Int) -> List[T]"},
	"map_par":       {"map_par(List[T], (T) -> U) -> List[U]"},
	"sort":          {"sort(List[T]) -> List[T]"},
	"reduce":        {"reduce(List[T], (T, T) -> T) -> T?"},
	"env":           {"env(Str) -> Str?"},
	"lower":         {"lower(Str) -> Str"},
	"words":         {"words(Str) -> List[Str]"},
	"tally":         {"tally(List[T]) -> Map[T, Int]"},
	"top":           {"top(Map[K, Int], Int) -> List[Count[K]]"},
	"group_by":      {"group_by(List[T], (T) -> K) -> Map[K, List[T]]"},
	"map_values":    {"map_values(Map[K, V], (V) -> W) -> Map[K, W]"},
	"sum_by":        {"sum_by(List[T], (T) -> N) -> N"},
	"sort_by_value": {"sort_by_value(Map[K, V], asc | desc) -> List[Entry[K, V]]"},
	"csv.read":      {"csv.read[T](Str) -> List[T]!"},
	"flatten":       {"flatten(List[List[T]]) -> List[T]"},
	"unique":        {"unique(List[T]) -> List[T]"},
	"json.encode":   {"json.encode(T) -> Str"},
	"json.parse":    {"json.parse(Str) -> T!"},
	"fs.list":       {"fs.list(Str, Str) -> List[Str]!"},
	"fs.write":      {"fs.write(Str, Str) -> nothing!"},
	"fs.read":       {"fs.read(Str) -> Str!"},
	"int.parse":     {"int.parse(Str) -> Int!"},
	"split":         {"split(Str, Str) -> List[Str]"},
	"join":          {"join(List[Str], Str) -> Str"},
	"contains":      {"contains(Str, Str) -> Bool"},
	"starts_with":   {"starts_with(Str, Str) -> Bool"},
	"trim":          {"trim(Str) -> Str"},
	"pad_start":     {"pad_start(Str, Int) -> Str", "pad_start(Str, Int, Str) -> Str"},
	"pad_end":       {"pad_end(Str, Int) -> Str", "pad_end(Str, Int, Str) -> Str"},
	"to_float":      {"to_float(Int) -> Float"},
	"to_int":        {"to_int(Float) -> Int!"},
}

// builtinUsage is the help for a call to a built-in with the wrong number
// of arguments: its signature, or every form it has.
func builtinUsage(name string, n int) string {
	forms := builtinSigs[name]
	if len(forms) == 0 {
		return fmt.Sprintf("give it %s", plural2(n, "argument"))
	}
	return "it is written `" + strings.Join(forms, "` or `") + "`"
}
