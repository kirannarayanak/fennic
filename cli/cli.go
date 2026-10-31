// Package cli is the `fennic` command: `fennic run <file>`,
// `fennic test <file>`, `fennic fmt <file>...` and `fennic lsp`.
package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kirannarayanak/fennic/diag"
	"github.com/kirannarayanak/fennic/interp"
	"github.com/kirannarayanak/fennic/lsp"
	"github.com/kirannarayanak/fennic/parser"
	"github.com/kirannarayanak/fennic/types"
)

const usage = `usage:
  fennic run <file.fn> [args...]   run a program; args go to fn main(args: List[Str])
  fennic test <file.fn>            run the program's test blocks
  fennic fmt [--check] <file.fn>...
                                   format files in place; --check only lists those not formatted
  fennic lsp                       the language server, for an editor, on stdin and stdout
`

// Main runs the command and returns the process exit status.
func Main(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "fmt" {
		return formatFiles(args[1:], stdout, stderr)
	}
	if len(args) == 1 && args[0] == "lsp" {
		return lsp.Serve(os.Stdin, stdout, stderr)
	}
	if len(args) < 2 || (args[0] != "run" && args[0] != "test") || (args[0] == "test" && len(args) != 2) {
		fmt.Fprint(stderr, usage)
		return 2
	}
	path := args[1]
	if !strings.HasSuffix(path, ".fn") {
		fmt.Fprintf(stderr, "error: %s is not a Fennic file\nhelp: Fennic programs end in .fn\n", path)
		return 2
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "error: cannot read %s: %v\nhelp: check the path, which is relative to the directory you run fennic in\n", path, err)
		return 2
	}
	src := string(data)
	render := func(w io.Writer, d diag.Diagnostic) { diag.Render(w, path, src, d) }

	file, diags := parser.Parse(src)
	if len(diags) == 0 {
		diags = interp.Check(file, src)
	}
	var info *types.Info
	if len(diags) == 0 {
		diags, info = types.Check(file, src)
	}
	if len(diags) > 0 {
		for _, d := range diags {
			render(stderr, d)
		}
		return 1
	}

	in := interp.New(file, src, filepath.Dir(path), stdout, stderr)
	in.SetTargets(info.Targets)
	in.SetFallbacks(info.Fallbacks)
	if err := in.Run(); err != nil {
		render(stderr, err.(*interp.RuntimeError).Diag)
		return 1
	}
	if args[0] == "run" && in.HasMain() {
		status, err := in.Main(args[2:])
		if err != nil {
			render(stderr, err.(*interp.RuntimeError).Diag)
			return 1
		}
		return status
	}
	if args[0] == "test" {
		report, failed := interp.Report(in.RunTests(), func(d diag.Diagnostic) string {
			var b bytes.Buffer
			render(&b, d)
			return b.String()
		})
		fmt.Fprint(stdout, report)
		if failed > 0 {
			return 1
		}
	}
	return 0
}
