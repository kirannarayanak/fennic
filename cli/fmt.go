package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kirannarayanak/fennic/diag"
	"github.com/kirannarayanak/fennic/format"
)

// formatFiles is `fennic fmt [--check] <file.fn>...`. It rewrites each file
// in canonical form, or with --check changes nothing and names the files
// that are not formatted. A file that does not parse is reported and left
// alone. The status is 1 if any file did not parse or, with --check, is not
// formatted.
func formatFiles(args []string, stdout, stderr io.Writer) int {
	check := len(args) > 0 && args[0] == "--check"
	if check {
		args = args[1:]
	}
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	status := 0
	for _, path := range args {
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
		out, diags := format.Source(src)
		if len(diags) > 0 {
			for _, d := range diags {
				diag.Render(stderr, path, src, d)
			}
			status = 1
			continue
		}
		if out == src {
			continue
		}
		if check {
			fmt.Fprintln(stdout, path)
			status = 1
			continue
		}
		if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
			fmt.Fprintf(stderr, "error: cannot write %s: %v\nhelp: check that the file can be written\n", path, err)
			return 2
		}
	}
	return status
}
