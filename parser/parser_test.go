package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirannarayanak/fennic/ast"
)

type parseCase struct {
	name string
	src  string
	want string // Dump of each top-level item, one per line
}

func dumpItems(f *ast.File) string {
	var lines []string
	for _, it := range f.Items {
		lines = append(lines, ast.Dump(it))
	}
	return strings.Join(lines, "\n")
}

func runParseCases(t *testing.T, cases []parseCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, diags := Parse(c.src)
			for _, d := range diags {
				t.Errorf("unexpected diagnostic at %v: %s", d.Pos, d.Msg)
			}
			if got := dumpItems(f); got != c.want {
				t.Errorf("\nsrc:\n%s\ngot:\n%s\nwant:\n%s", c.src, got, c.want)
			}
		})
	}
}

func TestBindings(t *testing.T) {
	runParseCases(t, []parseCase{
		{"let", `let name = "Kiran"`, `(let name "Kiran")`},
		{"var", `var count = 0`, `(var count 0)`},
		{"assign", `count = count + 1`, `(= count (+ count 1))`},
		{"statements on lines", "let a = 1\nlet b = 2", "(let a 1)\n(let b 2)"},
		{"semicolons", "let a = 1; let b = 2", "(let a 1)\n(let b 2)"},
		{"literals", `let x = [1, 2.5, "s", true, false, none]`, `(let x (list 1 2.5 "s" true false none))`},
	})
}

func TestOperators(t *testing.T) {
	runParseCases(t, []parseCase{
		{"multiplication first", `a + b * c`, `(+ a (* b c))`},
		{"left associative", `a - b - c`, `(- (- a b) c)`},
		{"comparison below arithmetic", `w * h >= 40`, `(>= (* w h) 40)`},
		{"equality below comparison", `a < b == c > d`, `(== (< a b) (> c d))`},
		{"unary minus", `-w + 2`, `(+ (- w) 2)`},
		{"parentheses", `(a + b) * c`, `(* (+ a b) c)`},
		{"coalesce right associative", `a ?? b ?? c`, `(?? a (?? b c))`},
		{"coalesce below comparison", `a ?? b == c`, `(?? a (== b c))`},
		{"else loosest", `a ?? b else c`, `(else (?? a b) c)`},
	})
}

func TestFunctions(t *testing.T) {
	runParseCases(t, []parseCase{
		{"reference greet", "fn greet(who: Str) -> Str {\n  \"Hello, {who}\"\n}",
			`(fn greet ((who Str)) -> Str (block (str "Hello, " who "")))`},
		{"no result", `fn announce(msg: Str) { print(msg) }`, `(fn announce ((msg Str)) (block (call print msg)))`},
		{"no params", `fn f() -> Int { 1 }`, `(fn f () -> Int (block 1))`},
		{"several statements", "fn f(a: Int) -> Int {\n  let b = a\n  b * 2\n}", `(fn f ((a Int)) -> Int (block (let b a) (* b 2)))`},
		{"generic constrained", `fn largest[T: Ord](items: List[T]) -> T? { last(items) }`,
			`(fn largest [T: Ord] ((items List[T])) -> T? (block (call last items)))`},
		{"generic unconstrained", `fn id[T](x: T) -> T { x }`, `(fn id [T] ((x T)) -> T (block x))`},
		{"failable result", `fn load(path: Str) -> Config! { read(path)! }`,
			`(fn load ((path Str)) -> Config! (block (! (call read path))))`},
		{"method", `fn Console.write(self, line: Str) { print(line) }`,
			`(fn Console.write (self (line Str)) (block (call print line)))`},
		{"type function", `fn Config.default() -> Config { Config { name: "d" } }`,
			`(fn Config.default () -> Config (block (record Config (name "d"))))`},
		{"calls", `f(a, g(b), c.d(e))`, `(call f a (call g b) (call (. c d) e))`},
	})
}

