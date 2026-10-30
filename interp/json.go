package interp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/kirannarayanak/fennic/types"
)

// builtinParseJSON decodes a JSON document. In M1 an object becomes an
// untyped record whose fields keep their order in the source; a whole number
// becomes an Int, any other number a Float, and null becomes none.
func builtinParseJSON(in *Interp, c *callSite, args []Value) Value {
	text := in.strArg(c, "json.parse", args)
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	lines := &jsonLines{text: text, of: map[any]int{}}
	v, err := lines.decode(dec)
	if err != nil {
		return &Failure{Msg: jsonError(text, dec, err)}
	}
	if _, extra := dec.Token(); extra != io.EOF {
		return &Failure{Msg: "invalid JSON: unexpected text after the value"}
	}
	target, typed := in.targets[c.node]
	if !typed {
		return v
	}
	out, line, why := in.fromJSON(v, target, "", lines, 1)
	if why != "" {
		return &Failure{Msg: fmt.Sprintf("the JSON does not fit %s at line %d: %s", types.String(target), line, why)}
	}
	return out
}

// jsonLines remembers the line each decoded value starts on, so a value that
// does not fit can be found in the text. Lists and objects are keyed by
// their pointer; their members by `jsonAt`.
type jsonLines struct {
	text string
	of   map[any]int
}

type jsonAt struct {
	parent any
	index  int
}

// lineAt is the line of the first value at or after byte offset off.
func (j *jsonLines) lineAt(off int64) int {
	i := int(off)
	for i < len(j.text) && strings.ContainsRune(" \t\r\n,:", rune(j.text[i])) {
		i++
	}
	return 1 + strings.Count(j.text[:min(i, len(j.text))], "\n")
}

// fromJSON turns a decoded JSON value into the type the checker chose.
// path names the field being converted and line is where the value starts,
// for messages. A value that does not fit gives the line and the reason.
func (in *Interp) fromJSON(v Value, t types.Type, path string, lines *jsonLines, line int) (Value, int, string) {
	at := ""
	if path != "" {
		at = " at `" + path + "`"
	}
	wrong := func() (Value, int, string) {
		return nil, line, fmt.Sprintf("expected %s%s, but the JSON has %s", types.Article(types.String(t)), at, jsonKind(v))
	}
	switch t := t.(type) {
	case *types.Opt:
		if v == None {
			return None, 0, ""
		}
		return in.fromJSON(v, t.Elem, path, lines, line)
	case *types.Basic:
		switch t {
		case types.Int:
			if n, ok := v.(int64); ok {
				return n, 0, ""
			}
		case types.Float:
			// JSON writes 3 and 3.0 alike, so a whole number fits a Float.
			switch n := v.(type) {
			case float64:
				return n, 0, ""
			case int64:
				return float64(n), 0, ""
			}
		case types.Str:
			if s, ok := v.(string); ok {
				return s, 0, ""
			}
		case types.Bool:
			if b, ok := v.(bool); ok {
				return b, 0, ""
			}
		}
		return wrong()
	case *types.List:
		l, ok := v.(*List)
		if !ok {
			return wrong()
		}
		out := make([]Value, len(l.Elems))
		for i, e := range l.Elems {
			el, wl, why := in.fromJSON(e, t.Elem, fmt.Sprintf("%s[%d]", path, i), lines, lines.of[jsonAt{l, i}])
			if why != "" {
				return nil, wl, why
			}
			out[i] = el
		}
		return &List{Elems: out}, 0, ""
	case *types.Named:
		r, ok := v.(*Record)
		if !ok {
			return wrong()
		}
		rt := in.records[t.Name]
		fields := make([]Value, len(t.Fields))
		for i, f := range t.Fields {
			sub := f.Name
			if path != "" {
				sub = path + "." + f.Name
			}
			j := r.Type.index(f.Name)
			if j < 0 {
				if _, opt := f.Type.(*types.Opt); opt {
					fields[i] = None
					continue
				}
				return nil, lines.of[r], fmt.Sprintf("missing field `%s`", sub)
			}
			fv, wl, why := in.fromJSON(r.Fields[j], f.Type, sub, lines, lines.of[jsonAt{r, j}])
			if why != "" {
				return nil, wl, why
			}
			fields[i] = fv
		}
		return &Record{Type: rt, Fields: fields}, 0, ""
	}
	return wrong()
}

// jsonKind names a decoded JSON value the way JSON calls it.
func jsonKind(v Value) string {
	switch v.(type) {
	case int64, float64:
		return "a number"
	case string:
		return "a string"
	case bool:
		return "true or false"
	case *List:
		return "an array"
	case *Record:
		return "an object"
	case noneValue:
		return "null"
	}
	return "something else"
}

func (j *jsonLines) decode(dec *json.Decoder) (Value, error) {
	start := j.lineAt(dec.InputOffset())
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			rt := &RecordType{}
			var fields []Value
			var lineOf []int
			for dec.More() {
				key, err := dec.Token()
				if err != nil {
					return nil, err
				}
				line := j.lineAt(dec.InputOffset())
				v, err := j.decode(dec)
				if err != nil {
					return nil, err
				}
				name := key.(string)
				if i := rt.index(name); i >= 0 {
					fields[i] = v
					lineOf[i] = line
					continue
				}
				rt.Fields = append(rt.Fields, name)
				fields = append(fields, v)
				lineOf = append(lineOf, line)
			}
			if _, err := dec.Token(); err != nil { // }
				return nil, err
			}
			r := &Record{Type: rt, Fields: fields}
			j.of[r] = start
			for i, l := range lineOf {
				j.of[jsonAt{r, i}] = l
			}
			return r, nil
		case '[':
			var elems []Value
			var lineOf []int
			for dec.More() {
				lineOf = append(lineOf, j.lineAt(dec.InputOffset()))
				v, err := j.decode(dec)
				if err != nil {
					return nil, err
				}
				elems = append(elems, v)
			}
			if _, err := dec.Token(); err != nil { // ]
				return nil, err
			}
			l := &List{Elems: elems}
			j.of[l] = start
			for i, ln := range lineOf {
				j.of[jsonAt{l, i}] = ln
			}
			return l, nil
		}
	case json.Number:
		if n, err := t.Int64(); err == nil {
			return n, nil
		}
		f, err := t.Float64()
		if err != nil {
			return nil, err
		}
		return f, nil
	case string:
		return t, nil
	case bool:
		return t, nil
	case nil:
		return None, nil
	}
	return nil, fmt.Errorf("unexpected %v", tok)
}

// jsonError says where the text stopped being JSON, as line:col.
func jsonError(text string, dec *json.Decoder, err error) string {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return "invalid JSON: the text ends before the value is complete"
	}
	// With the token API, a syntax error's offset is the index of the
	// offending character; skip any whitespace in front of it.
	var se *json.SyntaxError
	off := int(dec.InputOffset())
	if errors.As(err, &se) {
		off = int(se.Offset)
	}
	for off >= 0 && off < len(text) && strings.ContainsRune(" \t\r\n", rune(text[off])) {
		off++
	}
	if off < 0 || off >= len(text) {
		return "invalid JSON: " + err.Error()
	}
	line, col := 1, 1
	for _, r := range text[:off] {
		if r == '\n' {
			line, col = line+1, 1
		} else {
			col++
		}
	}
	r, _ := utf8.DecodeRuneInString(text[off:])
	return fmt.Sprintf("invalid JSON at %d:%d: unexpected %s", line, col, show(string(r)))
}
