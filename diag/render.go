package diag

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Render writes a diagnostic in the one format Fennic uses everywhere:
//
//	error: what is wrong
//	 --> file.fn:3:12
//	  |
//	3 | the source line
//	  |            ^^^^
//	help: what to write instead
func Render(w io.Writer, filename, src string, d Diagnostic) {
	num := strconv.Itoa(d.Pos.Line)
	pad := strings.Repeat(" ", len(num))
	fmt.Fprintf(w, "error: %s\n", d.Msg)
	fmt.Fprintf(w, "%s--> %s:%d:%d\n", pad, filename, d.Pos.Line, d.Pos.Col)
	if line, ok := sourceLine(src, d.Pos.Line); ok {
		width := 1
		if d.End.Line == d.Pos.Line && d.End.Col > d.Pos.Col {
			width = d.End.Col - d.Pos.Col
		} else if d.End.Line > d.Pos.Line {
			width = max(1, utf8.RuneCountInString(line)-d.Pos.Col+1)
		}
		fmt.Fprintf(w, "%s |\n", pad)
		fmt.Fprintf(w, "%s | %s\n", num, line)
		fmt.Fprintf(w, "%s | %s%s\n", pad, strings.Repeat(" ", d.Pos.Col-1), strings.Repeat("^", width))
	}
	if d.Help != "" {
		fmt.Fprintf(w, "help: %s\n", d.Help)
	}
}

func sourceLine(src string, n int) (string, bool) {
	lines := strings.Split(src, "\n")
	if n < 1 || n > len(lines) {
		return "", false
	}
	return strings.TrimRight(lines[n-1], "\r"), true
}
