// Package diag holds the diagnostics every stage of Fennic reports.
package diag

import "fennic/token"

// Diagnostic is one problem in a source file. Msg says what is wrong; Help
// says what to write instead. Pos and End bound the source text to underline.
type Diagnostic struct {
	Pos  token.Pos
	End  token.Pos
	Msg  string
	Help string
}
