// Package format writes Fennic source in its one canonical form.
//
// The formatter works on tokens, not on the syntax tree, because the
// author's line breaks are kept: it never joins or splits a line. Within
// each line it decides the indentation and the spaces between tokens, and
// across lines it limits blank lines, adds or removes trailing commas, and
// aligns match arms and trailing comments. docs/m4.md has the rules.
package format

import (
	"strings"
	"unicode/utf8"

	"github.com/kirannarayanak/fennic/diag"
	"github.com/kirannarayanak/fennic/lexer"
	"github.com/kirannarayanak/fennic/parser"
	"github.com/kirannarayanak/fennic/token"
)

// Source formats src. A file that does not parse is not formatted: its
// syntax errors are returned and the text is left alone.
func Source(src string) (string, []diag.Diagnostic) {
	if _, diags := parser.Parse(src); len(diags) > 0 {
		return src, diags
	}
	toks, comments, _ := lexer.LexComments(src)
	f := &formatter{src: src}
	f.scan(toks)
	f.brackets()
	f.lines(comments)
	return f.print(), nil
}

// atom is one token as the formatter places it. An interpolated string is
// a single atom, written exactly as the source has it.
type atom struct {
	kind     token.Kind
	text     string
	pos, end token.Pos

	match   int       // for a bracket, the index of the one that closes or opens it
	brace   braceKind // for `{` and `}`
	list    bool      // for an opener: its elements are separated by commas
	isUnary bool      // for `-`
}

type braceKind int

const (
	braceBlock  braceKind = iota
	braceMap              // Map{...}: tight inside
	braceFields           // a record literal, `with { ... }`: comma-separated fields
	braceDecl             // `type T { ... }`, `interface I { ... }`: one per line
	braceMatch            // the arms of a match
)

type formatter struct {
	src   string
	atoms []*atom
	lns   []*line
}

func (f *formatter) scan(toks []token.Token) {
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		switch t.Kind {
		case token.Newline, token.EOF:
			continue
		case token.StrHead:
			depth := 0
			j := i
			for ; j < len(toks); j++ {
				switch toks[j].Kind {
				case token.StrHead:
					depth++
				case token.StrTail:
					depth--
				}
				if depth == 0 {
					break
				}
			}
			f.atoms = append(f.atoms, &atom{kind: token.Str, text: f.src[t.Pos.Offset:toks[j].End.Offset], pos: t.Pos, end: toks[j].End})
			i = j
			continue
		}
		f.atoms = append(f.atoms, &atom{kind: t.Kind, text: f.src[t.Pos.Offset:t.End.Offset], pos: t.Pos, end: t.End})
	}
}

func isOpener(k token.Kind) bool {
	return k == token.LParen || k == token.LBracket || k == token.LBrace
}

func isCloser(k token.Kind) bool {
	return k == token.RParen || k == token.RBracket || k == token.RBrace
}

// endsValue reports whether an atom of kind k can end an operand, so that a
// `-`, `(` or `[` after it is binary, a call or an index.
func endsValue(k token.Kind) bool {
	switch k {
	case token.Ident, token.Int, token.Float, token.Str, token.True, token.False, token.None,
		token.RParen, token.RBracket, token.RBrace, token.Bang, token.Question:
		return true
	}
	return false
}

