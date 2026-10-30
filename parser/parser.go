// Package parser builds the syntax tree from tokens.
//
// The parser never stops at the first error. It records a diagnostic, puts a
// Bad node in the tree, skips to the end of the statement and carries on.
// While it is skipping, further errors in the same statement are dropped so
// one mistake produces one message.
package parser

import (
	"fmt"
	"unicode"

	"fennic/ast"
	"fennic/diag"
	"fennic/lexer"
	"fennic/token"
)

type parser struct {
	toks     []token.Token
	i        int
	prevEnd  token.Pos
	diags    []diag.Diagnostic
	quiet    bool // an error was reported in this statement; drop the rest
	noRecord bool // `Name {` is not a record literal here (a match scrutinee)
}

// Parse lexes and parses src. The tree is always complete; problems are
// returned as diagnostics in source order of discovery.
func Parse(src string) (*ast.File, []diag.Diagnostic) {
	toks, lexDiags := lexer.Lex(src)
	p := &parser{toks: toks, diags: lexDiags}
	f := p.parseFile()
	return f, p.diags
}

// ---- token helpers ----

func (p *parser) cur() token.Token { return p.toks[p.i] }

func (p *parser) peek(n int) token.Token {
	if p.i+n < len(p.toks) {
		return p.toks[p.i+n]
	}
	return p.toks[len(p.toks)-1]
}

func (p *parser) at(k token.Kind) bool { return p.cur().Kind == k }

func (p *parser) next() token.Token {
	t := p.cur()
	if t.Kind != token.EOF {
		p.i++
	}
	p.prevEnd = t.End
	return t
}

func (p *parser) accept(k token.Kind) bool {
	if p.at(k) {
		p.next()
		return true
	}
	return false
}

func (p *parser) span(from token.Pos) ast.Span { return ast.Span{From: from, To: p.prevEnd} }

func (p *parser) skipNewlines() {
	for p.at(token.Newline) || p.at(token.Semi) {
		p.next()
	}
}

// describe names a token the way an error message should show it.
func describe(t token.Token) string {
	switch t.Kind {
	case token.Newline:
		return "the end of the line"
	case token.EOF:
		return "the end of the file"
	case token.Str, token.StrHead:
		return "a string"
	case token.Illegal:
		return fmt.Sprintf("`%s`", t.Text)
	}
	return fmt.Sprintf("`%s`", t.Text)
}

func (p *parser) errorAt(from, to token.Pos, help, format string, args ...any) {
	if p.quiet {
		return
	}
	p.quiet = true
	p.diags = append(p.diags, diag.Diagnostic{Pos: from, End: to, Msg: fmt.Sprintf(format, args...), Help: help})
}

// errorHere reports a problem with the current token.
func (p *parser) errorHere(help, format string, args ...any) {
	t := p.cur()
	end := t.End
	if t.Kind == token.Newline || t.Kind == token.EOF || p.onNewLine() {
		// Point just past the previous token: that is where something is missing.
		from := p.prevEnd
		end = from
		end.Col++
		p.errorAt(from, end, help, format, args...)
		return
	}
	p.errorAt(t.Pos, end, help, format, args...)
}

// onNewLine reports whether the current token starts a later line than the
// previous one. The lexer drops newlines inside brackets and after
// operators; when the source is broken, this still finds the line break.
func (p *parser) onNewLine() bool {
	return p.i > 0 && p.prevEnd.Line > 0 && p.cur().Pos.Line > p.prevEnd.Line
}

// found describes the current token for a "found ..." message. A token on a
// later line means the line ended without what was expected.
func (p *parser) found() string {
	if p.onNewLine() && !p.at(token.EOF) {
		return "the end of the line"
	}
	return describe(p.cur())
}

// expect consumes a token of kind k or reports what to write instead.
func (p *parser) expect(k token.Kind, help string) token.Token {
	if p.at(k) {
		return p.next()
	}
	p.errorHere(help, "expected `%s`, found %s", k, p.found())
	return token.Token{Kind: k, Pos: p.cur().Pos, End: p.cur().Pos}
}

