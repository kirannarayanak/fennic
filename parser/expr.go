package parser

import (
	"fmt"
	"strconv"

	"github.com/kirannarayanak/fennic/ast"
	"github.com/kirannarayanak/fennic/token"
)

// Precedence, loosest first:
//
//	or        x or fallback, x or err -> handler
//	??        right-associative
//	|>        x |> f(a) is f(x, a)
//	== !=
//	< <= > >=
//	+ -
//	* / %
//	-x
//	postfix   call, .name, ?.name, !, with { ... }

func (p *parser) parseExpr() ast.Expr { return p.parseOr() }

func (p *parser) parseOr() ast.Expr {
	start := p.cur().Pos
	x := p.parseCoalesce()
	for p.accept(token.Or) {
		or := &ast.OrExpr{X: x}
		if p.at(token.Ident) && p.peek(1).Kind == token.Arrow {
			or.ErrName = p.ident("")
			p.next() // ->
			or.Fallback = p.parseBody()
		} else {
			or.Fallback = p.parseCoalesce()
		}
		or.Span = p.span(start)
		x = or
	}
	return x
}

func (p *parser) parseCoalesce() ast.Expr {
	start := p.cur().Pos
	x := p.parsePipe()
	if p.accept(token.QQ) {
		r := p.parseCoalesce()
		return &ast.BinaryExpr{Span: p.span(start), Op: token.QQ, Left: x, Right: r}
	}
	return x
}

func (p *parser) parsePipe() ast.Expr {
	start := p.cur().Pos
	x := p.parseBinary(0)
	for p.at(token.Pipe) {
		p.next()
		rhs := p.parseCallee()
		call, ok := rhs.(*ast.CallExpr)
		if !ok {
			if _, bad := rhs.(*ast.BadExpr); !bad {
				p.errorAt(rhs.Pos(), rhs.End(), fmt.Sprintf("call it: `|> %s()`", ast.Dump(rhs)),
					"the right side of `|>` must be a call")
			}
			call = &ast.CallExpr{Span: ast.Span{From: rhs.Pos(), To: rhs.End()}, Fn: rhs}
		}
		x = &ast.PipeExpr{Span: p.span(start), X: x, Call: call}
		for p.accept(token.Bang) {
			x = &ast.PropagateExpr{Span: p.span(start), X: x}
		}
	}
	return x
}

// parseCallee parses the right side of `|>`: a name followed by field
// accesses and one call, with no `!` (that applies to the whole pipe).
func (p *parser) parseCallee() ast.Expr {
	start := p.cur().Pos
	if !p.at(token.Ident) {
		p.errorHere("write a call after `|>`, like `|> take(10)`", "expected a function call after `|>`, found %s", p.found())
		return &ast.BadExpr{Span: p.span(start)}
	}
	var x ast.Expr = p.ident("")
	for {
		switch {
		case p.at(token.Dot):
			p.next()
			x = &ast.FieldExpr{X: x, Name: p.ident("write a name after `.`"), Span: p.span(start)}
		case p.at(token.LParen):
			return p.parseCall(x, start)
		default:
			return x
		}
	}
}

var binaryLevels = [][]token.Kind{
	{token.Eq, token.NotEq},
	{token.Lt, token.LtEq, token.Gt, token.GtEq},
	{token.Plus, token.Minus},
	{token.Star, token.Slash, token.Percent},
}

func (p *parser) parseBinary(level int) ast.Expr {
	if level == len(binaryLevels) {
		return p.parseUnary()
	}
	start := p.cur().Pos
	x := p.parseBinary(level + 1)
	for {
		op := p.cur().Kind
		found := false
		for _, k := range binaryLevels[level] {
			if op == k {
				found = true
			}
		}
		if !found {
			return x
		}
		p.next()
		r := p.parseBinary(level + 1)
		x = &ast.BinaryExpr{Span: p.span(start), Op: op, Left: x, Right: r}
	}
}

func (p *parser) parseUnary() ast.Expr {
	if p.at(token.Minus) {
		start := p.next().Pos
		x := p.parseUnary()
		return &ast.UnaryExpr{Span: p.span(start), Op: token.Minus, X: x}
	}
	return p.parsePostfix()
}

