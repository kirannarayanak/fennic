package types

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirannarayanak/fennic/parser"
)

func check(t *testing.T, src string) []string {
	t.Helper()
	file, diags := parser.Parse(src)
	if len(diags) > 0 {
		t.Fatalf("does not parse: %s", diags[0].Msg)
	}
	ds, _ := Check(file, src)
	var out []string
	for _, d := range ds {
		out = append(out, d.Msg+" | "+d.Help)
	}
	return out
}

type typeCase struct {
	name string
	src  string
	want string // "" when the program is fine, else "message | help" of the first error
}

func runTypeCases(t *testing.T, cases []typeCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := check(t, c.src)
			if c.want == "" {
				if len(got) > 0 {
					t.Errorf("unexpected error:\n  %s\nsrc:\n%s", strings.Join(got, "\n  "), c.src)
				}
				return
			}
			if len(got) == 0 {
				t.Fatalf("no error, want:\n  %s\nsrc:\n%s", c.want, c.src)
			}
			if got[0] != c.want {
				t.Errorf("\ngot:  %s\nwant: %s\nsrc:\n%s", got[0], c.want, c.src)
			}
		})
	}
}

const shapes = "type Shape =\n  | Circle(r: Float)\n  | Rect(w: Float, h: Float)\n"
const user = "type User {\n  name: Str\n  age: Int\n  email: Str?\n}\n"

func TestNumbers(t *testing.T) {
	runTypeCases(t, []typeCase{
		{"int arithmetic", `let x = 1 + 2 * 3 % 4`, ""},
		{"float arithmetic", `let x = 1.5 * 2.0 - 0.5`, ""},
		{"negation", "let a = 2\nlet b = -a", ""},
		{"comparison", `let b = 1 < 2`, ""},
		{"string comparison", `let b = "a" < "b"`, ""},
		{"mixing", `let x = 1 + 2.0`, "`+` cannot mix an Int and a Float | write the Int as a Float, like `2.0` instead of `2`; Fennic never converts numbers for you"},
		{"float modulo", `let x = 1.0 % 2.0`, "`%` needs two Ints | `%` works on Ints"},
		{"adding strings", `let s = "a" + "b"`, "`+` adds numbers, not strings | to join strings, interpolate them: \"{a}{b}\""},
		{"comparing kinds", `let b = 1 < "x"`, "cannot compare an Int with a Str | compare two Ints, two Floats or two Strs"},
		{"equality across kinds", `let b = 1 == "x"`, "cannot compare an Int with a Str using `==` | compare two values of the same type"},
		{"negating a string", `let s = -"a"`, "`-` needs an Int or a Float, but this is a Str | "},
		{"arithmetic on an optional", "let a = first([1])\nlet b = a + 1", "`+` needs numbers, but this is an Int?, which might be none | give a default for when it is missing: `a ?? 0`"},
	})
}

func TestBindingsAndFunctions(t *testing.T) {
	runTypeCases(t, []typeCase{
		{"var keeps its type", "var n = 0\nn = n + 1", ""},
		{"var changes type", "var n = 0\nn = \"x\"", "`n` holds an Int, so it cannot be set to a Str | keep `n` an Int, or give the Str its own name with `let`"},
		{"shadowing may change type", "let a = 1\nlet a = \"x\"", ""},
		{"call", "fn f(a: Int) -> Int { a }\nlet x = f(1) + 1", ""},
		{"hoisted", "let x = f(1)\nfn f(a: Int) -> Int { a }", ""},
		{"argument", "fn f(a: Int) -> Int { a }\nlet x = f(\"1\")", "`f` needs an Int for `a`, but this is a Str | pass an Int"},
		{"argument to a Str parameter", "fn f(s: Str) { print(s) }\nf(3)", "`f` needs a Str for `s`, but this is an Int | to turn a value into a Str, interpolate it: \"{3}\""},
		{"arity", "fn f(a: Int) -> Int { a }\nlet x = f(1, 2)", "`f` takes 1 argument, but was given 2 | write `f(a)`"},
		{"result", "fn f() -> Int { \"x\" }", "`f` returns an Int, but this is a Str | end the body with an Int, or change the result type to `-> Str`"},
		{"result missing", "fn f() -> Int { let a = 1 }", "`f` returns an Int, but its body ends without a value | end the body with a value of type Int"},
		{"no result type ignores the last value", "fn f() { 1 }", ""},
		{"global used in function", "let limit = 3\nfn f() -> Int { limit }", ""},
		{"undefined", `print(nme)`, "`nme` is not defined | declare it with `let nme = ...` before this line, or check the spelling"},
		{"unknown type", "fn f(a: Nope) { }", "there is no type `Nope` | declare it with `type Nope { ... }`, or check the spelling"},
		{"double optional", "fn f(a: Int??) { }", "`Int??` is optional twice | write `Int?` once; a value is either there or not"},
		{"optional after failable", "fn f() -> Int!? { 1 }", "`Int!?` is not a type | put `?` before `!`: `Int?!`"},
		{"calling a non-function", "let x = 1\nlet y = x(2)", "this is an Int, not a function, so it cannot be called | "},
		{"builtin as a value", "let p = print", "`print` is built in, so it can only be called, not passed around | call it: `print(...)`"},
		{"assert needs a bool", "assert 1", "assert needs a Bool, but this is an Int | assert a comparison, like `assert x == 3`"},
	})
}

