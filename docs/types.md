# M2: the type checker

The checker runs over the M1 syntax tree after parsing and before anything
runs. If it reports a problem, `fennic run` and `fennic test` print every
problem and run nothing. It adds no syntax: everything below is checked
from what programs already write.

`examples/` (all twenty must still pass) and `examples/rejected/` (twenty
programs the checker must refuse, each with its expected error) are the
acceptance suite.

## Types

| Type | Written | Values |
| --- | --- | --- |
| Int, Float, Str, Bool | `Int` | `1`, `1.0`, `"a"`, `true` |
| nothing | (no `->` on a function) | what `print` and `each` return |
| list | `List[T]` | `[1, 2]` |
| optional | `T?` | a `T`, or `none` |
| failable | `T!` | a `T`, or a failure |
| record | `type User { ... }` | `User { ... }` |
| sum type | `type Shape = \| ...` | `Circle(1.0)` |
| interface | `interface Sink { ... }` | any value whose type has the methods |
| type parameter | `T` in `fn f[T](...)` | |
| error | (no name to write) | `err` in `else err -> ...` |
| tuple | (no name to write) | `(1, "a")` |
| map | `Map[K, V]` | `Map{"a": 1}` |
| channel | `Chan[T]` | `Chan[Str](100)` |
| date | `Date` | a calendar day, read from CSV or JSON as `YYYY-MM-DD` |
| map entry | (no name to write) | what `sort_by_value` and `top` give: `.key` and `.value`, or `.key` and `.count` |

Records and sum types are nominal: two types with the same fields are
different types. Interfaces are structural.

`?` and `!` each appear at most once, `!` outermost: `Str?!` is a Str that
might be missing and might fail. `Str??`, `Str!!` and `Str!?` are errors.

Functions have types inside the checker (a lambda is `(Int) -> Int`), but
there is no syntax to write one, so a top-level function cannot take a
function as a parameter. Only the built-ins do. This waits for the
three-programs rule.

## Inference

- Top-level function signatures are written in full: every parameter has a
  type, and no `->` means the function returns nothing.
- Inside bodies everything is inferred. `let` and `var` take the type of
  their value. A lambda's parameter types come from where it is used.
- An empty list `[]` and `none` take their element type from use. A list
  literal containing `none` is a list of optionals: `[1, none]` is a
  `List[Int?]`.
- Expected types flow inward: a function's result type is the expected type
  of its last expression, a parameter's type is the expected type of its
  argument. This is how `json.parse` knows what to build.
- A lambda bound with `let` has one type. It cannot be used at two types.

## Rules

**Numbers.** Arithmetic needs two Ints or two Floats; `%` needs Ints. `+`
never joins strings. Comparisons `< <= > >=` need two values of the same
`Ord` type: Int, Float or Str. `==` and `!=` need two values of the same
type, and neither may be a function or a failable.

**Optionals.** A `T` is accepted where a `T?` is expected. `none` is a `T?`.
`a ?? b` needs `a: T?` and `b: T`, and is a `T`. `x?.f` on `x: T?` is the
field's type made optional (`Str?` stays `Str?`). Reading `x.f` when `x`
might be none, or passing a `T?` where a `T` is needed, is an error that
names `?.` or `??`.

**Failures.** A call to a function returning `T!` is a `T!`. It must be
handled before its value is used:

