# What three real programs reached for

M3 step 4. The three programs in this directory were written to be kept
and used, not to exercise the language. This is the record of every point
where one of them reached for something Fennic does not have. That
includes gaps solved in a line, and places where the design changed to
avoid a gap. Each entry starts with what was reached for first, before
the workaround.

It is the only legitimate input to the one rule: no feature enters the
language until three real programs needed it and worked around its
absence. Nothing has been added yet.

## 1. Pipeline reconciliation (`reconcile/`)

1. **`match` on an optional with a `none` arm.** The first draft read
   `match w { none -> ... _ -> ... }`, and the parser refused it: "expected
   a pattern, found `none`". There is no pattern for none or for a present
   value.
2. **`to_map(pairs)` to build the join index.** It isn't a built-in, so the
   index is a `var` map filled by a `for` loop.
3. **`abs(x)`.** Wrote a three-line function.
4. **`max(a, b)`,** for a base that is the same whichever side is the
   reference. Wrote `larger`.
5. **Narrowing an optional after a `none` check.** After
   `if wh[k] != none`, the value is still a `Float?`. Wrote
   `wh[k] ?? 0.0` with a comment saying it cannot be none: a default the
   checker forces and the program never uses.
6. **Rounding, or any number formatting.** The report prints
   `3.1892422339888924%` where `3.2%` was wanted. There is no `round`, no
   format specifier, and no Float-to-Int conversion to build one from.
   **No workaround exists.** The output is shipped as it is.
7. **Iterating a map's keys, or set difference,** to find metrics present
   on one side only. Built a second index and filtered the rows instead.
8. **Sorting descending,** with `reverse` or a `desc` for `sort_by`.
   Negated the key: `sort_by(m -> -m.diff)`.
9. **A generic join helper,**
   `index_by[T](rows: List[T], key: (T) -> Str, value: (T) -> Float)`,
   reusable for any export. It needs a function-typed parameter, which a
   top-level function cannot have. Settled for `index(rows: List[Row])`,
   written for this one record.
10. **`join(list, ", ")`** to print the one-sided metrics. Printed the list
    display `["new_signups"]` instead.
11. **An exit status.** A reconciliation should exit 1 when anything
    differs, so a scheduler or CI notices. There is no `exit`. `fail` in
    `main` exits 1, but prints a compiler-style error with a source
    snippet, which is wrong for a report.
12. **Column alignment** (padding to a width). Used tabs.
13. **Counts print as `40211.0`,** because the CSV's value column must have
    one type, and some metrics are fractions.

What worked without friction: `csv.read` into a record, command-line
arguments with `args[0] ?? "default"`, `x != none` to test presence, and
`.push` into a `var` list.

## 2. Connector config audit (`connector-audit/`)

