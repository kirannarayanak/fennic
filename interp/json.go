package interp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// builtinParseJSON decodes a JSON document. In M1 an object becomes an
// untyped record whose fields keep their order in the source; a whole number
// becomes an Int, any other number a Float, and null becomes none.
func builtinParseJSON(in *Interp, c *callSite, args []Value) Value {
	text := in.strArg(c, "json.parse", args)
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	v, err := decodeJSON(dec)
	if err == nil {
		if _, extra := dec.Token(); extra != io.EOF {
			return &Failure{Msg: "invalid JSON: unexpected text after the value"}
		}
		return v
	}
	return &Failure{Msg: jsonError(text, dec, err)}
}

func decodeJSON(dec *json.Decoder) (Value, error) {
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
			for dec.More() {
				key, err := dec.Token()
				if err != nil {
					return nil, err
				}
				v, err := decodeJSON(dec)
				if err != nil {
					return nil, err
				}
				name := key.(string)
				if i := rt.index(name); i >= 0 {
					fields[i] = v
					continue
				}
				rt.Fields = append(rt.Fields, name)
				fields = append(fields, v)
			}
			if _, err := dec.Token(); err != nil { // }
				return nil, err
			}
			return &Record{Type: rt, Fields: fields}, nil
		case '[':
			var elems []Value
			for dec.More() {
				v, err := decodeJSON(dec)
				if err != nil {
					return nil, err
				}
				elems = append(elems, v)
			}
			if _, err := dec.Token(); err != nil { // ]
				return nil, err
			}
			return &List{Elems: elems}, nil
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
