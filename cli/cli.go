// Package cli is the `fennic` command: `fennic run <file>` and
// `fennic test <file>`.
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
	"github.com/kirannarayanak/fennic/parser"
)

const usage = `usage:
  fennic run <file.fn>    run a program
  fennic test <file.fn>   run the program's test blocks
`

// Main runs the command and returns the process exit status.
func Main(args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 || (args[0] != "run" && args[0] != "test") {
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
		fmt.Fprintf(stderr, "error: cannot read %s: %v\n", path, err)
		return 2
	}
	src := string(data)
	render := func(w io.Writer, d diag.Diagnostic) { diag.Render(w, path, src, d) }

	file, diags := parser.Parse(src)
	if len(diags) == 0 {
		diags = interp.Check(file, src)
	}
	if len(diags) > 0 {
		for _, d := range diags {
			render(stderr, d)
		}
		return 1
	}

	in := interp.New(file, src, filepath.Dir(path), stdout, stderr)
	if err := in.Run(); err != nil {
		render(stderr, err.(*interp.RuntimeError).Diag)
		return 1
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