1. **A typed record for the connector config.** Kafka Connect configs spell
   their keys with dots (`connector.class`, `topic.prefix`,
   `snapshot.mode`). A field name cannot contain a dot, and there is no way
   to map a JSON key to a field of another name (serde's `rename`). The
   parser's help for `connector.class: Str` suggests `connector: Str`,
   which is misleading.
2. **Falling back to `config: Map[Str, Str]`.** The type checker accepts it.
   The runtime then refuses: "expected a Map[Str, Str] at `config`, but the
   JSON has an object". **This is a bug, not a gap:** the checker and the
   runtime disagree about `json.parse` into a map.
3. **Falling back to reading the text and looking for the key.** There are
   no string functions at all: no `contains`, `split`, `replace` or
   `starts_with`.

   **Design changed:** the sample connectors spell the three keys the rules
   need with underscores. `real-format/pg-orders-cdc.json` shows the real
   shape. **As written, the program cannot audit a real Kafka Connect
   export.**
4. **Deciding a connector's kind from its class,** with
   `contains(class, "debezium")` or `ends_with(class, "SinkConnector")`.
   **Design changed:** an allow-list of known classes; any other class is a
   violation.
5. **Membership, `x in list`.** Wrote `contains` with
   `first(xs, y -> y == x) != none`.
6. **`join(parts, "-")`** to rebuild a name from its words. Wrote `joined`
   with `reduce` and interpolation.
7. **`split(prefix, ".")`** for the topic prefix. Used `words`, which also
   splits on `-` and `_`, so `prod.order_items` would be three parts. That
   is close enough for these rules, but wrong in general.
8. **A regular expression** for the naming rule
   (`^[a-z0-9]+(-[a-z0-9]+)*$`). Approximated with `lower(n) == n` and the
   words joined by `-` equalling `n`.
9. **An early return** when a file does not parse. I typed a placeholder
   call inside the `else` handler, then saw the handler must produce a
   `Connector`. There is no `return`, and no way to match on success or
   failure. **Design changed:** a failable `check_file`, whose `else` at the
   call site produces the whole result.
10. **`basename(file)` and trimming `.json`,** to check that a file is named
    after its connector. Compared against `"{dir}/{name}.json"` instead.
    That only works because `fs.list` joins the directory and name the same
    way; a trailing slash on the directory would break it.
11. **An exit status** of 1 when there are violations. No `exit`.
12. **A recursive walk of the folder.** `fs.list` reads one directory. Not
    needed for these flat samples.
13. **Column alignment.** Used tabs.

## 3. Schema drift (`schema-drift/`)

1. **`to_map(rows, key)` keyed by `(table, column)`.** There's no `to_map`,
   and map keys must be Str, Int or Bool, so a tuple key is refused.
   Settled on a `"table.column"` Str key, which collides if a name contains
   a dot.
2. **`.keys()` on a map.** Not there; filtered the rows instead.
3. **Set difference and intersection,** `keys_b - keys_a` and
   `keys_a & keys_b`. There is no Set type, and `&` isn't an operator. Wrote
   each as a filter over one side's rows against the other side's index.
4. **`split` at `(` and trimming `)`,** to normalise `varchar(255)`. Used
   `words`, which splits on every non-letter. `numeric(12,2)` comes apart
   and back together correctly, but `double precision` gets the base
   `double`, so `float8` and `double precision` compare unequal.
5. **Narrowing a map lookup after `== none`.** Wrote `other ?? c`, a
   default that can never be used, as in program 1.
6. **Printing the changes grouped by table, in table order:** `group_by`,
   then walk the map in key order. A map cannot be walked at all.
   `sort_by_value` needs orderable values ("these are `List[Change]`
   values"), and its help ("map the values to Ints, Floats or Strs first")
   points the wrong way. **Design changed:** no `group_by`; the changes are
   sorted by table, and a header printed when the table changes, tracked
   with a `var`.
7. **A second specialised `index` function,** for `Column`. Program 1's
   generic `index_by` would have served both programs, had function-typed
   parameters existed. This is a consequence of the gap, not a separate
   first reach.
8. **An exit status** of 1 when drift is found. No `exit`.
9. **Column alignment.** Used tabs.

## Across all three

**Paths on the command line do not work from another directory.** All
three programs take file paths as arguments. Fennic resolves every
relative path against the program file's directory, not the directory the
user is in. So `fennic run reconcile/reconcile.fn reconcile/source.csv`,
run from `programs/`, fails with "file not found": it looks for
`reconcile/reconcile/source.csv`. Each program works only when run from its
own folder. That rule was chosen in M1, so that examples find their
fixtures. Programs that take paths from a user need the opposite. **No
workaround exists inside the language.**

### Tally

The one rule needs three programs. Counting the programs whose record
names each gap as a first reach:

| Gap | Programs | Count |
| --- | --- | --- |
| String functions: `join`, `split`, `contains`, `starts_with`/`ends_with`, `replace`, `basename`, trim | 1, 2, 3 | **3** |
| Exit status from `main` | 1, 2, 3 | **3** |
| Column alignment, or padding a Str to a width | 1, 2, 3 | **3** |
| Paths relative to where the user runs the program | 1, 2, 3 | **3** |
| Narrowing a `T?` to `T` after a check, or a `none` pattern | 1, 3 | 2 |
| Walking a map: `.keys()`, entries in key order | 1, 3 | 2 |
| Building a map from a list (`to_map`) | 1, 3 | 2 |
| Sets, or set difference and intersection | 1, 3 | 2 |
| Function-typed parameters, so a user's function can take a lambda | 1 (3 by consequence) | 1 |
| Number formatting, rounding, Float to Int | 1 | 1 |
| `abs`, `max` | 1 | 1 |
| Sorting descending | 1 | 1 |
| Map keys that are tuples | 3 | 1 |
| JSON keys that are not valid field names (`rename`) | 2 | 1 |
| Early return, or matching on success and failure | 2 | 1 |
| Regular expressions | 2 | 1 |
| Recursive directory walk | 2 | 1 |

By the rule as written, four gaps have their three programs: **string
functions, an exit status, padding, and paths relative to the user's
directory**. The last is a change to an M1 decision, not a new feature.

The named candidate, **function types, does not have its evidence**. One
program reached for it; a second duplicated code because of it. It stays
out.

**Bug found:** `json.parse` into a `Map` passes the type checker and fails
at run time (program 2, entry 2). It's not a feature question; the checker
and the runtime must agree, one way or the other.

## Round two: what the fixes removed

M3 round two fixed three defects and added the two features with three
programs behind them. The programs were rewritten to drop the workarounds
those changes made unnecessary. The entries above are kept as they were
written, as evidence.

| Change | Entries it resolved |
| --- | --- |
| `json.parse` reads a JSON object into `Map[Str, V]` (a bug fix) | audit 1, 2: the audit reads real dotted-key configs; its samples are back in the real format |
| Paths resolve against the caller's working directory (a bug fix) | "Across all three": each program now runs from anywhere |
| `to_float`, `to_int` (a defect: no conversion existed) | reconcile 13: whole numbers print without `.0` |
| `split`, `join`, `contains`, `starts_with`, `trim`, `pad_start`, `pad_end` | reconcile 10, 12; audit 3, 4 (the class allow-list is gone), 6, 7, 10 (the path comparison no longer depends on how `fs.list` joins), 13; drift 4 (`float8` and `double precision` now compare equal), 9 |
| `fn main() -> Int` as the exit status | reconcile 11; audit 11; drift 8 |

Still standing, waiting for evidence or deferred:
- **Number formatting (reconcile 6).** Deferred: the report still prints
  `8.167938931297709%`.
- **Narrowing an optional after a check, or a `none` pattern** (reconcile
  5, drift 5).
- **Walking a map by key, `.keys()`, sets, `to_map`** (reconcile 7, drift
  1–3, 6).
- **Function types** (reconcile 9, drift 7).
- **`abs` and `max`** (reconcile 3, 4).
- **Sorting descending** (reconcile 8).
- **Early return, or matching on failure** (audit 9).
- **Regular expressions** (audit 8).
- **Membership in a list** (audit 5): `contains` is for text, so the audit
  keeps its own `member`.
- **Recursive directory walk** (audit 12).

### New reaches while rewriting

- **`ends_with`** (audit), to tell a sink connector by its class. It's not
  in the list that passed the rule; `contains(class, "SinkConnector")`
  stands in.
- **`xs |> len() == 0`** (audit) does not parse, because `|>` binds more
  loosely than `==`, and the error, "put each statement on its own line",
  does not say why. Wrote `(xs |> filter(...)).len() == 0`. A message
  for M4.
- **Removing every space from a Str** (drift), so `numeric(12, 2)` and
  `numeric(12,2)` normalise alike. Wrote `split(" ") |> join("")`; a
  `replace` would say it directly.
- **Destructuring a tuple in `for`,** `for (name, n) in rows`, when
  writing `examples/33_strings.fn`. That's an example, not a real program,
  so it doesn't count toward the rule.