func TestRecords(t *testing.T) {
	runParseCases(t, []parseCase{
		{"declaration", "type User {\n  name: Str\n  age: Int\n  email: Str?\n}",
			`(type User (name Str) (age Int) (email Str?))`},
		{"empty", `type Console {}`, `(type Console)`},
		{"literal", `User { name: "Ada", age: 41, email: none }`, `(record User (name "Ada") (age 41) (email none))`},
		{"literal across lines", "Book {\n  title: \"T\",\n  pages: 3,\n}", `(record Book (title "T") (pages 3))`},
		{"empty literal", `Console {}`, `(record Console)`},
		{"field access", `p.x`, `(. p x)`},
		{"with", `u with { age: 32 }`, `(with u (age 32))`},
		{"with chained", `u with { name: "K" } with { age: 50 }`, `(with (with u (name "K")) (age 50))`},
		{"with binds tighter than comparison", `u with { a: 1 } == v`, `(== (with u (a 1)) v)`},
		{"lowercase name before brace is not a record", "fn f() { x }", `(fn f () (block x))`},
	})
}

func TestOptionals(t *testing.T) {
	runParseCases(t, []parseCase{
		{"default", `email ?? "none"`, `(?? email "none")`},
		{"chain", `team.lead?.name ?? "nobody"`, `(?? (?. (. team lead) name) "nobody")`},
		{"chain method", `x?.label()`, `(call (?. x label))`},
	})
}

func TestSumTypesAndMatch(t *testing.T) {
	runParseCases(t, []parseCase{
		{"reference shape", "type Shape =\n  | Circle(radius: Float)\n  | Rect(w: Float, h: Float)",
			`(sum Shape (Circle (radius Float)) (Rect (w Float) (h Float)))`},
		{"variant without fields", "type Status =\n  | Queued\n  | Done(code: Int)", `(sum Status (Queued) (Done (code Int)))`},
		{"first bar optional", `type Shape = Circle(r: Float) | Rect(w: Float)`, `(sum Shape (Circle (r Float)) (Rect (w Float)))`},
		{"reference match", "match s {\n  Circle(r)  -> 3.14159 * r * r\n  Rect(w, h) -> w * h\n}",
			`(match s (arm (pattern Circle r) (* (* 3.14159 r) r)) (arm (pattern Rect w h) (* w h)))`},
		{"match value is an expression", "let k = match s {\n  Circle(r) -> r\n}", `(let k (match s (arm (pattern Circle r) r)))`},
		{"arm with block", "match s {\n  Circle(r) -> { let d = r * 2.0; d }\n}",
			`(match s (arm (pattern Circle r) (block (let d (* r 2.0)) d)))`},
		{"uppercase scrutinee is not a record", "match Shape {\n  Circle(r) -> r\n}", `(match Shape (arm (pattern Circle r) r))`},
		{"variant construction is a call", `Rect(3.0, 4.0)`, `(call Rect 3.0 4.0)`},
	})
}

func TestErrorValues(t *testing.T) {
	runParseCases(t, []parseCase{
		{"propagate", `let text = fs.read(path)!`, `(let text (! (call (. fs read) path)))`},
		{"else default", `load("app.json") else Config.default()`, `(else (call load "app.json") (call (. Config default)))`},
		{"else handler", `load("app.json") else err -> { log.warn("{err}"); Config.default() }`,
			`(else (call load "app.json") (err) (block (call (. log warn) (str "" err "")) (call (. Config default))))`},
		{"else handler expression", `f() else e -> 0`, `(else (call f) (e) 0)`},
		{"else chain", `a else b else c`, `(else (else a b) c)`},
	})
}