func TestStrings(t *testing.T) {
	runTypeCases(t, []typeCase{
		{"interpolate anything", user + "let u = User { name: \"a\", age: 1, email: none }\nprint(\"{u} {u.email} {[1]} {1.5}\")", ""},
		{"interpolate a failable", "let t = fs.read(\"x\")\nprint(\"{t}\")", "a Str! cannot go in a string until its failure is handled | handle the failure first: `t else \"\"` gives a fallback, and `t!` passes it on"},
		{"interpolate nothing", "fn f() { }\nprint(\"{f()}\")", "this gives nothing, so there is nothing to put in the string | interpolate a value instead"},
		{"print shows any value", "print(42)\nprint([1])\nprint(none)", ""},
		{"print cannot show a function", "fn f() { }\nprint(f)", "a function cannot go in a string | call the function and interpolate its result"},
		{"len of Str", `let n = len("abc") + 1`, ""},
		{"len of Int", `let n = len(3)`, "`len` needs a List or a Str, but this is an Int | "},
	})
}

func TestRecords(t *testing.T) {
	runTypeCases(t, []typeCase{
		{"literal", user + `let u = User { name: "a", age: 1, email: "e" }`, ""},
		{"optional field given none", user + `let u = User { name: "a", age: 1, email: none }`, ""},
		{"field types", user + `let u = User { name: "a", age: "1", email: none }`, "`User` field `age` is an Int, but this is a Str | give `age` an Int"},
		{"missing field", user + `let u = User { name: "a", age: 1 }`, "`User` is missing field `email` | records have no defaults; add `email: ...` (use `none` if it is optional)"},
		{"unknown field", user + `let u = User { name: "a", age: 1, email: none, mail: "x" }`, "`User` has no field `mail` | did you mean `email`?"},
		{"field twice", user + `let u = User { name: "a", name: "b", age: 1, email: none }`, "field `name` is given twice | remove one of them"},
		{"field access", user + "let u = User { name: \"a\", age: 1, email: none }\nlet n = u.age + 1", ""},
		{"unknown field access", user + "let u = User { name: \"a\", age: 1, email: none }\nlet n = u.agee", "`User` has no field `agee` | did you mean `age`?"},
		{"no close name", user + "let u = User { name: \"a\", age: 1, email: none }\nlet n = u.height", "`User` has no field `height` | its fields are name, age, email"},
		{"with", user + "let u = User { name: \"a\", age: 1, email: none }\nlet v = u with { age: 2 }", ""},
		{"with wrong type", user + "let u = User { name: \"a\", age: 1, email: none }\nlet v = u with { age: \"2\" }", "`User` field `age` is an Int, but this is a Str | give `age` an Int"},
		{"with unknown field", user + "let u = User { name: \"a\", age: 1, email: none }\nlet v = u with { agee: 2 }", "`User` has no field `agee` | did you mean `age`?"},
		{"records are nominal", "type A {\n  x: Int\n}\ntype B {\n  x: Int\n}\nfn f(a: A) { }\nf(B { x: 1 })", "`f` needs an A for `a`, but this is a B | pass an A"},
		{"variant built as record", shapes + "let c = Circle { r: 1.0 }", "`Circle` is a variant, not a record | build a variant with parentheses: `Circle(r)`"},
		{"record called like a function", user + "let u = User(1)", "`User` is a record type, not a function | build a record with braces: `User { ... }`"},
	})
}

func TestOptionals(t *testing.T) {
	team := user + "type Team {\n  lead: User?\n}\n"
	runTypeCases(t, []typeCase{
		{"value fits optional", team + "let u = User { name: \"a\", age: 1, email: none }\nlet t = Team { lead: u }", ""},
		{"default", user + "let u = User { name: \"a\", age: 1, email: none }\nlet e = u.email ?? \"none\"\nprint(e)", ""},
		{"chain", team + "let t = Team { lead: none }\nlet n = t.lead?.name ?? \"nobody\"\nprint(n)", ""},
		{"chain flattens", team + "let t = Team { lead: none }\nlet e = t.lead?.email ?? \"x\"\nprint(e)", ""},
		{"right-associative defaults", team + "let t = Team { lead: none }\nprint(t.lead?.email ?? t.lead?.email ?? \"x\")", ""},
		{"dot through optional", team + "let t = Team { lead: none }\nlet n = t.lead.name", "`t.lead` is a User?, so it might be none and `.name` cannot be read directly | use `?.` and give a default: `t.lead?.name ?? \"\"`"},
		{"optional where value needed", user + "fn shout(s: Str) { }\nlet u = User { name: \"a\", age: 1, email: none }\nshout(u.email)", "`shout` needs a Str for `s`, but this is a Str?, which might be none | give a default for when it is missing: `u.email ?? \"\"`"},
		{"default not needed", `let n = 1 ?? 2`, "`1` is an Int, which is never none, so `??` is not needed | remove `?? 2`"},
		{"default of wrong type", `let n = first([1]) ?? "x"`, "the default after `??` must be an Int, but this is a Str | give a default of the same type as the value"},
		{"chain not needed", user + "let u = User { name: \"a\", age: 1, email: none }\nlet n = u?.name", "`u` is a User, which is never none, so `?.` is not needed | use `.name`"},
		{"none takes its type from use", "let n = none\nfn f(a: Int?) { }\nf(n)", ""},
	})
}