// expectClose consumes the closer for open, or reports that open was never closed.
func (p *parser) expectClose(open token.Token, k token.Kind) {
	if p.accept(k) {
		return
	}
	p.errorHere(fmt.Sprintf("add `%s`", k), "expected `%s` to close the `%s` on line %d, found %s",
		k, open.Text, open.Pos.Line, p.found())
}

func (p *parser) ident(help string) *ast.Ident {
	if p.at(token.Ident) {
		t := p.next()
		return &ast.Ident{Span: ast.Span{From: t.Pos, To: t.End}, Name: t.Text}
	}
	p.errorHere(help, "expected a name, found %s", p.found())
	pos := p.cur().Pos
	return &ast.Ident{Span: ast.Span{From: pos, To: pos}, Name: "_"}
}

// sync skips to the end of the current statement: a newline, `;` or an
// unmatched `}` at this nesting level, or the end of the file. The `}` is
// left for the enclosing block.
func (p *parser) sync() {
	depth := 0
	for !p.at(token.EOF) {
		if depth == 0 && (startsStatement(p.cur().Kind) || p.onNewLine()) {
			return
		}
		switch p.cur().Kind {
		case token.LParen, token.LBracket, token.LBrace:
			depth++
		case token.RParen, token.RBracket:
			if depth > 0 {
				depth--
			}
		case token.RBrace:
			if depth == 0 {
				return
			}
			depth--
		case token.Newline, token.Semi:
			if depth == 0 {
				return
			}
		}
		p.next()
	}
}

// endStatement requires a statement to be followed by a newline, `;`, the
// closing `}` of the block or the end of the file. After an error it skips
// the rest of the statement and resumes reporting.
func (p *parser) endStatement() {
	switch p.cur().Kind {
	case token.Newline, token.Semi, token.RBrace, token.EOF:
	default:
		if p.onNewLine() {
			break
		}
		p.errorHere("put each statement on its own line", "expected the end of the statement, found %s", p.found())
	}
	if p.quiet {
		p.sync()
		p.quiet = false
	}
}

// ensureProgress skips one token when a loop iteration consumed nothing,
// so recovery can never spin in place.
func (p *parser) ensureProgress(before int) {
	if p.i == before && !p.at(token.EOF) {
		p.next()
		p.quiet = false
	}
}

func startsStatement(k token.Kind) bool {
	switch k {
	case token.Let, token.Var, token.Fn, token.Type, token.Interface, token.Test, token.Assert:
		return true
	}
	return false
}

func isTypeName(name string) bool {
	for _, r := range name {
		return unicode.IsUpper(r)
	}
	return false
}

// ---- file and declarations ----

func (p *parser) parseFile() *ast.File {
	start := p.cur().Pos
	f := &ast.File{}
	p.skipNewlines()
	for !p.at(token.EOF) {
		if p.at(token.RBrace) {
			p.errorHere("remove it, or add the `{` it was meant to close", "unexpected `}`")
			p.next()
			p.quiet = false
			p.skipNewlines()
			continue
		}
		before := p.i
		f.Items = append(f.Items, p.parseItem())
		p.endStatement()
		p.skipNewlines()
		p.ensureProgress(before)
	}
	f.Span = ast.Span{From: start, To: p.cur().End}
	return f
}

func (p *parser) parseItem() ast.Item {
	switch p.cur().Kind {
	case token.Fn:
		return p.parseFn()
	case token.Type:
		return p.parseTypeDecl()
	case token.Interface:
		return p.parseInterface()
	case token.Test:
		return p.parseTest()
	}
	return p.parseStmt()
}

