// Package lexer turns Fennic source text into tokens.
//
// Newlines end statements, so the lexer emits Newline tokens, with three
// exceptions that let an expression span lines:
//   - inside ( ) and [ ], newlines are dropped;
//   - after a token that cannot end an expression (an operator, a comma, an
//     opening bracket), the newline is dropped;
//   - before a line that starts with |>, | or else, the newline is dropped.
//
// Runs of blank lines and comment-only lines produce at most one Newline.
//
// The lexer never stops at an error. It records a diagnostic, skips the bad
// input and carries on, so the parser always gets a complete token stream.
package lexer

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kirannarayanak/fennic/diag"
	"github.com/kirannarayanak/fennic/token"
)

type frameKind int

const (
	frameParen frameKind = iota
	frameBracket
	frameBrace
	frameInterp // the { } of an interpolation inside a string
)

type frame struct {
	kind frameKind
	pos  token.Pos // where the opening delimiter (or the string's quote) is
}

type lexer struct {
	src   string
	off   int
	line  int
	col   int
	toks  []token.Token
	diags []diag.Diagnostic
	stack []frame

	pendingNL  bool
	pendingPos token.Pos
}

// Lex returns every token in src, ending with EOF, and every problem found.
func Lex(src string) ([]token.Token, []diag.Diagnostic) {
	l := &lexer{src: src, line: 1, col: 1}
	l.run()
	return l.toks, l.diags
}

func (l *lexer) pos() token.Pos { return token.Pos{Line: l.line, Col: l.col, Offset: l.off} }

func (l *lexer) peek() rune {
	if l.off >= len(l.src) {
		return -1
	}
	r, _ := utf8.DecodeRuneInString(l.src[l.off:])
	return r
}

func (l *lexer) peekAt(n int) rune {
	off := l.off
	for i := 0; i < n; i++ {
		if off >= len(l.src) {
			return -1
		}
		_, size := utf8.DecodeRuneInString(l.src[off:])
		off += size
	}
	if off >= len(l.src) {
		return -1
	}
	r, _ := utf8.DecodeRuneInString(l.src[off:])
	return r
}

func (l *lexer) advance() rune {
	r, size := utf8.DecodeRuneInString(l.src[l.off:])
	l.off += size
	if r == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return r
}

func (l *lexer) errorf(start, end token.Pos, help, format string, args ...any) {
	l.diags = append(l.diags, diag.Diagnostic{Pos: start, End: end, Msg: fmt.Sprintf(format, args...), Help: help})
}

func (l *lexer) top() (frame, bool) {
	if len(l.stack) == 0 {
		return frame{}, false
	}
	return l.stack[len(l.stack)-1], true
}

func (l *lexer) push(k frameKind, p token.Pos) { l.stack = append(l.stack, frame{k, p}) }

// pop removes frames down to and including the nearest frame of kind k,
// without crossing an interpolation. Mismatched brackets are the parser's to
// report, so a closer with no matching opener is left alone here.
func (l *lexer) pop(k frameKind) {
	for i := len(l.stack) - 1; i >= 0; i-- {
		if l.stack[i].kind == k {
			l.stack = l.stack[:i]
			return
		}
		if l.stack[i].kind == frameInterp {
			return
		}
	}
}

// continues reports whether a newline after a token of kind k is dropped
// because the expression is plainly not finished.
func continues(k token.Kind) bool {
	switch k {
	case token.Comma, token.Dot, token.QDot, token.Arrow, token.Pipe, token.Bar,
		token.QQ, token.Assign, token.Eq, token.NotEq, token.Lt, token.LtEq,
		token.Gt, token.GtEq, token.Plus, token.Minus, token.Star, token.Slash,
		token.Percent, token.Or, token.And, token.Not, token.Else, token.With, token.Colon,
		token.DotDot, token.PlusEq, token.In,
		token.LParen, token.LBracket, token.LBrace, token.Newline:
		return true
	}
	return false
}

