package types

import (
	"fmt"
	"strings"

	"github.com/kirannarayanak/fennic/ast"
	"github.com/kirannarayanak/fennic/token"
)

// expr checks an expression and returns its type. want, if set, is the type
// the surrounding code expects; it lets `[]`, `none`, lambdas and
// `json.parse` take their types from context. Reporting a value that does
// not fit want is the caller's job.
func (c *checker) expr(e ast.Expr, sc *scope, x *ctx, want Type) Type {
	switch e := e.(type) {
	case *ast.IntLit:
		return Int
	case *ast.FloatLit:
		return Float
	case *ast.StrLit:
		return Str
	case *ast.BoolLit:
		return Bool
	case *ast.NoneLit:
		if o, ok := prune(want).(*Opt); ok {
			return o
		}
		return &Opt{c.fresh()}
	case *ast.InterpStr:
		for _, p := range e.Parts {
			if _, text := p.(*ast.StrLit); text {
				continue
			}
			c.interpolated(p, c.expr(p, sc, x, nil))
		}
		return Str
	case *ast.Ident:
		return c.ident(e, sc)
	case *ast.ListLit:
		return c.list(e, sc, x, want)
	case *ast.RecordLit:
		return c.record(e, sc, x)
	case *ast.WithExpr:
		return c.with(e, sc, x)
	case *ast.FieldExpr:
		return c.field(e, sc, x)
	case *ast.UnaryExpr:
		if e.Op == token.Not {
			t := c.expr(e.X, sc, x, Bool)
			c.needBool(e.X, t, "`not`")
			return Bool
		}
		t := c.expr(e.X, sc, x, nil)
		switch prune(t).(type) {
		case *Bad:
			return bad
		case *Var:
			unify(t, Int)
			return Int
		}
		if prune(t) == Int || prune(t) == Float {
			return t
		}
		c.errorf(e, "", "`-` needs an Int or a Float, but this is %s", article(t))
		return bad
	case *ast.BinaryExpr:
		return c.binary(e, sc, x, want)
	case *ast.CallExpr:
		return c.call(e, e.Fn, e.Args, nil, sc, x, want)
	case *ast.PipeExpr:
		return c.call(e, e.Call.Fn, e.Call.Args, e.X, sc, x, want)
	case *ast.PropagateExpr:
		return c.propagate(e, sc, x, want)
	case *ast.ElseExpr:
		return c.elseExpr(e, sc, x, want)
	case *ast.IfExpr:
		return c.ifExpr(e, sc, x, want)
	case *ast.FailExpr:
		return c.fail(e, sc, x)
	case *ast.IndexExpr:
		return c.index(e, sc, x)
	case *ast.TupleLit:
		var wantElems []Type
		if tt, ok := prune(want).(*Tuple); ok && len(tt.Elems) == len(e.Elems) {
			wantElems = tt.Elems
		}
		elems := make([]Type, len(e.Elems))
		for i, el := range e.Elems {
			var w Type
			if wantElems != nil {
				w = wantElems[i]
			}
			elems[i] = c.expr(el, sc, x, w)
		}
		return &Tuple{elems}
	case *ast.FuncLit:
		return c.funcLit(e, sc, x)
	case *ast.LambdaExpr:
		return c.lambda(e, sc, x, want)
	case *ast.Block:
		return c.block(e, sc, x, want)
	case *ast.MatchExpr:
		return c.match(e, sc, x, want)
	case *ast.TogetherExpr:
		t := c.block(e.Body, sc, &ctx{kind: ctxTogether, parent: x}, success(want))
		return failable(t)
	}
	return bad
}

func (c *checker) interpolated(p ast.Expr, t Type) {
	switch g := prune(t).(type) {
	case *Fail:
		src := c.text(p)
		c.errorf(p, fmt.Sprintf("handle the failure first: `%s else %s` gives a fallback, and `%s!` passes it on", src, fallback(g.Elem), src),
			"%s cannot go in a string until its failure is handled", article(t))
	case *Func:
		c.errorf(p, "call the function and interpolate its result", "a function cannot go in a string")
	case *Basic:
		if g == Unit {
			c.errorf(p, "interpolate a value instead", "this gives nothing, so there is nothing to put in the string")
		}
	}
}

// builtins are the functions every program can call without declaring them.
var builtins = map[string]bool{
	"print": true, "len": true, "first": true, "last": true, "sum": true, "map": true,
	"each": true, "filter": true, "sort_by": true, "take": true, "map_par": true,
	"sort": true, "reduce": true, "env": true,
}

var modules = map[string]bool{"fs": true, "json": true, "log": true, "int": true}

