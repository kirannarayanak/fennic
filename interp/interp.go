package interp

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/kirannarayanak/fennic/ast"
	"github.com/kirannarayanak/fennic/diag"
	"github.com/kirannarayanak/fennic/types"
)

// Interp holds one loaded program.
type Interp struct {
	file    *ast.File
	src     string
	dir     string // fs.read paths resolve against this
	globals *Env

	records  map[string]*RecordType
	variants map[string]*VariantType
	methods  map[string]map[string]*Func // type name -> method name -> method

	targets map[ast.Node]types.Type // what each json.parse or csv.read call builds

	inMain bool // running `fn main`, for the wording of errors

	outMu sync.Mutex
	out   io.Writer
	errw  io.Writer
}

// RuntimeError is a mistake found while running: a wrong kind of value, a
// missing field. It stops the program. In M2 most of these become type errors.
type RuntimeError struct{ Diag diag.Diagnostic }

func (e *RuntimeError) Error() string { return e.Diag.Msg }

// propagation carries a failure from `!` up to the nearest function, lambda,
// `together` or test boundary.
type propagation struct {
	failure *Failure
	at      ast.Node // the *ast.PropagateExpr or *ast.FailExpr it came from
}

// span is the source to underline for a failure nobody handled: the value
// `!` was applied to, or the whole `fail`.
func (p *propagation) span() ast.Node {
	if pe, ok := p.at.(*ast.PropagateExpr); ok {
		return pe.X
	}
	return p.at
}

// assertion is a failed `assert`.
type assertion struct {
	stmt        *ast.AssertStmt
	left, right Value
	compared    bool // the condition was `a == b`; left and right are set
}

// task is what a running piece of code knows about its concurrency scope.
type task struct {
	ctx      context.Context
	cancel   context.CancelFunc // cancels the enclosing `together` scope
	together bool               // inside a `together` block, so map_par may spawn
}

// callSite is what a builtin needs to report errors and to spawn tasks.
type callSite struct {
	node ast.Node
	t    *task
}

func (in *Interp) fail(n ast.Node, help, format string, args ...any) {
	panic(&RuntimeError{diag.Diagnostic{Pos: n.Pos(), End: n.End(), Msg: fmt.Sprintf(format, args...), Help: help}})
}

// New prepares a program to run: it registers every type, function and
// method, so top-level code can use them before their declaration.
func New(file *ast.File, src, dir string, out, errw io.Writer) *Interp {
	in := &Interp{
		file: file, src: src, dir: dir, out: out, errw: errw,
		records:  map[string]*RecordType{},
		variants: map[string]*VariantType{},
		methods:  map[string]map[string]*Func{},
	}
	in.globals = NewEnv(in.prelude())
	for _, it := range file.Items {
		switch d := it.(type) {
		case *ast.FnDecl:
			f := &Func{Decl: d}
			if d.Recv == nil {
				in.globals.Define(d.Name.Name, f, false)
				continue
			}
			ms := in.methods[d.Recv.Name]
			if ms == nil {
				ms = map[string]*Func{}
				in.methods[d.Recv.Name] = ms
			}
			ms[d.Name.Name] = f
		case *ast.RecordDecl:
			rt := &RecordType{Name: d.Name.Name}
			for _, f := range d.Fields {
				rt.Fields = append(rt.Fields, f.Name.Name)
			}
			in.records[rt.Name] = rt
			in.globals.Define(rt.Name, &TypeRef{Name: rt.Name}, false)
		case *ast.SumDecl:
			in.globals.Define(d.Name.Name, &TypeRef{Name: d.Name.Name}, false)
			for _, v := range d.Variants {
				vt := &VariantType{Sum: d.Name.Name, Name: v.Name.Name}
				for _, f := range v.Fields {
					vt.Fields = append(vt.Fields, f.Name.Name)
				}
				in.variants[vt.Name] = vt
				if len(vt.Fields) == 0 {
					in.globals.Define(vt.Name, &Variant{Type: vt}, false) // a variant with no fields is a value
					continue
				}
				in.globals.Define(vt.Name, in.constructor(vt), false)
			}
		case *ast.InterfaceDecl:
			in.globals.Define(d.Name.Name, &TypeRef{Name: d.Name.Name}, false)
		}
	}
	return in
}