func TestFailures(t *testing.T) {
	runTypeCases(t, []typeCase{
		{"propagate in failable function", "fn f() -> Str! { fs.read(\"x\")! }", ""},
		{"body may end in T", "fn f() -> Int! { 1 }", ""},
		{"body may end in T!", "fn f() -> Str! { fs.read(\"x\") }", ""},
		{"propagate at top level", `let t = fs.read("x")!`, ""},
		{"propagate in a test", "test \"t\" {\n  let x = fs.read(\"x\")!\n}", ""},
		{"propagate in a lambda", `let ps = ["a"] |> map(f -> fs.read(f)!)`, ""},
		{"failable lambda makes map failable", "let ps = [\"a\"] |> map(f -> fs.read(f)!)\nprint(\"{len(ps)}\")", "`len` needs a List or a Str, but this is a List[Str]!, which might be a failure | handle the failure first: `ps else []` gives a fallback, and `ps!` passes it on"},
		{"handled failable map", "let ps = [\"a\"] |> map(f -> fs.read(f)!) else []\nprint(\"{len(ps)}\")", ""},
		{"propagate where nothing can fail", "fn f() -> Str { fs.read(\"x\")! }", "`!` passes a failure on, but `f` cannot fail | change its result type to `-> Str!`, or handle the failure here with `else`"},
		{"propagate a value that cannot fail", "let n = len(\"a\")!", "`len(\"a\")` cannot fail, so there is nothing for `!` to pass on | remove the `!`"},
		{"else", `let t = fs.read("x") else "none"`, ""},
		{"else handler", "let t = fs.read(\"x\") else err -> { log.warn(\"{err}\"); \"none\" }\nprint(t)", ""},
		{"else fallback type", `let t = fs.read("x") else 3`, "the fallback after `else` must be a Str, but this is an Int | give a fallback of the same type as the value"},
		{"else on a value that cannot fail", `let n = len("a") else 0`, "`len(\"a\")` cannot fail, so `else` never runs | remove `else` and its fallback"},
		{"unhandled where a value is needed", "let t = fs.read(\"x\")\nprint(t)", "`print` needs a value to show, but this is a Str!, which might be a failure | handle the failure first: `t else \"\"` gives a fallback, and `t!` passes it on"},
		{"field of a failable", "type C {\n  n: Int\n}\nfn load() -> C! { C { n: 1 } }\nlet n = load().n", "`load()` is a C!, which might be a failure, so `.n` cannot be read yet | handle the failure first: `load()!` passes it on, and `else` gives a fallback"},
	})
}

func TestMatch(t *testing.T) {
	runTypeCases(t, []typeCase{
		{"exhaustive", shapes + "fn area(s: Shape) -> Float {\n  match s {\n    Circle(r) -> r * r\n    Rect(w, h) -> w * h\n  }\n}", ""},
		{"missing variant", shapes + "fn area(s: Shape) -> Float {\n  match s {\n    Circle(r) -> r * r\n  }\n}", "this `match` does not handle `Rect` | add an arm: `Rect(w, h) -> ...`"},
		{"several missing", "type T = | A(x: Int) | B(x: Int) | C(x: Int)\nfn f(t: T) -> Int {\n  match t {\n    A(x) -> x\n  }\n}", "this `match` does not handle `B` or `C` | add arms: `B(x) -> ...`, `C(x) -> ...`"},
		{"unknown variant", shapes + "fn f(s: Shape) -> Int {\n  match s {\n    Circle(r) -> 0\n    Rect(w, h) -> 1\n    Tri(a) -> 2\n  }\n}", "`Shape` has no variant `Tri` | its variants are Circle and Rect"},
		{"bind count", shapes + "fn f(s: Shape) -> Float {\n  match s {\n    Circle(r) -> r\n    Rect(w) -> w\n  }\n}", "`Rect` has 2 fields, but the pattern names 1 | write `Rect(w, h)`"},
		{"twice", shapes + "fn f(s: Shape) -> Int {\n  match s {\n    Circle(r) -> 0\n    Circle(r) -> 1\n    Rect(w, h) -> 2\n  }\n}", "this arm can never run: `Circle` is already handled on line 6 | remove it, or move it above line 6"},
		{"arms differ", shapes + "let k = match Circle(1.0) {\n  Circle(r) -> \"c\"\n  Rect(w, h) -> 1\n}", "this arm gives an Int, but the first arm gives a Str | make every arm of a `match` give the same type"},
		{"bound fields have their types", shapes + "let k = match Circle(1.0) {\n  Circle(r) -> r + 1\n  Rect(w, h) -> w\n}", "`+` cannot mix an Int and a Float | write the Int as a Float, like `2.0` instead of `2`; Fennic never converts numbers for you"},
		{"not a sum type", "let k = match 3 {\n  A(x) -> x\n}", "this `match` does not handle every value | add `_ -> ...` as the last arm to handle the rest"},
		{"constructor arguments", shapes + "let c = Circle(1)", "`Circle` needs a Float for `r`, but this is an Int | pass a Float"},
		{"constructor arity", shapes + "let c = Rect(1.0)", "`Rect` takes 2 values, but was given 1 | write `Rect(w, h)`"},
	})
}

