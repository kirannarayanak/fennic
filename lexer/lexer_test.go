package lexer

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kirannarayanak/fennic/token"
)

// show renders tokens compactly: names and numbers as written, string
// pieces with their decoded value, NL for Newline, EOF left out.
func show(toks []token.Token) string {
	var parts []string
	for _, t := range toks {
		switch t.Kind {
		case token.EOF:
		case token.Newline:
			parts = append(parts, "NL")
		case token.Ident, token.Int, token.Float:
			parts = append(parts, t.Text)
		case token.Str:
			parts = append(parts, "Str"+strconv.Quote(t.Value))
		case token.StrHead:
			parts = append(parts, "Head"+strconv.Quote(t.Value))
		case token.StrMid:
			parts = append(parts, "Mid"+strconv.Quote(t.Value))
		case token.StrTail:
			parts = append(parts, "Tail"+strconv.Quote(t.Value))
		case token.Illegal:
			parts = append(parts, "ILLEGAL("+t.Text+")")
		default:
			parts = append(parts, t.Kind.String())
		}
	}
	return strings.Join(parts, " ")
}

type tokCase struct {
	name string
	src  string
	want string
}

func runTokCases(t *testing.T, cases []tokCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			toks, diags := Lex(c.src)
			if len(diags) > 0 {
				t.Fatalf("unexpected diagnostics: %+v", diags)
			}
			if got := show(toks); got != c.want {
				t.Errorf("\nsrc:  %q\ngot:  %s\nwant: %s", c.src, got, c.want)
			}
			if last := toks[len(toks)-1]; last.Kind != token.EOF {
				t.Errorf("last token is %v, want EOF", last.Kind)
			}
		})
	}
}

func TestBindings(t *testing.T) {
	runTokCases(t, []tokCase{
		{"let", `let name = "Kiran"`, `let name = Str"Kiran"`},
		{"var", `var count = 0`, `var count = 0`},
		{"assign", `count = count + 1`, `count = count + 1`},
		{"keyword prefix is a name", `letter lets variable`, `letter lets variable`},
		{"underscore and digits", `_x a1 sort_by`, `_x a1 sort_by`},
		{"unicode name", `let café = 1`, `let café = 1`},
		{"all keywords", `let var fn type match interface together test assert with or none true false`,
			`let var fn type match interface together test assert with or none true false`},
	})
}

func TestNumbers(t *testing.T) {
	runTokCases(t, []tokCase{
		{"int", `42`, `42`},
		{"float", `3.14159`, `3.14159`},
		{"negative is minus then int", `-1`, `- 1`},
		{"field access is not a float", `p.x`, `p . x`},
		{"int then method", `1.abs`, `1 . abs`},
		{"arithmetic", `7 / 2 % 3 * 4 - 5`, `7 / 2 % 3 * 4 - 5`},
	})
}

func TestOperators(t *testing.T) {
	runTokCases(t, []tokCase{
		{"comparison", `a == b != c < d <= e > f >= g`, `a == b != c < d <= e > f >= g`},
		{"arrow and pipe", `x |> f(u -> u)`, `x |> f ( u -> u )`},
		{"optional", `a?.b ?? c`, `a ?. b ?? c`},
		{"optional type", `email: Str?`, `email : Str ?`},
		{"failable type and propagate", `fn load() -> Config! { read()! }`, `fn load ( ) -> Config ! { read ( ) ! }`},
		{"bang then equals", `a != b`, `a != b`},
		{"generics", `fn largest[T: Ord](items: List[T]) -> T?`,
			`fn largest [ T : Ord ] ( items : List [ T ] ) -> T ?`},
		{"variant bar", `type Shape = | Circle(radius: Float)`, `type Shape = | Circle ( radius : Float )`},
		{"semicolon", `{ a; b }`, `{ a ; b }`},
		{"with", `u with { age: 32 }`, `u with { age : 32 }`},
	})
}

func TestStrings(t *testing.T) {
	runTokCases(t, []tokCase{
		{"plain", `"hello"`, `Str"hello"`},
		{"empty", `""`, `Str""`},
		{"newline escape", `"a\nb"`, `Str"a\nb"`},
		{"tab escape", `"a\tb"`, `Str"a\tb"`},
		{"quote escape", `"she said \"hi\""`, `Str"she said \"hi\""`},
		{"backslash escape", `"C:\\fennic"`, `Str"C:\\fennic"`},
		{"brace escape", `"\{not interpolated}"`, `Str"{not interpolated}"`},
		{"close brace is literal", `"a } b"`, `Str"a } b"`},
		{"unicode", `"naïve ✓"`, `Str"naïve ✓"`},
	})
}

