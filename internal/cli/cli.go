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
	"nav/internal/runner"
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

// commandFailed carries the exit status of a command nav ran, so Run can
// exit with it.
//
// It is not really an error on nav's part: the command ran and said what was
// wrong on its own stderr. Run therefore passes the status on without
// printing anything of its own.
type commandFailed struct{ status int }

func (e *commandFailed) Error() string {
	return fmt.Sprintf("command exited with status %d", e.status)
}

// Run executes nav with the given arguments (not including the program name)
// and returns a process exit code. stdout receives normal output, stderr
// receives errors, so tests can capture both.
func Run(args []string, stdout, stderr io.Writer) int {
	return exitCode(run(args, stdout, stderr), stderr)
}

// exitCode turns the outcome of a run into a process exit code, reporting
// the error on stderr when there is something worth saying.
func exitCode(err error, stderr io.Writer) int {
	switch {
	case err == nil:
		return ExitOK

	case errors.Is(err, flag.ErrHelp):
		// --help was requested: not an error.
		return ExitOK
	}

	// A command that ran and failed has already reported itself on its own
	// stderr, so nav adds nothing and just passes the status on.
	var cf *commandFailed
	if errors.As(err, &cf) {
		return cf.status
	}

	fmt.Fprintf(stderr, "nav: %s\n", err)

	var ue *usageError
	if errors.As(err, &ue) {
		fmt.Fprintf(stderr, "\nRun 'nav --help' for usage.\n")
		return ExitUsage
	}
	return ExitError
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
		printOnly   = fs.Bool("print", false, "print the completed command instead of offering to run it")
		assumeYes   = fs.Bool("yes", false, "run the completed command without asking")
	)
	fs.BoolVar(showHelp, "h", false, "shorthand for --help")
	fs.BoolVar(showVersion, "V", false, "shorthand for --version")
	fs.StringVar(query, "q", "", "shorthand for --query")
	fs.BoolVar(assumeYes, "y", false, "shorthand for --yes")

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
	if *printOnly && *assumeYes {
		return usagef("--print and --yes contradict each other: one prints the command, the other runs it")
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

	mode := askFirst
	switch {
	case *printOnly:
		mode = printOnce
	case *assumeYes:
		mode = runWithoutAsking
	}
	return selectCheat(cheats, *query, mode, stdout, stderr)
}

// execMode is what nav does once it has a completed command.
type execMode int

const (
	askFirst         execMode = iota // show it and ask whether to run it
	runWithoutAsking                 // run it straight away (--yes)
	printOnce                        // only print it (--print)
)

// selectCheat runs the interactive session and then either runs the chosen
// command or prints it. With no terminal to draw on it degrades to listing
// the matches, so `nav -q docker | ...` still does something sensible.
func selectCheat(cheats []cheat.Cheat, query string, mode execMode, stdout, stderr io.Writer) error {
	var copyFn func(string) error
	if clipboard.Available() {
		copyFn = clipboard.Copy
	}

	outcome, err := ui.Interact(cheats, ui.InteractOptions{
		Query:   query,
		Copy:    copyFn,
		Confirm: mode == askFirst,
	})
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

	return finish(outcome, mode, stdout, stderr)
}

// finish acts on the end of an interactive session: run the completed
// command, or print it.
func finish(outcome ui.Outcome, mode execMode, stdout, stderr io.Writer) error {
	if !outcome.Selected {
		// Quitting is a normal way to leave the selector, not a failure.
		if outcome.Copied {
			fmt.Fprintln(stderr, "nav: command copied to the clipboard")
		}
		return nil
	}

	// --yes skips the question, so no confirmation took place and the flag
	// itself is the answer.
	if mode == runWithoutAsking || outcome.Run {
		return execute(outcome.Command, stdout, stderr)
	}

	// Either --print was given, or the user declined to run it. Printing the
	// command is still useful: it can be piped, copied or pasted.
	printSelected(stdout, outcome.Cheat, outcome.Command)
	return nil
}

// execute runs the completed command, showing it first so there is a record
// of what produced the output that follows.
//
// The command inherits nav's own streams, so it can page, colour and prompt
// exactly as it would if the user had typed it. Its exit status becomes
// nav's, which is why a failure is returned as a commandFailed rather than
// as an ordinary error.
func execute(command string, stdout, stderr io.Writer) error {
	// The banner goes to stderr so that stdout carries only the command's
	// own output, keeping `nav --yes -q ... > file` useful.
	fmt.Fprintf(stderr, "%s\n", command)

	status, err := runner.Run(command, runner.Options{
		Stdout: stdout,
		Stderr: stderr,
	})
	if err != nil {
		return err
	}
	if status != 0 {
		return &commandFailed{status: status}
	}
	return nil
}

// printSelected shows the chosen cheat with its variables filled in. The
// command goes on a line of its own so it can be piped or copied; the
// metadata goes above it, commented out.
func printSelected(w io.Writer, c cheat.Cheat, command string) {
	fmt.Fprintf(w, "# %s\n", c.Description)
	fmt.Fprintf(w, "# tags: %s\n", strings.Join(c.Tags, ", "))
	fmt.Fprintf(w, "%s\n", command)
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
Pick a cheat with Enter, answer any <variable> prompts, and nav shows the
completed command and asks whether to run it. Declining prints the command
instead, so it can still be piped or pasted.

Flags:
  -h, --help          show this help text and exit
  -V, --version       print the nav version and exit
      --path <dir>    directory to load %s files from (default "%s")
  -q, --query <text>  search terms; pre-fills the interactive search box
      --list          print matching cheats and exit, without the interactive list
      --print         print the completed command instead of offering to run it
  -y, --yes           run the completed command without asking

Running commands:
  The command is shown in full and run only if you answer y; anything else
  declines. It is run with "$SHELL -c", or /bin/sh when $SHELL is unset, so
  pipes, redirections and quoting work as they would if you typed it. Note
  that -c does not read your shell's start-up files, so aliases and shell
  functions are not available.

  The command inherits nav's own input and output, so it can prompt, page and
  colour its output normally. nav then exits with the command's exit status,
  and with 130 if you interrupt it. Because of that, an exit status of 1 or 2
  after a command has run came from the command, not from nav.

  --print never asks and never runs anything, which is what you want in a
  script or when piping nav's output somewhere. --yes runs without asking.

Keys in the interactive list:
  type              filter the list
  up/down, ^P/^N    move the highlight
  page up/down      move a screenful
  enter             select the highlighted cheat
  ^Y                copy the highlighted command to the clipboard, as written
  ^W / ^U           delete the last word / clear the search box
  esc or ^C         quit without selecting

Variables:
  A command may contain <placeholder> parts. After a cheat is selected nav
  asks for each one in turn, showing the command as it fills in. A variable
  with predefined values gets a filterable list; anything else gets a text
  box. Typing a value the list does not contain is allowed: the predefined
  values are suggestions, not a restriction.

  Predefined values are declared in the cheatsheet with a "$ name:" block,
  one value per indented line:

    # List running containers
    docker ps --format "<format>"

    $ format:
        table {{.Names}}
        json

  A "$" block applies to every cheat in its %% tag section.

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
  $ branch:                           predefined values for <branch>,
      main                            one per indented line

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