// brackets matches each opener with its closer and decides what kind of
// list it holds.
func (f *formatter) brackets() {
	var stack []int
	matchDepth := -1 // the stack depth of a `match` whose `{` is still to come
	var matchStack []int
	fnDepth := -1 // likewise for the parameters of a `fn`
	var fnStack []int
	for i, a := range f.atoms {
		a.match = -1
		var prev *atom
		if i > 0 {
			prev = f.atoms[i-1]
		}
		switch a.kind {
		case token.Match:
			matchStack = append(matchStack, matchDepth)
			matchDepth = len(stack)
		case token.Fn:
			fnStack = append(fnStack, fnDepth)
			fnDepth = len(stack)
		case token.Minus:
			a.isUnary = prev == nil || !endsValue(prev.kind)
		}
		if isOpener(a.kind) {
			switch a.kind {
			case token.LParen:
				if fnDepth == len(stack) {
					// A parameter list: a trailing comma is not added there.
					fnDepth, fnStack = pop(fnStack)
				} else {
					a.list = prev != nil && endsValue(prev.kind) && prev.kind != token.RBrace
				}
			case token.LBracket:
				a.list = prev == nil || !endsValue(prev.kind)
			case token.LBrace:
				switch {
				case matchDepth == len(stack):
					a.brace = braceMatch
					matchDepth, matchStack = pop(matchStack)
				case prev != nil && prev.kind == token.Ident && prev.text == "Map":
					a.brace = braceMap
				case i >= 2 && (f.atoms[i-2].kind == token.Type || f.atoms[i-2].kind == token.Interface):
					a.brace = braceDecl
				}
			}
			stack = append(stack, i)
			continue
		}
		if isCloser(a.kind) && len(stack) > 0 {
			o := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			f.atoms[o].match = i
			a.match = o
			if f.atoms[o].kind == token.LBrace && f.atoms[o].brace == braceBlock && f.hasTopLevel(o, i, token.Colon) {
				f.atoms[o].brace = braceFields
			}
			if f.atoms[o].kind == token.LBrace {
				f.atoms[o].list = f.atoms[o].brace == braceMap || f.atoms[o].brace == braceFields
				a.brace = f.atoms[o].brace
			}
			a.list = f.atoms[o].list
		}
	}
}

func pop(s []int) (int, []int) {
	if len(s) == 0 {
		return -1, s
	}
	return s[len(s)-1], s[:len(s)-1]
}

// hasTopLevel reports whether an atom of kind k sits directly inside the
// brackets at from and to.
func (f *formatter) hasTopLevel(from, to int, k token.Kind) bool {
	depth := 0
	for _, a := range f.atoms[from+1 : to] {
		switch {
		case isOpener(a.kind):
			depth++
		case isCloser(a.kind):
			depth--
		case depth == 0 && a.kind == k:
			return true
		}
	}
	return false
}

// line is one line of output: its atoms (as indexes), a trailing comment,
// or a comment on its own.
type line struct {
	atoms   []int
	comment string
	blank   bool // a blank line came before it in the source

	indent    int
	code      string // the code, without indentation or comment
	armSplit  int    // in a match arm on one line, the byte offset of `->` in code
	padArm    int    // spaces added before the arm's `->`
	addComma  bool   // a trailing comma goes after the code
	firstLine int    // its first line in the source
	lastLine  int
}

// lines groups atoms and comments into the source's lines.
func (f *formatter) lines(comments []lexer.Comment) {
	ci := 0
	lastEnd := 0
	var cur *line
	start := func(l int) {
		cur = &line{blank: len(f.lns) > 0 && l > lastEnd+1, firstLine: l}
		f.lns = append(f.lns, cur)
	}
	addComment := func(c lexer.Comment) {
		if cur == nil || c.Pos.Line > lastEnd {
			start(c.Pos.Line)
		}
		cur.comment = c.Text
		cur.lastLine = c.Pos.Line
		lastEnd = c.Pos.Line
	}
	for i, a := range f.atoms {
		for ci < len(comments) && comments[ci].Pos.Offset < a.pos.Offset {
			addComment(comments[ci])
			ci++
		}
		if cur == nil || a.pos.Line > lastEnd || cur.comment != "" {
			start(a.pos.Line)
		}
		cur.atoms = append(cur.atoms, i)
		cur.lastLine = a.end.Line
		lastEnd = a.end.Line
	}
	for ; ci < len(comments); ci++ {
		addComment(comments[ci])
	}
}

// continuesLine reports whether a line ending in k goes on to the next.
func continuesLine(k token.Kind) bool {
	switch k {
	case token.Dot, token.QDot, token.Arrow, token.Pipe, token.Bar, token.QQ, token.Assign,
		token.Eq, token.NotEq, token.Lt, token.LtEq, token.Gt, token.GtEq, token.Plus,
		token.Minus, token.Star, token.Slash, token.Percent, token.Or, token.And, token.Not,
		token.Else, token.With, token.DotDot, token.PlusEq, token.In:
		return true
	}
	return false
}

