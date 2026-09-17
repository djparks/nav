// Package term puts a terminal into "raw" mode and reports its size.
//
// A terminal normally works in cooked mode: the kernel buffers a whole line,
// echoes what you type, and only hands the line to the program when you press
// Enter. An interactive list needs the opposite — every keypress delivered
// immediately and nothing echoed — which is what raw mode provides.
//
// This is the same job golang.org/x/term does. It is implemented here with
// the standard library's syscall package so that nav keeps no third-party
// dependencies.
package term

import "errors"

// ErrUnsupported is returned on platforms where nav cannot switch the
// terminal into raw mode.
var ErrUnsupported = errors.New("raw terminal mode is not supported on this platform")

// State holds the terminal settings that were in effect before MakeRaw, so
// Restore can put them back.
type State struct {
	termios termios
}

// Size is the usable character grid of a terminal.
type Size struct {
	Rows, Cols int
}

// FallbackSize is used when the real terminal size cannot be determined.
var FallbackSize = Size{Rows: 24, Cols: 80}
