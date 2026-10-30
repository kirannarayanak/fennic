# M0 acceptance

The twenty programs of `docs/m0-spec.md`, run as written. M3 is done when
every one is here and passes.

Each file is the program's text from the spec, byte for byte, then a line

    // ---- preamble: not part of the spec program ----

and a preamble that supplies only what the program leaves out: names it
uses but never declares, the program it depends on, and a `main` or a call
where it has none. The preamble never changes the spec text. A test checks
that the part above the line is identical to the spec.

The harness reads `// expect:` lines anywhere in the file: `test` runs the
test blocks, `args a b` passes command-line arguments, `env NAME=value` sets
an environment variable, `exit 1` expects a failure.

All twenty are here. Their fixtures: config.json (16), notes.txt (11),
sales.csv (17), pages/ (18, 19) and sales/ (20). Program 20 writes
summary.json; the test runs everything in a copy of this directory and
compares that file with 20_summary.json.expected.