func (c *checker) ident(e *ast.Ident, sc *scope) Type {
	if e.Name == "_" {
		c.errorf(e, "give the value a name to use it", "`_` stands for a value that is not used, so it cannot be read")
		return bad
	}
	if b, ok := sc.lookup(e.Name); ok {
		return b.t
	}
	if v, ok := c.variant[e.Name]; ok && len(v.Fields) == 0 {
		return v.Sum
	}
	if s, ok := c.funcs[e.Name]; ok {
		if len(s.tparams) > 0 {
			c.errorf(e, fmt.Sprintf("call it: `%s(...)`", e.Name), "`%s` is generic, so it can only be called, not passed around", e.Name)
			return bad
		}
		ps := make([]Type, len(s.params))
		for i, p := range s.params {
			ps[i] = p.Type
		}
		return &Func{ps, s.result}
	}
	switch {
	case builtins[e.Name]:
		c.errorf(e, fmt.Sprintf("call it: `%s(...)`", e.Name), "`%s` is built in, so it can only be called, not passed around", e.Name)
	case c.variant[e.Name] != nil:
		v := c.variant[e.Name]
		c.errorf(e, fmt.Sprintf("give its fields: `%s(%s)`", v.Name, fieldNames(v.Fields)), "`%s` needs its fields to make a value", e.Name)
	case c.named[e.Name] != nil || c.ifaces[e.Name] != nil:
		c.errorf(e, "use a value of the type, not the type itself", "`%s` is a type, not a value", e.Name)
	case modules[e.Name]:
		c.errorf(e, fmt.Sprintf("call one of its functions, like `%s.%s(...)`", e.Name, map[string]string{"fs": "read", "json": "parse", "log": "warn"}[e.Name]),
			"`%s` is a module, not a value", e.Name)
	default:
		c.errorf(e, fmt.Sprintf("declare it with `let %s = ...` before this line, or check the spelling", e.Name), "`%s` is not defined", e.Name)
	}
	return bad
}

func fieldNames(fs []Field) string {
	var names []string
	for _, f := range fs {
		names = append(names, f.Name)
	}
	return strings.Join(names, ", ")
}

func (c *checker) list(e *ast.ListLit, sc *scope, x *ctx, want Type) Type {
	var elem Type
	if l, ok := prune(want).(*List); ok {
		elem = l.Elem
	} else {
		elem = c.fresh()
		// `[1, none]` is a list of optionals, whatever comes first.
		for _, el := range e.Elems {
			if _, isNone := el.(*ast.NoneLit); isNone {
				elem = &Opt{elem}
				break
			}
		}
	}
	for _, el := range e.Elems {
		got := c.expr(el, sc, x, elem)
		if a := c.assign(elem, got); !a.ok {
			c.mismatchElem(el, elem, got, a)
		}
	}
	return &List{elem}
}

func (c *checker) mismatchElem(el ast.Expr, elem, got Type, a assignment) {
	if a.why != "" {
		c.mismatch(el, elem, got, a, "", "")
		return
	}
	c.errorf(el, "make every element "+article(elem), "this list holds %s, but this element is %s", plural(elem), article(got))
}

func (c *checker) record(e *ast.RecordLit, sc *scope, x *ctx) Type {
	n, ok := c.named[e.Type.Name]
	if !ok || n.IsSum() {
		for _, f := range e.Fields {
			c.expr(f.Value, sc, x, nil)
		}
		if v, isVariant := c.variant[e.Type.Name]; isVariant {
			c.errorf(e.Type, fmt.Sprintf("build a variant with parentheses: `%s(%s)`", v.Name, fieldNames(v.Fields)), "`%s` is a variant, not a record", v.Name)
		} else if ok {
			c.errorf(e.Type, "build one of its variants instead, like `"+n.Variants[0].Name+"(...)`", "`%s` is a sum type, not a record", n.Name)
		} else {
			c.errorf(e.Type, fmt.Sprintf("declare it with `type %s { ... }`", e.Type.Name), "there is no record type `%s`", e.Type.Name)
		}
		return bad
	}
	given := map[string]bool{}
	for _, fi := range e.Fields {
		f, exists := n.field(fi.Name.Name)
		if !exists {
			c.expr(fi.Value, sc, x, nil)
			c.errorf(fi.Name, c.didYouMean(fi.Name.Name, n, false), "`%s` has no field `%s`", n.Name, fi.Name.Name)
			continue
		}
		if given[f.Name] {
			c.errorf(fi.Name, "remove one of them", "field `%s` is given twice", f.Name)
		}
		given[f.Name] = true
		c.fieldValue(n, f, fi.Value, sc, x)
	}
	for _, f := range n.Fields {
		if !given[f.Name] {
			c.errorf(e, fmt.Sprintf("records have no defaults; add `%s: ...` (use `none` if it is optional)", f.Name),
				"`%s` is missing field `%s`", n.Name, f.Name)
			break
		}
	}
	return n
}

