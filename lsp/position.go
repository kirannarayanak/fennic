package lsp

import "unicode/utf8"

// position is a place in a document as the protocol writes it: a 0-based
// line, and a character offset counted in the session's encoding.
type position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// encoding is how the protocol counts characters within a line. It counts
// UTF-16 code units unless the client offers another; Fennic counts
// characters, so every position is converted.
type encoding struct {
	name  string
	units func(r rune) int
}

var (
	utf16 = encoding{"utf-16", func(r rune) int {
		if r >= 0x10000 {
			return 2
		}
		return 1
	}}
	utf8enc = encoding{"utf-8", func(r rune) int { return utf8.RuneLen(r) }}
	utf32   = encoding{"utf-32", func(rune) int { return 1 }}
)

var encodings = map[string]encoding{"utf-16": utf16, "utf-8": utf8enc, "utf-32": utf32}

// pos converts a byte offset in the text to a protocol position.
func (d *document) pos(offset int, enc encoding) position {
	line := 0
	for line+1 < len(d.lines) && d.lines[line+1] <= offset {
		line++
	}
	n := 0
	for _, r := range d.text[d.lines[line]:offset] {
		n += enc.units(r)
	}
	return position{line, n}
}

// offset converts a protocol position to a byte offset in the text. A
// position past the end of its line is not in the text.
func (d *document) offset(p position, enc encoding) (int, bool) {
	if p.Line < 0 || p.Line >= len(d.lines) || p.Character < 0 {
		return 0, false
	}
	start := d.lines[p.Line]
	n := 0
	for i, r := range d.text[start:] {
		if n >= p.Character {
			return start + i, r != '\n'
		}
		if r == '\n' {
			return 0, false
		}
		n += enc.units(r)
	}
	return 0, false
}

func (d *document) span(from, to int, enc encoding) map[string]any {
	return map[string]any{"start": d.pos(from, enc), "end": d.pos(to, enc)}
}