// SetTargets tells json.parse and csv.read what each call should build, as
// decided by the type checker. Without it json.parse builds untyped records.
func (in *Interp) SetTargets(t map[ast.Node]types.Type) { in.targets = t }

func (in *Interp) constructor(vt *VariantType) *Builtin {
	return &Builtin{Name: vt.Name, Fn: func(in *Interp, call *callSite, args []Value) Value {
		if len(args) != len(vt.Fields) {
			in.fail(call.node, fmt.Sprintf("write `%s(%s)`", vt.Name, joinNames(vt.Fields)),
				"`%s` takes %d %s, but was given %d", vt.Name, len(vt.Fields), plural(len(vt.Fields), "value", "values"), len(args))
		}
		return &Variant{Type: vt, Fields: args}
	}}
}

// Run executes the top-level statements in order. A failure that reaches the
// top level through `!` becomes a RuntimeError.
func (in *Interp) Run() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = in.toError(r)
		}
	}()
	t := &task{ctx: context.Background()}
	for _, it := range in.file.Items {
		if s, ok := it.(ast.Stmt); ok {
			in.evalStmt(s, in.globals, t)
		}
	}
	return nil
}

// HasMain reports whether the program declares `fn main`.
func (in *Interp) HasMain() bool {
	f, ok := in.globals.vars["main"]
	if !ok {
		return false
	}
	fn, isFunc := f.val.(*Func)
	return isFunc && fn.Decl.Recv == nil
}

// Main runs `fn main`, after Run has run the top level. args are the
// command-line arguments after the file name, passed if main takes them.
func (in *Interp) Main(args []string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = in.toError(r)
		}
	}()
	in.inMain = true
	d := in.globals.vars["main"].val.(*Func).Decl
	env := NewEnv(in.globals)
	if len(d.Params) == 1 {
		list := &List{}
		for _, a := range args {
			list.Elems = append(list.Elems, a)
		}
		env.Define(d.Params[0].Name.Name, list, false)
	}
	in.evalBlock(d.Body, env, &task{ctx: context.Background()})
	return nil
}

// toError turns a panic that escaped the program into the error to report.
func (in *Interp) toError(r any) error {
	switch r := r.(type) {
	case *RuntimeError:
		return r
	case *propagation:
		x := r.span()
		if _, isFail := r.at.(*ast.FailExpr); isFail {
			return &RuntimeError{diag.Diagnostic{
				Pos: x.Pos(), End: x.End(), Msg: r.failure.Msg,
				Help: "nothing handles this failure, so the program stops; call the code that fails with `else` to handle it",
			}}
		}
		text := in.src[x.Pos().Offset:x.End().Offset]
		fallback := r.failure.Fallback
		if fallback == "" {
			fallback = "<fallback value>"
		}
		where := "top-level code"
		if in.inMain {
			where = "`main`"
		}
		return &RuntimeError{diag.Diagnostic{
			Pos: x.Pos(), End: x.End(), Msg: r.failure.Msg,
			Help: fmt.Sprintf("`!` passes a failure to the caller, but %s has no caller. Handle it here with `else`, for example `%s else %s`", where, text, fallback),
		}}
	case *assertion:
		return &RuntimeError{diag.Diagnostic{
			Pos: r.stmt.Cond.Pos(), End: r.stmt.Cond.End(), Msg: in.assertMessage(r),
			Help: "outside a `test` block a failed assert stops the program",
		}}
	}
	panic(r)
}

