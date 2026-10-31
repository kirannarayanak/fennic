package interp

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/kirannarayanak/fennic/types"
)

// builtinReadCSV reads a CSV file whose first row names the columns. Each
// later row becomes a record of the type the checker chose: each field takes
// the column of the same name. An empty cell fills an optional field with
// none. A failure names the file, the line and the column.
func builtinReadCSV(in *Interp, c *callSite, args []Value) Value {
	path := in.strArg(c, "csv.read", args)
	if err := c.t.ctx.Err(); err != nil {
		return &Failure{Msg: "cancelled because another task in the same `together` failed", cancelled: true}
	}
	target, ok := in.targets[c.node].(*types.Named)
	if !ok {
		in.internal(c.node, "csv.read needs a record type, which the type checker should have given")
	}
	data, err := in.readFile(path)
	if err != nil {
		return &Failure{Msg: readFailure(path, err)}
	}
	r := csv.NewReader(strings.NewReader(string(data)))
	r.TrimLeadingSpace = true
	header, err := r.Read()
	if err != nil {
		return &Failure{Msg: fmt.Sprintf("%s: %s", path, csvReason(err, "the file has no header row"))}
	}
	cols := map[string]int{}
	for i, h := range header {
		cols[strings.TrimSpace(h)] = i
	}
	for _, f := range target.Fields {
		if _, ok := cols[f.Name]; !ok {
			if _, optional := f.Type.(*types.Opt); !optional {
				return &Failure{Msg: fmt.Sprintf("%s has no column `%s`, which `%s` needs", path, f.Name, target.Name)}
			}
		}
	}
	rt := in.records[target.Name]
	out := &List{}
	for {
		row, err := r.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return out
			}
			return &Failure{Msg: fmt.Sprintf("%s: %s", path, csvReason(err, ""))}
		}
		line, _ := r.FieldPos(0)
		fields := make([]Value, len(target.Fields))
		for i, f := range target.Fields {
			cell := ""
			if j, ok := cols[f.Name]; ok {
				cell = strings.TrimSpace(row[j])
			}
			v, why := cellValue(cell, f.Type)
			if why != "" {
				return &Failure{Msg: fmt.Sprintf("%s line %d, column `%s`: %s", path, line, f.Name, why)}
			}
			fields[i] = v
		}
		out.Elems = append(out.Elems, &Record{Type: rt, Fields: fields})
	}
}

// cellValue turns one CSV cell into a value of type t, or says why not.
func cellValue(cell string, t types.Type) (Value, string) {
	if o, ok := t.(*types.Opt); ok {
		if cell == "" {
			return None, ""
		}
		t = o.Elem
	}
	switch t {
	case types.Str:
		return cell, ""
	case types.Int:
		if n, err := strconv.ParseInt(cell, 10, 64); err == nil {
			return n, ""
		}
	case types.Float:
		if f, err := strconv.ParseFloat(cell, 64); err == nil {
			return f, ""
		}
	case types.Bool:
		switch cell {
		case "true":
			return true, ""
		case "false":
			return false, ""
		}
	case types.Date:
		if d, ok := parseDate(cell); ok {
			return d, ""
		}
		if cell != "" {
			return nil, fmt.Sprintf("%s is not a Date; write dates as YYYY-MM-DD", show(cell))
		}
	}
	if cell == "" {
		return nil, fmt.Sprintf("the cell is empty, but the field is %s; make it optional if it may be empty", types.Article(types.String(t)))
	}
	return nil, fmt.Sprintf("%s is not %s", show(cell), types.Article(types.String(t)))
}

func csvReason(err error, ifEmpty string) string {
	if errors.Is(err, io.EOF) && ifEmpty != "" {
		return ifEmpty
	}
	var pe *csv.ParseError
	if errors.As(err, &pe) {
		return fmt.Sprintf("line %d: %v", pe.Line, pe.Err)
	}
	return err.Error()
}

// parseDate reads a day written YYYY-MM-DD.
func parseDate(s string) (Date, bool) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return Date{}, false
	}
	return Date{t.Year(), int(t.Month()), t.Day()}, true
}