func TestInterpolation(t *testing.T) {
	runTokCases(t, []tokCase{
		{"one name", `"Hello, {who}"`, `Head"Hello, " who Tail""`},
		{"at start and end", `"{a}"`, `Head"" a Tail""`},
		{"two parts", `"{a} and {b}!"`, `Head"" a Mid" and " b Tail"!"`},
		{"expression", `"{w * h >= 40}"`, `Head"" w * h >= 40 Tail""`},
		{"nested string", `"x: {wrap("hi")}"`, `Head"x: " wrap ( Str"hi" ) Tail""`},
		{"nested interpolation", `"a{f("b{c}d")}e"`, `Head"a" f ( Head"b" c Tail"d" ) Tail"e"`},
		{"record literal inside", `"{p == Point { x: 3 }}"`, `Head"" p == Point { x : 3 } Tail""`},
		{"optional chain inside", `"{a?.b ?? "none"}"`, `Head"" a ?. b ?? Str"none" Tail""`},
		{"escape beside interpolation", `"\{{x}\}"`, `Head"{" x Tail"}"`},
	})
}

func TestNewlines(t *testing.T) {
	runTokCases(t, []tokCase{
		{"separates statements", "a\nb", "a NL b"},
		{"blank lines collapse", "a\n\n\n b", "a NL b"},
		{"no leading newline", "\n\na", "a"},
		{"no trailing newline", "a\n\n", "a"},
		{"crlf", "a\r\nb", "a NL b"},
		{"pipe continues", "users\n  |> filter(f)\n  |> take(3)\nx", "users |> filter ( f ) |> take ( 3 ) NL x"},
		{"variant bars continue", "type Shape =\n  | Circle(r: Float)\n  | Rect(w: Float)",
			"type Shape = | Circle ( r : Float ) | Rect ( w : Float )"},
		{"after operator", "a +\nb", "a + b"},
		{"after comma", "f(a,\nb)", "f ( a , b )"},
		{"inside parens", "f(\na\n)", "f ( a )"},
		{"inside brackets", "[\n1,\n2,\n]", "[ 1 , 2 , ]"},
		{"after open brace", "{\na\nb\n}", "{ a NL b NL }"},
		{"block inside parens keeps newlines", "map(x -> {\na\nb\n})", "map ( x -> { a NL b NL } )"},
		{"after arrow", "x ->\n y", "x -> y"},
		{"after or", "a or\n b", "a or b"},
		{"after bang ends statement", "f()!\nx", "f ( ) ! NL x"},
		{"after question ends statement", "email: Str?\nage: Int", "email : Str ? NL age : Int"},
	})
}

func TestComments(t *testing.T) {
	runTokCases(t, []tokCase{
		{"line comment", "// hello\na", "a"},
		{"trailing comment", "let a = 1   // one\nb", "let a = 1 NL b"},
		{"comment-only lines collapse", "a\n// x\n// y\nb", "a NL b"},
		{"slashes in string are not a comment", `"http://x"`, `Str"http://x"`},
		{"division", "a / b", "a / b"},
	})
}

func TestPositions(t *testing.T) {
	type pos struct {
		text      string
		line, col int
	}
	cases := []struct {
		name string
		src  string
		want []pos
	}{
		{"single line", `let x = 10`, []pos{{"let", 1, 1}, {"x", 1, 5}, {"=", 1, 7}, {"10", 1, 9}}},
		{"second line", "a\n  bb", []pos{{"a", 1, 1}, {"\n", 1, 2}, {"bb", 2, 3}}},
		{"unicode counts characters", `"é" x`, []pos{{`"é"`, 1, 1}, {"x", 1, 5}}},
		{"tab counts one", "\tx", []pos{{"x", 1, 2}}},
		{"interpolation pieces", `"a{b}c"`, []pos{{`"a{`, 1, 1}, {"b", 1, 4}, {`}c"`, 1, 5}}},
		{"two-char operators", "a |> b ?? c", []pos{{"a", 1, 1}, {"|>", 1, 3}, {"b", 1, 6}, {"??", 1, 8}, {"c", 1, 11}}},
		{"after comment", "// hi\n  x", []pos{{"x", 2, 3}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			toks, diags := Lex(c.src)
			if len(diags) > 0 {
				t.Fatalf("unexpected diagnostics: %+v", diags)
			}
			toks = toks[:len(toks)-1] // EOF
			if len(toks) != len(c.want) {
				t.Fatalf("got %d tokens (%s), want %d", len(toks), show(toks), len(c.want))
			}
			for i, w := range c.want {
				g := toks[i]
				if g.Text != w.text || g.Pos.Line != w.line || g.Pos.Col != w.col {
					t.Errorf("token %d: got %q at %d:%d, want %q at %d:%d", i, g.Text, g.Pos.Line, g.Pos.Col, w.text, w.line, w.col)
				}
				if c.src[g.Pos.Offset:g.End.Offset] != g.Text {
					t.Errorf("token %d: offsets %d..%d give %q, want %q", i, g.Pos.Offset, g.End.Offset, c.src[g.Pos.Offset:g.End.Offset], g.Text)
				}
			}
		})
	}
}

