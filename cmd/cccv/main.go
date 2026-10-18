// Command cccv is the For My Harness Core CLI.
package main

import (
	"os"

	"github.com/whyjp/cccv/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], os.Stdout, os.Stderr))
}
