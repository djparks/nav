package ui

import (
	"io"
	"strings"
)

// ANSI escape sequences. Kept as named constants so the drawing code stays
// readable.
const (
	ansiHome       = "\x1b[H"      // move the cursor to the top-left
	ansiClearBelow = "\x1b[J"      // erase from the cursor to the end of screen
	ansiClearLine  = "\x1b[K"      // erase from the cursor to the end of line
	ansiReverse    = "\x1b[7m"     // swap foreground and background
	ansiDim        = "\x1b[2m"     // reduced intensity
	ansiBold       = "\x1b[1m"     // bold
	ansiReset      = "\x1b[0m"     // back to normal
	ansiHideCursor = "\x1b[?25l"   // hide the hardware cursor
	ansiShowCursor = "\x1b[?25h"   // show it again
	ansiEnterAlt   = "\x1b[?1049h" // switch to the alternate screen
	ansiExitAlt    = "\x1b[?1049l" // switch back, restoring the user's screen
)

// screen builds one frame in memory and writes it in a single call, so the
// terminal never shows a half-drawn update.
type screen struct {
	b     strings.Builder
	width int
}

// newScreen starts a frame, homing the cursor rather than clearing the whole
// display: overwriting the previous frame line by line avoids the flicker a
// full clear produces.
func newScreen(width int) *screen {
	s := &screen{width: width}
	s.b.WriteString(ansiHome)
	return s
}

// line writes one screen row.
//
// Callers truncate the plain text they pass in; line must not, because ANSI
// codes occupy no columns and would be miscounted as width.
func (s *screen) line(text string) {
	s.b.WriteString(text)
	s.b.WriteString(ansiClearLine)
	// Raw mode does not translate \n, so the \r is required.
	s.b.WriteString("\r\n")
}

// blank writes n empty rows, used to pad a frame so its footer stays put.
func (s *screen) blank(n int) {
	for range n {
		s.line("")
	}
}

// flush writes the finished frame, erasing anything left below it.
func (s *screen) flush(w io.Writer) error {
	s.b.WriteString(ansiClearBelow)
	_, err := io.WriteString(w, s.b.String())
	return err
}

// truncate shortens text to at most width display columns, marking a cut
// with an ellipsis. It counts runes rather than bytes so multi-byte
// characters are not split, and assumes text carries no ANSI codes.
func truncate(text string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= width {
		return text
	}
	if width == 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}
