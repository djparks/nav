// Package ui implements nav's interactive cheat selector: a list that
// filters as you type, moves with the arrow keys, and copies the highlighted
// command to the clipboard.
//
// The selector is split into a pure part and a terminal part. The model in
// this file only ever reads keys from an io.Reader and writes frames to an
// io.Writer, so the whole interaction can be driven from a test. Putting the
// real terminal into raw mode lives in tty.go.
package ui

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"nav/internal/cheat"
	"nav/internal/search"
	"nav/internal/term"
)

// rowsPerEntry is how many screen lines one cheat occupies: a header line
// with tags and description, then the command itself.
const rowsPerEntry = 2

// reservedRows is the chrome around the list: the search prompt, a blank
// line, a blank line, and the help/status footer.
const reservedRows = 4

// Result reports what the user did.
type Result struct {
	// Cheat is the highlighted cheat when Selected is true.
	Cheat cheat.Cheat
	// Selected is true when the user pressed Enter, false when they quit.
	Selected bool
	// Copied is true if the user copied at least one command.
	Copied bool
}

// Options configures a selector run.
type Options struct {
	// Input supplies raw keypress bytes. Required.
	Input io.Reader
	// Output receives rendered frames. Required.
	Output io.Writer
	// Query pre-fills the search box.
	Query string
	// Size reports the terminal size before each frame, so a window resize
	// is picked up. Defaults to term.FallbackSize.
	Size func() term.Size
	// Copy puts text on the clipboard. When nil, the copy key reports that
	// copying is unavailable instead of failing.
	Copy func(string) error
}

// selector is the mutable state of one interactive session.
type selector struct {
	all     []cheat.Cheat
	matches []cheat.Cheat
	query   []rune

	cursor int // index into matches of the highlighted entry
	offset int // index into matches of the first visible entry

	status string // transient message shown in the footer
	copied bool
	copy   func(string) error

	size func() term.Size
	out  io.Writer
}

// action is what the event loop should do after handling a key.
type action int

const (
	actionContinue action = iota
	actionSelect
	actionQuit
)

// escDelay is how long the selector waits for the rest of an escape
// sequence before concluding that the user pressed Esc on its own.
//
// Pressing Esc and pressing the up arrow both start with the same byte. The
// only thing that distinguishes them is that the arrow's remaining bytes
// follow immediately, so telling them apart needs a short wait. Terminals
// send the whole sequence in one burst, and no human can type a second key
// this fast, which makes the wait invisible in practice.
var escDelay = 50 * time.Millisecond

// readResult is one delivery from the input-reading goroutine.
type readResult struct {
	data []byte
	err  error
}

