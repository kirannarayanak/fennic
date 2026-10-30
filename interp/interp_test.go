package interp

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fennic/diag"
	"fennic/parser"
)

// run executes src and returns what it printed, plus the error message and
// help if it stopped with one.
func run(t *testing.T, src string) (out, msg, help string) {
	t.Helper()
	file, diags := parser.Parse(src)
	if len(diags) == 0 {
		diags = Check(file, src)
	}
	if len(diags) > 0 {
		return "", diags[0].Msg, diags[0].Help
	}
	var buf bytes.Buffer
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hi"), 0o644)
	in := New(file, src, dir, &buf, &buf)
	if err := in.Run(); err != nil {
		re := err.(*RuntimeError)
		return buf.String(), re.Diag.Msg, re.Diag.Help
	}
	return buf.String(), "", ""
}

type runCase struct {
	name string
	src  string
	want string // printed output, or the error message when it should fail
}

// runCases checks programs that should succeed: want is their output, with
// a trailing newline added.
func runCases(t *testing.T, cases []runCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, msg, _ := run(t, c.src)
			if msg != "" {
				t.Fatalf("stopped with: %s", msg)
			}
			if out != c.want+"\n" {
				t.Errorf("\nsrc:\n%s\ngot:\n%s\nwant:\n%s", c.src, out, c.want)
			}
		})
	}
}

type errCase struct {
	name string
	src  string
	msg  string
	help string
}

// errCases checks programs that should stop: msg and help must match exactly.
func errCases(t *testing.T, cases []errCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, msg, help := run(t, c.src)
			if msg != c.msg || help != c.help {
				t.Errorf("\nsrc:\n%s\ngot:\n  %s\n  help: %s\nwant:\n  %s\n  help: %s", c.src, msg, help, c.msg, c.help)
			}
		})
	}
}

func TestArithmetic(t *testing.T) {
	runCases(t, []runCase{
		{"int", `print("{1 + 2 * 3 - 4}")`, "3"},
		{"int division truncates", `print("{7 / 2} {-7 / 2} {7 % 3}")`, "3 -3 1"},
		{"float", `print("{7.0 / 2.0} {0.1 + 0.2}")`, "3.5 0.30000000000000004"},
		{"float keeps a point", `print("{2.0 * 3.0}")`, "6.0"},
		{"unary minus", `print("{-(2 + 3)}")`, "-5"},
		{"comparisons", `print("{1 < 2} {2 <= 2} {3 > 4} {4 >= 5} {1 == 1} {1 != 1}")`, "true true false false true false"},
		{"string comparison", `print("{"apple" < "pear"}")`, "true"},
	})
	errCases(t, []errCase{
		{"mixing int and float", `let x = 1 + 2.0`, "`+` cannot mix an Int and a Float",
			"write the Int as a Float, like `2.0` instead of `2`; Fennic never converts numbers for you"},
		{"adding strings", `let s = "a" + "b"`, "`+` adds numbers, not strings", `to join strings, interpolate them: "{a}{b}"`},
		{"division by zero", `let x = 1 / 0`, "division by zero", "check the divisor before dividing"},
		{"comparing kinds", `let b = 1 < "x"`, "cannot compare an Int with a Str", "compare two Ints, two Floats or two Strs"},
	})
}

func TestBindingsAndScopes(t *testing.T) {
	runCases(t, []runCase{
		{"var changes", "var n = 1\nn = n + 1\nprint(\"{n}\")", "2"},
		{"shadowing", "let a = 1\nlet a = \"two\"\nprint(a)", "two"},
		{"block scope", "let f = x -> {\n  let a = x * 2\n  a\n}\nlet a = 5\nprint(\"{f(1)} {a}\")", "2 5"},
		{"closure capture", "let k = 10\nlet add = n -> n + k\nprint(\"{add(1)}\")", "11"},
		{"var changed inside a function body", "var total = 0\nfn bump() { total = total + 1 }\nbump()\nbump()\nprint(\"{total}\")", "2"},
	})
	errCases(t, []errCase{
		{"assign to let", "let a = 1\na = 2", "cannot assign to `a` because it was declared with `let`",
			"change line 1 to `var a = 1` if `a` needs to change"},
		{"assign to parameter", "fn f(x: Int) { x = 2 }", "cannot assign to `x` because it is a parameter",
			"make a changeable copy first: `var x_ = x`"},
		{"assign to undeclared", "y = 2", "`y` is not defined", "declare it first: `var y = ...`"},
		{"assign to function", "fn f() { 1 }\nf = 2", "cannot assign to `f` because it is a function",
			"make a changeable copy first: `var f_ = f`"},
		{"undefined name", `print(nme)`, "`nme` is not defined", "declare it with `let nme = ...` before this line, or check the spelling"},
	})
}

