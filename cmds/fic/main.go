// Command fic is the command line interface of the fixedincome library:
// curve fitting, bond, swap and option valuation and Monte Carlo
// simulation. Run "fic help" for the commands and "fic help <command>" for
// their flags; see the README for the conventions (percent, basis points,
// formats, exit codes).
package main

import (
	"os"

	"github.com/konimarti/fixedincome/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
