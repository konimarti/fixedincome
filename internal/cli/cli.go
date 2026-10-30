// Package cli implements the commands of the fic binary (and the legacy
// command line tools, which are thin shims over it).
//
// Conventions at the command line boundary (see units.go, the only place
// where units are converted): rates, yields, volatilities and dividend
// yields are given and printed in PERCENT, spreads in BASIS POINTS, dates as
// YYYY-MM-DD (bond and curve CSV files also accept DD.MM.YYYY), maturities
// and tenors as dates or as "6m", "2y". The library works with decimals.
//
// Exit codes of Run: 0 success, 1 runtime or numerical error (including
// unreadable files and failing computations), 2 usage error (unknown
// command or flag, invalid flag value, invalid input that the user has to
// correct on the command line). Errors are printed as one line
// "fic <command>: <message>" to stderr; stdout carries data only.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// Version is the version of the fic binary.
const Version = "0.5.0"

// Exit codes of Run.
const (
	ExitOK      = 0
	ExitError   = 1
	ExitUsage   = 2
	curveEnvVar = "FIC_CURVE"
	// curveEnvVarOld is the deprecated name of curveEnvVar
	curveEnvVarOld = "FI_CURVE"
)

// usageError is an error that is reported with exit code 2.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// usagef returns a usage error (exit code 2).
func usagef(format string, args ...any) error {
	return &usageError{msg: fmt.Sprintf(format, args...)}
}

// env is the execution environment of a command.
type env struct {
	stdin          io.Reader
	stdout, stderr io.Writer
	// name is the command name, e.g. "bond price"
	name string
}

// warnf prints a warning to stderr.
func (e *env) warnf(format string, args ...any) {
	fmt.Fprintf(e.stderr, "fic %s: warning: %s\n", e.name, oneLine(fmt.Sprintf(format, args...)))
}

// infof prints a diagnostic message to stderr.
func (e *env) infof(format string, args ...any) {
	fmt.Fprintf(e.stderr, "fic %s: %s\n", e.name, oneLine(fmt.Sprintf(format, args...)))
}

// oneLine replaces line breaks by "; ".
func oneLine(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\r", "")
	return strings.ReplaceAll(s, "\n", "; ")
}

// command is a subcommand of fic.
type command struct {
	// name is the full name, e.g. "curve fit"
	name    string
	summary string
	// build registers the flags in a new flag set and returns the function
	// that executes the command after the flags have been parsed
	build func(e *env) (*flag.FlagSet, func() error)
}

// commands returns the subcommands in the order of the help.
func commands() []command {
	return []command{
		{"curve fit", "fit a Nelson-Siegel-Svensson or spline curve to bond prices", buildCurveFit},
		{"curve fetch", "fetch the NSS curve parameters of the SNB or the ECB", buildCurveFetch},
		{"curve show", "zero rates, discount factors and forwards of a curve", buildCurveShow},
		{"curve forward", "forward rates of a given tenor along a curve", buildCurveForward},
		{"curve template", "print a template curve file", buildCurveTemplate},
		{"bond price", "price, yield, z-spread and risk of fixed-coupon bonds", buildBondPrice},
		{"swap rate", "par swap rates for a set of tenors", buildSwapRate},
		{"option price", "Black-Scholes price and Greeks of a European option", buildOptionPrice},
		{"sim zcb", "Monte Carlo zero bond prices of a short rate model vs. the curve", buildSimZCB},
	}
}

// Run executes the fic command line args (without the program name), reading
// stdin where a command takes "-" as input, and returns the exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	return RunIO(args, os.Stdin, stdout, stderr)
}

// RunIO is Run with an explicit standard input.
func RunIO(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	e := &env{stdin: stdin, stdout: stdout, stderr: stderr, name: ""}
	if len(args) == 0 {
		printHelp(stderr, nil)
		return ExitUsage
	}
	switch args[0] {
	case "help", "-h", "--help", "-help":
		return runHelp(e, args[1:])
	case "version", "--version", "-version":
		fmt.Fprintf(stdout, "fic %s\n", Version)
		return ExitOK
	}
	cmd, rest, ok := lookup(args)
	if !ok {
		e.name = args[0]
		if group := groupCommands(args[0]); len(group) > 0 {
			if len(args) > 1 && !strings.HasPrefix(args[1], "-") {
				return fail(e, usagef("unknown subcommand %q (try: %s)", args[1], strings.Join(subNames(group), ", ")))
			}
			if len(args) > 1 && (args[1] == "-h" || args[1] == "--help") {
				printHelp(stdout, group)
				return ExitOK
			}
			return fail(e, usagef("missing subcommand (try: %s)", strings.Join(subNames(group), ", ")))
		}
		return fail(e, usagef("unknown command %q (run \"fic help\")", args[0]))
	}
	e.name = cmd.name
	fs, exec := cmd.build(e)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(rest); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printCommandHelp(stdout, cmd, fs)
			return ExitOK
		}
		return fail(e, usagef("%v", err))
	}
	if fs.NArg() > 0 {
		return fail(e, usagef("unexpected argument %q", fs.Arg(0)))
	}
	return fail(e, exec())
}

