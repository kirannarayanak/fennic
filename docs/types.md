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
| error | (no name to write) | `err` in `or err -> ...` |

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
- `x or y` needs `y: T` and is a `T`. `x or err -> body` binds `err` to the
  error and needs `body: T`.
- `!` or `or` on something that cannot fail is an error.
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
`map_par` is allowed only inside `together`.

**Strings.** Interpolation accepts any value except a failable or a
function. `print` and `log.warn` take a Str. `assert` takes a Bool.

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
fs.read(Str) -> Str!
json.parse(Str) -> T!                          T from the expected type
```

If the function passed to `map`, `each`, `filter`, `sort_by` or `map_par`
uses `!`, the call's result is failable and the first failure stops it.

## Runtime changes that come with M2

- `json.parse` builds the record type the checker chose, so its values
  print and compare like any other record.
- `map`, `each`, `filter` and `sort_by` stop at the first failure from
  their function, as `map_par` already does. In M1 a failure became a list
  element.
- Mistakes the M1 interpreter found while running (mixing Int and Float,
  a missing record field, `.` on none) are now reported before running,
  with the same wording.
