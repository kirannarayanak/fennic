# M4 error messages

Each `NN_name.fn` here is a program that gets a poor message today.
`NN_name.out` is the exact message M4 must print, run as
`fennic run NN_name.fn` from this directory, with exit status 1.
They are checked by `TestMessages` in `cli/`, which also fails on any
diagnostic, anywhere in the acceptance suite, that has no help line.

Cases 01–10 are the messages `programs/RECORD.md` names. Cases 11–13 stand
for the inventory below, one per kind.

## Help for every site that had none

The type checker's 28 sites, at `3c95e05`. Where the help can quote the
source, it does: `<x>` below is the source text at the caret. The checker's
own tests now fail on any diagnostic with no help.

| Site | Message | Help |
| --- | --- | --- |
| `call.go:124` | `f` takes N arguments, but was given M | `f` is `(Int) -> Int`; give it 1 argument |
| `call.go:136` | this is an Int, not a function | a List or Map: read from it with `xs[i]` or `m[key]`; otherwise: only functions and lambdas can be called; remove the `(...)` |
| `call.go:305` | `len` takes 1 argument, but was given 2 | the built-in's forms, as `docs/types.md` writes them: it is written `len(List[T]) -> Int` or `len(Str) -> Int` |
| `call.go:321` | `print` needs a value to show (a failable) | handle the failure first: `<x> else ...` gives a fallback, and `<x>!` passes it on |
| `call.go:396` | `flatten` needs a list of lists | this list is already flat; remove the `flatten` |
| `call.go:492` | `reduce` combines two Ints into one, but this function gives a Str | make the function give an Int; to combine into another type, `map` the list to it first |
| `call.go:522–528` | `len` needs a List or a Str | a failable or optional: as `mismatch` says for them; otherwise case 11 |
| `call.go:553` | `sum` adds up Ints or Floats | Strs: `join(<x>, "")`; otherwise `<x> \|> sum_by(x -> x.amount)`, naming the record's first number field |
| `call.go:581` | `map` passes an Int to its function, but this one takes a Str | make the function take an Int, or leave out its parameter's type so it is inferred |
| `call.go:645, 649` | `filter` needs a List | a failable: `!` or `else`; an optional list: `(<x> ?? [])`; a Str: `split(<x>, " ")`; a Map: `sort_by_value(asc)` gives its entries as a List |
| `call.go:739` | `.push` takes 1 argument, but was given 2 | write `.push(x)` |
| `call.go:780, 784` | `map_values` needs a Map | a List: `<x> \|> group_by(x -> ...)`; otherwise pass a Map, like `Map{"a": 1}` |
| `call.go:862` | `.close` takes no arguments | write `.close()` |
| `expr.go:65` | `-` needs an Int or a Float | case 12 |
| `expr.go:220` | a list element of the wrong type, explained by an interface | the interface's fix, or make every element a T |
| `expr.go:281` | `with` copies a record | case 13 |
| `expr.go:359` | an Int has no field or method `foo` | a record or sum type: `fn T.foo(self) ...`; an interface: the methods it lists, and add `foo` to it; a List or Map: its methods; otherwise call a function with it: `foo(<x>)` |
| `expr.go:495` | `..` makes a range of Ints | a Float: `to_int(<x>)!`; a Str: `int.parse(<x>)!`; otherwise like `1..10` |
| `expr.go:566` | `*` needs two numbers, but got a Str and an Int | with a Str: `int.parse(...)`, or interpolate to join text; otherwise use two Ints or two Floats |
| `expr.go:814` | this pattern is a Str, but the value matched is an Int | write the literal as an Int, like `0` |
| `expr.go:823` | this pattern is a tuple of 3, but the value matched is a (Int, Int) | give one pattern per position, `(_, _)`, with `_` for those not needed |
| `expr.go:836` | this pattern is a list, but the value matched is a Str | match a Str with a literal, or `split` it into a List first |
| `expr.go:862` | this pattern is a Shape, but the value matched is a Color | its variants are `Red`, `Green` or `Blue` |
| `expr.go:1011` | only a List or a Map can be indexed | a Str: `split(<x>, "")[i]`; an optional: `(<x> ?? ...)[...]`; a failable: `!` or `else` |

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
