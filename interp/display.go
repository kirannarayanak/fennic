package interp

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Display renders a value the way `"{x}"` shows it: as you would write it in
// source, except that a Str at the top level appears without quotes.
func Display(v Value) string {
	if s, ok := v.(string); ok {
		return s
	}
	var b strings.Builder
	write(&b, v)
	return b.String()
}

func write(b *strings.Builder, v Value) {
	switch v := v.(type) {
	case int64:
		b.WriteString(strconv.FormatInt(v, 10))
	case float64:
		b.WriteString(formatFloat(v))
	case string:
		quote(b, v)
	case bool:
		b.WriteString(strconv.FormatBool(v))
	case noneValue:
		b.WriteString("none")
	case unitValue:
		b.WriteString("()")
	case *List:
		b.WriteByte('[')
		for i, e := range v.Elems {
			if i > 0 {
				b.WriteString(", ")
			}
			write(b, e)
		}
		b.WriteByte(']')
	case *Record:
		if v.Type.Name != "" {
			b.WriteString(v.Type.Name)
			b.WriteByte(' ')
		}
		if len(v.Fields) == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteString("{ ")
		for i, f := range v.Fields {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(v.Type.Fields[i])
			b.WriteString(": ")
			write(b, f)
		}
		b.WriteString(" }")
	case *Map:
		b.WriteString("Map{")
		for i, k := range v.Keys {
			if i > 0 {
				b.WriteString(", ")
			}
			write(b, k)
			b.WriteString(": ")
			write(b, v.Vals[i])
		}
		b.WriteByte('}')
	case Order:
		b.WriteString(string(v))
	case Date:
		fmt.Fprintf(b, "%04d-%02d-%02d", v.Year, v.Month, v.Day)
	case *Channel:
		b.WriteString("<channel>")
	case *Tuple:
		b.WriteByte('(')
		for i, e := range v.Elems {
			if i > 0 {
				b.WriteString(", ")
			}
			write(b, e)
		}
		b.WriteByte(')')
	case *Variant:
		b.WriteString(v.Type.Name)
		if len(v.Fields) == 0 {
			return
		}
		b.WriteByte('(')
		for i, f := range v.Fields {
			if i > 0 {
				b.WriteString(", ")
			}
			write(b, f)
		}
		b.WriteByte(')')
	case *Failure:
		b.WriteString(v.Msg)
	case *Func:
		b.WriteString("<fn " + v.Decl.Name.Name + ">")
	case *Bound:
		b.WriteString("<fn " + v.Func.Decl.Name.Name + ">")
	case *Builtin:
		b.WriteString("<fn " + v.Name + ">")
	case *Closure:
		b.WriteString("<fn>")
	case *TypeRef:
		b.WriteString(v.Name)
	case *Module:
		b.WriteString(v.Name)
	default:
		b.WriteString("<?>")
	}
}

// formatFloat prints the shortest form that reads back as the same Float,
// always with a decimal point so it cannot be mistaken for an Int.
func formatFloat(f float64) string {
	switch {
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	case math.IsNaN(f):
		return "nan"
	}
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}

// quote writes a Str as a Fennic string literal.
func quote(b *strings.Builder, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '{':
			b.WriteString(`\{`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
}
