package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// client drives a server over pipes, as an editor would.
type client struct {
	t      *testing.T
	w      *io.PipeWriter
	r      *bufio.Reader
	status chan int
	msgs   chan []byte
	errs   chan error
}

func start(t *testing.T) *client {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	c := &client{t: t, w: inW, r: bufio.NewReader(outR), status: make(chan int, 1), msgs: make(chan []byte, 100), errs: make(chan error, 1)}
	go func() {
		c.status <- Serve(inR, outW, io.Discard)
		outW.Close()
	}()
	go func() {
		for {
			body, err := readMessage(c.r)
			if err != nil {
				c.errs <- err
				return
			}
			c.msgs <- body
		}
	}()
	return c
}

func (c *client) send(msg json.RawMessage) {
	c.t.Helper()
	if _, err := fmt.Fprintf(c.w, "Content-Length: %d\r\n\r\n%s", len(msg), msg); err != nil {
		c.t.Fatalf("sending: %v", err)
	}
}

// next is the next message from the server, or nil when it has stopped.
func (c *client) next() []byte {
	c.t.Helper()
	select {
	case m := <-c.msgs:
		return m
	case <-c.errs:
		return nil
	case <-time.After(5 * time.Second):
		c.t.Fatal("no answer from the server in 5s")
		return nil
	}
}

// The scripted sessions in examples/m4/lsp (docs/m4.md, part 3).
func TestSessions(t *testing.T) {
	files, _ := filepath.Glob("../examples/m4/lsp/*.json")
	if len(files) < 9 {
		t.Fatalf("found %d sessions, want 9", len(files))
	}
	for _, f := range files {
		t.Run(strings.TrimSuffix(filepath.Base(f), ".json"), func(t *testing.T) {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			var session struct {
				Steps []struct {
					Send   json.RawMessage `json:"send"`
					Expect json.RawMessage `json:"expect"`
					Exit   *int            `json:"exit"`
				} `json:"steps"`
			}
			if err := json.Unmarshal(data, &session); err != nil {
				t.Fatal(err)
			}
			c := start(t)
			for i, step := range session.Steps {
				switch {
				case step.Send != nil:
					c.send(step.Send)
				case step.Expect != nil:
					got := c.next()
					if !sameJSON(got, step.Expect) {
						t.Fatalf("step %d:\ngot  %s\nwant %s", i, got, compact(step.Expect))
					}
				case step.Exit != nil:
					select {
					case status := <-c.status:
						if status != *step.Exit {
							t.Errorf("exit status %d, want %d", status, *step.Exit)
						}
					case <-time.After(5 * time.Second):
						t.Fatal("the server did not exit")
					}
					if extra := c.next(); extra != nil {
						t.Errorf("an unexpected message after the last step: %s", extra)
					}
				}
			}
		})
	}
}

func sameJSON(a, b []byte) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}

func compact(b []byte) string {
	var x any
	_ = json.Unmarshal(b, &x)
	out, _ := json.Marshal(x)
	return string(out)
}

// inProcess is a server answering into a buffer, for tests that send many
// messages.
func inProcess() (*server, *strings.Builder) {
	var out strings.Builder
	return &server{out: &out, log: io.Discard, docs: map[string]*document{}, enc: utf16}, &out
}

func message(id int, method string, params any) []byte {
	m := map[string]any{"jsonrpc": "2.0", "method": method, "params": params}
	if id > 0 {
		m["id"] = id
	}
	b, _ := json.Marshal(m)
	return b
}

// answers splits what the server wrote into its messages, and fails the
// test on any that is not well formed or is an error.
func answers(t *testing.T, out string) []map[string]any {
	t.Helper()
	var msgs []map[string]any
	r := bufio.NewReader(strings.NewReader(out))
	for {
		body, err := readMessage(r)
		if err == io.EOF {
			return msgs
		}
		if err != nil {
			t.Fatalf("badly framed output: %v", err)
		}
		var m map[string]any
		if err := json.Unmarshal(body, &m); err != nil {
			t.Fatalf("not JSON: %s", body)
		}
		if m["error"] != nil {
			t.Fatalf("an error answer: %s", body)
		}
		msgs = append(msgs, m)
	}
}

// The server never crashes: every prefix of every example, sent as one
// edit after another, gets its diagnostics, and hover and definition at
// every position of each example get a well-formed answer.
func TestEveryPrefix(t *testing.T) {
	files, _ := filepath.Glob("../examples/*.fn")
	m0, _ := filepath.Glob("../examples/m0/*.fn")
	files = append(files, m0...)
	if testing.Short() {
		files = files[:3]
	}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		text := string(src)
		t.Run(filepath.Base(f), func(t *testing.T) {
			s, out := inProcess()
			uri := "file:///work/main.fn"
			s.handle(message(0, "textDocument/didOpen", map[string]any{"textDocument": map[string]any{"uri": uri, "version": 1, "text": ""}}))
			v := 1
			for i := range text {
				v++
				s.handle(message(0, "textDocument/didChange", map[string]any{
					"textDocument": map[string]any{"uri": uri, "version": v}, "contentChanges": []any{map[string]any{"text": text[:i]}}}))
			}
			s.handle(message(0, "textDocument/didChange", map[string]any{
				"textDocument": map[string]any{"uri": uri, "version": v + 1}, "contentChanges": []any{map[string]any{"text": text}}}))
			id := 0
			for line, l := range strings.Split(text, "\n") {
				for ch := 0; ch <= len([]rune(l)); ch++ {
					for _, method := range []string{"textDocument/hover", "textDocument/definition"} {
						id++
						s.handle(message(id, method, map[string]any{"textDocument": map[string]any{"uri": uri}, "position": position{line, ch}}))
					}
				}
			}
			id++
			s.handle(message(id, "textDocument/formatting", map[string]any{"textDocument": map[string]any{"uri": uri}, "options": map[string]any{}}))
			msgs := answers(t, out.String())
			if want := len(text) + 2 + id; len(msgs) != want {
				t.Errorf("%d answers, want %d", len(msgs), want)
			}
		})
	}
}

// A panic while answering is reported as an internal error for that
// request, and the server keeps answering.
func TestPanicIsAnInternalError(t *testing.T) {
	s, out := inProcess()
	s.docs["file:///broken.fn"] = nil // not a document: hover on it panics
	s.handle(message(1, "textDocument/hover", map[string]any{"textDocument": map[string]any{"uri": "file:///broken.fn"}, "position": position{0, 0}}))
	if done, _ := s.handle(message(2, "shutdown", nil)); done {
		t.Fatal("the server stopped")
	}
	r := bufio.NewReader(strings.NewReader(out.String()))
	first, _ := readMessage(r)
	second, _ := readMessage(r)
	if !sameJSON(first, []byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32603,"message":"internal error in fennic: runtime error: invalid memory address or nil pointer dereference"}}`)) {
		t.Errorf("got %s", first)
	}
	if !sameJSON(second, []byte(`{"jsonrpc":"2.0","id":2,"result":null}`)) {
		t.Errorf("after the panic, got %s", second)
	}
}
