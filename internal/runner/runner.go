// Package runner executes a completed command and reports how it went.
//
// Commands in cheatsheets are written the way you would type them at a
// prompt, so they contain pipes, redirections, quoting and variable
// expansion. Running them therefore means handing the text to a shell rather
// than trying to split it into arguments here.
package runner

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
)

// DefaultShell is used when $SHELL is not set.
const DefaultShell = "/bin/sh"

// Options configures a run. The zero value runs the command with the user's
// shell, connected to the current process's standard streams.
type Options struct {
	// Shell is the shell binary to run the command with. Defaults to Shell().
	Shell string
	// Stdin, Stdout and Stderr are the command's streams. A nil stream is
	// connected to the corresponding stream of this process, so interactive
	// commands and pagers keep working.
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	// Dir is the working directory. Empty means the current one.
	Dir string
	// Env replaces the environment when non-nil. Normally left unset so the
	// command sees the same environment as nav.
	Env []string
}

// Shell reports the shell that commands are run with: $SHELL if it is set,
// and DefaultShell otherwise.
//
// Note that `-c` does not read interactive start-up files, so the user's
// aliases and functions are not available to the command. What using their
// shell does buy is that shell-specific syntax in a cheat behaves the way it
// would if they pasted the command themselves.
func Shell() string {
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	return DefaultShell
}

// Run executes command and returns its exit status.
//
// The returned error is non-nil only when the command could not be started
// at all — a missing shell, say. A command that ran and failed is not an
// error here: it reports a non-zero status, which the caller passes on. The
// command has already explained itself on stderr, so there is nothing useful
// to add.
func Run(command string, opts Options) (status int, err error) {
	shell := opts.Shell
	if shell == "" {
		shell = Shell()
	}

	cmd := exec.Command(shell, "-c", command)
	cmd.Dir = opts.Dir
	cmd.Env = opts.Env

	// Connecting the real streams by default lets the command prompt, page
	// and colour its output exactly as it would when run by hand.
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if opts.Stdin != nil {
		cmd.Stdin = opts.Stdin
	}
	if opts.Stdout != nil {
		cmd.Stdout = opts.Stdout
	}
	if opts.Stderr != nil {
		cmd.Stderr = opts.Stderr
	}

	// While the command runs it owns the terminal, so Ctrl-C is its business.
	// Without this nav would take the SIGINT as well and die before it could
	// report what happened to the command.
	restore := ignoreInterrupts()
	defer restore()

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("could not run %s: %w", shell, err)
	}

	// Wait only errors in ways that ProcessState already describes, so the
	// error itself is not needed: a command that exits non-zero, or is
	// killed by a signal, is reported through the status.
	_ = cmd.Wait()
	return exitStatus(cmd.ProcessState), nil
}

// ignoreInterrupts stops SIGINT and SIGQUIT from reaching nav, and returns a
// function that restores the previous behaviour.
func ignoreInterrupts() func() {
	// Notifying a channel nobody reads is how a Go program ignores a signal
	// without losing the ability to restore the default handler afterwards.
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, interruptSignals()...)
	return func() { signal.Stop(ch) }
}
