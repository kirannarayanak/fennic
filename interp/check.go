package interp

import (
	"fmt"

	"github.com/kirannarayanak/fennic/ast"
	"github.com/kirannarayanak/fennic/diag"
)

// Check finds the mistakes that are certain before the program runs:
// assigning to something that is not a `var`. It is not a type checker; that
// is M2.
func Check(file *ast.File, src string) []diag.Diagnostic {
	c := &checker{src: src}
	c.push()
	for _, it := range file.Items {
		switch d := it.(type) {
		case *ast.FnDecl:
			if d.Recv == nil {
				c.declare(d.Name.Name, &decl{kind: "a function"})
			}
		case *ast.RecordDecl:
			c.declare(d.Name.Name, &decl{kind: "a type"})
		case *ast.SumDecl:
			c.declare(d.Name.Name, &decl{kind: "a type"})
			for _, v := range d.Variants {
				c.declare(v.Name.Name, &decl{kind: "a variant"})
			}
		case *ast.InterfaceDecl:
			c.declare(d.Name.Name, &decl{kind: "an interface"})
		}
	}
	// Top-level statements first, in order, so function bodies see every
	// top-level binding.
	for _, it := range file.Items {
		if s, ok := it.(ast.Stmt); ok {
			c.stmt(s)
		}
	}
	for _, it := range file.Items {
		switch d := it.(type) {
		case *ast.FnDecl:
			c.push()
			for _, p := range d.Params {
				c.declare(p.Name.Name, &decl{kind: "a parameter"})
			}
			c.block(d.Body)
			c.pop()
		case *ast.TestDecl:
			c.block(d.Body)
		}
	}
	return c.diags
}

type decl struct {
	kind string       // for names that are not bindings: "a function", "a parameter"
	let  *ast.LetStmt // set for let and var
}

type checker struct {
	src    string
	scopes []map[string]*decl
	diags  []diag.Diagnostic
}

func (c *checker) push()                        { c.scopes = append(c.scopes, map[string]*decl{}) }
func (c *checker) pop()                         { c.scopes = c.scopes[:len(c.scopes)-1] }
func (c *checker) declare(name string, d *decl) { c.scopes[len(c.scopes)-1][name] = d }

func (c *checker) lookup(name string) *decl {
	for i := len(c.scopes) - 1; i >= 0; i-- {
		if d, ok := c.scopes[i][name]; ok {
			return d
		}
	}
	return nil
}

func (c *checker) report(n ast.Node, help, format string, args ...any) {
	c.diags = append(c.diags, diag.Diagnostic{Pos: n.Pos(), End: n.End(), Msg: fmt.Sprintf(format, args...), Help: help})
}

func (c *checker) block(b *ast.Block) {
	c.push()
	for _, s := range b.Stmts {
		c.stmt(s)
	}
	c.pop()
}

func (c *checker) stmt(s ast.Stmt) {
	switch s := s.(type) {
	case *ast.LetStmt:
		c.expr(s.Value)
		c.declare(s.Name.Name, &decl{let: s})
	case *ast.AssignStmt:
		c.expr(s.Value)
		c.assign(s.Name, "assign to")
	case *ast.IndexAssignStmt:
		c.expr(s.Index)
		c.expr(s.Value)
		c.assign(s.Name, "change an entry of")
	case *ast.ForStmt:
		c.expr(s.In)
		c.push()
		c.declare(s.Var.Name, &decl{kind: "a loop variable"})
		c.block(s.Body)
		c.pop()
	case *ast.AssertStmt:
		c.expr(s.Cond)
	case *ast.ExprStmt:
		c.expr(s.X)
	}
}

// assign checks that id names a `var`. verb says what is being done to it:
// "assign to", or "push onto".
func (c *checker) assign(id *ast.Ident, verb string) {
	name := id.Name
	d := c.lookup(name)
	switch {
	case d == nil:
		c.report(id, fmt.Sprintf("declare it first: `var %s = ...`", name), "`%s` is not defined", name)
	case d.let != nil && !d.let.Mutable:
		rest := c.src[d.let.Name.Pos().Offset:d.let.End().Offset]
		c.report(id, fmt.Sprintf("change line %d to `var %s` if `%s` needs to change", d.let.Pos().Line, rest, name),
			"cannot %s `%s` because it was declared with `let`", verb, name)
	case d.let == nil:
		c.report(id, fmt.Sprintf("make a changeable copy first: `var %s_ = %s`", name, name),
			"cannot %s `%s` because it is %s", verb, name, d.kind)
	}
}

func (c *checker) expr(e ast.Expr) {
	switch e := e.(type) {
	case *ast.InterpStr:
		for _, p := range e.Parts {
			c.expr(p)
		}
	case *ast.ListLit:
		for _, x := range e.Elems {
			c.expr(x)
		}
	case *ast.RecordLit:
		for _, f := range e.Fields {
			c.expr(f.Value)
		}
	case *ast.WithExpr:
		c.expr(e.X)
		for _, f := range e.Fields {
			c.expr(f.Value)
		}
	case *ast.UnaryExpr:
		c.expr(e.X)
	case *ast.BinaryExpr:
		c.expr(e.Left)
		c.expr(e.Right)
	case *ast.PipeExpr:
		c.expr(e.X)
		c.expr(e.Call)
	case *ast.CallExpr:
		c.expr(e.Fn)
		for _, a := range e.Args {
			c.expr(a)
		}
		// `xs.push(x)` changes xs, so xs must be a `var`.
		if f, ok := e.Fn.(*ast.FieldExpr); ok && f.Name.Name == "push" {
			if id, ok := f.X.(*ast.Ident); ok && c.lookup(id.Name) != nil {
				c.assign(id, "push onto")
			}
		}
	case *ast.IndexExpr:
		c.expr(e.X)
		c.expr(e.Index)
	case *ast.TupleLit:
		for _, x := range e.Elems {
			c.expr(x)
		}
	case *ast.MapLit:
		for _, en := range e.Entries {
			c.expr(en.Key)
			c.expr(en.Value)
		}
	case *ast.IfExpr:
		c.expr(e.Cond)
		c.block(e.Then)
		if e.Else != nil {
			c.expr(e.Else)
		}
	case *ast.FailExpr:
		c.expr(e.Msg)
	case *ast.SpawnExpr:
		c.expr(e.Fn)
	case *ast.FuncLit:
		c.push()
		for _, p := range e.Params {
			c.declare(p.Name.Name, &decl{kind: "a parameter"})
		}
		c.block(e.Body)
		c.pop()
	case *ast.FieldExpr:
		c.expr(e.X)
	case *ast.PropagateExpr:
		c.expr(e.X)
	case *ast.ElseExpr:
		c.expr(e.X)
		c.push()
		if e.ErrName != nil {
			c.declare(e.ErrName.Name, &decl{kind: "the error from `else`"})
		}
		c.expr(e.Fallback)
		c.pop()
	case *ast.LambdaExpr:
		c.push()
		for _, p := range e.Params {
			c.declare(p.Name, &decl{kind: "a parameter"})
		}
		c.expr(e.Body)
		c.pop()
	case *ast.Block:
		c.block(e)
	case *ast.MatchExpr:
		c.expr(e.X)
		for _, arm := range e.Arms {
			c.push()
			for _, b := range ast.PatternBinds(arm.Pattern) {
				c.declare(b.Name, &decl{kind: "a name bound by `match`"})
			}
			c.expr(arm.Body)
			c.pop()
		}
	case *ast.TogetherExpr:
		c.block(e.Body)
	}
}