func TestPipelinesAndLambdas(t *testing.T) {
	runParseCases(t, []parseCase{
		{"reference pipeline", "let top = users\n  |> filter(u -> u.age >= 18)\n  |> sort_by(u -> u.name)\n  |> take(10)",
			`(let top (|> (|> (|> users (call filter (lambda (u) (>= (. u age) 18)))) (call sort_by (lambda (u) (. u name)))) (call take 10)))`},
		{"pipe below arithmetic", `a + b |> f()`, `(|> (+ a b) (call f))`},
		{"pipe above coalesce", `xs |> last() ?? 0`, `(?? (|> xs (call last)) 0)`},
		{"bang after pipe applies to pipe", `xs |> map_par(f)!`, `(! (|> xs (call map_par f)))`},
		{"pipe into qualified call", `x |> json.parse()`, `(|> x (call (. json parse)))`},
		{"lambda", `let f = n -> n + 1`, `(let f (lambda (n) (+ n 1)))`},
		{"lambda block", "let g = s -> {\n  let c = s + 1\n  c * 2\n}", `(let g (lambda (s) (block (let c (+ s 1)) (* c 2))))`},
		{"lambda calls", `add_one(41)`, `(call add_one 41)`},
	})
}

func TestStrings(t *testing.T) {
	runParseCases(t, []parseCase{
		{"plain", `"hi"`, `"hi"`},
		{"interpolation", `"{a} and {b}"`, `(str "" a " and " b "")`},
		{"expression inside", `"{w * h}"`, `(str "" (* w h) "")`},
		{"nested string", `"x: {wrap("hi")}"`, `(str "x: " (call wrap "hi") "")`},
		{"record inside", `"{p == Point { x: 3 }}"`, `(str "" (== p (record Point (x 3))) "")`},
		{"coalesce inside", `"{a ?? "none"}"`, `(str "" (?? a "none") "")`},
	})
}

func TestInterfacesTogetherTests(t *testing.T) {
	runParseCases(t, []parseCase{
		{"interface one line", `interface Sink { fn write(self, line: Str) }`, `(interface Sink (fn write (self (line Str))))`},
		{"interface lines", "interface M {\n  fn area(self) -> Int\n  fn label(self) -> Str\n}",
			`(interface M (fn area (self) -> Int) (fn label (self) -> Str))`},
		{"together", "let pages = together {\n  urls |> map_par(u -> http.get(u)!)\n}!",
			`(let pages (! (together (block (|> urls (call map_par (lambda (u) (! (call (. http get) u)))))))))`},
		{"together else", `together { x } else e -> []`, `(else (together (block x)) (e) (list))`},
		{"test", "test \"greet uses the name\" {\n  assert greet(\"Kiran\") == \"Hello, Kiran\"\n}",
			`(test "greet uses the name" (block (assert (== (call greet "Kiran") "Hello, Kiran"))))`},
	})
}

func TestSpans(t *testing.T) {
	// The `!` diagnostic in examples/12 underlines the call, not the `!`.
	f, diags := Parse(`let text = fs.read("data/missing.txt")!`)
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	prop := f.Items[0].(*ast.LetStmt).Value.(*ast.PropagateExpr)
	if p, e := prop.X.Pos(), prop.X.End(); p.Col != 12 || e.Col != 39 {
		t.Errorf("call spans %v..%v, want 1:12..1:39", p, e)
	}
	if e := prop.End(); e.Col != 40 {
		t.Errorf("propagate ends at %v, want 1:40", e)
	}
	// examples/20 underlines the assigned name.
	f, _ = Parse("let total = 10\ntotal = total + 5")
	as := f.Items[1].(*ast.AssignStmt)
	if p, e := as.Name.Pos(), as.Name.End(); p.Line != 2 || p.Col != 1 || e.Col != 6 {
		t.Errorf("name spans %v..%v, want 2:1..2:6", p, e)
	}
}