// startsContinuation reports whether a line starting with k continues the
// line before it.
func startsContinuation(k token.Kind) bool {
	switch k {
	case token.Pipe, token.Bar, token.Else, token.Dot, token.QDot:
		return true
	}
	return false
}

type open struct {
	atom         int
	outer, inner int
}

func (f *formatter) print() string {
	var stack []open
	var lastCode *line
	for _, l := range f.lns {
		if len(l.atoms) == 0 {
			continue // a comment on its own line: indented below
		}
		first := f.atoms[l.atoms[0]]
		switch {
		case isCloser(first.kind) && len(stack) > 0:
			l.indent = stack[len(stack)-1].outer
			if f.atoms[first.match].list && lastCode != nil {
				last := f.atoms[lastCode.atoms[len(lastCode.atoms)-1]]
				if last.kind != token.Comma && !isOpener(last.kind) {
					lastCode.addComma = true
				}
			}
		default:
			if len(stack) > 0 {
				l.indent = stack[len(stack)-1].inner
			}
			if lastCode != nil {
				prevLast := f.atoms[lastCode.atoms[len(lastCode.atoms)-1]]
				if startsContinuation(first.kind) || continuesLine(prevLast.kind) && !(prevLast.kind == token.Minus && prevLast.isUnary) {
					l.indent++
				}
			}
		}
		inMatch := len(stack) > 0 && f.atoms[stack[len(stack)-1].atom].brace == braceMatch && !isCloser(first.kind)
		for _, i := range l.atoms {
			a := f.atoms[i]
			if isOpener(a.kind) {
				stack = append(stack, open{atom: i, outer: l.indent, inner: l.indent + 1})
			} else if isCloser(a.kind) && len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
		l.code, l.armSplit = f.render(l.atoms, inMatch)
		lastCode = l
	}
	f.indentComments()
	f.alignArms()
	return f.write()
}

// render writes a line's atoms with the spaces between them. In a match,
// it also finds the arm's `->`.
func (f *formatter) render(idx []int, inMatch bool) (string, int) {
	var b strings.Builder
	arm := -1
	depth := 0
	for n, i := range idx {
		a := f.atoms[i]
		if a.kind == token.Comma && n+1 < len(idx) {
			if next := f.atoms[idx[n+1]]; isCloser(next.kind) && next.list {
				continue // no trailing comma on one line
			}
		}
		if n > 0 && f.space(f.atoms[idx[n-1]], a) {
			b.WriteByte(' ')
		}
		if a.kind == token.Arrow && inMatch && depth == 0 && arm < 0 {
			arm = b.Len()
		}
		switch {
		case isOpener(a.kind):
			depth++
		case isCloser(a.kind):
			depth--
		}
		b.WriteString(a.text)
	}
	if depth != 0 {
		arm = -1 // the arm goes on past this line
	}
	return b.String(), arm
}

// space reports whether a space goes between atoms a and b on one line.
func (f *formatter) space(a, b *atom) bool {
	if a.kind == token.Comma && isCloser(b.kind) {
		return false // the comma was dropped
	}
	switch b.kind {
	case token.Comma, token.Semi, token.RParen, token.RBracket, token.Colon,
		token.Dot, token.QDot, token.DotDot, token.Bang, token.Question:
		return false
	case token.LParen:
		return !(endsValue(a.kind) && a.kind != token.RBrace || a.kind == token.Fn) && a.kind != token.LParen && a.kind != token.LBracket
	case token.LBracket:
		return !endsValue(a.kind) && a.kind != token.LParen && a.kind != token.LBracket
	case token.LBrace:
		return !(a.kind == token.Ident && a.text == "Map")
	case token.RBrace:
		return b.brace != braceMap && a.kind != token.LBrace
	}
	switch a.kind {
	case token.LParen, token.LBracket, token.Dot, token.QDot, token.DotDot:
		return false
	case token.LBrace:
		return a.brace != braceMap
	case token.Minus:
		return !a.isUnary
	}
	return true
}

// indentComments indents each comment on its own line like the code after
// it, or inside the block a closing bracket ends.
func (f *formatter) indentComments() {
	next := 0
	for i := len(f.lns) - 1; i >= 0; i-- {
		l := f.lns[i]
		if len(l.atoms) > 0 {
			next = l.indent
			if isCloser(f.atoms[l.atoms[0]].kind) {
				next++
			}
			continue
		}
		l.indent = next
	}
}

// alignArms lines up the `->` of consecutive match arms that are each on
// one line.
func (f *formatter) alignArms() {
	for i := 0; i < len(f.lns); {
		if f.lns[i].armSplit < 0 || len(f.lns[i].atoms) == 0 {
			i++
			continue
		}
		j := i
		width := 0
		for ; j < len(f.lns); j++ {
			l := f.lns[j]
			if len(l.atoms) == 0 || l.armSplit < 0 || l.indent != f.lns[i].indent || j > i && l.blank {
				break
			}
			width = max(width, runes(strings.TrimRight(l.code[:l.armSplit], " ")))
		}
		for _, l := range f.lns[i:j] {
			l.padArm = width - runes(strings.TrimRight(l.code[:l.armSplit], " "))
		}
		i = j
	}
}

func runes(s string) int { return utf8.RuneCountInString(s) }

// write joins the lines, aligning the trailing comments of consecutive
// lines.
func (f *formatter) write() string {
	text := make([]string, len(f.lns))
	for i, l := range f.lns {
		code := l.code
		if l.armSplit >= 0 && l.padArm > 0 {
			head := strings.TrimRight(code[:l.armSplit], " ")
			code = head + strings.Repeat(" ", l.padArm+1) + code[l.armSplit:]
		}
		if l.addComma {
			code += ","
		}
		if code != "" {
			code = strings.Repeat("  ", l.indent) + code
		}
		text[i] = code
	}
	var out strings.Builder
	for i := 0; i < len(f.lns); {
		l := f.lns[i]
		if len(l.atoms) == 0 || l.comment == "" {
			f.writeLine(&out, i, text[i], -1)
			i++
			continue
		}
		j := i
		width := 0
		for ; j < len(f.lns); j++ {
			m := f.lns[j]
			if len(m.atoms) == 0 || m.comment == "" || j > i && m.blank || strings.Contains(text[j], "\n") {
				break
			}
			width = max(width, runes(text[j]))
		}
		if j == i {
			j, width = i+1, -1 // a code line spanning lines: one space
		}
		for k := i; k < j; k++ {
			f.writeLine(&out, k, text[k], width)
		}
		i = j
	}
	return out.String()
}

func (f *formatter) writeLine(out *strings.Builder, i int, code string, width int) {
	l := f.lns[i]
	if l.blank && i > 0 && !f.opensBlock(i-1) && !f.closesBlock(i) {
		out.WriteString("\n")
	}
	switch {
	case len(l.atoms) == 0:
		out.WriteString(strings.Repeat("  ", l.indent) + l.comment)
	case l.comment != "":
		pad := 1
		if width >= 0 {
			pad = width - runes(lastLineOf(code)) + 1
		}
		out.WriteString(code + strings.Repeat(" ", pad) + l.comment)
	default:
		out.WriteString(code)
	}
	out.WriteString("\n")
}

func lastLineOf(s string) string {
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}

// opensBlock reports whether line i ends with an opening bracket.
func (f *formatter) opensBlock(i int) bool {
	l := f.lns[i]
	return len(l.atoms) > 0 && isOpener(f.atoms[l.atoms[len(l.atoms)-1]].kind)
}

// closesBlock reports whether line i starts with a closing bracket.
func (f *formatter) closesBlock(i int) bool {
	l := f.lns[i]
	return len(l.atoms) > 0 && isCloser(f.atoms[l.atoms[0]].kind)
}