func (p *parser) parsePostfix() ast.Expr {
	start := p.cur().Pos
	x := p.parsePrimary()
	for {
		switch {
		case p.at(token.LParen):
			x = p.parseCall(x, start)
		case p.at(token.Dot) || p.at(token.QDot):
			opt := p.next().Kind == token.QDot
			name := p.ident("write a field or method name after `.`")
			x = &ast.FieldExpr{Span: p.span(start), X: x, Name: name, Optional: opt}
		case p.at(token.Bang):
			p.next()
			x = &ast.PropagateExpr{Span: p.span(start), X: x}
		case p.at(token.With):
			p.next()
			if !p.at(token.LBrace) {
				p.errorHere("list the changed fields in braces: `u with { age: 32 }`", "expected `{` after `with`, found %s", p.found())
				return &ast.BadExpr{Span: p.span(start)}
			}
			x = &ast.WithExpr{X: x, Fields: p.parseFieldInits()}
			x.(*ast.WithExpr).Span = p.span(start)
		default:
			return x
		}
	}
}

func (p *parser) parseCall(fn ast.Expr, start token.Pos) ast.Expr {
	open := p.next() // (
	call := &ast.CallExpr{Fn: fn}
	call.Args = p.parseList(token.RParen, "write an argument")
	p.expectClose(open, token.RParen)
	call.Span = p.span(start)
	return call
}

// parseList parses comma-separated expressions up to (not including) the
// closer. A trailing comma is allowed.
func (p *parser) parseList(closer token.Kind, help string) []ast.Expr {
	saved := p.noRecord
	p.noRecord = false
	defer func() { p.noRecord = saved }()
	var xs []ast.Expr
	for !p.at(closer) && !p.at(token.EOF) {
		before := p.i
		xs = append(xs, p.parseExpr())
		if p.i == before {
			break
		}
		if !p.accept(token.Comma) {
			break
		}
	}
	return xs
}

// parseFieldInits parses `{ name: value, ... }` for record literals and `with`.
func (p *parser) parseFieldInits() []*ast.FieldInit {
	open := p.next() // {
	saved := p.noRecord
	p.noRecord = false
	defer func() { p.noRecord = saved }()
	var fs []*ast.FieldInit
	p.skipNewlines()
	for !p.at(token.RBrace) && !p.at(token.EOF) {
		before := p.i
		start := p.cur().Pos
		name := p.ident("write a field, like `name: value`")
		if p.i == before {
			break
		}
		p.expect(token.Colon, fmt.Sprintf("give the field a value: `%s: ...`", name.Name))
		v := p.parseExpr()
		fs = append(fs, &ast.FieldInit{Span: p.span(start), Name: name, Value: v})
		p.skipNewlines()
		if !p.accept(token.Comma) {
			break
		}
		p.skipNewlines()
	}
	p.skipNewlines()
	p.expectClose(open, token.RBrace)
	return fs
}

// parseBody parses what follows `->`: a block or a single expression.
func (p *parser) parseBody() ast.Expr {
	if p.at(token.LBrace) {
		return p.parseBlock()
	}
	return p.parseExpr()
}

