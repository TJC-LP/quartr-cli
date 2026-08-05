// Command quartr is a dependency-free Go CLI for the Quartr Public API v3.
//
// See README.md and `quartr --help` for usage.
package main

import (
	"os"

	"github.com/TJC-LP/quartr-cli/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