- `x!` is a `T`. It is allowed in a function whose result is failable, in
  a lambda (which makes the lambda's result failable), in a `together`
  block, in a `test`, and at the top level (where it stops the program).
  Anywhere else it is an error naming the result type to change.
- `x else y` needs `y: T` and is a `T`. `x else err -> body` binds `err`
  to the error and needs `body: T`.
- `!` or `else` on something that cannot fail is an error.
- `fail "message"` stops with a failure. It is allowed where `!` is, and
  has whatever type its place needs, so it can be a branch or an arm.
- Using the boolean `or` on a failure is reported with the fix: `else`.
- Storing a `T!` in a `let` is allowed; using it as a `T` is not.

A failable function's body may end in a `T` or a `T!`.

**Records.** A literal gives every field exactly once with the declared
type. `with` names existing fields with their declared types. Reading a
field that does not exist suggests the nearest name.

**Sum types and match.** The value matched must be a sum type. Each arm
names one of its variants and binds every field. Every variant must have an
arm, and no variant may have two. All arms give the same type.

**Methods.** `fn T.m(self, ...)` gives `self` the type `T`. `x.m()` finds
the method by `x`'s type. `T.f()` calls a function declared without `self`.

**Interfaces.** A type satisfies an interface when it has every listed
method with the same parameter types and result type. Extra methods do not
matter. A value may be passed where an interface is expected only if its
type satisfies it; the error names the missing or mismatched method.

**Generics.** Type parameters are inferred at each call by unification. A
constraint `[T: I]` is checked once `T` is known. `Ord` is built in and
holds for Int, Float and Str; any other constraint is an interface.

**Concurrency.** `together { ... }` has the block's type made failable.
`map_par` and `spawn` are allowed only inside `together`, without leaving
the function they are in. `spawn f` starts `f`, a function of no
arguments, as a task; it gives nothing. The block waits for every task it
started; the first task to fail cancels the others, and its failure is the
block's. A runtime error in a task stops the program. `Chan[T](n)` makes a
channel holding up to `n` values; `.send(x)` and `.close()` are its
methods, and `for x in ch` receives until it is closed and empty. Sending
and receiving give up when the task is cancelled.

**Strings.** Interpolation accepts any value except a failable or a
function. `print` and `log.warn` take a Str. `assert` takes a Bool.

## Added in M3

**Control flow.** `if c { a } else { b }` needs a Bool condition, and both
branches give the same type; without `else` it gives nothing. `and`, `or`
and `not` need Bools. `for x in xs` goes over a List; `a..b` is the List of
Ints from `a` to `b`, both included. `x += e` is `x = x + e`.

**Patterns.** `match` works on any value. A pattern is a variant, which may
have no fields (`Queued`) or nested patterns (`Done(0)`); a literal; a
name, which binds; `_`; a tuple `(p, q)`; or a list `[p, q]` of exactly
that length. A match must cover every value: each variant needs an arm
whose inner patterns all match anything, a Bool needs `true` and `false`,
and anything else needs a final `_` or name. An arm that can never run is
an error.

**Values.** Tuples `(a, b)` have a type per position. `xs[i]` gives `T?`,
none when out of range. Lambdas take one or more parameters,
`(a, b) -> ...`. `fn(params) -> T { ... }` is an anonymous function.

**Calls.** Type arguments may be written, `pair[Str]("a", "b")` and
`json.parse[Config](text)`, or come from context. `.len()` works on a List
or a Str, and `.push(x)` on a List held by a `var`; on a `let` it is an
error. `print` shows any value interpolation accepts.

**Maps.** `Map{k: v, ...}` has one key type and one value type; keys are
Str, Int or Bool. `m[k]` gives `V?`. `m[k] = v` changes one entry of a map
held by a `var`; like `.push`, it makes a new map, so another name holding
the old one does not see the change. Maps keep their keys in the order they
were first set, and two maps are equal when they hold the same entries.

**Entry point.** `fn main()` or `fn main(args: List[Str])` runs after the
top-level statements, and may not have a result. `!` and `fail` in `main`
stop the program, as at the top level.

## Built-ins

```
print(Str)                        log.warn(Str)
len(List[T]) -> Int               len(Str) -> Int
first(List[T]) -> T?              last(List[T]) -> T?
sum(List[Int]) -> Int             sum(List[Float]) -> Float
map(List[T], (T) -> U) -> List[U]
each(List[T], (T) -> U)
filter(List[T], (T) -> Bool) -> List[T]
sort_by(List[T], (T) -> K) -> List[T]          K: Ord
take(List[T], Int) -> List[T]
map_par(List[T], (T) -> U) -> List[U]          inside together only
sort(List[T]) -> List[T]                       T: Ord
reduce(List[T], (T, T) -> T) -> T?
first(List[T], (T) -> Bool) -> T?
env(Str) -> Str?
lower(Str) -> Str
words(Str) -> List[Str]                        runs of letters, digits and apostrophes
tally(List[T]) -> Map[T, Int]                  T is a key type
top(Map[K, Int], Int) -> List[Count[K]]        most first; ties in first-seen order
group_by(List[T], (T) -> K) -> Map[K, List[T]]
map_values(Map[K, V], (V) -> W) -> Map[K, W]
sum_by(List[T], (T) -> N) -> N                 N is Int or Float
sort_by_value(Map[K, V], asc | desc) -> List[Entry[K, V]]
csv.read[T](Str) -> List[T]!                   T is a record; or from context
flatten(List[List[T]]) -> List[T]
unique(List[T]) -> List[T]                     first of each, in order
json.encode(T) -> Str                          records, lists, Str-keyed maps, plain values
fs.list(Str, Str) -> List[Str]!                files in a directory matching a pattern
fs.write(Str, Str) -> nothing!
Chan[T](Int) -> Chan[T]
int.parse(Str) -> Int!
fs.read(Str) -> Str!
json.parse(Str) -> T!                          T from the expected type, or json.parse[T]
```

If the function passed to `map`, `each`, `filter`, `sort_by`, `map_par`,
`group_by`, `map_values` or `sum_by` uses `!`, the call's result is
failable and the first failure stops it.

`csv.read` takes the first row as column names; each record field takes
the column of the same name, in any order. An empty cell fills an optional
field with none. A failure names the file, the line and the column.
`json.parse` failures name the line where the value that does not fit
starts.

## Runtime changes that come with M2

- `json.parse` builds the record type the checker chose, so its values
  print and compare like any other record.
- `map`, `each`, `filter` and `sort_by` stop at the first failure from
  their function, as `map_par` already does. In M1 a failure became a list
  element.
- Mistakes the M1 interpreter found while running (mixing Int and Float,
  a missing record field, `.` on none) are now reported before running,
  with the same wording.