func TestMethodsAndInterfaces(t *testing.T) {
	sink := "interface Sink { fn write(self, line: Str) }\n"
	runTypeCases(t, []typeCase{
		{"method", "type C {\n  n: Int\n}\nfn C.twice(self) -> Int { self.n * 2 }\nlet x = C { n: 1 }.twice() + 1", ""},
		{"type function", "type C {\n  n: Int\n}\nfn C.make() -> C { C { n: 0 } }\nlet c = C.make()", ""},
		{"method needs a value", "type C {\n  n: Int\n}\nfn C.get(self) -> Int { self.n }\nlet x = C.get()", "`C.get` is a method and needs a value to work on | call it on a value, like `x.get(...)`"},
		{"no such type function", "type C {\n  n: Int\n}\nlet x = C.make()", "`C` has no function `make` | declare it with `fn C.make(...)`"},
		{"method argument", "type C {\n  n: Int\n}\nfn C.add(self, k: Int) -> Int { self.n + k }\nlet x = C { n: 1 }.add(\"2\")", "`add` needs an Int for `k`, but this is a Str | pass an Int"},
		{"method on an unknown type", "fn Nope.f(self) { }", "there is no type `Nope` to add a method to | declare it with `type Nope { ... }`"},
		{"satisfied structurally", sink + "type Con {}\nfn Con.write(self, line: Str) { print(line) }\nfn r(s: Sink) { s.write(\"x\") }\nr(Con {})", ""},
		{"extra methods are fine", sink + "type Con {}\nfn Con.write(self, line: Str) { }\nfn Con.other(self) { }\nfn r(s: Sink) { }\nr(Con {})", ""},
		{"missing method", sink + "type Con {}\nfn r(s: Sink) { }\nr(Con {})", "`Con` cannot be used as a `Sink`: it has no method `write` | add `fn Con.write(self, line: Str) { ... }`"},
		{"wrong parameter", sink + "type Con {}\nfn Con.write(self, line: Int) { }\nfn r(s: Sink) { }\nr(Con {})", "`Con` cannot be used as a `Sink`: its `write` takes `line: Int`, but `Sink` needs `line: Str` | change it to `fn Con.write(self, line: Str)`"},
		{"wrong arity", sink + "type Con {}\nfn Con.write(self) { }\nfn r(s: Sink) { }\nr(Con {})", "`Con` cannot be used as a `Sink`: its `write` takes 0 arguments, but `Sink` needs 1 | change it to `fn Con.write(self, line: Str)`"},
		{"wrong result", "interface M { fn area(self) -> Int }\ntype Sq {}\nfn Sq.area(self) -> Float { 1.0 }\nfn r(m: M) { }\nr(Sq {})", "`Sq` cannot be used as a `M`: its `area` returns a Float, but `M` needs an Int | change it to `fn Sq.area(self) -> Int`"},
		{"interface method call", sink + "fn r(s: Sink) { s.write(1) }", "`write` needs a Str, but this is an Int | to turn a value into a Str, interpolate it: \"{1}\""},
		{"interface has no such method", sink + "fn r(s: Sink) { s.flush() }", "a Sink has no field or method `flush` | "},
	})
}

func TestGenerics(t *testing.T) {
	largest := "fn largest[T: Ord](items: List[T]) -> T? {\n  items |> sort_by(x -> x) |> last()\n}\n"
	runTypeCases(t, []typeCase{
		{"inferred", largest + "let n = largest([1, 2]) ?? 0\nlet s = largest([\"a\"]) ?? \"\"", ""},
		{"inferred later", largest + "let n = largest([]) ?? 0", ""},
		{"result type follows", largest + "let n = (largest([1]) ?? 0) + 1.0", "`+` cannot mix an Int and a Float | write the Int as a Float, like `2.0` instead of `2`; Fennic never converts numbers for you"},
		{"Ord not satisfied", largest + "type P {\n  x: Int\n}\nlet p = largest([P { x: 1 }])", "`largest` needs `T: Ord`, but here `T` is `P`, which cannot be ordered | only Int, Float and Str can be ordered; to order records, sort by a field: `sort_by(p -> p.x)`"},
		{"Ord decided later", largest + "type P {\n  x: Int\n}\nlet p = largest([]) ?? P { x: 1 }", "`largest` needs `T: Ord`, but here `T` is `P`, which cannot be ordered | only Int, Float and Str can be ordered; to order records, sort by a field: `sort_by(p -> p.x)`"},
		{"unconstrained", "fn first_or[T](items: List[T], d: T) -> T { first(items) ?? d }\nlet x = first_or([1], 0) + 1", ""},
		{"arguments must agree", "fn first_or[T](items: List[T], d: T) -> T { first(items) ?? d }\nlet x = first_or([1], \"a\")", "`first_or` needs an Int for `d`, but this is a Str | pass an Int"},
		{"type parameters are rigid", "fn f[T](x: T) -> Int { x + 1 }", "`+` needs two numbers, but got a T and an Int | "},
		{"interface constraint", "interface Named { fn name(self) -> Str }\ntype D {}\nfn D.name(self) -> Str { \"d\" }\nfn hi[T: Named](x: T) -> Str { x.name() }\nlet s = hi(D {})", ""},
		{"interface constraint not satisfied", "interface Named { fn name(self) -> Str }\ntype D {}\nfn hi[T: Named](x: T) -> Str { x.name() }\nlet s = hi(D {})", "`hi` needs `T: Named`, but here `T` is `D`: it has no method `name` | add `fn D.name(self) -> Str { ... }`"},
		{"not an interface", "fn f[T: Int](x: T) { }", "`Int` is not an interface, so it cannot constrain a type parameter | use `Ord` or the name of an interface"},
	})
}

