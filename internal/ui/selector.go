// Package ui implements nav's interactive screens: the cheat selector, which
// filters a list as you type, and the prompts that fill in a command's
// `<variable>` placeholders.
//
// The package is split into a pure part and a terminal part. The views here
// only ever read keys from an io.Reader and write frames to an io.Writer, so
// the whole interaction can be driven from a test. Putting the real terminal
// into raw mode lives in tty.go.
package ui

import (
	"errors"
	"fmt"
	"io"
	"strings"

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

// Result reports what the user did in the selector.
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

// Run shows the selector and blocks until the user selects a cheat or quits.
func Run(cheats []cheat.Cheat, opts Options) (Result, error) {
	if opts.Input == nil || opts.Output == nil {
		return Result{}, errors.New("ui.Run: Input and Output are required")
	}
	return runSelector(newKeyReader(opts.Input), cheats, opts)
}

// runSelector is Run with the input reader supplied by the caller, so a
// session can share one reader across the list and the prompts that follow.
func runSelector(kr *keyReader, cheats []cheat.Cheat, opts Options) (Result, error) {
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

	act, err := runLoop(kr, s)
	if err != nil {
		return Result{}, err
	}
	if act == actionAccept {
		return Result{Cheat: s.matches[s.cursor], Selected: true, Copied: s.copied}, nil
	}
	return Result{Copied: s.copied}, nil
}

// handleKey applies one keypress to the selector.
func (s *selector) handleKey(k key) action {
	// Any keypress clears the previous transient message.
	s.status = ""

	switch k.kind {
	case keyQuit:
		return actionCancel

	case keyEnter:
		if len(s.matches) == 0 {
			s.status = "no matches"
			return actionContinue
		}
		return actionAccept

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
//
// The command is copied exactly as written, so any `<variable>` placeholders
// are copied too. Filling them in happens after a cheat is selected.
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
	s.cursor, s.offset = moveInList(s.cursor, s.offset, delta, len(s.matches), s.visibleEntries())
}

// visibleEntries is how many cheats fit on screen at the current size.
func (s *selector) visibleEntries() int {
	rows := s.size().Rows - reservedRows
	if rows < rowsPerEntry {
		return 1
	}
	return rows / rowsPerEntry
}

// render draws one frame.
func (s *selector) render() error {
	size := s.size()
	width := max(size.Cols, 20)

	sc := newScreen(width)

	const prompt = "Search: "
	sc.line(ansiBold + prompt + ansiReset + truncate(string(s.query), width-len(prompt)))
	sc.line("")

	visible := s.visibleEntries()
	end := min(s.offset+visible, len(s.matches))

	if len(s.matches) == 0 {
		sc.line(ansiDim + "  no cheats match" + ansiReset)
		sc.blank(visible*rowsPerEntry - 1)
	} else {
		for i := s.offset; i < end; i++ {
			c := s.matches[i]
			header := fmt.Sprintf("%s  %s", strings.Join(c.Tags, ","), c.Description)
			if i == s.cursor {
				// Truncate before adding colour so the escape codes, which
				// occupy no screen columns, are not counted as width.
				sc.line(ansiReverse + "> " + truncate(header, width-2) + ansiReset)
				sc.line(ansiBold + "    " + truncate(c.Command, width-4) + ansiReset)
			} else {
				sc.line("  " + truncate(header, width-2))
				sc.line(ansiDim + "    " + truncate(c.Command, width-4) + ansiReset)
			}
		}
		// Pad so the footer stays put as the match count changes.
		sc.blank((visible - (end - s.offset)) * rowsPerEntry)
	}

	sc.line("")
	sc.line(ansiDim + truncate(s.footer(), width) + ansiReset)

	return sc.flush(s.out)
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

// moveInList moves a highlight by delta within a list of n items, clamping at
// both ends, and scrolls a window of the given size to keep the highlight
// visible. It returns the new cursor and window offset.
//
// Both the cheat selector and the value prompt scroll this way, so the
// arithmetic lives in one place.
func moveInList(cursor, offset, delta, n, visible int) (int, int) {
	if n == 0 {
		return 0, 0
	}

	cursor = min(max(cursor+delta, 0), n-1)

	if cursor < offset {
		offset = cursor
	}
	if cursor >= offset+visible {
		offset = cursor - visible + 1
	}
	return cursor, max(offset, 0)
}
