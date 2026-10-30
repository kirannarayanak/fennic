# Fennic M0 spec: the language in one page, and the twenty programs

## What this file is, and why it is arriving late

This is the missing half of the M0 spec. It was written before any Go existed and it is the document `CLAUDE.md` was condensed from. It was never handed over, so M1 and M2 were built from `CLAUDE.md` alone, which contains the frozen decisions and a shortened syntax reference but none of the programs below.

That is the whole explanation for the gap list. The suite in `examples/` was not narrowed to fit the implementation. It was written from a source that did not contain `if`, `for ... in`, `fail`, list patterns or the collection operations, because those constructs live only in this file.

Naming: this document predates the rename. Brisk is now Fennic, and the file extension `.bk` is now `.fn`. Nothing else changes.

Scope: only the two sections below contain language constructs, so only these two are authoritative for the M3 diff. The frozen decisions and non-goals in `CLAUDE.md` are unchanged and are not repeated here.

## The language in one page

The whole surface, in the order a newcomer meets it. One file is one module, and the last expression in a function body is its return value.

```
// bindings
let name = "Kiran"        // immutable
var count = 0             // mutable

// functions, with interpolation
fn greet(who: Str) -> Str {
  "Hello, {who}"
}

// pipelines, the main way data moves
let top = users
  |> filter(u -> u.age >= 18)
  |> sort_by(u -> u.name)
  |> take(10)

// records
type User {
  name: Str
  age: Int
  email: Str?        // optional, there is no null
}

// sum types and exhaustive matching
type Shape =
  | Circle(radius: Float)
  | Rect(w: Float, h: Float)

fn area(s: Shape) -> Float {
  match s {
    Circle(r) -> 3.14159 * r * r
    Rect(w, h) -> w * h
  }
}

// failure is in the type, and visible at the call site
fn load(path: Str) -> Config! {
  let text = fs.read(path)!          // propagate upward
  json.parse(text)!
}

let cfg = load("app.json") else Config.default()   // or handle it here

// generics with constraints
fn largest[T: Ord](items: List[T]) -> T? {
  items |> reduce((a, b) -> if a > b { a } else { b })
}

// interfaces describe behaviour, nothing inherits
interface Named {
  fn label(self) -> Str
}

// concurrency: looks blocking, is not
let pages = together {
  paths |> map_par(p -> fs.read(p)!)
}

// tests live next to the code and need no framework
test "greet uses the name" {
  assert greet("Kiran") == "Hello, Kiran"
}
```

Note that the `largest` body uses `if ... else` as an expression. `CLAUDE.md` elided that body to `{ ... }`, which is how `if` went missing.

## Twenty programs

No compiler existed behind any of these. They were written by hand to find out whether the language reads better than what it replaces.

### 1. Hello

```
fn main() {
  print("Hello, world")
}
```

### 2. Bindings and interpolation

```
fn main() {
  let name = "Kiran"
  var greetings = 0
  print("Hello, {name}")
  greetings += 1
}
```

### 3. Control flow, with tuple patterns

```
fn main() {
  for n in 1..100 {
    print(match (n % 3, n % 5) {
      (0, 0) -> "FizzBuzz"
      (0, _) -> "Fizz"
      (_, 0) -> "Buzz"
      _      -> "{n}"
    })
  }
}
```

### 4. Lists and pipelines

```
fn main() {
  let nums = [3, 1, 4, 1, 5, 9, 2, 6]
  let big = nums
    |> filter(n -> n > 3)
    |> map(n -> n * 10)
    |> sort()
  print(big)       // [40, 50, 60, 90]
}
```

### 5. Maps, where lookup is honest

```
fn main() {
  var stock = Map{"apples": 3, "pears": 7}
  stock["plums"] = 2
  let n = stock["figs"] ?? 0     // lookup returns Int?, never crashes
  print("figs: {n}")
}
```

### 6. Records, copied rather than mutated

```
type User {
  name: Str
  age: Int
}

fn main() {
  let u = User{name: "Asha", age: 31}
  let older = u with { age: 32 }
  print("{older.name} is {older.age}")
}
```