func (c *checker) fieldValue(n *Named, f Field, v ast.Expr, sc *scope, x *ctx) {
	got := c.expr(v, sc, x, f.Type)
	if a := c.assign(f.Type, got); !a.ok {
		c.mismatch(v, f.Type, got, a, fmt.Sprintf("`%s` field `%s` is %s", n.Name, f.Name, article(f.Type)),
			fmt.Sprintf("give `%s` %s", f.Name, article(f.Type)))
	}
}

func (c *checker) with(e *ast.WithExpr, sc *scope, x *ctx) Type {
	t := c.expr(e.X, sc, x, nil)
	n, ok := prune(t).(*Named)
	if !ok || n.IsSum() {
		for _, fi := range e.Fields {
			c.expr(fi.Value, sc, x, nil)
		}
		if _, isBad := prune(t).(*Bad); !isBad {
			c.errorf(e.X, "", "`with` copies a record, but this is %s", article(t))
		}
		return bad
	}
	for _, fi := range e.Fields {
		f, exists := n.field(fi.Name.Name)
		if !exists {
			c.expr(fi.Value, sc, x, nil)
			c.errorf(fi.Name, c.didYouMean(fi.Name.Name, n, false), "`%s` has no field `%s`", n.Name, fi.Name.Name)
			continue
		}
		c.fieldValue(n, f, fi.Value, sc, x)
	}
	return n
}

// field checks `x.name` and `x?.name`. A method name gives the method as a
// function value.
func (c *checker) field(e *ast.FieldExpr, sc *scope, x *ctx) Type {
	if id, ok := e.X.(*ast.Ident); ok {
		if _, isValue := sc.lookup(id.Name); !isValue {
			if s, ok := c.methods[id.Name][e.Name.Name]; ok && !s.self {
				return s.funcType()
			}
		}
	}
	recv := c.expr(e.X, sc, x, nil)
	return c.member(e, recv, e.X, e.Name, e.Optional)
}

func (s *sig) funcType() *Func {
	ps := make([]Type, len(s.params))
	for i, p := range s.params {
		ps[i] = p.Type
	}
	return &Func{ps, s.result}
}

// member finds the field or method `name` on a value of type recv.
func (c *checker) member(e ast.Node, recv Type, xe ast.Expr, name *ast.Ident, optional bool) Type {
	r := prune(recv)
	if _, ok := r.(*Bad); ok {
		return bad
	}
	if o, ok := r.(*Opt); ok {
		if !optional {
			src := c.text(xe)
			ft := c.lookupMember(o.Elem, name.Name)
			c.errorf(xe, fmt.Sprintf("use `?.` and give a default: `%s?.%s ?? %s`", src, name.Name, fallback(ft)),
				"`%s` is %s, so it might be none and `.%s` cannot be read directly", src, article(recv), name.Name)
			return bad
		}
		t := c.member(e, o.Elem, xe, name, false)
		return optional2(t)
	}
	if optional {
		if _, unknown := r.(*Var); !unknown {
			c.errorf(name, fmt.Sprintf("use `.%s`", name.Name), "`%s` is %s, which is never none, so `?.` is not needed", c.text(xe), article(recv))
		}
	}
	if f, ok := r.(*Fail); ok {
		src := c.text(xe)
		c.errorf(xe, fmt.Sprintf("handle the failure first: `%s!` passes it on, and `else` gives a fallback", src),
			"`%s` is %s, which might be a failure, so `.%s` cannot be read yet", src, article(recv), name.Name)
		_ = f
		return bad
	}
	if t := c.lookupMember(recv, name.Name); t != nil {
		return t
	}
	switch r := r.(type) {
	case *Var:
		c.errorf(xe, "use it where its type is known, such as the argument of `map` or `filter`", "cannot tell what type `%s` is, so `.%s` cannot be checked", c.text(xe), name.Name)
	case *Named:
		c.errorf(name, c.didYouMean(name.Name, r, true), "`%s` has no field `%s`", r.Name, name.Name)
	default:
		c.errorf(name, "", "%s has no field or method `%s`", Article(String(recv)), name.Name)
	}
	return bad
}

func optional2(t Type) Type {
	if _, ok := prune(t).(*Bad); ok {
		return t
	}
	return optional(t)
}