// fail reports err (if any) and returns the exit code.
func fail(e *env, err error) int {
	if err == nil {
		return ExitOK
	}
	var ue *usageError
	prefix := "fic"
	if e.name != "" {
		prefix = "fic " + e.name
	}
	fmt.Fprintf(e.stderr, "%s: %s\n", prefix, oneLine(err.Error()))
	if errors.As(err, &ue) {
		return ExitUsage
	}
	return ExitError
}

// lookup finds the command for args and returns the remaining arguments.
func lookup(args []string) (command, []string, bool) {
	for _, c := range commands() {
		parts := strings.Fields(c.name)
		if len(args) >= len(parts) && strings.Join(args[:len(parts)], " ") == c.name {
			return c, args[len(parts):], true
		}
	}
	return command{}, nil, false
}

func groupCommands(group string) []command {
	var out []command
	for _, c := range commands() {
		if strings.HasPrefix(c.name, group+" ") {
			out = append(out, c)
		}
	}
	return out
}

func subNames(cs []command) []string {
	var s []string
	for _, c := range cs {
		s = append(s, strings.Fields(c.name)[1])
	}
	return s
}

// runHelp prints the help for "fic help [command]".
func runHelp(e *env, args []string) int {
	if len(args) == 0 {
		printHelp(e.stdout, nil)
		return ExitOK
	}
	args = strings.Fields(strings.Join(args, " "))
	if cmd, _, ok := lookup(args); ok {
		e.name = cmd.name
		fs, _ := cmd.build(e)
		printCommandHelp(e.stdout, cmd, fs)
		return ExitOK
	}
	if g := groupCommands(args[0]); len(g) > 0 && len(args) == 1 {
		printHelp(e.stdout, g)
		return ExitOK
	}
	e.name = "help"
	return fail(e, usagef("unknown command %q", strings.Join(args, " ")))
}

const helpHeader = `fic - fixed income valuation tool

Usage: fic <command> [flags]

Conventions: rates, yields, volatilities and dividend yields in percent,
spreads in basis points, dates as YYYY-MM-DD, tenors as 6m, 2y. Rates are
continuously compounded unless marked _annual. Data goes to stdout,
diagnostics to stderr. Exit codes: 0 ok, 1 runtime error, 2 usage error.
`

// printHelp prints the command overview (all commands or a group).
func printHelp(w io.Writer, group []command) {
	cs := group
	if cs == nil {
		fmt.Fprint(w, helpHeader)
		cs = commands()
	}
	fmt.Fprintln(w, "\nCommands:")
	for _, c := range cs {
		fmt.Fprintf(w, "  fic %-15s %s\n", c.name, c.summary)
	}
	if group == nil {
		fmt.Fprintf(w, "  fic %-15s %s\n", "version", "print the version")
		fmt.Fprintf(w, "  fic %-15s %s\n", "help [command]", "show the flags of a command")
	}
}

// printCommandHelp prints the flags of a command with double dashes.
func printCommandHelp(w io.Writer, c command, fs *flag.FlagSet) {
	fmt.Fprintf(w, "fic %s - %s\n\nFlags:\n", c.name, c.summary)
	var names []string
	fs.VisitAll(func(f *flag.Flag) { names = append(names, f.Name) })
	sort.Strings(names)
	for _, n := range names {
		f := fs.Lookup(n)
		arg, usage := flag.UnquoteUsage(f)
		head := "--" + f.Name
		if len(f.Name) == 1 {
			head = "-" + f.Name
		}
		if arg != "" {
			head += " " + arg
		}
		line := "  " + head
		if len(head) < 24 {
			line += strings.Repeat(" ", 24-len(head))
		} else {
			line += "\n" + strings.Repeat(" ", 26)
		}
		line += usage
		if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "0" && f.DefValue != "NaN" {
			line += fmt.Sprintf(" (default %s)", f.DefValue)
		}
		fmt.Fprintln(w, line)
	}
}

// setFlags returns the names of the flags that were set on the command line.
func setFlags(fs *flag.FlagSet) map[string]bool {
	m := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { m[f.Name] = true })
	return m
}
