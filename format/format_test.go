package format

import (
	"github.com/kirannarayanak/fennic/ast"
	"github.com/kirannarayanak/fennic/lexer"
	"github.com/kirannarayanak/fennic/parser"
	"github.com/kirannarayanak/fennic/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The pairs in examples/m4/fmt: each .in must format to its .fn.
func TestPairs(t *testing.T) {
	ins, _ := filepath.Glob("../examples/m4/fmt/*.in")
	if len(ins) < 12 {
		t.Fatalf("found %d formatter pairs, want 12", len(ins))
	}
	for _, in := range ins {
		name := strings.TrimSuffix(filepath.Base(in), ".in")
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(in)
			if err != nil {
				t.Fatal(err)
			}
			got, diags := Source(string(src))
			want, err := os.ReadFile(strings.TrimSuffix(in, ".in") + ".fn")
			if os.IsNotExist(err) {
				// A file that does not parse is left alone.
				if len(diags) == 0 || got != string(src) {
					t.Errorf("formatted a file that does not parse:\n%s", got)
				}
				return
			}
			if len(diags) > 0 {
				t.Fatalf("does not parse: %s", diags[0].Msg)
			}
			if got != string(want) {
				t.Errorf("got\n%s\nwant\n%s", got, want)
			}
		})
	}
}

// corpus is every .fn file in the repository that parses.
func corpus(t *testing.T) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir("..", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") && path != ".." {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".fn") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if _, diags := parser.Parse(string(src)); len(diags) == 0 {
			files[path] = string(src)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 100 {
		t.Fatalf("found %d .fn files, want the whole repository", len(files))
	}
	return files
}

func tree(src string) string {
	f, _ := parser.Parse(src)
	var b strings.Builder
	for _, it := range f.Items {
		b.WriteString(ast.Dump(it))
		b.WriteString("\n")
	}
	return b.String()
}

func commentTexts(src string) []string {
	_, cs, _ := lexer.LexComments(src)
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Text
	}
	return out
}

// What formatting guarantees, over every file: the same program, the same
// comments in the same order, and nothing more to change the second time.
func TestCorpusGuarantees(t *testing.T) {
	for path, src := range corpus(t) {
		t.Run(path, func(t *testing.T) {
			once, diags := Source(src)
			if len(diags) > 0 {
				t.Fatalf("formatting broke it: %s", diags[0].Msg)
			}
			if once != src {
				if _, diags := parser.Parse(once); len(diags) > 0 {
					t.Fatalf("formatted text does not parse: %s at %v\n%s", diags[0].Msg, diags[0].Pos, once)
				}
			}
			if tree(once) != tree(src) {
				t.Errorf("the program changed:\n--- before\n%s--- after\n%s", tree(src), tree(once))
			}
			if a, b := strings.Join(commentTexts(src), "\n"), strings.Join(commentTexts(once), "\n"); a != b {
				t.Errorf("comments changed:\n--- before\n%s\n--- after\n%s", a, b)
			}
			if twice, _ := Source(once); twice != once {
				t.Errorf("not idempotent:\n--- once\n%s--- twice\n%s", once, twice)
			}
		})
	}
}

// The repository is formatted: every file is already in canonical form.
func TestCorpusIsFormatted(t *testing.T) {
	for path, src := range corpus(t) {
		if got, _ := Source(src); got != src {
			t.Errorf("%s is not formatted; run `fennic fmt %s`", path, path)
		}
	}
}

// Spacing and indentation carry no meaning: any file, with its spaces and
// indentation scrambled, formats back to the canonical text.
func TestScrambledFormatsBack(t *testing.T) {
	for path, src := range corpus(t) {
		want, _ := Source(src)
		t.Run(path, func(t *testing.T) {
			got, diags := Source(scramble(want))
			if len(diags) > 0 {
				t.Fatalf("scrambled text does not parse: %s at %v\n%s", diags[0].Msg, diags[0].Pos, scramble(want))
			}
			if got != want {
				t.Errorf("got\n%s\nwant\n%s", got, want)
			}
		})
	}
}

// scramble rewrites src with the same tokens and line breaks, but no
// indentation and three spaces between tokens, or none where both sides
// are punctuation.
func scramble(src string) string {
	toks, comments, _ := lexer.LexComments(src)
	var b strings.Builder
	line, ci := 1, 0
	prevEnd := -1
	emit := func(text string, p, e token.Pos) {
		for line < p.Line {
			b.WriteString("\n")
			line++
			prevEnd = -1
		}
		if prevEnd >= 0 {
			b.WriteString("   ")
		} else if line%2 == 0 {
			b.WriteString("\t   ")
		}
		b.WriteString(text)
		line = e.Line
		prevEnd = e.Offset
	}
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		for ci < len(comments) && comments[ci].Pos.Offset < t.Pos.Offset {
			emit(comments[ci].Text, comments[ci].Pos, comments[ci].End)
			ci++
		}
		if t.Kind == token.Newline || t.Kind == token.EOF {
			continue
		}
		end := t.End
		if t.Kind == token.StrHead {
			// The whole interpolated string, as written.
			for depth := 0; ; i++ {
				switch toks[i].Kind {
				case token.StrHead:
					depth++
				case token.StrTail:
					depth--
				}
				if depth == 0 {
					end = toks[i].End
					break
				}
			}
		}
		emit(src[t.Pos.Offset:end.Offset], t.Pos, end)
	}
	for ; ci < len(comments); ci++ {
		emit(comments[ci].Text, comments[ci].Pos, comments[ci].End)
	}
	return b.String() + "\n"
}

// The code in the docs is formatted too, where it is a whole program:
// fragments with `...` in them do not parse and are left as written.
func TestDocsAreFormatted(t *testing.T) {
	docs, _ := filepath.Glob("../*.md")
	more, _ := filepath.Glob("../docs/*.md")
	examples, _ := filepath.Glob("../examples/*/README.md")
	for _, path := range append(append(docs, more...), examples...) {
		text, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		blocks := strings.Split(string(text), "```")
		for i := 1; i < len(blocks); i += 2 {
			lang, code, _ := strings.Cut(blocks[i], "\n")
			if lang != "" && lang != "fennic" {
				continue
			}
			if _, diags := parser.Parse(code); len(diags) > 0 {
				continue
			}
			if got, _ := Source(code); got != code {
				line := strings.Count(strings.Join(blocks[:i], "```"), "\n") + 1
				t.Errorf("%s:%d: the code is not formatted; it should read\n%s", path, line, got)
			}
		}
	}
}