// lookupMember returns the type of field or method `name` on t, or nil.
func (c *checker) lookupMember(t Type, name string) Type {
	switch t := prune(t).(type) {
	case *Named:
		if f, ok := t.field(name); ok {
			return f.Type
		}
		if s, ok := c.methods[t.Name][name]; ok && s.self {
			return s.funcType()
		}
	case *Iface:
		return ifaceMember(t, name)
	case *Param:
		if in, ok := t.Constraint.(*Iface); ok && in != Ord {
			return ifaceMember(in, name)
		}
	}
	return nil
}

func ifaceMember(in *Iface, name string) Type {
	for _, m := range in.Methods {
		if m.Name == name {
			ps := make([]Type, len(m.Params))
			for i, p := range m.Params {
				ps[i] = p.Type
			}
			return &Func{ps, m.Result}
		}
	}
	return nil
}

// didYouMean suggests the closest field (or method) name, or lists the fields.
func (c *checker) didYouMean(name string, n *Named, withMethods bool) string {
	var names []string
	for _, f := range n.Fields {
		names = append(names, f.Name)
	}
	if withMethods {
		for m, s := range c.methods[n.Name] {
			if s.self {
				names = append(names, m)
			}
		}
	}
	best, bestDist := "", 3
	for _, cand := range names {
		if d := distance(name, cand); d < bestDist {
			best, bestDist = cand, d
		}
	}
	if best != "" {
		return fmt.Sprintf("did you mean `%s`?", best)
	}
	if len(n.Fields) == 0 {
		return ""
	}
	return "its fields are " + fieldNames(n.Fields)
}

// distance is the edit distance between two names.
func distance(a, b string) int {
	ar, br := []rune(a), []rune(b)
	prev := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		cur := make([]int, len(br)+1)
		cur[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(br)]
}

func (c *checker) binary(e *ast.BinaryExpr, sc *scope, x *ctx, want Type) Type {
	switch e.Op {
	case token.QQ:
		return c.coalesce(e, sc, x, want)
	case token.And, token.Or:
		return c.logical(e, sc, x)
	}
	l := c.expr(e.Left, sc, x, nil)
	r := c.expr(e.Right, sc, x, nil)
	if isBad(l) || isBad(r) {
		if e.Op == token.Eq || e.Op == token.NotEq || e.Op == token.Lt || e.Op == token.LtEq || e.Op == token.Gt || e.Op == token.GtEq {
			return Bool
		}
		return bad
	}
	for _, side := range []struct {
		t Type
		n ast.Expr
	}{{l, e.Left}, {r, e.Right}} {
		if f, ok := prune(side.t).(*Fail); ok {
			src := c.text(side.n)
			c.errorf(side.n, fmt.Sprintf("handle the failure first: `%s else %s` gives a fallback, and `%s!` passes it on", src, fallback(f.Elem), src),
				"`%s` needs values, but this is %s, which might be a failure", e.Op, article(side.t))
			return bad
		}
	}
	switch e.Op {
	case token.DotDot:
		c.numberLike(l, r)
		for _, side := range []struct {
			t Type
			n ast.Expr
		}{{l, e.Left}, {r, e.Right}} {
			if prune(side.t) != Int {
				c.errorf(side.n, "", "`..` makes a range of Ints, but this is %s", article(side.t))
				return bad
			}
		}
		return &List{Int}
	case token.Eq, token.NotEq:
		if _, ok := prune(l).(*Func); ok {
			c.errorf(e, "compare the results of calling them instead", "functions cannot be compared with `%s`", e.Op)
			return Bool
		}
		if !c.assign(l, r).ok && !c.assign(r, l).ok {
			c.errorf(e, "compare two values of the same type", "cannot compare %s with %s using `%s`", article(l), article(r), e.Op)
		}
		return Bool
	case token.Lt, token.LtEq, token.Gt, token.GtEq:
		c.numberLike(l, r)
		if !unify(l, r) || !ordered(l) {
			c.errorf(e, "compare two Ints, two Floats or two Strs", "cannot compare %s with %s", article(l), article(r))
		}
		return Bool
	}
	return c.arith(e, l, r)
}

func isBad(t Type) bool { _, ok := prune(t).(*Bad); return ok }

// numberLike solves an unknown side of an operator from the known side.
func (c *checker) numberLike(l, r Type) {
	_, lv := prune(l).(*Var)
	_, rv := prune(r).(*Var)
	if lv && !rv {
		unify(l, r)
	} else if rv && !lv {
		unify(r, l)
	}
}