func TestDisplay(t *testing.T) {
	runCases(t, []runCase{
		{"top-level string unquoted", `print("{"x"}")`, "x"},
		{"strings in lists quoted", `print("{["a", "b\"c"]}")`, `["a", "b\"c"]`},
		{"nested list", `print("{[[1], []]}")`, "[[1], []]"},
		{"record", "type P {\n  x: Int\n  s: Str\n}\nprint(\"{P { x: 1, s: \"a\" }}\")", `P { x: 1, s: "a" }`},
		{"empty record", "type E {}\nprint(\"{E {}}\")", "E {}"},
		{"variant", "type S = | C(r: Float) | R(w: Float, h: Float)\nprint(\"{[C(1.0), R(2.0, 3.0)]}\")", "[C(1.0), R(2.0, 3.0)]"},
		{"none and bool", `print("{none} {true}")`, "none true"},
	})
	errCases(t, []errCase{
		{"print needs a Str", `print(42)`, "`print` needs a Str, but was given an Int", `turn it into a Str with interpolation: print("{x}")`},
	})
}

func TestFunctionsAndMethods(t *testing.T) {
	runCases(t, []runCase{
		{"hoisting", "print(f())\nfn f() -> Str { \"late\" }", "late"},
		{"no result", "fn f() { 1 }\nprint(\"{f()}\")", "()"},
		{"type function", "type C {\n  n: Int\n}\nfn C.make() -> C { C { n: 3 } }\nprint(\"{C.make().n}\")", "3"},
		{"method", "type C {\n  n: Int\n}\nfn C.twice(self) -> Int { self.n * 2 }\nprint(\"{C { n: 4 }.twice()}\")", "8"},
		{"method on variant", "type S = | A(n: Int)\nfn S.get(self) -> Int { match self {\n  A(n) -> n\n} }\nprint(\"{A(5).get()}\")", "5"},
		{"method value", "type C {\n  n: Int\n}\nfn C.show(self) -> Str { \"{self.n}\" }\nlet f = C { n: 1 }.show\nprint(f())", "1"},
	})
	errCases(t, []errCase{
		{"wrong arity", "fn f(a: Int) -> Int { a }\nf(1, 2)", "`f` takes 1 argument, but was given 2", "write `f(a)`"},
		{"method without value", "type C {\n  n: Int\n}\nfn C.get(self) -> Int { self.n }\nC.get()",
			"`C.get` is a method and needs a value to work on", "call it on a value, like `x.get(...)`"},
		{"calling a non-function", "let x = 1\nx()", "this is a Int, not a function, so it cannot be called", ""},
		{"calling a record type", "type C {\n  n: Int\n}\nC(1)", "`C` is a record type, not a function", "build a record with braces: `C { ... }`"},
	})
}

func TestRecordsAndWith(t *testing.T) {
	runCases(t, []runCase{
		{"field order follows declaration", "type P {\n  a: Int\n  b: Int\n}\nprint(\"{P { b: 2, a: 1 }}\")", "P { a: 1, b: 2 }"},
		{"with copies", "type P {\n  a: Int\n}\nlet p = P { a: 1 }\nlet q = p with { a: 2 }\nprint(\"{p.a} {q.a}\")", "1 2"},
		{"structural equality", "type P {\n  a: Int\n}\nprint(\"{P { a: 1 } == P { a: 1 }} {P { a: 1 } == P { a: 2 }}\")", "true false"},
	})
	errCases(t, []errCase{
		{"missing field", "type P {\n  a: Int\n  b: Int\n}\nlet p = P { a: 1 }", "`P` is missing field `b`",
			"records have no defaults; add `b: ...` (use `none` if it is optional)"},
		{"unknown field", "type P {\n  a: Int\n}\nlet p = P { a: 1, c: 2 }", "`P` has no field `c`", "its fields are a"},
		{"unknown type", "let p = Q { a: 1 }", "there is no record type `Q`", "declare it with `type Q { ... }`"},
		{"variant built with braces", "type S = | C(r: Float)\nlet c = C { r: 1.0 }", "`C` is a variant, not a record",
			"build a variant with parentheses: `C(r)`"},
		{"reading a missing field", "type P {\n  a: Int\n}\nlet x = P { a: 1 }.b", "a P has no field or method `b`", "its fields are a"},
		{"with on a non-record", "let x = 1 with { a: 2 }", "`with` copies a record, but this is not one", ""},
	})
}

