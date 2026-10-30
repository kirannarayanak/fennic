package parser

import (
	"os"
	"path/filepath"
	"testing"
)

// Every prefix of every example is a broken program. Parsing one must
// terminate without panicking; that is what an editor sees while typing.
func TestTruncatedExamplesTerminate(t *testing.T) {
	files, _ := filepath.Glob("../examples/*.fn")
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for n := 0; n <= len(src); n++ {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("%s cut at byte %d: panic: %v", filepath.Base(f), n, r)
					}
				}()
				Parse(string(src[:n]))
			}()
		}
	}
}
