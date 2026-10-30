package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAcceptance is the M1 definition of done: every program in examples/
// runs and its output matches the .out file beside it. See examples/README.md
// for the directives on line 1.
func TestAcceptance(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir("../examples"); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)

	files, _ := filepath.Glob("*.fn")
	if len(files) != 20 {
		t.Fatalf("found %d examples, want 20", len(files))
	}
	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(strings.TrimSuffix(f, ".fn") + ".out")
			if err != nil {
				t.Fatal(err)
			}
			mode, wantCode := "run", 0
			first, _, _ := strings.Cut(string(src), "\n")
			switch strings.TrimSpace(first) {
			case "// expect: test":
				mode = "test"
			case "// expect: exit 1":
				wantCode = 1
			}

			var out bytes.Buffer // stdout and stderr together, as a terminal shows them
			code := Main([]string{mode, f}, &out, &out)
			if code != wantCode {
				t.Errorf("exit status %d, want %d", code, wantCode)
			}
			if got := out.String(); got != string(want) {
				t.Errorf("output differs\n--- got\n%s--- want\n%s", got, want)
			}
		})
	}
}

func TestUsage(t *testing.T) {
	cases := []struct {
		name string
		args []string
		code int
		want string
	}{
		{"no arguments", nil, 2, "usage:"},
		{"unknown command", []string{"build", "x.fn"}, 2, "usage:"},
		{"wrong extension", []string{"run", "x.txt"}, 2, "help: Fennic programs end in .fn"},
		{"missing file", []string{"run", "nope.fn"}, 2, "error: cannot read nope.fn"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out bytes.Buffer
			if code := Main(c.args, &out, &out); code != c.code {
				t.Errorf("exit %d, want %d", code, c.code)
			}
			if !strings.Contains(out.String(), c.want) {
				t.Errorf("output %q does not contain %q", out.String(), c.want)
			}
		})
	}
}