// Recovery: every case reports exactly the listed problems (one per broken
// statement), and the statements after a mistake still parse.
func TestRecovery(t *testing.T) {
	type want struct {
		line, col int
		msg, help string
	}
	cases := []struct {
		name  string
		src   string
		diags []want
		items string
	}{
		{"missing value", "let a =\nlet b = 2",
			[]want{{1, 8, "expected an expression, found the end of the line", "write a value or expression here"}},
			"(let a (bad))\n(let b 2)"},
		{"missing equals", "let a 1\nprint(a)",
			[]want{{1, 7, "expected `=`, found `1`", "give it a value: `let a = ...`"}},
			"(let a 1)\n(call print a)"},
		{"unclosed call", "print(a\nlet b = 2",
			[]want{{1, 8, "expected `)` to close the `(` on line 1, found the end of the line", "add `)`"}},
			"(call print a)\n(let b 2)"},
		{"parameter without type", "fn f(x) -> Int { x }\nlet y = 1",
			[]want{{1, 6, "parameter `x` needs a type", "give it a type, like `x: Str`"}},
			"(fn f (x) -> Int (block x))\n(let y 1)"},
		{"return", "fn f() -> Int {\n  return 1\n}",
			[]want{{2, 3, "Fennic has no `return`", "the last expression of a block is its value; remove `return`"}},
			"(fn f () -> Int (block 1))"},
		{"null", "let a = null\nlet b = 1",
			[]want{{1, 9, "there is no `null` in Fennic", "use `none` for a missing value, and give the type as `T?`"}},
			"(let a (bad))\n(let b 1)"},
		{"two statements on a line", "let a = 1 let b = 2\nlet c = 3",
			[]want{{1, 11, "expected the end of the statement, found `let`", "put each statement on its own line"}},
			"(let a 1)\n(let b 2)\n(let c 3)"},
		{"comma between record fields", "type P {\n  x: Int,\n  y: Int\n}",
			[]want{{2, 9, "unexpected `,` between fields", "put each field on its own line, without commas"}},
			"(type P (x Int) (y Int))"},
		{"pipe into non-call", "xs |> len\nlet b = 1",
			[]want{{1, 7, "the right side of `|>` must be a call", "call it: `|> len()`"}},
			"(|> xs (call len))\n(let b 1)"},
		{"fn inside block", "fn f() {\n  fn g() { 1 }\n  2\n}\nlet b = 1",
			[]want{{2, 3, "`fn` declarations belong at the top level, not inside a block", "move it to the top level of the file"}},
			"(fn f () (block (bad) 2))\n(let b 1)"},
		{"errors in two statements", "let a = )\nlet b = 1\nlet c = ]",
			[]want{
				{1, 9, "expected an expression, found `)`", "write a value or expression here"},
				{3, 9, "expected an expression, found `]`", "write a value or expression here"},
			},
			"(let a (bad))\n(let b 1)\n(let c (bad))"},
		{"lexer error is reported once", "let a = 'x\nlet b = 1",
			[]want{{1, 9, "`'` does not start a string", `strings use double quotes: "like this"`}},
			"(let a (bad))\n(let b 1)"},
		{"signature and body errors both reported", "fn f(x) -> Int {\n  return 1\n}",
			[]want{
				{1, 6, "parameter `x` needs a type", "give it a type, like `x: Str`"},
				{2, 3, "Fennic has no `return`", "the last expression of a block is its value; remove `return`"},
			},
			"(fn f (x) -> Int (block 1))"},
		{"operator the lexer rejected is reported once", "let ok = 1 && 2\nlet b = 1",
			[]want{{1, 12, "`&&` is not an operator in Fennic", "there is no boolean `&&`; use `match` on the condition"}},
			"(let ok 1)\n(let b 1)"},
		{"single-quoted string recovers as a string", "let s = 'hi'\nprint(s)",
			[]want{{1, 9, "`'hi'` uses single quotes", `strings use double quotes: write "hi"`}},
			"(let s \"hi\")\n(call print s)"},
		{"lexer and parser errors in source order", "let a = )\nlet b = 'x\nlet c = ]",
			[]want{
				{1, 9, "expected an expression, found `)`", "write a value or expression here"},
				{2, 9, "`'` does not start a string", `strings use double quotes: "like this"`},
				{3, 9, "expected an expression, found `]`", "write a value or expression here"},
			},
			"(let a (bad))\n(let b (bad))\n(let c (bad))"},
		{"if without braces", "if a 1\nlet b = 1",
			[]want{{1, 6, "expected `{` after the condition, found `1`", "write the branch in braces: `if x > 0 { ... }`"}},
			"(bad)\n(let b 1)"},
		{"for without in", "for x xs { }\nlet b = 1",
			[]want{{1, 7, "expected `in`, found `xs`", "write `for x in ...`"}},
			"(for x xs (block))\n(let b 1)"},
		{"bad pattern", "match x {\n  + -> 1\n}",
			[]want{{2, 3, "expected a pattern, found `+`", "start the arm with a pattern: a variant like `Circle(r)`, a literal, a name, `_`, `(a, b)` or `[x]`"}},
			"(match x (arm _ (bad)))"},
		{"stray closing brace", "}\nlet b = 1",
			[]want{{1, 1, "unexpected `}`", "remove it, or add the `{` it was meant to close"}},
			"(let b 1)"},
		{"unclosed block at end of file", "fn f() {\n  1",
			[]want{{2, 4, "expected `}` to close the `{` on line 1, found the end of the file", "add `}`"}},
			"(fn f () (block 1))"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, diags := Parse(c.src)
			if len(diags) != len(c.diags) {
				for _, d := range diags {
					t.Logf("  %v: %s", d.Pos, d.Msg)
				}
				t.Fatalf("got %d diagnostics, want %d", len(diags), len(c.diags))
			}
			for i, w := range c.diags {
				d := diags[i]
				if d.Pos.Line != w.line || d.Pos.Col != w.col || d.Msg != w.msg || d.Help != w.help {
					t.Errorf("diagnostic %d:\ngot  %d:%d %s | %s\nwant %d:%d %s | %s",
						i, d.Pos.Line, d.Pos.Col, d.Msg, d.Help, w.line, w.col, w.msg, w.help)
				}
			}
			if got := dumpItems(f); got != c.items {
				t.Errorf("tree\ngot:\n%s\nwant:\n%s", got, c.items)
			}
		})
	}
}

