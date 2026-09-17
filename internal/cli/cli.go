// Package cli holds nav's command-line entry point: flag parsing, the help
// and version output, and the wiring from flags to the cheat package.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"nav/internal/cheat"
)

// Version is the version nav reports for --version. It is overridable at
// build time with:
//
//	go build -ldflags "-X nav/internal/cli.Version=1.2.3"
var Version = "0.2.0-dev"

// Exit codes returned by Run.
const (
	ExitOK    = 0
	ExitError = 1
	ExitUsage = 2
)

// DefaultCheatDir is where nav looks for cheatsheets when --path is not
// given. Making this configurable is Phase 6.
const DefaultCheatDir = "cheats"

// usageError marks a problem with how the user invoked nav, as opposed to a
// problem doing the work. It maps to ExitUsage.
type usageError struct{ err error }

func (e *usageError) Error() string { return e.err.Error() }
func (e *usageError) Unwrap() error { return e.err }

func usagef(format string, args ...any) error {
	return &usageError{fmt.Errorf(format, args...)}
}

// Run executes nav with the given arguments (not including the program name)
// and returns a process exit code. stdout receives normal output, stderr
// receives errors, so tests can capture both.
func Run(args []string, stdout, stderr io.Writer) int {
	err := run(args, stdout, stderr)
	switch {
	case err == nil:
		return ExitOK
	case errors.Is(err, flag.ErrHelp):
		// --help was requested: not an error.
		return ExitOK
	default:
		fmt.Fprintf(stderr, "nav: %s\n", err)

		var ue *usageError
		if errors.As(err, &ue) {
			fmt.Fprintf(stderr, "\nRun 'nav --help' for usage.\n")
			return ExitUsage
		}
		return ExitError
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("nav", flag.ContinueOnError)
	// Print usage ourselves so --help and a bad flag look the same.
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}

	var (
		showHelp    = fs.Bool("help", false, "show this help text and exit")
		showVersion = fs.Bool("version", false, "print the nav version and exit")
		path        = fs.String("path", DefaultCheatDir, "directory to load .cheat files from")
		list        = fs.Bool("list", false, "print every cheat that was loaded")
	)
	fs.BoolVar(showHelp, "h", false, "shorthand for --help")
	fs.BoolVar(showVersion, "V", false, "shorthand for --version")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) { // -h handled by flag itself
			printUsage(stdout)
			return flag.ErrHelp
		}
		return &usageError{err}
	}

	switch {
	case *showHelp:
		printUsage(stdout)
		return nil
	case *showVersion:
		fmt.Fprintf(stdout, "nav %s\n", Version)
		return nil
	}

	if rest := fs.Args(); len(rest) > 0 {
		return usagef("unexpected argument %q", rest[0])
	}

	cheats, err := cheat.LoadDir(*path)
	// Parse problems are worth reporting even when usable cheats were found.
	var pe cheat.ParseErrors
	if errors.As(err, &pe) {
		for _, p := range pe {
			fmt.Fprintf(stderr, "nav: %s\n", p)
		}
		if len(cheats) == 0 {
			return fmt.Errorf("no usable cheats in %s", *path)
		}
	} else if err != nil {
		return err
	}

	if *list {
		printCheats(stdout, cheats)
		return nil
	}

	// Searching and selecting cheats is Phase 3. Until then, say what we
	// have rather than pretending to do more.
	fmt.Fprintf(stdout, "Loaded %d cheat(s) from %s.\n", len(cheats), *path)
	fmt.Fprintf(stdout, "Interactive search is not implemented yet; use --list to see them.\n")
	return nil
}

// printCheats writes one line per cheat, tags first, then the command.
func printCheats(w io.Writer, cheats []cheat.Cheat) {
	for _, c := range cheats {
		fmt.Fprintf(w, "%s\n    %s\n", c, c.Command)
	}
}

const usageText = `nav - a small command-line cheatsheet tool

Usage:
  nav [flags]

Flags:
  -h, --help          show this help text and exit
  -V, --version       print the nav version and exit
      --path <dir>    directory to load .cheat files from (default "%s")
      --list          print every cheat that was loaded

Cheatsheet format (*%s files):
  %% git, branch                       tags for the cheats that follow
  # Show the current branch name      description of the next command
  git rev-parse --abbrev-ref HEAD     the command itself
  ; this line is a comment            ignored, as are blank lines

Exit codes:
  0  success
  1  error
  2  incorrect usage
`

func printUsage(w io.Writer) {
	fmt.Fprint(w, strings.TrimLeft(fmt.Sprintf(usageText, DefaultCheatDir, cheat.Extension), "\n"))
}

// Main is the thin wrapper main() calls.
func Main() {
	os.Exit(Run(os.Args[1:], os.Stdout, os.Stderr))
}
