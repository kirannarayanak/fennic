package parser

import (
	"fennic/ast"
	"fennic/token"
)

func (p *parser) parseBlock() *ast.Block {
	open := p.next() // {
	// Whatever went wrong before this brace, the statements inside are new
	// statements and get their own diagnostics.
	p.quiet = false
	saved := p.noRecord
	p.noRecord = false
	defer func() { p.noRecord = saved }()

	b := &ast.Block{}
	p.skipNewlines()
	for !p.at(token.RBrace) && !p.at(token.EOF) {
		if p.at(token.Fn) || p.at(token.Type) || p.at(token.Interface) || p.at(token.Test) {
			t := p.cur()
			p.errorAt(t.Pos, t.End, "move it to the top level of the file", "`%s` declarations belong at the top level, not inside a block", t.Text)
			p.parseItem() // parse it so its body does not derail the block, then drop it
			b.Stmts = append(b.Stmts, &ast.BadStmt{Span: p.span(t.Pos)})
			p.endStatement()
			p.skipNewlines()
			continue
		}
		before := p.i
		b.Stmts = append(b.Stmts, p.parseStmt())
		p.endStatement()
		p.skipNewlines()
		p.ensureProgress(before)
	}
	p.expectClose(open, token.RBrace)
	b.Span = p.span(open.Pos)
	return b
}

func (p *parser) parseStmt() ast.Stmt {
	start := p.cur().Pos
	switch t := p.cur(); {
	case t.Kind == token.Let || t.Kind == token.Var:
		p.next()
		s := &ast.LetStmt{Mutable: t.Kind == token.Var}
		s.Name = p.ident("write a name, like `" + t.Text + " count = 0`")
		p.expect(token.Assign, "give it a value: `"+t.Text+" "+s.Name.Name+" = ...`")
		s.Value = p.parseExpr()
		s.Span = p.span(start)
		return s
	case t.Kind == token.Assert:
		p.next()
		s := &ast.AssertStmt{Cond: p.parseExpr()}
		s.Span = p.span(start)
		return s
	case t.Kind == token.Ident && p.peek(1).Kind == token.Assign:
		name := p.ident("")
		p.next() // =
		s := &ast.AssignStmt{Name: name, Value: p.parseExpr()}
		s.Span = p.span(start)
		return s
	case t.Kind == token.Ident && t.Text == "return":
		p.errorAt(t.Pos, t.End, "the last expression of a block is its value; remove `return`", "Fennic has no `return`")
		p.next()
		p.quiet = false // the expression after it is still worth checking
		x := p.parseExpr()
		return &ast.ExprStmt{Span: p.span(start), X: x}
	}
	x := p.parseExpr()
	return &ast.ExprStmt{Span: p.span(start), X: x}
}
