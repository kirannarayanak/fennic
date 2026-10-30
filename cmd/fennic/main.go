// Command fennic runs Fennic programs.
package main

import (
	"os"

	"github.com/kirannarayanak/fennic/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], os.Stdout, os.Stderr))
}
