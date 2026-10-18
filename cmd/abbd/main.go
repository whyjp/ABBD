// Command abbd is the ABBD Core CLI.
package main

import (
	"os"

	"github.com/whyjp/abbd/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], os.Stdout, os.Stderr))
}
