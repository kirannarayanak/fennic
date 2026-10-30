package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
	if len(files) < 20 {
		t.Fatalf("found %d examples, want at least the original 20", len(files))
	}
	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			runExpecting(t, f)
		})
	}
}

// runExpecting runs one acceptance program and compares its output with
// the .out file beside it. Lines starting `// expect:` set how it runs:
// `test` runs its test blocks, `exit 1` expects a failure, `args a b` passes
// command-line arguments, and `env NAME=value` sets an environment variable.
func runExpecting(t *testing.T, f string) {
	t.Helper()
	src, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(strings.TrimSuffix(f, ".fn") + ".out")
	if err != nil {
		t.Fatal(err)
	}
	mode, wantCode := "run", 0
	var extra []string
	for _, line := range strings.Split(string(src), "\n") {
		directive, ok := strings.CutPrefix(strings.TrimSpace(line), "// expect: ")
		if !ok {
			continue
		}
		switch {
		case directive == "test":
			mode = "test"
		case directive == "exit 1":
			wantCode = 1
		case strings.HasPrefix(directive, "args "):
			extra = strings.Fields(strings.TrimPrefix(directive, "args "))
		case strings.HasPrefix(directive, "env "):
			name, value, _ := strings.Cut(strings.TrimPrefix(directive, "env "), "=")
			t.Setenv(name, value)
		}
	}
	var out bytes.Buffer // stdout and stderr together, as a terminal shows them
	code := Main(append([]string{mode, f}, extra...), &out, &out)
	if code != wantCode {
		t.Errorf("exit status %d, want %d", code, wantCode)
	}
	if got := out.String(); got != string(want) {
		t.Errorf("output differs\n--- got\n%s--- want\n%s", got, want)
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

// A file with several mistakes reports each one once, in source order, and
// runs nothing.
func TestManyErrors(t *testing.T) {
	var out bytes.Buffer
	if code := Main([]string{"run", "testdata/many_errors.fn"}, &out, &out); code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
	var got []string
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.HasPrefix(line, "error: ") {
			got = append(got, strings.TrimPrefix(line, "error: "))
		}
		if _, loc, ok := strings.Cut(line, "--> testdata/many_errors.fn:"); ok {
			got[len(got)-1] += " @ " + loc
		}
	}
	want := []string{
		"unexpected `,` between fields @ 2:12",
		"parameter `who` needs a type @ 6:10",
		"Fennic has no `return` @ 7:3",
		"there is no `null` in Fennic @ 10:9",
		"`'hi'` uses single quotes @ 11:9",
		"`&&` is not an operator in Fennic @ 12:12",
		"expected `)` to close the `(` on line 13, found the end of the line @ 13:19",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestRejected is the M2 definition of done, with TestAcceptance: every
// program in examples/rejected/ is refused with exactly the error beside it,
// and nothing runs.
func TestRejected(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir("../examples/rejected"); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)

	files, _ := filepath.Glob("*.fn")
	if len(files) < 20 {
		t.Fatalf("found %d rejected programs, want at least the original 20", len(files))
	}
	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			want, err := os.ReadFile(strings.TrimSuffix(f, ".fn") + ".out")
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if code := Main([]string{"run", f}, &out, &out); code != 1 {
				t.Errorf("exit status %d, want 1", code)
			}
			if got := out.String(); got != string(want) {
				t.Errorf("output differs\n--- got\n%s--- want\n%s", got, want)
			}
		})
	}
}