func (p *parser) parseFn() ast.Item {
	start := p.next().Pos // fn
	fd := &ast.FnDecl{}
	name := p.ident("write the function's name after `fn`, like `fn greet(who: Str) -> Str { ... }`")
	if p.accept(token.Dot) {
		fd.Recv = name
		name = p.ident("write the method's name after the dot, like `fn Console.write(self, line: Str)`")
	}
	fd.Name = name
	if p.at(token.LBracket) {
		open := p.next()
		for !p.at(token.RBracket) && !p.at(token.EOF) {
			tpStart := p.cur().Pos
			tp := &ast.TypeParam{Name: p.ident("write a type parameter name, like `[T]` or `[T: Ord]`")}
			if p.accept(token.Colon) {
				tp.Constraint = p.parseType()
			}
			tp.Span = p.span(tpStart)
			fd.TypeParams = append(fd.TypeParams, tp)
			if !p.accept(token.Comma) {
				break
			}
		}
		p.expectClose(open, token.RBracket)
	}
	fd.Params = p.parseParams()
	if p.accept(token.Arrow) {
		fd.Result = p.parseType()
	}
	if !p.at(token.LBrace) {
		p.errorHere("write the body in braces: `{ ... }`", "expected the function body, found %s", p.found())
		fd.Body = &ast.Block{Span: p.span(p.cur().Pos)}
	} else {
		fd.Body = p.parseBlock()
	}
	fd.Span = p.span(start)
	return fd
}

func (p *parser) parseParams() []*ast.Param {
	if !p.at(token.LParen) {
		p.errorHere("write the parameters in parentheses, like `(who: Str)`, or `()` for none",
			"expected `(`, found %s", p.found())
		return nil
	}
	open := p.next()
	var params []*ast.Param
	for !p.at(token.RParen) && !p.at(token.EOF) {
		start := p.cur().Pos
		name := p.ident("write a parameter, like `who: Str`")
		prm := &ast.Param{Name: name}
		if name.Name == "self" && !p.at(token.Colon) {
			// self takes the receiver's type
		} else if p.accept(token.Colon) {
			prm.Type = p.parseType()
		} else {
			p.errorAt(name.Pos(), name.End(),
				fmt.Sprintf("give it a type, like `%s: Str`", name.Name),
				"parameter `%s` needs a type", name.Name)
		}
		prm.Span = p.span(start)
		params = append(params, prm)
		if !p.accept(token.Comma) {
			break
		}
	}
	p.expectClose(open, token.RParen)
	return params
}

func (p *parser) parseType() ast.TypeExpr {
	start := p.cur().Pos
	var t ast.TypeExpr
	if !p.at(token.Ident) {
		p.errorHere("write a type, like `Int`, `Str?` or `List[T]`", "expected a type, found %s", p.found())
		return &ast.NamedType{Span: p.span(start), Name: &ast.Ident{Name: "_"}}
	}
	nt := &ast.NamedType{Name: p.ident("")}
	if p.at(token.LBracket) {
		open := p.next()
		for !p.at(token.RBracket) && !p.at(token.EOF) {
			nt.Args = append(nt.Args, p.parseType())
			if !p.accept(token.Comma) {
				break
			}
		}
		p.expectClose(open, token.RBracket)
	}
	nt.Span = p.span(start)
	t = nt
	for {
		switch {
		case p.accept(token.Question):
			t = &ast.OptionalType{Span: p.span(start), Elem: t}
		case p.accept(token.Bang):
			t = &ast.FailableType{Span: p.span(start), Elem: t}
		default:
			return t
		}
	}
}