func (p *parser) parsePrimary() ast.Expr {
	t := p.cur()
	start := t.Pos
	switch t.Kind {
	case token.Ident:
		if p.peek(1).Kind == token.Arrow {
			param := p.ident("")
			p.next() // ->
			body := p.parseBody()
			return &ast.LambdaExpr{Span: p.span(start), Params: []*ast.Ident{param}, Body: body}
		}
		if t.Text == "null" || t.Text == "nil" {
			p.errorHere("use `none` for a missing value, and give the type as `T?`", "there is no `%s` in Fennic", t.Text)
			p.next()
			return &ast.BadExpr{Span: p.span(start)}
		}
		id := p.ident("")
		if isTypeName(id.Name) && p.at(token.LBrace) && !p.noRecord {
			rl := &ast.RecordLit{Type: id, Fields: p.parseFieldInits()}
			rl.Span = p.span(start)
			return rl
		}
		return id
	case token.Int:
		p.next()
		v, err := strconv.ParseInt(t.Text, 10, 64)
		if err != nil {
			p.errorAt(t.Pos, t.End, "the largest Int is 9223372036854775807", "`%s` is too large for an Int", t.Text)
		}
		return &ast.IntLit{Span: p.span(start), Value: v}
	case token.Float:
		p.next()
		v, _ := strconv.ParseFloat(t.Text, 64)
		return &ast.FloatLit{Span: p.span(start), Value: v}
	case token.Str:
		p.next()
		return &ast.StrLit{Span: p.span(start), Value: t.Value}
	case token.StrHead:
		return p.parseInterp()
	case token.True, token.False:
		p.next()
		return &ast.BoolLit{Span: p.span(start), Value: t.Kind == token.True}
	case token.None:
		p.next()
		return &ast.NoneLit{Span: p.span(start)}
	case token.LParen:
		open := p.next()
		saved := p.noRecord
		p.noRecord = false
		x := p.parseExpr()
		p.noRecord = saved
		p.expectClose(open, token.RParen)
		return x
	case token.LBracket:
		open := p.next()
		elems := p.parseList(token.RBracket, "write a list element")
		p.expectClose(open, token.RBracket)
		return &ast.ListLit{Span: p.span(start), Elems: elems}
	case token.Match:
		return p.parseMatch()
	case token.Together:
		p.next()
		if !p.at(token.LBrace) {
			p.errorHere("write the tasks in braces: `together { ... }`", "expected `{` after `together`, found %s", p.found())
			return &ast.BadExpr{Span: p.span(start)}
		}
		body := p.parseBlock()
		return &ast.TogetherExpr{Span: p.span(start), Body: body}
	case token.LBrace:
		p.errorHere("a block can follow `->`, `together` or a declaration; to build a record, name its type: `User { ... }`",
			"unexpected `{`")
		return &ast.BadExpr{Span: p.span(start)}
	case token.Illegal:
		// The lexer has reported it.
		p.next()
		p.quiet = true
		return &ast.BadExpr{Span: p.span(start)}
	}
	p.errorHere("write a value or expression here", "expected an expression, found %s", p.found())
	return &ast.BadExpr{Span: ast.Span{From: start, To: start}}
}

func (p *parser) parseInterp() ast.Expr {
	start := p.cur().Pos
	saved := p.noRecord
	p.noRecord = false
	defer func() { p.noRecord = saved }()
	s := &ast.InterpStr{}
	piece := p.next() // StrHead
	for {
		s.Parts = append(s.Parts, &ast.StrLit{Span: ast.Span{From: piece.Pos, To: piece.End}, Value: piece.Value})
		if piece.Kind == token.StrTail {
			break
		}
		s.Parts = append(s.Parts, p.parseExpr())
		if !p.at(token.StrMid) && !p.at(token.StrTail) {
			p.errorHere("close the interpolation with `}`", "expected `}` to end the interpolation, found %s", p.found())
			// Skip to the rest of the string so the pieces stay paired.
			for !p.at(token.StrMid) && !p.at(token.StrTail) && !p.at(token.EOF) {
				p.next()
			}
			if p.at(token.EOF) {
				break
			}
		}
		piece = p.next()
	}
	s.Span = p.span(start)
	return s
}

func (p *parser) parseMatch() ast.Expr {
	start := p.next().Pos // match
	m := &ast.MatchExpr{}
	saved := p.noRecord
	p.noRecord = true
	m.X = p.parseExpr()
	p.noRecord = saved
	if !p.at(token.LBrace) {
		p.errorHere("write the arms in braces: `match s { Circle(r) -> ... }`", "expected `{` after the matched value, found %s", p.found())
		return &ast.BadExpr{Span: p.span(start)}
	}
	open := p.next()
	p.skipNewlines()
	for !p.at(token.RBrace) && !p.at(token.EOF) {
		before := p.i
		armStart := p.cur().Pos
		pat := &ast.VariantPattern{Name: p.ident("start each arm with a variant, like `Circle(r) -> ...`")}
		if p.at(token.LParen) {
			popen := p.next()
			for !p.at(token.RParen) && !p.at(token.EOF) {
				pat.Binds = append(pat.Binds, p.ident("bind each field to a name, like `Rect(w, h)`"))
				if !p.accept(token.Comma) {
					break
				}
			}
			p.expectClose(popen, token.RParen)
		} else {
			p.errorHere(fmt.Sprintf("name the fields to bind: `%s(...)`", pat.Name.Name),
				"expected `(` after `%s` in the pattern, found %s", pat.Name.Name, p.found())
		}
		pat.Span = p.span(armStart)
		p.expect(token.Arrow, "separate the pattern from its result with `->`")
		body := p.parseBody()
		m.Arms = append(m.Arms, &ast.MatchArm{Span: p.span(armStart), Pattern: pat, Body: body})
		p.endStatement()
		p.skipNewlines()
		p.ensureProgress(before)
	}
	p.expectClose(open, token.RBrace)
	m.Span = p.span(start)
	return m
}