func TestListsAndPipelines(t *testing.T) {
	runTypeCases(t, []typeCase{
		{"pipeline", user + "let us = [User { name: \"a\", age: 1, email: none }]\nlet names = us |> filter(u -> u.age >= 18) |> sort_by(u -> u.name) |> take(10) |> map(u -> u.name)\nprint(\"{names}\")", ""},
		{"lambda parameter from the list", "let xs = [1, 2] |> map(n -> n + 1.0)", "`+` cannot mix an Int and a Float | write the Int as a Float, like `2.0` instead of `2`; Fennic never converts numbers for you"},
		{"mixed list", `let xs = [1, "a"]`, "this list holds Ints, but this element is a Str | make every element an Int"},
		{"list of optionals", "let xs = [1, none]", ""},
		{"empty list from use", "let xs = []\nlet n = (first(xs) ?? 0) + 1", ""},
		{"filter needs a Bool", `let xs = [1] |> filter(n -> n)`, "filter's function must return a Bool, but this one returns an Int | give filter a function that returns a Bool, like `x -> x > 0`"},
		{"sort key must be ordered", "let xs = [[1]] |> sort_by(v -> v)", "sort_by needs a key that can be ordered, but this one is a List[Int] | return an Int, a Float or a Str from the key function"},
		{"sum of strings", `let n = sum(["a"])`, "`sum` adds up Ints or Floats, but this list holds Strs | "},
		{"not a list", `let n = first(3)`, "`first` needs a List, but this is an Int | "},
		{"take count", `let xs = [1] |> take("2")`, "`take` needs an Int, but this is a Str | write a count, like `take(10)`"},
		{"lambda value", "let inc = n -> n + 1\nlet x = inc(2) + inc(3)", ""},
		{"lambda value used at one type", "let inc = n -> n + 1\nlet x = inc(2.0)", "`inc` needs an Int, but this is a Float | pass an Int"},
		{"lambda that needs context", "let f = u -> u.name", "cannot tell what type `u` is, so `.name` cannot be checked | use it where its type is known, such as the argument of `map` or `filter`"},
		{"map_par outside together", `let xs = [1] |> map_par(n -> n)`, "map_par spawns tasks, so it must run inside a `together` block | wrap it: `together { xs |> map_par(f) }!`"},
		{"map_par inside together", "let xs = together { [1] |> map_par(n -> n * 2) }!\nlet n = sum(xs)", ""},
		{"together is failable", "let xs = together { [1] |> map_par(n -> n) }\nlet n = sum(xs)", "`sum` needs a List, but this is a List[Int]!, which might be a failure | handle the failure first: `xs else []` gives a fallback, and `xs!` passes it on"},
	})
}

func TestJSON(t *testing.T) {
	cfg := "type Config {\n  name: Str\n  port: Int\n}\n"
	runTypeCases(t, []typeCase{
		{"from the result type", cfg + "fn load(t: Str) -> Config! { json.parse(t)! }", ""},
		{"from a parameter", cfg + "fn use(c: Config) { }\nuse(json.parse(\"\") else Config { name: \"\", port: 0 })", ""},
		{"from a pipe", cfg + "fn load(t: Str) -> Config! { t |> json.parse() }", ""},
		{"lists of records", cfg + "fn load(t: Str) -> List[Config]! { json.parse(t)! }", ""},
		{"no expected type", "let c = json.parse(\"\\{}\")!", "cannot tell what type `json.parse` should produce here | parse inside a function whose result type says it, like `fn load(text: Str) -> Config! { json.parse(text)! }`"},
		{"sum types have no JSON form", shapes + "fn load(t: Str) -> Shape! { json.parse(t)! }", "`json.parse` cannot produce a Shape: sum types have no JSON form | parse into a record, a List, a Map with Str keys, a Str, an Int, a Float, a Bool or a Date"},
	})
}

// Several mistakes in one program are all reported, in source order.
func TestSeveralErrors(t *testing.T) {
	got := check(t, "let a = 1 + 2.0\nlet b = \"x\" + \"y\"\nlet c = len(3)")
	if len(got) != 3 {
		t.Fatalf("got %d errors, want 3:\n%s", len(got), strings.Join(got, "\n"))
	}
	for i, prefix := range []string{"`+` cannot mix", "`+` adds numbers", "`len` needs a List or a Str"} {
		if !strings.HasPrefix(got[i], prefix) {
			t.Errorf("error %d: got %q, want it to start with %q", i, got[i], prefix)
		}
	}
}

// Deleting any one line of an example gives a program that is often wrong.
// The checker must report, never crash.
func TestDamagedExamplesDoNotCrash(t *testing.T) {
	files, _ := filepath.Glob("../examples/*.fn")
	more, _ := filepath.Glob("../examples/rejected/*.fn")
	files = append(files, more...)
	if len(files) < 40 {
		t.Fatalf("found %d programs, want at least 40", len(files))
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(data), "\n")
		for i := range lines {
			src := strings.Join(append(append([]string{}, lines[:i]...), lines[i+1:]...), "\n")
			file, diags := parser.Parse(src)
			if len(diags) > 0 {
				continue
			}
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("%s without line %d: panic: %v", filepath.Base(f), i+1, r)
					}
				}()
				Check(file, src)
			}()
		}
	}
}

