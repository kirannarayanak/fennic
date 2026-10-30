# Fennic

A general-purpose programming language that keeps complexity in the compiler
and out of your code. It reads close to Python, checks close to Rust, and
ships as one static binary with no build configuration.

```
fn greet(who: Str) -> Str {
  "Hello, {who}"
}

let adults = users
  |> filter(u -> u.age >= 18)
  |> sort_by(u -> u.name)

let cfg = load("app.json") else Config.default()
```

**Status: M3.** Fennic is a tree-walking interpreter with a type checker
that runs before anything else: a program with a type error does not run.
All twenty programs of [`docs/m0-spec.md`](docs/m0-spec.md) run as
written. Expect the language to change.

## Install

Fennic needs [Go](https://go.dev/dl/) 1.24 or newer to build. It has no
other dependencies.

```bash
go install github.com/kirannarayanak/fennic/cmd/fennic@latest
```

This puts `fennic` in `$(go env GOPATH)/bin`. If your shell cannot find
it, add that directory to your `PATH`:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

To build from a checkout instead:

```bash
git clone https://github.com/kirannarayanak/fennic.git
cd fennic
go build -o fennic ./cmd/fennic
```

## Usage

Fennic programs are files ending in `.fn`.

```bash
fennic run hello.fn           # run a program
fennic run tool.fn a b        # run it with command-line arguments
fennic test hello.fn          # run the program, then its test blocks
```

`fennic run` exits with status 0 on success and 1 if the program has an
error or stops on an unhandled failure. `fennic test` exits with 1 if any
test fails.

### A first program

Save this as `hello.fn`:

```
type User {
  name: Str
  age: Int
  email: Str?
}

fn greet(who: Str) -> Str {
  "Hello, {who}"
}

let users = [
  User { name: "Mira", age: 34, email: "mira@example.com" },
  User { name: "Tomas", age: 17, email: none },
  User { name: "Ada", age: 41, email: none },
]

users
  |> filter(u -> u.age >= 18)
  |> sort_by(u -> u.name)
  |> each(u -> print("{greet(u.name)} <{u.email ?? "no email"}>"))

let notes = fs.read("notes.txt") else "no notes yet"
print(notes)

test "greet uses the name" {
  assert greet("Kiran") == "Hello, Kiran"
}
```

```
$ fennic run hello.fn
Hello, Ada <no email>
Hello, Mira <mira@example.com>
no notes yet
```

`fennic test` runs the top-level code first, so its output appears before
the test results:

```
$ fennic test hello.fn
Hello, Ada <no email>
Hello, Mira <mira@example.com>
no notes yet
test "greet uses the name" ... ok

1 passed, 0 failed
```

### Errors

Every error says what is wrong, where, and what to write instead:

```
error: cannot assign to `limit` because it was declared with `let`
 --> check.fn:4:1
  |
4 | limit = limit + 1
  | ^^^^^
help: change line 1 to `var limit = 10` if `limit` needs to change
```

A file with several mistakes reports all of them, in order, before
anything runs. Type errors are caught the same way:

```
error: `print` needs a value to show, but this is a Str!, which might be a failure
 --> notes.fn:2:7
  |
2 | print(notes)
  |       ^^^^^
help: handle the failure first: `notes else ""` gives a fallback, and `notes!` passes it on
```

## The language in brief

| Feature | Looks like |
| --- | --- |
| Bindings | `let` is immutable, `var` is mutable |
| Functions | `fn area(w: Int, h: Int) -> Int { w * h }`; the last expression is the result |
| Strings | `"Hello, {name}"` interpolates any expression |
| Records | `type User { name: Str }`, built with `User { name: "Ada" }`, copied with `u with { name: "Bo" }` |
| Sum types | `type Shape = \| Circle(r: Float) \| Rect(w: Float, h: Float)`, taken apart with `match` |
| No null | `T?` is optional; `none`, `??` for a default, `?.` to chain |
| Errors are values | a failing function returns `T!`; `x!` passes the failure up, `x else fallback` handles it, `fail "why"` makes one |
| Control flow | `if a > b { a } else { b }` is an expression; `and`, `or`, `not` combine Bools |
| Loops | `for x in xs { ... }`, `for n in 1..10 { ... }` (both ends included), `total += n` |
| Patterns | `match` takes apart variants, tuples `(0, _)`, lists `[x, y]` and literals, and must cover every case |
| Maps | `Map{"apples": 3}`; `m[k]` gives an optional, `m[k] = v` sets an entry of a `var` |
| Entry point | `fn main()` or `fn main(args: List[Str])` runs after the top level |
| Pipelines | `xs \|> filter(x -> x > 0) \|> take(10)` |
| Interfaces | satisfied structurally; methods are `fn Type.name(self, ...)` |
| Concurrency | `together { xs \|> map_par(f) }!` runs tasks in a scope that cancels on failure; `spawn fn() { ... }` starts one; `Chan[T](n)` carries values between them |
| Tests | `test "name" { assert ... }` |

The [`examples/`](examples) directory has runnable programs, one or more per
feature, each with its expected output beside it. `examples/rejected` holds
programs the checker must refuse, and `examples/m0` runs the programs of the
spec as written.

Built-in functions: `print`, `len`, `first`, `last`, `sum`, `map`, `each`,
`filter`, `sort_by`, `sort`, `reduce`, `take`, `map_par`, `env`, `lower`,
`words`, `tally`, `top`, `group_by`, `map_values`, `sum_by`,
`sort_by_value`, `flatten`, `unique`, plus `fs.read`, `fs.write`,
`fs.list`, `json.parse`, `json.encode`, `csv.read`, `int.parse` and
`log.warn`. Lists have `.len()` and `.push(x)`; strings and maps have
`.len()`. Backtick strings, `` `{"a": 1}` ``, have no
escapes and no interpolation.

## Development

```bash
go test ./...
```

This runs the lexer, parser, type checker and interpreter tests, runs the
twenty programs in `examples/` against their expected output, and checks
that the twenty in `examples/rejected/` are refused with exactly the error
beside each.

| Directory | Contents |
| --- | --- |
| `token`, `lexer` | source text to tokens, with line and column on each |
| `ast`, `parser` | the syntax tree, and a parser that recovers from errors |
| `types` | the type checker; the rules are in [`docs/types.md`](docs/types.md) |
| `interp` | the tree-walking interpreter, built-ins and test runner |
| `diag` | error messages and how they are printed |
| `cli`, `cmd/fennic` | the `fennic` command |
| `examples` | the acceptance suite, with programs that must be refused in `examples/rejected` |

[`CLAUDE.md`](CLAUDE.md) records the design rules and the decisions made so
far.
