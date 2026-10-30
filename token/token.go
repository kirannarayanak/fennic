// Package token defines the lexical tokens of Fennic and their source positions.
package token

import "fmt"

// Pos is a location in a source file. Line and Col are 1-based; Col counts
// characters (runes), not bytes. Offset is the 0-based byte offset.
type Pos struct {
	Line   int
	Col    int
	Offset int
}

func (p Pos) String() string { return fmt.Sprintf("%d:%d", p.Line, p.Col) }

type Kind int

const (
	Illegal Kind = iota
	EOF
	Newline

	Ident
	Int
	Float

	// A string with no interpolation is a single Str token. An interpolated
	// string is StrHead, the tokens of the first expression, then StrMid and
	// another expression as often as needed, then StrTail. The Value of each
	// piece is its decoded text.
	Str
	StrHead
	StrMid
	StrTail

	// Keywords.
	Let
	Var
	Fn
	Type
	Match
	Interface
	Together
	Test
	Assert
	With
	Or
	None
	True
	False

	// Punctuation and operators.
	LParen   // (
	RParen   // )
	LBrace   // {
	RBrace   // }
	LBracket // [
	RBracket // ]
	Comma    // ,
	Colon    // :
	Semi     // ;
	Dot      // .
	Arrow    // ->
	Pipe     // |>
	Bar      // |
	Question // ?
	QDot     // ?.
	QQ       // ??
	Bang     // !
	Assign   // =
	Eq       // ==
	NotEq    // !=
	Lt       // <
	LtEq     // <=
	Gt       // >
	GtEq     // >=
	Plus     // +
	Minus    // -
	Star     // *
	Slash    // /
	Percent  // %
)

var names = map[Kind]string{
	Illegal: "illegal character", EOF: "end of file", Newline: "end of line",
	Ident: "name", Int: "integer", Float: "float",
	Str: "string", StrHead: "string", StrMid: "string", StrTail: "string",
	Let: "let", Var: "var", Fn: "fn", Type: "type", Match: "match",
	Interface: "interface", Together: "together", Test: "test", Assert: "assert",
	With: "with", Or: "or", None: "none", True: "true", False: "false",
	LParen: "(", RParen: ")", LBrace: "{", RBrace: "}", LBracket: "[", RBracket: "]",
	Comma: ",", Colon: ":", Semi: ";", Dot: ".", Arrow: "->", Pipe: "|>", Bar: "|",
	Question: "?", QDot: "?.", QQ: "??", Bang: "!", Assign: "=", Eq: "==", NotEq: "!=",
	Lt: "<", LtEq: "<=", Gt: ">", GtEq: ">=",
	Plus: "+", Minus: "-", Star: "*", Slash: "/", Percent: "%",
}

func (k Kind) String() string {
	if s, ok := names[k]; ok {
		return s
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

var Keywords = map[string]Kind{
	"let": Let, "var": Var, "fn": Fn, "type": Type, "match": Match,
	"interface": Interface, "together": Together, "test": Test, "assert": Assert,
	"with": With, "or": Or, "none": None, "true": True, "false": False,
}

// Token is one lexeme. Text is the exact source text; Value is the decoded
// text of string pieces. End is the position just past the last character.
type Token struct {
	Kind  Kind
	Text  string
	Value string
	Pos   Pos
	End   Pos
}