func (c *checker) arith(e *ast.BinaryExpr, l, r Type) Type {
	c.numberLike(l, r)
	if _, ok := prune(l).(*Var); ok {
		unify(l, Int)
		unify(r, Int)
	}
	pl, pr := prune(l), prune(r)
	isNum := func(t Type) bool { return t == Int || t == Float }
	switch {
	case pl == pr && pl == Int:
		return Int
	case pl == pr && pl == Float:
		if e.Op == token.Percent {
			c.errorf(e, "`%` works on Ints", "`%%` needs two Ints")
			return bad
		}
		return Float
	case isNum(pl) && isNum(pr):
		c.errorf(e, "write the Int as a Float, like `2.0` instead of `2`; Fennic never converts numbers for you",
			"`%s` cannot mix an Int and a Float", e.Op)
	case e.Op == token.Plus && (pl == Str || pr == Str):
		c.errorf(e, `to join strings, interpolate them: "{a}{b}"`, "`+` adds numbers, not strings")
	default:
		if o, ok := pl.(*Opt); ok && isNum(prune(o.Elem)) {
			src := c.text(e.Left)
			c.errorf(e.Left, fmt.Sprintf("give a default for when it is missing: `%s ?? %s`", src, fallback(o.Elem)),
				"`%s` needs numbers, but this is %s, which might be none", e.Op, article(l))
			return bad
		}
		if o, ok := pr.(*Opt); ok && isNum(prune(o.Elem)) {
			src := c.text(e.Right)
			c.errorf(e.Right, fmt.Sprintf("give a default for when it is missing: `%s ?? %s`", src, fallback(o.Elem)),
				"`%s` needs numbers, but this is %s, which might be none", e.Op, article(r))
			return bad
		}
		c.errorf(e, "", "`%s` needs two numbers, but got %s and %s", e.Op, article(l), article(r))
	}
	return bad
}

func (c *checker) coalesce(e *ast.BinaryExpr, sc *scope, x *ctx, want Type) Type {
	var wantL Type
	if want != nil {
		wantL = optional(success(want))
	}
	l := c.expr(e.Left, sc, x, wantL)
	var elem Type
	switch pl := prune(l).(type) {
	case *Bad:
		c.expr(e.Right, sc, x, want)
		return bad
	case *Opt:
		elem = pl.Elem
	case *Var:
		elem = c.fresh()
		unify(pl, &Opt{elem})
	default:
		c.expr(e.Right, sc, x, nil)
		c.errorSpan(e.Left.End(), e.End(), "remove `?? "+c.text(e.Right)+"`", "`%s` is %s, which is never none, so `??` is not needed", c.text(e.Left), article(l))
		return l
	}
	r := c.expr(e.Right, sc, x, elem)
	if ro, ok := prune(r).(*Opt); ok {
		// a ?? b ?? c: the right side may itself be optional
		if a := c.assign(elem, ro.Elem); !a.ok {
			c.mismatch(e.Right, elem, r, a, "the default after `??` must be "+article(elem), "give a default of the same type as the value")
		}
		return &Opt{elem}
	}
	if a := c.assign(elem, r); !a.ok {
		c.mismatch(e.Right, elem, r, a, "the default after `??` must be "+article(elem), "give a default of the same type as the value")
	}
	return elem
}

// propagate checks `x!`.
func (c *checker) propagate(e *ast.PropagateExpr, sc *scope, x *ctx, want Type) Type {
	var wantX Type
	if want != nil {
		wantX = failable(success(want))
	}
	t := c.expr(e.X, sc, x, wantX)
	f, ok := prune(t).(*Fail)
	if !ok {
		if !isBad(t) {
			c.errorSpan(e.X.End(), e.End(), "remove the `!`", "`%s` cannot fail, so there is nothing for `!` to pass on", c.text(e.X))
		}
		return t
	}
	for k := x; k != nil; k = k.parent {
		switch k.kind {
		case ctxLambda:
			k.usedBang = true
			return f.Elem
		case ctxTogether, ctxTest, ctxTop:
			return f.Elem
		case ctxFn:
			if _, can := prune(k.fn.result).(*Fail); !can && !k.fn.isMain() {
				c.errorf(e, fmt.Sprintf("change its result type to `-> %s!`, or handle the failure here with `else`", String(k.fn.result)),
					"`!` passes a failure on, but `%s` cannot fail", k.fn.name)
			}
			return f.Elem
		}
	}
	return f.Elem
}