func (l *lexer) emit(t token.Token) {
	if l.pendingNL {
		l.pendingNL = false
		prevContinues := len(l.toks) == 0 || continues(l.toks[len(l.toks)-1].Kind)
		nextContinues := t.Kind == token.Pipe || t.Kind == token.Bar || t.Kind == token.Else
		if !prevContinues && !nextContinues && t.Kind != token.EOF {
			end := l.pendingPos
			end.Col++
			end.Offset++
			l.toks = append(l.toks, token.Token{Kind: token.Newline, Text: "\n", Pos: l.pendingPos, End: end})
		}
	}
	l.toks = append(l.toks, t)
}

func (l *lexer) tok(k token.Kind, start token.Pos) {
	l.emit(token.Token{Kind: k, Text: l.src[start.Offset:l.off], Pos: start, End: l.pos()})
}

func (l *lexer) run() {
	for {
		l.skipSpaceAndComments()
		if l.off >= len(l.src) {
			l.closeOpenStrings()
			l.emit(token.Token{Kind: token.EOF, Pos: l.pos(), End: l.pos()})
			return
		}
		start := l.pos()
		r := l.peek()
		switch {
		case r == '\n':
			l.newline()
		case r == '"':
			l.advance()
			l.lexString(start, true)
		case r == '`':
			l.lexRawString(start)
		case r == '_' || unicode.IsLetter(r):
			l.lexIdent(start)
		case r >= '0' && r <= '9':
			l.lexNumber(start)
		default:
			l.lexPunct(start, r)
		}
	}
}

func (l *lexer) skipSpaceAndComments() {
	for l.off < len(l.src) {
		switch r := l.peek(); {
		case r == ' ' || r == '\t' || r == '\r':
			l.advance()
		case r == '/' && l.peekAt(1) == '/':
			for l.off < len(l.src) && l.peek() != '\n' {
				l.advance()
			}
		default:
			return
		}
	}
}

func (l *lexer) newline() {
	l.closeOpenStrings()
	p := l.pos()
	l.advance()
	if f, ok := l.top(); ok && (f.kind == frameParen || f.kind == frameBracket) {
		return
	}
	if !l.pendingNL {
		l.pendingNL = true
		l.pendingPos = p
	}
}

func (l *lexer) lexIdent(start token.Pos) {
	for l.off < len(l.src) {
		r := l.peek()
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			break
		}
		l.advance()
	}
	text := l.src[start.Offset:l.off]
	kind := token.Ident
	if k, ok := token.Keywords[text]; ok {
		kind = k
	}
	l.tok(kind, start)
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func (l *lexer) lexNumber(start token.Pos) {
	for isDigit(l.peek()) {
		l.advance()
	}
	kind := token.Int
	if l.peek() == '.' && isDigit(l.peekAt(1)) {
		kind = token.Float
		l.advance()
		for isDigit(l.peek()) {
			l.advance()
		}
	}
	if r := l.peek(); r == '_' || unicode.IsLetter(r) {
		for r := l.peek(); r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r); r = l.peek() {
			l.advance()
		}
		text := l.src[start.Offset:l.off]
		l.errorf(start, l.pos(), "names cannot start with a digit; put a space between a number and a name",
			"`%s` is not a number or a name", text)
		l.emit(token.Token{Kind: token.Illegal, Text: text, Pos: start, End: l.pos()})
		return
	}
	l.tok(kind, start)
}