func (p *parser) parseTypeDecl() ast.Item {
	start := p.next().Pos // type
	name := p.ident("write the type's name after `type`, like `type User { ... }`")
	switch {
	case p.at(token.LBrace):
		open := p.next()
		rd := &ast.RecordDecl{Name: name}
		p.skipNewlines()
		for !p.at(token.RBrace) && !p.at(token.EOF) {
			before := p.i
			fstart := p.cur().Pos
			fname := p.ident("write a field, like `name: Str`")
			p.expect(token.Colon, fmt.Sprintf("give the field a type, like `%s: Str`", fname.Name))
			ftype := p.parseType()
			rd.Fields = append(rd.Fields, &ast.FieldDecl{Span: p.span(fstart), Name: fname, Type: ftype})
			if p.at(token.Comma) {
				p.errorHere("put each field on its own line, without commas", "unexpected `,` between fields")
				p.next()
			}
			p.endStatement()
			p.skipNewlines()
			p.ensureProgress(before)
		}
		p.expectClose(open, token.RBrace)
		rd.Span = p.span(start)
		return rd
	case p.at(token.Assign):
		p.next()
		sd := &ast.SumDecl{Name: name}
		p.accept(token.Bar)
		for {
			vstart := p.cur().Pos
			v := &ast.Variant{Name: p.ident("write a variant, like `| Circle(radius: Float)`")}
			if !p.at(token.LParen) {
				p.errorHere("give the variant its fields in parentheses, like `Circle(radius: Float)`",
					"expected `(` after variant `%s`, found %s", v.Name.Name, p.found())
			} else {
				open := p.next()
				for !p.at(token.RParen) && !p.at(token.EOF) {
					fstart := p.cur().Pos
					fname := p.ident("write a field, like `radius: Float`")
					p.expect(token.Colon, fmt.Sprintf("give the field a type, like `%s: Float`", fname.Name))
					ftype := p.parseType()
					v.Fields = append(v.Fields, &ast.FieldDecl{Span: p.span(fstart), Name: fname, Type: ftype})
					if !p.accept(token.Comma) {
						break
					}
				}
				p.expectClose(open, token.RParen)
			}
			v.Span = p.span(vstart)
			sd.Variants = append(sd.Variants, v)
			if !p.accept(token.Bar) {
				break
			}
		}
		sd.Span = p.span(start)
		return sd
	}
	p.errorHere("write `{` to declare a record, or `=` to declare variants",
		"expected `{` or `=` after `type %s`, found %s", name.Name, p.found())
	return &ast.BadStmt{Span: p.span(start)}
}

func (p *parser) parseInterface() ast.Item {
	start := p.next().Pos // interface
	id := &ast.InterfaceDecl{Name: p.ident("write the interface's name, like `interface Sink { ... }`")}
	if !p.at(token.LBrace) {
		p.errorHere("write the methods in braces: `interface Sink { fn write(self, line: Str) }`",
			"expected `{`, found %s", p.found())
		return &ast.BadStmt{Span: p.span(start)}
	}
	open := p.next()
	p.skipNewlines()
	for !p.at(token.RBrace) && !p.at(token.EOF) {
		before := p.i
		mstart := p.cur().Pos
		p.expect(token.Fn, "each interface member is a method signature, like `fn write(self, line: Str)`")
		m := &ast.MethodSig{Name: p.ident("write the method's name after `fn`")}
		m.Params = p.parseParams()
		if p.accept(token.Arrow) {
			m.Result = p.parseType()
		}
		if p.at(token.LBrace) {
			p.errorHere("remove the body; define the method as `fn Type.name(self, ...) { ... }` at the top level",
				"an interface lists methods without bodies")
		}
		m.Span = p.span(mstart)
		id.Methods = append(id.Methods, m)
		p.endStatement()
		p.skipNewlines()
		p.ensureProgress(before)
	}
	p.expectClose(open, token.RBrace)
	id.Span = p.span(start)
	return id
}

func (p *parser) parseTest() ast.Item {
	start := p.next().Pos // test
	td := &ast.TestDecl{}
	if p.at(token.Str) {
		td.Name = p.next().Value
	} else {
		p.errorHere(`name the test in quotes: test "adds numbers" { ... }`, "expected the test's name, found %s", p.found())
	}
	if !p.at(token.LBrace) {
		p.errorHere("write the test body in braces: `{ ... }`", "expected `{`, found %s", p.found())
		return &ast.BadStmt{Span: p.span(start)}
	}
	td.Body = p.parseBlock()
	td.Span = p.span(start)
	return td
}