func TestControlFlowTypes(t *testing.T) {
	runTypeCases(t, []typeCase{
		{"if expression", `let x = if 1 > 0 { "a" } else { "b" }` + "\nprint(x)", ""},
		{"else if chain", "fn f(n: Int) -> Str {\n  if n < 0 { \"neg\" } else if n == 0 { \"zero\" } else { \"pos\" }\n}", ""},
		{"if without else gives nothing", "let x = if true { 1 }\nlet y = x + 1", "`+` needs two numbers, but got nothing and an Int | "},
		{"branches differ", `let x = if true { 1 } else { "a" }`, "this branch gives a Str, but the first branch gives an Int | make both branches of the `if` give the same type"},
		{"condition must be Bool", `if 1 { print("x") }`, "`if` needs a Bool, but this is an Int | compare it to make a Bool, like `x > 0`"},
		{"and or not", `let b = not (1 < 2 and 3 > 4) or false`, ""},
		{"and needs Bools", `let b = 1 and true`, "`and` needs a Bool, but this is an Int | compare it to make a Bool, like `x > 0`"},
		{"or on a failure suggests else", `let t = fs.read("x") or ""`, "`or` combines two Bools, but this is a Str!, which might be a failure | to give a fallback for a failure, write `else`: `fs.read(\"x\") else \"\"`"},
		{"for over a list", "for x in [1, 2] {\n  print(\"{x + 1}\")\n}", ""},
		{"for over a range", "var t = 0\nfor n in 1..10 {\n  t += n\n}", ""},
		{"for over a number", "for n in 3 { }", "`for` goes over a List or a channel, but this is an Int | go over a List or a range, like `for n in 1..10`"},
		{"range of floats", `let r = 1.0..2.0`, "`..` makes a range of Ints, but this is a Float | "},
		{"plus equals keeps the type", "var s = 0\ns += 1.5", "`+` cannot mix an Int and a Float | write the Int as a Float, like `2.0` instead of `2`; Fennic never converts numbers for you"},
		{"underscore cannot be read", "for _ in [1] {\n  print(_)\n}", "`_` stands for a value that is not used, so it cannot be read | give the value a name to use it"},
	})
}

func TestFailExpr(t *testing.T) {
	runTypeCases(t, []typeCase{
		{"in a failable function", "fn f(n: Int) -> Int! {\n  if n < 0 { fail \"negative\" }\n  n\n}", ""},
		{"as a branch value", "fn f(n: Int) -> Int! {\n  if n < 0 { fail \"negative\" } else { n }\n}", ""},
		{"as a match arm", "fn f(n: Int) -> Str! {\n  match n {\n    0 -> fail \"zero\"\n    _ -> \"ok\"\n  }\n}", ""},
		{"where nothing can fail", "fn f(n: Int) -> Int {\n  fail \"no\"\n}", "`fail` stops `f` with a failure, but `f` cannot fail | change its result type to `-> Int!`"},
		{"in a lambda makes it failable", "let xs = [1] |> map(n -> if n > 0 { n } else { fail \"neg\" })\nlet n = len(xs)", "`len` needs a List or a Str, but this is a List[Int]!, which might be a failure | handle the failure first: `xs else []` gives a fallback, and `xs!` passes it on"},
		{"message must be a Str", "fn f() -> Int! { fail 3 }", "`fail` needs a Str, but this is an Int | to turn a value into a Str, interpolate it: \"{3}\""},
		{"in main", "fn main() {\n  fail \"stop\"\n}", ""},
		{"propagate in main", "fn main() {\n  let t = fs.read(\"x\")!\n}", ""},
	})
}

func TestPatternTypes(t *testing.T) {
	status := "type Status =\n  | Queued\n  | Done(code: Int)\n"
	runTypeCases(t, []typeCase{
		{"literal inside a variant", status + "fn f(s: Status) -> Str {\n  match s {\n    Queued -> \"q\"\n    Done(0) -> \"ok\"\n    Done(c) -> \"{c}\"\n  }\n}", ""},
		{"literal arm does not cover the variant", status + "fn f(s: Status) -> Str {\n  match s {\n    Queued -> \"q\"\n    Done(0) -> \"ok\"\n  }\n}", "this `match` does not handle `Done` | add an arm: `Done(code) -> ...`"},
		{"variant value with no fields", status + "let s = Queued\nlet t = [Queued, Done(1)]", ""},
		{"tuple patterns", "let s = match (1, \"a\") {\n  (0, _) -> \"zero\"\n  (n, t) -> \"{n}{t}\"\n}", ""},
		{"tuple pattern of the wrong size", "let s = match (1, 2) {\n  (a, b, c) -> 0\n  _ -> 1\n}", "this pattern is a tuple of 3, but the value matched is a (Int, Int) | "},
		{"list patterns need a catch-all", "fn f(xs: List[Int]) -> Int {\n  match xs {\n    [] -> 0\n    [x] -> x\n  }\n}", "this `match` does not handle every value | add `_ -> ...` as the last arm to handle the rest"},
		{"list pattern types", "fn f(xs: List[Str]) -> Int {\n  match xs {\n    [x] -> x + 1\n    _ -> 0\n  }\n}", "`+` adds numbers, not strings | to join strings, interpolate them: \"{a}{b}\""},
		{"literal of the wrong type", "let s = match 3 {\n  \"a\" -> 1\n  _ -> 2\n}", "this pattern is a Str, but the value matched is an Int | "},
		{"bool covered by true and false", "let s = match 1 > 0 {\n  true -> 1\n  false -> 0\n}", ""},
		{"arm after a catch-all", "let s = match 3 {\n  _ -> 1\n  0 -> 2\n}", "this arm can never run: the arm on line 2 already matches everything | remove it, or move it above line 2"},
		{"unknown variant", status + "let s = match Queued {\n  Running(p) -> 1\n  _ -> 0\n}", "`Status` has no variant `Running` | its variants are Queued and Done"},
	})
}