func (in *Interp) assertMessage(a *assertion) string {
	if a.compared {
		return fmt.Sprintf("assertion failed: left is %s, right is %s", show(a.left), show(a.right))
	}
	return "assertion failed: the condition is false"
}

// show renders a value inside a message, with Strs quoted so they are visible.
func show(v Value) string {
	if s, ok := v.(string); ok {
		var b strings.Builder
		quote(&b, s)
		return b.String()
	}
	return Display(v)
}

func (in *Interp) print(w io.Writer, s string) {
	in.outMu.Lock()
	defer in.outMu.Unlock()
	io.WriteString(w, s)
}

// ---- calls ----

func (in *Interp) call(fn Value, args []Value, site *callSite) Value {
	switch f := fn.(type) {
	case *Builtin:
		return f.Fn(in, site, args)
	case *Closure:
		return in.callClosure(f, args, site)
	case *Func:
		if f.Decl.HasSelf() {
			in.fail(site.node, fmt.Sprintf("call it on a value, like `x.%s(...)`", f.Decl.Name.Name),
				"`%s.%s` is a method and needs a value to work on", f.Decl.Recv.Name, f.Decl.Name.Name)
		}
		return in.callFunc(f, nil, args, site)
	case *Bound:
		return in.callFunc(f.Func, f.Self, args, site)
	case *TypeRef:
		if _, ok := in.records[f.Name]; ok {
			in.fail(site.node, fmt.Sprintf("build a record with braces: `%s { ... }`", f.Name), "`%s` is a record type, not a function", f.Name)
		}
		in.fail(site.node, "", "`%s` is a type, not a function", f.Name)
	}
	in.fail(site.node, "", "this is a %s, not a function, so it cannot be called", kindName(fn))
	return nil
}

func (in *Interp) callFunc(f *Func, self Value, args []Value, site *callSite) (result Value) {
	d := f.Decl
	params := d.Params
	env := NewEnv(in.globals)
	if d.HasSelf() {
		env.Define("self", self, false)
		params = params[1:]
	}
	if len(args) != len(params) {
		in.fail(site.node, "write `"+callShape(d)+"`",
			"`%s` takes %d %s, but was given %d", d.Name.Name, len(params), plural(len(params), "argument", "arguments"), len(args))
	}
	for i, p := range params {
		env.Define(p.Name.Name, args[i], false)
	}
	defer func() {
		if r := recover(); r != nil {
			p, ok := r.(*propagation)
			if !ok {
				panic(r)
			}
			// The failure is the function's result. Its fallback hint no
			// longer fits: the caller expects this function's type.
			result = &Failure{Msg: p.failure.Msg, cancelled: p.failure.cancelled}
		}
	}()
	v := in.evalBlock(d.Body, env, site.t)
	if d.Result == nil {
		return Unit // no result type: the function is called for its effect
	}
	return v
}

func (in *Interp) callClosure(c *Closure, args []Value, site *callSite) (result Value) {
	if len(args) != len(c.Params) {
		in.fail(site.node, "", "this function takes %d %s, but was given %d",
			len(c.Params), plural(len(c.Params), "argument", "arguments"), len(args))
	}
	env := NewEnv(c.Env)
	for i, p := range c.Params {
		env.Define(p, args[i], false)
	}
	defer func() {
		if r := recover(); r != nil {
			p, ok := r.(*propagation)
			if !ok {
				panic(r)
			}
			result = p.failure
		}
	}()
	v := in.eval(c.Body, env, site.t)
	if c.NoResult {
		return Unit
	}
	return v
}

func callShape(d *ast.FnDecl) string {
	var names []string
	for _, p := range d.Params {
		if p.Type != nil {
			names = append(names, p.Name.Name)
		}
	}
	prefix := d.Name.Name
	if d.Recv != nil && !d.HasSelf() {
		prefix = d.Recv.Name + "." + prefix
	}
	return prefix + "(" + joinNames(names) + ")"
}