### 7. Optionals instead of null

```
fn find(users: List[User], name: Str) -> User? {
  users |> first(u -> u.name == name)
}

fn main() {
  let age = find(users, "Asha")?.age ?? 0
  print("age: {age}")
}
```

### 8. Sum types and exhaustive match

```
type Status =
  | Queued
  | Running(pid: Int)
  | Done(code: Int)

fn describe(s: Status) -> Str {
  match s {
    Queued       -> "waiting"
    Running(pid) -> "running as {pid}"
    Done(0)      -> "finished cleanly"
    Done(c)      -> "failed with {c}"
  }
}
```

Leave out a case and it does not compile. The error names the case you missed.

### 9. Failure in the signature

```
fn parse_port(s: Str) -> Int! {
  let n = int.parse(s)!
  if n < 1 or n > 65535 {
    fail "port out of range: {n}"
  }
  n
}
```

### 10. Failure handled at the call site

```
fn main() {
  let port = parse_port(env("PORT") ?? "8080") else err -> {
    log.warn("{err}, falling back to 8080")
    8080
  }
  serve(port)
}
```

### 11. Read a file and count words

```
fn main() {
  let text = fs.read("notes.txt") else ""
  text
    |> lower()
    |> words()
    |> tally()
    |> top(5)
    |> each(e -> print("{e.key}\t{e.count}"))
}
```

### 12. Generics

```
fn chunk[T](items: List[T], size: Int) -> List[List[T]] {
  var out = []
  var batch = []
  for item in items {
    batch.push(item)
    if batch.len() == size {
      out.push(batch)
      batch = []
    }
  }
  if batch.len() > 0 { out.push(batch) }
  out
}
```

### 13. Interfaces, satisfied structurally

```
interface Sink {
  fn write(self, line: Str)
}

type Console {}

fn Console.write(self, line: Str) {
  print(line)
}

fn report(out: Sink, rows: List[Str]) {
  rows |> each(r -> out.write(r))
}
```

Nothing declares that `Console` implements `Sink`. It has the method, so it qualifies.

### 14. Tests, with no framework to install

```
test "chunk splits evenly and keeps the remainder" {
  assert chunk([1, 2, 3, 4, 5], 2) == [[1, 2], [3, 4], [5]]
  assert chunk([], 3) == []
}
```

### 15. A command line tool

```
fn main(args: List[Str]) {
  match args {
    [path]         -> tally(path, 10)
    [path, n]      -> tally(path, int.parse(n) else 10)
    _              -> print("usage: tally <file> [n]")
  }
}
```

### 16. JSON into a typed record

```
type Config {
  host: Str
  port: Int
  debug: Bool
}

fn load(path: Str) -> Config! {
  json.parse[Config](fs.read(path)!)!
}
```

If the file is missing a field or has the wrong type, the failure names the field and the line.

### 17. Group and summarise a CSV

```
type Sale {
  region: Str
  amount: Float
}

fn main() {
  let totals = csv.read[Sale]("sales.csv")!
    |> group_by(s -> s.region)
    |> map_values(rows -> rows |> sum_by(s -> s.amount))

  totals
    |> sort_by_value(desc)
    |> each(e -> print("{e.key}\t{e.value}"))
}
```

### 18. Parallel work, scoped

```
fn main() {
  let bodies = together {
    paths |> map_par(p -> fs.read(p)!)
  }!
  print("read {bodies.len()} pages")
}
```

The `together` block does not finish until every task inside it does. One failure cancels the rest and surfaces as the block's failure. No task can outlive its scope.

### 19. A worker pool over a channel

```
fn main() {
  let jobs = Chan[Str](100)

  together {
    for _ in 1..8 {
      spawn fn() {
        for path in jobs {
          save(fs.read(path) else "")
        }
      }
    }
    for path in paths { jobs.send(path) }
    jobs.close()
  }
}
```

### 20. A whole small program