// lexString scans one piece of a string literal, starting just after its
// opening quote (head) or just after the } that closed an interpolation.
func (l *lexer) lexString(start token.Pos, head bool) {
	var b strings.Builder
	for {
		if l.off >= len(l.src) || l.peek() == '\n' {
			quote := start
			if !head {
				quote = l.stringStart()
				l.pop(frameInterp)
			}
			l.errorf(quote, l.pos(), "add a closing \" at the end of the string",
				"this string is not closed before the end of the line")
			kind := token.Str
			if !head {
				kind = token.StrTail
			}
			l.emit(token.Token{Kind: kind, Text: l.src[start.Offset:l.off], Value: b.String(), Pos: start, End: l.pos()})
			return
		}
		r := l.peek()
		switch r {
		case '"':
			l.advance()
			kind := token.Str
			if !head {
				kind = token.StrTail
				l.pop(frameInterp)
			}
			l.emit(token.Token{Kind: kind, Text: l.src[start.Offset:l.off], Value: b.String(), Pos: start, End: l.pos()})
			return
		case '\\':
			escStart := l.pos()
			l.advance()
			if l.off >= len(l.src) || l.peek() == '\n' {
				continue // reported as an unclosed string on the next pass
			}
			e := l.advance()
			switch e {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case '"', '\\', '{', '}':
				b.WriteRune(e)
			default:
				l.errorf(escStart, l.pos(), `the escapes are \n \t \" \\ \{ and \}; write \\ for a backslash`,
					"`\\%c` is not an escape", e)
				b.WriteRune('\\')
				b.WriteRune(e)
			}
		case '{':
			braceStart := l.pos()
			l.advance()
			if l.peek() == '}' {
				l.advance()
				l.errorf(braceStart, l.pos(), `put an expression inside, or write \{ for a literal brace`,
					"empty `{}` in a string")
				b.WriteString("{}")
				continue
			}
			kind := token.StrHead
			if !head {
				kind = token.StrMid
			}
			quote := start
			if !head {
				quote = l.stringStart()
			}
			l.emit(token.Token{Kind: kind, Text: l.src[start.Offset:l.off], Value: b.String(), Pos: start, End: l.pos()})
			if !head {
				l.pop(frameInterp)
			}
			l.push(frameInterp, quote)
			return
		default:
			b.WriteRune(l.advance())
		}
	}
}

// closeOpenStrings ends every string whose interpolation is still open at
// the end of a line or file. Strings never span lines, so each one is an
// unclosed string; an empty StrTail keeps the token stream well formed.
func (l *lexer) closeOpenStrings() {
	outer := -1
	for i, f := range l.stack {
		if f.kind == frameInterp {
			outer = i
			break
		}
	}
	if outer < 0 {
		return
	}
	l.errorf(l.stack[outer].pos, l.pos(), "close the string with \" before the end of the line",
		"this string is not closed before the end of the line")
	for i := len(l.stack) - 1; i >= outer; i-- {
		if l.stack[i].kind == frameInterp {
			l.emit(token.Token{Kind: token.StrTail, Pos: l.pos(), End: l.pos()})
		}
	}
	l.stack = l.stack[:outer]
}

// lexRawString scans a backtick string: no escapes, no interpolation, and
// it may span lines, so JSON and other text can be written as it is.
func (l *lexer) lexRawString(start token.Pos) {
	l.advance() // `
	from := l.off
	for l.off < len(l.src) && l.peek() != '`' {
		l.advance()
	}
	if l.off >= len(l.src) {
		l.errorf(start, l.pos(), "add a closing ` at the end of the string", "this raw string is never closed")
		l.emit(token.Token{Kind: token.Str, Text: l.src[start.Offset:l.off], Value: l.src[from:l.off], Pos: start, End: l.pos()})
		return
	}
	value := l.src[from:l.off]
	l.advance() // `
	l.emit(token.Token{Kind: token.Str, Text: l.src[start.Offset:l.off], Value: value, Pos: start, End: l.pos()})
}

// stringStart is the position of the opening quote of the string whose
// interpolation is innermost.
func (l *lexer) stringStart() token.Pos {
	if f, ok := l.top(); ok && f.kind == frameInterp {
		return f.pos
	}
	return l.pos()
}

