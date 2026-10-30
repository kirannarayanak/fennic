# M3 diff: the M0 spec against the language as built

Compared: `docs/m0-spec.md` (the one-page tour and the twenty programs,
with its three amendments applied) against the language at `5f6af77`, the
end of M2. Each construct below was run through `fennic`, not assumed.

This list is the M3 scope. Nothing outside it enters M3.

**Where things stand:** none of the twenty programs produces its intended
output today. Program 6 uses only features that exist, but prints nothing,
because `fn main` is declared and never called. Program 13 type-checks
because it is declarations only.

## A. Language constructs

| # | Construct | Used in | Today |
| --- | --- | --- | --- |
| 1 | `fn main()` as the entry point | 1–7, 10, 11, 15, 17–20 | declared, never called; top-level statements run instead |
| 2 | `fn main(args: List[Str])`, command-line arguments | 15, 20 | missing |
| 3 | `if ... else ...` as an expression | tour (`largest`) | missing |
| 4 | `if` without `else`, as a statement | 9, 12 | missing |
| 5 | boolean `or` | 9 | missing; `or` is the failure fallback (collision 1) |
| 6 | `for x in list` | 12, 19 | missing |
| 7 | ranges `a..b` and `for n in a..b` | 3, 19 | missing |
| 8 | `_` as a pattern and as a `for` binding | 3, 15, 19 | missing |
| 9 | compound assignment `+=` | 2 | missing |
| 10 | `fail "message"`, with interpolation | 9 | missing; only built-ins can fail |
| 11 | tuples: `(a, b)` values and tuple patterns | 3 | missing; a new type |
| 12 | literal patterns: `0`, `(0, _)`, `Done(0)` | 3, 8 | missing; arm order becomes meaningful |
| 13 | list patterns `[x]`, `[x, y]` | 15 | missing |
| 14 | `match` on values that are not sum types (tuples, lists) | 3, 15 | missing; `match` needs a sum type |
| 15 | variants with no fields: `\| Queued`, pattern `Queued` | 8 | missing |
| 16 | lambdas with several parameters: `(a, b) -> ...` | tour (`reduce`) | missing |
| 17 | anonymous functions: `fn() { ... }` | 19 | missing |
| 18 | methods on built-in types: `.len()`, `.push(x)`, `.send(x)`, `.close()` | 12, 18, 19, 20 | missing; only `len(x)` |
| 19 | indexing `xs[i]`, giving `T?` | 20 (`args[0] ?? "."`) | missing |
| 20 | explicit type arguments at a call: `json.parse[Config](...)`, `csv.read[Sale](...)`, `Chan[Str](100)` | 16, 17, 19, 20 | missing; conflicts with the M2 decision (D5) |
| 21 | `print` of any value | 4 (`print(big)`) | only a Str (D6) |
| 22 | maps: `Map[K, V]`, `Map{k: v}`, `m[k]` giving `V?`, `m[k] = v`, `.len()` | 5, 17, 20 | missing |
| 23 | `spawn` inside `together` | 19 | missing |
| 24 | channels: `Chan[T](n)`, `.send`, `.close`, `for x in chan` | 19 | missing |
| 25 | a `Date` type | 20 | missing; `csv.read` must produce it |

## B. Built-in functions and modules

| Built-in | Used in | Shape the program implies |
| --- | --- | --- |
| `reduce` | tour | `List[T]`, `(T, T) -> T`, gives `T?` |
| `sort()` | 4 | natural order, no key |
| `first(pred)` | 7 | the first element that passes, as `T?`; today `first` takes no function |
| `int.parse` | 9, 15 | `Str -> Int!` |
| `env` | 10 | `Str -> Str?` |
| `lower`, `words` | 11 | `Str -> Str`, `Str -> List[Str]` |
| `tally`, `top(n)` | 11 | counts, as entries with `.key` and `.count` |
| `group_by`, `map_values` | 17, 20 | `List[T] -> Map[K, List[T]]`, then map each value |
| `sum_by` | 17, 20 | sum of a key function over a list |
| `sort_by_value(desc)`, `desc` | 17 | sort map entries by value; entries have `.key` and `.value` |
| `csv.read[T]` | 17, 20 | parse a CSV file into records; failable |
| `fs.list(dir, pattern)`, `fs.write(path, text)` | 20 | list matching files; write a file, failable |
| `json.encode` | 20 | any value to JSON text |
| `flatten`, `unique` | 20 | `List[List[T]] -> List[T]`; drop repeats |
| `http.get` | tour, 18, 19 | collision 3 |