// Run shows the selector and blocks until the user selects a cheat or quits.
//
// Running out of input — a closed pipe, or the end of a scripted test input —
// is treated as quitting, not as an error.
func Run(cheats []cheat.Cheat, opts Options) (Result, error) {
	if opts.Input == nil || opts.Output == nil {
		return Result{}, errors.New("ui.Run: Input and Output are required")
	}

	size := opts.Size
	if size == nil {
		size = func() term.Size { return term.FallbackSize }
	}

	s := &selector{
		all:   cheats,
		query: []rune(opts.Query),
		copy:  opts.Copy,
		size:  size,
		out:   opts.Output,
	}
	s.refilter()

	if err := s.render(); err != nil {
		return Result{}, err
	}

	// Reading happens on its own goroutine so the main loop can put a
	// deadline on "is more of this escape sequence coming?". The goroutine
	// ends when the input reports an error, which happens when the caller
	// closes the terminal after Run returns.
	reads := make(chan readResult, 4)
	go readInput(opts.Input, reads)

	// apply feeds one key to the selector. done is true when Run should
	// return, carrying the result.
	apply := func(k key) (result Result, done bool, err error) {
		switch s.handle(k) {
		case actionSelect:
			return Result{Cheat: s.matches[s.cursor], Selected: true, Copied: s.copied}, true, nil
		case actionQuit:
			return Result{Copied: s.copied}, true, nil
		}
		if err := s.render(); err != nil {
			return Result{}, true, err
		}
		return Result{}, false, nil
	}

	// pending holds bytes that have arrived but not yet formed a whole key.
	var pending []byte
	var inputErr error

	for {
		// Handle every key the buffered bytes already contain.
		for {
			k, used, ok := decode(pending)
			if !ok {
				break
			}
			pending = pending[used:]
			if result, done, err := apply(k); done {
				return result, err
			}
		}

		// pending is now either empty or an incomplete sequence.
		if inputErr != nil {
			if len(pending) > 0 {
				// Nothing more is coming, so take the leftover at face value.
				k, used := decodeFinal(pending)
				pending = pending[used:]
				if result, done, err := apply(k); done {
					return result, err
				}
				continue
			}
			if errors.Is(inputErr, io.EOF) {
				return Result{Copied: s.copied}, nil
			}
			return Result{}, inputErr
		}

		var res readResult
		if len(pending) > 0 {
			// Waiting on the rest of a sequence: give up after escDelay and
			// take the bytes at face value.
			timer := time.NewTimer(escDelay)
			select {
			case res = <-reads:
				timer.Stop()
			case <-timer.C:
				k, used := decodeFinal(pending)
				pending = pending[used:]
				if result, done, err := apply(k); done {
					return result, err
				}
				continue
			}
		} else {
			res = <-reads
		}

		pending = append(pending, res.data...)
		if res.err != nil {
			inputErr = res.err
		}
	}
}

// readInput copies keypress bytes from r onto ch until r fails.
func readInput(r io.Reader, ch chan<- readResult) {
	for {
		buf := make([]byte, 64)
		n, err := r.Read(buf)
		if n > 0 {
			ch <- readResult{data: buf[:n]}
		}
		if err != nil {
			ch <- readResult{err: err}
			return
		}
	}
}

// handle applies one keypress to the selector.
func (s *selector) handle(k key) action {
	// Any keypress clears the previous transient message.
	s.status = ""

	switch k.kind {
	case keyQuit:
		return actionQuit

	case keyEnter:
		if len(s.matches) == 0 {
			s.status = "no matches"
			return actionContinue
		}
		return actionSelect

	case keyCopy:
		s.doCopy()

	case keyRune:
		s.query = append(s.query, k.r)
		s.refilter()

	case keyBackspace:
		if len(s.query) > 0 {
			s.query = s.query[:len(s.query)-1]
			s.refilter()
		}

	case keyClearQuery:
		if len(s.query) > 0 {
			s.query = s.query[:0]
			s.refilter()
		}

	case keyDeleteWord:
		s.deleteWord()

	case keyUp:
		s.moveCursor(-1)
	case keyDown:
		s.moveCursor(1)
	case keyPageUp:
		s.moveCursor(-s.visibleEntries())
	case keyPageDown:
		s.moveCursor(s.visibleEntries())
	case keyHome:
		s.moveCursor(-len(s.matches))
	case keyEnd:
		s.moveCursor(len(s.matches))
	}
	return actionContinue
}

// doCopy puts the highlighted command on the clipboard.
func (s *selector) doCopy() {
	if len(s.matches) == 0 {
		s.status = "nothing to copy"
		return
	}
	if s.copy == nil {
		s.status = "clipboard unavailable"
		return
	}
	if err := s.copy(s.matches[s.cursor].Command); err != nil {
		s.status = "copy failed: " + err.Error()
		return
	}
	s.copied = true
	s.status = "copied to clipboard"
}

// deleteWord removes the last whitespace-delimited word of the query.
func (s *selector) deleteWord() {
	i := len(s.query)
	for i > 0 && s.query[i-1] == ' ' {
		i--
	}
	for i > 0 && s.query[i-1] != ' ' {
		i--
	}
	if i != len(s.query) {
		s.query = s.query[:i]
		s.refilter()
	}
}