func TestOptionals(t *testing.T) {
	runCases(t, []runCase{
		{"default used", `print(none ?? "d")`, "d"},
		{"default skipped", `print("v" ?? "d")`, "v"},
		{"default is lazy", `print("v" ?? undefined_name)`, "v"},
		{"chain stops at none", "type U {\n  n: Str\n}\ntype T {\n  u: U?\n}\nlet t = T { u: none }\nprint(t.u?.n ?? \"nobody\")", "nobody"},
		{"first and last", `print("{first([]) ?? 0} {last([1, 2]) ?? 0}")`, "0 2"},
	})
	errCases(t, []errCase{
		{"dot on none", "type T {\n  u: Str?\n}\nlet n = none\nlet x = n.u", "this value is none, so it has no `u`",
			"use `?.u` to skip a missing value, or `??` to give a default"},
	})
}

func TestMatch(t *testing.T) {
	runCases(t, []runCase{
		{"binds fields", "type S = | C(r: Int) | R(w: Int, h: Int)\nlet a = match R(2, 3) {\n  C(r) -> r\n  R(w, h) -> w * h\n}\nprint(\"{a}\")", "6"},
	})
	errCases(t, []errCase{
		{"no arm", "type S = | C(r: Int) | R(w: Int)\nlet x = match R(1) {\n  C(r) -> r\n}", "no arm matches `R`", "add an arm: `R(w) -> ...`"},
		{"wrong bind count", "type S = | R(w: Int, h: Int)\nlet x = match R(1, 2) {\n  R(w) -> w\n}", "`R` has 2 fields, but the pattern names 1", "write `R(w, h)`"},
		{"not a variant", "let x = match 3 {\n  A(n) -> n\n}", "`match` takes apart a sum type, but this is an Int", ""},
		{"constructor arity", "type S = | R(w: Int, h: Int)\nlet r = R(1)", "`R` takes 2 values, but was given 1", "write `R(w, h)`"},
	})
}

func TestFailures(t *testing.T) {
	runCases(t, []runCase{
		{"read succeeds", `print(fs.read("hello.txt")!)`, "hi"},
		{"or default", `print(fs.read("nope.txt") or "fallback")`, "fallback"},
		{"or handler", `print(fs.read("nope.txt") or e -> "handled: {e}")`, `handled: cannot read "nope.txt": file not found`},
		{"or skipped on success", `print(fs.read("hello.txt") or "fallback")`, "hi"},
		{"propagates through functions", "fn a() -> Str! { b()! }\nfn b() -> Str! { fs.read(\"nope.txt\")! }\nprint(a() or e -> \"{e}\")",
			`cannot read "nope.txt": file not found`},
		{"failure is a value until handled", "let f = fs.read(\"nope.txt\")\nprint(\"later: {f}\")", `later: cannot read "nope.txt": file not found`},
		{"json", `let v = json.parse("\{\"a\": [1, 2.5, \"s\", true, null]}")!` + "\n" + `print("{v.a}")`, `[1, 2.5, "s", true, none]`},
		{"json error", `print(json.parse("[1,]") or e -> "{e}")`, `invalid JSON at 1:4: unexpected "]"`},
		{"json incomplete", `print(json.parse("[1") or e -> "{e}")`, "invalid JSON: the text ends before the value is complete"},
	})
	errCases(t, []errCase{
		{"uncaught at top level", `let t = fs.read("nope.txt")!`, `cannot read "nope.txt": file not found`,
			"`!` passes a failure to the caller, but top-level code has no caller. Handle it here with `or`, for example `fs.read(\"nope.txt\") or \"\"`"},
		{"uncaught from a function has no type hint", "fn f() -> Str! { fs.read(\"nope.txt\")! }\nlet t = f()!", `cannot read "nope.txt": file not found`,
			"`!` passes a failure to the caller, but top-level code has no caller. Handle it here with `or`, for example `f() or <fallback value>`"},
	})
}

func TestPipelinesAndBuiltins(t *testing.T) {
	runCases(t, []runCase{
		{"pipe passes first argument", `print("{[3, 1, 2] |> take(2)}")`, "[3, 1]"},
		{"sort_by is stable", `print("{["bb", "a", "cc", "d"] |> sort_by(s -> len(s))}")`, `["a", "d", "bb", "cc"]`},
		{"take more than length", `print("{[1] |> take(5)}")`, "[1]"},
		{"filter map sum", `print("{[1, 2, 3, 4] |> filter(n -> n % 2 == 0) |> map(n -> n * 10) |> sum()}")`, "60"},
		{"sum of floats", `print("{sum([0.5, 0.25])}")`, "0.75"},
		{"sum of empty", `print("{sum([])}")`, "0"},
		{"len of str counts characters", `print("{len("héllo")}")`, "5"},
		{"each", `[1, 2] |> each(n -> print("n{n}"))`, "n1\nn2"},
	})
	errCases(t, []errCase{
		{"filter needs bool", `let x = [1] |> filter(n -> n)`, "filter's function must return a Bool", "give filter a function that returns a Bool, like `x -> x > 0`"},
		{"len of int", `let x = len(3)`, "`len` needs a List or a Str, but was given an Int", ""},
		{"take negative", `let x = [1] |> take(-1)`, "take needs a count that is an Int of 0 or more", "write a count, like `take(10)`"},
		{"sort mixed keys", `let x = [1, "a"] |> sort_by(v -> v)`, "sort_by keys must all be one kind, but found a Str and an Int", "return the same kind of key for every element"},
		{"sort unorderable keys", `let x = [[1], [2]] |> sort_by(v -> v)`, "sort_by cannot order List keys", "return an Int, a Float or a Str from the key function"},
		{"map_par outside together", `let x = [1] |> map_par(n -> n)`, "map_par spawns tasks, so it must run inside a `together` block",
			"wrap it: `together { xs |> map_par(f) }!`"},
	})
}

