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

let cfg = load("app.json") or Config.default()
```

**Status: M2.** Fennic is a tree-walking interpreter with a type checker
that runs before anything else: a program with a type error does not run.
Expect the language to change.

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
fennic run hello.fn     # run a program
fennic test hello.fn    # run the program, then its test blocks
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

let notes = fs.read("notes.txt") or "no notes yet"
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
error: `print` needs a Str, but this is a Str!, which might be a failure
 --> notes.fn:2:7
  |
2 | print(notes)
  |       ^^^^^
help: handle the failure first: `notes or ""` gives a fallback, and `notes!` passes it on
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
| Errors are values | a failing function returns `T!`; `x!` passes the failure up, `x or fallback` handles it |
| Pipelines | `xs \|> filter(x -> x > 0) \|> take(10)` |
| Interfaces | satisfied structurally; methods are `fn Type.name(self, ...)` |
| Concurrency | `together { xs \|> map_par(f) }!` runs tasks in a scope that cancels on failure |
| Tests | `test "name" { assert ... }` |

The [`examples/`](examples) directory has twenty runnable programs, one or
more per feature, each with its expected output beside it. They are the
acceptance suite for this milestone.

Built-in functions: `print`, `len`, `first`, `last`, `sum`, `map`, `each`,
`filter`, `sort_by`, `take`, `map_par`, plus `fs.read`, `json.parse` and
`log.warn`.

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
