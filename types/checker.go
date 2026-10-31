package types

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kirannarayanak/fennic/ast"
	"github.com/kirannarayanak/fennic/diag"
	"github.com/kirannarayanak/fennic/token"
)

// Info is what the checker learned that running the program needs.
type Info struct {
	// Targets is the type each `json.parse` or `csv.read` call builds,
	// keyed by the call (a *ast.CallExpr, or the *ast.PipeExpr of
	// `text |> json.parse()`). For csv.read it is the record of each row.
	Targets map[ast.Node]Type
	// Fallbacks is, for each `!` at the top level or in `main`, the text to
	// suggest after `else` if the failure stops the program: a plain value of
	// the type, or a handler.
	Fallbacks map[*ast.PropagateExpr]string
	// Names is what each identifier the checker resolved names, for the
	// language server.
	Names map[*ast.Ident]*Name
}

// Check type-checks a parsed program. It returns every problem found, in
// source order, and the information the interpreter needs.
func Check(file *ast.File, src string) ([]diag.Diagnostic, *Info) {
	c := &checker{
		src:     src,
		named:   map[string]*Named{},
		ifaces:  map[string]*Iface{},
		variant: map[string]*Variant{},
		funcs:   map[string]*sig{},
		methods: map[string]map[string]*sig{},
		info:    &Info{Targets: map[ast.Node]Type{}, Fallbacks: map[*ast.PropagateExpr]string{}, Names: map[*ast.Ident]*Name{}},
		types:   map[ast.Expr]Type{},
	}
	c.declareNames(file)
	c.declare(file)
	c.body(file)
	c.finishNames()
	sort.SliceStable(c.diags, func(i, j int) bool { return c.diags[i].Pos.Offset < c.diags[j].Pos.Offset })
	for n, t := range c.info.Targets {
		c.info.Targets[n] = Resolve(t)
	}
	return c.diags, c.info
}

// sig is a declared function or method signature.
type sig struct {
	decl    *ast.FnDecl
	name    string // as the program calls it: `greet`, `Config.default`
	tparams []*Param
	params  []Field // without self
	result  Type
	self    bool
}

type checker struct {
	src     string
	named   map[string]*Named
	ifaces  map[string]*Iface
	variant map[string]*Variant
	funcs   map[string]*sig
	methods map[string]map[string]*sig // type name -> method name -> signature
	info    *Info
	diags   []diag.Diagnostic

	decl         decls
	types        map[ast.Expr]Type // the type of each expression checked
	builtinCalls []builtinCall
	nextVar      int
	pending      []constraintCheck
}

func (c *checker) fresh() *Var { c.nextVar++; return &Var{id: c.nextVar} }

func (c *checker) errorf(n ast.Node, help, format string, args ...any) {
	c.errorSpan(n.Pos(), n.End(), help, format, args...)
}

func (c *checker) errorSpan(from, to token.Pos, help, format string, args ...any) {
	c.diags = append(c.diags, diag.Diagnostic{Pos: from, End: to, Msg: fmt.Sprintf(format, args...), Help: help})
}

// text is the source text of a node, for quoting it back in a message.
func (c *checker) text(n ast.Node) string { return c.src[n.Pos().Offset:n.End().Offset] }

// ---- declarations ----

