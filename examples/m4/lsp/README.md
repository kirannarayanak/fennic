# Language server sessions

Each `NN_name.json` is a scripted session with `fennic lsp`:

- `about` says what it shows.
- `steps` run in order. `send` is a message the client writes to the
  server's standard input, framed with `Content-Length`. `expect` is the
  next message the server must write, compared as JSON, so key order and
  spacing do not matter. `exit` is the status the server must exit with
  after the last message.

The server writes nothing that a step does not expect: after the last
`expect`, its output must be empty.

Positions in these files are 0-based lines and characters, as LSP
defines them, in the encoding the session agreed: UTF-16 unless the
client offered another. Every document is `file:///work/main.fn`; no
file is read from disk.

| Session | Shows |
| --- | --- |
| 01 | `initialize` and its capabilities; an unknown request gets "method not found"; an unknown notification is ignored; `shutdown`, `exit` |
| 02 | a type error published on `didOpen`, with its help; cleared on a `didChange` that fixes it |
| 03 | a syntax error published; hover, definition and formatting answer nothing while the file does not parse |
| 04 | formatting replaces the whole document and ignores the client's tab options; no edits once formatted |
| 05 | hover on a `var`, a function, a field, a `let`, a `for` variable and a built-in (at the type it is used with); nothing on a keyword, inside a string, or past the end of a line |
| 06 | definition of a `var`, a `for` variable, a function, a field, a type and a parameter used inside interpolation; nothing for a built-in |
| 07–09 | the same diagnostic after a 🦊 in UTF-16 (the default), UTF-32 and UTF-8 positions |

Beyond these sessions, a test sends every prefix of every example in
`examples/` as one `didChange` after another, and requests hover and
definition at each position of the last. Every request must get a
well-formed answer, and the server must not exit.