// elseExpr checks `x else fallback` and `x else err -> handler`.
func (c *checker) elseExpr(e *ast.ElseExpr, sc *scope, x *ctx, want Type) Type {
	var wantX Type
	if want != nil {
		wantX = failable(want)
	}
	t := c.expr(e.X, sc, x, wantX)
	inner := sc
	if e.ErrName != nil {
		inner = newScope(sc)
		inner.vars[e.ErrName.Name] = &binding{t: Error}
	}
	f, ok := prune(t).(*Fail)
	if !ok {
		c.expr(e.Fallback, inner, x, nil)
		if !isBad(t) {
			c.errorSpan(e.X.End(), e.End(), "remove `else` and its fallback", "`%s` cannot fail, so `else` never runs", c.text(e.X))
		}
		return t
	}
	got := c.expr(e.Fallback, inner, x, f.Elem)
	if a := c.assign(f.Elem, got); !a.ok {
		at := ast.Expr(e.Fallback)
		if b, isBlock := e.Fallback.(*ast.Block); isBlock {
			if last := lastExpr(b); last != nil {
				at = last
			}
		}
		c.mismatch(at, f.Elem, got, a, "the fallback after `else` must be "+article(f.Elem), "give a fallback of the same type as the value")
	}
	return f.Elem
}

// lambda checks `x -> body`. If want is a function type, its parameter
// types become the lambda's.
func (c *checker) lambda(e *ast.LambdaExpr, sc *scope, x *ctx, want Type) Type {
	wf, _ := prune(want).(*Func)
	inner := newScope(sc)
	params := make([]Type, len(e.Params))
	for i, p := range e.Params {
		if wf != nil && i < len(wf.Params) {
			params[i] = wf.Params[i]
		} else {
			params[i] = c.fresh()
		}
		inner.vars[p.Name] = &binding{t: params[i]}
	}
	lx := &ctx{kind: ctxLambda, parent: x}
	var wantResult Type
	if wf != nil {
		wantResult = success(wf.Result)
		if _, unknown := prune(wantResult).(*Var); unknown {
			wantResult = nil
		}
	}
	result := c.expr(e.Body, inner, lx, wantResult)
	if lx.usedBang {
		result = failable(result)
	}
	return &Func{params, result}
}

func (c *checker) match(e *ast.MatchExpr, sc *scope, x *ctx, want Type) Type {
	t := c.expr(e.X, sc, x, nil)
	head := ast.Span{From: e.Pos(), To: e.X.End()}
	var result Type
	var total ast.Node               // the first arm that matches anything
	covered := map[string]ast.Node{} // variant name -> the arm that handles all of it
	seenTrue, seenFalse := false, false
	for _, arm := range e.Arms {
		inner := newScope(sc)
		c.pattern(arm.Pattern, t, inner)
		switch {
		case total != nil:
			c.errorf(arm.Pattern, fmt.Sprintf("remove it, or move it above line %d", total.Pos().Line),
				"this arm can never run: the arm on line %d already matches everything", total.Pos().Line)
		case irrefutable(arm.Pattern):
			total = arm.Pattern
		default:
			if vp, ok := arm.Pattern.(*ast.VariantPattern); ok {
				name := vp.Name.Name
				if prev, done := covered[name]; done {
					c.errorf(arm.Pattern, fmt.Sprintf("remove it, or move it above line %d", prev.Pos().Line),
						"this arm can never run: `%s` is already handled on line %d", name, prev.Pos().Line)
				} else if allIrrefutable(vp.Args) {
					covered[name] = arm.Pattern
				}
			}
			if lp, ok := arm.Pattern.(*ast.LiteralPattern); ok {
				if b, isBool := lp.Value.(*ast.BoolLit); isBool {
					seenTrue = seenTrue || b.Value
					seenFalse = seenFalse || !b.Value
				}
			}
		}
		if result == nil {
			result = c.expr(arm.Body, inner, x, want)
			continue
		}
		got := c.expr(arm.Body, inner, x, result)
		if a := c.assign(result, got); !a.ok {
			at := arm.Body
			if b, isBlock := arm.Body.(*ast.Block); isBlock {
				if last := lastExpr(b); last != nil {
					at = last
				}
			}
			c.errorf(at, "make every arm of a `match` give the same type", "this arm gives %s, but the first arm gives %s", article(got), article(result))
		}
	}
	if total == nil && !isBad(t) {
		n, isSum := prune(t).(*Named)
		switch {
		case isSum && n.IsSum():
			var names, arms []string
			for _, v := range n.Variants {
				if _, ok := covered[v.Name]; !ok {
					names = append(names, "`"+v.Name+"`")
					arms = append(arms, "`"+variantPatternText(v)+" -> ...`")
				}
			}
			if len(names) > 0 {
				help := "add an arm: " + arms[0]
				if len(arms) > 1 {
					help = "add arms: " + strings.Join(arms, ", ")
				}
				c.errorf(head, help, "this `match` does not handle %s", joinOr(names))
			}
		case prune(t) == Bool && seenTrue && seenFalse:
		default:
			c.errorf(head, "add `_ -> ...` as the last arm to handle the rest", "this `match` does not handle every value")
		}
	}
	if result == nil {
		return bad
	}
	return result
}