```
type Row {
  day: Date
  region: Str
  amount: Float
}

type Summary {
  total: Float
  days: Int
}

fn main(args: List[Str]) {
  let dir = args[0] ?? "."

  let rows = together {
    fs.list(dir, "*.csv")! |> map_par(f -> csv.read[Row](f)!)
  }! |> flatten()

  let summary = rows
    |> filter(r -> r.amount > 0.0)
    |> group_by(r -> r.region)
    |> map_values(rs -> Summary {
         total: rs |> sum_by(r -> r.amount),
         days: rs |> map(r -> r.day) |> unique() |> len(),
       })

  fs.write("summary.json", json.encode(summary))!
  print("wrote {summary.len()} regions")
}
```

That last one is the benchmark. If it is not clearly nicer than the Python equivalent, with its try blocks, its thread pool and its missing types, the language has no reason to exist.

## Amendments already decided since this was written

Apply these when diffing. They are decisions, not open questions.

1. Record field defaults are cut. Program 16 loses its `= false`. Use an explicit constructor such as `Config.default()`.
2. `together` both scopes and returns. It scopes its spawns and evaluates to its last expression like any other block. Program 18 returns a value because `map_par` is its last expression; program 19 returns nothing because its body ends in a statement.
3. Chapter and verse on names: Brisk is Fennic, `.bk` is `.fn`.

## Three collisions the diff must resolve

These are real conflicts between this document and what now exists. None has a decided answer yet.

1. `or` is overloaded. Program 9 uses it as a boolean operator (`n < 1 or n > 65535`) and programs 10, 11 and 15 use it as the failure-fallback keyword. Both are in this spec. One of them has to move.
2. `.push` meets immutability. Program 12 calls `.push` on `var` bindings. The spec never says whether `.push` mutates in place or returns a new list, and `let` lists are immutable.
3. `http.get` is referenced but unimplemented, and programs 18, 19 and 20 depend on network or filesystem operations that an offline acceptance run cannot perform as written.

## Decisions recorded at M3 step 2

Made after the M3 diff (`docs/m3-diff.md`, section D). The program texts
above already include the changes marked "applied".

1. **`or` is boolean; the failure fallback is `else`.** `and`, `or` and
   `not` are the boolean operators. A failure is handled with
   `x else fallback` or `x else err -> handler`. Applied to the tour and
   programs 10, 11, 15 and 19.
2. **`.push`** changes a `var` list in place. On a `let` list it is a
   compile error.
3. **`http.get` is cut.** Applied: the tour and programs 18 and 19 read
   local files with `fs.read` instead of fetching URLs, so they still show
   scoping, cancellation and a worker pool without a network. Program 20
   never used `http.get`.
4. **`and` and `not`** come in with decision 1, although no program uses
   them.
5. **Type arguments may be explicit or come from context.**
   `json.parse[Config](text)` and `json.parse(text)` where the expected
   type says `Config` both work; `Chan[Str](100)` needs the explicit form.
6. **`print` takes any value** that interpolation accepts: anything but a
   failable or a function.
7. **Fragments run with a preamble.** Each acceptance file is the program
   text from this document verbatim, plus a separate preamble that supplies
   only what is missing: undefined names, the program it depends on, and a
   `main` or a call where there is none.
8. **Program 20 uses a record, not a map, for each summary.** Its
   `Map{"total": ..., "days": ...}` held a Float and an Int, which a typed
   map cannot. Applied: `type Summary { total: Float  days: Int }`.
9. **`a..b` includes both ends.** `1..100` is one to a hundred.
10. **Methods on built-in types.** The `len` function stays, and `.len()`,
    `.push`, `.send` and `.close` are methods on the built-in types.
11. **Raw strings use backticks.** `` `{"port": 80}` `` has no escapes and
    no interpolation, so JSON can be written as it is.

Amendment 1 is also applied above: program 16's `debug` field has no
default.
12. **`fs.list` can fail**, like `fs.read`: a missing directory is a
    failure, not an empty list. Applied: program 20 writes
    `fs.list(dir, "*.csv")!`.
13. **Program 20 compares Floats with a Float.** It wrote
    `r.amount > 0`, comparing a Float with an Int, which the type checker
    refuses because implicit conversion is a non-goal. Applied: `0.0`.
