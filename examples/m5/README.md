# M5 acceptance

What modules and imports must do (`docs/m5.md`), written before the code.
Nothing here runs yet: `import` is not in the parser.

## `programs/`

Each directory is one program of several files. The harness runs
`fennic run main.fn` from inside the directory, or `fennic test main.fn`
when `main.fn` has `// expect: test`, and compares stdout and stderr
together with `main.out`. Each must exit with 0.

| Program | Shows |
| --- | --- |
| 01 | qualified names: functions, a type in a signature, a record literal, variants built and matched; a module's `main` is not run |
| 02 | paths relative to the importing file: `main.fn` imports `lib/report`, which imports `format`, meaning `lib/format.fn` |
| 03 | a module imported by two files is one module: its `let` is computed once, before the program runs |
| 04 | spec program 14 as two files: a test file importing program 12's `chunk`; the module's own test does not run |
| 05 | a function declared on an imported type, `sinks.Console.new`; interfaces satisfied across modules, both ways |

Each was checked by pasting its modules into one file, without the module
names: it runs today and prints `main.out`.

## `rejected/`

Each must stop with exit status 1, run nothing, and print exactly
`main.out`, run as `fennic run main.fn` from inside the directory.

| Case | Refused |
| --- | --- |
| 01 | an import of a file that is not there |
| 02 | two files that import each other; the message names the circle |
| 03 | a file named like a built-in module, `fs.fn` |
| 04 | a local name that is an imported module's name |
| 05 | a top-level statement in an imported file |
| 06 | a `var` in an imported file |
| 07 | a name the module does not have, with the nearest one |
| 08 | a module used without importing it, imported only by another file |
| 09 | an import after other code |
| 10 | two types named `User` from two modules, which are different types |
| 11 | a method declared outside the file of its type |
| 12 | an import path ending in `.fn` |
| 13 | an absolute import path |
| 14 | a type error inside an imported file, reported in that file |
| 15 | a path that looks like a git dependency |

## `fmt/`

`01_imports.in` must format to `01_imports.fn`: `import` lines spaced
like any other, not sorted, at most one blank line after them.

## `lsp/`

Two sessions in the format of `examples/m4/lsp`, over `project/`. The
harness replaces `{root}` with the `file://` URI of `project/`.

| Session | Shows |
| --- | --- |
| 01 | hover and definition on `report.line` reach `lib/report.fn`; on `report`, the import and the file |
| 02 | an imported file with a problem gets one diagnostic on its import; an import that is not there gets `fennic run`'s message |
