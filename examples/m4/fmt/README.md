# Formatter pairs

`NN_name.in` is a program as someone might write it; `NN_name.fn` is what
`fennic fmt` must turn it into, byte for byte. The harness copies the `.in`
to a temporary `NN_name.fn`, formats it, and compares. Both sides run and
print the same thing today.

`12_syntax_error.in` does not parse. `fennic fmt` must leave it unchanged,
print `12_syntax_error.out`, and exit with 1.

| Pair | Rule in `docs/m4.md` |
| --- | --- |
| 01 | spaces around operators, `->`, `=`, `+=`; after `,` and `:`; none inside `( )` and `[ ]`; one after a keyword such as `not` |
| 02 | two-space indentation, tabs included; a continued `\|>` line is one level deeper |
| 03 | no blank lines at the start or end of a block or file; at most one between statements |
| 04 | trailing commas added on multi-line lists, maps, records and arguments, removed on one line |
| 05 | `->` aligned across consecutive one-line arms; an arm with a block body is not aligned against |
| 06 | own-line comments indented like the next line; trailing comments on consecutive lines aligned; a lone one gets one space; comment text unchanged |
| 07 | `Point { x: 3 }` and `with { y: 5 }` spaced; `Map{"a": 1}` tight |
| 08 | one-line blocks stay on one line, spaced `{ a }`; `;` followed by one space |
| 09 | literals exactly as written: `2.50`, escapes, backtick strings, and the code inside `{...}` in a string |
| 10 | no width: a long line is not split, a broken argument list is not joined |
| 11 | comments inside a list and between match arms; a trailing comma goes before a trailing comment |

Beyond these pairs, the formatter tests run over every `.fn` file in the
repository: formatting changes nothing (the repository is a fixed point),
the syntax tree is unchanged, and the comments are the same, in order.