// refilter recomputes the match list and puts the cursor back at the top,
// which is what you want after the query changes.
func (s *selector) refilter() {
	s.matches = search.Filter(s.all, string(s.query))
	s.cursor = 0
	s.offset = 0
}

// moveCursor moves the highlight by delta entries, clamping at both ends,
// and scrolls the window to keep the highlight visible.
func (s *selector) moveCursor(delta int) {
	if len(s.matches) == 0 {
		return
	}

	s.cursor += delta
	if s.cursor < 0 {
		s.cursor = 0
	}
	if s.cursor > len(s.matches)-1 {
		s.cursor = len(s.matches) - 1
	}

	visible := s.visibleEntries()
	if s.cursor < s.offset {
		s.offset = s.cursor
	}
	if s.cursor >= s.offset+visible {
		s.offset = s.cursor - visible + 1
	}
	if s.offset < 0 {
		s.offset = 0
	}
}

// visibleEntries is how many cheats fit on screen at the current size.
func (s *selector) visibleEntries() int {
	rows := s.size().Rows - reservedRows
	if rows < rowsPerEntry {
		return 1
	}
	return rows / rowsPerEntry
}

// ANSI escape sequences. Kept as named constants so render stays readable.
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

// render draws one frame.
func (s *selector) render() error {
	size := s.size()
	width := size.Cols
	if width < 20 {
		width = 20
	}

	var b strings.Builder
	b.WriteString(ansiHome)

	// line writes one screen row. Callers truncate the plain text they pass
	// in; line itself must not, because ANSI codes occupy no columns and
	// would be miscounted as width.
	line := func(text string) {
		b.WriteString(text)
		b.WriteString(ansiClearLine)
		// Raw mode does not translate \n, so the \r is required.
		b.WriteString("\r\n")
	}

	const prompt = "Search: "
	line(ansiBold + prompt + ansiReset + truncate(string(s.query), width-len(prompt)))
	line("")

	visible := s.visibleEntries()
	end := min(s.offset+visible, len(s.matches))

	if len(s.matches) == 0 {
		line(ansiDim + "  no cheats match" + ansiReset)
		for i := 1; i < visible*rowsPerEntry; i++ {
			line("")
		}
	} else {
		for i := s.offset; i < end; i++ {
			c := s.matches[i]
			header := fmt.Sprintf("%s  %s", strings.Join(c.Tags, ","), c.Description)
			if i == s.cursor {
				// Truncate before adding colour so the escape codes, which
				// occupy no screen columns, are not counted as width.
				line(ansiReverse + "> " + truncate(header, width-2) + ansiReset)
				line(ansiBold + "    " + truncate(c.Command, width-4) + ansiReset)
			} else {
				line("  " + truncate(header, width-2))
				line(ansiDim + "    " + truncate(c.Command, width-4) + ansiReset)
			}
		}
		// Pad so the footer stays put as the match count changes.
		for i := end - s.offset; i < visible; i++ {
			line("")
			line("")
		}
	}

	line("")
	line(ansiDim + truncate(s.footer(), width) + ansiReset)

	b.WriteString(ansiClearBelow)
	_, err := io.WriteString(s.out, b.String())
	return err
}

// footer is the bottom line, as plain text: the match count plus either a
// transient status message or the key hints.
func (s *selector) footer() string {
	shown := 0
	if len(s.matches) > 0 {
		shown = s.cursor + 1
	}

	right := "↑↓ move   ⏎ select   ^Y copy   esc quit"
	if s.status != "" {
		right = s.status
	}
	return fmt.Sprintf("%d of %d matched   %s", shown, len(s.matches), right)
}

// truncate shortens text to at most width display columns, marking a cut
// with an ellipsis. It counts runes rather than bytes so multi-byte
// characters are not split, and ignores any ANSI codes already present.
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