Program 16's prose also promises that a JSON failure "names the field and
the line". Today it names the field path but not the line.

## C. Program by program

"No main" means the program is declarations only, so it has nothing to run
on its own. "Undefined" lists names the program uses but never declares.

| # | Needs (numbers from A) and built-ins (B) | No main | Undefined |
| --- | --- | --- | --- |
| 1 | 1 | | |
| 2 | 1, 9 | | |
| 3 | 1, 7, 8, 11, 12, 14 | | |
| 4 | 1, 21, `sort` | | |
| 5 | 1, 22 | | |
| 6 | 1 | | |
| 7 | 1, `first(pred)` | | `users`, and type `User` from program 6 |
| 8 | 12, 15 | yes | |
| 9 | 4, 5, 10, `int.parse` | yes | |
| 10 | 1, `env` | | `serve`; `parse_port` comes from program 9 |
| 11 | 1, `lower`, `words`, `tally`, `top` | | needs a `notes.txt` |
| 12 | 4, 6, 18 | yes | |
| 13 | (nothing missing) | yes | |
| 14 | runs program 12 | | `chunk` comes from program 12 |
| 15 | 2, 8, 13, 14, `int.parse` | | `tally(path, n)`, a user function (D10) |
| 16 | 20 (and amendment 1 removes the field default) | yes | |
| 17 | 1, 20, 22, `csv.read`, `group_by`, `map_values`, `sum_by`, `sort_by_value`, `desc` | | needs a `sales.csv` |
| 18 | 1, 18, `http.get` | | `urls` |
| 19 | 1, 6, 7, 8, 17, 18, 20, 23, 24, `http.get` | | `urls`, `save` |
| 20 | 2, 19, 20, 22, 25, `fs.list`, `csv.read`, `flatten`, `group_by`, `map_values`, `sum_by`, `unique`, `fs.write`, `json.encode` | | needs CSV files; does not type-check as written (D8) |

## D. Decisions needed before building

**All accepted as recommended.** They are recorded in `docs/m0-spec.md`
under "Decisions recorded at M3 step 2".

The first three are the collisions the spec names, with the recommendations
from the M3 brief. The rest were found by the diff.

1. **`or` is overloaded.** Program 9 uses boolean `or`; programs 10, 11
   and 15 use failure-fallback `or`. Recommendation: `or`, `and`, `not`
   are boolean, and the failure fallback becomes `else`
   (`load(path) else Config.default()`). That rewrites every use of the
   fallback once. It touches the syntax reference, examples 13, 14, 18 and
   19, the help text of every message that suggests `or`, and the expected
   output that quotes it (example 12, rejected 09 and 10). Kiran decides.
2. **`.push` and immutability.** Recommendation: `.push` changes a `var`
   list in place, and is a compile error on a `let` list.
3. **`http.get`.** Recommendation: cut it from the spec and rewrite
   programs 18, 19 and 20 against a local operation, so they still show
   scoping and cancellation.
4. **`and` and `not` are not in the spec.** No program uses them; only
   program 9's `or` is boolean. Recommendation 1 adds them, which this diff
   cannot justify on its own. Boolean `or` without `and` would be odd, so
   I suggest taking them as part of decision 1. Kiran decides.
5. **Explicit type arguments against the M2 decision.** Programs 16, 17,
   19 and 20 write `json.parse[Config](...)`, `csv.read[Sale](...)` and
   `Chan[Str](100)`. At the start of M2 the choice was "from context", and
   "both" was declined. `Chan[Str](100)` has no context to come from, so
   the explicit form is needed at least there. Options: keep context and
   allow the explicit form too; or explicit only, rewriting examples 13
   and 14 and the syntax reference's `load`.
6. **`print` of any value.** Program 4 prints a list. Today `print` takes
   only a Str. Recommendation: `print` accepts what interpolation accepts,
   anything but a failable or a function. That is display, not a
   conversion.
