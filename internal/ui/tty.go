package ui

import (
	"fmt"
	"io"
	"os"

	"nav/internal/cheat"
	"nav/internal/term"
)

// ErrNoTerminal is returned by RunTTY when there is no terminal to draw on,
// for example when nav's input is a pipe. Callers should fall back to
// printing the matching cheats.
var ErrNoTerminal = fmt.Errorf("no terminal available: %w", term.ErrUnsupported)

// RunTTY is Run wrapped in the terminal handling it needs: raw mode, the
// alternate screen, and a hidden cursor, each undone on the way out.
//
// It deliberately talks to /dev/tty rather than to stdin and stdout, so the
// selector still works when nav's output is being piped somewhere — which is
// how the selected command gets used.
func RunTTY(cheats []cheat.Cheat, query string, copyFn func(string) error) (Result, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return Result{}, ErrNoTerminal
	}
	defer tty.Close()

	fd := int(tty.Fd())
	if !term.IsTerminal(fd) {
		return Result{}, ErrNoTerminal
	}

	state, err := term.MakeRaw(fd)
	if err != nil {
		return Result{}, fmt.Errorf("could not switch the terminal to raw mode: %w", err)
	}

	// Leaving the terminal in raw mode would break the user's shell, so the
	// teardown has to run whatever happens, panics included.
	io.WriteString(tty, ansiEnterAlt+ansiHideCursor)
	defer func() {
		io.WriteString(tty, ansiShowCursor+ansiExitAlt)
		term.Restore(fd, state)
	}()

	return Run(cheats, Options{
		Input:  tty,
		Output: tty,
		Query:  query,
		Copy:   copyFn,
		Size:   func() term.Size { return term.GetSize(fd) },
	})
}