func TestErrors(t *testing.T) {
	cases := []struct {
		name      string
		src       string
		msg       string
		help      string
		line, col int
		tokens    string // the stream must still be complete
	}{
		{"unclosed string", "let s = \"abc\nx", "this string is not closed before the end of the line",
			`add a closing " at the end of the string`, 1, 9, `let s = Str"abc" NL x`},
		{"unclosed string at eof", `"abc`, "this string is not closed before the end of the line",
			`add a closing " at the end of the string`, 1, 1, `Str"abc"`},
		{"unclosed interpolation", "\"a{b\nx", "this string is not closed before the end of the line",
			`close the string with " before the end of the line`, 1, 1, `Head"a" b Tail"" NL x`},
		{"unclosed call in interpolation", "\"a{f(b\nx", "this string is not closed before the end of the line",
			`close the string with " before the end of the line`, 1, 1, `Head"a" f ( b Tail"" NL x`},
		{"bad escape", `"a\qb"`, "`\\q` is not an escape",
			`the escapes are \n \t \" \\ \{ and \}; write \\ for a backslash`, 1, 3, `Str"a\\qb"`},
		{"empty interpolation", `"a{}b"`, "empty `{}` in a string",
			`put an expression inside, or write \{ for a literal brace`, 1, 3, `Str"a{}b"`},
		{"single quote", `x = 'a`, "`'` does not start a string", `strings use double quotes: "like this"`, 1, 5,
			`x = ILLEGAL(') a`},
		{"single-quoted string", `let s = 'hi'`, "`'hi'` uses single quotes", `strings use double quotes: write "hi"`, 1, 9,
			`let s = Str"hi"`},
		{"hash comment", "# note", "`#` does not start a comment", "start a comment with //", 1, 1, "ILLEGAL(#) note"},
		{"block comment", "/* x", "`/*` does not start a comment", "start each comment line with //", 1, 1, "ILLEGAL(/*) x"},
		{"double ampersand", "a && b", "`&&` is not an operator in Fennic",
			"there is no boolean `&&`; use `match` on the condition", 1, 3, "a ILLEGAL(&&) b"},
		{"double bar", "a || b", "`||` is not an operator in Fennic",
			"to handle a failure write `or`; there is no boolean `||`", 1, 3, "a ILLEGAL(||) b"},
		{"digit then letters", "let 2x = 1", "`2x` is not a number or a name",
			"names cannot start with a digit; put a space between a number and a name", 1, 5, "let ILLEGAL(2x) = 1"},
		{"stray character", "a @ b", "unexpected character `@`", "remove it", 1, 3, "a ILLEGAL(@) b"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			toks, diags := Lex(c.src)
			if len(diags) != 1 {
				t.Fatalf("got %d diagnostics %+v, want 1", len(diags), diags)
			}
			d := diags[0]
			if d.Msg != c.msg || d.Help != c.help {
				t.Errorf("got\n  msg:  %s\n  help: %s\nwant\n  msg:  %s\n  help: %s", d.Msg, d.Help, c.msg, c.help)
			}
			if d.Pos.Line != c.line || d.Pos.Col != c.col {
				t.Errorf("at %d:%d, want %d:%d", d.Pos.Line, d.Pos.Col, c.line, c.col)
			}
			if got := show(toks); got != c.tokens {
				t.Errorf("tokens\ngot:  %s\nwant: %s", got, c.tokens)
			}
		})
	}
}

// Every acceptance program must lex without a diagnostic.
func TestExamplesLexCleanly(t *testing.T) {
	files, err := filepath.Glob("../examples/*.fn")
	if err != nil || len(files) != 20 {
		t.Fatalf("found %d examples (err %v), want 20", len(files), err)
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			_, diags := Lex(string(src))
			for _, d := range diags {
				t.Errorf("%s:%d:%d: %s", f, d.Pos.Line, d.Pos.Col, d.Msg)
			}
		})
	}
}
