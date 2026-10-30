package interp

import (
	"context"
	"fmt"
	"strconv"

	"github.com/kirannarayanak/fennic/ast"
	"github.com/kirannarayanak/fennic/diag"
)

// TestResult is the outcome of one `test` block. Diag is set when it failed.
type TestResult struct {
	Name string
	Diag *diag.Diagnostic
}

// RunTests runs every `test` block in order, each in a fresh scope over the
// program's globals. Run must have been called first so top-level bindings
// exist.
func (in *Interp) RunTests() []TestResult {
	var results []TestResult
	for _, it := range in.file.Items {
		td, ok := it.(*ast.TestDecl)
		if !ok {
			continue
		}
		results = append(results, TestResult{Name: td.Name, Diag: in.runTest(td)})
	}
	return results
}

func (in *Interp) runTest(td *ast.TestDecl) (failure *diag.Diagnostic) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		switch r := r.(type) {
		case *assertion:
			failure = &diag.Diagnostic{Pos: r.stmt.Cond.Pos(), End: r.stmt.Cond.End(), Msg: in.assertMessage(r)}
		case *propagation:
			x := r.span()
			failure = &diag.Diagnostic{Pos: x.Pos(), End: x.End(), Msg: r.failure.Msg,
				Help: "an unhandled failure fails the test; handle it with `else` if it is expected"}
		case *RuntimeError:
			failure = &r.Diag
		default:
			panic(r)
		}
	}()
	in.evalBlock(td.Body, NewEnv(in.globals), &task{ctx: context.Background()})
	return nil
}

// Report formats test results the way `fennic test` prints them.
func Report(results []TestResult, render func(diag.Diagnostic) string) (string, int) {
	out, failed := "", 0
	for _, r := range results {
		if r.Diag == nil {
			out += fmt.Sprintf("test %s ... ok\n", strconv.Quote(r.Name))
			continue
		}
		failed++
		out += fmt.Sprintf("test %s ... FAIL\n", strconv.Quote(r.Name))
		out += render(*r.Diag)
	}
	out += fmt.Sprintf("\n%d passed, %d failed\n", len(results)-failed, failed)
	return out, failed
}