// Behaviour that changed in M2 at run time.
func TestTypedRuntime(t *testing.T) {
	config := "type Config {\n  name: Str\n  port: Int\n  tags: List[Str]\n  owner: Str?\n}\n" +
		"fn parse(text: Str) -> Config! { json.parse(text)! }\n" +
		"fn report(text: Str) { let c = parse(text) else e -> { print(\"{e}\"); Config { name: \"\", port: 0, tags: [], owner: none } } }\n"
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"json builds the declared record", config +
			`let c = parse("\{\"name\": \"a\", \"port\": 80, \"tags\": [\"x\"]}")!` + "\n" +
			`print("{c}")` + "\n" +
			`print("{c == Config { name: "a", port: 80, tags: ["x"], owner: none }}")`,
			"Config { name: \"a\", port: 80, tags: [\"x\"], owner: none }\ntrue\n"},
		{"json missing field fails", config +
			`report("\{\"name\": \"a\"}")`,
			"the JSON does not fit Config at line 1: missing field `port`\n"},
		{"json wrong kind fails", config +
			`report("\{\"name\": \"a\", \"port\": \"80\", \"tags\": []}")`,
			"the JSON does not fit Config at line 1: expected an Int at `port`, but the JSON has a string\n"},
		{"json nested path", config +
			`report("\{\"name\": \"a\", \"port\": 1, \"tags\": [1]}")`,
			"the JSON does not fit Config at line 1: expected a Str at `tags[0]`, but the JSON has a number\n"},
		{"json names the line", config +
			"report(`{\n  \"name\": \"a\",\n  \"port\": 80,\n  \"tags\": [\n    \"x\",\n    7\n  ]\n}`)",
			"the JSON does not fit Config at line 6: expected a Str at `tags[1]`, but the JSON has a number\n"},
		{"json missing field names the object's line", config +
			"report(`[\n]` )\nreport(`\n\n{\"name\": \"a\"}`)",
			"the JSON does not fit Config at line 1: expected a Config, but the JSON has an array\nthe JSON does not fit Config at line 3: missing field `port`\n"},
		{"map stops at the first failure",
			`let xs = ["missing.txt", "also-missing.txt"] |> map(f -> fs.read(f)!) else e -> ["{e}"]` + "\n" + `print("{xs}")`,
			"[\"cannot read \\\"missing.txt\\\": file not found\"]\n"},
		{"each stops at the first failure",
			`["missing.txt", "x"] |> each(f -> print(fs.read(f)!)) else e -> print("stopped: {e}")`,
			"stopped: cannot read \"missing.txt\": file not found\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "prog.fn")
			if err := os.WriteFile(path, []byte(c.src), 0o644); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			code := Main([]string{"run", path}, &out, &out)
			if code != 0 || out.String() != c.want {
				t.Errorf("exit %d\ngot:\n%s\nwant:\n%s", code, out.String(), c.want)
			}
		})
	}
}

// TestM0 is the M3 definition of done: the programs of docs/m0-spec.md
// run as written. Each file in examples/m0 starts with a program's text,
// byte for byte, followed by a preamble; see examples/m0/README.md.
func TestM0(t *testing.T) {
	spec, err := os.ReadFile("../docs/m0-spec.md")
	if err != nil {
		t.Fatal(err)
	}
	programs := map[string]string{}
	for _, m := range regexp.MustCompile("(?s)### (\\d+)\\. [^\\n]*\\n\\n```\\n(.*?)```").FindAllStringSubmatch(string(spec), -1) {
		programs[fmt.Sprintf("%02s", m[1])] = m[2]
	}
	if len(programs) != 20 {
		t.Fatalf("found %d programs in the spec, want 20", len(programs))
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir("../examples/m0"); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)

	files, _ := filepath.Glob("*.fn")
	if len(files) == 0 {
		t.Fatal("no programs in examples/m0")
	}
	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			text, _, _ := strings.Cut(string(src), "\n// ---- preamble: not part of the spec program ----\n")
			if want := programs[f[:2]]; text != want {
				t.Fatalf("the program text differs from docs/m0-spec.md\n--- file\n%s--- spec\n%s", text, want)
			}
			runExpecting(t, f)
		})
	}
}

// csv.read failures name the file, the line and the column.
func TestCSVFailures(t *testing.T) {
	prog := "type R {\n  name: Str\n  n: Int\n  note: Str?\n}\n" +
		"fn main() {\n  let rows = csv.read[R](\"data.csv\") else err -> { print(\"{err}\"); [] }\n  print(\"{rows.len()} rows\")\n}\n"
	cases := []struct{ name, csv, want string }{
		{"good", "name,n,note\na,1,\nb,2,x\n", "2 rows\n"},
		{"columns in any order", "n,name\n1,a\n", "1 rows\n"},
		{"bad cell", "name,n\na,1\nb,two\n", "data.csv line 3, column `n`: \"two\" is not an Int\n0 rows\n"},
		{"empty cell for a required field", "name,n\na,\n", "data.csv line 2, column `n`: the cell is empty, but the field is an Int; make it optional if it may be empty\n0 rows\n"},
		{"missing column", "name,note\na,x\n", "data.csv has no column `n`, which `R` needs\n0 rows\n"},
		{"ragged row", "name,n\na,1,extra\n", "data.csv: line 2: wrong number of fields\n0 rows\n"},
		{"no header", "", "data.csv: the file has no header row\n0 rows\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			os.WriteFile(filepath.Join(dir, "data.csv"), []byte(c.csv), 0o644)
			path := filepath.Join(dir, "prog.fn")
			os.WriteFile(path, []byte(prog), 0o644)
			var out bytes.Buffer
			if code := Main([]string{"run", path}, &out, &out); code != 0 || out.String() != c.want {
				t.Errorf("exit %d\ngot:\n%s\nwant:\n%s", code, out.String(), c.want)
			}
		})
	}
}