func (c *checker) declare(file *ast.File) {
	// Names first, so declarations can refer to each other in any order.
	for _, it := range file.Items {
		switch d := it.(type) {
		case *ast.RecordDecl:
			c.named[d.Name.Name] = &Named{Name: d.Name.Name}
		case *ast.SumDecl:
			n := &Named{Name: d.Name.Name, Variants: []*Variant{}}
			c.named[d.Name.Name] = n
			for _, v := range d.Variants {
				vt := &Variant{Name: v.Name.Name, Sum: n}
				n.Variants = append(n.Variants, vt)
				c.variant[vt.Name] = vt
			}
		case *ast.InterfaceDecl:
			c.ifaces[d.Name.Name] = &Iface{Name: d.Name.Name}
		}
	}
	for _, it := range file.Items {
		switch d := it.(type) {
		case *ast.RecordDecl:
			n := c.named[d.Name.Name]
			for _, f := range d.Fields {
				n.Fields = append(n.Fields, Field{f.Name.Name, c.typeOf(f.Type, nil)})
			}
		case *ast.SumDecl:
			n := c.named[d.Name.Name]
			for i, v := range d.Variants {
				for _, f := range v.Fields {
					n.Variants[i].Fields = append(n.Variants[i].Fields, Field{f.Name.Name, c.typeOf(f.Type, nil)})
				}
			}
		case *ast.InterfaceDecl:
			in := c.ifaces[d.Name.Name]
			for _, m := range d.Methods {
				meth := &Method{Name: m.Name.Name, Result: Unit}
				for _, p := range m.Params {
					if p.Type != nil {
						meth.Params = append(meth.Params, Field{p.Name.Name, c.typeOf(p.Type, nil)})
					}
				}
				if m.Result != nil {
					meth.Result = c.typeOf(m.Result, nil)
				}
				in.Methods = append(in.Methods, meth)
			}
		}
	}
	for _, it := range file.Items {
		d, ok := it.(*ast.FnDecl)
		if !ok {
			continue
		}
		s := c.signature(d)
		if d.Recv == nil {
			c.funcs[d.Name.Name] = s
			if s.isMain() {
				c.checkMain(s)
			}
			continue
		}
		if _, known := c.named[d.Recv.Name]; !known {
			c.errorf(d.Recv, fmt.Sprintf("declare it with `type %s { ... }`", d.Recv.Name), "there is no type `%s` to add a method to", d.Recv.Name)
			continue
		}
		ms := c.methods[d.Recv.Name]
		if ms == nil {
			ms = map[string]*sig{}
			c.methods[d.Recv.Name] = ms
		}
		ms[d.Name.Name] = s
	}
}

func (c *checker) signature(d *ast.FnDecl) *sig {
	s := &sig{decl: d, name: d.Name.Name, result: Unit, self: d.HasSelf()}
	if d.Recv != nil && !s.self {
		s.name = d.Recv.Name + "." + d.Name.Name
	}
	scope := map[string]*Param{}
	for _, tp := range d.TypeParams {
		p := &Param{Name: tp.Name.Name}
		if tp.Constraint != nil {
			p.Constraint = c.constraint(tp.Constraint)
		}
		scope[p.Name] = p
		s.tparams = append(s.tparams, p)
	}
	for _, p := range d.Params {
		if p.Type == nil {
			continue // self
		}
		s.params = append(s.params, Field{p.Name.Name, c.typeOf(p.Type, scope)})
	}
	if d.Result != nil {
		s.result = c.typeOf(d.Result, scope)
	}
	return s
}

func (c *checker) constraint(t ast.TypeExpr) Type {
	if nt, ok := t.(*ast.NamedType); ok && len(nt.Args) == 0 {
		if nt.Name.Name == "Ord" {
			return Ord
		}
		if in, ok := c.ifaces[nt.Name.Name]; ok {
			return in
		}
	}
	c.errorf(t, "use `Ord` or the name of an interface", "`%s` is not an interface, so it cannot constrain a type parameter", c.text(t))
	return nil
}