func TestTogether(t *testing.T) {
	runCases(t, []runCase{
		{"order kept", `print("{together { [5, 4, 3, 2, 1] |> map_par(n -> n * n) }!}")`, "[25, 16, 9, 4, 1]"},
		{"the failing task is reported", `print("{together { ["hello.txt", "hello.txt", "x.txt", "hello.txt"] |> map_par(f -> fs.read(f)!) } or e -> "{e}"}")`,
			`cannot read "x.txt": file not found`},
		{"real failure beats cancellation", `print("{together { ["hello.txt", "x.txt", "hello.txt"] |> map_par(f -> fs.read(f)!) } or e -> "{e}"}")`,
			`cannot read "x.txt": file not found`},
		{"failure inside the block", `print(together { fs.read("nope.txt")! } or e -> "caught")`, "caught"},
	})
	errCases(t, []errCase{
		{"runtime error in a task", `let x = together { [1, 0] |> map_par(n -> 1 / n) }!`, "division by zero", "check the divisor before dividing"},
	})
}

// With two tasks failing on their own, either may be the cause, but the
// result is always one of their real errors, never a cancellation.
func TestTogetherSeveralFailures(t *testing.T) {
	for i := 0; i < 50; i++ {
		out, msg, _ := run(t, `print(together { ["a.txt", "b.txt", "c.txt"] |> map_par(f -> fs.read(f)!) } or e -> "{e}")`)
		if msg != "" {
			t.Fatal(msg)
		}
		switch out {
		case "cannot read \"a.txt\": file not found\n", "cannot read \"b.txt\": file not found\n", "cannot read \"c.txt\": file not found\n":
		default:
			t.Fatalf("got %q", out)
		}
	}
}

func TestAssertAtTopLevel(t *testing.T) {
	errCases(t, []errCase{
		{"comparison", "assert 1 + 1 == 3", "assertion failed: left is 2, right is 3", "outside a `test` block a failed assert stops the program"},
		{"strings quoted", `assert "a" == "b"`, `assertion failed: left is "a", right is "b"`, "outside a `test` block a failed assert stops the program"},
		{"plain condition", "assert 1 > 2", "assertion failed: the condition is false", "outside a `test` block a failed assert stops the program"},
		{"not a bool", "assert 1", "assert needs a Bool, but this is a Int", "assert a comparison, like `assert x == 3`"},
	})
}

func TestTestBlocks(t *testing.T) {
	src := strings.Join([]string{
		`fn add(a: Int, b: Int) -> Int { a + b }`,
		`let base = 10`,
		`test "passes" {`,
		`  assert add(1, 2) == 3`,
		`}`,
		`test "sees top level" {`,
		`  assert base == 10`,
		`}`,
		`test "fails" {`,
		`  assert add(2, 2) == 5`,
		`}`,
		`test "propagated failure fails" {`,
		`  let t = fs.read("nope.txt")!`,
		`}`,
	}, "\n")
	file, diags := parser.Parse(src)
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	var buf bytes.Buffer
	in := New(file, src, t.TempDir(), &buf, &buf)
	if err := in.Run(); err != nil {
		t.Fatal(err)
	}
	results := in.RunTests()
	want := []struct{ name, msg string }{
		{"passes", ""},
		{"sees top level", ""},
		{"fails", "assertion failed: left is 4, right is 5"},
		{"propagated failure fails", `cannot read "nope.txt": file not found`},
	}
	if len(results) != len(want) {
		t.Fatalf("got %d results, want %d", len(results), len(want))
	}
	for i, w := range want {
		r := results[i]
		msg := ""
		if r.Diag != nil {
			msg = r.Diag.Msg
		}
		if r.Name != w.name || msg != w.msg {
			t.Errorf("result %d: got %q %q, want %q %q", i, r.Name, msg, w.name, w.msg)
		}
	}
	report, failed := Report(results, func(d diag.Diagnostic) string { return "  " + d.Msg + "\n" })
	if failed != 2 || !strings.HasSuffix(report, "\n2 passed, 2 failed\n") {
		t.Errorf("report:\n%s", report)
	}
}
