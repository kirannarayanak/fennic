// Package lsp is `fennic lsp`, the language server: diagnostics as you
// type, formatting, hover and go to definition, over the Language Server
// Protocol on standard input and output. docs/m4.md has what it does.
package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"

	"github.com/kirannarayanak/fennic/ast"
	"github.com/kirannarayanak/fennic/diag"
	"github.com/kirannarayanak/fennic/format"
	"github.com/kirannarayanak/fennic/interp"
	"github.com/kirannarayanak/fennic/parser"
	"github.com/kirannarayanak/fennic/types"
)

// Serve answers messages from in on out until the client says exit or
// closes in, and returns the exit status: 0 after a shutdown request, 1
// otherwise, as the protocol says. Problems in the server go to logw.
func Serve(in io.Reader, out io.Writer, logw io.Writer) int {
	s := &server{out: out, log: logw, docs: map[string]*document{}, enc: utf16}
	r := bufio.NewReader(in)
	for {
		body, err := readMessage(r)
		if err != nil {
			if err != io.EOF {
				fmt.Fprintf(logw, "fennic lsp: %v\n", err)
			}
			return 1
		}
		if done, status := s.handle(body); done {
			return status
		}
	}
}

// readMessage reads one message framed by a Content-Length header.
func readMessage(r *bufio.Reader) ([]byte, error) {
	length := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if err == io.EOF && line == "" {
				return nil, io.EOF
			}
			return nil, fmt.Errorf("reading a header: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if ok && strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return nil, fmt.Errorf("bad Content-Length %q", value)
			}
			length = n
		}
	}
	if length < 0 {
		return nil, fmt.Errorf("a message without Content-Length")
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, fmt.Errorf("reading a message: %w", err)
	}
	return body, nil
}

type server struct {
	out      io.Writer
	log      io.Writer
	docs     map[string]*document
	enc      encoding
	shutdown bool
}

type request struct {
	ID     *json.RawMessage `json:"id"`
	Method string           `json:"method"`
	Params json.RawMessage  `json:"params"`
}

type responseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

const (
	errParse          = -32700
	errInvalidRequest = -32600
	errMethodNotFound = -32601
	errInternal       = -32603
)

func (s *server) send(msg any) {
	body, err := json.Marshal(msg)
	if err != nil {
		fmt.Fprintf(s.log, "fennic lsp: %v\n", err)
		return
	}
	fmt.Fprintf(s.out, "Content-Length: %d\r\n\r\n%s", len(body), body)
}

func (s *server) reply(id *json.RawMessage, result any) {
	s.send(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (s *server) fail(id *json.RawMessage, code int, msg string) {
	s.send(map[string]any{"jsonrpc": "2.0", "id": id, "error": responseError{code, msg}})
}

func (s *server) notify(method string, params any) {
	s.send(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

// handle answers one message. It reports whether the server should stop,
// and with what status. A panic while answering is a bug in the server: it
// is logged, a request gets an internal error, and the server carries on.
func (s *server) handle(body []byte) (done bool, status int) {
	var req request
	if err := json.Unmarshal(body, &req); err != nil {
		s.fail(nil, errParse, "the message is not JSON: "+err.Error())
		return false, 0
	}
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(s.log, "fennic lsp: internal error in %s: %v\n%s", req.Method, r, debug.Stack())
			if req.ID != nil {
				s.fail(req.ID, errInternal, fmt.Sprintf("internal error in fennic: %v", r))
			}
			done = false
		}
	}()
	if req.Method == "" {
		if req.ID != nil {
			s.fail(req.ID, errInvalidRequest, "the message has no method")
		}
		return false, 0
	}
	if req.ID == nil {
		return s.notification(req)
	}
	result, err := s.request(req)
	if err != nil {
		s.fail(req.ID, err.Code, err.Message)
	} else {
		s.reply(req.ID, result)
	}
	return false, 0
}

func (s *server) notification(req request) (bool, int) {
	switch req.Method {
	case "exit":
		if s.shutdown {
			return true, 0
		}
		return true, 1
	case "textDocument/didOpen":
		var p struct {
			TextDocument struct {
				URI     string `json:"uri"`
				Version int    `json:"version"`
				Text    string `json:"text"`
			} `json:"textDocument"`
		}
		if json.Unmarshal(req.Params, &p) == nil {
			s.update(p.TextDocument.URI, p.TextDocument.Version, p.TextDocument.Text)
		}
	case "textDocument/didChange":
		var p struct {
			TextDocument struct {
				URI     string `json:"uri"`
				Version int    `json:"version"`
			} `json:"textDocument"`
			ContentChanges []struct {
				Text string `json:"text"`
			} `json:"contentChanges"`
		}
		if json.Unmarshal(req.Params, &p) == nil && len(p.ContentChanges) > 0 {
			// The server asks for the whole document on every change.
			s.update(p.TextDocument.URI, p.TextDocument.Version, p.ContentChanges[len(p.ContentChanges)-1].Text)
		}
	case "textDocument/didClose":
		var p struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
		}
		if json.Unmarshal(req.Params, &p) == nil {
			delete(s.docs, p.TextDocument.URI)
			s.notify("textDocument/publishDiagnostics", map[string]any{"uri": p.TextDocument.URI, "diagnostics": []any{}})
		}
	}
	// Anything else, `initialized` included, needs no answer.
	return false, 0
}

func (s *server) request(req request) (any, *responseError) {
	switch req.Method {
	case "initialize":
		return s.initialize(req.Params), nil
	case "shutdown":
		s.shutdown = true
		return nil, nil
	case "textDocument/formatting":
		return s.withDoc(req.Params, func(d *document, _ position) any { return d.formatting(s.enc) }), nil
	case "textDocument/hover":
		return s.withDoc(req.Params, func(d *document, p position) any { return d.hover(p, s.enc) }), nil
	case "textDocument/definition":
		return s.withDoc(req.Params, func(d *document, p position) any { return d.definition(p, s.enc) }), nil
	}
	return nil, &responseError{errMethodNotFound, "method not found: " + req.Method}
}

func (s *server) initialize(params json.RawMessage) any {
	var p struct {
		Capabilities struct {
			General struct {
				PositionEncodings []string `json:"positionEncodings"`
			} `json:"general"`
		} `json:"capabilities"`
	}
	_ = json.Unmarshal(params, &p)
	for _, name := range p.Capabilities.General.PositionEncodings {
		if e, ok := encodings[name]; ok {
			s.enc = e
			break
		}
	}
	return map[string]any{
		"capabilities": map[string]any{
			"positionEncoding":           s.enc.name,
			"textDocumentSync":           1, // the whole document on every change
			"documentFormattingProvider": true,
			"hoverProvider":              true,
			"definitionProvider":         true,
		},
		"serverInfo": map[string]any{"name": "fennic"},
	}
}

// withDoc runs f on the document a request names, at its position if it
// has one. A document the server has not been given gets null.
func (s *server) withDoc(params json.RawMessage, f func(*document, position) any) any {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Position position `json:"position"`
	}
	if json.Unmarshal(params, &p) != nil {
		return nil
	}
	d, ok := s.docs[p.TextDocument.URI]
	if !ok {
		return nil
	}
	return f(d, p.Position)
}