func TestExamplesParseCleanly(t *testing.T) {
	files, err := filepath.Glob("../examples/*.fn")
	if err != nil || len(files) < 20 {
		t.Fatalf("found %d examples (err %v), want at least 20", len(files), err)
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			_, diags := Parse(string(src))
			for _, d := range diags {
				t.Errorf("%d:%d: %s", d.Pos.Line, d.Pos.Col, d.Msg)
			}
		})
	}
}

// The M2 rejected programs must be valid syntax: only the type checker may
// refuse them.
func TestRejectedExamplesParseCleanly(t *testing.T) {
	files, err := filepath.Glob("../examples/rejected/*.fn")
	if err != nil || len(files) < 20 {
		t.Fatalf("found %d rejected examples (err %v), want at least 20", len(files), err)
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			_, diags := Parse(string(src))
			for _, d := range diags {
				t.Errorf("%d:%d: %s", d.Pos.Line, d.Pos.Col, d.Msg)
			}
		})
	}
}

func TestControlFlow(t *testing.T) {
	runParseCases(t, []parseCase{
		{"if else", `let x = if a { 1 } else { 2 }`, `(let x (if a (block 1) (block 2)))`},
		{"else if", `if a { 1 } else if b { 2 } else { 3 }`, `(if a (block 1) (if b (block 2) (block 3)))`},
		{"if as a statement", "if a {\n  f()\n}", `(if a (block (call f)))`},
		{"else on its own line", "if a {\n  1\n}\nelse {\n  2\n}", `(if a (block 1) (block 2))`},
		{"record after if is the block", "if ok { User { name: 1 } }", `(if ok (block (record User (name 1))))`},
		{"for over a list", "for x in xs {\n  f(x)\n}", `(for x xs (block (call f x)))`},
		{"for over a range", `for _ in 1..8 { f() }`, `(for _ (.. 1 8) (block (call f)))`},
		{"plus equals", `n += 1`, `(+= n 1)`},
		{"fail", `fail "bad: {n}"`, `(fail (str "bad: " n ""))`},
		{"and binds tighter than or", `a or b and c`, `(or a (and b c))`},
		{"not binds tighter than and", `not a and b`, `(and (not a) b)`},
		{"comparison inside or", `n < 1 or n > 9`, `(or (< n 1) (> n 9))`},
		{"else is loosest", `a or b else c`, `(else (or a b) c)`},
		{"range below arithmetic", `1..n + 1`, `(.. 1 (+ n 1))`},
		{"range above comparison", `1..3 == xs`, `(== (.. 1 3) xs)`},
	})
}