func TestM3Calls(t *testing.T) {
	runTypeCases(t, []typeCase{
		{"lambda of two", "let add = (a, b) -> a + b\nlet n = add(1, 2) + 1", ""},
		{"reduce", "let m = [1, 2] |> reduce((a, b) -> if a > b { a } else { b })\nlet n = (m ?? 0) + 1", ""},
		{"reduce needs two values", "let m = [1] |> reduce(a -> a)", "`reduce` needs a function of two values | give it a function of two values, like `(a, b) -> a + b`"},
		{"sort", "let xs = [3, 1] |> sort()\nlet n = (first(xs) ?? 0) + 1", ""},
		{"sort needs ordered values", "type P {\n  x: Int\n}\nlet xs = [P { x: 1 }] |> sort()", "`sort` needs Ints, Floats or Strs, but this list holds Ps | sort by a key instead: `sort_by(x -> ...)`"},
		{"first with a predicate", "let x = [1, 2] |> first(n -> n > 1)\nlet y = (x ?? 0) + 1", ""},
		{"int.parse fails", "let n = int.parse(\"3\")\nlet m = n + 1", "`+` needs values, but this is an Int!, which might be a failure | handle the failure first: `n else 0` gives a fallback, and `n!` passes it on"},
		{"env is optional", "let p = env(\"PORT\") ?? \"8080\"", ""},
		{"len method", "let n = [1, 2].len() + \"ab\".len()", ""},
		{"push", "var xs = []\nxs.push(1)\nlet n = (first(xs) ?? 0) + 1", ""},
		{"push of the wrong type", "var xs = [1]\nxs.push(\"a\")", "this list holds Ints, so `.push` needs an Int, but this is a Str | push an Int"},
		{"index", "let xs = [\"a\"]\nlet x = xs[0] ?? \"\"", ""},
		{"index needs an Int", "let xs = [1]\nlet x = xs[\"a\"]", "a list index needs an Int, but this is a Str | pass an Int"},
		{"index of a non-list", "let x = 3[0]", "only a List or a Map can be indexed with `[...]`, but this is an Int | "},
		{"explicit type arguments", "fn pair[T](a: T, b: T) -> List[T] { [a, b] }\nlet xs = pair[Str](\"a\", \"b\")", ""},
		{"explicit type arguments are checked", "fn pair[T](a: T, b: T) -> List[T] { [a, b] }\nlet xs = pair[Str](1, 2)", "`pair` needs a Str for `a`, but this is an Int | to turn a value into a Str, interpolate it: \"{1}\""},
		{"too many type arguments", "fn id[T](a: T) -> T { a }\nlet x = id[Int, Str](1)", "`id` takes 1 type argument, but was given 2 | give one type for each of T"},
		{"json.parse with a type argument", "type C {\n  a: Int\n}\nlet c = json.parse[C](`{\"a\": 1}`) else C { a: 0 }", ""},
		{"anonymous function", "let f = fn() { print(\"x\") }\nf()", ""},
		{"anonymous function with a result", "let f = fn(n: Int) -> Int { n * 2 }\nlet x = f(2) + 1", ""},
	})
}

func TestMainSignature(t *testing.T) {
	runTypeCases(t, []typeCase{
		{"no arguments", "fn main() { }", ""},
		{"command-line arguments", "fn main(args: List[Str]) {\n  print(args[0] ?? \".\")\n}", ""},
		{"wrong arguments", "fn main(n: Int) { }", "`main` takes no arguments, or the command-line arguments as a List[Str] | write `fn main()`, or `fn main(args: List[Str])` to receive the command-line arguments"},
		{"a result", "fn main() -> Int { 0 }", "`main` returns nothing | remove the result type; use `!` or `fail` inside main to stop with a failure"},
	})
}

