// Command holee is DEPRECATED: it is a shim over the fic binary ("fic sim zcb"; legacy CSV kept).
// It keeps its old flags and prints a deprecation note to stderr; see the
// README for the mapping of the old flags to fic.
package main

import (
	"os"

	"github.com/konimarti/fixedincome/internal/cli"
)

func main() {
	os.Exit(cli.LegacyHolee(os.Args[1:], os.Stdout, os.Stderr))
}