// typeOf turns a written type into a Type. scope holds the type parameters
// in view.
func (c *checker) typeOf(t ast.TypeExpr, scope map[string]*Param) Type {
	switch t := t.(type) {
	case *ast.OptionalType:
		elem := c.typeOf(t.Elem, scope)
		switch prune(elem).(type) {
		case *Opt:
			c.errorf(t, "write `"+c.text(t.Elem)+"` once; a value is either there or not", "`%s` is optional twice", c.text(t))
			return elem
		case *Fail:
			c.errorf(t, "put `?` before `!`: `"+String(success(elem))+"?!`", "`%s` is not a type", c.text(t))
			return elem
		}
		return &Opt{elem}
	case *ast.FailableType:
		elem := c.typeOf(t.Elem, scope)
		if _, ok := prune(elem).(*Fail); ok {
			c.errorf(t, "write `!` once", "`%s` can fail twice", c.text(t))
			return elem
		}
		return &Fail{elem}
	case *ast.NamedType:
		name := t.Name.Name
		c.typeName(t.Name)
		if p, ok := scope[name]; ok {
			return p
		}
		switch name {
		case "Int":
			return Int
		case "Float":
			return Float
		case "Str":
			return Str
		case "Bool":
			return Bool
		case "Date":
			return Date
		case "Chan":
			if len(t.Args) != 1 {
				c.errorf(t, "write what it carries: `Chan[Str]`", "`Chan` needs one element type")
				return bad
			}
			return &Chan{c.typeOf(t.Args[0], scope)}
		case "List":
			if len(t.Args) != 1 {
				c.errorf(t, "write the element type: `List[Int]`", "`List` needs one element type")
				return bad
			}
			return &List{c.typeOf(t.Args[0], scope)}
		case "Map":
			if len(t.Args) != 2 {
				c.errorf(t, "write the key and value types: `Map[Str, Int]`", "`Map` needs a key type and a value type")
				return bad
			}
			m := &Map{c.typeOf(t.Args[0], scope), c.typeOf(t.Args[1], scope)}
			if !keyType(m.Key) {
				c.errorf(t.Args[0], "use Str, Int or Bool keys", "map keys must be Str, Int or Bool, but this is %s", article(m.Key))
			}
			return m
		}
		if len(t.Args) > 0 {
			c.errorf(t, "remove the `[...]`", "`%s` takes no type arguments", name)
		}
		if n, ok := c.named[name]; ok {
			return n
		}
		if in, ok := c.ifaces[name]; ok {
			return in
		}
		c.errorf(t, fmt.Sprintf("declare it with `type %s { ... }`, or check the spelling", name), "there is no type `%s`", name)
		return bad
	}
	return bad
}

// ---- assignability ----

// assignment is the outcome of checking a value against an expected type.
// When an interface is not satisfied, why and fix say so.
type assignment struct {
	ok       bool
	why, fix string
}

// assign checks that a value of type got can be used where want is
// expected: a T fits a T? and a T!, and a type fits an interface it
// satisfies. Variables are solved along the way.
func (c *checker) assign(want, got Type) assignment {
	want, got = prune(want), prune(got)
	switch got.(type) {
	case *Bad:
		return assignment{ok: true}
	case *Var:
		if _, ok := want.(*Var); !ok {
			return assignment{ok: unify(want, got)}
		}
	}
	switch w := want.(type) {
	case *Bad:
		return assignment{ok: true}
	case *Var:
		return assignment{ok: unify(w, got)}
	case *Opt:
		if g, ok := got.(*Opt); ok {
			return c.assign(w.Elem, g.Elem)
		}
		if _, ok := got.(*Fail); ok {
			return assignment{}
		}
		return c.assign(w.Elem, got)
	case *Fail:
		if g, ok := got.(*Fail); ok {
			return c.assign(w.Elem, g.Elem)
		}
		return c.assign(w.Elem, got)
	case *Iface:
		return c.satisfies(got, w)
	}
	return assignment{ok: unify(want, got)}
}