func TestCollections(t *testing.T) {
	runTypeCases(t, []typeCase{
		{"map literal", "let m = Map{\"a\": 1}\nlet n = (m[\"a\"] ?? 0) + 1", ""},
		{"empty map from use", "var m = Map{}\nm[\"a\"] = 1\nlet n = (m[\"a\"] ?? 0) + 1", ""},
		{"map type written", "fn f(m: Map[Str, Int]) -> Int { m.len() }", ""},
		{"map keys must be simple", "type P {\n  x: Int\n}\nlet m = Map{P { x: 1 }: 1}", "map keys must be Str, Int or Bool, but these are Ps | use Str, Int or Bool keys"},
		{"map type keys", "fn f(m: Map[List[Int], Int]) { }", "map keys must be Str, Int or Bool, but this is a List[Int] | use Str, Int or Bool keys"},
		{"value of the wrong type", "var m = Map{\"a\": 1}\nm[\"b\"] = \"x\"", "this map's values are Ints, so the value must be an Int, but this is a Str | pass an Int"},
		{"tally and top", "let t = \"a b a\" |> words() |> tally() |> top(1)\nlet s = (first(t)?.key ?? \"\") \nlet n = (first(t)?.count ?? 0) + 1", ""},
		{"tally of records", "type P {\n  x: Int\n}\nlet t = [P { x: 1 }] |> tally()", "`tally` counts Strs, Ints or Bools, but this list holds Ps | tally a list of Strs, Ints or Bools, such as a key taken from each element with `map`"},
		{"group_by and map_values", "type S {\n  r: Str\n  a: Float\n}\nlet m = [S { r: \"w\", a: 1.0 }] |> group_by(s -> s.r) |> map_values(rs -> rs |> sum_by(s -> s.a))\nlet v = (m[\"w\"] ?? 0.0) + 1.0", ""},
		{"group_by key", "let m = [1] |> group_by(n -> [n])", "`group_by` needs a key that is a Str, an Int or a Bool, but this one is a List[Int] | group by a Str, an Int or a Bool, such as one field"},
		{"sum_by", "let s = [\"ab\"] |> sum_by(w -> w)", "`sum_by` adds up Ints or Floats, but this function gives a Str | return an Int or a Float from the function"},
		{"sort_by_value", "let e = Map{\"a\": 2} |> sort_by_value(desc)\nlet k = first(e)?.key ?? \"\"\nlet v = (first(e)?.value ?? 0) + 1", ""},
		{"sort_by_value needs an order", "let e = Map{\"a\": 2} |> sort_by_value(1)", "`sort_by_value` needs an order, but this is an Int | write `asc` or `desc`"},
		{"entry fields", "let e = Map{\"a\": 2} |> sort_by_value(asc) |> map(x -> x.count)", "an Entry[Str, Int] has no field `count` | its fields are key and value"},
		{"lower and words", "let w = \"A b\" |> lower() |> words()\nlet n = w.len()", ""},
		{"csv.read", "type R {\n  a: Int\n  b: Str?\n}\nlet rows = csv.read[R](\"x.csv\") else []\nlet n = rows.len()", ""},
		{"csv.read from context", "type R {\n  a: Int\n}\nfn load() -> List[R]! { csv.read(\"x.csv\") }", ""},
		{"csv.read needs a record", "let rows = csv.read[Int](\"x.csv\")", "`csv.read` reads rows into a record, but this is an Int | read into a record type, one field per column"},
		{"csv.read field types", "type R {\n  a: List[Int]\n}\nlet rows = csv.read[R](\"x.csv\")", "`csv.read` cannot fill field `a` of `R`: a CSV cell cannot become a List[Int] | give each column an Int, a Float, a Str, a Bool or a Date field, optional if it may be empty"},
	})
}

func TestConcurrencyTypes(t *testing.T) {
	runTypeCases(t, []typeCase{
		{"spawn in together", "together {\n  spawn fn() { print(\"x\") }\n}", ""},
		{"spawn a failing function", "together {\n  spawn fn() { fail \"x\" }\n}", ""},
		{"spawn outside together", "spawn fn() { }", "`spawn` starts a task, so it must run inside a `together` block | put it inside a `together { ... }` block, which waits for every task it starts"},
		{"spawn a value", "together {\n  spawn 3\n}", "`spawn` needs a function, but this is an Int | spawn a function, like `spawn fn() { ... }`"},
		{"channel", "let c = Chan[Int](1)\nc.send(1)\nc.close()\nfor n in c {\n  print(\"{n + 1}\")\n}", ""},
		{"channel type written", "fn feed(c: Chan[Str]) { c.send(\"x\") }", ""},
		{"channel size", "let c = Chan[Int](\"big\")", "`Chan` needs an Int, how many values it can hold, but this is a Str | pass an Int, like `Chan[Str](100)`"},
		{"channel method", "let c = Chan[Int](1)\nc.push(1)", "a channel has no method `push` | a channel has `.send(x)` and `.close()`; read it with `for x in ch`"},
		{"dates", "type V {\n  day: Date\n}\nlet vs = csv.read[V](\"v.csv\") else []\nlet days = vs |> map(v -> v.day) |> unique()", ""},
		{"fs.list fails", "let fs_ = fs.list(\".\", \"*\")\nlet n = fs_.len()", "`fs_` is a List[Str]!, which might be a failure, so `.len` cannot be read yet | handle the failure first: `fs_!` passes it on, and `else` gives a fallback"},
		{"fs.write fails", "fn main() {\n  fs.write(\"a\", \"b\")!\n}", ""},
		{"flatten needs lists", "let x = [1] |> flatten()", "`flatten` needs a list of lists, but this list holds Ints | "},
		{"json.encode of a map with Int keys", "print(json.encode(Map{1: \"a\"}))", "`json.encode` cannot encode a Map[Int, Str]: only maps with Str keys have a JSON form | encode records, lists, maps with Str keys, and plain values"},
	})
}
