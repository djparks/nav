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
	"nav/internal/clipboard"
	"nav/internal/search"
	"nav/internal/ui"
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
		query       = fs.String("query", "", "search terms; pre-fills the interactive search box")
	)
	fs.BoolVar(showHelp, "h", false, "shorthand for --help")
	fs.BoolVar(showVersion, "V", false, "shorthand for --version")
	fs.StringVar(query, "q", "", "shorthand for --query")

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

	// --list is the non-interactive path: print what matches and stop.
	if *list {
		matches := search.Filter(cheats, *query)
		if len(matches) == 0 {
			return noMatchesError(*query)
		}
		printCheats(stdout, matches)
		return nil
	}

	return selectCheat(cheats, *query, stdout, stderr)
}

// selectCheat runs the interactive selector and prints whatever the user
// chose. With no terminal to draw on it degrades to listing the matches, so
// `nav -q docker | ...` still does something sensible.
func selectCheat(cheats []cheat.Cheat, query string, stdout, stderr io.Writer) error {
	var copyFn func(string) error
	if clipboard.Available() {
		copyFn = clipboard.Copy
	}

	result, err := ui.RunTTY(cheats, query, copyFn)
	if errors.Is(err, ui.ErrNoTerminal) {
		matches := search.Filter(cheats, query)
		if len(matches) == 0 {
			return noMatchesError(query)
		}
		fmt.Fprintf(stderr, "nav: no terminal available, listing %d matching cheat(s) instead\n", len(matches))
		printCheats(stdout, matches)
		return nil
	}
	if err != nil {
		return err
	}

	if !result.Selected {
		// Quitting is a normal way to leave the selector, not a failure.
		if result.Copied {
			fmt.Fprintln(stderr, "nav: command copied to the clipboard")
		}
		return nil
	}

	printSelected(stdout, result.Cheat)
	return nil
}

// printSelected shows the chosen cheat. The command goes on a line of its
// own so it can be piped or copied; the metadata goes above it.
func printSelected(w io.Writer, c cheat.Cheat) {
	fmt.Fprintf(w, "# %s\n", c.Description)
	fmt.Fprintf(w, "# tags: %s\n", strings.Join(c.Tags, ", "))
	fmt.Fprintf(w, "%s\n", c.Command)
}

// noMatchesError explains that a search came up empty.
func noMatchesError(query string) error {
	if query == "" {
		return errors.New("no cheats to show")
	}
	return fmt.Errorf("no cheats match %q", query)
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

Running nav with no flags opens an interactive list that filters as you type.
Pick a cheat with Enter to print it, or copy it straight to the clipboard.

Flags:
  -h, --help          show this help text and exit
  -V, --version       print the nav version and exit
      --path <dir>    directory to load %s files from (default "%s")
  -q, --query <text>  search terms; pre-fills the interactive search box
      --list          print matching cheats and exit, without the interactive list

Keys in the interactive list:
  type              filter the list
  up/down, ^P/^N    move the highlight
  page up/down      move a screenful
  enter             select the highlighted cheat and print it
  ^Y                copy the highlighted command to the clipboard
  ^W / ^U           delete the last word / clear the search box
  esc or ^C         quit without selecting

Search terms are matched case-insensitively against a cheat's tags,
description and command text. Every term must match, in any order, so
"docker ps" finds cheats mentioning both. A term can be limited to one field:

  nav -q 'tag:git branch'     'branch' may match anywhere, 'git' only in tags
  nav -q 'cmd:rev-parse'      match only against the command text
  nav -q 'desc:current'       match only against the description

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
	fmt.Fprint(w, strings.TrimLeft(fmt.Sprintf(usageText,
		cheat.Extension, DefaultCheatDir, cheat.Extension), "\n"))
}

// Main is the thin wrapper main() calls.
func Main() {
	os.Exit(Run(os.Args[1:], os.Stdout, os.Stderr))
}