func TestPatterns(t *testing.T) {
	runParseCases(t, []parseCase{
		{"tuple and wildcard", "match t {\n  (0, _) -> 1\n  _ -> 2\n}", `(match t (arm (tuple 0 _) 1) (arm _ 2))`},
		{"list", "match xs {\n  [] -> 0\n  [x] -> 1\n  [x, y] -> 2\n}", `(match xs (arm (list) 0) (arm (list x) 1) (arm (list x y) 2))`},
		{"literals", "match n {\n  -1 -> \"m\"\n  \"a\" -> 1\n  true -> 2\n}", `(match n (arm (- 1) "m") (arm "a" 1) (arm true 2))`},
		{"variant with no fields", "match s {\n  Queued -> 0\n}", `(match s (arm (pattern Queued) 0))`},
		{"nested literal", "match s {\n  Done(0) -> 0\n}", `(match s (arm (pattern Done 0) 0))`},
		{"bind", "match s {\n  other -> 0\n}", `(match s (arm other 0))`},
		{"tuple scrutinee", "match (a, b) {\n  _ -> 0\n}", `(match (tuple a b) (arm _ 0))`},
	})
}

func TestM3Expressions(t *testing.T) {
	runParseCases(t, []parseCase{
		{"lambda of two", `let f = (a, b) -> a + b`, `(let f (lambda (a b) (+ a b)))`},
		{"tuple", `let t = (1, "a")`, `(let t (tuple 1 "a"))`},
		{"group is not a tuple", `let t = (1)`, `(let t 1)`},
		{"anonymous function", `let f = fn() { g() }`, `(let f (fn () (block (call g))))`},
		{"anonymous function as a statement", "fn() { g() }", `(fn () (block (call g)))`},
		{"index", `let x = xs[0] ?? 1`, `(let x (?? (index xs 0) 1))`},
		{"type arguments", `json.parse[Config](t)`, `(call (. json parse)[Config] t)`},
		{"type arguments vs index", `xs[i](t)`, `(call (index xs i) t)`},
		{"method", `batch.push(item)`, `(call (. batch push) item)`},
		{"else handler", "let p = parse(s) else err -> {\n  0\n}", `(let p (else (call parse s) (err) (block 0)))`},
	})
}

func TestMaps(t *testing.T) {
	runParseCases(t, []parseCase{
		{"literal", `let m = Map{"a": 1, "b": 2}`, `(let m (map ("a" 1) ("b" 2)))`},
		{"empty", `let m = Map{}`, `(let m (map))`},
		{"across lines", "let m = Map{\n  \"a\": 1,\n  \"b\": 2,\n}", `(let m (map ("a" 1) ("b" 2)))`},
		{"set an entry", `stock["plums"] = 2`, `(= (index stock "plums") 2)`},
		{"look up", `let n = stock["figs"] ?? 0`, `(let n (?? (index stock "figs") 0))`},
		{"Map type", "fn f(m: Map[Str, Int]) { }", `(fn f ((m Map[Str, Int])) (block))`},
	})
}
