// Command fennic runs Fennic programs.
package main

import (
	"os"

	"fennic/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], os.Stdout, os.Stderr))
}