// update analyses a document's new text and publishes its diagnostics.
func (s *server) update(uri string, version int, text string) {
	d := analyse(uri, text)
	s.docs[uri] = d
	diags := make([]any, 0, len(d.diags))
	for _, dg := range d.diags {
		msg := dg.Msg
		if dg.Help != "" {
			msg += "\n\nhelp: " + dg.Help
		}
		diags = append(diags, map[string]any{
			"range":    d.span(dg.Pos.Offset, dg.End.Offset, s.enc),
			"severity": 1,
			"source":   "fennic",
			"message":  msg,
		})
	}
	s.notify("textDocument/publishDiagnostics", map[string]any{"uri": uri, "version": version, "diagnostics": diags})
}

// document is one open file and what the checker found in it.
type document struct {
	uri   string
	text  string
	lines []int // the byte offset where each line starts
	diags []diag.Diagnostic
	// parsed is false while the file has syntax errors: then hover,
	// definition and formatting answer nothing.
	parsed bool
	names  map[*ast.Ident]*types.Name
}

func analyse(uri, text string) *document {
	d := &document{uri: uri, text: text, lines: []int{0}}
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			d.lines = append(d.lines, i+1)
		}
	}
	file, diags := parser.Parse(text)
	d.diags = diags
	if len(diags) == 0 {
		d.parsed = true
		d.diags = interp.Check(file, text)
		typeDiags, info := types.Check(file, text)
		d.diags = append(d.diags, typeDiags...)
		d.names = info.Names
		sort.SliceStable(d.diags, func(i, j int) bool { return d.diags[i].Pos.Offset < d.diags[j].Pos.Offset })
	}
	return d
}

func (d *document) formatting(enc encoding) any {
	if !d.parsed {
		return []any{}
	}
	out, diags := format.Source(d.text)
	if len(diags) > 0 || out == d.text {
		return []any{}
	}
	end := d.pos(len(d.text), enc)
	return []any{map[string]any{
		"range":   map[string]any{"start": position{0, 0}, "end": end},
		"newText": out,
	}}
}

// nameAt is the identifier at a position, and what it names.
func (d *document) nameAt(p position, enc encoding) (*ast.Ident, *types.Name) {
	if !d.parsed {
		return nil, nil
	}
	off, ok := d.offset(p, enc)
	if !ok {
		return nil, nil
	}
	for id, n := range d.names {
		if id.Pos().Offset <= off && off < id.End().Offset {
			return id, n
		}
	}
	return nil, nil
}

func (d *document) hover(p position, enc encoding) any {
	id, n := d.nameAt(p, enc)
	if id == nil {
		return nil
	}
	return map[string]any{
		"contents": map[string]any{"kind": "markdown", "value": "```fennic\n" + n.Hover(id) + "\n```"},
		"range":    d.span(id.Pos().Offset, id.End().Offset, enc),
	}
}

func (d *document) definition(p position, enc encoding) any {
	_, n := d.nameAt(p, enc)
	if n == nil || n.Decl == nil {
		return nil
	}
	return map[string]any{"uri": d.uri, "range": d.span(n.Decl.Pos().Offset, n.Decl.End().Offset, enc)}
}
