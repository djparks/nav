package ui

import (
	"errors"
	"io"
	"strings"

	"nav/internal/term"
)

// ConfirmOptions configures a run of Confirm.
type ConfirmOptions struct {
	// Input supplies raw keypress bytes. Required.
	Input io.Reader
	// Output receives rendered frames. Required.
	Output io.Writer
	// Command is the completed command the user is being asked about.
	Command string
	// Description is shown above the command, for context.
	Description string
	// Size reports the terminal size. Defaults to term.FallbackSize.
	Size func() term.Size
}

// Confirm shows the completed command and asks whether to run it.
//
// The answer defaults to no: only an explicit "y" agrees. Anything else —
// "n", Enter, Esc, Ctrl-C, or the input closing — declines, because running
// a command the user did not mean to run is the one outcome worth going out
// of the way to avoid.
func Confirm(opts ConfirmOptions) (bool, error) {
	if opts.Input == nil || opts.Output == nil {
		return false, errors.New("ui.Confirm: Input and Output are required")
	}
	return runConfirm(newKeyReader(opts.Input), opts)
}

// runConfirm is Confirm with the input reader supplied by the caller, so a
// session can share one reader across all of its screens.
func runConfirm(kr *keyReader, opts ConfirmOptions) (bool, error) {
	size := opts.Size
	if size == nil {
		size = func() term.Size { return term.FallbackSize }
	}

	c := &confirmView{
		command:     opts.Command,
		description: opts.Description,
		size:        size,
		out:         opts.Output,
	}

	act, err := runLoop(kr, c)
	if err != nil {
		return false, err
	}
	return act == actionAccept, nil
}

// confirmView is the yes/no screen shown before a command runs.
type confirmView struct {
	command     string
	description string
	rejected    bool // a key other than y/n was pressed

	size func() term.Size
	out  io.Writer
}

func (c *confirmView) handleKey(k key) action {
	switch k.kind {
	case keyQuit:
		return actionCancel

	case keyEnter:
		// The prompt reads "[y/N]", so Enter takes the default: no.
		return actionCancel

	case keyRune:
		switch k.r {
		case 'y', 'Y':
			return actionAccept
		case 'n', 'N':
			return actionCancel
		}
		// Anything else is neither answer. Say so rather than guessing.
		c.rejected = true
	}
	return actionContinue
}

func (c *confirmView) render() error {
	width := max(c.size().Cols, 20)

	sc := newScreen(width)

	sc.line(ansiDim + "Command:" + ansiReset)
	if c.description != "" {
		sc.line(ansiDim + "  # " + truncate(c.description, width-4) + ansiReset)
	}
	// The command is bold and on its own line: it is the thing the user is
	// being asked to agree to, so it should be the easiest thing to read.
	for _, part := range wrap(c.command, width-2) {
		sc.line(ansiBold + "  " + part + ansiReset)
	}
	sc.line("")

	prompt := "Run it? [y/N] "
	if c.rejected {
		prompt = "Run it? Please answer y or n. [y/N] "
	}
	sc.line(truncate(prompt, width) + ansiReverse + " " + ansiReset)
	sc.line("")
	sc.line(ansiDim + truncate("y run   n or esc do not run, print the command instead", width) + ansiReset)

	return sc.flush(c.out)
}

// wrap breaks text into chunks of at most width columns, so a long command
// is shown in full rather than cut short. The user is agreeing to run this,
// so hiding part of it would be the wrong trade.
func wrap(text string, width int) []string {
	if width <= 0 {
		return []string{""}
	}

	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return []string{""}
	}

	var lines []string
	for len(runes) > width {
		lines = append(lines, string(runes[:width]))
		runes = runes[width:]
	}
	return append(lines, string(runes))
}