func variantPatternText(v *Variant) string {
	if len(v.Fields) == 0 {
		return v.Name
	}
	return fmt.Sprintf("%s(%s)", v.Name, fieldNames(v.Fields))
}

// irrefutable reports whether a pattern matches every value of its type.
func irrefutable(p ast.Pattern) bool {
	switch p := p.(type) {
	case *ast.WildcardPattern, *ast.BindPattern:
		return true
	case *ast.TuplePattern:
		return allIrrefutable(p.Elems)
	}
	return false
}

func allIrrefutable(ps []ast.Pattern) bool {
	for _, p := range ps {
		if !irrefutable(p) {
			return false
		}
	}
	return true
}

// pattern checks a pattern against the type of the value it matches, and
// binds its names in sc.
func (c *checker) pattern(p ast.Pattern, t Type, sc *scope) {
	switch p := p.(type) {
	case *ast.WildcardPattern:
	case *ast.BindPattern:
		sc.vars[p.Name.Name] = &binding{t: t}
	case *ast.LiteralPattern:
		lt := c.expr(p.Value, sc, nil, nil)
		if !unify(t, lt) && !isBad(t) {
			c.errorf(p, "", "this pattern is %s, but the value matched is %s", article(lt), article(t))
		}
	case *ast.TuplePattern:
		elems := make([]Type, len(p.Elems))
		for i := range elems {
			elems[i] = c.fresh()
		}
		if !unify(t, &Tuple{elems}) {
			if !isBad(t) {
				c.errorf(p, "", "this pattern is a tuple of %d, but the value matched is %s", len(p.Elems), article(t))
			}
			for i := range elems {
				elems[i] = bad
			}
		}
		for i, ep := range p.Elems {
			c.pattern(ep, elems[i], sc)
		}
	case *ast.ListPattern:
		elem := Type(c.fresh())
		if !unify(t, &List{elem}) {
			if !isBad(t) {
				c.errorf(p, "", "this pattern is a list, but the value matched is %s", article(t))
			}
			elem = bad
		}
		for _, ep := range p.Elems {
			c.pattern(ep, elem, sc)
		}
	case *ast.VariantPattern:
		name := p.Name.Name
		var v *Variant
		n, isSum := prune(t).(*Named)
		switch {
		case isSum && n.IsSum():
			for _, cand := range n.Variants {
				if cand.Name == name {
					v = cand
				}
			}
			if v == nil {
				c.errorf(p.Name, "its variants are "+variantList(n), "`%s` has no variant `%s`", n.Name, name)
			}
		case isBad(t):
		default:
			if gv, ok := c.variant[name]; ok && unify(t, gv.Sum) {
				v = gv
			} else if ok {
				c.errorf(p, "", "this pattern is %s, but the value matched is %s", Article(gv.Sum.Name), article(t))
			} else {
				c.errorf(p.Name, "declare it in a sum type, like `type T = | "+name+"(...)`", "there is no variant `%s`", name)
			}
		}
		if v != nil && len(p.Args) != len(v.Fields) {
			c.errorf(p, fmt.Sprintf("write `%s`", variantPatternText(v)),
				"`%s` has %d %s, but the pattern names %d", v.Name, len(v.Fields), plural2(len(v.Fields), "field"), len(p.Args))
			v = nil
		}
		for i, ap := range p.Args {
			ft := Type(bad)
			if v != nil {
				ft = v.Fields[i].Type
			}
			c.pattern(ap, ft, sc)
		}
	}
}

func variantList(n *Named) string {
	var names []string
	for _, v := range n.Variants {
		names = append(names, v.Name)
	}
	return joinAnd(names)
}

func joinAnd(names []string) string { return joinWith(names, "and") }
func joinOr(names []string) string  { return joinWith(names, "or") }

func joinWith(names []string, word string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " " + word + " " + names[len(names)-1]
}

func (c *checker) needBool(n ast.Expr, t Type, what string) {
	if a := c.assign(Bool, t); !a.ok {
		c.mismatch(n, Bool, t, a, what+" needs a Bool", "compare it to make a Bool, like `x > 0`")
	}
}

