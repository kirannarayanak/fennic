# Acceptance suite

M1 is done when every program here runs and its output matches the file beside it.

## How an example is run

- Each `NN_name.fn` has an expected-output file `NN_name.out`.
- The harness runs from this directory: `fennic run NN_name.fn`.
- `.out` holds stdout and stderr together, in the order they were written. Both streams go to the same pipe, so the order is deterministic.
- Directives on line 1, used by the harness only:
  - `// expect: exit 1` means the program must exit with status 1. Otherwise it must exit with 0.
  - `// expect: test` means run `fennic test NN_name.fn` instead of `fennic run`.
- File paths resolve against the working directory, so the harness runs each program from inside `examples/`. Fixtures live in `data/`.

## Coverage

| Feature (syntax reference) | Examples |
| --- | --- |
| `let`, `var`, reassignment, shadowing | 02, 20 |
| comments | all |
| `fn` with annotated signature, last expression returns | 03 |
| string interpolation | 01, 04 |
| `\|>` with `filter`, `sort_by`, `take` | 07 |
| lambdas `x -> e`, block lambdas | 06 |
| record `type`, optional field `T?` | 07, 08, 10 |
| `with` copy-with-change | 09, 17, 19 |
| sum types and `match` | 11 |
| `T!`, `!` propagation | 12, 13 |
| `else` default, `else err -> { ...; ... }` | 13, 14, 18 |
| `??`, `?.` | 10, 15 |
| generics `[T: Ord]`, `[T]` | 15 |
| `interface`, structural methods `fn Type.method(self, ...)` | 16, 17 |
| `Type.fn()` without `self` (`Config.default()`) | 14, 17 |
| `together`, `map_par` | 18 |
| `test` and `assert` | 19 |
| error messages that name the fix | 12, 20 |
| `if`/`else` expressions, `and`, `or`, `not` | 21, 23 |
| `for ... in`, ranges `a..b`, `_`, `+=`, `.push`, `.len()`, indexing | 22 |
| `fail` | 23 |
| patterns: variants without fields, literals, tuples, lists, `_` | 24 |
| lambdas of several values, `reduce`, `sort`, `first(pred)`, `fn() { }`, type arguments, backtick strings | 25 |
| `fn main(args)`, `env`, `int.parse` | 26 |
| maps: `Map{...}`, `m[k]`, `m[k] = v`, `map_values`, `sort_by_value` | 27 |
| `lower`, `words`, `tally`, `top` | 28 |
| `csv.read`, `group_by`, `sum_by` | 29 |
| `spawn`, channels, cancellation | 30 |
| `fs.list`, `Date`, `flatten`, `unique`, `json.encode` | 31 |
| `to_float`, `to_int` | 32 |
| `split`, `join`, `contains`, `starts_with`, `trim`, `pad_start`, `pad_end` | 33 |

Line-1 directives may also be `// expect: args a b` (command-line
arguments) or `// expect: env NAME=value`. `examples/m0/` holds the
programs of `docs/m0-spec.md`; see its README.

## Rejected programs (M2)

`rejected/` holds twenty programs that are valid syntax but wrong, each
with the one error the type checker must report in `NN_name.out`. The
harness runs `fennic run NN_name.fn` from inside `rejected/`; every one must
exit with status 1, print exactly its `.out` file, and run nothing.

M2 is done when these twenty match and the twenty programs above still
pass. The rules are in `docs/types.md`.
