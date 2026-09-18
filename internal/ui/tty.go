package ui

import (
	"fmt"
	"io"
	"os"

	"nav/internal/cheat"
	"nav/internal/term"
	"nav/internal/variables"
)

// ErrNoTerminal is returned when there is no terminal to draw on, for
// example when nav's input is a pipe. Callers should fall back to printing
// the matching cheats.
var ErrNoTerminal = fmt.Errorf("no terminal available: %w", term.ErrUnsupported)

// Outcome is the end state of a whole interactive session.
type Outcome struct {
	// Cheat is the cheat the user picked.
	Cheat cheat.Cheat
	// Command is Cheat.Command with its variables filled in. It equals
	// Cheat.Command when the command uses no variables.
	Command string
	// Selected is true when the user picked a cheat and supplied every
	// value it needed. It is false if they quit at either step.
	Selected bool
	// Run is true when the user confirmed that the command should be
	// executed. It is always false when InteractOptions.Confirm was false.
	Run bool
	// Copied is true if the user copied a command to the clipboard.
	Copied bool
}

// InteractOptions configures an interactive session.
type InteractOptions struct {
	// Query pre-fills the search box.
	Query string
	// Copy puts text on the clipboard, or is nil when no clipboard tool was
	// found.
	Copy func(string) error
	// Confirm asks, as a last step, whether the completed command should be
	// run. When false the session ends as soon as the command is complete.
	Confirm bool
}

// Interact runs the whole terminal flow: pick a cheat from the list, fill in
// any `<variable>` placeholders its command contains, and optionally confirm
// that it should be run.
//
// It deliberately talks to /dev/tty rather than to stdin and stdout, so the
// selector still works when nav's output is being piped somewhere — which is
// how the completed command gets used.
func Interact(cheats []cheat.Cheat, opts InteractOptions) (Outcome, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return Outcome{}, ErrNoTerminal
	}
	defer tty.Close()

	fd := int(tty.Fd())
	if !term.IsTerminal(fd) {
		return Outcome{}, ErrNoTerminal
	}

	state, err := term.MakeRaw(fd)
	if err != nil {
		return Outcome{}, fmt.Errorf("could not switch the terminal to raw mode: %w", err)
	}

	// Leaving the terminal in raw mode would break the user's shell, so the
	// teardown has to run whatever happens, panics included.
	io.WriteString(tty, ansiEnterAlt+ansiHideCursor)
	defer func() {
		io.WriteString(tty, ansiShowCursor+ansiExitAlt)
		term.Restore(fd, state)
	}()

	size := func() term.Size { return term.GetSize(fd) }

	// Both steps share one raw-mode session, so moving from the list to the
	// prompts does not flicker through the user's shell, and one keyReader,
	// so keys typed ahead of the prompts are not dropped in between.
	kr := newKeyReader(tty)

	// Input is left unset: keys come from kr, not from Options.
	result, err := runSelector(kr, cheats, Options{
		Output: tty,
		Query:  opts.Query,
		Copy:   opts.Copy,
		Size:   size,
	})
	if err != nil || !result.Selected {
		return Outcome{Copied: result.Copied}, err
	}

	command, ok, err := fill(kr, result.Cheat, tty, size)
	if err != nil {
		return Outcome{Copied: result.Copied}, err
	}
	if !ok {
		return Outcome{Copied: result.Copied}, nil
	}

	outcome := Outcome{
		Cheat:    result.Cheat,
		Command:  command,
		Selected: true,
		Copied:   result.Copied,
	}

	if opts.Confirm {
		// Asking on the alternate screen keeps the whole flow in one place.
		// The screen is restored immediately afterwards, so the command's
		// output lands on the user's normal terminal, not over the top of
		// nav's own drawing.
		outcome.Run, err = runConfirm(kr, ConfirmOptions{
			Output:      tty,
			Command:     command,
			Description: result.Cheat.Description,
			Size:        size,
		})
		if err != nil {
			return Outcome{Copied: result.Copied}, err
		}
	}

	return outcome, nil
}

// fill prompts for every variable in c's command and returns the completed
// command. A command with no variables is returned unchanged without
// bothering the user.
func fill(kr *keyReader, c cheat.Cheat, out io.Writer, size func() term.Size) (string, bool, error) {
	names := variables.Names(c.Command)
	if len(names) == 0 {
		return c.Command, true, nil
	}

	values, ok, err := runPrompts(kr, names, PromptOptions{
		Output:  out,
		Command: c.Command,
		Values:  c.Values,
		Size:    size,
	})
	if err != nil || !ok {
		return "", false, err
	}
	return variables.Substitute(c.Command, values), true, nil
}