// satisfies checks that t has every method the interface lists.
func (c *checker) satisfies(t Type, in *Iface) assignment {
	switch t := prune(t).(type) {
	case *Iface:
		return assignment{ok: t == in}
	case *Param:
		return assignment{ok: t.Constraint == in}
	case *Named:
		for _, m := range in.Methods {
			want := ifaceMethodText(t.Name, m)
			impl, ok := c.methods[t.Name][m.Name]
			if !ok || !impl.self {
				return assignment{why: fmt.Sprintf("it has no method `%s`", m.Name), fix: "add `" + want + " { ... }`"}
			}
			if len(impl.params) != len(m.Params) {
				return assignment{
					why: fmt.Sprintf("its `%s` takes %d %s, but `%s` needs %d", m.Name, len(impl.params), plural2(len(impl.params), "argument"), in.Name, len(m.Params)),
					fix: "change it to `" + want + "`",
				}
			}
			for i, p := range m.Params {
				if !unify(impl.params[i].Type, p.Type) {
					return assignment{
						why: fmt.Sprintf("its `%s` takes `%s: %s`, but `%s` needs `%s: %s`", m.Name, impl.params[i].Name, String(impl.params[i].Type), in.Name, p.Name, String(p.Type)),
						fix: "change it to `" + want + "`",
					}
				}
			}
			if !unify(impl.result, m.Result) {
				return assignment{
					why: fmt.Sprintf("its `%s` returns %s, but `%s` needs %s", m.Name, article(impl.result), in.Name, article(m.Result)),
					fix: "change it to `" + want + "`",
				}
			}
		}
		return assignment{ok: true}
	}
	return assignment{}
}

func ifaceMethodText(typeName string, m *Method) string {
	parts := []string{"self"}
	for _, p := range m.Params {
		parts = append(parts, p.Name+": "+String(p.Type))
	}
	s := fmt.Sprintf("fn %s.%s(%s)", typeName, m.Name, strings.Join(parts, ", "))
	if m.Result != Unit {
		s += " -> " + String(m.Result)
	}
	return s
}

