# M4 error messages

Each `NN_name.fn` here is a program that gets a poor message today.
`NN_name.out` is the exact message M4 must print, run as
`fennic run NN_name.fn` from this directory, with exit status 1.
Each `.out` is new text: none of them passes today.

Cases 01–10 are the messages `programs/RECORD.md` names. Cases 11–13 stand
for the inventory below, one per kind.

## Proposed help for every site that has none

The type checker's 28 sites, at `3c95e05`. Where the help can quote the
source or a type, it does; `<x>` below is the source text at the caret.

| Site | Message | Proposed help |
| --- | --- | --- |
| `call.go:124` | `f` takes N arguments, but was given M | the signature: `f` is `(Int, Str) -> Bool` |
| `call.go:136` | this is an Int, not a function | if it is a list or a map, read from it with `<x>[...]` (a List or Map); otherwise "only functions and lambdas can be called" |
| `call.go:305` | `len` takes 1 argument, but was given 2 | the built-in's signature from `docs/types.md`: `len(List[T]) -> Int` |
| `call.go:321` | `print` needs a value to show (a failable) | handle the failure first: `<x>!` passes it on, `<x> else ...` gives a fallback |
| `call.go:396` | `flatten` needs a list of lists | this list is already flat; drop the `flatten` |
| `call.go:492` | `reduce` combines two Ints into one, but this function gives a Str | make the function give an Int; to fold into another type, use `map` first |
| `call.go:522–528` | `len` needs a List or a Str | failable: `<x>!` or `else`; optional: `(<x> ?? [])`; otherwise case 11 |
| `call.go:553` | `sum` adds up Ints or Floats | `sum_by(x -> ...)` picks the number from each element |
| `call.go:581` | `map` passes an Int to its function, but this one takes a Str | give the lambda's parameter no annotation, or convert: the conversion named by the types (`to_float`, interpolation for Str) |
| `call.go:645, 649` | `filter` needs a List | failable: `!` or `else`; optional: `?? []`; a Map: `map_values`, or `sort_by_value` to get its entries as a List |
| `call.go:739` | `.push` takes 1 argument, but was given 2 | the method's signature: `.push(T)` |
| `call.go:780, 784` | `map_values` needs a Map | a List: `group_by(...)` makes a Map from it |
| `call.go:862` | `.close` takes no arguments | write `ch.close()` |
| `expr.go:65` | `-` needs an Int or a Float | case 12 |
| `expr.go:220` | a list element of the wrong type, explained by an interface | the interface's missing method, as `mismatch` already says for arguments |
| `expr.go:281` | `with` copies a record | case 13 |
| `expr.go:359` | an Int has no field or method `foo` | the methods it does have, if any (`.len()`, `.push()`, ...); for a record-like name, "only records have fields" |
| `expr.go:495` | `..` makes a range of Ints | Float: `to_int(x)!`; Str: `int.parse(x)!` |
| `expr.go:566` | `*` needs two numbers, but got a Str and an Int | Str: `int.parse` it; to join text, interpolate: `"{a}{b}"` |
| `expr.go:814` | this pattern is a Str, but the value matched is an Int | write the literal as an Int: `0`, not `"0"` (the pattern rewritten in the value's type, where it converts plainly) |
| `expr.go:823` | this pattern is a tuple of 2, but the value is a tuple of 3 | give one pattern per position: `(a, b, c)`; `_` for those not needed |
| `expr.go:836` | this pattern is a list, but the value matched is a Str | match a Str with a literal, or `split` it into a List first |
| `expr.go:862` | this pattern is a Shape, but the value matched is a Color | the value's own variants: `Red`, `Green` or `Blue` |
| `expr.go:1011` | only a List or a Map can be indexed | a Str: `split(s, "")[i]`; an optional: `(x ?? [])[i]` |

The runtime's 18 sites (`interp/builtins.go`, `csv.go`, `eval.go`,
`interp.go`) all become:

```
error: internal error: <the message as today>
 --> file:line:col
  ...
help: this is a bug in Fennic, not in your program: the type checker should have refused it. Please report it with this program
```

## Fallbacks in the top-level `!` help

Cases 09 and 10. The checker records the type of each `!`; the help
suggests the plainest value of it:

| Type | Fallback |
| --- | --- |
| Int, Float, Str, Bool | `0`, `0.0`, `""`, `false` |
| `List[T]`, `Map[K, V]` | `[]`, `Map{}` |
| `T?` | `none` |
| nothing | (no fallback: `<x> else err -> log.warn("{err}")`) |
| a record or sum type with a function `T.default()` | `T.default()` |
| any other type | `err -> ...`, the handler form |
