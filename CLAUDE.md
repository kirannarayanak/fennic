# Fennic

A general-purpose programming language that keeps complexity in the compiler and out of the user's code. Reads close to Python, checks close to Rust, ships as one static binary with no build configuration.

Binary `fennic`, file extension `.fn`.

## The one rule

Every feature is judged on whether it makes the person writing Fennic code do less. Making the language more capable is not a reason to add anything.

No feature enters the language until three real programs already written needed it and worked around its absence.

This rule governs features that were never specified. It does not govern constructs that are in the M0 spec (`docs/m0-spec.md`) but were not built: those are unfinished work, and building them needs no evidence. The only legitimate input to the rule is the record kept for the three real programs of M3 step 4. Known candidate waiting on that evidence: function types, so a user's own function can take a lambda.

## Frozen decisions

These are settled. Do not reopen them, do not propose alternatives, do not add escape hatches.

| Decision | Choice |
| --- | --- |
| Memory | Tracing GC, inherited from the host language. No manual control, no `unsafe`, not ever |
| Errors | Values, not exceptions. A failing function returns `T!`. Call site writes `!` to propagate or `else` to handle |
| Concurrency | Green threads with structured scope. All I/O looks blocking. No async/await, no colored functions |
| Null | Does not exist. `T?` is an option type. `??` for defaults, `?.` for chaining |
| Mutability | `let` immutable, `var` mutable, immutable by default |
| Generics | In from day one, with inference and interface constraints |
| Inference | Full inside function bodies. Mandatory annotations on top-level function signatures |
| Layout | Braces and newlines, no semicolons. One formatter, zero options |
| Interfaces | Satisfied structurally. Nothing declares that it implements anything |

## Non-goals

1. No macros or compile-time code generation.
2. No inheritance or class hierarchies. Records hold data, interfaces describe behaviour, composition joins them.
3. No operator overloading. `+` is addition.
4. No implicit conversions. Every conversion is a visible call.
5. No manual memory control.

Deferred, not refused: a package registry. Dependencies are git URLs pinned to a commit.

## Syntax reference

```
let name = "Kiran"          // immutable
var count = 0               // mutable

fn greet(who: Str) -> Str {
  "Hello, {who}"            // last expression is the return value
}

let top = users
  |> filter(u -> u.age >= 18)
  |> sort_by(u -> u.name)
  |> take(10)

type User {
  name: Str
  age: Int
  email: Str?               // optional; there is no null
}

let older = u with { age: 32 }    // copy with change

type Shape =
  | Circle(radius: Float)
  | Rect(w: Float, h: Float)

match s {
  Circle(r)  -> 3.14159 * r * r
  Rect(w, h) -> w * h
}                           // non-exhaustive match is a compile error

fn load(path: Str) -> Config! {
  let text = fs.read(path)!        // propagate
  json.parse(text)!
}

let cfg = load("app.json") else Config.default()
let cfg = load("app.json") else err -> { log.warn("{err}"); Config.default() }

fn largest[T: Ord](items: List[T]) -> T? { ... }

interface Sink { fn write(self, line: Str) }
fn Console.write(self, line: Str) { print(line) }   // structural, no impl block

let pages = together {              // scopes its spawns, returns its last expression
  urls |> map_par(u -> http.get(u)!)
}!

test "greet uses the name" {
  assert greet("Kiran") == "Hello, Kiran"
}
```

Records have no field defaults. Use an explicit constructor such as `Config.default()`.

## M1 scope

A tree-walking interpreter in **Go**, single static binary, no dependencies outside the Go standard library.

Go is chosen because Fennic values become Go values (so Go's GC is Fennic's GC) and Fennic tasks become goroutines (so Go's scheduler is Fennic's green-thread runtime, and `together` is a wait group with a cancel context). Do not write a garbage collector. Do not write a scheduler.

M1 is **untyped**. The type checker is M2 and runs over the same AST. Do not start it early.

In scope: lexer, parser, AST, tree-walking interpreter, `fennic run <file>`, the built-in test runner.

Out of scope for M1: type checking, package manager, formatter, language server, bytecode VM, optimisation of any kind.

### Acceptance test

`examples/` holds twenty programs with expected output beside each. M1 is done when all twenty run and match. Nothing else counts as done.

## Build order