7. **How "run as written" works for fragments.**
   - Programs 8, 9, 12, 13 and 16 have no `main`.
   - Programs 7, 10, 15, 18 and 19 use names they never define.
   - Program 14 needs program 12, and program 10 needs program 9.

   Proposal: each acceptance file is the spec program verbatim, plus a
   separate preamble that supplies only what is missing: the undefined
   names, the program it depends on, and a `main` or a call where there is
   none. The preamble never edits spec text, and its expected output sits
   beside it.
8. **Program 20 does not type-check as written.** Its `Map{"total": ...,
   "days": ...}` holds a Float and an Int. In a typed map that is an error,
   and converting one to the other implicitly is a non-goal. Suggested
   spec change: a record `type Summary { total: Float  days: Int }` in
   place of the inner map.
9. **What `a..b` includes.** Program 3's FizzBuzz over `1..100` reads as
   inclusive. Recommendation: `a..b` includes both ends.
10. **`tally` names two things.** It is a built-in in program 11 and a
    two-argument user function in program 15. A user's own definition
    already shadows a built-in, so program 15 works if its preamble defines
    it. Noted, not a conflict.
11. **`.len()` and `len()` both appear.** Program 20 uses `|> len()` and
    `summary.len()`. Recommendation: keep the `len` function, and add
    `.len()`, `.push`, `.send` and `.close` as methods on the built-in
    types, matching the spec.

## E. Where the shipped examples worked around a missing construct

- **Example 15's `largest`** uses `sort_by(x -> x) |> last()`. The tour
  uses `reduce` with an `if` expression.
- **Example 18** reads files with `fs.read` instead of `http.get`.
- **Every example** runs top-level statements rather than `fn main`, uses
  `len(x)` rather than `.len()`, uses `first` with no function, and
  interpolates values to print them because `print` takes only a Str.

## F. Size

M3 is several times M2.
- **Core language:** items 1–21 extend the parser, checker and interpreter.
- **Collections:** maps, tuples, and section B's built-ins.
- **Concurrency and data:** `spawn`, channels, CSV and `Date`.

Suggested build order, each group landing with its own passing and refused
examples: core language first, then collections and built-ins, then
concurrency and data formats. It ends with the twenty programs as the
acceptance run.

## G. Consistency fixes, not gated on this diff

- **`map_par`'s reported failure** is now recorded in CLAUDE.md as
  explicitly unspecified.
- **Raw strings.** JSON in a string needs `\{`. One option is `r"..."` with
  no escapes and no interpolation, e.g. `r"{"port": 80}"`. A raw string
  cannot contain `"`, which JSON needs, so a better fit is a delimiter that
  JSON never uses, such as backticks:
  `` `{"port": 80}` ``. Not decided; this is outside the spec, so it needs
  your call.

## Progress

**Core language: done.**
- **Language constructs:** section A items 1–21. Items 17, 18 and 20 are
  only partly done: their channel and `spawn` parts belong to concurrency.
- **Built-ins:** `reduce`, `sort`, `first(pred)`, `int.parse` and `env`.
- **Raw strings,** from section G.
- **Spec programs 1, 2, 3, 4, 6, 7, 8, 9, 10, 12, 13, 14, 15 and 16** run
  as written in `examples/m0`.

**Collections and built-ins: done.**
- Maps (item 22).
- `lower`, `words`, `tally`, `top`, `group_by`, `map_values`, `sum_by`,
  `sort_by_value` with `asc` and `desc`, and `csv.read`.
- JSON failures that name the line.
- Spec programs 5, 11 and 17 run as written, with no preamble.

**Concurrency and data formats: last.**
- `spawn`, channels, `Date` and CSV dates (items 23–25).
- The built-ins only program 20 uses: `fs.list`, `fs.write`,
  `json.encode`, `flatten` and `unique`. Moved here from the collections
  group, so they land with the program that needs them.
- The rewrite of programs 18, 19 and 20 without `http.get` (decision 3).
- **A question to settle then:** program 20 pipes
  `fs.list(dir, "*.csv")` straight into `map_par` without `!`. So either
  `fs.list` cannot fail, and a missing directory gives an empty list, or
  the spec text changes to `fs.list(dir, "*.csv")!`.