// logical checks `a and b` and `a or b`.
func (c *checker) logical(e *ast.BinaryExpr, sc *scope, x *ctx) Type {
	l := c.expr(e.Left, sc, x, Bool)
	if f, ok := prune(l).(*Fail); ok && e.Op == token.Or {
		// The fallback used to be written `or`.
		c.expr(e.Right, sc, x, nil)
		c.errorf(e, fmt.Sprintf("to give a fallback for a failure, write `else`: `%s else %s`", c.text(e.Left), c.text(e.Right)),
			"`or` combines two Bools, but this is %s, which might be a failure", article(f))
		return bad
	}
	r := c.expr(e.Right, sc, x, Bool)
	c.needBool(e.Left, l, "`"+e.Op.String()+"`")
	c.needBool(e.Right, r, "`"+e.Op.String()+"`")
	return Bool
}

// ifExpr checks `if c { ... } else { ... }`. Without an else it gives
// nothing; with one, both branches give the same type.
func (c *checker) ifExpr(e *ast.IfExpr, sc *scope, x *ctx, want Type) Type {
	ct := c.expr(e.Cond, sc, x, Bool)
	c.needBool(e.Cond, ct, "`if`")
	if e.Else == nil {
		c.block(e.Then, sc, x, nil)
		return Unit
	}
	then := c.block(e.Then, sc, x, want)
	if want == nil {
		if _, never := prune(then).(*Var); !never {
			want = then
		}
	}
	other := c.expr(e.Else, sc, x, want)
	if a := c.assign(then, other); !a.ok {
		if a2 := c.assign(other, then); a2.ok {
			return other // the then branch is the one that cannot finish, as with fail
		}
		at := ast.Node(e.Else)
		if b, ok := e.Else.(*ast.Block); ok {
			if last := lastExpr(b); last != nil {
				at = last
			}
		}
		c.errorf(at, "make both branches of the `if` give the same type", "this branch gives %s, but the first branch gives %s", article(other), article(then))
	}
	return then
}

// fail checks `fail "message"`. It never gives a value, so its type is
// whatever the place it stands in needs.
func (c *checker) fail(e *ast.FailExpr, sc *scope, x *ctx) Type {
	mt := c.expr(e.Msg, sc, x, Str)
	if a := c.assign(Str, mt); !a.ok {
		c.mismatch(e.Msg, Str, mt, a, "`fail` needs a Str", "pass a Str")
	}
	if fn := c.cannotFail(x); fn != nil {
		c.errorf(e, fmt.Sprintf("change its result type to `-> %s!`", String(fn.result)), "`fail` stops `%s` with a failure, but `%s` cannot fail", fn.name, fn.name)
	}
	return c.fresh()
}

// cannotFail returns the function whose result a failure would become, if
// that function cannot fail. Lambdas become failable instead.
func (c *checker) cannotFail(x *ctx) *sig {
	for k := x; k != nil; k = k.parent {
		switch k.kind {
		case ctxLambda:
			k.usedBang = true
			return nil
		case ctxTogether, ctxTest, ctxTop:
			return nil
		case ctxFn:
			if _, can := prune(k.fn.result).(*Fail); can || k.fn.isMain() {
				return nil
			}
			return k.fn
		}
	}
	return nil
}

// index checks `xs[i]`, which gives an optional: the index may be out of range.
func (c *checker) index(e *ast.IndexExpr, sc *scope, x *ctx) Type {
	t := c.expr(e.X, sc, x, nil)
	it := c.expr(e.Index, sc, x, Int)
	switch l := prune(t).(type) {
	case *Bad:
		return bad
	case *List:
		if a := c.assign(Int, it); !a.ok {
			c.mismatch(e.Index, Int, it, a, "a list index needs an Int", "pass an Int")
		}
		return optional(l.Elem)
	case *Var:
		elem := c.fresh()
		unify(l, &List{elem})
		return optional(elem)
	}
	c.errorf(e.X, "", "only a List can be indexed with `[...]`, but this is %s", article(t))
	return bad
}

// funcLit checks an anonymous function, `fn() { ... }`.
func (c *checker) funcLit(e *ast.FuncLit, sc *scope, x *ctx) Type {
	inner := newScope(sc)
	params := make([]Type, len(e.Params))
	for i, p := range e.Params {
		params[i] = c.typeOf(p.Type, nil)
		inner.vars[p.Name.Name] = &binding{t: params[i]}
	}
	lx := &ctx{kind: ctxLambda, parent: x}
	var result Type
	if e.Result != nil {
		result = c.typeOf(e.Result, nil)
		got := c.block(e.Body, inner, lx, result)
		if a := c.assign(result, got); !a.ok {
			if last := lastExpr(e.Body); last != nil {
				c.mismatch(last, result, got, a, "this function returns "+article(result), "end the body with "+article(result))
			}
		}
	} else {
		c.block(e.Body, inner, lx, nil)
		result = Unit
	}
	if lx.usedBang {
		result = failable(result)
	}
	return &Func{params, result}
}