1. **Lexer.** Characters to tokens. Record line and column on every token. Every error message the language will ever print depends on this.
2. **Parser.** Define AST types first, then parse. Errors recover and continue rather than stopping at the first one, so a future language server can work on broken files.
3. **Interpreter.** Walk the AST, run the programs.
4. Stop. That is M1.

## Conventions

- Go standard library only. Every new dependency needs an argument.
- Table-driven tests, one table per language feature.
- `go test ./...` passes before anything is called done.
- Error messages name the fix, not the parse state. This is a product feature, not polish. A message that says what was expected and what to do instead is worth more than a faster interpreter.
- Commit messages: plain, no attribution lines.

## Task 0: the name

Done. The name is Fennic (from fennec, a small quick fox): free on crates.io, npm and PyPI at the time of choosing. GitHub owner names and trademark registers were not checkable from the build session and still need a manual check.

## M2: the type checker

`docs/types.md` holds the rules. Acceptance: the twenty programs in
`examples/` still pass, and the twenty in `examples/rejected/` each report
exactly the error in their `.out` file. `json.parse` takes the type it
builds from the expected type at the call (decided at the start of M2).

## M3: finish the language

The M0 spec had two halves. `CLAUDE.md` was condensed from the first (frozen decisions and a short syntax reference). The second, a one-page tour and twenty programs, is `docs/m0-spec.md` and was not available until M3. M1 and M2 were built from the first half only, which is why `if`, loops, `fail` and the collection operations were missing.

1. The diff, `docs/m3-diff.md`, lists every construct the spec uses that the language lacks. It is the M3 scope; nothing outside it enters M3.
2. The collisions it names are decided and recorded in the spec before building. Done: see "Decisions recorded at M3 step 2" in `docs/m0-spec.md`. The failure fallback is now `else`; `and`, `or`, `not` are boolean.
3. Each construct goes through lexer, parser, checker and interpreter, with passing examples and refused cases. Where a spec program cannot run as written, the spec changes and the commit says why.
4. Three real programs, kept and used, live outside `examples/`, each with a record of what it needed and could not have.

Acceptance: the twenty programs in `docs/m0-spec.md` run as written, with its amendments and the step 2 decisions applied.

Out of scope: formatter, language server, package manager, bytecode VM, and any type system change the diff does not demand. Leave alone: `let xs = []` passing with an undecided type.

When the implementation reports a construct missing from its source, first ask whether the source is complete.

## M4, for context

Tooling: error messages that name the fix, a formatter with no options, a language server. It comes after M3 step 4, because the error messages worth fixing are the ones that get in the way while writing real programs.

## Decisions made while writing the acceptance suite

The syntax reference left these open. They were approved along with `examples/`; `examples/README.md` has the harness conventions.

- Record literal: `User { name: "Ada", age: 41, email: none }`, commas between fields, trailing comma allowed. Type declarations keep newline-separated fields.
- `none` is the absent optional. A present value is written bare.
- List literals `[1, 2, 3]`. Variants are constructed positionally: `Circle(1.0)`.
- `;` separates statements on one line inside a block.
- Interpolated values display as source syntax (`Point { x: 3, y: 4 }`, `["a"]`); a top-level Str displays unquoted. Floats print shortest round-trip form and always keep a `.0`.
- Built-ins beyond the reference: `len`, `first`, `last` (both return `T?`), `sum`, `map`, `each`.
- `!` at top level stops the program with exit 1 and a diagnostic.
- `json.parse` returns an untyped record in M1. How M2 names the target type is open.
- `map_par` keeps input order and runs only inside `together`. The first task to fail cancels the others, and a failure is reported, never a cancellation. **Which failure is reported when several tasks fail is explicitly unspecified**: it depends on timing, and no program or test may depend on it. (Approved as "lowest index"; that cannot be guaranteed once cancelled tasks never run.)
- A function with no `->` result type returns nothing, whatever its last expression.
- `log.warn` writes `warn: <msg>` to stderr.
- `fennic test` evaluates the top level, then each test.
- Assigning to a `let` is caught before the program runs.
- `fs.read` paths resolve against the program file's directory.
- Diagnostics: `error:` line, ` --> file:line:col`, source line with carets, `help:` line naming the fix. Columns count characters, 1-based.