func plural2(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

// fallback is source text for a sensible default of type t, used in help.
func (c *checker) fallback(t Type) string {
	if v := c.plainValue(t); v != "" {
		return v
	}
	return "..."
}

// plainValue is the simplest value of t to write as a fallback, or "" when
// t has none: `0`, `[]`, `none`, or `T.default()` where the program has one.
func (c *checker) plainValue(t Type) string {
	switch t := prune(t).(type) {
	case *Basic:
		switch t {
		case Str:
			return `""`
		case Int:
			return "0"
		case Float:
			return "0.0"
		case Bool:
			return "false"
		}
	case *List:
		return "[]"
	case *Map:
		return "Map{}"
	case *Opt:
		return "none"
	case *Named:
		if d, ok := c.methods[t.Name]["default"]; ok && !d.self && len(d.params) == 0 && len(d.tparams) == 0 && prune(d.result) == t {
			return t.Name + ".default()"
		}
	}
	return ""
}

// topFallback is what to suggest after `else` for a failure of a T that
// stops the program.
func (c *checker) topFallback(t Type) string {
	if v := c.plainValue(t); v != "" {
		return v
	}
	if prune(t) == Unit {
		return `err -> log.warn("{err}")`
	}
	return "err -> ..."
}

// mismatch reports a value of type got where want was expected. lead is the
// start of the message, such as "`greet` needs a Str for `who`".
func (c *checker) mismatch(n ast.Node, want, got Type, a assignment, lead, plainHelp string) {
	src := c.text(n)
	if a.why != "" {
		help := a.fix
		if help == "" {
			help = plainHelp
		}
		c.errorf(n, help, "`%s` cannot be used as a `%s`: %s", String(got), String(want), a.why)
		return
	}
	switch g := prune(got).(type) {
	case *Fail:
		if c.assign(want, g.Elem).ok {
			c.errorf(n, fmt.Sprintf("handle the failure first: `%s else %s` gives a fallback, and `%s!` passes it on", src, c.fallback(g.Elem), src),
				"%s, but this is %s, which might be a failure", lead, article(got))
			return
		}
	case *Opt:
		if c.assign(want, g.Elem).ok {
			c.errorf(n, fmt.Sprintf("give a default for when it is missing: `%s ?? %s`", src, c.fallback(g.Elem)),
				"%s, but this is %s, which might be none", lead, article(got))
			return
		}
	}
	help := plainHelp
	// Where a Str is passed, interpolation is the fix. Elsewhere, such as a
	// function's result, the caller's help names the better one.
	if prune(want) == Str && plainHelp == "pass a Str" {
		help = fmt.Sprintf(`to turn a value into a Str, interpolate it: "{%s}"`, src)
	}
	c.errorf(n, help, "%s, but this is %s", lead, article(got))
}

// ---- scopes and bodies ----

type binding struct {
	t       Type
	mutable bool
	decl    *ast.Ident // where it is declared
}

type scope struct {
	vars   map[string]*binding
	parent *scope
}

func newScope(parent *scope) *scope { return &scope{vars: map[string]*binding{}, parent: parent} }

func (s *scope) lookup(name string) (*binding, bool) {
	for ; s != nil; s = s.parent {
		if b, ok := s.vars[name]; ok {
			return b, true
		}
	}
	return nil, false
}

// ctxKind says what a stretch of code is inside, which decides where `!`
// may send a failure.
type ctxKind int

const (
	ctxTop ctxKind = iota
	ctxFn
	ctxLambda
	ctxTogether
	ctxTest
)

type ctx struct {
	kind     ctxKind
	fn       *sig // for ctxFn
	usedBang bool // a lambda used `!`, so its result is failable
	parent   *ctx
}

// insideTogether reports whether code is lexically inside a `together`
// block, without leaving the function it is in.
func (x *ctx) insideTogether() bool {
	for ; x != nil; x = x.parent {
		switch x.kind {
		case ctxTogether:
			return true
		case ctxFn, ctxTest, ctxTop:
			return false
		}
	}
	return false
}

func (c *checker) body(file *ast.File) {
	globals := newScope(nil)
	top := &ctx{kind: ctxTop}
	for _, it := range file.Items {
		if s, ok := it.(ast.Stmt); ok {
			c.stmt(s, globals, top)
			c.flushConstraints(false)
		}
	}
	for _, it := range file.Items {
		switch d := it.(type) {
		case *ast.FnDecl:
			c.fnBody(d, globals)
		case *ast.TestDecl:
			c.block(d.Body, newScope(globals), &ctx{kind: ctxTest, parent: top}, nil)
		}
		c.flushConstraints(false)
	}
	c.flushConstraints(true)
}

func (c *checker) fnBody(d *ast.FnDecl, globals *scope) {
	var s *sig
	if d.Recv == nil {
		s = c.funcs[d.Name.Name]
	} else if ms, ok := c.methods[d.Recv.Name]; ok {
		s = ms[d.Name.Name]
	}
	if s == nil || s.decl != d {
		return
	}
	sc := newScope(globals)
	if s.self {
		self := d.Params[0].Name
		sc.vars["self"] = &binding{t: c.named[d.Recv.Name], decl: self}
		c.name(self, &Name{Decl: self, Type: c.named[d.Recv.Name]})
	}
	for _, p := range s.params {
		var id *ast.Ident
		for _, ap := range d.Params {
			if ap.Name.Name == p.Name {
				id = ap.Name
			}
		}
		sc.vars[p.Name] = &binding{t: p.Type, decl: id}
		c.name(id, &Name{Decl: id, Type: p.Type})
	}
	x := &ctx{kind: ctxFn, fn: s}
	if s.result == Unit {
		c.block(d.Body, sc, x, nil)
		return
	}
	last := lastExpr(d.Body)
	got := c.block(d.Body, sc, x, s.result)
	if last == nil {
		c.errorf(d.Name, "end the body with a value of type "+String(s.result), "`%s` returns %s, but its body ends without a value", s.name, article(s.result))
		return
	}
	if a := c.assign(s.result, got); !a.ok {
		c.mismatch(last, s.result, got, a, fmt.Sprintf("`%s` returns %s", s.name, article(s.result)),
			fmt.Sprintf("end the body with %s, or change the result type to `-> %s`", article(s.result), String(got)))
	}
}

func lastExpr(b *ast.Block) ast.Expr {
	if len(b.Stmts) == 0 {
		return nil
	}
	if es, ok := b.Stmts[len(b.Stmts)-1].(*ast.ExprStmt); ok {
		return es.X
	}
	return nil
}

// block checks a block in a new scope. want, if set, is the type expected
// of its last expression. The result is the block's type.
func (c *checker) block(b *ast.Block, sc *scope, x *ctx, want Type) Type {
	inner := newScope(sc)
	var t Type = Unit
	for i, s := range b.Stmts {
		if es, ok := s.(*ast.ExprStmt); ok && i == len(b.Stmts)-1 {
			t = c.expr(es.X, inner, x, want)
			continue
		}
		c.stmt(s, inner, x)
		t = Unit
	}
	return t
}

func (c *checker) stmt(s ast.Stmt, sc *scope, x *ctx) {
	switch s := s.(type) {
	case *ast.LetStmt:
		t := c.expr(s.Value, sc, x, nil)
		sc.vars[s.Name.Name] = &binding{t: t, mutable: s.Mutable, decl: s.Name}
		c.name(s.Name, &Name{Decl: s.Name, Type: t})
	case *ast.AssignStmt:
		b, ok := sc.lookup(s.Name.Name)
		var want Type
		if ok {
			want = b.t
			c.name(s.Name, &Name{Decl: b.decl, Type: b.t})
		}
		got := c.expr(s.Value, sc, x, want)
		if !ok || !b.mutable {
			return // the M1 check reports these
		}
		if s.Op != 0 {
			// `x += e` is `x = x + e`, but x's type is fixed by its declaration,
			// so a Float added to an Int var means the var should be a Float.
			if prune(b.t) == Int && prune(got) == Float {
				c.errorf(s, fmt.Sprintf("declare `%s` as a Float, like `var %s = 0.0`, or convert the value with `to_int(...)`", s.Name.Name, s.Name.Name),
					"`%s` holds an Int, so it cannot have a Float added to it", s.Name.Name)
				return
			}
			if prune(b.t) == Float && prune(got) == Int {
				c.errorf(s.Value, fmt.Sprintf("convert it with `to_float(%s)`; Fennic never converts numbers for you", c.text(s.Value)),
					"`%s` holds a Float, so it cannot have an Int added to it", s.Name.Name)
				return
			}
			c.arith(&ast.BinaryExpr{Span: s.Span, Op: token.Plus, Left: s.Name, Right: s.Value}, b.t, got)
			return
		}
		if a := c.assign(b.t, got); !a.ok {
			c.errorf(s.Value, fmt.Sprintf("keep `%s` %s, or give the %s its own name with `let`", s.Name.Name, article(b.t), String(got)),
				"`%s` holds %s, so it cannot be set to %s", s.Name.Name, article(b.t), article(got))
		}
	case *ast.AssertStmt:
		got := c.expr(s.Cond, sc, x, Bool)
		if a := c.assign(Bool, got); !a.ok {
			c.mismatch(s.Cond, Bool, got, a, "assert needs a Bool", "assert a comparison, like `assert x == 3`")
		}
	case *ast.ExprStmt:
		c.expr(s.X, sc, x, nil)
	case *ast.IndexAssignStmt:
		b, ok := sc.lookup(s.Name.Name)
		if ok {
			c.name(s.Name, &Name{Decl: b.decl, Type: b.t})
		}
		if !ok || !b.mutable {
			c.expr(s.Index, sc, x, nil)
			c.expr(s.Value, sc, x, nil)
			return // the M1 check reports these
		}
		m, isMap := prune(b.t).(*Map)
		if !isMap {
			c.expr(s.Index, sc, x, nil)
			c.expr(s.Value, sc, x, nil)
			if !isBad(b.t) {
				c.errorf(s.Name, "only a map entry can be set this way; to change a list, build a new one", "`%s` is %s, so `%s[...] = ...` cannot change it", s.Name.Name, article(b.t), s.Name.Name)
			}
			return
		}
		kt := c.expr(s.Index, sc, x, m.Key)
		if a := c.assign(m.Key, kt); !a.ok {
			c.mismatch(s.Index, m.Key, kt, a, "this map's keys are "+plural(m.Key)+", so the key must be "+article(m.Key), "pass "+article(m.Key))
		}
		vt := c.expr(s.Value, sc, x, m.Val)
		if a := c.assign(m.Val, vt); !a.ok {
			c.mismatch(s.Value, m.Val, vt, a, "this map's values are "+plural(m.Val)+", so the value must be "+article(m.Val), "pass "+article(m.Val))
		}
	case *ast.ForStmt:
		t := c.expr(s.In, sc, x, nil)
		var elem Type = bad
		switch l := prune(t).(type) {
		case *List:
			elem = l.Elem
		case *Chan:
			elem = l.Elem // until the channel is closed
		case *Var:
			elem = c.fresh()
			unify(l, &List{elem})
		case *Bad:
		default:
			c.mismatch(s.In, &List{c.fresh()}, t, assignment{}, "`for` goes over a List or a channel", "go over a List or a range, like `for n in 1..10`")
		}
		inner := newScope(sc)
		if s.Var.Name != "_" {
			inner.vars[s.Var.Name] = &binding{t: elem, decl: s.Var}
			c.name(s.Var, &Name{Decl: s.Var, Type: elem})
		}
		c.block(s.Body, inner, x, nil)
	}
}

func (s *sig) isMain() bool { return s.name == "main" && s.decl.Recv == nil }

// checkMain checks the signature of `fn main`: no arguments or
// `args: List[Str]`, and no result or an Int, the exit status.
func (c *checker) checkMain(s *sig) {
	d := s.decl
	if len(s.params) > 1 || (len(s.params) == 1 && !unify(s.params[0].Type, &List{Str})) {
		c.errorf(d.Name, "write `fn main()`, or `fn main(args: List[Str])` to receive the command-line arguments",
			"`main` takes no arguments, or the command-line arguments as a List[Str]")
	}
	if len(d.TypeParams) > 0 {
		c.errorf(d.Name, "remove the type parameters", "`main` cannot be generic")
	}
	if d.Result != nil && !unify(s.result, Int) {
		c.errorf(d.Result, "write `-> Int` to give the exit status, or remove the result type to exit with 0",
			"`main` returns nothing or an Int, the exit status, but this is %s", article(s.result))
	}
}

// constraintCheck is a type parameter's constraint, waiting for inference
// to decide the type it stands for.
type constraintCheck struct {
	t          Type
	constraint Type
	fn, tparam string
	at         ast.Node
}

// flushConstraints checks every constraint whose type is now known. With
// final set, it drops those that never became known.
func (c *checker) flushConstraints(final bool) {
	var keep []constraintCheck
	for _, k := range c.pending {
		if _, unknown := prune(k.t).(*Var); unknown {
			if !final {
				keep = append(keep, k)
			}
			continue
		}
		c.checkConstraint(k)
	}
	c.pending = keep
}

func (c *checker) checkConstraint(k constraintCheck) {
	t := prune(k.t)
	if k.constraint == Ord {
		if !ordered(t) {
			c.errorf(k.at, "only Int, Float and Str can be ordered; to order records, sort by a field: `sort_by(p -> p.x)`",
				"`%s` needs `%s: Ord`, but here `%s` is `%s`, which cannot be ordered", k.fn, k.tparam, k.tparam, String(t))
		}
		return
	}
	in := k.constraint.(*Iface)
	if a := c.satisfies(t, in); !a.ok {
		why := a.why
		if why == "" {
			why = "it is not " + Article(in.Name)
		}
		c.errorf(k.at, a.fix, "`%s` needs `%s: %s`, but here `%s` is `%s`: %s", k.fn, k.tparam, in.Name, k.tparam, String(t), why)
	}
}