func (l *lexer) lexPunct(start token.Pos, r rune) {
	two := func(next rune, pair, single token.Kind) {
		l.advance()
		if l.peek() == next {
			l.advance()
			l.tok(pair, start)
			return
		}
		l.tok(single, start)
	}
	switch r {
	case '(':
		l.advance()
		l.push(frameParen, start)
		l.tok(token.LParen, start)
	case ')':
		l.advance()
		l.pop(frameParen)
		l.tok(token.RParen, start)
	case '[':
		l.advance()
		l.push(frameBracket, start)
		l.tok(token.LBracket, start)
	case ']':
		l.advance()
		l.pop(frameBracket)
		l.tok(token.RBracket, start)
	case '{':
		l.advance()
		l.push(frameBrace, start)
		l.tok(token.LBrace, start)
	case '}':
		if f, ok := l.top(); ok && f.kind == frameInterp {
			l.advance()
			l.lexString(start, false)
			return
		}
		l.advance()
		l.pop(frameBrace)
		l.tok(token.RBrace, start)
	case ',':
		l.advance()
		l.tok(token.Comma, start)
	case ':':
		l.advance()
		l.tok(token.Colon, start)
	case ';':
		l.advance()
		l.tok(token.Semi, start)
	case '.':
		two('.', token.DotDot, token.Dot)
	case '+':
		two('=', token.PlusEq, token.Plus)
	case '*':
		l.advance()
		l.tok(token.Star, start)
	case '%':
		l.advance()
		l.tok(token.Percent, start)
	case '-':
		two('>', token.Arrow, token.Minus)
	case '=':
		two('=', token.Eq, token.Assign)
	case '!':
		two('=', token.NotEq, token.Bang)
	case '<':
		two('=', token.LtEq, token.Lt)
	case '>':
		two('=', token.GtEq, token.Gt)
	case '|':
		l.advance()
		switch l.peek() {
		case '>':
			l.advance()
			l.tok(token.Pipe, start)
		case '|':
			l.advance()
			l.illegal(start, "`||` is not an operator in Fennic", "write `or`: `a or b`; to handle a failure, write `else`")
		default:
			l.tok(token.Bar, start)
		}
	case '?':
		l.advance()
		switch l.peek() {
		case '?':
			l.advance()
			l.tok(token.QQ, start)
		case '.':
			l.advance()
			l.tok(token.QDot, start)
		default:
			l.tok(token.Question, start)
		}
	case '/':
		l.advance()
		if l.peek() == '*' {
			l.advance()
			l.illegal(start, "`/*` does not start a comment", "start each comment line with //")
			return
		}
		l.tok(token.Slash, start)
	case '\'':
		l.advance()
		// A closed 'text' on one line is almost certainly a string with the
		// wrong quotes: report it once and carry on as if it were "text".
		if end := strings.IndexAny(l.src[l.off:], "'\n"); end >= 0 && l.src[l.off+end] == '\'' {
			text := l.src[l.off : l.off+end]
			for l.peek() != '\'' {
				l.advance()
			}
			l.advance()
			l.errorf(start, l.pos(), fmt.Sprintf(`strings use double quotes: write "%s"`, text),
				"`'%s'` uses single quotes", text)
			l.emit(token.Token{Kind: token.Str, Text: l.src[start.Offset:l.off], Value: text, Pos: start, End: l.pos()})
			return
		}
		l.illegal(start, "`'` does not start a string", `strings use double quotes: "like this"`)
	case '#':
		l.advance()
		l.illegal(start, "`#` does not start a comment", "start a comment with //")
	case '&':
		l.advance()
		if l.peek() == '&' {
			l.advance()
			l.illegal(start, "`&&` is not an operator in Fennic", "write `and`: `a and b`")
			return
		}
		l.illegal(start, "`&` is not an operator in Fennic", "remove it")
	default:
		l.advance()
		l.illegal(start, fmt.Sprintf("unexpected character `%c`", r), "remove it")
	}
}

func (l *lexer) illegal(start token.Pos, msg, help string) {
	l.errorf(start, l.pos(), help, "%s", msg)
	l.tok(token.Illegal, start)
}
